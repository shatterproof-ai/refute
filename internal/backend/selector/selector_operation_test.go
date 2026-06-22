package selector

import (
	"context"
	"errors"
	"testing"

	"github.com/shatterproof-ai/refute/internal/config"
)

// TestCandidatesFor_GoSupportsExtract verifies the Go ladder is a single LSP
// rung that supports extract-function.
func TestCandidatesFor_GoSupportsExtract(t *testing.T) {
	cands, err := CandidatesFor(context.Background(), &config.Config{}, "/ws", "/ws/main.go", "extract-function")
	if err != nil {
		t.Fatalf("CandidatesFor: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("expected 1 candidate, got %d: %+v", len(cands), cands)
	}
	if cands[0].BackendName != "lsp" {
		t.Errorf("backend = %q, want lsp", cands[0].BackendName)
	}
	if !cands[0].Supported {
		t.Errorf("go lsp should support extract-function")
	}
}

// TestSelectForOperation_TypeScriptExtractRefused verifies that, with no
// ts-morph adapter available, the TypeScript ladder falls through to the LSP
// server (rename-only) and an extract request is refused explicitly rather than
// attempted.
func TestSelectForOperation_TypeScriptExtractRefused(t *testing.T) {
	// /ws does not exist, so tsMorphAvailable is false; the only rung is the
	// typescript-language-server LSP backend, which is rename-only.
	_, err := SelectForOperation(context.Background(), &config.Config{}, "/ws", "/ws/app.ts", "extract-function")
	if err == nil {
		t.Fatal("expected extract-function on TypeScript to be refused")
	}
	if !errors.Is(err, ErrOperationUnsupported) {
		t.Fatalf("expected ErrOperationUnsupported, got %v", err)
	}
}

// TestSelectForOperation_RenameSupported verifies rename resolves to the LSP
// backend for a rename-only language when no ts-morph adapter is present.
// tsMorphAvailable is mocked to false because the dev-fallback path in
// resolveAdapterPaths (step 4: repo-relative via runtime.Caller) finds the
// built adapter regardless of workspaceRoot, causing the test to return
// "tsmorph" instead of "lsp" when the adapter is installed locally (#128).
func TestSelectForOperation_RenameSupported(t *testing.T) {
	oldAvail := tsMorphAvailable
	tsMorphAvailable = func(_, _ string) bool { return false }
	t.Cleanup(func() { tsMorphAvailable = oldAvail })

	sel, err := SelectForOperation(context.Background(), &config.Config{}, "/ws", "/ws/app.ts", "rename")
	if err != nil {
		t.Fatalf("SelectForOperation(rename): %v", err)
	}
	if sel.BackendName != "lsp" {
		t.Errorf("backend = %q, want lsp", sel.BackendName)
	}
	if !backendSupports(sel.Backend, "rename") {
		t.Errorf("selected backend should support rename")
	}
}

// TestCandidatesFor_TypeScriptPrefersTSMorphThenLSP verifies the ladder order:
// when ts-morph is available it is the preferred rung, with the LSP server
// behind it as a fall-through.
func TestCandidatesFor_TypeScriptPrefersTSMorphThenLSP(t *testing.T) {
	oldAvail := tsMorphAvailable
	tsMorphAvailable = func(_, _ string) bool { return true }
	t.Cleanup(func() { tsMorphAvailable = oldAvail })

	cands, err := CandidatesFor(context.Background(), &config.Config{}, "/ws", "/ws/app.ts", "rename")
	if err != nil {
		t.Fatalf("CandidatesFor: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates (tsmorph, lsp), got %d: %+v", len(cands), cands)
	}
	if cands[0].BackendName != "tsmorph" || cands[1].BackendName != "lsp" {
		t.Fatalf("ladder order = [%q, %q], want [tsmorph, lsp]", cands[0].BackendName, cands[1].BackendName)
	}
	// rename is supported by the preferred ts-morph rung.
	if !cands[0].Supported {
		t.Errorf("tsmorph should support rename")
	}
}
