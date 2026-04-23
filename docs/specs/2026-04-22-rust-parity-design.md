# Rust Parity — Design Spec

**Date:** 2026-04-22
**Status:** Design approved; implementation plan pending.
**Supersedes:** n/a. Extends the closed `rust-support` expedition (basic rename wiring) with code-action parity, Tier-1 qualified-name resolution, and hardening.
**Prerequisite:** `docs/plans/2026-04-17-go-code-actions-tier1-v2.md` (the "Go tier-1 v2" plan) must land first. This spec assumes the infrastructure that plan introduces.

---

## Goal

Bring Rust support in `refute` to parity with Go tier-1: extract-function, extract-variable, inline (single call site), plus qualified-name symbol resolution (`--symbol "crate::module::Type::method"`) via `workspace/symbol`. Harden the existing rename path with tests for non-top-level symbols, improve the missing-server error experience, add a support-matrix document, and strip snippet placeholders from code-action edits before applying.

## Non-Goals (v1)

- Definition-wide inline (inline into all callers). The plumbing exists as `opInlineAllCallers` but is not wired to a CLI flag.
- Rust macro expansion rename. Not asserted by tests.
- `MoveToFile` for Rust.
- Generic-instantiation narrowing in symbol resolution.
- Tree-sitter-based parsing of qualified names.
- Any direct `cargo` or `rustc` invocation by refute outside of integration tests.

## Baseline (what already exists)

- `rust-analyzer` registered as a built-in LSP server (`internal/config/config.go`).
- `.rs` extension → `rust` language mapping (`internal/cli/rename.go`).
- `Cargo.toml` recognized as a workspace-root marker.
- Indexing wait and `ContentModified` retry logic in the LSP adapter (`internal/backend/lsp/adapter.go`, `client.go`).
- Fixture at `testdata/fixtures/rust/rename/` with a Cargo package (`lib.rs`, `main.rs`) containing `format_greeting` + `Greeter`.
- Integration tests: `TestEndToEnd_RenameRustFunction`, `TestEndToEnd_RustDryRun`, `TestEndToEnd_RenameRustStruct`.

## Architecture

Rust parity reuses the Go tier-1 v2 surface verbatim:

- `Client.CodeActions`, `Client.ResolveCodeActionEdit`, `Client.WorkspaceSymbol` — LSP methods from Go v2.
- Adapter methods: `FindSymbol`, `ExtractFunction`, `ExtractVariable`, `InlineSymbol` — defined by Go v2.
- `cli/workspace.go`, `cli/errors.go`, `cli/extract.go`, `cli/inline.go`, `edit/json.go` — created by Go v2.

Language-specific pieces added here:

- **Priming walker** (`rust_priming.go`): open up to `maxPrimedFiles` `*.rs` files under the workspace root, skipping `target/`, `.git/`, `node_modules/`, `.cargo/`. Mirrors the existing `PrimeTSWorkspace` pattern.
- **Code-action matcher** (`rust_actions.go`): lookup table of `(Kind prefix, Title regex)` per operation. Prefix-matches `refactor.extract`, `refactor.extract.function`, etc. Regex on the human-readable title disambiguates extract-function vs extract-variable, inline-at-call vs inline-all-callers.
- **Container parser** (`rust_container.go`): normalizes the `containerName` field from `workspace/symbol` results into `(modulePath []string, trait string, type string)`. Branches on Task-0 spike outcome — see Risk section.
- **Qualified-name parser** (`cli/rust_symbol.go`): hand-rolled parser for user CLI input; see Qualified-Name Syntax below.
- **Generalized missing-server error** (`cli/errors.go`): `ErrLSPServerMissing` with install-hint field; Rust is the first consumer, type is reusable for Go/TS.
- **Snippet-placeholder stripping** (`edit/snippet.go`): `stripSnippetPlaceholders` removes `$0`, `${1:name}`, `${2|a,b|}` tokens introduced by rust-analyzer assists before edits reach the applier.

The adapter grows a language-dispatch layer: `Adapter.PrimeWorkspace()` picks the right walker; `Adapter.matchAction(op)` picks the right filter table. No Rust-specific logic lives in the shared adapter file.

## Qualified-Name Syntax

`--symbol` input accepts seven forms. The parser splits on `::` with angle-bracket balancing for trait qualification:

| Form | Example | Parsed as `(containerPath, trait, name)` |
|---|---|---|
| 1 | `format_greeting` | `([], "", "format_greeting")` |
| 2 | `greet::format_greeting` | `(["greet"], "", "format_greeting")` |
| 3 | `crate::greet::format_greeting` | `(["greet"], "", "format_greeting")` — `crate::` stripped |
| 4 | `Greeter::new` | `(["Greeter"], "", "new")` |
| 5 | `Greeter::greet` | `(["Greeter"], "", "greet")` |
| 6 | `<Greeter as Display>::fmt` | `(["Greeter"], "Display", "fmt")` |
| 7 | `greet::Greeter::new` | `(["greet", "Greeter"], "", "new")` |

Form 6 combined with form 7 (`greet::<Greeter as Display>::fmt`) is also accepted; module path precedes the bracketed type.

**Parser algorithm:**

1. Trim whitespace.
2. Reject inputs containing `(`, `)`, `{`, `}`, `;`, or interior whitespace — likely source-code paste errors.
3. Scan for a leading `<`. If found, walk forward tracking angle-bracket depth until the matching `>` is consumed by `>::` — this is form 6. Parse the bracket interior as `Type as Trait`, and the remainder after `>::` as the name.
4. Otherwise split on `::`. Strip a leading `crate` segment if present. Last segment is `name`; the rest is `containerPath`.

**Resolution:**

After calling `client.WorkspaceSymbol(name)`, filter candidates:

- Name matches exactly (case-sensitive).
- Container path matches: empty `containerPath` accepts any container; non-empty requires the candidate's `containerName` (normalized across rust-analyzer's formatting variants) to end with the joined path.
- Trait match (form 6 only) — see Risk section below for cheap vs expensive branch.

Outcomes: exactly one match proceeds; zero matches returns `ErrSymbolNotFound` with the parsed components in the message; multiple matches returns `status: "ambiguous"` JSON with a `candidates` array.

## Code-Action Matching

```go
type rustActionPattern struct {
    kind        string
    titleRegexp *regexp.Regexp
}

var rustActionPatterns = map[rustActionOp]rustActionPattern{
    opExtractFunction:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*function`)},
    opExtractVariable:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*variable`)},
    opInlineCallSite:   {"refactor.inline", regexp.MustCompile(`(?i)^inline( call)?$`)},
    opInlineAllCallers: {"refactor.inline", regexp.MustCompile(`(?i)inline .*all callers`)},
}
```

- `Kind` match is prefix-based (`refactor.extract` accepts `refactor.extract.function`).
- `Title` match is case-insensitive regex; resilient to minor rust-analyzer wording changes.
- Unit tests pin current rust-analyzer titles; CI fails with a clear diagnostic (listing offered titles) if no action matches a pattern.
- `opInlineAllCallers` is defined but not exposed via CLI in v1. Reserved for a follow-up plan.

## Inline Semantics (I2)

Inline targets a single call site by default. The CLI accepts two shapes:

```
refute inline --file <path> --line <n> --column <n>
refute inline --symbol <qualified-name> --call-site <file>:<line>:<column>
```

**Rule:** `--symbol` with inline always requires `--call-site`. A definition-position resolution without a call site is rejected with: *"symbol resolved to a definition; inline requires a call site. Pass --call-site <file>:<line>:<column>."*

**Flow:**

1. Resolve target position (direct flags, or `--call-site`).
2. Call `client.CodeActions(uri, range)`; filter via `matchRustAction(..., opInlineCallSite)`.
3. If no matching action: `ErrActionNotOffered` with rust-analyzer's offered titles in the message.
4. Resolve edits via `client.ResolveCodeActionEdit` when the action's `edit` is absent.
5. Apply or dry-run via the existing edit applier.

## Hardening

| ID | Change | Location |
|---|---|---|
| H1 | Local-variable rename integration test | `internal/integration_test.go` |
| H2 | Parameter rename integration test | `internal/integration_test.go` |
| H3 | `ErrLSPServerMissing` with per-server install hints | `internal/cli/errors.go` + `internal/config/config.go` |
| H4 | Strip `$0` / `${N:...}` / `${N|a,b|}` snippet placeholders from code-action edits | `internal/edit/snippet.go` (new) |
| H5 | `docs/support-matrix.md` — language × operation × backend | new |
| H6 | `--language` flag help text includes `rust` | `internal/cli/rename.go`, `extract.go`, `inline.go` |

H3 applies to every LSP-backed language; refactor is language-agnostic, Rust is the first beneficiary.
H4 is triggered whenever a `WorkspaceEdit` contains `$N` / `${N:...}` / `${N|...|}` tokens in `newText`.

## File Structure

```
internal/backend/lsp/
├── rust_actions.go          CREATE — pattern table + matchRustAction
├── rust_actions_test.go     CREATE — table-driven tests + rust-analyzer drift guard
├── rust_priming.go          CREATE — PrimeRustWorkspace
├── rust_priming_test.go     CREATE — walker skip-list + cap tests
├── rust_container.go        CREATE — parseRustContainer; cheap/expensive branch
├── rust_container_test.go   CREATE — parser tests
├── adapter.go               MODIFY — language-dispatch PrimeWorkspace, matchAction

internal/cli/
├── rust_symbol.go           CREATE — parseRustQualifiedName (forms 1-7)
├── rust_symbol_test.go      CREATE — form coverage + error cases
├── rename.go                MODIFY — dispatch Rust qualified names
├── extract.go               MODIFY — language-aware action match (Go v2 creates file)
├── inline.go                MODIFY — add --call-site flag; require it for --symbol
├── errors.go                MODIFY — add ErrLSPServerMissing (Go v2 creates file)

internal/edit/
├── snippet.go               CREATE — stripSnippetPlaceholders
├── snippet_test.go          CREATE — round-trip coverage

internal/integration_test.go MODIFY — add tests listed in Integration Tests section

testdata/fixtures/rust/rename/
├── Cargo.toml               KEEP
├── src/lib.rs               EXTEND — multi-trait impl, extractable fn, variable/param rename targets
├── src/main.rs              EXTEND — cross-file call site for inline
├── src/util.rs              CREATE — form-7 qualified-name target

docs/
├── specs/2026-04-22-rust-parity-design.md   THIS FILE
├── plans/2026-04-22-rust-parity.md          CREATE (writing-plans phase)
├── plans/2026-04-22-rust-parity-log.md      CREATE (writing-plans phase)
├── plans/2026-04-22-rust-parity-handoff.md  CREATE (writing-plans phase)
└── support-matrix.md                        CREATE — H5
```

## Integration Tests

| Test | Operation | Verifies |
|---|---|---|
| `TestEndToEnd_RenameRustLocalVariable` | rename local var | H1; non-top-level rename |
| `TestEndToEnd_RenameRustParameter` | rename parameter | H2; parameter-scope rename |
| `TestEndToEnd_ExtractRustFunction` | extract-function | code action + `cargo build` |
| `TestEndToEnd_ExtractRustVariable` | extract-variable | `--name` honored via placeholder rewrite |
| `TestEndToEnd_InlineRustCallSite` | inline (I2) | single call site replaced; definition untouched |
| `TestEndToEnd_InlineRustRequiresCallSite` | inline with `--symbol`, no `--call-site` | error mentions `--call-site` |
| `TestEndToEnd_Tier1RustRename` | `--symbol "greet::Greeter::new"` | form 7 resolves |
| `TestEndToEnd_Tier1RustTraitQualified` | `--symbol "<Greeter as Display>::fmt"` | form 6 disambiguates |
| `TestEndToEnd_Tier1RustAmbiguous` | `--symbol "fmt"` | ambiguous → JSON `candidates` |
| `TestEndToEnd_Tier1RustNotFound` | bogus symbol | `ErrSymbolNotFound` exit code + message |
| `TestEndToEnd_RustAnalyzerMissing` | scrubbed PATH | H3; typed error + install hint |
| `TestEndToEnd_RustSnippetPlaceholderStripped` | extract with `--name foo` | H4; no `$0`/`${N:...}` in output |

Every test skips when `rust-analyzer` is absent, except `TestEndToEnd_RustAnalyzerMissing` which requires the absence.

## Support Matrix (H5)

`docs/support-matrix.md` is the source of truth, referenced by README and by `docs/specs/2026-04-15-refute-design.md`. Initial contents:

| Language   | LSP Server                       | Workspace Marker         | Rename | Extract Fn | Extract Var | Inline |
|------------|----------------------------------|--------------------------|--------|------------|-------------|--------|
| Go         | gopls                            | `go.mod`, `go.work`      | ✅     | ✅         | ✅          | ✅     |
| Rust       | rust-analyzer                    | `Cargo.toml`             | ✅     | ✅         | ✅          | ✅ (1) |
| TypeScript | typescript-language-server       | `package.json`, `tsconfig.json` | ✅ | ❌       | ❌          | ❌     |
| Python     | pyright (fallback only)          | `pyproject.toml`         | ⚠️     | ❌         | ❌          | ❌     |

(1) Single call site only. Definition-wide inline is a planned follow-up.

## Risk: Task-0 Empirical Spike

Form-6 (`<Type as Trait>::method`) resolution cost depends on what rust-analyzer puts in `workspace/symbol` result's `containerName` for a method inside `impl Display for Greeter`. Behavior is not documented; the plan's first task runs rust-analyzer against the multi-trait fixture and branches on the observation:

- **Cheap branch** — `containerName` includes the trait (e.g., `"impl Display for Greeter"` or `"Greeter (Display)"`): ~10 lines of substring matching in `rust_container.go`.
- **Expensive branch** — `containerName` is just `"Greeter"`: the plan adds `Client.DocumentSymbol` to the LSP client, fetches hierarchical document symbols per candidate location, walks ancestors for an `impl <Trait> for <Type>` node, and filters by the requested trait. ~100–200 additional lines, its own test file, and exposure to rust-analyzer output-format drift.

The spike's outcome determines scope. The plan writes both branches as conditional tasks; only the branch the spike selects runs.

## Deferred Follow-Ups

- Definition-wide inline (`opInlineAllCallers` exposure via `--inline-all` flag or `inline-all` command).
- Rust macro-expansion rename handling.
- `MoveToFile` for Rust.
- Generic-instantiation narrowing in `workspace/symbol` matching.
- Tree-sitter migration of `parseRustQualifiedName` — trigger when tree-sitter-rust arrives for another reason (e.g., structural-pattern matching moves in-process).
- Virtual-manifest Cargo workspace testing.

## Expedition Posture

- Expedition name: `rust-parity`.
- Branch: `feature/rust-parity` via `bento:launch-work` in a linked worktree; current dirty `e2e-test-coverage` branch is untouched.
- Landing: task-by-task direct merge to `main` per global instructions — no PRs.
- Handoff protocol: same pattern as Go tier-1 v2 (plan + log + handoff files in `docs/plans/`).
