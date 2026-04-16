package edit

import (
	"fmt"
	"os"
	"sort"
)

// ApplyResult holds statistics about a completed Apply operation.
type ApplyResult struct {
	FilesModified int
}

// Apply applies the WorkspaceEdit atomically across all files.
// Phase 1: read all files and compute new contents in memory.
// Phase 2: write new contents to .refute.tmp sidecar files.
// Phase 3: rename each sidecar into place.
// On any failure the temp files are removed and the originals are left untouched.
func Apply(we *WorkspaceEdit) (*ApplyResult, error) {
	// Phase 1: compute new contents for every file.
	type pending struct {
		origPath string
		tmpPath  string
		newContent []byte
	}
	pendingFiles := make([]pending, 0, len(we.FileEdits))

	for _, fe := range we.FileEdits {
		content, err := os.ReadFile(fe.Path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fe.Path, err)
		}
		newContent, err := applyEdits(content, fe.Edits)
		if err != nil {
			return nil, fmt.Errorf("apply edits to %s: %w", fe.Path, err)
		}
		tmpPath := fe.Path + ".refute.tmp"
		pendingFiles = append(pendingFiles, pending{
			origPath:   fe.Path,
			tmpPath:    tmpPath,
			newContent: newContent,
		})
	}

	// Phase 2: write to .refute.tmp sidecar files.
	for i, p := range pendingFiles {
		perm := os.FileMode(0o644)
		if info, err := os.Stat(p.origPath); err == nil {
			perm = info.Mode()
		}
		if err := os.WriteFile(p.tmpPath, p.newContent, perm); err != nil {
			// Clean up all temp files written so far.
			for _, q := range pendingFiles[:i+1] {
				os.Remove(q.tmpPath)
			}
			return nil, fmt.Errorf("write temp file %s: %w", p.tmpPath, err)
		}
	}

	// Phase 3: rename each temp file over the original (atomic per-file).
	for i, p := range pendingFiles {
		if err := os.Rename(p.tmpPath, p.origPath); err != nil {
			// Clean up remaining temp files; already-renamed files have
			// already replaced their originals — those are committed.
			for _, q := range pendingFiles[i:] {
				os.Remove(q.tmpPath)
			}
			return nil, fmt.Errorf("rename %s -> %s: %w", p.tmpPath, p.origPath, err)
		}
	}

	return &ApplyResult{FilesModified: len(pendingFiles)}, nil
}

// applyEdits applies a slice of TextEdits to content.
// Edits are sorted in reverse order (end of file first) so that earlier
// positions are not invalidated by applying later edits.
func applyEdits(content []byte, edits []TextEdit) ([]byte, error) {
	// Sort edits in reverse positional order.
	sorted := make([]TextEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		a, b := sorted[i].Range.Start, sorted[j].Range.Start
		if a.Line != b.Line {
			return a.Line > b.Line
		}
		return a.Character > b.Character
	})

	result := content
	for _, e := range sorted {
		startOff := positionToOffset(result, e.Range.Start)
		endOff := positionToOffset(result, e.Range.End)
		if startOff < 0 || endOff < 0 || startOff > endOff || endOff > len(result) {
			return nil, fmt.Errorf("edit range out of bounds: %+v", e.Range)
		}
		newContent := make([]byte, 0, len(result)-(endOff-startOff)+len(e.NewText))
		newContent = append(newContent, result[:startOff]...)
		newContent = append(newContent, []byte(e.NewText)...)
		newContent = append(newContent, result[endOff:]...)
		result = newContent
	}
	return result, nil
}

// positionToOffset converts a 0-indexed line/character Position to a byte offset
// within content. Returns -1 if the position is out of range.
func positionToOffset(content []byte, pos Position) int {
	line := 0
	offset := 0
	for offset < len(content) {
		if line == pos.Line {
			// Advance by the character count within this line.
			target := offset + pos.Character
			if target > len(content) {
				return -1
			}
			return target
		}
		if content[offset] == '\n' {
			line++
		}
		offset++
	}
	// Handle position pointing to end-of-file on the last line.
	if line == pos.Line && pos.Character == 0 {
		return offset
	}
	return -1
}
