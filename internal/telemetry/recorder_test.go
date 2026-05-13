package telemetry_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/telemetry"
)

func TestRecorderWritesStartEndWithProjectAndSession(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module github.com/acme/widget\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "telemetry.jsonl")
	rec := telemetry.Start([]string{"rename", "--dry-run"}, workspace, telemetry.Options{
		TelemetryPath: path,
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ: []string{
			"CLAUDE_CODE_SESSION_ID=session-123",
			"CLAUDE_CODE_ENTRYPOINT=cli",
			"CLAUDE_API_KEY=sk-secret",
		},
		Now: fixedClock(time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)),
	})
	rec.SetOperation(telemetry.OperationContext{
		Operation:     "rename",
		Language:      "go",
		Backend:       "lsp",
		WorkspaceRoot: workspace,
		Status:        edit.StatusDryRun,
		FilesModified: 2,
	})
	rec.Finish(telemetry.FinishInfo{ExitCode: 0})

	events := readTelemetryEvents(t, path)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2: %+v", len(events), events)
	}
	if events[0]["event"] != telemetry.EventInvocationStart {
		t.Fatalf("first event = %v, want start", events[0]["event"])
	}
	if events[1]["event"] != telemetry.EventInvocationEnd {
		t.Fatalf("second event = %v, want end", events[1]["event"])
	}
	if events[0]["invocationId"] == "" || events[0]["invocationId"] != events[1]["invocationId"] {
		t.Fatalf("invocationId mismatch: %+v", events)
	}
	if events[1]["durationMs"].(float64) < 0 {
		t.Fatalf("durationMs must be non-negative: %+v", events[1])
	}
	if events[1]["exitCode"].(float64) != 0 {
		t.Fatalf("exitCode = %v, want 0", events[1]["exitCode"])
	}
	if events[1]["status"] != edit.StatusDryRun {
		t.Fatalf("status = %v, want dry-run", events[1]["status"])
	}
	agent := events[1]["agent"].(map[string]any)
	if agent["sessionId"] != "session-123" || agent["entrypoint"] != "cli" {
		t.Fatalf("agent = %+v", agent)
	}
	env := events[0]["env"].(map[string]any)
	if _, ok := env["CLAUDE_API_KEY"]; ok {
		t.Fatalf("secret leaked into env snapshot: %+v", env)
	}
	project := events[1]["project"].(map[string]any)
	if project["module"] != "github.com/acme/widget" {
		t.Fatalf("project module = %v, want github.com/acme/widget; project=%+v", project["module"], project)
	}
}

func TestRecorderCapturesCompressedSnapshots(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nfunc oldName() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := telemetry.Start([]string{"rename"}, workspace, telemetry.Options{
		TelemetryPath: filepath.Join(dir, "telemetry.jsonl"),
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ:       []string{"CLAUDE_CODE_SESSION_ID=session-123"},
		Now:           fixedClock(time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)),
	})
	rec.SetOperation(telemetry.OperationContext{Operation: "rename", WorkspaceRoot: workspace})

	we := &edit.WorkspaceEdit{FileEdits: []edit.FileEdit{{
		Path: source,
		Edits: []edit.TextEdit{{
			Range:   edit.Range{Start: edit.Position{Line: 2, Character: 5}, End: edit.Position{Line: 2, Character: 12}},
			NewText: "newName",
		}},
	}}}
	manifest, err := rec.CaptureSnapshot(we)
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}
	if manifest == nil || len(manifest.Files) != 1 {
		t.Fatalf("manifest = %+v, want one file", manifest)
	}
	entry := manifest.Files[0]
	before := readGzipFile(t, filepath.Join(filepath.Dir(manifest.Path), entry.BeforePath))
	after := readGzipFile(t, filepath.Join(filepath.Dir(manifest.Path), entry.AfterPath))
	if string(before) != "package main\n\nfunc oldName() {}\n" {
		t.Fatalf("before snapshot wrong: %q", before)
	}
	if string(after) != "package main\n\nfunc newName() {}\n" {
		t.Fatalf("after snapshot wrong: %q", after)
	}
	if !strings.HasSuffix(entry.BeforePath, ".gz") || !strings.HasSuffix(entry.AfterPath, ".gz") {
		t.Fatalf("snapshot paths should be gzip files: %+v", entry)
	}
	if entry.BeforeCompressedBytes == 0 || entry.AfterCompressedBytes == 0 {
		t.Fatalf("compressed byte counts missing: %+v", entry)
	}
}

func TestRecorderWritesTranscriptAndVerboseSummary(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}

	rec := telemetry.Start([]string{"rename"}, workspace, telemetry.Options{
		TelemetryPath: filepath.Join(dir, "telemetry.jsonl"),
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ:       []string{"CLAUDE_CODE_SESSION_ID=session-123"},
		Verbose:       true,
		Stderr:        &stderr,
		Now:           fixedClock(time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)),
	})
	rec.SetOperation(telemetry.OperationContext{
		Operation:     "rename",
		Language:      "go",
		Backend:       "lsp",
		WorkspaceRoot: workspace,
		Status:        edit.StatusApplied,
		FilesModified: 1,
	})
	rec.Finish(telemetry.FinishInfo{ExitCode: 0})

	if !strings.Contains(stderr.String(), "refute rename") || !strings.Contains(stderr.String(), "applied") {
		t.Fatalf("verbose summary missing command/status:\n%s", stderr.String())
	}
	transcript := filepath.Join(dir, "sessions", "claude", "session-123.log")
	data, err := os.ReadFile(transcript)
	if err != nil {
		t.Fatalf("read transcript: %v", err)
	}
	if !strings.Contains(string(data), "refute rename") || !strings.Contains(string(data), "files: 1") {
		t.Fatalf("transcript missing summary:\n%s", data)
	}
}

func TestRecorderTelemetryOptOut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telemetry.jsonl")
	rec := telemetry.Start([]string{"rename"}, dir, telemetry.Options{
		TelemetryPath: path,
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ:       []string{"REFUTE_TELEMETRY=0", "CLAUDE_CODE_SESSION_ID=session-123"},
	})
	rec.SetOperation(telemetry.OperationContext{Operation: "rename", WorkspaceRoot: dir})
	rec.Finish(telemetry.FinishInfo{ExitCode: 0})

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("telemetry file should not exist when opted out; stat err=%v", err)
	}
}

func TestRecorderSnapshotOptOutKeepsMetadata(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nfunc oldName() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "telemetry.jsonl")
	rec := telemetry.Start([]string{"rename"}, workspace, telemetry.Options{
		TelemetryPath: path,
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ:       []string{"REFUTE_TELEMETRY_SNAPSHOTS=0", "CLAUDE_CODE_SESSION_ID=session-123"},
		Now:           fixedClock(time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)),
	})
	rec.SetOperation(telemetry.OperationContext{Operation: "rename", WorkspaceRoot: workspace})
	we := &edit.WorkspaceEdit{FileEdits: []edit.FileEdit{{
		Path: source,
		Edits: []edit.TextEdit{{
			Range:   edit.Range{Start: edit.Position{Line: 2, Character: 5}, End: edit.Position{Line: 2, Character: 12}},
			NewText: "newName",
		}},
	}}}
	manifest, err := rec.CaptureSnapshot(we)
	if err != nil {
		t.Fatalf("CaptureSnapshot: %v", err)
	}
	if manifest != nil {
		t.Fatalf("manifest = %+v, want nil when snapshots are disabled", manifest)
	}
	rec.Finish(telemetry.FinishInfo{ExitCode: 0})

	events := readTelemetryEvents(t, path)
	if len(events) != 2 {
		t.Fatalf("events len = %d, want 2", len(events))
	}
	if _, ok := events[1]["snapshotManifest"]; ok {
		t.Fatalf("snapshotManifest should be absent when snapshots are disabled: %+v", events[1])
	}
	if _, err := os.Stat(filepath.Join(dir, "snapshots")); !os.IsNotExist(err) {
		t.Fatalf("snapshots dir should not exist when snapshot telemetry is disabled; stat err=%v", err)
	}
}

func fixedClock(t time.Time) func() time.Time {
	current := t
	return func() time.Time {
		out := current
		current = current.Add(125 * time.Millisecond)
		return out
	}
}

func readTelemetryEvents(t *testing.T, path string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid telemetry line %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

func readGzipFile(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(zr); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
