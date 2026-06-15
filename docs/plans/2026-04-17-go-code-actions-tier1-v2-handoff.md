# Agent Handoff Instructions — Go Code Actions v2

> **Status:** executed — handoff notes for the completed Go Code Actions v2
> plan (landed 2026-04-28, merge `78c5970`). Historical artifact; see
> [README.md](README.md) for status semantics.
> **Landing:** 2026-04-28, merge `78c5970`.
> **Disposition:** retained historical artifact.

These are standing instructions for any Claude Code agent picking up this plan in a fresh context. Read them in full before touching anything.

**Plan:** `docs/plans/2026-04-17-go-code-actions-tier1-v2.md`
**Progress log:** `docs/plans/2026-04-17-go-code-actions-tier1-v2-log.md`

---

## Start of Session

Do these in order before any implementation work:

1. **Read the plan.** You need to know the whole shape, not just your task. Skim tasks you won't touch so you don't accidentally re-invent earlier work.
2. **Read the log.** The Status Dashboard tells you which tasks are `done`, `in-progress`, `blocked`, `pending`. The Deviations and Open Issues sections tell you what the plan got wrong or where it's waiting on a decision.
3. **Read the NEXT-SESSION PROMPT at the bottom of the log.** That is your literal starting instruction, written by the previous agent. If it contradicts the plan, prefer the prompt — the previous agent knew something about current state you don't.
4. **Check git state.** Run `git status` and `git log --oneline -10`. The log's "Commits" column should match reality. If it doesn't, something has drifted — stop and ask the user.
5. **Verify the environment builds clean** before you change anything:
   ```bash
   go build ./... && go test ./... -timeout 90s
   ```
   If this fails on `main` with no changes, report to the user and stop. You are not the one who broke it.
6. **Claim your task.** Set its row in the Status Dashboard to `in-progress` and fill in the Session column (use a short agent label like `opus-1` or `sonnet-A` — whatever the user gave you).

## While Working

- **Follow the plan verbatim.** Every task in the plan has concrete code, exact file paths, and exact commands. The plan was written to be executed as-is.
- **If the plan is wrong, record it.** When you find a bug, an outdated assumption, or a step that doesn't apply, DO NOT silently fix the plan. Do what the bug requires, then add an entry to the **Deviations from the Plan** section of the log with:
  - Which task and step
  - What the plan said
  - What you did instead
  - Why
- **Commit at the granularity the plan specifies.** Each task has its own commit (or small number of commits). Don't collapse, don't split further.
- **Run the test commands the plan specifies after each task.** If a test fails and you can't fix it before your session ends, mark the task `blocked` and document the failure in the log.
- **Don't skip the priming step in Tier 1.** The v1 plan failed specifically because it relied on un-primed `workspace/symbol`. Task 4's `PrimeGoWorkspace` exists for this reason.
- **Never create pull requests or force-push.** Merge feature branches directly to main per the user's global instructions. For this plan, tasks commit directly to `main`.

## End of Session

Whether you finished a task, finished multiple, or stopped mid-task, do all of these before ending:

1. **Update the Status Dashboard.** Set completed tasks to `done`. If you stopped mid-task, leave it `in-progress` with a note in the Session column like `opus-1 (paused at step 3)`.
2. **Add a Session Entry** at the top of the **Session Entries** section:
   ```
   ### YYYY-MM-DD — Task N (Agent <label>)

   **Completed:** Task N steps 1-6
   **Commits:** abc1234 def5678
   **Test status:** go test ./... -timeout 90s → all pass
   **Notes:** <anything surprising, anything worth warning the next agent about>
   ```
3. **Replace the NEXT-SESSION PROMPT** block at the bottom of the log. Write a fresh prompt that:
   - Names the exact task (and step, if mid-task) to resume at
   - Flags anything in the Deviations or Open Issues sections the next agent must address first
   - Lists any uncommitted work or environment state they need to know about
   - Is self-contained — written assuming the reader knows nothing about this codebase or this plan yet

   **Prompt format** (see the template below — keep this exact structure so the user can copy/paste it verbatim).
4. **Commit the log update** as its own commit:
   ```bash
   git add docs/plans/2026-04-17-go-code-actions-tier1-v2-log.md
   git commit -m "docs: log Task N complete and hand off Task N+1"
   ```
5. **Print the prompt to the user** as your final message so they can copy it without opening the file.

---

## NEXT-SESSION PROMPT Template

Replace `<...>` placeholders with concrete text. Keep the markdown quote-block structure so the user can copy it cleanly.

```markdown
### NEXT-SESSION PROMPT

> Continue work on the refute v2 Go code actions plan.
>
> 1. Read `docs/plans/2026-04-17-go-code-actions-tier1-v2-handoff.md` in full.
> 2. Read `docs/plans/2026-04-17-go-code-actions-tier1-v2-log.md` — especially the Status Dashboard, Deviations, and most recent Session Entry.
> 3. Read `docs/plans/2026-04-17-go-code-actions-tier1-v2.md` for the task you're about to work on.
>
> Resume point: **Task <N>** (<task title>). <If mid-task: "The previous agent completed through step X; start at step Y.">
>
> Before you start:
> - <any uncommitted state they need to deal with, or "working tree is clean">
> - <any deviation from the plan they need to know about, or "no deviations so far">
> - <any open issue they must resolve first, or "no open issues">
>
> When you finish (complete a task, or hit a stop), follow the end-of-session protocol in the handoff doc: update the Status Dashboard, append a Session Entry, write a fresh NEXT-SESSION PROMPT, and commit the log.
```

---

## Rules for Writing a Good Prompt

A good prompt gets the next agent productive within 5 minutes of opening it. A bad one sends them back to re-read the whole plan.

- **Name the exact task number and title.** "Task 5 (FindSymbol Tier 1)" not "the symbol stuff".
- **Say what was just finished and what's next.** The log entry has details; the prompt just needs to orient.
- **Warn about landmines.** If you fought gopls for an hour to figure out Unicode columns matter, put it in the prompt — don't make the next agent re-discover.
- **Mention uncommitted work explicitly.** If there's a WIP change on disk, the next agent must know before they run `git status` and panic.
- **Don't summarize the plan.** The plan is already written. Point at it; don't restate it.
- **Don't flatter or editorialize.** "Great progress last session!" is noise. Facts only.

## If You're Stuck

- **Plan step doesn't match reality** (e.g., the file it says to modify doesn't exist): check git log for recent renames; if still unclear, stop and ask the user.
- **Test failure you can't diagnose in 15 minutes:** mark the task `blocked`, write a Session Entry describing the failure with the full error, write a NEXT-SESSION PROMPT that foregrounds the blocker. Don't push a broken `main`.
- **You discover the plan's approach is wrong** (not just a step, but the design): stop, write an entry in **Open Issues / Questions**, write a NEXT-SESSION PROMPT that asks the user to resolve the question before continuing. Do not attempt to redesign the plan unilaterally.

## Don't Do

- Don't skip the "run existing tests before changing anything" check.
- Don't rewrite the plan file. Deviations go in the log.
- Don't collapse multiple tasks into one commit. Commit granularity is part of the plan.
- Don't leave the NEXT-SESSION PROMPT stale. An agent who reads an outdated prompt wastes a session.
- Don't mention this handoff protocol, the log mechanics, or "I'm about to update the log" in normal conversation with the user. They know. Just do it.
