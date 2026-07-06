---
schema_version: 1
title: Change a function signature by removing an unused parameter
slug: change-signature
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Change a function signature by removing an unused parameter

## Intent
The user points `refute change-signature` at a function parameter and asks to remove it; the tool rewrites the declaration and every call site in one pass, or refuses without touching anything when the removal cannot be done safely.

## Story
When a parameter outlives its usefulness, a developer or agent wants it gone from the declaration and every call site without hand-editing each caller. The user supplies the file path and the 1-indexed line/column of the parameter and passes `--remove`. On Go, `refute change-signature` drives gopls's `removeUnusedParam` code action, which gopls offers only when the parameter is genuinely unused by the function body and every call site can be rewritten with confidence. When gopls will not offer that action — because the parameter is used, or its call sites cannot all be resolved — the tool refuses rather than emitting a partial edit, and the workspace is left unchanged. This is the first parameterized operation: it flows through the optional `ParameterizedBackend` structured `Refactor` entry point rather than a fixed-arity backend method.

## Expected Behavior
Running `refute change-signature --file <path> --line <n> --col <c> --remove` removes the parameter at that position, updating the declaration and all call sites, and exits 0. With `--dry-run` the same invocation prints the diff and modifies no files. With `--json` it emits a single structured envelope. When the removal is unsafe (the parameter is in use, or call sites cannot all be rewritten), the command exits non-zero with the `unsafe-refactor` error code under the `unsupported` status and changes nothing. `--file`, `--line`, `--col`, and `--remove` are all required; `--remove` is the only signature change supported in this release.

## Boundaries
This story covers only removing a single, unused parameter on Go. It does not cover adding, reordering, or renaming parameters, changing return types, or any non-Go language — those requests are refused. The declarative parameter-edit model in `SignatureParams` carries the full add/remove/reorder vocabulary so the design generalizes, but only a single `remove` edit is honored today. It does not provide rollback beyond the refuse-before-edit guarantee.

## Auditable Claims
- `--file`, `--line`, `--col`, and `--remove` are all required; omitting any exits with an error.
- A parameter that is still in use is refused with the `unsafe-refactor` error code under the `unsupported` status, exit code 1, and nothing is changed.
- Removing an unused parameter rewrites the declaration and every call site, including cross-file call sites, and the project still compiles.
- `--dry-run` prints the diff and modifies no files.
- `--json` is accepted and emits a single structured envelope for both success and refusal.

## Evidence
### Tests
- `internal/cli/validate_test.go` — `TestValidateChangeSignatureFlags` covers required-flag and `--remove` validation.
- `internal/cli/operation_error_test.go` — `TestEmitJSONOperationError_StatusRouting` (`unsafe-refactor` case) covers `ErrUnsafeRefactor` mapping to the `unsupported` status with the `unsafe-refactor` code.
- `internal/backend/lsp/capabilities_test.go` — `TestAdapterCapabilitiesMatchSupportMatrix` pins that Go advertises `change-signature` and agrees with `config.SupportMatrix`.
- `internal/integration_go_test.go` (build tag `integration`) — `TestEndToEnd_ChangeSignatureRemoveParam` (apply + cross-file rewrite + recompile), `TestEndToEnd_ChangeSignatureDryRun` (diff, no file change), `TestEndToEnd_ChangeSignatureRefusesUsedParam` (refusal leaves files untouched), and `TestEndToEnd_ChangeSignatureJSON` (structured envelope) run end-to-end against `testdata/fixtures/go/changesig/` via gopls.
### Surface
- `cli: refute change-signature --file <path> --line <n> --col <c> --remove`
### Docs
- `docs/support-matrix.md`
- `docs/json-schema.md`
