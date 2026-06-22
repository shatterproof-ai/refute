//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameJavaMethod(t *testing.T) {
	requireExperimentalIntegration(t, "Java")
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
