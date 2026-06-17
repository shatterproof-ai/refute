# Release Verification

Before building or tagging a release, verify the candidate with one command:

```bash
make verify          # fast feedback: stop at the first failing gate
make verify-report   # full audit: run every gate, then report a summary
```

Both run `scripts/verify.sh`, the unified pre-release check harness. It prints
the environment and backend versions, then runs the full suite sequentially
with explicit `PASS` / `FAIL` / `SKIP` / `UNAVAIL` output for each check. Run it
from a clean checkout of the commit you intend to release; run it before
`scripts/release.sh`, which builds artifacts without first running checks.

`make verify` stops at the first failing gate, so a quick local check fails fast.
`make verify-report` keeps going through every gate even when one fails, so a
single failure never hides later checks — **use `make verify-report` (or
`scripts/verify.sh --keep-going`) for a full audit.** Both exit non-zero if any
required gate fails.

## What it runs

In order:

| Check | Command | Notes |
| --- | --- | --- |
| `go vet` | `go vet ./...` | static analysis |
| `gofmt` | `gofmt -l .` | fails if any file is unformatted |
| `govulncheck` | `govulncheck ./...` | UNAVAIL if `govulncheck` is not installed |
| unit tests | `go test ./...` | |
| build | `go build ./cmd/refute` | release-binary build check |
| smoke | runs the built binary | `refute version` and `refute --help` must succeed |
| integration tests | `go test -tags integration ./internal/...` | drives real backends against the `testdata` corpus; UNAVAIL when `gopls` is absent, SKIP with `--no-integration` |
| shim conformance | `scripts/shim-conformance.sh` | cross-shim toolchain harness; self-skips per toolchain |
| docs links | relative links in `docs/README.md` resolve | guards against a dangling docs index |

## Outcomes

Each check reports one of four outcomes, kept distinct so an audit can tell a
deliberate skip apart from a missing tool:

| Outcome | Meaning |
| --- | --- |
| `PASS` | the check ran and succeeded |
| `FAIL` | the check ran and failed |
| `SKIP` | the check was intentionally disabled (e.g. `--no-integration`) |
| `UNAVAIL` | a required tool or backend is absent, so the check could not run |

The exit code distinguishes the three states a maintainer needs to tell apart:

| Exit | Meaning |
| --- | --- |
| `0` | every required check passed; skips and unavailable tools are reported but do not fail the run |
| `1` | at least one required check FAILED; the summary lists which |
| `2` | unsupported environment — no Go toolchain on `PATH`, or not inside a git checkout |

A skipped (`SKIP`) or unavailable (`UNAVAIL`) optional backend is reported loudly
and never turns a passing run into a failure. The summary counts the two
separately, so a full audit can see whether a category was turned off on purpose
or simply could not run for want of a backend. Decide per release whether a gap
matters; CI installs every backend, so a fully green CI run is the canonical
"nothing skipped, nothing unavailable" reference.

## Options

```bash
make verify-report                                  # keep-going full audit
make verify VERIFY_FLAGS=--no-integration           # skip the tagged integration suite
make verify VERIFY_FLAGS=--no-conformance           # skip the cross-shim harness
scripts/verify.sh --keep-going                       # full audit, direct invocation
scripts/verify.sh --fail-fast                        # stop at the first failure
scripts/verify.sh --help                             # full flag and env-var reference
```

`make verify` runs `--fail-fast` and `make verify-report` runs `--keep-going`;
either accepts extra flags through `VERIFY_FLAGS`. The same toggles are available
as `REFUTE_VERIFY_FAIL_FAST=1`, `REFUTE_VERIFY_NO_INTEGRATION=1`, and
`REFUTE_VERIFY_NO_CONFORMANCE=1`.

## Backend prerequisites

The version banner shows which backends were found. To exercise every check
without skips, install the backends listed under "Integration Backend
Prerequisites" in `AGENTS.md` (at minimum `gopls` for the supported Go path),
plus `govulncheck`:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```
