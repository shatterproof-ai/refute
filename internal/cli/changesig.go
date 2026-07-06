package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type changeSigFlags struct {
	File   string
	Line   int
	Col    int
	Remove bool
}

func addChangeSignatureFlags(cmd *cobra.Command, flags *changeSigFlags) {
	cmd.Flags().StringVar(&flags.File, "file", "", "source file path (required)")
	cmd.Flags().IntVar(&flags.Line, "line", 0, "line of the parameter to change, 1-indexed (required)")
	cmd.Flags().IntVar(&flags.Col, "col", 0, "column of the parameter to change, 1-indexed (required)")
	cmd.Flags().BoolVar(&flags.Remove, "remove", false, "remove the parameter at --line/--col (the only signature change supported in this release)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	for _, f := range []string{"file", "line", "col"} {
		_ = cmd.MarkFlagRequired(f)
	}
}

func init() {
	flags := &changeSigFlags{}
	cmd := &cobra.Command{
		Use:   "change-signature",
		Short: "Change a function signature (Go: remove an unused parameter)",
		Long: "Change a function's signature, updating the declaration and every call site. " +
			"This release supports removing an unused parameter on Go (via gopls): point --file/--line/--col " +
			"at the parameter to remove and pass --remove. When gopls cannot rewrite all call sites safely " +
			"(e.g. the parameter is used), the operation is refused and nothing is changed. See " + supportMatrixURL + ".",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateChangeSignatureFlags(cmd, flags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChangeSignature(flags, operationFlagsFromCmd(cmd))
		},
	}
	addChangeSignatureFlags(cmd, flags)
	RootCmd.AddCommand(cmd)
}

func validateChangeSignatureFlags(cmd *cobra.Command, flags *changeSigFlags) error {
	if err := validateFileExists(flags.File); err != nil {
		return err
	}
	if err := validatePositiveCoord(cmd, "line", flags.Line); err != nil {
		return err
	}
	if err := validatePositiveCoord(cmd, "col", flags.Col); err != nil {
		return err
	}
	if !flags.Remove {
		return fmt.Errorf("change-signature requires --remove (removing a parameter is the only signature change supported in this release)")
	}
	return nil
}

func runChangeSignature(flags *changeSigFlags, opts operationFlags) error {
	ctx := jsonContext{Operation: backend.OperationChangeSignature}
	err := runChangeSignatureInner(flags, &ctx, opts)
	return routeOperationError(ctx, err, opts)
}

func runChangeSignatureInner(flags *changeSigFlags, ctx *jsonContext, opts operationFlags) error {
	operation := backend.OperationChangeSignature
	telemetrySetContext(*ctx)
	absFile, err := filepath.Abs(flags.File)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	*ctx = contextFromFile(operation, absFile)
	sel, workspaceRoot, err := buildBackend(absFile, operation, opts)
	if err != nil {
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	// The selector only routes here when the backend advertises the operation,
	// so a backend that does not implement the structured entry point is a
	// programming error; refuse it as unsupported rather than panicking.
	pb, ok := sel.Backend.(backend.ParameterizedBackend)
	if !ok {
		return fmt.Errorf("%s: %w", operation, backend.ErrUnsupported)
	}

	*ctx = contextFromSelection(operation, sel, workspaceRoot)
	telemetrySetContext(*ctx)

	params, err := json.Marshal(backend.SignatureParams{
		Parameters: []backend.ParamEdit{{Op: backend.ParamOpRemove, FromIndex: -1, ToIndex: -1}},
	})
	if err != nil {
		return fmt.Errorf("encoding change-signature params: %w", err)
	}
	req := backend.RefactorRequest{
		Operation: operation,
		Target: backend.Target{Location: &symbol.Location{
			File:   absFile,
			Line:   flags.Line,
			Column: flags.Col,
		}},
		Params: params,
	}

	refactorDone := telemetryPhase("backend-refactor-request")
	result, err := pb.Refactor(req)
	refactorDone()
	if err != nil {
		return err
	}
	if result == nil || len(result.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(result, *ctx, opts)
}
