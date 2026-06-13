package toolchain

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	if err := validateArtifactForSync(artifact); err != nil {
		return SyncResult{}, err
	}

	toolRoot := filepath.Join(root, ToolRoot)
	if err := ensureRealDirectory(toolRoot); err != nil {
		return SyncResult{}, err
	}
	cacheRoot := filepath.Join(toolRoot, "cache")
	if err := ensureRealDirectory(cacheRoot); err != nil {
		return SyncResult{}, err
	}

	active := filepath.Join(root, ActiveBinPath)
	activeDir := filepath.Dir(active)
	if err := ensureRealDirectory(activeDir); err != nil {
		return SyncResult{}, err
	}
	if activeMatches(active, artifact.SHA256) {
		return SyncResult{Installed: false, Path: active, SHA256: artifact.SHA256}, nil
	}

	cacheDir, err := pathUnder(cacheRoot, artifact.SHA256)
	if err != nil {
		return SyncResult{}, err
	}
	cachedBinary, err := pathUnder(cacheDir, binaryName())
	if err != nil {
		return SyncResult{}, err
	}
	if !cacheMatches(cachedBinary, artifact.SHA256) {
		if err := os.RemoveAll(cacheDir); err != nil {
			return SyncResult{}, fmt.Errorf("clear cache %s: %w", cacheDir, err)
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return SyncResult{}, fmt.Errorf("create cache %s: %w", cacheDir, err)
		}
		archivePath := filepath.Join(cacheDir, "artifact")
		if artifact.Filename != "" {
			archivePath, err = pathUnder(cacheDir, artifact.Filename)
			if err != nil {
				return SyncResult{}, err
			}
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
		if err := writeFileAtomic(cachedBinary+".artifact-sha256", []byte(artifact.SHA256+"\n"), 0o644); err != nil {
			return SyncResult{}, fmt.Errorf("write cache marker: %w", err)
		}
		binarySHA, err := fileDigest(cachedBinary)
		if err != nil {
			return SyncResult{}, fmt.Errorf("checksum cached binary: %w", err)
		}
		if err := writeFileAtomic(cachedBinary+".binary-sha256", []byte(binarySHA+"\n"), 0o644); err != nil {
			return SyncResult{}, fmt.Errorf("write cache binary marker: %w", err)
		}
	}

	if err := copyFileAtomic(cachedBinary, active); err != nil {
		return SyncResult{}, err
	}
	if err := writeFileAtomic(active+".artifact-sha256", []byte(artifact.SHA256+"\n"), 0o644); err != nil {
		return SyncResult{}, fmt.Errorf("write active marker: %w", err)
	}
	binarySHA, err := fileDigest(active)
	if err != nil {
		return SyncResult{}, fmt.Errorf("checksum active binary: %w", err)
	}
	if err := writeFileAtomic(active+".binary-sha256", []byte(binarySHA+"\n"), 0o644); err != nil {
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

func extractRefuteBinary(archivePath, dest string) (err error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", archivePath, err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("read gzip archive %s: %w", archivePath, err)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close gzip archive %s: %w", archivePath, closeErr)
		}
	}()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive %s: %w", archivePath, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if tarMemberBase(header.Name) != "refute" {
			continue
		}
		if !safeTarMemberName(header.Name) {
			return fmt.Errorf("archive %s contains unsafe refute member %q", archivePath, header.Name)
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

func safeTarMemberName(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || hasWindowsDrivePrefix(name) {
		return false
	}
	for _, part := range tarMemberParts(name) {
		if part == ".." {
			return false
		}
	}
	return true
}

func tarMemberBase(name string) string {
	parts := tarMemberParts(name)
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func tarMemberParts(name string) []string {
	return strings.FieldsFunc(name, func(r rune) bool {
		return r == '/' || r == '\\'
	})
}

func validateArtifactForSync(artifact Artifact) error {
	if !validSHA256Hex(artifact.SHA256) {
		return fmt.Errorf("invalid artifact sha256 %q", artifact.SHA256)
	}
	if artifact.Filename != "" && !safeLockFilename(artifact.Filename) {
		return fmt.Errorf("unsafe artifact filename %q", artifact.Filename)
	}
	return nil
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, char := range value {
		if !('0' <= char && char <= '9') && !('a' <= char && char <= 'f') && !('A' <= char && char <= 'F') {
			return false
		}
	}
	return true
}

func safeLockFilename(name string) bool {
	return name != "" &&
		!strings.Contains(name, "/") &&
		!strings.Contains(name, `\`) &&
		!strings.Contains(name, "..") &&
		!hasWindowsDrivePrefix(name)
}

func hasWindowsDrivePrefix(name string) bool {
	return len(name) >= 2 && name[1] == ':' && (('A' <= name[0] && name[0] <= 'Z') || ('a' <= name[0] && name[0] <= 'z'))
}

func pathUnder(root string, child string) (string, error) {
	if filepath.IsAbs(child) || hasWindowsDrivePrefix(child) {
		return "", fmt.Errorf("path %s escapes %s", child, root)
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", root, err)
	}
	candidate := filepath.Join(root, child)
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", candidate, err)
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return "", fmt.Errorf("compare %s to %s: %w", candidateAbs, rootAbs, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %s escapes %s", candidate, root)
	}
	return candidateAbs, nil
}

func ensureRealDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s is not a real directory", path)
	}
	return nil
}

func hasDigest(path, want string) bool {
	got, err := fileDigest(path)
	return err == nil && got == want
}

func activeMatches(path, artifactSHA string) bool {
	if !isRegularNonSymlink(path) {
		return false
	}
	return markerMatches(path+".artifact-sha256", artifactSHA) && digestMarkerMatches(path, path+".binary-sha256")
}

func cacheMatches(path, artifactSHA string) bool {
	if !isRegularNonSymlink(path) {
		return false
	}
	return markerMatches(path+".artifact-sha256", artifactSHA) && digestMarkerMatches(path, path+".binary-sha256")
}

func markerMatches(path, artifactSHA string) bool {
	if !isRegularNonSymlink(path) {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) == artifactSHA
}

func digestMarkerMatches(path, markerPath string) bool {
	if !isRegularNonSymlink(markerPath) {
		return false
	}
	got, err := fileDigest(path)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(markerPath)
	return err == nil && strings.TrimSpace(string(data)) == got
}

func isRegularNonSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
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

func writeFileAtomic(dest string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".refute-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, dest); err != nil {
		return fmt.Errorf("install file: %w", err)
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
