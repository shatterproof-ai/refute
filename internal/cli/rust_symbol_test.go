package cli

import "testing"

func TestParseRustQualifiedName(t *testing.T) {
	cases := []struct {
		name, in            string
		wantModule          []string
		wantTrait, wantName string
		wantErr             bool
	}{
		{"form 1 bare", "format_greeting", nil, "", "format_greeting", false},
		{"form 2 module", "greet::format_greeting", []string{"greet"}, "", "format_greeting", false},
		{"form 3 crate prefix", "crate::greet::format_greeting", []string{"greet"}, "", "format_greeting", false},
		{"form 4 assoc fn", "Greeter::new", []string{"Greeter"}, "", "new", false},
		{"form 5 method", "Greeter::greet", []string{"Greeter"}, "", "greet", false},
		{"form 7 module+type+method", "greet::Greeter::new", []string{"greet", "Greeter"}, "", "new", false},
		{"form 6 trait qualified", "<Greeter as Display>::fmt", []string{"Greeter"}, "Display", "fmt", false},
		{"form 6 nested generics", "<Vec<T> as IntoIterator>::next", []string{"Vec<T>"}, "IntoIterator", "next", false},
		{"form 6+7", "greet::<Greeter as Display>::fmt", []string{"greet", "Greeter"}, "Display", "fmt", false},

		{"err parens", "foo()", nil, "", "", true},
		{"err whitespace", "foo bar", nil, "", "", true},
		{"err unmatched angle", "<Greeter as Display::fmt", nil, "", "", true},
		{"err missing as", "<Greeter Display>::fmt", nil, "", "", true},
		{"err trailing colons", "greet::", nil, "", "", true},
		{"err empty", "", nil, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod, tr, n, err := ParseRustQualifiedName(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if !stringSliceEq(mod, c.wantModule) {
				t.Errorf("module=%v want %v", mod, c.wantModule)
			}
			if tr != c.wantTrait {
				t.Errorf("trait=%q want %q", tr, c.wantTrait)
			}
			if n != c.wantName {
				t.Errorf("name=%q want %q", n, c.wantName)
			}
		})
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
