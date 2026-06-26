package backend

import (
	"errors"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

var ErrUnsupported = errors.New("refactoring not supported by this backend")
var ErrSymbolNotFound = errors.New("symbol not found")

type ErrAmbiguous struct {
	Candidates []symbol.Location
}

func (e *ErrAmbiguous) Error() string {
	return "ambiguous symbol: multiple candidates found"
}

type Capability struct {
	Operation string
}

type RefactoringBackend interface {
	Initialize(workspaceRoot string) error
	Shutdown() error
	FindSymbol(query symbol.Query) ([]symbol.Location, error)
	Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error)
	ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error)
	ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error)
	InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error)
	MoveToFile(loc symbol.Location, destination string) (*edit.WorkspaceEdit, error)
	Capabilities() []Capability
}

// KindResolver is an optional backend capability: reporting the actual
// SymbolKind of the symbol at an already-resolved Location. The rename-* kind
// variants use it to validate the requested kind against the target before an
// edit is computed. A backend that cannot determine the kind returns
// symbol.KindUnknown (no error), which callers treat as "unknown, do not
// reject". Backends that do not implement this interface are not kind-validated.
type KindResolver interface {
	ResolveKind(loc symbol.Location) (symbol.SymbolKind, error)
}
