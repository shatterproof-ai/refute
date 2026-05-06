package lsp_test

import (
	"bytes"
	"fmt"
	"strings"
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
