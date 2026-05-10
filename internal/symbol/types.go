package symbol

// SymbolKind classifies symbols for kind-specific rename commands.
type SymbolKind int

const (
	KindUnknown   SymbolKind = iota
	KindFunction
	KindClass
	KindField
	KindVariable
	KindParameter
	KindType
	KindMethod
)

func (k SymbolKind) String() string {
	names := [...]string{
		"unknown", "function", "class", "field",
		"variable", "parameter", "type", "method",
	}
	if int(k) < len(names) {
		return names[k]
	}
	return "unknown"
}

// Location identifies a symbol in a source file. Line and Column are 1-indexed.
type Location struct {
	File      string
	Line      int
	Column    int
	Name      string
	Kind      SymbolKind
	Container string // workspace/symbol containerName; used by Rust disambiguation
}

// SourceRange identifies a range of source code. 1-indexed.
type SourceRange struct {
	File      string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Query represents a request to find a symbol.
type Query struct {
	QualifiedName string
	File          string
	Line          int
	Column        int
	Name          string
	Kind          SymbolKind
}

// Tier returns which resolution tier this query uses.
func (q Query) Tier() int {
	if q.QualifiedName != "" {
		return 1
	}
	if q.Column > 0 {
		return 3
	}
	if q.Name != "" {
		return 2
	}
	return 0
}
