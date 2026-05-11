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
	reader := bytes.NewBuffer(lspFrame([]byte(`{"jsonrpc":"2.0","id":1,"result":null}`)))
	client, stderrPath := newShutdownTestClient(t, NewTransport(reader, io.Discard), exitedCommand(t))
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

func lspFrame(body []byte) []byte {
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}
