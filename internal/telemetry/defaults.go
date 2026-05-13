package telemetry

import (
	"os"
	"path/filepath"
)

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "refute")
}

// DefaultSnapshotRoot returns the default compressed snapshot directory.
func DefaultSnapshotRoot() string {
	dir := defaultDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "snapshots")
}

// DefaultSessionRoot returns the default human-readable agent session log root.
func DefaultSessionRoot() string {
	dir := defaultDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "sessions")
}
