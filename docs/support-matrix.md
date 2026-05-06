# Support Matrix

This document is the source of truth for what `refute` supports in the
current release. The runtime `refute doctor` command reflects the same data;
if the two ever disagree, the discrepancy is a bug.

## Status definitions

| Status | Meaning |
| --- | --- |
| **supported** | Documented, integration-tested, installable from a tagged release. Refactoring quality is bounded only by the underlying language server. |
| **experimental** | Implemented and tested in development, but missing one or more of: stable adapter packaging, integration coverage in CI, documented install path. May regress between releases. |
| **implemented but not packaged** | Code exists in the repo but cannot be reached from a `go install` build because adapter assets are repo-local. |
| **planned** | Acknowledged target. Not yet wired up or not yet covered by tests. |
| **unsupported** | Not on the roadmap for v0.1. Listed only to make the boundary explicit. |

A feature is **supported** only when all three are true: documentation exists
in this repo, integration coverage exists in CI or the local test suite, and a
user can install the required backend from a documented command. Anything
short of all three is at most **experimental**.

## Language matrix

| Language | Extensions | Backend | Dependency install | Operations | Test coverage | Status | Caveats |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Go | `.go` | `lsp/gopls` | `go install golang.org/x/tools/gopls@latest` | rename, extract-function, extract-variable, inline | unit + integration (`internal/integration_test.go`) | supported | Primary v0.1 dogfood target. |
| Rust | `.rs` | `lsp/rust-analyzer` | `rustup component add rust-analyzer` | rename | unit + integration (`internal/integration_test.go`; CI installs rust-analyzer) | experimental | Experimental while dogfood confidence is still building. |
| TypeScript | `.ts`, `.tsx` | `lsp/typescript-language-server` | `npm install -g typescript-language-server typescript` | rename | unit | experimental | ts-morph adapter is **implemented but not packaged**; tracked in [issue #1](https://github.com/shatterproof-ai/refute/issues/1). |
| JavaScript | `.js`, `.jsx` | `lsp/typescript-language-server` | `npm install -g typescript-language-server typescript` | rename | unit | experimental | Same packaging caveat as TypeScript. |
| Python | `.py` | `lsp/pyright` | `npm install -g pyright` | rename | none | planned | Promote once fixture and integration coverage land. |
| Java | `.java` | OpenRewrite | — | — | none | unsupported | Not claimed for v0.1. JAR packaging deferred. |
| Kotlin | `.kt` | OpenRewrite | — | — | none | unsupported | Not claimed for v0.1. |

## How operations map to backends

The `Operations` column lists the refactorings that route through the
backend in the current release. Operations not listed return the
`unsupported` JSON status when invoked.

For Tier 1 rename (`refute rename --symbol pkg.Func --new-name New`) the
backend is selected from the file's extension once the symbol is resolved,
following the same rules as Tier 2.

## Process for promoting a status

To move a language from **experimental** to **supported**:

1. Add or extend integration tests under `internal/integration_test.go`
   (build tag `integration`) that exercise rename and any other claimed
   operations end-to-end.
2. Confirm CI runs those integration tests with the dependency installed.
3. Update this matrix and `refute doctor`'s caveat string in lockstep.
4. Reference the promotion in the next release's changelog.

To move a language from **implemented but not packaged** to **experimental**:

1. Solve adapter packaging (e.g., separate npm package per issue #1, or
   `//go:embed` of a self-contained bundle).
2. Update `refute doctor` so the install hint reflects the actual
   distribution path.
3. Update this matrix.
