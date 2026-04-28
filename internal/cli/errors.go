package cli

import (
	"errors"
	"fmt"
	"os"
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

// Run executes fn and maps any returned error to an exit code:
//
//	nil            → 0
//	*ExitCodeError → e.Code (message printed to stderr only if non-empty)
//	anything else  → 1 (message printed to stderr)
func Run(fn func() error) {
	err := fn()
	if err == nil {
		os.Exit(0)
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		if ec.Message != "" {
			fmt.Fprintln(os.Stderr, ec.Message)
		}
		os.Exit(ec.Code)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
