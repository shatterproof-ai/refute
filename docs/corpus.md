# Real-World Refactoring Corpus

The corpus lane exercises refute refactorings against **real upstream
projects** pinned at fixed commits, complementing the small hand-written
fixtures under `testdata/fixtures/`. It is the cross-language regression net for
backend drift: when a language server or rewrite adapter changes behaviour, a
rename that used to keep a real project compiling will start to fail here.

## What it does

For each target in [`testdata/corpus/manifest.json`](../testdata/corpus/manifest.json):

1. **Materialize** — clone the pinned repository at its exact commit into the
   gitignored cache (`.corpus-cache/<name>`). Idempotent across runs.
2. **Refactor** — copy the relevant subtree to a scratch directory and run one
   `refute` rename against it.
3. **Assert propagation** — confirm the new identifier appears in every file the
   rename was expected to touch (cross-file propagation reached each one).
4. **Verify** — run the project's own build / typecheck / test as the ground
   truth that the edit kept the project valid.

The build/test step is the real correctness gate: an incomplete or wrong rename
surfaces as a compile failure with full output, which is exactly the backend
drift the corpus is meant to catch.

## Running it

The lane is guarded by the `corpus` build tag, so it never runs as part of
`go test ./...`. It is network-dependent (it fetches upstream repositories).

```bash
make corpus                                  # fetch all targets + run the lane
scripts/corpus-fetch.sh go-x-example-hello   # materialize a single target
go test -tags corpus ./internal/corpus/ -v   # run after a manual fetch
```

A target whose **backend** (e.g. `gopls`, `rust-analyzer`) is not on `PATH`
**skips** with an explicit reason rather than failing, so the lane is
reproducible on a partial toolchain. Verify steps that need a package registry
(`npm ci`, `mvn compile`) are additionally gated behind
`REFUTE_CORPUS_ALLOW_NETWORK_VERIFY=1`.

## Current targets

| Target | Language | Repo | Refactoring | Verify |
| --- | --- | --- | --- | --- |
| `go-x-example-hello` | Go | golang/example (`hello/`) | rename `reverse.String` | `go build` |
| `rust-itoa` | Rust | dtolnay/itoa | rename the `Buffer` type | `cargo check` |
| `ts-ky` | TypeScript | sindresorhus/ky | rename the `Ky` class | `tsc --noEmit` (network) |
| `python-six` | Python | benjaminp/six | rename `with_metaclass` | `py_compile` |
| `java-json` | Java | stleary/JSON-java | rename `Property.toJSONObject` | `mvn compile` (network) |

Go and Rust run end-to-end wherever `gopls` / `rust-analyzer` are installed. The
TypeScript, Python, and Java targets skip unless their backend and toolchain are
present; their pins are real and reproducible, ready to run once those backends
are installed locally or in CI.

## Adding a target

Append an entry to `testdata/corpus/manifest.json` — no code changes are
required. Fields:

- `name`, `language`, `description` — identity and human context.
- `repo`, `commit`, `subdir` — the pinned source (`subdir` is `"."` for a
  whole-repo project).
- `backendTool` *or* `backendEnv` — the `PATH` binary the backend needs, or an
  environment variable that must point at a built adapter (Java/OpenRewrite uses
  `REFUTE_OPENREWRITE_JAR`).
- `rename` — `command` (`rename`, `rename-type`, `rename-method`, …), `file`,
  `line`, `name`, `newName`.
- `expectRenamed` — files (relative to the project) that must contain `newName`
  after the rename.
- `verify` — ordered commands, each with `needsTool` (skip the step if absent)
  and optional `network: true` (gate behind `REFUTE_CORPUS_ALLOW_NETWORK_VERIFY`).

Pick a rename whose old/new identifiers are distinctive and prefer renames that
do not require renaming the source file (e.g. a Java *method* rather than a
public type, whose file would otherwise have to move to stay compilable).

## Debugging failures

Failures print the target name, repository, pinned commit, the exact command,
and full combined output. A `verify` failure after a successful rename points at
backend drift (the rename under-reached); a `corpus-fetch` failure is an
environment/network issue (the lane skips) or a stale pin (the script names the
missing commit or subdir).
