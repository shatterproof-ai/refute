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

// TestVerifyReportTargetWiredInMakefile checks the keep-going audit entry point
// exists (issue #120): a `verify-report` target that delegates to verify.sh in
// keep-going mode.
func TestVerifyReportTargetWiredInMakefile(t *testing.T) {
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "verify-report:") {
		t.Errorf("Makefile missing verify-report target")
	}
	if !strings.Contains(content, "--keep-going") {
		t.Errorf("Makefile verify-report should invoke scripts/verify.sh --keep-going")
	}
}

// TestVerifyScriptHelpAdvertisesModes checks that the new keep-going / fail-fast
// flags are documented in the usage text so agents can discover them (issue
// #120).
func TestVerifyScriptHelpAdvertisesModes(t *testing.T) {
	code, out := exitCode(t, exec.Command("bash", "../scripts/verify.sh", "--help"))
	if code != 0 {
		t.Fatalf("verify.sh --help exit = %d, want 0\n%s", code, out)
	}
	for _, want := range []string{"--keep-going", "--fail-fast"} {
		if !strings.Contains(out, want) {
			t.Errorf("verify.sh --help missing %q:\n%s", want, out)
		}
	}
}

// TestVerifySelftestKeepGoing drives the script's synthetic self-test harness in
// keep-going mode. A failing synthetic check must NOT hide the later checks: the
// summary distinguishes failed, skipped, and unavailable outcomes, the run
// reaches the final synthetic check, and it exits non-zero because a required
// check failed (issue #120).
func TestVerifySelftestKeepGoing(t *testing.T) {
	code, out := exitCode(t, exec.Command("bash", "../scripts/verify.sh", "--selftest", "--keep-going"))
	if code != 1 {
		t.Fatalf("verify.sh --selftest --keep-going exit = %d, want 1\n%s", code, out)
	}
	// Keep-going: the check after the failing one still ran.
	if !strings.Contains(out, "selftest-pass-2") {
		t.Errorf("keep-going did not run checks after the first failure:\n%s", out)
	}
	// The three outcomes are reported distinctly.
	for _, want := range []string{"FAIL selftest-fail", "SKIP selftest-skip", "UNAVAIL selftest-unavail"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing distinct outcome %q:\n%s", want, out)
		}
	}
	// The summary line counts all four states separately.
	for _, want := range []string{"failed", "skipped", "unavailable"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q tally:\n%s", want, out)
		}
	}
}

// TestVerifySelftestFailFast drives the synthetic harness in fail-fast mode: the
// run stops at the first failing check and never reaches the later one, while
// still exiting non-zero (issue #120 — fail-fast preserved as an option).
func TestVerifySelftestFailFast(t *testing.T) {
	code, out := exitCode(t, exec.Command("bash", "../scripts/verify.sh", "--selftest", "--fail-fast"))
	if code != 1 {
		t.Fatalf("verify.sh --selftest --fail-fast exit = %d, want 1\n%s", code, out)
	}
	if strings.Contains(out, "selftest-pass-2") {
		t.Errorf("fail-fast kept running after the first failure:\n%s", out)
	}
	if !strings.Contains(out, "FAIL selftest-fail") {
		t.Errorf("fail-fast did not report the failing check:\n%s", out)
	}
}
