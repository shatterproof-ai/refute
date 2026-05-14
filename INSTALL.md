# Install refute

`refute` can be installed globally, but consuming projects should prefer an
install path that their normal dependency update workflow can see. Use the Go
tool dependency path for Go modules, release archives for pinned binary
installs, and nightlies only when a project needs an unreleased build from
`main`.

## Go module tool dependency

For consuming projects that have a `go.mod` and Go 1.24 or newer, track
`refute` as a Go tool dependency:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@latest
go tool refute version
go tool refute doctor
```

This records `refute` in the consuming project's `go.mod`, so dependency
automation can review, pin, and update it like other Go module dependencies.
If no semver tag has been published yet, Go records a pseudo-version for the
selected commit. Prefer semver tags for normal consumer updates, and use a
specific tag or commit when the project needs reproducible changes:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@vX.Y.Z
go get -tool github.com/shatterproof-ai/refute/cmd/refute@<commit>
```

Update to the latest available module version with:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@latest
go mod tidy
```

This path builds from source using the consuming project's Go toolchain. Use a
release archive instead when the project needs the exact binary metadata
stamped by `scripts/release.sh`.

## Project-local release archive install

For non-Go projects, or projects that want the exact released binary rather
than a source build, install a semver release archive into the project-local
tool directory. Download the archive through the project's normal automation,
then install it with:

```bash
bash /path/to/refute/scripts/install-nightly.sh \
  --project . \
  --archive /path/to/refute_v0.1.0_linux_amd64.tar.gz
```

This installs to:

```bash
.agents/bin/refute
```

Package-manager wrappers for npm, Homebrew, or asdf/mise should use the same
release archives and checksums so those ecosystems can pin `refute` in their
native lockfiles or manifests without inventing a separate build path.

## Project-local nightly install

Use the nightly installer only when a consuming project needs the newest
unreleased binary from `main`. From the project that will use `refute`:

```bash
bash /path/to/refute/scripts/install-nightly.sh --project .
```

This installs the latest unofficial nightly to:

```bash
.agents/bin/refute
```

Check it with:

```bash
.agents/bin/refute version
.agents/bin/refute doctor
```

For Kapow on this machine:

```bash
cd ~/project/kapow
bash ~/project/refute/scripts/install-nightly.sh --project .
.agents/bin/refute version
```

## Agent instructions

For Go-module consumers, add this to the consuming project's `AGENTS.md`:

````md
## Refute

Use the repo-managed `refute` tool for symbol-aware refactors:

```bash
go tool refute
```

Install or update it with:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@latest
go mod tidy
```

TRIGGER WHEN:

- You are renaming a Go function, method, type, field, variable, or parameter.
- The symbol appears in more than one file or package.
- A textual search finds both real references and unrelated strings/comments.
- You need a machine-readable preview before editing files.
- The user asks for a rename, inline, extract-function, or extract-variable
  refactor and the target language is supported by `go tool refute doctor`.

SKIP:

- The edit is plain text, docs, comments, config, JSON, YAML, SQL, or generated
  data.
- The requested change intentionally renames only a string literal, CLI flag,
  environment variable, database column, GraphQL field, or API route.
- `go tool refute doctor` reports the required backend as missing and
  installing it is outside the task scope.
- The refactor requires behavior not listed in `go tool refute <command>
  --help`.
- The working tree already contains unrelated user edits that the preview would
  touch.

Before refactoring:

```bash
go tool refute doctor
```

Always preview first:

```bash
go tool refute rename --dry-run --json \
  --file <path.go> \
  --line <line> \
  --name <oldName> \
  --new-name <newName>
```

If the preview is correct, apply with the same command without `--dry-run`,
then run the project's required verification gate.

If the preview is empty, touches unexpected files, or reports an error, stop
and use normal code-editing workflow instead of forcing the refactor.
````

For consumers that use a project-local binary instead, add this to
`AGENTS.md`:

````md
## Refute

Use the project-local `refute` binary for symbol-aware refactors:

```bash
.agents/bin/refute
```

Install or update it with:

```bash
bash ~/project/refute/scripts/install-nightly.sh --project .
```

TRIGGER WHEN:

- You are renaming a Go function, method, type, field, variable, or parameter.
- The symbol appears in more than one file or package.
- A textual search finds both real references and unrelated strings/comments.
- You need a machine-readable preview before editing files.
- The user asks for a rename, inline, extract-function, or extract-variable
  refactor and the target language is supported by `refute doctor`.

SKIP:

- The edit is plain text, docs, comments, config, JSON, YAML, SQL, or generated
  data.
- The requested change intentionally renames only a string literal, CLI flag,
  environment variable, database column, GraphQL field, or API route.
- `refute doctor` reports the required backend as missing and installing it is
  outside the task scope.
- The refactor requires behavior not listed in `refute <command> --help`.
- The working tree already contains unrelated user edits that the preview would
  touch.

Before refactoring:

```bash
.agents/bin/refute doctor
```

Always preview first:

```bash
.agents/bin/refute rename --dry-run --json \
  --file <path.go> \
  --line <line> \
  --name <oldName> \
  --new-name <newName>
```

If the preview is correct, apply with the same command without `--dry-run`,
then run the project's required verification gate.

If the preview is empty, touches unexpected files, or reports an error, stop
and use normal code-editing workflow instead of forcing the refactor.

Refute keeps local observability data outside the project tree:

- invocation JSONL: `~/.local/share/refute/telemetry.jsonl`
- compressed before/after snapshots: `~/.local/share/refute/snapshots/`
- agent-session transcripts: `~/.local/share/refute/sessions/`

Use `--verbose` when you want the agent transcript summary echoed into the
current session. Set `REFUTE_TELEMETRY=0` to disable all telemetry, or
`REFUTE_TELEMETRY_SNAPSHOTS=0` to skip snapshots only.
````

## Global install

For personal shell use from source:

```bash
go install github.com/shatterproof-ai/refute/cmd/refute@latest
refute version
```

To install a downloaded release or nightly archive into `~/.local/bin`:

```bash
bash scripts/install-nightly.sh --install-dir ~/.local/bin --archive /path/to/refute_v0.1.0_linux_amd64.tar.gz
refute version
```

## Requirements

- Go tool dependency and `go install` workflows require a Go version compatible
  with this repository's `go.mod`.
- GitHub nightly installs require GitHub CLI (`gh`) authenticated with access
  to `shatterproof-ai/refute`.
- Go language refactors require `gopls`:

```bash
go install golang.org/x/tools/gopls@latest
```

To test the installer from an already downloaded archive:

```bash
bash scripts/install-nightly.sh --project /path/to/project --archive /path/to/refute_nightly_linux_amd64.tar.gz
```

## Notes

- Nightly builds are unofficial and stamped as
  `nightly-<UTC YYYYMMDD>-<short commit>`.
- The nightly release updates automatically from `main`.
- The binary is intentionally not committed into consuming projects.
