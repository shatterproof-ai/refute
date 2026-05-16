---
schema_version: 1
title: Inline a symbol at its use site
slug: inline-symbol
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Inline a symbol at its use site

## Intent
The user points `refute inline` at a variable or function call site, and the tool replaces the reference with the symbol's definition body in place.

## Story
When a named intermediate — a variable holding a computed value, or a single-use helper function — becomes more noise than signal, the developer or agent wants to collapse it back to its inline form. The user supplies the file, line, and column (or a name hint on the line) identifying the call site. For Rust, which requires disambiguating the specific call site, the user may supply `--symbol` together with `--call-site`. `refute inline` forwards the request to the language server, receives the workspace edit, and applies it. The result is a workspace where the reference is gone and the body appears in its place.

## Expected Behavior
Running `refute inline --file <path> --line <n> --col <c>` collapses the symbol at that position and applies edits to all affected files. With `--json`, the output is a structured description of the edits. If the backend reports no edits, the command exits with an informative error rather than silently succeeding.

## Boundaries
This story does not promise inlining across file boundaries where the language server does not support it. It does not cover inlining at definition sites (only at call or use sites). It does not provide rollback.

## Auditable Claims
- `--symbol` without `--call-site` exits with an error on the Rust path.
- `--file` and `--line` are required when `--call-site` is not provided.
- When the backend returns zero edits, the command returns a `NoEditsError` rather than a zero exit code with no output.
- `--json` flag is accepted and emits structured output.

## Evidence
### Tests
- `internal/cli/inline_test.go`
### Surface
- `cli: refute inline --file <path> --line <n> --col <c>`
- `cli: refute inline --symbol <sym> --call-site <file>:<line>:<col>`
### Docs
- `docs/support-matrix.md`
