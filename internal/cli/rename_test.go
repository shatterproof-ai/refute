package cli

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// fakeTier1Resolver satisfies tier1Resolver so the CLI orchestration in
// resolveRustTier1Symbol can be tested without a live language server. The Rust
// candidate-filtering domain logic itself is tested in internal/backend/lsp.
type fakeTier1Resolver struct {
	findQuery symbol.Query
	locs      []symbol.Location
	findErr   error
	filtered  []symbol.Location
	gotName   string
}

func (f *fakeTier1Resolver) FindSymbol(q symbol.Query) ([]symbol.Location, error) {
	f.findQuery = q
	return f.locs, f.findErr
}

func (f *fakeTier1Resolver) FilterRustCandidates(infos []symbol.Location, modulePath []string, trait, name string) []symbol.Location {
	f.gotName = name
	return f.filtered
}

func TestResolveRustTier1Symbol_SingleCandidate(t *testing.T) {
	want := symbol.Location{File: "src/lib.rs", Line: 9, Column: 1, Name: "fmt"}
	f := &fakeTier1Resolver{
		locs:     []symbol.Location{{Name: "fmt"}, {Name: "fmt"}},
		filtered: []symbol.Location{want},
	}

	loc, err := resolveRustTier1Symbol(f, symbol.Query{QualifiedName: "module::<Greeter as Display>::fmt"})
	if err != nil {
		t.Fatalf("resolveRustTier1Symbol: %v", err)
	}
	if loc != want {
		t.Fatalf("loc = %+v, want %+v", loc, want)
	}
	// The qualified name is reduced to the bare leaf before the workspace lookup.
	if f.findQuery.QualifiedName != "fmt" || f.gotName != "fmt" {
		t.Fatalf("FindSymbol query=%q, filter name=%q, want fmt", f.findQuery.QualifiedName, f.gotName)
	}
}

func TestResolveRustTier1Symbol_Ambiguous(t *testing.T) {
	cands := []symbol.Location{
		{File: "src/lib.rs", Line: 4, Name: "rename_me"},
		{File: "src/other.rs", Line: 7, Name: "rename_me"},
	}
	f := &fakeTier1Resolver{locs: cands, filtered: cands}

	_, err := resolveRustTier1Symbol(f, symbol.Query{QualifiedName: "module::rename_me"})
	var ambiguous *backend.ErrAmbiguous
	if !errors.As(err, &ambiguous) {
		t.Fatalf("err = %#v, want backend.ErrAmbiguous", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(ambiguous.Candidates))
	}
}

func TestResolveRustTier1Symbol_NotFound(t *testing.T) {
	f := &fakeTier1Resolver{
		locs:     []symbol.Location{{Name: "rename_me"}},
		filtered: nil,
	}
	_, err := resolveRustTier1Symbol(f, symbol.Query{QualifiedName: "module::rename_me"})
	var notFound *ErrSymbolNotFound
	if !errors.As(err, &notFound) {
		t.Fatalf("err = %#v, want *ErrSymbolNotFound", err)
	}
}

func TestResolveRustTier1Symbol_ParseError(t *testing.T) {
	f := &fakeTier1Resolver{}
	_, err := resolveRustTier1Symbol(f, symbol.Query{QualifiedName: "foo()"})
	var parseErr *tier1RustSymbolParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("err = %#v, want *tier1RustSymbolParseError", err)
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
