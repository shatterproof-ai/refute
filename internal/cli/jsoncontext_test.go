package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestEmitJSONError_Golden(t *testing.T) {
	out := captureStdout(t, func() {
		err := emitJSONError(
			jsonContext{
				Operation:     "rename",
				Language:      "go",
				Backend:       "lsp",
				WorkspaceRoot: "/workspace",
			},
			edit.StatusInvalidPosition,
			"invalid-position",
			"name not found",
			"Check --file, --line, --col, and --name.",
		)
		var ec *ExitCodeError
		if !errors.As(err, &ec) || ec.Code != 1 {
			t.Fatalf("expected exit code 1 error, got %#v", err)
		}
	})

	want := `{
  "schemaVersion": "1",
  "status": "invalid-position",
  "operation": "rename",
  "language": "go",
  "backend": "lsp",
  "workspaceRoot": "/workspace",
  "filesModified": 0,
  "error": {
    "code": "invalid-position",
    "message": "name not found",
    "hint": "Check --file, --line, --col, and --name."
  }
}
`
	if out != want {
		t.Fatalf("JSON error envelope mismatch\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestRunRename_JSONSymbolResolutionError(t *testing.T) {
	resetRenameFlags := func() {
		flagFile = ""
		flagLine = 0
		flagCol = 0
		flagName = ""
		flagNewName = ""
		flagSymbol = ""
		flagJSON = false
		flagDryRun = false
		flagConfig = ""
	}
	resetRenameFlags()
	t.Cleanup(resetRenameFlags)

	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	flagFile = filePath
	flagLine = 3
	flagName = "missing"
	flagNewName = "renamed"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code != 1 {
		t.Fatalf("expected JSON exit error, got %#v", runErr)
	}

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal JSON envelope: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusInvalidPosition {
		t.Fatalf("status = %q, want %q", got.Status, edit.StatusInvalidPosition)
	}
	if got.Error == nil || got.Error.Code != "invalid-position" {
		t.Fatalf("unexpected error object: %+v", got.Error)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}
	return string(data)
}
