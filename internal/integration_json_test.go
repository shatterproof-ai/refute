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
	requireExperimentalIntegration(t, "Rust")
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
		"--line", "5",
		"--name", "format_greeting",
		"--new-name", "build_greeting",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for bad server, got success; output:\n%s", out)
	}
	if !strings.Contains(string(out), "initializing backend") &&
		!strings.Contains(string(out), "not found on PATH") {
		t.Errorf("expected backend/server error in output, got:\n%s", out)
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
	// The CLI's --file validation reports a missing path with "does not exist"
	// and echoes the offending path. Assert on both so the test stays useful
	// without being coupled to incidental wording.
	if !strings.Contains(string(out), "does not exist") ||
		!strings.Contains(string(out), "/nonexistent/path/to/file.go") {
		t.Errorf("expected missing-file error naming the path in output, got:\n%s", out)
	}
}

// TestEndToEnd_JSONBackendMissing exercises the --json failure envelope when
// the configured LSP binary cannot be found on PATH. Uses the tier-1 setup
// path so no real LSP server is required.
func TestEndToEnd_JSONBackendMissing(t *testing.T) {
	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	cfgContent := `{"servers": {"go": {"command": "refute-nonexistent-lsp-binary-xyz"}}}`
	cfgFile := filepath.Join(t.TempDir(), "bad-config.json")
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"--config", cfgFile,
		"rename-function",
		"--symbol", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--json",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for missing backend, got success:\n%s", out)
	}
	envelope := extractJSONEnvelope(t, out)
	var parsed struct {
		SchemaVersion string `json:"schemaVersion"`
		Status        string `json:"status"`
		Error         *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Hint    string `json:"hint"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(envelope), &parsed); err != nil {
		t.Fatalf("parse JSON envelope: %v\nraw:\n%s", err, out)
	}
	if parsed.SchemaVersion != "1" {
		t.Errorf("schemaVersion = %q, want \"1\"", parsed.SchemaVersion)
	}
	if parsed.Status != "backend-missing" {
		t.Errorf("status = %q, want backend-missing; envelope:\n%s", parsed.Status, envelope)
	}
	if parsed.Error == nil || parsed.Error.Code == "" {
		t.Errorf("missing error code; envelope:\n%s", envelope)
	}
	if parsed.Error != nil && parsed.Error.Hint == "" {
		t.Errorf("expected non-empty hint; envelope:\n%s", envelope)
	}
}

// TestEndToEnd_JSONInvalidPosition exercises the --json failure envelope when
// the requested symbol position cannot be resolved.
func TestEndToEnd_JSONInvalidPosition(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found on PATH")
	}
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
		"--new-name", "Whatever",
		"--json",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for invalid position, got success:\n%s", out)
	}
	envelope := extractJSONEnvelope(t, out)
	var parsed struct {
		Status string `json:"status"`
		Error  *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(envelope), &parsed); err != nil {
		t.Fatalf("parse JSON envelope: %v\nraw:\n%s", err, out)
	}
	if parsed.Status != "invalid-position" {
		t.Errorf("status = %q, want invalid-position; envelope:\n%s", parsed.Status, envelope)
	}
	if parsed.Error == nil || parsed.Error.Code == "" {
		t.Errorf("missing error code; envelope:\n%s", envelope)
	}
}

// TestEndToEnd_JSONBackendFailed exercises the --json failure envelope when
// the configured LSP binary spawns but exits immediately.
func TestEndToEnd_JSONBackendFailed(t *testing.T) {
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("/bin/false not available")
	}
	srcDir := filepath.Join("../testdata/fixtures/go/rename")
	dir := t.TempDir()
	copyDir(t, srcDir, dir)

	cfgContent := `{"servers": {"go": {"command": "` + falseBin + `"}}}`
	cfgFile := filepath.Join(t.TempDir(), "false-config.json")
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	refuteBin := buildRefute(t)
	cmd := exec.Command(refuteBin,
		"--config", cfgFile,
		"rename-function",
		"--symbol", "FormatGreeting",
		"--new-name", "BuildGreeting",
		"--json",
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit for failing backend, got success:\n%s", out)
	}
	envelope := extractJSONEnvelope(t, out)
	var parsed struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(envelope), &parsed); err != nil {
		t.Fatalf("parse JSON envelope: %v\nraw:\n%s", err, out)
	}
	if parsed.Status != "backend-failed" && parsed.Status != "backend-missing" {
		t.Errorf("status = %q, want backend-failed (or backend-missing); envelope:\n%s", parsed.Status, envelope)
	}
}

// extractJSONEnvelope pulls the first balanced JSON object out of combined
// stdout/stderr. The CLI prints the envelope to stdout; backend stderr noise
// can be interleaved by CombinedOutput.
func extractJSONEnvelope(t *testing.T, out []byte) string {
	t.Helper()
	start := -1
	depth := 0
	for i, b := range out {
		if b == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if b == '}' {
			depth--
			if depth == 0 && start != -1 {
				return string(out[start : i+1])
			}
		}
	}
	t.Fatalf("no JSON object found in output:\n%s", out)
	return ""
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
