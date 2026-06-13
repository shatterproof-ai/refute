package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	contentLengthHeader = "Content-Length"
	maxMessageBytes     = 32 * 1024 * 1024
)

// Transport implements the LSP base protocol framing (Content-Length headers).
type Transport struct {
	reader  *bufio.Reader
	writer  io.Writer
	writeMu sync.Mutex
}

// NewTransport creates a Transport. reader may be nil if only writing; writer
// may be nil if only reading.
func NewTransport(reader io.Reader, writer io.Writer) *Transport {
	t := &Transport{writer: writer}
	if reader != nil {
		t.reader = bufio.NewReader(reader)
	}
	return t
}

// Write sends data with a Content-Length frame. The header and body are
// composed into a single buffer and emitted with one write to the underlying
// writer, under a mutex, so concurrent callers cannot interleave frames and
// corrupt the LSP stream.
func (t *Transport) Write(data []byte) error {
	var frame bytes.Buffer
	frame.Grow(len(contentLengthHeader) + len(data) + 16)
	fmt.Fprintf(&frame, "%s: %d\r\n\r\n", contentLengthHeader, len(data))
	frame.Write(data)

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err := t.writer.Write(frame.Bytes())
	return err
}

// Read reads the next Content-Length-framed message.
func (t *Transport) Read() ([]byte, error) {
	contentLength := -1

	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		// Empty line signals end of headers.
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(parts[0], contentLengthHeader) {
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length value %q: %w", parts[1], err)
			}
			if n <= 0 {
				return nil, fmt.Errorf("invalid Content-Length value %d: must be positive", n)
			}
			if n > maxMessageBytes {
				return nil, fmt.Errorf("Content-Length %d exceeds maximum LSP message size %d", n, maxMessageBytes)
			}
			contentLength = n
		}
		// Ignore Content-Type and other headers.
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length in LSP headers")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, err
	}
	return body, nil
}
