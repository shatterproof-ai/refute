# rust-support Expedition Plan

## Goal

Add Rust language support to `refute` via the `rust-analyzer` LSP server, enabling
rename and dry-run preview for Rust symbols from the CLI.

## Success Criteria

- `refute rename --file foo.rs --line N --name OldName --new-name NewName` works
  end-to-end on a real Rust crate, renaming cross-file references.
- `--dry-run` shows a unified diff without modifying files.
- Unit tests for new config and workspace-detection logic pass (`go test ./...`).
- Integration test for Rust rename passes (skipped when rust-analyzer is not installed).

## Task Sequence

1. **01 config-detect** — Add `rust-analyzer` to `builtinServers`; add `Cargo.toml`/`Cargo.lock` to workspace-root markers.
2. **02 rust-fixture-and-test** — Create Rust testdata fixture and integration test.

### Task 01 — config-detect

**Outcome:** Config and workspace detection recognise Rust out of the box.

**Changes:**
- `internal/config/config.go`: add `"rust": {Command: "rust-analyzer"}` to `builtinServers`
- `internal/cli/rename.go` (`findWorkspaceRoot`): prepend `Cargo.toml` and `Cargo.lock` to the markers slice

**Verification gates:**
- `go test ./internal/config/...`
- `go test ./internal/cli/...`
- `go build ./...`

**Branch slug seed:** `config-detect`

### Task 02 — rust-fixture-and-test

**Outcome:** End-to-end integration test for Rust rename works.

**Changes:**
- `testdata/fixtures/rust/rename/` — small Cargo crate (`Cargo.toml`, `src/lib.rs`, `src/main.rs`)
- `internal/integration_test.go` — `TestEndToEnd_RenameRustFunction`
  (skips if rust-analyzer not on PATH; copies fixture to temp dir; checks cross-file rename)

**Verification gates:**
- `go test -run TestEndToEnd_RenameRustFunction -tags integration ./internal/...`
- `go test ./...`

**Branch slug seed:** `rust-fixture-and-test`

## Experiment Register

None planned.

## Verification Gates

- `go test ./...` — must pass before every merge into the base branch
- `go test -tags integration ./internal/...` — must pass (or skip) on the rebased base branch before landing
