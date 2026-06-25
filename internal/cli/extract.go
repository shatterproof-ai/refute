package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type extractFlags struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	Name      string
}

func addExtractFlags(cmd *cobra.Command, flags *extractFlags) {
	cmd.Flags().StringVar(&flags.File, "file", "", "source file path (required)")
	cmd.Flags().IntVar(&flags.StartLine, "start-line", 0, "start line, 1-indexed (required)")
	cmd.Flags().IntVar(&flags.StartCol, "start-col", 0, "start column, 1-indexed (required)")
	cmd.Flags().IntVar(&flags.EndLine, "end-line", 0, "end line, 1-indexed (required)")
	cmd.Flags().IntVar(&flags.EndCol, "end-col", 0, "end column, 1-indexed (required)")
	cmd.Flags().StringVar(&flags.Name, "name", "", "name for the extracted symbol (optional; gopls default used if empty)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	for _, f := range []string{"file", "start-line", "start-col", "end-line", "end-col"} {
		_ = cmd.MarkFlagRequired(f)
	}
}

func init() {
	funcFlags := &extractFlags{}
	extractFuncCmd := &cobra.Command{
		Use:   "extract-function",
		Short: "Extract a selection into a new function (Go, Rust)",
		Long:  "Extract the selected code range into a new named function. The selection is given by --file with --start-line/--start-col and --end-line/--end-col. Supports Go (gopls) and Rust (rust-analyzer). See " + supportMatrixURL + ".",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeExtract, funcFlags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("function", funcFlags)
		},
	}
	addExtractFlags(extractFuncCmd, funcFlags)

	varFlags := &extractFlags{}
	extractVarCmd := &cobra.Command{
		Use:   "extract-variable",
		Short: "Extract a selection into a new variable (Go, Rust)",
		Long:  "Extract the selected code range into a new named variable. The selection is given by --file with --start-line/--start-col and --end-line/--end-col. Supports Go (gopls) and Rust (rust-analyzer). See " + supportMatrixURL + ".",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeExtract, varFlags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("variable", varFlags)
		},
	}
	addExtractFlags(extractVarCmd, varFlags)

	RootCmd.AddCommand(extractFuncCmd)
	RootCmd.AddCommand(extractVarCmd)
}

func runExtract(kind string, flags *extractFlags) error {
	ctx := jsonContext{Operation: "extract-" + kind}
	err := runExtractInner(kind, flags, &ctx)
	return routeOperationError(ctx, err)
}

// runExtractInner performs the extraction and returns terminal errors for the
// shared wrapper to route. It populates *ctx with best-available metadata
// (language/workspace from the file, then backend once selected) so an error
// envelope emitted by routeOperationError is fully attributed.
func runExtractInner(kind string, flags *extractFlags, ctx *jsonContext) error {
	operation := "extract-" + kind
	telemetrySetContext(*ctx)
	absFile, err := filepath.Abs(flags.File)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	*ctx = contextFromFile(operation, absFile)
	sel, workspaceRoot, err := buildBackend(absFile, operation)
	if err != nil {
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	r := symbol.SourceRange{
		File:      absFile,
		StartLine: flags.StartLine,
		StartCol:  flags.StartCol,
		EndLine:   flags.EndLine,
		EndCol:    flags.EndCol,
	}

	*ctx = contextFromSelection(operation, sel, workspaceRoot)
	telemetrySetContext(*ctx)
	refactorDone := telemetryPhase("backend-refactor-request")
	var result *edit.WorkspaceEdit
	switch kind {
	case "function":
		result, err = sel.Backend.ExtractFunction(r, flags.Name)
	case "variable":
		result, err = sel.Backend.ExtractVariable(r, flags.Name)
	default:
		refactorDone()
		return fmt.Errorf("unknown extract kind %q", kind)
	}
	refactorDone()
	if err != nil {
		return fmt.Errorf("%s failed: %w", operation, err)
	}
	if len(result.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(result, *ctx)
}
