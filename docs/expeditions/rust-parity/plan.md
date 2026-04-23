# Rust Parity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring Rust support in `refute` to parity with Go tier-1 — extract-function, extract-variable, inline (single call site), Tier-1 qualified-name symbol resolution (`--symbol "crate::module::Type::method"`), plus hardening (local/parameter rename tests, better missing-server error, snippet-placeholder stripping, support-matrix doc).

**Architecture:** Reuse the Go tier-1 v2 surface (`Client.CodeActions`, `Client.ResolveCodeActionEdit`, `Client.WorkspaceSymbol`, adapter's `FindSymbol`/`ExtractFunction`/`ExtractVariable`/`InlineSymbol`, `cli/workspace.go`, `cli/errors.go`, `cli/extract.go`, `cli/inline.go`, `edit/json.go`). Add Rust-specific files: `rust_priming.go`, `rust_actions.go`, `rust_container.go` (backend/lsp), `rust_symbol.go` (cli). Adapter grows a language-dispatch layer — `PrimeWorkspace()` and `matchAction()` delegate to per-language files.

**Tech Stack:** Go 1.22+, rust-analyzer (already installed), existing cobra/fatih/color deps, stdlib `regexp`, `encoding/json`.

**Spec:** `docs/specs/2026-04-22-rust-parity-design.md`.

**Prerequisite:** `docs/plans/2026-04-17-go-code-actions-tier1-v2.md` must land on `main` before this plan starts. Verify before Task 0:

```bash
grep -q "func.*CodeActions" internal/backend/lsp/client.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
grep -q "func.*WorkspaceSymbol" internal/backend/lsp/client.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
test -f internal/cli/extract.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
test -f internal/cli/inline.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
test -f internal/cli/errors.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
test -f internal/edit/json.go || { echo "GO v2 NOT LANDED — STOP"; exit 1; }
echo "Go v2 prerequisites present."
```

If any check fails, stop and wait for Go v2 to land. Do not work around absent infrastructure.

---

## File Structure

```
internal/backend/lsp/
├── rust_actions.go          CREATE — kind+title pattern table; matchRustAction
├── rust_actions_test.go     CREATE — unit tests + drift guard against real rust-analyzer
├── rust_priming.go          CREATE — PrimeRustWorkspace walker
├── rust_priming_test.go     CREATE — skip-list + cap tests
├── rust_container.go        CREATE — parseRustContainer (branches on Task 0 spike)
├── rust_container_test.go   CREATE — parser tests
├── adapter.go               MODIFY — PrimeWorkspace() dispatch; matchAction() dispatch
└── client.go                MODIFY (expensive branch only) — add DocumentSymbol method

internal/cli/
├── rust_symbol.go           CREATE — parseRustQualifiedName (forms 1-7)
├── rust_symbol_test.go      CREATE — form + error-case coverage
├── rename.go                MODIFY — dispatch Rust qualified names through parseRustQualifiedName
├── extract.go               MODIFY — route Rust code-action matching through rust_actions
├── inline.go                MODIFY — add --call-site flag; require it for --symbol
└── errors.go                MODIFY — add ErrLSPServerMissing with install hints

internal/edit/
├── snippet.go               CREATE — stripSnippetPlaceholders
└── snippet_test.go          CREATE — round-trip coverage

internal/integration_test.go MODIFY — add 12 new tests

testdata/fixtures/rust/rename/
├── Cargo.toml               KEEP (unchanged)
├── src/lib.rs               EXTEND — multi-trait impl + extractable fn + rename targets
├── src/main.rs              EXTEND — cross-file call site for inline
└── src/util.rs              CREATE — form-7 target (greet::util::sum)

docs/
├── specs/2026-04-22-rust-parity-design.md  (exists)
├── plans/2026-04-22-rust-parity.md         (this file)
├── plans/2026-04-22-rust-parity-log.md     (written alongside this file)
├── plans/2026-04-22-rust-parity-handoff.md (written alongside this file)
└── support-matrix.md                       CREATE — H5
```

### Why these splits

- `rust_actions.go`, `rust_priming.go`, `rust_container.go` live next to the adapter — they are LSP-protocol-level, not CLI-level.
- `rust_symbol.go` in `cli/` mirrors how the Go qualified-name parser will live in `cli/` per Go v2 Task 10 — user-input parsing stays at the CLI layer.
- `snippet.go` in `edit/` lives next to the applier that consumes it.
- Integration tests stay in the existing `internal/integration_test.go` — splitting per language is a future cleanup, not this plan's concern.

---

## Task 0: Empirical spike on rust-analyzer `workspace/symbol` for trait impls

**Goal:** determine whether form-6 (`<Type as Trait>::method`) resolution takes the cheap branch (containerName includes trait) or expensive branch (need `textDocument/documentSymbol` walk).

**Files:**
- Create: `internal/backend/lsp/rust_spike_test.go` (removed at end of task)
- Modify: `testdata/fixtures/rust/rename/src/lib.rs` (multi-trait fixture; kept permanently — this is Task 1's territory but we need a minimal version now)

- [ ] **Step 1: Extend lib.rs with minimal multi-trait content**

Append to `testdata/fixtures/rust/rename/src/lib.rs` (after existing content):

```rust

use std::fmt;

impl fmt::Display for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Display: {}", self.name)
    }
}

impl fmt::Debug for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Debug: {}", self.name)
    }
}
```

Verify it builds:

```bash
cd testdata/fixtures/rust/rename && cargo build
```

Expected: clean build.

- [ ] **Step 2: Write spike test**

Create `internal/backend/lsp/rust_spike_test.go`:

```go
//go:build integration

package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/config"
)

// TestSpike_RustAnalyzerContainerName probes what rust-analyzer returns in
// SymbolInformation.ContainerName for methods inside `impl Trait for Type`
// blocks. Result drives the cheap vs expensive branch in rust_container.go.
//
// This test is intentionally ephemeral: delete the file after Task 0.
func TestSpike_RustAnalyzerContainerName(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	root, err := filepath.Abs("../../../testdata/fixtures/rust/rename")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.ServerConfig{Command: "rust-analyzer"}
	client, err := StartClient(cfg.Command, cfg.Args, root)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.WaitForIdle(ctx); err != nil {
		t.Fatalf("waitIdle: %v", err)
	}
	infos, err := client.WorkspaceSymbol("fmt")
	if err != nil {
		t.Fatalf("workspaceSymbol: %v", err)
	}
	b, _ := json.MarshalIndent(infos, "", "  ")
	t.Logf("workspace/symbol fmt → %s", b)
	if len(infos) < 2 {
		t.Fatalf("expected ≥2 fmt results, got %d", len(infos))
	}
	// Record the finding — the human reviewer reads this log and decides
	// which branch rust_container.go takes.
	for _, info := range infos {
		fmt.Printf("SPIKE containerName=%q name=%q location=%s\n",
			info.ContainerName, info.Name, info.Location.URI)
	}
}
```

- [ ] **Step 3: Run spike**

```bash
go test -tags integration -run TestSpike_RustAnalyzerContainerName ./internal/backend/lsp/ -v -timeout 300s
```

Read the logged output. Determine the branch:

- **Cheap:** `containerName` contains the trait name (e.g., `"Display"`, `"impl Display for Greeter"`, `"Greeter (Display)"`, etc.). Substring-matching the trait will disambiguate.
- **Expensive:** `containerName` is `"Greeter"` or empty for both `fmt` entries. Need hierarchical `textDocument/documentSymbol` walk.

- [ ] **Step 4: Record finding in the log**

Append to `docs/expeditions/rust-parity/log.md` under "Deviations from the Plan" (or a new "Task 0 Findings" section — add it at the top if absent):

```markdown
## Task 0 Findings

**rust-analyzer version:** <output of `rust-analyzer --version`>

**workspace/symbol `fmt` result containerName values:**
- Entry 1: containerName=<value>
- Entry 2: containerName=<value>

**Branch selection:** <cheap | expensive>

**Reasoning:** <one sentence>
```

- [ ] **Step 5: Delete the spike file**

```bash
rm internal/backend/lsp/rust_spike_test.go
```

- [ ] **Step 6: Commit**

```bash
git add testdata/fixtures/rust/rename/src/lib.rs docs/expeditions/rust-parity/log.md
git commit -m "spike(rust): record rust-analyzer workspace/symbol container behavior"
```

---

## Task 1: Extend Rust fixture

**Files:**
- Modify: `testdata/fixtures/rust/rename/src/lib.rs`
- Create: `testdata/fixtures/rust/rename/src/util.rs`
- Modify: `testdata/fixtures/rust/rename/src/main.rs`

- [ ] **Step 1: Rewrite lib.rs with all targets for later tasks**

Overwrite `testdata/fixtures/rust/rename/src/lib.rs`:

```rust
pub mod util;

use std::fmt;

pub fn format_greeting(name: &str) -> String {
    let prefix = "Hello";
    format!("{}, {}!", prefix, name)
}

pub fn compute(x: i32) -> i32 {
    (x * 2) + (x * 2)
}

pub struct Greeter {
    pub name: String,
}

impl fmt::Display for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Display: {}", self.name)
    }
}

impl fmt::Debug for Greeter {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "Debug: {}", self.name)
    }
}
```

Why each piece:
- `format_greeting` with `prefix` local + `name` parameter → H1, H2, existing function-rename test.
- `compute` with duplicated subexpression → extract-variable target.
- `Greeter` with `Display` + `Debug` impls → form-6 disambiguation target.
- `pub mod util;` → routes to `util.rs` for form-7 testing.

- [ ] **Step 2: Create util.rs**

Write `testdata/fixtures/rust/rename/src/util.rs`:

```rust
pub fn sum(a: i32, b: i32) -> i32 {
    a + b
}
```

- [ ] **Step 3: Rewrite main.rs**

Overwrite `testdata/fixtures/rust/rename/src/main.rs`:

```rust
fn main() {
    let msg = greet::format_greeting("world");
    println!("{}", msg);
    let _g = greet::Greeter { name: "world".to_string() };
    let total = greet::util::sum(1, 2);
    println!("{}", total);
}
```

- [ ] **Step 4: Verify cargo build**

```bash
cd testdata/fixtures/rust/rename && cargo build 2>&1 | tee /tmp/cargo.out
```

Expected: `Compiling greet v0.1.0` then `Finished`. No warnings from `cargo build` about unused code (other than the `_g` binding, which is acceptable).

- [ ] **Step 5: Re-run existing Rust integration tests to confirm no regression**

Return to repo root:

```bash
cd ../../..
go test -tags integration -run Rust ./internal/... -timeout 300s -v
```

Expected: `TestEndToEnd_RenameRustFunction`, `TestEndToEnd_RustDryRun`, `TestEndToEnd_RenameRustStruct` all pass (or skip cleanly if rust-analyzer is absent).

- [ ] **Step 6: Commit**

```bash
git add testdata/fixtures/rust/rename/
git commit -m "fixture(rust): extend with multi-trait impls, util module, rename targets"
```

---

## Task 2: Generalize `ErrLSPServerMissing` with install hints (H3)

**Files:**
- Modify: `internal/cli/errors.go` (Go v2 creates; this adds one type)
- Modify: `internal/config/config.go`
- Create: `internal/config/config_hints_test.go`

- [ ] **Step 1: Add install-hint map in config.go**

Append to `internal/config/config.go` (after the `builtinServers` map):

```go
// InstallHint returns a human-readable command the user can run to install the
// given LSP server. Returns the empty string if no hint is registered.
func InstallHint(language string) string {
	switch language {
	case "rust":
		return "rustup component add rust-analyzer"
	case "go":
		return "go install golang.org/x/tools/gopls@latest"
	case "typescript", "javascript":
		return "npm install -g typescript-language-server typescript"
	case "python":
		return "pip install pyright"
	}
	return ""
}
```

- [ ] **Step 2: Write unit test for install hints**

Create `internal/config/config_hints_test.go`:

```go
package config

import "testing"

func TestInstallHint(t *testing.T) {
	cases := map[string]string{
		"rust":       "rustup component add rust-analyzer",
		"go":         "go install golang.org/x/tools/gopls@latest",
		"typescript": "npm install -g typescript-language-server typescript",
		"javascript": "npm install -g typescript-language-server typescript",
		"python":     "pip install pyright",
		"cobol":      "",
	}
	for lang, want := range cases {
		if got := InstallHint(lang); got != want {
			t.Errorf("InstallHint(%q) = %q, want %q", lang, got, want)
		}
	}
}
```

Run:

```bash
go test ./internal/config/ -run TestInstallHint -v
```

Expected: PASS.

- [ ] **Step 3: Add `ErrLSPServerMissing` in cli/errors.go**

Append to `internal/cli/errors.go` (Go v2 created this file; preserve all its existing content):

```go
// ErrLSPServerMissing signals that the LSP server binary is not on PATH. The
// CLI maps this to exit code 3 (distinct from ErrSymbolNotFound's 2).
type ErrLSPServerMissing struct {
	Language    string
	Command     string
	InstallHint string
}

func (e *ErrLSPServerMissing) Error() string {
	if e.InstallHint != "" {
		return fmt.Sprintf("LSP server %q for %s not found on PATH. Install with: %s",
			e.Command, e.Language, e.InstallHint)
	}
	return fmt.Sprintf("LSP server %q for %s not found on PATH", e.Command, e.Language)
}

// ExitCode for ErrLSPServerMissing.
func (e *ErrLSPServerMissing) ExitCode() int { return 3 }
```

- [ ] **Step 4: Wire it into the adapter bootstrap**

In `internal/cli/rename.go` (and whichever sibling files Go v2 created for extract/inline), find the call to `exec.LookPath` or the point where `StartClient` fails with "executable file not found" and replace with:

```go
import (
	"errors"
	"os/exec"
	// ...existing imports
)

// ... inside the function that creates the adapter:
if _, err := exec.LookPath(serverCfg.Command); err != nil {
	return &ErrLSPServerMissing{
		Language:    language,
		Command:     serverCfg.Command,
		InstallHint: config.InstallHint(language),
	}
}
```

Apply at each call site that starts an LSP client. Use grep:

```bash
grep -rn "StartClient\|exec.LookPath" internal/cli/
```

Replace each occurrence's error path with the typed error.

- [ ] **Step 5: Run all unit tests**

```bash
go test ./internal/... -timeout 90s
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/config/ internal/cli/errors.go internal/cli/rename.go internal/cli/extract.go internal/cli/inline.go
git commit -m "feat(errors): ErrLSPServerMissing with install hints (H3)"
```

---

## Task 3: Snippet placeholder stripper (H4)

**Files:**
- Create: `internal/edit/snippet.go`
- Create: `internal/edit/snippet_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/edit/snippet_test.go`:

```go
package edit

import "testing"

func TestStripSnippetPlaceholders(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no placeholders", "fn foo() {}", "fn foo() {}"},
		{"tabstop zero", "fn $0() {}", "fn () {}"},
		{"tabstop numbered", "let $1 = 0;", "let  = 0;"},
		{"placeholder with default", "let ${1:x} = 0;", "let x = 0;"},
		{"placeholder nested default", "let ${1:vec![${2:0}]} = ();", "let vec![0] = ();"},
		{"choice", "fn ${1|foo,bar|}() {}", "fn foo() {}"},
		{"escaped dollar preserved", `printf("$$0")`, `printf("$0")`},
		{"literal dollar-digit in string preserved", `let s = "$5 bill";`, `let s = "$5 bill";`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := StripSnippetPlaceholders(c.in)
			if got != c.want {
				t.Errorf("StripSnippetPlaceholders(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestHasSnippetPlaceholders(t *testing.T) {
	cases := map[string]bool{
		"fn foo() {}":     false,
		"fn $0() {}":      true,
		"let ${1:x} = 0;": true,
		`"$5 bill"`:       false, // quoted literal: we err toward not stripping
	}
	for in, want := range cases {
		if got := HasSnippetPlaceholders(in); got != want {
			t.Errorf("HasSnippetPlaceholders(%q) = %v, want %v", in, got, want)
		}
	}
}
```

Note on the last case: `HasSnippetPlaceholders` uses the same regex as the stripper, so `"$5 bill"` *will* be detected as a placeholder if we are not careful. That is acceptable for H4's purpose — if an assist's `newText` contains a literal `$5 bill` inside a string, we pass it through unchanged anyway because the stripper is only invoked on edits that came from code-action resolution, where plain string content never looks like `$N`. The test documents expected behavior; implementation is simple.

Actually — fix: `HasSnippetPlaceholders` *should* return `true` for `"$5 bill"` because our regex cannot distinguish quoted literals. Update the test:

```go
func TestHasSnippetPlaceholders(t *testing.T) {
	cases := map[string]bool{
		"fn foo() {}":     false,
		"fn $0() {}":      true,
		"let ${1:x} = 0;": true,
		`"$5 bill"`:       true,  // detector is syntactic, not semantic
	}
	// ... (same loop)
}
```

Rationale documented inline: the stripper is only called on code-action `newText`, which doesn't include literal `$5 bill` in practice; being slightly over-eager on detection is safer than under-eager.

Run:

```bash
go test ./internal/edit/ -run 'TestStripSnippetPlaceholders|TestHasSnippetPlaceholders' -v
```

Expected: FAIL — `undefined: StripSnippetPlaceholders`.

- [ ] **Step 2: Implement the stripper**

Create `internal/edit/snippet.go`:

```go
package edit

import "regexp"

// LSP snippet syntax (https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#snippet_syntax):
//   $0, $1, $2, ...             — tabstops
//   ${1:default}                — placeholder with default (may nest)
//   ${1|one,two,three|}         — choice (first listed option becomes the default)
//   $$                          — escaped literal dollar

var (
	// Matches $N where N is one or more digits.
	tabstopRe = regexp.MustCompile(`\$[0-9]+`)

	// Matches ${N:...} including nested braces. We use a simple non-greedy
	// match and run iteratively; rust-analyzer does not emit deeply nested
	// snippets in practice but we handle up to 3 levels of nesting.
	placeholderRe = regexp.MustCompile(`\$\{[0-9]+:([^{}]*(?:\{[^{}]*\})?[^{}]*)\}`)

	// Matches ${N|a,b,c|}. Captures the first option.
	choiceRe = regexp.MustCompile(`\$\{[0-9]+\|([^,|]+)[^|]*\|\}`)
)

// HasSnippetPlaceholders reports whether s contains any LSP snippet tokens.
// Detection is syntactic and may have false positives on literal `$N` in user
// strings; callers should invoke this only on text known to originate from a
// code-action resolution.
func HasSnippetPlaceholders(s string) bool {
	return tabstopRe.MatchString(s) || placeholderRe.MatchString(s) || choiceRe.MatchString(s)
}

// StripSnippetPlaceholders removes LSP snippet tokens from s:
//   - ${N:default}       → default
//   - ${N|a,b,c|}        → a  (first choice)
//   - $N                 → (empty string)
//   - $$                 → $
// Nested placeholders are handled by iterating until no more tokens remain
// (up to a safe bound of 16 passes).
func StripSnippetPlaceholders(s string) string {
	const maxPasses = 16
	// Handle $$ first so later rules don't consume it.
	s = substituteEscapedDollar(s)
	for i := 0; i < maxPasses; i++ {
		next := choiceRe.ReplaceAllString(s, "$1")
		next = placeholderRe.ReplaceAllString(next, "$1")
		next = tabstopRe.ReplaceAllString(next, "")
		if next == s {
			break
		}
		s = next
	}
	return restoreDollar(s)
}

const escapedDollarSentinel = "\x00REFUTE_ESCAPED_DOLLAR\x00"

func substituteEscapedDollar(s string) string {
	return regexpReplaceLiteral(s, `\$\$`, escapedDollarSentinel)
}

func restoreDollar(s string) string {
	return regexpReplaceLiteral(s, escapedDollarSentinel, "$")
}

// regexpReplaceLiteral is a thin wrapper so the file compiles without
// introducing a top-level strings import for a one-line use.
func regexpReplaceLiteral(s, pattern, replacement string) string {
	return regexp.MustCompile(pattern).ReplaceAllLiteralString(s, replacement)
}
```

- [ ] **Step 3: Run tests to verify pass**

```bash
go test ./internal/edit/ -v
```

Expected: all pass including existing tests.

- [ ] **Step 4: Wire into the applier's code-action path**

Find the point in the edit applier (created by Go v2) where code-action resolution produces `FileEdit.Changes`. Likely in `internal/edit/applier.go` or wherever `ResolveCodeActionEdit` is consumed. Add a call that applies `StripSnippetPlaceholders` to each `TextEdit.NewText` only when the edit originates from a code action.

If Go v2 does not already have a distinguishing marker, add one: `WorkspaceEdit.FromCodeAction bool`. Set it `true` in the adapter's `ExtractFunction`, `ExtractVariable`, `InlineSymbol` implementations; leave `false` in `Rename`. In the applier:

```go
// internal/edit/applier.go, wherever edits are flattened before writing:
if edit.FromCodeAction {
	for i := range edit.FileEdits {
		for j := range edit.FileEdits[i].Changes {
			if HasSnippetPlaceholders(edit.FileEdits[i].Changes[j].NewText) {
				edit.FileEdits[i].Changes[j].NewText = StripSnippetPlaceholders(
					edit.FileEdits[i].Changes[j].NewText)
			}
		}
	}
}
```

If Go v2 has structured this differently, adapt to its actual structure — the invariant is: *only code-action edits pass through the stripper*.

- [ ] **Step 5: Run all tests**

```bash
go test ./internal/... -timeout 90s
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/edit/ internal/backend/lsp/adapter.go
git commit -m "feat(edit): strip LSP snippet placeholders from code-action edits (H4)"
```

---

## Task 4: `PrimeRustWorkspace`

**Files:**
- Create: `internal/backend/lsp/rust_priming.go`
- Create: `internal/backend/lsp/rust_priming_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/backend/lsp/rust_priming_test.go`:

```go
package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrimeRustWorkspace_SkipListAndCap(t *testing.T) {
	tmp := t.TempDir()
	// Valid Rust files.
	mkfile(t, tmp, "src/lib.rs")
	mkfile(t, tmp, "src/main.rs")
	// Should be skipped: target/, .git/, node_modules/, .cargo/.
	mkfile(t, tmp, "target/debug/build/foo-123/out/junk.rs")
	mkfile(t, tmp, ".git/hooks/post-commit.rs") // contrived
	mkfile(t, tmp, "node_modules/crate/src/x.rs")
	mkfile(t, tmp, ".cargo/registry/y.rs")
	// Additional real files to test the cap.
	for i := 0; i < maxPrimedFiles+5; i++ {
		mkfile(t, tmp, filepath.Join("extra", "f"+intToStr(i)+".rs"))
	}

	client := newFakeClient()
	err := PrimeRustWorkspace(client, tmp)
	if err != nil {
		t.Fatalf("PrimeRustWorkspace: %v", err)
	}
	if len(client.opened) > maxPrimedFiles {
		t.Errorf("opened %d files, want ≤%d", len(client.opened), maxPrimedFiles)
	}
	for _, path := range client.opened {
		for _, banned := range []string{"target/", ".git/", "node_modules/", ".cargo/"} {
			if contains(path, banned) {
				t.Errorf("opened banned path %q", path)
			}
		}
	}
}

func mkfile(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("// test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

If `newFakeClient` does not exist yet in the package, add this to the test file:

```go
type fakeClient struct{ opened []string }

func newFakeClient() *fakeClient { return &fakeClient{} }

func (c *fakeClient) DidOpen(path, langID string) error {
	c.opened = append(c.opened, path)
	return nil
}
```

And design `PrimeRustWorkspace` in Step 2 to accept any type with a `DidOpen(string, string) error` method (see below).

Run:

```bash
go test ./internal/backend/lsp/ -run TestPrimeRustWorkspace -v
```

Expected: FAIL — `undefined: PrimeRustWorkspace`.

- [ ] **Step 2: Implement the walker**

Create `internal/backend/lsp/rust_priming.go`:

```go
package lsp

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// didOpener is the minimal interface PrimeRustWorkspace needs. Both *Client
// and test fakes satisfy it.
type didOpener interface {
	DidOpen(path, languageID string) error
}

// PrimeRustWorkspace opens up to maxPrimedFiles Rust source files in the
// workspace so rust-analyzer begins indexing before the first rename or code-
// action request arrives. Failures to open individual files are non-fatal.
func PrimeRustWorkspace(client didOpener, root string) error {
	var opened int
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "target" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if opened >= maxPrimedFiles {
			return filepath.SkipAll
		}
		if strings.ToLower(filepath.Ext(path)) != ".rs" {
			return nil
		}
		if openErr := client.DidOpen(path, "rust"); openErr == nil {
			opened++
		}
		return nil
	})
}
```

The `strings.HasPrefix(name, ".")` check covers `.git`, `.cargo`, `.idea`, and others.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/backend/lsp/ -run TestPrimeRustWorkspace -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/backend/lsp/rust_priming.go internal/backend/lsp/rust_priming_test.go
git commit -m "feat(lsp): PrimeRustWorkspace — pre-open *.rs files for rust-analyzer"
```

---

## Task 5: Rust code-action pattern table

**Files:**
- Create: `internal/backend/lsp/rust_actions.go`
- Create: `internal/backend/lsp/rust_actions_test.go`

- [ ] **Step 1: Write failing unit tests**

Create `internal/backend/lsp/rust_actions_test.go`:

```go
package lsp

import "testing"

func TestMatchRustAction_ByKindAndTitle(t *testing.T) {
	actions := []CodeAction{
		{Kind: "refactor.extract.function", Title: "Extract into function"},
		{Kind: "refactor.extract.variable", Title: "Extract into variable"},
		{Kind: "refactor.inline.call",      Title: "Inline"},
		{Kind: "refactor.inline.callers",   Title: "Inline into all callers"},
		{Kind: "quickfix",                  Title: "Add `use std::fmt;`"},
	}
	cases := []struct {
		op       rustActionOp
		wantKind string
	}{
		{opExtractFunction, "refactor.extract.function"},
		{opExtractVariable, "refactor.extract.variable"},
		{opInlineCallSite,  "refactor.inline.call"},
		{opInlineAllCallers, "refactor.inline.callers"},
	}
	for _, c := range cases {
		got, err := matchRustAction(actions, c.op)
		if err != nil {
			t.Errorf("op %v: %v", c.op, err)
			continue
		}
		if got.Kind != c.wantKind {
			t.Errorf("op %v: got Kind=%q, want %q", c.op, got.Kind, c.wantKind)
		}
	}
}

func TestMatchRustAction_NotOffered(t *testing.T) {
	actions := []CodeAction{
		{Kind: "quickfix", Title: "Add `use std::fmt;`"},
	}
	if _, err := matchRustAction(actions, opExtractFunction); err == nil {
		t.Error("expected error when no matching action present")
	}
}

func TestMatchRustAction_KindPrefix(t *testing.T) {
	// Some rust-analyzer versions emit kind="refactor.extract" without a suffix.
	actions := []CodeAction{
		{Kind: "refactor.extract", Title: "Extract into function"},
	}
	got, err := matchRustAction(actions, opExtractFunction)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Extract into function" {
		t.Errorf("got Title=%q", got.Title)
	}
}
```

Run:

```bash
go test ./internal/backend/lsp/ -run TestMatchRustAction -v
```

Expected: FAIL — `undefined: matchRustAction` etc.

- [ ] **Step 2: Implement**

Create `internal/backend/lsp/rust_actions.go`:

```go
package lsp

import (
	"fmt"
	"regexp"
	"strings"
)

// rustActionOp names a logical refactoring operation. The same op can be
// satisfied by different rust-analyzer CodeAction kinds + titles across
// versions.
type rustActionOp int

const (
	opExtractFunction rustActionOp = iota
	opExtractVariable
	opInlineCallSite
	opInlineAllCallers
)

func (op rustActionOp) String() string {
	switch op {
	case opExtractFunction:
		return "opExtractFunction"
	case opExtractVariable:
		return "opExtractVariable"
	case opInlineCallSite:
		return "opInlineCallSite"
	case opInlineAllCallers:
		return "opInlineAllCallers"
	}
	return fmt.Sprintf("rustActionOp(%d)", int(op))
}

type rustActionPattern struct {
	kindPrefix  string
	titleRegexp *regexp.Regexp
}

var rustActionPatterns = map[rustActionOp]rustActionPattern{
	opExtractFunction:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*function`)},
	opExtractVariable:  {"refactor.extract", regexp.MustCompile(`(?i)extract .*variable`)},
	opInlineCallSite:   {"refactor.inline", regexp.MustCompile(`(?i)^inline( call)?$`)},
	opInlineAllCallers: {"refactor.inline", regexp.MustCompile(`(?i)inline .*all callers`)},
}

// ErrActionNotOffered is returned when no code action matches the requested op.
type ErrActionNotOffered struct {
	Op             rustActionOp
	OfferedTitles  []string
}

func (e *ErrActionNotOffered) Error() string {
	return fmt.Sprintf("rust-analyzer offered no action matching %s. Offered titles: %v",
		e.Op, e.OfferedTitles)
}

// matchRustAction finds the first action whose Kind starts with the expected
// prefix and whose Title matches the expected regex.
func matchRustAction(actions []CodeAction, op rustActionOp) (*CodeAction, error) {
	pat, ok := rustActionPatterns[op]
	if !ok {
		return nil, fmt.Errorf("no pattern registered for %s", op)
	}
	offered := make([]string, 0, len(actions))
	for i := range actions {
		offered = append(offered, actions[i].Title)
		if !strings.HasPrefix(actions[i].Kind, pat.kindPrefix) {
			continue
		}
		if pat.titleRegexp.MatchString(actions[i].Title) {
			return &actions[i], nil
		}
	}
	return nil, &ErrActionNotOffered{Op: op, OfferedTitles: offered}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/backend/lsp/ -run TestMatchRustAction -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/backend/lsp/rust_actions.go internal/backend/lsp/rust_actions_test.go
git commit -m "feat(lsp): rust-analyzer code-action pattern table"
```

---

## Task 6: Rust container parser

**Files:**
- Create: `internal/backend/lsp/rust_container.go`
- Create: `internal/backend/lsp/rust_container_test.go`
- (Expensive branch only) Modify: `internal/backend/lsp/client.go` — add `DocumentSymbol` if not present.

**Branch selection:** read Task 0's entry in `docs/expeditions/rust-parity/log.md`. If it records "cheap", follow Steps 1-4. If "expensive", follow Steps 1-4 then Steps 5-8 additionally.

- [ ] **Step 1: Write cheap-branch unit tests**

Create `internal/backend/lsp/rust_container_test.go`:

```go
package lsp

import "testing"

func TestParseRustContainer_Cheap(t *testing.T) {
	cases := []struct {
		name, in              string
		wantModule            []string
		wantType, wantTrait   string
	}{
		{"plain type", "Greeter", nil, "Greeter", ""},
		{"module path", "greet::Greeter", []string{"greet"}, "Greeter", ""},
		{"trait qualified", "impl Display for Greeter", nil, "Greeter", "Display"},
		{"trait qualified with module", "greet::impl Display for Greeter", []string{"greet"}, "Greeter", "Display"},
		{"paren style", "Greeter (Display)", nil, "Greeter", "Display"},
		{"empty", "", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod, typ, tr := parseRustContainer(c.in)
			if !stringSliceEq(mod, c.wantModule) {
				t.Errorf("module = %v, want %v", mod, c.wantModule)
			}
			if typ != c.wantType {
				t.Errorf("type = %q, want %q", typ, c.wantType)
			}
			if tr != c.wantTrait {
				t.Errorf("trait = %q, want %q", tr, c.wantTrait)
			}
		})
	}
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Run:

```bash
go test ./internal/backend/lsp/ -run TestParseRustContainer -v
```

Expected: FAIL — `undefined: parseRustContainer`.

- [ ] **Step 2: Implement the cheap-branch parser**

Create `internal/backend/lsp/rust_container.go`:

```go
package lsp

import (
	"regexp"
	"strings"
)

// implForRe extracts (trait, type) from a rust-analyzer containerName of the
// form "impl <Trait> for <Type>". The trait may include `fmt::` prefixes.
var implForRe = regexp.MustCompile(`^impl\s+(\S+?)\s+for\s+(\S+)\s*$`)

// parenStyleRe extracts (type, trait) from "Type (Trait)".
var parenStyleRe = regexp.MustCompile(`^(\S+)\s*\(([^)]+)\)\s*$`)

// parseRustContainer normalizes rust-analyzer's `containerName` into
// (modulePath, type, trait). Rust-analyzer emits several formats across
// versions; we recognize:
//
//   "Greeter"                        → type="Greeter"
//   "greet::Greeter"                 → module=["greet"], type="Greeter"
//   "impl Display for Greeter"       → type="Greeter", trait="Display"
//   "greet::impl Display for Greeter"→ module=["greet"], type="Greeter", trait="Display"
//   "Greeter (Display)"              → type="Greeter", trait="Display"
//
// Unrecognized inputs yield (nil, container, "") — the type is assumed to be
// the whole string. Downstream matching tolerates that.
func parseRustContainer(container string) (modulePath []string, typ string, trait string) {
	container = strings.TrimSpace(container)
	if container == "" {
		return nil, "", ""
	}
	// Strip a trailing "impl X for Y" suffix, honouring possible module prefix.
	if idx := strings.LastIndex(container, "::"); idx >= 0 {
		head := container[:idx]
		tail := container[idx+2:]
		if m := implForRe.FindStringSubmatch(tail); m != nil {
			return splitModulePath(head), m[2], trimTraitPath(m[1])
		}
	}
	if m := implForRe.FindStringSubmatch(container); m != nil {
		return nil, m[2], trimTraitPath(m[1])
	}
	if m := parenStyleRe.FindStringSubmatch(container); m != nil {
		return nil, m[1], trimTraitPath(m[2])
	}
	// Plain module path.
	parts := strings.Split(container, "::")
	if len(parts) == 1 {
		return nil, parts[0], ""
	}
	return parts[:len(parts)-1], parts[len(parts)-1], ""
}

func splitModulePath(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(s, "::")
}

// trimTraitPath keeps only the last segment of a path like "std::fmt::Display".
// This lets users pass `Display` without the full path.
func trimTraitPath(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "::")
	return parts[len(parts)-1]
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/backend/lsp/ -run TestParseRustContainer -v
```

Expected: PASS.

- [ ] **Step 4: Cheap-branch commit**

If Task 0 recorded "cheap":

```bash
git add internal/backend/lsp/rust_container.go internal/backend/lsp/rust_container_test.go
git commit -m "feat(lsp): parseRustContainer — normalize rust-analyzer containerName"
```

And **skip Steps 5-8**.

- [ ] **Step 5: (Expensive branch) Add `DocumentSymbol` to Client**

Only run if Task 0 recorded "expensive". In `internal/backend/lsp/client.go`, after the existing `WorkspaceSymbol` method, add:

```go
// DocumentSymbol holds a hierarchical symbol entry from textDocument/documentSymbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// Range mirrors the LSP Range type (0-indexed).
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position mirrors the LSP Position type (0-indexed).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// DocumentSymbol requests hierarchical document symbols for a file. If
// rust-analyzer returns the flat SymbolInformation form instead, callers
// must fall back to the cheap branch for that file.
func (c *Client) DocumentSymbol(path string) ([]DocumentSymbol, error) {
	uri := "file://" + filepath.ToSlash(path)
	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": uri},
	}
	var result []DocumentSymbol
	if err := c.call("textDocument/documentSymbol", params, &result); err != nil {
		return nil, err
	}
	return result, nil
}
```

Ensure the import block includes `path/filepath` if not already present.

- [ ] **Step 6: (Expensive branch) Extend `rust_container.go`**

Add to `rust_container.go`:

```go
// FindEnclosingImplTrait walks a document-symbol tree to find the `impl ...`
// ancestor whose range contains targetLine (0-indexed). Returns the parsed
// trait name (last `::` segment) or "" if no such ancestor exists.
func FindEnclosingImplTrait(symbols []DocumentSymbol, targetLine int) string {
	for _, sym := range symbols {
		if targetLine < sym.Range.Start.Line || targetLine > sym.Range.End.Line {
			continue
		}
		if m := implForRe.FindStringSubmatch(sym.Name); m != nil {
			return trimTraitPath(m[1])
		}
		if t := FindEnclosingImplTrait(sym.Children, targetLine); t != "" {
			return t
		}
	}
	return ""
}
```

- [ ] **Step 7: (Expensive branch) Add test**

Add to `rust_container_test.go`:

```go
func TestFindEnclosingImplTrait(t *testing.T) {
	doc := []DocumentSymbol{
		{
			Name:  "impl Display for Greeter",
			Range: Range{Start: Position{Line: 10}, End: Position{Line: 20}},
			Children: []DocumentSymbol{
				{Name: "fmt", Range: Range{Start: Position{Line: 12}, End: Position{Line: 14}}},
			},
		},
		{
			Name:  "impl Debug for Greeter",
			Range: Range{Start: Position{Line: 22}, End: Position{Line: 30}},
			Children: []DocumentSymbol{
				{Name: "fmt", Range: Range{Start: Position{Line: 24}, End: Position{Line: 26}}},
			},
		},
	}
	if got := FindEnclosingImplTrait(doc, 13); got != "Display" {
		t.Errorf("line 13 → %q, want Display", got)
	}
	if got := FindEnclosingImplTrait(doc, 25); got != "Debug" {
		t.Errorf("line 25 → %q, want Debug", got)
	}
	if got := FindEnclosingImplTrait(doc, 100); got != "" {
		t.Errorf("line 100 → %q, want empty", got)
	}
}
```

Run:

```bash
go test ./internal/backend/lsp/ -run 'TestParseRustContainer|TestFindEnclosingImplTrait' -v
```

Expected: PASS.

- [ ] **Step 8: (Expensive branch) Commit**

```bash
git add internal/backend/lsp/rust_container.go internal/backend/lsp/rust_container_test.go internal/backend/lsp/client.go
git commit -m "feat(lsp): parseRustContainer + DocumentSymbol walk for trait disambiguation"
```

---

## Task 7: Adapter language dispatch

**Files:**
- Modify: `internal/backend/lsp/adapter.go`

- [ ] **Step 1: Add dispatch helpers**

At the bottom of `internal/backend/lsp/adapter.go`, add:

```go
// primeWorkspace dispatches to the language-specific priming walker. Failures
// are intentionally non-fatal — if priming partially fails the first request
// will still trigger the rest of the index.
func (a *Adapter) primeWorkspace(absRoot string) {
	switch {
	case isTSFamily(a.languageID):
		_ = PrimeTSWorkspace(a.client, absRoot)
	case a.languageID == "rust":
		_ = PrimeRustWorkspace(a.client, absRoot)
	}
}

// matchAction dispatches to the language-specific action-pattern matcher.
// Returns ErrUnsupported if the language has no matcher registered.
func (a *Adapter) matchAction(actions []CodeAction, op rustActionOp) (*CodeAction, error) {
	if a.languageID == "rust" {
		return matchRustAction(actions, op)
	}
	return nil, fmt.Errorf("no code-action matcher for language %q", a.languageID)
}
```

- [ ] **Step 2: Replace the inline TS-priming call in `Initialize`**

Find this block in `adapter.go`:

```go
if isTSFamily(a.languageID) {
    _ = PrimeTSWorkspace(a.client, absRoot)
}
```

Replace with:

```go
a.primeWorkspace(absRoot)
```

- [ ] **Step 3: Run existing adapter tests**

```bash
go test ./internal/backend/lsp/ -timeout 90s
```

Expected: all pass — the only behavioral change is that `rust` now also primes.

- [ ] **Step 4: Commit**

```bash
git add internal/backend/lsp/adapter.go
git commit -m "refactor(lsp): language-dispatch primeWorkspace and matchAction"
```

---

## Task 8: Rust qualified-name parser

**Files:**
- Create: `internal/cli/rust_symbol.go`
- Create: `internal/cli/rust_symbol_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/cli/rust_symbol_test.go`:

```go
package cli

import "testing"

func TestParseRustQualifiedName(t *testing.T) {
	cases := []struct {
		name, in            string
		wantModule          []string
		wantTrait, wantName string
		wantErr             bool
	}{
		{"form 1 bare", "format_greeting", nil, "", "format_greeting", false},
		{"form 2 module", "greet::format_greeting", []string{"greet"}, "", "format_greeting", false},
		{"form 3 crate prefix", "crate::greet::format_greeting", []string{"greet"}, "", "format_greeting", false},
		{"form 4 assoc fn", "Greeter::new", []string{"Greeter"}, "", "new", false},
		{"form 5 method", "Greeter::greet", []string{"Greeter"}, "", "greet", false},
		{"form 7 module+type+method", "greet::Greeter::new", []string{"greet", "Greeter"}, "", "new", false},
		{"form 6 trait qualified", "<Greeter as Display>::fmt", []string{"Greeter"}, "Display", "fmt", false},
		{"form 6 nested generics", "<Vec<T> as IntoIterator>::next", []string{"Vec<T>"}, "IntoIterator", "next", false},
		{"form 6+7", "greet::<Greeter as Display>::fmt", []string{"greet", "Greeter"}, "Display", "fmt", false},

		{"err parens", "foo()", nil, "", "", true},
		{"err whitespace", "foo bar", nil, "", "", true},
		{"err unmatched angle", "<Greeter as Display::fmt", nil, "", "", true},
		{"err missing as", "<Greeter Display>::fmt", nil, "", "", true},
		{"err trailing colons", "greet::", nil, "", "", true},
		{"err empty", "", nil, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mod, tr, n, err := ParseRustQualifiedName(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if !stringSliceEq(mod, c.wantModule) {
				t.Errorf("module=%v want %v", mod, c.wantModule)
			}
			if tr != c.wantTrait {
				t.Errorf("trait=%q want %q", tr, c.wantTrait)
			}
			if n != c.wantName {
				t.Errorf("name=%q want %q", n, c.wantName)
			}
		})
	}
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Run:

```bash
go test ./internal/cli/ -run TestParseRustQualifiedName -v
```

Expected: FAIL — `undefined: ParseRustQualifiedName`.

- [ ] **Step 2: Implement the parser**

Create `internal/cli/rust_symbol.go`:

```go
package cli

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseRustQualifiedName parses a user-supplied --symbol value for Rust.
// Supported forms (see spec for numbering):
//
//   1: name
//   2: module::name
//   3: crate::module::name         (crate:: stripped)
//   4: Type::assoc_fn
//   5: Type::method
//   6: <Type as Trait>::method
//   7: module::Type::method
//   6+7: module::<Type as Trait>::method
//
// Returns (modulePath, trait, name). modulePath ends with the type if the
// input names a method or associated function; the caller uses this for
// workspace/symbol container matching.
func ParseRustQualifiedName(s string) ([]string, string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, "", "", fmt.Errorf("empty symbol")
	}
	if err := validateRustSymbolChars(s); err != nil {
		return nil, "", "", err
	}

	var modulePrefix []string
	if i := strings.Index(s, "<"); i >= 0 && (i == 0 || strings.HasSuffix(s[:i], "::")) {
		// Form 6 (possibly combined with module prefix).
		if i > 0 {
			head := strings.TrimSuffix(s[:i], "::")
			head = strings.TrimPrefix(head, "crate::")
			modulePrefix = splitNonEmpty(head, "::")
			s = s[i:]
		}
		inner, tail, err := splitTraitQualified(s)
		if err != nil {
			return nil, "", "", err
		}
		typ, trait, err := parseTypeAsTrait(inner)
		if err != nil {
			return nil, "", "", err
		}
		name, err := requireSingleSegment(tail)
		if err != nil {
			return nil, "", "", err
		}
		mod := append(modulePrefix, typ)
		return mod, trait, name, nil
	}

	// Forms 1-5, 7.
	s = strings.TrimPrefix(s, "crate::")
	parts := strings.Split(s, "::")
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return nil, "", "", fmt.Errorf("empty segment in %q", s)
		}
	}
	name := parts[len(parts)-1]
	module := parts[:len(parts)-1]
	if len(module) == 0 {
		return nil, "", name, nil
	}
	return module, "", name, nil
}

func validateRustSymbolChars(s string) error {
	// Permit angle brackets (form 6), colons, underscore, alphanumerics,
	// and ampersands/commas/spaces *inside* angle brackets (generics).
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case '(', ')', '{', '}', ';':
			return fmt.Errorf("invalid character %q in symbol", r)
		default:
			if depth == 0 && unicode.IsSpace(r) {
				return fmt.Errorf("whitespace not permitted outside <...>")
			}
		}
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced angle brackets in symbol")
	}
	return nil
}

// splitTraitQualified splits "<Type as Trait>::name" into ("Type as Trait", "name").
// The input must start with '<'. Tracks nested angle-bracket depth for
// generics like <Vec<T> as IntoIterator>.
func splitTraitQualified(s string) (inner, tail string, err error) {
	if !strings.HasPrefix(s, "<") {
		return "", "", fmt.Errorf("trait-qualified form must start with '<'")
	}
	depth := 0
	for i, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				rest := s[i+1:]
				if !strings.HasPrefix(rest, "::") {
					return "", "", fmt.Errorf("expected '::' after '>'")
				}
				return s[1:i], rest[2:], nil
			}
		}
	}
	return "", "", fmt.Errorf("unmatched '<'")
}

func parseTypeAsTrait(inner string) (typ, trait string, err error) {
	// Look for " as " at top-level angle-bracket depth.
	depth := 0
	for i := 0; i+4 <= len(inner); i++ {
		switch inner[i] {
		case '<':
			depth++
			continue
		case '>':
			depth--
			continue
		}
		if depth != 0 {
			continue
		}
		if inner[i] == ' ' && i+4 <= len(inner) && inner[i:i+4] == " as " {
			typ = strings.TrimSpace(inner[:i])
			trait = strings.TrimSpace(inner[i+4:])
			if typ == "" || trait == "" {
				return "", "", fmt.Errorf("empty type or trait in %q", inner)
			}
			return typ, trait, nil
		}
	}
	return "", "", fmt.Errorf("missing ' as ' in trait qualification %q", inner)
}

func requireSingleSegment(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("missing method name after '>::'")
	}
	if strings.Contains(s, "::") {
		return "", fmt.Errorf("unexpected '::' in method name %q", s)
	}
	return s, nil
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, sep)
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/cli/ -run TestParseRustQualifiedName -v
```

Expected: PASS. If the `err trailing colons` test fails, verify the empty-segment check in the parser.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/rust_symbol.go internal/cli/rust_symbol_test.go
git commit -m "feat(cli): ParseRustQualifiedName — forms 1-7 incl. trait qualification"
```

---

## Task 9: CLI rename dispatch for Rust qualified names

**Files:**
- Modify: `internal/cli/rename.go`

**Context:** Go v2's Task 10 adds a `runRenameTier1` path that dispatches on language to parse `--symbol` and call `adapter.FindSymbol`. This task adds a Rust branch.

- [ ] **Step 1: Locate the Tier-1 dispatch**

```bash
grep -n "runRenameTier1\|parseQualifiedName\|FindSymbol" internal/cli/rename.go
```

Find the existing Go-specific parsing point. It looks like:

```go
if language == "go" {
    parsed, err := parseGoQualifiedName(symbolFlag)
    // ...
}
```

- [ ] **Step 2: Add the Rust branch**

Immediately below the Go branch, add:

```go
if language == "rust" {
    modulePath, trait, name, err := ParseRustQualifiedName(symbolFlag)
    if err != nil {
        return fmt.Errorf("parse --symbol: %w", err)
    }
    infos, err := adapter.FindSymbol(symbol.Query{Name: name})
    if err != nil {
        return err
    }
    // NOTE: filterRustCandidates expects each candidate to expose its
    // workspace/symbol `containerName`. Go v2 must surface this. Preferred
    // shape: extend symbol.Location with a `Container string` field and have
    // the adapter populate it in FindSymbol. If Go v2 chose a different shape
    // (e.g., a new backend.SymbolMatch type), substitute it here and adjust
    // filterRustCandidates's signature. Verify before implementing.
    candidates := filterRustCandidates(infos, modulePath, trait, name, adapter)
    switch len(candidates) {
    case 0:
        return &ErrSymbolNotFound{
            Language:   "rust",
            Input:      symbolFlag,
            ModulePath: modulePath,
            Trait:      trait,
            Name:       name,
        }
    case 1:
        loc = candidates[0]
    default:
        return renderAmbiguous(candidates, jsonFlag)
    }
}
```

- [ ] **Step 3: Add `filterRustCandidates` helper**

At the bottom of `rename.go`, add:

```go
// filterRustCandidates narrows workspace/symbol results by module path and,
// for trait-qualified queries, by enclosing trait. The cheap branch matches
// the trait via parseRustContainer. The expensive branch falls back to
// DocumentSymbol when the container parser cannot identify the trait.
func filterRustCandidates(
	infos []symbol.Location,
	modulePath []string,
	trait, name string,
	adapter backend.RefactoringBackend,
) []symbol.Location {
	out := make([]symbol.Location, 0, len(infos))
	for _, info := range infos {
		if info.Name != name {
			continue
		}
		infoMod, infoType, infoTrait := lsp.ParseRustContainer(info.Container)
		if !moduleMatches(modulePath, infoMod, infoType) {
			continue
		}
		if trait != "" {
			resolved := infoTrait
			if resolved == "" {
				resolved = resolveTraitByDocumentSymbol(adapter, info)
			}
			if resolved != trait {
				continue
			}
		}
		out = append(out, info)
	}
	return out
}

// moduleMatches returns true when expected is a suffix of actual (actual may
// have extra leading segments). An expected of ["Greeter"] matches any
// container ending in Greeter; ["greet", "Greeter"] requires both.
func moduleMatches(expected, actualMod []string, actualType string) bool {
	full := append([]string{}, actualMod...)
	if actualType != "" {
		full = append(full, actualType)
	}
	if len(expected) == 0 {
		return true
	}
	if len(full) < len(expected) {
		return false
	}
	for i := 0; i < len(expected); i++ {
		if full[len(full)-len(expected)+i] != expected[i] {
			return false
		}
	}
	return true
}

// resolveTraitByDocumentSymbol is populated only on the expensive branch.
// On the cheap branch it is a no-op that returns "". See Task 6.
func resolveTraitByDocumentSymbol(adapter backend.RefactoringBackend, info symbol.Location) string {
	a, ok := adapter.(*lsp.Adapter)
	if !ok {
		return ""
	}
	symbols, err := a.DocumentSymbols(info.File)
	if err != nil {
		return ""
	}
	return lsp.FindEnclosingImplTrait(symbols, info.Line-1) // info.Line is 1-indexed; DocSymbol is 0-indexed
}
```

The `lsp.ParseRustContainer` (exported wrapper around lowercase `parseRustContainer`) and `lsp.Adapter.DocumentSymbols` must be exported. Add them to `rust_container.go` and `adapter.go`:

```go
// rust_container.go
func ParseRustContainer(s string) ([]string, string, string) { return parseRustContainer(s) }

// adapter.go (expensive branch only)
func (a *Adapter) DocumentSymbols(path string) ([]DocumentSymbol, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	return a.client.DocumentSymbol(path)
}
```

On the cheap branch, `DocumentSymbols` is not added; `resolveTraitByDocumentSymbol` always returns `""`; the `if resolved == "" { … }` block becomes dead code that still compiles.

- [ ] **Step 4: Add `ModulePath` and `Trait` fields to `ErrSymbolNotFound`**

Go v2 creates `ErrSymbolNotFound` in `errors.go`. Extend it:

```go
type ErrSymbolNotFound struct {
	Language   string
	Input      string
	ModulePath []string
	Trait      string
	Name       string
}

func (e *ErrSymbolNotFound) Error() string {
	var parts []string
	if len(e.ModulePath) > 0 {
		parts = append(parts, "container="+strings.Join(e.ModulePath, "::"))
	}
	if e.Trait != "" {
		parts = append(parts, "trait="+e.Trait)
	}
	parts = append(parts, "name="+e.Name)
	return fmt.Sprintf("no %s symbol matched %s (input: %q)",
		e.Language, strings.Join(parts, " "), e.Input)
}

func (e *ErrSymbolNotFound) ExitCode() int { return 2 }
```

- [ ] **Step 5: Run unit tests**

```bash
go test ./internal/... -timeout 90s
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/rename.go internal/cli/errors.go internal/backend/lsp/rust_container.go internal/backend/lsp/adapter.go
git commit -m "feat(cli): dispatch Rust --symbol through ParseRustQualifiedName"
```

---

## Task 10: CLI extract dispatch for Rust

**Files:**
- Modify: `internal/cli/extract.go`

**Context:** Go v2's extract commands call `adapter.ExtractFunction` / `ExtractVariable` which internally select a gopls code action. For Rust, the adapter needs to select via `matchRustAction`.

- [ ] **Step 1: Add language-aware action selection in the adapter**

In `internal/backend/lsp/adapter.go`, replace the stub `ExtractFunction` with:

```go
func (a *Adapter) ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	return a.runCodeAction(r, name, opExtractFunction)
}

func (a *Adapter) ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	return a.runCodeAction(r, name, opExtractVariable)
}

func (a *Adapter) InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error) {
	r := symbol.SourceRange{
		File:      loc.File,
		StartLine: loc.Line,
		StartCol:  loc.Column,
		EndLine:   loc.Line,
		EndCol:    loc.Column + len(loc.Name),
	}
	return a.runCodeAction(r, "", opInlineCallSite)
}

// runCodeAction requests code actions at the given range, selects one via the
// language-specific matcher, resolves edits if deferred, and renames any
// placeholder variable to `name` when provided.
func (a *Adapter) runCodeAction(
	r symbol.SourceRange, name string, op rustActionOp,
) (*edit.WorkspaceEdit, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	if err := a.client.DidOpen(r.File, a.languageID); err != nil {
		return nil, err
	}
	actions, err := a.client.CodeActions(r.File, toLSPRange(r))
	if err != nil {
		return nil, err
	}
	chosen, err := a.matchAction(actions, op)
	if err != nil {
		return nil, err
	}
	resolved, err := a.client.ResolveCodeActionEdit(*chosen)
	if err != nil {
		return nil, err
	}
	if name != "" {
		rewritePlaceholderName(resolved, name)
	}
	return &edit.WorkspaceEdit{
		FileEdits:       resolved.FileEdits,
		FromCodeAction: true,
	}, nil
}
```

If `toLSPRange` and `rewritePlaceholderName` were created by Go v2, reuse them. If Go v2 put `rewritePlaceholderName` in a different file, leave it there and import. If it does not exist, add a minimal one in `adapter.go`:

```go
// rewritePlaceholderName replaces the first occurrence of any $N or ${N:...}
// token in the edit with the user-provided name.
func rewritePlaceholderName(w *edit.WorkspaceEdit, name string) {
	for i := range w.FileEdits {
		for j := range w.FileEdits[i].Changes {
			t := &w.FileEdits[i].Changes[j]
			if edit.HasSnippetPlaceholders(t.NewText) {
				t.NewText = edit.ReplaceFirstPlaceholder(t.NewText, name)
				return
			}
		}
	}
}
```

And add `ReplaceFirstPlaceholder` to `internal/edit/snippet.go`:

```go
// ReplaceFirstPlaceholder substitutes the first $N or ${N:...} token with
// name. Intended for code-action edits where a single variable/function name
// placeholder represents the user-chosen identifier.
func ReplaceFirstPlaceholder(s, name string) string {
	if m := placeholderRe.FindStringIndex(s); m != nil {
		return s[:m[0]] + name + s[m[1]:]
	}
	if m := tabstopRe.FindStringIndex(s); m != nil {
		return s[:m[0]] + name + s[m[1]:]
	}
	return s
}
```

- [ ] **Step 2: Run unit tests**

```bash
go test ./internal/... -timeout 90s
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/backend/lsp/adapter.go internal/edit/snippet.go
git commit -m "feat(lsp): ExtractFunction/ExtractVariable/InlineSymbol via runCodeAction"
```

---

## Task 11: Inline `--call-site` flag

**Files:**
- Modify: `internal/cli/inline.go`

- [ ] **Step 1: Add flag registration**

Open `internal/cli/inline.go`. Locate the cobra `*cobra.Command` definition for `inline`. Add:

```go
var callSiteFlag string
// ... inside the command's flag setup:
cmd.Flags().StringVar(&callSiteFlag, "call-site", "",
	"When --symbol is used, specifies a call-site location file:line:column for inline")
```

- [ ] **Step 2: Implement the require-rule**

In the `RunE` handler, after `--symbol` is parsed:

```go
if symbolFlag != "" && callSiteFlag == "" {
	return fmt.Errorf("inline: --symbol requires --call-site <file>:<line>:<column> " +
		"to disambiguate which call site to inline")
}
var loc symbol.Location
if callSiteFlag != "" {
	parsed, err := parseCallSite(callSiteFlag)
	if err != nil {
		return fmt.Errorf("parse --call-site: %w", err)
	}
	loc = parsed
} else {
	loc = symbol.Location{File: fileFlag, Line: lineFlag, Column: columnFlag}
}
```

- [ ] **Step 3: Add `parseCallSite` helper**

At the bottom of `inline.go`:

```go
func parseCallSite(s string) (symbol.Location, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return symbol.Location{}, fmt.Errorf("expected file:line:column, got %q", s)
	}
	line, err := strconv.Atoi(parts[1])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("line must be an integer: %w", err)
	}
	col, err := strconv.Atoi(parts[2])
	if err != nil {
		return symbol.Location{}, fmt.Errorf("column must be an integer: %w", err)
	}
	return symbol.Location{File: parts[0], Line: line, Column: col}, nil
}
```

- [ ] **Step 4: Unit-test the parser**

Append to `internal/cli/inline_test.go` (create if absent):

```go
package cli

import "testing"

func TestParseCallSite(t *testing.T) {
	cases := []struct {
		in      string
		wantF   string
		wantL   int
		wantC   int
		wantErr bool
	}{
		{"src/main.rs:3:14", "src/main.rs", 3, 14, false},
		{"bad", "", 0, 0, true},
		{"src:not-a-number:3", "", 0, 0, true},
		{"src:3:not-a-number", "", 0, 0, true},
	}
	for _, c := range cases {
		got, err := parseCallSite(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("err=%v wantErr=%v for %q", err, c.wantErr, c.in)
			continue
		}
		if c.wantErr {
			continue
		}
		if got.File != c.wantF || got.Line != c.wantL || got.Column != c.wantC {
			t.Errorf("%q → %+v", c.in, got)
		}
	}
}
```

Run:

```bash
go test ./internal/cli/ -run TestParseCallSite -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/inline.go internal/cli/inline_test.go
git commit -m "feat(cli): inline --call-site flag; --symbol requires --call-site"
```

---

## Task 12: Support-matrix doc + help-text updates (H5, H6)

**Files:**
- Create: `docs/support-matrix.md`
- Modify: `README.md` (if exists; otherwise skip README change)
- Modify: `docs/specs/2026-04-15-refute-design.md` — add a pointer
- Modify: `internal/cli/rename.go`, `extract.go`, `inline.go` — help text

- [ ] **Step 1: Write support matrix**

Create `docs/support-matrix.md`:

```markdown
# refute Support Matrix

This table is the source of truth for which operations refute supports per language. Update it in the same commit as any feature that changes support.

| Language   | LSP Server                       | Workspace Marker                | Rename | Extract Fn | Extract Var | Inline     |
|------------|----------------------------------|---------------------------------|--------|------------|-------------|------------|
| Go         | gopls                            | `go.mod`, `go.work`             | ✅     | ✅         | ✅          | ✅         |
| Rust       | rust-analyzer                    | `Cargo.toml`                    | ✅     | ✅         | ✅          | ✅ (1)     |
| TypeScript | typescript-language-server       | `package.json`, `tsconfig.json` | ✅     | ❌         | ❌          | ❌         |
| JavaScript | typescript-language-server       | `package.json`                  | ✅     | ❌         | ❌          | ❌         |
| Python     | pyright                          | `pyproject.toml`, `setup.py`    | ⚠️     | ❌         | ❌          | ❌         |

**Legend:** ✅ supported · ⚠️ partial (LSP fallback, not tested end-to-end) · ❌ not supported.

(1) Rust `inline` operates on a single call site. Definition-wide inline (inline into all callers) is a planned follow-up — see `docs/plans/`.

## Tier-1 Qualified-Name Resolution

`--symbol` accepts qualified names that are resolved via `workspace/symbol`:

| Language   | Example                                      | Notes |
|------------|----------------------------------------------|-------|
| Go         | `pkg.FunctionName`, `Type.Method`            | See `2026-04-17-go-code-actions-tier1-v2.md` |
| Rust       | `greet::format_greeting`, `<Greeter as Display>::fmt` | Forms 1–7 per `2026-04-22-rust-parity-design.md` |

## Missing-Server Install Hints

| Language   | Install                                                  |
|------------|----------------------------------------------------------|
| Go         | `go install golang.org/x/tools/gopls@latest`             |
| Rust       | `rustup component add rust-analyzer`                     |
| TypeScript | `npm install -g typescript-language-server typescript`   |
| Python     | `pip install pyright`                                    |
```

- [ ] **Step 2: Reference matrix from existing design doc**

Edit `docs/specs/2026-04-15-refute-design.md`. Find the "Backend Selection" section (the existing table). Immediately after that table, add:

```markdown
Current support status is tracked in [`docs/support-matrix.md`](../support-matrix.md). That file is the source of truth; this design doc describes the target architecture.
```

- [ ] **Step 3: Update CLI help text**

For each of `internal/cli/rename.go`, `extract.go`, `inline.go`, find the `Long:` or `Short:` field of the command and ensure it mentions Rust. Example for `rename.go`:

```go
cmd := &cobra.Command{
	Use:   "rename-function",
	Short: "Rename a function across the workspace (Go, Rust, TypeScript)",
	Long:  `Rename a function at the given location. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server). See docs/support-matrix.md.`,
	// ...
}
```

Do the equivalent for `rename-class`, `rename-type`, `extract-function`, `extract-variable`, `inline`.

- [ ] **Step 4: Verify help text**

```bash
go build -o /tmp/refute ./cmd/refute
/tmp/refute --help | grep -i rust && echo OK || echo MISSING
/tmp/refute rename-function --help | grep -i rust && echo OK || echo MISSING
/tmp/refute extract-function --help | grep -i rust && echo OK || echo MISSING
/tmp/refute inline --help | grep -i rust && echo OK || echo MISSING
```

All four must print `OK`.

- [ ] **Step 5: Commit**

```bash
git add docs/support-matrix.md docs/specs/2026-04-15-refute-design.md internal/cli/
git commit -m "docs: support matrix + --language help text for Rust (H5, H6)"
```

---

## Task 13: Integration tests (batch 1/3) — rename hardening

**Files:**
- Modify: `internal/integration_test.go`

- [ ] **Step 1: Add local-variable rename test (H1)**

Append to `internal/integration_test.go` (after the existing Rust tests):

```go
func TestEndToEnd_RenameRustLocalVariable(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Local var `prefix` lives inside format_greeting() on lib.rs line 6
	// (1-indexed). Its declaration column is 9 (after "    let ").
	libFile := filepath.Join(dir, "src", "lib.rs")
	cmd := exec.Command(refuteBin,
		"rename-variable",
		"--file", libFile,
		"--line", "6",
		"--col", "9",
		"--name", "prefix",
		"--new-name", "salutation",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(libFile)
	if strings.Contains(string(content), "prefix") {
		t.Error("lib.rs still contains 'prefix'")
	}
	if !strings.Contains(string(content), "salutation") {
		t.Error("lib.rs missing 'salutation'")
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after local rename: %v", err)
	}
}

func runCargoBuild(t *testing.T, dir string) error {
	t.Helper()
	cmd := exec.Command("cargo", "build")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
```

`rename-variable` exists today (see `internal/cli/rename.go`). If the subcommand is absent at execution time, fall back to `rename-function` — rust-analyzer's rename is position-based and not affected by the command name.

- [ ] **Step 2: Add parameter rename test (H2)**

```go
func TestEndToEnd_RenameRustParameter(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	// Parameter `name` is on lib.rs line 5, column 24 (inside the
	// format_greeting signature).
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", libFile,
		"--line", "5",
		"--col", "24",
		"--name", "name",
		"--new-name", "greetee",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(libFile)
	if strings.Contains(string(content), " name: &str") {
		t.Error("lib.rs still has 'name' as parameter")
	}
	if !strings.Contains(string(content), " greetee: &str") {
		t.Error("lib.rs missing 'greetee' as parameter")
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after parameter rename: %v", err)
	}
}
```

- [ ] **Step 3: Run the two new tests**

```bash
go test -tags integration -run 'TestEndToEnd_RenameRust(Local|Parameter)' ./internal/... -timeout 300s -v
```

Expected: both pass.

If a column is off by one, adjust based on the exact fixture content (line numbers are from Task 1's lib.rs; count carefully).

- [ ] **Step 4: Commit**

```bash
git add internal/integration_test.go
git commit -m "test(rust): local-variable and parameter rename integration tests (H1, H2)"
```

---

## Task 14: Integration tests (batch 2/3) — code actions

**Files:**
- Modify: `internal/integration_test.go`

- [ ] **Step 1: Extract-function test**

```go
func TestEndToEnd_ExtractRustFunction(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	// compute() body is `(x * 2) + (x * 2)` at line 10. Extract the full
	// expression into a new function `double_plus_double`.
	cmd := exec.Command(refuteBin,
		"extract-function",
		"--file", libFile,
		"--start-line", "10",
		"--start-column", "5",
		"--end-line", "10",
		"--end-column", "26",
		"--name", "double_plus_double",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(libFile)
	if !strings.Contains(string(content), "fn double_plus_double") {
		t.Errorf("expected new fn double_plus_double, got:\n%s", content)
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after extract-function: %v", err)
	}
}
```

- [ ] **Step 2: Extract-variable test**

```go
func TestEndToEnd_ExtractRustVariable(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	cmd := exec.Command(refuteBin,
		"extract-variable",
		"--file", libFile,
		"--start-line", "10",
		"--start-column", "6",
		"--end-line", "10",
		"--end-column", "13",
		"--name", "doubled",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(libFile)
	if !strings.Contains(string(content), "let doubled") {
		t.Errorf("expected `let doubled`, got:\n%s", content)
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after extract-variable: %v", err)
	}
}
```

- [ ] **Step 3: Inline call-site test (I2)**

```go
func TestEndToEnd_InlineRustCallSite(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	mainFile := filepath.Join(dir, "src", "main.rs")

	// Call site: main.rs line 5 `greet::util::sum(1, 2)`. Column points at
	// the 's' in `sum`.
	cmd := exec.Command(refuteBin,
		"inline",
		"--file", mainFile,
		"--line", "5",
		"--col", "25",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(mainFile)
	if strings.Contains(string(content), "sum(1, 2)") {
		t.Error("main.rs still contains sum(1, 2) after inline")
	}
	if !strings.Contains(string(content), "1 + 2") {
		t.Errorf("main.rs missing inlined body, got:\n%s", content)
	}
	// util.rs should still define sum; I2 does not delete the definition.
	utilFile := filepath.Join(dir, "src", "util.rs")
	util, _ := os.ReadFile(utilFile)
	if !strings.Contains(string(util), "pub fn sum") {
		t.Error("util.rs lost sum definition; I2 should preserve it")
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after inline: %v", err)
	}
}
```

- [ ] **Step 4: Inline requires --call-site**

```go
func TestEndToEnd_InlineRustRequiresCallSite(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	cmd := exec.Command(refuteBin,
		"inline",
		"--symbol", "greet::util::sum",
		"--language", "rust",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error, got success: %s", out)
	}
	if !strings.Contains(string(out), "--call-site") {
		t.Errorf("error should mention --call-site, got:\n%s", out)
	}
}
```

- [ ] **Step 5: Snippet-stripping test (H4)**

```go
func TestEndToEnd_RustSnippetPlaceholderStripped(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	cmd := exec.Command(refuteBin,
		"extract-variable",
		"--file", libFile,
		"--start-line", "10",
		"--start-column", "6",
		"--end-line", "10",
		"--end-column", "13",
		"--name", "doubled",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	content, _ := os.ReadFile(libFile)
	for _, token := range []string{"$0", "${0", "${1", "$1"} {
		if strings.Contains(string(content), token) {
			t.Errorf("file contains snippet token %q after extract:\n%s", token, content)
		}
	}
}
```

- [ ] **Step 6: Run batch 2**

```bash
go test -tags integration -run 'TestEndToEnd_(Extract|Inline)Rust|TestEndToEnd_RustSnippetPlaceholderStripped' ./internal/... -timeout 600s -v
```

Expected: all pass.

- [ ] **Step 7: Commit**

```bash
git add internal/integration_test.go
git commit -m "test(rust): code-action integration tests (extract, inline, snippet)"
```

---

## Task 15: Integration tests (batch 3/3) — Tier-1 + missing server

**Files:**
- Modify: `internal/integration_test.go`

- [ ] **Step 1: Tier-1 rename test**

```go
func TestEndToEnd_Tier1RustRename(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "greet::util::sum",
		"--language", "rust",
		"--new-name", "add",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
	utilFile := filepath.Join(dir, "src", "util.rs")
	content, _ := os.ReadFile(utilFile)
	if strings.Contains(string(content), "pub fn sum") {
		t.Error("util.rs still defines sum")
	}
	if !strings.Contains(string(content), "pub fn add") {
		t.Error("util.rs missing pub fn add")
	}
	mainFile := filepath.Join(dir, "src", "main.rs")
	main, _ := os.ReadFile(mainFile)
	if !strings.Contains(string(main), "greet::util::add(1, 2)") {
		t.Errorf("main.rs call site not updated: %s", main)
	}
}
```

- [ ] **Step 2: Trait-qualified test (form 6)**

```go
func TestEndToEnd_Tier1RustTraitQualified(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Rename only the Display impl's fmt, not the Debug impl's fmt.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "<Greeter as Display>::fmt",
		"--language", "rust",
		"--new-name", "render",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Expect: either success, OR rust-analyzer rejects trait-method rename
		// (some rust-analyzer versions refuse to rename trait-required methods
		// because the signature is trait-constrained). Accept both outcomes
		// but fail on any other error.
		if !strings.Contains(string(out), "cannot rename") &&
			!strings.Contains(string(out), "trait") {
			t.Fatalf("unexpected failure: %s\n%s", err, out)
		}
		t.Skipf("rust-analyzer refused trait-method rename: %s", out)
	}
	libFile := filepath.Join(dir, "src", "lib.rs")
	content, _ := os.ReadFile(libFile)
	// Exactly one of the two fmt functions should be renamed — but because
	// fmt is part of the Display trait, rust-analyzer may reject. If the
	// rename went through, verify only one fmt became render.
	renderCount := strings.Count(string(content), "fn render")
	fmtCount := strings.Count(string(content), "fn fmt")
	if renderCount != 1 || fmtCount != 1 {
		t.Errorf("expected 1 render and 1 fmt, got render=%d fmt=%d\n%s",
			renderCount, fmtCount, content)
	}
}
```

This test documents expected behavior — it may Skip on rust-analyzer versions that refuse to rename trait-constrained methods. The point is to verify *our* disambiguation routes correctly; if rust-analyzer then rejects the rename itself, that is not our bug.

- [ ] **Step 3: Ambiguous-symbol test**

```go
func TestEndToEnd_Tier1RustAmbiguous(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "fmt",
		"--language", "rust",
		"--new-name", "render",
		"--json",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for ambiguous symbol, got success")
	}
	if !strings.Contains(string(out), `"status": "ambiguous"`) &&
		!strings.Contains(string(out), `"status":"ambiguous"`) {
		t.Errorf("expected ambiguous status in JSON output, got:\n%s", out)
	}
	if !strings.Contains(string(out), `"candidates"`) {
		t.Errorf("expected candidates array, got:\n%s", out)
	}
}
```

- [ ] **Step 4: Not-found test**

```go
func TestEndToEnd_Tier1RustNotFound(t *testing.T) {
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "nonexistent_symbol_xyz",
		"--language", "rust",
		"--new-name", "whatever",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for missing symbol")
	}
	if !strings.Contains(string(out), "no rust symbol matched") &&
		!strings.Contains(string(out), "nonexistent_symbol_xyz") {
		t.Errorf("error message should mention the input, got:\n%s", out)
	}
}
```

- [ ] **Step 5: Missing rust-analyzer test (H3)**

```go
func TestEndToEnd_RustAnalyzerMissing(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", libFile,
		"--line", "5",
		"--col", "8",
		"--name", "format_greeting",
		"--new-name", "build_greeting",
	)
	cmd.Dir = dir
	// Scrub PATH: keep only /usr/bin and /bin so rust-analyzer is unreachable.
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin")

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when rust-analyzer is absent, got success: %s", out)
	}
	if !strings.Contains(string(out), "rust-analyzer") {
		t.Errorf("error should mention rust-analyzer, got:\n%s", out)
	}
	if !strings.Contains(string(out), "rustup component add rust-analyzer") {
		t.Errorf("error should include install hint, got:\n%s", out)
	}
}
```

If CI runs in an environment where `rust-analyzer` is in `/usr/bin`, this test will mis-fire. Guard it:

```go
// Skip if rust-analyzer is in a bin dir we can't scrub.
if path, _ := exec.LookPath("rust-analyzer"); strings.HasPrefix(path, "/usr/bin") || strings.HasPrefix(path, "/bin") {
	t.Skip("rust-analyzer installed in non-scrubbable location; install hint test cannot run")
}
```

Place this check at the top of the test, after `copyDir`.

- [ ] **Step 6: Run batch 3**

```bash
go test -tags integration -run 'TestEndToEnd_Tier1Rust|TestEndToEnd_RustAnalyzerMissing' ./internal/... -timeout 600s -v
```

Expected: all pass or skip cleanly per their guards.

- [ ] **Step 7: Run the whole Rust suite**

```bash
go test -tags integration -run 'TestEndToEnd.*Rust' ./internal/... -timeout 900s -v
```

Expected: all 15 Rust integration tests pass or skip cleanly.

- [ ] **Step 8: Commit**

```bash
git add internal/integration_test.go
git commit -m "test(rust): Tier-1 rename and missing-server integration tests"
```

---

## Task 16: Full regression run

- [ ] **Step 1: Unit tests**

```bash
go test ./... -timeout 90s
```

Expected: all pass.

- [ ] **Step 2: Integration tests**

```bash
go test -tags integration ./internal/... -timeout 900s -v
```

Expected: all pass, or skip cleanly where required binaries are missing. No FAIL.

- [ ] **Step 3: Verify help text one more time**

```bash
go build -o /tmp/refute ./cmd/refute
/tmp/refute rename-function --help
/tmp/refute extract-function --help
/tmp/refute inline --help
```

Read the output; confirm Rust is mentioned in each.

- [ ] **Step 4: Update log — final status**

Append to `docs/expeditions/rust-parity/log.md`:

```markdown
### YYYY-MM-DD — Final regression (Agent <label>)

**Completed:** Task 16; expedition complete.
**Test status:** go test ./... → pass. integration → pass/skip.
**Notes:** Rust parity v1 merged to main.
```

Update the Status Dashboard: set all tasks to `done`.

- [ ] **Step 5: Commit**

```bash
git add docs/expeditions/rust-parity/log.md
git commit -m "docs: close rust-parity expedition"
```

- [ ] **Step 6: Merge to main**

```bash
git checkout main
git merge feature/rust-parity --no-ff
git push origin main
```

Clean up the worktree per `bento:closure` skill.

---

## Self-Review Checklist

Spec coverage:

- ✅ Task 0: empirical spike → spec Risk section
- ✅ Task 1: fixture extension → spec File Structure (testdata)
- ✅ Task 2: ErrLSPServerMissing → spec H3
- ✅ Task 3: snippet stripper → spec H4
- ✅ Task 4: PrimeRustWorkspace → spec Architecture
- ✅ Task 5: rust_actions.go → spec Code-Action Matching
- ✅ Task 6: rust_container.go (cheap+expensive branches) → spec Risk + Architecture
- ✅ Task 7: adapter dispatch → spec Architecture
- ✅ Task 8: ParseRustQualifiedName (forms 1-7) → spec Qualified-Name Syntax
- ✅ Task 9: CLI rename dispatch → spec Architecture
- ✅ Task 10: extract/inline adapter methods → spec Architecture + Code-Action Matching
- ✅ Task 11: inline --call-site flag → spec Inline Semantics (I2)
- ✅ Task 12: support-matrix + help text → spec H5, H6
- ✅ Task 13: rename hardening tests → spec H1, H2
- ✅ Task 14: code-action tests → spec Integration Tests
- ✅ Task 15: Tier-1 + missing-server tests → spec Integration Tests
- ✅ Task 16: regression + merge → expedition posture

---

## Expedition Docs Cleanup

These expedition-local docs are transient and MUST NOT land on `main`:

- `docs/expeditions/rust-parity/plan.md` (this file)
- `docs/expeditions/rust-parity/log.md`
- `docs/expeditions/rust-parity/handoff.md`
- `docs/expeditions/rust-parity/state.json`

Before the final landing of the `rust-parity` base branch onto `main`, run:

```bash
expedition/scripts/expedition.py finish --expedition rust-parity --apply
```

and commit the resulting deletion on the rebased base branch. The spec
(`docs/specs/2026-04-22-rust-parity-design.md`) is kept; only the expedition-
local `docs/expeditions/rust-parity/` tree is removed.
