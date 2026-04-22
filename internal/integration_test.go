//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameGoFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	// Copy fixture to temp dir so we don't modify the original.
	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Run rename: FormatGreeting → BuildGreeting.
	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	// Verify: old name should be gone from all Go files.
	helperContent, _ := os.ReadFile(helperFile)
	if strings.Contains(string(helperContent), "FormatGreeting") {
		t.Error("helper.go still contains FormatGreeting")
	}
	if !strings.Contains(string(helperContent), "BuildGreeting") {
		t.Error("helper.go missing BuildGreeting")
	}

	mainFile := filepath.Join(dir, "main.go")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "FormatGreeting") {
		t.Error("main.go still contains FormatGreeting after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "BuildGreeting") {
		t.Error("main.go missing BuildGreeting")
	}

	// Verify: project still compiles.
	goCheck := exec.Command("go", "build", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}

func TestEndToEnd_DryRun(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	helperFile := filepath.Join(dir, "util", "helper.go")

	// Read original content.
	originalContent, _ := os.ReadFile(helperFile)

	// Run with --dry-run.
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute dry-run failed: %s\n%s", err, out)
	}

	// Verify: diff output should contain both old and new names.
	if !strings.Contains(string(out), "FormatGreeting") || !strings.Contains(string(out), "BuildGreeting") {
		t.Errorf("dry-run output should show diff, got:\n%s", out)
	}

	// Verify: file should be unchanged.
	afterContent, _ := os.ReadFile(helperFile)
	if string(afterContent) != string(originalContent) {
		t.Error("dry-run should not modify files")
	}
}

func TestEndToEnd_RenameTypeScriptFunction(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// Rename greet → welcome.
	greeterFile := filepath.Join(dir, "src", "greeter.ts")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", greeterFile,
		"--line", "1",
		"--name", "greet",
		"--new-name", "welcome",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	greeterContent, _ := os.ReadFile(greeterFile)
	if strings.Contains(string(greeterContent), "greet") {
		t.Error("greeter.ts still contains old name 'greet'")
	}
	if !strings.Contains(string(greeterContent), "welcome") {
		t.Error("greeter.ts missing new name 'welcome'")
	}

	// Verify cross-file rename: main.ts imports and calls greet.
	mainFile := filepath.Join(dir, "src", "main.ts")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "greet") {
		t.Error("main.ts still contains old name 'greet' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "welcome") {
		t.Error("main.ts missing 'welcome' after cross-file rename")
	}
}

func TestEndToEnd_TypeScriptDryRun(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("typescript-language-server not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	greeterFile := filepath.Join(dir, "src", "greeter.ts")
	originalContent, _ := os.ReadFile(greeterFile)

	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", greeterFile,
		"--line", "1",
		"--name", "greet",
		"--new-name", "welcome",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute dry-run failed: %s\n%s", err, out)
	}

	if !strings.Contains(string(out), "greet") || !strings.Contains(string(out), "welcome") {
		t.Errorf("dry-run output should show diff, got:\n%s", out)
	}

	afterContent, _ := os.ReadFile(greeterFile)
	if string(afterContent) != string(originalContent) {
		t.Error("dry-run should not modify files")
	}
}

func TestEndToEnd_RenameRustFunction(t *testing.T) {
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
		"--line", "1",
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
		t.Error("dry-run should not modify files")
	}
}

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

// copyDir recursively copies a directory tree.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			os.MkdirAll(dstPath, 0755)
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("reading %s: %v", srcPath, err)
			}
			os.WriteFile(dstPath, data, 0644)
		}
	}
}
