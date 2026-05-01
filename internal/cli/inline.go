package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

func init() {
	inlineCmd := &cobra.Command{
		Use:   "inline",
		Short: "Inline the symbol at the given position",
		Long: `Inline inlines a variable or function call at the specified file position.
Requires --file and either --line --col (exact position) or --line --name (scan line).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInline()
		},
	}
	inlineCmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	inlineCmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	inlineCmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed)")
	inlineCmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	inlineCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	_ = inlineCmd.MarkFlagRequired("file")
	_ = inlineCmd.MarkFlagRequired("line")

	RootCmd.AddCommand(inlineCmd)
}

func runInline() error {
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
	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	sel, workspaceRoot, err := buildBackend(loc.File)
	if err != nil {
		return err
	}
	defer sel.Backend.Shutdown()

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
