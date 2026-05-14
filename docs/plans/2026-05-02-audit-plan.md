# Refute Audit Plan — 2026-05-02

A one-off audit playbook tailored to the refute repository. Generated via
`bento:generate-audit`. Run phases sequentially; capture findings under each
phase, then consolidate into the action list at the end.

## Scope

- Primary surface: Go module at `github.com/shatterproof-ai/refute`
  (`cmd/refute`, `internal/{cli,backend,edit,symbol,config}`).
- Secondary surfaces: Java OpenRewrite adapter under `adapters/openrewrite/`,
  TypeScript ts-morph adapter under `adapters/tsmorph/`, multi-language
  fixtures under `testdata/`.
- Out of scope (repo states v0.1, single-shot CLI only): daemon, MCP server,
  any feature absent from `docs/support-matrix.md`.

## Severity model

- **blocker** — broken install path, broken security boundary, leaked
  secret, broken documented command on the supported (Go) path.
- **high** — silent regression risk (skipped tests masking real failure),
  missing CI gate, drift between docs and code on a supported feature.
- **medium** — quality smell, missing static-analysis tool with clear value,
  outdated dependency without known CVE, unclear contributor docs.
- **low** — stylistic issue, minor doc utility issue, optional tooling gap.

## Phase 1 — Build health

```bash
go build ./...
go vet ./...
go test ./... -timeout 90s
```

Expected gates per `docs/expeditions/go-code-actions/plan.md`. Any compile,
vet, or unit-test failure on `main` is a **blocker**.

Integration suite (only when gopls is on PATH):

```bash
command -v gopls && go test -tags integration ./internal/... -timeout 120s
```

If `gopls` is absent, record this as evidence for Phase 6 (skip-test audit),
not as a Phase 1 failure.

Adapters:

```bash
( cd adapters/tsmorph    && npm install && npm test 2>&1 | head -200 )
( cd adapters/openrewrite && mvn -B verify 2>&1 | head -200 )
```

Adapter build failures are **high** unless the adapter is documented as
unsupported in `docs/support-matrix.md`.
The OpenRewrite adapter build requires JDK 17 or newer and Maven on PATH.

## Phase 2 — Static analysis (detected tools)

Each block: command → interpretation → severity mapping.

### gofmt

```bash
gofmt -l $(git ls-files '*.go' | grep -v '^testdata/')
```

Any non-empty output → **medium** finding. Remediate with `gofmt -w`.

### govulncheck

```bash
govulncheck ./...
```

Any reported vulnerability that reaches an imported symbol → **blocker**.
Vulns in unreached code → **medium** with version-bump remediation.

### gocyclo

```bash
gocyclo -over 10 $(git ls-files '*.go' | grep -v '^testdata/' | xargs -I{} dirname {} | sort -u)
```

Cyclomatic complexity over 10 in `internal/backend/lsp/`,
`internal/edit/applier.go`, or `internal/cli/` → **medium**; refactor
candidates. Over 15 → **high**.

### deadcode

```bash
deadcode ./...
```

Any reachable-from-`main` dead symbol → **medium**; delete in a separate PR.
Test-only fixtures excluded.

### shellcheck

```bash
git ls-files '*.sh' | xargs -r shellcheck
```

`.shatter/` scripts are the most likely target. Errors → **medium**,
warnings → **low**.

### go test cover (gap signal, not threshold)

```bash
go test -coverprofile=/tmp/refute.coverout ./... -timeout 90s
go tool cover -func=/tmp/refute.coverout | sort -k3 -n | head -20
```

Use the lowest-coverage list to drive Phase 8 risk-surface gaps. **No fixed
percentage target.**

## Phase 3 — Static-analysis gaps

Detected tooling does not include a multi-linter for Go, a duplication
checker, or a secret scanner. Highest-value missing tool per gap:

- **golangci-lint** (Go multi-linter): install via
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`,
  add `.golangci.yml` enabling `errcheck`, `gosec`, `staticcheck`,
  `govet`, `ineffassign`, `unused`. Wire into Phase 1. **Recommendation
  severity: medium.** Skip `dupl` and `nancy` until golangci-lint is on.
- **gitleaks** (cross-language secret scanner): see Phase 4.
- **eslint + tsc** for `adapters/tsmorph/`: the package has no `tsc` or
  `eslint` script wired. Recommendation severity: **medium** — adapter is
  experimental, but JSON-RPC handlers without typecheck are a high-drift
  surface.

Existing tool configs to inspect for weakened rules (currently none
detected — each tool is invoked CLI-only with no config file). Flag any
newly introduced config that disables rules without a written reason.

## Phase 4 — Secrets scan (mandatory)

Working tree:

```bash
docker run --rm -v "$PWD:/repo" zricethezav/gitleaks:latest \
  detect --source=/repo --no-banner --redact
```

If Docker is unavailable, fall back to:

```bash
go install github.com/gitleaks/gitleaks/v8@latest
gitleaks detect --no-banner --redact
```

Git history:

```bash
gitleaks detect --no-banner --redact --log-opts="--all"
```

Any positive hit → **blocker** until rotated and history-scrubbed. Record
false positives in `.gitleaksignore` rather than disabling rules.

Manual sweep for hand-rolled risks:

```bash
git grep -nE '(api[_-]?key|secret|token|password)\s*[:=]' -- ':!testdata' ':!docs'
```

## Phase 5 — Code quality (model-based)

Sample order, per `quality-standards.md`:

1. **Risk surfaces first**:
   - `internal/backend/lsp/transport.go`, `client.go`, `adapter.go`,
     `priming.go`, `workspace.go` — JSON-RPC framing, subprocess lifecycle,
     stdio plumbing.
   - `internal/edit/applier.go`, `diff.go`, `json.go` — file mutation path.
   - `internal/cli/rename.go`, `inline.go`, `extract.go`, `workspace.go`,
     `errors.go`, `jsoncontext.go` — input validation, exit codes.
   - `adapters/openrewrite/.../RenameHandler.java` — external input handler.
2. **High-churn files**:
   ```bash
   git log --format='' --name-only -- 'internal/**/*.go' 'cmd/**/*.go' \
     | sort | uniq -c | sort -rn | head -20
   ```
3. Remainder.

Smell catalog to apply (call out file:line):

- Subprocess leaks, missing `cmd.Wait` / context cancellation in LSP client.
- Unchecked errors on `io.Writer` / `os.File` close paths.
- Path traversal in edit applier (any `..` in workspace-relative paths).
- Goroutine leaks in transport read loops.
- Magic numbers (timeouts, retry counts) without named constants — see
  global naming guidance.
- Mixed responsibility in `cmd/` files (CLI wiring should not contain
  business logic; flag any non-trivial logic that belongs in `internal/`).
- Inconsistent error wrapping (`fmt.Errorf("...: %w", err)` vs returning
  raw errors).

Severity: subprocess/leak/path-traversal → **high**; naming, wrapping,
magic numbers → **medium**.

## Phase 6 — Test health and skip audit

Count and classify skipped tests:

```bash
git grep -nE 't\.Skip\(' -- 'internal/**/*.go' 'cmd/**/*.go'
```

Findings to produce:

- For each skip, record the guard (`gopls not found`, `inline not supported
  by this gopls version`, etc.) and the integration scenario it covers.
- Verify whether any CI workflow enforces presence of `gopls` so the skip
  cannot silently skip there. (Currently no `.github/workflows/` exists —
  treat absence of CI gating as a separate **high** finding under Phase 11.)
- Whether the skip is a permanent feature gate or a temporary mask.
  Permanent gates should be a build tag or env-conditional, not a runtime
  skip.

A silent skip on the supported Go path without a CI guarantee that gopls is
installed → **high**.

## Phase 7 — Contract and schema consistency

The repo has no API schema or generated-code paths, but it does have
internal contracts worth verifying:

- `internal/edit/json.go` against `internal/edit/contract_test.go` — make
  sure the contract tests cover every public field; add cases for
  unrecognized fields and version skew.
- `docs/position-encoding.md` against actual byte/rune handling in
  `internal/backend/lsp/` and `internal/edit/`. Pick one multi-byte test
  fixture and trace it end-to-end.
- LSP method strings (`textDocument/rename`, `textDocument/codeAction`,
  etc.) referenced in code vs the LSP spec version pinned in docs.

Drift → **high** on supported (Go) surfaces, **medium** elsewhere.

## Phase 8 — Coverage gap analysis (no percentage target)

Cross-reference Phase 2 coverage output against Phase 5 risk surfaces.
Findings should look like: "no test exercises subprocess SIGKILL path in
`internal/backend/lsp/transport.go`" rather than "package X is at 62%."

Specific gaps to confirm or refute:

- LSP transport: malformed Content-Length header, oversized payload,
  partial reads, server crash mid-response.
- Edit applier: overlapping edits, edits on file with CRLF, edits on
  symlinked path, edit failure rollback.
- CLI: missing required flags, mutually exclusive flags, non-existent
  file, line/column out of range.
- Doctor command: each documented backend's "missing" branch.

Each missing case → **medium** (or **high** if it touches the LSP
transport, which is the most fragile surface).

## Phase 9 — Documentation truthfulness

Run documented commands and record outcomes:

```bash
# README install path (use a sandbox dir, do NOT install into the audit env)
GOBIN=/tmp/refute-audit-bin go install ./cmd/refute
/tmp/refute-audit-bin/refute version
/tmp/refute-audit-bin/refute doctor
```

The fenced "First use" block in README is split across multiple shell
fences — verify it copy-pastes cleanly (a known authoring pitfall). If it
does not, **medium** finding.

Audit each doc against utility, not just presence:

- `AGENTS.md` — currently almost entirely an `rtk` cheat-sheet. It contains
  no refute-specific contributor guidance (build, test, layout, conventions,
  how to add a backend). **medium** documentation-utility finding;
  remediation: replace or extend with a refute-onboarding section.
- `README.md` — verify every status claim in the support-matrix table
  against repo state (e.g. "TypeScript / JavaScript: ts-morph adapter not
  packaged" against `adapters/tsmorph/` reality).
- `docs/support-matrix.md` — confirm canonical claims match what
  `refute doctor` evaluates locally.
- `docs/specs/2026-04-15-refute-design.md` and
  `docs/specs/2026-04-22-rust-parity-design.md` — flag any "we will" /
  "TBD" sections that have since been resolved or abandoned.
- `docs/plans/` and `docs/expeditions/` — these are historical execution
  artifacts. Verify they are not being read as current contracts; if any
  contributor doc links to them as authoritative, **medium**.

## Phase 10 — Dependency health

Go:

```bash
go list -m -u all   | head -50
go mod tidy -diff  || go mod tidy && git diff -- go.mod go.sum
govulncheck ./...   # already in Phase 2
```

ts-morph adapter:

```bash
( cd adapters/tsmorph && npm outdated; npm audit --omit=dev )
```

OpenRewrite adapter (Maven):

```bash
( cd adapters/openrewrite && mvn -B versions:display-dependency-updates || true )
```

Major-version drift on actively used deps → **medium**. Any CVE on a
reached dep → **blocker**. Untracked `go mod tidy` diff → **medium**.

## Phase 11 — CI and workflow gaps

There are no `.github/workflows/`, no Makefile, and no task runner. The
quality gates documented in `docs/expeditions/go-code-actions/plan.md`
are not enforced by automation. **high** finding.

Minimum CI proposal (record as remediation, do not implement during audit):

- One workflow that runs Phase 1 + Phase 2 (gofmt, govulncheck, gocyclo,
  deadcode, shellcheck) on every push.
- Integration job that explicitly installs `gopls` and runs the
  `-tags integration` suite, so Phase 6 skips become impossible there.
- Adapter jobs gated on `paths:` so they only run when the adapter
  changes.

## Phase 12 — Duplication scan

```bash
go install github.com/mibk/dupl@latest
dupl -t 50 ./internal/...
```

Cross-adapter duplication (LSP vs ts-morph vs OpenRewrite) is expected at
the boundary; flag only duplication inside a single adapter or shared
helper that should live in `internal/`. Severity: **medium**.

## Final action list

Consolidate findings into three buckets at the end of the audit run.

### Immediate fixes

- Anything from Phase 1, Phase 4, Phase 6 (silent skips on supported path
  without CI guarantee), or Phase 9 (broken install/quickstart) classified
  **blocker** or **high**.

### Medium-term improvements

- Add `golangci-lint` + `.golangci.yml` and wire into a CI workflow
  (Phase 3 + Phase 11).
- Rewrite `AGENTS.md` to be refute-specific contributor guidance, not an
  `rtk` cheat-sheet (Phase 9).
- Wire `tsc` and `eslint` into `adapters/tsmorph/package.json` (Phase 3).
- Add gitleaks to CI and commit `.gitleaksignore` for vetted false
  positives (Phase 4 + Phase 11).
- Convert "skip if gopls missing" runtime guards to a build tag or
  environment-conditional, plus a CI lane that always has gopls (Phase 6).

### Structural investments

- Establish a CI baseline (Phase 11). Without it, every other gate is
  advisory.
- Decide whether `docs/plans/` and `docs/expeditions/` are historical
  artifacts or live contracts, and update navigation accordingly
  (Phase 9).
- If TypeScript or Rust are to graduate from "experimental," define and
  enforce the integration coverage and CI lane required by
  `docs/support-matrix.md`.

## Notes

- This audit predates the project tracker requirement. Current follow-up work
  should be filed as GitHub Issues.
- The repo uses linked worktrees and the bento workflow skills. Any
  remediation work should be launched via `bento:launch-work` per the
  global agent guidance.
