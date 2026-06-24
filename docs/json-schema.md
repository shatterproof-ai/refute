# JSON Output and Exit Codes

This document is the normative reference for `refute --json`, `refute doctor
--json`, and process exit codes. The examples and field names here describe
schema version `"1"`.

## Operation Result Envelope

Refactoring commands that support `--json` write a single JSON object to stdout.
Successful, dry-run, no-op, and error results all use the same top-level
envelope:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `schemaVersion` | string | yes | JSON contract version. Current value is `"1"`. |
| `status` | string | yes | Operation outcome. See [Statuses](#statuses). |
| `operation` | string | no | CLI operation name, such as `rename` or `inline`. |
| `language` | string | no | Detected source language for the operation. |
| `backend` | string | no | Backend selected for the operation, such as `lsp/gopls` or `tsmorph`. |
| `workspaceRoot` | string | no | Absolute workspace root used for backend setup and relative analysis. |
| `filesModified` | number | yes | Count of affected files: file-edit entries in `edits` plus `fileOps` entries; `0` when none are present. |
| `edits` | array | no | File edits grouped by path. Omitted when empty. |
| `fileOps` | array | no | Create/rename/delete file operations. Omitted when empty. See [File Operations](#file-operations). |
| `newSymbol` | object | no | New symbol location after an operation that can report one. |
| `candidates` | array | no | Candidate symbol locations when a request is ambiguous. |
| `warnings` | array | no | Non-fatal warning strings. |
| `error` | object | no | Structured error detail when no edit was produced because of an error. |

Consumers should branch first on `schemaVersion`, then on `status`. Optional
fields can be absent for any status.

## Statuses

The status list below is checked against `internal/edit/json.go`.

<!-- json-statuses:start -->
- `applied` -- the operation completed and applied edits to disk.
- `dry-run` -- the operation completed and returned a preview without modifying files.
- `no-op` -- the operation completed but produced no edits.
- `ambiguous` -- the request matched multiple possible symbols; inspect `candidates`.
- `unsupported` -- the selected language or backend does not support the requested operation.
- `backend-missing` -- a required backend binary or adapter dependency is not available.
- `backend-failed` -- the backend was available but failed while serving the request.
- `invalid-position` -- the requested source position or symbol query did not identify a valid target.
<!-- json-statuses:end -->

Do not treat status as a closed enum in consumers. Future schema versions may
add status strings; older consumers should preserve unknown statuses and route
them to a default branch.

## Coordinates

All JSON coordinates are 1-indexed.

`edits[].changes[]` entries use half-open ranges:

| Field | Type | Meaning |
| --- | --- | --- |
| `startLine` | number | 1-indexed start line. |
| `startCol` | number | 1-indexed start column. |
| `endLine` | number | 1-indexed end line. |
| `endCol` | number | 1-indexed exclusive end column. |
| `newText` | string | Replacement text for the range. |

The replacement covers text from `startLine:startCol` up to, but not including,
`endLine:endCol`. Insertions have identical start and end coordinates.

Columns in CLI arguments and JSON output are 1-indexed byte-offset columns. LSP
backends use UTF-16 code-unit columns internally; refute converts at the LSP
boundary. See [Position Encoding](position-encoding.md) for the Unicode caveat.

`newSymbol` and `candidates` use this location shape:

| Field | Type | Meaning |
| --- | --- | --- |
| `file` | string | Source file path for the symbol. |
| `line` | number | 1-indexed line. |
| `column` | number | 1-indexed byte-offset column. |
| `name` | string | Symbol name. |

## File Operations

`fileOps[]` entries describe create, rename, and delete operations that
accompany text edits (for example, an "extract to new file" refactoring creates
a file and edits both it and the original). The field is additive under
`schemaVersion: "1"`; consumers that ignore it still process `edits` correctly.

| Field | Type | Meaning |
| --- | --- | --- |
| `op` | string | One of `create`, `rename`, or `delete`. |
| `file` | string | Target path for `create`/`delete`; source path for `rename`. |
| `newFile` | string | Destination path for `rename`; omitted otherwise. |

When both `edits` and `fileOps` are present, refute applies creates first (so a
text edit can populate a new file), then text edits, then renames, then
deletes, as a single all-or-nothing transaction.

## Error Object

When present, `error` has this shape:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `code` | string | yes | Stable, machine-readable error category for the failure. |
| `message` | string | yes | Human-readable detail. |
| `hint` | string | no | Suggested remediation, usually an install command or `refute doctor` prompt. |

Scripts should use `status` for high-level routing and `error.code` for a more
specific branch inside an error status.

Common backend setup error codes:

| `status` | `error.code` | Exit | Cause |
| --- | --- | --- | --- |
| `backend-missing` | `backend-missing` | `3` | A configured LSP server binary is missing; `error.hint` carries the install hint when one is known. |
| `backend-missing` | `adapter-runtime-missing` | `3` | A subprocess adapter or adapter runtime dependency is missing, such as the ts-morph package or OpenRewrite adapter JAR; `error.hint` carries the adapter install/build command. |
| `backend-failed` | `backend-init-failed` | `1` | A backend was selected but failed during initialization for a reason other than a missing dependency. |
| `backend-failed` | `backend-unavailable` | `1` | Backend setup failed before the operation could run, but the failure did not match a more specific typed setup category. |

## list-symbols Result

`refute list-symbols --json` is a read-only discovery query rather than an edit,
so it uses its own envelope: a `symbols` array in place of `edits`/`fileOps`,
and the success status `ok` rather than the edit-oriented statuses above. It
resolves candidates through the LSP `workspace/symbol` request. Consumers branch
on `schemaVersion`, then on `status`.

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `schemaVersion` | string | yes | JSON contract version. Current value is `"1"`. |
| `status` | string | yes | `ok` for a successful listing (including an empty match set); otherwise an error status (see below). |
| `operation` | string | no | Always `list-symbols`. |
| `language` | string | no | Resolved query language (from `--lang`, else detected from `--file`, else `go`). |
| `backend` | string | no | Backend used, normally `lsp`. |
| `backendVersion` | string | no | Resolved language-server version, when observable. |
| `workspaceRoot` | string | no | Absolute workspace root used for symbol resolution. |
| `query` | string | no | The `--query` value, when one was supplied. |
| `symbols` | array | yes | Matching symbols. Present and empty (`[]`) when nothing matched. |
| `warnings` | array | no | Non-fatal warning strings, e.g. a `--file` scope that matched no symbols though the server reported some elsewhere. |
| `error` | object | no | Structured error detail when the listing failed. Uses the [Error Object](#error-object) shape. |

Each `symbols[]` entry has this shape:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `file` | string | yes | Absolute path of the file declaring the symbol. |
| `line` | number | yes | 1-indexed line. |
| `column` | number | yes | 1-indexed byte-offset column. |
| `kind` | string | yes | Symbol kind, e.g. `function`, `type`, `field`, `method`, `variable`, `class`, `parameter`. |
| `name` | string | yes | Symbol name as reported by the language server. |
| `container` | string | no | Enclosing container (package, type, module); omitted when none. |
| `qualifiedName` | string | yes | `container.name` when a container is present, else the bare name. |

Entries are sorted by `file`, then `line`, then `column`.

### list-symbols statuses and exit codes

| `status` | `error.code` | Exit | Cause |
| --- | --- | --- | --- |
| `ok` | — | `0` | Listing succeeded. An empty `symbols` array is still `ok`, not an error. |
| `backend-missing` | `backend-missing` | `3` | The language server for the resolved language is not installed; `error.hint` carries the install hint when one is known. |
| `backend-missing` | `adapter-runtime-missing` | `3` | A selected adapter runtime dependency is missing; `error.hint` carries the adapter install/build command. |
| `unsupported` | `unsupported-language` | `1` | The language has no LSP backend (e.g. Java, Kotlin); `error.hint` lists the supported languages. |
| `backend-failed` | `invalid-request` | `1` | An invalid `--kind` value was supplied. |
| `backend-failed` | `backend-init-failed` | `1` | The language backend was selected but failed during initialization for a reason other than a missing dependency. |
| `backend-failed` | `backend-unavailable` | `1` | Backend setup failed before symbol discovery could run, but the failure did not match a more specific typed setup category. |
| `backend-failed` | `operation-failed` | `1` | The backend was reachable but the `workspace/symbol` query failed. |

A successful listing example:

```json
{
  "schemaVersion": "1",
  "status": "ok",
  "operation": "list-symbols",
  "language": "go",
  "backend": "lsp",
  "backendVersion": "golang.org/x/tools/gopls v0.22.0",
  "workspaceRoot": "/abs/workspace",
  "query": "User",
  "symbols": [
    {
      "file": "/abs/workspace/util/user.go",
      "line": 4,
      "column": 6,
      "kind": "type",
      "name": "User",
      "container": "example.com/renametest/util",
      "qualifiedName": "example.com/renametest/util.User"
    }
  ]
}
```

## Doctor Report

`refute doctor --json` writes a `DoctorReport` object to stdout and exits `0`
even when dependencies are missing:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `schemaVersion` | string | yes | JSON contract version. Current value is `"1"`. |
| `command` | string | yes | Always `doctor`. |
| `backends` | array | yes | Backend readiness rows. |

Each `backends[]` row has this shape:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `language` | string | yes | Language the row describes, such as `go` or `typescript`. |
| `backend` | string | yes | Backend identifier, such as `lsp/gopls`, `tsmorph`, or `openrewrite`. |
| `status` | string | yes | Backend readiness: `ok`, `missing`, `experimental`, `planned`, or `not-claimed`. |
| `binary` | string | no | Resolved binary path when a binary dependency is present. |
| `operations` | array | no | Operation names this backend can serve in the current release. |
| `missingDependency` | string | no | Binary or package name that was not found. |
| `installHint` | string | no | Suggested install command for a missing dependency. |
| `caveats` | string | no | Human-readable support note. |

Doctor status meanings:

- `ok` -- installed and claimed as ready for its documented operations.
- `missing` -- required dependency was not found on this host.
- `experimental` -- installed but the language/backend is not release-supported.
- `planned` -- tracked as future support, not release-supported.
- `not-claimed` -- intentionally not claimed for the current release.

## Exit Codes

`refute` uses these process exit codes:

| Code | Meaning |
| --- | --- |
| `0` | Command succeeded. `refute doctor` also exits `0` when it reports missing dependencies. |
| `1` | General failure, invalid request, unsupported operation, ambiguous match, or backend failure. |
| `2` | No edits were produced or no matching symbol was found. In JSON mode this commonly pairs with `no-op` or `invalid-position`. |
| `3` | Required backend binary or adapter dependency is missing. In JSON mode this pairs with `backend-missing`. |

In JSON mode, prefer the JSON `status` and `error.code` fields for detailed
routing. Use the process exit code as the shell-level success/failure signal.

## Stability and Versioning

See [Versioning and Compatibility](versioning.md) for the broader CLI, JSON,
and future MCP compatibility policy.

For `schemaVersion: "1"`:

- Existing field names and meanings are stable.
- Removing a field, renaming a field, or changing a field's meaning requires a
  schema version bump.
- Adding a new optional field does not require a version bump.
- Consumers should tolerate unknown fields.
- Consumers should preserve unknown statuses and future `schemaVersion` values
  so they can fail closed or route to compatibility handling.
