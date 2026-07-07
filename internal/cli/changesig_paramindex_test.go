package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
)

const paramIndexSource = `package main

func render(name string, verbosity int) string {
	return "hello " + name
}

func many(a, b int, c string) {}
`

func writeParamIndexFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "greeter.go")
	if err := os.WriteFile(file, []byte(paramIndexSource), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return file
}

func TestResolveGoParamIndex(t *testing.T) {
	file := writeParamIndexFile(t)
	// Columns are 1-indexed bytes. `func render(` is 12 chars, so `name` starts
	// at col 13 and `verbosity` at col 26 on line 3. In `many`, a/b/c are
	// individual parameters at indices 0/1/2.
	tests := []struct {
		name      string
		line, col int
		want      int
	}{
		{"first param by name", 3, 13, 0},
		{"second param by name", 3, 26, 1},
		{"second param by type", 3, 36, 1}, // `int` in `verbosity int`
		{"shared field first name", 7, 11, 0},
		{"shared field second name", 7, 14, 1},
		{"third param", 7, 21, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveGoParamIndex(file, tt.line, tt.col)
			if err != nil {
				t.Fatalf("resolveGoParamIndex(%d,%d): %v", tt.line, tt.col, err)
			}
			if got != tt.want {
				t.Errorf("resolveGoParamIndex(%d,%d) = %d, want %d", tt.line, tt.col, got, tt.want)
			}
		})
	}
}

func TestResolveGoParamIndex_NotFound(t *testing.T) {
	file := writeParamIndexFile(t)
	if _, err := resolveGoParamIndex(file, 4, 1); err == nil {
		t.Error("expected error resolving a position that is not on a parameter")
	}
}

// TestResolveGoParamIndex_FeedsValidRemoveEdit ties the resolver to the contract
// it exists to satisfy: the resolved index yields a ParamEdit that survives
// DecodeSignatureParams, whereas the old hardcoded FromIndex -1 does not. This is
// the check that would have caught issue #135.
func TestResolveGoParamIndex_FeedsValidRemoveEdit(t *testing.T) {
	file := writeParamIndexFile(t)
	idx, err := resolveGoParamIndex(file, 3, 26)
	if err != nil {
		t.Fatalf("resolveGoParamIndex: %v", err)
	}
	if idx < 0 {
		t.Fatalf("resolved index must be a real 0-based position, got %d", idx)
	}
	raw, err := json.Marshal(backend.SignatureParams{
		Parameters: []backend.ParamEdit{{Op: backend.ParamOpRemove, FromIndex: idx, ToIndex: -1}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := backend.DecodeSignatureParams(raw); err != nil {
		t.Fatalf("resolved remove edit must decode cleanly, got: %v", err)
	}
}
