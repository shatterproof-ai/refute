package lsp_test

import (
	"bytes"
	"fmt"
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
