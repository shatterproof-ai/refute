---
schema_version: 1
title: Discover candidate symbols before refactoring
slug: list-symbols
status: active
authority: observed
change_resistance: medium
tests_applicable: true
locked_sections: []
---

# Discover candidate symbols before refactoring

## Intent
The user runs `refute list-symbols` with an optional query, file scope, and kind filter, and the tool returns the exact file, line, column, kind, and qualified name of each matching symbol so a refactoring can be requested without guessing coordinates.

## Story
Agents that drive `refute` need to locate a symbol's precise position before invoking a position-based (Tier-3) operation such as `rename` or `extract`. Rather than scraping source files heuristically, the agent asks `refute list-symbols` for candidates. `refute` resolves them through the language server's `workspace/symbol` request (gopls for Go, rust-analyzer for Rust, typescript-language-server for TypeScript), primes the workspace so the whole module is visible, and returns each candidate with the coordinates needed to act on it. When more than one symbol matches, the agent disambiguates by file and line; when none match, the listing is empty but still successful. Languages without an LSP backend (e.g. Java, Kotlin) return a structured unsupported result instead of a backend failure.

## Expected Behavior
Running `refute list-symbols --query <name>` prints every matching symbol as `file:line:column<TAB>kind<TAB>qualifiedName`. `--file <path>` restricts results to symbols declared in that file and selects the language from the file's extension; `--lang <key>` selects the language explicitly (default `go`); `--kind <kind>` filters by symbol kind. With `--json`, the output is a structured envelope (`schemaVersion`, `status`, `symbols[]`) where each symbol carries `file`, `line`, `column`, `kind`, `name`, `container`, and `qualifiedName`. An empty match set returns status `ok` with an empty `symbols` array. An unsupported language returns status `unsupported` with an `unsupported-language` error.

## Boundaries
This story does not modify source code; it is read-only discovery. It does not guarantee that every symbol in a workspace is returned for an empty query — coverage depends on what the language server reports for the primed workspace. It does not resolve symbols for languages that lack an LSP backend.

## Auditable Claims
- `refute list-symbols` is registered and callable.
- `--json` emits a structured envelope with a `symbols` array including `file`, `line`, `column`, `kind`, and `qualifiedName`.
- An unsupported language (e.g. `--lang java`) returns status `unsupported` with code `unsupported-language`.
- An invalid `--kind` value exits with an error.
- Results are resolved via the LSP `workspace/symbol` request.

## Evidence
### Tests
- `internal/cli/listsymbols_test.go`
- `internal/integration_test.go`
### Surface
- `cli: refute list-symbols --query <name>`
- `cli: refute list-symbols --file <path>`
- `cli: refute list-symbols --kind <kind>`
- `cli: refute list-symbols --lang <key> --json`
### Docs
- `docs/support-matrix.md`
