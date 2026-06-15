// Package capture provides a small helper for capturing a subprocess's stderr
// to a temp file so diagnostics can be folded into error messages instead of
// being discarded. It is shared by the LSP client and the OpenRewrite adapter,
// which both drive subprocesses and surface their stderr on failure.
package capture

import (
	"io"
	"os"
	"strings"
	"sync"
)

// DefaultMaxBytes is the default cap on how much captured stderr is folded into
// an error message.
const DefaultMaxBytes = 64 * 1024

// Stderr captures a subprocess's stderr to a temp file. Assign File() to the
// subprocess's Stderr, read the captured output with Read, and release the
// temp file with Cleanup. The zero value is not usable; construct one with New.
// All methods are safe for concurrent use and tolerate a nil receiver.
type Stderr struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// New creates a temp file (via os.CreateTemp with the given pattern) to capture
// a subprocess's stderr. The caller assigns File() to exec.Cmd.Stderr and calls
// Cleanup when finished.
func New(pattern string) (*Stderr, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, err
	}
	return &Stderr{file: f, path: f.Name()}, nil
}

// File returns the temp file to assign to exec.Cmd.Stderr.
func (s *Stderr) File() *os.File {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file
}

// Read returns the captured stderr, trimmed of surrounding whitespace and
// bounded to max bytes. When the captured output exceeds max bytes the output
// is truncated to max bytes and a truncation marker is appended. It returns ""
// when nothing was captured or the capture has already been cleaned up.
func (s *Stderr) Read(max int64) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return ""
	}
	if s.file != nil {
		_ = s.file.Sync()
	}
	f, err := os.Open(s.path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Read one byte past the cap so we can detect (and mark) truncation.
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return ""
	}
	truncated := int64(len(data)) > max
	if truncated {
		data = data[:max]
	}
	msg := strings.TrimSpace(string(data))
	if msg != "" && truncated {
		msg += " ... [stderr truncated]"
	}
	return msg
}

// Cleanup closes and removes the temp file. It is idempotent and safe to call
// more than once.
func (s *Stderr) Cleanup() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	if s.path != "" {
		os.Remove(s.path)
		s.path = ""
	}
}
