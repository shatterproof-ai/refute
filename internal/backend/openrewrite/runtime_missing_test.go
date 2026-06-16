package openrewrite

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
)

// TestResolveJar_ExplicitMissing verifies that an explicitly configured JAR
// path that does not exist yields a typed ErrAdapterRuntimeMissing rather than a
// generic error, so the CLI can report "build the adapter JAR" distinctly.
func TestResolveJar_ExplicitMissing(t *testing.T) {
	a := NewAdapter(filepath.Join(t.TempDir(), "nonexistent-openrewrite-adapter.jar"))

	_, err := a.resolveJar(t.TempDir())
	if err == nil {
		t.Fatal("expected error for missing explicit JAR path")
	}
	var rt *backend.ErrAdapterRuntimeMissing
	if !errors.As(err, &rt) {
		t.Fatalf("expected *backend.ErrAdapterRuntimeMissing, got %T: %v", err, err)
	}
	if rt.AdapterName != "openrewrite" || rt.Language != "java" {
		t.Errorf("unexpected typed fields: %+v", rt)
	}
	if !strings.Contains(rt.MissingRuntime, "JAR") {
		t.Errorf("MissingRuntime = %q, want mention of JAR", rt.MissingRuntime)
	}
}

// TestResolveJar_DefaultMissing verifies the conventional-build-path branch also
// returns the typed error and carries an mvn install hint.
func TestResolveJar_DefaultMissing(t *testing.T) {
	// A temp dir with a go.mod looks like a checkout root but has no built JAR.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/x\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := NewAdapter("")
	_, err := a.resolveJar(root)
	var rt *backend.ErrAdapterRuntimeMissing
	if !errors.As(err, &rt) {
		t.Fatalf("expected *backend.ErrAdapterRuntimeMissing, got %T: %v", err, err)
	}
	if !strings.Contains(rt.InstallHint, "mvn package") {
		t.Errorf("InstallHint = %q, want mvn package command", rt.InstallHint)
	}
}
