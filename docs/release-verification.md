# Release Verification

Before building or tagging a release, verify the candidate with one command:

```bash
make verify
```

This runs `scripts/verify.sh`, the unified pre-release check harness. It prints
the environment and backend versions, then runs the full suite sequentially
with explicit `PASS` / `FAIL` / `SKIP` output for each check. Run it from a
clean checkout of the commit you intend to release; run it before
`scripts/release.sh`, which builds artifacts without first running checks.

## What it runs

In order:

| Check | Command | Notes |
| --- | --- | --- |
| `go vet` | `go vet ./...` | static analysis |
| `gofmt` | `gofmt -l .` | fails if any file is unformatted |
| `govulncheck` | `govulncheck ./...` | SKIP if `govulncheck` is not installed |
| unit tests | `go test ./...` | |
| build | `go build ./cmd/refute` | release-binary build check |
| smoke | runs the built binary | `refute version` and `refute --help` must succeed |
| integration tests | `go test -tags integration ./internal/...` | drives real backends against the `testdata` corpus; SKIP when `gopls` is absent |
| shim conformance | `scripts/shim-conformance.sh` | cross-shim toolchain harness; self-skips per toolchain |
| docs links | relative links in `docs/README.md` resolve | guards against a dangling docs index |

## Outcomes

The exit code distinguishes the three states a maintainer needs to tell apart:

| Exit | Meaning |
| --- | --- |
| `0` | every required check passed; any skips are reported but do not fail the run |
| `1` | at least one required check FAILED; the summary lists which |
| `2` | unsupported environment — no Go toolchain on `PATH`, or not inside a git checkout |

A skipped optional backend (no `gopls`, no `govulncheck`, etc.) is reported
loudly as `SKIP` and never turns a passing run into a failure. Decide per
release whether a skipped category matters; CI installs every backend, so a
fully green CI run is the canonical "nothing skipped" reference.

## Options

```bash
make verify VERIFY_FLAGS=--no-integration    # skip the tagged integration suite
make verify VERIFY_FLAGS=--no-conformance    # skip the cross-shim harness
scripts/verify.sh --help                     # full flag and env-var reference
```

The same toggles are available as `REFUTE_VERIFY_NO_INTEGRATION=1` and
`REFUTE_VERIFY_NO_CONFORMANCE=1`.

## Backend prerequisites

The version banner shows which backends were found. To exercise every check
without skips, install the backends listed under "Integration Backend
Prerequisites" in `AGENTS.md` (at minimum `gopls` for the supported Go path),
plus `govulncheck`:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
```
