package toolchain

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLockAndSelectArtifact(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, LockfileName)
	writeFile(t, lockPath, `{
		"version": "v1.2.3",
		"manifest_url": "https://github.com/shatterproof-ai/refute/releases/download/v1.2.3/refute-manifest-v1.2.3.json",
		"artifacts": [
			{"platform": "linux", "architecture": "amd64", "url": "https://example.com/linux.tgz", "sha256": "abc"},
			{"platform": "darwin", "architecture": "arm64", "url": "https://example.com/darwin.tgz", "sha256": "def"}
		]
	}`)

	lock, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock: %v", err)
	}
	if lock.Version != "v1.2.3" {
		t.Fatalf("Version = %q", lock.Version)
	}
	artifact, err := SelectArtifact(lock, "darwin", "arm64")
	if err != nil {
		t.Fatalf("SelectArtifact: %v", err)
	}
	if artifact.SHA256 != "def" {
		t.Fatalf("selected sha = %q", artifact.SHA256)
	}
}

func TestSelectArtifactUnsupportedPlatform(t *testing.T) {
	lock := Lock{
		Version: "v1.2.3",
		Artifacts: []Artifact{{
			Platform:     "linux",
			Architecture: "amd64",
			URL:          "https://example.com/refute.tar.gz",
			SHA256:       "abc",
		}},
	}
	_, err := SelectArtifact(lock, "windows", "amd64")
	if err == nil {
		t.Fatal("SelectArtifact unexpectedly succeeded")
	}
}

func TestLoadLockRejectsIncompleteArtifact(t *testing.T) {
	root := t.TempDir()
	lockPath := filepath.Join(root, LockfileName)
	writeFile(t, lockPath, `{"version":"v1.2.3","manifest_url":"https://example.com/manifest.json","artifacts":[{"platform":"linux"}]}`)
	if _, err := LoadLock(lockPath); err == nil {
		t.Fatal("LoadLock unexpectedly accepted incomplete artifact")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
