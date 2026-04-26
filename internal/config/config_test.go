package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Use a nonexistent workspace root so no project config is found.
	cfg, err := config.Load("", "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	const wantTimeout = 30000
	if cfg.Timeout != wantTimeout {
		t.Errorf("Timeout: got %d, want %d", cfg.Timeout, wantTimeout)
	}

	goServer := cfg.Server("go")
	if goServer.Command != "gopls" {
		t.Errorf("go server command: got %q, want %q", goServer.Command, "gopls")
	}

	rustServer := cfg.Server("rust")
	if rustServer.Command != "rust-analyzer" {
		t.Errorf("rust server command: got %q, want %q", rustServer.Command, "rust-analyzer")
	}
}

func TestLoad_ProjectConfig(t *testing.T) {
	dir := t.TempDir()

	projectCfg := map[string]any{
		"servers": map[string]any{
			"go": map[string]any{
				"command": "custom-gopls",
				"args":    []string{"--custom"},
			},
		},
		"timeout": 60000,
	}
	data, err := json.Marshal(projectCfg)
	if err != nil {
		t.Fatalf("failed to marshal project config: %v", err)
	}
	cfgPath := filepath.Join(dir, "refute.config.json")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("failed to write project config: %v", err)
	}

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	const wantTimeout = 60000
	if cfg.Timeout != wantTimeout {
		t.Errorf("Timeout: got %d, want %d", cfg.Timeout, wantTimeout)
	}

	goServer := cfg.Server("go")
	if goServer.Command != "custom-gopls" {
		t.Errorf("go server command: got %q, want %q", goServer.Command, "custom-gopls")
	}

	// TypeScript should still fall back to built-in default.
	tsServer := cfg.Server("typescript")
	if tsServer.Command != "typescript-language-server" {
		t.Errorf("typescript server command: got %q, want %q", tsServer.Command, "typescript-language-server")
	}
}

func TestLoad_JavaKotlinDefaults(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	javaServer := cfg.Server("java")
	if javaServer.Command != "jdtls" {
		t.Errorf("java server command: got %q, want %q", javaServer.Command, "jdtls")
	}

	kotlinServer := cfg.Server("kotlin")
	if kotlinServer.Command != "kotlin-language-server" {
		t.Errorf("kotlin server command: got %q, want %q", kotlinServer.Command, "kotlin-language-server")
	}
}

func TestLoad_ExplicitPath(t *testing.T) {
	dir := t.TempDir()

	explicitCfg := map[string]any{
		"servers": map[string]any{
			"go": map[string]any{
				"command": "explicit-gopls",
				"args":    []string{"--explicit"},
			},
		},
		"timeout": 99000,
	}
	data, err := json.Marshal(explicitCfg)
	if err != nil {
		t.Fatalf("failed to marshal explicit config: %v", err)
	}
	explicitPath := filepath.Join(dir, "my-refute.json")
	if err := os.WriteFile(explicitPath, data, 0o644); err != nil {
		t.Fatalf("failed to write explicit config: %v", err)
	}

	// Use a different (nonexistent) workspace root so only the explicit file applies.
	cfg, err := config.Load(explicitPath, "/nonexistent/workspace/root")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	const wantTimeout = 99000
	if cfg.Timeout != wantTimeout {
		t.Errorf("Timeout: got %d, want %d", cfg.Timeout, wantTimeout)
	}

	goServer := cfg.Server("go")
	if goServer.Command != "explicit-gopls" {
		t.Errorf("go server command: got %q, want %q", goServer.Command, "explicit-gopls")
	}
}

func TestResolvedServer_PrefersLocalTypeScriptLanguageServer(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules bin: %v", err)
	}
	serverName := "typescript-language-server"
	if err := os.WriteFile(filepath.Join(binDir, serverName), []byte(""), 0o755); err != nil {
		t.Fatalf("write local typescript-language-server: %v", err)
	}

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	tsServer := cfg.ResolvedServer("typescript", dir)
	want := filepath.Join(dir, "node_modules", ".bin", serverName)
	if tsServer.Command != want {
		t.Fatalf("ResolvedServer(typescript): got %q, want %q", tsServer.Command, want)
	}

	jsServer := cfg.ResolvedServer("javascript", dir)
	if jsServer.Command != want {
		t.Fatalf("ResolvedServer(javascript): got %q, want %q", jsServer.Command, want)
	}
}

func TestResolvedServer_PrefersExplicitConfigOverLocalTypeScriptLanguageServer(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir node_modules bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "typescript-language-server"), []byte(""), 0o755); err != nil {
		t.Fatalf("write local typescript-language-server: %v", err)
	}

	projectCfg := map[string]any{
		"servers": map[string]any{
			"typescript": map[string]any{
				"command": "custom-tsls",
				"args":    []string{"--custom"},
			},
		},
	}
	data, err := json.Marshal(projectCfg)
	if err != nil {
		t.Fatalf("marshal project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refute.config.json"), data, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	tsServer := cfg.ResolvedServer("typescript", dir)
	if tsServer.Command != "custom-tsls" {
		t.Fatalf("ResolvedServer(typescript): got %q, want %q", tsServer.Command, "custom-tsls")
	}
}
