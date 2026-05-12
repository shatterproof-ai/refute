<!-- headroom:rtk-instructions -->
# RTK (Rust Token Killer) - Token-Optimized Commands

When running shell commands, **always prefix with `rtk`**. This reduces context
usage by 60-90% with zero behavior change. If rtk has no filter for a command,
it passes through unchanged — so it is always safe to use.

## Key Commands
```bash
# Git (59-80% savings)
rtk git status          rtk git diff            rtk git log

# Files & Search (60-75% savings)
rtk ls <path>           rtk read <file>         rtk grep <pattern>
rtk find <pattern>      rtk diff <file>

# Test (90-99% savings) — shows failures only
rtk pytest tests/       rtk cargo test          rtk test <cmd>

# Build & Lint (80-90% savings) — shows errors only
rtk tsc                 rtk lint                rtk cargo build
rtk prettier --check    rtk mypy                rtk ruff check

# Analysis (70-90% savings)
rtk err <cmd>           rtk log <file>          rtk json <file>
rtk summary <cmd>       rtk deps                rtk env

# GitHub (26-87% savings)
rtk gh pr view <n>      rtk gh run list         rtk gh issue list

# Infrastructure (85% savings)
rtk docker ps           rtk kubectl get         rtk docker logs <c>

# Package managers (70-90% savings)
rtk pip list            rtk pnpm install        rtk npm run <script>
```

## Rules
- In command chains, prefix each segment: `rtk git add . && rtk git commit -m "msg"`
- For debugging, use raw command without rtk prefix
- `rtk proxy <cmd>` runs command without filtering but tracks usage
<!-- /headroom:rtk-instructions -->

## Repo Layout

- `cmd/` — CLI entrypoints (`cmd/refute/`).
- `internal/` — core packages: `backend/` (LSP and rewrite drivers), `cli/`,
  `config/`, `edit/` (edit applier), `symbol/`, `telemetry/`, plus
  cross-package integration tests.
- `adapters/` — non-Go adapters invoked as subprocesses (`openrewrite/`,
  `tsmorph/`).
- `docs/` — design notes, plans, and reference material.
- `scripts/` — repo automation and developer helpers.
- `testdata/` — fixtures consumed by tests.

## Build and Test Commands

```bash
go test ./...                                  # unit tests
go test -tags integration ./internal/...       # integration tests (need backends)
go vet ./...                                   # static checks
gofmt -l .                                     # formatting; output must be empty
govulncheck ./...                              # vulnerability scan
```

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

## Landing Work

Never use the pull request workflow. Always finish feature branches using the
`bento:land-work` skill, which merges directly to main.
