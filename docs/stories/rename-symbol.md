---
schema_version: 1
title: Rename a symbol across the workspace
slug: rename-symbol
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Rename a symbol across the workspace

## Intent
The user points `refute rename` at a symbol by file and line, provides a new name, and the tool renames every reference in the workspace atomically.

## Story
Agents and developers working in large codebases need to rename identifiers — functions, variables, types, fields, methods, parameters — without manually tracking down every call site. The user supplies the source file, the line (and optionally column or name hint) where the symbol appears, and the desired new name. `refute` delegates to the appropriate language server (gopls for Go, rust-analyzer for Rust, typescript-language-server for TypeScript), receives the full workspace edit, and applies all file changes in one pass. The operation is atomic from the user's perspective: either all references are updated or none are, with a clear error if the backend cannot complete the rename.

## Expected Behavior
Running `refute rename --file <path> --line <n> --name <old> --new-name <new>` applies edits to every file in the workspace that references the symbol and prints a summary of changed files. Typed variants (`rename-function`, `rename-class`, `rename-field`, `rename-variable`, `rename-parameter`, `rename-type`, `rename-method`) behave identically to plain `rename` when the target's actual kind matches the variant, but reject a mismatch before computing any edit: they exit nonzero with a message naming the symbol's actual kind and suggesting `rename` or the matching `rename-<kind>`. Kinds a language does not have (for example `class` in Go and Rust) always error there. With `--json`, a mismatch emits a structured error with `status: "kind-mismatch"` and the same exit code as human mode.

## Boundaries
This story does not cover cross-language renames. It does not provide an undo mechanism; the caller is responsible for version-control rollback. It does not promise to rename symbols that the language server cannot locate (e.g., dynamically constructed names).

## Auditable Claims
- `refute rename` requires `--new-name`; omitting it exits with an error.
- Kind-specific subcommands (`rename-function`, `rename-class`, etc.) are registered and callable.
- A kind-specific subcommand rejects a target whose actual kind differs from the variant (or a kind the language lacks) before any edit, exiting nonzero; `--json` reports `status: "kind-mismatch"`.
- `--json` flag emits structured output rather than human-readable text.
- Backend selection is language-driven: `.go` files use gopls, Rust files use rust-analyzer, TypeScript files use tsmorph.

## Evidence
### Tests
- `internal/cli/rename_test.go`
- `internal/cli/renamekind_test.go`
### Surface
- `cli: refute rename --file <path> --line <n> --new-name <new>`
- `cli: refute rename-function --file <path> --line <n> --new-name <new>`
- `cli: refute rename-class --file <path> --line <n> --new-name <new>`
- `cli: refute rename-field --file <path> --line <n> --new-name <new>`
- `cli: refute rename-variable --file <path> --line <n> --new-name <new>`
- `cli: refute rename-parameter --file <path> --line <n> --new-name <new>`
- `cli: refute rename-type --file <path> --line <n> --new-name <new>`
- `cli: refute rename-method --file <path> --line <n> --new-name <new>`
### Docs
- `docs/support-matrix.md`
