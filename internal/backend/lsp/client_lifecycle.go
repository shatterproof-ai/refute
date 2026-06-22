package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend/capture"
)

// defaultShutdownTimeout bounds how long Shutdown waits for a server that has
// acknowledged the shutdown request to actually exit before it is force-killed.
const defaultShutdownTimeout = 5 * time.Second

// StartClient launches an LSP server subprocess communicating via stdin/stdout
// and completes the initialize/initialized handshake.
//
// ctx is the base context for the client's requests; when it is cancelled
// (e.g. SIGINT from the CLI) in-flight requests return promptly. A nil ctx is
// treated as context.Background(). requestTimeout bounds each individual LSP
// request; a non-positive value falls back to defaultRequestTimeout.
func StartClient(ctx context.Context, command string, args []string, workspaceRoot string, requestTimeout time.Duration) (*Client, error) {
	if requestTimeout <= 0 {
		requestTimeout = defaultRequestTimeout
	}
	cmd := exec.Command(command, args...)
	stderr, err := capture.New("refute-lsp-stderr-*")
	if err != nil {
		return nil, fmt.Errorf("stderr temp file: %w", err)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		stderr.Cleanup()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stderr.Cleanup()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = stderr.File()

	if err := cmd.Start(); err != nil {
		stderr.Cleanup()
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	c := &Client{
		transport:       NewTransport(stdout, stdin),
		process:         cmd,
		stderr:          stderr,
		pending:         make(map[int]chan jsonrpcResponse),
		done:            make(chan struct{}),
		progress:        newProgressTracker(),
		requestTimeout:  requestTimeout,
		shutdownTimeout: defaultShutdownTimeout,
		ctx:             ctx,
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
	// Do not advertise workspace.workspaceEdit.documentChanges yet: refute
	// normalizes file edits by path for deterministic diff and JSON output,
	// including when a server sends documentChanges without that capability.
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

// Shutdown sends shutdown request then exit notification, waits for process.
func (c *Client) Shutdown() error {
	var shutdownErr error
	c.shutdownOnce.Do(func() {
		defer c.cleanupStderr()

		_, err := c.request("shutdown", nil)
		if err != nil {
			shutdownErr = c.withServerStderr(fmt.Errorf("shutdown request: %w", err))
			if cleanupErr := c.killAndWaitProcess(); cleanupErr != nil {
				shutdownErr = errors.Join(shutdownErr, cleanupErr)
			}
			return
		}

		if err := c.notify("exit", nil); err != nil {
			shutdownErr = c.withServerStderr(fmt.Errorf("exit notification: %w", err))
			if cleanupErr := c.killAndWaitProcess(); cleanupErr != nil {
				shutdownErr = errors.Join(shutdownErr, cleanupErr)
			}
			return
		}

		// Wait for the process to exit, but time-box it: a server that
		// acknowledges shutdown yet never exits must not hang the CLI. On
		// timeout, fall through to a force kill.
		if err := c.waitProcessTimeout(); err != nil {
			shutdownErr = c.withServerStderr(fmt.Errorf("wait process: %w", err))
		}
	})
	return shutdownErr
}

// waitProcessTimeout waits for the server process to exit on its own within the
// shutdown timeout. If it does not, the process is force-killed and reaped so
// the call always returns and stderr cleanup can run. A clean self-exit returns
// the process's own wait error (if any); the kill path returns nil because the
// kill is the intended, successful fallback.
func (c *Client) waitProcessTimeout() error {
	timeout := c.shutdownTimeout
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}

	waitErr := make(chan error, 1)
	go func() { waitErr <- c.process.Wait() }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-waitErr:
		return err
	case <-timer.C:
		if c.process.Process != nil {
			if err := c.process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("kill unresponsive server: %w", err)
			}
		}
		// Reap the killed process. Wait returns a "signal: killed" error, which
		// is expected here and not surfaced as a Shutdown failure.
		<-waitErr
		return nil
	}
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
