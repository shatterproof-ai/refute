//go:build corpus

// Package corpus_test drives refute refactorings against pinned real-world
// repositories (issue #96). It is guarded by the `corpus` build tag so it never
// runs as part of `go test ./...`; the lane is network-dependent (it fetches
// upstream repositories) and is exercised explicitly via `make corpus` or
// `go test -tags corpus ./internal/corpus/`.
//
// Each target in testdata/corpus/manifest.json pins a repository at a fixed
// commit, applies one rename through the refute CLI, asserts the rename
// propagated, and runs the project's own build/typecheck/test as the ground
// truth that the edit kept the project valid. A target whose backend or verify
// toolchain is absent skips with an explicit reason rather than failing, so the
// lane is reproducible on partial toolchains while still catching backend drift
// where the tools exist.
package corpus_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// allowNetworkVerifyEnv opts in to verify steps that need to install
// dependencies (npm ci, mvn compile). Materialization always needs the network;
// these steps additionally need a package registry, so they are off by default
// even in the corpus lane.
const allowNetworkVerifyEnv = "REFUTE_CORPUS_ALLOW_NETWORK_VERIFY"

type manifest struct {
	CacheDir string   `json:"cacheDir"`
	Targets  []target `json:"targets"`
}

type target struct {
	Name          string       `json:"name"`
	Language      string       `json:"language"`
	Description   string       `json:"description"`
	Repo          string       `json:"repo"`
	Commit        string       `json:"commit"`
	Subdir        string       `json:"subdir"`
	BackendTool   string       `json:"backendTool"`
	BackendEnv    string       `json:"backendEnv"`
	Rename        renameSpec   `json:"rename"`
	ExpectRenamed []string     `json:"expectRenamed"`
	Verify        []verifyStep `json:"verify"`
}

type renameSpec struct {
	Command string `json:"command"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Name    string `json:"name"`
	NewName string `json:"newName"`
}

type verifyStep struct {
	Cmd       []string `json:"cmd"`
	NeedsTool string   `json:"needsTool"`
	Network   bool     `json:"network"`
}

func TestCorpus(t *testing.T) {
	root := repoRoot(t)
	m := loadManifest(t, filepath.Join(root, "testdata", "corpus", "manifest.json"))
	refuteBin := buildRefute(t, root)

	for _, tgt := range m.Targets {
		t.Run(tgt.Name, func(t *testing.T) {
			requireBackend(t, tgt)
			projectDir := materialize(t, root, m.CacheDir, tgt)
			applyRename(t, refuteBin, projectDir, tgt)
			assertRenamed(t, projectDir, tgt)
			runVerify(t, projectDir, tgt)
		})
	}
}

// requireBackend skips the target when any refactoring backend it needs is not
// available. A target may declare more than one prerequisite — a required
// environment variable pointing at a built adapter (backendEnv, used by the
// OpenRewrite Java path) and/or a PATH binary (backendTool) — and every declared
// prerequisite must be satisfied. When several are missing the skip reason names
// all of them so the gap is obvious in one pass.
func requireBackend(t *testing.T, tgt target) {
	t.Helper()
	if reason := backendSkipReason(tgt); reason != "" {
		t.Skip(reason)
	}
}

// backendSkipReason returns a human-readable reason to skip tgt when one or more
// of its declared backends is unavailable, or "" when every declared backend is
// present. It checks all declared backends (backendEnv and backendTool),
// accumulating a combined reason rather than stopping at the first one.
func backendSkipReason(tgt target) string {
	var missing []string
	if tgt.BackendEnv != "" && os.Getenv(tgt.BackendEnv) == "" {
		missing = append(missing, fmt.Sprintf("set %s to the built adapter", tgt.BackendEnv))
	}
	if tgt.BackendTool != "" {
		if _, err := exec.LookPath(tgt.BackendTool); err != nil {
			missing = append(missing, fmt.Sprintf("%q not on PATH", tgt.BackendTool))
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("%s backend unavailable for %s target: %s",
		tgt.Language, tgt.Name, strings.Join(missing, "; "))
}

// materialize fetches the target at its pinned commit (via the shared
// corpus-fetch.sh, which caches across runs) and copies the relevant subtree to
// an isolated temp directory so the rename never mutates the cache. A fetch
// failure skips rather than fails: the network is an environment precondition of
// the lane, not the backend behaviour under test. The fetch output is included
// so a stale pin is still debuggable.
func materialize(t *testing.T, root, cacheDir string, tgt target) string {
	t.Helper()
	fetch := exec.Command(filepath.Join(root, "scripts", "corpus-fetch.sh"), tgt.Name)
	fetch.Dir = root
	if out, err := fetch.CombinedOutput(); err != nil {
		t.Skipf("corpus-fetch failed for %s (network or pin issue), skipping:\n%s", tgt.Name, out)
	}

	src := filepath.Join(root, cacheDir, tgt.Name)
	if tgt.Subdir != "" && tgt.Subdir != "." {
		src = filepath.Join(src, tgt.Subdir)
	}
	dst := t.TempDir()
	copyTree(t, src, dst)
	return dst
}

// applyRename runs the configured refute rename against the materialized copy.
func applyRename(t *testing.T, refuteBin, projectDir string, tgt target) {
	t.Helper()
	r := tgt.Rename
	args := []string{
		r.Command,
		"--file", r.File,
		"--line", strconv.Itoa(r.Line),
		"--name", r.Name,
		"--new-name", r.NewName,
	}
	cmd := exec.Command(refuteBin, args...)
	cmd.Dir = projectDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("refute %s failed for %s (%s @ %s):\nargs: %v\n%s",
			r.Command, tgt.Name, tgt.Repo, tgt.Commit, args, out)
	}
}

// assertRenamed checks that the new identifier is present in every file the
// rename was expected to touch — proving cross-file propagation reached each
// one. The old identifier is intentionally not asserted absent: common tokens
// (e.g. "String") legitimately survive in unrelated references and doc
// comments, so the build/typecheck verify below is the real correctness gate.
func assertRenamed(t *testing.T, projectDir string, tgt target) {
	t.Helper()
	for _, rel := range tgt.ExpectRenamed {
		path := filepath.Join(projectDir, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading expected-renamed file %s for %s: %v", rel, tgt.Name, err)
		}
		if !strings.Contains(string(data), tgt.Rename.NewName) {
			t.Fatalf("%s: %s does not contain new name %q after rename; the backend did not propagate the edit there",
				tgt.Name, rel, tgt.Rename.NewName)
		}
	}
}

// runVerify runs each verify command in the materialized project. Steps whose
// tool is missing, or which need the network without opt-in, are skipped with a
// log line. A failing step is fatal and dumps full context to debug drift.
func runVerify(t *testing.T, projectDir string, tgt target) {
	t.Helper()
	ran := 0
	for _, step := range tgt.Verify {
		if len(step.Cmd) == 0 {
			continue
		}
		if step.NeedsTool != "" {
			if _, err := exec.LookPath(step.NeedsTool); err != nil {
				t.Logf("%s: skipping verify %v (%q not on PATH)", tgt.Name, step.Cmd, step.NeedsTool)
				continue
			}
		}
		if step.Network && os.Getenv(allowNetworkVerifyEnv) == "" {
			t.Logf("%s: skipping network verify %v (set %s=1 to enable)", tgt.Name, step.Cmd, allowNetworkVerifyEnv)
			continue
		}
		cmd := exec.Command(step.Cmd[0], step.Cmd[1:]...)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("verify %v failed for %s (%s @ %s) after rename %s->%s:\n%s",
				step.Cmd, tgt.Name, tgt.Repo, tgt.Commit, tgt.Rename.Name, tgt.Rename.NewName, out)
		}
		ran++
	}
	if ran == 0 {
		t.Logf("%s: rename applied and propagated; all verify steps skipped (no toolchain)", tgt.Name)
	}
}

func loadManifest(t *testing.T, path string) manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading corpus manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing corpus manifest: %v", err)
	}
	if len(m.Targets) == 0 {
		t.Fatal("corpus manifest has no targets")
	}
	return m
}

// buildRefute compiles the refute CLI once and returns the binary path.
func buildRefute(t *testing.T, root string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/refute")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building refute binary:\n%s", out)
	}
	return bin
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) above %s", dir)
		}
		dir = parent
	}
}

// copyTree recursively copies src into dst, skipping any .git directory so the
// materialized copy is a clean working tree.
func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copying corpus target %s -> %s: %v", src, dst, err)
	}
}
