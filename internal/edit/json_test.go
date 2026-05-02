package edit_test

import (
	"encoding/json"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestRenderJSON_convertsIndexing(t *testing.T) {
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: "/tmp/a.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 4},
							End:   edit.Position{Line: 2, Character: 11},
						},
						NewText: "newName",
					},
				},
			},
		},
	}
	res := edit.RenderJSON(we, "applied")

	if res.Status != "applied" {
		t.Errorf("status = %q, want applied", res.Status)
	}
	if res.FilesModified != 1 {
		t.Errorf("filesModified = %d, want 1", res.FilesModified)
	}
	if len(res.Edits) != 1 || len(res.Edits[0].Changes) != 1 {
		t.Fatalf("expected 1 edit with 1 change, got %+v", res.Edits)
	}
	c := res.Edits[0].Changes[0]
	if c.StartLine != 3 || c.StartCol != 5 || c.EndLine != 3 || c.EndCol != 12 {
		t.Errorf("position conversion wrong: %+v", c)
	}

	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundtrip edit.JSONResult
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if roundtrip.Status != "applied" {
		t.Errorf("roundtrip status = %q", roundtrip.Status)
	}
}

func TestRenderJSON_SuccessGolden(t *testing.T) {
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: "/tmp/a.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 4},
							End:   edit.Position{Line: 2, Character: 11},
						},
						NewText: "newName",
					},
				},
			},
		},
	}
	res := edit.RenderJSON(we, edit.StatusDryRun)
	res.Operation = "rename"
	res.Language = "go"
	res.Backend = "lsp"
	res.WorkspaceRoot = "/workspace"

	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data) + "\n"
	want := `{
  "schemaVersion": "1",
  "status": "dry-run",
  "operation": "rename",
  "language": "go",
  "backend": "lsp",
  "workspaceRoot": "/workspace",
  "filesModified": 1,
  "edits": [
    {
      "file": "/tmp/a.go",
      "changes": [
        {
          "startLine": 3,
          "startCol": 5,
          "endLine": 3,
          "endCol": 12,
          "newText": "newName"
        }
      ]
    }
  ]
}
`
	if got != want {
		t.Fatalf("success JSON envelope mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderJSON_nilAndEmpty(t *testing.T) {
	if res := edit.RenderJSON(nil, "no-op"); res.FilesModified != 0 || len(res.Edits) != 0 {
		t.Errorf("nil input should produce empty result, got %+v", res)
	}
	if res := edit.RenderJSON(&edit.WorkspaceEdit{}, "no-op"); res.FilesModified != 0 {
		t.Errorf("empty WorkspaceEdit should produce 0 filesModified, got %d", res.FilesModified)
	}
}
