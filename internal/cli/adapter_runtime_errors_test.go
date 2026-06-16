package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// stubBackend is a fake RefactoringBackend whose Initialize returns a
// preconfigured error. It lets the CLI error-classification tests exercise the
// adapter-runtime-missing and init-failure paths without a real subprocess.
type stubBackend struct {
	initErr error
}

func (s *stubBackend) Initialize(string) error { return s.initErr }
func (s *stubBackend) Shutdown() error         { return nil }
func (s *stubBackend) FindSymbol(symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) Rename(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) ExtractFunction(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) ExtractVariable(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) InlineSymbol(symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) MoveToFile(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}
func (s *stubBackend) Capabilities() []backend.Capability { return nil }

// initThroughStub mimics how the rename path wraps a backend's Initialize error
// (fmt.Errorf("initializing backend: %w", err)) so tests cover the wrapped form
// the CLI actually sees.
func initThroughStub(initErr error) error {
	b := &stubBackend{initErr: initErr}
	if err := b.Initialize("/workspace"); err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	return nil
}

func tsMorphRuntimeMissing() *backend.ErrAdapterRuntimeMissing {
	return &backend.ErrAdapterRuntimeMissing{
		Language:       "typescript",
		AdapterName:    "ts-morph",
		MissingRuntime: "ts-morph adapter and node dependencies not installed",
		InstallHint:    "npm install -g @shatterproof-ai/refute-ts-adapter",
	}
}

func openRewriteJarMissing() *backend.ErrAdapterRuntimeMissing {
	return &backend.ErrAdapterRuntimeMissing{
		Language:       "java",
		AdapterName:    "openrewrite",
		MissingRuntime: "OpenRewrite adapter JAR (not found at /repo/adapters/openrewrite/target/openrewrite-adapter.jar)",
		InstallHint:    "mvn package -f /repo/adapters/openrewrite/pom.xml -q",
	}
}

// TestEmitJSONBackendSetupError_AdapterRuntimeMissing covers the two adapter
// runtime-missing cases (ts-morph node deps, OpenRewrite JAR). Both must emit a
// backend-missing JSON envelope with the adapter-runtime-missing code, carry the
// install hint, and exit with the shared backend-missing code (3).
func TestEmitJSONBackendSetupError_AdapterRuntimeMissing(t *testing.T) {
	ctx := jsonContext{Operation: "rename", Language: "typescript", Backend: "ts-morph", WorkspaceRoot: "/workspace"}

	cases := []struct {
		name     string
		typed    *backend.ErrAdapterRuntimeMissing
		wantHint string
	}{
		{"ts-morph dependency missing", tsMorphRuntimeMissing(), "npm install -g @shatterproof-ai/refute-ts-adapter"},
		{"openrewrite JAR missing", openRewriteJarMissing(), "mvn package -f /repo/adapters/openrewrite/pom.xml -q"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pass the error in its wrapped form, exactly as the rename path
			// surfaces a backend Initialize failure.
			wrapped := initThroughStub(tc.typed)

			var emitErr error
			out := captureStdout(t, func() {
				emitErr = emitJSONBackendSetupError(ctx, wrapped)
			})

			var ec *ExitCodeError
			if !errors.As(emitErr, &ec) {
				t.Fatalf("expected *ExitCodeError, got %#v", emitErr)
			}
			if ec.Code != backendMissingExitCode {
				t.Errorf("exit code = %d, want %d", ec.Code, backendMissingExitCode)
			}

			var got edit.JSONResult
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
			}
			if got.SchemaVersion != edit.SchemaVersion {
				t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, edit.SchemaVersion)
			}
			if got.Status != edit.StatusBackendMissing {
				t.Errorf("status = %q, want %q", got.Status, edit.StatusBackendMissing)
			}
			if got.Error == nil {
				t.Fatalf("missing error object\nraw:\n%s", out)
			}
			if got.Error.Code != "adapter-runtime-missing" {
				t.Errorf("error code = %q, want adapter-runtime-missing", got.Error.Code)
			}
			if got.Error.Hint != tc.wantHint {
				t.Errorf("hint = %q, want %q", got.Error.Hint, tc.wantHint)
			}
			if !strings.Contains(got.Error.Message, tc.typed.AdapterName) {
				t.Errorf("message %q does not mention adapter %q", got.Error.Message, tc.typed.AdapterName)
			}
		})
	}
}

// TestEmitJSONBackendSetupError_InitFailure verifies a generic backend
// initialization failure (no missing tool) maps to backend-failed with the
// backend-init-failed code and exit 1 — distinct from "install this tool".
func TestEmitJSONBackendSetupError_InitFailure(t *testing.T) {
	ctx := jsonContext{Operation: "rename", Language: "go", Backend: "lsp", WorkspaceRoot: "/workspace"}
	wrapped := initThroughStub(NewBackendInitFailure("lsp", errors.New("handshake timed out")))

	var emitErr error
	out := captureStdout(t, func() {
		emitErr = emitJSONBackendSetupError(ctx, wrapped)
	})

	var ec *ExitCodeError
	if !errors.As(emitErr, &ec) {
		t.Fatalf("expected *ExitCodeError, got %#v", emitErr)
	}
	if ec.Code != 1 {
		t.Errorf("exit code = %d, want 1", ec.Code)
	}

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusBackendFailed {
		t.Errorf("status = %q, want %q", got.Status, edit.StatusBackendFailed)
	}
	if got.Error == nil || got.Error.Code != "backend-init-failed" {
		t.Fatalf("error code = %+v, want backend-init-failed", got.Error)
	}
}

// TestExitDetails_AdapterRuntimeMissing verifies the human-readable (non-JSON)
// path: a wrapped adapter-runtime-missing error exits 3 and the message carries
// the install hint so the user knows the next action.
func TestExitDetails_AdapterRuntimeMissing(t *testing.T) {
	wrapped := initThroughStub(tsMorphRuntimeMissing())

	code, msg := exitDetails(wrapped)
	if code != backendMissingExitCode {
		t.Errorf("exit code = %d, want %d", code, backendMissingExitCode)
	}
	if !strings.Contains(msg, "npm install") {
		t.Errorf("message %q does not carry install hint", msg)
	}
	if defaultStatusForError(wrapped) != edit.StatusBackendMissing {
		t.Errorf("status = %q, want %q", defaultStatusForError(wrapped), edit.StatusBackendMissing)
	}
}

// TestExitDetails_BackendInitFailure verifies a wrapped generic init failure
// exits 1 (not the backend-missing code) and reports backend-failed status.
func TestExitDetails_BackendInitFailure(t *testing.T) {
	wrapped := initThroughStub(NewBackendInitFailure("openrewrite", errors.New("JVM crashed")))

	code, _ := exitDetails(wrapped)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if defaultStatusForError(wrapped) != edit.StatusBackendFailed {
		t.Errorf("status = %q, want %q", defaultStatusForError(wrapped), edit.StatusBackendFailed)
	}
}

// TestBackendInitFailure_UnwrapsToRuntimeMissing verifies that wrapping a
// missing adapter runtime inside ErrBackendInitFailure still classifies as
// backend-missing (exit 3) — the more specific "install this" wins over the
// generic "backend crashed".
func TestBackendInitFailure_UnwrapsToRuntimeMissing(t *testing.T) {
	wrapped := initThroughStub(NewBackendInitFailure("ts-morph", tsMorphRuntimeMissing()))

	if !isBackendRuntimeMissing(wrapped) {
		t.Fatalf("expected wrapped error to classify as backend runtime missing")
	}
	code, _ := exitDetails(wrapped)
	if code != backendMissingExitCode {
		t.Errorf("exit code = %d, want %d", code, backendMissingExitCode)
	}
}
