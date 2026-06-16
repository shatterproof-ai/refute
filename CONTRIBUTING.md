# Contributing to refute

Thanks for your interest in improving `refute`. This guide is the entry point
for human contributors. The full project contract — repo layout, backend
internals, and the conventions agents follow — lives in
[`AGENTS.md`](AGENTS.md); this file points into it rather than duplicating it.

## Build and Test Gates

Run these from the repo root before sending changes; all must be clean:

```bash
go test ./...        # unit tests
go vet ./...         # static checks
gofmt -l .           # formatting — output must be empty
govulncheck ./...    # vulnerability scan
```

Integration tests (`go test -tags integration ./internal/...`) exercise real
language servers and rewrite tools. They skip when a backend is missing, so
install the backend(s) you touch first — see
[Integration Backend Prerequisites](AGENTS.md#integration-backend-prerequisites).

## Issue First

Every change must be backed by a GitHub issue opened before work begins. If you
have a bug or proposal, file an issue and reference it in your branch and
commits. See [Issue-Backed Work](AGENTS.md#issue-backed-work) and
[Issue Traceability](AGENTS.md#issue-traceability).

## Submitting Changes (External Pull Requests)

External contributors submit work as GitHub pull requests against `main`. A
maintainer reviews the PR and lands the commits through the project's normal
flow. The "no PRs" rule you may see in `AGENTS.md` governs maintainer- and
agent-originated work; it does not apply to contributions you send in. See
[External Contributions](AGENTS.md#external-contributions).

Please keep PRs focused on a single issue, include tests for behavioral
changes, and ensure the gates above pass locally before submitting.
