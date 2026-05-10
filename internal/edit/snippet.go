package edit

import "regexp"

// LSP snippet syntax (https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#snippet_syntax):
//
//	$0, $1, $2, ...     — tabstops
//	${1:default}        — placeholder with default (may nest)
//	${1|one,two,three|} — choice (first listed option becomes the default)
//	$$                  — escaped literal dollar

var (
	tabstopRe     = regexp.MustCompile(`\$[0-9]+`)
	placeholderRe = regexp.MustCompile(`\$\{[0-9]+:([^{}]*(?:\{[^{}]*\})?[^{}]*)\}`)
	choiceRe      = regexp.MustCompile(`\$\{[0-9]+\|([^,|]+)[^|]*\|\}`)
	escapedRe     = regexp.MustCompile(`\$\$`)
)

const escapedDollarSentinel = "\x00REFUTE_DOLLAR\x00"

// HasSnippetPlaceholders reports whether s contains any LSP snippet tokens.
// Detection is syntactic; callers should invoke this only on code-action newText.
func HasSnippetPlaceholders(s string) bool {
	return tabstopRe.MatchString(s) || placeholderRe.MatchString(s) || choiceRe.MatchString(s)
}

// ReplaceFirstPlaceholder substitutes the first $N or ${N:...} token with
// name. Intended for code-action edits where a single variable/function name
// placeholder represents the user-chosen identifier.
func ReplaceFirstPlaceholder(s, name string) string {
	if m := placeholderRe.FindStringIndex(s); m != nil {
		return s[:m[0]] + name + s[m[1]:]
	}
	if m := tabstopRe.FindStringIndex(s); m != nil {
		return s[:m[0]] + name + s[m[1]:]
	}
	return s
}

// StripSnippetPlaceholders removes LSP snippet tokens from s:
//   - ${N:default}  → default
//   - ${N|a,b,c|}   → a  (first choice)
//   - $N            → (empty string)
//   - $$            → $
//
// Nested placeholders are handled by iterating until stable (max 16 passes).
func StripSnippetPlaceholders(s string) string {
	s = escapedRe.ReplaceAllLiteralString(s, escapedDollarSentinel)
	for i := 0; i < 16; i++ {
		next := choiceRe.ReplaceAllString(s, "$1")
		next = placeholderRe.ReplaceAllString(next, "$1")
		next = tabstopRe.ReplaceAllString(next, "")
		if next == s {
			break
		}
		s = next
	}
	return regexp.MustCompile(regexp.QuoteMeta(escapedDollarSentinel)).
		ReplaceAllLiteralString(s, "$")
}
