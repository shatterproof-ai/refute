package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/telemetry"
)

// backendMissingExitCode is the conventional process exit code for "a required
// backend tool is absent" — shared by ErrLSPServerMissing and the adapter
// runtime-missing path so scripts can branch on "install something" uniformly.
const backendMissingExitCode = 3

// noOpExitCode is the refute convention for "nothing to do" — no edits produced
// or no matching symbol — so scripts can distinguish it from a real failure.
const noOpExitCode = 2

// ExitCodeError carries a requested process exit code alongside an optional
// message. Commands return this instead of calling os.Exit so deferred
// cleanup (Shutdown, file close) always runs.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string {
	return e.Message
}

func (e *ExitCodeError) ExitCode() int { return e.Code }

type exitCoder interface {
	ExitCode() int
}

// NoEditsError is returned when a refactoring produced no changes. Exit 2 is
// the refute convention for "nothing to do" (useful for scripting).
func NoEditsError() error {
	return &ExitCodeError{Code: noOpExitCode, Message: "no changes produced"}
}

// ErrLSPServerMissing signals that the LSP server binary is not on PATH.
// Exit code 3 distinguishes this from other errors (1 = general, 2 = no edits).
type ErrLSPServerMissing struct {
	Language    string
	Command     string
	InstallHint string
}

func (e *ErrLSPServerMissing) Error() string {
	if e.InstallHint != "" {
		return fmt.Sprintf("LSP server %q for %s not found on PATH. Install with: %s",
			e.Command, e.Language, e.InstallHint)
	}
	return fmt.Sprintf("LSP server %q for %s not found on PATH", e.Command, e.Language)
}

func (e *ErrLSPServerMissing) ExitCode() int { return backendMissingExitCode }

// ErrBackendInitFailure wraps an error returned by a backend's Initialize so
// callers can distinguish "the backend failed to start" from a missing tool or
// an unsupported operation. Cause is preserved and exposed via Unwrap, so a
// wrapped typed error (such as backend.ErrAdapterRuntimeMissing or
// ErrLSPServerMissing) still satisfies errors.As/errors.Is and its more specific
// classification wins over the generic "backend crashed" mapping.
type ErrBackendInitFailure struct {
	Backend string
	Cause   error
}

// NewBackendInitFailure wraps a backend Initialize error with the backend name.
func NewBackendInitFailure(backendName string, cause error) *ErrBackendInitFailure {
	return &ErrBackendInitFailure{Backend: backendName, Cause: cause}
}

func (e *ErrBackendInitFailure) Error() string {
	if e.Backend != "" {
		return fmt.Sprintf("backend %q failed to initialize: %v", e.Backend, e.Cause)
	}
	return fmt.Sprintf("backend failed to initialize: %v", e.Cause)
}

func (e *ErrBackendInitFailure) Unwrap() error { return e.Cause }

// isBackendRuntimeMissing reports whether err (or anything it wraps) is a
// missing-backend-tool error: a missing LSP server or a missing adapter
// runtime. These all map to exit code 3 and status backend-missing.
func isBackendRuntimeMissing(err error) bool {
	var lsp *ErrLSPServerMissing
	var rt *backend.ErrAdapterRuntimeMissing
	return errors.As(err, &lsp) || errors.As(err, &rt)
}

// ErrSymbolNotFound is returned by the CLI Rust rename path when no symbol
// matches the parsed --symbol query. Exit code 2 signals "no match found".
type ErrSymbolNotFound struct {
	Language   string
	Input      string
	ModulePath []string
	Trait      string
	Name       string
}

func (e *ErrSymbolNotFound) Error() string {
	var parts []string
	if len(e.ModulePath) > 0 {
		parts = append(parts, "container="+strings.Join(e.ModulePath, "::"))
	}
	if e.Trait != "" {
		parts = append(parts, "trait="+e.Trait)
	}
	parts = append(parts, "name="+e.Name)
	return fmt.Sprintf("no %s symbol matched %s (input: %q)",
		e.Language, strings.Join(parts, " "), e.Input)
}

func (e *ErrSymbolNotFound) ExitCode() int { return 2 }

// Run executes fn and maps any returned error to an exit code:
//
//	nil                   → 0
//	interface{ExitCode()} → ExitCode() (message printed to stderr only if non-empty)
//	anything else         → 1 (message printed to stderr)
func Run(fn func() error) {
	cwd, _ := os.Getwd()
	rec := telemetry.Start(os.Args[1:], cwd, telemetry.Options{Verbose: argsContainVerbose(os.Args[1:])})
	setActiveTelemetry(rec)

	err := fn()
	code, message := exitDetails(err)
	if err != nil {
		telemetrySetDefaultStatus(defaultStatusForError(err))
		telemetrySetDefaultError("error", err.Error())
	}
	rec.Finish(telemetry.FinishInfo{ExitCode: code})
	if message != "" {
		fmt.Fprintln(os.Stderr, message)
	}
	os.Exit(code)
}

func exitDetails(err error) (int, string) {
	if err == nil {
		return 0, ""
	}
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode(), exitMessage(err, ec)
	}
	// A missing adapter runtime carries no ExitCode of its own (it originates in
	// the backend layer), so map it here to the shared backend-missing code.
	if isBackendRuntimeMissing(err) {
		return backendMissingExitCode, err.Error()
	}
	return 1, err.Error()
}

func exitMessage(err error, ec exitCoder) string {
	switch e := ec.(type) {
	case *ExitCodeError:
		return e.Message
	case *ErrLSPServerMissing:
		return e.Error()
	default:
		return err.Error()
	}
}

func defaultStatusForError(err error) string {
	if isBackendRuntimeMissing(err) {
		return edit.StatusBackendMissing
	}
	var initFail *ErrBackendInitFailure
	if errors.As(err, &initFail) {
		return edit.StatusBackendFailed
	}
	var ec exitCoder
	if errors.As(err, &ec) && ec.ExitCode() == 2 {
		return edit.StatusNoOp
	}
	return "failed"
}
