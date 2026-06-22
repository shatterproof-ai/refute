//go:build integration

package internal_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// listSymbolsJSON is the subset of `refute list-symbols --json` output the
// integration test asserts on.
type listSymbolsJSON struct {
	SchemaVersion string `json:"schemaVersion"`
	Status        string `json:"status"`
	Language      string `json:"language"`
	Symbols       []struct {
		File          string `json:"file"`
		Line          int    `json:"line"`
		Column        int    `json:"column"`
		Kind          string `json:"kind"`
		Name          string `json:"name"`
		QualifiedName string `json:"qualifiedName"`
	} `json:"symbols"`
}

func runListSymbolsCLI(t *testing.T, bin, dir string, args ...string) listSymbolsJSON {
	t.Helper()
	full := append([]string{"list-symbols", "--lang", "go", "--json"}, args...)
	cmd := exec.Command(bin, full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list-symbols %v: %s\n%s", args, err, out)
	}
	var got listSymbolsJSON
	if jErr := json.Unmarshal(out, &got); jErr != nil {
		t.Fatalf("unmarshal list-symbols output: %v\nraw:\n%s", jErr, out)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %q, want ok\nraw:\n%s", got.Status, out)
	}
	return got
}

// TestEndToEnd_ListSymbols drives the real gopls workspace/symbol path and
// covers the empty, single, and ambiguous result sets called out in #94.
func TestEndToEnd_ListSymbols(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	refuteBin := buildRefute(t)

	// Single: an exact, unique name resolves to one candidate with the
	// file/line/column an agent needs to feed a Tier-3 operation.
	single := runListSymbolsCLI(t, refuteBin, dir, "--query", "FormatGreeting")
	if len(single.Symbols) != 1 {
		t.Fatalf("FormatGreeting: want 1 symbol, got %d: %+v", len(single.Symbols), single.Symbols)
	}
	s := single.Symbols[0]
	if s.Name != "FormatGreeting" {
		t.Errorf("name = %q, want FormatGreeting", s.Name)
	}
	if !strings.HasSuffix(s.File, filepath.Join("util", "helper.go")) {
		t.Errorf("file = %q, want .../util/helper.go", s.File)
	}
	if s.Line != 4 || s.Column < 1 {
		t.Errorf("coordinates = %d:%d, want line 4 with a positive column", s.Line, s.Column)
	}
	if s.Kind != "function" {
		t.Errorf("kind = %q, want function", s.Kind)
	}

	// Ambiguous: a substring query gopls fuzzy-matches to multiple symbols
	// (User, NewUser) that agents disambiguate by location.
	ambiguous := runListSymbolsCLI(t, refuteBin, dir, "--query", "User")
	if len(ambiguous.Symbols) < 2 {
		t.Fatalf("User: want >=2 ambiguous candidates, got %d: %+v", len(ambiguous.Symbols), ambiguous.Symbols)
	}

	// Kind filter narrows the ambiguous set to type declarations only.
	types := runListSymbolsCLI(t, refuteBin, dir, "--query", "User", "--kind", "type")
	for _, sym := range types.Symbols {
		if sym.Kind != "type" {
			t.Errorf("kind filter leaked %q symbol %q", sym.Kind, sym.Name)
		}
	}

	// Empty: a name that does not exist returns a successful, empty listing.
	empty := runListSymbolsCLI(t, refuteBin, dir, "--query", "ZzNoSuchSymbolXyz")
	if len(empty.Symbols) != 0 {
		t.Errorf("want 0 symbols for nonexistent query, got %d: %+v", len(empty.Symbols), empty.Symbols)
	}
}
