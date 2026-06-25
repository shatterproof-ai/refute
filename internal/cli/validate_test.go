package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// addInlineTestFlags mirrors the inline command's flag registration so tests can
// drive validateLocationFlags without the global command tree.
func addInlineTestFlags(cmd *cobra.Command, flags *inlineFlags) {
	cmd.Flags().StringVar(&flags.File, "file", "", "")
	cmd.Flags().IntVar(&flags.Line, "line", 0, "")
	cmd.Flags().IntVar(&flags.Col, "col", 0, "")
	cmd.Flags().StringVar(&flags.Name, "name", "", "")
	cmd.Flags().StringVar(&flags.Symbol, "symbol", "", "")
	cmd.Flags().StringVar(&flags.CallSite, "call-site", "", "")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "")
}

// validateWith builds a command with the given flag registrar, applies the
// supplied flag values (which marks them Changed), and runs the mode's
// validator. existingFile, when non-empty, is substituted for the literal
// "<EXISTING>" placeholder so a real on-disk path can be injected per case.
func validateWith(t *testing.T, mode locationMode, set map[string]string) error {
	t.Helper()
	flagJSON = false
	cmd := &cobra.Command{Use: "test"}
	var flags any
	switch mode {
	case modeRename:
		rename := &renameFlags{}
		addRenameFlags(cmd, rename)
		flags = rename
	case modeExtract:
		extract := &extractFlags{}
		addExtractFlags(cmd, extract)
		flags = extract
	case modeInline:
		inline := &inlineFlags{}
		addInlineTestFlags(cmd, inline)
		flags = inline
	default:
		t.Fatalf("unknown validation mode %d", mode)
	}
	for k, v := range set {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s=%q: %v", k, v, err)
		}
	}
	return validateLocationFlags(cmd, mode, flags)
}

func TestValidateRenameFlags(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(existing, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		set     map[string]string
		wantErr string // substring; "" means expect success
	}{
		{"symbol and position together", map[string]string{"symbol": "pkg.Foo", "file": existing, "line": "1"}, "not both"},
		{"no addressing at all", map[string]string{}, "specify the symbol"},
		{"file without line", map[string]string{"file": existing}, "--line is required"},
		{"nonexistent file", map[string]string{"file": "/no/such/file.go", "line": "1"}, "does not exist"},
		{"directory as file", map[string]string{"file": t.TempDir(), "line": "1"}, "is a directory"},
		{"zero line", map[string]string{"file": existing, "line": "0"}, ">= 1"},
		{"negative col", map[string]string{"file": existing, "line": "1", "col": "-3"}, ">= 1"},
		{"malformed rust symbol", map[string]string{"symbol": "Foo::"}, "invalid --symbol"},
		{"valid position", map[string]string{"file": existing, "line": "3"}, ""},
		{"valid rust naked symbol", map[string]string{"symbol": "mod::Thing::run"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateWith(t, modeRename, c.set)
			assertErrContains(t, err, c.wantErr)
		})
	}
}

func TestValidateExtractFlags(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(existing, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := func(extra map[string]string) map[string]string {
		m := map[string]string{"file": existing, "start-line": "1", "start-col": "1", "end-line": "2", "end-col": "5"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	cases := []struct {
		name    string
		set     map[string]string
		wantErr string
	}{
		{"nonexistent file", base(map[string]string{"file": "/no/such.go"}), "does not exist"},
		{"zero start-line", base(map[string]string{"start-line": "0"}), ">= 1"},
		{"start after end (line)", base(map[string]string{"start-line": "5", "end-line": "3"}), "after end"},
		{"start after end (same line, col)", base(map[string]string{"start-line": "2", "start-col": "9", "end-line": "2", "end-col": "4"}), "after end"},
		{"valid range", base(nil), ""},
		{"valid single point", base(map[string]string{"start-line": "2", "start-col": "5", "end-line": "2", "end-col": "5"}), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateWith(t, modeExtract, c.set)
			assertErrContains(t, err, c.wantErr)
		})
	}
}

func TestValidateInlineFlags(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(existing, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		set     map[string]string
		wantErr string
	}{
		{"symbol without call-site", map[string]string{"symbol": "Greeter::greet"}, "requires --call-site"},
		{"call-site and position together", map[string]string{"call-site": existing + ":3:4", "file": existing, "line": "1"}, "not both"},
		{"malformed call-site", map[string]string{"call-site": "nope"}, "invalid --call-site"},
		{"call-site non-positive line", map[string]string{"call-site": existing + ":0:4"}, "invalid --call-site"},
		{"file without line", map[string]string{"file": existing}, "--line is required"},
		{"nonexistent file", map[string]string{"file": "/no/such.go", "line": "1"}, "does not exist"},
		{"valid position", map[string]string{"file": existing, "line": "3"}, ""},
		{"valid call-site", map[string]string{"call-site": existing + ":3:4"}, ""},
		{"valid symbol with call-site", map[string]string{"symbol": "Greeter::greet", "call-site": existing + ":3:4"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateWith(t, modeInline, c.set)
			assertErrContains(t, err, c.wantErr)
		})
	}
}

func TestValidateLocationFlags_WrongFlagTypeReturnsError(t *testing.T) {
	cases := []struct {
		name string
		mode locationMode
		got  any
		want string
	}{
		{"rename", modeRename, &extractFlags{}, "rename validator received"},
		{"extract", modeExtract, &inlineFlags{}, "extract validator received"},
		{"inline", modeInline, &renameFlags{}, "inline validator received"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateLocationFlags(&cobra.Command{Use: "test"}, c.mode, c.got)
			assertErrContains(t, err, c.want)
		})
	}
}

func TestInferTier1Language(t *testing.T) {
	t.Run("file extension wins", func(t *testing.T) {
		lang, err := inferTier1Language("Thing", "/tmp/whatever/main.rs")
		if err != nil || lang != "rust" {
			t.Fatalf("got (%q, %v), want rust", lang, err)
		}
	})
	t.Run("double-colon implies rust", func(t *testing.T) {
		lang, err := inferTier1Language("mod::Thing::run", "")
		if err != nil || lang != "rust" {
			t.Fatalf("got (%q, %v), want rust", lang, err)
		}
	})
	t.Run("workspace marker implies go", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		lang, err := inferTier1Language("pkg.Func", "")
		if err != nil || lang != "go" {
			t.Fatalf("got (%q, %v), want go", lang, err)
		}
	})
	t.Run("no file, no separator, no marker errors", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := inferTier1Language("Func", "")
		if err == nil || !strings.Contains(err.Error(), "cannot infer a language") {
			t.Fatalf("expected inference error, got %v", err)
		}
	})
}

func TestDetectLanguageFromDir(t *testing.T) {
	goDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rustDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rustDir, "Cargo.toml"), []byte("[package]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectLanguageFromDir(goDir); got != "go" {
		t.Errorf("go dir: got %q, want go", got)
	}
	if got := DetectLanguageFromDir(rustDir); got != "rust" {
		t.Errorf("rust dir: got %q, want rust", got)
	}
	if got := DetectLanguageFromDir(t.TempDir()); got != "" {
		t.Errorf("bare dir: got %q, want empty", got)
	}

	// A polyglot root (both markers) is ambiguous and must not be guessed.
	mixed := t.TempDir()
	if err := os.WriteFile(filepath.Join(mixed, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mixed, "Cargo.toml"), []byte("[package]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := DetectLanguageFromDir(mixed); got != "" {
		t.Errorf("mixed dir: got %q, want empty (ambiguous)", got)
	}
}

// TestCommandsRejectPositionalArgs confirms every operation command wires an
// Args validator that rejects stray positional arguments instead of silently
// ignoring them.
func TestCommandsRejectPositionalArgs(t *testing.T) {
	for _, name := range []string{"rename", "rename-function", "extract-function", "extract-variable", "inline"} {
		t.Run(name, func(t *testing.T) {
			cmd := findSubcommand(t, name)
			if cmd.Args == nil {
				t.Fatalf("%s has no Args validator", name)
			}
			if err := cmd.Args(cmd, []string{"stray"}); err == nil {
				t.Errorf("%s accepted a positional argument", name)
			}
			if err := cmd.Args(cmd, nil); err != nil {
				t.Errorf("%s rejected zero positionals: %v", name, err)
			}
		})
	}
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
