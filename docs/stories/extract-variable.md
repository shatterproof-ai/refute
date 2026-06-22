---
schema_version: 1
title: Extract a code selection into a variable
slug: extract-variable
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Extract a code selection into a variable

## Intent
The user selects an expression by start and end position, and `refute extract-variable` binds it to a new named variable at the appropriate scope.

## Story
Complex or repeated sub-expressions benefit from being named. When a developer or agent wants to pull a sub-expression out and give it a meaningful name, they supply the file and range, optionally with a name. `refute extract-variable` sends the range to the language server, which decides the correct binding location and type, introduces the new variable declaration, and rewrites the original expression to reference it. The change is applied across all affected files in a single pass.

## Expected Behavior
Running `refute extract-variable --file <path> --start-line <n> --start-col <c> --end-line <m> --end-col <d> --name <var>` introduces a new variable bound to the selected expression and replaces the expression with the variable name. `--name` is optional. With `--json`, structured edit output is emitted. The command exits with an error if the backend returns no edits.

## Boundaries
This story does not cover hoisting a variable beyond the immediately enclosing scope when the language server does not support it. It does not promise a specific declaration keyword (e.g., `let` vs `const` in TypeScript). It does not provide rollback.

## Auditable Claims
- `--file`, `--start-line`, `--start-col`, `--end-line`, and `--end-col` are all required; omitting any exits with an error.
- `--name` is optional.
- Zero edits from the backend exits with a `NoEditsError`.
- `--json` is accepted and emits structured output.

## Evidence
### Tests
- `internal/cli/validate_test.go` — `TestValidateExtractFlags` (required flags / range validation, shared by both extract commands) and `TestCommandsRejectPositionalArgs` (extract-variable rejects stray args).
- `internal/cli/operation_error_test.go` — `TestOperationCommands_JSONBackendMissing` ("extract-variable" case) exercises `--json` structured output; `TestEmitJSONOperationError_StatusRouting` ("no-op" case) covers `NoEditsError` mapping to exit code 2.
- `internal/integration_test.go` (build tag `integration`) — `TestEndToEnd_ExtractRustVariable` (Rust via rust-analyzer) and `TestEndToEnd_RustSnippetPlaceholderStripped` run the real end-to-end extraction against fixtures under `testdata/fixtures/`.

Missing coverage: there is no Go end-to-end extract-variable test (only Rust is exercised end-to-end), no test asserts `--name` is optional by omitting it and expecting success, and `NoEditsError` is only reached through the shared router test rather than via an extract backend returning zero edits. Adding a Go integration case, a `--name`-omitted case, and an empty-edit backend case would close these gaps.
### Surface
- `cli: refute extract-variable --file <path> --start-line <n> --start-col <c> --end-line <m> --end-col <d>`
### Docs
- `docs/support-matrix.md`
