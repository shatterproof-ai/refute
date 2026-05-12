package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// jsonrpcRequest is a JSON-RPC 2.0 request or notification.
// When ID is nil it is a notification.
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcResponse is a JSON-RPC 2.0 response or server-to-client request.
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonrpcError is a JSON-RPC 2.0 error object.
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonrpcError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// ErrContentModified is returned when the server cancels a request because its
// internal state changed (LSP error code -32801). Callers should retry.
var ErrContentModified = fmt.Errorf("content modified")
var ErrRenamePositionUnavailable = fmt.Errorf("rename target not ready")
var ErrRequestTimeout = fmt.Errorf("lsp request timed out")

const lspContentModified = -32801
const lspInvalidParams = -32602
const defaultRequestTimeout = 30 * time.Second
const maxServerStderrBytes = 64 * 1024

// isLSPError reports whether err contains a jsonrpcError with the given code.
func isLSPError(err error, code int) bool {
	var jrpcErr *jsonrpcError
	return errors.As(err, &jrpcErr) && jrpcErr.Code == code
}

func isRetryableRenameError(err error) bool {
	var jrpcErr *jsonrpcError
	if !errors.As(err, &jrpcErr) {
		return false
	}
	return jrpcErr.Code == lspInvalidParams && strings.Contains(jrpcErr.Message, "No references found at position")
}

// progressTracker tracks $/progress begin/end events so callers can wait for
// the server to finish its initial indexing pass.
//
// Servers like gopls never send $/progress for rename readiness, so waitIdle
// gives them an initialQuiesce window; if no begin arrives, it returns
// immediately. Servers like rust-analyzer emit multiple sequential begin/end
// pairs (Fetching → Building CrateGraph → Roots Scanned) with brief idle gaps
// between phases, so waitIdle uses a settleTime debounce to avoid returning
// prematurely during inter-phase gaps.
type progressTracker struct {
	mu       sync.Mutex
	active   map[string]struct{}
	idle     chan struct{} // closed when active is empty; replaced on 0→1 transition
	anyBegin chan struct{} // closed on the first begin event ever seen
}

const (
	// initialQuiesce is how long waitIdle waits for the first $/progress begin
	// before concluding the server will not send any (covers servers like gopls).
	initialQuiesce = 500 * time.Millisecond

	// settleTime is how long waitIdle waits after the active set empties before
	// declaring success. This prevents false-idle signals during the brief gaps
	// between consecutive progress phases (e.g. Fetching→CrateGraph→Roots Scanned).
	settleTime = 200 * time.Millisecond
)

func newProgressTracker() *progressTracker {
	idle := make(chan struct{})
	close(idle) // starts idle (nothing in flight)
	return &progressTracker{
		active:   make(map[string]struct{}),
		idle:     idle,
		anyBegin: make(chan struct{}),
	}
}

func (p *progressTracker) begin(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.active) == 0 {
		p.idle = make(chan struct{}) // 0→1 transition: reopen idle gate
	}
	p.active[token] = struct{}{}
	select {
	case <-p.anyBegin:
	default:
		close(p.anyBegin)
	}
}

func (p *progressTracker) end(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, token)
	if len(p.active) == 0 {
		select {
		case <-p.idle:
		default:
			close(p.idle) // all tokens done; signal idle
		}
	}
}

// waitIdle blocks until the server is idle (no active progress tokens).
//
// Phase 1: wait up to initialQuiesce for the first begin event. If none
// arrives, the server is considered immediately ready (gopls case).
//
// Phase 2: debounce loop. Wait for the active set to empty, then wait
// settleTime for any follow-on begin events. Returns only when the active set
// has been empty for a full settleTime interval.
func (p *progressTracker) waitIdle(ctx context.Context) error {
	quiesceCtx, cancel := context.WithTimeout(ctx, initialQuiesce)
	defer cancel()
	select {
	case <-p.anyBegin:
	case <-quiesceCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil // no progress events; server is ready
	}

	for {
		// Wait for the active set to empty.
		p.mu.Lock()
		isEmpty := len(p.active) == 0
		ch := p.idle
		p.mu.Unlock()

		if !isEmpty {
			select {
			case <-ch:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Active set is empty. Debounce: wait to see if a new phase starts.
		select {
		case <-time.After(settleTime):
		case <-ctx.Done():
			return ctx.Err()
		}

		p.mu.Lock()
		done := len(p.active) == 0
		p.mu.Unlock()
		if done {
			return nil
		}
		// New phase started during settle; loop.
	}
}

// Client manages an LSP server subprocess and provides typed protocol methods.
type Client struct {
	transport      *Transport
	process        *exec.Cmd
	stderrMu       sync.Mutex
	stderrFile     *os.File
	stderrPath     string
	nextID         atomic.Int64
	mu             sync.Mutex
	pending        map[int]chan jsonrpcResponse
	serverCaps     serverCapabilities
	shutdownOnce   sync.Once
	done           chan struct{}
	progress       *progressTracker
	requestTimeout time.Duration
}

// serverCapabilities holds the subset of LSP server capabilities we care about.
type serverCapabilities struct {
	RenameProvider bool
}

// StartClient launches an LSP server subprocess communicating via stdin/stdout
// and completes the initialize/initialized handshake.
func StartClient(command string, args []string, workspaceRoot string) (*Client, error) {
	cmd := exec.Command(command, args...)
	stderrFile, err := os.CreateTemp("", "refute-lsp-stderr-*")
	if err != nil {
		return nil, fmt.Errorf("stderr temp file: %w", err)
	}
	stderrPath := stderrFile.Name()
	cleanupStderr := func() {
		stderrFile.Close()
		os.Remove(stderrPath)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cleanupStderr()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cleanupStderr()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = stderrFile

	if err := cmd.Start(); err != nil {
		cleanupStderr()
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	c := &Client{
		transport:      NewTransport(stdout, stdin),
		process:        cmd,
		stderrFile:     stderrFile,
		stderrPath:     stderrPath,
		pending:        make(map[int]chan jsonrpcResponse),
		done:           make(chan struct{}),
		progress:       newProgressTracker(),
		requestTimeout: defaultRequestTimeout,
	}

	go c.readLoop()

	if err := c.initialize(workspaceRoot); err != nil {
		err = c.withServerStderr(fmt.Errorf("initialize: %w", err))
		_ = c.Shutdown()
		c.cleanupStderr()
		return nil, err
	}

	return c, nil
}

// readLoop reads messages from the server and dispatches them to pending
// request channels or handles server-initiated requests.
func (c *Client) readLoop() {
	defer close(c.done)
	for {
		raw, err := c.transport.Read()
		if err != nil {
			// Server closed or process exited — drain pending waiters.
			c.mu.Lock()
			for id, ch := range c.pending {
				ch <- jsonrpcResponse{
					Error: &jsonrpcError{Code: -32000, Message: c.errorWithServerStderr(err)},
				}
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}

		var msg jsonrpcResponse
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		// Server-initiated request (has method AND id) — send empty response.
		if msg.Method != "" && msg.ID != nil {
			resp := jsonrpcRequest{
				JSONRPC: "2.0",
				ID:      msg.ID,
			}
			data, _ := json.Marshal(resp)
			_ = c.transport.Write(data)
			continue
		}

		// Notification from server (method, no id).
		if msg.Method != "" && msg.ID == nil {
			if msg.Method == "$/progress" {
				c.handleProgress(msg.Params)
			}
			continue
		}

		// Response to a client request.
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			c.mu.Unlock()
			if ok {
				ch <- msg
			}
		}
	}
}

// handleProgress parses a $/progress notification and updates the tracker.
func (c *Client) handleProgress(params json.RawMessage) {
	var p struct {
		Token json.RawMessage `json:"token"`
		Value struct {
			Kind string `json:"kind"`
		} `json:"value"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	token, ok := progressTokenKey(p.Token)
	if !ok {
		return
	}
	switch p.Value.Kind {
	case "begin":
		c.progress.begin(token)
	case "end":
		c.progress.end(token)
	}
}

func progressTokenKey(raw json.RawMessage) (string, bool) {
	var token string
	if err := json.Unmarshal(raw, &token); err == nil {
		return token, true
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var number json.Number
	if err := decoder.Decode(&number); err == nil {
		return number.String(), true
	}

	return "", false
}

// WaitForIdle blocks until all in-flight $/progress tokens have ended or ctx
// is cancelled. Use it after Initialize to wait for server-side indexing.
func (c *Client) WaitForIdle(ctx context.Context) error {
	return c.progress.waitIdle(ctx)
}

// request sends a JSON-RPC request with an auto-incremented ID and blocks until
// the response arrives or the request timeout elapses.
func (c *Client) request(method string, params interface{}) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	idVal := id
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      &idVal,
		Method:  method,
		Params:  json.RawMessage(paramsRaw),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	ch := make(chan jsonrpcResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.transport.Write(data); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("write request: %w", err)
	}

	timeout := c.requestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var resp jsonrpcResponse
	select {
	case resp = <-ch:
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("%s request: %w after %s", method, ErrRequestTimeout, timeout)
	case <-c.done:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, c.withServerStderr(fmt.Errorf("%s request: server exited before response", method))
	}
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
}

func (c *Client) withServerStderr(err error) error {
	if err == nil {
		return nil
	}
	msg := c.serverStderr()
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w; server stderr: %s", err, msg)
}

func (c *Client) errorWithServerStderr(err error) string {
	if err == nil {
		return ""
	}
	msg := c.serverStderr()
	if msg == "" {
		return err.Error()
	}
	return fmt.Sprintf("%s; server stderr: %s", err, msg)
}

func (c *Client) serverStderr() string {
	if c == nil {
		return ""
	}
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	if c.stderrPath == "" {
		return ""
	}
	if c.stderrFile != nil {
		_ = c.stderrFile.Sync()
	}
	f, err := os.Open(c.stderrPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxServerStderrBytes+1))
	if err != nil {
		return ""
	}
	truncated := len(data) > maxServerStderrBytes
	if truncated {
		data = data[:maxServerStderrBytes]
	}
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		return ""
	}
	if truncated {
		msg += " ... [stderr truncated]"
	}
	return msg
}

func (c *Client) cleanupStderr() {
	if c == nil {
		return
	}
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	if c.stderrPath == "" {
		return
	}
	if c.stderrFile != nil {
		c.stderrFile.Close()
		c.stderrFile = nil
	}
	os.Remove(c.stderrPath)
	c.stderrPath = ""
}

// notify sends a JSON-RPC notification (no ID, no response expected).
func (c *Client) notify(method string, params interface{}) error {
	paramsRaw, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal params: %w", err)
	}

	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  json.RawMessage(paramsRaw),
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	return c.transport.Write(data)
}

// initialize performs the LSP initialize/initialized handshake.
func (c *Client) initialize(workspaceRoot string) error {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("abs workspace root: %w", err)
	}
	rootURI := fileToURI(absRoot)

	// Advertise code action resolve support so servers like gopls return
	// refactor.extract actions with resolvable Data (and compute the edit on
	// codeAction/resolve) instead of command-based actions that require
	// workspace/executeCommand + server-initiated workspace/applyEdit.
	capabilities := map[string]any{
		"textDocument": map[string]any{
			"codeAction": map[string]any{
				"dataSupport":    true,
				"resolveSupport": map[string]any{"properties": []string{"edit"}},
				"codeActionLiteralSupport": map[string]any{
					"codeActionKind": map[string]any{
						"valueSet": []string{
							"quickfix",
							"refactor",
							"refactor.extract",
							"refactor.inline",
							"refactor.rewrite",
							"source",
						},
					},
				},
			},
		},
	}
	type initParams struct {
		ProcessID    int            `json:"processId"`
		RootURI      string         `json:"rootUri"`
		Capabilities map[string]any `json:"capabilities"`
	}

	result, err := c.request("initialize", initParams{
		ProcessID:    os.Getpid(),
		RootURI:      rootURI,
		Capabilities: capabilities,
	})
	if err != nil {
		return fmt.Errorf("initialize request: %w", err)
	}

	// Parse server capabilities — specifically renameProvider.
	var initResult struct {
		Capabilities struct {
			RenameProvider interface{} `json:"renameProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	// renameProvider can be bool or RenameOptions object.
	switch v := initResult.Capabilities.RenameProvider.(type) {
	case bool:
		c.serverCaps.RenameProvider = v
	case map[string]interface{}:
		c.serverCaps.RenameProvider = true
	case nil:
		c.serverCaps.RenameProvider = false
	default:
		c.serverCaps.RenameProvider = false
	}

	// Send initialized notification to complete the handshake.
	return c.notify("initialized", struct{}{})
}

// DidOpen notifies the server a file is open (reads file content, sends textDocument/didOpen).
func (c *Client) DidOpen(filePath string, languageID string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	type textDocumentItem struct {
		URI        string `json:"uri"`
		LanguageID string `json:"languageId"`
		Version    int    `json:"version"`
		Text       string `json:"text"`
	}
	type didOpenParams struct {
		TextDocument textDocumentItem `json:"textDocument"`
	}

	return c.notify("textDocument/didOpen", didOpenParams{
		TextDocument: textDocumentItem{
			URI:        fileToURI(absPath),
			LanguageID: languageID,
			Version:    1,
			Text:       string(content),
		},
	})
}

// Rename sends textDocument/rename and returns file edits.
// line and character are 0-indexed (LSP convention).
func (c *Client) Rename(filePath string, line, character int, newName string) ([]edit.FileEdit, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	type position struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type textDocumentIdentifier struct {
		URI string `json:"uri"`
	}
	type renameParams struct {
		TextDocument textDocumentIdentifier `json:"textDocument"`
		Position     position               `json:"position"`
		NewName      string                 `json:"newName"`
	}

	result, err := c.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: fileToURI(absPath)},
		Position:     position{Line: line, Character: character},
		NewName:      newName,
	})
	if err != nil {
		if isLSPError(err, lspContentModified) {
			return nil, fmt.Errorf("rename request: %w", ErrContentModified)
		}
		if isRetryableRenameError(err) {
			return nil, fmt.Errorf("rename request: %w", ErrRenamePositionUnavailable)
		}
		return nil, fmt.Errorf("rename request: %w", err)
	}

	return parseWorkspaceEdit(result)
}

// Shutdown sends shutdown request then exit notification, waits for process.
func (c *Client) Shutdown() error {
	var shutdownErr error
	c.shutdownOnce.Do(func() {
		defer c.cleanupStderr()

		_, err := c.request("shutdown", nil)
		if err != nil {
			shutdownErr = fmt.Errorf("shutdown request: %w", err)
			if cleanupErr := c.killAndWaitProcess(); cleanupErr != nil {
				shutdownErr = errors.Join(shutdownErr, cleanupErr)
			}
			return
		}

		if err := c.notify("exit", nil); err != nil {
			shutdownErr = fmt.Errorf("exit notification: %w", err)
			if cleanupErr := c.killAndWaitProcess(); cleanupErr != nil {
				shutdownErr = errors.Join(shutdownErr, cleanupErr)
			}
			return
		}

		// Wait for readLoop to drain and process to exit.
		<-c.done
		shutdownErr = c.process.Wait()
	})
	return shutdownErr
}

func (c *Client) killAndWaitProcess() error {
	if c == nil || c.process == nil {
		return nil
	}
	if c.process.Process != nil {
		if err := c.process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill process: %w", err)
		}
	}
	if err := c.process.Wait(); err != nil && c.process.ProcessState == nil {
		return fmt.Errorf("wait process: %w", err)
	}
	return nil
}

// RenameProvider returns true if the server advertised rename support.
func (c *Client) RenameProvider() bool {
	return c.serverCaps.RenameProvider
}

// CodeAction is an LSP code action (refactoring, quick fix, etc.).
type CodeAction struct {
	Title   string           `json:"title"`
	Kind    string           `json:"kind,omitempty"`
	Edit    *json.RawMessage `json:"edit,omitempty"`
	Data    *json.RawMessage `json:"data,omitempty"`
	Command *json.RawMessage `json:"command,omitempty"`
}

// WorkspaceSymbolInfo is a single result from workspace/symbol.
type WorkspaceSymbolInfo struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	} `json:"location"`
}

// CodeActions requests code actions for a range. kinds filters by action kind
// prefix (e.g., []string{"refactor.extract"} returns only extract actions).
// All positions are 0-indexed (LSP convention).
func (c *Client) CodeActions(filePath string, startLine, startChar, endLine, endChar int, kinds []string) ([]CodeAction, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": fileToURI(absPath)},
		"range": map[string]any{
			"start": map[string]any{"line": startLine, "character": startChar},
			"end":   map[string]any{"line": endLine, "character": endChar},
		},
		"context": map[string]any{
			"diagnostics": []any{},
			"only":        kinds,
		},
	}
	result, err := c.request("textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var actions []CodeAction
	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("parsing code actions: %w", err)
	}
	return actions, nil
}

// ResolveCodeActionEdit resolves a code action to its file edits. Use when the
// action returned by CodeActions has no Edit field attached.
func (c *Client) ResolveCodeActionEdit(action CodeAction) ([]edit.FileEdit, error) {
	result, err := c.request("codeAction/resolve", action)
	if err != nil {
		return nil, err
	}
	var resolved CodeAction
	if err := json.Unmarshal(result, &resolved); err != nil {
		return nil, fmt.Errorf("parsing resolved code action: %w", err)
	}
	if resolved.Edit == nil {
		return nil, fmt.Errorf("resolved code action %q has no edit", resolved.Title)
	}
	return parseWorkspaceEdit(*resolved.Edit)
}

// WorkspaceSymbol queries the server for symbols matching query. Results are
// limited to packages the server has already loaded — callers that need broad
// coverage should prime the workspace first.
func (c *Client) WorkspaceSymbol(query string) ([]WorkspaceSymbolInfo, error) {
	result, err := c.request("workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var syms []WorkspaceSymbolInfo
	if err := json.Unmarshal(result, &syms); err != nil {
		return nil, fmt.Errorf("parsing workspace symbols: %w", err)
	}
	return syms, nil
}

// DocumentSymbol holds a hierarchical symbol entry from textDocument/documentSymbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// Range mirrors the LSP Range type (0-indexed).
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position mirrors the LSP Position type (0-indexed).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// DocumentSymbol requests hierarchical document symbols for a file. If
// rust-analyzer returns the flat SymbolInformation form instead, callers
// must fall back to the cheap branch for that file.
func (c *Client) DocumentSymbol(path string) ([]DocumentSymbol, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}
	params := map[string]any{
		"textDocument": map[string]any{"uri": fileToURI(absPath)},
	}
	raw, err := c.request("textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var result []DocumentSymbol
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing document symbols: %w", err)
	}
	return result, nil
}

// parseWorkspaceEdit converts an LSP WorkspaceEdit JSON result into []edit.FileEdit.
// It handles both the `changes` map format and `documentChanges` array format.
func parseWorkspaceEdit(raw json.RawMessage) ([]edit.FileEdit, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	type lspPosition struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspRange struct {
		Start lspPosition `json:"start"`
		End   lspPosition `json:"end"`
	}
	type lspTextEdit struct {
		Range   lspRange `json:"range"`
		NewText string   `json:"newText"`
	}
	type lspTextDocumentEdit struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Edits []lspTextEdit `json:"edits"`
	}
	type lspWorkspaceEdit struct {
		Changes         map[string][]lspTextEdit `json:"changes"`
		DocumentChanges []lspTextDocumentEdit    `json:"documentChanges"`
	}

	var we lspWorkspaceEdit
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("parse workspace edit: %w", err)
	}

	convertEdits := func(path string, lspEdits []lspTextEdit) ([]edit.TextEdit, error) {
		out := make([]edit.TextEdit, 0, len(lspEdits))
		for _, e := range lspEdits {
			startCharacter, err := utf16CharacterToByteColumnInFile(path, e.Range.Start.Line, e.Range.Start.Character)
			if err != nil {
				return nil, err
			}
			endCharacter, err := utf16CharacterToByteColumnInFile(path, e.Range.End.Line, e.Range.End.Character)
			if err != nil {
				return nil, err
			}
			out = append(out, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: startCharacter - 1},
					End:   edit.Position{Line: e.Range.End.Line, Character: endCharacter - 1},
				},
				NewText: e.NewText,
			})
		}
		return out, nil
	}

	// Prefer documentChanges when present.
	if len(we.DocumentChanges) > 0 {
		fileEdits := make([]edit.FileEdit, 0, len(we.DocumentChanges))
		for _, dc := range we.DocumentChanges {
			path := uriToFile(dc.TextDocument.URI)
			edits, err := convertEdits(path, dc.Edits)
			if err != nil {
				return nil, err
			}
			fileEdits = append(fileEdits, edit.FileEdit{
				Path:  path,
				Edits: edits,
			})
		}
		return fileEdits, nil
	}

	// Fall back to changes map.
	if len(we.Changes) > 0 {
		fileEdits := make([]edit.FileEdit, 0, len(we.Changes))
		for uri, lspEdits := range we.Changes {
			path := uriToFile(uri)
			edits, err := convertEdits(path, lspEdits)
			if err != nil {
				return nil, err
			}
			fileEdits = append(fileEdits, edit.FileEdit{
				Path:  path,
				Edits: edits,
			})
		}
		return fileEdits, nil
	}

	return nil, nil
}

// fileToURI converts an absolute file path to a file:// URI.
func fileToURI(path string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}
	return u.String()
}

// uriToFile converts a file:// URI to an absolute file path.
func uriToFile(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	path := u.Path
	// On Windows, url.Path starts with /C:/... — trim leading slash.
	if strings.HasPrefix(path, "/") && len(path) > 2 && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
