# Current State

This assessment reflects the repository state on `main` as of 2026-05-09.

## Summary

`refute` is past the empty-prototype stage and is approaching a v0.1 dogfood
release. The single-shot CLI is functional, the JSON output contract is stable,
position handling is correct for the supported path, and Go is the primary
well-tested target. CI, release automation, a changelog, and a nightly installer
are in place.

The MCP server, daemon, backend pool, server acquisition, ast-grep adapter,
Python rope adapter, and most higher-level refactorings remain planned work.

The external LSP landscape still matters. LSP standardizes rename and code
actions, but refactoring support varies sharply across languages. `refute`
should keep treating LSP as a strong baseline with room for backend-specific
escape hatches.

## Implemented

### CLI Surface

The `cmd/refute` binary delegates to `internal/cli`. The root command supports
global `--config`, `--dry-run`, and `--verbose` flags. Implemented subcommands:

- `version` — prints build-stamped Version, Commit, and BuildDate via
  `-ldflags` injection;
- `rename`;
- `rename-function`;
- `rename-class`;
- `rename-field`;
- `rename-variable`;
- `rename-parameter`;
- `rename-type`;
- `rename-method`;
- `extract-function`;
- `extract-variable`;
- `inline`;
- `doctor` — reports configured languages, backend availability, installed
  tools, and supported operations; supports `--json`.

Rename supports three input tiers:

- `--symbol` for qualified-name resolution through the backend (Tier 1);
- `--file --line --name` for identifier-boundary-validated line scanning
  (Tier 2);
- `--file --line --col` for exact position (Tier 3).

### Backend Contract

`internal/backend/backend.go` defines a `RefactoringBackend` interface with
lifecycle methods, symbol lookup, rename, extract, inline, move, and capability
reporting. Unsupported operations return `backend.ErrUnsupported`.

### LSP Backend

`internal/backend/lsp` contains a real JSON-RPC/LSP client:

- starts an LSP subprocess over stdio;
- performs initialize/initialized;
- tracks `$/progress` notifications and waits for server idle;
- enforces a timeout on pending requests to avoid hangs;
- bounds incoming message sizes to prevent runaway memory use;
- surfaces server stderr in failure messages;
- sends `textDocument/didOpen`;
- calls `textDocument/rename`;
- calls `textDocument/codeAction` and `codeAction/resolve`;
- calls `workspace/symbol`;
- parses LSP `WorkspaceEdit` responses into the common edit model.

The LSP adapter implements rename, extract-function, extract-variable,
inline-symbol, and symbol lookup. It includes retry handling for rename races
(LSP content-modified errors). `Capabilities()` now accurately reports all four
implemented operations.

### LSP State of the Art

The [LSP specification](https://github.com/Microsoft/language-server-protocol/blob/gh-pages/_specifications/lsp/3.17/specification.md)
standardizes `textDocument/rename`, `workspace/symbol`, `WorkspaceEdit`, and
`textDocument/codeAction`, but it does not standardize a rich refactoring
taxonomy. Code actions may return edits directly, require `codeAction/resolve`,
or return commands that must be executed by the server. That means `refute` can
normalize transport mechanics but still needs language-specific operation
mapping and backend-specific escape hatches.

| Language | Main LSP/refactoring substrate | State for `refute` |
|---|---|---|
| Go | [`gopls`](https://go.dev/gopls/features/transformation) | Strong baseline. `gopls` documents rename plus extract function/method/variable, extract declarations to a new file, inline call, and parameter movement actions. It also documents that extract is less rigorous than rename/inline in some cases, so `refute` should keep Go-specific tests around each claimed operation. |
| TypeScript/JavaScript | TypeScript language service via [`typescript-language-server`](https://github.com/typescript-language-server/typescript-language-server) or direct ts-morph/tsserver APIs | Strong semantic engine, but many higher-level refactorings are exposed as TypeScript-specific refactor actions or workspace commands rather than simple generic LSP edits. `refute` should use the common rename/edit path where possible and keep a TypeScript-specific adapter for extract, move, organize imports, file rename, and interactive refactor arguments. |
| Python | Pyright/Pylance/Basedpyright plus rope | This is the most important missing planning area. [`pyright`](https://github.com/microsoft/pyright) is an open-source type checker with language-server functionality, while [`Pylance`](https://github.com/microsoft/pylance-release/blob/main/FAQ.md) is a closed-source language server that adds refactoring code actions such as extract variable and extract method on top of Pyright. VS Code's [Python editing docs](https://code.visualstudio.com/docs/python/editing#_refactorings) list Pylance refactorings including extract variable, extract method, rename module, move symbol, and implement inherited abstract classes. [`basedpyright`](https://docs.basedpyright.com/latest/configuration/language-server-settings/) is an open-source alternative with language services and additional settings beyond Pyright. [`rope`](https://rope.readthedocs.io/en/latest/rope.html) directly supports Python refactorings such as rename, extract, inline, move, and change signature. For robust Python refactoring, `refute` should probably pair LSP rename/discovery with rope for semantic refactorings. |
| Rust | [`rust-analyzer`](https://rust-analyzer.github.io/book/assists.html) | Good LSP foundation for rename, symbol discovery, assists, and code actions, but action names and availability are rust-analyzer-specific. Existing Rust rename fixtures are useful; extract/inline claims need direct rust-analyzer tests before being documented as supported. |
| Java | [`eclipse.jdt.ls`](https://github.com/eclipse-jdtls/eclipse.jdt.ls) plus OpenRewrite | JDT LS is a mature Java LSP with code actions, quick fixes, source actions, and refactorings. OpenRewrite is still valuable for large recipe-driven transformations that exceed normal editor refactorings. `refute` should support both rather than forcing Java into one generic path. |
| Kotlin | [`kotlin-lsp`](https://github.com/Kotlin/kotlin-lsp) and IntelliJ-derived tooling | Official Kotlin LSP exists but is explicitly pre-alpha/experimental. Kotlin support should be treated as exploratory unless a stable backend is pinned and tested. |
| C/C++ | [`clangd`](https://clangd.llvm.org/features) | `clangd` has production LSP support and documented rename, but its own docs call out limitations around templates, macros, overridden methods, comments, and stale indexes. `refute` can likely support rename first, with careful refusal behavior for broader refactorings. |
| C# | Roslyn-backed LSPs, C# Dev Kit/Roslyn, `csharp-ls` | Roslyn is a strong semantic foundation, but the LSP ecosystem is split between Microsoft tooling and community servers. Some integrations require non-standard extensions. Treat C# as a later backend-specific adapter, not a generic-LSP-only target. |
| PHP/Ruby | Intelephense, Solargraph | These are useful LSPs for rename and code intelligence, but support and licensing differ. Intelephense gates rename/code actions behind premium licensing, while Solargraph supports rename and marks code actions as work in progress. These should be opportunistic later targets. |

### Backend Selection

`internal/backend/selector` maps file extensions to backend choices:

- TypeScript/JavaScript prefer ts-morph when available;
- Java/Kotlin prefer OpenRewrite;
- other configured languages use the generic LSP adapter.

Built-in language-server defaults live in `internal/config/config.go` for Go,
Rust, TypeScript, JavaScript, Python, Java, and Kotlin. Project and user config
layers can override server commands.

### ts-morph Adapter

`internal/backend/tsmorph` wraps a Node.js script at
`adapters/tsmorph/rename.cjs`. The adapter supports rename and Tier 1 symbol
resolution via `workspace/symbol`-equivalent calls through tsserver. It checks
for Node.js, the wrapper script, and installed `ts-morph` dependencies before
claiming availability.

The adapter is **implemented but not packaged** — it requires repo-local
adapter assets and will not work from a bare `go install` build until adapter
packaging is resolved.

### OpenRewrite Adapter

`internal/backend/openrewrite` defines a JVM subprocess adapter for Java/Kotlin
rename paths. It expects a shaded adapter JAR at
`adapters/openrewrite/target/openrewrite-adapter.jar` and shells out to
`java -jar`. The adapter source lives under `adapters/openrewrite/src/` and is
built with Maven:

```bash
mvn -B package --file adapters/openrewrite/pom.xml
```

The build requires JDK 17 or newer and Maven on PATH. The Go side can build
rename request parameters and parse line-delimited JSON responses, but
OpenRewrite remains **unsupported for v0.1** while adapter packaging is
deferred.

### Edit Model and Applier

`internal/edit` contains:

- `WorkspaceEdit`, `FileEdit`, and `TextEdit` types;
- a diff renderer;
- JSON rendering for structured output with a stable `schemaVersion: "1"` contract;
- a file applier that computes edits in memory, writes `.refute.tmp` sidecar
  files, and renames them into place.

The applier is now rollback-safe for multi-file operations: if a rename fails
after some files have been replaced, already-written files are reverted. The
JSON contract covers `applied`, `dry-run`, `no-op`, `ambiguous`, `unsupported`,
`backend-missing`, and `backend-error` statuses.

### Position Encoding

CLI columns are 1-indexed byte offsets. LSP uses 0-indexed UTF-16 code units.
The adapter correctly converts between the two using
`byteColumnToUTF16Character` and `utf16CharacterToByteColumn`, with round-trip
tests covering ASCII, multi-byte Unicode, and emoji. The earlier deferred-status
note in `docs/position-encoding.md` is no longer accurate.

### Symbol Resolution

Tier 2 (`--file --line --name`) now validates identifier boundaries using
`findNameMatches`, detects multiple same-name occurrences on the line and
returns ambiguity, and avoids selecting names inside comments or string
literals. Tests cover repeated identifiers, substrings, comments, strings, and
partial identifier matches.

### Invocation Telemetry

A user-local invocation log records command, exit code, timestamp, and duration
per invocation. The log is stored outside the project tree and is strictly
opt-out.

### Tests and Fixtures

The repository has unit tests for config loading, edit application, diff/JSON
rendering, symbol resolution, backend selection, LSP transport/client/adapter
logic, ts-morph adapter behavior, and OpenRewrite helper behavior. Integration
tests under the `integration` build tag exercise end-to-end CLI behavior against
fixture projects. CI requires the supported Go integration path with `gopls`
installed. Rust, TypeScript, JavaScript, and unsupported-language integration
tests are opt-in with `REFUTE_EXPERIMENTAL_INTEGRATION=1`; CI runs them in a
separate non-blocking experimental lane with Rust and TS/JS fixture dependencies
installed.

### Release Infrastructure

- GitHub Actions CI runs on pull requests and main pushes.
- A nightly release workflow builds and publishes pre-release binaries.
- A project-local `INSTALL.md` documents nightly installs for agent-driven
  repositories.
- Version, commit, and build date are stamped via `-ldflags` at build time.
- `docs/release.md` documents repeatable v0.1 release commands.
- A v0.1.0 changelog is in place.

## Partial or Inconsistent

- The top-level target design includes a daemon and MCP server, but no
  `cmd/refuted`, `internal/daemon`, or `internal/mcp` implementation exists.
- The design spec lists `rope`, `ast-grep`, server registry/cache, move, and
  signature refactorings, but those are not implemented.
- Python is configured only as a pyright-backed LSP fallback. There is no
  Python fixture suite, no rope adapter, and no documented decision about
  Pyright/Pylance/Basedpyright/rope responsibilities.
- `MoveToFile` exists in the backend interface but returns unsupported in all
  current adapters.
- Missing-backend errors (missing `gopls`, missing adapter runtime, unsupported
  operation) are not yet typed or structured for JSON emission. Users and agents
  cannot yet reliably distinguish "install this tool" from "operation not
  supported" without parsing stderr.
- The dated design spec says the Go adapter is a gopls CLI/LSP hybrid, but the
  present implementation routes Go through the generic LSP adapter.
- The OpenRewrite adapter expects a JAR that cannot be built from the currently
  committed Java sources because those sources are not present.
- `--json --dry-run` golden tests are not yet comprehensive enough to serve as
  a stability contract; the schema is defined but not golden-tested for all
  outcomes.

## Missing for the Full Vision

- MCP server transport and tool schemas.
- Daemon process, socket protocol, backend lifecycle management, and backend
  pooling.
- `list-symbols` CLI command backed by LSP `workspace/symbol`.
- Structured missing-backend and unsupported-operation errors with JSON output.
- Backend-specific capability tests.
- Python rope adapter.
- ast-grep pattern rewrite adapter and CLI command.
- Golden JSON tests for all agent-facing output shapes.
- Real-world corpus tests pinned to external projects.
- Operation metadata (backend name, language, support path) in every result.

## Main Risks

1. **The design docs are ahead of implementation.**

   Manageable, but the project needs stable status docs so users and agents do
   not mistake target architecture for shipped behavior. The support matrix now
   helps, but it needs to stay in sync as features land.

2. **Backend behavior will drift.**

   LSP servers and refactoring libraries change action titles, response shapes,
   and readiness behavior. Integration tests need to pin expected behavior and
   produce useful skip/failure messages.

3. **Agent safety depends on better introspection.**

   Agents need `list_symbols`, `list_backends`, dry-run JSON, clear
   unsupported-operation errors, and stable MCP schemas before broad automated
   use is safe. `refute doctor` helps; `list-symbols` and structured error JSON
   are still missing.

4. **All-or-nothing editing is now safer but not fully proven.**

   The applier stages content and rolls back on failure, but multi-file apply
   under adversarial conditions (disk full, concurrent writes) has not been
   tested at scale.

## Recommended Near-Term Focus

The CLI core is now honest, well-tested, and agent-readable for Go. The next
highest-leverage work is:

- finish golden tests for all JSON output shapes (`--json --dry-run` coverage);
- add typed structured errors for missing backend, missing adapter runtime, and
  unsupported operation;
- add a `list-symbols` command backed by LSP `workspace/symbol`;
- then build the MCP layer on top of those contracts.
