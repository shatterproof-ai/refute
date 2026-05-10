package lsp

import (
	"regexp"
	"strings"
)

// implForRe extracts (trait, type) from a rust-analyzer containerName of the
// form "impl <Trait> for <Type>". The trait may include `fmt::` prefixes.
var implForRe = regexp.MustCompile(`^impl\s+(\S+?)\s+for\s+(\S+)\s*$`)

// parenStyleRe extracts (type, trait) from "Type (Trait)".
var parenStyleRe = regexp.MustCompile(`^(\S+)\s*\(([^)]+)\)\s*$`)

// parseRustContainer normalizes rust-analyzer's `containerName` into
// (modulePath, type, trait). Rust-analyzer emits several formats across
// versions; we recognize:
//
//	"Greeter"                        → type="Greeter"
//	"greet::Greeter"                 → module=["greet"], type="Greeter"
//	"impl Display for Greeter"       → type="Greeter", trait="Display"
//	"greet::impl Display for Greeter"→ module=["greet"], type="Greeter", trait="Display"
//	"Greeter (Display)"              → type="Greeter", trait="Display"
//
// Unrecognized inputs yield (nil, container, "") — the type is assumed to be
// the whole string. Downstream matching tolerates that.
func parseRustContainer(container string) (modulePath []string, typ string, trait string) {
	container = strings.TrimSpace(container)
	if container == "" {
		return nil, "", ""
	}
	// Strip a trailing "impl X for Y" suffix, honouring possible module prefix.
	if idx := strings.LastIndex(container, "::"); idx >= 0 {
		head := container[:idx]
		tail := container[idx+2:]
		if m := implForRe.FindStringSubmatch(tail); m != nil {
			return splitModulePath(head), m[2], trimTraitPath(m[1])
		}
	}
	if m := implForRe.FindStringSubmatch(container); m != nil {
		return nil, m[2], trimTraitPath(m[1])
	}
	if m := parenStyleRe.FindStringSubmatch(container); m != nil {
		return nil, m[1], trimTraitPath(m[2])
	}
	// Plain module path.
	parts := strings.Split(container, "::")
	if len(parts) == 1 {
		return nil, parts[0], ""
	}
	return parts[:len(parts)-1], parts[len(parts)-1], ""
}

func splitModulePath(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "::")
}

// trimTraitPath keeps only the last segment of a path like "std::fmt::Display".
// This lets users pass `Display` without the full path.
func trimTraitPath(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "::")
	return parts[len(parts)-1]
}

// ParseRustContainer is the exported wrapper around parseRustContainer for use
// by packages outside the lsp package.
func ParseRustContainer(s string) ([]string, string, string) { return parseRustContainer(s) }

// FindEnclosingImplTrait walks a document-symbol tree to find the `impl ...`
// ancestor whose range contains targetLine (0-indexed). Returns the parsed
// trait name (last `::` segment) or "" if no such ancestor exists.
func FindEnclosingImplTrait(symbols []DocumentSymbol, targetLine int) string {
	for _, sym := range symbols {
		if targetLine < sym.Range.Start.Line || targetLine > sym.Range.End.Line {
			continue
		}
		if m := implForRe.FindStringSubmatch(sym.Name); m != nil {
			return trimTraitPath(m[1])
		}
		if t := FindEnclosingImplTrait(sym.Children, targetLine); t != "" {
			return t
		}
	}
	return ""
}
