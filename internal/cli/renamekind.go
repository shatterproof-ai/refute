package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// languageKinds records which SymbolKinds the rename-* variants accept for each
// language. It encodes the deliberate gaps between refute's kind vocabulary and
// a language's actual concepts: Go and Rust have no `class` (their nominal types
// are `type`), so rename-class can never apply there. A language absent from the
// map is treated permissively — every kind is allowed — so a newly added backend
// is never blocked by a missing entry before its mapping is reviewed.
//
// The canonical, user-facing version of this table lives in the rename command
// Long help (see renameKindMappingHelp); keep the two in sync.
//
// Cross-reference: this table must be kept in sync with languageProfiles in
// internal/backend/lsp/profile.go. When a language gains an LSP backend there,
// add a matching entry here too. Because kindValidForLanguage treats an absent
// language permissively (every kind allowed), forgetting this entry does not
// fail loudly — it silently skips upfront kind validation for that language, so
// rename-class against a language that has no class concept would slip through.
var languageKinds = map[string]map[symbol.SymbolKind]bool{
	"go":         kindSet(symbol.KindFunction, symbol.KindField, symbol.KindVariable, symbol.KindParameter, symbol.KindType, symbol.KindMethod),
	"rust":       kindSet(symbol.KindFunction, symbol.KindField, symbol.KindVariable, symbol.KindParameter, symbol.KindType, symbol.KindMethod),
	"typescript": kindSet(symbol.KindFunction, symbol.KindClass, symbol.KindField, symbol.KindVariable, symbol.KindParameter, symbol.KindType, symbol.KindMethod),
	"javascript": kindSet(symbol.KindFunction, symbol.KindClass, symbol.KindField, symbol.KindVariable, symbol.KindParameter, symbol.KindType, symbol.KindMethod),
	"python":     kindSet(symbol.KindFunction, symbol.KindClass, symbol.KindField, symbol.KindVariable, symbol.KindParameter, symbol.KindType, symbol.KindMethod),
}

func kindSet(kinds ...symbol.SymbolKind) map[symbol.SymbolKind]bool {
	set := make(map[symbol.SymbolKind]bool, len(kinds))
	for _, k := range kinds {
		set[k] = true
	}
	return set
}

// kindValidForLanguage reports whether the requested kind is a concept the
// language has at all. KindUnknown (plain rename) is always valid. Unknown
// languages are permissive.
func kindValidForLanguage(language string, kind symbol.SymbolKind) bool {
	if kind == symbol.KindUnknown {
		return true
	}
	set, ok := languageKinds[language]
	if !ok {
		return true
	}
	return set[kind]
}

// ErrKindMismatch reports that a rename-<kind> variant resolved a symbol whose
// actual kind is not the kind the variant requires (or that the variant's kind
// does not exist in the target language at all). It is raised before any edit is
// computed so a mismatched variant never silently rewrites the wrong concept.
type ErrKindMismatch struct {
	SymbolName string
	Language   string
	Requested  symbol.SymbolKind
	Actual     symbol.SymbolKind // KindUnknown when the actual kind could not be determined
	// LangInvalid is set when Requested is not a concept the language has at
	// all (e.g. rename-class in Go), which sharpens the message.
	LangInvalid bool
}

func (e *ErrKindMismatch) ExitCode() int { return 1 }

func (e *ErrKindMismatch) Error() string {
	subject := "the symbol"
	if e.SymbolName != "" {
		subject = fmt.Sprintf("%q", e.SymbolName)
	}
	switch {
	case e.LangInvalid && e.Actual != symbol.KindUnknown:
		return fmt.Sprintf("%s has no %s symbols; %s is a %s. %s",
			e.Language, e.Requested, subject, e.Actual, e.suggestion())
	case e.LangInvalid:
		return fmt.Sprintf("%s has no %s symbols. %s",
			e.Language, e.Requested, e.suggestion())
	default:
		return fmt.Sprintf("%s is a %s, not a %s. %s",
			subject, e.Actual, e.Requested, e.suggestion())
	}
}

// Hint is the remediation surfaced in JSON output; it mirrors the suggestion in
// Error so human and --json modes recommend the same fix.
func (e *ErrKindMismatch) Hint() string { return e.suggestion() }

// suggestion points at the kind-agnostic `rename` plus, when the actual kind is
// known and has its own variant, the matching rename-<actual> command.
func (e *ErrKindMismatch) suggestion() string {
	if cmd := kindCommand(e.Actual); cmd != "" {
		return fmt.Sprintf("Use `rename` (kind-agnostic) or `%s`.", cmd)
	}
	return "Use `rename` (kind-agnostic) to rename any symbol."
}

// kindCommand returns the rename-<kind> command name for a kind, or "" for
// KindUnknown which has no dedicated variant.
func kindCommand(kind symbol.SymbolKind) string {
	if kind == symbol.KindUnknown {
		return ""
	}
	return "rename-" + kind.String()
}

// validateResolvedKind compares the requested variant kind against the symbol's
// actual kind and returns an *ErrKindMismatch when they conflict. It is the
// single decision point shared by every resolution tier:
//
//   - requested KindUnknown (plain rename): always nil — rename stays kind-agnostic.
//   - actual kind known and equal to requested: nil.
//   - actual kind known and different: mismatch naming the actual kind.
//   - actual kind unknown (probe missed): reject only when the requested kind
//     does not exist in the language at all; otherwise allow, so a kind-correct
//     rename never breaks on a probe miss and behaves exactly like plain rename.
func validateResolvedKind(language string, requested, actual symbol.SymbolKind, symbolName string) error {
	if requested == symbol.KindUnknown {
		return nil
	}
	langInvalid := !kindValidForLanguage(language, requested)
	if actual == symbol.KindUnknown {
		if langInvalid {
			return &ErrKindMismatch{SymbolName: symbolName, Language: language, Requested: requested, Actual: symbol.KindUnknown, LangInvalid: true}
		}
		return nil
	}
	if actual == requested {
		return nil
	}
	return &ErrKindMismatch{SymbolName: symbolName, Language: language, Requested: requested, Actual: actual, LangInvalid: langInvalid}
}

// probeSymbolKind asks the backend for the actual kind of the symbol at loc.
// Backends that do not implement backend.KindResolver, or that cannot classify
// the symbol, yield KindUnknown, which validateResolvedKind treats as "do not
// reject on this basis alone".
func probeSymbolKind(b backend.RefactoringBackend, loc symbol.Location) symbol.SymbolKind {
	resolver, ok := b.(backend.KindResolver)
	if !ok {
		return symbol.KindUnknown
	}
	kind, err := resolver.ResolveKind(loc)
	if err != nil {
		return symbol.KindUnknown
	}
	return kind
}

// renameKindMappingHelp is the user-facing language→kind mapping appended to
// each rename-<kind> command's Long help. It documents the gaps that make some
// variants inapplicable to some languages (so an "always errors here" outcome is
// discoverable from --help, not only from an error).
var renameKindMappingHelp = buildRenameKindMappingHelp()

func buildRenameKindMappingHelp() string {
	var b strings.Builder
	b.WriteString("Kind validation: this variant rejects a target whose actual kind differs\n")
	b.WriteString("from the requested kind before any edit is computed; plain `rename` stays\n")
	b.WriteString("kind-agnostic. Applicable kinds per language:\n")
	languages := make([]string, 0, len(languageKinds))
	for lang := range languageKinds {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	for _, lang := range languages {
		b.WriteString(fmt.Sprintf("  - %s: %s\n", lang, strings.Join(sortedKindNames(languageKinds[lang]), ", ")))
	}
	b.WriteString("Kinds absent from a language (e.g. class in Go and Rust) always error there.")
	return b.String()
}

func sortedKindNames(set map[symbol.SymbolKind]bool) []string {
	names := make([]string, 0, len(set))
	for k := range set {
		names = append(names, k.String())
	}
	sort.Strings(names)
	return names
}
