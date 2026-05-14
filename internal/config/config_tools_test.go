package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/config"
)

func TestLoad_ToolsTSMorphAdapterPath(t *testing.T) {
	dir := t.TempDir()

	data, err := json.Marshal(map[string]any{
		"tools": map[string]any{
			"tsmorph": map[string]any{
				"adapter": "/custom/path/rename.cjs",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refute.config.json"), data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Tools.TSMorph.Adapter; got != "/custom/path/rename.cjs" {
		t.Errorf("Tools.TSMorph.Adapter = %q, want %q", got, "/custom/path/rename.cjs")
	}
}

func TestLoad_ToolsDefaultsEmpty(t *testing.T) {
	cfg, err := config.Load("", "/nonexistent")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Tools.TSMorph.Adapter != "" {
		t.Errorf("default Tools.TSMorph.Adapter = %q, want empty string", cfg.Tools.TSMorph.Adapter)
	}
}

func TestLoad_ToolsExplicitPathOverrides(t *testing.T) {
	projectDir := t.TempDir()
	explicitDir := t.TempDir()

	// Project config sets one path.
	projectData, _ := json.Marshal(map[string]any{
		"tools": map[string]any{
			"tsmorph": map[string]any{"adapter": "/project/rename.cjs"},
		},
	})
	if err := os.WriteFile(filepath.Join(projectDir, "refute.config.json"), projectData, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	// Explicit override wins.
	explicitData, _ := json.Marshal(map[string]any{
		"tools": map[string]any{
			"tsmorph": map[string]any{"adapter": "/explicit/rename.cjs"},
		},
	})
	explicitPath := filepath.Join(explicitDir, "override.json")
	if err := os.WriteFile(explicitPath, explicitData, 0o644); err != nil {
		t.Fatalf("write explicit config: %v", err)
	}

	cfg, err := config.Load(explicitPath, projectDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := cfg.Tools.TSMorph.Adapter; got != "/explicit/rename.cjs" {
		t.Errorf("Tools.TSMorph.Adapter = %q, want /explicit/rename.cjs (explicit layer wins)", got)
	}
}
