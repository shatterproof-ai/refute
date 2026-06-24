# refute Stories

This directory contains intent stories for the `refute` CLI — prose-first descriptions of user-facing capabilities, written for humans and read by agents.
Stories capture *what* the tool is supposed to do; code and tests are the evidence that it does so.
See the [Storystore spec](https://github.com/ketang/storystore/blob/main/spec.md) for schema and tooling details.

## Story coverage policy

Stories so far cover refactoring operations (rename, list-symbols, inline, extract)
and `refute doctor`. `refute` exposes several other user-facing surfaces, and not
every surface needs an intent story. This policy records which surface categories
**require a story** and which may take a **documented exclusion**, so future
coverage audits have an agreed baseline rather than re-deciding each surface.

Definitions:

- **Story required** — the surface carries user-facing behavior whose *intent*
  is not fully self-evident from its tests, so it should have an active story in
  this directory. Until one is authored, the gap is recorded under
  [Missing coverage](#missing-coverage) below.
- **Documented exclusion** — the surface is intentionally not given a story.
  Either it is a thin pass-through with no `refute`-specific intent (e.g.
  framework-generated behavior) or it is a maintainer-facing process rather than
  a user-facing command. The exclusion holds only while the stated condition
  remains true; if the condition changes, the disposition flips to story required.

### Dispositions

| Surface | Category | Disposition | Notes |
|---------|----------|-------------|-------|
| `refute version` | Version reporting | Documented exclusion | Reports build metadata (`Version`, `Commit`, `BuildDate`) and an optional `--json` shape. No `refute`-specific intent beyond standard version output; covered by `internal/cli/version_test.go`. Flips to story required only if it gains semantics beyond reporting build identity. |
| Shell completion support | Shell integration | Documented exclusion | Provided by the cobra framework's default completion machinery; no custom completion logic exists today. Flips to story required if `refute` adds custom completion semantics (e.g. dynamic symbol/operation completion). |
| Local telemetry / snapshot behavior | Telemetry (`internal/telemetry/`) | Story required | User-facing data-handling behavior — what is recorded locally, retention, and snapshot output (`recorder.go`, `retention.go`, `snapshot.go`) — whose intent should be stated for users, not just tested. Currently missing. |
| `refute-tool` lockfile sync / run / doctor behavior | Toolchain bootstrap (`cmd/refute-tool/`, `internal/toolchain/`) | Story required | The lockfile-bootstrap shim has user-visible sync/run/doctor behavior (`lock.go`, `run.go`, `sync.go`) that affects reproducible toolchain setup. Currently missing. |
| Package-manager shims (`adapters/cargo`, `adapters/npm`, `adapters/python`, JVM packaging under `adapters/jvm`) | Distribution shims | Story required | These redistribute the `refute-tool` bootstrap per ecosystem; their install/run intent is user-facing. Satisfiable as **either** one packaging-shim story covering all ecosystems **or** per-ecosystem stories. Currently missing. |
| Install flows | Installation (`internal/cli/lspsetup.go`, `refute:install-refute` skill) | Story required | How a user installs `refute` and its backends is core onboarding behavior. Currently missing. |
| Release verification flows | Maintainer process (`make verify`, `make verify-report`) | Documented exclusion | A maintainer audit process, not a user-facing command. See [`docs/release-verification.md`](../release-verification.md). Flips to story required if any verification flow becomes a user-facing `refute` subcommand. |

### Evidence rule

Every active story must name concrete evidence — specific test files, fixture
paths, or command-surface entries — or explicitly state in its Evidence section
that coverage is missing. A story that asserts behavior without resolvable
evidence is incomplete and should be flagged by the next coverage audit. The
existing refactoring and doctor stories satisfy this rule (each names tests under
`internal/cli/` and the support matrix); any story authored to fill a gap below
must do the same.

### Missing coverage

The following surfaces are **story required** by this policy but have no story
yet. These are follow-up recommendations for future authoring, recorded here for
traceability — no tracker issues are filed for them as part of this policy.

- **Local telemetry / snapshot behavior** — author a story describing what
  `refute` records locally, the retention policy, and snapshot output. Evidence
  exists to cite: `internal/telemetry/recorder_test.go`,
  `internal/telemetry/internal_test.go`.
- **`refute-tool` lockfile sync/run/doctor** — author a story for the bootstrap
  shim's reproducible-toolchain behavior. Evidence to cite:
  `cmd/refute-tool/sync_test.go`, `cmd/refute-tool/lock_test.go`,
  `cmd/refute-tool/run_test.go`.
- **Package-manager shims** — author either a single packaging-shim story or
  per-ecosystem stories for `adapters/cargo`, `adapters/npm`, `adapters/python`,
  and JVM packaging under `adapters/jvm`. Cite the redistribution evidence in
  each adapter directory.
- **Install flows** — author a story for the install path (`refute` binary plus
  backends). Cite `internal/cli/lspsetup.go` and the `refute:install-refute`
  skill.

When any of these stories is authored, add it to [`INDEX.md`](INDEX.md) and move
the corresponding line out of this list.
