package cli

import (
	"strings"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/telemetry"
)

var activeTelemetry *telemetry.Recorder

func setActiveTelemetry(r *telemetry.Recorder) {
	activeTelemetry = r
}

func telemetrySetContext(ctx jsonContext) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetOperation(telemetry.OperationContext{
		Operation:     ctx.Operation,
		Language:      ctx.Language,
		Backend:       ctx.Backend,
		WorkspaceRoot: ctx.WorkspaceRoot,
	})
}

func telemetrySetStatus(status string) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetStatus(status)
}

func telemetrySetDefaultStatus(status string) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetDefaultStatus(status)
}

func telemetrySetFilesModified(n int) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetOperation(telemetry.OperationContext{FilesModified: n})
}

func telemetrySetError(code, message string) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetError(code, message)
}

func telemetrySetDefaultError(code, message string) {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.SetDefaultError(code, message)
}

func telemetryPhase(name string) func() {
	if activeTelemetry == nil {
		return func() {}
	}
	return activeTelemetry.StartPhase(name)
}

func telemetryCaptureSnapshot(we *edit.WorkspaceEdit) {
	if activeTelemetry == nil {
		return
	}
	manifest, _ := activeTelemetry.CaptureSnapshot(we)
	if manifest == nil && we != nil {
		telemetrySetFilesModified(countEditedFiles(we))
	}
}

func telemetryMarkApplied() {
	if activeTelemetry == nil {
		return
	}
	activeTelemetry.MarkSnapshotApplied()
}

func argsContainVerbose(args []string) bool {
	for _, arg := range args {
		if arg == "--verbose" || strings.HasPrefix(arg, "--verbose=") {
			return true
		}
	}
	return false
}

func countEditedFiles(we *edit.WorkspaceEdit) int {
	if we == nil {
		return 0
	}
	n := 0
	for _, fe := range we.FileEdits {
		if len(fe.Edits) > 0 {
			n++
		}
	}
	return n
}
