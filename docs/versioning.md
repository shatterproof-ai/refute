# Versioning and Compatibility

This document defines the compatibility policy for released `refute` artifacts.
It covers the CLI, machine-readable JSON output, and the future MCP server.
The current release line is pre-1.0 and dogfood-oriented, so the policy favors
clear change notices over broad immutability.

## Release Versioning

`refute` release tags use semantic versions in the form `vMAJOR.MINOR.PATCH`.
For `v1.0.0` and later releases:

- `PATCH` releases contain bug fixes, documentation corrections, packaging
  fixes, and compatible schema additions.
- `MINOR` releases may add commands, flags, operations, backend support,
  statuses, optional JSON fields, or MCP tools without removing documented
  behavior from the same major line.
- `MAJOR` releases may remove or rename documented surfaces, change field
  meanings, or make other breaking changes.

Nightly builds are not semantic-version releases. Full nightly tags in the form
`nightly-<UTC date>-<commit>-<workflow run>` are immutable snapshots for early
testing and may include behavior that changes before the next tagged release.
The `nightly` tag and prerelease are moving convenience channels, not
compatibility targets.

## Pre-1.0 Policy

Before `v1.0.0`, every documented surface may still change as the project
learns from dogfood use. That includes CLI flags, command names, text output,
JSON schemas, adapter packaging, and future MCP tool schemas. Breaking changes
may occur in minor releases during this line, but they must not be silent.
Patch releases should avoid breakage except when needed for a security fix,
data-loss fix, or correction to behavior that was never usable as documented.

Pre-1.0 changes still need an explicit migration path when they affect scripts
or agents:

- document the changed surface in `CHANGELOG.md`;
- update the affected contract document in the same change;
- bump `schemaVersion` when a JSON or MCP schema change is incompatible;
- keep compatibility shims for at least one minor release when the maintenance
  cost is low.

Experimental and not-claimed language support can regress between pre-1.0
releases. Supported language paths should not regress silently; if a supported
path changes, the changelog must call it out.

## CLI Compatibility

The stable CLI surface is the set of documented commands, flags, exit codes,
machine-readable outputs, and support claims in `README.md`,
`docs/json-schema.md`, and `docs/support-matrix.md`. `docs/release.md` defines
artifact, package-channel, and release-process policy rather than command
syntax.

Compatible CLI changes include:

- adding a new command, subcommand, or flag;
- adding support for a new language, backend, or operation;
- improving human-readable text while leaving JSON contracts intact;
- adding new non-zero failure cases that are represented by an existing
  documented exit code and JSON status.

Breaking CLI changes include:

- removing or renaming a documented command or flag;
- changing the meaning of a documented flag;
- changing documented exit-code meanings;
- moving a supported language or operation to experimental or not claimed
  without a release note and migration reason.

Human-readable stdout and stderr are not a stable parser contract. Scripts and
agents should use `--json` wherever a command supports it.

## JSON Schema Compatibility

`docs/json-schema.md` is the normative reference for exact JSON fields,
statuses, exit codes, and `schemaVersion: "1"` compatibility rules. This
document defines the release policy around those rules.

`schemaVersion` is the compatibility boundary for JSON output. Consumers should
branch on `schemaVersion` first, then on `status`. Compatible changes include
additive optional fields and new status or error-code values. Incompatible
changes require a new schema version and changelog entry.

Consumers must tolerate unknown fields and unknown status strings. Consumers
that see a future `schemaVersion` should fail closed or route to explicit
compatibility handling instead of assuming version `1` semantics.

## MCP Compatibility

The MCP server is not part of `v0.1` and has no stable shipped schema yet. When
MCP lands, each tool request and response schema must declare a schema version
or reference a versioned JSON schema shared with the CLI.

MCP compatibility will follow the same rules as CLI JSON:

- adding a tool, optional request field, optional response field, or new error
  code is compatible;
- removing or renaming a tool, changing a field meaning, changing a field type,
  or tightening required fields is incompatible;
- incompatible request or response changes require a schema-version bump and a
  changelog entry.

Where practical, MCP responses should reuse the CLI JSON result envelope so
agents have one compatibility model for preview, apply, errors, backend
metadata, and warnings.

## Schema Change Process

Any change to CLI JSON or MCP schemas must include:

1. A contract-doc update describing the new or changed fields.
2. A changelog entry under the release being prepared.
3. Tests or fixtures for the affected output shape when automated coverage is
   practical.
4. A migration note when the change is incompatible.

Schema-version bumps should preserve old-version handling when practical. If
old-version handling is removed before `v1.0.0`, the changelog must say so
explicitly.
