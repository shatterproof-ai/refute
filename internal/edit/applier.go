package edit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Crash-safety note: commits use a write-temp-then-rename strategy with a
// per-file backup, but do not fsync the temp file or its parent directory
// before renaming. A power loss or kernel crash mid-commit can therefore leave
// a truncated file even though the rename appears atomic. This is an accepted
// limitation for now; a durable-commit redesign is tracked separately and is
// out of scope here.

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

	// File operations (create/rename/delete) require the ordered, journalled
	// transaction. The text-edits-only path keeps its original batched
	// staging untouched.
	if len(we.FileOps) > 0 {
		return applyWithFileOps(we, boundary)
	}

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
			rbErr := rollback(committed)
			cleanupPending(pendingFiles[i:])
			return errors.Join(fmt.Errorf("reserve backup for %s: %w", p.origPath, err), rbErr)
		}
		p.backupPath = backupPath

		if err := os.Rename(p.origPath, p.backupPath); err != nil {
			rbErr := rollback(committed)
			cleanupPending(pendingFiles[i:])
			return errors.Join(fmt.Errorf("backup %s -> %s: %w", p.origPath, p.backupPath, err), rbErr)
		}
		if err := os.Rename(p.tmpPath, p.origPath); err != nil {
			if restoreErr := os.Rename(p.backupPath, p.origPath); restoreErr != nil {
				err = fmt.Errorf("%w; restore %s -> %s: %w", err, p.backupPath, p.origPath, restoreErr)
			}
			rbErr := rollback(committed)
			cleanupPending(pendingFiles[i:])
			return errors.Join(fmt.Errorf("rename %s -> %s: %w", p.tmpPath, p.origPath, err), rbErr)
		}
		p.tmpPath = ""
		committed = append(committed, *p)
	}

	// All files committed: remove their backups. Surface (rather than
	// silently discard) any leftover .refute.bak files so the user can clean
	// them up.
	var removeErrs []error
	for _, p := range committed {
		if err := os.Remove(p.backupPath); err != nil && !os.IsNotExist(err) {
			removeErrs = append(removeErrs, fmt.Errorf("remove leftover backup %s: %w", p.backupPath, err))
		}
	}
	return errors.Join(removeErrs...)
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

// rollback restores already-committed files from their backups, in reverse
// order. If a restore fails the workspace is left inconsistent; rollback
// returns a joined error naming each file it could not restore and the
// leftover .refute.bak backup that remains, so callers can surface them
// instead of silently leaving a corrupted workspace.
func rollback(files []pendingFile) error {
	var errs []error
	for i := len(files) - 1; i >= 0; i-- {
		p := files[i]
		if p.backupPath == "" {
			continue
		}
		_ = os.Remove(p.origPath)
		if err := os.Rename(p.backupPath, p.origPath); err != nil {
			errs = append(errs, fmt.Errorf(
				"rollback failed to restore %s from backup %s: %w; %s is left inconsistent and backup %s remains",
				p.origPath, p.backupPath, err, p.origPath, p.backupPath))
		}
	}
	return errors.Join(errs...)
}

// applyEdits applies a slice of TextEdits to content.
// Edits are sorted in reverse order (end of file first) so that earlier
// positions are not invalidated by applying later edits.
func applyEdits(content []byte, edits []TextEdit) ([]byte, error) {
	// Sort edits in reverse positional order so applying a later edit cannot
	// invalidate the offsets of an earlier one. Track the original array
	// index and use a stable sort with an index tiebreak so that edits
	// sharing a position (e.g. several zero-width inserts) apply
	// deterministically. Under reverse application, a later array element
	// inserted first ends up nearer the start, so to honor the LSP rule that
	// same-position inserts appear in array order we break ties by original
	// index descending.
	type indexedEdit struct {
		edit TextEdit
		orig int
	}
	sorted := make([]indexedEdit, len(edits))
	for i, e := range edits {
		sorted[i] = indexedEdit{edit: e, orig: i}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].edit.Range.Start, sorted[j].edit.Range.Start
		if a.Line != b.Line {
			return a.Line > b.Line
		}
		if a.Character != b.Character {
			return a.Character > b.Character
		}
		return sorted[i].orig > sorted[j].orig
	})

	for i := 0; i < len(sorted)-1; i++ {
		later := sorted[i].edit.Range
		earlier := sorted[i+1].edit.Range
		if positionLess(later.Start, earlier.End) {
			return nil, fmt.Errorf("overlapping edits: %+v and %+v", earlier, later)
		}
	}

	result := content
	for _, ie := range sorted {
		e := ie.edit
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
	if pos.Line < 0 || pos.Character < 0 {
		return -1
	}
	line := 0
	offset := 0
	for offset < len(content) {
		if line == pos.Line {
			// Bound the in-line advance to this line. The line ends at the
			// next '\n' (or EOF); Character may point at that terminating
			// newline (offset+Character == lineEnd) but must not spill past
			// it into a following line.
			lineEnd := offset
			for lineEnd < len(content) && content[lineEnd] != '\n' {
				lineEnd++
			}
			target := offset + pos.Character
			if target > lineEnd {
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
