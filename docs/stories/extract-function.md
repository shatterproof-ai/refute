---
schema_version: 1
title: Extract a code selection into a function
slug: extract-function
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Extract a code selection into a function

## Intent
The user selects a range of code by start and end position, and `refute extract-function` lifts it into a new named function with the correct signature.

## Story
When a block of code inside a function grows complex enough to deserve a name, the developer or agent wants to extract it without manually managing parameter lists, return types, and call-site rewrites. The user supplies the file path and a rectangular byte range (start line/col, end line/col) along with an optional name for the new function. `refute extract-function` hands the range to the language server, which computes the right parameter and return types, creates the new function, and replaces the original range with a call to it. All affected files are updated in one pass.

## Expected Behavior
Running `refute extract-function --file <path> --start-line <n> --start-col <c> --end-line <m> --end-col <d> --name <fn>` introduces a new function named `<fn>` and replaces the selected range with a call. The `--name` flag is optional; when omitted the language server chooses a default name. With `--json`, the output is a structured description of edits. The command exits with an error if the backend returns no edits.

## Boundaries
This story does not cover extracting to a different file or package. It does not guarantee a particular parameter naming style — that is controlled by the language server. It does not provide rollback.

## Auditable Claims
- `--file`, `--start-line`, `--start-col`, `--end-line`, and `--end-col` are all required flags; omitting any exits with an error.
- `--name` is optional; when absent the backend default is used.
- Zero edits from the backend exits with a `NoEditsError`.
- `--json` is accepted and emits structured output.

## Evidence
### Tests
- (integration tests consuming fixtures under `testdata/`)
### Surface
- `cli: refute extract-function --file <path> --start-line <n> --start-col <c> --end-line <m> --end-col <d>`
### Docs
- `docs/support-matrix.md`
