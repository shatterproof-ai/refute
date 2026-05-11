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

// TestJSONResult_TolerateUnknownFields asserts that decoders built on the
// published JSONResult shape accept extra/unknown fields. This is the
// forward-compat guarantee for consumers pinned to an older refute release
// when reading output from a newer one.
func TestJSONResult_TolerateUnknownFields(t *testing.T) {
	payload := []byte(`{
		"schemaVersion": "1",
		"status": "applied",
		"operation": "rename",
		"filesModified": 0,
		"futureField": "ignored-by-old-consumer",
		"nestedFuture": {"a": 1, "b": [1, 2, 3]},
		"telemetry": {"durationMs": 42}
	}`)

	var res edit.JSONResult
	if err := json.Unmarshal(payload, &res); err != nil {
		t.Fatalf("unknown fields must not break unmarshal: %v", err)
	}
	if res.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want %q", res.SchemaVersion, "1")
	}
	if res.Status != edit.StatusApplied {
		t.Errorf("status = %q, want %q", res.Status, edit.StatusApplied)
	}
	if res.Operation != "rename" {
		t.Errorf("operation = %q, want rename", res.Operation)
	}
}

// TestJSONResult_FutureSchemaVersion asserts that a future schemaVersion
// round-trips intact through the published struct so that consumers can
// branch on it and degrade gracefully rather than misinterpreting payloads.
func TestJSONResult_FutureSchemaVersion(t *testing.T) {
	payload := []byte(`{
		"schemaVersion": "99",
		"status": "applied",
		"filesModified": 0
	}`)

	var res edit.JSONResult
	if err := json.Unmarshal(payload, &res); err != nil {
		t.Fatalf("future schemaVersion must not break unmarshal: %v", err)
	}
	if res.SchemaVersion == edit.SchemaVersion {
		t.Errorf("decoder collapsed future schemaVersion to current %q; consumer cannot detect skew", res.SchemaVersion)
	}
	if res.SchemaVersion != "99" {
		t.Errorf("schemaVersion = %q, want %q", res.SchemaVersion, "99")
	}
}

// TestJSONResult_MissingSchemaVersion asserts that an envelope without
// schemaVersion unmarshals to the empty string so consumers can treat it
// as unknown rather than silently aliasing it to the current version.
func TestJSONResult_MissingSchemaVersion(t *testing.T) {
	payload := []byte(`{"status": "applied", "filesModified": 0}`)

	var res edit.JSONResult
	if err := json.Unmarshal(payload, &res); err != nil {
		t.Fatalf("missing schemaVersion must not break unmarshal: %v", err)
	}
	if res.SchemaVersion != "" {
		t.Errorf("missing schemaVersion should decode to \"\", got %q", res.SchemaVersion)
	}
}

// TestJSONResult_UnknownStatus asserts that an unknown status string is
// preserved verbatim. Status is an open enum from the consumer's perspective;
// future refute versions may add statuses that older consumers must be able
// to observe and route to a default branch.
func TestJSONResult_UnknownStatus(t *testing.T) {
	payload := []byte(`{
		"schemaVersion": "1",
		"status": "future-status-code",
		"filesModified": 0
	}`)

	var res edit.JSONResult
	if err := json.Unmarshal(payload, &res); err != nil {
		t.Fatalf("unknown status must not break unmarshal: %v", err)
	}
	if res.Status != "future-status-code" {
		t.Errorf("unknown status not preserved: got %q", res.Status)
	}
}

// TestJSONResult_ConsumerForwardCompat simulates an older consumer that pins
// a narrow view of the envelope and reads a newer producer's output. The
// consumer must see the fields it knows about and not error on the rest.
func TestJSONResult_ConsumerForwardCompat(t *testing.T) {
	// Simulated future producer output: new top-level fields, new fields
	// inside JSONChange, an unknown status, and a bumped schemaVersion.
	future := []byte(`{
		"schemaVersion": "2",
		"status": "applied-with-followups",
		"operation": "rename",
		"language": "go",
		"backend": "lsp",
		"workspaceRoot": "/w",
		"filesModified": 1,
		"edits": [
			{
				"file": "/w/a.go",
				"changes": [
					{
						"startLine": 3,
						"startCol": 5,
						"endLine": 3,
						"endCol": 12,
						"newText": "NewName",
						"confidence": 0.97,
						"sourceRule": "go.rename.v2"
					}
				],
				"renamePreview": "old -> new"
			}
		],
		"followups": [{"kind": "format", "file": "/w/a.go"}],
		"telemetry": {"durationMs": 17}
	}`)

	// Consumer's narrow view of the envelope: only the fields it knows.
	type consumerChange struct {
		StartLine int    `json:"startLine"`
		StartCol  int    `json:"startCol"`
		EndLine   int    `json:"endLine"`
		EndCol    int    `json:"endCol"`
		NewText   string `json:"newText"`
	}
	type consumerFileEdit struct {
		File    string           `json:"file"`
		Changes []consumerChange `json:"changes"`
	}
	type consumerEnvelope struct {
		SchemaVersion string             `json:"schemaVersion"`
		Status        string             `json:"status"`
		FilesModified int                `json:"filesModified"`
		Edits         []consumerFileEdit `json:"edits"`
	}

	var got consumerEnvelope
	if err := json.Unmarshal(future, &got); err != nil {
		t.Fatalf("older consumer must tolerate newer producer output: %v", err)
	}
	if got.SchemaVersion != "2" {
		t.Errorf("schemaVersion = %q, want \"2\"", got.SchemaVersion)
	}
	if got.Status != "applied-with-followups" {
		t.Errorf("status = %q, want applied-with-followups", got.Status)
	}
	if got.FilesModified != 1 {
		t.Errorf("filesModified = %d, want 1", got.FilesModified)
	}
	if len(got.Edits) != 1 || len(got.Edits[0].Changes) != 1 {
		t.Fatalf("expected 1 edit with 1 change, got %+v", got.Edits)
	}
	c := got.Edits[0].Changes[0]
	if c.StartLine != 3 || c.StartCol != 5 || c.EndLine != 3 || c.EndCol != 12 || c.NewText != "NewName" {
		t.Errorf("known change fields not decoded correctly: %+v", c)
	}
}

// TestJSONResult_NewOptionalFieldsOmitted asserts that all optional fields
// stay absent from the marshalled envelope when unset. Adding an optional
// field that always marshals (no omitempty / wrong zero) would break older
// consumers that strict-decode or schema-validate.
func TestJSONResult_NewOptionalFieldsOmitted(t *testing.T) {
	res := &edit.JSONResult{
		SchemaVersion: edit.SchemaVersion,
		Status:        edit.StatusNoOp,
	}
	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(data)
	for _, banned := range []string{
		`"operation"`,
		`"language"`,
		`"backend"`,
		`"workspaceRoot"`,
		`"edits"`,
		`"newSymbol"`,
		`"candidates"`,
		`"warnings"`,
		`"error"`,
	} {
		if strings.Contains(got, banned) {
			t.Errorf("optional field %s leaked into minimal envelope:\n%s", banned, got)
		}
	}
	// filesModified is a required numeric field; it must be present even at zero.
	if !strings.Contains(got, `"filesModified": 0`) {
		t.Errorf("filesModified must be present in minimal envelope:\n%s", got)
	}
}

// TestSchemaVersion_StableConstant asserts that the published SchemaVersion
// constant is non-empty and a plain string token. Consumers branch on this
// value; if it ever became empty or whitespace, downstream comparisons would
// silently misroute.
func TestSchemaVersion_StableConstant(t *testing.T) {
	v := edit.SchemaVersion
	if v == "" {
		t.Fatal("SchemaVersion must not be empty")
	}
	if strings.TrimSpace(v) != v {
		t.Errorf("SchemaVersion has surrounding whitespace: %q", v)
	}
}
