# Release Process

This project ships a CLI-only v0.1 release. The release automation builds
static `refute` archives for linux and macOS on amd64 and arm64, stamps version
metadata into `refute version`, and writes SHA-256 checksums.

## Local Artifact Build

From the repository root:

```bash
scripts/release.sh v0.1.0
```

Outputs are written to `dist/`:

| File | Contents |
| --- | --- |
| `refute_v0.1.0_linux_amd64.tar.gz` | linux amd64 binary |
| `refute_v0.1.0_linux_arm64.tar.gz` | linux arm64 binary |
| `refute_v0.1.0_darwin_amd64.tar.gz` | macOS amd64 binary |
| `refute_v0.1.0_darwin_arm64.tar.gz` | macOS arm64 binary |
| `checksums.txt` | SHA-256 checksums for all archives |
| `release-notes.md` | Release notes used by the GitHub release workflow |

The build embeds:

```bash
-X github.com/shatterproof-ai/refute/internal/cli.Version=v0.1.0
-X github.com/shatterproof-ai/refute/internal/cli.Commit=<short commit>
-X github.com/shatterproof-ai/refute/internal/cli.BuildDate=<UTC RFC3339 timestamp>
```

## GitHub Release

Create and push an annotated tag from the commit being released:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The `Release` workflow builds the same archives with `scripts/release.sh`,
uploads them as workflow artifacts, and publishes a GitHub release using the
existing tag. The workflow can also be run manually with an existing `vX.Y.Z`
tag.

## Nightly Release

The `Nightly Release` workflow builds an unofficial channel from `main` every
day and on manual dispatch. It stamps binaries with:

```bash
nightly-<UTC YYYYMMDD>-<short commit>
```

The workflow force-moves the lightweight `nightly` tag to the current `main`
commit, deletes any existing `nightly` GitHub release, and recreates it as a
prerelease with fresh archives and checksums. Nightly builds are intentionally
not semver releases.

Use this channel when another project needs the newest refute binary before a
formal `vX.Y.Z` tag exists. Agents should verify the installed binary with:

```bash
refute version
refute doctor
```

## Support Claims

Release notes must not claim unsupported adapters. For v0.1:

| Language | Release claim |
| --- | --- |
| Go | Supported |
| Rust | Experimental |
| TypeScript / JavaScript | Experimental |
| Java / Kotlin | Not claimed |
| Python | Planned |
