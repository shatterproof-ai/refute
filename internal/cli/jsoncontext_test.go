package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
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
		flagJSON = false
		flagDryRun = false
		flagConfig = ""
	}
	resetRenameFlags()
	t.Cleanup(resetRenameFlags)
	flags := &renameFlags{}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	flags.File = filePath
	flags.Line = 3
	flags.Name = "missing"
	flags.NewName = "renamed"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
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

func TestRenameNoEditsExhaustion_JSONIsFailureEnvelope(t *testing.T) {
	ctx := jsonContext{
		Operation:     "rename",
		Language:      "go",
		Backend:       "lsp",
		WorkspaceRoot: "/workspace",
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = emitJSONOperationError(ctx, fmt.Errorf("rename failed: %w", lsp.ErrRenameNoEdits))
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code != 1 {
		t.Fatalf("expected JSON exit error 1, got %#v", runErr)
	}

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal JSON envelope: %v\nraw:\n%s", err, out)
	}
	if got.Status == edit.StatusApplied || got.Status == edit.StatusDryRun {
		t.Fatalf("rename retry exhaustion reported success status %q\nraw:\n%s", got.Status, out)
	}
	if got.Status != edit.StatusBackendFailed {
		t.Fatalf("status = %q, want %q\nraw:\n%s", got.Status, edit.StatusBackendFailed, out)
	}
	if got.FilesModified != 0 || len(got.Edits) != 0 {
		t.Fatalf("error envelope should not report edits: files=%d edits=%+v", got.FilesModified, got.Edits)
	}
	if got.Error == nil || got.Error.Code != "operation-failed" {
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
