//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameJavaScriptFunction(t *testing.T) {
	requireExperimentalIntegration(t, "JavaScript")
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

func TestEndToEnd_RenameJavaScriptFunctionBySymbol(t *testing.T) {
	requireExperimentalIntegration(t, "JavaScript")
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
		"--symbol", "sum",
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
		t.Error("main.js still contains 'sum' after Tier 1 rename")
	}
	if !strings.Contains(string(mainContent), "add") {
		t.Error("main.js missing 'add' after Tier 1 rename")
	}
}

func TestEndToEnd_RenameJavaScriptJSXComponent(t *testing.T) {
	requireExperimentalIntegration(t, "JavaScript")
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
