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
