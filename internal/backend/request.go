package backend

import (
	"encoding/json"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// RefactorRequest is the structured entry point for parameterized refactorings
// (change-signature and future recipe-style operations). Fixed-arity operations
// (rename, extract, inline, move-to-file) keep their typed methods on
// RefactoringBackend; this envelope exists so an operation can carry an
// operation-specific parameter object plus optional backend-specific extensions
// without growing a new interface method per operation. See
// docs/plans/refactoring-extension-model.md §3.
type RefactorRequest struct {
	// Operation is the capability string (e.g. "change-signature"). It must
	// match a Capability.Operation the backend advertises.
	Operation string

	// Target identifies what the operation acts on. Exactly one of Location or
	// Range is set, per the operation's target kind.
	Target Target

	// Params is the operation-specific, backend-agnostic parameter object,
	// encoded as canonical JSON. Each operation defines its own struct (e.g.
	// SignatureParams); the envelope stays operation-agnostic.
	Params json.RawMessage

	// BackendParams is an opaque, backend-specific extension block. A backend
	// that does not recognize a key MUST ignore it; a backend that needs a key
	// it does not find applies its documented default. It is never required for
	// the common path to succeed. Unused by the initial change-signature
	// implementation but carried through so the wire contract is stable.
	BackendParams json.RawMessage
}

// Target is a discriminated reference to the code an operation acts on. Exactly
// one field is set: Location for symbol-addressed operations, Range for
// range-addressed ones.
type Target struct {
	Location *symbol.Location    // for symbol-addressed operations
	Range    *symbol.SourceRange // for range-addressed operations
}

// ParameterizedBackend is the optional interface a backend implements to accept
// structured, parameterized refactoring requests. It is deliberately separate
// from RefactoringBackend so a backend that supports only fixed-arity
// operations (rename/extract/inline/move) needs no no-op Refactor method; the
// CLI type-asserts for it and refuses the operation when a backend does not
// implement it (docs/plans/refactoring-extension-model.md §9).
type ParameterizedBackend interface {
	// Refactor performs a parameterized refactoring described by req and returns
	// the resulting edit. It returns *ErrUnsafeRefactor when the operation is
	// supported in general but cannot be performed safely for this invocation,
	// and ErrUnsupported when it does not handle req.Operation at all.
	Refactor(req RefactorRequest) (*edit.WorkspaceEdit, error)
}
