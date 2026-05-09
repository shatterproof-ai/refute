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

_No sessions yet._

---

## Handoff Prompt (for next session)

The agent that just finished a session writes the block below. The user copies it verbatim into a fresh Claude Code session to start the next one. Keep only the **most recent** prompt here — older ones move into the session entry that produced them.

---

### NEXT-SESSION PROMPT

> Start work on the refute Rust parity plan.
>
> 1. Read `docs/expeditions/rust-parity/handoff.md` in full.
> 2. Read `docs/expeditions/rust-parity/log.md` to see what has been done.
> 3. Read `docs/expeditions/rust-parity/plan.md` to see the plan.
> 4. Verify Go tier-1 v2 prerequisites are met (the plan's preamble has the exact `grep` checks). If any check fails, stop and report — Go v2 has not landed yet.
> 5. No tasks have been started. Begin with Task 0 (empirical spike).
>
> When you finish the session (either a task is complete or you hit a stopping point), follow the handoff protocol — update the log, append a session entry, and replace the NEXT-SESSION PROMPT block at the bottom of the log with one that points the next agent at the right resume point.
