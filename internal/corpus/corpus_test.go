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

	"github.com/shatterproof-ai/refute/internal/testutil"
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
	refuteBin := testutil.BuildRefute(t, root)

	for _, tgt := range m.Targets {
		t.Run(tgt.Name, func(t *testing.T) {
			requireBackend(t, tgt)
			projectDir := materialize(t, root, m.CacheDir, tgt)
			before := snapshotNewNameCounts(t, projectDir, tgt)
			applyRename(t, refuteBin, projectDir, tgt)
			assertRenamed(t, projectDir, tgt, before)
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
	testutil.CopyDir(t, src, dst)
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

// snapshotNewNameCounts records, for every file the rename is expected to touch,
// how many times the new identifier already appears before the rename runs. This
// baseline is what lets assertRenamed demand a strict increase rather than mere
// presence (see its doc comment for why presence alone is too weak). Files that
// do not yet exist count as zero. The snapshot must be taken after materialize
// and before applyRename, while projectDir still holds the original sources.
func snapshotNewNameCounts(t *testing.T, projectDir string, tgt target) map[string]int {
	t.Helper()
	counts := make(map[string]int, len(tgt.ExpectRenamed))
	for _, rel := range tgt.ExpectRenamed {
		data, err := os.ReadFile(filepath.Join(projectDir, rel))
		if err != nil {
			if os.IsNotExist(err) {
				counts[rel] = 0
				continue
			}
			t.Fatalf("snapshotting expected-renamed file %s for %s: %v", rel, tgt.Name, err)
		}
		counts[rel] = strings.Count(string(data), tgt.Rename.NewName)
	}
	return counts
}

// assertRenamed checks that the rename strictly increased the number of new-name
// occurrences in every file it was expected to touch, using the pre-rename
// baseline captured by snapshotNewNameCounts. Requiring an increase — rather than
// mere presence — is what makes this a real propagation gate: a backend that
// no-ops a file leaves its new-name count unchanged and now fails here.
//
// This specifically closes the substring/pre-existing false positive that a bare
// strings.Contains check suffers. When the new name contains the old name
// (rust-itoa renames Buffer -> ItoaBuffer) a no-op file may still literally
// contain "ItoaBuffer" in a comment, and an unrelated file may already contain
// the new name for other reasons; in both cases Contains passes green even
// though nothing was rewritten. A strict count increase does not.
//
// What this still does NOT guarantee: that the increase came from renaming the
// intended symbol rather than some incidental text rewrite, nor that the old
// identifier is gone — common tokens legitimately survive in unrelated
// references and doc comments, and when the old name is a substring of the new
// name (Buffer inside ItoaBuffer) its literal count need not drop at all. The
// old name is therefore intentionally not asserted absent; the build/typecheck
// verify below remains the real semantic correctness gate.
func assertRenamed(t *testing.T, projectDir string, tgt target, before map[string]int) {
	t.Helper()
	for _, rel := range tgt.ExpectRenamed {
		path := filepath.Join(projectDir, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading expected-renamed file %s for %s: %v", rel, tgt.Name, err)
		}
		after := strings.Count(string(data), tgt.Rename.NewName)
		if after <= before[rel] {
			t.Fatalf("%s: %s new-name %q count did not increase after rename (before=%d, after=%d); the backend did not propagate the edit there",
				tgt.Name, rel, tgt.Rename.NewName, before[rel], after)
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
