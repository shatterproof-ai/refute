package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

var (
	flagStartLine int
	flagStartCol  int
	flagEndLine   int
	flagEndCol    int
	flagExtName   string
)

func addExtractFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagStartLine, "start-line", 0, "start line (1-indexed)")
	cmd.Flags().IntVar(&flagStartCol, "start-col", 0, "start column (1-indexed)")
	cmd.Flags().IntVar(&flagEndLine, "end-line", 0, "end line (1-indexed)")
	cmd.Flags().IntVar(&flagEndCol, "end-col", 0, "end column (1-indexed)")
	cmd.Flags().StringVar(&flagExtName, "name", "", "name for the extracted symbol (optional; gopls default used if empty)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	for _, f := range []string{"file", "start-line", "start-col", "end-line", "end-col"} {
		_ = cmd.MarkFlagRequired(f)
	}
}

func init() {
	extractFuncCmd := &cobra.Command{
		Use:   "extract-function",
		Short: "Extract a selection into a new function (Go, Rust, TypeScript)",
		Long:  "Extract the selected code range into a new named function. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server). See docs/support-matrix.md.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("function")
		},
	}
	addExtractFlags(extractFuncCmd)

	extractVarCmd := &cobra.Command{
		Use:   "extract-variable",
		Short: "Extract a selection into a new variable (Go, Rust, TypeScript)",
		Long:  "Extract the selected code range into a new named variable. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server). See docs/support-matrix.md.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("variable")
		},
	}
	addExtractFlags(extractVarCmd)

	RootCmd.AddCommand(extractFuncCmd)
	RootCmd.AddCommand(extractVarCmd)
}

func runExtract(kind string) error {
	absFile, err := filepath.Abs(flagFile)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	sel, workspaceRoot, err := buildBackend(absFile)
	if err != nil {
		return err
	}
	defer sel.Backend.Shutdown()

	r := symbol.SourceRange{
		File:      absFile,
		StartLine: flagStartLine,
		StartCol:  flagStartCol,
		EndLine:   flagEndLine,
		EndCol:    flagEndCol,
	}

	switch kind {
	case "function":
		ctx := contextFromSelection("extract-function", sel, workspaceRoot)
		result, err := sel.Backend.ExtractFunction(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-function failed: %w", err)
		}
		if len(result.FileEdits) == 0 {
			return NoEditsError()
		}
		return applyOrPreview(result, ctx)
	case "variable":
		ctx := contextFromSelection("extract-variable", sel, workspaceRoot)
		result, err := sel.Backend.ExtractVariable(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-variable failed: %w", err)
		}
		if len(result.FileEdits) == 0 {
			return NoEditsError()
		}
		return applyOrPreview(result, ctx)
	default:
		return fmt.Errorf("unknown extract kind %q", kind)
	}
}
