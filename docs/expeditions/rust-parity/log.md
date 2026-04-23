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
| 0  | Empirical spike on rust-analyzer containerName        | pending  |         |        |       |
| 1  | Extend Rust fixture                                   | pending  |         |        |       |
| 2  | ErrLSPServerMissing with install hints (H3)           | pending  |         |        |       |
| 3  | Snippet placeholder stripper (H4)                     | pending  |         |        |       |
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

(Populated by Task 0; the cheap-vs-expensive branch decision drives Task 6.)

- _not yet recorded_

---

## Deviations from the Plan

(Record any step where you did something different from what the plan says, and why. If this section stays empty, that's the ideal outcome.)

- _none yet_

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
