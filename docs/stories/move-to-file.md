---
schema_version: 1
title: Move a declaration to another file in the same package
slug: move-to-file
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Move a declaration to another file in the same package

## Intent
The user points `refute move` at a top-level declaration and names a `--destination` file; the tool moves the declaration — and the imports it needs — into that file, leaving a workspace that still compiles.

## Story
As a Go package grows, a developer or agent wants to split one oversized file into several without hand-copying declarations and fixing up imports. The user identifies the declaration by `--file` and `--line` (with `--col` for an exact column or `--name` to scan the line) and names a `--destination` file in the same package. `refute move` drives gopls's extract-to-new-file command, captures the resulting workspace edit (a new file plus edits removing the declaration and its now-unused imports from the source), retargets the new file to the user's chosen destination, and applies it. Because the move stays within one package, references elsewhere in the package need no rewrite.

Move is deliberately conservative: it refuses rather than emit a broken edit. Moving to a different directory (a different package) is refused because the imports that a cross-package move would require are not computed; moving onto a file that already exists is refused rather than overwriting it; and a target gopls cannot move to a new file is refused as unmovable. Each refusal names the specific constraint, and — because refusal happens before any edit — `--dry-run` and apply behave identically for a refused move.

## Expected Behavior
Running `refute move --file <path> --line <n> --destination <file>` moves the declaration at that position into `<file>` in the same directory and applies the create plus text edits atomically. `--destination` is resolved relative to the source file's directory, so a bare filename lands beside the source. With `--json`, the output is a structured envelope including the `fileOps` create entry. A cross-package destination, an existing destination, or an unmovable symbol exits non-zero with an actionable message; in `--json` mode these report status `unsupported` with error code `unsafe-refactor`. Move is Go-only and experimental for v0.1.

## Boundaries
This story does not promise cross-package moves, moves onto existing files, or moving symbols gopls cannot extract to a new file. It does not cover languages other than Go. It does not provide rollback beyond the applier's all-or-nothing transaction.

## Auditable Claims
- `--file`, `--line`, and `--destination` are required.
- A destination in a different directory than the source is refused with `ErrUnsafeRefactor` code `cross-package-move`.
- A destination file that already exists is refused with `ErrUnsafeRefactor` code `destination-exists`.
- In `--json` mode a refused move reports status `unsupported` and error code `unsafe-refactor`.
- Only Go advertises the `move` operation; other languages refuse it as `unsupported-operation`.

## Evidence
### Tests
- `internal/cli/move_test.go`
- `internal/backend/lsp/adapter_internal_test.go`
- `internal/integration_go_test.go`
### Surface
- `cli: refute move --file <path> --line <n> --destination <file>`
### Docs
- `docs/support-matrix.md`
- `docs/json-schema.md`
