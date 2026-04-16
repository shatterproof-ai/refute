package symbol

import (
	"fmt"
	"os"
	"strings"
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

	line := lines[lineIdx]
	col := strings.Index(line, query.Name)
	if col < 0 {
		return Location{}, fmt.Errorf("name %q not found on line %d of %s", query.Name, query.Line, query.File)
	}

	return Location{
		File:   query.File,
		Line:   query.Line,
		Column: col + 1, // Convert 0-indexed to 1-indexed.
		Name:   query.Name,
		Kind:   query.Kind,
	}, nil
}
