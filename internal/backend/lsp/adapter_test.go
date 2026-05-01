package lsp_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
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

func TestAdapter_FindSymbol_barename(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	if err := adapter.DidOpen(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	locs, err := adapter.FindSymbol(symbol.Query{
		QualifiedName: "oldName",
		Kind:          symbol.KindFunction,
	})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one location for oldName")
	}
	for _, l := range locs {
		if l.Name != "oldName" {
			t.Errorf("unexpected name in results: %s", l.Name)
		}
	}
}

func TestAdapter_FindSymbol_notFound(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	if err := adapter.DidOpen(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	_, err := adapter.FindSymbol(symbol.Query{QualifiedName: "doesNotExist"})
	if !errors.Is(err, backend.ErrSymbolNotFound) {
		t.Errorf("expected ErrSymbolNotFound, got: %v", err)
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

func TestByteColumnToUTF16Character(t *testing.T) {
	line := `const label = "é𝄞"; target := 1`
	byteColumn := strings.Index(line, "target") + 1
	got, err := lsp.ByteColumnToUTF16CharacterForTest(line, byteColumn)
	if err != nil {
		t.Fatalf("ByteColumnToUTF16CharacterForTest: %v", err)
	}
	want := 23
	if got != want {
		t.Fatalf("expected UTF-16 character %d, got %d", want, got)
	}
}

func TestAdapter_ExtractFunction_honorsName(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Gopls only returns refactor.extract.function for multi-statement
	// selections (not for single constant expressions like `1 + 2`, which
	// yield refactor.extract.constant instead).
	mainSrc := `package main

func main() {
	x := 10
	println(x)
	println(x+1)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	// LSP 0-indexed range [3,1]-[5,13] covers the three statements in main's
	// body; SourceRange uses 1-indexed line/col.
	r := symbol.SourceRange{
		File:      filepath.Join(dir, "main.go"),
		StartLine: 4, StartCol: 2,
		EndLine: 6, EndCol: 14,
	}
	we, err := adapter.ExtractFunction(r, "sum")
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits")
	}
	saw := false
	for _, fe := range we.FileEdits {
		for _, te := range fe.Edits {
			if strings.Contains(te.NewText, "sum") {
				saw = true
			}
		}
	}
	if !saw {
		t.Errorf("expected extracted name 'sum' to appear in edits, got: %+v", we)
	}
}

func TestAdapter_ExtractVariable(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Gopls returns refactor.extract.variable for a function-call result,
	// but refactor.extract.constant for a constant expression like `1 + 2`.
	mainSrc := `package main

func src() int { return 42 }

func main() {
	println(src())
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	// LSP 0-indexed range [5,9]-[5,14] selects the `src()` call inside println.
	r := symbol.SourceRange{
		File:      filepath.Join(dir, "main.go"),
		StartLine: 6, StartCol: 10,
		EndLine: 6, EndCol: 15,
	}
	we, err := adapter.ExtractVariable(r, "")
	if err != nil {
		t.Fatalf("ExtractVariable: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits")
	}
}

func TestAdapter_InlineSymbol(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func add(a, b int) int { return a + b }

func main() {
	println(add(1, 2))
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	// main.go line 6 col 10 (1-indexed) points at 'add' inside println(add(1, 2)).
	loc := symbol.Location{
		File:   filepath.Join(dir, "main.go"),
		Line:   6, Column: 10,
		Name: "add",
		Kind: symbol.KindFunction,
	}
	we, err := adapter.InlineSymbol(loc)
	if err != nil {
		if errors.Is(err, backend.ErrUnsupported) {
			t.Skip("inline not supported by this gopls version")
		}
		t.Fatalf("InlineSymbol: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits from inline")
	}
}

func TestReplaceWholeIdent_respectsIdentifierBoundaries(t *testing.T) {
	got := lsp.ReplaceWholeIdentForTest("newFunction()\nnewFunctionCall()\n_ = newFunction", "newFunction", "sum")
	want := "sum()\nnewFunctionCall()\n_ = sum"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
