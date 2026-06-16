package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpDocumentsExitCodes(t *testing.T) {
	var buf bytes.Buffer
	RootCmd.SetOut(&buf)
	RootCmd.SetErr(&buf)
	RootCmd.SetArgs([]string{"--help"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("refute --help: %v", err)
	}

	out := buf.String()
	for _, want := range []string{
		"EXIT CODES",
		"0  command succeeded",
		"1  general failure",
		"2  no edits or no matching symbol",
		"3  required backend missing",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("root help missing %q:\n%s", want, out)
		}
	}
}
