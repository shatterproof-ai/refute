# Install refute

`refute` can be installed globally, but agent-driven projects should prefer a
project-local install so each repository controls the binary its agents use.

## Project-local nightly install

From the project that will use `refute`:

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

Add this to a consuming project's `AGENTS.md`:

````md
## Refute

Use the project-local `refute` binary for symbol-aware Go refactors:

```bash
.agents/bin/refute
```

Install or update it with:

```bash
bash ~/project/refute/scripts/install-nightly.sh --project .
```

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
````

## Global install

For personal shell use, install into `~/.local/bin`:

```bash
bash scripts/install-nightly.sh --install-dir ~/.local/bin
refute version
```

## Requirements

- GitHub CLI (`gh`) authenticated with access to `shatterproof-ai/refute`.
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
