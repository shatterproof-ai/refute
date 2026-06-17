package lsp

import (
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// FilterRustCandidates narrows workspace/symbol results to those whose name,
// enclosing module path, and (when given) trait match a parsed Rust qualified
// name. Trait matching prefers the container string and falls back to
// DocumentSymbol inspection when the container does not name the trait.
//
// This lives on the adapter so the DocumentSymbol fallback can call the LSP
// client directly rather than forcing the CLI to type-assert *Adapter.
func (a *Adapter) FilterRustCandidates(infos []symbol.Location, modulePath []string, trait, name string) []symbol.Location {
	out := make([]symbol.Location, 0, len(infos))
	for _, info := range infos {
		if info.Name != name {
			continue
		}
		infoMod, infoType, infoTrait := parseRustContainer(info.Container)
		if !rustModuleMatches(modulePath, infoMod, infoType) {
			continue
		}
		if trait != "" {
			resolved := infoTrait
			if resolved == "" {
				resolved = a.resolveTraitByDocumentSymbol(info)
			}
			if resolved != trait {
				continue
			}
		}
		out = append(out, info)
	}
	return out
}

// resolveTraitByDocumentSymbol inspects the file's DocumentSymbols to find the
// trait of the impl block enclosing info. Returns "" when the symbols cannot be
// fetched or no enclosing trait impl is found.
func (a *Adapter) resolveTraitByDocumentSymbol(info symbol.Location) string {
	symbols, err := a.DocumentSymbols(info.File)
	if err != nil {
		return ""
	}
	return FindEnclosingImplTrait(symbols, info.Line-1) // info.Line is 1-indexed; DocSymbol is 0-indexed
}

// rustModuleMatches returns true when expected is a suffix of the actual
// container path (actual may carry extra leading segments). An expected of
// ["Greeter"] matches any container ending in Greeter; ["greet", "Greeter"]
// requires both.
func rustModuleMatches(expected, actualMod []string, actualType string) bool {
	full := append([]string{}, actualMod...)
	if actualType != "" {
		full = append(full, actualType)
	}
	if len(expected) == 0 {
		return true
	}
	if len(full) == 0 {
		// rust-analyzer omits containerName for some module-level functions;
		// accept any name match when no container info is available.
		return true
	}
	// Use the shorter length for suffix matching: rust-analyzer may return an
	// abbreviated container (e.g., "util" instead of "greet::util").
	n := len(full)
	if len(expected) < n {
		n = len(expected)
	}
	for i := 0; i < n; i++ {
		if full[len(full)-n+i] != expected[len(expected)-n+i] {
			return false
		}
	}
	return true
}
