package telemetry

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// SnapshotManifest describes compressed before/after files for an invocation.
type SnapshotManifest struct {
	Path          string         `json:"path"`
	SchemaVersion string         `json:"schemaVersion"`
	InvocationID  string         `json:"invocationId"`
	WorkspaceRoot string         `json:"workspaceRoot,omitempty"`
	Compression   string         `json:"compression"`
	Files         []SnapshotFile `json:"files"`
}

// SnapshotFile is one touched file in a snapshot manifest.
type SnapshotFile struct {
	File                  string `json:"file"`
	RelativePath          string `json:"relativePath,omitempty"`
	BeforePath            string `json:"beforePath"`
	AfterPath             string `json:"afterPath"`
	BeforeBytes           int64  `json:"beforeBytes"`
	AfterBytes            int64  `json:"afterBytes"`
	BeforeCompressedBytes int64  `json:"beforeCompressedBytes"`
	AfterCompressedBytes  int64  `json:"afterCompressedBytes"`
	BeforeSHA256          string `json:"beforeSha256"`
	AfterSHA256           string `json:"afterSha256"`
	ActualAfterSHA256     string `json:"actualAfterSha256,omitempty"`
	ActualMatchesPlanned  *bool  `json:"actualMatchesPlanned,omitempty"`
}

func writeSnapshot(root, invocationID, workspaceRoot string, we *edit.WorkspaceEdit) (*SnapshotManifest, error) {
	if root == "" {
		return nil, nil
	}
	previews, err := edit.PreviewWithin(we, workspaceRoot)
	if err != nil {
		return nil, err
	}
	snapshotDir := filepath.Join(root, invocationID)
	beforeDir := filepath.Join(snapshotDir, "before")
	afterDir := filepath.Join(snapshotDir, "after")
	if err := os.MkdirAll(beforeDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(afterDir, 0o755); err != nil {
		return nil, err
	}

	manifest := &SnapshotManifest{
		Path:          filepath.Join(snapshotDir, "manifest.json"),
		SchemaVersion: SchemaVersion,
		InvocationID:  invocationID,
		WorkspaceRoot: workspaceRoot,
		Compression:   "gzip",
		Files:         make([]SnapshotFile, 0, len(previews)),
	}
	for i, preview := range previews {
		name := snapshotFileName(i, preview.ResolvedPath)
		beforeRel := filepath.Join("before", name)
		afterRel := filepath.Join("after", name)
		beforeCompressed, err := writeGzip(filepath.Join(snapshotDir, beforeRel), preview.Before)
		if err != nil {
			return nil, err
		}
		afterCompressed, err := writeGzip(filepath.Join(snapshotDir, afterRel), preview.After)
		if err != nil {
			return nil, err
		}
		manifest.Files = append(manifest.Files, SnapshotFile{
			File:                  preview.ResolvedPath,
			RelativePath:          relativePath(workspaceRoot, preview.ResolvedPath),
			BeforePath:            beforeRel,
			AfterPath:             afterRel,
			BeforeBytes:           int64(len(preview.Before)),
			AfterBytes:            int64(len(preview.After)),
			BeforeCompressedBytes: beforeCompressed,
			AfterCompressedBytes:  afterCompressed,
			BeforeSHA256:          sha256Hex(preview.Before),
			AfterSHA256:           sha256Hex(preview.After),
		})
	}
	if err := writeManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func finalizeSnapshotActual(manifest *SnapshotManifest) error {
	if manifest == nil {
		return nil
	}
	for i := range manifest.Files {
		data, err := os.ReadFile(manifest.Files[i].File)
		if err != nil {
			return err
		}
		actual := sha256Hex(data)
		matches := actual == manifest.Files[i].AfterSHA256
		manifest.Files[i].ActualAfterSHA256 = actual
		manifest.Files[i].ActualMatchesPlanned = &matches
	}
	return writeManifest(manifest)
}

func writeManifest(manifest *SnapshotManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifest.Path, append(data, '\n'), 0o644)
}

func writeGzip(path string, data []byte) (int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	zw := gzip.NewWriter(f)
	_, writeErr := zw.Write(data)
	closeErr := zw.Close()
	fileCloseErr := f.Close()
	if writeErr != nil {
		return 0, writeErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	if fileCloseErr != nil {
		return 0, fileCloseErr
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func snapshotFileName(index int, path string) string {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "file"
	}
	return fmt.Sprintf("%04d-%s.gz", index+1, safePathPart(base))
}

func relativePath(root, path string) string {
	if root == "" {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return ""
	}
	return rel
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
