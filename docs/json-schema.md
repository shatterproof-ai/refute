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
| `filesModified` | number | yes | Count of file entries in `edits`; `0` when no edits are present. |
| `edits` | array | no | File edits grouped by path. Omitted when empty. |
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

## Error Object

When present, `error` has this shape:

| Field | Type | Required | Meaning |
| --- | --- | --- | --- |
| `code` | string | yes | Stable, machine-readable error category for the failure. |
| `message` | string | yes | Human-readable detail. |
| `hint` | string | no | Suggested remediation, usually an install command or `refute doctor` prompt. |

Scripts should use `status` for high-level routing and `error.code` for a more
specific branch inside an error status.

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
