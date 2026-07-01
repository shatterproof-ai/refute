package cli

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestParseCallSite(t *testing.T) {
	cases := []struct {
		in      string
		wantF   string // relative path; compared after filepath.Abs
		wantL   int
		wantC   int
		wantErr bool
	}{
		{"src/main.rs:3:14", "src/main.rs", 3, 14, false},
		// A Windows drive path contains a colon; line/col come from the last two
		// fields so the path survives intact.
		{`C:\proj\main.rs:3:14`, `C:\proj\main.rs`, 3, 14, false},
		{"bad", "", 0, 0, true},
		{"src:not-a-number:3", "", 0, 0, true},
		{"src:3:not-a-number", "", 0, 0, true},
		{":3:14", "", 0, 0, true},        // empty path
		{"src.rs:0:14", "", 0, 0, true},  // non-positive line
		{"src.rs:3:0", "", 0, 0, true},   // non-positive column
		{"src.rs:-1:14", "", 0, 0, true}, // negative line
	}
	for _, c := range cases {
		got, err := parseCallSite(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("err=%v wantErr=%v for %q", err, c.wantErr, c.in)
			continue
		}
		if c.wantErr {
			continue
		}
		wantAbs, _ := filepath.Abs(c.wantF)
		if got.File != wantAbs || got.Line != c.wantL || got.Column != c.wantC {
			t.Errorf("%q → %+v (want file %q line %d col %d)", c.in, got, wantAbs, c.wantL, c.wantC)
		}
	}
}

// TestRunInline_InputValidationErrors covers the CLI-facing argument-validation
// branches of runInline that return before any backend is selected. These are
// the paths that feed the inline command and were previously untested (only
// parseCallSite had coverage). Each case asserts a terminal error whose message
// names the offending flag combination.
func TestRunInline_InputValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		flags    *inlineFlags
		wantSub  string
		wantCode int
	}{
		{
			name:     "symbol without call-site",
			flags:    &inlineFlags{Symbol: "Greeter::greet"},
			wantSub:  "--symbol requires --call-site",
			wantCode: 1,
		},
		{
			name:     "unparseable call-site",
			flags:    &inlineFlags{CallSite: "not-a-location"},
			wantSub:  "parse --call-site",
			wantCode: 1,
		},
		{
			name:     "invalid symbol with valid call-site",
			flags:    &inlineFlags{Symbol: "bad name", CallSite: "src/main.rs:3:5"},
			wantSub:  "invalid --symbol",
			wantCode: 1,
		},
		{
			name:     "missing file without call-site",
			flags:    &inlineFlags{Line: 3, Name: "x"},
			wantSub:  "--file is required",
			wantCode: 1,
		},
		{
			name:     "missing line without call-site",
			flags:    &inlineFlags{File: "main.go", Name: "x"},
			wantSub:  "--line is required",
			wantCode: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := runInline(tc.flags, operationFlags{})
			if err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
			if got := exitCodeForError(err); got != tc.wantCode {
				t.Fatalf("exit code = %d, want %d", got, tc.wantCode)
			}
		})
	}
}

// TestRunInline_JSONErrorContract covers the #57 JSON contract on runInline: an
// input-validation error funneled through routeOperationError with --json set
// produces exactly one structured envelope on stdout and returns the jsonEmitted
// sentinel, never a bare error that Run would print a second time.
func TestRunInline_JSONErrorContract(t *testing.T) {
	flags := &inlineFlags{Symbol: "Greeter::greet"} // no --call-site

	var runErr error
	out := captureStdout(t, func() {
		runErr = runInline(flags, operationFlags{JSON: true})
	})

	var emitted *jsonEmitted
	if !errors.As(runErr, &emitted) {
		t.Fatalf("error = %#v, want jsonEmitted", runErr)
	}
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	assertSingleJSONEnvelope(t, out)
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Operation != "inline" {
		t.Errorf("operation = %q, want inline; envelope:\n%s", got.Operation, out)
	}
	if got.Error == nil {
		t.Fatalf("missing error object; envelope:\n%s", out)
	}
}

// TestRunInline_SymbolResolutionJSONInvalidPosition covers the inline-specific
// branch where symbol resolution fails and runInlineInner itself emits an
// invalid-position envelope before returning a jsonEmitted error. The shared
// router must pass that sentinel through unchanged, so stdout holds exactly one
// envelope with status invalid-position.
func TestRunInline_SymbolResolutionJSONInvalidPosition(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.go")

	flags := &inlineFlags{File: missing, Line: 1, Name: "x"}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runInline(flags, operationFlags{JSON: true})
	})

	var emitted *jsonEmitted
	if !errors.As(runErr, &emitted) {
		t.Fatalf("error = %#v, want jsonEmitted", runErr)
	}
	assertSingleJSONEnvelope(t, out)
	assertJSONErrorEnvelope(t, []byte(out), edit.StatusInvalidPosition, "invalid-position")
}
