# Go Code Actions + Tier 1 Symbol Resolution — Implementation Plan (v2)

> **Status:** executed — Go extract/inline code actions and Tier 1 symbol
> resolution shipped via the `go-code-actions` expedition (landed to `main`
> 2026-04-28, merge `78c5970`). Historical artifact; see
> [README.md](README.md) for status semantics.
> **Landing:** 2026-04-28, merge `78c5970`.
> **Disposition:** retained historical artifact.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Go LSP backend with extract-function, extract-variable, and inline via gopls code actions, and implement Tier 1 qualified-name symbol resolution (`--symbol "pkg.FunctionName"`) via `workspace/symbol` — with the workspace priming, disambiguation, and agent-friendly output that make it usable from an MCP server.

**Architecture:** New operations route through the existing `lsp.Client`. Three new Client methods (`CodeActions`, `ResolveCodeActionEdit`, `WorkspaceSymbol`) and three new Adapter methods (`FindSymbol`, `ExtractFunction`, `ExtractVariable`, `InlineSymbol`) are added. Tier 1 resolution is handled in `cli/rename.go` by calling `adapter.FindSymbol` after priming the workspace. A new `--json` flag emits structured output for MCP consumers.

**Tech Stack:** Go 1.22+, gopls (already installed), existing cobra/fatih/color deps, stdlib `encoding/json`.

**Supersedes:** `docs/plans/2026-04-16-go-code-actions-tier1.md`. Key changes from v1:
- Tier 1 now primes the workspace (didOpen all relevant files) before querying `workspace/symbol`. v1 would have returned empty results against an un-indexed workspace.
- Qualified-name parser disambiguates `Type.Method` vs `pkg.Func` via capitalization heuristic.
- Package matching uses gopls's `ContainerName`, not filesystem path heuristics.
- `ExtractFunction`/`ExtractVariable` honor `--name` via placeholder rewriting (no longer silently ignored).
- `InlineSymbol` sends the identifier-width range, not a zero-length range.
- New `--json` output path for structured agent consumption.
- `findWorkspaceRoot` takes a directory; no more `dummy.go` trick.
- `os.Exit(2)` replaced with a typed error that lets deferred `Shutdown` run.
- `inline.go` compile bug fixed.
- Commit splits separate pure refactor from feature addition.

**Prerequisites:** Plan 1 complete — `internal/backend/lsp/{transport,client,adapter}.go` all exist and pass tests.

---

## File Structure

```
internal/backend/lsp/
├── client.go          — ADD: CodeAction, WorkspaceSymbolInfo types; CodeActions(),
│                              ResolveCodeActionEdit(), WorkspaceSymbol()
├── client_test.go     — ADD: TestClient_CodeActions, TestClient_WorkspaceSymbol
├── adapter.go         — ADD: FindSymbol(), ExtractFunction(), ExtractVariable(),
│                              InlineSymbol(), DidOpen(); helpers parseQualifiedName,
│                              lspKindToSymbolKind, resolveAction, rewritePlaceholder
├── adapter_test.go    — ADD: TestAdapter_FindSymbol, _ExtractFunction,
│                              _ExtractVariable, _InlineSymbol
└── workspace.go       — CREATE: PrimeGoWorkspace (walks *.go files, skips vendor/node_modules/.git)

internal/edit/
├── json.go            — CREATE: RenderJSON() produces agent-consumable output
└── json_test.go       — CREATE: roundtrip tests

internal/cli/
├── errors.go          — CREATE: ExitCodeError type, Run() wrapper that maps to exit codes
├── workspace.go       — CREATE: findWorkspaceRoot(dir), detectLanguage (moved from rename.go)
├── rename.go          — MODIFY: extract buildAdapter, finishRename, applyOrPreview;
│                              add runRenameTier1
├── extract.go         — CREATE: extract-function, extract-variable commands
└── inline.go          — CREATE: inline command

cmd/refute/main.go     — MODIFY: use cli.Run wrapper for exit codes

internal/integration_test.go  — ADD: TestEndToEnd_ExtractFunction,
                                      TestEndToEnd_Tier1Rename,
                                      TestEndToEnd_Tier1Ambiguous,
                                      TestEndToEnd_Tier1NotFound

testdata/fixtures/go/rename/main.go — MODIFY: add extractable expression

docs/
└── position-encoding.md          — CREATE: short note on byte vs UTF-16 column semantics
```

### Why these splits

- `workspace.go` (backend/lsp) holds the priming logic — it's a Go-specific helper today, and lives near the adapter that uses it. When TypeScript/Python backends arrive, each gets its own `workspace.go` in its own package.
- `workspace.go` (cli) holds path and language helpers used by every command. `rename.go` no longer owns them.
- `errors.go` (cli) centralizes exit-code mapping so commands return errors and `main` decides the process exit.
- `edit/json.go` keeps JSON rendering next to diff rendering. Both answer "how do we show a WorkspaceEdit to a consumer?"

---

## Output Contract (JSON)

All commands emit this shape when `--json` is passed. This is the contract Plan 4 (MCP) will forward.

```json
{
  "status": "applied" | "dry-run" | "no-op",
  "filesModified": 2,
  "edits": [
    {
      "file": "/abs/path/foo.go",
      "changes": [
        {"startLine": 3, "startCol": 5, "endLine": 3, "endCol": 12, "newText": "newName"}
      ]
    }
  ],
  "newSymbol": {
    "file": "/abs/path/foo.go",
    "line": 20,
    "column": 1,
    "name": "sum"
  },
  "candidates": [
    {"file": "/abs/a.go", "line": 5, "column": 6, "name": "Foo", "kind": "function"}
  ]
}
```

`newSymbol` is populated for extract operations when the adapter can project the new identifier's post-edit location. `candidates` is populated when a Tier 1 query is ambiguous (multiple matches); in that case `status` is `"ambiguous"` and `edits` is empty.

---

### Task 1: Workspace helpers — refactor `findWorkspaceRoot`, add language helper file

**Files:**
- Create: `internal/cli/workspace.go`
- Modify: `internal/cli/rename.go` (remove `findWorkspaceRoot`, `detectLanguage`)

Pure refactor — no behavior change. Splitting this out now so the Tier 1 task (Task 8) has a clean `findWorkspaceRoot(dir string)` to call.

- [ ] **Step 1: Create `internal/cli/workspace.go`**

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// workspaceMarkers are files that indicate a project root.
var workspaceMarkers = []string{
	"go.mod", "go.work", "package.json", "tsconfig.json",
	"pyproject.toml", "setup.py",
}

// FindWorkspaceRootFromDir walks up from dir to find a directory containing
// any workspaceMarker. Returns dir if no marker is found before the filesystem
// root (caller decides whether that's acceptable).
func FindWorkspaceRootFromDir(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", dir, err)
	}
	cur := absDir
	for {
		for _, m := range workspaceMarkers {
			if _, err := os.Stat(filepath.Join(cur, m)); err == nil {
				return cur, nil
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return absDir, nil
		}
		cur = parent
	}
}

// FindWorkspaceRootFromFile is a convenience wrapper that starts the walk
// from the directory containing filePath.
func FindWorkspaceRootFromFile(filePath string) (string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", filePath, err)
	}
	return FindWorkspaceRootFromDir(filepath.Dir(abs))
}

// DetectLanguage returns the LSP language ID based on file extension.
// Returns "" when the extension is unknown.
func DetectLanguage(filePath string) string {
	switch filepath.Ext(filePath) {
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

- [ ] **Step 2: Remove the old helpers from `rename.go`**

Delete `findWorkspaceRoot` and `detectLanguage` from `internal/cli/rename.go` (lines 171-213 in the current file). Replace call sites in `runRename`:

```go
// Was: workspaceRoot, err := findWorkspaceRoot(loc.File)
workspaceRoot, err := FindWorkspaceRootFromFile(loc.File)

// Was: language := detectLanguage(loc.File)
language := DetectLanguage(loc.File)
```

- [ ] **Step 3: Verify build and tests**

```bash
cd /home/ketan/project/refute && go build ./... && go test ./... -timeout 60s
```

Expected: clean build, all existing tests pass (including integration tests if `gopls` is on PATH).

- [ ] **Step 4: Commit**

```bash
git add internal/cli/workspace.go internal/cli/rename.go
git commit -m "refactor: extract FindWorkspaceRoot and DetectLanguage into cli/workspace.go"
```

---

### Task 2: Exit-code error type

**Files:**
- Create: `internal/cli/errors.go`
- Modify: `cmd/refute/main.go`
- Modify: `internal/cli/rename.go` (replace `os.Exit(2)` call site)

The current `runRename` calls `os.Exit(2)` when no edits are produced. Because the function has `defer adapter.Shutdown()` above that point, `os.Exit` skips the defer and orphans the gopls subprocess. Fix by returning a typed error that `main` maps to an exit code.

- [ ] **Step 1: Create `internal/cli/errors.go`**

```go
package cli

import (
	"errors"
	"fmt"
	"os"
)

// ExitCodeError carries a requested process exit code alongside an optional
// message. Commands return this instead of calling os.Exit, so deferred
// cleanup (Shutdown, file close) always runs.
type ExitCodeError struct {
	Code    int
	Message string
}

func (e *ExitCodeError) Error() string {
	return e.Message
}

// NoEditsError is returned when a refactoring produced no changes. Exit 2 is
// the refute convention for "nothing to do" (useful for scripting).
func NoEditsError() error {
	return &ExitCodeError{Code: 2, Message: "no changes produced"}
}

// Run executes cmd and maps any returned error to an exit code:
//
//	nil                → 0
//	*ExitCodeError     → e.Code (message printed to stderr only if non-empty)
//	anything else      → 1      (message printed to stderr)
func Run(run func() error) {
	err := run()
	if err == nil {
		os.Exit(0)
	}
	var ec *ExitCodeError
	if errors.As(err, &ec) {
		if ec.Message != "" {
			fmt.Fprintln(os.Stderr, ec.Message)
		}
		os.Exit(ec.Code)
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
```

- [ ] **Step 2: Wire `cli.Run` into `cmd/refute/main.go`**

Read the current `cmd/refute/main.go` and replace whatever pattern calls `RootCmd.Execute()` directly with:

```go
package main

import "github.com/shatterproof-ai/refute/internal/cli"

func main() {
	cli.Run(cli.RootCmd.Execute)
}
```

- [ ] **Step 3: Replace `os.Exit(2)` in `rename.go`**

In `runRename`, change the no-edits block:

```go
// Was:
//   if len(we.FileEdits) == 0 {
//       fmt.Fprintln(os.Stderr, "No changes produced.")
//       os.Exit(2)
//   }
// Becomes:
if len(we.FileEdits) == 0 {
	return NoEditsError()
}
```

- [ ] **Step 4: Verify build and tests**

```bash
go build ./... && go test ./... -timeout 60s
```

Expected: clean build, all tests pass. The dry-run integration test still passes because it produces edits.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/errors.go cmd/refute/main.go internal/cli/rename.go
git commit -m "refactor: replace os.Exit with ExitCodeError so defers run"
```

---

### Task 3: LSP Client — code action and workspace/symbol methods

**Files:**
- Modify: `internal/backend/lsp/client.go`
- Modify: `internal/backend/lsp/client_test.go`

- [ ] **Step 1: Add types and methods to `client.go`**

Add these types near the existing `lspTextEdit`:

```go
// CodeAction is an LSP code action (refactoring, quick fix, etc.).
type CodeAction struct {
	Title string           `json:"title"`
	Kind  string           `json:"kind,omitempty"`
	Edit  *json.RawMessage `json:"edit,omitempty"`
	Data  *json.RawMessage `json:"data,omitempty"`
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

Both `Edit` and `Data` are pointer types with `omitempty` so empty/absent fields never produce `"data":null` in outgoing messages — some servers reject unexpected nulls.

Add these three methods to `Client`:

```go
// CodeActions requests code actions for a range. kinds filters by action kind
// prefix (e.g., []string{"refactor.extract"} returns only extract actions).
// All positions are 0-indexed (LSP convention).
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
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var actions []CodeAction
	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("parsing code actions: %w", err)
	}
	return actions, nil
}

// ResolveCodeActionEdit resolves a code action to its file edits. Use when the
// action returned by CodeActions has no Edit field attached.
func (c *Client) ResolveCodeActionEdit(action CodeAction) ([]edit.FileEdit, error) {
	result, err := c.request("codeAction/resolve", action)
	if err != nil {
		return nil, err
	}
	var resolved CodeAction
	if err := json.Unmarshal(result, &resolved); err != nil {
		return nil, fmt.Errorf("parsing resolved code action: %w", err)
	}
	if resolved.Edit == nil {
		return nil, fmt.Errorf("resolved code action %q has no edit", resolved.Title)
	}
	return parseWorkspaceEdit(*resolved.Edit)
}

// WorkspaceSymbol queries the server for symbols matching query. Results are
// limited to packages that the server has already loaded — callers that need
// broad coverage should prime the workspace first.
func (c *Client) WorkspaceSymbol(query string) ([]WorkspaceSymbolInfo, error) {
	result, err := c.request("workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var syms []WorkspaceSymbolInfo
	if err := json.Unmarshal(result, &syms); err != nil {
		return nil, fmt.Errorf("parsing workspace symbols: %w", err)
	}
	return syms, nil
}
```

- [ ] **Step 2: Add client tests**

In `internal/backend/lsp/client_test.go`, add `"strings"` to the import block and append:

```go
func TestClient_CodeActions(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func main() {
	x := 1 + 2
	println(x)
}
`
	mainFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainFile, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	// "1 + 2" is on 0-indexed line 3, columns 6-11 (after "\tx := ").
	actions, err := client.CodeActions(mainFile, 3, 6, 3, 11, []string{"refactor.extract"})
	if err != nil {
		t.Fatalf("CodeActions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one extract action")
	}
	found := false
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.Title), "extract") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no extract action in results: %v", actions)
	}
}

func TestClient_WorkspaceSymbol(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	// Prime gopls: workspace/symbol only searches loaded packages.
	if err := client.DidOpen(filepath.Join(dir, "main.go"), "go"); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	syms, err := client.WorkspaceSymbol("oldName")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	found := false
	for _, s := range syms {
		if s.Name == "oldName" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("oldName not in results: %v", syms)
	}
}
```

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/backend/lsp/... -run "TestClient_CodeActions|TestClient_WorkspaceSymbol" -v -timeout 30s
```

Expected: both tests pass (or skip if `gopls` missing).

- [ ] **Step 4: Run full suite**

```bash
go test ./... -timeout 60s
```

Expected: all existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/backend/lsp/client.go internal/backend/lsp/client_test.go
git commit -m "feat: add CodeActions, ResolveCodeActionEdit, WorkspaceSymbol to LSP client"
```

---

### Task 4: Workspace priming helper for Go

**Files:**
- Create: `internal/backend/lsp/workspace.go`
- Create: `internal/backend/lsp/workspace_test.go`

`workspace/symbol` in gopls only returns results for packages the server has loaded. A freshly-initialized client has loaded nothing. This helper walks a Go workspace and sends `didOpen` notifications so gopls can build its index.

- [ ] **Step 1: Create `internal/backend/lsp/workspace.go`**

```go
package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// primeFilesCap bounds how many files we'll open during priming. Large
// monorepos can exceed this; callers then get partial results. A future
// change can make this configurable.
const primeFilesCap = 500

// skipDirs are directories we never recurse into during priming.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	".svn":         true,
	".hg":          true,
	"testdata":     false, // allow — refute's own tests live here
}

// PrimeGoWorkspace opens every *.go file under workspaceRoot (skipping
// vendor/node_modules/.git) via DidOpen so gopls can index their packages.
// Stops after primeFilesCap files. Returns the number of files opened and
// any fatal walk error.
//
// NOTE: DidOpen is an LSP notification (fire-and-forget). Package loading
// happens asynchronously in gopls. Callers who immediately issue
// workspace/symbol may see partial results during indexing. This helper
// issues a zero-result workspace/symbol call at the end to force a
// round-trip, which drains the notification queue on the client side but
// does not wait for gopls's background loader to finish.
func (c *Client) PrimeGoWorkspace(workspaceRoot string) (int, error) {
	opened := 0
	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil // skip unreadable paths silently
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if skipDirs[base] || strings.HasPrefix(base, ".") && base != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if opened >= primeFilesCap {
			return filepath.SkipAll
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if err := c.DidOpen(path, "go"); err != nil {
			return fmt.Errorf("DidOpen %s: %w", path, err)
		}
		opened++
		return nil
	})
	if err != nil {
		return opened, err
	}
	// Round-trip to drain our notification queue. Result ignored.
	_, _ = c.WorkspaceSymbol("__refute_prime_sentinel__")
	return opened, nil
}
```

- [ ] **Step 2: Create `internal/backend/lsp/workspace_test.go`**

```go
package lsp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

func TestPrimeGoWorkspace_findsSymbolsAcrossPackages(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module example.com/multi\n\ngo 1.22\n",
		"main.go": `package main

import "example.com/multi/util"

func main() { println(util.Greet()) }
`,
		"util/helper.go": `package util

func Greet() string { return "hi" }
`,
		"vendor/ignored/x.go": `package ignored

func ShouldNotIndex() {}
`,
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	opened, err := client.PrimeGoWorkspace(dir)
	if err != nil {
		t.Fatalf("PrimeGoWorkspace: %v", err)
	}
	if opened < 2 {
		t.Errorf("expected to open main.go and util/helper.go, opened %d", opened)
	}

	syms, err := client.WorkspaceSymbol("Greet")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	found := false
	for _, s := range syms {
		if s.Name == "Greet" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Greet not found after priming. Results: %v", syms)
	}

	// Confirm vendored symbol was NOT indexed.
	vsyms, _ := client.WorkspaceSymbol("ShouldNotIndex")
	for _, s := range vsyms {
		if s.Name == "ShouldNotIndex" {
			t.Errorf("vendored symbol should not be indexed, but was: %+v", s)
		}
	}
}
```

- [ ] **Step 3: Run the new test**

```bash
go test ./internal/backend/lsp/... -run TestPrimeGoWorkspace -v -timeout 45s
```

Expected: passes (or skips). Indexing across multiple packages can be slow; 45s timeout is deliberate.

- [ ] **Step 4: Run full suite**

```bash
go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/backend/lsp/workspace.go internal/backend/lsp/workspace_test.go
git commit -m "feat: add PrimeGoWorkspace to index Go packages before workspace/symbol"
```

---

### Task 5: LSP Adapter — FindSymbol (Tier 1)

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

- [ ] **Step 1: Add `DidOpen`, `FindSymbol`, and helpers to `adapter.go`**

Add `"strings"`, `"unicode"` to the import block (alongside existing imports). Then add:

```go
// DidOpen exposes file-open notification for tests and Tier 1 priming.
func (a *Adapter) DidOpen(filePath string) error {
	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}
	return a.client.DidOpen(filePath, a.languageID)
}

// FindSymbol resolves a Tier 1 qualified name via workspace/symbol.
// Supported forms (for Go; other languages will add their own rules):
//
//	"Name"                    — symbol name only
//	"pkg.Name" or "Type.Name" — two-part: disambiguated by capitalization
//	                            of the first component (lowercase → package,
//	                            uppercase → type)
//	"pkg.Type.Name"           — three-part: container must match Type
//
// Returns ErrSymbolNotFound when nothing matches, or an ErrAmbiguous
// containing all candidates when multiple match.
func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	parts := parseQualifiedName(query.QualifiedName)
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return nil, fmt.Errorf("empty qualified name")
	}
	leaf := parts[len(parts)-1]

	syms, err := a.client.WorkspaceSymbol(leaf)
	if err != nil {
		return nil, err
	}

	var matches []symbol.Location
	for _, s := range syms {
		if s.Name != leaf {
			continue
		}
		if !qualifiedNameMatches(parts, s) {
			continue
		}
		if query.Kind != symbol.KindUnknown && lspKindToSymbolKind(s.Kind) != query.Kind {
			continue
		}
		matches = append(matches, symbol.Location{
			File:   uriToFile(s.Location.URI),
			Line:   s.Location.Range.Start.Line + 1,
			Column: s.Location.Range.Start.Character + 1,
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
func parseQualifiedName(name string) []string {
	if name == "" {
		return nil
	}
	return strings.Split(name, ".")
}

// qualifiedNameMatches returns true if the workspace symbol s matches the
// qualified-name parts. Matching rules documented on FindSymbol.
func qualifiedNameMatches(parts []string, s WorkspaceSymbolInfo) bool {
	switch len(parts) {
	case 1:
		// Bare name — any symbol with matching leaf passes.
		return true
	case 2:
		first := parts[0]
		if startsUppercase(first) {
			// Treat as Type.Method: container must equal the type name.
			return s.ContainerName == first
		}
		// Treat as pkg.Name: container must equal or end with "/pkg".
		return s.ContainerName == first ||
			strings.HasSuffix(s.ContainerName, "/"+first)
	case 3:
		// pkg.Type.Method: container must contain Type as its suffix segment.
		typeName := parts[1]
		// gopls uses "pkgpath.TypeName" or just "TypeName" depending on scope.
		return s.ContainerName == typeName ||
			strings.HasSuffix(s.ContainerName, "."+typeName) ||
			strings.HasSuffix(s.ContainerName, "/"+typeName)
	default:
		// 4+ parts: not a form we recognize. Reject.
		return false
	}
}

func startsUppercase(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	return unicode.IsUpper(r)
}

// lspKindToSymbolKind maps LSP SymbolKind integers to refute's SymbolKind.
// See https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#symbolKind
func lspKindToSymbolKind(lspKind int) symbol.SymbolKind {
	switch lspKind {
	case 5: // Class
		return symbol.KindClass
	case 6: // Method
		return symbol.KindMethod
	case 7: // Property — Go field
		return symbol.KindField
	case 8: // Field
		return symbol.KindField
	case 12: // Function
		return symbol.KindFunction
	case 13: // Variable
		return symbol.KindVariable
	case 14: // Constant
		return symbol.KindVariable
	case 22: // EnumMember
		return symbol.KindField
	case 23: // Struct
		return symbol.KindType
	case 26: // TypeParameter
		return symbol.KindType
	default:
		return symbol.KindUnknown
	}
}
```

Replace the existing `FindSymbol` stub (currently returns `ErrUnsupported`) with the new implementation above.

- [ ] **Step 2: Add adapter tests**

In `internal/backend/lsp/adapter_test.go`, add `"errors"` to the import block and append:

```go
func TestAdapter_FindSymbol_barename(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	if err := adapter.DidOpen(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	locs, err := adapter.FindSymbol(symbol.Query{
		QualifiedName: "oldName",
		Kind:          symbol.KindFunction,
	})
	if err != nil {
		t.Fatalf("FindSymbol: %v", err)
	}
	if len(locs) == 0 {
		t.Fatal("expected at least one location for oldName")
	}
	for _, l := range locs {
		if l.Name != "oldName" {
			t.Errorf("unexpected name in results: %s", l.Name)
		}
	}
}

func TestAdapter_FindSymbol_notFound(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	if err := adapter.DidOpen(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	_, err := adapter.FindSymbol(symbol.Query{QualifiedName: "doesNotExist"})
	if !errors.Is(err, backend.ErrSymbolNotFound) {
		t.Errorf("expected ErrSymbolNotFound, got: %v", err)
	}
}
```

Qualified-name disambiguation (`Type.Method` vs `pkg.Func`) is covered at the integration-test layer in Task 13, where real gopls output exercises the ContainerName matching paths. A unit-level table test would need to mock `WorkspaceSymbolInfo` values, which is lower value than the integration coverage.

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/backend/lsp/... -run "TestAdapter_FindSymbol" -v -timeout 45s
```

Expected: both tests pass (or skip).

- [ ] **Step 4: Run full suite**

```bash
go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement FindSymbol via workspace/symbol with Tier 1 disambiguation"
```

---

### Task 6: LSP Adapter — ExtractFunction, ExtractVariable (with `--name` honored)

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

`ExtractFunction(r, name)` now honors `name`: if non-empty, it text-replaces gopls's placeholder identifier (e.g., `newFunction`) across all edits in the returned WorkspaceEdit. Same for `ExtractVariable`.

- [ ] **Step 1: Implement ExtractFunction, ExtractVariable, and the placeholder rewriter**

In `internal/backend/lsp/adapter.go`, replace the existing `ExtractFunction` and `ExtractVariable` stubs with:

```go
func (a *Adapter) ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	we, placeholder, err := a.extractImpl(r, "function")
	if err != nil {
		return nil, err
	}
	if name != "" && placeholder != "" && placeholder != name {
		rewritePlaceholder(we, placeholder, name)
	}
	return we, nil
}

func (a *Adapter) ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	we, placeholder, err := a.extractImpl(r, "variable")
	if err != nil {
		return nil, err
	}
	if name != "" && placeholder != "" && placeholder != name {
		rewritePlaceholder(we, placeholder, name)
	}
	return we, nil
}

// extractImpl requests a refactor.extract code action from gopls, resolves it
// to a WorkspaceEdit, and returns the placeholder identifier gopls assigned
// to the new symbol. kind is "function" or "variable".
func (a *Adapter) extractImpl(r symbol.SourceRange, kind string) (*edit.WorkspaceEdit, string, error) {
	if a.client == nil {
		return nil, "", fmt.Errorf("adapter not initialized")
	}
	if err := a.client.DidOpen(r.File, a.languageID); err != nil {
		return nil, "", fmt.Errorf("DidOpen %s: %w", r.File, err)
	}
	actions, err := a.client.CodeActions(
		r.File,
		r.StartLine-1, r.StartCol-1,
		r.EndLine-1, r.EndCol-1,
		[]string{"refactor.extract"},
	)
	if err != nil {
		return nil, "", err
	}

	kindSuffix := "refactor.extract." + kind
	titleNeedle := kind

	for _, action := range actions {
		if !strings.HasPrefix(action.Kind, kindSuffix) &&
			!strings.Contains(strings.ToLower(action.Title), titleNeedle) {
			continue
		}
		we, err := a.resolveAction(action)
		if err != nil {
			return nil, "", err
		}
		placeholder := findExtractPlaceholder(we, kind)
		return we, placeholder, nil
	}
	return nil, "", backend.ErrUnsupported
}

// resolveAction returns the WorkspaceEdit for a code action, resolving it via
// codeAction/resolve if the action was returned without an inline Edit.
func (a *Adapter) resolveAction(action CodeAction) (*edit.WorkspaceEdit, error) {
	var fileEdits []edit.FileEdit
	var err error
	if action.Edit != nil {
		fileEdits, err = parseWorkspaceEdit(*action.Edit)
	} else {
		fileEdits, err = a.client.ResolveCodeActionEdit(action)
	}
	if err != nil {
		return nil, err
	}
	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

// findExtractPlaceholder locates the identifier gopls assigned to the
// extracted symbol by scanning the inserted text for "func <ID>(" (function
// extract) or "<ID> :=" (variable extract). Returns "" if not found — caller
// then skips placeholder rewriting.
func findExtractPlaceholder(we *edit.WorkspaceEdit, kind string) string {
	for _, fe := range we.FileEdits {
		for _, te := range fe.Edits {
			if te.NewText == "" {
				continue
			}
			var id string
			switch kind {
			case "function":
				id = matchIdentAfter(te.NewText, "func ")
			case "variable":
				id = matchIdentBefore(te.NewText, " :=")
				if id == "" {
					id = matchIdentBefore(te.NewText, ":=")
				}
			}
			if id != "" {
				return id
			}
		}
	}
	return ""
}

// matchIdentAfter returns the identifier that appears immediately after needle
// in s, or "" if none.
func matchIdentAfter(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	rest := s[i+len(needle):]
	end := 0
	for end < len(rest) && isIdentByte(rest[end]) {
		end++
	}
	if end == 0 {
		return ""
	}
	return rest[:end]
}

// matchIdentBefore returns the identifier that appears immediately before
// needle in s, or "" if none.
func matchIdentBefore(s, needle string) string {
	i := strings.Index(s, needle)
	if i <= 0 {
		return ""
	}
	start := i
	for start > 0 && isIdentByte(s[start-1]) {
		start--
	}
	if start == i {
		return ""
	}
	return s[start:i]
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

// rewritePlaceholder replaces every occurrence of old (as a whole identifier)
// with new in every edit's NewText. Whole-identifier means bounded by
// non-identifier bytes on both sides — "newFunction" inside "newFunctionCall"
// is NOT touched.
func rewritePlaceholder(we *edit.WorkspaceEdit, old, newID string) {
	for fi := range we.FileEdits {
		for ei := range we.FileEdits[fi].Edits {
			we.FileEdits[fi].Edits[ei].NewText = replaceWholeIdent(
				we.FileEdits[fi].Edits[ei].NewText, old, newID,
			)
		}
	}
}

func replaceWholeIdent(s, old, newID string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], old)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		j += i
		leftOK := j == 0 || !isIdentByte(s[j-1])
		rightIdx := j + len(old)
		rightOK := rightIdx >= len(s) || !isIdentByte(s[rightIdx])
		b.WriteString(s[i:j])
		if leftOK && rightOK {
			b.WriteString(newID)
		} else {
			b.WriteString(old)
		}
		i = rightIdx
	}
	return b.String()
}
```

- [ ] **Step 2: Add adapter tests**

Append to `internal/backend/lsp/adapter_test.go`:

```go
func TestAdapter_ExtractFunction_honorsName(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func main() {
	x := 1 + 2
	println(x)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	r := symbol.SourceRange{
		File:      filepath.Join(dir, "main.go"),
		StartLine: 4, StartCol: 7,
		EndLine: 4, EndCol: 12,
	}
	we, err := adapter.ExtractFunction(r, "sum")
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits")
	}
	// At least one NewText should contain our chosen name.
	saw := false
	for _, fe := range we.FileEdits {
		for _, te := range fe.Edits {
			if strings.Contains(te.NewText, "sum") {
				saw = true
			}
		}
	}
	if !saw {
		t.Errorf("expected extracted name 'sum' to appear in edits, got: %+v", we)
	}
}

func TestAdapter_ExtractVariable(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func main() {
	println(1 + 2)
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	r := symbol.SourceRange{
		File:      filepath.Join(dir, "main.go"),
		StartLine: 4, StartCol: 10,
		EndLine: 4, EndCol: 15,
	}
	we, err := adapter.ExtractVariable(r, "")
	if err != nil {
		t.Fatalf("ExtractVariable: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits")
	}
}

func TestReplaceWholeIdent_respectsIdentifierBoundaries(t *testing.T) {
	got := lsp.ReplaceWholeIdentForTest("newFunction()\nnewFunctionCall()\n_ = newFunction", "newFunction", "sum")
	want := "sum()\nnewFunctionCall()\n_ = sum"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
```

The last test requires a test-only export. Add to `internal/backend/lsp/adapter.go`:

```go
// ReplaceWholeIdentForTest is a testing hook for rewritePlaceholder's internal
// helper. Not for production use.
func ReplaceWholeIdentForTest(s, old, newID string) string {
	return replaceWholeIdent(s, old, newID)
}
```

- [ ] **Step 3: Run the new tests**

```bash
go test ./internal/backend/lsp/... -run "TestAdapter_Extract|TestReplaceWholeIdent" -v -timeout 45s
```

Expected: three tests pass (or skip for the gopls-dependent ones).

- [ ] **Step 4: Run full suite**

```bash
go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement ExtractFunction, ExtractVariable; honor --name via placeholder rewrite"
```

---

### Task 7: LSP Adapter — InlineSymbol with identifier-width range

**Files:**
- Modify: `internal/backend/lsp/adapter.go`
- Modify: `internal/backend/lsp/adapter_test.go`

- [ ] **Step 1: Replace the InlineSymbol stub**

```go
func (a *Adapter) InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, fmt.Errorf("DidOpen %s: %w", loc.File, err)
	}

	// Use the identifier-width range so gopls recognizes the cursor is on a
	// renameable/inlineable symbol. A zero-width range often returns no
	// actions.
	startLine := loc.Line - 1
	startChar := loc.Column - 1
	endChar := startChar + max(len(loc.Name), 1)

	actions, err := a.client.CodeActions(
		loc.File,
		startLine, startChar, startLine, endChar,
		[]string{"refactor.inline"},
	)
	if err != nil {
		return nil, err
	}
	for _, action := range actions {
		if strings.HasPrefix(action.Kind, "refactor.inline") ||
			strings.Contains(strings.ToLower(action.Title), "inline") {
			return a.resolveAction(action)
		}
	}
	return nil, backend.ErrUnsupported
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

- [ ] **Step 2: Add test**

Append to `internal/backend/lsp/adapter_test.go`:

```go
func TestAdapter_InlineSymbol(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func add(a, b int) int { return a + b }

func main() {
	println(add(1, 2))
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	cfg := config.ServerConfig{Command: "gopls", Args: []string{"serve"}}
	adapter := lsp.NewAdapter(cfg, "go", []string{"*.go"})
	if err := adapter.Initialize(dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	defer adapter.Shutdown()

	loc := symbol.Location{
		File:   filepath.Join(dir, "main.go"),
		Line:   6, Column: 10,
		Name: "add",
		Kind: symbol.KindFunction,
	}
	we, err := adapter.InlineSymbol(loc)
	if err != nil {
		if errors.Is(err, backend.ErrUnsupported) {
			t.Skip("inline not supported by this gopls version")
		}
		t.Fatalf("InlineSymbol: %v", err)
	}
	if len(we.FileEdits) == 0 {
		t.Fatal("expected file edits from inline")
	}
}
```

- [ ] **Step 3: Run and commit**

```bash
go test ./internal/backend/lsp/... -run TestAdapter_InlineSymbol -v -timeout 45s
go test ./... -timeout 90s
git add internal/backend/lsp/adapter.go internal/backend/lsp/adapter_test.go
git commit -m "feat: implement InlineSymbol with identifier-width range"
```

---

### Task 8: JSON output for the edit package

**Files:**
- Create: `internal/edit/json.go`
- Create: `internal/edit/json_test.go`

Output shape defined in the Output Contract section at the top of this plan.

- [ ] **Step 1: Create `internal/edit/json.go`**

```go
package edit

import (
	"encoding/json"
)

// JSONChange is a single text edit in JSON output. All positions are 1-indexed.
type JSONChange struct {
	StartLine int    `json:"startLine"`
	StartCol  int    `json:"startCol"`
	EndLine   int    `json:"endLine"`
	EndCol    int    `json:"endCol"`
	NewText   string `json:"newText"`
}

// JSONFileEdit groups changes by file.
type JSONFileEdit struct {
	File    string       `json:"file"`
	Changes []JSONChange `json:"changes"`
}

// JSONSymbolLoc is a 1-indexed symbol location used for the newSymbol field.
type JSONSymbolLoc struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Name   string `json:"name"`
}

// JSONResult is the full refute JSON output envelope.
type JSONResult struct {
	Status        string          `json:"status"` // "applied" | "dry-run" | "no-op" | "ambiguous"
	FilesModified int             `json:"filesModified"`
	Edits         []JSONFileEdit  `json:"edits,omitempty"`
	NewSymbol     *JSONSymbolLoc  `json:"newSymbol,omitempty"`
	Candidates    []JSONSymbolLoc `json:"candidates,omitempty"`
}

// RenderJSON converts a WorkspaceEdit into the JSONResult envelope with the
// given status. Positions are converted from LSP's 0-indexed to refute's
// 1-indexed convention.
func RenderJSON(we *WorkspaceEdit, status string) *JSONResult {
	res := &JSONResult{Status: status}
	if we == nil {
		return res
	}
	for _, fe := range we.FileEdits {
		if len(fe.Edits) == 0 {
			continue
		}
		jfe := JSONFileEdit{File: fe.Path}
		for _, te := range fe.Edits {
			jfe.Changes = append(jfe.Changes, JSONChange{
				StartLine: te.Range.Start.Line + 1,
				StartCol:  te.Range.Start.Character + 1,
				EndLine:   te.Range.End.Line + 1,
				EndCol:    te.Range.End.Character + 1,
				NewText:   te.NewText,
			})
		}
		res.Edits = append(res.Edits, jfe)
	}
	res.FilesModified = len(res.Edits)
	return res
}

// Marshal returns indented JSON suitable for stdout.
func (r *JSONResult) Marshal() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
```

- [ ] **Step 2: Create `internal/edit/json_test.go`**

```go
package edit_test

import (
	"encoding/json"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
)

func TestRenderJSON_convertsIndexing(t *testing.T) {
	we := &edit.WorkspaceEdit{
		FileEdits: []edit.FileEdit{
			{
				Path: "/tmp/a.go",
				Edits: []edit.TextEdit{
					{
						Range: edit.Range{
							Start: edit.Position{Line: 2, Character: 4},
							End:   edit.Position{Line: 2, Character: 11},
						},
						NewText: "newName",
					},
				},
			},
		},
	}
	res := edit.RenderJSON(we, "applied")

	if res.Status != "applied" {
		t.Errorf("status = %q, want applied", res.Status)
	}
	if res.FilesModified != 1 {
		t.Errorf("filesModified = %d, want 1", res.FilesModified)
	}
	if len(res.Edits) != 1 || len(res.Edits[0].Changes) != 1 {
		t.Fatalf("expected 1 edit with 1 change, got %+v", res.Edits)
	}
	c := res.Edits[0].Changes[0]
	if c.StartLine != 3 || c.StartCol != 5 || c.EndLine != 3 || c.EndCol != 12 {
		t.Errorf("position conversion wrong: %+v", c)
	}

	data, err := res.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundtrip edit.JSONResult
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("roundtrip unmarshal: %v", err)
	}
	if roundtrip.Status != "applied" {
		t.Errorf("roundtrip status = %q", roundtrip.Status)
	}
}

func TestRenderJSON_nilAndEmpty(t *testing.T) {
	if res := edit.RenderJSON(nil, "no-op"); res.FilesModified != 0 || len(res.Edits) != 0 {
		t.Errorf("nil input should produce empty result, got %+v", res)
	}
	if res := edit.RenderJSON(&edit.WorkspaceEdit{}, "no-op"); res.FilesModified != 0 {
		t.Errorf("empty WorkspaceEdit should produce 0 filesModified, got %d", res.FilesModified)
	}
}
```

- [ ] **Step 3: Run and commit**

```bash
go test ./internal/edit/... -run "TestRenderJSON" -v
go test ./... -timeout 90s
git add internal/edit/json.go internal/edit/json_test.go
git commit -m "feat: add JSON output rendering for WorkspaceEdit"
```

---

### Task 9: CLI — rename.go refactor (no behavior change)

**Files:**
- Modify: `internal/cli/rename.go`

Pure refactor — extract `buildAdapter`, `finishRename`, `applyOrPreview` helpers without adding any new behavior. Task 10 adds Tier 1 on top.

- [ ] **Step 1: Rewrite `internal/cli/rename.go`**

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
	flagJSON    bool
)

func addRenameFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	cmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed, optional)")
	cmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	cmd.Flags().StringVar(&flagNewName, "new-name", "", "new name for the symbol")
	cmd.Flags().StringVar(&flagSymbol, "symbol", "", "qualified symbol name (e.g., pkg.Func or Type.Method)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	_ = cmd.MarkFlagRequired("new-name")
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
		Short: "Rename a symbol (kind-agnostic)",
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

	if query.File != "" {
		abs, err := filepath.Abs(query.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query.File = abs
	}

	// Tier 1 handled separately (Task 10) — stub here until then.
	if query.Tier() == 1 {
		return fmt.Errorf("tier 1 rename not yet wired in; this task is pure refactor")
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

// buildAdapter creates and initializes an LSP adapter for the given file.
func buildAdapter(filePath string) (*lsp.Adapter, string, error) {
	workspaceRoot, err := FindWorkspaceRootFromFile(filePath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	language := DetectLanguage(filePath)
	if language == "" {
		return nil, "", fmt.Errorf("unsupported file type for %s", filePath)
	}
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

// finishRename requests the rename edit and routes it through the output pipeline.
func finishRename(adapter *lsp.Adapter, workspaceRoot string, loc symbol.Location, newName string) error {
	we, err := adapter.Rename(loc, newName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(we, workspaceRoot)
}

// applyOrPreview emits the result per --dry-run/--json/default flags.
func applyOrPreview(we *edit.WorkspaceEdit, workspaceRoot string) error {
	if flagJSON {
		return emitJSON(we, statusForFlags())
	}
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
		if diff, err := edit.RenderDiff(we); err == nil && diff != "" {
			fmt.Print(diff)
		}
	}
	return nil
}

func statusForFlags() string {
	if flagDryRun {
		return "dry-run"
	}
	return "applied"
}

func emitJSON(we *edit.WorkspaceEdit, status string) error {
	res := edit.RenderJSON(we, status)
	if !flagDryRun {
		if _, err := edit.Apply(we); err != nil {
			return fmt.Errorf("applying edits: %w", err)
		}
	}
	data, err := res.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
```

- [ ] **Step 2: Verify build and run existing tests**

```bash
go build ./... && go test ./... -timeout 90s
```

Expected: all existing tests still pass. The Tier 1 stub error is unreachable through existing tests.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/rename.go
git commit -m "refactor: extract buildAdapter, finishRename, applyOrPreview helpers from runRename"
```

---

### Task 10: CLI — Tier 1 rename path

**Files:**
- Modify: `internal/cli/rename.go`

- [ ] **Step 1: Add Tier 1 handling**

Replace the Tier 1 stub block in `runRename`:

```go
// Was:
//   if query.Tier() == 1 {
//       return fmt.Errorf("tier 1 rename not yet wired in; this task is pure refactor")
//   }
if query.Tier() == 1 {
	return runRenameTier1(query)
}
```

Then add `runRenameTier1`:

```go
func runRenameTier1(query symbol.Query) error {
	workspaceRoot, err := tier1WorkspaceRoot()
	if err != nil {
		return err
	}

	language := "go"
	if query.File != "" {
		language = DetectLanguage(query.File)
	}
	if language == "" {
		language = "go" // fallback for naked --symbol without --file
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

	// Prime so workspace/symbol sees the whole module.
	if _, err := adapter.PrimeWorkspace(workspaceRoot); err != nil {
		return fmt.Errorf("priming workspace: %w", err)
	}

	locs, err := adapter.FindSymbol(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}
	if len(locs) > 1 {
		return ambiguousError(locs)
	}
	return finishRename(adapter, workspaceRoot, locs[0], flagNewName)
}

// tier1WorkspaceRoot resolves the workspace root for a Tier 1 query.
// If --file is provided, walk up from it; otherwise walk up from cwd.
func tier1WorkspaceRoot() (string, error) {
	if flagFile != "" {
		abs, err := filepath.Abs(flagFile)
		if err != nil {
			return "", err
		}
		return FindWorkspaceRootFromDir(filepath.Dir(abs))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return FindWorkspaceRootFromDir(cwd)
}

// ambiguousError formats a Tier 1 ambiguity result. In JSON mode, emit a
// structured candidates list; otherwise print a human-readable message.
func ambiguousError(locs []symbol.Location) error {
	if flagJSON {
		res := &edit.JSONResult{Status: "ambiguous"}
		for _, l := range locs {
			res.Candidates = append(res.Candidates, edit.JSONSymbolLoc{
				File:   l.File,
				Line:   l.Line,
				Column: l.Column,
				Name:   l.Name,
			})
		}
		data, _ := res.Marshal()
		fmt.Println(string(data))
		return &ExitCodeError{Code: 1}
	}
	var msg string
	msg = "Ambiguous — multiple candidates:\n"
	for _, l := range locs {
		msg += fmt.Sprintf("  %s:%d:%d  %s\n", l.File, l.Line, l.Column, l.Name)
	}
	msg += "Use --file and --line to narrow the selection."
	return &ExitCodeError{Code: 1, Message: msg}
}
```

- [ ] **Step 2: Add `PrimeWorkspace` to `adapter.go`**

Append to `internal/backend/lsp/adapter.go`:

```go
// PrimeWorkspace delegates to the client's PrimeGoWorkspace when the
// language is Go. For other languages, this is currently a no-op — each
// adapter will grow its own priming strategy.
func (a *Adapter) PrimeWorkspace(workspaceRoot string) (int, error) {
	if a.client == nil {
		return 0, fmt.Errorf("adapter not initialized")
	}
	if a.languageID == "go" {
		return a.client.PrimeGoWorkspace(workspaceRoot)
	}
	return 0, nil
}
```

- [ ] **Step 3: Verify build and existing tests**

```bash
go build ./... && go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 4: Manual smoke test**

```bash
mkdir -p /tmp/refute-v2-smoke && cp -r testdata/fixtures/go/rename/* /tmp/refute-v2-smoke/
go build -o /tmp/refute-bin ./cmd/refute
cd /tmp/refute-v2-smoke && /tmp/refute-bin rename-function --symbol "FormatGreeting" --new-name "BuildGreeting" --dry-run
```

Expected: unified diff showing the rename across `util/helper.go` and `main.go`. Clean up: `rm -rf /tmp/refute-v2-smoke /tmp/refute-bin`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/rename.go internal/backend/lsp/adapter.go
git commit -m "feat: wire Tier 1 rename path with workspace priming and ambiguity handling"
```

---

### Task 11: CLI — extract-function, extract-variable

**Files:**
- Create: `internal/cli/extract.go`

- [ ] **Step 1: Create `internal/cli/extract.go`**

```go
package cli

import (
	"fmt"
	"path/filepath"

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
	cmd.Flags().IntVar(&flagStartLine, "start-line", 0, "start line (1-indexed)")
	cmd.Flags().IntVar(&flagStartCol, "start-col", 0, "start column (1-indexed)")
	cmd.Flags().IntVar(&flagEndLine, "end-line", 0, "end line (1-indexed)")
	cmd.Flags().IntVar(&flagEndCol, "end-col", 0, "end column (1-indexed)")
	cmd.Flags().StringVar(&flagExtName, "name", "", "name for the extracted symbol (optional; gopls default used if empty)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	for _, f := range []string{"file", "start-line", "start-col", "end-line", "end-col"} {
		_ = cmd.MarkFlagRequired(f)
	}
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
	absFile, err := filepath.Abs(flagFile)
	if err != nil {
		return fmt.Errorf("resolving file path: %w", err)
	}
	adapter, workspaceRoot, err := buildAdapter(absFile)
	if err != nil {
		return err
	}
	defer adapter.Shutdown()

	r := symbol.SourceRange{
		File:      absFile,
		StartLine: flagStartLine,
		StartCol:  flagStartCol,
		EndLine:   flagEndLine,
		EndCol:    flagEndCol,
	}

	switch kind {
	case "function":
		result, err := adapter.ExtractFunction(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-function failed: %w", err)
		}
		if len(result.FileEdits) == 0 {
			return NoEditsError()
		}
		return applyOrPreview(result, workspaceRoot)
	case "variable":
		result, err := adapter.ExtractVariable(r, flagExtName)
		if err != nil {
			return fmt.Errorf("extract-variable failed: %w", err)
		}
		if len(result.FileEdits) == 0 {
			return NoEditsError()
		}
		return applyOrPreview(result, workspaceRoot)
	default:
		return fmt.Errorf("unknown extract kind %q", kind)
	}
}
```

Add `"path/filepath"` to the import block.

- [ ] **Step 2: Build and smoke-test help**

```bash
go build ./...
go build -o /tmp/refute-bin ./cmd/refute && /tmp/refute-bin --help 2>&1 | grep -E "extract-(function|variable)"
rm /tmp/refute-bin
```

Expected: both commands listed.

- [ ] **Step 3: Full test suite**

```bash
go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/extract.go
git commit -m "feat: add extract-function and extract-variable CLI commands"
```

---

### Task 12: CLI — inline command

**Files:**
- Create: `internal/cli/inline.go`

- [ ] **Step 1: Create `internal/cli/inline.go`**

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
	inlineCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	_ = inlineCmd.MarkFlagRequired("file")
	_ = inlineCmd.MarkFlagRequired("line")

	RootCmd.AddCommand(inlineCmd)
}

func runInline() error {
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
		return NoEditsError()
	}
	return applyOrPreview(we, workspaceRoot)
}
```

Note: the v1 plan had `fmt.Fprintln(flagFile, ...)` which doesn't compile (`flagFile` is a string, not an `io.Writer`). This version uses `NoEditsError()` for the empty-result case, consistent with rename.

- [ ] **Step 2: Build and test**

```bash
go build ./... && go test ./... -timeout 90s
go build -o /tmp/refute-bin ./cmd/refute && /tmp/refute-bin --help 2>&1 | grep "^  inline"
rm /tmp/refute-bin
```

Expected: `inline` listed in help.

- [ ] **Step 3: Commit**

```bash
git add internal/cli/inline.go
git commit -m "feat: add inline CLI command"
```

---

### Task 13: End-to-end integration tests (happy and error paths)

**Files:**
- Modify: `testdata/fixtures/go/rename/main.go` (add extractable expression)
- Modify: `internal/integration_test.go`

- [ ] **Step 1: Update the fixture**

Replace the contents of `testdata/fixtures/go/rename/main.go` with:

```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	result := 6*7 + 1
	println(msg, result)
}
```

- [ ] **Step 2: Verify the fixture compiles**

```bash
cd /home/ketan/project/refute/testdata/fixtures/go/rename && go build ./... && cd /home/ketan/project/refute
```

Expected: clean build.

- [ ] **Step 3: Add integration tests**

Append to `internal/integration_test.go` (after the existing `copyDir`):

```go
func buildRefute(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	build := exec.Command("go", "build", "-o", bin, "./cmd/refute")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build refute: %s\n%s", err, out)
	}
	return bin
}

func TestEndToEnd_ExtractFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	mainFile := filepath.Join(dir, "main.go")

	// Line 7: "\tresult := 6*7 + 1"
	// "6*7 + 1" starts at col 12 (tab=1, "result := "=10 chars).
	cmd := exec.Command(refuteBin,
		"extract-function",
		"--file", mainFile,
		"--start-line", "7",
		"--start-col", "12",
		"--end-line", "7",
		"--end-col", "19",
		"--name", "computeResult",
	)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extract-function: %s\n%s", err, out)
	}

	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after extract:\n%s", out)
	}

	mainContent, _ := os.ReadFile(mainFile)
	if !strings.Contains(string(mainContent), "computeResult") {
		t.Errorf("expected 'computeResult' in main.go after extract, got:\n%s", mainContent)
	}
}

func TestEndToEnd_Tier1Rename(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tier 1 rename: %s\n%s", err, out)
	}

	helperContent, _ := os.ReadFile(filepath.Join(dir, "util", "helper.go"))
	if strings.Contains(string(helperContent), "FormatGreeting") {
		t.Error("helper.go still contains FormatGreeting")
	}
	mainContent, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if strings.Contains(string(mainContent), "FormatGreeting") {
		t.Error("main.go still contains FormatGreeting")
	}

	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after rename:\n%s", out)
	}
}

func TestEndToEnd_Tier1NotFound(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "DoesNotExistAnywhere",
		"--new-name", "StillDoesNotExist",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for nonexistent symbol, got success:\n%s", out)
	}
	if !strings.Contains(string(out), "symbol not found") {
		t.Errorf("expected 'symbol not found' in output, got:\n%s", out)
	}
}

func TestEndToEnd_JSONOutput(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--json",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rename --json --dry-run: %s\n%s", err, out)
	}
	var parsed struct {
		Status        string `json:"status"`
		FilesModified int    `json:"filesModified"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("parsing JSON: %v\nraw:\n%s", err, out)
	}
	if parsed.Status != "dry-run" {
		t.Errorf("status = %q, want dry-run", parsed.Status)
	}
	if parsed.FilesModified < 2 {
		t.Errorf("filesModified = %d, want >= 2 (helper.go + main.go)", parsed.FilesModified)
	}
}
```

Also add `"encoding/json"` to the import block.

- [ ] **Step 4: Run integration tests**

```bash
go test -tags integration ./internal/ -v -timeout 180s
```

Expected: all integration tests pass (or skip when gopls is missing). The `Tier1Rename` test may be slow on first run — gopls is indexing.

- [ ] **Step 5: Run full suite**

```bash
go test ./... -timeout 90s
```

Expected: all unit tests pass.

- [ ] **Step 6: Commit**

```bash
git add testdata/fixtures/go/rename/main.go internal/integration_test.go
git commit -m "feat: add E2E tests for extract, Tier 1 rename, not-found, JSON output"
```

---

### Task 14: Document position encoding constraint

**Files:**
- Create: `docs/position-encoding.md`

- [ ] **Step 1: Create the doc**

```markdown
# Position Encoding

`refute` uses **1-indexed byte-offset columns** in its CLI and JSON output.
LSP servers use **0-indexed UTF-16 code-unit columns**.

For ASCII source (the vast majority of code), these are identical after the
1→0 conversion. For source containing multi-byte Unicode (non-ASCII identifiers,
emoji in string literals, etc.) the two encodings diverge, which means a
`refute` column computed from a byte count will NOT match the LSP column.

## Current behavior

The conversion in `internal/backend/lsp/adapter.go` is a straight subtract-1.
This is correct for ASCII only.

## If you need Unicode

Convert at the LSP boundary. Read the source line as a string, slice to the
1-indexed byte offset, count UTF-16 code units via `utf16.Encode` (or the
smaller-memory `utf8.DecodeRuneInString` + UTF-16 counter). Send that count
as the LSP character. Reverse on the way back.

This is deferred to a follow-up. File an issue and reference this document
if you hit a Unicode case.
```

- [ ] **Step 2: Commit**

```bash
git add docs/position-encoding.md
git commit -m "docs: note the byte vs UTF-16 column-encoding gap"
```

---

## Verification Checklist

Run this at the end to confirm the plan landed:

```bash
# Build
go build -o /tmp/refute-bin ./cmd/refute

# New commands present
/tmp/refute-bin --help 2>&1 | grep -E "^  (extract-function|extract-variable|inline|rename)"
# Expected: all four listed

# JSON output works
mkdir -p /tmp/refute-v2-test && cp -r testdata/fixtures/go/rename/* /tmp/refute-v2-test/
/tmp/refute-bin rename-function \
  --symbol "FormatGreeting" \
  --file /tmp/refute-v2-test/util/helper.go \
  --new-name "BuildGreeting" \
  --dry-run --json | head -5
# Expected: valid JSON with "status": "dry-run"

# Tier 1 rename (with priming, no --file needed)
cd /tmp/refute-v2-test && /tmp/refute-bin rename-function \
  --symbol "FormatGreeting" \
  --new-name "BuildGreeting" \
  --dry-run
cd -
# Expected: diff across helper.go and main.go

# Extract function with name
/tmp/refute-bin extract-function \
  --file /tmp/refute-v2-test/main.go \
  --start-line 7 --start-col 12 \
  --end-line 7 --end-col 19 \
  --name computeResult \
  --dry-run
# Expected: diff introducing func computeResult

# All unit tests
go test ./... -timeout 90s
# Expected: all pass

# Integration tests
go test -tags integration ./internal/ -timeout 180s
# Expected: 6 tests pass (2 existing + 4 new)

# Clean up
rm -rf /tmp/refute-bin /tmp/refute-v2-test
```

---

## Deferred

- **Chained rename after extract:** placeholder rewriting handles the common case. If gopls's extracted-symbol name conflicts with something in scope, a proper rename post-extract is safer. Track for Plan 3.
- **`didClose` matching `didOpen`:** not needed for CLI one-shot mode; will be needed for daemon mode (Plan 4) to bound gopls memory.
- **Gopls indexing wait:** after `PrimeGoWorkspace`, gopls indexes in the background. A follow-up can subscribe to `$/progress` to wait for `initialLoad` completion, removing first-call races on very large repos.
- **Unicode-safe column conversion:** documented in `docs/position-encoding.md`; deferred until a Unicode case is reported.
- **Request timeouts:** `Client.request` blocks indefinitely on gopls. Add a per-request timeout (configurable via config) in Plan 3.
- **Non-Go priming:** `PrimeWorkspace` is a no-op for languages other than Go. ts-morph/rope will get their own priming strategies in Plan 3.
- **TypeScript and Python backends:** Plan 3.
- **Daemon mode + MCP server:** Plan 4 (now much simpler because `--json` is already the transport shape).
- **`refute move`**: `MoveToFile` adapter implementation — deferred.
- **`refute pattern`**: ast-grep integration — deferred.
