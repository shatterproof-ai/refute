# Changelog

## v0.1.0 - 2026-05-06

Initial dogfood release of the `refute` single-shot CLI.

This release is scoped to command-line refactoring workflows. The daemon and
MCP server described in design documents are not part of v0.1.0.

### Supported

- Go is the supported v0.1.0 path via `gopls`.
- Go operations include symbol rename, extract function, extract variable, and
  inline.
- `refute doctor --json` reports local backend availability and install hints.
- `--dry-run --json` provides machine-readable previews before applying edits.
- `refute version` reports build metadata when installed from release
  artifacts produced by `scripts/release.sh`.

### Experimental

- Rust rename is available through `rust-analyzer`. It has CI coverage and
  passed dogfood canaries, but remains experimental while release confidence
  builds.
- TypeScript and JavaScript rename are routed through
  `typescript-language-server` when installed. They remain experimental because
  the ts-morph adapter is implemented but not packaged for a bare
  `go install` build.

### Not Claimed

- Java and Kotlin support through OpenRewrite is not claimed for v0.1.0.
- Python support through pyright is planned but not yet covered by release
  tests.
- Repository-local adapter assets are not bundled into `go install` builds.

### Safety And Reliability

- Workspace edit application is rollback-safe when a later file write fails.
- LSP request handling has per-request timeouts to avoid indefinite waits.
- LSP server stderr is surfaced in startup and request failures.
- LSP transport frames reject non-positive and oversized `Content-Length`
  values before allocating message buffers.
- Integration builds disable VCS stamping where temporary Git metadata would
  otherwise make tests flaky.

### Dogfood Notes

- Clean install from `github.com/shatterproof-ai/refute/cmd/refute` was
  verified for `@main` and `@latest` in a clean temp install area. The reachable
  remote pseudo-version still reports development metadata until v0.1.0 is
  tagged or release artifacts are used.
- Go dogfood canaries passed in disposable temp copies of `zolem`,
  `kapow/api`, `kapow/tools/dbverify`, and `shatter/shatter-go`.
- Rust rename dogfood passed in a disposable temp copy of `shatter/shatter-rust`
  with `rust-analyzer` installed.
- Some broad project verification commands in dogfood repos were too slow for
  the canary window and were classified as inconclusive; focused verification
  around the applied edits passed.

### Release Notes

`refute` v0.1.0 is a CLI-only dogfood release for scripted refactoring.

Use Go through `gopls` as the supported path. Rust, TypeScript, and JavaScript
are experimental. Java, Kotlin, and Python are not release-supported in v0.1.0.

Install the CLI with:

```bash
go install github.com/shatterproof-ai/refute/cmd/refute@latest
```

For release artifacts, use `scripts/release.sh v0.1.0` or the GitHub release
workflow described in `docs/release.md`; those paths stamp version, commit, and
build date metadata into `refute version`.
