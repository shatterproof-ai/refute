package lsp_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func TestTransport_WriteAndRead(t *testing.T) {
	var buf bytes.Buffer

	writer := lsp.NewTransport(nil, &buf)
	payload := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
	if err := writer.Write(payload); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	reader := lsp.NewTransport(&buf, nil)
	got, err := reader.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if !bytes.Equal(got, payload) {
		t.Errorf("round-trip mismatch\ngot:  %s\nwant: %s", got, payload)
	}
}

func TestTransport_ReadMultipleMessages(t *testing.T) {
	msg1 := []byte(`{"jsonrpc":"2.0","method":"initialize","id":1}`)
	msg2 := []byte(`{"jsonrpc":"2.0","method":"shutdown","id":2}`)

	var buf bytes.Buffer
	for _, msg := range [][]byte{msg1, msg2} {
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))
		buf.WriteString(header)
		buf.Write(msg)
	}

	transport := lsp.NewTransport(&buf, nil)

	got1, err := transport.Read()
	if err != nil {
		t.Fatalf("Read message 1 failed: %v", err)
	}
	if !bytes.Equal(got1, msg1) {
		t.Errorf("message 1 mismatch\ngot:  %s\nwant: %s", got1, msg1)
	}

	got2, err := transport.Read()
	if err != nil {
		t.Fatalf("Read message 2 failed: %v", err)
	}
	if !bytes.Equal(got2, msg2) {
		t.Errorf("message 2 mismatch\ngot:  %s\nwant: %s", got2, msg2)
	}
}

func TestTransport_ConcurrentWrites(t *testing.T) {
	// Many goroutines share one Transport and write distinct payloads
	// concurrently. The underlying buffer is NOT internally synchronized, so
	// the Transport itself must serialize writes and emit each frame
	// atomically. On the pre-fix code (two unlocked writes per message) this
	// races the buffer (caught by -race) and interleaves header/body framing,
	// corrupting the stream so the payloads cannot be read back intact.
	var buf bytes.Buffer
	writer := lsp.NewTransport(nil, &buf)

	const n = 64
	payloads := make([][]byte, n)
	for i := range payloads {
		payloads[i] = fmt.Appendf(nil,
			`{"jsonrpc":"2.0","id":%d,"pad":%q}`, i, strings.Repeat("x", i*53))
	}

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range payloads {
		go func(p []byte) {
			defer wg.Done()
			if err := writer.Write(p); err != nil {
				t.Errorf("Write failed: %v", err)
			}
		}(payloads[i])
	}
	wg.Wait()

	// Read every frame back and confirm the multiset matches what was sent.
	// Interleaved framing would surface as Read errors or mismatched payloads.
	reader := lsp.NewTransport(&buf, nil)
	got := make(map[string]int, n)
	for i := range n {
		msg, err := reader.Read()
		if err != nil {
			t.Fatalf("Read frame %d failed (stream corrupted): %v", i, err)
		}
		got[string(msg)]++
	}
	for _, p := range payloads {
		if got[string(p)] != 1 {
			t.Errorf("payload missing or corrupted (seen %d times): %s", got[string(p)], p)
		}
	}
}

func TestTransport_ReadRejectsOversizedContentLength(t *testing.T) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Content-Length: %d\r\n\r\n", 33*1024*1024)

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded for oversized Content-Length")
	}
	if !strings.Contains(err.Error(), "exceeds maximum LSP message size") {
		t.Fatalf("Read error = %q, want maximum-size error", err)
	}
}

func TestTransport_ReadRejectsNegativeContentLength(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Length: -1\r\n\r\n")

	transport := lsp.NewTransport(&buf, nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Read panicked for negative Content-Length: %v", recovered)
		}
	}()

	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded for negative Content-Length")
	}
	if !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Fatalf("Read error = %q, want invalid Content-Length error", err)
	}
}

func TestTransport_ReadRejectsMissingContentLength(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}")

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded without Content-Length")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Fatalf("Read error = %q, want missing Content-Length error", err)
	}
}

func TestTransport_ReadRejectsMalformedContentLength(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Length: not-a-number\r\n\r\n")

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded with malformed Content-Length")
	}
	if !strings.Contains(err.Error(), "invalid Content-Length") {
		t.Fatalf("Read error = %q, want invalid Content-Length error", err)
	}
}

func TestTransport_ReadMalformedHeaderFramingDoesNotSetContentLength(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Length:2\r\n\r\n{}")

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded with malformed Content-Length header framing")
	}
	if !strings.Contains(err.Error(), "missing Content-Length") {
		t.Fatalf("Read error = %q, want missing Content-Length error", err)
	}
}

func TestTransport_ReadRejectsTruncatedBody(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Length: 5\r\n\r\nabc")

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded with truncated body")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Read error = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

func TestTransport_ReadRejectsEOFWhileReadingBody(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString("Content-Length: 5\r\n\r\n")

	transport := lsp.NewTransport(&buf, nil)
	_, err := transport.Read()
	if err == nil {
		t.Fatal("Read succeeded after EOF before response body")
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("Read error = %v, want %v", err, io.EOF)
	}
}

func FuzzRead(f *testing.F) {
	f.Add([]byte("Content-Length: 2\r\n\r\n{}"))
	f.Add([]byte("Content-Length: 13\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\nhello, world!"))
	f.Add([]byte("\r\n\r\nContent-Length: 2\r\n\r\n{}"))
	f.Add([]byte("Content-Length: 0\r\n\r\n"))
	f.Add([]byte(""))
	f.Add([]byte("Content-Length: -1\r\n\r\n"))
	f.Add([]byte("Content-Length: not-a-number\r\n\r\n"))
	f.Add([]byte("Content-Length: 99999999999999999999\r\n\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		transport := lsp.NewTransport(bytes.NewReader(data), nil)
		body, err := transport.Read()
		if err == nil {
			// On success: bounded to the documented 32 MiB cap.
			const cap = 32 * 1024 * 1024
			if len(body) > cap {
				t.Fatalf("Read returned %d bytes, exceeds cap %d", len(body), cap)
			}
		}
	})
}
