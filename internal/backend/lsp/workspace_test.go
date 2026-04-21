package lsp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func TestPrimeGoWorkspace_findsSymbolsAcrossPackages(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/multi\n\ngo 1.22\n",
		"main.go": `package main

import "example.com/multi/util"

func main() { println(util.Greet()) }
`,
		"util/helper.go": `package util

func Greet() string { return "hi" }
`,
		"vendor/ignored/x.go": `package ignored

func ShouldNotIndex() {}
`,
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	opened, err := client.PrimeGoWorkspace(dir)
	if err != nil {
		t.Fatalf("PrimeGoWorkspace: %v", err)
	}
	if opened < 2 {
		t.Errorf("expected to open main.go and util/helper.go, opened %d", opened)
	}

	syms, err := client.WorkspaceSymbol("Greet")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	found := false
	for _, s := range syms {
		if s.Name == "Greet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Greet not found after priming. Results: %v", syms)
	}

	// Confirm vendored symbol was NOT indexed.
	vsyms, _ := client.WorkspaceSymbol("ShouldNotIndex")
	for _, s := range vsyms {
		if s.Name == "ShouldNotIndex" {
			t.Errorf("vendored symbol should not be indexed, but was: %+v", s)
		}
	}
}
