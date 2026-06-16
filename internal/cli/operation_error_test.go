package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// resetOperationFlagsForTest clears every package-level flag the operation
// commands read so each table case runs against a clean invocation.
func resetOperationFlagsForTest(t *testing.T) {
	t.Helper()
	prevConfig := flagConfig
	clear := func() {
		flagFile = ""
		flagLine = 0
		flagCol = 0
		flagName = ""
		flagNewName = ""
		flagSymbol = ""
		flagExtName = ""
		flagStartLine = 0
		flagStartCol = 0
		flagEndLine = 0
		flagEndCol = 0
		callSiteFlag = ""
		flagJSON = false
		flagDryRun = false
	}
	clear()
	flagConfig = ""
	t.Cleanup(func() {
		clear()
		flagConfig = prevConfig
	})
}

// TestOperationCommands_JSONBackendMissing verifies that every operation
// command emits exactly one structured envelope with status backend-missing on
// stdout when the configured language server binary is absent — for rename,
// extract-function, extract-variable, and inline alike.
func TestOperationCommands_JSONBackendMissing(t *testing.T) {
	cases := []struct {
		name string
		run  func(mainFile string) error
	}{
		{
			name: "rename",
			run: func(mainFile string) error {
				flagFile = mainFile
				flagLine = 3
				flagName = "hello"
				flagNewName = "hi"
				return runRename(symbol.KindFunction)
			},
		},
		{
			name: "extract-function",
			run: func(mainFile string) error {
				flagFile = mainFile
				flagStartLine, flagStartCol = 3, 1
				flagEndLine, flagEndCol = 3, 16
				return runExtract("function")
			},
		},
		{
			name: "extract-variable",
			run: func(mainFile string) error {
				flagFile = mainFile
				flagStartLine, flagStartCol = 3, 1
				flagEndLine, flagEndCol = 3, 16
				return runExtract("variable")
			},
		},
		{
			name: "inline",
			run: func(mainFile string) error {
				flagFile = mainFile
				flagLine = 3
				flagName = "hello"
				return runInline()
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetOperationFlagsForTest(t)
			dir := t.TempDir()
			mainFile := writeGoFixture(t, dir)
			flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")
			flagJSON = true

			var runErr error
			out := captureStdout(t, func() {
				runErr = tc.run(mainFile)
			})

			var ec *ExitCodeError
			if !errors.As(runErr, &ec) || ec.Code == 0 {
				t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
			}
			var got edit.JSONResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
			}
			if got.SchemaVersion != edit.SchemaVersion {
				t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, edit.SchemaVersion)
			}
			if got.Status != edit.StatusBackendMissing {
				t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusBackendMissing, out)
			}
			if got.Error == nil || got.Error.Code == "" {
				t.Fatalf("missing error object: %+v", got.Error)
			}
		})
	}
}

// TestEmitJSONOperationError_StatusRouting is a table-driven check that the
// shared router maps each error category to the correct envelope status and
// code for any operation context. This covers unsupported-operation and
// symbol-not-found, which require a live backend to reach through the command
// surface but are deterministic at the router.
func TestEmitJSONOperationError_StatusRouting(t *testing.T) {
	resetOperationFlagsForTest(t)
	flagJSON = true

	cases := []struct {
		name       string
		err        error
		wantStatus string
		wantCode   string
		wantExit   int
	}{
		{
			name:       "unsupported",
			err:        fmt.Errorf("extract-variable failed: %w", backend.ErrUnsupported),
			wantStatus: edit.StatusUnsupported,
			wantCode:   "unsupported-operation",
			wantExit:   1,
		},
		{
			name:       "symbol-not-found",
			err:        fmt.Errorf("inline failed: %w", backend.ErrSymbolNotFound),
			wantStatus: edit.StatusInvalidPosition,
			wantCode:   "symbol-not-found",
			wantExit:   2,
		},
		{
			name:       "backend-missing",
			err:        &ErrLSPServerMissing{Language: "go", Command: "gopls"},
			wantStatus: edit.StatusBackendMissing,
			wantCode:   "backend-missing",
			wantExit:   3,
		},
		{
			name:       "no-op",
			err:        NoEditsError(),
			wantStatus: edit.StatusNoOp,
			wantCode:   "no-op",
			wantExit:   2,
		},
		{
			name:       "backend-failed",
			err:        fmt.Errorf("extract-function failed: boom"),
			wantStatus: edit.StatusBackendFailed,
			wantCode:   "operation-failed",
			wantExit:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := jsonContext{Operation: "op", Language: "go", Backend: "lsp", WorkspaceRoot: "/ws"}
			var runErr error
			out := captureStdout(t, func() {
				runErr = emitJSONOperationError(ctx, tc.err)
			})
			var ec *ExitCodeError
			if !errors.As(runErr, &ec) {
				t.Fatalf("expected ExitCodeError, got %#v", runErr)
			}
			if ec.Code != tc.wantExit {
				t.Errorf("exit = %d, want %d", ec.Code, tc.wantExit)
			}
			var got edit.JSONResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.Error == nil || got.Error.Code != tc.wantCode {
				t.Errorf("error code = %+v, want %q", got.Error, tc.wantCode)
			}
		})
	}
}

// TestApplyFailureAfterPreview_JSONEnvelope verifies that when ApplyWithin fails
// after the success envelope would otherwise render, emitJSON emits a single
// structured apply-failed envelope on stdout rather than plain wrapped text.
func TestApplyFailureAfterPreview_JSONEnvelope(t *testing.T) {
	resetOperationFlagsForTest(t)
	flagJSON = true

	dir := t.TempDir()
	// An edit whose path resolves outside the workspace root forces ApplyWithin
	// to reject it after the success envelope would otherwise have rendered.
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: filepath.Join(dir, "..", "escape.go"),
				Edits: []edit.TextEdit{
					{
						Range:   edit.Range{Start: edit.Position{Line: 0, Character: 0}, End: edit.Position{Line: 0, Character: 0}},
						NewText: "x",
					},
				},
			},
		},
	}
	ctx := jsonContext{Operation: "rename", Language: "go", Backend: "lsp", WorkspaceRoot: dir}

	var runErr error
	out := captureStdout(t, func() {
		runErr = emitJSON(we, ctx, edit.StatusApplied)
	})

	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("apply-failure output is not a single JSON envelope: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusBackendFailed {
		t.Errorf("status = %q, want %q", got.Status, edit.StatusBackendFailed)
	}
	if got.Error == nil || got.Error.Code != "apply-failed" {
		t.Errorf("error = %+v, want code apply-failed", got.Error)
	}
}
