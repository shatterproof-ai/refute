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
	Operation      string
	Language       string
	Backend        string
	BackendVersion string
	WorkspaceRoot  string
}

func contextFromSelection(operation string, sel *selector.Selection, workspaceRoot string) jsonContext {
	ctx := jsonContext{Operation: operation, WorkspaceRoot: workspaceRoot}
	if sel != nil {
		ctx.Language = sel.Language
		ctx.Backend = sel.BackendName
		ctx.BackendVersion = backendVersionForSelection(sel)
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
			ctx.Language = DetectLanguageName(abs)
		}
	}
	return ctx
}

func statusForFlags(opts operationFlags) string {
	if opts.DryRun {
		return edit.StatusDryRun
	}
	return edit.StatusApplied
}

// jsonEmitted wraps an error whose structured JSON envelope has already been
// written to stdout. Routing wrappers detect it (via errors.As) and pass it
// through unchanged instead of emitting a second envelope. It unwraps to the
// underlying error so exit-code mapping and errors.As(&ExitCodeError) keep
// working transparently.
type jsonEmitted struct{ err error }

func (e *jsonEmitted) Error() string { return e.err.Error() }
func (e *jsonEmitted) Unwrap() error { return e.err }

// routeOperationError converts a terminal operation error into exactly one
// structured JSON envelope when --json is set. Errors whose envelope was
// already written (jsonEmitted) pass through untouched so no second envelope is
// printed; outside --json mode the error is returned verbatim for stderr
// rendering by Run. This is the shared wrapper that rename, extract-function,
// extract-variable, and inline funnel their terminal errors through.
func routeOperationError(ctx jsonContext, err error, opts operationFlags) error {
	if err == nil || !opts.JSON {
		return err
	}
	var emitted *jsonEmitted
	if errors.As(err, &emitted) {
		return err
	}
	return emitJSONOperationError(ctx, err)
}

// emitJSONError writes a structured error envelope to stdout and returns an
// ExitCodeError so Run() exits with a non-zero status without printing the
// message twice. Intended for use only when flagJSON is set.
func emitJSONError(ctx jsonContext, status, code, message, hint string, exitCode ...int) error {
	telemetrySetContext(ctx)
	telemetrySetStatus(status)
	telemetrySetError(code, message)
	res := &edit.JSONResult{
		SchemaVersion:  edit.SchemaVersion,
		Status:         status,
		Operation:      ctx.Operation,
		Language:       ctx.Language,
		Backend:        ctx.Backend,
		BackendVersion: ctx.BackendVersion,
		WorkspaceRoot:  ctx.WorkspaceRoot,
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
	return &jsonEmitted{err: &ExitCodeError{Code: jsonErrorExitCode(status, exitCode...)}}
}

func jsonErrorExitCode(status string, exitCode ...int) int {
	if len(exitCode) > 0 {
		return exitCode[0]
	}
	switch status {
	case edit.StatusNoOp:
		return 2
	case edit.StatusBackendMissing:
		return 3
	default:
		return 1
	}
}

func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}

// languageUnsupportedHint is the remediation shown when an operation targets a
// language the support matrix marks unsupported. It points at doctor and the
// matrix doc rather than at backend installation, since no backend is claimed.
const languageUnsupportedHint = "Run `refute doctor` to see which languages are supported. See " + supportMatrixURL + "."

// unsupportedOperationHint is shown when a language/backend exists but none of
// its configured candidates supports the requested operation.
const unsupportedOperationHint = "Run `refute doctor` to see supported operations for each backend. See " + supportMatrixURL + "."

// emitLanguageUnsupportedError emits the documented unsupported envelope for a
// language the support matrix marks unsupported (gated in selection before any
// backend init). It returns false if err is not an ErrLanguageUnsupported so
// callers can fall through to their existing classification.
func emitLanguageUnsupportedError(ctx jsonContext, err error) (error, bool) {
	var langUnsupported *selector.ErrLanguageUnsupported
	if !errors.As(err, &langUnsupported) {
		return nil, false
	}
	return emitJSONError(ctx, edit.StatusUnsupported, "unsupported-language", err.Error(), languageUnsupportedHint), true
}

func backendErrorStatus(err error) string {
	if isBackendRuntimeMissing(err) {
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

// emitJSONBackendSetupError classifies a backend setup or initialization error
// and emits the matching structured envelope. It distinguishes a missing
// adapter runtime (install hint, exit 3) and a missing LSP server (exit 3) from
// a generic backend initialization failure (exit 1). Errors that are none of
// these fall back to backendErrorStatus heuristics.
func emitJSONBackendSetupError(ctx jsonContext, err error) error {
	var runtimeMissing *backend.ErrAdapterRuntimeMissing
	if errors.As(err, &runtimeMissing) {
		return emitJSONError(ctx, edit.StatusBackendMissing, "adapter-runtime-missing",
			err.Error(), runtimeMissing.InstallHint, backendMissingExitCode)
	}
	var lspMissing *ErrLSPServerMissing
	if errors.As(err, &lspMissing) {
		return emitJSONError(ctx, edit.StatusBackendMissing, "backend-missing",
			err.Error(), lspMissing.InstallHint, backendMissingExitCode)
	}
	var initFail *ErrBackendInitFailure
	if errors.As(err, &initFail) {
		return emitJSONError(ctx, edit.StatusBackendFailed, "backend-init-failed",
			err.Error(), "Run `refute doctor` for backend setup details.")
	}
	status := backendErrorStatus(err)
	code := "backend-unavailable"
	if status == edit.StatusBackendMissing {
		code = "backend-missing"
	}
	return emitJSONError(ctx, status, code,
		err.Error(), "Run `refute doctor` for backend setup details.")
}

// emitJSONOperationError is the shared router that maps a terminal operation
// error to a structured envelope. It recognizes every failure category an
// operation can surface so rename, extract, and inline all report identical
// statuses for identical causes. Backend setup and initialization errors are
// delegated to emitJSONBackendSetupError so the typed adapter/LSP/init
// distinctions are honored regardless of which operation surfaced them.
func emitJSONOperationError(ctx jsonContext, err error) error {
	if emitted, ok := emitLanguageUnsupportedError(ctx, err); ok {
		return emitted
	}
	var ec exitCoder
	var symbolMissing *ErrSymbolNotFound
	var kindMismatch *ErrKindMismatch
	switch {
	case errors.As(err, &kindMismatch):
		return emitJSONError(ctx, edit.StatusKindMismatch, "kind-mismatch", err.Error(), kindMismatch.Hint(), kindMismatch.ExitCode())
	case isBackendSetupError(err):
		return emitJSONBackendSetupError(ctx, err)
	// SelectForOperation refuses unsupported operations before backend setup;
	// backend.ErrUnsupported is the equivalent refusal from a backend that was
	// already selected. Consumers see the same JSON contract for both.
	case errors.Is(err, selector.ErrOperationUnsupported):
		return emitJSONError(ctx, edit.StatusUnsupported, "unsupported-operation", err.Error(), unsupportedOperationHint)
	case errors.Is(err, backend.ErrUnsupported):
		return emitJSONError(ctx, edit.StatusUnsupported, "unsupported-operation", err.Error(), unsupportedOperationHint)
	case errors.Is(err, backend.ErrSymbolNotFound):
		return emitJSONError(ctx, edit.StatusInvalidPosition, "symbol-not-found", err.Error(), "", 2)
	case errors.As(err, &symbolMissing):
		return emitJSONError(ctx, edit.StatusInvalidPosition, "symbol-not-found", err.Error(), "", symbolMissing.ExitCode())
	case errors.As(err, &ec) && ec.ExitCode() == 2:
		return emitJSONError(ctx, edit.StatusNoOp, "no-op", err.Error(), "", ec.ExitCode())
	default:
		status := backendErrorStatus(err)
		return emitJSONError(ctx, status, operationErrorCode(status), err.Error(), backendStatusHint(status), exitCodeForError(err))
	}
}

// isBackendSetupError reports whether err is one of the typed backend setup or
// initialization failures that emitJSONBackendSetupError classifies.
func isBackendSetupError(err error) bool {
	var runtimeMissing *backend.ErrAdapterRuntimeMissing
	var lspMissing *ErrLSPServerMissing
	var initFail *ErrBackendInitFailure
	return errors.As(err, &runtimeMissing) ||
		errors.As(err, &lspMissing) ||
		errors.As(err, &initFail)
}

// operationErrorCode picks the error.code for a generic operation failure from
// the resolved status: a missing backend reports backend-missing, everything
// else reports the operation-failed catch-all.
func operationErrorCode(status string) string {
	if status == edit.StatusBackendMissing {
		return "backend-missing"
	}
	return "operation-failed"
}

// backendStatusHint returns the remediation hint for backend-related statuses.
func backendStatusHint(status string) string {
	if status == edit.StatusBackendMissing {
		return "Run `refute doctor` for backend setup details."
	}
	return ""
}
