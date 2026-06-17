package internal

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// exitCode runs the command and returns its exit code, stdout+stderr combined.
func exitCode(t *testing.T, cmd *exec.Cmd) (int, string) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("run %v: %v", cmd.Args, err)
	return -1, ""
}

// TestVerifyScriptHelp checks that --help is a clean exit (0) and prints usage,
// distinct from the failure and unsupported-environment exit codes.
func TestVerifyScriptHelp(t *testing.T) {
	code, out := exitCode(t, exec.Command("bash", "../scripts/verify.sh", "--help"))
	if code != 0 {
		t.Fatalf("verify.sh --help exit = %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "usage:") {
		t.Errorf("verify.sh --help missing usage text:\n%s", out)
	}
}

// TestVerifyScriptUnknownArg checks that a bad argument exits 2 (unsupported
// invocation), keeping it distinct from a real check failure (exit 1).
func TestVerifyScriptUnknownArg(t *testing.T) {
	code, out := exitCode(t, exec.Command("bash", "../scripts/verify.sh", "--no-such-flag"))
	if code != 2 {
		t.Fatalf("verify.sh --no-such-flag exit = %d, want 2\n%s", code, out)
	}
}

// TestVerifyScriptUnsupportedEnv checks that a missing Go toolchain is reported
// as an unsupported environment (exit 2), not a check failure (exit 1). The
// empty PATH makes `command -v go` fail before any check runs.
func TestVerifyScriptUnsupportedEnv(t *testing.T) {
	cmd := exec.Command("bash", "../scripts/verify.sh")
	cmd.Env = []string{"PATH="}
	code, out := exitCode(t, cmd)
	if code != 2 {
		t.Fatalf("verify.sh with no go on PATH exit = %d, want 2\n%s", code, out)
	}
	if !strings.Contains(out, "unsupported environment") {
		t.Errorf("verify.sh missing unsupported-environment message:\n%s", out)
	}
}

// TestVerifyTargetWiredInMakefile checks the documented entry point exists: the
// Makefile `verify` target delegates to scripts/verify.sh.
func TestVerifyTargetWiredInMakefile(t *testing.T) {
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)
	for _, want := range []string{"verify:", "scripts/verify.sh"} {
		if !strings.Contains(content, want) {
			t.Errorf("Makefile missing %q", want)
		}
	}
}
