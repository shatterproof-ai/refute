# Go Code Actions + Tier 1 Symbol Resolution — Implementation Plan

> **Status:** abandoned — superseded by the v2 rewrite
> `2026-04-17-go-code-actions-tier1-v2.md`, which is the plan that actually
> shipped (landed 2026-04-28, merge `78c5970`). Kept as a record of the
> superseded approach. See [README.md](README.md) for status semantics.
> **Landing:** not applicable — abandoned before execution; replacement landed
> 2026-04-28, merge `78c5970`.
> **Disposition:** retained historical artifact.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Go LSP backend with extract-function, extract-variable, and inline via gopls code actions, and implement Tier 1 qualified-name symbol resolution (`--symbol "pkg.FunctionName"`) via `workspace/symbol`.

**Architecture:** All new operations route through the existing `lsp.Client` infrastructure. Three new methods are added to `Client` (`CodeActions`, `ResolveCodeAction`, `WorkspaceSymbol`), three new methods are wired into the `Adapter` (`ExtractFunction`, `ExtractVariable`, `InlineSymbol`, `FindSymbol`), and two new CLI command files are added (`cli/extract.go`, `cli/inline.go`). Tier 1 resolution is handled in `cli/rename.go` by calling `adapter.FindSymbol` before `adapter.Rename` when `--symbol` is provided.

**Tech Stack:** Go 1.22+, gopls (already installed), existing cobra/fatih/color deps.

**Prerequisites:** Plan 1 complete — `internal/backend/lsp/{transport,client,adapter}.go` all exist and pass tests.

---

## File Structure

```
internal/backend/lsp/
├── client.go          — ADD: CodeActions(), ResolveCodeAction(), WorkspaceSymbol()
├── client_test.go     — ADD: TestClient_CodeActions, TestClient_WorkspaceSymbol
├── adapter.go         — ADD: FindSymbol(), ExtractFunction(), ExtractVariable(), InlineSymbol(); ADD: resolveAction(), lspKindToSymbolKind(), parseQualifiedName()
└── adapter_test.go    — ADD: TestAdapter_FindSymbol, TestAdapter_ExtractFunction, TestAdapter_ExtractVariable, TestAdapter_InlineSymbol

internal/cli/
├── rename.go          — MODIFY: runRename() handles Tier 1 via adapter.FindSymbol; ADD: workspaceRootForTier1(), finishRename()
├── extract.go         — CREATE: extract-function, extract-variable commands
└── inline.go          — CREATE: inline command

internal/integration_test.go  — ADD: TestEndToEnd_ExtractFunction, TestEndToEnd_Tier1Rename
```

---

### Task 1: LSP Client — Code Action Methods

**Files:**
- Modify: `internal/backend/lsp/client.go`
- Modify: `internal/backend/lsp/client_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/backend/lsp/client_test.go`:

```go
func TestClient_CodeActions(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {
	x := 1 + 2
	println(x)
}
`), 0644)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient failed: %v", err)
	}
	defer client.Shutdown()

	mainFile := filepath.Join(dir, "main.go")
	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen failed: %v", err)
	}

	// Request code actions for the range covering "1 + 2" on line 3 (0-indexed).
	actions, err := client.CodeActions(mainFile, 3, 6, 3, 11, []string{"refactor.extract"})
	if err != nil {
		t.Fatalf("CodeActions failed: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one code action for extractable expression")
	}
	found := false
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.Title), "extract") {
			found = true
		}
	}
	if !found {
		t.Errorf("no extract action found among: %v", actions)
	}
}

func TestClient_WorkspaceSymbol(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t) // reuses helper from client_test.go

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient failed: %v", err)
	}
	defer client.Shutdown()

	mainFile := filepath.Join(dir, "main.go")
	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen failed: %v", err)
	}

	syms, err := client.WorkspaceSymbol("oldName")
	if err != nil {
		t.Fatalf("WorkspaceSymbol failed: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("expected at least one symbol named oldName")
	}
	found := false
	for _, s := range syms {
		if s.Name == "oldName" {
			found = true
		}
	}
	if !found {
		t.Errorf("symbol oldName not in results: %v", syms)
	}
}
```

Also add `"strings"` to the import block in `client_test.go`.

- [ ] **Step 2: Run to verify tests fail**

```bash
cd /home/ketan/project/refute && go test ./internal/backend/lsp/... -run "TestClient_CodeActions|TestClient_WorkspaceSymbol" -v 2>&1 | head -20
```

Expected: compilation error — `client.CodeActions` and `client.WorkspaceSymbol` undefined.

- [ ] **Step 3: Add CodeAction type and CodeActions/ResolveCodeAction/WorkspaceSymbol to client.go**

In `internal/backend/lsp/client.go`, add after the `lspTextEdit` type:

```go
// CodeAction is an LSP code action (refactoring, quick fix, etc.).
type CodeAction struct {
	Title string           `json:"title"`
	Kind  string           `json:"kind,omitempty"`
	Edit  *json.RawMessage `json:"edit,omitempty"`
	Data  json.RawMessage  `json:"data,omitempty"`
}

// WorkspaceSymbolInfo is a single result from workspace/symbol.
type WorkspaceSymbolInfo struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	} `json:"location"`
}
```

Then add these three methods to `Client`:

```go
// CodeActions requests code actions for a range. kinds filters by action kind prefix
// (e.g., []string{"refactor.extract"} returns only extract actions).
func (c *Client) CodeActions(filePath string, startLine, startChar, endLine, endChar int, kinds []string) ([]CodeAction, error) {
	params := map[string]any{
		"textDocument": map[string]any{"uri": fileToURI(filePath)},
		"range": map[string]any{
			"start": map[string]any{"line": startLine, "character": startChar},
			"end":   map[string]any{"line": endLine, "character": endChar},
		},
		"context": map[string]any{
			"diagnostics": []any{},
			"only":        kinds,
		},
	}
	result, err := c.request("textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	var actions []CodeAction
	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("parsing code actions: %w", err)
	}
	return actions, nil
}

// ResolveCodeAction resolves a code action to obtain its WorkspaceEdit.
// Use when the action returned by CodeActions has no Edit field.
func (c *Client) ResolveCodeAction(action CodeAction) ([]edit.FileEdit, error) {
	result, err := c.request("codeAction/resolve", action)
	if err != nil {
		return nil, err
	}
	var resolved CodeAction
	if err := json.Unmarshal(result, &resolved); err != nil {
		return nil, fmt.Errorf("parsing resolved code action: %w", err)
	}
	if resolved.Edit == nil {
		return nil, fmt.Errorf("resolved code action has no edit")
	}
	return parseWorkspaceEdit(*resolved.Edit)
}

// WorkspaceSymbol queries all open workspaces for symbols matching query.
func (c *Client) WorkspaceSymbol(query string) ([]WorkspaceSymbolInfo, error) {
	result, err := c.request("workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	var syms []WorkspaceSymbolInfo
	if err := json.Unmarshal(result, &syms); err != nil {
		return nil, fmt.Errorf("parsing workspace symbols: %w", err)
	}
	return syms, nil
}
```

- [ ] **Step 4: Run to verify tests pass**

```bash
go test ./internal/backend/lsp/... -run "TestClient_CodeActions|TestClient_WorkspaceSymbol" -v -timeout 30s
```

Expected: both tests pass (or skip if gopls not found).

- [ ] **Step 5: Run full test suite to confirm no regressions**

```bash
go test ./... -timeout 60s
```

Expected: all 20 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/backend/lsp/client.go internal/backend/lsp/client_test.go
git commit -m "feat: add CodeActions, ResolveCodeAction, WorkspaceSymbol to LSP client"
```

---

### Task 2: LSP Adapter — FindSymbol (Tier 1)

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/backend/lsp/adapter_test.go`:

```go
func TestAdapter_FindSymbol(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func targetFunc() string {
	return "hello"
}

func main() {
	targetFunc()
}
`), 0644)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer adapter.Shutdown()

	// Open the file so gopls indexes it.
	mainFile := filepath.Join(dir, "main.go")
	adapter.DidOpen(mainFile, "go")

	locs, err := adapter.FindSymbol(symbol.Query{
		QualifiedName: "targetFunc",
		Kind:          symbol.KindFunction,
	})
	if err != nil {
		t.Fatalf("FindSymbol failed: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one location for targetFunc")
	}
	found := false
	for _, l := range locs {
		if l.Name == "targetFunc" {
			found = true
		}
	}
	if !found {
		t.Errorf("targetFunc not in results: %v", locs)
	}
}
```

Note: `adapter.DidOpen` needs to be exported from the adapter. Add this method to `adapter.go` in Step 3 alongside `FindSymbol`.

- [ ] **Step 2: Run to verify test fails**

```bash
go test ./internal/backend/lsp/... -run TestAdapter_FindSymbol -v 2>&1 | head -20
```

Expected: compilation error — `adapter.FindSymbol` and `adapter.DidOpen` undefined.

- [ ] **Step 3: Implement FindSymbol and helpers in adapter.go**

Add to `internal/backend/lsp/adapter.go`:

```go
import (
	"path/filepath"
	"strings"
	// existing imports unchanged
)

// DidOpen exposes file-open notification for use in tests and Tier 1 resolution.
func (a *Adapter) DidOpen(filePath string, languageID string) error {
	return a.client.DidOpen(filePath, languageID)
}

// FindSymbol resolves a Tier 1 qualified name to concrete locations using workspace/symbol.
// Qualified name formats supported:
//   - "FunctionName"           — match by name only
//   - "pkg.FunctionName"       — match by name, filter by package path or containerName
//   - "pkg.Type.MethodName"    — match by name, filter by containerName == "Type"
func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	parts := parseQualifiedName(query.QualifiedName)
	symbolName := parts[len(parts)-1]

	syms, err := a.client.WorkspaceSymbol(symbolName)
	if err != nil {
		return nil, err
	}

	var matches []symbol.Location
	for _, s := range syms {
		if s.Name != symbolName {
			continue
		}
		// For "pkg.Type.Method" — containerName must match "Type".
		if len(parts) >= 3 {
			container := parts[len(parts)-2]
			if s.ContainerName != container {
				continue
			}
		}
		// For "pkg.Symbol" — file path or containerName must relate to "pkg".
		if len(parts) == 2 {
			pkg := parts[0]
			filePath := uriToFile(s.Location.URI)
			inPkg := strings.Contains(filePath, "/"+pkg+"/") ||
				strings.HasSuffix(filepath.Dir(filePath), pkg) ||
				s.ContainerName == pkg
			if !inPkg {
				continue
			}
		}
		// Filter by SymbolKind if the caller specified one.
		if query.Kind != symbol.KindUnknown && lspKindToSymbolKind(s.Kind) != query.Kind {
			continue
		}
		matches = append(matches, symbol.Location{
			File:   uriToFile(s.Location.URI),
			Line:   s.Location.Range.Start.Line + 1,      // 0-indexed → 1-indexed
			Column: s.Location.Range.Start.Character + 1, // 0-indexed → 1-indexed
			Name:   s.Name,
			Kind:   lspKindToSymbolKind(s.Kind),
		})
	}

	if len(matches) == 0 {
		return nil, backend.ErrSymbolNotFound
	}
	return matches, nil
}

// parseQualifiedName splits a dot-separated qualified name.
// "A.B.C" → ["A", "B", "C"]
func parseQualifiedName(name string) []string {
	return strings.Split(name, ".")
}

// lspKindToSymbolKind maps LSP integer symbol kinds to our SymbolKind enum.
func lspKindToSymbolKind(lspKind int) symbol.SymbolKind {
	switch lspKind {
	case 12: // Function
		return symbol.KindFunction
	case 6: // Method
		return symbol.KindMethod
	case 5: // Class
		return symbol.KindClass
	case 23: // Struct (Go structs map here)
		return symbol.KindType
	case 8: // Field
		return symbol.KindField
	case 13: // Variable
		return symbol.KindVariable
	default:
		return symbol.KindUnknown
	}
}
```

Also replace the existing stub `FindSymbol` in `adapter.go`:

```go
// Old stub — delete this:
// func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
// 	return nil, backend.ErrUnsupported
// }
```

- [ ] **Step 4: Run to verify test passes**

```bash
go test ./internal/backend/lsp/... -run TestAdapter_FindSymbol -v -timeout 30s
```

Expected: test passes (or skips if gopls not found).

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -timeout 60s
```

Expected: all 20 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement FindSymbol via workspace/symbol for Tier 1 resolution"
```

---

### Task 3: LSP Adapter — Extract Function and Variable

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/backend/lsp/adapter_test.go`:

```go
func TestAdapter_ExtractFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {
	x := 1 + 2
	println(x)
}
`), 0644)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer adapter.Shutdown()

	// Extract the expression "1 + 2" on line 4, cols 7-12 (1-indexed).
	r := symbol.SourceRange{
		File:     filepath.Join(dir, "main.go"),
		StartLine: 4, StartCol: 7,
		EndLine:   4, EndCol: 12,
	}
	we, err := adapter.ExtractFunction(r, "sum")
	if err != nil {
		t.Fatalf("ExtractFunction failed: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits from extract")
	}
	totalEdits := 0
	for _, fe := range we.FileEdits {
		totalEdits += len(fe.Edits)
	}
	if totalEdits == 0 {
		t.Fatal("expected at least one text edit")
	}
}

func TestAdapter_ExtractVariable(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() {
	println(1 + 2)
}
`), 0644)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer adapter.Shutdown()

	// Extract "1 + 2" on line 4, cols 10-15 (1-indexed).
	r := symbol.SourceRange{
		File:     filepath.Join(dir, "main.go"),
		StartLine: 4, StartCol: 10,
		EndLine:   4, EndCol: 15,
	}
	we, err := adapter.ExtractVariable(r, "sum")
	if err != nil {
		t.Fatalf("ExtractVariable failed: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits from extract")
	}
}
```

- [ ] **Step 2: Run to verify tests fail**

```bash
go test ./internal/backend/lsp/... -run "TestAdapter_Extract" -v 2>&1 | head -20
```

Expected: compilation error — `adapter.ExtractFunction` and `adapter.ExtractVariable` undefined (currently return `ErrUnsupported`).

Wait — these methods already exist on the adapter (returning `ErrUnsupported`). The tests will compile but fail at runtime. That's correct TDD behavior.

- [ ] **Step 3: Implement ExtractFunction, ExtractVariable, and resolveAction in adapter.go**

Replace the existing stubs in `adapter.go`:

```go
func (a *Adapter) ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	if err := a.client.DidOpen(r.File, a.languageID); err != nil {
		return nil, err
	}
	actions, err := a.client.CodeActions(
		r.File,
		r.StartLine-1, r.StartCol-1, // 1-indexed → 0-indexed
		r.EndLine-1, r.EndCol-1,
		[]string{"refactor.extract"},
	)
	if err != nil {
		return nil, err
	}
	for i, action := range actions {
		if action.Kind == "refactor.extract.function" ||
			(strings.HasPrefix(action.Kind, "refactor.extract") &&
				strings.Contains(strings.ToLower(action.Title), "function")) {
			return a.resolveAction(actions[i])
		}
	}
	return nil, backend.ErrUnsupported
}

func (a *Adapter) ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	if err := a.client.DidOpen(r.File, a.languageID); err != nil {
		return nil, err
	}
	actions, err := a.client.CodeActions(
		r.File,
		r.StartLine-1, r.StartCol-1,
		r.EndLine-1, r.EndCol-1,
		[]string{"refactor.extract"},
	)
	if err != nil {
		return nil, err
	}
	for i, action := range actions {
		if action.Kind == "refactor.extract.variable" ||
			(strings.HasPrefix(action.Kind, "refactor.extract") &&
				strings.Contains(strings.ToLower(action.Title), "variable")) {
			return a.resolveAction(actions[i])
		}
	}
	return nil, backend.ErrUnsupported
}

// resolveAction obtains the WorkspaceEdit for a code action.
// If the action already has an Edit, parse it; otherwise send codeAction/resolve.
func (a *Adapter) resolveAction(action CodeAction) (*edit.WorkspaceEdit, error) {
	var fileEdits []edit.FileEdit
	var err error
	if action.Edit != nil {
		fileEdits, err = parseWorkspaceEdit(*action.Edit)
	} else {
		fileEdits, err = a.client.ResolveCodeAction(action)
	}
	if err != nil {
		return nil, err
	}
	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}
```

Add `"strings"` to the import block in `adapter.go` if not already present.

- [ ] **Step 4: Run to verify tests pass**

```bash
go test ./internal/backend/lsp/... -run "TestAdapter_Extract" -v -timeout 30s
```

Expected: both tests pass (or skip if gopls not found).

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement ExtractFunction and ExtractVariable via gopls code actions"
```

---

### Task 4: LSP Adapter — Inline Symbol

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/backend/lsp/adapter_test.go`:

```go
func TestAdapter_InlineSymbol(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func add(a, b int) int { return a + b }

func main() {
	println(add(1, 2))
}
`), 0644)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer adapter.Shutdown()

	// Inline the call to add() on line 6, col 10 (1-indexed, on "add").
	loc := symbol.Location{
		File:   filepath.Join(dir, "main.go"),
		Line:   6,
		Column: 10,
		Name:   "add",
		Kind:   symbol.KindFunction,
	}
	we, err := adapter.InlineSymbol(loc)
	if err != nil {
		// gopls inline support varies by version; accept ErrUnsupported gracefully.
		if errors.Is(err, backend.ErrUnsupported) {
			t.Skip("inline not supported by this gopls version")
		}
		t.Fatalf("InlineSymbol failed: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits from inline")
	}
}
```

Add `"errors"` to the import block in `adapter_test.go`.

- [ ] **Step 2: Run to verify test fails**

```bash
go test ./internal/backend/lsp/... -run TestAdapter_InlineSymbol -v 2>&1 | head -20
```

Expected: compilation error (missing `errors` import) or test fails with `ErrUnsupported`.

- [ ] **Step 3: Implement InlineSymbol in adapter.go**

Replace the existing stub:

```go
func (a *Adapter) InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error) {
	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, err
	}
	// Inline operates on a point position; use a zero-length range.
	line := loc.Line - 1
	char := loc.Column - 1
	actions, err := a.client.CodeActions(
		loc.File,
		line, char, line, char,
		[]string{"refactor.inline"},
	)
	if err != nil {
		return nil, err
	}
	for i, action := range actions {
		if strings.HasPrefix(action.Kind, "refactor.inline") ||
			strings.Contains(strings.ToLower(action.Title), "inline") {
			return a.resolveAction(actions[i])
		}
	}
	return nil, backend.ErrUnsupported
}
```

- [ ] **Step 4: Run to verify test passes**

```bash
go test ./internal/backend/lsp/... -run TestAdapter_InlineSymbol -v -timeout 30s
```

Expected: test passes or skips.

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement InlineSymbol via gopls refactor.inline code action"
```

---

### Task 5: CLI — Update Rename for Tier 1

**Files:**
- Modify: `internal/cli/rename.go`

The current `runRename` does `symbol.Resolve(query)` first, which fails for Tier 1. Tier 1 needs the adapter initialized before resolution.

- [ ] **Step 1: Refactor runRename to extract finishRename and workspaceRootForTier1 helpers**

Replace the entire body of `internal/cli/rename.go` with:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
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
	query := symbol.Query{
		QualifiedName: flagSymbol,
		File:          flagFile,
		Line:          flagLine,
		Column:        flagCol,
		Name:          flagName,
		Kind:          kind,
	}

	if query.Tier() == 1 {
		return runRenameTier1(query)
	}

	// Tier 2 or 3: resolve position from file.
	if query.File != "" {
		abs, err := filepath.Abs(query.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query.File = abs
	}
	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	adapter, workspaceRoot, err := buildAdapter(loc.File)
	if err != nil {
		return err
	}
	defer adapter.Shutdown()
	return finishRename(adapter, workspaceRoot, loc, flagNewName)
}

func runRenameTier1(query symbol.Query) error {
	workspaceRoot, err := workspaceRootForTier1()
	if err != nil {
		return err
	}

	language := "go"
	if flagFile != "" {
		language = detectLanguage(flagFile)
	}

	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	serverCfg := cfg.Server(language)
	if serverCfg.Command == "" {
		return fmt.Errorf("no server configured for language %q", language)
	}

	adapter := lsp.NewAdapter(serverCfg, language, nil)
	if err := adapter.Initialize(workspaceRoot); err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer adapter.Shutdown()

	locs, err := adapter.FindSymbol(query)
	if err != nil {
		return fmt.Errorf("symbol not found: %w", err)
	}
	if len(locs) > 1 {
		fmt.Fprintln(os.Stderr, "Ambiguous — multiple candidates:")
		for _, l := range locs {
			fmt.Fprintf(os.Stderr, "  %s:%d:%d  %s\n", l.File, l.Line, l.Column, l.Name)
		}
		return fmt.Errorf("use --file and --line to narrow the selection")
	}
	return finishRename(adapter, workspaceRoot, locs[0], flagNewName)
}

// buildAdapter creates and initializes an LSP adapter for the given file.
func buildAdapter(filePath string) (*lsp.Adapter, string, error) {
	workspaceRoot, err := findWorkspaceRoot(filePath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	language := detectLanguage(filePath)
	serverCfg := cfg.Server(language)
	if serverCfg.Command == "" {
		return nil, "", fmt.Errorf("no server configured for language %q", language)
	}
	adapter := lsp.NewAdapter(serverCfg, language, nil)
	if err := adapter.Initialize(workspaceRoot); err != nil {
		return nil, "", fmt.Errorf("initializing backend: %w", err)
	}
	return adapter, workspaceRoot, nil
}

// finishRename applies or previews the rename edit.
func finishRename(adapter *lsp.Adapter, workspaceRoot string, loc symbol.Location, newName string) error {
	we, err := adapter.Rename(loc, newName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		fmt.Fprintln(os.Stderr, "No changes produced.")
		os.Exit(2)
	}
	return applyOrPreview(we, workspaceRoot)
}

// applyOrPreview either prints a diff (--dry-run) or applies edits to disk.
func applyOrPreview(we *edit.WorkspaceEdit, workspaceRoot string) error {
	if flagDryRun {
		diff, err := edit.RenderDiff(we)
		if err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}
	result, err := edit.Apply(we)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}
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

// workspaceRootForTier1 determines the workspace root when no --file is given.
// Prefers --file if provided; falls back to the current directory.
func workspaceRootForTier1() (string, error) {
	if flagFile != "" {
		abs, err := filepath.Abs(flagFile)
		if err != nil {
			return "", err
		}
		return findWorkspaceRoot(abs)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	root, err := findWorkspaceRoot(cwd + "/dummy.go") // trick findWorkspaceRoot into scanning from cwd
	if err != nil {
		return cwd, nil
	}
	return root, nil
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

// Suppress unused import warning — backend is used via errors.Is in callers.
var _ = backend.ErrUnsupported
```

- [ ] **Step 2: Verify it builds**

```bash
go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 3: Run all tests**

```bash
go test ./... -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 4: Smoke-test Tier 1 rename manually**

```bash
cp -r testdata/fixtures/go/rename /tmp/refute-tier1-test
go build -o /tmp/refute-bin ./cmd/refute

/tmp/refute-bin rename-function \
  --symbol "FormatGreeting" \
  --new-name "BuildGreeting" \
  --dry-run 2>&1
```

Run this from inside `/tmp/refute-tier1-test`. Expected: unified diff showing the rename (or an error message about ambiguity/not found — acceptable at this stage since gopls workspace/symbol requires files to be open).

Clean up: `rm -rf /tmp/refute-tier1-test /tmp/refute-bin`

- [ ] **Step 5: Commit**

```bash
git add internal/cli/rename.go
git commit -m "refactor: split runRename into Tier 1 / Tier 2-3 paths; extract finishRename, buildAdapter helpers"
```

---

### Task 6: CLI — extract-function and extract-variable Commands

**Files:**
- Create: `internal/cli/extract.go`

- [ ] **Step 1: Create the extract commands**

Create `internal/cli/extract.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

var (
	flagStartLine int
	flagStartCol  int
	flagEndLine   int
	flagEndCol    int
	flagExtName   string
)

func addExtractFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagStartLine, "start-line", 0, "start line of selection (1-indexed)")
	cmd.Flags().IntVar(&flagStartCol, "start-col", 0, "start column of selection (1-indexed)")
	cmd.Flags().IntVar(&flagEndLine, "end-line", 0, "end line of selection (1-indexed)")
	cmd.Flags().IntVar(&flagEndCol, "end-col", 0, "end column of selection (1-indexed)")
	cmd.Flags().StringVar(&flagExtName, "name", "", "name for the extracted symbol (note: not yet applied automatically)")
	cmd.MarkFlagRequired("file")
	cmd.MarkFlagRequired("start-line")
	cmd.MarkFlagRequired("start-col")
	cmd.MarkFlagRequired("end-line")
	cmd.MarkFlagRequired("end-col")
}

func init() {
	extractFuncCmd := &cobra.Command{
		Use:   "extract-function",
		Short: "Extract a selection into a new function",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("function")
		},
	}
	addExtractFlags(extractFuncCmd)

	extractVarCmd := &cobra.Command{
		Use:   "extract-variable",
		Short: "Extract a selection into a new variable",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExtract("variable")
		},
	}
	addExtractFlags(extractVarCmd)

	RootCmd.AddCommand(extractFuncCmd)
	RootCmd.AddCommand(extractVarCmd)
}

func runExtract(kind string) error {
	if flagFile == "" {
		return fmt.Errorf("--file is required")
	}
	if flagStartLine == 0 || flagStartCol == 0 || flagEndLine == 0 || flagEndCol == 0 {
		return fmt.Errorf("--start-line, --start-col, --end-line, --end-col are all required")
	}

	adapter, workspaceRoot, err := buildAdapter(flagFile)
	if err != nil {
		return err
	}
	defer adapter.Shutdown()

	r := symbol.SourceRange{
		File:      flagFile,
		StartLine: flagStartLine,
		StartCol:  flagStartCol,
		EndLine:   flagEndLine,
		EndCol:    flagEndCol,
	}

	var we interface{ FileEdits() []interface{} }
	_ = we

	switch kind {
	case "function":
		result, err := adapter.ExtractFunction(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-function failed: %w", err)
		}
		return applyOrPreview(result, workspaceRoot)
	case "variable":
		result, err := adapter.ExtractVariable(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-variable failed: %w", err)
		}
		return applyOrPreview(result, workspaceRoot)
	default:
		return fmt.Errorf("unknown extract kind %q", kind)
	}
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 3: Verify help output shows new commands**

```bash
go build -o /tmp/refute-bin ./cmd/refute && /tmp/refute-bin --help 2>&1 | grep extract
```

Expected: lines for `extract-function` and `extract-variable`.

Clean up: `rm /tmp/refute-bin`

- [ ] **Step 4: Run all tests**

```bash
go test ./... -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/extract.go
git commit -m "feat: add extract-function and extract-variable CLI commands"
```

---

### Task 7: CLI — inline Command

**Files:**
- Create: `internal/cli/inline.go`

- [ ] **Step 1: Create the inline command**

Create `internal/cli/inline.go`:

```go
package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

func init() {
	inlineCmd := &cobra.Command{
		Use:   "inline",
		Short: "Inline the symbol at the given position",
		Long: `Inline inlines a variable or function call at the specified file position.
Requires --file and either --line --col (exact position) or --line --name (scan line).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInline()
		},
	}
	inlineCmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	inlineCmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	inlineCmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed)")
	inlineCmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	inlineCmd.MarkFlagRequired("file")
	inlineCmd.MarkFlagRequired("line")

	RootCmd.AddCommand(inlineCmd)
}

func runInline() error {
	if flagFile == "" || flagLine == 0 {
		return fmt.Errorf("--file and --line are required")
	}

	absFile, err := filepath.Abs(flagFile)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}

	query := symbol.Query{
		File:   absFile,
		Line:   flagLine,
		Column: flagCol,
		Name:   flagName,
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	adapter, workspaceRoot, err := buildAdapter(loc.File)
	if err != nil {
		return err
	}
	defer adapter.Shutdown()

	we, err := adapter.InlineSymbol(loc)
	if err != nil {
		return fmt.Errorf("inline failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		fmt.Fprintln(flagFile, "No changes produced.")
		return nil
	}
	return applyOrPreview(we, workspaceRoot)
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
go build ./... 2>&1
```

Expected: clean build.

- [ ] **Step 3: Verify help shows inline command**

```bash
go build -o /tmp/refute-bin ./cmd/refute && /tmp/refute-bin --help 2>&1 | grep inline
```

Expected: `inline` listed.

Clean up: `rm /tmp/refute-bin`

- [ ] **Step 4: Run all tests**

```bash
go test ./... -timeout 60s
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/inline.go
git commit -m "feat: add inline CLI command"
```

---

### Task 8: End-to-End Integration Tests

**Files:**
- Modify: `internal/integration_test.go`
- Modify: `testdata/fixtures/go/rename/main.go` (add a function usable for extract test)

- [ ] **Step 1: Add an extractable expression to the fixture**

The existing fixture `testdata/fixtures/go/rename/main.go` is:
```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	println(msg)
}
```

Replace it with a version that has an inline arithmetic expression for the extract test:

```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	result := 6*7 + 1
	println(msg, result)
}
```

Write this to `testdata/fixtures/go/rename/main.go`.

- [ ] **Step 2: Verify the fixture still compiles**

```bash
cd /home/ketan/project/refute/testdata/fixtures/go/rename && go build ./... && cd /home/ketan/project/refute
```

Expected: clean build.

- [ ] **Step 3: Add integration tests**

In `internal/integration_test.go`, add after the existing `copyDir` function:

```go
func TestEndToEnd_ExtractFunction(t *testing.T) {
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

	mainFile := filepath.Join(dir, "main.go")

	// Extract the expression "6*7 + 1" on line 7, cols 12-18 (1-indexed).
	// Line 7 is: "\tresult := 6*7 + 1"
	// "6*7 + 1" starts at col 12 (tab=1, "result := " = 10 chars, so col 12).
	cmd := exec.Command(refuteBin,
		"extract-function",
		"--file", mainFile,
		"--start-line", "7",
		"--start-col", "12",
		"--end-line", "7",
		"--end-col", "19",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute extract-function failed: %s\n%s", err, out)
	}

	// Verify: project still compiles after extract.
	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after extract:\n%s", out)
	}
}

func TestEndToEnd_Tier1Rename(t *testing.T) {
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

	// Tier 1 rename: --symbol "FormatGreeting" with --file to scope workspace.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "FormatGreeting",
		"--file", helperFile,
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tier 1 rename failed: %s\n%s", err, out)
	}

	// Verify: cross-file rename applied.
	helperContent, _ := os.ReadFile(helperFile)
	if strings.Contains(string(helperContent), "FormatGreeting") {
		t.Error("helper.go still contains FormatGreeting after tier 1 rename")
	}

	mainContent, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if strings.Contains(string(mainContent), "FormatGreeting") {
		t.Error("main.go still contains FormatGreeting after tier 1 rename")
	}

	// Verify: project still compiles.
	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after tier 1 rename:\n%s", out)
	}
}
```

- [ ] **Step 4: Run the integration tests**

```bash
cd /home/ketan/project/refute && go test -tags integration ./internal/ -v -timeout 120s
```

Expected: all 4 integration tests pass (or skip if gopls not found).

- [ ] **Step 5: Run full test suite**

```bash
go test ./... -timeout 60s
```

Expected: all unit tests pass.

- [ ] **Step 6: Commit**

```bash
git add testdata/fixtures/go/rename/main.go internal/integration_test.go
git commit -m "feat: add E2E tests for extract-function and Tier 1 rename"
```

---

## Verification Checklist

After all tasks, run the full verification:

```bash
# Build
go build -o /tmp/refute-bin ./cmd/refute

# Help shows new commands
/tmp/refute-bin --help
# Expected: extract-function, extract-variable, inline listed alongside rename-*

# Tier 1 rename (dry-run)
cp -r testdata/fixtures/go/rename /tmp/refute-v2-test
/tmp/refute-bin rename-function \
  --symbol "FormatGreeting" \
  --file /tmp/refute-v2-test/util/helper.go \
  --new-name "BuildGreeting" \
  --dry-run
# Expected: diff across helper.go and main.go

# Extract function (dry-run)
/tmp/refute-bin extract-function \
  --file /tmp/refute-v2-test/main.go \
  --start-line 7 --start-col 12 \
  --end-line 7 --end-col 19 \
  --dry-run
# Expected: diff showing new function extracted from main.go

# All unit tests
go test ./... -timeout 60s
# Expected: all pass

# Integration tests
go test -tags integration ./internal/ -timeout 120s
# Expected: 4 tests pass

# Clean up
rm -rf /tmp/refute-bin /tmp/refute-v2-test
```

---

## Deferred

- **`--name` for extract**: gopls chooses the extracted symbol's name automatically. Applying a user-specified name requires a follow-up rename of the generated symbol — deferred to Plan 3.
- **TypeScript and Python backends**: ts-morph and rope subprocesses — Plan 3.
- **Daemon mode**: long-lived process, Unix socket, backend pool — Plan 4.
- **MCP server**: requires daemon — Plan 4.
- **`refute move`**: `MoveToFile` adapter implementation — deferred.
- **`refute pattern`**: ast-grep integration — deferred.
