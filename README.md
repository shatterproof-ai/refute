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
| TypeScript / JavaScript | ts-morph adapter preferred; typescript-language-server fallback | Experimental — rename only; ts-morph adapter is a separate dependency. |
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

For projects that already use Go modules with Go 1.24 or newer, prefer
tracking `refute` as a Go tool dependency:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@latest
go tool refute version
```

For personal shell use:

```bash
go install github.com/shatterproof-ai/refute/cmd/refute@latest
refute version
```

`refute` builds with the toolchain pinned in `go.mod` (currently Go 1.26.4).
See [`INSTALL.md`](INSTALL.md) for dependency-managed project installs,
project-local release binaries, and nightlies intended for agent-driven
repositories.

## Backend prerequisites

`refute` invokes a language server or adapter for each supported language.
Install the backend for the language(s) you plan to refactor:

```bash
# Go (required for the supported path)
go install golang.org/x/tools/gopls@latest

# TypeScript / JavaScript (experimental; preferred adapter when available)
npm install -g @shatterproof-ai/refute-ts-adapter

# TypeScript / JavaScript fallback (rename-only LSP path)
npm install -g typescript-language-server typescript

# Rust (conditional)
rustup component add rust-analyzer
```

For TypeScript and JavaScript, `refute` prefers the ts-morph adapter when it is
available in the workspace or configured explicitly, then falls back to
`typescript-language-server` for rename-only LSP coverage.

Refactoring quality is bounded by the backing language server. Out-of-date
backends produce out-of-date refactorings.

## First use

Preview a Go rename without touching the working tree, then apply it:

```bash
# Preview — print the diff to stdout, no files modified.
refute rename \
  --file testdata/fixtures/go/rename/util/helper.go \
  --line 4 \
  --name FormatGreeting \
  --new-name BuildGreeting \
  --dry-run

# Apply the same rename.
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

## Local telemetry

`refute` writes local, opt-out invocation telemetry to:

```bash
~/.local/share/refute/telemetry.jsonl
```

Each invocation records start/end events with timing, exit status, detected
agent session, project identity, backend metadata, and edit counts. Refactoring
operations also store compressed before/planned-after snapshots under:

```bash
~/.local/share/refute/snapshots/
```

When an agent session ID is detected, `refute` appends a human-readable session
transcript under `~/.local/share/refute/sessions/`. Passing `--verbose` prints
the same invocation summary to stderr; normal output stays quiet.

### Telemetry retention & opt-out

Snapshots are on by default. To opt out, set either variable to a disable value
— `0`, `false`, `off`, or `no` (case-insensitive):

- `REFUTE_TELEMETRY=0` disables telemetry entirely (no files written).
- `REFUTE_TELEMETRY_SNAPSHOTS=0` keeps metadata but skips before/after snapshots.

Retention is bounded automatically so telemetry never grows without limit. A
best-effort sweep runs at the end of each invocation and never affects the
refactoring result:

- **Snapshots:** the newest `200` invocation directories are kept, capped at
  `200 MiB` total — whichever limit is reached first. Older directories are
  pruned.
- **`telemetry.jsonl`:** rotated to `telemetry.jsonl.1` once it exceeds `50 MiB`
  (one previous generation retained).

These limits are the named constants `MaxSnapshotInvocations`,
`MaxSnapshotBytes`, and `MaxTelemetryLogBytes` in `internal/telemetry`.

Project identity (including the git dirtiness probe) is detected lazily only for
real operations; informational commands such as `refute version` and
`refute --help` spawn no git subprocesses.

## Operations

| Command | Purpose |
| --- | --- |
| `refute rename` | Rename a symbol. Supports Tier-1 qualified-name lookup with `--symbol <qualified-name>` plus kind-specific variants: `rename-function`, `rename-class`, `rename-field`, `rename-variable`, `rename-parameter`, `rename-type`, `rename-method`. |
| `refute extract-function` | Extract a selection into a new function. |
| `refute extract-variable` | Extract a selection into a new variable. |
| `refute inline` | Inline a variable or function call at the given position. Rust also supports `--symbol <qualified-name>` with `--call-site <file>:<line>:<col>` for single-call-site inline. |
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
