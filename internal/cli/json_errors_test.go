package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// resetRenameFlagsForTest clears the package-level rename flags so individual
// test cases can configure a fresh invocation without bleed-through.
func resetRenameFlagsForTest(t *testing.T) {
	t.Helper()
	prevConfig := flagConfig
	flagFile = ""
	flagLine = 0
	flagCol = 0
	flagName = ""
	flagNewName = ""
	flagSymbol = ""
	flagJSON = false
	flagDryRun = false
	flagConfig = ""
	t.Cleanup(func() {
		flagFile = ""
		flagLine = 0
		flagCol = 0
		flagName = ""
		flagNewName = ""
		flagSymbol = ""
		flagJSON = false
		flagDryRun = false
		flagConfig = prevConfig
	})
}

// writeGoFixture writes a minimal Go module under dir so tier-1 setup can find
// a workspace root and a language hint without needing gopls itself.
func writeGoFixture(t *testing.T, dir string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fix\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	mainFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(mainFile, []byte("package main\n\nfunc hello() {}\n\nfunc main() { hello() }\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	return mainFile
}

// writeServerConfig writes a refute config that points the given language at
// the supplied command. Returns the config path.
func writeServerConfig(t *testing.T, dir, language, command string) string {
	t.Helper()
	cfg := `{"servers": {"` + language + `": {"command": "` + command + `"}}}`
	p := filepath.Join(dir, "refute.json")
	if err := os.WriteFile(p, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// TestTier1Rename_JSONBackendMissing exercises the tier-1 setup path with a
// non-existent LSP binary configured. The CLI must emit a JSON envelope with
// status backend-missing rather than a plain stderr error.
func TestTier1Rename_JSONBackendMissing(t *testing.T) {
	resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flagFile = mainFile
	flagSymbol = "hello"
	flagNewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.SchemaVersion != edit.SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", got.SchemaVersion, edit.SchemaVersion)
	}
	if got.Status != edit.StatusBackendMissing {
		t.Errorf("status = %q, want %q", got.Status, edit.StatusBackendMissing)
	}
	if got.Error == nil || got.Error.Code == "" {
		t.Fatalf("missing error object: %+v", got.Error)
	}
	if got.Error.Hint == "" {
		t.Errorf("expected non-empty hint, got empty; envelope: %+v", got.Error)
	}
}

// TestTier1Rename_JSONNoServerConfigured exercises the tier-1 setup path with
// an empty server.command, which historically returned a plain error. Must
// emit a backend-missing JSON envelope under --json.
func TestTier1Rename_JSONNoServerConfigured(t *testing.T) {
	resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "")

	flagFile = mainFile
	flagSymbol = "hello"
	flagNewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusBackendMissing {
		t.Errorf("status = %q, want %q (no server configured); envelope:\n%s", got.Status, edit.StatusBackendMissing, out)
	}
	if got.Error == nil {
		t.Fatalf("missing error object")
	}
}

// TestRename_JSONBackendMissing covers the non-tier-1 path where the resolved
// position triggers buildBackend() and the configured server is missing.
func TestRename_JSONBackendMissing(t *testing.T) {
	resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flagFile = mainFile
	flagLine = 3
	flagName = "hello"
	flagNewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusBackendMissing {
		t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusBackendMissing, out)
	}
}
