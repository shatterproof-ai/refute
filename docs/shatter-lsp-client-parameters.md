# Shatter Guidance For LSP Client Parameters

This note is for Refute maintainers and agents using Shatter against Refute's
LSP backend code. It explains what to do when Shatter reports that a target
takes an `lsp.Client` or `*lsp.Client` parameter that it cannot construct.

## Read These Shatter Docs First

Consult these files in the Shatter repository before changing Refute or adding
Shatter configuration:

- `~/project/shatter/docs/resource-parameters.md` for practical user guidance
  on live resource parameters, opaque types, setup files, generators, execution
  adapters, and interface refactors.
- `~/project/shatter/docs/execution-adapters.md` for the architecture boundary:
  resource-owned invocation belongs in a language frontend adapter, not in
  `shatter-core`.
- `~/project/shatter/SPEC.md` for the current implemented command and config
  surface. Architecture notes are not a guarantee that a specific adapter ID is
  implemented.

## The Refute Case

The current direct problem target is:

```text
internal/backend/lsp/priming.go:PrimeWorkspace
```

with this signature:

```go
func PrimeWorkspace(client *Client, root string, languageID string) error
```

The `*Client` value is not normal generated input. It wraps a live LSP server
subprocess, JSON-RPC streams, request state, server capabilities, timeouts, and
shutdown behavior. Shatter cannot synthesize that value from JSON data.

If Shatter reports something like this, treat it as expected:

```text
param "client" -> inner -> lsp.Client
type with no constructor - has no exported constructor or factory function
```

That message means direct parameter generation is the wrong model for this
target. It does not mean agents should add a trivial constructor or try to
serialize a client.

## What To Do

Use the smallest option that preserves the behavior you need to validate.

### 1. Broad Scan Or Audit: Mark It Opaque

If the goal is broad Shatter coverage of Refute and live LSP subprocess behavior
is not in scope, mark the client type or target as intentionally unsupported in
Shatter config. This keeps reports honest and prevents agents from spending time
on impossible direct invocations.

Example shape:

```yaml
opaque_types:
  - name: lsp.Client
    reason: "requires a live LSP subprocess and deterministic cleanup"
```

Use the exact type name that Shatter reports for the active frontend. If it
reports the fully qualified package path, use that form.

### 2. Unit-Level Behavior: Refactor To A Small Interface

If the behavior worth exploring is the file-walking and `DidOpen` selection
logic, prefer a small interface boundary over a live `*Client` parameter.

For example, split or adapt the implementation around the one behavior
`PrimeWorkspace` needs:

```go
type fileOpener interface {
    DidOpen(filePath string, languageID string) error
}

func PrimeWorkspace(client fileOpener, root string, languageID string) error {
    // walks files and calls client.DidOpen(...)
}
```

Then Shatter and ordinary tests can explore:

- hidden-directory and generated-directory skipping
- maximum file count behavior
- extension-to-language-ID filtering
- ignored `DidOpen` failures
- walk errors

The live LSP client remains covered by integration tests around `Adapter` and
`StartClient`, where subprocess lifecycle is explicit.

### 3. End-To-End LSP Behavior: Add Shatter Adapter Support

If the goal is true end-to-end Shatter execution of `PrimeWorkspace` with a live
LSP server, Refute alone should not fake the client. Shatter needs frontend
support that can create and clean up the resource.

The right shape is a Go execution adapter or runtime provider in Shatter that
can:

- create or select a disposable workspace
- start the configured LSP server, such as `gopls` or `rust-analyzer`
- initialize the JSON-RPC/LSP session
- build the `*lsp.Client` argument
- invoke the target
- observe results and side effects in Shatter's normal response shape
- shut down the client and clean up temporary state

Configuration would be selected through Shatter's `execution_profile` concept.
Only use an adapter ID that the active Shatter Go frontend actually implements.
If the adapter does not exist, file Shatter work for the adapter rather than
adding project-local hacks in Refute.

Illustrative shape only:

```yaml
functions:
  "internal/backend/lsp/priming.go:PrimeWorkspace":
    execution_profile:
      adapters:
        - id: go/lsp-client
          apply: required
          options:
            command: gopls
            workspace: "./testdata/fixtures/go"
            language_id: go
```

## What Not To Do

- Do not add an exported constructor solely to satisfy Shatter. A constructor
  that starts subprocesses or owns cleanup is a lifecycle contract, not plain
  input generation.
- Do not try to pass `*Client` through JSON, generator output, or setup-file
  return data. JSON can describe a workspace or command, not an in-memory client
  pointer.
- Do not hide live-client failures by using a nil or stub `*Client` in the real
  target. That explores a different behavior and can produce misleading
  coverage.
- Do not add LSP-specific logic to Shatter core. Resource-owned invocation
  belongs in Shatter's Go frontend or adapter layer.

## Recommended Near-Term Path

For Refute's current codebase, prefer this order:

1. Mark `lsp.Client` opaque or skip `PrimeWorkspace` in broad Shatter scans.
2. If branch coverage for priming logic is important, refactor the function to a
   small `DidOpen` interface and test it with a fake opener.
3. If full live-LSP exploration becomes necessary, open Shatter issues for a Go
   LSP-client execution adapter and keep Refute changes limited to accepting the
   adapter-owned client path.
