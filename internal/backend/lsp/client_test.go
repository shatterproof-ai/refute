package lsp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
)

// setupGoProject creates a temporary directory with a minimal Go module and
// a main.go file that declares and calls a function named oldName.
// It returns the path to the temp directory.
func setupGoProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	goMod := `module example.com/testpkg

go 1.21
`
	mainGo := `package main

func oldName() {}

func main() {
	oldName()
}
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(mainGo), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	return dir
}

// requireGopls skips the test if gopls is not found on PATH.
func requireGopls(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH; skipping integration test")
	}
}

func TestClient_Initialize(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	if !client.RenameProvider() {
		t.Error("expected RenameProvider capability to be true from gopls")
	}
}

func TestClient_CodeActions(t *testing.T) {
	requireGopls(t)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainSrc := `package main

func main() {
	x := 1 + 2
	println(x)
}
`
	mainFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainFile, []byte(mainSrc), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	// "1 + 2" is on 0-indexed line 3, columns 6-11 (after "\tx := ").
	actions, err := client.CodeActions(mainFile, 3, 6, 3, 11, []string{"refactor.extract"})
	if err != nil {
		t.Fatalf("CodeActions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("expected at least one extract action")
	}
	found := false
	for _, a := range actions {
		if strings.Contains(strings.ToLower(a.Title), "extract") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no extract action in results: %v", actions)
	}
}

func TestClient_WorkspaceSymbol(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer client.Shutdown()

	// Prime gopls: workspace/symbol only searches loaded packages.
	if err := client.DidOpen(filepath.Join(dir, "main.go"), "go"); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	syms, err := client.WorkspaceSymbol("oldName")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}
	found := false
	for _, s := range syms {
		if s.Name == "oldName" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("oldName not in results: %v", syms)
	}
}

func TestClient_Rename(t *testing.T) {
	requireGopls(t)
	dir := setupGoProject(t)

	client, err := lsp.StartClient("gopls", []string{"serve"}, dir)
	if err != nil {
		t.Fatalf("StartClient: %v", err)
	}
	defer func() {
		if err := client.Shutdown(); err != nil {
			t.Logf("Shutdown: %v", err)
		}
	}()

	mainFile := filepath.Join(dir, "main.go")
	if err := client.DidOpen(mainFile, "go"); err != nil {
		t.Fatalf("DidOpen: %v", err)
	}

	// "func oldName()" is on line 3 (0-indexed: line=2), character 5.
	// Line 3 of main.go: `func oldName() {}`
	// 'o' in oldName starts at character index 5 (after "func ").
	fileEdits, err := client.Rename(mainFile, 2, 5, "newName")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Count total edits across all files (declaration + call site = at least 2).
	totalEdits := 0
	for _, fe := range fileEdits {
		totalEdits += len(fe.Edits)
	}

	if totalEdits < 2 {
		t.Errorf("expected at least 2 edits (declaration + call site), got %d", totalEdits)
	}
}
