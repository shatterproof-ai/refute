//go:build integration

package internal_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const experimentalIntegrationEnv = "REFUTE_EXPERIMENTAL_INTEGRATION"

func requireExperimentalIntegration(t *testing.T, area string) {
	t.Helper()
	if os.Getenv(experimentalIntegrationEnv) == "" {
		t.Skipf("%s integration is experimental. Experimental integration tests are opt-in: set %s=1 to run this lane.", area, experimentalIntegrationEnv)
	}
}

// buildRefute compiles the refute binary into a temp dir and returns its path.
func buildRefute(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "refute")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", bin, "./cmd/refute")
	cmd.Dir = ".."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build refute: %v\n%s", err, out)
	}
	return bin
}

func requireFixtureTypeScriptLanguageServer(t *testing.T, fixtureDir string) string {
	t.Helper()
	server := filepath.Join(fixtureDir, "node_modules", ".bin", "typescript-language-server")
	if _, err := os.Stat(server); err != nil {
		t.Skipf("local typescript-language-server not found at %s; run npm install in %s", server, fixtureDir)
	}
	abs, err := filepath.Abs(server)
	if err != nil {
		t.Fatalf("resolve typescript-language-server path: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(abs)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return abs
}

func linkFixtureNodeModules(t *testing.T, fixtureDir string, workspaceDir string) {
	t.Helper()
	target, err := filepath.Abs(filepath.Join(fixtureDir, "node_modules"))
	if err != nil {
		t.Fatalf("resolve fixture node_modules path: %v", err)
	}
	link := filepath.Join(workspaceDir, "node_modules")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link fixture node_modules: %v", err)
	}
}

// copyDir recursively copies a directory tree.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("reading %s: %v", src, err)
	}
	for _, e := range entries {
		if e.Name() == "node_modules" {
			continue
		}
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			os.MkdirAll(dstPath, 0755)
			copyDir(t, srcPath, dstPath)
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("reading %s: %v", srcPath, err)
			}
			os.WriteFile(dstPath, data, 0644)
		}
	}
}
