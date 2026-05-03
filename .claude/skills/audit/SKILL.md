---
name: audit
description: Run the refute repository audit. Use when the user asks to audit refute, run a quality/security review across the repo, or refresh the audit baseline. Tailored to refute's stack (Go primary, Java/TS adapters, LSP-driven backends) and the documented v0.1 scope.
---

# Refute Audit

Repo-local audit skill, generated 2026-05-02 from
`docs/plans/2026-05-02-audit-plan.md`. Keep this skill lean. When discovery
changes (new language surface, new CI, new tracker), regenerate via
`bento:generate-audit` rather than expanding this file.

## Purpose and scope

Drive a repeatable audit across the refute repo. Supported (Go) surface is
the bar for **high** and **blocker** severities; experimental surfaces (TS,
Rust, Java adapters) audit at **medium** unless a finding is a security
issue.

Out of scope: daemon, MCP server, any feature absent from
`docs/support-matrix.md`. The repo is at v0.1 single-shot CLI only.

## Severity model

- **blocker** — broken install path, broken security boundary, leaked
  secret, broken documented command on the supported (Go) path.
- **high** — silent regression risk, missing CI gate, drift between docs
  and code on a supported feature.
- **medium** — quality smell, valuable missing tool, outdated dep without
  CVE, unclear contributor docs.
- **low** — stylistic, optional tooling gap, minor doc utility issue.

## Audit phases

Run sequentially. Capture findings under each phase, then consolidate.

### 1. Build health

```bash
go build ./...
go vet ./...
go test ./... -timeout 90s
command -v gopls && go test -tags integration ./internal/... -timeout 120s
( cd adapters/tsmorph && npm install && npm test ) 2>&1 | head -200
( cd adapters/openrewrite && ./gradlew --no-daemon build ) 2>&1 | head -200
```

Failures on the Go path → **blocker**. Adapter failures → **high** unless
documented as unsupported.

### 2. Static analysis (detected)

```bash
gofmt -l $(git ls-files '*.go' | grep -v '^testdata/')
govulncheck ./...
gocyclo -over 10 .
deadcode ./...
git ls-files '*.sh' | xargs -r shellcheck
go test -coverprofile=/tmp/refute.coverout ./... -timeout 90s \
  && go tool cover -func=/tmp/refute.coverout | sort -k3 -n | head -20
```

- gofmt non-empty → **medium**.
- govulncheck reachable vuln → **blocker**; unreached → **medium**.
- gocyclo >10 → **medium**, >15 → **high** in `internal/backend/lsp/`,
  `internal/edit/`, `internal/cli/`.
- Reachable deadcode → **medium**.
- Coverage output drives Phase 7 — no fixed percentage target.

### 3. Static-analysis gaps

Recommend (one per gap, highest-value only):

- **golangci-lint** with `errcheck`, `gosec`, `staticcheck`, `govet`,
  `ineffassign`, `unused`. Skip `dupl`/`nancy` until this lands.
- **gitleaks** — see Phase 4.
- **tsc + eslint** wired into `adapters/tsmorph/package.json`.

Flag any newly added linter config that disables rules without a written
reason as a finding, not a note.

### 4. Secrets scan (mandatory)

```bash
# Working tree (Docker preferred; falls back to Go install)
docker run --rm -v "$PWD:/repo" zricethezav/gitleaks:latest \
  detect --source=/repo --no-banner --redact \
  || gitleaks detect --no-banner --redact

# History
gitleaks detect --no-banner --redact --log-opts="--all"

# Hand-rolled sweep
git grep -nE '(api[_-]?key|secret|token|password)\s*[:=]' \
  -- ':!testdata' ':!docs'
```

Any positive hit → **blocker** until rotated and history-scrubbed. Track
vetted false positives in `.gitleaksignore`.

### 5. Code quality (model-based)

Sample order:

1. Risk surfaces:
   - `internal/backend/lsp/{transport,client,adapter,priming,workspace}.go`
   - `internal/edit/{applier,diff,json}.go`
   - `internal/cli/{rename,inline,extract,workspace,errors,jsoncontext}.go`
   - `adapters/openrewrite/.../RenameHandler.java`
2. High-churn files:
   ```bash
   git log --format='' --name-only -- 'internal/**/*.go' 'cmd/**/*.go' \
     | sort | uniq -c | sort -rn | head -20
   ```
3. Remainder.

Smell catalog (call out file:line):

- Subprocess leaks; missing `cmd.Wait` / context cancellation in LSP
  client.
- Goroutine leaks in transport read loops.
- Unchecked errors on close paths.
- Path traversal in edit applier.
- Magic numbers (timeouts, retries) without named constants.
- CLI files containing non-trivial business logic that belongs in
  `internal/`.
- Inconsistent error wrapping.

Subprocess/leak/path-traversal → **high**; the rest → **medium**.

### 6. Test health and skip audit

```bash
git grep -nE 't\.Skip\(' -- 'internal/**/*.go' 'cmd/**/*.go'
```

For each skip: record the guard, the scenario it covers, and whether any
CI lane guarantees the dependency (e.g. gopls) is present. A silent skip
on the supported Go path with no CI guarantee → **high**. Permanent gates
should be a build tag, not a runtime skip.

### 7. Coverage gap analysis (no percentage target)

Cross-reference Phase 2 coverage with Phase 5 risk surfaces. Specific
gaps to confirm or refute every audit run:

- LSP transport: malformed Content-Length, oversized payload, partial
  reads, server crash mid-response.
- Edit applier: overlapping edits, CRLF, symlinked path, rollback on
  failure.
- CLI: missing required flags, mutually exclusive flags, non-existent
  file, line/column out of range.
- Doctor: each backend's "missing" branch.

Missing case in LSP transport → **high**; elsewhere → **medium**.

### 8. Contract and schema consistency

- `internal/edit/json.go` ↔ `internal/edit/contract_test.go` — verify
  every public field is covered, including unrecognized-field and version
  skew.
- `docs/position-encoding.md` ↔ multi-byte handling in
  `internal/backend/lsp/` and `internal/edit/`. Trace one fixture
  end-to-end.
- LSP method strings in code ↔ pinned spec version in docs.

Drift on Go path → **high**, elsewhere → **medium**.

### 9. Dependency health

```bash
go list -m -u all | head -50
go mod tidy -diff || (go mod tidy && git diff -- go.mod go.sum)
( cd adapters/tsmorph && npm outdated && npm audit --omit=dev )
( cd adapters/openrewrite && ./gradlew dependencyUpdates -q || true )
```

Reached CVE → **blocker**. Major-version drift on used dep → **medium**.
Untracked `go mod tidy` diff → **medium**.

### 10. Documentation truthfulness and utility

Run install path in a sandbox:

```bash
GOBIN=/tmp/refute-audit-bin go install ./cmd/refute
/tmp/refute-audit-bin/refute version
/tmp/refute-audit-bin/refute doctor
```

Verify the README "First use" fenced block copy-pastes cleanly.

Audit utility, not just presence:

- `AGENTS.md` — must contain refute-specific contributor guidance (build,
  test, layout, how to add a backend). An `rtk` cheat-sheet alone is a
  **medium** documentation-utility finding.
- `README.md` — every support-matrix claim must match repo state.
- `docs/support-matrix.md` — must match what `refute doctor` evaluates.
- `docs/specs/` — flag stale "we will / TBD" sections.
- `docs/plans/` and `docs/expeditions/` — confirm these are not linked as
  current contracts; they are historical execution artifacts.

### 11. CI and workflow gaps

If `.github/workflows/` is empty or missing, that is a **high** finding —
the gates in `docs/expeditions/go-code-actions/plan.md` are unenforced.
Minimum proposal (do not implement during the audit run):

- Push workflow: Phase 1 + Phase 2.
- Integration job with gopls explicitly installed, running
  `-tags integration`.
- Adapter jobs gated on `paths:`.
- Gitleaks job (Phase 4).

### 12. Duplication

```bash
go install github.com/mibk/dupl@latest
dupl -t 50 ./internal/...
```

Within-adapter or shared-helper duplication → **medium**. Cross-adapter
duplication at the LSP/ts-morph/OpenRewrite boundary is expected; do not
flag.

## Output format

Produce a single audit report with:

1. **Executive summary** — counts by severity, top 3 blockers/highs.
2. **Per-phase findings** — for each phase, list findings as
   `[severity] file:line — finding — remediation`.
3. **Consolidated action list** — three buckets:
   - Immediate fixes (blockers + highs).
   - Medium-term improvements.
   - Structural investments (CI baseline, doc structure, language
     promotion criteria).
4. **Run metadata** — date, git HEAD, tool versions used.

## Optional post-audit actions

- File the report under `docs/audits/YYYY-MM-DD-audit.md` on a new
  branch via `bento:launch-work`.
- For each immediate fix, open a separate branch + worktree per the
  one-task-one-branch rule.
- This repo has no detected tracker — do not create tracker items.
- Do not auto-commit remediation alongside the report.
