package symbol

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Resolve converts a Query into a concrete Location by reading the source file
// and finding the symbol. Supports Tier 2 (file+line+name) and Tier 3 (file+line+col).
// Tier 1 (qualified name) requires a backend and is handled separately.
func Resolve(query Query) (Location, error) {
	switch query.Tier() {
	case 3:
		return resolveTier3(query), nil
	case 2:
		return resolveTier2(query)
	case 1:
		return Location{}, fmt.Errorf("tier 1 (qualified name) resolution requires a backend")
	default:
		return Location{}, fmt.Errorf("invalid query: must specify symbol, file+line+name, or file+line+col")
	}
}

func resolveTier3(query Query) Location {
	return Location{
		File:   query.File,
		Line:   query.Line,
		Column: query.Column,
		Kind:   query.Kind,
	}
}

func resolveTier2(query Query) (Location, error) {
	content, err := os.ReadFile(query.File)
	if err != nil {
		return Location{}, fmt.Errorf("reading %s: %w", query.File, err)
	}

	lines := strings.Split(string(content), "\n")
	lineIdx := query.Line - 1 // Convert 1-indexed to 0-indexed.
	if lineIdx < 0 || lineIdx >= len(lines) {
		return Location{}, fmt.Errorf("line %d out of range (file has %d lines)", query.Line, len(lines))
	}

	matches := findNameMatches(lines, lineIdx, query.Name)
	if len(matches) == 0 {
		return Location{}, fmt.Errorf("name %q not found on line %d of %s", query.Name, query.Line, query.File)
	}
	if len(matches) > 1 {
		return Location{}, fmt.Errorf("name %q is ambiguous on line %d of %s", query.Name, query.Line, query.File)
	}

	return Location{
		File:   query.File,
		Line:   query.Line,
		Column: matches[0] + 1, // Convert 0-indexed byte offset to 1-indexed.
		Name:   query.Name,
		Kind:   query.Kind,
	}, nil
}

func findNameMatches(lines []string, lineIdx int, name string) []int {
	line := lines[lineIdx]
	code := codeMaskForLine(lines, lineIdx)
	var matches []int
	for searchFrom := 0; searchFrom <= len(line); {
		idx := strings.Index(line[searchFrom:], name)
		if idx < 0 {
			break
		}
		idx += searchFrom
		end := idx + len(name)
		if isCodeRange(code, idx, end) && hasIdentifierBoundaries(line, idx, end) {
			matches = append(matches, idx)
		}
		searchFrom = idx + len(name)
	}
	return matches
}

func isCodeRange(code []bool, start, end int) bool {
	if start < 0 || end > len(code) || start >= end {
		return false
	}
	for i := start; i < end; i++ {
		if !code[i] {
			return false
		}
	}
	return true
}

func hasIdentifierBoundaries(line string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(line[:start])
		if isIdentifierRune(r) {
			return false
		}
	}
	if end < len(line) {
		r, _ := utf8.DecodeRuneInString(line[end:])
		if isIdentifierRune(r) {
			return false
		}
	}
	return true
}

func isIdentifierRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func codeMaskForLine(lines []string, targetLine int) []bool {
	const (
		stateCode = iota
		stateBlockComment
		stateDoubleString
		stateSingleString
		stateRawString
	)
	state := stateCode
	var targetMask []bool
	for lineIdx, line := range lines {
		mask := make([]bool, len(line))
		for i := 0; i < len(line); {
			switch state {
			case stateBlockComment:
				if strings.HasPrefix(line[i:], "*/") {
					i += 2
					state = stateCode
				} else {
					i++
				}
			case stateRawString:
				if line[i] == '`' {
					state = stateCode
				}
				i++
			case stateDoubleString:
				if line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == '"' {
					state = stateCode
				}
				i++
			case stateSingleString:
				if line[i] == '\\' && i+1 < len(line) {
					i += 2
					continue
				}
				if line[i] == '\'' {
					state = stateCode
				}
				i++
			default:
				switch {
				case strings.HasPrefix(line[i:], "//"):
					i = len(line)
				case strings.HasPrefix(line[i:], "/*"):
					i += 2
					state = stateBlockComment
				case line[i] == '"':
					i++
					state = stateDoubleString
				case line[i] == '\'':
					i++
					state = stateSingleString
				case line[i] == '`':
					i++
					state = stateRawString
				default:
					mask[i] = true
					i++
				}
			}
		}
		if state == stateDoubleString || state == stateSingleString {
			state = stateCode
		}
		if lineIdx == targetLine {
			targetMask = mask
			break
		}
	}
	return targetMask
}
