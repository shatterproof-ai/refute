package edit_test

import (
	"encoding/json"
	"strings"
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

func TestRenderJSON_ordersFilesAndChangesDeterministically(t *testing.T) {
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: "/tmp/b.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 3, Character: 0},
							End:   edit.Position{Line: 3, Character: 1},
						},
						NewText: "late",
					},
					{
						Range: edit.Range{
							Start: edit.Position{Line: 1, Character: 0},
							End:   edit.Position{Line: 1, Character: 1},
						},
						NewText: "early",
					},
					{
						Range: edit.Range{
							Start: edit.Position{Line: 1, Character: 0},
							End:   edit.Position{Line: 1, Character: 1},
						},
						NewText: "same-range",
					},
				},
			},
			{
				Path: "/tmp/a.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 0},
							End:   edit.Position{Line: 0, Character: 1},
						},
						NewText: "first-file",
					},
				},
			},
		},
	}

	res := edit.RenderJSON(we, edit.StatusDryRun)
	if len(res.Edits) != 2 {
		t.Fatalf("got %d file edits, want 2", len(res.Edits))
	}
	if res.Edits[0].File != "/tmp/a.go" || res.Edits[1].File != "/tmp/b.go" {
		t.Fatalf("files not sorted by path: %+v", res.Edits)
	}

	changes := res.Edits[1].Changes
	if got := []string{changes[0].NewText, changes[1].NewText, changes[2].NewText}; got[0] != "early" || got[1] != "same-range" || got[2] != "late" {
		t.Fatalf("changes not sorted by position with stable equal-range order: got %v", got)
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

func TestJSONResult_BackendVersionRoundTripsAndOmitsWhenEmpty(t *testing.T) {
	withVersion := edit.JSONResult{
		SchemaVersion:  edit.SchemaVersion,
		Status:         edit.StatusApplied,
		BackendVersion: "gopls v1.2.3",
	}
	data, err := json.Marshal(withVersion)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got edit.JSONResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.BackendVersion != "gopls v1.2.3" {
		t.Errorf("backendVersion = %q, want round-trip preserved", got.BackendVersion)
	}

	// schemaVersion stays "1": backendVersion is an additive optional field.
	if edit.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want \"1\" (additive field must not bump it)", edit.SchemaVersion)
	}

	// Omitted when empty so existing consumers see an unchanged envelope.
	bare, err := json.Marshal(edit.JSONResult{SchemaVersion: edit.SchemaVersion, Status: edit.StatusApplied})
	if err != nil {
		t.Fatalf("marshal bare: %v", err)
	}
	if strings.Contains(string(bare), "backendVersion") {
		t.Errorf("empty backendVersion must be omitted, got: %s", bare)
	}
}
