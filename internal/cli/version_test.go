package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_PrintsAllFields(t *testing.T) {
	origVersion, origCommit, origBuildDate := Version, Commit, BuildDate
	t.Cleanup(func() {
		Version, Commit, BuildDate = origVersion, origCommit, origBuildDate
	})

	Version = "9.9.9-test"
	Commit = "deadbeef"
	BuildDate = "2026-04-30T00:00:00Z"

	var buf bytes.Buffer
	versionCmd.SetOut(&buf)
	versionCmd.SetErr(&buf)
	versionCmd.SetArgs(nil)
	if err := versionCmd.Execute(); err != nil {
		t.Fatalf("versionCmd.Execute: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"9.9.9-test", "deadbeef", "2026-04-30T00:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Errorf("version output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestVersionCommand_DefaultsAreReadable(t *testing.T) {
	if Version == "" {
		t.Error("Version default must not be empty")
	}
	if Commit == "" {
		t.Error("Commit default must not be empty")
	}
	if BuildDate == "" {
		t.Error("BuildDate default must not be empty")
	}
	if !strings.Contains(Version, "dev") {
		t.Errorf("default Version should signal a development build, got %q", Version)
	}
}
