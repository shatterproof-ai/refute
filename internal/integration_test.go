//go:build integration

package internal_test

import (
	"encoding/json"
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
	goCheck := exec.Command("go", "build", "-buildvcs=false", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}

func TestEndToEnd_RenameGoAfterNonASCIIText(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/unicode\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainFile := filepath.Join(dir, "main.go")
	src := `package main

func oldName() {}

func main() { println("é𝄞"); oldName() }
`
	if err := os.WriteFile(mainFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", mainFile,
		"--line", "5",
		"--name", "oldName",
		"--new-name", "newName",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed after non-ASCII text: %s\n%s", err, out)
	}

	content, _ := os.ReadFile(mainFile)
	if strings.Contains(string(content), "oldName") {
		t.Error("main.go still contains oldName")
	}
	if !strings.Contains(string(content), "newName") {
		t.Error("main.go missing newName")
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
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

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
	if strings.Contains(string(mainContent), "import { greet }") || strings.Contains(string(mainContent), "greet(\"world\")") {
		t.Error("main.ts still contains old function reference 'greet' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "import { welcome }") || !strings.Contains(string(mainContent), "welcome(\"world\")") {
		t.Error("main.ts missing 'welcome' after cross-file rename")
	}
}

func TestEndToEnd_TypeScriptDryRun(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

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

func TestEndToEnd_RenameTypeScriptClass(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

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

	personContent, _ := os.ReadFile(personFile)
	if strings.Contains(string(personContent), "class Person") {
		t.Error("person.ts still contains 'class Person'")
	}
	if !strings.Contains(string(personContent), "class Individual") {
		t.Error("person.ts missing 'class Individual'")
	}

	mainFile := filepath.Join(dir, "src", "main.ts")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "Person") {
		t.Error("main.ts still contains 'Person' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "Individual") {
		t.Error("main.ts missing 'Individual' after cross-file rename")
	}
}

func TestEndToEnd_RenameTypeScriptInterface(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	typesFile := filepath.Join(dir, "src", "types.ts")
	cmd := exec.Command(refuteBin,
		"rename-type",
		"--file", typesFile,
		"--line", "1",
		"--name", "NamedThing",
		"--new-name", "LabeledThing",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	typesContent, _ := os.ReadFile(typesFile)
	if strings.Contains(string(typesContent), "NamedThing") {
		t.Error("types.ts still contains 'NamedThing'")
	}
	if !strings.Contains(string(typesContent), "LabeledThing") {
		t.Error("types.ts missing 'LabeledThing'")
	}

	mainFile := filepath.Join(dir, "src", "main.ts")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "NamedThing") {
		t.Error("main.ts still contains 'NamedThing' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "LabeledThing") {
		t.Error("main.ts missing 'LabeledThing' after cross-file rename")
	}
}

func TestEndToEnd_RenameTypeScriptLocalVariable(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	localFile := filepath.Join(dir, "src", "local.ts")
	cmd := exec.Command(refuteBin,
		"rename-variable",
		"--file", localFile,
		"--line", "2",
		"--name", "totalCount",
		"--new-name", "itemTotal",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	localContent, _ := os.ReadFile(localFile)
	if strings.Contains(string(localContent), "totalCount") {
		t.Error("local.ts still contains 'totalCount'")
	}
	if !strings.Contains(string(localContent), "itemTotal") {
		t.Error("local.ts missing 'itemTotal'")
	}
}

func TestEndToEnd_RenameTypeScriptTSXComponent(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	badgeFile := filepath.Join(dir, "src", "badge.tsx")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", badgeFile,
		"--line", "5",
		"--name", "Badge",
		"--new-name", "StatusBadge",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	badgeContent, _ := os.ReadFile(badgeFile)
	if strings.Contains(string(badgeContent), "function Badge") {
		t.Error("badge.tsx still contains 'function Badge'")
	}
	if !strings.Contains(string(badgeContent), "function StatusBadge") {
		t.Error("badge.tsx missing 'function StatusBadge'")
	}

	dashboardFile := filepath.Join(dir, "src", "dashboard.tsx")
	dashboardContent, _ := os.ReadFile(dashboardFile)
	if strings.Contains(string(dashboardContent), "import { Badge }") || strings.Contains(string(dashboardContent), "<Badge ") {
		t.Error("dashboard.tsx still contains old component reference 'Badge'")
	}
	if !strings.Contains(string(dashboardContent), "import { StatusBadge }") || !strings.Contains(string(dashboardContent), "<StatusBadge ") {
		t.Error("dashboard.tsx missing 'StatusBadge' after cross-file rename")
	}
}

func TestEndToEnd_RenameJavaScriptFunction(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/javascript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	mathFile := filepath.Join(dir, "src", "math.js")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", mathFile,
		"--line", "1",
		"--name", "sum",
		"--new-name", "add",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	mathContent, _ := os.ReadFile(mathFile)
	if strings.Contains(string(mathContent), "sum") {
		t.Error("math.js still contains 'sum'")
	}
	if !strings.Contains(string(mathContent), "add") {
		t.Error("math.js missing 'add'")
	}

	mainFile := filepath.Join(dir, "src", "main.js")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), "sum") {
		t.Error("main.js still contains 'sum' after cross-file rename")
	}
	if !strings.Contains(string(mainContent), "add") {
		t.Error("main.js missing 'add' after cross-file rename")
	}
}

func TestEndToEnd_RenameJavaScriptJSXComponent(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/javascript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	buttonFile := filepath.Join(dir, "src", "button.jsx")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", buttonFile,
		"--line", "1",
		"--name", "Button",
		"--new-name", "ActionButton",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	buttonContent, _ := os.ReadFile(buttonFile)
	if strings.Contains(string(buttonContent), "function Button") {
		t.Error("button.jsx still contains 'function Button'")
	}
	if !strings.Contains(string(buttonContent), "function ActionButton") {
		t.Error("button.jsx missing 'function ActionButton'")
	}

	screenFile := filepath.Join(dir, "src", "screen.jsx")
	screenContent, _ := os.ReadFile(screenFile)
	if strings.Contains(string(screenContent), "import { Button }") || strings.Contains(string(screenContent), "<Button ") {
		t.Error("screen.jsx still contains old component reference 'Button'")
	}
	if !strings.Contains(string(screenContent), "import { ActionButton }") || !strings.Contains(string(screenContent), "<ActionButton ") {
		t.Error("screen.jsx missing 'ActionButton' after cross-file rename")
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

func TestEndToEnd_RenameJavaMethod(t *testing.T) {
	if _, err := exec.LookPath("jdtls"); err != nil {
		t.Skip("jdtls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/java/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)

	// greet() is declared on line 4 of Greeter.java (1-indexed).
	greeterFile := filepath.Join(dir, "src", "main", "java", "com", "example", "Greeter.java")
	cmd := exec.Command(refuteBin,
		"rename-method",
		"--file", greeterFile,
		"--line", "4",
		"--name", "greet",
		"--new-name", "hello",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	greeterContent, _ := os.ReadFile(greeterFile)
	if strings.Contains(string(greeterContent), "greet(") {
		t.Error("Greeter.java still contains greet()")
	}
	if !strings.Contains(string(greeterContent), "hello(") {
		t.Error("Greeter.java missing hello(")
	}

	mainFile := filepath.Join(dir, "src", "main", "java", "com", "example", "Main.java")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), ".greet(") {
		t.Error("Main.java still contains .greet() after cross-file rename")
	}
	if !strings.Contains(string(mainContent), ".hello(") {
		t.Error("Main.java missing .hello(")
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
	goCheck := exec.Command("go", "build", "-buildvcs=false", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project no longer compiles after rename:\n%s", out)
	}
}

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

// buildRefute compiles the refute binary into a temp dir and returns its path.
func buildRefute(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/refute")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build refute: %v\n%s", err, out)
	}
	return bin
}

func requireFixtureTypeScriptLanguageServer(t *testing.T, fixtureDir string) string {
	t.Helper()
	server := filepath.Join(fixtureDir, "node_modules", ".bin", "typescript-language-server")
	if _, err := os.Stat(server); err != nil {
		t.Skipf("local typescript-language-server not found at %s; run npm install in %s", server, fixtureDir)
	}
	abs, err := filepath.Abs(server)
	if err != nil {
		t.Fatalf("resolve typescript-language-server path: %v", err)
	}
	return abs
}

func linkFixtureNodeModules(t *testing.T, fixtureDir string, workspaceDir string) {
	t.Helper()
	target, err := filepath.Abs(filepath.Join(fixtureDir, "node_modules"))
	if err != nil {
		t.Fatalf("resolve fixture node_modules path: %v", err)
	}
	link := filepath.Join(workspaceDir, "node_modules")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link fixture node_modules: %v", err)
	}
}

// copyDir recursively copies a directory tree.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == "node_modules" {
			continue
		}
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

func TestEndToEnd_ExtractFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	mainFile := filepath.Join(dir, "main.go")

	// Line 7: "\tresult := 6*7 + 1"
	// gopls only offers refactor.extract.function on a full statement,
	// so the range covers "result := 6*7 + 1" (col 2 after the tab through col 19).
	cmd := exec.Command(refuteBin,
		"extract-function",
		"--file", mainFile,
		"--start-line", "7",
		"--start-col", "2",
		"--end-line", "7",
		"--end-col", "19",
		"--name", "computeResult",
	)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("extract-function: %s\n%s", err, out)
	}

	goCheck := exec.Command("go", "build", "-buildvcs=false", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after extract:\n%s", out)
	}

	mainContent, _ := os.ReadFile(mainFile)
	if !strings.Contains(string(mainContent), "computeResult") {
		t.Errorf("expected 'computeResult' in main.go after extract, got:\n%s", mainContent)
	}
}

func TestEndToEnd_Tier1Rename(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tier 1 rename: %s\n%s", err, out)
	}

	helperContent, _ := os.ReadFile(filepath.Join(dir, "util", "helper.go"))
	if strings.Contains(string(helperContent), "FormatGreeting") {
		t.Error("helper.go still contains FormatGreeting")
	}
	mainContent, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if strings.Contains(string(mainContent), "FormatGreeting") {
		t.Error("main.go still contains FormatGreeting")
	}

	goCheck := exec.Command("go", "build", "-buildvcs=false", "./...")
	goCheck.Dir = dir
	if out, err := goCheck.CombinedOutput(); err != nil {
		t.Fatalf("project does not compile after rename:\n%s", out)
	}
}

func TestEndToEnd_Tier1NotFound(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--symbol", "DoesNotExistAnywhere",
		"--new-name", "StillDoesNotExist",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for nonexistent symbol, got success:\n%s", out)
	}
	if !strings.Contains(string(out), "symbol not found") {
		t.Errorf("expected 'symbol not found' in output, got:\n%s", out)
	}
}

func TestEndToEnd_JSONOutput(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
	srcDir := "../testdata/fixtures/go/rename"
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	refuteBin := buildRefute(t)
	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-function",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--json",
		"--dry-run",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rename --json --dry-run: %s\n%s", err, out)
	}
	var parsed struct {
		SchemaVersion string `json:"schemaVersion"`
		Status        string `json:"status"`
		Operation     string `json:"operation"`
		Language      string `json:"language"`
		Backend       string `json:"backend"`
		WorkspaceRoot string `json:"workspaceRoot"`
		FilesModified int    `json:"filesModified"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("parsing JSON: %v\nraw:\n%s", err, out)
	}
	if parsed.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want \"1\"", parsed.SchemaVersion)
	}
	if parsed.Status != "dry-run" {
		t.Errorf("status = %q, want dry-run", parsed.Status)
	}
	if parsed.Operation != "rename" {
		t.Errorf("operation = %q, want rename", parsed.Operation)
	}
	if parsed.Language != "go" {
		t.Errorf("language = %q, want go", parsed.Language)
	}
	if parsed.Backend != "lsp" {
		t.Errorf("backend = %q, want lsp", parsed.Backend)
	}
	if parsed.WorkspaceRoot == "" {
		t.Error("workspaceRoot must not be empty")
	}
	if parsed.FilesModified < 2 {
		t.Errorf("filesModified = %d, want >= 2 (helper.go + main.go)", parsed.FilesModified)
	}
}
