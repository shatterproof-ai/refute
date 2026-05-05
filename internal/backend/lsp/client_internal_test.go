package lsp

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestClientRequestTimesOutWhenServerDoesNotRespond(t *testing.T) {
	client := &Client{
		transport:      NewTransport(nil, io.Discard),
		pending:        make(map[int]chan jsonrpcResponse),
		requestTimeout: 10 * time.Millisecond,
	}

	_, err := client.request("workspace/symbol", map[string]string{"query": "NoResponse"})
	if err == nil {
		t.Fatal("expected request timeout, got nil")
	}
	if !errors.Is(err, ErrRequestTimeout) {
		t.Fatalf("expected ErrRequestTimeout, got %v", err)
	}
}
