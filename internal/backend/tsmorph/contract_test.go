package tsmorph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// contractDir holds the shared golden wire-contract fixtures, consumed by both
// this Go driver test and adapters/tsmorph/contract.test.cjs (issue #76).
const contractDir = "../../../testdata/adapter-contracts/tsmorph"

func readContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(contractDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// assertJSONEqual compares two JSON documents semantically (independent of key
// order and whitespace).
func assertJSONEqual(t *testing.T, got []byte, fixtureName string) {
	t.Helper()
	want := readContractFixture(t, fixtureName)
	var gotAny, wantAny any
	if err := json.Unmarshal(got, &gotAny); err != nil {
		t.Fatalf("unmarshal got: %v\n%s", err, got)
	}
	if err := json.Unmarshal(want, &wantAny); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", fixtureName, err)
	}
	if !reflect.DeepEqual(gotAny, wantAny) {
		t.Errorf("wire mismatch with %s:\n got: %s\nwant: %s", fixtureName, got, want)
	}
}

func fixtureProtocolVersion(t *testing.T, data []byte) int {
	t.Helper()
	var meta struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal protocolVersion: %v", err)
	}
	return meta.ProtocolVersion
}

func TestContract_RenameRequestMatchesGolden(t *testing.T) {
	req := renameRequest{
		ProtocolVersion: ProtocolVersion,
		Operation:       "rename",
		WorkspaceRoot:   "/workspace",
		File:            "/workspace/greeter.ts",
		Line:            1,
		Column:          17,
		NewName:         "welcome",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertJSONEqual(t, data, "rename.request.json")
}

func TestContract_FindSymbolRequestMatchesGolden(t *testing.T) {
	req := findSymbolRequest{
		ProtocolVersion: ProtocolVersion,
		Operation:       "findSymbol",
		WorkspaceRoot:   "/workspace",
		File:            "/workspace/greeter.ts",
		QualifiedName:   "greeter:greet",
		Kind:            "function",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	assertJSONEqual(t, data, "find-symbol.request.json")
}

func TestContract_RenameResponseParsesGolden(t *testing.T) {
	data := readContractFixture(t, "rename.response.json")
	if got := fixtureProtocolVersion(t, data); got != ProtocolVersion {
		t.Errorf("response protocolVersion = %d, want %d", got, ProtocolVersion)
	}

	var resp renameResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.FileEdits) != 1 {
		t.Fatalf("fileEdits len = %d, want 1", len(resp.FileEdits))
	}
	fe := resp.FileEdits[0]
	if fe.Path != "/workspace/greeter.ts" {
		t.Errorf("path = %q", fe.Path)
	}
	if len(fe.Edits) != 1 {
		t.Fatalf("edits len = %d, want 1", len(fe.Edits))
	}
	e := fe.Edits[0]
	if e.Range.Start.Line != 0 || e.Range.Start.Character != 0 {
		t.Errorf("start = %+v, want {0 0}", e.Range.Start)
	}
	if e.Range.End.Line != 3 || e.Range.End.Character != 0 {
		t.Errorf("end = %+v, want {3 0}", e.Range.End)
	}
	if e.NewText != "export function welcome() {\n  return \"hello\";\n}\n" {
		t.Errorf("newText = %q", e.NewText)
	}
}

func TestContract_FindSymbolResponseParsesGolden(t *testing.T) {
	data := readContractFixture(t, "find-symbol.response.json")
	if got := fixtureProtocolVersion(t, data); got != ProtocolVersion {
		t.Errorf("response protocolVersion = %d, want %d", got, ProtocolVersion)
	}

	var resp findSymbolResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Candidates) != 1 {
		t.Fatalf("candidates len = %d, want 1", len(resp.Candidates))
	}
	c := resp.Candidates[0]
	if c.File != "/workspace/greeter.ts" || c.Line != 1 || c.Column != 17 || c.Name != "greet" || c.Kind != "function" {
		t.Errorf("candidate = %+v", c)
	}
}
