package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

var callSiteFlag string

func init() {
	inlineCmd := &cobra.Command{
		Use:   "inline",
		Short: "Inline a symbol at the given position (Go, Rust)",
		Long: "Inline a variable or function call at the specified file position. Supports Go (gopls) and Rust (rust-analyzer).\n" +
			"Requires --file and either --line --col (exact position) or --line --name (scan line).\n" +
			"For Rust: use --symbol with --call-site <file>:<line>:<column>. See " + supportMatrixURL + ".",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeInline)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInline()
		},
	}
	inlineCmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	inlineCmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	inlineCmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed)")
	inlineCmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	inlineCmd.Flags().StringVar(&flagSymbol, "symbol", "", "qualified symbol name (e.g. Greeter::greet)")
	inlineCmd.Flags().StringVar(&callSiteFlag, "call-site", "",
		"When --symbol is used, specifies a call-site location file:line:column for inline")
	inlineCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")

	RootCmd.AddCommand(inlineCmd)
}

func runInline() error {
	ctx := jsonContext{Operation: "inline"}
	return routeOperationError(ctx, runInlineInner(&ctx))
}

// runInlineInner performs the inline and returns terminal errors for the shared
// wrapper to route. It populates *ctx as it learns the file and backend so an
// error envelope emitted by routeOperationError is fully attributed. A symbol
// resolution failure emits an invalid-position envelope inline; the wrapper
// passes the resulting jsonEmitted error through unchanged.
func runInlineInner(ctx *jsonContext) error {
	telemetrySetContext(*ctx)
	if flagSymbol != "" && callSiteFlag == "" {
		return fmt.Errorf("inline: --symbol requires --call-site <file>:<line>:<column> " +
			"to disambiguate which call site to inline")
	}

	var loc symbol.Location
	if callSiteFlag != "" {
		parsed, err := parseCallSite(callSiteFlag)
		if err != nil {
			return fmt.Errorf("parse --call-site: %w", err)
		}
		if flagSymbol != "" {
			_, _, name, err := ParseRustQualifiedName(flagSymbol)
			if err != nil {
				return fmt.Errorf("invalid --symbol %q: %w", flagSymbol, err)
			}
			parsed.Name = name
		}
		loc = parsed
	} else {
		if flagFile == "" {
			return fmt.Errorf("inline: --file is required when --call-site is not provided")
		}
		if flagLine == 0 {
			return fmt.Errorf("inline: --line is required when --call-site is not provided")
		}
		absFile, err := filepath.Abs(flagFile)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query := symbol.Query{
			File:   absFile,
			Line:   flagLine,
			Column: flagCol,
			Name:   flagName,
		}
		resolveDone := telemetryPhase("symbol-resolution")
		resolved, err := symbol.Resolve(query)
		resolveDone()
		if err != nil {
			if flagJSON {
				return emitJSONError(contextFromFile("inline", absFile),
					edit.StatusInvalidPosition, "invalid-position", err.Error(),
					"Check --file, --line, --col, and --name.")
			}
			return fmt.Errorf("symbol resolution: %w", err)
		}
		loc = resolved
	}

	*ctx = contextFromFile("inline", loc.File)
	sel, workspaceRoot, err := buildBackend(loc.File)
	if err != nil {
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	*ctx = contextFromSelection("inline", sel, workspaceRoot)
	telemetrySetContext(*ctx)
	refactorDone := telemetryPhase("backend-refactor-request")
	we, err := sel.Backend.InlineSymbol(loc)
	refactorDone()
	if err != nil {
		return fmt.Errorf("inline failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(we, *ctx)
}

// parseCallSite parses a --call-site value of the form file:line:column. The
// file may itself contain colons (e.g. a Windows drive path like C:\src\x.rs),
// so line and column are taken from the last two colon-separated fields and the
// remainder is the path. The path is absolute-resolved and the coordinates must
// be positive (1-indexed).
func parseCallSite(s string) (symbol.Location, error) {
	lastColon := strings.LastIndex(s, ":")
	if lastColon < 0 {
		return symbol.Location{}, fmt.Errorf("expected file:line:column, got %q", s)
	}
	secondColon := strings.LastIndex(s[:lastColon], ":")
	if secondColon < 0 {
		return symbol.Location{}, fmt.Errorf("expected file:line:column, got %q", s)
	}
	file := s[:secondColon]
	if file == "" {
		return symbol.Location{}, fmt.Errorf("call-site file path must not be empty: %q", s)
	}
	line, err := strconv.Atoi(s[secondColon+1 : lastColon])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("line must be an integer: %w", err)
	}
	col, err := strconv.Atoi(s[lastColon+1:])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("column must be an integer: %w", err)
	}
	if line < 1 {
		return symbol.Location{}, fmt.Errorf("line must be >= 1 (1-indexed), got %d", line)
	}
	if col < 1 {
		return symbol.Location{}, fmt.Errorf("column must be >= 1 (1-indexed), got %d", col)
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return symbol.Location{}, fmt.Errorf("resolving call-site path %q: %w", file, err)
	}
	return symbol.Location{File: abs, Line: line, Column: col}, nil
}
