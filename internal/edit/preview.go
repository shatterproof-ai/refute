package edit

import (
	"fmt"
	"os"
)

// FilePreview contains the original and planned content for one file edit.
type FilePreview struct {
	RequestedPath string
	ResolvedPath  string
	Before        []byte
	After         []byte
}

// PreviewWithin computes the same per-file content transformation as
// ApplyWithin without writing to disk.
func PreviewWithin(we *WorkspaceEdit, workspaceRoot string) ([]FilePreview, error) {
	boundary, err := resolveWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	normalizeCodeActionPlaceholders(we)

	fileEdits, err := resolveFileEdits(we, boundary)
	if err != nil {
		return nil, err
	}
	previews := make([]FilePreview, 0, len(fileEdits))
	seen := make(map[string]struct{}, len(fileEdits))
	for _, fe := range fileEdits {
		if _, ok := seen[fe.resolvedPath]; ok {
			return nil, fmt.Errorf("duplicate file edit for %s", fe.requestedPath)
		}
		seen[fe.resolvedPath] = struct{}{}

		before, err := os.ReadFile(fe.resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fe.requestedPath, err)
		}
		after, err := applyEdits(before, fe.edits)
		if err != nil {
			return nil, fmt.Errorf("apply edits to %s: %w", fe.requestedPath, err)
		}
		previews = append(previews, FilePreview{
			RequestedPath: fe.requestedPath,
			ResolvedPath:  fe.resolvedPath,
			Before:        before,
			After:         after,
		})
	}
	return previews, nil
}
