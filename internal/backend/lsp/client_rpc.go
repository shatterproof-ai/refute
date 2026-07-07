package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend/capture"
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

		// Server-initiated request (has method AND id).
		if msg.Method != "" && msg.ID != nil {
			if msg.Method == "workspace/applyEdit" {
				c.handleApplyEdit(*msg.ID, msg.Params)
				continue
			}
			// Everything else: acknowledge with an empty response.
			resp := jsonrpcResponse{JSONRPC: "2.0", ID: msg.ID}
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

// handleApplyEdit responds to a server-initiated workspace/applyEdit request.
// When an applyEditSink is armed (during ExecuteCommand) it captures the edit
// payload and answers applied=true so the server believes the client applied
// it; refute then routes the captured edit through its own preview/apply
// pipeline rather than letting the server mutate the workspace. When no sink is
// armed it answers applied=false, since nothing consumed the edit.
func (c *Client) handleApplyEdit(id int, params json.RawMessage) {
	var p struct {
		Edit json.RawMessage `json:"edit"`
	}
	_ = json.Unmarshal(params, &p)

	c.mu.Lock()
	sink := c.applyEditSink
	if sink != nil && len(p.Edit) > 0 {
		sink.edits = append(sink.edits, p.Edit)
	}
	c.mu.Unlock()

	type applyResult struct {
		Applied bool `json:"applied"`
	}
	idVal := id
	resp := struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      *int        `json:"id"`
		Result  applyResult `json:"result"`
	}{JSONRPC: "2.0", ID: &idVal, Result: applyResult{Applied: sink != nil}}
	data, _ := json.Marshal(resp)
	_ = c.transport.Write(data)
}

// armApplyEditCapture installs a sink that captures workspace/applyEdit payloads
// until disarmApplyEditCapture is called. It returns the sink whose edits field
// is populated by readLoop as applyEdit requests arrive.
func (c *Client) armApplyEditCapture() *applyEditSink {
	sink := &applyEditSink{}
	c.mu.Lock()
	c.applyEditSink = sink
	c.mu.Unlock()
	return sink
}

func (c *Client) disarmApplyEditCapture() {
	c.mu.Lock()
	c.applyEditSink = nil
	c.mu.Unlock()
}

// ExecuteCommand runs workspace/executeCommand and returns any WorkspaceEdits
// the server pushed via server-initiated workspace/applyEdit while the command
// ran. gopls implements refactorings like extract_to_new_file as commands that
// deliver their edit through applyEdit rather than returning it, so callers that
// want to preview/apply the edit themselves (instead of letting the server write
// files) must go through this method. The applyEdit requests are handled by
// readLoop, which populates the armed sink before the executeCommand response
// arrives, so the captured edits are complete once request returns.
func (c *Client) ExecuteCommand(command string, arguments []json.RawMessage) ([]*edit.WorkspaceEdit, error) {
	sink := c.armApplyEditCapture()
	defer c.disarmApplyEditCapture()

	if _, err := c.request("workspace/executeCommand", map[string]any{
		"command":   command,
		"arguments": arguments,
	}); err != nil {
		return nil, err
	}

	c.mu.Lock()
	raws := append([]json.RawMessage(nil), sink.edits...)
	c.mu.Unlock()

	var edits []*edit.WorkspaceEdit
	for _, raw := range raws {
		we, err := parseWorkspaceEdit(raw)
		if err != nil {
			return nil, fmt.Errorf("parse applyEdit workspace edit: %w", err)
		}
		if we != nil {
			edits = append(edits, we)
		}
	}
	return edits, nil
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
		return nil, c.withServerStderr(fmt.Errorf("write request: %w", err))
	}

	timeout := c.requestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ctx := c.baseContext()

	var resp jsonrpcResponse
	select {
	case resp = <-ch:
	case <-timer.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, c.withServerStderr(fmt.Errorf("%s request: %w after %s", method, ErrRequestTimeout, timeout))
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, c.withServerStderr(fmt.Errorf("%s request: %w", method, ctx.Err()))
	case <-c.done:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, c.withServerStderr(fmt.Errorf("%s request: server exited before response", method))
	}
	if resp.Error != nil {
		return nil, c.withServerStderr(resp.Error)
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
	return c.stderr.Read(capture.DefaultMaxBytes)
}

func (c *Client) cleanupStderr() {
	if c == nil {
		return
	}
	c.stderr.Cleanup()
}
