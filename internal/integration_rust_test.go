//go:build integration

package internal_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameRustFunction(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}

	// Copy fixture to temp dir so we don't modify the original.
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Run rename: format_greeting → build_greeting.
	libFile := filepath.Join(dir, "src", "lib.rs")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", libFile,
		"--line", "5",
		"--name", "format_greeting",
		"--new-name", "build_greeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// Verify lib.rs was updated.
	libContent, _ := os.ReadFile(libFile)
	if strings.Contains(string(libContent), "format_greeting") {
		t.Error("lib.rs still contains format_greeting")
	}
	if !strings.Contains(string(libContent), "build_greeting") {
		t.Error("lib.rs missing build_greeting")
	}

	// Verify main.rs cross-file reference was updated.
	mainFile := filepath.Join(dir, "src", "main.rs")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "format_greeting") {
		t.Error("main.rs still contains format_greeting after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "build_greeting") {
		t.Error("main.rs missing build_greeting")
	}

	// Verify: project still compiles.
	cargoCheck := exec.Command("cargo", "build")
	cargoCheck.Dir = dir
	if out, err := cargoCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}

func TestEndToEnd_RustDryRun(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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
		"--line", "5",
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
		t.Error("dry-run should not modify files")
	}
}

func TestEndToEnd_RenameRustStruct(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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
		"--line", "14",
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

func TestEndToEnd_RenameRustLocalVariable(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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

func TestEndToEnd_RenameRustParameter(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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
	if strings.Contains(string(content), "name: &str") {
		t.Error("lib.rs still has 'name' as parameter")
	}
	if !strings.Contains(string(content), "greetee: &str") {
		t.Error("lib.rs missing 'greetee' as parameter")
	}
	if err := runCargoBuild(t, dir); err != nil {
		t.Fatalf("cargo build failed after parameter rename: %v", err)
	}
}

func TestEndToEnd_ExtractRustFunction(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	// compute() body `(x * 2) + (x * 2)` is on lib.rs line 11 (1-indexed).
	// Start col 5 (`(`), end col 22 (one past closing `)`).
	cmd := exec.Command(refuteBin,
		"extract-function",
		"--file", libFile,
		"--start-line", "11",
		"--start-col", "5",
		"--end-line", "11",
		"--end-col", "22",
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

func TestEndToEnd_ExtractRustVariable(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	// Extract `x * 2` (first occurrence) on line 11, cols 6-11.
	cmd := exec.Command(refuteBin,
		"extract-variable",
		"--file", libFile,
		"--start-line", "11",
		"--start-col", "6",
		"--end-line", "11",
		"--end-col", "11",
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

func TestEndToEnd_InlineRustCallSite(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	mainFile := filepath.Join(dir, "src", "main.rs")

	// Call site: main.rs line 5 `greet::util::sum(1, 2)`. Column 30 points at
	// the 's' in `sum`.
	cmd := exec.Command(refuteBin,
		"inline",
		"--file", mainFile,
		"--line", "5",
		"--col", "30",
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

func TestEndToEnd_InlineRustRequiresCallSite(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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

func TestEndToEnd_RustSnippetPlaceholderStripped(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
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
		"--start-line", "11",
		"--start-col", "6",
		"--end-line", "11",
		"--end-col", "11",
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

func TestEndToEnd_Tier1RustRename(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	utilFile := filepath.Join(dir, "src", "util.rs")

	// --file provides the language hint (rust-analyzer) for Tier-1 symbol rename.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "greet::util::sum",
		"--file", utilFile,
		"--new-name", "add",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}
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

func TestEndToEnd_Tier1RustTraitQualified(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	// Rename only the Display impl's fmt, not the Debug impl's fmt.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "<Greeter as Display>::fmt",
		"--file", libFile,
		"--new-name", "render",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Some rust-analyzer versions refuse to rename trait-required methods.
		if !strings.Contains(string(out), "cannot rename") &&
			!strings.Contains(string(out), "trait") {
			t.Fatalf("unexpected failure: %s\n%s", err, out)
		}
		t.Skipf("rust-analyzer refused trait-method rename: %s", out)
	}
	content, _ := os.ReadFile(libFile)
	renderCount := strings.Count(string(content), "fn render")
	fmtCount := strings.Count(string(content), "fn fmt")
	if renderCount != 1 || fmtCount != 1 {
		t.Errorf("expected 1 render and 1 fmt, got render=%d fmt=%d\n%s",
			renderCount, fmtCount, content)
	}
}

func TestEndToEnd_Tier1RustAmbiguous(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "fmt",
		"--file", libFile,
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

func TestEndToEnd_Tier1RustNotFound(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	if _, err := exec.LookPath("rust-analyzer"); err != nil {
		t.Skip("rust-analyzer not found on PATH")
	}
	srcDir := filepath.Join("../testdata/fixtures/rust/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	libFile := filepath.Join(dir, "src", "lib.rs")

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "nonexistent_symbol_xyz",
		"--file", libFile,
		"--new-name", "whatever",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for missing symbol")
	}
	if !strings.Contains(string(out), "no rust symbol matched") &&
		!strings.Contains(string(out), "nonexistent_symbol_xyz") &&
		!strings.Contains(string(out), "symbol not found") {
		t.Errorf("error message should indicate symbol not found, got:\n%s", out)
	}
}

func TestEndToEnd_RustAnalyzerMissing(t *testing.T) {
	requireExperimentalIntegration(t, "Rust")
	// Skip if rust-analyzer is in a bin dir we can't scrub.
	if path, _ := exec.LookPath("rust-analyzer"); strings.HasPrefix(path, "/usr/bin") || strings.HasPrefix(path, "/bin") {
		t.Skip("rust-analyzer installed in non-scrubbable location; install hint test cannot run")
	}

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
