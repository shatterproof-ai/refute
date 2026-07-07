package lsp

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend/capture"
)

// Client manages an LSP server subprocess and provides typed protocol methods.
type Client struct {
	transport      *Transport
	process        *exec.Cmd
	stderr         *capture.Stderr
	nextID         atomic.Int64
	mu             sync.Mutex
	pending        map[int]chan jsonrpcResponse
	serverCaps     serverCapabilities
	shutdownOnce   sync.Once
	done           chan struct{}
	progress       *progressTracker
	requestTimeout time.Duration
	// shutdownTimeout bounds the happy-path wait for the server to exit after
	// the exit notification before Shutdown force-kills it. Zero means
	// defaultShutdownTimeout.
	shutdownTimeout time.Duration
	// ctx is the base context propagated from the CLI. When it is cancelled
	// (e.g. on SIGINT) in-flight requests return promptly. Nil means
	// context.Background().
	ctx context.Context

	// applyEditSink, when non-nil, captures the WorkspaceEdit params of any
	// server-initiated workspace/applyEdit request instead of discarding them.
	// It is armed for the duration of an ExecuteCommand call so a command-based
	// refactoring (e.g. gopls extract_to_new_file) can be intercepted rather than
	// applied by the server. Guarded by mu.
	applyEditSink *applyEditSink
}

// applyEditSink collects the raw `edit` payloads of server-initiated
// workspace/applyEdit requests received while it is armed.
type applyEditSink struct {
	edits []json.RawMessage
}

// baseContext returns the client's base context, defaulting to
// context.Background() when none was provided (e.g. in direct test construction).
func (c *Client) baseContext() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

// serverCapabilities holds the subset of LSP server capabilities we care about.
type serverCapabilities struct {
	RenameProvider bool
}

// RenameProvider returns true if the server advertised rename support.
func (c *Client) RenameProvider() bool {
	return c.serverCaps.RenameProvider
}
