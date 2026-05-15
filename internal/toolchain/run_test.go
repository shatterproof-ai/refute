package toolchain

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunPreservesArgsAndOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is unix-only")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ActiveBinPath), "#!/bin/sh\necho \"$1:$2\"\nexit 7\n")
	if err := os.Chmod(filepath.Join(root, ActiveBinPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	err := Run(context.Background(), RunOptions{
		ProjectRoot: root,
		Args:        []string{"alpha", "beta"},
		Stdout:      &stdout,
	})
	if err == nil {
		t.Fatal("Run unexpectedly succeeded")
	}
	if strings.TrimSpace(stdout.String()) != "alpha:beta" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDoctorReportsStateAndDelegates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is unix-only")
	}
	root := t.TempDir()
	writeFile(t, filepath.Join(root, LockfileName), "{}")
	writeFile(t, filepath.Join(root, ActiveBinPath), "#!/bin/sh\necho delegated:$1\n")
	if err := os.Chmod(filepath.Join(root, ActiveBinPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Doctor(context.Background(), DoctorOptions{ProjectRoot: root, Stdout: &stdout}); err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	got := stdout.String()
	for _, want := range []string{"lockfile: present", "binary: present", "delegated:doctor"} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}
