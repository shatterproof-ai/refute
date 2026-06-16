package tsmorph_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
)

// makeFakeAdapter creates a minimal fake adapter layout in dir/node_modules/@shatterproof-ai/refute-ts-adapter.
// If hoistTsMorph is true, ts-morph goes at dir/node_modules/ts-morph (hoisted).
func makeFakeAdapter(t *testing.T, dir string, hoistTsMorph bool) {
	t.Helper()
	pkgDir := filepath.Join(dir, "node_modules", "@shatterproof-ai", "refute-ts-adapter")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkgDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "rename.cjs"), []byte("// fake"), 0o644); err != nil {
		t.Fatalf("write rename.cjs: %v", err)
	}
	var tsMorphDir string
	if hoistTsMorph {
		tsMorphDir = filepath.Join(dir, "node_modules", "ts-morph")
	} else {
		tsMorphDir = filepath.Join(pkgDir, "node_modules", "ts-morph")
	}
	if err := os.MkdirAll(tsMorphDir, 0o755); err != nil {
		t.Fatalf("mkdir ts-morph: %v", err)
	}
}

func TestAvailableAt_WorkspaceNodeModules(t *testing.T) {
	requireNode(t)

	dir := t.TempDir()
	makeFakeAdapter(t, dir, false)
	if !tsmorph.AvailableAt(dir, "") {
		t.Error("AvailableAt should return true when workspace contains npm package with bundled ts-morph")
	}
}

func TestAvailableAt_WorkspaceNodeModulesHoistedTsMorph(t *testing.T) {
	requireNode(t)

	dir := t.TempDir()
	makeFakeAdapter(t, dir, true)
	if !tsmorph.AvailableAt(dir, "") {
		t.Error("AvailableAt should return true when workspace has adapter and ts-morph is hoisted")
	}
}

func TestAvailableAt_ExplicitAdapterPath(t *testing.T) {
	requireNode(t)

	dir := t.TempDir()
	// Place the script directly in dir; ts-morph adjacent.
	scriptPath := filepath.Join(dir, "rename.cjs")
	if err := os.WriteFile(scriptPath, []byte("// fake"), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	tsMorphDir := filepath.Join(dir, "node_modules", "ts-morph")
	if err := os.MkdirAll(tsMorphDir, 0o755); err != nil {
		t.Fatalf("mkdir ts-morph: %v", err)
	}
	if !tsmorph.AvailableAt("", scriptPath) {
		t.Error("AvailableAt should return true when an explicit adapter path is given and ts-morph is adjacent")
	}
}

func TestAvailableAt_EmptyWorkspaceReturnsFalseWithoutRepoPaths(t *testing.T) {
	dir := t.TempDir()
	// Empty temp dir: no adapter anywhere. Repo-relative paths may still match
	// when tests run from within the repo — skip the assertion in that case.
	if tsmorph.Available() {
		t.Skip("repo-relative adapter present; skipping empty-workspace check")
	}
	if tsmorph.AvailableAt(dir, "") {
		t.Error("AvailableAt should return false when workspace is empty and no explicit path given")
	}
}

func TestInitializeMissingAdapterReportsReleaseInstallHint(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := tsmorph.NewAdapter().Initialize(t.TempDir())
	if err == nil {
		t.Fatal("Initialize succeeded with node absent, want missing adapter error")
	}

	const wantHint = "npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz"
	if !strings.Contains(err.Error(), wantHint) {
		t.Fatalf("Initialize error = %q, want install hint %q", err, wantHint)
	}
	if strings.Contains(err.Error(), "npm install -g @shatterproof-ai/refute-ts-adapter") {
		t.Fatalf("Initialize error still references nonexistent npm registry package: %q", err)
	}
}
