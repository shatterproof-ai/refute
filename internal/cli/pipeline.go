package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// The output pipeline turns a computed WorkspaceEdit into user-visible output —
// a diff (dry-run), applied changes (default), or a JSON envelope (--json). It
// is intentionally free of cobra so a future daemon/MCP server can drive the
// same apply/preview/emit logic outside the CLI command tree.

// applyOrPreview emits the result per --dry-run/--json/default flags.
func applyOrPreview(we *edit.WorkspaceEdit, ctx jsonContext, opts operationFlags) error {
	telemetrySetContext(ctx)
	if opts.JSON {
		return emitJSON(we, ctx, statusForFlags(opts), opts)
	}
	if opts.DryRun {
		telemetrySetStatus(edit.StatusDryRun)
		telemetryCaptureSnapshot(we)
		renderDone := telemetryPhase("output-rendering")
		diff, err := edit.RenderDiff(we)
		renderDone()
		if err != nil {
			telemetrySetStatus("failed")
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}
	telemetryCaptureSnapshot(we)
	applyDone := telemetryPhase("apply")
	result, err := edit.ApplyWithin(we, ctx.WorkspaceRoot)
	applyDone()
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}
	telemetrySetStatus(edit.StatusApplied)
	telemetryMarkApplied()
	telemetrySetFilesModified(result.FilesModified)
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s Modified %d file(s):", green("ok"), result.FilesModified)
	for _, fe := range we.FileEdits {
		rel, _ := filepath.Rel(ctx.WorkspaceRoot, fe.Path)
		if rel == "" {
			rel = fe.Path
		}
		fmt.Fprintf(os.Stderr, " %s", rel)
	}
	fmt.Fprintln(os.Stderr)
	if flagVerbose {
		if diff, err := edit.RenderDiff(we); err == nil && diff != "" {
			fmt.Print(diff)
		}
	}
	return nil
}

func emitJSON(we *edit.WorkspaceEdit, ctx jsonContext, status string, opts operationFlags) error {
	telemetrySetContext(ctx)
	if opts.DryRun {
		telemetrySetStatus(status)
	}
	telemetryCaptureSnapshot(we)
	renderDone := telemetryPhase("output-rendering")
	res := edit.RenderJSON(we, status)
	res.Operation = ctx.Operation
	res.Language = ctx.Language
	res.Backend = ctx.Backend
	res.BackendVersion = ctx.BackendVersion
	res.WorkspaceRoot = ctx.WorkspaceRoot
	renderDone()
	if !opts.DryRun {
		applyDone := telemetryPhase("apply")
		if _, err := edit.ApplyWithin(we, ctx.WorkspaceRoot); err != nil {
			applyDone()
			// The success envelope was built but never written. Emit a single
			// apply-failed error envelope instead of plain text so JSON
			// consumers get parseable output on the most dangerous failure
			// (a partial apply after preview).
			return emitJSONError(ctx, edit.StatusBackendFailed, "apply-failed",
				fmt.Sprintf("applying edits: %v", err),
				"Inspect the workspace for a partial apply before retrying.")
		}
		applyDone()
		telemetrySetStatus(status)
		telemetryMarkApplied()
	}
	telemetrySetFilesModified(res.FilesModified)
	marshalDone := telemetryPhase("output-rendering")
	data, err := res.Marshal()
	marshalDone()
	if err != nil {
		telemetrySetStatus("failed")
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
