package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

// locationMode selects which addressing rules validateLocationFlags enforces.
// Each operation command wires one mode as its PreRunE so invalid input is
// rejected with a clear message before any backend is started.
type locationMode int

const (
	modeRename locationMode = iota
	modeExtract
	modeInline
)

// validateLocationFlags is the shared PreRunE input check for the rename,
// extract, and inline commands. It rejects: a nonexistent or directory --file,
// non-positive 1-indexed coordinates, a start that comes after the end of an
// extract range, mutually-exclusive addressing flags used together, and a
// malformed or unroutable --symbol. Every rejection is a plain error (exit 1);
// in --json mode Execute wraps it into an invalid-request envelope.
func validateLocationFlags(cmd *cobra.Command, mode locationMode) error {
	switch mode {
	case modeRename:
		return validateRenameFlags(cmd)
	case modeExtract:
		return validateExtractFlags(cmd)
	case modeInline:
		return validateInlineFlags(cmd)
	default:
		return nil
	}
}

func validateRenameFlags(cmd *cobra.Command) error {
	symbolSet := flagSymbol != ""
	positionSet := anyChanged(cmd, "file", "line", "col", "name")

	if symbolSet && positionSet {
		return fmt.Errorf("use either --symbol or --file/--line position addressing, not both")
	}
	if !symbolSet && !positionSet {
		return fmt.Errorf("specify the symbol to rename: a qualified name with --symbol, or a position with --file and --line")
	}

	if symbolSet {
		if err := validateSymbolValue(flagSymbol); err != nil {
			return err
		}
		// --symbol reaches here only without position flags (the mutual-exclusion
		// check above already rejected the combination), so this is always a
		// naked symbol that must be routable to a language backend.
		if _, err := inferTier1Language(flagSymbol, ""); err != nil {
			return err
		}
		return nil
	}

	// Position addressing: --file with --line, then --col or --name.
	if flagFile == "" {
		return fmt.Errorf("--file is required for position addressing")
	}
	if !cmd.Flags().Changed("line") {
		return fmt.Errorf("--line is required with --file")
	}
	if err := validateFileExists(flagFile); err != nil {
		return err
	}
	if err := validatePositiveCoord(cmd, "line", flagLine); err != nil {
		return err
	}
	return validatePositiveCoord(cmd, "col", flagCol)
}

func validateExtractFlags(cmd *cobra.Command) error {
	if err := validateFileExists(flagFile); err != nil {
		return err
	}
	for _, c := range []struct {
		flag string
		val  int
	}{
		{"start-line", flagStartLine},
		{"start-col", flagStartCol},
		{"end-line", flagEndLine},
		{"end-col", flagEndCol},
	} {
		if err := validatePositiveCoord(cmd, c.flag, c.val); err != nil {
			return err
		}
	}
	// The selection start must not come after its end.
	if flagStartLine > flagEndLine ||
		(flagStartLine == flagEndLine && flagStartCol > flagEndCol) {
		return fmt.Errorf("selection start %d:%d is after end %d:%d",
			flagStartLine, flagStartCol, flagEndLine, flagEndCol)
	}
	return nil
}

func validateInlineFlags(cmd *cobra.Command) error {
	symbolSet := flagSymbol != ""
	callSiteSet := callSiteFlag != ""
	positionSet := anyChanged(cmd, "file", "line", "col", "name")

	if symbolSet && !callSiteSet {
		return fmt.Errorf("--symbol requires --call-site <file>:<line>:<column> to disambiguate which call site to inline")
	}
	if callSiteSet && positionSet {
		return fmt.Errorf("use either --call-site or --file/--line position addressing, not both")
	}

	if callSiteSet {
		if symbolSet {
			if err := validateSymbolValue(flagSymbol); err != nil {
				return err
			}
		}
		if _, err := parseCallSite(callSiteFlag); err != nil {
			return fmt.Errorf("invalid --call-site: %w", err)
		}
		return nil
	}

	// Position addressing requires --file and --line.
	if flagFile == "" {
		return fmt.Errorf("--file is required when --call-site is not provided")
	}
	if !cmd.Flags().Changed("line") {
		return fmt.Errorf("--line is required when --call-site is not provided")
	}
	if err := validateFileExists(flagFile); err != nil {
		return err
	}
	if err := validatePositiveCoord(cmd, "line", flagLine); err != nil {
		return err
	}
	return validatePositiveCoord(cmd, "col", flagCol)
}

// validateFileExists confirms --file names an existing regular file.
func validateFileExists(path string) error {
	if path == "" {
		return fmt.Errorf("--file is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving --file %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("--file %q does not exist", path)
		}
		return fmt.Errorf("checking --file %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("--file %q is a directory, not a source file", path)
	}
	return nil
}

// validatePositiveCoord rejects an explicitly supplied 1-indexed coordinate
// that is less than 1. An unset flag (Changed == false) is left alone so the
// resolver's tier logic can apply its own defaults.
func validatePositiveCoord(cmd *cobra.Command, flag string, value int) error {
	if cmd.Flags().Changed(flag) && value < 1 {
		return fmt.Errorf("--%s must be >= 1 (positions are 1-indexed), got %d", flag, value)
	}
	return nil
}

// validateSymbolValue rejects an empty or syntactically malformed --symbol. A
// qualified name using Rust's :: separators is parsed eagerly so a malformed
// value is reported rather than silently downgraded.
func validateSymbolValue(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("--symbol must not be empty")
	}
	if strings.Contains(value, "::") {
		if _, _, _, err := symbol.ParseRustQualifiedName(value); err != nil {
			return fmt.Errorf("invalid --symbol %q: %w", value, err)
		}
	}
	return nil
}

// anyChanged reports whether the user explicitly set any of the named flags.
func anyChanged(cmd *cobra.Command, names ...string) bool {
	for _, n := range names {
		if cmd.Flags().Changed(n) {
			return true
		}
	}
	return false
}
