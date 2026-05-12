package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/edit"
)

// jsonContext carries the operation, language, backend, and workspace root
// metadata that get attached to every JSON envelope so consumers do not need
// to scrape stderr to know what was attempted.
type jsonContext struct {
	Operation     string
	Language      string
	Backend       string
	WorkspaceRoot string
}

func contextFromSelection(operation string, sel *selector.Selection, workspaceRoot string) jsonContext {
	ctx := jsonContext{Operation: operation, WorkspaceRoot: workspaceRoot}
	if sel != nil {
		ctx.Language = sel.Language
		ctx.Backend = sel.BackendName
	}
	return ctx
}

func contextFromFile(operation, filePath string) jsonContext {
	ctx := jsonContext{Operation: operation}
	if filePath != "" {
		if abs, err := filepath.Abs(filePath); err == nil {
			if root, err := FindWorkspaceRootFromFile(abs); err == nil {
				ctx.WorkspaceRoot = root
			}
			ctx.Language = DetectServerKey(abs)
		}
	}
	return ctx
}

func statusForFlags() string {
	if flagDryRun {
		return edit.StatusDryRun
	}
	return edit.StatusApplied
}

// emitJSONError writes a structured error envelope to stdout and returns an
// ExitCodeError so Run() exits with a non-zero status without printing the
// message twice. Intended for use only when flagJSON is set.
func emitJSONError(ctx jsonContext, status, code, message, hint string) error {
	res := &edit.JSONResult{
		SchemaVersion: edit.SchemaVersion,
		Status:        status,
		Operation:     ctx.Operation,
		Language:      ctx.Language,
		Backend:       ctx.Backend,
		WorkspaceRoot: ctx.WorkspaceRoot,
		Error: &edit.JSONError{
			Code:    code,
			Message: message,
			Hint:    hint,
		},
	}
	data, err := res.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling JSON error: %w", err)
	}
	fmt.Println(string(data))
	return &ExitCodeError{Code: 1}
}

func backendErrorStatus(err error) string {
	var missing *ErrLSPServerMissing
	if errors.As(err, &missing) {
		return edit.StatusBackendMissing
	}
	msg := err.Error()
	if strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no server configured") ||
		strings.Contains(msg, "not found on PATH") {
		return edit.StatusBackendMissing
	}
	return edit.StatusBackendFailed
}

func emitJSONOperationError(ctx jsonContext, err error) error {
	var ec *ExitCodeError
	switch {
	case errors.Is(err, backend.ErrUnsupported):
		return emitJSONError(ctx, edit.StatusUnsupported, "unsupported-operation", err.Error(), "")
	case errors.Is(err, backend.ErrSymbolNotFound):
		return emitJSONError(ctx, edit.StatusInvalidPosition, "symbol-not-found", err.Error(), "")
	case errors.As(err, &ec) && ec.Code == 2:
		return emitJSONError(ctx, edit.StatusNoOp, "no-op", err.Error(), "")
	default:
		return emitJSONError(ctx, edit.StatusBackendFailed, "operation-failed", err.Error(), "")
	}
}
