package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/cobra"
)

// findSubcommand returns the registered command with the given Use name.
func findSubcommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	for _, c := range RootCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("command %q not registered", name)
	return nil
}

// TestVersionJSON covers issue #89 item 2: `version --json` must succeed and
// emit a structured object, and the human form must not have the double space.
func TestVersionJSON(t *testing.T) {
	origJSON := flagJSON
	origV, origC, origB := Version, Commit, BuildDate
	t.Cleanup(func() {
		flagJSON = origJSON
		Version, Commit, BuildDate = origV, origC, origB
	})
	Version, Commit, BuildDate = "9.9.9-test", "deadbeef", "2026-04-30T00:00:00Z"

	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"version", "--json"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("version --json: %v", err)
	}
	var got versionInfo
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("version --json output not parseable: %v\nraw:\n%s", err, buf.String())
	}
	if got.Version != "9.9.9-test" || got.Commit != "deadbeef" || got.BuildDate != "2026-04-30T00:00:00Z" {
		t.Fatalf("unexpected json fields: %+v", got)
	}

	flagJSON = false
	buf.Reset()
	RootCmd.SetArgs([]string{"version"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if strings.Contains(buf.String(), "built:  ") {
		t.Errorf("human version output has a double space after 'built:'\nraw:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "built: 2026-04-30T00:00:00Z") {
		t.Errorf("human version output missing single-spaced built line\nraw:\n%s", buf.String())
	}
}

// TestRequiredFlagMarkersAndURL covers items 1 and 4: required flags are marked
// in --help text and Long strings link to the GitHub URL, not a repo path.
func TestRequiredFlagMarkersAndURL(t *testing.T) {
	rename := findSubcommand(t, "rename")
	if u := rename.Flags().Lookup("new-name").Usage; !strings.Contains(u, "(required)") {
		t.Errorf("rename --new-name usage missing (required): %q", u)
	}
	if !strings.Contains(rename.Long, supportMatrixURL) {
		t.Errorf("rename Long should link to %s, got:\n%s", supportMatrixURL, rename.Long)
	}
	if strings.Contains(rename.Long, "See docs/support-matrix.md") {
		t.Errorf("rename Long still uses the repo-relative path:\n%s", rename.Long)
	}

	extract := findSubcommand(t, "extract-function")
	for _, f := range []string{"file", "start-line", "start-col", "end-line", "end-col"} {
		if u := extract.Flags().Lookup(f).Usage; !strings.Contains(u, "(required)") {
			t.Errorf("extract-function --%s usage missing (required): %q", f, u)
		}
	}
	if !strings.Contains(extract.Long, supportMatrixURL) {
		t.Errorf("extract-function Long should link to the URL, got:\n%s", extract.Long)
	}
}

// TestDoctorLongCapitalized covers item 4: doctor's Long must start uppercase.
func TestDoctorLongCapitalized(t *testing.T) {
	doctor := findSubcommand(t, "doctor")
	r := []rune(doctor.Long)
	if len(r) == 0 || !unicode.IsUpper(r[0]) {
		t.Errorf("doctor Long should start with an uppercase letter, got: %q", doctor.Long)
	}
}

// TestTranslateRenameError covers item 3: backend rename failures are mapped
// into refute vocabulary, with same-name treated as the exit-2 no-op.
func TestTranslateRenameError(t *testing.T) {
	t.Run("same-name maps to exit-2 no-op", func(t *testing.T) {
		err := translateRenameError(
			fmt.Errorf("rename: rename request: JSON-RPC error 0: old and new names are the same: FormatGreeting"),
			"FormatGreeting")
		var ec *ExitCodeError
		if !errors.As(err, &ec) || ec.Code != noOpExitCode {
			t.Fatalf("expected exit-2 ExitCodeError, got %#v", err)
		}
		if strings.Contains(ec.Message, "JSON-RPC") || strings.Contains(ec.Message, "rename failed") {
			t.Errorf("message still leaks plumbing: %q", ec.Message)
		}
		if !strings.Contains(ec.Message, "FormatGreeting") {
			t.Errorf("message should name the symbol: %q", ec.Message)
		}
	})

	t.Run("same-name surfaces a no-op envelope in --json mode", func(t *testing.T) {
		origJSON := flagJSON
		t.Cleanup(func() { flagJSON = origJSON })
		flagJSON = true

		renameErr := translateRenameError(
			fmt.Errorf("rename: rename request: JSON-RPC error 0: old and new names are the same: Foo"),
			"Foo")
		ctx := jsonContext{Operation: "rename", Language: "go", Backend: "lsp", WorkspaceRoot: "/ws"}

		var routed error
		out := captureStdout(t, func() {
			routed = routeOperationError(ctx, renameErr, operationFlagsFromGlobals())
		})
		var ec *ExitCodeError
		if !errors.As(routed, &ec) || ec.Code != noOpExitCode {
			t.Fatalf("expected exit-2 ExitCodeError, got %#v", routed)
		}
		if !strings.Contains(out, `"status": "no-op"`) {
			t.Errorf("expected no-op envelope, got:\n%s", out)
		}
		if strings.Contains(out, "JSON-RPC") {
			t.Errorf("envelope leaks plumbing:\n%s", out)
		}
	})

	t.Run("invalid identifier is humanized", func(t *testing.T) {
		err := translateRenameError(
			fmt.Errorf("rename: rename request: JSON-RPC error 0: \"1bad\" is not a valid identifier"),
			"1bad")
		if strings.Contains(err.Error(), "JSON-RPC") {
			t.Errorf("message still leaks JSON-RPC: %q", err.Error())
		}
		if !strings.Contains(err.Error(), "1bad") || !strings.Contains(err.Error(), "valid identifier") {
			t.Errorf("unexpected message: %q", err.Error())
		}
	})

	t.Run("generic failure strips plumbing prefixes", func(t *testing.T) {
		err := translateRenameError(
			fmt.Errorf("rename: rename request: JSON-RPC error 0: backend exploded"),
			"X")
		got := err.Error()
		if strings.Contains(got, "JSON-RPC") || strings.Contains(got, "rename request:") {
			t.Errorf("message still leaks plumbing: %q", got)
		}
		if !strings.HasPrefix(got, "rename failed: ") || !strings.Contains(got, "backend exploded") {
			t.Errorf("unexpected message: %q", got)
		}
	})
}
