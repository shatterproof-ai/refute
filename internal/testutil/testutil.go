// Package testutil holds helpers shared across refute's build-tag-gated test
// lanes — the integration lane (//go:build integration) and the corpus lane
// (//go:build corpus). It carries no build tag of its own so test files in
// either lane can import it; it is only ever referenced from _test.go files, so
// it never enters a production build despite importing "testing".
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BuildRefute compiles the refute CLI into a temp dir and returns the binary
// path. repoRoot is the working directory for `go build` and must contain the
// module's go.mod (e.g. "." from the repo root, ".." from internal/).
func BuildRefute(t *testing.T, repoRoot string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/refute")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building refute binary:\n%s", out)
	}
	return bin
}

// CopyDir recursively copies the directory tree rooted at src into dst, creating
// dst as needed. It skips version-control and dependency directories (.git,
// node_modules) so the result is a clean working tree, and copies only regular
// files: symlinks, devices, and other irregular entries are skipped rather than
// silently materialized as regular files. File permission bits are preserved.
func CopyDir(t *testing.T, src, dst string) {
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
			if rel != "." && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copying tree %s -> %s: %v", src, dst, err)
	}
}
