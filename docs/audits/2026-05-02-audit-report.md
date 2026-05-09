# Refute Audit Report — 2026-05-02

Run against `main` at `53028c0fb0f31876e968ccc1d7a5bc6b08f3f4a0`.
Procedure: `docs/plans/2026-05-02-audit-plan.md`.

## Executive summary

| Severity | Count |
| --- | --- |
| blocker | 0 |
| high    | 6 |
| medium  | 11 |
| low     | 4 |

Top items requiring attention:

1. **No CI** — `.github/` is missing entirely; every documented gate is
   advisory.
2. **Edit applier is not atomic across files** — `internal/edit/applier.go`
   doc claims atomicity across files, but Phase 3 leaves partially-renamed
   files committed if any rename fails after the first.
3. **4/4 of the Go end-to-end integration suite fails** (locally,
   reproducibly) because the test does not pass `-buildvcs=false` to its
   post-rename `go build` check, so a transient git ownership / VCS-probe
   issue surfaces as a test failure even though the rename itself worked.

## Phase 1 — Build health

- `go build ./...` — pass.
- `go vet ./...` — pass.
- `go test ./... -timeout 90s` — 64 passed across 11 packages.
- `go test -tags integration ./internal/... -timeout 120s` — 81 passed,
  4 failed, 1 skipped:

  - `[high] internal/integration_test.go:63 — TestEndToEnd_RenameGoFunction`
  - `[high] internal/integration_test.go:662 — TestEndToEnd_RenameGoType`
  - `[high] internal/integration_test.go:837 — TestEndToEnd_ExtractFunction`
  - `[high] internal/integration_test.go:877 — TestEndToEnd_Tier1Rename`

  Each failure: `project no longer compiles after rename: error obtaining
  VCS status: exit status 128 — Use -buildvcs=false to disable VCS
  stamping.` The rename itself succeeds (string-content assertions earlier
  in the test pass); only the `go build ./...` invocation that the test
  uses to verify post-rename compilability fails. The test does not pass
  `-buildvcs=false`, so an environmental git-VCS probe error surfaces as a
  refute failure. **Remediation:** in
  `internal/integration_test.go`, set `goCheck.Env = append(os.Environ(),
  "GOFLAGS=-buildvcs=false")` (or pass the flag explicitly) for every
  post-action `go build` exec. Apply across all four call sites.

- Adapters:
  - `adapters/tsmorph/` — no `test` script; nothing to run. See Phase 3.
  - `adapters/openrewrite/` — Maven (`pom.xml`), not Gradle as the audit
    plan assumed. Build not exercised this run; record as Phase 9 follow-up.

## Phase 2 — Static analysis (detected)

### gofmt — `[medium] 3 files`

```
internal/backend/lsp/adapter_test.go
internal/edit/applier.go
internal/symbol/types.go
```

Sample diff (`internal/symbol/types.go`): one extra space in an iota
declaration. Cosmetic. Remediate: `gofmt -w` on the three files.

### govulncheck / gocyclo / deadcode — `[medium] not installed`

Discovery reported these tools as "detected." They are not on PATH in
this environment (`which govulncheck gocyclo deadcode` empty). The audit
plan recommends installing and running them; until then, the gates they
imply are uncovered. Treat as a **medium** static-analysis-gap finding
(not a tool-misconfiguration finding).

### shellcheck — `[ok] no shell scripts in repo`

`git ls-files '*.sh'` is empty. Tool is irrelevant here.

### Coverage — `49.8% overall (statements)`

Lowest-coverage hot spots (0%):

- `cmd/refute/main.go:5 main` — entrypoint exclusion is normal.
- `internal/backend/lsp/adapter.go:167 startsUppercase`,
  `:360 matchIdentAfter`, `:497 MoveToFile`, `:511 PrimeWorkspace` —
  exported `MoveToFile` with zero coverage on a documented-supported
  surface is a **medium** coverage gap.
- `internal/backend/lsp/client.go:108 begin`, `:122 end`,
  `:299 handleProgress`, `:45 Error`, `:58 isLSPError`,
  `:63 isRetryableRenameError` — the LSP transport's progress tracker and
  the JSON-RPC error-classification helpers are uncovered. **high**
  coverage gap given this is the primary risk surface.
- `internal/backend/lsp/priming.go:24 PrimeWorkspace` — uncovered.
- `internal/backend/openrewrite/adapter.go:103-148` — entire adapter
  surface uncovered. Acceptable while support-matrix lists Java as
  `unsupported`, but see Phase 8 drift finding.

No fixed percentage target is asserted; report lists gaps only.

## Phase 3 — Static-analysis gaps

- `[medium] Go has no multi-linter`. Recommend `golangci-lint` with
  `errcheck`, `gosec`, `staticcheck`, `govet`, `ineffassign`, `unused`.
  Install: `go install
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`. Wire
  into the future CI workflow (Phase 11).
- `[medium] No secret scanner`. See Phase 4.
- `[medium] adapters/tsmorph/ has no tsc, no eslint, no test`.
  `package.json` declares only dependencies (`ts-morph`, `typescript`)
  and zero scripts. The adapter ships a CLI (`rename.cjs`) handler that
  is invoked via subprocess but has no automated checks at all. Wire at
  minimum: `"scripts": { "tsc": "tsc --noEmit", "test": "node
  --test" }`. **medium** because the adapter is documented as
  experimental/not-packaged.

## Phase 4 — Secrets scan

- `[medium] No secret scanner installed`. `gitleaks` is not on PATH and
  Docker was not used in this run.
- Manual sweep: one false positive at
  `internal/backend/lsp/client.go:309 — token := string(p.Token)`
  (LSP `$/progress` token, not a credential). **No real hits.**
- **Remediation:** install `gitleaks` and run `gitleaks detect
  --no-banner --redact` plus `--log-opts="--all"` against history. Add a
  `.gitleaksignore` only with documented justifications.

## Phase 5 — Code quality (model-based)

### `internal/edit/applier.go` — `[high] non-atomic Apply contradicts its docstring`

Lines 14-19 declare: "applies the WorkspaceEdit atomically across all
files." Lines 60-70 then commit per-file: if `os.Rename` fails on file
N, files 0..N-1 are already replaced with new content, and the cleanup
loop only removes remaining sidecar `.refute.tmp` files. The function
returns an error, but the workspace is left in a half-renamed state. A
real atomic implementation would either:

1. Keep originals as `.refute.bak`, rename all sidecars, and only delete
   backups after every rename succeeds (rolling back originals on
   failure), or
2. Use copy-write-rename with a journal file and explicit rollback.

**Remediation:** either fix the implementation or weaken the docstring
to "atomic per file, best-effort across files" and surface the partial
state to the caller (return list of completed paths so the CLI can
print recovery instructions).

### `internal/edit/applier.go:108 positionToOffset` — `[medium] O(n) per edit, byte-only`

Each call walks `content` from offset 0. With many edits per file the
cost is O(edits × content). Pre-build a line offset table once per
file. Already documented in `docs/position-encoding.md` that this is
byte-only (LSP is UTF-16); not a bug given the doc, but performance is.

### `internal/backend/lsp/client.go:218 cmd.Stderr = nil` — `[high] LSP server stderr discarded`

When gopls/rust-analyzer crashes or panics, its stderr is dropped and
all the user sees is a generic JSON-RPC error. Capture stderr to a
ring buffer or temp file and surface the last N lines on protocol
error. **Remediation:** assign `cmd.Stderr = &ringBuffer{}` and include
its contents in the error returned by `request` when a transport read
fails after a successful write.

### `internal/backend/lsp/client.go:326 request(...)` — `[high] no per-request timeout`

`request` calls `c.transport.Write(data)` then blocks on `<-ch`
indefinitely. If the server hangs (gopls deadlock during indexing,
rust-analyzer Crate Graph stall) the CLI never returns. The
`progressTracker.waitIdle` is the only ctx-aware path; `request` itself
takes no `context.Context`. **Remediation:** thread a context through
`request`, `Rename`, `CodeActions`, etc., with `select { case resp :=
<-ch: ... case <-ctx.Done(): ...}` and a default CLI flag for the
timeout (e.g. `--lsp-timeout=30s`).

### `internal/backend/lsp/client.go:263-264 readLoop unmarshal silently continues` — `[medium]`

If a server sends malformed JSON the message is dropped and no log is
emitted. Combined with `Stderr = nil` this is a debugging dead end.
Log to a ring buffer or `slog` at warn level.

### `internal/backend/lsp/client.go:309 token := string(p.Token)` — `[medium] mixed-type token mishandling`

`p.Token` is `json.RawMessage`. A server that sends `{"token": 5,
...}` gets keyed as `"5"`; a server that sends `{"token": "5", ...}`
gets keyed as `"\"5\""`. begin/end matching across mixed-type tokens
will silently leak tracker state. Normalize: unmarshal Token into
`any`, then format with `%v` or compare canonical types.

### `internal/backend/lsp/transport.go:73 unbounded body allocation` — `[medium] DoS resilience`

`make([]byte, contentLength)` trusts the server's Content-Length. A
malicious or buggy server can claim a multi-GB body and OOM the
process. Cap with a sane upper bound (e.g. 64 MiB; LSP messages are
rarely >1 MiB). **Remediation:** `if contentLength > maxLSPBody {
return nil, fmt.Errorf(...) }`.

### `internal/backend/lsp/transport.go:69 zero Content-Length rejected` — `[low]`

A 0-length `null` body is technically possible. Minor; document or
relax.

### Magic numbers — `[low]`

`internal/backend/lsp/client.go:90,95` already extracts
`initialQuiesce`/`settleTime` as named constants. Sweep for raw
literal timeouts elsewhere is clean. No finding.

### `cmd/refute/main.go` — `[low] version stamping unwired`

`refute version` reports `commit: unknown / built: unknown`. Wire via
`-ldflags='-X main.Commit=$(git rev-parse HEAD)'` or use `runtime/
debug.ReadBuildInfo()`. Cosmetic until release.

## Phase 6 — Test health and skip audit

14 `t.Skip` call sites total:

| Guard | Count | Where |
| --- | --- | --- |
| gopls not found on PATH | 8 | `internal/integration_test.go`, `lsp/client_test.go` |
| rust-analyzer not found | 3 | `internal/integration_test.go` |
| jdtls not found | 1 | `internal/integration_test.go:482` |
| inline not supported by gopls version | 1 | `internal/backend/lsp/adapter_test.go:275` |
| ts-morph backend not installed | 1 | `internal/backend/tsmorph/adapter_test.go:14` |

- `[high] No CI lane guarantees gopls is installed`. With no
  `.github/workflows/`, every gopls integration test is at risk of
  silently skipping in any future automation. Once CI lands (Phase 11),
  the supported-Go lane MUST install gopls and treat skip-on-missing as
  a hard failure (use `t.Fatal` if `GOPLS_REQUIRED=1`).
- `[medium] jdtls skip exists for an "unsupported / not-claimed"
  language`. `internal/integration_test.go:482` skips on missing jdtls,
  but `docs/support-matrix.md` declares Java/Kotlin
  `unsupported`/`not-claimed` and the README says "Not claimed for
  v0.1." Either the test is dead code, or the support matrix is
  understating coverage. Decide and align.
- `[medium] inline-version skip is a runtime feature gate`.
  `lsp/adapter_test.go:275` skips on a server-version capability check.
  Convert to a build tag or environment-pinned gopls version, so
  presence/absence is explicit at CI configuration time, not runtime.

## Phase 7 — Coverage gap analysis (no percentage target)

Confirmed gaps against the audit plan's risk-surface checklist:

- LSP transport (`transport.go`):
  - **No test for malformed Content-Length** (e.g. `Content-Length:
    abc`). The error path at line 62 is reachable but uncovered.
  - **No test for oversized payload** (because no cap exists; see
    Phase 5).
  - **No test for partial reads / server crash mid-response**.
- Edit applier (`applier.go`):
  - **No test for rename mid-batch failure** (the partial-state path
    described in Phase 5).
  - **No test for CRLF input** — `positionToOffset` walks `\n` only,
    assuming LSP gives us LF positions.
  - **No test for symlinked target file**.
- CLI (`internal/cli/*`):
  - Coverage exists for happy paths; verify (next audit) that
    mutually-exclusive flag combinations are rejected.
- Doctor (`internal/cli/doctor.go`):
  - `_test.go` exists but does not exercise the "missing" branch for
    each documented backend.

Severity: LSP transport gaps **high**; applier gaps **high** (compound
with the atomicity finding); CLI/doctor gaps **medium**.

## Phase 8 — Contract and schema consistency

- `[medium] docs/support-matrix.md vs adapters/tsmorph reality`. The
  matrix says TypeScript / JavaScript have **unit** test coverage. The
  ts-morph adapter directory contains no test files and `package.json`
  declares no test script. Either the matrix is overstating coverage,
  or the unit tests live in `internal/backend/tsmorph/adapter_test.go`
  alone (which has a single skip-on-missing test). Tighten the matrix
  language: "unit (Go-side adapter wiring only)" or similar.
- `[medium] docs/support-matrix.md links to GitHub issue #1`. Repo
  states "GitHub" but no `.github/` directory and no CI tells us this
  is a public repo with issues. Verify the link still resolves;
  otherwise replace with an internal tracker reference.
- `[ok] docs/position-encoding.md vs internal/edit/applier.go`. The
  doc honestly states "byte-only, ASCII-correct, Unicode deferred."
  `positionToOffset` matches that contract.
- `[ok] LSP method strings`. `textDocument/rename`,
  `textDocument/codeAction`, `codeAction/resolve`, `workspace/symbol`,
  `$/progress`, `initialize`, `initialized`, `shutdown`, `exit` all
  match LSP 3.17.

## Phase 9 — Dependency health

Go (`go list -m -u all`):

```
github.com/cpuguy83/go-md2man/v2  v2.0.6 → v2.0.7
github.com/mattn/go-isatty        v0.0.20 → v0.0.22
github.com/spf13/pflag            v1.0.9  → v1.0.10
golang.org/x/sys                  v0.42.0 → v0.43.0
gopkg.in/check.v1                 (test-only) → v1.0.0
```

All minor/patch; **low** drift. `go mod tidy -diff` clean.

ts-morph adapter:

- `ts-morph ^26.0.0` and `typescript ^5.9.3` — current as of audit
  date; `npm outdated` not run because the audit plan assumes a
  packaged adapter.

OpenRewrite adapter (`pom.xml`):

- `openrewrite.version 8.33.3` — OpenRewrite has shipped a major
  release (9.x) since this pin. **medium** drift.
- `jackson.version 2.17.0` — current line is 2.18.x. **low** drift.

govulncheck not run (Phase 2 finding). Treat any unreached vuln as
**medium** until proven; reached vuln in deps would be a **blocker**.

## Phase 10 — Documentation truthfulness and utility

Documented commands, executed:

- `go install ./cmd/refute` → success.
- `refute version` → success (prints `0.1.0-dev / commit: unknown /
  built: unknown` — see Phase 5 low finding).
- `refute doctor` → success; output matches `docs/support-matrix.md`
  (Go ok, rust experimental, ts/js missing, python missing/planned,
  java/kotlin not-claimed).
- README "First use" fenced block — multiple short fences for one
  command (`\`\`\`bash refute rename \\\n--file ... \`\`\` \`\`\`bash
  --line 42 \\\n\`\`\` ...`). **medium** doc-utility — copy-paste of
  the block as written produces a series of broken shell snippets
  rather than one runnable command. Collapse into a single fenced
  block.

Doc utility findings:

- `[medium] AGENTS.md is an rtk cheat-sheet, not a refute contributor
  doc`. Of the file's content, ~95% is generic `rtk` command examples
  that appear identically in any project that adopts the wrapper. The
  file contains no refute-specific build, test, layout, or
  add-a-backend guidance. Replace with refute-onboarding or split:
  global rtk content stays in a global location, refute-specific
  content fills `AGENTS.md`.
- `[medium] docs/plans/ and docs/expeditions/ are historical
  artifacts`. The skill-discovery pass picked them up as "design /
  install / quickstart" docs because of their structure, but they are
  execution logs for completed milestones. Add a `README.md` in
  `docs/plans/` clarifying that these are historical and pointing
  contributors to `docs/specs/` and `docs/support-matrix.md` as live
  contracts.
- `[low] docs/position-encoding.md` is correct but talks past the
  reader — the immediate next-step ("If you need Unicode, file an
  issue and reference this document") is not actionable until a
  tracker exists.
- `[ok] docs/support-matrix.md` is the strongest doc in the repo:
  status definitions are crisp, the matrix has caveats, and runtime
  `refute doctor` is wired to the same data.

## Phase 11 — CI and workflow gaps

- `[high] No .github/ directory at all`. No GitHub Actions workflows.
  No Makefile. No Taskfile/just/mage. The verification gates
  documented in `docs/expeditions/go-code-actions/plan.md` (`go build
  ./...`, `go test ./... -timeout 90s`, integration with `-tags
  integration`) are advisory only and rely on contributor discipline.
- Minimum proposal (record only; do not implement during audit):
  - `.github/workflows/go.yml` — push + PR. Steps: `setup-go`, install
    `gopls`, run gofmt diff check, `go vet`, unit tests, integration
    tests with the gopls dependency required (no skip).
  - `.github/workflows/lint.yml` — golangci-lint v2.
  - `.github/workflows/secrets.yml` — gitleaks on push and history.
  - `.github/workflows/adapters.yml` — `paths:` gated. tsmorph runs
    `tsc --noEmit`; openrewrite runs `mvn verify` once `package.json`
    and pom support it.

## Phase 12 — Duplication

- `dupl` not installed. No duplication scan run.
- Manual scan of `internal/backend/{lsp,tsmorph,openrewrite}/adapter.go`
  shows expected boundary similarity (each implements
  `RefactoringBackend`); no obvious within-package duplication. Defer
  formal scan until `dupl` lands.

## Final action list

### Immediate fixes (blockers + highs)

1. `internal/integration_test.go` — pass `GOFLAGS=-buildvcs=false` (or
   `-buildvcs=false` flag) to every `go build` exec inside a temp dir;
   covers the 4 currently-failing post-rename verification calls.
2. `internal/edit/applier.go` — either implement true atomicity
   (`.refute.bak` + journal + rollback) or weaken the docstring and
   surface partially-renamed paths in the returned error.
3. `internal/backend/lsp/client.go` — capture LSP server stderr
   (currently dropped at line 218); thread `context.Context` through
   `request`, `Rename`, `CodeActions`; add a default LSP-call timeout.
4. Add a CI baseline (`.github/workflows/go.yml`) that runs unit +
   integration with `gopls` installed and treats missing-binary skips
   as hard failures.
5. Backfill tests for the three highest-impact gaps: malformed
   Content-Length, partial-batch rename failure, CRLF in source.

### Medium-term improvements

- Install + run govulncheck, golangci-lint, gitleaks, dupl. Wire each
  into CI.
- Cap `transport.go` Content-Length reads at a sane upper bound.
- Fix `handleProgress` token normalization.
- `gofmt -w internal/backend/lsp/adapter_test.go internal/edit/applier.go
  internal/symbol/types.go`.
- Resolve the jdtls integration-test vs unsupported-Java contradiction.
- Tighten `docs/support-matrix.md` ts-morph "unit coverage" claim.
- Rewrite `AGENTS.md` to be refute-specific contributor guidance.
- Wire `tsc --noEmit` and a smoke test into `adapters/tsmorph/
  package.json`.
- Fix README "First use" fenced-block fragmentation.
- Bump OpenRewrite (8.33.3 → 9.x) and Jackson (2.17 → 2.18) in the
  Maven adapter; assess API breaks.
- Wire `refute version` build stamping via `-ldflags`.
- Add a `docs/plans/README.md` clarifying that the directory is
  historical.

### Structural investments

- CI is the single highest-leverage missing piece. Without it the rest
  of the audit findings cannot be enforced.
- Decide a tracker (GitHub Issues vs Beads vs internal) and wire
  `docs/position-encoding.md`'s "file an issue" guidance to it.
- Establish a promotion checklist for "experimental → supported": doc,
  CI lane, integration coverage, install path, doctor entry. Apply to
  Rust first, then TypeScript / JavaScript.

## Run metadata

- Date: 2026-05-02
- HEAD: `53028c0fb0f31876e968ccc1d7a5bc6b08f3f4a0`
- go version: `go1.26.1 linux/amd64`
- gopls: `/home/ketan/go/bin/gopls` (version not captured)
- Tools missing in environment: `govulncheck`, `gocyclo`, `deadcode`,
  `dupl`, `gitleaks`, `golangci-lint`. Findings dependent on those
  tools are recorded as gaps, not omissions.
