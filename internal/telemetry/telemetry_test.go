package telemetry_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/telemetry"
)

func TestCapture_fields(t *testing.T) {
	args := []string{"rename", "--file", "foo.go", "--new-name", "Bar"}
	cwd := "/tmp/myproject"
	e := telemetry.Capture(args, cwd)
	if e.Cwd != cwd {
		t.Errorf("cwd: got %q, want %q", e.Cwd, cwd)
	}
	if len(e.Args) != len(args) || e.Args[0] != args[0] {
		t.Errorf("args: got %v, want %v", e.Args, args)
	}
	if e.Ts == "" {
		t.Error("ts must not be empty")
	}
}

func TestCapture_caller_claude(t *testing.T) {
	e := telemetry.CaptureFrom(nil, "", []string{"CLAUDE_TOOL_USE=1"})
	if e.Caller != "claude" {
		t.Errorf("caller: got %q, want %q", e.Caller, "claude")
	}
}

func TestCapture_caller_github_actions(t *testing.T) {
	e := telemetry.CaptureFrom(nil, "", []string{"GITHUB_ACTIONS=true"})
	if e.Caller != "github-actions" {
		t.Errorf("caller: got %q, want %q", e.Caller, "github-actions")
	}
}

func TestCapture_caller_priority(t *testing.T) {
	// Claude takes priority over github-actions when both are present.
	e := telemetry.CaptureFrom(nil, "", []string{"GITHUB_ACTIONS=true", "CLAUDE_TOOL_USE=1"})
	if e.Caller != "claude" {
		t.Errorf("caller: got %q, want %q", e.Caller, "claude")
	}
}

func TestCapture_no_caller(t *testing.T) {
	e := telemetry.CaptureFrom(nil, "", []string{"HOME=/home/user", "PATH=/usr/bin"})
	if e.Caller != "" {
		t.Errorf("caller: got %q, want empty", e.Caller)
	}
}

func TestCapture_env_filtered(t *testing.T) {
	env := []string{"CLAUDE_SESSION=abc", "HOME=/home/user"}
	e := telemetry.CaptureFrom(nil, "", env)
	if _, ok := e.Env["HOME"]; ok {
		t.Error("HOME must not appear in env snapshot")
	}
	if v, ok := e.Env["CLAUDE_SESSION"]; !ok || v != "abc" {
		t.Errorf("CLAUDE_SESSION missing or wrong: %v", e.Env)
	}
}

func TestCapture_no_secrets(t *testing.T) {
	e := telemetry.CaptureFrom(nil, "", []string{"CLAUDE_API_KEY=sk-secret"})
	if _, ok := e.Env["CLAUDE_API_KEY"]; ok {
		t.Error("secret key must not appear in env snapshot")
	}
}

func TestAppend_creates_file_and_dirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "telemetry.jsonl")
	e := telemetry.Entry{Ts: "2026-05-09T00:00:00Z", Args: []string{"rename"}, Cwd: "/tmp"}
	telemetry.Append(path, e)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	var got telemetry.Entry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, data)
	}
	if got.Ts != e.Ts {
		t.Errorf("ts: got %q, want %q", got.Ts, e.Ts)
	}
}

func TestAppend_appends_multiple(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	e1 := telemetry.Entry{Ts: "2026-05-09T00:00:00Z", Args: []string{"rename"}, Cwd: "/a"}
	e2 := telemetry.Entry{Ts: "2026-05-09T00:00:01Z", Args: []string{"extract-function"}, Cwd: "/b"}
	telemetry.Append(path, e1)
	telemetry.Append(path, e2)
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d:\n%s", len(lines), data)
	}
	var got2 telemetry.Entry
	if err := json.Unmarshal([]byte(lines[1]), &got2); err != nil {
		t.Fatalf("second line invalid JSON: %v", err)
	}
	if got2.Cwd != "/b" {
		t.Errorf("second entry cwd: got %q, want %q", got2.Cwd, "/b")
	}
}

func TestAppend_noop_on_empty_path(t *testing.T) {
	// Must not panic.
	telemetry.Append("", telemetry.Entry{Ts: "x"})
}

func TestDefaultPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "refute", "telemetry.jsonl")
	if got := telemetry.DefaultPath(); got != want {
		t.Errorf("DefaultPath: got %q, want %q", got, want)
	}
}
