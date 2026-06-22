package lsp

import (
	"context"
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
