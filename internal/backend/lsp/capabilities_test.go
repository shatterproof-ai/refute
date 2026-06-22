package lsp_test

import (
	"slices"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
)

func opNames(caps []backend.Capability) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, c.Operation)
	}
	return out
}

// TestAdapterCapabilitiesFromProfile asserts Capabilities() derives from the
// language profile rather than a static list: full-refactoring languages
// advertise all operations, rename-only languages advertise only rename, and an
// unregistered language falls back to the conservative rename-only default.
func TestAdapterCapabilitiesFromProfile(t *testing.T) {
	full := []string{"rename", "extract-function", "extract-variable", "inline"}
	renameOnly := []string{"rename"}
	cases := map[string][]string{
		"go":         full,
		"rust":       full,
		"typescript": renameOnly,
		"javascript": renameOnly,
		"python":     renameOnly,
		"cobol-9000": renameOnly, // unregistered -> default
	}
	for langID, want := range cases {
		a := lsp.NewAdapter(config.ServerConfig{}, langID, nil)
		got := opNames(a.Capabilities())
		if !slices.Equal(got, want) {
			t.Errorf("Capabilities(%q) = %v, want %v", langID, got, want)
		}
	}
}

// TestAdapterCapabilitiesMatchSupportMatrix guards against drift between the
// LSP language profiles and the documented support matrix: for every LSP
// language in the matrix, the adapter's advertised operations must equal the
// matrix's Operations.
func TestAdapterCapabilitiesMatchSupportMatrix(t *testing.T) {
	for _, lang := range []string{"go", "rust", "typescript", "javascript", "python"} {
		sup, ok := config.SupportFor(lang)
		if !ok {
			t.Fatalf("support matrix missing language %q", lang)
		}
		a := lsp.NewAdapter(config.ServerConfig{}, lang, nil)
		got := opNames(a.Capabilities())
		if !slices.Equal(got, sup.Operations) {
			t.Errorf("language %q: adapter advertises %v but support matrix lists %v", lang, got, sup.Operations)
		}
	}
}
