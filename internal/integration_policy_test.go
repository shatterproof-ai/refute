package internal_test

import (
	"os"
	"strings"
	"testing"
)

func TestIntegrationLanePolicy(t *testing.T) {
	workflow := readRepoFile(t, ".github/workflows/ci.yml")
	integrationTests := readRepoFile(t, "internal/integration_test.go")

	assertContains(t, workflow, "name: Run supported Go integration tests")
	assertContains(t, workflow, "go test -tags integration ./internal/")
	assertContains(t, workflow, "name: Run experimental integration tests")
	assertContains(t, workflow, "REFUTE_EXPERIMENTAL_INTEGRATION: \"1\"")
	assertContains(t, workflow, "continue-on-error: true")

	assertContains(t, integrationTests, "REFUTE_EXPERIMENTAL_INTEGRATION")
	assertContains(t, integrationTests, "Experimental integration tests are opt-in")
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertContains(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected content to contain %q", want)
	}
}
