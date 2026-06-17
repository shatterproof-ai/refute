# Roadmap

This roadmap turns the current implementation into the intended CLI and MCP
server for deterministic, multi-language refactoring. It is ordered to reduce
risk: stabilize the core contract first, then expose it to agents, then widen
language and operation coverage.

The roadmap assumes a common baseline model for preview, apply, errors, and
capability reporting, but not a lowest-common-denominator backend API. LSP
support varies by language, and some backends need typed extension fields,
operation-specific commands, or non-LSP engines to preserve their real
refactoring power.

## Phase 0: Stabilize the Existing CLI Core

Goal: make the current single-shot CLI reliable and accurately documented.

Deliverables:

- Publish `docs/support-matrix.md` with language, backend, operation, and test
  status.
- Maintain [`docs/lsp-landscape.md`](lsp-landscape.md) as the shared reference
  for current LSP/refactoring substrates and backend-expansion posture.
- Align `Capabilities()` with implemented adapter behavior.
- Define a stable JSON result schema for dry-run, apply, no-op, ambiguity,
  unsupported operation, and backend failure.
- Fix CLI/LSP position conversion for UTF-16 code units.
- Harden Tier-2 `--file --line --name` resolution to validate identifier
  boundaries and reject multiple same-line matches.
- Improve missing-backend errors with install hints and detected language.
- Add regression tests for no-op, ambiguity, unsupported operation, bad
  position, and non-ASCII source.

Exit criteria:

- `go test ./...` passes.
- Integration tests pass or skip cleanly when optional tools are absent.
- README and support matrix match actual code behavior.
- Each claimed LSP-backed operation names whether it uses standard LSP,
  server-specific code actions, execute-command, or a custom adapter.

## Phase 1: Make Backend Results Agent-Ready

Goal: ensure an agent can ask what will happen, inspect it, and decide whether
to apply.

Deliverables:

- Make `--json --dry-run` the canonical machine-readable preview path.
- Include backend name, language, operation, files changed, warnings, and
  candidate symbols in JSON output.
- Add a `list-symbols` CLI command backed by LSP `workspace/symbol` where
  available. **(Shipped.)**
- Add a `list-backends` or `doctor` command that reports available tools,
  missing dependencies, and supported operations.
- Add golden JSON tests for all agent-facing output shapes.

Exit criteria:

- Agents can use CLI JSON without scraping human-readable stderr.
- Unsupported operations and missing tools are represented as structured
  outcomes.

## Phase 2: Build the Daemon

Goal: avoid cold-start costs and provide one local service abstraction for CLI
and MCP clients.

Deliverables:

- Add `cmd/refuted`.
- Implement a Unix socket JSON-RPC API for operations currently available via
  the CLI.
- Add backend pooling keyed by workspace root and backend type.
- Add lazy backend startup and idle eviction.
- Add daemon commands: `daemon start`, `daemon stop`, `daemon status`.
- Route CLI commands through the daemon when requested or configured.
- Preserve single-shot mode for simple use and CI.

Exit criteria:

- CLI behavior is equivalent in single-shot and daemon modes.
- Long-lived backends shut down cleanly.
- Daemon status reports active workspaces, backend type, uptime, and idle time.

## Phase 3: Add the MCP Server

Goal: expose the refactoring core directly to agents.

Deliverables:

- Implement MCP tools for:
  - `rename`;
  - `extract_function`;
  - `extract_variable`;
  - `inline`;
  - `list_symbols`;
  - `list_backends`;
  - `preview`;
  - `apply`.
- Use the same request and result schemas as CLI JSON where possible.
- Support stdio MCP mode for agent-managed processes.
- Support daemon-hosted MCP mode for backend reuse.
- Add schema and protocol tests.

Exit criteria:

- An MCP client can discover symbols, preview edits, apply edits, and receive
  structured errors without invoking shell commands.

## LSP Strategy Snapshot

Before expanding backend coverage, use
[`docs/lsp-landscape.md`](lsp-landscape.md) as the shared language-by-language
strategy snapshot. This roadmap intentionally links to that source instead of
duplicating its research tables.

## Phase 4: Expand Backend Coverage

Goal: make multi-language support real rather than aspirational.

Deliverables:

- Define a per-language backend strategy before implementing more operations:
  common LSP where it is good enough, server-specific LSP extensions where
  needed, and dedicated adapters where the language ecosystem has better tools.
  Track those choices in [`docs/lsp-landscape.md`](lsp-landscape.md).
- Finish TypeScript/JavaScript support through the TypeScript-specific adapter
  path described in the LSP landscape.
- Make Python a first-class track using the Pyright/Pylance/Basedpyright/rope
  split described in the LSP landscape.
- Build a Python fixture suite before claiming support. Cover module-level
  rename, class/function/method rename, local variable rename, extract method,
  extract variable, inline, import updates, and package/module moves.
- Decide whether OpenRewrite support is in-process, subprocess JSON-RPC, or
  recipe-file driven; commit the Java adapter sources needed to build the JAR.
- Keep Java/Kotlin/C/C++/C#/PHP/Ruby expansion aligned with the posture in the
  LSP landscape, including documented capability and licensing limits before
  support claims.
- Add ast-grep structural rewrite support as an explicit pattern operation, not
  as a hidden fallback for semantic refactorings.
- Add backend-specific capability tests.

Exit criteria:

- At least Go, TypeScript/JavaScript, Rust, Java, and Python have documented
  support levels with passing fixture tests for their claimed operations.

## Phase 5: Add Higher-Level Refactorings

Goal: move beyond primitive rename/extract/inline into the operations agents
need for larger codebase changes.

Deliverables:

- Move declaration or symbol to file/module with import updates where backends
  support it.
- Add signature operations:
  - add parameter;
  - remove parameter;
  - reorder parameters;
  - introduce parameter object;
  - expand parameter object.
- Add operation planning primitives that combine discovery, preview, and apply
  steps safely.

Exit criteria:

- Each advanced operation has a backend-specific support matrix, fixture tests,
  and documented refusal behavior for unsupported languages.

## Phase 6: Real-World Corpus and Release Readiness

Goal: prove robustness on repositories larger and messier than fixtures.

Deliverables:

- Curate pinned real-world projects per language.
- Run nightly or pre-release corpus tests that apply refactorings, then build
  or typecheck the projects.
- Add performance benchmarks for daemon warm start versus single-shot mode.
- Define installation packages and dependency bootstrap guidance.
- Add versioning and release notes.

Exit criteria:

- The project can publish a release with clear supported languages,
  installation instructions, known limitations, and repeatable verification.

## Cross-Cutting Work

These items should be handled continuously rather than deferred to one phase:

- Keep documentation status separate from target design.
- Prefer small fixture regressions for every backend bug.
- Track backend versions in integration test logs.
- Keep human-readable CLI output useful, but treat structured output as the
  compatibility contract.
- Avoid fallback text edits for semantic operations unless the command name
  explicitly says it is a pattern or textual rewrite.
