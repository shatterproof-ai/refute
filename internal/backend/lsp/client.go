package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

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

// Client manages an LSP server subprocess and provides typed protocol methods.
type Client struct {
	transport    *Transport
	process      *exec.Cmd
	nextID       atomic.Int64
	mu           sync.Mutex
	pending      map[int]chan jsonrpcResponse
	serverCaps   serverCapabilities
	shutdownOnce sync.Once
	done         chan struct{}
}

// serverCapabilities holds the subset of LSP server capabilities we care about.
type serverCapabilities struct {
	RenameProvider bool
}

// StartClient launches an LSP server subprocess communicating via stdin/stdout
// and completes the initialize/initialized handshake.
func StartClient(command string, args []string, workspaceRoot string) (*Client, error) {
	cmd := exec.Command(command, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	// Discard stderr to avoid blocking.
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	c := &Client{
		transport: NewTransport(stdout, stdin),
		process:   cmd,
		pending:   make(map[int]chan jsonrpcResponse),
		done:      make(chan struct{}),
	}

	go c.readLoop()

	if err := c.initialize(workspaceRoot); err != nil {
		_ = c.Shutdown()
		return nil, fmt.Errorf("initialize: %w", err)
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
					Error: &jsonrpcError{Code: -32000, Message: err.Error()},
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

		// Notification from server (method, no id) — ignore.
		if msg.Method != "" && msg.ID == nil {
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

// request sends a JSON-RPC request with an auto-incremented ID and blocks until
// the response arrives.
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

	resp := <-ch
	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
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

	type clientCapabilities struct{}
	type initParams struct {
		ProcessID    int                `json:"processId"`
		RootURI      string             `json:"rootUri"`
		Capabilities clientCapabilities `json:"capabilities"`
	}

	result, err := c.request("initialize", initParams{
		ProcessID:    os.Getpid(),
		RootURI:      rootURI,
		Capabilities: clientCapabilities{},
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
		return nil, fmt.Errorf("rename request: %w", err)
	}

	return parseWorkspaceEdit(result)
}

// Shutdown sends shutdown request then exit notification, waits for process.
func (c *Client) Shutdown() error {
	var shutdownErr error
	c.shutdownOnce.Do(func() {
		_, err := c.request("shutdown", nil)
		if err != nil {
			shutdownErr = fmt.Errorf("shutdown request: %w", err)
			return
		}

		if err := c.notify("exit", nil); err != nil {
			shutdownErr = fmt.Errorf("exit notification: %w", err)
			return
		}

		// Wait for readLoop to drain and process to exit.
		<-c.done
		shutdownErr = c.process.Wait()
	})
	return shutdownErr
}

// RenameProvider returns true if the server advertised rename support.
func (c *Client) RenameProvider() bool {
	return c.serverCaps.RenameProvider
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
		Changes         map[string][]lspTextEdit  `json:"changes"`
		DocumentChanges []lspTextDocumentEdit     `json:"documentChanges"`
	}

	var we lspWorkspaceEdit
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("parse workspace edit: %w", err)
	}

	convertEdits := func(lspEdits []lspTextEdit) []edit.TextEdit {
		out := make([]edit.TextEdit, 0, len(lspEdits))
		for _, e := range lspEdits {
			out = append(out, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: e.Range.Start.Character},
					End:   edit.Position{Line: e.Range.End.Line, Character: e.Range.End.Character},
				},
				NewText: e.NewText,
			})
		}
		return out
	}

	// Prefer documentChanges when present.
	if len(we.DocumentChanges) > 0 {
		fileEdits := make([]edit.FileEdit, 0, len(we.DocumentChanges))
		for _, dc := range we.DocumentChanges {
			path := uriToFile(dc.TextDocument.URI)
			fileEdits = append(fileEdits, edit.FileEdit{
				Path:  path,
				Edits: convertEdits(dc.Edits),
			})
		}
		return fileEdits, nil
	}

	// Fall back to changes map.
	if len(we.Changes) > 0 {
		fileEdits := make([]edit.FileEdit, 0, len(we.Changes))
		for uri, lspEdits := range we.Changes {
			path := uriToFile(uri)
			fileEdits = append(fileEdits, edit.FileEdit{
				Path:  path,
				Edits: convertEdits(lspEdits),
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
