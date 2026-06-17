# Release Readiness Criteria

This document defines what "ready for real usage" means for `refute` and gives
a maintainer a reviewable checklist to run before claiming a release is ready.
It is scoped to the first usable release line (`v0.1.0`, dogfood) and the rules
that promote later releases.

Every criterion below maps to a concrete verifier: a **test suite**, a **docs
page**, a **GitHub issue**, or a **manual check** with the exact command. A
criterion is met only when its evidence exists and is current — not when it is
merely planned.

Related context: [Current State](current-state.md) (status snapshot),
[Support Matrix](support-matrix.md) (canonical support source of truth),
[Versioning and Compatibility](versioning.md) (compatibility policy),
[Release Process](release.md) (artifact mechanics), and
[Release Verification](release-verification.md) (`make verify` harness).
Release execution is tracked separately in issue #60; this document defines the
criteria, not the act of tagging.

## How to use this checklist

1. Run `make verify` from a clean checkout of the release commit. This gate
   covers the bulk of the mechanical criteria (see
   [Release Verification](release-verification.md)).
2. Walk the tables below. For each row, confirm the evidence resolves and is
   current for the commit being released.
3. Reconcile every **supported** claim across `README.md`,
   `docs/support-matrix.md`, `CHANGELOG.md`, `docs/release.md`, and
   `refute doctor` — they must agree. A disagreement is a release blocker.
4. Record any intentionally skipped optional check and why.

## 1. Scope: supported languages and operations

The first usable release claims exactly one **supported** path. Everything else
is explicitly labeled so users and agents do not mistake target architecture for
shipped behavior.

| Criterion | Verified by | Status / blocker |
| --- | --- | --- |
| Go is **supported** for rename, extract-function, extract-variable, inline | Test: `internal/integration_test.go` (build tag `integration`), required CI lane with `gopls`; Docs: `docs/support-matrix.md` | Required |
| Rust is **experimental** (rename + extract + inline), labeled as such everywhere | Test: experimental integration lane (`REFUTE_EXPERIMENTAL_INTEGRATION=1`); Docs: `docs/support-matrix.md` | Required label accuracy |
| TypeScript / JavaScript are **experimental** (rename), labeled as such | Test: experimental integration lane; Docs: `docs/support-matrix.md` | Required label accuracy |
| Python is **planned**, not claimed as working | Docs: `docs/support-matrix.md`; Issue: backend promotion tracked when fixtures land | Required label accuracy |
| Java / Kotlin are **not claimed** for v0.1 | Docs: `docs/support-matrix.md`, `docs/release.md`; Issue: #74 (OpenRewrite JAR discovery) | Required label accuracy |
| Support claims agree across README, support matrix, changelog, release notes, and `refute doctor` | Check: manual reconciliation; the matrix is the source of truth | Release blocker on any drift |
| Operations not routed to a backend return the `unsupported` JSON status | Test: `internal/edit/contract_test.go` + `internal/cli/operation_error_test.go` + integration; Docs: `docs/json-schema.md` | Required |

## 2. Required tests

| Criterion | Verified by | Status / blocker |
| --- | --- | --- |
| Unit tests pass | Check: `go test ./...` (run by `make verify`) | Required |
| Static analysis clean | Check: `go vet ./...` | Required |
| Formatting clean | Check: `gofmt -l .` outputs nothing | Required |
| Vulnerability scan clean | Check: `govulncheck ./...` | Required (SKIP only if tool absent, recorded) |
| Required integration lane (Go) passes in CI with pinned `gopls` | Test: `go test -tags integration ./internal/...`; Check: `GOPLS_VERSION` pinned in `.github/workflows/ci.yml` | Required |
| Experimental integration lane runs non-blocking with backends installed | Test: experimental lane (`REFUTE_EXPERIMENTAL_INTEGRATION=1`) | Required to exist; non-blocking |
| Tier-1/Tier-2/Tier-3 symbol resolution covered (boundary, ambiguity, comments/strings) | Test: `internal/cli` + `internal/symbol` unit tests | Required |
| Position encoding round-trips (ASCII, multi-byte, emoji) | Test: position-encoding unit tests; Docs: `docs/position-encoding.md` | Required |
| `--json` envelope contract tested for success and each failure status | Test: `internal/edit/contract_test.go` (schema + status constants) and `internal/cli/operation_error_test.go` (per-command failure envelopes); Issue: #90 (test gap sweep) for remaining golden gaps | Required for documented statuses; gaps tracked in #90 |
| Integration backend flakiness reconciled | Issue: #52 (flaky Tier-1 integration tests) | Must be resolved or explicitly waived |
| Real-world pinned corpus exercised | Issue: #96 (pinned real-world corpus) | Stretch for v0.1; required before broad-usage claim |

## 3. Required docs

| Criterion | Verified by | Status / blocker |
| --- | --- | --- |
| Install path documented and working | Docs: `INSTALL.md`, `README.md`; Check: `go install .../cmd/refute@latest` smoke | Required |
| Support matrix current and authoritative | Docs: `docs/support-matrix.md` | Required |
| JSON output and exit codes documented (normative) | Docs: `docs/json-schema.md` | Required |
| Versioning and compatibility policy documented | Docs: `docs/versioning.md` | Required |
| Release process and artifacts documented | Docs: `docs/release.md` | Required |
| Release verification harness documented | Docs: `docs/release-verification.md` | Required |
| Current-state snapshot refreshed for the release commit | Docs: `docs/current-state.md` (review before each release candidate) | Required |
| Intent stories match shipped behavior | Docs: `docs/stories/` (rename, extract-function, extract-variable, inline, doctor, list-symbols) | Required for user-facing changes |
| Docs index links resolve | Check: docs-links check in `make verify` | Required |

## 4. Required install / packaging steps

| Criterion | Verified by | Status / blocker |
| --- | --- | --- |
| Static archives build for linux/macOS on amd64/arm64 | Check: `scripts/release.sh vX.Y.Z`; Docs: `docs/release.md` | Required |
| Version/commit/build-date stamped into `refute version` | Check: binary smoke test in `make verify`; Docs: `docs/release.md` | Required |
| Canonical manifest + SHA-256 checksums produced for every artifact | Check: `scripts/release.sh` outputs (`refute-manifest-*.json`, `checksums.txt`) | Required |
| Shim/package adapters (npm, pip, Cargo, Maven/Gradle, Go-tool) conform | Check: `scripts/shim-conformance.sh` (run by `make verify`); self-skips per toolchain | Required for claimed channels |
| Go-tool dependency path works (`go get -tool .../cmd/refute@vX.Y.Z`) | Docs: `docs/release.md`; requires a published semver tag | Tied to issue #60 |
| TS/JS adapter packaged and attached to the GitHub release | Check: `refute-ts-adapter-*.tgz` in release artifacts; Docs: `docs/release.md`, `docs/support-matrix.md` install hints | Required for experimental TS/JS claim |

## 5. Known limitations to document

These must be stated plainly in `CHANGELOG.md` / release notes so the release
does not over-claim. Each maps to its tracking evidence.

| Limitation | Evidence | Disposition |
| --- | --- | --- |
| No MCP server in v0.1 | Docs: `docs/versioning.md` (MCP not shipped); Issue: #49 | Documented, deferred |
| No daemon / backend pool | Docs: `docs/current-state.md` | Documented, deferred |
| OpenRewrite (Java/Kotlin) not packaged/claimed | Docs: `docs/support-matrix.md`; Issue: #74 | Documented, not claimed |
| Python is fallback-only, no fixtures | Docs: `docs/support-matrix.md` | Documented, planned |
| `MoveToFile` returns unsupported in all adapters | Docs: `docs/current-state.md`; Issue: #100 (move-to-file) | Documented, planned |
| No signature refactoring | Issue: #101 (signature refactoring) | Documented, planned |
| Multi-file apply not stress-tested under adversarial I/O | Docs: `docs/current-state.md` (Main Risks) | Documented risk |
| ast-grep / rope adapters absent | Docs: `docs/current-state.md` | Documented, planned |

## 6. JSON and MCP schema compatibility expectations

| Criterion | Verified by | Status / blocker |
| --- | --- | --- |
| JSON envelope carries `schemaVersion` as the compatibility boundary | Docs: `docs/json-schema.md`, `docs/versioning.md`; Test: `internal/edit` JSON tests | Required |
| Documented statuses emitted exactly once per operation in `--json` | Docs: `docs/support-matrix.md` (status list); Test: integration + `internal/cli/operation_error_test.go` | Required |
| Consumers can tolerate unknown fields / future `schemaVersion` (fail-closed guidance) | Docs: `docs/versioning.md` | Required (documented contract) |
| Any incompatible JSON change bumps `schemaVersion` + changelog entry | Docs: `docs/versioning.md` (Schema Change Process) | Required process gate |
| MCP schema policy defined even though MCP is not shipped | Docs: `docs/versioning.md` (MCP Compatibility); Issue: #49 | Required (policy exists); implementation deferred |

## 7. Minimum external dependency versions

| Dependency | Minimum / pin | Verified by |
| --- | --- | --- |
| Go toolchain | Go 1.24+ (Go-tool install path) | Docs: `docs/release.md`; Check: build in `make verify` |
| `gopls` (Go, supported) | Pinned tag `GOPLS_VERSION` in CI | Check: `.github/workflows/ci.yml`; Docs: `docs/support-matrix.md` (Backend versions) |
| `rust-analyzer` (Rust, experimental) | Present on `PATH`; version captured by `refute doctor` | Docs: `docs/support-matrix.md`; Check: experimental lane |
| Node.js + `ts-morph` adapter (TS/JS, experimental) | Adapter tarball from GitHub release; fallback `typescript-language-server` | Docs: `docs/support-matrix.md` install hints |
| JDK / Maven (OpenRewrite) | JDK 17+, Maven (not claimed for v0.1) | Docs: `docs/current-state.md`; Issue: #74 |
| `govulncheck` | Latest (`go install golang.org/x/vuln/cmd/govulncheck@latest`) | Docs: `docs/release-verification.md` |

Backend versions are captured at runtime: `refute doctor` reports a `version`
per backend, successful operations carry `backendVersion` in the JSON envelope,
and CI pins `gopls` so a backend release cannot silently change refactoring
behavior. The determinism promise is conditional on matching backend versions.

## 8. Release sign-off

The release is ready when:

- [ ] `make verify` exits `0` on a clean checkout of the release commit (skips
      recorded and accepted).
- [ ] Every row in sections 1–7 has current, resolving evidence, or a recorded
      waiver with a tracking issue.
- [ ] Support claims are reconciled across README, support matrix, changelog,
      release notes, and `refute doctor`.
- [ ] `CHANGELOG.md` has a dated entry for the release with Supported /
      Experimental / Not-Claimed sections and any compatibility notes.
- [ ] Known limitations (section 5) are stated in the release notes.
- [ ] Artifacts, manifest, and checksums are produced and the install smoke
      test passes (`refute version`, `refute doctor`).

Tagging and publishing the release per `docs/release.md` is tracked in issue
#60 and is out of scope for this criteria document.
