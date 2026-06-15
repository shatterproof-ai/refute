# End-to-End Test Coverage — Implementation Plan

> **Status:** executed — E2E coverage (dry-run, non-function rename, error
> cases) landed as part of the `go-code-actions` expedition (task 13). The
> integration suite under `internal/integration_test.go` reflects this work.
> Historical artifact; see [README.md](README.md) for status semantics.
> **Landing:** 2026-04-28, merge `78c5970`.
> **Disposition:** retained historical artifact.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Achieve Option C E2E coverage: dry-run tests for all languages, non-function symbol rename (struct/class/type) for Rust/Go/TypeScript, and error-case tests (symbol not found, bad server config, file not found).

**Architecture:** All new tests live in `internal/integration_test.go` alongside existing ones; a new `buildRefute(t)` helper extracted from repeated boilerplate. Fixtures extended in-place (new files/lines added; no existing fixture content changed). No new packages.

**Tech Stack:** Go `testing`, `os/exec`, `go test -tags integration`. LSP servers: `rust-analyzer`, `gopls`, `typescript-language-server`. Tests skip automatically when the required server is not on PATH; error-case tests have no server dependency.

---

## Scope

**Tests added:**
| Test | Language | Skip condition |
|---|---|---|
| `TestEndToEnd_RustDryRun` | Rust | no `rust-analyzer` |
| `TestEndToEnd_RenameRustStruct` | Rust | no `rust-analyzer` |
| `TestEndToEnd_RenameGoType` | Go | no `gopls` |
| `TestEndToEnd_RenameTypeScriptClass` | TypeScript | no `typescript-language-server` |
| `TestEndToEnd_SymbolNotFound` | — (Go fixture, no server) | none |
| `TestEndToEnd_BadServerConfig` | — (Rust fixture path, no server) | none |
| `TestEndToEnd_FileNotFound` | — (no server) | none |

**Fixtures extended:**
- `testdata/fixtures/rust/rename/src/lib.rs` — add `Greeter` struct (lines 5–7)
- `testdata/fixtures/rust/rename/src/main.rs` — add `Greeter` instantiation (line 4)
- `testdata/fixtures/go/rename/util/user.go` — new file with `User` struct
- `testdata/fixtures/go/rename/main.go` — add `util.User` reference at bottom
- `testdata/fixtures/typescript/rename/src/person.ts` — new file with `Person` class
- `testdata/fixtures/typescript/rename/src/main.ts` — add `Person` import + use

**Helper extracted:**
- `buildRefute(t *testing.T) string` replaces repeated build boilerplate in all existing tests.

---

## File Changes

```
internal/integration_test.go        — add buildRefute helper; refactor 5 existing tests;
                                       add 7 new tests

testdata/fixtures/rust/rename/
  src/lib.rs                         — APPEND Greeter struct (3 lines)
  src/main.rs                        — ADD Greeter instantiation before closing brace

testdata/fixtures/go/rename/
  util/user.go                       — CREATE: User struct + NewUser constructor
  main.go                            — APPEND two lines referencing util.User

testdata/fixtures/typescript/rename/
  src/person.ts                      — CREATE: Person class
  src/main.ts                        — APPEND Person import + instantiation
```

---

## Task 1: Extract `buildRefute` helper and refactor existing tests

`buildRefute` eliminates the 6-line build boilerplate repeated in every test. After extraction, all 5 existing tests are shorter and identical future tests are shorter too.

**Files:**
- Modify: `internal/integration_test.go`

- [ ] **Step 1.1: Add `buildRefute` to integration_test.go**

Insert just before `func copyDir`:

```go
// buildRefute compiles the refute binary into a temp dir and returns its path.
func buildRefute(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/refute")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build refute: %v\n%s", err, out)
	}
	return bin
}
```

- [ ] **Step 1.2: Refactor `TestEndToEnd_RenameGoFunction`**

Replace the 4-line build block:
```go
	refuteBin := filepath.Join(t.TempDir(), "refute")
	build := exec.Command("go", "build", "-o", refuteBin, "./cmd/refute")
	build.Dir = filepath.Join("..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %s\n%s", err, out)
	}
```
With:
```go
	refuteBin := buildRefute(t)
```

Apply the same replacement in these four tests (same pattern, same replacement each time):
- `TestEndToEnd_DryRun`
- `TestEndToEnd_RenameTypeScriptFunction`
- `TestEndToEnd_TypeScriptDryRun`
- `TestEndToEnd_RenameRustFunction`

- [ ] **Step 1.3: Run unit tests to confirm no regressions**

```bash
go test ./...
```
Expected: all 20 existing unit tests pass, build succeeds.

- [ ] **Step 1.4: Commit**

```bash
git add internal/integration_test.go
git commit -m "refactor: extract buildRefute helper in integration tests"
```

---

## Task 2: Rust dry-run test

Reuses the existing `testdata/fixtures/rust/rename/` fixture unchanged. Mirrors the Go `TestEndToEnd_DryRun` pattern.

**Files:**
- Modify: `internal/integration_test.go`

- [ ] **Step 2.1: Add `TestEndToEnd_RustDryRun` to integration_test.go**

Append before `// copyDir`:

```go
func TestEndToEnd_RustDryRun(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	libFile := filepath.Join(dir, "src", "lib.rs")
	originalContent, _ := os.ReadFile(libFile)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", libFile,
		"--line", "1",
		"--name", "format_greeting",
		"--new-name", "build_greeting",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute dry-run failed: %s\n%s", err, out)
	}

	if !strings.Contains(string(out), "format_greeting") || !strings.Contains(string(out), "build_greeting") {
		t.Errorf("dry-run output should show diff with both names, got:\n%s", out)
	}

	afterContent, _ := os.ReadFile(libFile)
	if string(afterContent) != string(originalContent) {
		t.Error("dry-run must not modify files")
	}
}
```

- [ ] **Step 2.2: Run the new test**

```bash
go test -tags integration -run TestEndToEnd_RustDryRun ./internal/... -timeout 300s -v
```
Expected: `PASS` (or `SKIP` if rust-analyzer not on PATH).

- [ ] **Step 2.3: Commit**

```bash
git add internal/integration_test.go
git commit -m "test: add Rust dry-run integration test"
```

---

## Task 3: Rust struct rename — fixture extension + test

Extends the Rust fixture in-place by appending a `Greeter` struct to `lib.rs` and adding a usage in `main.rs`. Existing tests are unaffected because they only look at lines 1–3 of `lib.rs`.

**Files:**
- Modify: `testdata/fixtures/rust/rename/src/lib.rs`
- Modify: `testdata/fixtures/rust/rename/src/main.rs`
- Modify: `internal/integration_test.go`

- [ ] **Step 3.1: Extend `lib.rs` with a Greeter struct**

The file currently has 3 lines. Append to make it:

```rust
pub fn format_greeting(name: &str) -> String {
    format!("Hello, {}!", name)
}

pub struct Greeter {
    pub name: String,
}
```

`Greeter` is on line 5. `rename-class --line 5 --name Greeter` will resolve to character 11 (0-indexed: after "pub struct ").

- [ ] **Step 3.2: Extend `main.rs` to reference Greeter**

The file currently has 4 lines. Append a `Greeter` instantiation before the closing brace so cross-file rename can be verified:

```rust
fn main() {
    let msg = greet::format_greeting("world");
    println!("{}", msg);
    let _g = greet::Greeter { name: "world".to_string() };
}
```

- [ ] **Step 3.3: Verify the fixture still compiles**

```bash
cd testdata/fixtures/rust/rename && cargo build 2>&1; cd -
```
Expected: `Finished` with no errors.

- [ ] **Step 3.4: Add `TestEndToEnd_RenameRustStruct` to integration_test.go**

```go
func TestEndToEnd_RenameRustStruct(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	libFile := filepath.Join(dir, "src", "lib.rs")
	cmd := exec.Command(refuteBin,
		"rename-class",
		"--file", libFile,
		"--line", "5",
		"--name", "Greeter",
		"--new-name", "Welcomer",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// lib.rs: struct definition renamed.
	libContent, _ := os.ReadFile(libFile)
	if strings.Contains(string(libContent), "Greeter") {
		t.Error("lib.rs still contains old name 'Greeter'")
	}
	if !strings.Contains(string(libContent), "Welcomer") {
		t.Error("lib.rs missing new name 'Welcomer'")
	}

	// main.rs: cross-file usage renamed.
	mainFile := filepath.Join(dir, "src", "main.rs")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "Greeter") {
		t.Error("main.rs still contains 'Greeter' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "Welcomer") {
		t.Error("main.rs missing 'Welcomer' after cross-file rename")
	}

	// Project still compiles.
	cargoCheck := exec.Command("cargo", "build")
	cargoCheck.Dir = dir
	if out, err := cargoCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}
```

- [ ] **Step 3.5: Run the new test**

```bash
go test -tags integration -run TestEndToEnd_RenameRustStruct ./internal/... -timeout 300s -v
```
Expected: `PASS` (or `SKIP`).

- [ ] **Step 3.6: Run all existing Rust tests to confirm no regression**

```bash
go test -tags integration -run TestEndToEnd_Rust ./internal/... -timeout 300s -v
```
Expected: all three Rust tests pass or skip.

- [ ] **Step 3.7: Commit**

```bash
git add testdata/fixtures/rust/ internal/integration_test.go
git commit -m "test: add Rust struct rename integration test and extend fixture"
```

---

## Task 4: Go type rename — fixture extension + test

Adds a `User` struct to the Go fixture via a new file `util/user.go`. Appends a reference in `main.go` so the cross-file rename is verifiable. Existing tests are unaffected.

**Files:**
- Create: `testdata/fixtures/go/rename/util/user.go`
- Modify: `testdata/fixtures/go/rename/main.go`
- Modify: `internal/integration_test.go`

- [ ] **Step 4.1: Create `util/user.go`**

```go
package util

// User represents a named user.
type User struct {
	Name string
}

// NewUser creates a User with the given name.
func NewUser(name string) *User {
	return &User{Name: name}
}
```

`User` is on line 4. `rename-type --line 4 --name User` will resolve to character 5 (after "type ").

- [ ] **Step 4.2: Append a `User` reference to `main.go`**

Current `main.go`:
```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	println(msg)
}
```

New `main.go`:
```go
package main

import "example.com/renametest/util"

func main() {
	msg := util.FormatGreeting("world")
	println(msg)
	u := util.NewUser("world")
	println(u.Name)
}
```

- [ ] **Step 4.3: Verify the fixture still compiles**

```bash
cd testdata/fixtures/go/rename && go build ./... 2>&1; cd -
```
Expected: no output (success).

- [ ] **Step 4.4: Add `TestEndToEnd_RenameGoType` to integration_test.go**

```go
func TestEndToEnd_RenameGoType(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	userFile := filepath.Join(dir, "util", "user.go")
	cmd := exec.Command(refuteBin,
		"rename-type",
		"--file", userFile,
		"--line", "4",
		"--name", "User",
		"--new-name", "Member",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// util/user.go: type definition renamed.
	userContent, _ := os.ReadFile(userFile)
	if strings.Contains(string(userContent), "type User struct") {
		t.Error("user.go still contains 'type User struct'")
	}
	if !strings.Contains(string(userContent), "type Member struct") {
		t.Error("user.go missing 'type Member struct'")
	}

	// main.go: cross-file usage renamed.
	mainFile := filepath.Join(dir, "main.go")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "util.User") {
		t.Error("main.go still contains 'util.User' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "util.Member") {
		t.Error("main.go missing 'util.Member' after cross-file rename")
	}

	// Project still compiles.
	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}
```

- [ ] **Step 4.5: Run the new test**

```bash
go test -tags integration -run TestEndToEnd_RenameGoType ./internal/... -timeout 120s -v
```
Expected: `PASS` (or `SKIP`).

- [ ] **Step 4.6: Run all Go E2E tests to confirm no regression**

```bash
go test -tags integration -run "TestEndToEnd_RenameGoFunction|TestEndToEnd_DryRun|TestEndToEnd_RenameGoType" ./internal/... -timeout 120s -v
```
Expected: all three tests pass or skip.

- [ ] **Step 4.7: Commit**

```bash
git add testdata/fixtures/go/rename/ internal/integration_test.go
git commit -m "test: add Go type rename integration test and extend fixture"
```

---

## Task 5: TypeScript class rename — fixture extension + test

Adds a `Person` class to the TypeScript fixture in a new file and imports it in `main.ts`. Existing tests use `greeter.ts` exclusively and are unaffected.

**Files:**
- Create: `testdata/fixtures/typescript/rename/src/person.ts`
- Modify: `testdata/fixtures/typescript/rename/src/main.ts`
- Modify: `internal/integration_test.go`

- [ ] **Step 5.1: Create `src/person.ts`**

```ts
export class Person {
    constructor(public readonly name: string) {}

    greet(): string {
        return `Hi, ${this.name}!`;
    }
}
```

`Person` is on line 1. `rename-class --line 1 --name Person` resolves to the character after `export class `.

- [ ] **Step 5.2: Append `Person` usage to `src/main.ts`**

Current `main.ts`:
```ts
import { greet } from "./greeter";

const message = greet("world");
console.log(message);
```

New `main.ts`:
```ts
import { greet } from "./greeter";
import { Person } from "./person";

const message = greet("world");
console.log(message);
const p = new Person("world");
console.log(p.greet());
```

- [ ] **Step 5.3: Add `TestEndToEnd_RenameTypeScriptClass` to integration_test.go**

```go
func TestEndToEnd_RenameTypeScriptClass(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	personFile := filepath.Join(dir, "src", "person.ts")
	cmd := exec.Command(refuteBin,
		"rename-class",
		"--file", personFile,
		"--line", "1",
		"--name", "Person",
		"--new-name", "Individual",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// person.ts: class definition renamed.
	personContent, _ := os.ReadFile(personFile)
	if strings.Contains(string(personContent), "class Person") {
		t.Error("person.ts still contains 'class Person'")
	}
	if !strings.Contains(string(personContent), "class Individual") {
		t.Error("person.ts missing 'class Individual'")
	}

	// main.ts: import and usage renamed.
	mainFile := filepath.Join(dir, "src", "main.ts")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "Person") {
		t.Error("main.ts still contains 'Person' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "Individual") {
		t.Error("main.ts missing 'Individual' after cross-file rename")
	}
}
```

- [ ] **Step 5.4: Run the new test**

```bash
go test -tags integration -run TestEndToEnd_RenameTypeScriptClass ./internal/... -timeout 120s -v
```
Expected: `PASS` (or `SKIP`).

- [ ] **Step 5.5: Run all TypeScript E2E tests to confirm no regression**

```bash
go test -tags integration -run "TestEndToEnd_RenameTypeScriptFunction|TestEndToEnd_TypeScriptDryRun|TestEndToEnd_RenameTypeScriptClass" ./internal/... -timeout 120s -v
```
Expected: all three tests pass or skip.

- [ ] **Step 5.6: Commit**

```bash
git add testdata/fixtures/typescript/rename/ internal/integration_test.go
git commit -m "test: add TypeScript class rename integration test and extend fixture"
```

---

## Task 6: Error case tests

Three tests that cover failure paths. None require a language server to be installed — they exercise the symbol resolver and CLI error handling layers.

**Files:**
- Modify: `internal/integration_test.go`

### Error path reference

**SymbolNotFound:** `symbol.Resolve` (Tier 2) calls `os.ReadFile`, then `strings.Index(line, name)`. When the name is absent, returns:
```
fmt.Errorf("name %q not found on line %d of %s", name, line, file)
```
CLI wraps as: `Error: symbol resolution: name "..." not found on line N of ...`

**BadServerConfig:** `adapter.Initialize` → `StartClient(command, ...)` → `exec.Command(command).Start()` fails when the binary doesn't exist. Returns wrapped error containing `"initializing backend"` and `"start"`.

**FileNotFound:** `symbol.Resolve` calls `os.ReadFile(file)` which fails with `"open ...: no such file or directory"`. CLI wraps as `Error: symbol resolution: reading ...: open ...: no such file or directory`.

- [ ] **Step 6.1: Add `TestEndToEnd_SymbolNotFound`**

This test uses the existing Go rename fixture. No language server is started because `symbol.Resolve` fails before the adapter is created.

```go
func TestEndToEnd_SymbolNotFound(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "NoSuchSymbol",
		"--new-name", "NewName",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for symbol-not-found, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "not found on line") {
		t.Errorf("expected 'not found on line' in output, got:\n%s", out)
	}
}
```

- [ ] **Step 6.2: Add `TestEndToEnd_BadServerConfig`**

Writes a temporary config JSON that overrides the `rust` server command with a nonexistent binary. Uses the path to a `.rs` fixture file to route to the rust backend. No rust-analyzer needed — the test verifies what happens when the LSP binary can't be launched.

```go
func TestEndToEnd_BadServerConfig(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Write a config that replaces rust-analyzer with a nonexistent binary.
	cfgContent := `{"servers": {"rust": {"command": "nonexistent-lsp-server-xyz"}}}`
	cfgFile := filepath.Join(t.TempDir(), "bad-config.json")
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	libFile := filepath.Join(dir, "src", "lib.rs")
	cmd := exec.Command(refuteBin,
		"--config", cfgFile,
		"rename-function",
		"--file", libFile,
		"--line", "1",
		"--name", "format_greeting",
		"--new-name", "build_greeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for bad server, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "initializing backend") {
		t.Errorf("expected 'initializing backend' in output, got:\n%s", out)
	}
}
```

- [ ] **Step 6.3: Add `TestEndToEnd_FileNotFound`**

```go
func TestEndToEnd_FileNotFound(t *testing.T) {
	refuteBin := buildRefute(t)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", "/nonexistent/path/to/file.go",
		"--line", "1",
		"--name", "Foo",
		"--new-name", "Bar",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for nonexistent file, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "no such file") {
		t.Errorf("expected 'no such file' in output, got:\n%s", out)
	}
}
```

- [ ] **Step 6.4: Run all three error tests (no server required, run without -tags integration)**

The error tests don't need a running server, but they're still in the integration build tag file. Run them with the tag but they should be fast:

```bash
go test -tags integration -run "TestEndToEnd_SymbolNotFound|TestEndToEnd_BadServerConfig|TestEndToEnd_FileNotFound" ./internal/... -timeout 60s -v
```
Expected: all three `PASS` immediately (no server startup delay).

- [ ] **Step 6.5: Commit**

```bash
git add internal/integration_test.go
git commit -m "test: add error-case integration tests (symbol not found, bad server, file not found)"
```

---

## Task 7: Full regression run + push

- [ ] **Step 7.1: Run all unit tests**

```bash
go test ./...
```
Expected: 20 packages pass.

- [ ] **Step 7.2: Run full integration suite**

```bash
go test -tags integration ./internal/... -timeout 600s -v 2>&1 | grep -E "^(=== RUN|--- PASS|--- FAIL|--- SKIP|FAIL|ok)"
```
Expected: every test is `PASS` or `SKIP` — none are `FAIL`.

- [ ] **Step 7.3: Push**

```bash
git push origin main
```

---

## Self-Review

**Spec coverage:**
- Rust dry-run ✓ Task 2
- Rust struct rename ✓ Task 3
- Go type rename ✓ Task 4
- TypeScript class rename ✓ Task 5
- Symbol not found ✓ Task 6.1
- Bad server config ✓ Task 6.2
- File not found ✓ Task 6.3
- `buildRefute` helper ✓ Task 1

**Placeholder scan:** None. All code blocks are complete and runnable.

**Type consistency:**
- `buildRefute` defined in Task 1.1, used identically in Tasks 2–6.
- Fixture line numbers: `lib.rs` Greeter on line 5 (Task 3.1 appends at line 5 after blank line 4). `user.go` User on line 4 (Task 4.1). `person.ts` Person on line 1 (Task 5.1).
- `rename-class` used for Rust struct (Task 3) — LSP rename is kind-agnostic at the position level, so this is correct.
- `rename-type` used for Go struct (Task 4) — `KindType` is the right kind for a Go `type` declaration.
