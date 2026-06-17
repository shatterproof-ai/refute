package lsp

import (
	"testing"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

// TestFilterRustCandidates exercises the Rust candidate narrowing that moved out
// of the CLI layer. A nil-client *Adapter is sufficient: the trait fallback via
// DocumentSymbols returns "" gracefully when the client is unset, so cases whose
// container already names the trait need no live server.
func TestFilterRustCandidates(t *testing.T) {
	a := &Adapter{}

	t.Run("trait-qualified picks the matching impl", func(t *testing.T) {
		infos := []symbol.Location{
			{File: "src/lib.rs", Line: 4, Name: "fmt", Container: "module::Other"},
			{File: "src/lib.rs", Line: 9, Name: "fmt", Container: "module::impl Display for Greeter"},
		}
		got := a.FilterRustCandidates(infos, []string{"module", "Greeter"}, "Display", "fmt")
		if len(got) != 1 || got[0].Line != 9 {
			t.Fatalf("got %+v, want single candidate at line 9", got)
		}
	})

	t.Run("module-qualified keeps all name matches", func(t *testing.T) {
		infos := []symbol.Location{
			{File: "src/lib.rs", Line: 4, Name: "rename_me", Container: "module"},
			{File: "src/other.rs", Line: 7, Name: "rename_me", Container: "module"},
			{File: "src/x.rs", Line: 1, Name: "other", Container: "module"},
		}
		got := a.FilterRustCandidates(infos, []string{"module"}, "", "rename_me")
		if len(got) != 2 {
			t.Fatalf("got %d candidates, want 2", len(got))
		}
	})

	t.Run("name mismatch is dropped", func(t *testing.T) {
		infos := []symbol.Location{{Name: "other", Container: "module"}}
		if got := a.FilterRustCandidates(infos, nil, "", "rename_me"); len(got) != 0 {
			t.Fatalf("got %+v, want none", got)
		}
	})
}
