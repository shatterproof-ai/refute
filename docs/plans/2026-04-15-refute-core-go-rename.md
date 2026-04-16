# refute: Core Infrastructure + Go Rename — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Build the core `refute` CLI that can rename Go symbols via gopls, proving the entire architecture end-to-end.

**Architecture:** Go CLI sends rename commands to a generic LSP backend adapter, which communicates with gopls over JSON-RPC/stdio. The adapter returns a WorkspaceEdit, which the edit applier writes atomically to disk (or renders as a diff in dry-run mode). Symbol resolution converts human-friendly identifiers (file+line+name or file+line+col) into the LSP positions the server needs.

**Tech Stack:** Go 1.22+, cobra (CLI), go-difflib (diffs), fatih/color (terminal output). No LSP library dependencies — the JSON-RPC transport and LSP protocol subset are implemented from scratch (~200 lines) to keep the dependency tree minimal.

**Spec:** `docs/specs/2026-04-15-refute-design.md`

**Prerequisite:** `gopls` installed and on `$PATH` (`go install golang.org/x/tools/gopls@latest`).

**This plan delivers:** `refute rename-function --file main.go --line 10 --name "oldFunc" --new-name "newFunc"` working against real Go code.

**Deferred to later plans:** additional refactorings (extract, inline, move, pattern), TypeScript/Python backends, daemon mode, MCP server, Open VSX registry, qualified name resolution (tier 1), e2e test corpus.

---

## File Structure

```
cmd/refute/main.go                    — CLI entrypoint, wires cobra root command
internal/
├── edit/
│   ├── types.go                      — WorkspaceEdit, TextEdit, Position, Range
│   ├── applier.go                    — Apply WorkspaceEdit to filesystem atomically
│   ├── applier_test.go
│   ├── diff.go                       — Render WorkspaceEdit as colored unified diff
│   └── diff_test.go
├── symbol/
│   ├── types.go                      — SymbolLocation, SourceRange, SymbolQuery, SymbolKind
│   ├── resolver.go                   — Tier 2 & 3 resolution (line+name, line+col)
│   └── resolver_test.go
├── backend/
│   ├── backend.go                    — RefactoringBackend interface, ErrUnsupported, capability types
│   └── lsp/
│       ├── adapter.go                — LSP backend adapter (implements RefactoringBackend)
│       ├── adapter_test.go           — Integration test with gopls
│       ├── client.go                 — LSP client (initialize, rename, shutdown)
│       ├── client_test.go            — Integration test for LSP handshake
│       ├── transport.go              — JSON-RPC over stdio with Content-Length framing
│       └── transport_test.go         — Unit test with in-memory readers/writers
├── config/
│   ├── config.go                     — Config struct, loader, resolution order, defaults
│   └── config_test.go
└── cli/
    ├── root.go                       — Root command, global flags, version
    └── rename.go                     — Rename subcommands (rename, rename-function, etc.)
testdata/
└── fixtures/
    └── go/
        └── rename/                   — Multi-file Go project for rename integration tests
            ├── go.mod
            ├── main.go
            └── util/
                └── helper.go
go.mod
go.sum
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `cmd/refute/main.go`
- Create: all directories listed in file structure

- [x] **Step 1: Initialize Go module**

```bash
cd /home/ketan/project/refute
go mod init github.com/shatterproof-ai/refute
```

- [x] **Step 2: Create directory structure**

```bash
mkdir -p cmd/refute
mkdir -p internal/edit
mkdir -p internal/symbol
mkdir -p internal/backend/lsp
mkdir -p internal/config
mkdir -p internal/cli
mkdir -p testdata/fixtures/go/rename/util
```

- [x] **Step 3: Write minimal main.go**

Create `cmd/refute/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("refute %s\n", version)
		return
	}
	fmt.Fprintln(os.Stderr, "refute: automated source code refactoring")
	fmt.Fprintln(os.Stderr, "run 'refute version' to see version")
}
```

- [x] **Step 4: Verify it builds and runs**

```bash
go build -o refute ./cmd/refute && ./refute version
```

Expected: `refute 0.1.0-dev`

- [x] **Step 5: Commit**

```bash
git add go.mod cmd/ internal/ testdata/
git commit -m "feat: scaffold project structure and minimal main"
```

---

### Task 2: Core Types

**Files:**
- Create: `internal/edit/types.go`
- Create: `internal/symbol/types.go`
- Create: `internal/backend/backend.go`

- [x] **Step 1: Write edit types**

Create `internal/edit/types.go`:

```go
package edit

// Position in a text document. 0-indexed line and character, matching LSP convention.
// The CLI converts 1-indexed user input to 0-indexed before constructing these.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit is a single edit to a text document: replace the Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// FileEdit holds all edits for a single file.
type FileEdit struct {
	Path  string     // Absolute file path
	Edits []TextEdit
}

// WorkspaceEdit describes changes across multiple files.
// This is the universal return type from all refactoring backends.
type WorkspaceEdit struct {
	FileEdits []FileEdit
}
```

- [x] **Step 2: Write symbol types**

Create `internal/symbol/types.go`:

```go
package symbol

// SymbolKind classifies symbols for kind-specific rename commands.
type SymbolKind int

const (
	KindUnknown   SymbolKind = iota
	KindFunction
	KindClass
	KindField
	KindVariable
	KindParameter
	KindType
	KindMethod
)

func (k SymbolKind) String() string {
	names := [...]string{
		"unknown", "function", "class", "field",
		"variable", "parameter", "type", "method",
	}
	if int(k) < len(names) {
		return names[k]
	}
	return "unknown"
}

// Location identifies a symbol in a source file.
// Line and Column are 1-indexed (human convention). The backend adapter
// converts to 0-indexed LSP positions internally.
type Location struct {
	File   string
	Line   int // 1-indexed
	Column int // 1-indexed
	Name   string
	Kind   SymbolKind
}

// SourceRange identifies a range of source code. 1-indexed.
type SourceRange struct {
	File     string
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// Query represents a request to find a symbol.
type Query struct {
	QualifiedName string     // Tier 1: e.g., "package.Type.Method"
	File          string     // Tier 2 & 3
	Line          int        // Tier 2 & 3 (1-indexed)
	Column        int        // Tier 3 only (1-indexed, 0 means unset)
	Name          string     // Tier 2: symbol name to find on the line
	Kind          SymbolKind // Optional filter
}

// Tier returns which resolution tier this query uses.
func (q Query) Tier() int {
	if q.QualifiedName != "" {
		return 1
	}
	if q.Column > 0 {
		return 3
	}
	if q.Name != "" {
		return 2
	}
	return 0
}
```

- [x] **Step 3: Write backend interface**

Create `internal/backend/backend.go`:

```go
package backend

import (
	"errors"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// ErrUnsupported indicates the backend does not support this refactoring.
var ErrUnsupported = errors.New("refactoring not supported by this backend")

// ErrSymbolNotFound indicates the symbol could not be located.
var ErrSymbolNotFound = errors.New("symbol not found")

// ErrAmbiguous indicates multiple symbols matched the query.
type ErrAmbiguous struct {
	Candidates []symbol.Location
}

func (e *ErrAmbiguous) Error() string {
	return "ambiguous symbol: multiple candidates found"
}

// Capability describes a refactoring operation a backend supports.
type Capability struct {
	Operation string // e.g., "rename", "extract-function"
}

// RefactoringBackend is the uniform interface all backends implement.
type RefactoringBackend interface {
	// Initialize prepares the backend for the given workspace.
	Initialize(workspaceRoot string) error

	// Shutdown releases all resources.
	Shutdown() error

	// FindSymbol resolves a symbol query to concrete locations.
	FindSymbol(query symbol.Query) ([]symbol.Location, error)

	// Rename renames the symbol at the given location.
	Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error)

	// ExtractFunction extracts the given range into a new function.
	ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error)

	// ExtractVariable extracts the given range into a new variable.
	ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error)

	// InlineSymbol inlines the symbol at the given location.
	InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error)

	// MoveToFile moves the symbol to a different file.
	MoveToFile(loc symbol.Location, destination string) (*edit.WorkspaceEdit, error)

	// Capabilities returns the list of supported operations.
	Capabilities() []Capability
}
```

- [x] **Step 4: Verify compilation**

```bash
go build ./...
```

Expected: clean build, no errors.

- [x] **Step 5: Commit**

```bash
git add internal/edit/types.go internal/symbol/types.go internal/backend/backend.go
git commit -m "feat: define core types — WorkspaceEdit, Symbol, RefactoringBackend"
```

---

### Task 3: Edit Applier

**Files:**
- Create: `internal/edit/applier.go`
- Create: `internal/edit/applier_test.go`

- [x] **Step 1: Write the failing test — single file edit**

Create `internal/edit/applier_test.go`:

```go
package edit_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestApply_SingleFileRename(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc oldName() {}\n"), 0644)

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: filePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 5},
							End:   edit.Position{Line: 2, Character: 12},
						},
						NewText: "newName",
					},
				},
			},
		},
	}

	result, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if result.FilesModified != 1 {
		t.Errorf("expected 1 file modified, got %d", result.FilesModified)
	}

	content, _ := os.ReadFile(filePath)
	expected := "package main\n\nfunc newName() {}\n"
	if string(content) != expected {
		t.Errorf("unexpected content:\ngot:  %q\nwant: %q", string(content), expected)
	}
}

func TestApply_MultipleEditsReverseOrder(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("aaa bbb ccc\n"), 0644)

	// Replace "bbb" with "xxx" and "ccc" with "yyy" — edits must be applied
	// in reverse order so positions stay valid.
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: filePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 4},
							End:   edit.Position{Line: 0, Character: 7},
						},
						NewText: "xxx",
					},
					{
						Range: edit.Range{
							Start: edit.Position{Line: 0, Character: 8},
							End:   edit.Position{Line: 0, Character: 11},
						},
						NewText: "yyy",
					},
				},
			},
		},
	}

	_, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "aaa xxx yyy\n"
	if string(content) != expected {
		t.Errorf("unexpected content:\ngot:  %q\nwant: %q", string(content), expected)
	}
}

func TestApply_MultiFileEdit(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.go")
	os.WriteFile(file1, []byte("package a\n\nfunc Foo() {}\n"), 0644)
	os.WriteFile(file2, []byte("package b\n\nimport \"a\"\n\nfunc bar() { a.Foo() }\n"), 0644)

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: file1,
				Edits: []edit.TextEdit{
					{
						Range:   edit.Range{Start: edit.Position{Line: 2, Character: 5}, End: edit.Position{Line: 2, Character: 8}},
						NewText: "Bar",
					},
				},
			},
			{
				Path: file2,
				Edits: []edit.TextEdit{
					{
						Range:   edit.Range{Start: edit.Position{Line: 4, Character: 15}, End: edit.Position{Line: 4, Character: 18}},
						NewText: "Bar",
					},
				},
			},
		},
	}

	result, err := edit.Apply(we)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if result.FilesModified != 2 {
		t.Errorf("expected 2 files modified, got %d", result.FilesModified)
	}

	content1, _ := os.ReadFile(file1)
	if got := string(content1); got != "package a\n\nfunc Bar() {}\n" {
		t.Errorf("file1 unexpected:\n%s", got)
	}
	content2, _ := os.ReadFile(file2)
	if got := string(content2); got != "package b\n\nimport \"a\"\n\nfunc bar() { a.Bar() }\n" {
		t.Errorf("file2 unexpected:\n%s", got)
	}
}

func TestApply_RollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	os.WriteFile(file1, []byte("original\n"), 0644)

	// Second file doesn't exist — should cause failure and rollback.
	nonexistent := filepath.Join(dir, "no", "such", "file.go")

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path:  file1,
				Edits: []edit.TextEdit{{Range: edit.Range{Start: edit.Position{}, End: edit.Position{Character: 8}}, NewText: "modified"}},
			},
			{
				Path:  nonexistent,
				Edits: []edit.TextEdit{{Range: edit.Range{}, NewText: "x"}},
			},
		},
	}

	_, err := edit.Apply(we)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	// First file should be unchanged (rollback).
	content, _ := os.ReadFile(file1)
	if string(content) != "original\n" {
		t.Errorf("file1 should be unchanged after rollback, got: %q", string(content))
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

```bash
cd /home/ketan/project/refute && go test ./internal/edit/...
```

Expected: compilation error — `edit.Apply` not defined.

- [x] **Step 3: Implement the edit applier**

Create `internal/edit/applier.go`:

```go
package edit

import (
	"fmt"
	"os"
	"sort"
)

// ApplyResult describes the outcome of applying a WorkspaceEdit.
type ApplyResult struct {
	FilesModified int
}

// Apply applies a WorkspaceEdit to the filesystem atomically.
// If any file fails, all changes are rolled back.
func Apply(we *WorkspaceEdit) (*ApplyResult, error) {
	if len(we.FileEdits) == 0 {
		return &ApplyResult{}, nil
	}

	// Phase 1: compute all new file contents in memory.
	type pendingWrite struct {
		path    string
		content []byte
		mode    os.FileMode
	}
	var writes []pendingWrite

	for _, fe := range we.FileEdits {
		original, err := os.ReadFile(fe.Path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", fe.Path, err)
		}
		info, err := os.Stat(fe.Path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", fe.Path, err)
		}

		modified, err := applyEdits(original, fe.Edits)
		if err != nil {
			return nil, fmt.Errorf("applying edits to %s: %w", fe.Path, err)
		}

		writes = append(writes, pendingWrite{
			path:    fe.Path,
			content: modified,
			mode:    info.Mode(),
		})
	}

	// Phase 2: write to temp files.
	tmpPaths := make([]string, len(writes))
	for i, w := range writes {
		tmp := w.path + ".refute.tmp"
		if err := os.WriteFile(tmp, w.content, w.mode); err != nil {
			// Clean up any temp files already written.
			for j := 0; j < i; j++ {
				os.Remove(tmpPaths[j])
			}
			return nil, fmt.Errorf("writing temp file for %s: %w", w.path, err)
		}
		tmpPaths[i] = tmp
	}

	// Phase 3: rename temp files into place (atomic per-file).
	for i, w := range writes {
		if err := os.Rename(tmpPaths[i], w.path); err != nil {
			// Partial rename — best effort: remaining temps are cleaned up.
			for j := i; j < len(tmpPaths); j++ {
				os.Remove(tmpPaths[j])
			}
			return nil, fmt.Errorf("renaming temp file for %s: %w", w.path, err)
		}
	}

	return &ApplyResult{FilesModified: len(writes)}, nil
}

// applyEdits applies text edits to file content. Edits are sorted in reverse
// document order before application so earlier edits don't shift later positions.
func applyEdits(content []byte, edits []TextEdit) ([]byte, error) {
	sorted := make([]TextEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		si, sj := sorted[i].Range.Start, sorted[j].Range.Start
		if si.Line != sj.Line {
			return si.Line > sj.Line
		}
		return si.Character > sj.Character
	})

	result := make([]byte, len(content))
	copy(result, content)

	for _, te := range sorted {
		startOff := positionToOffset(result, te.Range.Start)
		endOff := positionToOffset(result, te.Range.End)
		if startOff < 0 || endOff < 0 || startOff > len(result) || endOff > len(result) || startOff > endOff {
			return nil, fmt.Errorf("invalid edit range: start=%d end=%d len=%d", startOff, endOff, len(result))
		}

		newContent := make([]byte, 0, startOff+len(te.NewText)+len(result)-endOff)
		newContent = append(newContent, result[:startOff]...)
		newContent = append(newContent, []byte(te.NewText)...)
		newContent = append(newContent, result[endOff:]...)
		result = newContent
	}

	return result, nil
}

// positionToOffset converts a 0-indexed Position to a byte offset in content.
func positionToOffset(content []byte, pos Position) int {
	line := 0
	offset := 0
	for offset < len(content) && line < pos.Line {
		if content[offset] == '\n' {
			line++
		}
		offset++
	}
	return offset + pos.Character
}
```

- [x] **Step 4: Run tests to verify they pass**

```bash
cd /home/ketan/project/refute && go test ./internal/edit/... -v
```

Expected: all 4 tests pass.

- [x] **Step 5: Commit**

```bash
git add internal/edit/applier.go internal/edit/applier_test.go
git commit -m "feat: implement atomic edit applier with rollback"
```

---

### Task 4: Diff Renderer

**Files:**
- Create: `internal/edit/diff.go`
- Create: `internal/edit/diff_test.go`

- [x] **Step 1: Add go-difflib dependency**

```bash
cd /home/ketan/project/refute && go get github.com/pmezard/go-difflib/difflib
```

- [x] **Step 2: Write the failing test**

Create `internal/edit/diff_test.go`:

```go
package edit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestRenderDiff_SingleFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc oldName() {}\n"), 0644)

	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: filePath,
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 5},
							End:   edit.Position{Line: 2, Character: 12},
						},
						NewText: "newName",
					},
				},
			},
		},
	}

	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}

	if !strings.Contains(diff, "-func oldName()") {
		t.Errorf("diff should contain removed line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+func newName()") {
		t.Errorf("diff should contain added line, got:\n%s", diff)
	}
}

func TestRenderDiff_NoEdits(t *testing.T) {
	we := &edit.WorkspaceEdit{}
	diff, err := edit.RenderDiff(we)
	if err != nil {
		t.Fatalf("RenderDiff failed: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff, got:\n%s", diff)
	}
}
```

- [x] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/edit/... -run TestRenderDiff
```

Expected: compilation error — `edit.RenderDiff` not defined.

- [x] **Step 4: Implement the diff renderer**

Create `internal/edit/diff.go`:

```go
package edit

import (
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// RenderDiff computes the workspace edit and returns a unified diff string
// showing what would change, without modifying any files.
func RenderDiff(we *WorkspaceEdit) (string, error) {
	if len(we.FileEdits) == 0 {
		return "", nil
	}

	var parts []string

	for _, fe := range we.FileEdits {
		original, err := os.ReadFile(fe.Path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", fe.Path, err)
		}

		modified, err := applyEdits(original, fe.Edits)
		if err != nil {
			return "", fmt.Errorf("computing edits for %s: %w", fe.Path, err)
		}

		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(original)),
			B:        difflib.SplitLines(string(modified)),
			FromFile: fe.Path,
			ToFile:   fe.Path,
			Context:  3,
		}

		text, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			return "", fmt.Errorf("generating diff for %s: %w", fe.Path, err)
		}
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, "\n"), nil
}
```

- [x] **Step 5: Run tests to verify they pass**

```bash
go test ./internal/edit/... -v
```

Expected: all tests pass (including applier tests from Task 3).

- [x] **Step 6: Commit**

```bash
git add internal/edit/diff.go internal/edit/diff_test.go go.mod go.sum
git commit -m "feat: implement diff renderer for dry-run preview"
```

---

### Task 5: Config System

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Timeout != 30000 {
		t.Errorf("expected default timeout 30000, got %d", cfg.Timeout)
	}
	gopls := cfg.Server("go")
	if gopls.Command != "gopls" {
		t.Errorf("expected default gopls command, got %q", gopls.Command)
	}
}

func TestLoad_ProjectConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "refute.config.json")
	os.WriteFile(cfgPath, []byte(`{
		"servers": {
			"go": {
				"command": "/custom/gopls",
				"args": ["serve", "--debug"]
			}
		},
		"timeout": 60000
	}`), 0644)

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Timeout != 60000 {
		t.Errorf("expected timeout 60000, got %d", cfg.Timeout)
	}
	gopls := cfg.Server("go")
	if gopls.Command != "/custom/gopls" {
		t.Errorf("expected custom command, got %q", gopls.Command)
	}
	if len(gopls.Args) != 2 || gopls.Args[0] != "serve" {
		t.Errorf("expected custom args, got %v", gopls.Args)
	}
}

func TestLoad_ExplicitPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "my-config.json")
	os.WriteFile(cfgPath, []byte(`{"timeout": 5000}`), 0644)

	cfg, err := config.Load(cfgPath, "/nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Timeout != 5000 {
		t.Errorf("expected timeout 5000, got %d", cfg.Timeout)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: compilation error — `config` package not defined.

- [x] **Step 3: Implement the config system**

Create `internal/config/config.go`:

```go
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ServerConfig describes how to launch a language server.
type ServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// DaemonConfig controls daemon behavior.
type DaemonConfig struct {
	AutoStart   bool `json:"autoStart"`
	IdleTimeout int  `json:"idleTimeout"` // milliseconds
}

// Config is the top-level configuration.
type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
	Timeout int                     `json:"timeout"` // milliseconds
	Daemon  DaemonConfig            `json:"daemon"`
}

// Server returns the server config for a language, falling back to defaults.
func (c *Config) Server(language string) ServerConfig {
	if sc, ok := c.Servers[language]; ok {
		return sc
	}
	if sc, ok := defaultServers[language]; ok {
		return sc
	}
	return ServerConfig{}
}

var defaultServers = map[string]ServerConfig{
	"go": {
		Command: "gopls",
		Args:    []string{"serve"},
	},
	"typescript": {
		Command: "typescript-language-server",
		Args:    []string{"--stdio"},
	},
	"python": {
		Command: "pyright-langserver",
		Args:    []string{"--stdio"},
	},
}

const defaultTimeout = 30000 // 30 seconds

// Load reads config using this resolution order:
//  1. explicitPath (--config flag) if non-empty
//  2. refute.config.json in workspaceRoot
//  3. ~/.config/refute/config.json
//  4. Built-in defaults
//
// Later sources fill in fields not set by earlier sources.
func Load(explicitPath string, workspaceRoot string) (*Config, error) {
	cfg := &Config{
		Servers: make(map[string]ServerConfig),
		Timeout: defaultTimeout,
		Daemon: DaemonConfig{
			IdleTimeout: 600000, // 10 minutes
		},
	}

	var paths []string

	// User-level config.
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "refute", "config.json"))
	}

	// Project-level config.
	if workspaceRoot != "" {
		paths = append(paths, filepath.Join(workspaceRoot, "refute.config.json"))
	}

	// Explicit config (highest priority, applied last).
	if explicitPath != "" {
		paths = append(paths, explicitPath)
	}

	for _, p := range paths {
		if err := mergeFromFile(cfg, p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
	}

	return cfg, nil
}

func mergeFromFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var overlay Config
	if err := json.Unmarshal(data, &overlay); err != nil {
		return err
	}

	if overlay.Timeout > 0 {
		cfg.Timeout = overlay.Timeout
	}
	if overlay.Daemon.IdleTimeout > 0 {
		cfg.Daemon.IdleTimeout = overlay.Daemon.IdleTimeout
	}
	if overlay.Daemon.AutoStart {
		cfg.Daemon.AutoStart = true
	}
	for lang, sc := range overlay.Servers {
		cfg.Servers[lang] = sc
	}

	return nil
}
```

- [x] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all 3 tests pass.

- [x] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: implement config loading with defaults and merge"
```

---

### Task 6: LSP Transport Layer

**Files:**
- Create: `internal/backend/lsp/transport.go`
- Create: `internal/backend/lsp/transport_test.go`

- [x] **Step 1: Write the failing test — message framing**

Create `internal/backend/lsp/transport_test.go`:

```go
package lsp_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func TestTransport_WriteAndRead(t *testing.T) {
	var buf bytes.Buffer
	transport := lsp.NewTransport(io.NopCloser(&buf), &buf)

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"test"}`)
	if err := transport.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Reset reader to read what was written.
	reader := bytes.NewReader(buf.Bytes())
	transport2 := lsp.NewTransport(io.NopCloser(reader), nil)

	got, err := transport2.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(got) != string(msg) {
		t.Errorf("round-trip mismatch:\ngot:  %s\nwant: %s", got, msg)
	}
}

func TestTransport_ReadMultipleMessages(t *testing.T) {
	var buf bytes.Buffer

	msg1 := []byte(`{"id":1}`)
	msg2 := []byte(`{"id":2}`)

	// Manually write two framed messages.
	writeFramed(&buf, msg1)
	writeFramed(&buf, msg2)

	transport := lsp.NewTransport(io.NopCloser(&buf), nil)

	got1, err := transport.Read()
	if err != nil {
		t.Fatalf("Read 1 failed: %v", err)
	}
	got2, err := transport.Read()
	if err != nil {
		t.Fatalf("Read 2 failed: %v", err)
	}

	if string(got1) != string(msg1) {
		t.Errorf("msg1 mismatch: got %s", got1)
	}
	if string(got2) != string(msg2) {
		t.Errorf("msg2 mismatch: got %s", got2)
	}
}

func writeFramed(buf *bytes.Buffer, data []byte) {
	header := "Content-Length: " + itoa(len(data)) + "\r\n\r\n"
	buf.WriteString(header)
	buf.Write(data)
}

func itoa(n int) string {
	return bytes.NewBufferString("").String() // placeholder
}

func init() {
	// Fix the itoa helper — using fmt to avoid import cycle.
	_ = writeFramed
}
```

Actually, this test helper has an issue. Let me fix it in the plan.

Replace the test file with a corrected version:

Create `internal/backend/lsp/transport_test.go`:

```go
package lsp_test

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func writeFramed(buf *bytes.Buffer, data []byte) {
	fmt.Fprintf(buf, "Content-Length: %d\r\n\r\n", len(data))
	buf.Write(data)
}

func TestTransport_WriteAndRead(t *testing.T) {
	var buf bytes.Buffer
	writeTransport := lsp.NewTransport(nil, &buf)

	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"test"}`)
	if err := writeTransport.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	readTransport := lsp.NewTransport(io.NopCloser(bytes.NewReader(buf.Bytes())), nil)
	got, err := readTransport.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(got) != string(msg) {
		t.Errorf("round-trip mismatch:\ngot:  %s\nwant: %s", got, msg)
	}
}

func TestTransport_ReadMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	msg1 := []byte(`{"id":1}`)
	msg2 := []byte(`{"id":2}`)
	writeFramed(&buf, msg1)
	writeFramed(&buf, msg2)

	transport := lsp.NewTransport(io.NopCloser(&buf), nil)

	got1, err := transport.Read()
	if err != nil {
		t.Fatalf("Read 1 failed: %v", err)
	}
	got2, err := transport.Read()
	if err != nil {
		t.Fatalf("Read 2 failed: %v", err)
	}

	if string(got1) != string(msg1) {
		t.Errorf("msg1 mismatch: got %s", got1)
	}
	if string(got2) != string(msg2) {
		t.Errorf("msg2 mismatch: got %s", got2)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/backend/lsp/... -run TestTransport
```

Expected: compilation error — `lsp.NewTransport` not defined.

- [x] **Step 3: Implement the transport**

Create `internal/backend/lsp/transport.go`:

```go
package lsp

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Transport handles JSON-RPC message framing over LSP's base protocol
// (Content-Length headers over a byte stream).
type Transport struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewTransport creates a transport. Either reader or writer may be nil
// if only one direction is needed.
func NewTransport(reader io.Reader, writer io.Writer) *Transport {
	var br *bufio.Reader
	if reader != nil {
		br = bufio.NewReader(reader)
	}
	return &Transport{reader: br, writer: writer}
}

// Write sends a message with Content-Length framing.
func (t *Transport) Write(data []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(t.writer, header); err != nil {
		return fmt.Errorf("writing header: %w", err)
	}
	if _, err := t.writer.Write(data); err != nil {
		return fmt.Errorf("writing body: %w", err)
	}
	return nil
}

// Read reads the next framed message.
func (t *Transport) Read() ([]byte, error) {
	contentLength := -1

	// Read headers until empty line (\r\n\r\n).
	for {
		line, err := t.reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			// End of headers.
			break
		}

		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
			contentLength = n
		}
		// Ignore other headers (e.g., Content-Type).
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.reader, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	return body, nil
}
```

- [x] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/backend/lsp/... -run TestTransport -v
```

Expected: both tests pass.

- [x] **Step 5: Commit**

```bash
git add internal/backend/lsp/transport.go internal/backend/lsp/transport_test.go
git commit -m "feat: implement LSP JSON-RPC transport with Content-Length framing"
```

---

### Task 7: LSP Client Protocol

**Files:**
- Create: `internal/backend/lsp/client.go`
- Create: `internal/backend/lsp/client_test.go`

- [x] **Step 1: Write the failing integration test**

This test requires `gopls` installed. It verifies the full LSP lifecycle: initialize, didOpen, rename, shutdown.

Create `internal/backend/lsp/client_test.go`:

```go
package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func requireGopls(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH, skipping integration test")
	}
}

func setupGoProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func oldName() {}

func main() {
	oldName()
}
`), 0644)

	return dir
}

func TestClient_Initialize(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient failed: %v", err)
	}
	defer client.Shutdown()

	caps := client.ServerCapabilities()
	if !caps.RenameProvider {
		t.Error("expected server to support rename")
	}
}

func TestClient_Rename(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient failed: %v", err)
	}
	defer client.Shutdown()

	mainFile := filepath.Join(dir, "main.go")
	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen failed: %v", err)
	}

	// Rename "oldName" at line 2, character 5 (0-indexed) to "newName".
	edits, err := client.Rename(mainFile, 2, 5, "newName")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if len(edits) == 0 {
		t.Fatal("expected at least one file edit")
	}

	// The rename should produce edits for main.go touching both
	// the declaration and the call site.
	totalEdits := 0
	for _, fe := range edits {
		totalEdits += len(fe.Edits)
	}
	if totalEdits < 2 {
		t.Errorf("expected at least 2 text edits (decl + call), got %d", totalEdits)
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/backend/lsp/... -run TestClient -v
```

Expected: compilation error — `lsp.StartClient` not defined.

- [x] **Step 3: Implement the LSP client**

Create `internal/backend/lsp/client.go`:

```go
package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// Client manages an LSP server subprocess and provides typed protocol methods.
type Client struct {
	cmd       *exec.Cmd
	transport *Transport
	nextID    atomic.Int64
	pending   map[int64]chan json.RawMessage
	mu        sync.Mutex
	serverCap serverCapabilities
}

type serverCapabilities struct {
	RenameProvider bool
}

// ServerCapabilities returns the server's declared capabilities.
func (c *Client) ServerCapabilities() serverCapabilities {
	return c.serverCap
}

// StartClient launches an LSP server and completes the initialize handshake.
func StartClient(command string, args []string, workspaceRoot string) (*Client, error) {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", command, err)
	}

	c := &Client{
		cmd:       cmd,
		transport: NewTransport(stdout, stdin),
		pending:   make(map[int64]chan json.RawMessage),
	}

	// Start the reader goroutine.
	go c.readLoop()

	// Initialize handshake.
	if err := c.initialize(workspaceRoot); err != nil {
		c.kill()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	return c, nil
}

func (c *Client) initialize(workspaceRoot string) error {
	rootURI := fileToURI(workspaceRoot)

	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"rename": map[string]any{
					"prepareSupport": true,
				},
			},
		},
	}

	result, err := c.request("initialize", params)
	if err != nil {
		return err
	}

	// Parse capabilities.
	var initResult struct {
		Capabilities struct {
			RenameProvider any `json:"renameProvider"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("parsing initialize result: %w", err)
	}
	// renameProvider can be bool or object; either truthy value means supported.
	c.serverCap.RenameProvider = initResult.Capabilities.RenameProvider != nil &&
		initResult.Capabilities.RenameProvider != false

	// Send initialized notification.
	return c.notify("initialized", map[string]any{})
}

// DidOpen notifies the server that a file is open.
func (c *Client) DidOpen(filePath string, languageID string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	return c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        fileToURI(filePath),
			"languageId": languageID,
			"version":    1,
			"text":       string(content),
		},
	})
}

// Rename sends a textDocument/rename request and returns the edits.
func (c *Client) Rename(filePath string, line, character int, newName string) ([]edit.FileEdit, error) {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri": fileToURI(filePath),
		},
		"position": map[string]any{
			"line":      line,
			"character": character,
		},
		"newName": newName,
	}

	result, err := c.request("textDocument/rename", params)
	if err != nil {
		return nil, err
	}

	return parseWorkspaceEdit(result)
}

// Shutdown sends shutdown and exit, then waits for the process.
func (c *Client) Shutdown() error {
	_, _ = c.request("shutdown", nil)
	_ = c.notify("exit", nil)
	return c.cmd.Wait()
}

func (c *Client) kill() {
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
		c.cmd.Wait()
	}
}

// request sends a JSON-RPC request and waits for the response.
func (c *Client) request(method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	ch := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if err := c.transport.Write(data); err != nil {
		return nil, err
	}

	result := <-ch
	return result, nil
}

// notify sends a JSON-RPC notification (no id, no response expected).
func (c *Client) notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return c.transport.Write(data)
}

// readLoop reads messages from the server and dispatches responses.
func (c *Client) readLoop() {
	for {
		data, err := c.transport.Read()
		if err != nil {
			return // Server closed or error — stop reading.
		}

		var msg struct {
			ID     *int64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
			Method string `json:"method"` // For server-initiated requests/notifications.
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// If this is a server-initiated request (has method + id), send empty response.
		if msg.Method != "" && msg.ID != nil {
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      *msg.ID,
				"result":  nil,
			}
			respData, _ := json.Marshal(resp)
			c.transport.Write(respData)
			continue
		}

		// If this is a response to one of our requests.
		if msg.ID != nil {
			c.mu.Lock()
			ch, ok := c.pending[*msg.ID]
			if ok {
				delete(c.pending, *msg.ID)
			}
			c.mu.Unlock()

			if ok {
				if msg.Error != nil {
					ch <- nil // Signal error via nil result.
				} else {
					ch <- msg.Result
				}
			}
		}
		// Server notifications (no id) — ignore for now.
	}
}

// parseWorkspaceEdit converts an LSP WorkspaceEdit JSON into our edit types.
func parseWorkspaceEdit(data json.RawMessage) ([]edit.FileEdit, error) {
	if data == nil {
		return nil, fmt.Errorf("server returned null result")
	}

	var lspEdit struct {
		Changes         map[string][]lspTextEdit `json:"changes"`
		DocumentChanges []json.RawMessage        `json:"documentChanges"`
	}
	if err := json.Unmarshal(data, &lspEdit); err != nil {
		return nil, fmt.Errorf("parsing workspace edit: %w", err)
	}

	var fileEdits []edit.FileEdit

	// Handle "changes" field (simple map of URI → edits).
	for uri, edits := range lspEdit.Changes {
		path := uriToFile(uri)
		fe := edit.FileEdit{Path: path}
		for _, e := range edits {
			fe.Edits = append(fe.Edits, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: e.Range.Start.Character},
					End:   edit.Position{Line: e.Range.End.Line, Character: e.Range.End.Character},
				},
				NewText: e.NewText,
			})
		}
		fileEdits = append(fileEdits, fe)
	}

	// Handle "documentChanges" field (array of TextDocumentEdits).
	for _, raw := range lspEdit.DocumentChanges {
		var docEdit struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			Edits []lspTextEdit `json:"edits"`
		}
		if err := json.Unmarshal(raw, &docEdit); err != nil {
			continue // Skip entries we don't understand (CreateFile, etc.).
		}
		if docEdit.TextDocument.URI == "" {
			continue
		}

		path := uriToFile(docEdit.TextDocument.URI)
		fe := edit.FileEdit{Path: path}
		for _, e := range docEdit.Edits {
			fe.Edits = append(fe.Edits, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: e.Range.Start.Character},
					End:   edit.Position{Line: e.Range.End.Line, Character: e.Range.End.Character},
				},
				NewText: e.NewText,
			})
		}
		if len(fe.Edits) > 0 {
			fileEdits = append(fileEdits, fe)
		}
	}

	return fileEdits, nil
}

type lspTextEdit struct {
	Range struct {
		Start struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"start"`
		End struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"end"`
	} `json:"range"`
	NewText string `json:"newText"`
}

func fileToURI(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	if runtime.GOOS == "windows" {
		absPath = "/" + filepath.ToSlash(absPath)
	}
	return "file://" + url.PathEscape(absPath)
}

func uriToFile(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	path := parsed.Path
	if runtime.GOOS == "windows" && len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	unescaped, err := url.PathUnescape(path)
	if err != nil {
		return path
	}
	return unescaped
}
```

- [x] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/backend/lsp/... -run TestClient -v -timeout 30s
```

Expected: both `TestClient_Initialize` and `TestClient_Rename` pass (or skip if gopls not installed).

- [x] **Step 5: Commit**

```bash
git add internal/backend/lsp/client.go internal/backend/lsp/client_test.go
git commit -m "feat: implement LSP client with initialize, rename, shutdown"
```

---

### Task 8: LSP Backend Adapter

**Files:**
- Create: `internal/backend/lsp/adapter.go`
- Create: `internal/backend/lsp/adapter_test.go`

- [x] **Step 1: Write the failing integration test**

Create `internal/backend/lsp/adapter_test.go`:

```go
package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestAdapter_Rename(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func oldFunc() string {
	return "hello"
}

func main() {
	oldFunc()
}
`), 0644)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})

	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer adapter.Shutdown()

	loc := symbol.Location{
		File:   filepath.Join(dir, "main.go"),
		Line:   3, // 1-indexed: "func oldFunc()"
		Column: 6, // 1-indexed: start of "oldFunc"
		Name:   "oldFunc",
		Kind:   symbol.KindFunction,
	}

	we, err := adapter.Rename(loc, "newFunc")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits")
	}

	totalEdits := 0
	for _, fe := range we.FileEdits {
		totalEdits += len(fe.Edits)
	}
	if totalEdits < 2 {
		t.Errorf("expected at least 2 edits (decl + call), got %d", totalEdits)
	}
}

func TestAdapter_Capabilities(t *testing.T) {
	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})

	caps := adapter.Capabilities()
	found := false
	for _, c := range caps {
		if c.Operation == "rename" {
			found = true
		}
	}
	if !found {
		t.Error("expected rename in capabilities")
	}
}
```

- [x] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/backend/lsp/... -run TestAdapter -v
```

Expected: compilation error — `lsp.NewAdapter` not defined.

- [x] **Step 3: Implement the adapter**

Create `internal/backend/lsp/adapter.go`:

```go
package lsp

import (
	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// Adapter implements backend.RefactoringBackend using an LSP server.
type Adapter struct {
	serverCfg    config.ServerConfig
	languageID   string
	filePatterns []string
	client       *Client
	workspaceRoot string
}

// NewAdapter creates an LSP backend adapter for the given server configuration.
func NewAdapter(cfg config.ServerConfig, languageID string, filePatterns []string) *Adapter {
	return &Adapter{
		serverCfg:    cfg,
		languageID:   languageID,
		filePatterns: filePatterns,
	}
}

func (a *Adapter) Initialize(workspaceRoot string) error {
	client, err := StartClient(a.serverCfg.Command, a.serverCfg.Args, workspaceRoot)
	if err != nil {
		return err
	}
	a.client = client
	a.workspaceRoot = workspaceRoot
	return nil
}

func (a *Adapter) Shutdown() error {
	if a.client != nil {
		return a.client.Shutdown()
	}
	return nil
}

func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error) {
	// Open the file so the server knows about it.
	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, err
	}

	// Convert 1-indexed Location to 0-indexed LSP position.
	line := loc.Line - 1
	character := loc.Column - 1

	fileEdits, err := a.client.Rename(loc.File, line, character, newName)
	if err != nil {
		return nil, err
	}

	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

func (a *Adapter) ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) MoveToFile(loc symbol.Location, destination string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) Capabilities() []backend.Capability {
	return []backend.Capability{
		{Operation: "rename"},
	}
}

// Verify interface compliance at compile time.
var _ backend.RefactoringBackend = (*Adapter)(nil)
```

- [x] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/backend/lsp/... -run TestAdapter -v -timeout 30s
```

Expected: both tests pass (or skip if gopls not installed).

- [x] **Step 5: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement LSP backend adapter with rename support"
```

---

### Task 9: Symbol Resolution (Tiers 2 & 3)

**Files:**
- Create: `internal/symbol/resolver.go`
- Create: `internal/symbol/resolver_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/symbol/resolver_test.go`:

```go
package symbol_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestResolve_Tier3_ExactPosition(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0644)

	query := symbol.Query{
		File:   filePath,
		Line:   3,
		Column: 6,
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if loc.Line != 3 || loc.Column != 6 {
		t.Errorf("expected line=3 col=6, got line=%d col=%d", loc.Line, loc.Column)
	}
	if loc.File != filePath {
		t.Errorf("expected file %s, got %s", filePath, loc.File)
	}
}

func TestResolve_Tier2_FindNameOnLine(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc helloWorld() {}\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "helloWorld",
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if loc.Column != 6 { // "func " = 5 chars, "helloWorld" starts at col 6 (1-indexed)
		t.Errorf("expected column 6, got %d", loc.Column)
	}
	if loc.Name != "helloWorld" {
		t.Errorf("expected name helloWorld, got %s", loc.Name)
	}
}

func TestResolve_Tier2_NameNotFound(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	os.WriteFile(filePath, []byte("package main\n\nfunc hello() {}\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "nonexistent",
	}

	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for name not found on line")
	}
}

func TestResolve_Tier2_MultipleOccurrences(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	// "x" appears twice on this line: as parameter and in body.
	os.WriteFile(filePath, []byte("package main\n\nfunc f(x int) int { return x }\n"), 0644)

	query := symbol.Query{
		File: filePath,
		Line: 3,
		Name: "x",
	}

	// Should return the first occurrence.
	loc, err := symbol.Resolve(query)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if loc.Column != 8 { // "func f(" = 7 chars, "x" at col 8 (1-indexed)
		t.Errorf("expected column 8 (first occurrence), got %d", loc.Column)
	}
}

func TestResolve_InvalidTier(t *testing.T) {
	query := symbol.Query{} // No fields set.
	_, err := symbol.Resolve(query)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/symbol/... -v
```

Expected: compilation error — `symbol.Resolve` not defined.

- [ ] **Step 3: Implement the resolver**

Create `internal/symbol/resolver.go`:

```go
package symbol

import (
	"fmt"
	"os"
	"strings"
)

// Resolve converts a Query into a concrete Location by reading the source file
// and finding the symbol. Supports Tier 2 (file+line+name) and Tier 3 (file+line+col).
// Tier 1 (qualified name) requires a backend and is handled separately.
func Resolve(query Query) (Location, error) {
	switch query.Tier() {
	case 3:
		return resolveTier3(query), nil
	case 2:
		return resolveTier2(query)
	case 1:
		return Location{}, fmt.Errorf("tier 1 (qualified name) resolution requires a backend")
	default:
		return Location{}, fmt.Errorf("invalid query: must specify symbol, file+line+name, or file+line+col")
	}
}

func resolveTier3(query Query) Location {
	return Location{
		File:   query.File,
		Line:   query.Line,
		Column: query.Column,
		Kind:   query.Kind,
	}
}

func resolveTier2(query Query) (Location, error) {
	content, err := os.ReadFile(query.File)
	if err != nil {
		return Location{}, fmt.Errorf("reading %s: %w", query.File, err)
	}

	lines := strings.Split(string(content), "\n")
	lineIdx := query.Line - 1 // Convert 1-indexed to 0-indexed.
	if lineIdx < 0 || lineIdx >= len(lines) {
		return Location{}, fmt.Errorf("line %d out of range (file has %d lines)", query.Line, len(lines))
	}

	line := lines[lineIdx]
	col := strings.Index(line, query.Name)
	if col < 0 {
		return Location{}, fmt.Errorf("name %q not found on line %d of %s", query.Name, query.Line, query.File)
	}

	return Location{
		File:   query.File,
		Line:   query.Line,
		Column: col + 1, // Convert 0-indexed to 1-indexed.
		Name:   query.Name,
		Kind:   query.Kind,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/symbol/... -v
```

Expected: all 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/symbol/resolver.go internal/symbol/resolver_test.go
git commit -m "feat: implement symbol resolution for tiers 2 and 3"
```

---

### Task 10: CLI Rename Commands

**Files:**
- Modify: `cmd/refute/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/rename.go`

- [ ] **Step 1: Add cobra dependency**

```bash
cd /home/ketan/project/refute && go get github.com/spf13/cobra
```

- [ ] **Step 2: Add color dependency**

```bash
go get github.com/fatih/color
```

- [ ] **Step 3: Implement the root command**

Create `internal/cli/root.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.1.0-dev"

var (
	flagConfig  string
	flagDryRun  bool
	flagVerbose bool
)

// RootCmd is the top-level CLI command.
var RootCmd = &cobra.Command{
	Use:   "refute",
	Short: "Automated source code refactoring",
	Long:  "refute orchestrates existing refactoring engines to provide IDE-quality refactoring from the command line.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("refute %s\n", version)
	},
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "path to config file")
	RootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show diff without applying changes")
	RootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "show detailed output")
	RootCmd.AddCommand(versionCmd)
}
```

- [ ] **Step 4: Implement the rename commands**

Create `internal/cli/rename.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

var (
	flagFile    string
	flagLine    int
	flagCol     int
	flagName    string
	flagNewName string
	flagSymbol  string
)

func addRenameFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	cmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed, optional)")
	cmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	cmd.Flags().StringVar(&flagNewName, "new-name", "", "new name for the symbol")
	cmd.Flags().StringVar(&flagSymbol, "symbol", "", "qualified symbol name (e.g., ClassName.method)")
	cmd.MarkFlagRequired("new-name")
}

func makeRenameCmd(use string, kind symbol.SymbolKind) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Rename a %s", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(kind)
		},
	}
	addRenameFlags(cmd)
	return cmd
}

func init() {
	// Generic rename (requires exact position).
	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a symbol (kind-agnostic, requires --file --line --col)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(symbol.KindUnknown)
		},
	}
	addRenameFlags(renameCmd)

	RootCmd.AddCommand(renameCmd)
	RootCmd.AddCommand(makeRenameCmd("rename-function", symbol.KindFunction))
	RootCmd.AddCommand(makeRenameCmd("rename-class", symbol.KindClass))
	RootCmd.AddCommand(makeRenameCmd("rename-field", symbol.KindField))
	RootCmd.AddCommand(makeRenameCmd("rename-variable", symbol.KindVariable))
	RootCmd.AddCommand(makeRenameCmd("rename-parameter", symbol.KindParameter))
	RootCmd.AddCommand(makeRenameCmd("rename-type", symbol.KindType))
	RootCmd.AddCommand(makeRenameCmd("rename-method", symbol.KindMethod))
}

func runRename(kind symbol.SymbolKind) error {
	// Build the symbol query from flags.
	query := symbol.Query{
		QualifiedName: flagSymbol,
		File:          flagFile,
		Line:          flagLine,
		Column:        flagCol,
		Name:          flagName,
		Kind:          kind,
	}

	// Resolve file path to absolute.
	if query.File != "" {
		abs, err := filepath.Abs(query.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query.File = abs
	}

	// Resolve the symbol to a concrete location.
	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	// Determine workspace root (walk up to find go.mod or similar).
	workspaceRoot, err := findWorkspaceRoot(loc.File)
	if err != nil {
		return err
	}

	// Load config.
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Detect language and create adapter.
	language := detectLanguage(loc.File)
	serverCfg := cfg.Server(language)
	if serverCfg.Command == "" {
		return fmt.Errorf("no server configured for language %q", language)
	}

	adapter := lsp.NewAdapter(serverCfg, language, nil)
	if err := adapter.Initialize(workspaceRoot); err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer adapter.Shutdown()

	// Perform the rename.
	we, err := adapter.Rename(loc, flagNewName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	if len(we.FileEdits) == 0 {
		fmt.Fprintln(os.Stderr, "No changes produced.")
		os.Exit(2)
	}

	// Dry-run: show diff and exit.
	if flagDryRun {
		diff, err := edit.RenderDiff(we)
		if err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}

	// Apply edits.
	result, err := edit.Apply(we)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}

	// Print summary.
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s Modified %d file(s):", green("ok"), result.FilesModified)
	for _, fe := range we.FileEdits {
		rel, _ := filepath.Rel(workspaceRoot, fe.Path)
		if rel == "" {
			rel = fe.Path
		}
		fmt.Fprintf(os.Stderr, " %s", rel)
	}
	fmt.Fprintln(os.Stderr)

	if flagVerbose {
		diff, err := edit.RenderDiff(we)
		if err == nil && diff != "" {
			fmt.Print(diff)
		}
	}

	return nil
}

// findWorkspaceRoot walks up from the file to find a directory with go.mod,
// package.json, or similar project markers.
func findWorkspaceRoot(filePath string) (string, error) {
	dir := filepath.Dir(filePath)
	markers := []string{"go.mod", "go.work", "package.json", "tsconfig.json", "pyproject.toml", "setup.py"}

	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding a marker.
			return filepath.Dir(filePath), nil
		}
		dir = parent
	}
}

// detectLanguage returns the LSP language ID based on file extension.
func detectLanguage(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}
```

- [ ] **Step 5: Update main.go to use cobra**

Replace `cmd/refute/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/shatterproof-ai/refute/internal/cli"
)

func main() {
	if err := cli.RootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Verify it builds and shows help**

```bash
go build -o refute ./cmd/refute && ./refute --help
```

Expected: help output listing rename, rename-function, rename-class, etc.

```bash
./refute version
```

Expected: `refute 0.1.0-dev`

- [ ] **Step 7: Commit**

```bash
git add cmd/refute/main.go internal/cli/root.go internal/cli/rename.go go.mod go.sum
git commit -m "feat: implement CLI with rename commands via cobra"
```

---

### Task 11: End-to-End Integration Test

**Files:**
- Create: `testdata/fixtures/go/rename/go.mod`
- Create: `testdata/fixtures/go/rename/main.go`
- Create: `testdata/fixtures/go/rename/util/helper.go`
- Create: `internal/integration_test.go`

- [ ] **Step 1: Create the Go fixture project**

Create `testdata/fixtures/go/rename/go.mod`:

```
module example.com/renametest

go 1.22
```

Create `testdata/fixtures/go/rename/main.go`:

```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	println(msg)
}
```

Create `testdata/fixtures/go/rename/util/helper.go`:

```go
package util

// FormatGreeting returns a greeting string for the given name.
func FormatGreeting(name string) string {
	return "Hello, " + name + "!"
}
```

- [ ] **Step 2: Verify the fixture compiles**

```bash
cd /home/ketan/project/refute/testdata/fixtures/go/rename && go build ./...
```

Expected: clean build.

```bash
cd /home/ketan/project/refute
```

- [ ] **Step 3: Write the end-to-end test**

Create `internal/integration_test.go`:

```go
//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameGoFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	// Copy fixture to temp dir so we don't modify the original.
	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	// Build refute.
	refuteBin := filepath.Join(t.TempDir(), "refute")
	build := exec.Command("go", "build", "-o", refuteBin, "./cmd/refute")
	build.Dir = filepath.Join("..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}

	// Run rename: FormatGreeting → BuildGreeting.
	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// Verify: old name should be gone from all Go files.
	helperContent, _ := os.ReadFile(helperFile)
	if strings.Contains(string(helperContent), "FormatGreeting") {
		t.Error("helper.go still contains FormatGreeting")
	}
	if !strings.Contains(string(helperContent), "BuildGreeting") {
		t.Error("helper.go missing BuildGreeting")
	}

	mainFile := filepath.Join(dir, "main.go")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "FormatGreeting") {
		t.Error("main.go still contains FormatGreeting after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "BuildGreeting") {
		t.Error("main.go missing BuildGreeting")
	}

	// Verify: project still compiles.
	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}

func TestEndToEnd_DryRun(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := filepath.Join(t.TempDir(), "refute")
	build := exec.Command("go", "build", "-o", refuteBin, "./cmd/refute")
	build.Dir = filepath.Join("..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}

	helperFile := filepath.Join(dir, "util", "helper.go")

	// Read original content.
	originalContent, _ := os.ReadFile(helperFile)

	// Run with --dry-run.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute dry-run failed: %s\n%s", err, out)
	}

	// Verify: diff output should contain both old and new names.
	if !strings.Contains(string(out), "FormatGreeting") || !strings.Contains(string(out), "BuildGreeting") {
		t.Errorf("dry-run output should show diff, got:\n%s", out)
	}

	// Verify: file should be unchanged.
	afterContent, _ := os.ReadFile(helperFile)
	if string(afterContent) != string(originalContent) {
		t.Error("dry-run should not modify files")
	}
}

// copyDir recursively copies a directory tree.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			os.MkdirAll(dstPath, 0755)
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("reading %s: %v", srcPath, err)
			}
			os.WriteFile(dstPath, data, 0644)
		}
	}
}
```

- [ ] **Step 4: Run the integration tests**

```bash
cd /home/ketan/project/refute && go test -tags integration ./internal/ -v -timeout 60s
```

Expected: both `TestEndToEnd_RenameGoFunction` and `TestEndToEnd_DryRun` pass (or skip if gopls not installed).

- [ ] **Step 5: Run all tests to verify nothing is broken**

```bash
go test ./... -v -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add testdata/fixtures/ internal/integration_test.go
git commit -m "feat: add end-to-end integration tests for Go rename"
```

---

## Verification Checklist

After all tasks are complete, verify the full workflow:

```bash
# Build
go build -o refute ./cmd/refute

# Version
./refute version
# Expected: refute 0.1.0-dev

# Help
./refute rename-function --help
# Expected: flags listed (--file, --line, --name, --new-name, --dry-run, etc.)

# Dry-run rename on the fixture project
cp -r testdata/fixtures/go/rename /tmp/refute-test
./refute rename-function \
  --file /tmp/refute-test/util/helper.go \
  --line 4 \
  --name FormatGreeting \
  --new-name BuildGreeting \
  --dry-run
# Expected: unified diff output showing the rename

# Apply rename
./refute rename-function \
  --file /tmp/refute-test/util/helper.go \
  --line 4 \
  --name FormatGreeting \
  --new-name BuildGreeting
# Expected: "ok Modified 2 file(s): util/helper.go main.go"

# Verify compilation
cd /tmp/refute-test && go build ./...
# Expected: clean build

# Run all tests
cd /home/ketan/project/refute && go test ./... -timeout 60s
# Expected: all pass
```
