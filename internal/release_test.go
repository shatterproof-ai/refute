//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestReleaseScriptDefaultDistDir guards against a regression where
// scripts/release.sh silently failed under CI-realistic conditions: no
// DIST_DIR override, so npm-package staging directories are passed to `npm
// pack` as bare relative paths (e.g. "dist/ts-adapter-package"). npm's
// argument parser treats a bare "word/word" spec as a GitHub "owner/repo"
// shorthand rather than a local directory, so the build failed with exit
// 128 and no diagnostic output (issue #134). The fix must keep working when
// DIST_DIR is left at its default relative value.
//
// This exercises the real Go/npm release path shipped for v0.1 (not an
// experimental language backend), so it skips only on a missing npm/javac
// rather than gating behind REFUTE_EXPERIMENTAL_INTEGRATION like the
// backend-integration tests in this package. It builds real cross-platform
// archives via release.sh end to end, so it runs on the order of tens of
// seconds, heavier than the other fixtures in this package.
func TestReleaseScriptDefaultDistDir(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not on PATH")
	}
	if _, err := exec.LookPath("javac"); err != nil {
		t.Skip("javac not on PATH")
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	worktree := t.TempDir()
	// Remove the dir so `git worktree add` can create it fresh.
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatalf("clear worktree dir: %v", err)
	}
	addCmd := exec.Command("git", "worktree", "add", "--detach", worktree, "HEAD")
	addCmd.Dir = repoRoot
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		rmCmd := exec.Command("git", "worktree", "remove", "--force", worktree)
		rmCmd.Dir = repoRoot
		if out, err := rmCmd.CombinedOutput(); err != nil {
			t.Logf("git worktree remove %s: %v\n%s", worktree, err, out)
		}
	})

	cmd := exec.Command("./scripts/release.sh", "v0.0.0-releasetest")
	cmd.Dir = worktree
	// Intentionally scrub DIST_DIR so the script falls back to its default
	// relative "dist", the case that broke in CI.
	cmd.Env = filterEnv(os.Environ(), "DIST_DIR")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release.sh with default DIST_DIR failed: %v\n%s", err, out)
	}

	adapterTarball := filepath.Join(worktree, "dist", "refute-ts-adapter-0.0.0-releasetest.tgz")
	if _, err := os.Stat(adapterTarball); err != nil {
		t.Errorf("expected ts-adapter tarball at %s: %v", adapterTarball, err)
	}
}

func filterEnv(env []string, dropKey string) []string {
	prefix := dropKey + "="
	filtered := make([]string, 0, len(env))
	for _, kv := range env {
		if len(kv) >= len(prefix) && kv[:len(prefix)] == prefix {
			continue
		}
		filtered = append(filtered, kv)
	}
	return filtered
}
