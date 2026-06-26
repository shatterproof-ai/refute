package lsp

import "testing"

func TestDocumentSymbolKindAt(t *testing.T) {
	// A top-level function on line 4 (0-indexed 3) and a type with a method
	// child, so the deepest-match rule can be exercised.
	syms := []DocumentSymbol{
		{
			Name:           "FormatGreeting",
			Kind:           12, // Function
			SelectionRange: Range{Start: Position{Line: 3, Character: 5}},
		},
		{
			Name:           "User",
			Kind:           23, // Struct
			Range:          Range{Start: Position{Line: 9}, End: Position{Line: 20}},
			SelectionRange: Range{Start: Position{Line: 9, Character: 5}},
			Children: []DocumentSymbol{
				{
					Name:           "Greet",
					Kind:           6, // Method
					SelectionRange: Range{Start: Position{Line: 12, Character: 1}},
				},
			},
		},
	}

	cases := []struct {
		name     string
		line     int
		symName  string
		wantKind int
		wantOK   bool
	}{
		{name: "function by line+name", line: 3, symName: "FormatGreeting", wantKind: 12, wantOK: true},
		{name: "function by line only", line: 3, symName: "", wantKind: 12, wantOK: true},
		{name: "nested method", line: 12, symName: "Greet", wantKind: 6, wantOK: true},
		{name: "type declaration", line: 9, symName: "User", wantKind: 23, wantOK: true},
		{name: "name mismatch on line", line: 3, symName: "Other", wantOK: false},
		{name: "no symbol on line", line: 99, symName: "", wantOK: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, ok := documentSymbolKindAt(syms, tc.line, tc.symName)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && kind != tc.wantKind {
				t.Errorf("kind = %d, want %d", kind, tc.wantKind)
			}
		})
	}
}
