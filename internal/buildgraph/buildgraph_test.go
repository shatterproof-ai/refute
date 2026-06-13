// Package buildgraph holds tests that guard the shape of the root module's
// build graph. It deliberately has no non-test Go source.
package buildgraph

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to the directory that holds the root
// go.mod, so the test works regardless of the caller's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate test file")
	}
	// thisFile is .../internal/buildgraph/buildgraph_test.go; the root module
	// is two directories up.
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// TestNoNodeModulesInBuildGraph asserts that `go list ./...` from the root
// module never reports a package whose import path contains "node_modules".
//
// adapters/tsmorph installs npm dependencies under
// adapters/tsmorph/node_modules, some of which ship their own Go source (e.g.
// flatted/golang). Without a module boundary at adapters/tsmorph those
// packages join the root module's build, vet, coverage, and govulncheck
// surface — but only on machines where node_modules is present, so local and
// CI build graphs diverge. A nested adapters/tsmorph/go.mod keeps them out.
// This test fails loudly if that boundary regresses (e.g. the nested go.mod is
// removed). See GitHub issue #77.
func TestNoNodeModulesInBuildGraph(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH; skipping build-graph check")
	}

	// Run in module mode, offline: the root module's packages all resolve
	// from the module cache / local source, so no network access is needed.
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(cmd.Environ(), "GOFLAGS=-mod=readonly", "GOPROXY=off")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list ./... failed: %v\n%s", err, out)
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		if strings.Contains(pkg, "node_modules") {
			t.Errorf("root build graph includes a node_modules package: %q\n"+
				"third-party Go under adapters/tsmorph/node_modules must stay out "+
				"of the root module — confirm adapters/tsmorph/go.mod still exists",
				pkg)
		}
	}
}
