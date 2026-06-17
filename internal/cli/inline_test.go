package cli

import (
	"path/filepath"
	"testing"
)

func TestParseCallSite(t *testing.T) {
	cases := []struct {
		in      string
		wantF   string // relative path; compared after filepath.Abs
		wantL   int
		wantC   int
		wantErr bool
	}{
		{"src/main.rs:3:14", "src/main.rs", 3, 14, false},
		// A Windows drive path contains a colon; line/col come from the last two
		// fields so the path survives intact.
		{`C:\proj\main.rs:3:14`, `C:\proj\main.rs`, 3, 14, false},
		{"bad", "", 0, 0, true},
		{"src:not-a-number:3", "", 0, 0, true},
		{"src:3:not-a-number", "", 0, 0, true},
		{":3:14", "", 0, 0, true},        // empty path
		{"src.rs:0:14", "", 0, 0, true},  // non-positive line
		{"src.rs:3:0", "", 0, 0, true},   // non-positive column
		{"src.rs:-1:14", "", 0, 0, true}, // negative line
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
		wantAbs, _ := filepath.Abs(c.wantF)
		if got.File != wantAbs || got.Line != c.wantL || got.Column != c.wantC {
			t.Errorf("%q → %+v (want file %q line %d col %d)", c.in, got, wantAbs, c.wantL, c.wantC)
		}
	}
}
