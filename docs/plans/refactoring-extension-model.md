# Higher-Level Refactoring Extension Model

> **Status:** draft — design for issue #99. Defines the request/result model,
> backend-parameter mechanism, refusal contract, and capability/MCP surfacing
> that P5-02 (move-to-file, #100) and P5-03 (signature refactoring, #101) build
> on. No advanced operation is implemented by this document; it is the design
> they execute against.
> **Landing:** pending — commit with the first operation that adopts the model
> (#100), or land standalone if the model is approved ahead of implementation.
> **Disposition:** retained historical artifact once #100/#101 ship; this is
> durable design rationale, not temporary execution notes.

**Issue:** #99 (Phase 5, P5-01). **Blocks:** #100 (move-to-file), #101
(signature refactoring). **Structurally depends on:** #80 (WorkspaceEdit
file-op support — landed; `edit.FileOperation` and `edit.FileOpKind` exist).

---

## Goal

Support richer refactorings — move-to-file, change-signature, and operations we
have not named yet — **without collapsing backend-specific power into a
lowest-common-denominator API**. A backend that can do more than the common
contract (gopls move semantics, a future OpenRewrite recipe parameter) must be
able to expose that, and a backend that cannot do an operation safely must
refuse it explicitly rather than silently produce a partial or wrong edit.

This document defines four things the issue calls for:

1. Request/result structures for move and signature refactorings (§2, §3).
2. Backend-specific parameter blocks hung off the common structure (§4).
3. Refusal behavior when a backend cannot support an operation safely (§5).
4. How advanced operations appear in capabilities and MCP schemas (§6).

It also states **where the new operation types sit in the drift-control
source-of-truth model** (§7), because adding an operation touches every
normative surface the [Drift-Control Policy](../drift-control.md) governs.

---

## 1. Constraints from the current design

The model must fit the surfaces that already exist; it does not get to rewrite
them.

- **The `RefactoringBackend` interface is typed and positional.** Today
  (`internal/backend/backend.go`) each operation is its own method:
  `Rename(loc, newName)`, `ExtractFunction(r, name)`, `MoveToFile(loc, dest)`,
  etc. `MoveToFile` is already on the interface and returns
  `backend.ErrUnsupported` from the LSP adapter
  (`internal/backend/lsp/adapter.go`). The model keeps these typed methods for
  common operations and adds **one structured entry point** for parameterized
  advanced operations rather than growing an unbounded method set.
- **`edit.WorkspaceEdit` already carries file operations.** `FileEdits` plus
  `FileOps` ([]FileOperation) with create/rename/delete, applied in a fixed
  atomic order (creates → text edits → renames → deletes). Move-to-file and any
  operation that adds/removes files **already has a result representation** — no
  new result container is needed for the edit payload.
- **Capabilities are derived, not declared ad hoc.** `Capabilities()` returns
  `[]backend.Capability{Operation: string}` derived from the language profile's
  `operations` list (`internal/backend/lsp/profile.go`), and the selector's
  `backendSupports` (`internal/backend/selector/selector.go`) reads exactly that
  list to decide routing. A new operation that is not in a backend's capability
  list is already refused up front with `selector.ErrOperationUnsupported`.
- **The JSON envelope is the contract with agents.** `edit.JSONResult`
  (`internal/edit/json.go`) already emits `fileOps`, `newSymbol`, `warnings`,
  and a typed `error{code,message,hint}`. Advanced operations reuse this
  envelope; `schemaVersion` stays `"1"` as long as additions are optional
  fields (see `docs/versioning.md`).
- **Unsupported is a first-class status.** `StatusUnsupported` +
  `ErrOperationUnsupported` (operation not offered by any backend for the
  language) and `ErrLanguageUnsupported` (language gated out entirely) already
  exist and are tested by the drift guardrails. Refusal of an advanced
  operation is a specialization of this existing path, not a new mechanism.

**Design principle:** extend at the seams that already exist (capability list,
WorkspaceEdit, JSONResult, unsupported status) and add the **minimum** new
surface — a parameterized request envelope and a richer capability descriptor —
needed to carry operation-specific parameters and per-backend extensions.

---

## 2. The operation taxonomy

Operations fall into two tiers, distinguished by whether the operation needs
**parameters beyond a target location**.

| Tier | Examples | Shape | Entry point |
| --- | --- | --- | --- |
| **Common / fixed-arity** | rename, extract-function, extract-variable, inline, move-to-file | Target (location or range) + at most one scalar (new name, destination path) | Typed interface method (existing pattern) |
| **Parameterized / extensible** | change-signature, and future recipe-style ops | Target + a structured, operation-specific parameter object, optionally with backend-specific extensions | Structured `Refactor(RefactorRequest)` entry point (new, §3) |

Move-to-file is deliberately classified **common**: its only parameter is a
destination path, so it keeps its typed `MoveToFile(loc, destination)` method.
What #100 needs from this design is not a new request shape but the **refusal
contract** (§5) and the **capability descriptor** (§6) — move can fail safety
checks (moving a method, moving a symbol whose unexported dependencies cannot
follow it) and must refuse rather than emit a broken edit.

Change-signature is the first **parameterized** operation and is the reason the
structured entry point exists.

---

## 3. Request and result structures

### 3.1 Common request envelope

A single envelope carries any parameterized operation. It lives in
`internal/backend` (proposed: `internal/backend/request.go`).

```go
// RefactorRequest is the structured entry point for parameterized refactorings
// (change-signature and future recipe-style operations). Fixed-arity operations
// (rename, extract, inline, move-to-file) keep their typed methods; this
// envelope exists so an operation can carry an operation-specific parameter
// object plus optional backend-specific extensions without growing a new
// interface method per operation.
type RefactorRequest struct {
	// Operation is the capability string (e.g. "change-signature"). It must
	// match a backend.Capability.Operation the backend advertises.
	Operation string

	// Target identifies what the operation acts on. Exactly one of Location or
	// Range is set, per the operation's TargetKind (see Capability, §6).
	Target Target

	// Params is the operation-specific, backend-agnostic parameter object,
	// encoded as canonical JSON. Each operation defines its own struct (e.g.
	// SignatureParams, §3.3); the envelope stays operation-agnostic.
	Params json.RawMessage

	// BackendParams is an opaque, backend-specific extension block (§4). A
	// backend that does not recognize a key MUST ignore it; a backend that
	// needs a key it does not find applies its documented default. It is never
	// required for the common path to succeed.
	BackendParams json.RawMessage
}

// Target is a discriminated reference to the code an operation acts on.
type Target struct {
	Location *symbol.Location    // for symbol-addressed operations
	Range    *symbol.SourceRange // for range-addressed operations
}
```

`Params` and `BackendParams` are `json.RawMessage` rather than `map[string]any`
so that (a) the CLI, the future MCP server, and the backend all decode against
the **same** typed struct, and (b) an unknown/garbage parameter object is
rejected at decode time with a precise error rather than surfacing as a nil-map
panic deep in a backend.

### 3.2 Result

Parameterized operations return the **existing** result types. There is no new
result container:

```go
func (b SomeBackend) Refactor(req RefactorRequest) (*edit.WorkspaceEdit, error)
```

- The edit payload (text edits + file ops) is `*edit.WorkspaceEdit` — already
  sufficient for move (file ops) and signature change (multi-file text edits at
  every call site).
- The outcome status, warnings, and refusal reason flow through the existing
  `edit.JSONResult` envelope (§5, §6).
- A newly created symbol/file is reported via the existing
  `JSONResult.NewSymbol` / `fileOps` fields.

This is the load-bearing reuse decision: **#80 already gave us the result
vocabulary**, so the extension model is request-side and capability-side only.

### 3.3 Operation-specific parameter objects

Each parameterized operation defines one Go struct, decoded from
`RefactorRequest.Params`. These are **backend-agnostic** — they describe *what*
the user wants, not *how* a backend does it.

**Move-to-file** stays fixed-arity (typed `MoveToFile(loc, destination)`), so it
needs no Params struct. Its design work in #100 is the safety/refusal contract
(§5.2), not a parameter object.

**Change-signature** (`SignatureParams`, for #101). A signature edit is a list
of ordered parameter operations plus optional return-type changes:

```go
type SignatureParams struct {
	// Parameters is the desired final parameter list, expressed as edits keyed
	// to the original positions. The backend computes the call-site rewrites.
	Parameters []ParamEdit `json:"parameters"`
	// ReturnType, when set, requests a return-type change (backends that cannot
	// safely rewrite return sites refuse — see §5).
	ReturnType *string `json:"returnType,omitempty"`
}

type ParamEdit struct {
	// Op is "keep", "add", "remove", or "reorder".
	Op string `json:"op"`
	// FromIndex is the parameter's original 0-based position ("keep",
	// "remove", "reorder"); -1 for "add".
	FromIndex int `json:"fromIndex"`
	// ToIndex is the parameter's final 0-based position ("keep", "add",
	// "reorder"); -1 for "remove".
	ToIndex int `json:"toIndex"`
	// Name / Type / Default describe an added parameter (and may rename/retype
	// a kept one where the backend supports it).
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Default string `json:"default,omitempty"`
}
```

The parameter object is **declarative and total** — it names the final desired
shape so a backend can diff it against the current signature rather than
replaying imperative steps. This keeps the request reproducible and lets a
backend reject a request it cannot satisfy *before* editing anything (e.g. an
`add` with no `Default` for a language that has no default arguments).

---

## 4. Backend-specific parameter blocks

The acceptance criterion is "supports common cases **and** backend-specific
extensions." `BackendParams` is that extension seam.

**Contract:**

- `BackendParams` is opaque to the common code path. The CLI and MCP layers pass
  it through verbatim; only the resolved backend decodes it.
- A backend decodes `BackendParams` against its own typed struct, namespaced by
  backend (e.g. `lsp/gopls` reads a `GoplsSignatureParams`, a future OpenRewrite
  backend reads recipe options).
- **Forward/backward compatibility rule:** a backend MUST ignore keys it does
  not recognize, and MUST apply a documented default for a recognized key that
  is absent. A request that succeeds on the common path MUST NOT be *failed*
  solely because a `BackendParams` key was unknown — unknown extension keys are
  ignored, not errors. (A backend MAY warn; see `JSONResult.Warnings`.)
- **No silent power loss:** when a backend *requires* a parameter to act safely
  and it is neither in `Params` nor derivable, the backend refuses (§5), it does
  not guess.

**Why a separate block rather than merging into `Params`:** `Params` is the
portable contract that the MCP schema publishes and that every backend for the
language must honor identically. `BackendParams` is explicitly non-portable: two
backends for the same language (ts-morph vs typescript-language-server) may
accept different extension keys. Keeping them separate means the portable schema
stays stable while backends evolve their own options, and a request written
against the common schema runs on any backend for the language.

**Discovery:** the set of `BackendParams` keys a backend honors is advertised in
its capability descriptor (§6), so an agent does not have to guess. The common
`Params` schema is mandatory and published; the `BackendParams` schema is
optional and per-backend.

---

## 5. Refusal behavior

**The governing rule from issue #99: no advanced operation is implemented before
its safety/refusal behavior is specified.** Refusal is not an afterthought —
for move and signature it is the *majority* of the design, because the edit
itself is the easy part and the unsafe cases are where text-substitution tools
get refactoring wrong.

### 5.1 The two refusal layers

| Layer | Trigger | Mechanism | JSON status / code | Exit |
| --- | --- | --- | --- | --- |
| **Capability refusal** (up front, no work) | The backend does not advertise the operation for this language | `selector.ErrOperationUnsupported` from `SelectForOperation`; never reaches backend setup | `unsupported` / `unsupported-operation` | 1 |
| **Language gate** (up front) | The language is `LevelUnsupported` in `SupportMatrix` (Java/Kotlin) | `selector.ErrLanguageUnsupported`, gated in `backendLadder` before any backend constructor | `unsupported` / `unsupported-language` | 1 |
| **Safety refusal** (after analysis, before edit) | The operation *is* supported in general, but *this* invocation cannot be performed safely | `backend.ErrUnsafeRefactor` (new, §5.3), returned by the backend | `unsupported` / `unsafe-refactor` | 1 |

Capability refusal and the language gate already exist and are guarded by
`internal/backend/selector/matrix_routing_test.go`. The model adds only the
**safety refusal** layer.

### 5.2 What counts as unsafe — per operation

A backend MUST refuse (not emit a partial edit) when it cannot guarantee the
edit preserves behavior. The named unsafe cases each operation must specify
**before** it is implemented:

**Move-to-file (#100) refuses when:**

- The destination cannot be represented as a file op the applier supports (e.g.
  moving across a package boundary that would require an import the backend
  cannot compute).
- The moved symbol has unexported dependencies that cannot follow it and cannot
  be promoted, so the move would not compile.
- The symbol is not movable as a unit (e.g. a method bound to a receiver, where
  "move" is ambiguous) — refuse with a message naming the constraint.
- The destination file exists and the move would collide with an existing
  symbol of the same name.

**Change-signature (#101) refuses when:**

- The backend cannot find and rewrite **all** call sites with confidence
  (dynamic dispatch, reflection, calls through interfaces the analyzer cannot
  resolve). Partial call-site rewriting is the canonical unsafe outcome and MUST
  be refused, not emitted.
- An `add` parameter has no default and the language requires every call site to
  be updated but some call sites are outside the analyzed workspace.
- A type change cannot be propagated to all uses.

Each operation's implementing issue must enumerate its full unsafe set in its
own plan; this document fixes the *contract* (refuse via `ErrUnsafeRefactor`
with a specific reason) and the *principle* (no partial edits).

### 5.3 The refusal error type

```go
// ErrUnsafeRefactor is returned when a backend supports an operation in general
// but cannot perform THIS invocation without risking incorrect output. It
// carries a machine code and a human reason so the CLI/MCP layers can surface a
// specific, actionable refusal rather than a generic "unsupported".
type ErrUnsafeRefactor struct {
	Operation string
	Reason    string // human-readable, names the specific constraint
	Code      string // stable, e.g. "incomplete-call-sites", "unmovable-symbol"
}

func (e *ErrUnsafeRefactor) Error() string { return e.Reason }
```

The CLI routes `ErrUnsafeRefactor` to `StatusUnsupported` with error code
`unsafe-refactor` and `JSONError.Hint` carrying `Reason`. This reuses the
existing `emitJSONOperationError` path; the only new vocabulary is the
`unsafe-refactor` code, which is an **additive** JSON change (no
`schemaVersion` bump).

### 5.4 Refusal is preview-safe

Because refusal happens **before** any edit is applied, it composes with the
existing preview-then-apply discipline: a refused operation in `--dry-run`
prints the refusal and changes nothing, identically to apply mode. There is no
state to roll back, and `WorkspaceEdit`'s atomic-apply guarantee is never
engaged for a refused operation.

---

## 6. Capabilities and MCP schemas

### 6.1 Richer capability descriptor

`backend.Capability` today is `{Operation string}` — enough to answer "can this
backend rename?" but not "what parameters does change-signature take, and which
are required?" The model extends it (additively):

```go
type Capability struct {
	Operation string

	// Stability mirrors the support-matrix status for this operation on this
	// backend: "supported" | "experimental" | "planned". Derived from the
	// profile + SupportMatrix, not declared independently.
	Stability string `json:"stability,omitempty"`

	// TargetKind is "location" or "range" — which Target field a request sets.
	TargetKind string `json:"targetKind,omitempty"`

	// ParamsSchema is the JSON Schema for the operation-agnostic Params object
	// (empty for fixed-arity operations like rename/move that take no Params).
	ParamsSchema json.RawMessage `json:"paramsSchema,omitempty"`

	// BackendParamsSchema is the JSON Schema for this backend's optional
	// extension block (§4); empty when the backend honors no extensions.
	BackendParamsSchema json.RawMessage `json:"backendParamsSchema,omitempty"`
}
```

All new fields are `omitempty`, so an existing backend that returns only
`{Operation}` is unchanged and the `Capabilities()` contract stays
backward-compatible. `Stability` lets `refute doctor --json` show that, e.g.,
move-to-file is `experimental` on Go while rename is `supported`, without a
second source of truth — it is read from the same profile/`SupportMatrix` pair
the operation list comes from.

### 6.2 Surfacing path (CLI / doctor / MCP)

The capability descriptor flows to three consumers from the **single** backend
`Capabilities()` source:

1. **`refute doctor --json`** gains, per backend, the operation list it already
   reports, now annotated with `stability` and (for parameterized ops) a
   reference to the params schema. This stays derived from
   `internal/config.SupportMatrix` + the profile, preserving the drift-control
   invariant that doctor cannot disagree with the matrix.
2. **CLI commands** for advanced operations (`refute move`, `refute
   change-signature`) validate their flags against `ParamsSchema` and emit the
   same `JSONResult` envelope as the existing operations. The CLI command
   inventory remains the normative source for "what subcommands exist" per
   drift-control; adding a command updates `docs/current-state.md` "CLI Surface"
   and `README.md` in the same branch.
3. **MCP server** (roadmap Phase 3 — `docs/roadmap.md`; not yet implemented).
   When it lands, each operation becomes an MCP tool whose JSON-Schema `inputSchema`
   is assembled from the capability descriptor: the common fields (target) plus
   `ParamsSchema`, with `BackendParamsSchema` offered as an optional object
   property. **The MCP tool schema is generated from `Capability`, not authored
   by hand**, so the agent-facing schema cannot drift from what the backend
   actually accepts. This is the concrete reason the capability descriptor
   carries machine-readable schemas rather than prose.

### 6.3 Why schemas live on the capability, not in docs

An MCP agent must be able to *discover* an operation's parameters at runtime, and
the published schema must match the backend that will actually run. Putting
`ParamsSchema`/`BackendParamsSchema` on the capability that the backend returns
makes the backend the single source for both routing (does it support the op?)
and shape (what params?). The MCP layer and the support-matrix doc both derive
from it; neither is allowed to assert a parameter the backend does not accept.

---

## 7. Where this sits in the drift-control model

Adding an operation type is a drift-sensitive change touching nearly every
normative surface in the [Drift-Control Policy](../drift-control.md). The model
maps each new operation onto the existing sources of truth — it introduces **no
new source of truth**:

| Claim domain | Normative source | What a new operation must update (same branch) |
| --- | --- | --- |
| Backend support / operation set | `internal/config.SupportMatrix` (`internal/config/support.go`), via `refute doctor --json` | Add the operation to the language's `Operations`; reconcile `docs/support-matrix.md` |
| Backend routing | `profile.operations` + `selector.backendSupports` | Add the operation string to the profile so `SelectForOperation` routes (or refuses) it; the `matrix_routing_test` guardrail enforces agreement |
| CLI command inventory | Registered cobra commands in `internal/cli` | Add the subcommand; `command_doc_drift_test` requires it in `README.md` "Operations" and `docs/current-state.md` "CLI Surface" |
| JSON output / codes | `internal/edit/json.go` + `docs/json-schema.md` | Register the `unsafe-refactor` error code and any new optional fields; additive only → no `schemaVersion` bump |
| Intent stories | `docs/stories/` | Add a story for each new user-facing operation (move-to-file, change-signature) before promoting it past experimental |
| Roadmap / current-state | `docs/current-state.md`, `docs/roadmap.md` | Move the operation from planned → implemented when it lands; refresh the "as of" date |

**The operation set's source of truth is unchanged:** it remains
`config.SupportMatrix` as surfaced by `refute doctor --json`, mirrored in prose
by `docs/support-matrix.md`. `Capability.Stability` is *derived* from that pair,
not a competing declaration. This is what keeps the richer capability descriptor
from becoming a second, driftable source of operation status.

**Refusal statuses reuse existing vocabulary:** `unsupported` status with codes
`unsupported-operation`, `unsupported-language` (both existing), and
`unsafe-refactor` (new, additive). No new status value is introduced.

---

## 8. Readiness for #100 and #101

This design is concrete enough to start both successor issues:

**#100 (move-to-file) can start with:**

- The result representation is settled: `edit.WorkspaceEdit.FileOps`
  (create/rename/delete) from #80. No new request shape — `MoveToFile(loc,
  destination)` stays typed.
- The work is: (a) implement the gopls-backed move, (b) implement the §5.2 move
  refusal set via `ErrUnsafeRefactor`, (c) add `move` to the Go profile +
  `SupportMatrix` + CLI + a story, (d) advertise the capability with
  `Stability:"experimental"`.

**#101 (change-signature) can start with:**

- The request shape is settled: `RefactorRequest{Operation:"change-signature",
  Target, Params: SignatureParams}` plus optional `BackendParams`, decoded via
  the new structured `Refactor` entry point (§3.1, §3.3).
- The refusal contract is settled: refuse on incomplete call-site coverage and
  the rest of the §5.2 set, via `ErrUnsafeRefactor`.
- The capability descriptor and MCP-schema derivation are settled (§6); the CLI
  validates `--param` edits against `ParamsSchema`.

**Sequencing note:** implement the structured `Refactor` entry point and the
`ErrUnsafeRefactor` type as part of #100 or as a small precursor, since
move-to-file needs the refusal type even though it does not need the parameter
envelope. #101 then adds the first `Params`-bearing operation on top of an
already-proven refusal path.

---

## 9. Open questions (resolve before implementing the operation they gate)

- **Does the structured `Refactor` entry point go on the `RefactoringBackend`
  interface, or a separate optional `ParameterizedBackend` interface a backend
  may also implement?** Recommended: a separate optional interface, type-asserted
  by the CLI, so a backend that supports only fixed-arity operations needs no
  no-op `Refactor` method. Decide when #101 starts.
- **How are `BackendParams` extension keys versioned per backend?** Initial
  answer: documented per backend, ignored-if-unknown (§4); revisit if two
  backends for one language diverge enough to need negotiated capability flags.
- **Change-signature call-site confidence threshold:** what analysis does each
  backend run to decide "all call sites found"? This is per-backend and belongs
  in #101's plan, but the *refuse-on-doubt* default is fixed here.
