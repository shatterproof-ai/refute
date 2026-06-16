# Adapter Wire Contracts

`refute` drives two refactoring backends through subprocess adapters whose
request/response shapes are a wire contract between a Go driver and a
non-Go adapter:

| Backend | Go driver | Adapter | Transport |
| --- | --- | --- | --- |
| ts-morph (TypeScript/JavaScript) | `internal/backend/tsmorph` | `adapters/tsmorph/rename.cjs` (Node) | one JSON object per process invocation |
| OpenRewrite (Java/Kotlin) | `internal/backend/openrewrite` | `adapters/openrewrite` (Java) | newline-delimited JSON-RPC 2.0 over a long-lived process |

Both contracts were previously implicit — two parallel implementations whose
agreement was "an accident of low churn rather than an enforced property." This
document is the source of truth for both, and shared golden fixtures pin both
sides to it.

## Protocol version

Every request and response on both protocols carries an integer
`protocolVersion`. The driver sends its version; the adapter rejects a request
whose version it does not support, and echoes its version on every response; the
driver rejects a response whose version does not match. A mismatch is a hard,
actionable error (reinstall/rebuild the adapter), never a best-effort execution
against an unknown shape.

The current version is **1** on both protocols. The constants that must stay in
lockstep:

| Side | Symbol |
| --- | --- |
| tsmorph Go | `tsmorph.ProtocolVersion` (`internal/backend/tsmorph/adapter.go`) |
| tsmorph Node | `PROTOCOL_VERSION` (`adapters/tsmorph/rename.cjs`) |
| openrewrite Go | `openrewrite.ProtocolVersion` (`internal/backend/openrewrite/adapter.go`) |
| openrewrite Java | `Main.PROTOCOL_VERSION` (`adapters/openrewrite/.../Main.java`) |

Bump the version on **all** sides of a protocol whenever its request or response
shape changes incompatibly, and update the corresponding golden fixtures.

## Golden fixtures

Shared request/response fixtures live under `testdata/adapter-contracts/` and are
consumed by **both** sides of each protocol so neither can drift unobserved:

- `testdata/adapter-contracts/tsmorph/` — consumed by
  `internal/backend/tsmorph/contract_test.go` (Go) and
  `adapters/tsmorph/contract.test.cjs` (Node).
- `testdata/adapter-contracts/openrewrite/` — consumed by
  `internal/backend/openrewrite/contract_test.go` (Go) and
  `adapters/openrewrite/src/test/java/.../WireContractTest.java` (Java).

Fixtures use the placeholder workspace path `/workspace`; tests that run a real
adapter rewrite the temp workspace path back to the placeholder before
comparing. CI runs both sides: the Go fixtures run in the main `go test ./...`
lane (no backend needed — pure serialization), the Node fixtures in the ts-morph
lane (`npm test`), and the Java fixtures in the OpenRewrite lane (`mvn verify`).

## ts-morph protocol

**Framing.** The Go driver spawns `node rename.cjs` once per operation, writes a
single JSON request object to stdin, and reads a single JSON response object
from stdout. Exit code 0 with a JSON response is success; a non-zero exit is a
failure whose message is taken from stderr (there is no structured error
envelope on this protocol).

**Indexing.** Requests use **1-indexed** `line` and **1-indexed UTF-16**
`column` (JavaScript string-index semantics). Responses use **0-indexed** `line`
and **0-indexed UTF-16** `character`, matching LSP. The Go driver converts
between refute's 1-indexed byte columns and the adapter's UTF-16 columns on the
way in and out.

### rename

Request (`rename.request.json`):

```json
{
  "protocolVersion": 1,
  "operation": "rename",
  "workspaceRoot": "/workspace",
  "file": "/workspace/greeter.ts",
  "line": 1,
  "column": 17,
  "newName": "welcome"
}
```

Response (`rename.response.json`) — one entry per changed file, each a single
full-file replacement whose range spans the original file:

```json
{
  "protocolVersion": 1,
  "fileEdits": [
    {
      "path": "/workspace/greeter.ts",
      "edits": [
        {
          "range": { "start": { "line": 0, "character": 0 }, "end": { "line": 3, "character": 0 } },
          "newText": "export function welcome() {\n  return \"hello\";\n}\n"
        }
      ]
    }
  ]
}
```

### findSymbol

Request (`find-symbol.request.json`) — `file` and `kind` are optional:

```json
{
  "protocolVersion": 1,
  "operation": "findSymbol",
  "workspaceRoot": "/workspace",
  "file": "/workspace/greeter.ts",
  "qualifiedName": "greeter:greet",
  "kind": "function"
}
```

Response (`find-symbol.response.json`) — `candidates` use **1-indexed** line and
**1-indexed UTF-16** column:

```json
{
  "protocolVersion": 1,
  "candidates": [
    { "file": "/workspace/greeter.ts", "line": 1, "column": 17, "name": "greet", "kind": "function" }
  ]
}
```

### Errors

A bad request (unknown `operation`, unknown `protocolVersion`, missing file,
target not found) makes the adapter write the message to stderr and exit 1. The
driver surfaces it as `ts-morph operation failed: <message>`.

## OpenRewrite protocol

**Framing.** The Go driver launches one long-lived JVM and exchanges
**newline-delimited JSON-RPC 2.0** messages: one request object per line to
stdin, one response object per line from stdout. The driver decodes responses
with a streaming `json.Decoder` so large `newContent` payloads are not
truncated. `protocolVersion` travels in the JSON-RPC envelope (a sibling of
`jsonrpc`/`id`/`method`).

**Method.** Only `rename` is implemented, with two parameter shapes selected by
the symbol kind:

- **method rename** — `params.methodPattern` (e.g. `com.example.Greeter greet(..)`) + `newName`
- **type rename** — `params.oldFullyQualifiedName` (e.g. `com.example.Greeter`) + `newName`

Both also carry `params.workspaceRoot`. The adapter validates the params shape
before scanning the workspace, so a malformed request fails fast.

Method-rename request (`rename-method.request.json`):

```json
{
  "jsonrpc": "2.0",
  "protocolVersion": 1,
  "id": 1,
  "method": "rename",
  "params": {
    "workspaceRoot": "/workspace",
    "newName": "hello",
    "methodPattern": "com.example.Greeter greet(..)"
  }
}
```

Type-rename request (`rename-type.request.json`) uses
`"oldFullyQualifiedName": "com.example.Greeter"` in place of `methodPattern`.

Success response (`rename.response.json`) — `result` is one `{ path, newContent }`
entry per changed file, each carrying the full new file contents:

```json
{
  "jsonrpc": "2.0",
  "protocolVersion": 1,
  "id": 1,
  "result": [
    { "path": "/workspace/src/main/java/com/example/Greeter.java", "newContent": "package com.example;\n..." }
  ]
}
```

### Errors

Errors use the JSON-RPC `error` object with a numeric `code` and `message`:

| Code | Meaning |
| --- | --- |
| `-32700` | Parse error (request line was not valid JSON). |
| `-32600` | Invalid request — used for an unsupported `protocolVersion`. |
| `-32000` | Handler error (e.g. params missing both `methodPattern` and `oldFullyQualifiedName`, or a missing required param). |

Error response (`error.response.json`):

```json
{
  "jsonrpc": "2.0",
  "protocolVersion": 1,
  "id": 1,
  "error": { "code": -32000, "message": "params must include either 'methodPattern' or 'oldFullyQualifiedName'" }
}
```

The driver surfaces it as `OpenRewrite error <code>: <message>`.
