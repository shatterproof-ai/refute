package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// resetOperationFlagsForTest clears the shared invocation flags each operation
// command still reads so table cases run against a clean invocation.
func resetOperationFlagsForTest(t *testing.T) {
	t.Helper()
	prevConfig := flagConfig
	clear := func() {
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
				return runRename(symbol.KindFunction, &renameFlags{
					File:    mainFile,
					Line:    3,
					Name:    "hello",
					NewName: "hi",
				})
			},
		},
		{
			name: "extract-function",
			run: func(mainFile string) error {
				return runExtract("function", &extractFlags{
					File:      mainFile,
					StartLine: 3,
					StartCol:  1,
					EndLine:   3,
					EndCol:    16,
				})
			},
		},
		{
			name: "extract-variable",
			run: func(mainFile string) error {
				return runExtract("variable", &extractFlags{
					File:      mainFile,
					StartLine: 3,
					StartCol:  1,
					EndLine:   3,
					EndCol:    16,
				})
			},
		},
		{
			name: "inline",
			run: func(mainFile string) error {
				return runInline(&inlineFlags{
					File: mainFile,
					Line: 3,
					Name: "hello",
				})
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
			if got.Error == nil {
				t.Fatalf("missing error object")
			}
			if got.Error.Code != "backend-missing" {
				t.Fatalf("error code = %q, want backend-missing; envelope:\n%s", got.Error.Code, out)
			}
			if got.Error.Hint == "" {
				t.Fatalf("hint is empty; envelope:\n%s", out)
			}
		})
	}
}

func TestOperationCommands_JSONUnsupportedOperationBeforeBackendSetup(t *testing.T) {
	cases := []struct {
		name string
		run  func(tsFile string) error
	}{
		{
			name: "extract-function",
			run: func(tsFile string) error {
				return runExtract("function", &extractFlags{
					File:      tsFile,
					StartLine: 1,
					StartCol:  1,
					EndLine:   1,
					EndCol:    31,
				})
			},
		},
		{
			name: "extract-variable",
			run: func(tsFile string) error {
				return runExtract("variable", &extractFlags{
					File:      tsFile,
					StartLine: 2,
					StartCol:  10,
					EndLine:   2,
					EndCol:    18,
				})
			},
		},
		{
			name: "inline",
			run: func(tsFile string) error {
				return runInline(&inlineFlags{
					File: tsFile,
					Line: 1,
					Name: "add",
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetOperationFlagsForTest(t)
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}\n"), 0o644); err != nil {
				t.Fatalf("write package.json: %v", err)
			}
			tsFile := filepath.Join(dir, "math.ts")
			if err := os.WriteFile(tsFile, []byte("export function add() {\n  return left + right;\n}\n"), 0o644); err != nil {
				t.Fatalf("write TS fixture: %v", err)
			}
			flagConfig = writeServerConfig(t, dir, "typescript", "refute-nonexistent-lsp-binary-xyz")
			flagJSON = true

			var runErr error
			out := captureStdout(t, func() {
				runErr = tc.run(tsFile)
			})

			var ec *ExitCodeError
			if !errors.As(runErr, &ec) || ec.Code != 1 {
				t.Fatalf("expected exit code 1, got %#v", runErr)
			}
			var got edit.JSONResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
			}
			if got.Status != edit.StatusUnsupported {
				t.Fatalf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusUnsupported, out)
			}
			if got.Error == nil || got.Error.Code != "unsupported-operation" {
				t.Fatalf("error = %+v, want code unsupported-operation; envelope:\n%s", got.Error, out)
			}
			if got.Error.Hint == "" {
				t.Fatalf("hint is empty; envelope:\n%s", out)
			}
			if got.Language != "typescript" {
				t.Fatalf("language = %q, want typescript; envelope:\n%s", got.Language, out)
			}
			if got.WorkspaceRoot != dir {
				t.Fatalf("workspaceRoot = %q, want %q; envelope:\n%s", got.WorkspaceRoot, dir, out)
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
			name:       "selector-operation-unsupported",
			err:        fmt.Errorf("extract-variable: %w", selector.ErrOperationUnsupported),
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
			name:       "adapter-runtime-missing",
			err:        tsMorphRuntimeMissing(),
			wantStatus: edit.StatusBackendMissing,
			wantCode:   "adapter-runtime-missing",
			wantExit:   3,
		},
		{
			name:       "backend-init-failed",
			err:        NewBackendInitFailure("lsp", errors.New("handshake timed out")),
			wantStatus: edit.StatusBackendFailed,
			wantCode:   "backend-init-failed",
			wantExit:   1,
		},
		{
			name:       "language-unsupported",
			err:        &selector.ErrLanguageUnsupported{Language: "java", Caveat: "Java/OpenRewrite support is not claimed for v0.1."},
			wantStatus: edit.StatusUnsupported,
			wantCode:   "unsupported-language",
			wantExit:   1,
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
			if tc.wantCode == "unsupported-operation" && got.Error.Hint == "" {
				t.Errorf("hint is empty for unsupported-operation")
			}
		})
	}
}

// TestRename_UnsupportedLanguageReportsDocumentedStatus is the issue #110
// end-to-end guard: a Java rename dry run must return the documented
// language-unsupported envelope before any OpenRewrite setup, rather than
// failing later with a backend setup error. The fixture has no
// language server configured, so reaching a backend at all would surface a
// different status.
func TestRename_UnsupportedLanguageReportsDocumentedStatus(t *testing.T) {
	resetOperationFlagsForTest(t)
	dir := t.TempDir()
	javaFile := filepath.Join(dir, "Main.java")
	if err := os.WriteFile(javaFile, []byte("public class Main {\n    int value = 1;\n    void hello() {}\n}\n"), 0o644); err != nil {
		t.Fatalf("write java fixture: %v", err)
	}

	flagJSON = true
	flags := &renameFlags{
		File:    javaFile,
		Line:    3,
		Name:    "hello",
		NewName: "greet",
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
	})

	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusUnsupported {
		t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusUnsupported, out)
	}
	if got.Error == nil || got.Error.Code != "unsupported-language" {
		t.Fatalf("error = %+v, want code unsupported-language", got.Error)
	}
	if !strings.Contains(got.Error.Message, "java is not supported") {
		t.Errorf("message = %q, want it to mention java is not supported", got.Error.Message)
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

// TestRouteOperationError_NoDoubleEnvelope verifies that an error whose
// envelope was already written (here, a Tier 1 ambiguous rename) passes through
// the shared wrapper without a second envelope being emitted. Two concatenated
// JSON objects on stdout would make the output unparseable for consumers.
func TestRouteOperationError_NoDoubleEnvelope(t *testing.T) {
	resetOperationFlagsForTest(t)
	flagJSON = true

	ctx := jsonContext{Operation: "rename", Language: "rust", Backend: "lsp", WorkspaceRoot: "/workspace"}
	ambiguous := &backend.ErrAmbiguous{Candidates: []symbol.Location{
		{File: "/workspace/src/lib.rs", Line: 4, Column: 1, Name: "rename_me"},
	}}

	var runErr error
	out := captureStdout(t, func() {
		// Mirror the live path: runRenameInner returns the tier-1 handler's
		// result, then runRename hands it to routeOperationError.
		runErr = routeOperationError(ctx, handleTier1RenameError(ctx, ambiguous))
	})

	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code != 1 {
		t.Fatalf("expected exit code 1, got %#v", runErr)
	}
	// Exactly one JSON object must be present; a trailing second object would
	// decode successfully and fail this guard.
	dec := json.NewDecoder(strings.NewReader(out))
	var first edit.JSONResult
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first envelope: %v\nraw:\n%s", err, out)
	}
	if first.Status != edit.StatusAmbiguous {
		t.Fatalf("status = %q, want %q", first.Status, edit.StatusAmbiguous)
	}
	if err := dec.Decode(new(edit.JSONResult)); err == nil {
		t.Fatalf("expected exactly one envelope, found a second:\n%s", out)
	}
}
