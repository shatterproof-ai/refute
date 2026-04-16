# Implementation Log: Core Infrastructure + Go Rename

**Plan:** `docs/plans/2026-04-15-refute-core-go-rename.md`
**Spec:** `docs/specs/2026-04-15-refute-design.md`
**Started:** 2026-04-15
**Status:** Tasks 1-8 complete, Tasks 9-11 remaining

## Completed Work

### Task 1: Project Scaffolding (commit `37384bd`)
- Initialized `go.mod` with module `github.com/shatterproof-ai/refute`
- Created directory structure: `cmd/refute/`, `internal/{edit,symbol,backend/lsp,config,cli}/`, `testdata/fixtures/go/rename/util/`
- Minimal `cmd/refute/main.go` with version command

### Task 2: Core Types (commit `78b27e3`)
- `internal/edit/types.go` — Position, Range, TextEdit, FileEdit, WorkspaceEdit (0-indexed, matching LSP)
- `internal/symbol/types.go` — SymbolKind enum with String(), Location (1-indexed), SourceRange, Query with Tier() method
- `internal/backend/backend.go` — RefactoringBackend interface, ErrUnsupported, ErrSymbolNotFound, ErrAmbiguous, Capability

### Task 3: Edit Applier (commit `096f800`)
- `internal/edit/applier.go` — Apply() with 3-phase atomic writes (compute → temp files → rename), rollback on failure
- `internal/edit/applier_test.go` — 4 tests: single rename, multiple edits reverse order, multi-file, rollback
- Helper: positionToOffset (0-indexed line/char → byte offset), applyEdits (reverse-sorted application)

### Task 4: Diff Renderer (commit `d138c37`)
- `internal/edit/diff.go` — RenderDiff() using go-difflib for unified diffs
- `internal/edit/diff_test.go` — 2 tests: single file diff, empty edit
- Added dependency: `github.com/pmezard/go-difflib`

### Task 5: Config System (commit `f6b578e`)
- `internal/config/config.go` — Config, ServerConfig, DaemonConfig types. Load() merges 4 layers: built-in defaults → user global → project → explicit path. Built-in defaults for go/typescript/python servers.
- `internal/config/config_test.go` — 3 tests: defaults, project config, explicit path

### Task 6: LSP Transport Layer (commit `064d221`)
- `internal/backend/lsp/transport.go` — Transport with Content-Length framing. Write() and Read() over io.Reader/io.Writer.
- `internal/backend/lsp/transport_test.go` — 2 tests: round-trip write/read, multiple messages

### Task 7: LSP Client Protocol (commit `932f7fa`)
- `internal/backend/lsp/client.go` — Client struct managing LSP server subprocess. StartClient() launches process, runs initialize handshake. readLoop goroutine dispatches responses. Methods: DidOpen, Rename, Shutdown. Handles both `changes` and `documentChanges` WorkspaceEdit formats. fileToURI/uriToFile helpers.
- `internal/backend/lsp/client_test.go` — 2 integration tests (require gopls): TestClient_Initialize, TestClient_Rename

### Task 8: LSP Backend Adapter (commit `809a592`)
- `internal/backend/lsp/adapter.go` — Adapter implementing RefactoringBackend. NewAdapter(cfg, languageID, filePatterns). Converts 1-indexed Location to 0-indexed LSP positions. Rename via DidOpen + client.Rename. Compile-time interface check.
- `internal/backend/lsp/adapter_test.go` — 2 tests: TestAdapter_Rename (integration), TestAdapter_Capabilities

## Test Results

All 12 tests pass (including gopls integration tests):

```
internal/backend/lsp    — 6 tests (transport: 2, client: 2, adapter: 2)
internal/config         — 3 tests
internal/edit           — 6 tests (applier: 4, diff: 2)
```

gopls integration tests verified with gopls v0.21.1 (installed during session).

## Remaining Work

### Task 9: Symbol Resolution (Tiers 2 & 3)
- Create `internal/symbol/resolver.go` and `resolver_test.go`
- Resolve() function: Tier 2 (file+line+name → scan line, find column), Tier 3 (file+line+col → pass through)
- 5 unit tests

### Task 10: CLI Rename Commands
- Create `internal/cli/root.go` and `internal/cli/rename.go`
- Update `cmd/refute/main.go` to use cobra
- Add dependencies: cobra, fatih/color
- Rename subcommands: rename, rename-function, rename-class, rename-field, rename-variable, rename-parameter, rename-type, rename-method
- Flags: --file, --line, --col, --name, --new-name, --symbol, --dry-run, --verbose, --config
- Wiring: symbol.Resolve → config.Load → detectLanguage → lsp.NewAdapter → adapter.Rename → edit.Apply or edit.RenderDiff
- findWorkspaceRoot() walks up to find go.mod/package.json/etc.
- detectLanguage() maps file extensions to LSP language IDs

### Task 11: End-to-End Integration Test
- Create Go fixture project in testdata/fixtures/go/rename/ (multi-file with cross-package reference)
- Create `internal/integration_test.go` (build tag: integration)
- TestEndToEnd_RenameGoFunction: build refute, rename FormatGreeting→BuildGreeting, verify cross-file update, verify compilation
- TestEndToEnd_DryRun: same rename with --dry-run, verify diff output, verify files unchanged

### Verification Checklist (after all tasks)
- Build and run `refute version`
- `refute rename-function --help` shows flags
- Dry-run rename on fixture project shows diff
- Apply rename, verify compilation
- All tests pass

## Notes

- gopls needs to be on $PATH for integration tests. Install: `go install golang.org/x/tools/gopls@latest`
- Subagents during this session created merge commits for some tasks (worktree-based workflow). This is cosmetic — all code landed on main.
- The plan's code for Tasks 9-11 was verified against the actual implementations of Tasks 1-8. Types, function signatures, and import paths are consistent.
