# Release Process

This project ships a CLI release plus registryless package-manager adapters.
The release automation builds static `refute` archives for linux and macOS on
amd64 and arm64, stamps version metadata into `refute version`, writes a
canonical manifest, and checksums every binary and adapter artifact.

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
| `refute-manifest-v0.1.0.json` | Canonical lockfile source for platform artifact URLs and SHA-256 values |
| `refute-tool-npm-0.1.0.tgz` | npm-family shim package |
| `refute_tool-0.1.0-py3-none-any.whl` | pip/uv shim package |
| `cargo-refute-0.1.0.tar.gz` | Cargo helper source package |
| `refute-tool-maven-repository-0.1.0.tar.gz` | File-backed Maven repository bundle for Maven and Gradle |
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

## Consumer Update Channels

Prefer semver tags for dependency-manager driven consumers. Go-module projects
can track `refute` directly in `go.mod` as a tool dependency:

```bash
go get -tool github.com/shatterproof-ai/refute/cmd/refute@v0.1.0
go tool refute version
```

That keeps `refute` visible to the consuming repository's ordinary Go module
update flow. Use `@latest` to move to the newest tagged release, or a commit
hash when a project deliberately needs an unreleased source build.
If no semver tag has been published, Go consumers will get a pseudo-version
instead of a clean release version, so tagged releases are the dependency
manager friendly path.

Use release manifests when a consuming project needs pinned binary artifacts,
checksums, and version metadata stamped by this release workflow.
`refute.lock.json` should point at immutable release URLs, select the platform
artifact by OS and architecture, and install the one active project binary at
`.refute/bin/refute`. npm, Python, Cargo, Go, Maven, and Gradle adapters are
thin shims over that path rather than separate refute owners.

## Nightly Release

The `Nightly Release` workflow builds an unofficial channel from `main` on
every push, every day, and on manual dispatch. It stamps binaries with:

```bash
nightly-<UTC YYYYMMDD>-<short commit>-<workflow run id>
```

The workflow publishes an immutable prerelease under that full nightly tag,
then force-moves the lightweight `nightly` tag and recreates the moving
`nightly` prerelease as a convenience channel. Lockfiles and package-manager
dependencies must use the immutable nightly tag, not the moving `nightly`
release. Automatic push and schedule runs skip publication when the existing
moving nightly release is less than five minutes old; manual runs bypass that
cooldown. Nightly binary versions are intentionally not semver releases; adapter
package versions use semver-compatible prerelease versions such as
`0.0.0-nightly.20260507.abc1234.123456789`.

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
