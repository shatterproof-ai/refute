# Drift-Control Policy

This policy keeps `refute`'s capability, status, and roadmap documentation in
sync with what the tool actually does. Runtime/docs drift has recurred — a
command ships while docs still call it future, support claims disagree between
files, adapter docs go stale — so this document names the normative source for
each kind of claim and the rules that keep dependent docs honest.

This is governance, not automation. Automated drift checks are tracked
separately by issue #119; this policy defines the contract those checks would
enforce and the manual discipline that applies until they exist.

## Normative sources

For each kind of user-facing claim, exactly one source is normative. When any
other document, story, or help string disagrees with the normative source, the
other document is wrong and must be corrected — the normative source is not
edited to match stale prose.

| Claim domain | Normative source (source of truth) | Derived / dependent surfaces that must match |
| --- | --- | --- |
| Backend support (per-language status, operations, install hints, caveats) | Runtime: `internal/config.SupportMatrix` (`internal/config/support.go`), as surfaced by `refute doctor --json` | `docs/support-matrix.md`, `AGENTS.md` "Backend Support At A Glance", `README.md`, install/doctor skills |
| CLI command inventory (subcommands and flags that exist) | The registered commands in `internal/cli` (the actual cobra/command tree) | `docs/current-state.md` "CLI Surface", `README.md` command overview, stories, `--help` text |
| JSON output and exit codes (statuses, error codes, schema version, envelope shape) | `internal/edit/json.go` statuses + `docs/json-schema.md` as the written normative reference | `docs/support-matrix.md` status descriptions, stories that assert JSON behavior |
| Intent stories (user-facing behavior of covered surfaces) | `docs/stories/` (each story is normative for the behavior it covers) and `docs/stories/INDEX.md` for the catalog | Any doc describing a story-covered surface; tests that assert that behavior |
| Roadmap / current-state status (what is shipped vs planned) | `docs/current-state.md` for shipped/partial/missing status; `docs/roadmap.md` for sequencing of planned work | `README.md`, `AGENTS.md` summaries, `docs/project-goals.md` |

`refute doctor`'s human and `--json` output, the missing-server install hints,
and the `refute rename` missing-server error are all derived from the single
`SupportMatrix` table in `internal/config`, so they cannot drift from each
other. `docs/support-matrix.md` is the prose mirror of that table; see the
freshness rule below.

## Support-matrix freshness rule

`docs/support-matrix.md` is **checked against** the runtime `SupportMatrix`, not
generated from it (generation is out of scope for this issue; see #119). The
rule:

- The per-language status, operations, install hints, and Tier-1 examples in
  `docs/support-matrix.md` must match `internal/config.SupportMatrix` and the
  output of `refute doctor --json`.
- Any change to `internal/config/support.go` that alters a status, operation
  set, install hint, or caveat must update `docs/support-matrix.md` in the same
  branch.
- When verifying, run `refute doctor --json` and reconcile each row against the
  document. A disagreement is a bug, not a documentation preference.

## Roadmap and current-state freshness rule

Status claims have an explicit freshness obligation:

- A status claim ("planned", "missing", "not yet wired up") must be updated the
  moment the feature lands. When a feature ships, move its line from the
  planned/missing section to the implemented section in `docs/current-state.md`
  in the same branch that lands the feature.
- `docs/current-state.md` carries a dated "reflects the repository state as of
  `<date>`" header. Update that date whenever the file is revised, and review
  the file before each release candidate.
- A status claim that is no longer current must either be corrected or moved to
  a clearly historical/plans section — it must not be left asserting a stale
  present-tense fact.

## Same-branch update rule for drift-sensitive changes

Any change that touches a drift-sensitive surface must update the dependent
docs, stories, and tests **in the same branch**, or explicitly file a follow-up
issue and reference it in the commit/branch. The drift-sensitive surfaces are:

- adding, removing, or renaming a CLI command or flag;
- changing an operation's support status or backend routing;
- adding or changing a JSON status, error code, or schema field;
- changing behavior of a surface covered by an intent story;
- changing per-language backend support, install hints, or caveats.

"I'll update the docs in a follow-up" is only acceptable when that follow-up is
a filed, referenced issue. Silent deferral is what produces drift.

## Historical-document labeling

Design specs, plans, logs, handoffs, and audits describe a moment in time, not a
current contract. They must be labeled as historical so future readers (human or
agent) do not mistake target architecture for shipped behavior:

- Historical artifacts are listed under the "Historical Artifacts" heading in
  `docs/README.md` and must not be presented as current contracts.
- When a design spec or plan is superseded by shipped behavior, add a short note
  at its top pointing to the current normative source (e.g. "Historical:
  superseded by `docs/current-state.md`"), or move it under the historical
  section of the index.
- `docs/plans/` follows the lifecycle-status convention in
  `docs/plans/README.md`.

## Pre-landing checklist for drift-sensitive changes

Before landing any branch that touches a drift-sensitive surface, confirm each
item (skip items the change does not touch):

- [ ] **CLI inventory:** Added/removed/renamed a command or flag? Updated
  `docs/current-state.md` "CLI Surface", `README.md`, `--help`, and any affected
  story.
- [ ] **Support matrix:** Changed `internal/config/support.go`? Ran
  `refute doctor --json` and reconciled `docs/support-matrix.md` and the
  `AGENTS.md` "At A Glance" summary against it.
- [ ] **JSON contract:** Added/changed a status, error code, or schema field?
  Updated `docs/json-schema.md` and any story asserting that behavior; bumped
  `schemaVersion` only if the change is non-additive (see `docs/versioning.md`).
- [ ] **Stories:** Changed a story-covered surface? Updated the affected story
  and regenerated `docs/stories/INDEX.md`.
- [ ] **Status freshness:** Did a feature land or change status? Moved its line
  to the correct section of `docs/current-state.md` and refreshed the file's
  "as of" date.
- [ ] **Historical labeling:** Did this supersede a design spec or plan? Labeled
  that document historical or pointed it at the current source.
- [ ] **Follow-ups:** For any dependent doc/story/test intentionally not updated
  in this branch, filed and referenced a follow-up issue.
