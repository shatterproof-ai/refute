package toolchain

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncInstallsAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	archive, digest := writeRefuteArchive(t, root, "#!/bin/sh\necho synced\n")
	writeLock(t, root, archive, digest)

	result, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !result.Installed {
		t.Fatal("first sync did not install")
	}
	active := filepath.Join(root, ActiveBinPath)
	data, err := os.ReadFile(active)
	if err != nil {
		t.Fatalf("read active binary: %v", err)
	}
	if !strings.Contains(string(data), "synced") {
		t.Fatalf("active binary content = %q", data)
	}

	second, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("second Sync: %v", err)
	}
	if second.Installed {
		t.Fatal("second sync should be idempotent")
	}
}

func TestSyncReplacesStaleActiveBinary(t *testing.T) {
	root := t.TempDir()
	archive, digest := writeRefuteArchive(t, root, "#!/bin/sh\necho fresh\n")
	writeLock(t, root, archive, digest)
	active := filepath.Join(root, ActiveBinPath)
	writeFile(t, active, "#!/bin/sh\necho stale\n")
	writeFile(t, active+".artifact-sha256", "old\n")

	result, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !result.Installed {
		t.Fatal("sync did not replace stale binary")
	}
	got, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "stale") {
		t.Fatalf("active binary was not replaced: %q", got)
	}
}

func TestSyncReplacesChecksumMismatchedActiveBinary(t *testing.T) {
	root := t.TempDir()
	archive, digest := writeRefuteArchive(t, root, "#!/bin/sh\necho fresh\n")
	writeLock(t, root, archive, digest)
	if _, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"}); err != nil {
		t.Fatalf("initial Sync: %v", err)
	}
	active := filepath.Join(root, ActiveBinPath)
	writeFile(t, active, "#!/bin/sh\necho tampered\n")

	result, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if !result.Installed {
		t.Fatal("sync did not replace checksum-mismatched binary")
	}
	got, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "tampered") {
		t.Fatalf("active binary was not replaced: %q", got)
	}
}

func TestSyncRejectsChecksumMismatch(t *testing.T) {
	root := t.TempDir()
	archive, _ := writeRefuteArchive(t, root, "#!/bin/sh\necho bad\n")
	writeLock(t, root, archive, strings.Repeat("0", 64))

	_, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "linux", Arch: "amd64"})
	if err == nil {
		t.Fatal("Sync unexpectedly accepted checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error = %v", err)
	}
}

func TestSyncRejectsUnsupportedPlatform(t *testing.T) {
	root := t.TempDir()
	archive, digest := writeRefuteArchive(t, root, "#!/bin/sh\necho ok\n")
	writeLock(t, root, archive, digest)
	_, err := Sync(context.Background(), SyncOptions{ProjectRoot: root, Platform: "plan9", Arch: "arm"})
	if err == nil {
		t.Fatal("Sync unexpectedly accepted unsupported platform")
	}
}

func writeLock(t *testing.T, root, archive, digest string) {
	t.Helper()
	lock := fmt.Sprintf(`{
		"version": "v9.9.9",
		"manifest_url": "file://%s",
		"artifacts": [{
			"platform": "linux",
			"architecture": "amd64",
			"url": "file://%s",
			"sha256": "%s",
			"filename": "refute_v9.9.9_linux_amd64.tar.gz"
		}]
	}`, filepath.ToSlash(filepath.Join(root, "manifest.json")), filepath.ToSlash(archive), digest)
	writeFile(t, filepath.Join(root, LockfileName), lock)
}

func writeRefuteArchive(t *testing.T, root, script string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(script)
	if err := tw.WriteHeader(&tar.Header{Name: "refute", Mode: 0o755, Size: int64(len(body))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	path := filepath.Join(root, "refute.tar.gz")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, hex.EncodeToString(sum[:])
}
