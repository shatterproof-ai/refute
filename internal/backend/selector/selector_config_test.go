package selector

import (
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/config"
)

// TestForFile_TSMorphConfigAdapterPathThreaded verifies that ForFile passes
// both the workspace root and the tools.tsmorph.adapter config value to the
// availability check so the caller can override the adapter location.
func TestForFile_TSMorphConfigAdapterPathThreaded(t *testing.T) {
	dir := t.TempDir()

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.Tools.TSMorph.Adapter = "/explicit/rename.cjs"

	var gotWorkspaceRoot, gotExplicitPath string
	oldAvailable := tsMorphAvailable
	oldNew := newTSMorphBackend
	tsMorphAvailable = func(workspaceRoot, explicitPath string) bool {
		gotWorkspaceRoot = workspaceRoot
		gotExplicitPath = explicitPath
		return true
	}
	newTSMorphBackend = func(_ string) backend.RefactoringBackend { return fakeBackend{} }
	t.Cleanup(func() {
		tsMorphAvailable = oldAvailable
		newTSMorphBackend = oldNew
	})

	if _, err := ForFile(cfg, dir, filepath.Join(dir, "src", "app.ts")); err != nil {
		t.Fatalf("ForFile: %v", err)
	}

	if gotWorkspaceRoot != dir {
		t.Errorf("workspaceRoot passed = %q, want %q", gotWorkspaceRoot, dir)
	}
	if gotExplicitPath != "/explicit/rename.cjs" {
		t.Errorf("explicitPath passed = %q, want %q", gotExplicitPath, "/explicit/rename.cjs")
	}
}
