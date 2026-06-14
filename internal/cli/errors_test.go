package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

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
		{name: "json backend missing", mode: "json-backend-missing", wantCode: 3, wantStatus: edit.StatusBackendMissing, wantErr: "backend-unavailable"},
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
		return emitJSONError(ctx, backendErrorStatus(err), "backend-unavailable", err.Error(), "Run `refute doctor` for backend setup details.")
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
