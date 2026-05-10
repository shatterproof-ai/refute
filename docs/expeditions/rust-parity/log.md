# Progress Log — Rust Parity

**Plan:** `docs/expeditions/rust-parity/plan.md`
**Handoff Protocol:** `docs/expeditions/rust-parity/handoff.md`
**Spec:** `docs/specs/2026-04-22-rust-parity-design.md`
**Started:** (not yet)

This log is the single source of truth for "what has been done and what is next" on this plan. Every agent working on the plan appends here. Newest entries go at the top of **Session Entries**; the status dashboard is edited in place.

---

## Status Dashboard

| #  | Task                                                  | Status   | Session | Commit | Notes |
|----|-------------------------------------------------------|----------|---------|--------|-------|
| 0  | Empirical spike on rust-analyzer containerName        | done        | sonnet-1 |   | expensive branch — containerName has no trait info |
| 1  | Extend Rust fixture                                   | done        | sonnet-1 |      | line# update in existing tests — see Deviations |
| 2  | ErrLSPServerMissing with install hints (H3)           | done        | sonnet-1 |      |       |
| 3  | Snippet placeholder stripper (H4)                     | done        | sonnet-1 |      |       |
| 4  | PrimeRustWorkspace                                    | done     | sonnet-2 | 43a608f |      |
| 5  | Rust code-action pattern table                        | done     | sonnet-2 | 29f887c |      |
| 6  | Rust container parser (branch on Task 0)              | done     | sonnet-2 | bd027f7 | expensive branch — DocumentSymbol walk |
| 7  | Adapter language dispatch                             | done     | sonnet-2 | b7aca60 | shouldPrimeWorkspace removed |
| 8  | ParseRustQualifiedName (forms 1-7)                    | done     | sonnet-2 | e8e550c |      |
| 9  | CLI rename dispatch for Rust qualified names          | done     | sonnet-2 | ea75e05 | Container field added to symbol.Location |
| 10 | CLI extract/inline adapter wiring                     | done     | sonnet-2 | 2ccee85 | runCodeAction + literal-name fallback |
| 11 | Inline --call-site flag                               | done     | sonnet-2 | f218557 |      |
| 12 | Support-matrix doc + help text (H5, H6)               | done     | sonnet-2 | e524807 |      |
| 13 | Integration tests batch 1 — rename hardening (H1, H2) | done     | sonnet-2 | 185e0ed | assertion fix: no leading space in param check |
| 14 | Integration tests batch 2 — code actions              | done     | sonnet-2 | df70e40 | inline regex fix; literal name fallback |
| 15 | Integration tests batch 3 — Tier-1 + missing server   | done     | sonnet-2 | fc8e007 | --language flag doesn't exist; use --file |
| 16 | Full regression run + merge to main                   | done     | sonnet-2 |         |      |

**Legend:** `pending` → not started · `in-progress` → claimed by an active session · `blocked` → see notes · `done` → merged

---

## Task 0 Findings

**rust-analyzer version:** rust-analyzer 1.93.1 (01f6ddf 2026-02-11)

**workspace/symbol `fmt` result containerName values:**
- Entry 1 (Display::fmt): `containerName="Greeter"`, kind=12 (Function), lib.rs line 11
- Entry 2 (Debug::fmt): `containerName="Greeter"`, kind=12 (Function), lib.rs line 17

**workspace/symbol `format_greeting` result:**
- containerName="" (empty — top-level function, no container)

**workspace/symbol `Greeter` result:**
- containerName="", kind=23 (Struct)

**Branch selection:** expensive

**Reasoning:** Both `impl fmt::Display for Greeter` and `impl fmt::Debug for Greeter` produce `containerName="Greeter"` with no trait information present. Substring-matching on containerName cannot distinguish them; Task 6 must implement the `textDocument/documentSymbol` walk to identify the enclosing `impl Trait for Type` block.

**Note for Task 6:** The spike required `DidOpen` + a second `WaitForIdle` before workspace/symbol returned results. The `WaitForIdle` after `StartClient` alone is not sufficient — rust-analyzer returns idle before it indexes for workspace/symbol queries. Task 4's `PrimeRustWorkspace` (which opens files via DidOpen) is therefore load-bearing for Tier-1 symbol resolution, not just performance.

---

## Deviations from the Plan

(Record any step where you did something different from what the plan says, and why. If this section stays empty, that's the ideal outcome.)

- **Task 1 (2026-05-09):** Rewriting `lib.rs` moved `format_greeting` from line 1 → line 5 and `Greeter` from line 5 → line 14. The existing tests `TestEndToEnd_RenameRustFunction`, `TestEndToEnd_RustDryRun`, and `TestEndToEnd_RenameRustStruct` had hardcoded `--line` flags that broke. Updated their `--line` arguments in `integration_test.go` as part of Task 1 commit. This was not in the plan but is the obvious fix.

- **Pre-execution corrections (2026-05-09):** Plan was updated before Task 0 began to fix two incorrect assumptions:
  1. **Task 7**: `isTSFamily()` and `PrimeTSWorkspace()` were referenced but neither exists. Corrected dispatch uses `PrimeWorkspace(client, root, languageID)` for TS family and `PrimeRustWorkspace(client, root)` for Rust. Added Step 3 to remove the now-dead `shouldPrimeWorkspace()` from `priming.go`.
  2. **Task 12**: Plan said to CREATE `docs/support-matrix.md` but it already exists on `main`. Step 1 rewritten to UPDATE the Rust row and append new sections.

- **Task 6 (2026-05-09):** Plan used `c.call(...)` (non-existent method). Used `c.request` + `json.Unmarshal` pattern (consistent with existing `WorkspaceSymbol`). Added `DocumentSymbols` method directly to `adapter.go` alongside the type definitions.

- **Task 9 (2026-05-09):** Plan used `symbol.Query{Name: name}` which would fail (FindSymbol requires `QualifiedName`). Used `symbol.Query{QualifiedName: name}` instead. Also added `Container string` field to `symbol.Location` (plan left this as "preferred shape") and populated it in `FindSymbol`.

- **Task 10 (2026-05-09):** Plan replaced existing `ExtractFunction`/`ExtractVariable`/`InlineSymbol` with `runCodeAction` wholesale. This would break Go extraction. Instead, added language dispatch at the beginning of each method: Rust uses `runCodeAction`, Go uses existing `extractImpl` path. `rewritePlaceholderName` extended with literal-name fallback (`fun_name`, `var_name`) since rust-analyzer uses literal identifiers not snippet placeholders for extracted names.

- **Task 11 (2026-05-09):** Removed `MarkFlagRequired` for `--file` and `--line` on the inline command, since `--call-site` provides an alternative source for the location. Added validation in `RunE` instead.

- **Task 13 (2026-05-09):** Plan's parameter-rename assertion used `" name: &str"` (with leading space), but fixture has `(name: &str` (after open paren). Fixed to `"name: &str"` and `"greetee: &str"` without leading space.

- **Task 14 (2026-05-09):** Multiple corrections:
  1. `--start-column`/`--end-column` don't exist; the flags are `--start-col`/`--end-col`.
  2. Plan used line 10 for `compute` body, but body is on line 11 after Task 1's fixture change.
  3. `opInlineCallSite` regex `^inline( call)?$` doesn't match rust-analyzer's title `Inline \`greet::util::sum\``. Go RE2 has no negative lookahead, so added `titleExcludes` field to `rustActionPattern` and updated `matchRustAction`.
  4. `--language` flag doesn't exist on inline command; `TestEndToEnd_InlineRustRequiresCallSite` adapted.

- **Task 15 (2026-05-09):** All Tier-1 tests used `--language rust` flag which doesn't exist. Fixed to use `--file <rust-file>` for language detection (DetectServerKey reads extension). `TestEndToEnd_BadServerConfig` also needed `--line 1` → `--line 5` (Task 1 fixture change missed earlier) and assertion updated for `ErrLSPServerMissing` introduced in Task 2. `moduleMatches` fixed to handle suffix matching when rust-analyzer provides shorter containerName (e.g., `"util"` instead of `"greet::util"`).

---

## Open Issues / Questions

(Anything that needs the user's decision before the next agent can proceed.)

- _none yet_

---

## Session Entries

(Newest at top. One entry per agent session. Template:)

```
### YYYY-MM-DD — Task N (Agent <short-id or "anon">)

**Completed:** Task N step X through Task N step Y
**Commits:** <short sha> <short sha>
**Test status:** `go test ./... -timeout 90s` → pass / fail (detail)
**Notes:** what surprised you, anything worth warning the next agent about
```

### 2026-05-09 — Tasks 4-16 (Agent sonnet-2)

**Completed:** Tasks 4-16 fully — all steps including commits, integration test fixes, full regression
**Commits:** 43a608f (T4) 29f887c (T5) bd027f7 (T6) b7aca60 (T7) e8e550c (T8) ea75e05 (T9) 2ccee85 (T10) f218557 (T11) e524807 (T12) 185e0ed (T13) df70e40 (T14) fc8e007 (T15)
**Test status:** `go test ./... -timeout 90s` → 124 passed; `go test -tags integration ./internal/... -timeout 900s` → 149 passed
**Notes:**
- Multiple plan deviations required (see Deviations section). Key ones: `--language` flag doesn't exist (use `--file` for detection); `opInlineCallSite` regex needed `titleExcludes` for Go RE2 compatibility; rust-analyzer uses literal `fun_name`/`var_name` not snippet placeholders; `moduleMatches` needed suffix-length flexibility for abbreviated containerName; `TestEndToEnd_BadServerConfig` line number missed in Task 1 deviation.
- `symbol.Location` now has `Container string` field (populated from workspace/symbol `containerName`).
- `rewritePlaceholderName` returns bool; `runCodeAction` falls back to `rewritePlaceholder(we, lit, name)` if no snippet found.
- Expedition complete. All 15 Rust integration tests pass or skip cleanly.

---

### 2026-05-09 — Tasks 0-3 (Agent sonnet-1)

**Completed:** Tasks 0, 1, 2, 3 fully — all steps including commits
**Commits:** 75f1e87 (Task 0) 32265d2 (Task 1) 873dc0a (Task 2) 8664647 (Task 3)
**Test status:** `go test ./internal/... -timeout 90s` → 95 passed, 0 failed
**Notes:**
- The rust-parity branch was 50 commits behind main (all Go v2 work). Merged main before starting Task 0. Clean merge, no conflicts.
- Task 0: WaitForIdle alone is not enough for workspace/symbol to return results. DidOpen + second WaitForIdle is required. **Expensive branch confirmed** — both Display::fmt and Debug::fmt return containerName="Greeter", no trait info present.
- Task 1: Rewriting lib.rs moved format_greeting from line 1 → 5 and Greeter from line 5 → 14. Updated hardcoded --line args in the three existing Rust integration tests (not in original plan; deviation recorded in log).
- Task 3 deviation: the plan's test case `"literal dollar-digit in string preserved"` is contradictory — the regex cannot distinguish `$5 bill` from a snippet tabstop. Dropped that test case with a comment explaining the limitation. HasSnippetPlaceholders correctly returns true for `$5 bill` per the plan's own correction note.

---

## Handoff Prompt (for next session)

The agent that just finished a session writes the block below. The user copies it verbatim into a fresh Claude Code session to start the next one. Keep only the **most recent** prompt here — older ones move into the session entry that produced them.

---

### NEXT-SESSION PROMPT

> Continue work on the refute Rust parity plan.
>
> 1. Read `docs/expeditions/rust-parity/handoff.md` in full.
> 2. Read `docs/expeditions/rust-parity/log.md` — especially the Status Dashboard, Task 0 Findings, Deviations, and most recent Session Entry.
> 3. Read `docs/expeditions/rust-parity/plan.md` for Task 4 onwards.
>
> Resume point: **Task 4** (PrimeRustWorkspace). Tasks 0–3 are done. Working tree is clean. Branch `rust-parity` is at 8664647.
>
> Before you start:
> - Working tree is clean; `go build ./... && go test ./internal/... -timeout 90s` → 95 pass.
> - Main has been merged into rust-parity (clean merge at b5a88ac). No further main merges needed unless you hit a conflict.
> - Task 0 determined: **expensive branch** for Task 6 — both Display::fmt and Debug::fmt return containerName="Greeter". See Task 0 Findings in the log.
> - Task 7 plan is already corrected (uses `shouldPrimeWorkspace` pattern, not `isTSFamily`). Task 12 is corrected (UPDATE not CREATE for support-matrix.md). No further plan corrections needed.
> - Integration tests for Tasks 13-15 require rust-analyzer on PATH (present: rust-analyzer 1.93.1).
>
> When you finish, follow the end-of-session protocol in the handoff doc: update the Status Dashboard, append a Session Entry, write a fresh NEXT-SESSION PROMPT, and commit the log.
