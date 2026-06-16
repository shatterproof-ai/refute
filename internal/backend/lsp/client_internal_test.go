package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend/capture"
	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestParseWorkspaceEditSortsChangesMapByFilePath(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.go")
	bPath := filepath.Join(dir, "b.go")
	if err := os.WriteFile(aPath, []byte("package main\n\nvar A = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte("package main\n\nvar B = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := fmt.Sprintf(`{
		"changes": {
			%q: [{"range":{"start":{"line":2,"character":4},"end":{"line":2,"character":5}},"newText":"RenamedB"}],
			%q: [{"range":{"start":{"line":2,"character":4},"end":{"line":2,"character":5}},"newText":"RenamedA"}]
		}
	}`, fileToURI(bPath), fileToURI(aPath))

	we, err := parseWorkspaceEdit(json.RawMessage(input))
	if err != nil {
		t.Fatalf("parseWorkspaceEdit: %v", err)
	}
	fileEdits := we.FileEdits

	if len(fileEdits) != 2 {
		t.Fatalf("got %d file edits, want 2: %+v", len(fileEdits), fileEdits)
	}
	if fileEdits[0].Path != aPath || fileEdits[1].Path != bPath {
		t.Fatalf("file edits not sorted by path: got [%q, %q], want [%q, %q]", fileEdits[0].Path, fileEdits[1].Path, aPath, bPath)
	}
}

func TestParseWorkspaceEditRejectsEmptyURIInDocumentChanges(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name:  "missing textDocument field",
			input: `{"documentChanges":[{}]}`,
		},
		{
			name:  "explicit empty uri",
			input: `{"documentChanges":[{"textDocument":{"uri":""}}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseWorkspaceEdit(json.RawMessage(tc.input))
			if err == nil {
				t.Fatal("expected error for documentChanges entry with empty or missing textDocument.uri, got nil")
			}
		})
	}
}

func TestParseWorkspaceEditExtractToNewFileRoundTrips(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, "orig.go")
	newFile := filepath.Join(dir, "extracted.go")
	if err := os.WriteFile(orig, []byte("package p\n\nfunc Keep() {}\nfunc Move() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Synthetic gopls "extract to new file": create the new file, insert the
	// extracted content into it, and delete the moved function from the
	// original.
	raw := fmt.Sprintf(`{"documentChanges":[
		{"kind":"create","uri":%q},
		{"textDocument":{"uri":%q},"edits":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"package p\n\nfunc Move() {}\n"}]},
		{"textDocument":{"uri":%q},"edits":[{"range":{"start":{"line":3,"character":0},"end":{"line":4,"character":0}},"newText":""}]}
	]}`, fileToURI(newFile), fileToURI(newFile), fileToURI(orig))

	we, err := parseWorkspaceEdit(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseWorkspaceEdit: %v", err)
	}
	if len(we.FileOps) != 1 || we.FileOps[0].Kind != edit.FileOpCreate || we.FileOps[0].Path != newFile {
		t.Fatalf("expected one create op for %s, got %+v", newFile, we.FileOps)
	}
	if len(we.FileEdits) != 2 {
		t.Fatalf("expected 2 file edits, got %d: %+v", len(we.FileEdits), we.FileEdits)
	}

	if _, err := edit.ApplyWithin(we, dir); err != nil {
		t.Fatalf("ApplyWithin: %v", err)
	}

	gotNew, err := os.ReadFile(newFile)
	if err != nil {
		t.Fatalf("read new file: %v", err)
	}
	if string(gotNew) != "package p\n\nfunc Move() {}\n" {
		t.Fatalf("new file content = %q", gotNew)
	}
	gotOrig, err := os.ReadFile(orig)
	if err != nil {
		t.Fatalf("read orig: %v", err)
	}
	if string(gotOrig) != "package p\n\nfunc Keep() {}\n" {
		t.Fatalf("orig content = %q", gotOrig)
	}
}

func TestParseWorkspaceEditParsesRenameAndDeleteOps(t *testing.T) {
	oldURI := fileToURI("/ws/old.go")
	newURI := fileToURI("/ws/new.go")
	goneURI := fileToURI("/ws/gone.go")
	raw := fmt.Sprintf(`{"documentChanges":[
		{"kind":"rename","oldUri":%q,"newUri":%q,"options":{"overwrite":true}},
		{"kind":"delete","uri":%q,"options":{"ignoreIfNotExists":true}}
	]}`, oldURI, newURI, goneURI)

	we, err := parseWorkspaceEdit(json.RawMessage(raw))
	if err != nil {
		t.Fatalf("parseWorkspaceEdit: %v", err)
	}
	if len(we.FileOps) != 2 {
		t.Fatalf("expected 2 file ops, got %+v", we.FileOps)
	}
	ren := we.FileOps[0]
	if ren.Kind != edit.FileOpRename || ren.Path != "/ws/old.go" || ren.NewPath != "/ws/new.go" || !ren.Overwrite {
		t.Fatalf("rename op = %+v", ren)
	}
	del := we.FileOps[1]
	if del.Kind != edit.FileOpDelete || del.Path != "/ws/gone.go" || !del.IgnoreIfNotExists {
		t.Fatalf("delete op = %+v", del)
	}
}

func FuzzParseWorkspaceEdit(f *testing.F) {
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"changes":{"file:///tmp/x.go":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"x"}]}}`))
	f.Add([]byte(`{"documentChanges":[{"textDocument":{"uri":"file:///tmp/x.go"},"edits":[{"range":{"start":{"line":0,"character":0},"end":{"line":0,"character":0}},"newText":"y"}]}]}`))
	f.Add([]byte(`{"changes":{},"documentChanges":[]}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Invariant: never panic. The fuzzer's default panic-recovery treats
		// a panic as a failure, so the bare call is the test.
		_, _ = parseWorkspaceEdit(json.RawMessage(data))
	})
}

func TestClientRequestTimesOutWhenServerDoesNotRespond(t *testing.T) {
	client := &Client{
		transport:      NewTransport(nil, io.Discard),
		pending:        make(map[int]chan jsonrpcResponse),
		requestTimeout: 10 * time.Millisecond,
	}

	_, err := client.request("workspace/symbol", map[string]string{"query": "NoResponse"})
	if err == nil {
		t.Fatal("expected request timeout, got nil")
	}
	if !errors.Is(err, ErrRequestTimeout) {
		t.Fatalf("expected ErrRequestTimeout, got %v", err)
	}
}

func TestClientRequestTimeoutIncludesServerStderr(t *testing.T) {
	stderr, err := capture.New("refute-lsp-stderr-test-*")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	t.Cleanup(stderr.Cleanup)
	const stderrMessage = "panic: gopls exploded"
	if _, err := stderr.File().WriteString(stderrMessage + "\n"); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	client := &Client{
		transport:      NewTransport(nil, io.Discard),
		pending:        make(map[int]chan jsonrpcResponse),
		stderr:         stderr,
		requestTimeout: 10 * time.Millisecond,
	}

	_, err = client.request("workspace/symbol", map[string]string{"query": "x"})
	if err == nil {
		t.Fatal("expected request timeout, got nil")
	}
	if !errors.Is(err, ErrRequestTimeout) {
		t.Fatalf("expected ErrRequestTimeout in chain, got %v", err)
	}
	if !strings.Contains(err.Error(), stderrMessage) {
		t.Fatalf("expected error to include server stderr %q, got %v", stderrMessage, err)
	}
}

func TestClientHandleProgressNormalizesMixedTokenTypes(t *testing.T) {
	tests := []struct {
		name  string
		begin string
		end   string
	}{
		{
			name:  "numeric begin string end",
			begin: `{"token":5,"value":{"kind":"begin"}}`,
			end:   `{"token":"5","value":{"kind":"end"}}`,
		},
		{
			name:  "string begin numeric end",
			begin: `{"token":"5","value":{"kind":"begin"}}`,
			end:   `{"token":5,"value":{"kind":"end"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{progress: newProgressTracker()}

			client.handleProgress([]byte(tt.begin))
			client.handleProgress([]byte(tt.end))

			client.progress.mu.Lock()
			defer client.progress.mu.Unlock()
			if len(client.progress.active) != 0 {
				t.Fatalf("expected mixed numeric/string progress token to end, active tokens: %v", client.progress.active)
			}
		})
	}
}

func TestStartClientIncludesServerStderrOnInitializeFailure(t *testing.T) {
	dir := t.TempDir()
	server := filepath.Join(dir, "stderr-lsp")
	const stderrMessage = "lsp failed while loading workspace"
	if err := os.WriteFile(server, []byte("#!/bin/sh\necho '"+stderrMessage+"' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write fake server: %v", err)
	}

	_, err := StartClient(context.Background(), server, nil, dir, 0)
	if err == nil {
		t.Fatal("expected StartClient to fail")
	}
	if !strings.Contains(err.Error(), stderrMessage) {
		t.Fatalf("expected error to include server stderr %q, got %v", stderrMessage, err)
	}
}

func TestClientShutdownCleansUpAfterShutdownRequestFailure(t *testing.T) {
	client, stderrPath := newShutdownTestClient(t, NewTransport(nil, errWriter{}), longRunningCommand(t))

	err := client.Shutdown()
	if err == nil {
		t.Fatal("expected Shutdown to fail")
	}
	if !strings.Contains(err.Error(), "shutdown request") {
		t.Fatalf("expected shutdown request error, got %v", err)
	}

	requireShutdownCleanup(t, client, stderrPath)
}

func TestClientShutdownCleansUpAfterExitNotificationFailure(t *testing.T) {
	// Transport.Write emits one write per message: the shutdown request is
	// write 1 (succeeds), the exit notification is write 2 (fails here).
	responder := newResponseAfterWrite(t, lspFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	writer := &failAfterResponseWrite{responseAfterWrite: responder, failAt: 2}
	client, stderrPath := newShutdownTestClient(t, NewTransport(responder.reader, writer), longRunningCommand(t))
	go client.readLoop()

	err := client.Shutdown()
	if err == nil {
		t.Fatal("expected Shutdown to fail")
	}
	if !strings.Contains(err.Error(), "exit notification") {
		t.Fatalf("expected exit notification error, got %v", err)
	}

	requireShutdownCleanup(t, client, stderrPath)
}

func TestClientShutdownTimesOutAndKillsUnresponsiveServer(t *testing.T) {
	// The server acknowledges the shutdown request and accepts the exit
	// notification, but never exits and keeps its connection open. Without a
	// time-box, Shutdown blocks forever on <-c.done / process.Wait().
	responder := newRespondKeepOpen(t, lspFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	client, stderrPath := newShutdownTestClient(t, NewTransport(responder.reader, responder), longRunningCommand(t))
	client.shutdownTimeout = 50 * time.Millisecond
	go client.readLoop()

	start := time.Now()
	if err := client.Shutdown(); err != nil {
		t.Fatalf("Shutdown should succeed via kill fallback: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Shutdown did not time-box; it blocked for %s", elapsed)
	}

	requireShutdownCleanup(t, client, stderrPath)
}

func TestClientRequestCancelsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &Client{
		transport:      NewTransport(nil, io.Discard),
		pending:        make(map[int]chan jsonrpcResponse),
		requestTimeout: time.Minute, // long, so cancellation ends the call, not the timeout
		ctx:            ctx,
	}

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := client.request("workspace/symbol", map[string]string{"query": "x"})
	if err == nil {
		t.Fatal("expected request to fail on context cancellation, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled in chain, got %v", err)
	}
}

func TestClientShutdownCleansUpAfterNormalShutdown(t *testing.T) {
	responder := newResponseAfterWrite(t, lspFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	client, stderrPath := newShutdownTestClient(t, NewTransport(responder.reader, responder), exitedCommand(t))
	go client.readLoop()

	if err := client.Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	requireShutdownCleanup(t, client, stderrPath)
}

func newShutdownTestClient(t *testing.T, transport *Transport, cmd *exec.Cmd) (*Client, string) {
	t.Helper()
	stderr, err := capture.New("refute-lsp-shutdown-test-*")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	stderrPath := stderr.File().Name()
	t.Cleanup(func() {
		stderr.Cleanup()
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	return &Client{
		transport:      transport,
		process:        cmd,
		stderr:         stderr,
		pending:        make(map[int]chan jsonrpcResponse),
		done:           make(chan struct{}),
		requestTimeout: time.Second,
	}, stderrPath
}

func longRunningCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start long-running command: %v", err)
	}
	return cmd
}

func exitedCommand(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start exited command: %v", err)
	}
	return cmd
}

func requireShutdownCleanup(t *testing.T, client *Client, stderrPath string) {
	t.Helper()
	if client.process.ProcessState == nil {
		t.Fatal("expected Shutdown to reap process")
	}
	if _, err := os.Stat(stderrPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stderr temp file to be removed, stat error: %v", err)
	}
	if client.stderr.File() != nil {
		t.Fatal("expected stderr file handle to be cleared")
	}
	if got := client.stderr.Read(capture.DefaultMaxBytes); got != "" {
		t.Fatalf("expected cleaned stderr capture to read empty, got %q", got)
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

type failAfterWrite struct {
	writes atomic.Int64
	failAt int64
}

func (w *failAfterWrite) Write(p []byte) (int, error) {
	if w.writes.Add(1) >= w.failAt {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

type failAfterResponseWrite struct {
	*responseAfterWrite
	failAt int64
}

func (w *failAfterResponseWrite) Write(p []byte) (int, error) {
	if w.writes.Add(1) >= w.failAt {
		return 0, errors.New("write failed")
	}
	if w.writes.Load() == 1 {
		go func() {
			_, _ = w.writer.Write(w.body)
			_ = w.writer.Close()
		}()
	}
	return len(p), nil
}

type responseAfterWrite struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	body   []byte
	writes atomic.Int64
}

func newResponseAfterWrite(t *testing.T, body []byte) *responseAfterWrite {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	return &responseAfterWrite{
		reader: reader,
		writer: writer,
		body:   body,
	}
}

func (w *responseAfterWrite) Write(p []byte) (int, error) {
	if w.writes.Add(1) == 1 {
		go func() {
			_, _ = w.writer.Write(w.body)
			_ = w.writer.Close()
		}()
	}
	return len(p), nil
}

// respondKeepOpen answers the first write with body but, unlike
// responseAfterWrite, never closes the pipe — simulating a server that
// acknowledges shutdown yet keeps its connection open and never exits.
type respondKeepOpen struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	body   []byte
	writes atomic.Int64
}

func newRespondKeepOpen(t *testing.T, body []byte) *respondKeepOpen {
	t.Helper()
	reader, writer := io.Pipe()
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
	})
	return &respondKeepOpen{reader: reader, writer: writer, body: body}
}

func (w *respondKeepOpen) Write(p []byte) (int, error) {
	if w.writes.Add(1) == 1 {
		go func() { _, _ = w.writer.Write(w.body) }()
	}
	return len(p), nil
}

func lspFrame(body []byte) []byte {
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}
