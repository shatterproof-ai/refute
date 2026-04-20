# Expedition Plan — TypeScript Support

**Expedition:** `typescript-support`
**Base branch:** `typescript-support`
**Base worktree:** `/home/ketan/project/refute-typescript-support`
**Primary branch:** `main`

## Goal

Enable `refute rename` (and rename-function, rename-class, etc.) to work on
TypeScript (`.ts`, `.tsx`) and JavaScript (`.js`, `.jsx`) files using
`typescript-language-server`, with the same cross-file accuracy the Go backend
provides via `gopls`.

## Success Criteria

1. `refute rename-function --file src/greeter.ts --line 3 --name greet --new-name welcome` renames `greet` in all files of a TypeScript workspace.
2. Integration tests pass against real `typescript-language-server` (skipped gracefully when the server is absent from PATH).
3. `go build ./...` and `go test ./...` continue to pass with no regressions.

## Background

The following already exists (no changes needed):

- `config.go` — `builtinServers` already has `"typescript": {Command: "typescript-language-server", Args: ["--stdio"]}`.
- `cli/rename.go` — `detectLanguage` already maps `.ts`/`.tsx` → `"typescript"` and `.js`/`.jsx` → `"javascript"`. `findWorkspaceRoot` already checks `tsconfig.json` and `package.json`.
- `lsp/adapter.go` — the generic `Adapter` is language-agnostic.

Two bugs block TypeScript from working end-to-end:

1. `detectLanguage` returns `"typescript"` for both `.ts` and `.tsx`, but the correct LSP language ID for `.tsx` is `"typescriptreact"`. Similarly `.jsx` → `"javascriptreact"`. The same string is used for both server-config lookup and as the LSP language ID — these must be separated.
2. `detectLanguage` returns `"javascript"` for `.js`/`.jsx` but there is no `"javascript"` entry in `builtinServers`, so `cfg.Server("javascript")` returns an empty `ServerConfig` and the command errors.

Additionally, `typescript-language-server` — like `gopls` — must open at least one file before rename requests work reliably. Without priming, rename returns empty edits.

## Task Sequence

### Task 1 — Language ID and server key separation

**Branch slug:** `typescript-support-01-lang-id`

**Goal:** Fix the two bugs above so the rename pipeline correctly handles all four TypeScript-family extensions.

**Files:**
- `internal/cli/rename.go`
- `internal/config/config.go`

**Changes:**

In `config.go`, add `"javascript"` to `builtinServers` pointing to the same server:
```go
"javascript": {
    Command: "typescript-language-server",
    Args:    []string{"--stdio"},
},
```

In `rename.go`, rename `detectLanguage` → `detectServerKey` (same body). Add `detectLanguageID`:
```go
func detectLanguageID(filePath string) string {
    switch filepath.Ext(filePath) {
    case ".ts":  return "typescript"
    case ".tsx": return "typescriptreact"
    case ".js":  return "javascript"
    case ".jsx": return "javascriptreact"
    case ".go":  return "go"
    case ".py":  return "python"
    case ".java": return "java"
    case ".kt":  return "kotlin"
    case ".rs":  return "rust"
    case ".cs":  return "csharp"
    default:     return ""
    }
}
```

In `runRename`, replace:
```go
language := detectLanguage(loc.File)
serverCfg := cfg.Server(language)
adapter := lsp.NewAdapter(serverCfg, language, nil)
```
with:
```go
serverKey := detectServerKey(loc.File)
serverCfg := cfg.Server(serverKey)
languageID := detectLanguageID(loc.File)
adapter := lsp.NewAdapter(serverCfg, languageID, nil)
```

**Verification:** `go build ./... && go test ./... -timeout 90s`

**Commit:** `fix: separate LSP language ID from server config key`

---

### Task 2 — TypeScript workspace priming

**Branch slug:** `typescript-support-02-ts-priming`

**Goal:** Add `PrimeTSWorkspace` so `typescript-language-server` initialises
its project graph before rename requests arrive.

**Files:**
- `internal/backend/lsp/priming.go` (new)
- `internal/backend/lsp/adapter.go` (call priming from `Initialize`)

**`priming.go`:**
```go
package lsp

import (
    "io/fs"
    "path/filepath"
    "strings"
)

const maxPrimedFiles = 10

// PrimeTSWorkspace opens up to maxPrimedFiles TypeScript source files so
// typescript-language-server initialises its project graph before the first
// rename request arrives.
func PrimeTSWorkspace(client *Client, root string) error {
    var opened int
    return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
                return filepath.SkipDir
            }
            return nil
        }
        if opened >= maxPrimedFiles {
            return filepath.SkipAll
        }
        ext := strings.ToLower(filepath.Ext(path))
        langID := ""
        switch ext {
        case ".ts":
            langID = "typescript"
        case ".tsx":
            langID = "typescriptreact"
        }
        if langID != "" {
            if openErr := client.DidOpen(path, langID); openErr == nil {
                opened++
            }
        }
        return nil
    })
}
```

**In `adapter.go`**, after starting the client in `Initialize`, add:
```go
if isTSFamily(a.languageID) {
    _ = PrimeTSWorkspace(a.client, absRoot) // non-fatal; priming failure is logged but not returned
}
```

Add helper (can be a package-level unexported func in `adapter.go` or `priming.go`):
```go
func isTSFamily(id string) bool {
    switch id {
    case "typescript", "typescriptreact", "javascript", "javascriptreact":
        return true
    }
    return false
}
```

**Verification:** `go build ./... && go test ./... -timeout 90s`

**Commit:** `feat: add TypeScript workspace priming`

---

### Task 3 — TypeScript test fixtures

**Branch slug:** `typescript-support-03-ts-fixtures`

**Goal:** Create a minimal TypeScript workspace that mirrors the Go rename
fixture — a function defined in one file, imported and called in another.

**Files to create:**
- `testdata/fixtures/typescript/rename/package.json`
- `testdata/fixtures/typescript/rename/tsconfig.json`
- `testdata/fixtures/typescript/rename/src/greeter.ts`
- `testdata/fixtures/typescript/rename/src/main.ts`

`package.json`:
```json
{
  "name": "rename-fixture",
  "version": "1.0.0",
  "private": true
}
```

`tsconfig.json`:
```json
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "strict": true,
    "outDir": "dist"
  },
  "include": ["src"]
}
```

`src/greeter.ts` (note: `greet` is on line 1, column 17 — used by the test):
```typescript
export function greet(name: string): string {
    return `Hello, ${name}!`;
}
```

`src/main.ts`:
```typescript
import { greet } from "./greeter";

const message = greet("world");
console.log(message);
```

**Verification:** `go build ./... && go test ./... -timeout 90s`

**Commit:** `test: add TypeScript rename fixture`

---

### Task 4 — End-to-end integration tests for TypeScript rename

**Branch slug:** `typescript-support-04-ts-e2e`

**Goal:** Add integration tests that verify TypeScript rename works end-to-end
through `typescript-language-server`.

**File:** `internal/integration_test.go`

Add `TestEndToEnd_RenameTypeScriptFunction` and `TestEndToEnd_TypeScriptDryRun`
following the same pattern as the existing Go integration tests.
Both skip gracefully with `t.Skip` when `typescript-language-server` is not
on PATH.

`TestEndToEnd_RenameTypeScriptFunction`:
- Copies `testdata/fixtures/typescript/rename` to a temp dir.
- Builds `refute` binary.
- Runs `refute rename-function --file src/greeter.ts --line 1 --name greet --new-name welcome`.
- Asserts: `greeter.ts` no longer contains `greet`, now contains `welcome`.
- Asserts: `main.ts` no longer contains `greet`, now contains `welcome` (cross-file rename).

`TestEndToEnd_TypeScriptDryRun`:
- Same setup.
- Runs with `--dry-run`.
- Asserts: stdout contains both `greet` and `welcome` (diff).
- Asserts: `greeter.ts` is unchanged on disk.

**Verification:**
```bash
go test -tags integration ./internal/... -timeout 120s -run TypeScript
go test ./... -timeout 90s
```

**Commit:** `test: add TypeScript end-to-end integration tests`

---

## Experiment Register

No planned experiments at expedition start.

## Verification Gates

Per-task gate (run after every merge into the base branch):
```bash
go build ./...
go test ./... -timeout 90s
```

Final gate before landing (run from the base worktree after Task 4 merges):
```bash
go build ./...
go test ./... -timeout 90s
go test -tags integration ./internal/... -timeout 120s -run TypeScript
go test -tags integration ./internal/... -timeout 120s -run EndToEnd
```
