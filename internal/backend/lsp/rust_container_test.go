package lsp

import "testing"

func TestParseRustContainer_Cheap(t *testing.T) {
	cases := []struct {
		name, in            string
		wantModule          []string
		wantType, wantTrait string
	}{
		{"plain type", "Greeter", nil, "Greeter", ""},
		{"module path", "greet::Greeter", []string{"greet"}, "Greeter", ""},
		{"trait qualified", "impl Display for Greeter", nil, "Greeter", "Display"},
		{"trait qualified with module", "greet::impl Display for Greeter", []string{"greet"}, "Greeter", "Display"},
		{"paren style", "Greeter (Display)", nil, "Greeter", "Display"},
		{"empty", "", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod, typ, tr := parseRustContainer(c.in)
			if !stringSliceEq(mod, c.wantModule) {
				t.Errorf("module = %v, want %v", mod, c.wantModule)
			}
			if typ != c.wantType {
				t.Errorf("type = %q, want %q", typ, c.wantType)
			}
			if tr != c.wantTrait {
				t.Errorf("trait = %q, want %q", tr, c.wantTrait)
			}
		})
	}
}

func TestFindEnclosingImplTrait(t *testing.T) {
	doc := []DocumentSymbol{
		{
			Name:  "impl Display for Greeter",
			Range: Range{Start: Position{Line: 10}, End: Position{Line: 20}},
			Children: []DocumentSymbol{
				{Name: "fmt", Range: Range{Start: Position{Line: 12}, End: Position{Line: 14}}},
			},
		},
		{
			Name:  "impl Debug for Greeter",
			Range: Range{Start: Position{Line: 22}, End: Position{Line: 30}},
			Children: []DocumentSymbol{
				{Name: "fmt", Range: Range{Start: Position{Line: 24}, End: Position{Line: 26}}},
			},
		},
	}
	if got := FindEnclosingImplTrait(doc, 13); got != "Display" {
		t.Errorf("line 13 → %q, want Display", got)
	}
	if got := FindEnclosingImplTrait(doc, 25); got != "Debug" {
		t.Errorf("line 25 → %q, want Debug", got)
	}
	if got := FindEnclosingImplTrait(doc, 100); got != "" {
		t.Errorf("line 100 → %q, want empty", got)
	}
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
