package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/telemetry"
)

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

// NoEditsError is returned when a refactoring produced no changes. Exit 2 is
// the refute convention for "nothing to do" (useful for scripting).
func NoEditsError() error {
	return &ExitCodeError{Code: 2, Message: "no changes produced"}
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
//	nil                  → 0
//	*ExitCodeError       → e.Code (message printed to stderr only if non-empty)
//	*ErrLSPServerMissing → 3 (message printed to stderr)
//	anything else        → 1 (message printed to stderr)
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
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code, ec.Message
	}
	var em *ErrLSPServerMissing
	if errors.As(err, &em) {
		return 3, em.Error()
	}
	return 1, err.Error()
}

func defaultStatusForError(err error) string {
	var ec *ExitCodeError
	if errors.As(err, &ec) && ec.Code == 2 {
		return edit.StatusNoOp
	}
	var em *ErrLSPServerMissing
	if errors.As(err, &em) {
		return edit.StatusBackendMissing
	}
	return "failed"
}
