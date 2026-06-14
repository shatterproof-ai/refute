package telemetry_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shatterproof-ai/refute/internal/telemetry"
)

func TestDefaultPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".local", "share", "refute", "telemetry.jsonl")
	if got := telemetry.DefaultPath(); got != want {
		t.Errorf("DefaultPath: got %q, want %q", got, want)
	}
}
