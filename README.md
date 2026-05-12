# refute

Automated source code refactoring from the command line. `refute` drives
existing language servers and refactoring engines (gopls, rust-analyzer,
ts-morph, OpenRewrite) to deliver IDE-quality refactorings as scriptable CLI
operations.

## Status

This is an early dogfood release (v0.1). The single-shot CLI is the only
supported surface; the daemon and MCP server discussed in design docs are
explicitly out of scope for this release.

| Language | Backend | Status |
| --- | --- | --- |
| Go | gopls (LSP) | Supported — the primary v0.1 target. |
| Rust | rust-analyzer (LSP) | Experimental — covered in CI, still conditional for dogfood confidence. |
| TypeScript / JavaScript | typescript-language-server (LSP) | Experimental — ts-morph adapter not packaged. |
| Java / Kotlin | OpenRewrite | Not claimed for v0.1. |
| Python | pyright (LSP) | Planned. |

See [`docs/support-matrix.md`](docs/support-matrix.md) for the canonical
matrix, including extensions, dependency install commands, operation
coverage, test coverage, and the promotion process. Run `refute doctor`
to see the same data evaluated against your local environment.
See [`docs/release.md`](docs/release.md) for repeatable v0.1 release commands
and artifact naming.

A feature is "supported" only when it has documentation, integration
coverage, and a known install path. Everything else is best-effort and may
regress between releases.

## Install

```bash
go install github.com/shatterproof-ai/refute/cmd/refute@latest
refute version
```

`refute` builds with the toolchain pinned in `go.mod` (currently Go 1.26.1).
See [`INSTALL.md`](INSTALL.md) for project-local nightly installs intended for
agent-driven repositories.

## Backend prerequisites

`refute` invokes a language server or adapter for each supported language.
Install the backend for the language(s) you plan to refactor:

```bash
# Go (required for the supported path)
go install golang.org/x/tools/gopls@latest

# TypeScript / JavaScript (experimental)
npm install -g typescript-language-server typescript

# Rust (conditional)
rustup component add rust-analyzer
```

Refactoring quality is bounded by the backing language server. Out-of-date
backends produce out-of-date refactorings.

## First use

Preview a Go rename without touching the working tree:

```bash
refute rename \
  --file testdata/fixtures/go/rename/util/helper.go \
  --line 4 \
  --name FormatGreeting \
  --new-name BuildGreeting \
  --dry-run
```

Apply the same rename:

```bash
refute rename \
  --file testdata/fixtures/go/rename/util/helper.go \
  --line 4 \
  --name FormatGreeting \
  --new-name BuildGreeting
```

## Dry-run and JSON

Two flags shape every operation's output:

- `--dry-run` — print the unified diff to stdout; no files are modified.
- `--json` — emit a structured result on stdout. Combine with `--dry-run` for
  a machine-readable preview.

Scripts and agents should default to `--json --dry-run` to inspect a proposed
refactoring before applying it.

## Operations

| Command | Purpose |
| --- | --- |
| `refute rename` | Rename a symbol. Kind-specific variants: `rename-function`, `rename-class`, `rename-field`, `rename-variable`, `rename-parameter`, `rename-type`, `rename-method`. |
| `refute extract-function` | Extract a selection into a new function. |
| `refute extract-variable` | Extract a selection into a new variable. |
| `refute inline` | Inline a variable or function call at the given position. |
| `refute doctor` | Report which language backends are installed and ready. Supports `--json`. |
| `refute version` | Print version, commit, and build date. |

Run `refute <command> --help` for full flag documentation.

## Known limitations

- Rust, TypeScript, and JavaScript support is experimental and may regress
  between releases.
- Python support is planned but not release-supported in v0.1.
- Java and Kotlin support through OpenRewrite is not claimed for v0.1.
- Some backends (notably ts-morph and OpenRewrite) currently ship as
  repo-local adapter assets and will not work end-to-end from a bare
  `go install` build until adapter packaging is resolved.
- There is no daemon or MCP server in v0.1. Every invocation starts a fresh
  language server.

## Reporting issues

File issues at https://github.com/shatterproof-ai/refute/issues.
