package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type moveFlags struct {
	File        string
	Line        int
	Col         int
	Name        string
	Destination string
}

func init() {
	flags := &moveFlags{}
	moveCmd := &cobra.Command{
		Use:   "move",
		Short: "Move a declaration to another file in the same package (Go)",
		Long: "Move the top-level declaration at the given position into --destination, a file in the same package (directory). " +
			"Supports Go (gopls extract-to-new-file). The declaration and any imports it needs move to the destination; " +
			"references elsewhere in the package are unaffected.\n" +
			"Requires --file, --line, and --destination; use --col for an exact column or --name to scan the line.\n" +
			"--destination is resolved relative to the source file's directory. Moving across a package boundary, " +
			"onto an existing file, or a symbol gopls cannot move is refused (exit 1). See " + supportMatrixURL + ".",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeMove, flags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMove(flags, operationFlagsFromCmd(cmd))
		},
	}
	moveCmd.Flags().StringVar(&flags.File, "file", "", "source file path (required)")
	moveCmd.Flags().IntVar(&flags.Line, "line", 0, "line number, 1-indexed (required)")
	moveCmd.Flags().IntVar(&flags.Col, "col", 0, "column number, 1-indexed")
	moveCmd.Flags().StringVar(&flags.Name, "name", "", "symbol name to find on the line")
	moveCmd.Flags().StringVar(&flags.Destination, "destination", "", "destination file, in the same package as --file (required)")
	moveCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")

	RootCmd.AddCommand(moveCmd)
}

func runMove(flags *moveFlags, opts operationFlags) error {
	ctx := jsonContext{Operation: "move"}
	err := runMoveInner(flags, &ctx, opts)
	return routeOperationError(ctx, err, opts)
}

// runMoveInner resolves the target symbol, computes the move edit, and routes it
// through the shared apply/preview pipeline. It populates *ctx with file- then
// backend-derived metadata so an error envelope emitted by routeOperationError
// is fully attributed. A symbol-resolution failure emits an invalid-position
// envelope inline (JSON mode) or returns a plain error.
func runMoveInner(flags *moveFlags, ctx *jsonContext, opts operationFlags) error {
	telemetrySetContext(*ctx)

	absFile, err := filepath.Abs(flags.File)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}

	// Resolve --destination relative to the source file's directory when it is
	// not absolute: a same-package move names a sibling file, so a bare
	// "moved.go" should land beside --file, not in the process CWD.
	destination := flags.Destination
	if !filepath.IsAbs(destination) {
		destination = filepath.Join(filepath.Dir(absFile), destination)
	}

	query := symbol.Query{File: absFile, Line: flags.Line, Column: flags.Col, Name: flags.Name}
	resolveDone := telemetryPhase("symbol-resolution")
	loc, err := symbol.Resolve(query)
	resolveDone()
	if err != nil {
		if opts.JSON {
			return emitJSONError(contextFromFile("move", absFile),
				edit.StatusInvalidPosition, "invalid-position", err.Error(),
				"Check --file, --line, --col, and --name.")
		}
		return fmt.Errorf("symbol resolution: %w", err)
	}

	*ctx = contextFromFile("move", loc.File)
	sel, workspaceRoot, err := buildBackend(loc.File, "move", opts)
	if err != nil {
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	*ctx = contextFromSelection("move", sel, workspaceRoot)
	telemetrySetContext(*ctx)
	refactorDone := telemetryPhase("backend-refactor-request")
	we, err := sel.Backend.MoveToFile(loc, destination)
	refactorDone()
	if err != nil {
		return fmt.Errorf("move failed: %w", err)
	}
	if we == nil || (len(we.FileEdits) == 0 && len(we.FileOps) == 0) {
		return NoEditsError()
	}
	return applyOrPreview(we, *ctx, opts)
}
