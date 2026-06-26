package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestRunExitCodes_HumanAndJSON(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		wantCode   int
		wantStatus string
		wantErr    string
	}{
		{name: "human success", mode: "human-success", wantCode: 0},
		{name: "human generic failure", mode: "human-generic", wantCode: 1},
		{name: "human no match", mode: "human-symbol-not-found", wantCode: 2},
		{name: "human backend missing", mode: "human-backend-missing", wantCode: 3},
		{name: "json success", mode: "json-success", wantCode: 0},
		{name: "json generic failure", mode: "json-generic", wantCode: 1, wantStatus: edit.StatusBackendFailed, wantErr: "operation-failed"},
		{name: "json no edits", mode: "json-no-edits", wantCode: 2, wantStatus: edit.StatusNoOp, wantErr: "no-op"},
		{name: "json no match", mode: "json-symbol-not-found", wantCode: 2, wantStatus: edit.StatusInvalidPosition, wantErr: "symbol-not-found"},
		{name: "json backend missing", mode: "json-backend-missing", wantCode: 3, wantStatus: edit.StatusBackendMissing, wantErr: "backend-missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestRunExitCodeHelper")
			cmd.Env = append(os.Environ(),
				"REFUTE_EXIT_CODE_HELPER=1",
				"REFUTE_EXIT_CODE_MODE="+tt.mode,
			)
			out, err := cmd.CombinedOutput()
			if got := processExitCode(err); got != tt.wantCode {
				t.Fatalf("exit code = %d, want %d\nerr: %v\noutput:\n%s", got, tt.wantCode, err, out)
			}
			if tt.wantStatus != "" {
				assertJSONErrorEnvelope(t, out, tt.wantStatus, tt.wantErr)
			}
		})
	}
}

func TestRunExitCodeHelper(t *testing.T) {
	if os.Getenv("REFUTE_EXIT_CODE_HELPER") == "" {
		return
	}
	Run(func() error {
		return runExitCodeHelperError(os.Getenv("REFUTE_EXIT_CODE_MODE"))
	})
}

func runExitCodeHelperError(mode string) error {
	ctx := jsonContext{
		Operation:     "rename",
		Language:      "go",
		Backend:       "lsp",
		WorkspaceRoot: "/workspace",
	}
	switch mode {
	case "human-success":
		flagJSON = false
		return nil
	case "human-generic":
		flagJSON = false
		return errors.New("generic failure")
	case "human-symbol-not-found":
		flagJSON = false
		return symbolNotFoundForTest()
	case "human-backend-missing":
		flagJSON = false
		return backendMissingForTest()
	case "json-success":
		flagJSON = true
		return nil
	case "json-generic":
		flagJSON = true
		return emitJSONOperationError(ctx, errors.New("generic failure"))
	case "json-no-edits":
		flagJSON = true
		return emitJSONOperationError(ctx, NoEditsError())
	case "json-symbol-not-found":
		flagJSON = true
		return emitJSONOperationError(ctx, symbolNotFoundForTest())
	case "json-backend-missing":
		flagJSON = true
		err := backendMissingForTest()
		return emitJSONBackendSetupError(ctx, err)
	default:
		return errors.New("unknown test helper mode: " + mode)
	}
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func assertJSONErrorEnvelope(t *testing.T, data []byte, wantStatus, wantErr string) {
	t.Helper()
	var got edit.JSONResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal JSON envelope: %v\nraw:\n%s", err, data)
	}
	if got.Status != wantStatus {
		t.Fatalf("status = %q, want %q\nraw:\n%s", got.Status, wantStatus, data)
	}
	if got.Error == nil {
		t.Fatalf("missing error object\nraw:\n%s", data)
	}
	if got.Error.Code != wantErr {
		t.Fatalf("error code = %q, want %q\nraw:\n%s", got.Error.Code, wantErr, data)
	}
}

func symbolNotFoundForTest() error {
	return &ErrSymbolNotFound{
		Language: "rust",
		Input:    "crate::missing",
		Name:     "missing",
	}
}

func backendMissingForTest() *ErrLSPServerMissing {
	return &ErrLSPServerMissing{
		Language:    "go",
		Command:     "refute-nonexistent-lsp-binary-xyz",
		InstallHint: "go install golang.org/x/tools/gopls@latest",
	}
}

func TestExitCodeError_MessageAndCode(t *testing.T) {
	e := &ExitCodeError{Code: 7, Message: "boom"}
	if e.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", e.Error(), "boom")
	}
	if e.ExitCode() != 7 {
		t.Errorf("ExitCode() = %d, want 7", e.ExitCode())
	}
}

func TestNoEditsError(t *testing.T) {
	err := NoEditsError()
	var ec *ExitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("NoEditsError() = %T, want *ExitCodeError", err)
	}
	if ec.Code != noOpExitCode {
		t.Errorf("code = %d, want %d", ec.Code, noOpExitCode)
	}
	if ec.Message == "" {
		t.Error("NoEditsError() message is empty; scripts surface it to users")
	}
}

func TestErrLSPServerMissing_Error(t *testing.T) {
	withHint := &ErrLSPServerMissing{Language: "go", Command: "gopls", InstallHint: "go install gopls"}
	msg := withHint.Error()
	for _, sub := range []string{"gopls", "go", "Install with: go install gopls"} {
		if !strings.Contains(msg, sub) {
			t.Errorf("Error() = %q, missing %q", msg, sub)
		}
	}

	noHint := &ErrLSPServerMissing{Language: "rust", Command: "rust-analyzer"}
	msg = noHint.Error()
	if strings.Contains(msg, "Install with") {
		t.Errorf("Error() = %q, must not mention install when InstallHint is empty", msg)
	}
	if !strings.Contains(msg, "rust-analyzer") || !strings.Contains(msg, "rust") {
		t.Errorf("Error() = %q, want command and language", msg)
	}

	if withHint.ExitCode() != backendMissingExitCode {
		t.Errorf("ExitCode() = %d, want %d", withHint.ExitCode(), backendMissingExitCode)
	}
}

func TestErrBackendInitFailure_ErrorAndUnwrap(t *testing.T) {
	cause := errors.New("handshake timed out")
	named := NewBackendInitFailure("lsp", cause)
	if !strings.Contains(named.Error(), "lsp") || !strings.Contains(named.Error(), cause.Error()) {
		t.Errorf("Error() = %q, want backend name and cause", named.Error())
	}
	if !errors.Is(named, cause) {
		t.Error("NewBackendInitFailure must Unwrap to its cause")
	}

	anon := &ErrBackendInitFailure{Cause: cause}
	if strings.Contains(anon.Error(), "\"\"") {
		t.Errorf("Error() = %q, must not print an empty quoted backend name", anon.Error())
	}
	if !strings.Contains(anon.Error(), cause.Error()) {
		t.Errorf("Error() = %q, want cause", anon.Error())
	}
}

func TestErrSymbolNotFound_Error(t *testing.T) {
	full := &ErrSymbolNotFound{
		Language:   "rust",
		Input:      "crate::a::Trait::greet",
		ModulePath: []string{"a", "b"},
		Trait:      "Trait",
		Name:       "greet",
	}
	msg := full.Error()
	for _, sub := range []string{"rust", "container=a::b", "trait=Trait", "name=greet", `"crate::a::Trait::greet"`} {
		if !strings.Contains(msg, sub) {
			t.Errorf("Error() = %q, missing %q", msg, sub)
		}
	}

	bare := &ErrSymbolNotFound{Language: "rust", Input: "missing", Name: "missing"}
	msg = bare.Error()
	if strings.Contains(msg, "container=") || strings.Contains(msg, "trait=") {
		t.Errorf("Error() = %q, must omit empty container/trait parts", msg)
	}
	if full.ExitCode() != 2 {
		t.Errorf("ExitCode() = %d, want 2", full.ExitCode())
	}
}

func TestExitDetails(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
		wantMsg  string
	}{
		{name: "nil", err: nil, wantCode: 0, wantMsg: ""},
		{name: "generic", err: errors.New("kaboom"), wantCode: 1, wantMsg: "kaboom"},
		{name: "exit-code error", err: &ExitCodeError{Code: 2, Message: "no changes produced"}, wantCode: 2, wantMsg: "no changes produced"},
		{name: "lsp missing", err: backendMissingForTest(), wantCode: backendMissingExitCode, wantMsg: backendMissingForTest().Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, msg := exitDetails(tc.err)
			if code != tc.wantCode {
				t.Errorf("code = %d, want %d", code, tc.wantCode)
			}
			if msg != tc.wantMsg {
				t.Errorf("message = %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

// TestExitDetails_WrappedRuntimeMissing covers the branch where a missing
// adapter runtime carries no ExitCode of its own but must still map to the
// shared backend-missing exit code via the isBackendRuntimeMissing fallback.
func TestExitDetails_WrappedRuntimeMissing(t *testing.T) {
	wrapped := fmt.Errorf("backend setup: %w", &backend.ErrAdapterRuntimeMissing{
		Language:       "typescript",
		AdapterName:    "ts-morph",
		MissingRuntime: "ts-morph node modules",
		InstallHint:    "install node modules",
	})
	code, msg := exitDetails(wrapped)
	if code != backendMissingExitCode {
		t.Fatalf("code = %d, want %d", code, backendMissingExitCode)
	}
	if msg != wrapped.Error() {
		t.Errorf("message = %q, want %q", msg, wrapped.Error())
	}
}

func TestDefaultStatusForError_NoOpAndDefault(t *testing.T) {
	if got := defaultStatusForError(NoEditsError()); got != edit.StatusNoOp {
		t.Errorf("status for no-edits = %q, want %q", got, edit.StatusNoOp)
	}
	if got := defaultStatusForError(errors.New("generic")); got != "failed" {
		t.Errorf("status for generic error = %q, want %q", got, "failed")
	}
}
