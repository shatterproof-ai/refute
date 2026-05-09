# Project Goals

## Vision

`refute` should become a dependable refactoring substrate for agents and
automation. An agent should be able to ask for a semantic change, inspect the
exact edits, and apply them across a codebase without relying on brittle search
and replace.

The long-term product is two interfaces over the same refactoring core:

- a command-line tool for direct use in scripts, terminals, and agent shells;
- an MCP server exposing the same operations as structured tools for agents.

Both interfaces should produce deterministic results for the same repository
state, operation, inputs, backend versions, and configuration.

## Primary Users

- AI coding agents that need safe, high-leverage code modification tools.
- Developers who want editor-grade refactorings from the terminal.
- CI and migration workflows that need repeatable, reviewable code changes.
- Repository maintainers executing cross-language modernization work.

## Design Principles

1. Use semantic engines before text rewriting.

   Refactorings should come from language servers, compiler-aware libraries, or
   dedicated refactoring engines when possible. Text or structural pattern
   rewriting is useful, but it should be explicit and scoped to cases where
   semantic analysis is unnecessary or unavailable.

2. Normalize the core path, but allow deliberate abstraction leakage.

   Backends may speak LSP, JSON-RPC, command-line protocols, or library-specific
   APIs. Common operations should flow through a shared request/result shape and
   a common `WorkspaceEdit` representation where that fits. This must not become
   a lowest-common-denominator constraint. Some languages and engines expose
   stronger concepts than LSP can model cleanly, such as TypeScript interactive
   refactor arguments, OpenRewrite recipes, Python module moves, or language-
   specific change-signature operations. `refute` should preserve those
   capabilities through typed backend extensions instead of flattening them away.

3. Make preview the default agent posture.

   Agents need to inspect planned edits before applying them. Dry-run diffs and
   structured JSON output are first-class behavior, not debugging features.

4. Prefer all-or-nothing application.

   Partial refactorings are dangerous. The applier should validate and stage
   edits before modifying files, and it should fail loudly when it cannot
   preserve a coherent workspace state.

5. Keep backend behavior explicit.

   Refactoring availability depends on language, operation, project shape, and
   installed tools. The CLI and MCP server should report what backend was used,
   what capabilities are available, and why an operation is unsupported.

6. Build for multi-language expansion.

   Go, TypeScript/JavaScript, Rust, Java/Kotlin, and Python should share the
   same operation contract even when they use different engines internally.

7. Treat position handling as an API contract.

   Line and column encodings must be documented and tested. Ambiguous or
   invalid positions should be rejected instead of guessed.

## Target Operation Families

The project should grow in layers:

- symbol rename across definitions, references, imports, and exported APIs;
- extract function, extract variable, and inline operations;
- move symbol or declaration to another file/module with import updates;
- signature refactorings such as add, remove, reorder, and parameter object
  transformations;
- structural rewrites through an explicit pattern-rewrite interface;
- symbol discovery and capability listing for agent planning.

## Target Language Strategy

`refute` should not require every language to have a custom implementation.
The intended backend ladder is:

1. Use a high-quality language-specific refactoring engine when available.
2. Use LSP rename and code actions for broad language coverage.
3. Use structural tools such as ast-grep for syntax-aware but non-semantic
   transformations.
4. Refuse the operation when none of the above can perform it safely.

This ladder is intentionally not a purity rule. A backend can expose a common
operation and still attach backend-specific fields, constraints, diagnostics, or
follow-up commands when those details are necessary to perform the refactoring
well.

## Non-Goals

- Reimplementing every language parser and typechecker in Go.
- Hiding backend limitations behind best-effort text rewriting.
- Providing an interactive TUI before the non-interactive CLI and MCP contract
  are reliable.
- Replacing formatters, linters, or full project build systems.
