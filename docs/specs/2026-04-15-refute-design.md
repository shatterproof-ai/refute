# refute: Unified CLI for Automated Source Code Refactoring

**Repository:** `github.com/shatterproof-ai/refute`
**Language:** Go
**Date:** 2026-04-15

## Overview

`refute` is a command-line tool and MCP server that provides IDE-quality automated refactoring from the terminal. It orchestrates existing open-source refactoring engines behind a unified interface, selecting the best available backend per language and refactoring type.

## Architecture

```
CLI ──JSON-RPC──> Daemon ──JSON-RPC──> Backend subprocess (rope, ts-morph, OpenRewrite)
                    |
Agents ──MCP────>   |     ──LSP──────> LSP servers (gopls, rust-analyzer, etc.)
                    |
                    |     ──CLI──────> ast-grep
```

### Layers

1. **CLI Layer** — parses arguments, routes to the appropriate refactoring command. When the daemon is running, forwards commands over a Unix domain socket. Otherwise operates in single-shot mode (start backend, do the work, shut down).

2. **Daemon** — long-lived process that keeps backends warm between invocations. Listens on a Unix domain socket for CLI commands and serves an MCP interface for AI agents. Manages a pool of backend instances keyed by `(workspaceRoot, backendType)`. Backends launch lazily on first request and evict after configurable idle timeout (default 10 minutes).

3. **Backend Selector** — detects the target language from file extension, looks up registered backends ordered by capability, picks the first backend that supports the requested refactoring, falls back to the generic LSP adapter.

4. **Language Adapters** — each adapter wraps a specific refactoring engine and implements a uniform `RefactoringBackend` interface. Adapters translate `refute` commands into backend-specific API calls and normalize results into `WorkspaceEdit` objects.

5. **Edit Applier** — receives `WorkspaceEdit` from the adapter. In dry-run mode, renders a colored unified diff. In apply mode, writes changes atomically (temp files, then rename into place; rollback all on any failure).

## Backend Selection

| Language | Primary Backend | Fallback |
|---|---|---|
| TypeScript/JavaScript | ts-morph (Node.js subprocess) | LSP via typescript-language-server |
| Python | rope (Python subprocess) | LSP via pyright |
| Java/Kotlin | OpenRewrite (JVM subprocess) | LSP via jdtls |
| Go | gopls (CLI + LSP hybrid) | — |
| Any other language | LSP generic (whatever server is available) | — |
| Structural patterns | ast-grep (CLI subprocess) | — |

Current support status is tracked in [`docs/support-matrix.md`](../support-matrix.md). That file is the source of truth; this design doc describes the target architecture.

All non-Go backends run as subprocesses. The Go daemon manages their lifecycle via JSON-RPC (for rope, ts-morph, OpenRewrite) or LSP protocol (for language servers). Each backend has a thin wrapper script in the target language (Python, Node.js, JVM) that imports the underlying library and exposes it over JSON-RPC on stdin/stdout.

## Backend Interface

Every adapter implements this contract. Returning "unsupported" is valid for any operation.

```go
type RefactoringBackend interface {
    // Lifecycle
    Initialize(workspaceRoot string) error
    Shutdown() error

    // Symbol resolution
    FindSymbol(query SymbolQuery) ([]SymbolLocation, error)

    // Refactorings - each returns a WorkspaceEdit or ErrUnsupported
    Rename(location SymbolLocation, newName string) (*WorkspaceEdit, error)
    ExtractFunction(r SourceRange, name string) (*WorkspaceEdit, error)
    ExtractVariable(r SourceRange, name string) (*WorkspaceEdit, error)
    InlineSymbol(location SymbolLocation) (*WorkspaceEdit, error)
    MoveToFile(location SymbolLocation, destination string) (*WorkspaceEdit, error)

    // Signature refactorings (v1.5)
    AddParameter(location SymbolLocation, param ParameterSpec) (*WorkspaceEdit, error)
    RemoveParameter(location SymbolLocation, paramName string) (*WorkspaceEdit, error)
    ReorderParameters(location SymbolLocation, newOrder []int) (*WorkspaceEdit, error)
    IntroduceParameterObject(location SymbolLocation, params []string, objectName string) (*WorkspaceEdit, error)
    ExpandParameterObject(location SymbolLocation, paramName string) (*WorkspaceEdit, error)

    // Introspection
    Capabilities() []RefactoringCapability
}
```

`WorkspaceEdit` is the universal currency. Regardless of what the underlying engine returns, the adapter translates into this type for the edit applier to consume.

## Concrete Backend Adapters

### ts-morph (TypeScript/JavaScript)

ts-morph is a Node.js library. The adapter manages a Node.js subprocess running a wrapper script (`adapters/tsmorph/`) that accepts JSON-RPC commands and calls ts-morph's API.

- Initialize: create a `Project` from `tsconfig.json`
- Rename: `identifier.rename(newName)` — cross-file reference updates and import rewrites handled by ts-morph
- Change signature (v1.5): programmatic — find function declaration, manipulate parameter list, find call sites via `findReferences()`, update argument lists
- Move: `sourceFile.moveDeclaration()` — updates imports across project
- Extract function: not built-in to ts-morph — falls through to LSP

### rope (Python)

rope is a Python library. The adapter manages a Python subprocess running a wrapper script (`adapters/rope/`).

- Initialize: `rope.base.project.Project`
- Rename: `rope.refactor.rename.Rename`
- Extract: `rope.refactor.extract.ExtractMethod` / `ExtractVariable`
- Inline: `rope.refactor.inline.InlineVariable`
- Change signature (v1.5): `rope.refactor.change_signature.ChangeSignature` — rope supports this natively
- Move: `rope.refactor.move.MoveModule` / `MoveGlobal`

### OpenRewrite (Java/Kotlin)

Runs as a JVM subprocess. Refactorings are "recipes" configured via YAML.

- Rename: `org.openrewrite.java.ChangeMethodName`, `ChangeType`
- Change signature: `org.openrewrite.java.ChangeMethodSignature`
- Slower startup than other backends — benefits most from daemon mode

### gopls (Go)

Hybrid adapter. Uses `gopls` CLI subcommands where they exist, falls through to LSP protocol for code actions.

- Rename: `gopls rename -w <file>:<offset> <newName>` — direct CLI invocation
- Other refactorings: spin up gopls as an LSP server, use LSP code action requests

### LSP Generic (Fallback)

Generic LSP client that works with any language server. Handles the long tail of languages (Rust via rust-analyzer, C# via OmniSharp, etc.).

- Spawns the LSP server as a subprocess, communicates via JSON-RPC over stdio
- Performs initialization handshake, sends `textDocument/didOpen` for target files
- Rename: `textDocument/rename` — dedicated LSP method
- Other refactorings: `textDocument/codeAction` to discover available actions, filter by adapter mapping, `codeAction/resolve` to get the WorkspaceEdit
- Timeout handling: kill server after configurable duration if it hangs

### ast-grep (Structural Patterns)

CLI subprocess. Not tied to a specific language. Used for pattern-based structural rewrites.

- Invoked as: `sg --pattern '<old>' --rewrite '<new>' --lang <lang>`
- Exposed as: `refute pattern --match 'foo($A, $B)' --replace 'foo($B, $A)' --lang typescript`
- Useful for bulk mechanical changes that don't need semantic analysis

## Language Adapter Configuration

Adapters start as plain configuration but can include hook functions for server-specific quirks.

```go
type AdapterConfig struct {
    LanguageID   string            // LSP language identifier
    FilePatterns []string          // e.g., ["*.ts", "*.tsx"]
    ServerCmd    string            // e.g., "typescript-language-server"
    ServerArgs   []string          // e.g., ["--stdio"]
    InitOptions  map[string]any    // server-specific init options

    // Maps refute commands to server-specific code action identifiers
    Refactorings map[string]CodeActionMapping
}

type CodeActionMapping struct {
    CodeActionKind string // e.g., "refactor.extract.function"
    TitlePattern   string // regex fallback for servers with non-standard kinds
}
```

Optional hook functions for cases where config is insufficient:

- `PrepareCodeAction(params) params` — modify request before sending
- `ResolveCodeAction(action) action` — post-process returned code action
- `TransformEdit(edit) edit` — modify workspace edit before application

Defaults are pass-through. Simple adapters remain config-only.

## Refactoring Commands

### Rename (split by kind)

- `refute rename-function`
- `refute rename-class`
- `refute rename-field`
- `refute rename-variable`
- `refute rename-parameter`
- `refute rename-type`
- `refute rename` (kind-agnostic, requires exact position via tier 3)

### Extract/Inline

- `refute extract-function --file <path> --start-line <n> --start-col <n> --end-line <n> --end-col <n> --name <name>`
- `refute extract-variable --file <path> --start-line <n> --start-col <n> --end-line <n> --end-col <n> --name <name>`
- `refute inline --file <path> --line <n> --col <n>` or `--symbol <name> --kind <kind>`

### Move

- `refute move --file <path> --line <n> --col <n> --destination <path>`
- Or: `refute move --symbol "ClassName" --destination <path>`

### Pattern (ast-grep)

- `refute pattern --match '<pattern>' --replace '<replacement>' --lang <lang>`

### Signature Refactorings (v1.5)

- `refute add-parameter --symbol "Foo#bar" --name "ctx" --type "context.Context" --position 0 --default "context.Background()"`
- `refute remove-parameter --symbol "Foo#bar" --name "unused"`
- `refute reorder-parameters --symbol "Foo#bar" --order "ctx,name,opts"`
- `refute introduce-parameter-object --symbol "Foo#bar" --params "name,age,email" --object-name "UserInfo"`
- `refute expand-parameter-object --symbol "Foo#bar" --param "opts"`

## Symbol Resolution

Three tiers of symbol identification. All are first-class inputs. The tool detects the mode from which flags are present.

### Tier 1: Qualified Name

`refute rename-function --symbol "ClassName#methodName" --new-name "newName"`

Uses backend-specific resolution:
- ts-morph: project type-checker, searches source files for matching class + method
- rope: `rope.contrib.findit.find_definition()` with module path parsing
- OpenRewrite: accepts fully-qualified Java names directly
- gopls / LSP generic: `workspace/symbol` request, filter by kind and container name

Qualified name syntax is language-specific, defined per adapter:
- TypeScript: `ClassName.methodName` or `module/path:ClassName.methodName`
- Python: `module.ClassName.method_name`
- Java: `com.package.ClassName#methodName`
- Go: `package.FunctionName` or `package.Type.MethodName`

### Tier 2: File + Line + Name

`refute rename-variable --file src/foo.ts --line 42 --name "x" --new-name "y"`

The adapter scans the specified line for an occurrence of the given name. For backends with AST access (ts-morph, rope), it locates the AST node at that line matching the name. For LSP backends, it searches the line text for the string and derives the column, then uses `textDocument/prepareRename` to confirm.

Column is never required from the user in this tier.

### Tier 3: File + Line + Column

`refute rename --file src/foo.ts --line 42 --col 12 --new-name "y"`

Direct position. Passed straight to the backend. Works for scripting and automation where the caller already knows the exact position.

### Ambiguity Handling

If a symbol query matches multiple locations, `refute` prints all candidates with file:line context and exits with code 1. The user narrows with a more qualified name or adds `--file` to scope the search.

## Edit Application

### WorkspaceEdit Structure

A map of file paths to arrays of text edits (range + replacement string), plus optional file creates, renames, and deletes.

### Apply Logic

1. Sort edits per file in reverse document order (bottom-up) so earlier edits don't shift positions of later ones
2. Apply each text edit as a string replacement at the specified byte range
3. For document changes: create/rename/delete files as specified
4. Atomic writes: write to temp files first, then rename into place. If any write fails, roll back all changes

### Dry-Run Mode

`--dry-run` flag on any command:
- Computes edits but writes nothing
- Renders a colored unified diff to stdout
- Exit 0 if edits exist, exit 2 if no edits (useful for scripting)

### Output on Apply

- Print summary: `Modified 3 files: src/foo.ts, src/bar.ts, src/types.ts`
- `--verbose` flag also shows the full diff

## Server Management

### LSP Server Acquisition

Resolution order:
1. Explicit path in config file or CLI flag (override)
2. `refute` managed cache (`~/.cache/refute/servers/`)
3. `$PATH` lookup (use existing installation)

On first use for a language, if no server is found on `$PATH` or in cache, `refute` fetches the appropriate VS Code extension from the **Open VSX** registry, extracts the bundled server binary, and caches it locally.

### Server Registry

Shipped as a data file within `refute`, mapping languages to Open VSX extension IDs and the path to the server binary within the extension package:

```json
{
  "typescript": {
    "extensionId": "typescript-language-server/typescript-language-server",
    "serverPath": "server/src/server.js",
    "command": "node",
    "args": ["${serverPath}", "--stdio"]
  },
  "python": {
    "extensionId": "ms-python.python",
    "serverPath": "bin/pyright-langserver",
    "args": ["--stdio"]
  },
  "go": {
    "extensionId": "golang.go",
    "serverPath": "bin/gopls",
    "args": ["serve"]
  }
}
```

### Cache Management

- `refute servers list` — show installed servers and versions
- `refute servers update` — check Open VSX for newer versions
- `refute servers clean` — remove cached servers

## Daemon Mode

### Process Model

Two binaries:
- `refute` — the CLI client
- `refuted` — the daemon

The daemon listens on a Unix domain socket (`~/.cache/refute/refute.sock`) and serves MCP simultaneously.

### Backend Pool

- Backends keyed by `(workspaceRoot, backendType)`
- Lazy startup: first request for a language starts the corresponding backend
- Idle eviction: backends shut down after configurable timeout (default 10 minutes)
- Different backends can have different timeouts (JVM eviction is more costly than a lightweight LSP server)

### MCP Interface

Exposes refactoring operations as MCP tools:

- `rename` — `{ symbol?, file?, line?, col?, newName, kind? }`
- `extract_function` — `{ file, startLine, startCol, endLine, endCol, name }`
- `extract_variable` — `{ file, startLine, startCol, endLine, endCol, name }`
- `inline` — `{ file, line, col }` or `{ symbol, kind }`
- `move` — `{ symbol, kind, destination }`
- `pattern` — `{ match, replace, lang }`
- `list_symbols` — `{ file?, query?, kind? }` — lets agents discover before acting
- `list_backends` — `{}` — available backends and their capabilities per language
- `daemon_status` — `{}` — running backend instances, uptime, memory

### Startup Modes

- `refute daemon start` — starts daemon with Unix socket + MCP
- `refute daemon start --mcp-only` — MCP transport only
- `refute daemon start --stdio` — MCP over stdio (for editors/agents that manage the process)
- `refute daemon stop` — clean shutdown of all backends
- `refute daemon status` — show running state

### CLI Integration

When a CLI command runs, it checks for the daemon socket. If the daemon is running, the command forwards over the socket. If not, it operates in single-shot mode. The `--daemon` flag or `"daemon": true` in config auto-starts the daemon on first invocation.

## Configuration

### Config File

`refute.config.json` in the project root, or specified via `--config`. Optional — built-in defaults cover the common case.

```json
{
  "servers": {
    "typescript": {
      "command": "/custom/path/to/tsserver",
      "args": ["--stdio"]
    }
  },
  "timeout": 30000,
  "daemon": {
    "autoStart": false,
    "idleTimeout": 600000
  }
}
```

### Resolution Order

1. CLI flags (highest priority)
2. Project-level `refute.config.json`
3. User-level `~/.config/refute/config.json`
4. Built-in defaults

## Error Handling

Refactoring is all-or-nothing. Partial application is worse than failure.

### Error Categories

1. **Backend unavailable** — server not installed, subprocess won't start. Report which backend is needed and how to get it.
2. **Symbol not found** — qualified name doesn't resolve, or position doesn't land on a renameable symbol. Report what was searched for and where.
3. **Ambiguous symbol** — multiple matches. Print candidates with file:line context.
4. **Refactoring unsupported** — backend doesn't support the requested operation. Report what is available.
5. **Backend error** — underlying engine returns an error. Pass through the error message verbatim.
6. **Apply failure** — file write fails. Atomic strategy means all-or-nothing rollback.
7. **Timeout** — backend unresponsive. Kill it, report.

### Exit Codes

- `0` — refactoring applied successfully (or dry-run showed edits)
- `1` — error (any of the above categories)
- `2` — no edits produced (refactoring was a no-op)

## Testing Strategy

### Smoke Tests (CI, every commit)

Per-adapter tests with small fixture source files. Test the full round trip: input command, adapter translation, backend call, WorkspaceEdit production, file content verification.

Each refactoring operation x each supported language = one test suite. Tests assert on resulting file contents, not on WorkspaceEdit structure.

### End-to-End Tests Against Real-World Code (Nightly / Pre-Release)

Clone a curated set of real open-source projects pinned at specific commits:
- TypeScript: mid-size project with cross-module imports, generics, type aliases
- Python: project with class hierarchies, decorators, dynamic imports
- Go: multi-package project with interfaces and embedded types
- Java: Maven project with inheritance, generics, annotations

For each project x each refactoring:
1. Define a specific refactoring to perform
2. Run via `refute`
3. Verify the project still compiles/typechecks
4. Verify all references updated (grep for old name — should be gone from code)
5. Verify the project's own test suite still passes

### Symbol Resolution Tests

- Qualified name resolution per language
- File + line + name: correct identifier selected on lines with multiple identifiers
- Ambiguity: queries matching multiple symbols return all candidates

### Daemon Tests

- Start daemon, send commands over socket, verify responses
- Lazy startup: first TypeScript command starts ts-morph, Python backends remain idle
- Idle eviction: backend subprocess terminates after timeout

### MCP Tests

- Connect as MCP client, invoke tools, verify structured responses
- `list_symbols` and `daemon_status` return correct information

### Regression Collection

When a bug is found (a refactoring that produces incorrect code), the failing case gets extracted into a minimal fixture and added to the smoke suite permanently.

## Project Structure

```
refute/                                  (github.com/shatterproof-ai/refute)
├── cmd/
│   ├── refute/                          # CLI entrypoint
│   └── refuted/                         # Daemon entrypoint
├── internal/
│   ├── cli/                             # Argument parsing, command routing
│   ├── daemon/                          # Socket server, backend pool, lifecycle
│   ├── mcp/                             # MCP server implementation
│   ├── edit/                            # WorkspaceEdit type, atomic applier, diff renderer
│   ├── symbol/                          # Symbol resolution (qualified name, tier selection)
│   ├── backend/                         # Backend interface definition
│   │   ├── tsmorph/                     # ts-morph adapter
│   │   ├── rope/                        # rope adapter
│   │   ├── openrewrite/                 # OpenRewrite adapter
│   │   ├── gopls/                       # gopls adapter
│   │   ├── lsp/                         # Generic LSP adapter
│   │   └── astgrep/                     # ast-grep adapter
│   ├── registry/                        # Open VSX fetching, server caching
│   └── config/                          # Config file loading, resolution order
├── adapters/
│   ├── tsmorph/                         # Node.js wrapper script
│   ├── rope/                            # Python wrapper script
│   └── openrewrite/                     # JVM wrapper / recipe configs
├── testdata/
│   ├── fixtures/                        # Small per-language fixture files
│   └── corpus/                          # Scripts to clone/pin real-world projects
├── docs/
│   └── specs/
└── go.mod
```

## Scope

### v1

- Rename (all kinds: function, class, field, variable, parameter, type)
- Extract function, extract variable
- Inline variable/function
- Move to file
- Pattern-based rewrite (via ast-grep)
- Languages: TypeScript, Python, Go
- Flag-based CLI with three-tier symbol identification
- Daemon mode with MCP server
- Open VSX server management with local caching
- Dry-run preview, atomic file application

### v1.5

- Add parameter
- Remove parameter
- Reorder parameters
- Introduce parameter object
- Expand parameter object
- Java/Kotlin support via OpenRewrite
- Interactive/guided CLI mode
- Hybrid LSP + ast-grep approach for signature refactorings
