package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"time"
)

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
