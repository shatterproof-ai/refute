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
| 4  | PrimeRustWorkspace                                    | pending  |         |        |       |
| 5  | Rust code-action pattern table                        | pending  |         |        |       |
| 6  | Rust container parser (branch on Task 0)              | pending  |         |        |       |
| 7  | Adapter language dispatch                             | pending  |         |        |       |
| 8  | ParseRustQualifiedName (forms 1-7)                    | pending  |         |        |       |
| 9  | CLI rename dispatch for Rust qualified names          | pending  |         |        |       |
| 10 | CLI extract/inline adapter wiring                     | pending  |         |        |       |
| 11 | Inline --call-site flag                               | pending  |         |        |       |
| 12 | Support-matrix doc + help text (H5, H6)               | pending  |         |        |       |
| 13 | Integration tests batch 1 — rename hardening (H1, H2) | pending  |         |        |       |
| 14 | Integration tests batch 2 — code actions              | pending  |         |        |       |
| 15 | Integration tests batch 3 — Tier-1 + missing server   | pending  |         |        |       |
| 16 | Full regression run + merge to main                   | pending  |         |        |       |

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
