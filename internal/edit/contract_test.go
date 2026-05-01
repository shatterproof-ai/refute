package edit_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestJSONResult_SchemaVersion(t *testing.T) {
	res := edit.RenderJSON(&edit.WorkspaceEdit{}, edit.StatusNoOp)
	if res.SchemaVersion == "" {
		t.Fatal("RenderJSON must populate SchemaVersion")
	}
	if res.SchemaVersion != edit.SchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", res.SchemaVersion, edit.SchemaVersion)
	}
}

func TestJSONResult_StatusConstants(t *testing.T) {
	want := map[string]string{
		"applied":          edit.StatusApplied,
		"dry-run":          edit.StatusDryRun,
		"no-op":            edit.StatusNoOp,
		"ambiguous":        edit.StatusAmbiguous,
		"unsupported":      edit.StatusUnsupported,
		"backend-missing":  edit.StatusBackendMissing,
		"backend-failed":   edit.StatusBackendFailed,
		"invalid-position": edit.StatusInvalidPosition,
	}
	for literal, constant := range want {
		if literal != constant {
			t.Errorf("status constant for %q = %q, want %q", literal, constant, literal)
		}
	}
}

func TestJSONResult_AppliedEnvelope(t *testing.T) {
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: "/WORKSPACE/internal/widget/widget.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 9, Character: 5},
							End:   edit.Position{Line: 9, Character: 12},
						},
						NewText: "NewName",
					},
				},
			},
		},
	}
	res := edit.RenderJSON(we, edit.StatusApplied)
	res.Operation = "rename"
	res.Language = "go"
	res.Backend = "lsp"
	res.WorkspaceRoot = "/WORKSPACE"

	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`"schemaVersion": "1"`,
		`"status": "applied"`,
		`"operation": "rename"`,
		`"language": "go"`,
		`"backend": "lsp"`,
		`"workspaceRoot": "/WORKSPACE"`,
		`"filesModified": 1`,
		`"file": "/WORKSPACE/internal/widget/widget.go"`,
		`"newText": "NewName"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("envelope missing %s\nfull output:\n%s", want, got)
		}
	}

	if strings.Contains(got, `"error"`) {
		t.Errorf("success envelope should not include error field; got:\n%s", got)
	}

	var roundtrip edit.JSONResult
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip.Status != edit.StatusApplied || roundtrip.Operation != "rename" || roundtrip.SchemaVersion == "" {
		t.Errorf("roundtrip lost fields: %+v", roundtrip)
	}
}

func TestJSONResult_ErrorEnvelope(t *testing.T) {
	res := &edit.JSONResult{
		SchemaVersion: edit.SchemaVersion,
		Status:        edit.StatusBackendMissing,
		Operation:     "rename",
		Language:      "go",
		Error: &edit.JSONError{
			Code:    "backend-missing",
			Message: "gopls not found on PATH",
			Hint:    "go install golang.org/x/tools/gopls@latest",
		},
	}
	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`"status": "backend-missing"`,
		`"code": "backend-missing"`,
		`"message": "gopls not found on PATH"`,
		`"hint": "go install golang.org/x/tools/gopls@latest"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error envelope missing %s\nfull output:\n%s", want, got)
		}
	}

	if strings.Contains(got, `"edits"`) || strings.Contains(got, `"filesModified": 1`) {
		t.Errorf("error envelope should not carry edit data; got:\n%s", got)
	}
}
