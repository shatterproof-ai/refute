package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
)

// newMoveValidateCmd builds a cobra command with the move flags registered so
// validateMoveFlags sees accurate Changed() state after ParseFlags.
func newMoveValidateCmd(flags *moveFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "move"}
	cmd.Flags().StringVar(&flags.File, "file", "", "")
	cmd.Flags().IntVar(&flags.Line, "line", 0, "")
	cmd.Flags().IntVar(&flags.Col, "col", 0, "")
	cmd.Flags().StringVar(&flags.Name, "name", "", "")
	cmd.Flags().StringVar(&flags.Destination, "destination", "", "")
	return cmd
}

// TestValidateMoveFlags covers the PreRunE input checks for the move command:
// --file, --line, and --destination are required, and coordinates must be
// positive. Validation must reject bad input before any backend is started.
func TestValidateMoveFlags(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := filepath.Join(dir, "orig.go")
	if err := os.WriteFile(existing, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		args    []string
		wantSub string
	}{
		{"missing file", []string{"--line", "1", "--destination", "d.go"}, "--file is required"},
		{"missing line", []string{"--file", existing, "--destination", "d.go"}, "--line is required"},
		{"missing destination", []string{"--file", existing, "--line", "1"}, "--destination is required"},
		{"nonexistent file", []string{"--file", filepath.Join(dir, "nope.go"), "--line", "1", "--destination", "d.go"}, "does not exist"},
		{"nonpositive line", []string{"--file", existing, "--line", "0", "--destination", "d.go"}, "--line must be >= 1"},
		{"nonpositive col", []string{"--file", existing, "--line", "1", "--col", "0", "--destination", "d.go"}, "--col must be >= 1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			flags := &moveFlags{}
			cmd := newMoveValidateCmd(flags)
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatalf("ParseFlags: %v", err)
			}
			err := validateMoveFlags(cmd, flags)
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

// TestValidateMoveFlags_Accepts confirms a well-formed invocation passes
// validation.
func TestValidateMoveFlags_Accepts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	existing := filepath.Join(dir, "orig.go")
	if err := os.WriteFile(existing, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flags := &moveFlags{}
	cmd := newMoveValidateCmd(flags)
	if err := cmd.ParseFlags([]string{"--file", existing, "--line", "1", "--name", "X", "--destination", "moved.go"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if err := validateMoveFlags(cmd, flags); err != nil {
		t.Fatalf("validateMoveFlags rejected a valid invocation: %v", err)
	}
}

// TestRunMove_SymbolResolutionJSONInvalidPosition covers the move-specific
// branch where symbol resolution fails and runMoveInner emits an
// invalid-position envelope before any backend is selected. The shared router
// passes the sentinel through so stdout holds exactly one envelope.
func TestRunMove_SymbolResolutionJSONInvalidPosition(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.go")
	flags := &moveFlags{File: missing, Line: 1, Name: "x", Destination: "moved.go"}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runMove(flags, operationFlags{JSON: true})
	})

	var emitted *jsonEmitted
	if !errors.As(runErr, &emitted) {
		t.Fatalf("error = %#v, want jsonEmitted", runErr)
	}
	assertSingleJSONEnvelope(t, out)
	assertJSONErrorEnvelope(t, []byte(out), edit.StatusInvalidPosition, "invalid-position")
}

// TestEmitJSONOperationError_UnsafeRefactor pins the refusal contract's JSON
// mapping: a backend.ErrUnsafeRefactor (even wrapped) maps to status
// unsupported with error code unsafe-refactor and carries the reason as hint.
func TestEmitJSONOperationError_UnsafeRefactor(t *testing.T) {
	ctx := jsonContext{Operation: "move"}
	unsafe := &backend.ErrUnsafeRefactor{
		Operation: "move",
		Code:      "cross-package-move",
		Reason:    "destination is in a different directory than source",
	}
	wrapped := fmt.Errorf("move failed: %w", unsafe)

	var got edit.JSONResult
	out := captureStdout(t, func() {
		_ = emitJSONOperationError(ctx, wrapped)
	})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusUnsupported {
		t.Errorf("status = %q, want %q", got.Status, edit.StatusUnsupported)
	}
	if got.Error == nil || got.Error.Code != "unsafe-refactor" {
		t.Fatalf("error = %+v, want code unsafe-refactor", got.Error)
	}
	if got.Error.Hint != unsafe.Reason {
		t.Errorf("hint = %q, want %q", got.Error.Hint, unsafe.Reason)
	}
}
