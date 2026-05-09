# Issue Backlog: Real-Usage Readiness

This backlog converts the roadmap into issue-sized work. Priority order follows
the current product direction:

1. Phase 0: stabilize the existing CLI core.
2. Phase 1: make outputs and introspection agent-ready.
3. Phase 6: prove release readiness on real repositories.
4. Phase 3: expose the core through MCP.
5. Phase 5: add higher-level refactorings.

Phase 2 daemon work is intentionally not a top priority here. MCP can start in
stdio mode over the existing single-shot core; a daemon remains useful later for
warm backend reuse and shared lifecycle management.

---

## ✓ Completed Issues

### P0-01: Publish an Accurate Support Matrix ✓

Landed: `docs/support-matrix.md` with language, backend, operation, test
coverage, and promotion process. Linked from README.

### P0-02: Align Backend Capability Reporting With Reality ✓

Landed: LSP adapter now reports rename, extract-function, extract-variable, and
inline. ts-morph and OpenRewrite accurately report only rename, which is all
they implement.

### P0-03: Define Stable Operation Result JSON ✓

Landed: `schemaVersion: "1"` contract with `applied`, `dry-run`, `no-op`,
`ambiguous`, `unsupported`, `backend-missing`, and `backend-error` statuses.
Contract tests added in `internal/edit/contract_test.go`.

### P0-04: Fix CLI/LSP Position Encoding ✓

Landed: `byteColumnToUTF16Character` and `utf16CharacterToByteColumn` with
round-trip tests covering ASCII, multi-byte Unicode, and emoji.
`docs/position-encoding.md` remains but its deferred-status note is stale.

### P0-05: Harden Tier-2 Symbol Resolution ✓

Landed: `findNameMatches` validates identifier boundaries, rejects partial
matches, detects multiple same-line occurrences, and ignores names inside
comments or string literals. Tests cover all cases.

### P0-07: Strengthen Multi-File Apply Semantics ✓

Landed: workspace edit application is rollback-safe. Already-written files are
reverted when a later rename fails. Partial-success claims no longer possible.

### P1-03: Add `doctor` / `list-backends` ✓

Landed: `refute doctor` reports configured languages, backend availability,
installed tools, and supported operations. Supports `--json`.

---

## Open Issues

## P0-06: Improve Missing Backend and Unsupported Operation Errors

**Phase:** 0
**Priority:** P0
**Labels:** `errors`, `backend`, `cli`

### Goal

Make failures actionable when tools are missing or operations are unsupported.

### Scope

- Add typed errors for missing server, missing adapter runtime, unsupported
  operation, and backend initialization failure.
- Include detected language, backend name, command path, and install hint where
  possible.
- Emit structured JSON for these errors.
- Add tests for missing `gopls`, `rust-analyzer`, `typescript-language-server`,
  ts-morph dependencies, and OpenRewrite JAR.

### Acceptance Criteria

- Users and agents can distinguish "install this tool" from "operation not
  supported" and "backend crashed."
- Errors include next action when known.
- JSON and human-readable error paths are both tested.

## P0-08: Add a Real-Usage CLI Smoke Suite

**Phase:** 0
**Priority:** P0
**Labels:** `tests`, `cli`, `stabilization`

### Goal

Create a fast smoke suite that exercises the CLI like a user or agent would.

### Scope

- Build the `refute` binary once per suite.
- Run dry-run and apply paths for supported fixture operations.
- Assert stdout/stderr/exit-code behavior for success, no-op, ambiguity,
  missing backend, unsupported operation, and bad position.
- Keep optional external tools guarded with clear skips.

### Acceptance Criteria

- Smoke suite can run locally without mutating checked-in fixtures.
- All skips include the missing prerequisite.
- The suite catches regressions in command wiring and output contracts.

## P1-01: Make `--json --dry-run` the Canonical Preview API

**Phase:** 1
**Priority:** P0
**Labels:** `agent-api`, `json`, `dry-run`

### Goal

Give agents a reliable preview command with golden-tested output before applying
changes. The schema exists (`schemaVersion: "1"`); golden tests are incomplete.

### Scope

- Add golden tests for rename, extract, inline, no-op, ambiguity, missing
  backend, unsupported operation, and bad-position previews.
- Ensure every mutation command supports `--json --dry-run`.
- Preserve diff output for humans when `--json` is absent.

### Acceptance Criteria

- Every documented JSON outcome shape has a golden test.
- Agents do not need to parse unified diffs to decide whether a command found
  edits.
- Applying remains opt-in and separate.

## P1-02: Add `list-symbols`

**Phase:** 1
**Priority:** P1
**Labels:** `cli`, `agent-api`, `symbol-resolution`

### Goal

Let agents discover candidate symbols before requesting a refactoring.

### Scope

- Add `refute list-symbols`.
- Support query string, file scope, kind filter, and JSON output.
- Use LSP `workspace/symbol` where available.
- Report backend limitations explicitly.
- Add tests for empty, single, and ambiguous result sets.

### Acceptance Criteria

- Agents can discover exact file/line/column candidates without guessing.
- Results include enough information to feed Tier-3 operations.
- Unsupported languages return structured unsupported results.

## P1-04: Add Operation Metadata to Results

**Phase:** 1
**Priority:** P1
**Labels:** `agent-api`, `observability`, `json`

### Goal

Make every result explain how it was produced.

### Scope

- Include operation, target language, backend name, backend version when
  available, workspace root, input mode, and support path.
- Distinguish standard LSP, server-specific code action, execute-command,
  custom adapter, and structural rewrite.
- Add tests for metadata in successful and failed results.

### Acceptance Criteria

- Agents can audit which backend performed an edit.
- Support matrix claims can be cross-checked against actual command output.

## P1-05: Split Core Operation Execution From Cobra Commands

**Phase:** 1
**Priority:** P1
**Labels:** `architecture`, `cli`, `mcp-prep`

### Goal

Prepare the codebase for MCP without duplicating CLI logic.

### Scope

- Extract command execution into package-level operation functions that accept
  structured requests and return structured results.
- Keep Cobra responsible for flag parsing and output formatting only.
- Ensure CLI JSON and future MCP tools can call the same operation layer.

### Acceptance Criteria

- Rename, extract, and inline have reusable operation functions.
- Existing CLI behavior remains compatible.
- Unit tests can exercise operation logic without invoking Cobra.

## P6-01: Define Release Readiness Criteria

**Phase:** 6
**Priority:** P0
**Labels:** `release`, `docs`, `quality`

### Goal

Define what "ready for real usage" means before claiming it.

### Scope

- Document supported languages and operations for the first usable release.
- Define required tests, docs, install steps, and known limitations.
- Define compatibility expectations for JSON and MCP schemas.
- Define minimum external dependency versions where needed.

### Acceptance Criteria

- Release criteria are explicit and reviewable.
- Every criterion maps to tests, docs, or an issue.

## P6-02: Build a Pinned Real-World Corpus

**Phase:** 6
**Priority:** P0
**Labels:** `tests`, `corpus`, `release`

### Goal

Test refactorings on real projects, not only small fixtures.

### Scope

- Select small-to-medium pinned repositories for Go, TypeScript/JavaScript,
  Python, Rust, and Java.
- Add scripts to fetch or materialize each corpus target at a fixed commit.
- Define one or more safe refactorings per target.
- Verify build, typecheck, or test commands after applying edits.

### Acceptance Criteria

- Corpus tests are reproducible.
- Network-dependent setup is separated from normal unit tests.
- Failures produce enough context to debug backend drift.

## P6-03: Add Pre-Release Verification Command

**Phase:** 6
**Priority:** P1
**Labels:** `release`, `tests`, `tooling`

### Goal

Create one command or documented sequence for release verification.

### Scope

- Run unit tests, smoke tests, integration tests, corpus tests, docs checks,
  and build checks.
- Print environment and backend versions.
- Clearly mark skipped optional backends.

### Acceptance Criteria

- Maintainers can verify a release candidate with one documented command.
- The command distinguishes failure, skip, and unsupported environment.

## P6-04: Package and Installation Plan

**Phase:** 6
**Priority:** P1
**Labels:** `release`, `install`, `docs`

### Goal

Make installation and dependency setup predictable.

### Scope

- Document `go install` or release-binary installation.
- Document optional backend dependencies per language.
- Define adapter dependency bootstrap for ts-morph and OpenRewrite.
- Decide whether `refute` manages language servers or only detects them in
  the first real-usage release.

### Acceptance Criteria

- A new user can install `refute`, run `doctor`, and understand what works.
- Python, TypeScript, Java, Rust, and Go dependency setup is documented.

## P6-05: Versioning and Compatibility Policy

**Phase:** 6
**Priority:** P2
**Labels:** `release`, `api`, `docs`

### Goal

Set expectations for CLI, JSON, and MCP compatibility.

### Scope

- Define semantic versioning policy.
- State what can change before v1.0.
- Define compatibility guarantees for JSON and MCP schemas.
- Add changelog template.

### Acceptance Criteria

- Users and agents know which surfaces are stable.
- Future schema changes have a documented process.

## P3-01: Define MCP Tool Schemas

**Phase:** 3
**Priority:** P1
**Labels:** `mcp`, `agent-api`, `schema`

### Goal

Specify MCP tools before implementing transport.

### Scope

- Define schemas for `rename`, `extract_function`, `extract_variable`,
  `inline`, `list_symbols`, `list_backends`, `preview`, and `apply`.
- Reuse CLI JSON result shapes where practical.
- Include backend-specific extension fields without forcing every operation
  into a lowest-common-denominator shape.
- Add examples for success, ambiguity, unsupported operation, and backend
  missing.

### Acceptance Criteria

- Schemas are documented and reviewable.
- The design identifies which tools mutate files and which are read-only.
- The schema can represent backend-specific extension data.

## P3-02: Implement Stdio MCP Server Over Operation Layer

**Phase:** 3
**Priority:** P1
**Labels:** `mcp`, `architecture`, `agent-api`

### Goal

Expose current operations to agents without requiring a daemon first.

### Scope

- Add MCP stdio server entrypoint.
- Wire tools to the reusable operation layer from P1-05.
- Support preview-before-apply flow.
- Add protocol tests with fixture operations.

### Acceptance Criteria

- An MCP client can list symbols, preview a rename, and apply a rename.
- Tool results match the documented schemas.
- CLI behavior is not duplicated or forked.

## P3-03: Add MCP Safety and Audit Controls

**Phase:** 3
**Priority:** P1
**Labels:** `mcp`, `safety`, `agent-api`

### Goal

Make MCP usage safe for autonomous agents.

### Scope

- Separate read-only tools from mutating tools.
- Require explicit apply tool or equivalent opt-in for writes.
- Include dry-run diff/result IDs where useful.
- Add operation audit metadata to every result.

### Acceptance Criteria

- A client can enforce preview-before-apply.
- Mutating tools are clearly identifiable.
- Failed operations never report partial success.

## P5-01: Design Higher-Level Refactoring Extension Model

**Phase:** 5
**Priority:** P2
**Labels:** `architecture`, `refactoring`, `backend`

### Goal

Support richer operations without collapsing backend-specific power.

### Scope

- Define request/result structures for move and signature refactorings.
- Allow backend-specific parameter blocks.
- Define refusal behavior when a backend cannot support the operation safely.
- Document how advanced operations appear in capabilities and MCP schemas.

### Acceptance Criteria

- The design supports common cases and backend-specific extensions.
- No advanced operation is implemented before its safety/refusal behavior is
  specified.

## P5-02: Implement Move-To-File for One Reference Backend

**Phase:** 5
**Priority:** P2
**Labels:** `refactoring`, `backend`, `move`

### Goal

Prove the move operation end to end before broadening support.

### Scope

- Pick one backend with strong support, likely TypeScript/ts-morph or Go/gopls
  if available.
- Move a symbol or declaration to another file/module.
- Update imports and references.
- Add fixture and integration tests.

### Acceptance Criteria

- Move works for one documented language/backend.
- Unsupported languages refuse clearly.
- Support matrix marks all other languages accurately.

## P5-03: Implement One Signature Refactoring End to End

**Phase:** 5
**Priority:** P2
**Labels:** `refactoring`, `signature`, `backend`

### Goal

Prove the signature-refactoring model with one practical operation.

### Scope

- Pick one operation such as add parameter or remove parameter.
- Pick one backend with strong support.
- Update declaration and call sites.
- Add dry-run, apply, JSON, and fixture tests.

### Acceptance Criteria

- One signature operation is implemented, tested, and documented.
- The design can generalize to reorder parameters and parameter object work.
