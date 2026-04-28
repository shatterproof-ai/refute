# Expedition Plan — Go Code Actions + Tier 1

**Expedition:** `go-code-actions`
**Base branch:** `go-code-actions`
**Base worktree:** `/home/ketan/project/refute-go-code-actions`
**Primary branch:** `main`
**Source plan:** `docs/plans/2026-04-17-go-code-actions-tier1-v2.md`

## Goal

Extend the Go LSP backend with extract-function, extract-variable, and inline
via gopls code actions, and implement Tier 1 qualified-name symbol resolution
(`--symbol "pkg.FunctionName"`) via `workspace/symbol` — with workspace
priming, disambiguation, and `--json` output for MCP consumers.

## Success Criteria

1. `refute rename --symbol "util.FormatGreeting" --new-name BuildGreeting` resolves the symbol via workspace/symbol and renames it without requiring `--file`/`--line`.
2. `refute extract-function --file ... --start-line ... --end-line ... --name sum` extracts a function via gopls code actions.
3. `refute extract-variable --file ... --line ... --start-col ... --end-col ... --name x` extracts a variable.
4. `refute inline --file ... --line ... --col ...` inlines a symbol.
5. All commands support `--json` for structured output (MCP-consumable).
6. `go build ./... && go test ./... -timeout 90s` pass with no regressions.
7. Integration tests cover happy paths and error paths for all new commands.

## Known Deviations from Source Plan

**Task 1 — DetectLanguage:** The source plan moves `detectLanguage` into
`cli/workspace.go` as `DetectLanguage`. However, the TypeScript expedition
already split this into `detectServerKey` / `detectLanguageID` in `rename.go`
(unexported). Task 1 should instead:
- Export them as `DetectServerKey` and `DetectLanguageID` in `cli/workspace.go`
- Remove `detectServerKey` / `detectLanguageID` from `rename.go`
- Update all call sites in `rename.go` to use the exported versions

**Task 4 — `priming.go` already exists:** The TypeScript expedition created
`internal/backend/lsp/priming.go` with `PrimeTSWorkspace` and `isTSFamily`.
Task 4 creates `internal/backend/lsp/workspace.go` with `PrimeGoWorkspace`.
These are separate files with separate concerns — no conflict.

## Task Sequence

### Task 1 — Workspace helpers (`go-code-actions-01-workspace-helpers`)
Refactor: extract `findWorkspaceRoot` and language helpers from `rename.go`
into `cli/workspace.go`. Adapt for the TypeScript split: export as
`DetectServerKey` and `DetectLanguageID` (not the plan's single `DetectLanguage`).
Pure refactor, no behavior change.

### Task 2 — Exit-code error type (`go-code-actions-02-exit-code-error`)
Create `cli/errors.go` with `ExitCodeError` / `NoEditsError` / `Run`.
Wire into `cmd/refute/main.go`. Replace `os.Exit(2)` in `rename.go`.

### Task 3 — LSP Client: CodeActions + WorkspaceSymbol (`go-code-actions-03-lsp-client`)
Add `CodeAction` and `WorkspaceSymbolInfo` types; add `CodeActions()`,
`ResolveCodeActionEdit()`, `WorkspaceSymbol()` to `Client`. Add client tests.

### Task 4 — Go workspace priming (`go-code-actions-04-go-priming`)
Create `internal/backend/lsp/workspace.go` with `PrimeGoWorkspace`.
Add workspace tests.

### Task 5 — Adapter: FindSymbol Tier 1 (`go-code-actions-05-find-symbol`)
Implement `FindSymbol` in `adapter.go` using `PrimeGoWorkspace` +
`WorkspaceSymbol`. Add `parseQualifiedName` / `lspKindToSymbolKind` helpers.
Add adapter tests.

### Task 6 — Adapter: ExtractFunction + ExtractVariable (`go-code-actions-06-extract`)
Implement `ExtractFunction` and `ExtractVariable` using `CodeActions` +
`ResolveCodeActionEdit`. Honor `--name` via placeholder rewriting.
Add adapter tests.

### Task 7 — Adapter: InlineSymbol (`go-code-actions-07-inline`)
Implement `InlineSymbol` using the identifier-width range. Add adapter test.

### Task 8 — JSON output for edit package (`go-code-actions-08-json-output`)
Create `internal/edit/json.go` with `RenderJSON()`. Add roundtrip tests.

### Task 9 — CLI rename.go refactor (`go-code-actions-09-rename-refactor`)
Extract `buildAdapter`, `finishRename`, `applyOrPreview` helpers from
`runRename`. Add `runRenameTier1` stub. No behavior change.

### Task 10 — CLI: Tier 1 rename path (`go-code-actions-10-tier1-rename`)
Wire `FindSymbol` into `runRenameTier1`. Add `--json` flag. Handle
ambiguous/not-found cases with proper exit codes.

### Task 11 — CLI: extract-function + extract-variable (`go-code-actions-11-extract-cli`)
Create `cli/extract.go` with extract commands. Wire into `RootCmd`.

### Task 12 — CLI: inline command (`go-code-actions-12-inline-cli`)
Create `cli/inline.go` with inline command. Wire into `RootCmd`.

### Task 13 — End-to-end integration tests (`go-code-actions-13-e2e`)
Add `TestEndToEnd_ExtractFunction`, `TestEndToEnd_Tier1Rename`,
`TestEndToEnd_Tier1Ambiguous`, `TestEndToEnd_Tier1NotFound`.
Update `testdata/fixtures/go/rename/main.go` with extractable expression.

**Note from Task 6:** Modern gopls returns `refactor.extract.constant` —
not `refactor.extract.function`/`.variable` — for constant expressions
like `1 + 2`. Use these confirmed inputs in e2e tests:
- ExtractFunction: multi-statement block, e.g. selecting three lines
  (`x := 10; println(x); println(x+1)`) inside `func main()`.
- ExtractVariable: a function-call result, e.g. selecting `src()` inside
  `println(src())` where `src() int` is defined elsewhere.
Also note Task 6 added `textDocument.codeAction.resolveSupport` +
`dataSupport` to the LSP client's initialize capabilities so gopls
returns resolvable data-bearing actions for refactor.extract.*.

### Task 14 — Document position encoding (`go-code-actions-14-docs`)
Create `docs/position-encoding.md` describing byte vs UTF-16 column semantics.

## Experiment Register

No planned experiments.

## Verification Gates

Per-task gate (after every merge):
```bash
go build ./...
go test ./... -timeout 90s
```

Final gate before landing:
```bash
go build ./...
go test ./... -timeout 90s
go test -tags integration ./internal/... -timeout 120s
```
