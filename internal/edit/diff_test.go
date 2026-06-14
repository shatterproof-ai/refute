package edit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestRenderDiff_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "foo.go")
	content := "func oldName() {}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 5},
							End:   edit.Position{Line: 0, Character: 12},
						},
						NewText: "newName",
					},
				},
			},
		},
	}

	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}

	if !strings.Contains(diff, "-func oldName() {}") {
		t.Errorf("diff missing removal line; got:\n%s", diff)
	}
	if !strings.Contains(diff, "+func newName() {}") {
		t.Errorf("diff missing addition line; got:\n%s", diff)
	}
}

func TestRenderDiff_NoEdits(t *testing.T) {
	we := &edit.WorkspaceEdit{}

	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}

	if diff != "" {
		t.Errorf("expected empty string for no edits, got: %q", diff)
	}
}

func TestRenderDiff_ordersFilesDeterministically(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	if err := os.WriteFile(aPath, []byte("package main\n\nvar a = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("package main\n\nvar b = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: bPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 4},
							End:   edit.Position{Line: 2, Character: 5},
						},
						NewText: "renamedB",
					},
				},
			},
			{
				Path: aPath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 4},
							End:   edit.Position{Line: 2, Character: 5},
						},
						NewText: "renamedA",
					},
				},
			},
		},
	}

	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}

	aIndex := strings.Index(diff, "--- "+aPath)
	bIndex := strings.Index(diff, "--- "+bPath)
	if aIndex < 0 || bIndex < 0 {
		t.Fatalf("diff missing file headers for %q or %q:\n%s", aPath, bPath, diff)
	}
	if aIndex > bIndex {
		t.Fatalf("diff files not sorted by path:\n%s", diff)
	}
}

func TestRenderDiff_preservesEqualRangeInsertOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "letters.txt")
	if err := os.WriteFile(path, []byte("AB\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pos := edit.Position{Line: 0, Character: 1}
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: path,
				Edits: []edit.TextEdit{
					{Range: edit.Range{Start: pos, End: pos}, NewText: "X"},
					{Range: edit.Range{Start: pos, End: pos}, NewText: "Y"},
				},
			},
		},
	}

	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}
	if !strings.Contains(diff, "+AXYB") {
		t.Fatalf("same-position inserts did not keep array order:\n%s", diff)
	}
}
