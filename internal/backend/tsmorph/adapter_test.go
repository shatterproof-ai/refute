package tsmorph_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestAdapterRename(t *testing.T) {
	if !tsmorph.Available() {
		t.Skip("ts-morph backend not installed")
	}

	srcDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "typescript", "rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkNodeModules(t, srcDir, dir)

	adapter := tsmorph.NewAdapter()
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	greeterFile := filepath.Join(dir, "src", "greeter.ts")
	we, err := adapter.Rename(symbol.Location{
		File:   greeterFile,
		Line:   1,
		Column: 17,
		Name:   "greet",
		Kind:   symbol.KindFunction,
	}, "welcome")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if we == nil {
		t.Fatal("expected non-nil WorkspaceEdit")
	}
	if len(we.FileEdits) < 2 {
		t.Fatalf("expected edits across at least 2 files, got %d", len(we.FileEdits))
	}
}

func TestAdapterFindSymbolTypeScriptMethod(t *testing.T) {
	if !tsmorph.Available() {
		t.Skip("ts-morph backend not installed")
	}

	srcDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "typescript", "rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkNodeModules(t, srcDir, dir)

	adapter := tsmorph.NewAdapter()
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	locs, err := adapter.FindSymbol(symbol.Query{
		QualifiedName: "Person.greet",
		File:          filepath.Join(dir, "src", "person.ts"),
		Kind:          symbol.KindMethod,
	})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if got := locs[0]; got.Name != "greet" || got.Line != 4 {
		t.Fatalf("unexpected location: %+v", got)
	}
}

func TestAdapterFindSymbolJavaScriptFunction(t *testing.T) {
	if !tsmorph.Available() {
		t.Skip("ts-morph backend not installed")
	}

	srcDir := filepath.Join("..", "..", "..", "testdata", "fixtures", "javascript", "rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkNodeModules(t, srcDir, dir)

	adapter := tsmorph.NewAdapter()
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	locs, err := adapter.FindSymbol(symbol.Query{
		QualifiedName: "sum",
		File:          filepath.Join(dir, "src", "math.js"),
		Kind:          symbol.KindFunction,
	})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if got := locs[0]; got.Name != "sum" || got.Line != 1 {
		t.Fatalf("unexpected location: %+v", got)
	}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	for _, e := range entries {
		if e.Name() == "node_modules" {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dstPath, err)
			}
			copyDir(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dstPath, err)
		}
	}
}

func linkNodeModules(t *testing.T, fixtureDir string, workspaceDir string) {
	t.Helper()
	target, err := filepath.Abs(filepath.Join(fixtureDir, "node_modules"))
	if err != nil {
		t.Fatalf("resolve node_modules path: %v", err)
	}
	link := filepath.Join(workspaceDir, "node_modules")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link node_modules: %v", err)
	}
}
