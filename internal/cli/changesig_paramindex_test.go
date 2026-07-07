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

func g[T any](a T, b int) T {
	var z T
	return z
}

type S struct{}

func (s S) m(x int, y string) string {
	return y
}

type I interface {
	doIt(p int, q string)
}
`

func writeParamIndexFile(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "greeter.go")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return file
}

func TestResolveGoParamIndex(t *testing.T) {
	file := writeParamIndexFile(t, paramIndexSource)
	// Columns are 1-indexed bytes. Each case points at a parameter identifier —
	// the only position at which gopls's removeUnusedParam is offered.
	tests := []struct {
		name      string
		line, col int
		want      int
	}{
		{"first param by name", 3, 13, 0},
		{"second param by name", 3, 26, 1},
		{"second param by type", 3, 36, 1}, // `int` in `verbosity int` (single-name field)
		{"shared field first name", 7, 11, 0},
		{"shared field second name", 7, 14, 1},
		{"third param", 7, 21, 2},
		{"generic func first param", 9, 15, 0},       // `a` in g[T any](a T, b int)
		{"generic func second param", 9, 20, 1},      // `b`; type parameter T is not counted
		{"method first param", 16, 14, 0},            // `x`; receiver (s S) is not counted
		{"method second param", 16, 21, 1},           // `y`
		{"interface method first param", 21, 7, 0},   // `p` in doIt(p int, q string)
		{"interface method second param", 21, 14, 1}, // `q`
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

// TestResolveGoParamIndex_SharedTypeRegionRejected documents that a position on
// the shared type of a multi-name field cannot be attributed to a single
// parameter. gopls's removeUnusedParam is not offered there either, so rejecting
// it here does not gate out any case the backend would have acted on.
func TestResolveGoParamIndex_SharedTypeRegionRejected(t *testing.T) {
	file := writeParamIndexFile(t, paramIndexSource)
	// Line 7 `func many(a, b int, c string)`: the shared `int` starts at col 16.
	if _, err := resolveGoParamIndex(file, 7, 16); err == nil {
		t.Error("expected error resolving the shared type region of a multi-name field")
	}
}

func TestResolveGoParamIndex_NotFound(t *testing.T) {
	file := writeParamIndexFile(t, paramIndexSource)
	if _, err := resolveGoParamIndex(file, 4, 1); err == nil {
		t.Error("expected error resolving a position that is not on a parameter")
	}
}

// TestResolveGoParamIndex_BestEffortParse documents that a syntax error in an
// unrelated function does not block resolving a parameter of a function that
// still parses. The target parameter is resolved from the recovered partial AST.
func TestResolveGoParamIndex_BestEffortParse(t *testing.T) {
	src := `package main

func good(p int, q string) string {
	return q
}

func broken( {
`
	file := writeParamIndexFile(t, src)
	got, err := resolveGoParamIndex(file, 3, 11) // `p` in good(p int, q string)
	if err != nil {
		t.Fatalf("resolveGoParamIndex on a file with an unrelated syntax error: %v", err)
	}
	if got != 0 {
		t.Errorf("resolveGoParamIndex = %d, want 0", got)
	}
}

// TestResolveGoParamIndex_FeedsValidRemoveEdit ties the resolver to the contract
// it satisfies: a resolved index yields a "remove" ParamEdit that survives
// DecodeSignatureParams, whereas the -1 "add" sentinel used for FromIndex does
// not. This is the invariant the resolver exists to uphold.
func TestResolveGoParamIndex_FeedsValidRemoveEdit(t *testing.T) {
	file := writeParamIndexFile(t, paramIndexSource)
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
