# java-language-support Expedition Plan

## Goal

Add Java (and Kotlin) language support to `refute`, enabling rename refactoring
for Java codebases. Deliver in two phases:

- **Phase 1** — Wire jdtls (Eclipse JDT Language Server) through the existing
  generic LSP adapter so `refute rename` works on Java files immediately.
- **Phase 2** — Build an OpenRewrite adapter (JVM subprocess, JSON-RPC) that
  becomes the primary Java backend, with jdtls as the fallback.

The spec (`docs/specs/2026-04-15-refute-design.md`) places Java/Kotlin support
in v1.5, with OpenRewrite as the primary backend.

## Success Criteria

- `refute rename --symbol "com.example.Foo" --new-name "Bar"` renames a Java
  class across all files in a fixture project.
- `refute rename --symbol "com.example.Foo#greet" --new-name "hello"` renames
  a method.
- End-to-end integration tests pass for both operations.
- `go test ./...` passes on the base branch.
- Backend selector chooses OpenRewrite for `.java`/`.kt`; falls back to jdtls
  if OpenRewrite is unavailable.

## Task Sequence

1. **jdtls-lsp-config** — Configure jdtls as a language server via the existing
   `lsp.Adapter`. Wire `refute rename` to use it for `.java` files. Add config
   entries and a `config.ServerConfig` for jdtls.

2. **java-fixtures** — Add `testdata/fixtures/java/` with a minimal Java
   project (two classes that cross-reference each other). Add integration tests
   for class rename and method rename via jdtls.

3. **openrewrite-jvm-wrapper** — Create `adapters/openrewrite/` as a Maven
   project. Implement a JSON-RPC server in Java that applies OpenRewrite
   `ChangeMethodName` and `ChangeType` recipes. Build to a fat JAR.

4. **openrewrite-go-adapter** — Create `internal/backend/openrewrite/` with a
   Go adapter that manages the fat JAR subprocess and implements
   `backend.RefactoringBackend` (rename only; others return `ErrUnsupported`).

5. **backend-selector** — Register OpenRewrite as the primary backend for
   `.java`/`.kt`; jdtls as the fallback. Wire the selector into the CLI.

## Experiment Register

None planned yet.

## Verification Gates

```bash
go build ./...
go vet ./...
go test ./...
```

All three must pass before a kept task merges into the base branch.
