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

// resetRenameFlagsForTest clears the shared invocation flags and returns a
// fresh rename flag set for each test case.
func resetRenameFlagsForTest(t *testing.T) *renameFlags {
	t.Helper()
	flags := &renameFlags{}
	prevConfig := flagConfig
	flagJSON = false
	flagDryRun = false
	flagConfig = ""
	t.Cleanup(func() {
		flagJSON = false
		flagDryRun = false
		flagConfig = prevConfig
	})
	return flags
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

// writeJSFixture writes a minimal JavaScript file under dir so a pre-backend
// resolution error can report language metadata without needing ts-morph.
func writeJSFixture(t *testing.T, dir string) string {
	t.Helper()
	jsFile := filepath.Join(dir, "math.js")
	if err := os.WriteFile(jsFile, []byte("export function sum(left, right) {\n    return left + right;\n}\n"), 0o644); err != nil {
		t.Fatalf("write math.js: %v", err)
	}
	return jsFile
}

// TestRunRename_JSJSONErrorReportsJavaScriptLanguage covers the pre-backend
// resolution failure path for a .js file. JavaScript routes through the
// TypeScript config/server key internally, but the user-facing JSON metadata
// must report language "javascript", not the config key "typescript".
func TestRunRename_JSJSONErrorReportsJavaScriptLanguage(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	jsFile := writeJSFixture(t, dir)

	flags.File = jsFile
	flags.Line = 1
	flags.Name = "add"
	flags.NewName = "total"
	flagJSON = true
	flagDryRun = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusInvalidPosition {
		t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusInvalidPosition, out)
	}
	if got.Error == nil || got.Error.Code != "invalid-position" {
		t.Fatalf("unexpected error object: %+v\nraw:\n%s", got.Error, out)
	}
	if got.Language != "javascript" {
		t.Errorf("language = %q, want %q (config key must not leak); envelope:\n%s", got.Language, "javascript", out)
	}
}

// TestRunRename_TSJSONErrorReportsTypeScriptLanguage guards the symmetric
// TypeScript case so the JavaScript fix does not regress .ts metadata.
func TestRunRename_TSJSONErrorReportsTypeScriptLanguage(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	tsFile := filepath.Join(dir, "math.ts")
	if err := os.WriteFile(tsFile, []byte("export function sum(left: number, right: number) {\n    return left + right;\n}\n"), 0o644); err != nil {
		t.Fatalf("write math.ts: %v", err)
	}

	flags.File = tsFile
	flags.Line = 1
	flags.Name = "add"
	flags.NewName = "total"
	flagJSON = true
	flagDryRun = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Language != "typescript" {
		t.Errorf("language = %q, want %q; envelope:\n%s", got.Language, "typescript", out)
	}
}

// TestTier1Rename_JSONBackendMissing exercises the tier-1 setup path with a
// non-existent LSP binary configured. The CLI must emit a JSON envelope with
// status backend-missing rather than a plain stderr error.
func TestTier1Rename_JSONBackendMissing(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flags.File = mainFile
	flags.Symbol = "hello"
	flags.NewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
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
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "")

	flags.File = mainFile
	flags.Symbol = "hello"
	flags.NewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
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
	if got.Error.Code != "backend-missing" {
		t.Fatalf("error code = %q, want backend-missing; envelope:\n%s", got.Error.Code, out)
	}
}

// TestTier1Rename_JSONBackendInitFailed exercises the Tier 1 setup path where
// the LSP binary exists but exits before initialization can complete. It must
// use the same backend-init-failed envelope as file/position rename.
func TestTier1Rename_JSONBackendInitFailed(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	failingServer := filepath.Join(dir, "failing-lsp")
	if err := os.WriteFile(failingServer, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write failing lsp: %v", err)
	}
	flagConfig = writeServerConfig(t, dir, "go", failingServer)

	flags.File = mainFile
	flags.Symbol = "hello"
	flags.NewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code != 1 {
		t.Fatalf("expected exit code 1, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusBackendFailed {
		t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusBackendFailed, out)
	}
	if got.Error == nil || got.Error.Code != "backend-init-failed" {
		t.Fatalf("error = %+v, want code backend-init-failed; envelope:\n%s", got.Error, out)
	}
}

// writeJavaFixture writes a minimal Java source file under a Go workspace so a
// Tier 1 (--symbol) rename infers language "java" (an unsupported support-matrix
// row) while still finding a workspace root. The go.mod marker only anchors the
// workspace; the .java file drives language inference.
func writeJavaFixture(t *testing.T, dir string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fix\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	javaFile := filepath.Join(dir, "Greeter.java")
	if err := os.WriteFile(javaFile, []byte("class Greeter {\n    void greet() {}\n}\n"), 0o644); err != nil {
		t.Fatalf("write Greeter.java: %v", err)
	}
	return javaFile
}

// TestTier1Rename_JSONUnsupportedLanguage guards issue #124: a Tier 1 (--symbol)
// rename targeting a language the support matrix marks unsupported (Java) must
// report the documented unsupported-language status instead of falling through
// to backend setup and reporting backend-missing.
func TestTier1Rename_JSONUnsupportedLanguage(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	javaFile := writeJavaFixture(t, dir)

	flags.File = javaFile
	flags.Symbol = "Greeter.greet"
	flags.NewName = "hello"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindMethod, flags)
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}
	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusUnsupported {
		t.Errorf("status = %q, want %q; envelope:\n%s", got.Status, edit.StatusUnsupported, out)
	}
	if got.Error == nil || got.Error.Code != "unsupported-language" {
		t.Fatalf("error = %+v, want code unsupported-language; envelope:\n%s", got.Error, out)
	}
	if got.Language != "java" {
		t.Errorf("language = %q, want %q; envelope:\n%s", got.Language, "java", out)
	}
}

// TestRename_JSONBackendMissing covers the non-tier-1 path where the resolved
// position triggers buildBackend() and the configured server is missing.
func TestRename_JSONBackendMissing(t *testing.T) {
	flags := resetRenameFlagsForTest(t)
	dir := t.TempDir()
	mainFile := writeGoFixture(t, dir)
	flagConfig = writeServerConfig(t, dir, "go", "refute-nonexistent-lsp-binary-xyz")

	flags.File = mainFile
	flags.Line = 3
	flags.Name = "hello"
	flags.NewName = "hi"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runRename(symbol.KindFunction, flags)
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
