package openrewrite

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// contractDir holds the shared golden wire-contract fixtures, consumed by both
// this Go driver test and adapters/openrewrite/.../WireContractTest.java (#76).
const contractDir = "../../../testdata/adapter-contracts/openrewrite"

func readContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(contractDir, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func assertContractJSONEqual(t *testing.T, got []byte, fixtureName string) {
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

// contractAdapter returns an adapter whose stdin captures the request bytes and
// whose stdout replays the given fixture response, so callRename exercises the
// real serialization and parsing paths.
func contractAdapter(respFixture []byte) (*Adapter, *bytes.Buffer) {
	var written bytes.Buffer
	a := &Adapter{
		stdin:  nopWriteCloser{&written},
		stdout: json.NewDecoder(bytes.NewReader(append(append([]byte{}, respFixture...), '\n'))),
	}
	return a, &written
}

func TestContract_RenameMethodRequestMatchesGolden(t *testing.T) {
	a, written := contractAdapter(readContractFixture(t, "rename.response.json"))
	params := map[string]any{
		"workspaceRoot": "/workspace",
		"newName":       "hello",
		"methodPattern": "com.example.Greeter greet(..)",
	}
	if _, err := a.callRename(params); err != nil {
		t.Fatalf("callRename: %v", err)
	}
	assertContractJSONEqual(t, []byte(strings.TrimSpace(written.String())), "rename-method.request.json")
}

func TestContract_RenameTypeRequestMatchesGolden(t *testing.T) {
	a, written := contractAdapter(readContractFixture(t, "rename.response.json"))
	params := map[string]any{
		"workspaceRoot":         "/workspace",
		"newName":               "HelloService",
		"oldFullyQualifiedName": "com.example.Greeter",
	}
	if _, err := a.callRename(params); err != nil {
		t.Fatalf("callRename: %v", err)
	}
	assertContractJSONEqual(t, []byte(strings.TrimSpace(written.String())), "rename-type.request.json")
}

func TestContract_RenameResponseParsesGolden(t *testing.T) {
	a, _ := contractAdapter(readContractFixture(t, "rename.response.json"))
	fileEdits, err := a.callRename(map[string]any{"workspaceRoot": "/workspace", "newName": "hello", "methodPattern": "com.example.Greeter greet(..)"})
	if err != nil {
		t.Fatalf("callRename: %v", err)
	}
	if len(fileEdits) != 1 {
		t.Fatalf("fileEdits len = %d, want 1", len(fileEdits))
	}
	fe := fileEdits[0]
	if fe.Path != "/workspace/src/main/java/com/example/Greeter.java" {
		t.Errorf("path = %q", fe.Path)
	}
	if len(fe.Edits) == 0 {
		t.Fatal("expected at least one text edit")
	}
	// The full-file replacement must carry the renamed method.
	if !strings.Contains(fe.Edits[len(fe.Edits)-1].NewText, "public String hello(String name)") {
		t.Errorf("reconstructed content missing renamed method: %q", fe.Edits[0].NewText)
	}
}

func TestContract_ErrorResponseParsesGolden(t *testing.T) {
	a, _ := contractAdapter(readContractFixture(t, "error.response.json"))
	_, err := a.callRename(map[string]any{"workspaceRoot": "/workspace", "newName": "hello"})
	if err == nil {
		t.Fatal("expected error from golden error response, got nil")
	}
	if !strings.Contains(err.Error(), "params must include either") {
		t.Errorf("error %q does not surface the golden error message", err)
	}
}
