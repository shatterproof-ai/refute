package lsp

import (
	"bytes"
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
)

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
	stderrFile, err := os.CreateTemp("", "refute-lsp-stderr-test-*")
	if err != nil {
		t.Fatalf("create stderr temp file: %v", err)
	}
	stderrPath := stderrFile.Name()
	t.Cleanup(func() {
		stderrFile.Close()
		os.Remove(stderrPath)
	})
	const stderrMessage = "panic: gopls exploded"
	if _, err := stderrFile.WriteString(stderrMessage + "\n"); err != nil {
		t.Fatalf("write stderr: %v", err)
	}

	client := &Client{
		transport:      NewTransport(nil, io.Discard),
		pending:        make(map[int]chan jsonrpcResponse),
		stderrFile:     stderrFile,
		stderrPath:     stderrPath,
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

	_, err := StartClient(server, nil, dir)
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
	reader := bytes.NewBuffer(lspFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	writer := &failAfterWrite{failAt: 3}
	client, stderrPath := newShutdownTestClient(t, NewTransport(reader, writer), longRunningCommand(t))
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
	stderrFile, err := os.CreateTemp("", "refute-lsp-shutdown-test-*")
	if err != nil {
		t.Fatalf("create stderr temp file: %v", err)
	}
	stderrPath := stderrFile.Name()
	t.Cleanup(func() {
		stderrFile.Close()
		os.Remove(stderrPath)
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	return &Client{
		transport:      transport,
		process:        cmd,
		stderrFile:     stderrFile,
		stderrPath:     stderrPath,
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
	if client.stderrFile != nil {
		t.Fatal("expected stderr file handle to be cleared")
	}
	if client.stderrPath != "" {
		t.Fatalf("expected stderr path to be cleared, got %q", client.stderrPath)
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

func lspFrame(body []byte) []byte {
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}
