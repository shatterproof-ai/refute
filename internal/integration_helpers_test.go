//go:build integration

package internal_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/testutil"
)

const experimentalIntegrationEnv = "REFUTE_EXPERIMENTAL_INTEGRATION"

func requireExperimentalIntegration(t *testing.T, area string) {
	t.Helper()
	if os.Getenv(experimentalIntegrationEnv) == "" {
		t.Skipf("%s integration is experimental. Experimental integration tests are opt-in: set %s=1 to run this lane.", area, experimentalIntegrationEnv)
	}
}

// buildRefute compiles the refute binary into a temp dir and returns its path.
// The integration package runs from internal/, so the module root is "..".
func buildRefute(t *testing.T) string {
	t.Helper()
	return testutil.BuildRefute(t, "..")
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

// copyDir recursively copies a directory tree, skipping .git and node_modules.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	testutil.CopyDir(t, src, dst)
}
