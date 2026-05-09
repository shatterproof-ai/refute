package edit

import "testing"

func TestStripSnippetPlaceholders(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no placeholders", "fn foo() {}", "fn foo() {}"},
		{"tabstop zero", "fn $0() {}", "fn () {}"},
		{"tabstop numbered", "let $1 = 0;", "let  = 0;"},
		{"placeholder with default", "let ${1:x} = 0;", "let x = 0;"},
		{"placeholder nested default", "let ${1:vec![${2:0}]} = ();", "let vec![0] = ();"},
		{"choice", "fn ${1|foo,bar|}() {}", "fn foo() {}"},
		{"escaped dollar preserved", `printf("$$0")`, `printf("$0")`},
		// Note: "$5 bill" (literal dollar-digit) is indistinguishable from a
		// snippet tabstop by regex; the stripper is only called on code-action
		// newText where such literals do not appear in practice.
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StripSnippetPlaceholders(c.in)
			if got != c.want {
				t.Errorf("StripSnippetPlaceholders(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestHasSnippetPlaceholders(t *testing.T) {
	cases := map[string]bool{
		"fn foo() {}":     false,
		"fn $0() {}":      true,
		"let ${1:x} = 0;": true,
		`"$5 bill"`:       true, // detector is syntactic, not semantic
	}
	for in, want := range cases {
		if got := HasSnippetPlaceholders(in); got != want {
			t.Errorf("HasSnippetPlaceholders(%q) = %v, want %v", in, got, want)
		}
	}
}
