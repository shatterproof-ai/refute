package tsmorph

import (
	"errors"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
)

// TestRuntimeMissingErr verifies the ts-morph adapter reports a typed
// ErrAdapterRuntimeMissing (carrying an install hint) when its adapter script
// or bundled node dependencies cannot be located.
func TestRuntimeMissingErr(t *testing.T) {
	err := runtimeMissingErr()

	var rt *backend.ErrAdapterRuntimeMissing
	if !errors.As(err, &rt) {
		t.Fatalf("expected *backend.ErrAdapterRuntimeMissing, got %T: %v", err, err)
	}
	if rt.AdapterName != "ts-morph" || rt.Language != "typescript" {
		t.Errorf("unexpected typed fields: %+v", rt)
	}
	if !strings.Contains(rt.InstallHint, "npm install") {
		t.Errorf("InstallHint = %q, want npm install command", rt.InstallHint)
	}
	if !strings.Contains(rt.Error(), "ts-morph") {
		t.Errorf("Error() = %q, want mention of ts-morph", rt.Error())
	}
}
