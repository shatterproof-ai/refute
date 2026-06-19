# refute — Agent Instructions

`refute` is a symbol-aware, multi-language refactoring tool. It drives language
servers (LSP) and language-specific rewrite tools behind a single CLI so that
operations like rename are computed by real backends rather than text
substitution. Go is the primary implementation language; Java, TypeScript,
Rust, and Python backends are invoked as subprocess adapters.

> Run shell commands prefixed with `rtk` for token-efficient output (a
> maintainer's local tool; it passes commands through unchanged, so skip the
> prefix if `rtk` is unavailable).

## Repo Layout

- `cmd/` — CLI entrypoints (`cmd/refute/`) and the lockfile-bootstrap shim
  (`cmd/refute-tool/`).
- `internal/` — core packages: `backend/` (LSP and rewrite drivers), `cli/`,
  `config/`, `edit/` (edit applier), `symbol/`, `language/` (path-based
  language detection), `buildgraph/` (build-graph guard tests), `toolchain/`
  (lockfile + toolchain sync), `telemetry/`, plus cross-package integration
  tests (`internal/integration_test.go` and friends).
- `adapters/` — non-Go subprocess adapters. Rewrite backends: `openrewrite/`
  (Java) and `tsmorph/` (TypeScript). Registryless packaging shims that
  redistribute the `refute-tool` bootstrap: `cargo/`, `jvm/`, `npm/`,
  `python/`.
- `docs/` — design notes, plans (`docs/plans/`), and intent stories
  (`docs/stories/`); start with `docs/README.md` for current contracts vs
  historical artifacts.
- `scripts/` — repo automation and developer helpers.
- `testdata/` — fixtures consumed by tests.

## Build and Test Commands

```bash
go test ./...                                  # unit tests
go test -tags integration ./internal/...       # integration tests (need backends)
go vet ./...                                   # static checks
gofmt -l .                                     # formatting; output must be empty
govulncheck ./...                              # vulnerability scan
make verify-report                             # full audit: every gate, keep-going summary
```

For a full audit, run `make verify-report`: it runs every verification gate to
completion even when one fails (so a single failure never hides later checks) and
reports each as `PASS` / `FAIL` / `SKIP` / `UNAVAIL`. `make verify` is the
fast-feedback variant that stops at the first failing gate. See
[`docs/release-verification.md`](docs/release-verification.md) for the full
contract.

## Integration Backend Prerequisites

Integration tests shell out to real language servers and rewrite tools. Install
the backend(s) you intend to exercise before running the integration tag:

- Go: `gopls` on `PATH` (`go install golang.org/x/tools/gopls@latest`).
- Rust: `rust-analyzer` on `PATH`.
- TypeScript: the `tsmorph` adapter under `adapters/tsmorph/` (Node dependencies
  installed; see that directory's README).
- Java: OpenRewrite via the `adapters/openrewrite/` adapter.

Tests that cannot find their backend skip rather than fail; check skip output
when a backend appears silent.

## Adding or Updating a Backend

- LSP-driven backends live in `internal/backend/lsp/`. Register a new language
  by extending the selector in `internal/backend/selector/` and the dispatch in
  `internal/backend/backend.go`.
- Non-LSP rewrite backends live under `internal/backend/openrewrite/` and
  `internal/backend/tsmorph/`, with their subprocess adapters under
  `adapters/`. Update both the in-process driver and the adapter together when
  changing the wire contract.
- Add or expand fixtures under `testdata/` and integration coverage under
  `internal/integration_test.go` (build tag `integration`).

## Agent Tooling Artifacts

Tool configs and repo-local skills are tracked. Generated outputs (caches,
seeds, reports, binaries) are gitignored in the same change that introduces the
generator. State from abandoned tools is deleted — never renamed and left in
the tree.

## Issue-Backed Work

Every implementation or documentation change must be backed by a GitHub issue
before work begins. If the user reports a bug or requests a change directly in
chat, create or identify the GitHub issue first, then reference that issue while
working.

Do not treat an unavailable or misconfigured tracker CLI as permission to
continue without an issue. Stop and surface the tracker problem so the issue can
be created or selected before repository files are edited.

Tracker-only administration, such as creating or updating the issue itself, does
not require a separate issue.

## Issue Traceability

Feature branches are named `<flow>/issue-<N>-<slug>`; the merge commit subject
references `#<N>`. Work without an issue number in branch or merge subject is
not landed.

## Landing Work

Never use the pull request workflow for originating work. Always finish feature
branches using the `bento:land-work` skill, which merges directly to main.

## External Contributions

Maintainer/agent-originated work lands via `bento:land-work` (no PRs). External
contributions arrive as GitHub PRs; a maintainer reviews, then lands the commits
via the normal flow. The "never use PRs" rule applies to originating work, not
to receiving it. See `CONTRIBUTING.md` for the human-contributor entry point.

## Intent Stories

User-facing capability descriptions live in `docs/stories/`. Consult them before changing user-facing behavior and update them when behavior changes.
