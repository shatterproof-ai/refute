package toolchain

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type SyncOptions struct {
	ProjectRoot string
	Platform    string
	Arch        string
	Stdout      io.Writer
}

type SyncResult struct {
	Installed bool
	Path      string
	SHA256    string
}

func Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return SyncResult{}, err
		}
	}
	platform, arch := opts.Platform, opts.Arch
	if platform == "" || arch == "" {
		platform, arch = CurrentPlatform()
	}

	lock, err := LoadLock(filepath.Join(root, LockfileName))
	if err != nil {
		return SyncResult{}, err
	}
	artifact, err := SelectArtifact(lock, platform, arch)
	if err != nil {
		return SyncResult{}, err
	}

	active := filepath.Join(root, ActiveBinPath)
	if activeMatches(active, artifact.SHA256) {
		return SyncResult{Installed: false, Path: active, SHA256: artifact.SHA256}, nil
	}

	cacheDir := filepath.Join(root, ToolRoot, "cache", artifact.SHA256)
	cachedBinary := filepath.Join(cacheDir, binaryName())
	if !cacheMatches(cachedBinary, artifact.SHA256) {
		if err := os.RemoveAll(cacheDir); err != nil {
			return SyncResult{}, fmt.Errorf("clear cache %s: %w", cacheDir, err)
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return SyncResult{}, fmt.Errorf("create cache %s: %w", cacheDir, err)
		}
		archivePath := filepath.Join(cacheDir, "artifact")
		if artifact.Filename != "" {
			archivePath = filepath.Join(cacheDir, artifact.Filename)
		}
		if err := download(ctx, artifact.URL, archivePath); err != nil {
			return SyncResult{}, err
		}
		if digest, err := fileDigest(archivePath); err != nil {
			return SyncResult{}, err
		} else if digest != artifact.SHA256 {
			return SyncResult{}, fmt.Errorf("checksum mismatch for %s: got %s, want %s", artifact.URL, digest, artifact.SHA256)
		}
		if err := extractRefuteBinary(archivePath, cachedBinary); err != nil {
			return SyncResult{}, err
		}
		if err := os.WriteFile(cachedBinary+".artifact-sha256", []byte(artifact.SHA256+"\n"), 0o644); err != nil {
			return SyncResult{}, fmt.Errorf("write cache marker: %w", err)
		}
		binarySHA, err := fileDigest(cachedBinary)
		if err != nil {
			return SyncResult{}, fmt.Errorf("checksum cached binary: %w", err)
		}
		if err := os.WriteFile(cachedBinary+".binary-sha256", []byte(binarySHA+"\n"), 0o644); err != nil {
			return SyncResult{}, fmt.Errorf("write cache binary marker: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(active), 0o755); err != nil {
		return SyncResult{}, fmt.Errorf("create active bin dir: %w", err)
	}
	if err := copyFileAtomic(cachedBinary, active); err != nil {
		return SyncResult{}, err
	}
	if err := os.WriteFile(active+".artifact-sha256", []byte(artifact.SHA256+"\n"), 0o644); err != nil {
		return SyncResult{}, fmt.Errorf("write active marker: %w", err)
	}
	binarySHA, err := fileDigest(active)
	if err != nil {
		return SyncResult{}, fmt.Errorf("checksum active binary: %w", err)
	}
	if err := os.WriteFile(active+".binary-sha256", []byte(binarySHA+"\n"), 0o644); err != nil {
		return SyncResult{}, fmt.Errorf("write active binary marker: %w", err)
	}
	return SyncResult{Installed: true, Path: active, SHA256: artifact.SHA256}, nil
}

func binaryName() string {
	if strings.EqualFold(filepath.Ext(ActiveBinPath), ".exe") {
		return "refute.exe"
	}
	return "refute"
}

func download(ctx context.Context, rawURL, dest string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse artifact url %q: %w", rawURL, err)
	}
	switch parsed.Scheme {
	case "file":
		return copyFile(parsed.Path, dest, 0o644)
	case "http", "https":
		client := &http.Client{Timeout: 2 * time.Minute}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("download %s: %w", rawURL, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("download %s: status %s", rawURL, resp.Status)
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		defer out.Close()
		if _, err := io.Copy(out, resp.Body); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported artifact url scheme %q", parsed.Scheme)
	}
}

func extractRefuteBinary(archivePath, dest string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip archive %s: %w", archivePath, err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive %s: %w", archivePath, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(header.Name) != "refute" {
			continue
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return fmt.Errorf("create %s: %w", dest, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return fmt.Errorf("extract %s: %w", header.Name, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("close %s: %w", dest, err)
		}
		return nil
	}
	return fmt.Errorf("archive %s does not contain refute binary", archivePath)
}

func hasDigest(path, want string) bool {
	got, err := fileDigest(path)
	return err == nil && got == want
}

func activeMatches(path, artifactSHA string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return markerMatches(path+".artifact-sha256", artifactSHA) && digestMarkerMatches(path, path+".binary-sha256")
}

func cacheMatches(path, artifactSHA string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return markerMatches(path+".artifact-sha256", artifactSHA) && digestMarkerMatches(path, path+".binary-sha256")
}

func markerMatches(path, artifactSHA string) bool {
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) == artifactSHA
}

func digestMarkerMatches(path, markerPath string) bool {
	got, err := fileDigest(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(markerPath)
	return err == nil && strings.TrimSpace(string(data)) == got
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFileAtomic(src, dest string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".refute-*")
	if err != nil {
		return fmt.Errorf("create temp active binary: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	in, err := os.Open(src)
	if err != nil {
		tmp.Close()
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("copy active binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod active binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close active binary: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("install active binary: %w", err)
	}
	return nil
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dest, err)
	}
	return nil
}
