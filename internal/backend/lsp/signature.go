package lsp

import (
	"context"
	"fmt"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
)

// Compile-time check that the adapter accepts structured refactor requests.
var _ backend.ParameterizedBackend = (*Adapter)(nil)

// removeUnusedParamKind is the gopls code-action kind for removing a parameter
// that is unused by the function body. gopls offers it only when it can rewrite
// the declaration and every call site safely (the parameter is genuinely unused
// and the function's address is not taken), which is exactly the safety
// condition change-signature requires.
const removeUnusedParamKind = "refactor.rewrite.removeUnusedParam"

// Refactor performs a parameterized refactoring. It currently handles only
// change-signature; any other operation is refused with ErrUnsupported so the
// caller reports the standard unsupported-operation status.
func (a *Adapter) Refactor(req backend.RefactorRequest) (*edit.WorkspaceEdit, error) {
	if req.Operation != backend.OperationChangeSignature {
		return nil, backend.ErrUnsupported
	}
	return a.changeSignature(req)
}

// changeSignature implements change-signature by driving gopls's
// removeUnusedParam code action. This release supports exactly one signature
// edit — removing a single parameter identified by the target location — which
// is the operation gopls performs with strong, all-call-site guarantees. The
// declarative SignatureParams model carries the full add/remove/reorder
// vocabulary so the design generalizes, but requests beyond a single remove are
// refused here rather than partially performed.
func (a *Adapter) changeSignature(req backend.RefactorRequest) (*edit.WorkspaceEdit, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	params, err := backend.DecodeSignatureParams(req.Params)
	if err != nil {
		return nil, err
	}
	if params.ReturnType != nil {
		return nil, &backend.ErrUnsafeRefactor{
			Operation: backend.OperationChangeSignature,
			Code:      "unsupported-signature-change",
			Reason:    "return-type changes are not supported by this backend",
		}
	}
	if len(params.Parameters) != 1 || params.Parameters[0].Op != backend.ParamOpRemove {
		return nil, &backend.ErrUnsafeRefactor{
			Operation: backend.OperationChangeSignature,
			Code:      "unsupported-signature-change",
			Reason:    `this release supports removing a single parameter: pass exactly one edit with op "remove"`,
		}
	}
	loc := req.Target.Location
	if loc == nil {
		return nil, fmt.Errorf("change-signature requires a target location identifying the parameter to remove")
	}

	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, fmt.Errorf("DidOpen %s: %w", loc.File, err)
	}
	const analysisTimeout = 30 * time.Second
	waitCtx, waitCancel := context.WithTimeout(context.Background(), analysisTimeout)
	defer waitCancel()
	if err := a.client.WaitForIdle(waitCtx); err != nil {
		return nil, fmt.Errorf("waiting for analysis: %w", err)
	}

	lspLine := loc.Line - 1
	lspChar, err := byteColumnToUTF16CharacterInFile(loc.File, lspLine, loc.Column)
	if err != nil {
		return nil, err
	}

	// Request rewrite code actions at the parameter position. gopls returns a
	// removeUnusedParam action only when the parameter is unused and every call
	// site can be rewritten; its absence is the signal to refuse.
	actions, err := a.client.CodeActions(loc.File, lspLine, lspChar, lspLine, lspChar, []string{"refactor.rewrite"})
	if err != nil {
		return nil, err
	}
	for _, action := range actions {
		if action.Kind != removeUnusedParamKind {
			continue
		}
		we, err := a.resolveAction(action)
		if err != nil {
			return nil, err
		}
		if we == nil || len(we.FileEdits) == 0 {
			break
		}
		we.FromCodeAction = true
		return we, nil
	}

	// No removeUnusedParam action: gopls will not remove this parameter because
	// it is used, or its call sites cannot all be rewritten with confidence.
	// Refuse rather than emit a partial edit (design §5.2).
	return nil, &backend.ErrUnsafeRefactor{
		Operation: backend.OperationChangeSignature,
		Code:      "parameter-in-use",
		Reason: fmt.Sprintf(
			"cannot safely remove the parameter at %s:%d:%d: it is used by the function body or its call sites cannot all be rewritten; only unused parameters can be removed",
			loc.File, loc.Line, loc.Column),
	}
}
