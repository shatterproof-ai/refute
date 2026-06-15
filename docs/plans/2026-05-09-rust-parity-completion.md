# Rust Parity — Completion Plan

> **Status:** executed — Rust refactoring reached parity with Go tier-1; the
> Rust backend (priming, code-action matching, qualified-name resolution) and
> its tests shipped. Historical artifact; see [README.md](README.md) for status
> semantics.
> **Landing:** exact merge commit not recorded in this historical plan; current
> support status is reflected in `docs/support-matrix.md`.
> **Disposition:** retained historical artifact.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Execute all 17 pending tasks of the rust-parity expedition to bring Rust refactoring to parity with Go tier-1.

**Architecture:** Reuse Go tier-1 v2 surface (`Client.CodeActions`, `Client.ResolveCodeActionEdit`, `Client.WorkspaceSymbol`, adapter's `FindSymbol`/`ExtractFunction`/`ExtractVariable`/`InlineSymbol`). Add Rust-specific files for priming, code-action matching, container parsing, and qualified-name parsing. Adapter grows a language-dispatch layer.

**Tech Stack:** Go 1.22+, rust-analyzer, stdlib `regexp`/`encoding/json`/`io/fs`/`path/filepath`.

**Primary source:** `docs/expeditions/rust-parity/plan.md` on the `rust-parity` branch. This document provides status, corrections, and execution instructions. Apply corrections below where they conflict with the expedition plan.

**Worktree:** `/home/ketan/project/refute-rust-parity` on branch `rust-parity`. All implementation work happens there.

---

## State as of 2026-05-09

| # | Task | Status |
|---|---|---|
| 0 | Empirical spike on rust-analyzer containerName | pending |
| 1 | Extend Rust fixture | pending |
| 2 | ErrLSPServerMissing with install hints (H3) | pending |
| 3 | Snippet placeholder stripper (H4) | pending |
| 4 | PrimeRustWorkspace | pending |
| 5 | Rust code-action pattern table | pending |
| 6 | Rust container parser (branch on Task 0) | pending |
| 7 | Adapter language dispatch | pending |
| 8 | ParseRustQualifiedName (forms 1-7) | pending |
| 9 | CLI rename dispatch for Rust qualified names | pending |
| 10 | CLI extract dispatch for Rust | pending |
| 11 | Inline --call-site flag | pending |
| 12 | Support-matrix doc + help text (H5, H6) | pending |
| 13 | Integration tests batch 1 — rename hardening | pending |
| 14 | Integration tests batch 2 — code actions | pending |
| 15 | Integration tests batch 3 — Tier-1 + missing server | pending |
| 16 | Full regression run + merge to main | pending |

**Prerequisites:** Go tier-1 v2 is fully landed — `CodeActions`, `WorkspaceSymbol`, `extract.go`, `inline.go`, `errors.go`, `json.go` all present. Build is clean.

---

## Corrections to Expedition Plan

Two places where the expedition plan's assumptions do not match the current codebase. Apply these corrections when those tasks run; everything else in the expedition plan is correct as written.

---

### Correction A — Task 7: adapter dispatch

The expedition plan (Task 7, Steps 1–2) refers to `isTSFamily()` and `PrimeTSWorkspace()`. **Neither function exists.** The current code in `adapter.go:Initialize()` is:

```go
if shouldPrimeWorkspace(a.languageID) {
    _ = PrimeWorkspace(a.client, absRoot, a.languageID)
}
```

Replace Task 7 Steps 1 and 2 with the following corrected steps.

#### Corrected Task 7, Step 1: Add dispatch helpers

At the bottom of `internal/backend/lsp/adapter.go` add:

```go
// primeWorkspace dispatches to the language-specific priming walker.
// Failures are intentionally non-fatal.
func (a *Adapter) primeWorkspace(absRoot string) {
	switch a.languageID {
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		_ = PrimeWorkspace(a.client, absRoot, a.languageID)
	case "rust":
		_ = PrimeRustWorkspace(a.client, absRoot)
	}
}

// matchAction dispatches to the language-specific code-action matcher.
func (a *Adapter) matchAction(actions []CodeAction, op rustActionOp) (*CodeAction, error) {
	if a.languageID == "rust" {
		return matchRustAction(actions, op)
	}
	return nil, fmt.Errorf("no code-action matcher for language %q", a.languageID)
}
```

#### Corrected Task 7, Step 2: Replace priming call in Initialize

Find in `internal/backend/lsp/adapter.go`:

```go
if shouldPrimeWorkspace(a.languageID) {
    _ = PrimeWorkspace(a.client, absRoot, a.languageID)
}
```

Replace with:

```go
a.primeWorkspace(absRoot)
```

#### Corrected Task 7, Step 3: Remove dead `shouldPrimeWorkspace`

In `internal/backend/lsp/priming.go`, delete the `shouldPrimeWorkspace` function entirely — it is no longer called after Step 2:

```go
func shouldPrimeWorkspace(languageID string) bool {
	switch languageID {
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return true
	case "rust":
		return true
	}
	return false
}
```

Run tests after:

```bash
go test ./internal/backend/lsp/ -timeout 90s
```

Expected: all pass.

#### Corrected Task 7, Step 4: Commit

```bash
git add internal/backend/lsp/adapter.go internal/backend/lsp/priming.go
git commit -m "refactor(lsp): language-dispatch primeWorkspace and matchAction"
```

---

### Correction B — Task 12: support-matrix UPDATE not CREATE

`docs/support-matrix.md` **already exists** on `main` (landed separately). The expedition plan's Task 12 Step 1 says to create it from scratch — skip that step. Instead:

#### Corrected Task 12, Step 1: Update Rust row

Open `docs/support-matrix.md`. Find the existing Rust row in the language matrix (currently has only `rename` in Operations and `experimental` status). Update:

- **Operations**: `rename, extract-function, extract-variable, inline (single call site)`
- **Test coverage**: add the new integration tests from Tasks 13–15 to the existing note
- **Status**: keep `experimental` (H5 calls for this until dogfood confidence improves)
- **Caveats**: add `(1) Tier-1 qualified-name syntax: crate::module::Type::method, <Type as Trait>::method (forms 1–7).`

After the language matrix table, add two new sections if they are not already present:

```markdown
## Tier-1 Qualified-Name Resolution

`--symbol` accepts qualified names resolved via `workspace/symbol`:

| Language | Example | Notes |
|---|---|---|
| Go | `pkg.FunctionName`, `Type.Method` | dot-separated |
| Rust | `greet::format_greeting`, `<Greeter as Display>::fmt` | forms 1–7 per `docs/specs/2026-04-22-rust-parity-design.md` |

## Missing-Server Install Hints

When `refute` cannot find a language server it prints an install hint from the server config.

| Language | Install |
|---|---|
| Go | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rustup component add rust-analyzer` |
| TypeScript | `npm install -g typescript-language-server typescript` |
| Python | `pip install pyright` |
```

#### Corrected Task 12, Step 2 onwards

Steps 2 (reference from design doc), 3 (update CLI help text), 4 (verify help text), and 5 (commit) are unchanged from the expedition plan. Proceed with those.

---

## Execution Instructions

1. **cd into the worktree:**
   ```bash
   cd /home/ketan/project/refute-rust-parity
   ```

2. **Verify clean build before touching anything:**
   ```bash
   go build ./... && go test ./... -timeout 90s
   ```

3. **Read the expedition handoff instructions:**
   `docs/expeditions/rust-parity/handoff.md`

4. **Read the full expedition plan:**
   `docs/expeditions/rust-parity/plan.md`

5. **Execute Tasks 0–16 in order** per the expedition plan, substituting the corrected steps for Task 7 and Task 12 from this document.

6. **Log every completed task** in `docs/expeditions/rust-parity/log.md` per the handoff protocol.

---

## File Structure

```
internal/backend/lsp/
├── rust_actions.go          CREATE — kind+title pattern table; matchRustAction
├── rust_actions_test.go     CREATE
├── rust_priming.go          CREATE — PrimeRustWorkspace (skips target/, .git/, .cargo/)
├── rust_priming_test.go     CREATE
├── rust_container.go        CREATE — parseRustContainer (cheap or expensive per Task 0)
├── rust_container_test.go   CREATE
├── adapter.go               MODIFY — primeWorkspace/matchAction dispatch; remove shouldPrimeWorkspace call
└── priming.go               MODIFY — remove shouldPrimeWorkspace (dead after Task 7)

internal/cli/
├── rust_symbol.go           CREATE — ParseRustQualifiedName forms 1-7
├── rust_symbol_test.go      CREATE
├── rename.go                MODIFY — dispatch Rust qualified names
├── extract.go               MODIFY — route Rust code-action matching
├── inline.go                MODIFY — add --call-site flag
└── errors.go                MODIFY — add ErrLSPServerMissing with install hints

internal/edit/
├── snippet.go               CREATE — stripSnippetPlaceholders
└── snippet_test.go          CREATE

internal/integration_test.go MODIFY — 12 new integration tests

testdata/fixtures/rust/rename/
├── src/lib.rs               EXTEND — multi-trait impl + compute fn + rename targets
├── src/main.rs              EXTEND — cross-file inline call site
└── src/util.rs              CREATE — form-7 qualified-name target

docs/
└── support-matrix.md        MODIFY — update Rust row + add Tier-1 and install-hints tables
```

---

## Known Risks

- **Task 0 spike** determines whether Task 6 takes the cheap (~10 LOC) or expensive (~150 LOC + DocumentSymbol client method) branch. Record the finding in the expedition log before starting Task 6.
- **rust-analyzer version drift**: code-action titles in `rust_actions.go` regex patterns are pinned to current behavior. If `matchRustAction` returns `ErrActionNotOffered`, log the offered titles and update the regex.
- **Integration test skipping**: every test that exercises rust-analyzer must `t.Skip` when `exec.LookPath("rust-analyzer")` fails, except `TestEndToEnd_RustAnalyzerMissing` which requires its absence.
