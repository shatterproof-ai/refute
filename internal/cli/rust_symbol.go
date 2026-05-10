package cli

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseRustQualifiedName parses a user-supplied --symbol value for Rust.
// Supported forms (see spec for numbering):
//
//	1: name
//	2: module::name
//	3: crate::module::name         (crate:: stripped)
//	4: Type::assoc_fn
//	5: Type::method
//	6: <Type as Trait>::method
//	7: module::Type::method
//	6+7: module::<Type as Trait>::method
//
// Returns (modulePath, trait, name). modulePath ends with the type if the
// input names a method or associated function; the caller uses this for
// workspace/symbol container matching.
func ParseRustQualifiedName(s string) ([]string, string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", "", fmt.Errorf("empty symbol")
	}
	if err := validateRustSymbolChars(s); err != nil {
		return nil, "", "", err
	}

	var modulePrefix []string
	if i := strings.Index(s, "<"); i >= 0 && (i == 0 || strings.HasSuffix(s[:i], "::")) {
		// Form 6 (possibly combined with module prefix).
		if i > 0 {
			head := strings.TrimSuffix(s[:i], "::")
			head = strings.TrimPrefix(head, "crate::")
			modulePrefix = splitNonEmpty(head, "::")
			s = s[i:]
		}
		inner, tail, err := splitTraitQualified(s)
		if err != nil {
			return nil, "", "", err
		}
		typ, trait, err := parseTypeAsTrait(inner)
		if err != nil {
			return nil, "", "", err
		}
		name, err := requireSingleSegment(tail)
		if err != nil {
			return nil, "", "", err
		}
		mod := append(modulePrefix, typ)
		return mod, trait, name, nil
	}

	// Forms 1-5, 7.
	s = strings.TrimPrefix(s, "crate::")
	parts := strings.Split(s, "::")
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return nil, "", "", fmt.Errorf("empty segment in %q", s)
		}
	}
	name := parts[len(parts)-1]
	module := parts[:len(parts)-1]
	if len(module) == 0 {
		return nil, "", name, nil
	}
	return module, "", name, nil
}

func validateRustSymbolChars(s string) error {
	// Permit angle brackets (form 6), colons, underscore, alphanumerics,
	// and ampersands/commas/spaces *inside* angle brackets (generics).
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case '(', ')', '{', '}', ';':
			return fmt.Errorf("invalid character %q in symbol", r)
		default:
			if depth == 0 && unicode.IsSpace(r) {
				return fmt.Errorf("whitespace not permitted outside <...>")
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced angle brackets in symbol")
	}
	return nil
}

// splitTraitQualified splits "<Type as Trait>::name" into ("Type as Trait", "name").
// The input must start with '<'. Tracks nested angle-bracket depth for
// generics like <Vec<T> as IntoIterator>.
func splitTraitQualified(s string) (inner, tail string, err error) {
	if !strings.HasPrefix(s, "<") {
		return "", "", fmt.Errorf("trait-qualified form must start with '<'")
	}
	depth := 0
	for i, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				rest := s[i+1:]
				if !strings.HasPrefix(rest, "::") {
					return "", "", fmt.Errorf("expected '::' after '>'")
				}
				return s[1:i], rest[2:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unmatched '<'")
}

func parseTypeAsTrait(inner string) (typ, trait string, err error) {
	// Look for " as " at top-level angle-bracket depth.
	depth := 0
	for i := 0; i+4 <= len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
			continue
		case '>':
			depth--
			continue
		}
		if depth != 0 {
			continue
		}
		if inner[i] == ' ' && i+4 <= len(inner) && inner[i:i+4] == " as " {
			typ = strings.TrimSpace(inner[:i])
			trait = strings.TrimSpace(inner[i+4:])
			if typ == "" || trait == "" {
				return "", "", fmt.Errorf("empty type or trait in %q", inner)
			}
			return typ, trait, nil
		}
	}
	return "", "", fmt.Errorf("missing ' as ' in trait qualification %q", inner)
}

func requireSingleSegment(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("missing method name after '>::'")
	}
	if strings.Contains(s, "::") {
		return "", fmt.Errorf("unexpected '::' in method name %q", s)
	}
	return s, nil
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}
