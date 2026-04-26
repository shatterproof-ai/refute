package selector_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/config"
)

func TestForFile_TypeScriptTSXUsesLocalServer(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules bin: %v", err)
	}

	serverName := "typescript-language-server"
	if runtime.GOOS == "windows" {
		serverName += ".cmd"
	}
	localServer := filepath.Join(binDir, serverName)
	if err := os.WriteFile(localServer, []byte(""), 0o755); err != nil {
		t.Fatalf("write local typescript-language-server: %v", err)
	}

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	sel, err := selector.ForFile(cfg, dir, filepath.Join(dir, "src", "badge.tsx"))
	if err != nil {
		t.Fatalf("ForFile returned error: %v", err)
	}

	if sel.Language != "typescript" {
		t.Fatalf("Language: got %q, want %q", sel.Language, "typescript")
	}
	if sel.LanguageID != "typescriptreact" {
		t.Fatalf("LanguageID: got %q, want %q", sel.LanguageID, "typescriptreact")
	}
	if sel.Server.Command != localServer {
		t.Fatalf("Server.Command: got %q, want %q", sel.Server.Command, localServer)
	}
	if _, ok := sel.Backend.(*lsp.Adapter); !ok {
		t.Fatalf("Backend type: got %T, want *lsp.Adapter", sel.Backend)
	}
}

func TestForFile_JavaScriptJSXUsesBuiltinServer(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	sel, err := selector.ForFile(cfg, "/nonexistent/workspace/root", "/tmp/screen.jsx")
	if err != nil {
		t.Fatalf("ForFile returned error: %v", err)
	}

	if sel.Language != "javascript" {
		t.Fatalf("Language: got %q, want %q", sel.Language, "javascript")
	}
	if sel.LanguageID != "javascriptreact" {
		t.Fatalf("LanguageID: got %q, want %q", sel.LanguageID, "javascriptreact")
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

	_, err = selector.ForFile(cfg, "/nonexistent/workspace/root", "/tmp/file.unknown")
	if err == nil {
		t.Fatal("expected error for unknown extension")
	}
	if !strings.Contains(err.Error(), `no server configured for language ""`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
