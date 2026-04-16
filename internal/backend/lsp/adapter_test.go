package lsp_test

import (
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestAdapter_Rename(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	cfg := config.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
	}
	adapter := lsp.NewAdapter(cfg, "go", []string{"**/*.go"})

	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer func() {
		if err := adapter.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	// main.go line 3 column 6 (1-indexed) = "oldName" declaration.
	// Content: `func oldName() {}`
	// Line 3 (1-indexed), column 6 = 'o' in oldName (after "func ").
	loc := symbol.Location{
		File:   filepath.Join(dir, "main.go"),
		Line:   3,
		Column: 6,
		Name:   "oldName",
		Kind:   symbol.KindFunction,
	}

	we, err := adapter.Rename(loc, "newName")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we == nil {
		t.Fatal("expected non-nil WorkspaceEdit")
	}

	totalEdits := 0
	for _, fe := range we.FileEdits {
		totalEdits += len(fe.Edits)
	}

	if totalEdits < 2 {
		t.Errorf("expected at least 2 edits (declaration + call site), got %d", totalEdits)
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	cfg := config.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
	}
	adapter := lsp.NewAdapter(cfg, "go", []string{"**/*.go"})
	caps := adapter.Capabilities()

	found := false
	for _, cap := range caps {
		if cap.Operation == "rename" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'rename' in capabilities, got %v", caps)
	}
}
