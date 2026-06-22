//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndToEnd_RenameTypeScriptFunction(t *testing.T) {
	requireExperimentalIntegration(t, "TypeScript")
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
	requireExperimentalIntegration(t, "TypeScript")
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
	requireExperimentalIntegration(t, "TypeScript")
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
	requireExperimentalIntegration(t, "TypeScript")
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
	requireExperimentalIntegration(t, "TypeScript")
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
	requireExperimentalIntegration(t, "TypeScript")
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

func TestEndToEnd_RenameTypeScriptMethodBySymbol(t *testing.T) {
	requireExperimentalIntegration(t, "TypeScript")
	srcDir := filepath.Join("../testdata/fixtures/typescript/rename")
	requireFixtureTypeScriptLanguageServer(t, srcDir)
	dir := t.TempDir()
	copyDir(t, srcDir, dir)
	linkFixtureNodeModules(t, srcDir, dir)

	refuteBin := buildRefute(t)

	personFile := filepath.Join(dir, "src", "person.ts")
	cmd := exec.Command(refuteBin,
		"rename-method",
		"--file", personFile,
		"--symbol", "Person.greet",
		"--new-name", "salute",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("refute failed: %s\n%s", err, out)
	}

	personContent, _ := os.ReadFile(personFile)
	if strings.Contains(string(personContent), "greet(): string") {
		t.Error("person.ts still contains method name 'greet'")
	}
	if !strings.Contains(string(personContent), "salute(): string") {
		t.Error("person.ts missing method name 'salute'")
	}

	mainFile := filepath.Join(dir, "src", "main.ts")
	mainContent, _ := os.ReadFile(mainFile)
	if strings.Contains(string(mainContent), ".greet()") {
		t.Error("main.ts still contains '.greet()' after Tier 1 rename")
	}
	if !strings.Contains(string(mainContent), ".salute()") {
		t.Error("main.ts missing '.salute()' after Tier 1 rename")
	}
}
