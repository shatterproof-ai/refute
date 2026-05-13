package internal_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

type shatterProjectConfig struct {
	Include            []string `json:"include"`
	Exclude            []string `json:"exclude"`
	TimeoutTotal       int      `json:"timeout_total"`
	ExecTimeout        int      `json:"exec_timeout"`
	Parallelism        int      `json:"parallelism"`
	ParallelismMin     int      `json:"parallelism_min"`
	ParallelismMax     int      `json:"parallelism_max"`
	CacheDir           string   `json:"cache_dir"`
	SeedsDir           string   `json:"seeds_dir"`
	CaptureSideEffects bool     `json:"capture_side_effects"`
	Output             struct {
		Format string   `json:"format"`
		Paths  []string `json:"paths"`
		Stdout bool     `json:"stdout"`
	} `json:"output"`
}

func TestShatterProjectConfigCoversRefuteSources(t *testing.T) {
	data, err := os.ReadFile("../shatter.config.json")
	if err != nil {
		t.Fatalf("read shatter.config.json: %v", err)
	}

	var cfg shatterProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse shatter.config.json: %v", err)
	}

	expectedIncludes := []string{
		"cmd/**/*.go",
		"internal/**/*.go",
		"testdata/fixtures/go/**/*.go",
	}
	for _, pattern := range expectedIncludes {
		if !slices.Contains(cfg.Include, pattern) {
			t.Errorf("include is missing %q", pattern)
		}
	}

	expectedExcludes := []string{
		"**/.git/**",
		"**/node_modules/**",
		"**/vendor/**",
		"**/target/**",
		"**/dist/**",
		"**/.shatter-cache/**",
		"**/shatter-artifacts/**",
		"**/shatter-report/**",
		"adapters/openrewrite/**",
		"testdata/fixtures/java/**",
		"testdata/fixtures/javascript/**",
		"testdata/fixtures/rust/**",
		"testdata/fixtures/typescript/**",
	}
	for _, pattern := range expectedExcludes {
		if !slices.Contains(cfg.Exclude, pattern) {
			t.Errorf("exclude is missing %q", pattern)
		}
	}

	if cfg.TimeoutTotal != 900 {
		t.Errorf("timeout_total = %d, want 900", cfg.TimeoutTotal)
	}
	if cfg.ExecTimeout != 15 {
		t.Errorf("exec_timeout = %d, want 15", cfg.ExecTimeout)
	}
	if cfg.Parallelism != 0 || cfg.ParallelismMin != 2 || cfg.ParallelismMax != 8 {
		t.Errorf("parallelism = %d min=%d max=%d, want 0 min=2 max=8", cfg.Parallelism, cfg.ParallelismMin, cfg.ParallelismMax)
	}
	if cfg.CacheDir != ".shatter-cache/behavior-maps" {
		t.Errorf("cache_dir = %q", cfg.CacheDir)
	}
	if cfg.SeedsDir != ".shatter/seeds" {
		t.Errorf("seeds_dir = %q", cfg.SeedsDir)
	}
	if !cfg.CaptureSideEffects {
		t.Error("capture_side_effects should be enabled for full behavior capture")
	}
	if cfg.Output.Format != "markdown" {
		t.Errorf("output.format = %q, want markdown", cfg.Output.Format)
	}
	for _, path := range []string{"shatter-report/report.md", "shatter-report/report.json"} {
		if !slices.Contains(cfg.Output.Paths, path) {
			t.Errorf("output.paths is missing %q", path)
		}
	}
	if !cfg.Output.Stdout {
		t.Error("output.stdout should be true")
	}
}

func TestShatterHierarchicalConfigSetsFullCoverageDefaults(t *testing.T) {
	data, err := os.ReadFile("../.shatter/config.yaml")
	if err != nil {
		t.Fatalf("read .shatter/config.yaml: %v", err)
	}
	content := string(data)

	for _, want := range []string{
		"max_iterations: 250",
		"timeout: 120",
		"adaptive: true",
		"score_window: 100",
		"boundary: 0.35",
		"literals: 0.25",
		"random: 0.25",
		"mutation: 0.15",
	} {
		if !strings.Contains(content, want) {
			t.Errorf(".shatter/config.yaml missing %q", want)
		}
	}
}

func TestShatterCleanTargetExistsInMakefile(t *testing.T) {
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		"shatter-clean",
		"build:",
		"go build -buildvcs=false ./cmd/refute",
		".shatter-cache",
		"shatter-artifacts",
		"shatter-report",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("Makefile missing %q", want)
		}
	}
	if !strings.Contains(content, ".PHONY") {
		t.Error("Makefile missing .PHONY declaration")
	}
}

func TestMakefileDefaultTargetBuildsRefute(t *testing.T) {
	data, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ".") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			if line != "build:" {
				t.Fatalf("default Makefile target = %q, want build", strings.TrimSuffix(line, ":"))
			}
			return
		}
	}

	t.Fatal("Makefile has no default target")
}

func TestShatterGeneratedArtifactsAreIgnored(t *testing.T) {
	data, err := os.ReadFile("../.gitignore")
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	content := string(data)
	for _, want := range []string{
		".shatter-cache/",
		"shatter-artifacts/",
		"shatter-report/",
	} {
		if !strings.Contains(content, want) {
			t.Errorf(".gitignore missing %q", want)
		}
	}
}
