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

// TestEndToEnd_RenameGoClassRejectsFunction covers the headline acceptance check
// of issue #88: rename-class on a Go function must exit nonzero, name the actual
// kind (function), note that Go has no classes, suggest `rename`/`rename-function`,
// and leave the file untouched.
func TestEndToEnd_RenameGoClassRejectsFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	refuteBin := buildRefute(t)

	helperFile := filepath.Join(dir, "util", "helper.go")
	original, _ := os.ReadFile(helperFile)

	cmd := exec.Command(refuteBin,
		"rename-class",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for rename-class on a function, got success; output:\n%s", out)
	}
	msg := string(out)
	for _, want := range []string{"function", "go has no class", "rename-function"} {
		if !strings.Contains(msg, want) {
			t.Errorf("kind-mismatch message missing %q; got:\n%s", want, msg)
		}
	}

	// The file must be untouched: validation happens before any edit.
	after, _ := os.ReadFile(helperFile)
	if string(after) != string(original) {
		t.Error("rename-class rejection should not modify the file")
	}
}

// TestEndToEnd_RenameGoMethodRejectsFunction covers an in-language kind mismatch
// (method vs function — both are valid Go kinds) to show the variant validates
// the actual kind, not only language applicability.
func TestEndToEnd_RenameGoMethodRejectsFunction(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	refuteBin := buildRefute(t)

	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-method",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for rename-method on a function, got success; output:\n%s", out)
	}
	msg := string(out)
	if !strings.Contains(msg, "function") || !strings.Contains(msg, "not a method") {
		t.Errorf("expected 'function ... not a method' message; got:\n%s", msg)
	}
}

// TestEndToEnd_RenameGoKindCorrect confirms a kind-correct variant (rename-field
// on a struct field) behaves like plain rename and applies the edit.
func TestEndToEnd_RenameGoKindCorrect(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	refuteBin := buildRefute(t)

	userFile := filepath.Join(dir, "util", "user.go")
	cmd := exec.Command(refuteBin,
		"rename-field",
		"--file", userFile,
		"--line", "5",
		"--name", "Name",
		"--new-name", "FullName",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rename-field on a struct field should succeed: %s\n%s", err, out)
	}
	userContent, _ := os.ReadFile(userFile)
	if strings.Contains(string(userContent), "\tName string") {
		t.Error("user.go still contains the old field declaration 'Name string'")
	}
	if !strings.Contains(string(userContent), "FullName string") {
		t.Error("user.go missing renamed field 'FullName string'")
	}
}

// TestEndToEnd_RenameKindMismatchJSON pins the --json contract for a kind
// mismatch: status kind-mismatch, a structured error, and exit code 1 (matching
// human mode).
func TestEndToEnd_RenameKindMismatchJSON(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}

	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	refuteBin := buildRefute(t)

	helperFile := filepath.Join(dir, "util", "helper.go")
	cmd := exec.Command(refuteBin,
		"rename-class",
		"--json",
		"--file", helperFile,
		"--line", "4",
		"--name", "FormatGreeting",
		"--new-name", "BuildGreeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected *exec.ExitError, got %v; output:\n%s", err, out)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", exitErr.ExitCode(), out)
	}

	var result struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("unmarshal JSON: %v\nraw:\n%s", err, out)
	}
	if result.Status != "kind-mismatch" {
		t.Errorf("status = %q, want kind-mismatch", result.Status)
	}
	if result.Error.Code != "kind-mismatch" {
		t.Errorf("error.code = %q, want kind-mismatch", result.Error.Code)
	}
	if !strings.Contains(result.Error.Message, "function") {
		t.Errorf("error.message %q should name the actual kind", result.Error.Message)
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
