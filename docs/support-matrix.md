# Support Matrix

This document is the source of truth for what `refute` supports in the
current release. The runtime `refute doctor` command reflects the same data;
if the two ever disagree, the discrepancy is a bug.

## Status definitions

| Status | Meaning |
| --- | --- |
| **supported** | Documented, integration-tested, installable from a tagged release. Refactoring quality is bounded only by the underlying language server. |
| **experimental** | Implemented and tested in development, but missing one or more of: stable adapter packaging, integration coverage in CI, documented install path. May regress between releases. |
| **implemented but not packaged** | Code exists in the repo but cannot be reached from a `go install` build because adapter assets are repo-local. |
| **planned** | Acknowledged target. Not yet wired up or not yet covered by tests. |
| **unsupported** | Not on the roadmap for v0.1. Listed only to make the boundary explicit. |

A feature is **supported** only when all three are true: documentation exists
in this repo, integration coverage exists in CI or the local test suite, and a
user can install the required backend from a documented command. Anything
short of all three is at most **experimental**.

## Runtime status mapping

`refute doctor` reports local backend readiness, so some labels describe the
current host rather than release support. Interpret doctor statuses against the
matrix this way:

| Doctor status | Matrix meaning |
| --- | --- |
| `ok` | The dependency is present locally. The matrix status still controls whether the feature is supported or experimental. |
| `missing` | The dependency is absent locally. This is not a support level; install the hinted backend and re-check the matrix row. |
| `experimental` | The dependency is present for a matrix row that is still experimental. |
| `planned` | The dependency may be present, but the language or operation remains planned until tests and docs promote it. |
| `not-claimed` | The release boundary is explicit; this maps to **unsupported** in the matrix for v0.1. |

## Backend versions

`refute`'s determinism promise is conditional on running the same backend
versions, so the version is captured wherever it is observable:

- `refute doctor` probes each present backend for its version (`gopls version`,
  `rust-analyzer --version`, etc.) and reports it as a `version` line in the
  human output and a `version` field in each `--json` backend entry. Version
  capture is best-effort: the field is omitted when the binary cannot report a
  version.
- Successful operations carry the resolved backend's version in the JSON
  envelope's optional `backendVersion` field (`schemaVersion` is unchanged — the
  field is additive) and in telemetry's `backendVersion` field.
- CI pins `gopls` to a fixed tag (`GOPLS_VERSION` in `.github/workflows/ci.yml`)
  so a gopls release cannot silently change refactoring behavior. The unit-test
  lane sets `REFUTE_REQUIRE_GOPLS_INLINE=1` so a pinned gopls that drops the
  inline assist fails CI instead of silently skipping the inline test.

## Language matrix

| Language | Extensions | Backend | Dependency install | Operations | Test coverage | Status | Caveats |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Go | `.go` | `lsp/gopls` | `go install golang.org/x/tools/gopls@latest` | rename, extract-function, extract-variable, inline | unit + required integration (`internal/integration_test.go`) | supported | Primary v0.1 dogfood target. |
| Rust | `.rs` | `lsp/rust-analyzer` | `rustup component add rust-analyzer` | rename, extract-function, extract-variable, inline | unit + experimental integration (`internal/integration_test.go`; opt-in locally; non-blocking in CI) | experimental | Tier-1 --symbol supports forms 1–7 (crate::module::Type::method, <Type as Trait>::method). Inline is single-call-site only. Experimental while dogfood confidence is still building. |
| TypeScript | `.ts`, `.tsx` | `tsmorph` preferred; `lsp/typescript-language-server` fallback | `npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz`; fallback: `npm install -g typescript-language-server typescript` | rename | unit + experimental integration (`internal/integration_test.go`; opt-in locally; non-blocking in CI with fixture dependencies installed) | experimental | The adapter is a separate dependency distributed from GitHub Releases rather than the npm registry; fallback is rename-only LSP coverage. The adapter discovers root and nested `tsconfig.json`/`jsconfig.json` files outside `node_modules`. |
| JavaScript | `.js`, `.jsx` | `tsmorph` preferred; `lsp/typescript-language-server` fallback | `npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz`; fallback: `npm install -g typescript-language-server typescript` | rename | unit + experimental integration (`internal/integration_test.go`; opt-in locally; non-blocking in CI with fixture dependencies installed) | experimental | Same adapter and fallback caveats as TypeScript; the adapter discovers root and nested `tsconfig.json`/`jsconfig.json` files outside `node_modules`. |
| Python | `.py` | `lsp/pyright` | `npm install -g pyright` | rename | none | planned | Promote once fixture and integration coverage land. |
| Java | `.java` | OpenRewrite | — | — | none | unsupported | Not claimed for v0.1. JAR packaging deferred. |
| Kotlin | `.kt` | OpenRewrite | — | — | none | unsupported | Not claimed for v0.1. |

## How operations map to backends

The `Operations` column lists the refactorings that route through the
backend in the current release. Operations not listed return the
`unsupported` JSON status when invoked.

In `--json` mode every operation command (`rename`, `extract-function`,
`extract-variable`, and `inline`) emits exactly one structured envelope on
stdout for both success and failure. Failure envelopes carry the matching
status from `internal/edit/json.go`: `backend-missing` when the language
server is absent, `unsupported` for an operation the backend does not provide,
`invalid-position` when a symbol cannot be resolved, and `backend-failed`
(error code `apply-failed`) when applying edits fails after the preview is
computed.

For Tier 1 rename (`refute rename --symbol pkg.Func --new-name New`) the
backend is selected from the file's extension once the symbol is resolved,
following the same rules as Tier 2.

## Integration lane policy

The required CI integration lane covers the supported Go path only. CI installs
`gopls` and runs `go test -tags integration ./internal/...` without experimental
opt-in so Rust, TypeScript, JavaScript, and unsupported-language fixtures skip
with an explicit opt-in message.

Experimental integration scenarios are available by setting
`REFUTE_EXPERIMENTAL_INTEGRATION=1`. The CI workflow runs the same integration
suite in a separate non-blocking lane after installing `rust-analyzer` and the
TypeScript/JavaScript fixture dependencies. Local runs should use the same
environment variable when intentionally exercising these experimental paths.

## Process for promoting a status

To move a language from **experimental** to **supported**:

1. Add or extend integration tests under `internal/integration_test.go`
   (build tag `integration`) that exercise rename and any other claimed
   operations end-to-end.
2. Confirm CI runs those integration tests with the dependency installed.
3. Update this matrix and `refute doctor`'s caveat string in lockstep.
4. Reference the promotion in the next release's changelog.

To move a language from **implemented but not packaged** to **experimental**:

1. Solve adapter packaging (e.g., GitHub release tarball package, or
   `//go:embed` of a self-contained bundle).
2. Update `refute doctor` so the install hint reflects the actual
   distribution path.
3. Update this matrix.

## Tier-1 Qualified-Name Resolution

`--symbol` accepts qualified names resolved via `workspace/symbol`:

| Language | Example | Notes |
|---|---|---|
| Go | `pkg.FunctionName`, `Type.Method` | dot-separated |
| Rust | `greet::format_greeting`, `<Greeter as Display>::fmt` | forms 1–7 per `docs/specs/2026-04-22-rust-parity-design.md` |

## Missing-Server Install Hints

When `refute` cannot find a language server it prints an install hint. Sources:

| Language | Install |
|---|---|
| Go | `go install golang.org/x/tools/gopls@latest` |
| Rust | `rustup component add rust-analyzer` |
| TypeScript | `npm install -g typescript-language-server typescript` |
| TypeScript adapter | `npm install -g https://github.com/shatterproof-ai/refute/releases/download/v0.1.0/refute-ts-adapter-0.1.0.tgz` |
| Python | `npm install -g pyright` |

These hints mirror `refute doctor` because both are derived from the single
`SupportMatrix` table in `internal/config`; the table also feeds the
missing-server error shown by `refute rename`, so the three can never drift.
