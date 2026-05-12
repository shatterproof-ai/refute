package edit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ApplyResult holds statistics about a completed Apply operation.
type ApplyResult struct {
	FilesModified int
}

type pendingFile struct {
	origPath   string
	tmpPath    string
	backupPath string
	newContent []byte
	mode       os.FileMode
}

type resolvedFileEdit struct {
	requestedPath string
	resolvedPath  string
	edits         []TextEdit
}

type workspaceBoundary struct {
	requestedRoot string
	absRoot       string
	resolvedRoot  string
}

// ApplyWithin applies the WorkspaceEdit only after verifying every edit path is
// within workspaceRoot. Symlinks are resolved before writes: symlinks whose
// targets remain inside the workspace edit the target file, while symlinks that
// resolve outside the workspace are rejected.
func ApplyWithin(we *WorkspaceEdit, workspaceRoot string) (*ApplyResult, error) {
	boundary, err := resolveWorkspaceRoot(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return apply(we, boundary)
}

func apply(we *WorkspaceEdit, boundary workspaceBoundary) (*ApplyResult, error) {
	normalizeCodeActionPlaceholders(we)

	pendingFiles, err := computePendingFiles(we, boundary)
	if err != nil {
		return nil, err
	}
	if err := writePendingTempFiles(pendingFiles); err != nil {
		return nil, err
	}
	if err := commitPendingFiles(pendingFiles); err != nil {
		return nil, err
	}

	return &ApplyResult{FilesModified: len(pendingFiles)}, nil
}

func normalizeCodeActionPlaceholders(we *WorkspaceEdit) {
	if we.FromCodeAction {
		for i := range we.FileEdits {
			for j := range we.FileEdits[i].Edits {
				t := &we.FileEdits[i].Edits[j]
				if HasSnippetPlaceholders(t.NewText) {
					t.NewText = StripSnippetPlaceholders(t.NewText)
				}
			}
		}
	}
}

func computePendingFiles(we *WorkspaceEdit, boundary workspaceBoundary) ([]pendingFile, error) {
	pendingFiles := make([]pendingFile, 0, len(we.FileEdits))
	seen := make(map[string]struct{}, len(we.FileEdits))
	fileEdits, err := resolveFileEdits(we, boundary)
	if err != nil {
		return nil, err
	}

	for _, fe := range fileEdits {
		if _, ok := seen[fe.resolvedPath]; ok {
			return nil, fmt.Errorf("duplicate file edit for %s", fe.requestedPath)
		}
		seen[fe.resolvedPath] = struct{}{}

		content, err := os.ReadFile(fe.resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fe.requestedPath, err)
		}
		info, err := os.Stat(fe.resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", fe.requestedPath, err)
		}
		newContent, err := applyEdits(content, fe.edits)
		if err != nil {
			return nil, fmt.Errorf("apply edits to %s: %w", fe.requestedPath, err)
		}
		pendingFiles = append(pendingFiles, pendingFile{
			origPath:   fe.resolvedPath,
			newContent: newContent,
			mode:       info.Mode(),
		})
	}
	return pendingFiles, nil
}

func resolveFileEdits(we *WorkspaceEdit, boundary workspaceBoundary) ([]resolvedFileEdit, error) {
	fileEdits := make([]resolvedFileEdit, 0, len(we.FileEdits))
	for _, fe := range we.FileEdits {
		resolvedPath := fe.Path
		if boundary.requestedRoot != "" {
			var err error
			resolvedPath, err = resolvePathWithinWorkspace(fe.Path, boundary)
			if err != nil {
				return nil, err
			}
		}
		fileEdits = append(fileEdits, resolvedFileEdit{
			requestedPath: fe.Path,
			resolvedPath:  resolvedPath,
			edits:         fe.Edits,
		})
	}
	return fileEdits, nil
}

func resolveWorkspaceRoot(workspaceRoot string) (workspaceBoundary, error) {
	if workspaceRoot == "" {
		return workspaceBoundary{}, fmt.Errorf("workspace root is required to apply edits within a workspace")
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return workspaceBoundary{}, fmt.Errorf("resolve workspace root %s: %w", workspaceRoot, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return workspaceBoundary{}, fmt.Errorf("resolve workspace root %s: %w", workspaceRoot, err)
	}
	return workspaceBoundary{
		requestedRoot: workspaceRoot,
		absRoot:       filepath.Clean(absRoot),
		resolvedRoot:  filepath.Clean(resolvedRoot),
	}, nil
}

func resolvePathWithinWorkspace(path string, boundary workspaceBoundary) (string, error) {
	if path == "" {
		return "", fmt.Errorf("edit path is empty; expected a file path inside workspace %s", boundary.requestedRoot)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve edit path %s: %w", path, err)
	}
	absPath = filepath.Clean(absPath)
	if !isPathWithin(absPath, boundary.absRoot) && !isPathWithin(absPath, boundary.resolvedRoot) {
		return "", fmt.Errorf("edit path %s is outside workspace %s; refusing to write outside the workspace", path, boundary.requestedRoot)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve edit path %s within workspace %s: %w", path, boundary.requestedRoot, err)
	}
	resolvedPath = filepath.Clean(resolvedPath)
	if !isPathWithin(resolvedPath, boundary.resolvedRoot) {
		return "", fmt.Errorf("edit path %s resolves to %s, which is outside workspace %s; refusing to follow symlink outside the workspace", path, resolvedPath, boundary.requestedRoot)
	}
	return resolvedPath, nil
}

func isPathWithin(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func writePendingTempFiles(pendingFiles []pendingFile) error {
	for i, p := range pendingFiles {
		tmpPath, err := writeTempFile(p.origPath, p.newContent, p.mode)
		if err != nil {
			cleanupPending(pendingFiles[:i])
			return fmt.Errorf("write temp file for %s: %w", p.origPath, err)
		}
		pendingFiles[i].tmpPath = tmpPath
	}
	return nil
}

func commitPendingFiles(pendingFiles []pendingFile) error {
	committed := make([]pendingFile, 0, len(pendingFiles))
	for i := range pendingFiles {
		p := &pendingFiles[i]
		backupPath, err := reserveBackupPath(p.origPath)
		if err != nil {
			rollback(committed)
			cleanupPending(pendingFiles[i:])
			return fmt.Errorf("reserve backup for %s: %w", p.origPath, err)
		}
		p.backupPath = backupPath

		if err := os.Rename(p.origPath, p.backupPath); err != nil {
			rollback(committed)
			cleanupPending(pendingFiles[i:])
			return fmt.Errorf("backup %s -> %s: %w", p.origPath, p.backupPath, err)
		}
		if err := os.Rename(p.tmpPath, p.origPath); err != nil {
			if restoreErr := os.Rename(p.backupPath, p.origPath); restoreErr != nil {
				err = fmt.Errorf("%w; restore %s -> %s: %v", err, p.backupPath, p.origPath, restoreErr)
			}
			rollback(committed)
			cleanupPending(pendingFiles[i:])
			return fmt.Errorf("rename %s -> %s: %w", p.tmpPath, p.origPath, err)
		}
		p.tmpPath = ""
		committed = append(committed, *p)
	}

	for _, p := range committed {
		os.Remove(p.backupPath)
	}
	return nil
}

func writeTempFile(origPath string, content []byte, mode os.FileMode) (string, error) {
	dir := filepath.Dir(origPath)
	base := filepath.Base(origPath)
	f, err := os.CreateTemp(dir, "."+base+".*.refute.tmp")
	if err != nil {
		return "", err
	}
	tmpPath := f.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()
	if _, err = f.Write(content); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	return tmpPath, nil
}

func reserveBackupPath(origPath string) (string, error) {
	dir := filepath.Dir(origPath)
	base := filepath.Base(origPath)
	f, err := os.CreateTemp(dir, "."+base+".*.refute.bak")
	if err != nil {
		return "", err
	}
	backupPath := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(backupPath)
		return "", err
	}
	if err := os.Remove(backupPath); err != nil {
		return "", err
	}
	return backupPath, nil
}

func cleanupPending(files []pendingFile) {
	for _, p := range files {
		if p.tmpPath != "" {
			os.Remove(p.tmpPath)
		}
		if p.backupPath != "" {
			os.Remove(p.backupPath)
		}
	}
}

func rollback(files []pendingFile) {
	for i := len(files) - 1; i >= 0; i-- {
		p := files[i]
		if p.backupPath == "" {
			continue
		}
		os.Remove(p.origPath)
		os.Rename(p.backupPath, p.origPath)
	}
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

	for i := 0; i < len(sorted)-1; i++ {
		later := sorted[i].Range
		earlier := sorted[i+1].Range
		if positionLess(later.Start, earlier.End) {
			return nil, fmt.Errorf("overlapping edits: %+v and %+v", earlier, later)
		}
	}

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

func positionLess(a, b Position) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Character < b.Character
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
