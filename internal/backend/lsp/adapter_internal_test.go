package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestByteColumnToUTF16Character(t *testing.T) {
	line := `const label = "é𝄞"; target := 1`
	byteColumn := strings.Index(line, "target") + 1
	got, err := byteColumnToUTF16Character(line, byteColumn)
	if err != nil {
		t.Fatalf("byteColumnToUTF16Character: %v", err)
	}
	want := 21
	if got != want {
		t.Fatalf("expected UTF-16 character %d, got %d", want, got)
	}
}

func TestReplaceWholeIdent_respectsIdentifierBoundaries(t *testing.T) {
	got := replaceWholeIdent("newFunction()\nnewFunctionCall()\n_ = newFunction", "newFunction", "sum")
	want := "sum()\nnewFunctionCall()\n_ = sum"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRunCodeActionWaitsForIdleAfterDidOpen(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.rs")
	if err := os.WriteFile(filePath, []byte("fn main() {\n    let x = 1 + 2;\n}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	actionEdit := json.RawMessage(mustMarshalJSON(t, map[string]any{
		"changes": map[string]any{
			fileToURI(filePath): []map[string]any{
				{
					"range": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 0},
					},
					"newText": "fn extracted() {}\n",
				},
			},
		},
	}))
	codeActionResult := json.RawMessage(mustMarshalJSON(t, []CodeAction{
		{
			Title: "Extract into function",
			Kind:  "refactor.extract.function",
			Edit:  &actionEdit,
		},
	}))

	writer := &codeActionIdleWriter{
		t:                t,
		didOpenSent:      make(chan struct{}),
		codeActionSent:   make(chan struct{}),
		codeActionResult: codeActionResult,
	}
	client := &Client{
		transport:      NewTransport(nil, writer),
		pending:        make(map[int]chan jsonrpcResponse),
		done:           make(chan struct{}),
		progress:       newProgressTracker(),
		requestTimeout: time.Second,
	}
	writer.client = client
	adapter := &Adapter{languageID: "rust", client: client}

	client.progress.begin("rust-analyzer/startup")
	errCh := make(chan error, 1)
	go func() {
		we, err := adapter.runCodeAction(symbolRangeFor(filePath), "", opExtractFunction)
		if err == nil && len(we.FileEdits) == 0 {
			err = fmt.Errorf("expected code-action edits")
		}
		errCh <- err
	}()

	select {
	case <-writer.didOpenSent:
	case <-time.After(time.Second):
		t.Fatal("DidOpen was not sent")
	}

	select {
	case <-writer.codeActionSent:
		t.Fatal("code action was requested before analysis became idle")
	case <-time.After(50 * time.Millisecond):
	}

	client.progress.end("rust-analyzer/startup")

	select {
	case <-writer.codeActionSent:
	case <-time.After(time.Second):
		t.Fatal("code action was not requested after analysis became idle")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runCodeAction: %v", err)
	}
}

func symbolRangeFor(filePath string) symbol.SourceRange {
	return symbol.SourceRange{
		File:      filePath,
		StartLine: 2,
		StartCol:  13,
		EndLine:   2,
		EndCol:    18,
	}
}

func mustMarshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}

type codeActionIdleWriter struct {
	t                *testing.T
	client           *Client
	didOpenSent      chan struct{}
	codeActionSent   chan struct{}
	codeActionResult json.RawMessage
	didOpenOnce      sync.Once
	codeActionOnce   sync.Once
}

func (w *codeActionIdleWriter) Write(frame []byte) (int, error) {
	body, err := NewTransport(bytes.NewReader(frame), nil).Read()
	if err != nil {
		return 0, err
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return 0, err
	}
	switch req.Method {
	case "textDocument/didOpen":
		w.didOpenOnce.Do(func() { close(w.didOpenSent) })
	case "textDocument/codeAction":
		if req.ID == nil {
			return 0, fmt.Errorf("codeAction request missing id")
		}
		w.codeActionOnce.Do(func() { close(w.codeActionSent) })
		w.client.mu.Lock()
		ch := w.client.pending[*req.ID]
		w.client.mu.Unlock()
		if ch == nil {
			return 0, fmt.Errorf("missing pending request for id %d", *req.ID)
		}
		ch <- jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  w.codeActionResult,
		}
	}
	return len(frame), nil
}
