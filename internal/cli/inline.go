package cli

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

var callSiteFlag string

func init() {
	inlineCmd := &cobra.Command{
		Use:   "inline",
		Short: "Inline a symbol at the given position (Go, Rust, TypeScript)",
		Long: `Inline a variable or function call at the specified file position. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server).
Requires --file and either --line --col (exact position) or --line --name (scan line).
For Rust: use --symbol with --call-site <file>:<line>:<column>. See docs/support-matrix.md.`,
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
			if err == nil {
				parsed.Name = name
			}
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
		resolved, err := symbol.Resolve(query)
		if err != nil {
			return fmt.Errorf("symbol resolution: %w", err)
		}
		loc = resolved
	}

	sel, workspaceRoot, err := buildBackend(loc.File)
	if err != nil {
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	we, err := sel.Backend.InlineSymbol(loc)
	if err != nil {
		return fmt.Errorf("inline failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	ctx := contextFromSelection("inline", sel, workspaceRoot)
	return applyOrPreview(we, ctx)
}

func parseCallSite(s string) (symbol.Location, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return symbol.Location{}, fmt.Errorf("expected file:line:column, got %q", s)
	}
	line, err := strconv.Atoi(parts[1])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("line must be an integer: %w", err)
	}
	col, err := strconv.Atoi(parts[2])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("column must be an integer: %w", err)
	}
	return symbol.Location{File: parts[0], Line: line, Column: col}, nil
}
