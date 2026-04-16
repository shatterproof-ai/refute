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
