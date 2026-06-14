package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- caller / env snapshot coverage (formerly exercised via the removed
// Capture API; now tested directly against the internal helpers). ---

func TestDetectCaller_claude(t *testing.T) {
	if got := detectCaller([]string{"CLAUDE_TOOL_USE=1"}); got != "claude" {
		t.Errorf("caller: got %q, want %q", got, "claude")
	}
}

func TestDetectCaller_githubActions(t *testing.T) {
	if got := detectCaller([]string{"GITHUB_ACTIONS=true"}); got != "github-actions" {
		t.Errorf("caller: got %q, want %q", got, "github-actions")
	}
}

func TestDetectCaller_priority(t *testing.T) {
	// Claude takes priority over github-actions when both are present.
	if got := detectCaller([]string{"GITHUB_ACTIONS=true", "CLAUDE_TOOL_USE=1"}); got != "claude" {
		t.Errorf("caller: got %q, want %q", got, "claude")
	}
}

func TestDetectCaller_none(t *testing.T) {
	if got := detectCaller([]string{"HOME=/home/user", "PATH=/usr/bin"}); got != "" {
		t.Errorf("caller: got %q, want empty", got)
	}
}

func TestFilteredEnv_filtersAndRedacts(t *testing.T) {
	env := filteredEnv([]string{"CLAUDE_SESSION=abc", "HOME=/home/user", "CLAUDE_API_KEY=sk-secret"})
	if _, ok := env["HOME"]; ok {
		t.Error("HOME must not appear in env snapshot")
	}
	if _, ok := env["CLAUDE_API_KEY"]; ok {
		t.Error("secret key must not appear in env snapshot")
	}
	if v, ok := env["CLAUDE_SESSION"]; !ok || v != "abc" {
		t.Errorf("CLAUDE_SESSION missing or wrong: %v", env)
	}
}

// --- opt-out parsing ---

func TestIsDisabledValue(t *testing.T) {
	disabled := []string{"0", "false", "off", "no", "FALSE", "Off", "NO", "  no  ", "False"}
	for _, v := range disabled {
		if !isDisabledValue(v) {
			t.Errorf("isDisabledValue(%q) = false, want true", v)
		}
	}
	enabled := []string{"", "1", "true", "on", "yes", "enabled", "  "}
	for _, v := range enabled {
		if isDisabledValue(v) {
			t.Errorf("isDisabledValue(%q) = true, want false", v)
		}
	}
}

func TestIsInformationalArgs(t *testing.T) {
	info := [][]string{
		{"version"},
		{"help"},
		{"--help"},
		{"-h"},
		{"--version"},
		{"rename", "--help"},
	}
	for _, args := range info {
		if !isInformationalArgs(args) {
			t.Errorf("isInformationalArgs(%v) = false, want true", args)
		}
	}
	noninfo := [][]string{
		{"rename", "--new-name", "version"}, // "version" as a value, not subcommand
		{"rename", "--file", "help.go"},
		{"extract-function"},
		nil,
	}
	for _, args := range noninfo {
		if isInformationalArgs(args) {
			t.Errorf("isInformationalArgs(%v) = true, want false", args)
		}
	}
}

// --- snapshot retention ---

func makeSnapshotDir(t *testing.T, root, name string, size int64, mod time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if size > 0 {
		if err := os.WriteFile(filepath.Join(dir, "blob"), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(dir, mod, mod); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSweepSnapshots_capsByCount(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var dirs []string
	for i := 0; i < 5; i++ {
		// Older index = older modtime.
		dirs = append(dirs, makeSnapshotDir(t, root, "inv"+string(rune('a'+i)), 10, base.Add(time.Duration(i)*time.Hour)))
	}

	sweepSnapshots(root, 2, 0)

	// Newest two (inve, invd) survive; the older three are removed.
	for _, d := range dirs[:3] {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("expected %s removed, stat err=%v", d, err)
		}
	}
	for _, d := range dirs[3:] {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("expected %s kept, stat err=%v", d, err)
		}
	}
}

func TestSweepSnapshots_capsByBytes(t *testing.T) {
	root := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := makeSnapshotDir(t, root, "newest", 100, base.Add(3*time.Hour))
	mid := makeSnapshotDir(t, root, "mid", 100, base.Add(2*time.Hour))
	oldest := makeSnapshotDir(t, root, "oldest", 100, base.Add(1*time.Hour))

	// Budget fits two 100-byte dirs (plus their parent overhead is excluded).
	sweepSnapshots(root, 0, 250)

	if _, err := os.Stat(newest); err != nil {
		t.Errorf("newest should be kept: %v", err)
	}
	if _, err := os.Stat(mid); err != nil {
		t.Errorf("mid should be kept: %v", err)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Errorf("oldest should be removed once byte budget exceeded: stat err=%v", err)
	}
}

func TestSweepSnapshots_missingRootIsNoop(t *testing.T) {
	// Must not panic on a non-existent root.
	sweepSnapshots(filepath.Join(t.TempDir(), "does-not-exist"), 10, 10)
	sweepSnapshots("", 10, 10)
}

// --- telemetry.jsonl rotation ---

func TestRotateTelemetryLog_rotatesWhenOverCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	if err := os.WriteFile(path, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}

	rotateTelemetryLog(path, 50)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original log should be rotated away: stat err=%v", err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Errorf("rotated backup should exist: %v", err)
	}
}

func TestRotateTelemetryLog_keepsWhenUnderCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	if err := os.WriteFile(path, make([]byte, 10), 0o644); err != nil {
		t.Fatal(err)
	}

	rotateTelemetryLog(path, 50)

	if _, err := os.Stat(path); err != nil {
		t.Errorf("log under cap should be kept: %v", err)
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Errorf("no backup should be created under cap: stat err=%v", err)
	}
}
