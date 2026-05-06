package lsp

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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
