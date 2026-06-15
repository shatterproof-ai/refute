package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// TestRootCmdSilencesCobraOutput asserts the root command suppresses cobra's
// own error and usage printing so cli.Run is the single error printer.
func TestRootCmdSilencesCobraOutput(t *testing.T) {
	if !RootCmd.SilenceUsage {
		t.Errorf("RootCmd.SilenceUsage = false, want true")
	}
	if !RootCmd.SilenceErrors {
		t.Errorf("RootCmd.SilenceErrors = false, want true")
	}
}

// TestSilenceUsageHelper is a subprocess helper. It registers a command under
// RootCmd that returns a runtime error and routes execution through cli.Run,
// mirroring the real cmd/refute entrypoint. The "boom" command intentionally
// does not set its own Silence flags so the test exercises inheritance from
// RootCmd.
func TestSilenceUsageHelper(t *testing.T) {
	if os.Getenv("REFUTE_SILENCE_HELPER") == "" {
		return
	}
	mode := os.Getenv("REFUTE_SILENCE_MODE")
	boom := &cobra.Command{
		Use: "boom",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode == "runtime-unknown-command-text" {
				return staticErr(`unknown command "backend" for "refute"`)
			}
			if mode == "json" {
				flagJSON = true
				ctx := jsonContext{Operation: "rename", Language: "go", Backend: "lsp", WorkspaceRoot: "/workspace"}
				return emitJSONOperationError(ctx, errStaticRuntimeBoom)
			}
			return errStaticRuntimeBoom
		},
	}
	RootCmd.AddCommand(boom)
	switch mode {
	case "unknown-flag":
		os.Args = []string{"refute", "boom", "--bad-flag"}
	case "unknown-command":
		os.Args = []string{"refute", "does-not-exist"}
	case "json-required-flag":
		os.Args = []string{"refute", "rename", "--json", "--file", "x.go", "--line", "1"}
	default:
		os.Args = []string{"refute", "boom"}
	}
	Run(Execute)
}

var errStaticRuntimeBoom = staticErr("runtime boom")

type staticErr string

func (e staticErr) Error() string { return string(e) }

func runSilenceHelper(t *testing.T, mode string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestSilenceUsageHelper")
	cmd.Env = append(os.Environ(),
		"REFUTE_SILENCE_HELPER=1",
		"REFUTE_SILENCE_MODE="+mode,
	)
	var so, se bytes.Buffer
	cmd.Stdout = &so
	cmd.Stderr = &se
	err := cmd.Run()
	return so.String(), se.String(), processExitCode(err)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// TestRuntimeError_HumanNoUsageSpam verifies a runtime error produces exactly
// one concise stderr message with no usage block and no doubled "Error:" line.
func TestRuntimeError_HumanNoUsageSpam(t *testing.T) {
	stdout, stderr, code := runSilenceHelper(t, "human")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Errorf("stderr contains usage block:\n%s", stderr)
	}
	if strings.Contains(stderr, "Error:") {
		t.Errorf("stderr contains cobra \"Error:\" prefix (duplicate printing):\n%s", stderr)
	}
	lines := nonEmptyLines(stderr)
	if len(lines) != 1 {
		t.Fatalf("expected exactly one stderr line, got %d:\n%s", len(lines), stderr)
	}
	if lines[0] != "runtime boom" {
		t.Errorf("stderr line = %q, want %q", lines[0], "runtime boom")
	}
}

func TestRuntimeError_UnknownCommandTextNoUsageSpam(t *testing.T) {
	stdout, stderr, code := runSilenceHelper(t, "runtime-unknown-command-text")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if strings.Contains(stderr, "Usage:") {
		t.Fatalf("runtime error was misclassified as syntax and printed usage:\n%s", stderr)
	}
	lines := nonEmptyLines(stderr)
	if len(lines) != 1 {
		t.Fatalf("expected exactly one stderr line, got %d:\n%s", len(lines), stderr)
	}
	if lines[0] != `unknown command "backend" for "refute"` {
		t.Fatalf("stderr line = %q", lines[0])
	}
}

func TestJSONCommandValidationError_NoStderrNoise(t *testing.T) {
	stdout, stderr, code := runSilenceHelper(t, "json-required-flag")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw:\n%s", err, stdout)
	}
	if got.Operation != "rename" {
		t.Fatalf("operation = %q, want rename", got.Operation)
	}
	if got.Error == nil || got.Error.Code != "invalid-request" {
		t.Fatalf("unexpected JSON error: %+v", got.Error)
	}
	if !strings.Contains(got.Error.Message, `required flag(s) "new-name" not set`) {
		t.Fatalf("JSON error missing required-flag message: %+v", got.Error)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("stderr should be empty in --json mode, got:\n%s", stderr)
	}
}

// TestRuntimeError_JSONNoStderrNoise verifies that in --json mode a runtime
// error yields valid JSON on stdout and no Error:/usage noise on stderr.
func TestRuntimeError_JSONNoStderrNoise(t *testing.T) {
	stdout, stderr, code := runSilenceHelper(t, "json")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &got); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nraw:\n%s", err, stdout)
	}
	if got.Error == nil || got.Error.Code == "" {
		t.Fatalf("missing JSON error object: %+v", got.Error)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Errorf("stderr should be empty in --json mode, got:\n%s", stderr)
	}
	if strings.Contains(stderr, "Usage:") || strings.Contains(stderr, "Error:") {
		t.Errorf("stderr contains usage/Error noise:\n%s", stderr)
	}
}

// TestSyntaxErrors_ShowUsage verifies cobra usage is still visible for
// command-line syntax errors even though runtime errors suppress usage.
func TestSyntaxErrors_ShowUsage(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		wantError string
		wantUsage string
	}{
		{
			name:      "unknown flag",
			mode:      "unknown-flag",
			wantError: "unknown flag: --bad-flag",
			wantUsage: "refute boom [flags]",
		},
		{
			name:      "unknown command",
			mode:      "unknown-command",
			wantError: `unknown command "does-not-exist" for "refute"`,
			wantUsage: "refute [command]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runSilenceHelper(t, tt.mode)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, tt.wantUsage) {
				t.Fatalf("stderr missing usage %q:\n%s", tt.wantUsage, stderr)
			}
			if !strings.Contains(stderr, tt.wantError) {
				t.Fatalf("stderr missing error %q:\n%s", tt.wantError, stderr)
			}
			if strings.Contains(stderr, "Error:") {
				t.Fatalf("stderr contains cobra Error prefix:\n%s", stderr)
			}
		})
	}
}
