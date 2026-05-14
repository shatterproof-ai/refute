package selector

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type fakeBackend struct{}

func (fakeBackend) Initialize(string) error { return nil }
func (fakeBackend) Shutdown() error         { return nil }
func (fakeBackend) FindSymbol(symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) Rename(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) ExtractFunction(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) ExtractVariable(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) InlineSymbol(symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) MoveToFile(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (fakeBackend) Capabilities() []backend.Capability { return nil }

func TestForFile_TypeScriptTSXPrefersTSMorphWhenAvailable(t *testing.T) {
	dir := t.TempDir()

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	oldAvailable := tsMorphAvailable
	oldNew := newTSMorphBackend
	tsMorphAvailable = func(_, _ string) bool { return true }
	newTSMorphBackend = func(_ string) backend.RefactoringBackend { return fakeBackend{} }
	t.Cleanup(func() {
		tsMorphAvailable = oldAvailable
		newTSMorphBackend = oldNew
	})

	sel, err := ForFile(cfg, dir, filepath.Join(dir, "src", "badge.tsx"))
	if err != nil {
		t.Fatalf("ForFile returned error: %v", err)
	}

	if sel.Language != "typescript" {
		t.Fatalf("Language: got %q, want %q", sel.Language, "typescript")
	}
	if sel.LanguageID != "typescriptreact" {
		t.Fatalf("LanguageID: got %q, want %q", sel.LanguageID, "typescriptreact")
	}
	if sel.BackendName != "tsmorph" {
		t.Fatalf("BackendName: got %q, want %q", sel.BackendName, "tsmorph")
	}
	if _, ok := sel.Backend.(fakeBackend); !ok {
		t.Fatalf("Backend type: got %T, want fakeBackend", sel.Backend)
	}
}

func TestForFile_JavaScriptJSXUsesBuiltinServer(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	oldAvailable := tsMorphAvailable
	tsMorphAvailable = func(_, _ string) bool { return false }
	t.Cleanup(func() {
		tsMorphAvailable = oldAvailable
	})

	sel, err := ForFile(cfg, "/nonexistent/workspace/root", "/tmp/screen.jsx")
	if err != nil {
		t.Fatalf("ForFile returned error: %v", err)
	}

	if sel.Language != "javascript" {
		t.Fatalf("Language: got %q, want %q", sel.Language, "javascript")
	}
	if sel.LanguageID != "javascriptreact" {
		t.Fatalf("LanguageID: got %q, want %q", sel.LanguageID, "javascriptreact")
	}
	if sel.BackendName != "lsp" {
		t.Fatalf("BackendName: got %q, want %q", sel.BackendName, "lsp")
	}
	if sel.Server.Command != "typescript-language-server" {
		t.Fatalf("Server.Command: got %q, want %q", sel.Server.Command, "typescript-language-server")
	}
	if _, ok := sel.Backend.(*lsp.Adapter); !ok {
		t.Fatalf("Backend type: got %T, want *lsp.Adapter", sel.Backend)
	}
}

func TestForFile_UnknownExtensionReturnsNoServerConfigured(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	_, err = ForFile(cfg, "/nonexistent/workspace/root", "/tmp/file.unknown")
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
	if !strings.Contains(err.Error(), `no server configured for language ""`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
