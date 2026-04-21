# java-language-support Expedition Handoff

- Expedition: `java-language-support`
- Base branch: `java-language-support`
- Base worktree: `/home/ketan/project/refute-java`
- Status: **ALL TASKS COMPLETE — ready to land**
- Active task branch: `none`
- Last completed: `java-language-support-05-backend-selector (kept)`
- Primary branch: `main`

## What was done

All 5 planned tasks are complete and merged into the `java-language-support` base branch:

1. **jdtls-lsp-config** — Added `jdtls` and `kotlin-language-server` to `builtinServers` in `internal/config/config.go`. Added Maven/Gradle project markers to `findWorkspaceRoot` in `internal/cli/rename.go`.

2. **java-fixtures** — Created `testdata/fixtures/java/rename/` (Maven project with `Greeter.java` and `Main.java`). Added `TestEndToEnd_RenameJavaMethod` (integration build tag, skips if jdtls absent).

3. **openrewrite-jvm-wrapper** — Created `adapters/openrewrite/` as a Maven project. `Main.java` is a newline-delimited JSON-RPC server. `RenameHandler.java` applies `ChangeMethodName` or `ChangeType` OpenRewrite recipes. Build: `mvn package -f adapters/openrewrite/pom.xml -q` → `adapters/openrewrite/target/openrewrite-adapter.jar`.

4. **openrewrite-go-adapter** — Created `internal/backend/openrewrite/adapter.go` implementing `backend.RefactoringBackend`. Manages the JVM subprocess, parses package/class names from Java source to build OpenRewrite method patterns. Returns `ErrUnsupported` for non-rename operations.

5. **backend-selector** — Modified `internal/cli/rename.go` to call `selectBackend()`: uses OpenRewrite for `.java`/`.kt` (if JAR present + `java` on PATH), falls back to jdtls/kotlin-language-server LSP otherwise.

## Current state

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅ (31 tests pass)
- Integration tests: `TestEndToEnd_RenameJavaMethod` skips without jdtls but will run when jdtls is installed

## To land

The base branch is already rebased onto main. When ready:

```bash
cd /home/ketan/project/refute-java
# Optionally run expedition-finish to remove expedition docs first:
/home/ketan/.claude/skills/expedition/scripts/expedition-finish.py --expedition java-language-support --apply
git add -A && git commit -m "chore: remove expedition docs"

# Then merge to main:
cd /home/ketan/project/refute
git merge java-language-support --no-ff
git push origin main
```

## To build the OpenRewrite JAR

```bash
cd /path/to/refute
mvn package -f adapters/openrewrite/pom.xml -q
# Produces: adapters/openrewrite/target/openrewrite-adapter.jar
```
