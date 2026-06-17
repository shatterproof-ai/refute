package lsp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestResolveWithRetryReturnsFirstNonEmpty(t *testing.T) {
	hit := []symbol.Location{{Name: "FormatGreeting"}}
	calls := 0
	got, err := resolveWithRetry(func() ([]symbol.Location, error) {
		calls++
		// Empty for the first two attempts (server still indexing under
		// load), then a hit — exactly the Tier-1 flake scenario.
		if calls < 3 {
			return nil, nil
		}
		return hit, nil
	}, 5, time.Millisecond)
	if err != nil {
		t.Fatalf("resolveWithRetry: %v", err)
	}
	if len(got) != 1 || got[0].Name != "FormatGreeting" {
		t.Fatalf("expected the eventual hit, got %+v", got)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts before the hit, got %d", calls)
	}
}

func TestResolveWithRetryExhaustsToNotFound(t *testing.T) {
	calls := 0
	_, err := resolveWithRetry(func() ([]symbol.Location, error) {
		calls++
		return nil, nil
	}, 4, time.Millisecond)
	if !errors.Is(err, backend.ErrSymbolNotFound) {
		t.Fatalf("expected ErrSymbolNotFound after exhausting retries, got %v", err)
	}
	if calls != 4 {
		t.Fatalf("expected 4 attempts, got %d", calls)
	}
}

func TestResolveWithRetryPropagatesError(t *testing.T) {
	sentinel := errors.New("transport boom")
	calls := 0
	_, err := resolveWithRetry(func() ([]symbol.Location, error) {
		calls++
		return nil, sentinel
	}, 5, time.Millisecond)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the underlying error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("a hard error must not be retried; got %d attempts", calls)
	}
}

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

func TestUTF16CharacterToByteCharacter(t *testing.T) {
	line := "a𝄞b"
	tests := []struct {
		name      string
		character int
		want      int
	}{
		{name: "start", character: 0, want: 0},
		{name: "before surrogate pair", character: 1, want: 1},
		{name: "after surrogate pair", character: 3, want: 5},
		{name: "end", character: 4, want: 6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := utf16CharacterToByteCharacter(line, tc.character)
			if err != nil {
				t.Fatalf("utf16CharacterToByteCharacter: %v", err)
			}
			if got != tc.want {
				t.Fatalf("byte character = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestUTF16CharacterToByteCharacterRejectsSurrogateSplit(t *testing.T) {
	_, err := utf16CharacterToByteCharacter("a𝄞b", 2)
	if err == nil {
		t.Fatal("expected surrogate split error")
	}
	if !strings.Contains(err.Error(), "splits a UTF-16 surrogate pair") {
		t.Fatalf("error = %q, want surrogate split message", err)
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
	adapter, writer, filePath := newCodeActionIdleHarness(t)

	adapter.client.progress.begin("rust-analyzer/startup")
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

	adapter.client.progress.end("rust-analyzer/startup")

	select {
	case <-writer.codeActionSent:
	case <-time.After(time.Second):
		t.Fatal("code action was not requested after analysis became idle")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runCodeAction: %v", err)
	}
}

func TestRunCodeActionDoesNotDuplicatePrimedDidOpen(t *testing.T) {
	adapter, writer, filePath := newCodeActionIdleHarness(t)
	adapter.markOpen(filePath)

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
		t.Fatal("runCodeAction sent duplicate DidOpen for an already-open file")
	case <-writer.codeActionSent:
	case <-time.After(time.Second):
		t.Fatal("code action was not requested")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runCodeAction: %v", err)
	}
}

func newCodeActionIdleHarness(t *testing.T) (*Adapter, *codeActionIdleWriter, string) {
	t.Helper()
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
	return &Adapter{languageID: "rust", client: client}, writer, filePath
}

func TestRenameReturnsErrorWhenRetriesExhaustWithZeroEdits(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc target() {}\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	writer := &emptyRenameWriter{}
	client := &Client{
		transport:      NewTransport(nil, writer),
		pending:        make(map[int]chan jsonrpcResponse),
		done:           make(chan struct{}),
		progress:       newProgressTracker(),
		requestTimeout: time.Second,
	}
	writer.client = client
	adapter := &Adapter{
		languageID:       "go",
		client:           client,
		renameMaxRetries: 3,
		renameRetryDelay: time.Millisecond,
	}

	loc := symbol.Location{File: filePath, Line: 3, Column: 6}
	we, err := adapter.Rename(loc, "renamed")
	if err == nil {
		t.Fatalf("expected non-nil error on retry exhaustion, got edit %+v", we)
	}
	if !errors.Is(err, ErrRenameNoEdits) {
		t.Fatalf("expected ErrRenameNoEdits, got %v", err)
	}
	if writer.renameCalls != 3 {
		t.Fatalf("expected rename to be retried 3 times, got %d", writer.renameCalls)
	}
}

// emptyRenameWriter is a fake LSP server transport that acknowledges DidOpen and
// always answers textDocument/rename with an empty workspace edit, simulating a
// server stuck returning zero edits.
type emptyRenameWriter struct {
	client      *Client
	renameCalls int
}

func (w *emptyRenameWriter) Write(frame []byte) (int, error) {
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
		// notification; nothing to answer
	case "textDocument/rename":
		if req.ID == nil {
			return 0, fmt.Errorf("rename request missing id")
		}
		w.renameCalls++
		w.client.mu.Lock()
		ch := w.client.pending[*req.ID]
		w.client.mu.Unlock()
		if ch == nil {
			return 0, fmt.Errorf("missing pending request for id %d", *req.ID)
		}
		ch <- jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"changes":{}}`),
		}
	}
	return len(frame), nil
}

// TestFindSymbolRetriesThenNotFound drives FindSymbol through the adapter with a
// fake workspace/symbol server that always answers empty, exercising the
// symbolMaxRetries/symbolRetryDelay override plumbing and the adapter-level
// retry path that the free-function tests above do not reach. It pins the
// behaviour the Tier-1 flake fix depends on: an empty result is retried, not
// trusted, and exhaustion surfaces ErrSymbolNotFound.
func TestFindSymbolRetriesThenNotFound(t *testing.T) {
	writer := &emptySymbolWriter{}
	client := &Client{
		transport:      NewTransport(nil, writer),
		pending:        make(map[int]chan jsonrpcResponse),
		done:           make(chan struct{}),
		progress:       newProgressTracker(),
		requestTimeout: time.Second,
	}
	writer.client = client
	adapter := &Adapter{
		languageID:       "go",
		client:           client,
		symbolMaxRetries: 3,
		symbolRetryDelay: time.Millisecond,
	}

	_, err := adapter.FindSymbol(symbol.Query{QualifiedName: "Missing"})
	if !errors.Is(err, backend.ErrSymbolNotFound) {
		t.Fatalf("expected ErrSymbolNotFound after retries exhaust, got %v", err)
	}
	if writer.symbolCalls != 3 {
		t.Fatalf("expected workspace/symbol to be queried 3 times, got %d", writer.symbolCalls)
	}
}

// emptySymbolWriter is a fake LSP server transport that always answers
// workspace/symbol with an empty result, simulating gopls whose symbol index is
// still warming up.
type emptySymbolWriter struct {
	client      *Client
	symbolCalls int
}

func (w *emptySymbolWriter) Write(frame []byte) (int, error) {
	body, err := NewTransport(bytes.NewReader(frame), nil).Read()
	if err != nil {
		return 0, err
	}
	var req jsonrpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return 0, err
	}
	if req.Method == "workspace/symbol" {
		if req.ID == nil {
			return 0, fmt.Errorf("workspace/symbol request missing id")
		}
		w.symbolCalls++
		w.client.mu.Lock()
		ch := w.client.pending[*req.ID]
		w.client.mu.Unlock()
		if ch == nil {
			return 0, fmt.Errorf("missing pending request for id %d", *req.ID)
		}
		ch <- jsonrpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`[]`),
		}
	}
	return len(frame), nil
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
