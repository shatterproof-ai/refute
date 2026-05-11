package cli

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type tier1FakeBackend struct {
	findQuery symbol.Query
	locs      []symbol.Location
	err       error
}

func (b *tier1FakeBackend) Initialize(string) error { return nil }
func (b *tier1FakeBackend) Shutdown() error         { return nil }

func (b *tier1FakeBackend) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	b.findQuery = query
	return b.locs, b.err
}

func (b *tier1FakeBackend) Rename(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (b *tier1FakeBackend) ExtractFunction(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (b *tier1FakeBackend) ExtractVariable(symbol.SourceRange, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (b *tier1FakeBackend) InlineSymbol(symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (b *tier1FakeBackend) MoveToFile(symbol.Location, string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (b *tier1FakeBackend) Capabilities() []backend.Capability { return nil }

func TestResolveRustTier1SymbolFiltersCandidate(t *testing.T) {
	b := &tier1FakeBackend{
		locs: []symbol.Location{
			{File: "src/lib.rs", Line: 4, Column: 1, Name: "fmt", Container: "module::Other"},
			{File: "src/lib.rs", Line: 9, Column: 1, Name: "fmt", Container: "module::impl Display for Greeter"},
		},
	}

	loc, err := resolveRustTier1Symbol(b, symbol.Query{QualifiedName: "module::<Greeter as Display>::fmt"})
	if err != nil {
		t.Fatalf("resolveRustTier1Symbol returned error: %v", err)
	}
	if loc.Line != 9 || loc.Name != "fmt" {
		t.Fatalf("loc = %+v, want fmt at line 9", loc)
	}
	if b.findQuery.QualifiedName != "fmt" {
		t.Fatalf("FindSymbol query = %+v, want qualified name fmt", b.findQuery)
	}
}

func TestResolveRustTier1SymbolReturnsAmbiguousCandidates(t *testing.T) {
	want := []symbol.Location{
		{File: "src/lib.rs", Line: 4, Column: 1, Name: "rename_me", Container: "module"},
		{File: "src/other.rs", Line: 7, Column: 3, Name: "rename_me", Container: "module"},
	}
	b := &tier1FakeBackend{locs: want}

	_, err := resolveRustTier1Symbol(b, symbol.Query{QualifiedName: "module::rename_me"})
	var ambiguous *backend.ErrAmbiguous
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %#v, want backend.ErrAmbiguous", err)
	}
	if len(ambiguous.Candidates) != len(want) {
		t.Fatalf("candidates = %d, want %d", len(ambiguous.Candidates), len(want))
	}
}

func TestHandleTier1RenameErrorEmitsAmbiguousJSON(t *testing.T) {
	reset := func() { flagJSON = false }
	reset()
	t.Cleanup(reset)
	flagJSON = true

	ctx := jsonContext{
		Operation:     "rename",
		Language:      "rust",
		Backend:       "lsp",
		WorkspaceRoot: "/workspace",
	}
	err := &backend.ErrAmbiguous{Candidates: []symbol.Location{
		{File: "/workspace/src/lib.rs", Line: 4, Column: 1, Name: "rename_me"},
	}}

	out := captureStdout(t, func() {
		runErr := handleTier1RenameError(ctx, err)
		var ec *ExitCodeError
		if !errors.As(runErr, &ec) || ec.Code != 1 {
			t.Fatalf("runErr = %#v, want exit code 1", runErr)
		}
	})

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusAmbiguous {
		t.Fatalf("status = %q, want %q", got.Status, edit.StatusAmbiguous)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].Name != "rename_me" {
		t.Fatalf("candidates = %+v, want one rename_me candidate", got.Candidates)
	}
}
