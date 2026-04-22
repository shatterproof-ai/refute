# Java End-to-End Test Coverage Plan

Date: 2026-04-22
Expedition: `java-language-support` (task 07, slug `java-e2e-tests`)
Branch: `java-language-support-07-java-e2e-tests`

## Goal

Extend end-to-end integration coverage for Java/Kotlin rename refactoring
beyond the single `TestEndToEnd_RenameJavaMethod` case. Exercise both backends
(OpenRewrite primary, jdtls fallback), both rename kinds (method, class), both
symbol-resolution paths (positional and FQN), dry-run, and overload targeting.

This task writes the plan only. The tests themselves are implemented in a
follow-up task.

## Current Coverage

`internal/integration_test.go` currently has one Java test:

- `TestEndToEnd_RenameJavaMethod` — jdtls-backed method rename on the
  two-class Greeter/Main fixture. Skips when `jdtls` is not on `PATH`.

The OpenRewrite backend (`internal/backend/openrewrite/`) has unit tests for
its text-parsing helpers (`javaTypeFQN`, `javaMethodPatternPrefix`) but zero
integration tests. The full path — Go adapter → JVM subprocess → OpenRewrite
recipe → file edits — is untested end-to-end.

## Design Decisions

Resolved before drafting:

1. **OpenRewrite JAR handling** — Tests skip when the fat JAR is absent,
   mirroring the jdtls skip pattern. Devs run `mvn package` once in
   `adapters/openrewrite/` to build the JAR. CI steps that want real coverage
   build the JAR explicitly before running tests. Rationale: avoids coupling
   every test run to a Maven/JDK toolchain; keeps local `go test` fast.

2. **Symbol-resolution paths in scope** — Both positional
   (`--file --line --name`) and `--symbol com.example.Foo#bar` FQN are in
   scope. FQN coverage requires implementing `FindSymbol` for Java in
   `internal/backend/openrewrite/adapter.go`; a subtask covers this.

3. **Fixture organization** — One fixture directory per scenario under
   `testdata/fixtures/java/`, e.g. `rename-class/`, `rename-method-overload/`.
   Each fixture is a minimal self-contained Maven project. Isolation is worth
   the small duplication cost — a scenario-specific fixture documents intent
   and avoids cross-test coupling.

## Gaps To Close

Each gap maps to at least one scenario. Scenarios are the planning unit; a
single scenario may produce 2 tests (OpenRewrite + jdtls fallback).

| # | Gap                                         | Backend(s)            | Fixture                           |
|---|---------------------------------------------|-----------------------|-----------------------------------|
| 1 | OpenRewrite end-to-end method rename        | OpenRewrite           | reuse `rename/`                   |
| 2 | Class rename (covers task-6 FQN fix)         | OpenRewrite, jdtls    | `rename-class/`                   |
| 3 | Dry-run for Java                            | OpenRewrite           | reuse `rename/`                   |
| 4 | Overloaded-method targeting                 | OpenRewrite           | `rename-method-overload/`         |
| 5 | `--symbol` FQN method rename                | OpenRewrite           | reuse `rename/`                   |
| 6 | `--symbol` FQN class rename                 | OpenRewrite           | `rename-class/`                   |
| 7 | Fallback: OpenRewrite JAR missing → jdtls   | selector              | reuse `rename/`                   |

Kotlin coverage is explicitly out of scope for this task. It is captured as a
follow-up in the expedition's final handoff.

## Scenarios

### S1. OpenRewrite method rename (positional)

- Fixture: reuse `testdata/fixtures/java/rename/` (Greeter + Main).
- Skip condition: `adapters/openrewrite/target/openrewrite-fatjar.jar` absent.
- Invocation: `refute rename-method --file Greeter.java --line 4 --name greet --new-name hello`.
- Assertions: `greet(` gone from both files; `hello(` present in both.
- Test name: `TestEndToEnd_OpenRewrite_RenameJavaMethod`.

### S2. Class rename — OpenRewrite and jdtls

- Fixture: new `testdata/fixtures/java/rename-class/` with classes
  `com.example.Widget` and `com.example.Main`, where `Main` imports and
  instantiates `Widget` multiple times. Include a `pom.xml` matching the
  existing fixture shape.
- Two tests:
  - `TestEndToEnd_OpenRewrite_RenameJavaClass` — skip when JAR absent.
  - `TestEndToEnd_Jdtls_RenameJavaClass` — skip when `jdtls` absent; force
    jdtls backend (env var or CLI flag, see S7 on selector override).
- Invocation: `refute rename-class --file Widget.java --line 3 --name Widget --new-name Gadget`.
- Assertions: class declaration renamed, import lines updated, type
  references in `Main.java` updated, file renamed on disk
  (`Widget.java` → `Gadget.java`) if renamer supports it (otherwise note as
  a follow-up).
- This scenario locks in the task-6 FQN fix: without it, OpenRewrite would
  mangle the fully-qualified type name and the test would fail.

### S3. Dry-run for Java

- Fixture: reuse `rename/`.
- Test name: `TestEndToEnd_OpenRewrite_JavaDryRun` (mirroring
  `TestEndToEnd_DryRun` and `TestEndToEnd_TypeScriptDryRun`).
- Run the S1 invocation with `--dry-run` appended.
- Assertions: diff output contains both old and new names; files on disk
  unchanged after the command returns. Assertion wording follows the Go/TS
  dry-run tests for consistency.

### S4. Overloaded method targeting

- Fixture: new `testdata/fixtures/java/rename-method-overload/` with
  `com.example.Greeter` declaring two `greet` methods of different arities
  (`greet()` and `greet(String name)`), and a `Main` that calls both.
- Test name: `TestEndToEnd_OpenRewrite_RenameOverloadedJavaMethod`.
- Invocation targets the zero-arg overload by line/column (position of the
  first `greet` declaration).
- Assertions: only the zero-arg declaration and zero-arg call site are
  renamed; the `greet(String)` declaration and its call site are unchanged.
- Open design sub-question to resolve during implementation: OpenRewrite's
  `ChangeMethodName` by default renames all overloads unless a method
  pattern pins the parameter types. This scenario is the forcing function
  for adding overload-preserving pattern construction to the Go adapter.
  If the adapter cannot yet target a specific overload, the test is written
  first with `t.Skip("overload targeting not yet supported")` and the
  implementation task is filed as a follow-up.

### S5. `--symbol` FQN method rename

- Prerequisite: implement `FindSymbol` for Java in
  `internal/backend/openrewrite/adapter.go`. Minimum support: parse
  `package com.example;` declarations, walk fixture files, match
  `com.example.Greeter#greet` → `Greeter.java` line N.
- Fixture: reuse `rename/`.
- Test name: `TestEndToEnd_OpenRewrite_RenameJavaMethodBySymbol`.
- Invocation: `refute rename-method --symbol com.example.Greeter#greet --new-name hello`.
- Assertions: same as S1.

### S6. `--symbol` FQN class rename

- Fixture: reuse `rename-class/`.
- Test name: `TestEndToEnd_OpenRewrite_RenameJavaClassBySymbol`.
- Invocation: `refute rename-class --symbol com.example.Widget --new-name Gadget`.
- Assertions: same as S2 OpenRewrite variant.

### S7. Fallback when OpenRewrite JAR is missing

- Fixture: reuse `rename/`.
- Mechanism: the existing selector picks OpenRewrite first; it falls back
  to jdtls when the JAR is missing. Test simulates a missing JAR by pointing
  `REFUTE_OPENREWRITE_JAR` (or the equivalent config) to a non-existent path
  for the duration of the test. Requires `jdtls` on `PATH`.
- Test name: `TestEndToEnd_JavaBackendFallback_OpenRewriteMissing`.
- Skip condition: `jdtls` absent.
- Assertions: rename succeeds (S1 outcome), and stderr/log contains a
  clear fallback indication. If no such indicator exists yet, the
  implementation task adds one — a user-visible fallback log is worth
  having regardless of tests.
- Open sub-question: does the backend selector support an env-var override,
  or does the fake-path approach require a new CLI flag? Resolve during
  implementation; do not block the plan on it.

## FindSymbol Implementation Sub-task (Prereq for S5, S6)

Minimum viable `FindSymbol(symbol string)` for Java under the OpenRewrite
adapter:

1. Parse the `--symbol` string: `package.Class` or `package.Class#method`.
2. Walk the workspace for `*.java` files.
3. For each file, read the `package` declaration; if it matches the target
   package, scan for the class declaration, then (for methods) scan the
   class body for a matching method name. Return the first hit as
   `{file, line, column, name}`.
4. If no hit, return `ErrSymbolNotFound` so the CLI can print a clear error.

This is a Tier 1 resolver — no LSP, no AST library. Regex + lightweight
line scanning is sufficient for fixture-scale projects. Tests S5/S6 are
the sole coverage for it in this iteration; fuller AST-based resolution
is out of scope.

## Verification Gates

The plan's acceptance is a human review. The implementation task that
follows uses the expedition's standard gates:

```bash
go build ./...
go vet ./...
go test ./...
```

Plus, with toolchains available:

```bash
# OpenRewrite scenarios — require the fat JAR
(cd adapters/openrewrite && mvn -q -DskipTests package)
go test -tags=integration ./internal/...
```

## Deliverable Order (for the follow-up task)

Suggested sequence, smallest to largest:

1. S3 (dry-run, reuses existing fixture) — warms up OpenRewrite invocation path.
2. S1 (OpenRewrite method rename, reuses fixture).
3. S2 (class rename, new fixture + OpenRewrite + jdtls variants).
4. FindSymbol helper.
5. S5, S6 (FQN symbol paths).
6. S4 (overload fixture + targeting work).
7. S7 (fallback, depends on settling the JAR-missing override mechanism).

Each scenario lands as its own commit on the implementation task branch.

## Out Of Scope

- Kotlin rename E2E — separate expedition task, tracked in final handoff.
- `extract-method`, `extract-variable`, or any non-rename refactoring.
- Cross-module Java projects (multi-`pom.xml` setups).
- Gradle-backed fixtures.
- Performance/large-codebase fixtures.

## Open Questions For User Review

1. Should S2 assert that OpenRewrite renames the file on disk
   (`Widget.java` → `Gadget.java`)? OpenRewrite's `ChangeType` does this by
   default, but it conflicts with jdtls's behavior (which edits the file
   in place and leaves the filename). If both backends must match, jdtls
   needs a post-pass; if not, S2's OpenRewrite and jdtls variants assert
   different outcomes.

2. The S7 fallback mechanism: preferred override form — env var
   (`REFUTE_OPENREWRITE_JAR=/nonexistent`), a CLI flag
   (`--backend=jdtls`), or a config-file knob? Any is implementable; the
   decision affects plumbing scope for the follow-up task.

3. Should the follow-up task be a single large task, or split into
   (a) FindSymbol + fixtures, (b) S1–S4 tests, (c) S5–S7 tests? Splitting
   makes task boundaries cleaner at the cost of more expedition overhead.
