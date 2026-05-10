package cli

import "testing"

func TestParseCallSite(t *testing.T) {
	cases := []struct {
		in      string
		wantF   string
		wantL   int
		wantC   int
		wantErr bool
	}{
		{"src/main.rs:3:14", "src/main.rs", 3, 14, false},
		{"bad", "", 0, 0, true},
		{"src:not-a-number:3", "", 0, 0, true},
		{"src:3:not-a-number", "", 0, 0, true},
	}
	for _, c := range cases {
		got, err := parseCallSite(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("err=%v wantErr=%v for %q", err, c.wantErr, c.in)
			continue
		}
		if c.wantErr {
			continue
		}
		if got.File != c.wantF || got.Line != c.wantL || got.Column != c.wantC {
			t.Errorf("%q → %+v", c.in, got)
		}
	}
}
