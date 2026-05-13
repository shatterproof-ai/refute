package cli

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

func TestApplyOrPreviewCapturesTelemetrySnapshot(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(workspace, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nfunc oldName() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	resetFlags := func() {
		flagDryRun = false
		flagJSON = false
		flagVerbose = false
		activeTelemetry = nil
	}
	resetFlags()
	t.Cleanup(resetFlags)
	flagDryRun = true

	rec := telemetry.Start([]string{"rename", "--dry-run"}, workspace, telemetry.Options{
		TelemetryPath: filepath.Join(dir, "telemetry.jsonl"),
		SnapshotRoot:  filepath.Join(dir, "snapshots"),
		SessionRoot:   filepath.Join(dir, "sessions"),
		Environ:       []string{"CLAUDE_CODE_SESSION_ID=session-123"},
		Now:           fixedCLITelemetryClock(time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)),
	})
	setActiveTelemetry(rec)

	we := &edit.WorkspaceEdit{FileEdits: []edit.FileEdit{{
		Path: source,
		Edits: []edit.TextEdit{{
			Range:   edit.Range{Start: edit.Position{Line: 2, Character: 5}, End: edit.Position{Line: 2, Character: 12}},
			NewText: "newName",
		}},
	}}}
	var err error
	_ = captureStdout(t, func() {
		err = applyOrPreview(we, jsonContext{
			Operation:     "rename",
			Language:      "go",
			Backend:       "lsp",
			WorkspaceRoot: workspace,
		})
	})
	if err != nil {
		t.Fatalf("applyOrPreview: %v", err)
	}
	rec.Finish(telemetry.FinishInfo{ExitCode: 0})

	events := readCLITelemetryEvents(t, filepath.Join(dir, "telemetry.jsonl"))
	end := events[len(events)-1]
	if end["status"] != edit.StatusDryRun {
		t.Fatalf("status = %v, want dry-run; end=%+v", end["status"], end)
	}
	manifestPath, ok := end["snapshotManifest"].(string)
	if !ok || manifestPath == "" {
		t.Fatalf("snapshotManifest missing from end event: %+v", end)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest telemetry.SnapshotManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(manifest.Files) != 1 {
		t.Fatalf("manifest files = %d, want 1", len(manifest.Files))
	}
	after := readCLIGzipFile(t, filepath.Join(filepath.Dir(manifest.Path), manifest.Files[0].AfterPath))
	if !strings.Contains(string(after), "newName") {
		t.Fatalf("after snapshot missing newName: %q", after)
	}
}

func fixedCLITelemetryClock(t time.Time) func() time.Time {
	current := t
	return func() time.Time {
		out := current
		current = current.Add(125 * time.Millisecond)
		return out
	}
}

func readCLITelemetryEvents(t *testing.T, path string) []map[string]any {
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

func readCLIGzipFile(t *testing.T, path string) []byte {
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
