package edit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// committedOp records how to undo a single applied operation on rollback, and
// how to finalize it (e.g. discard a backup) once the whole batch succeeds.
type committedOp struct {
	undo     func() error
	finalize func() error
}

// applyWithFileOps applies a WorkspaceEdit that includes create/rename/delete
// file operations alongside text edits. Operations run in a fixed order —
// creates, then text edits, then renames, then deletes — and every applied
// step records an undo, so any failure rolls the whole batch back, leaving the
// workspace as it was. On success, per-op backups are discarded.
func applyWithFileOps(we *WorkspaceEdit, boundary workspaceBoundary) (*ApplyResult, error) {
	var journal []committedOp
	rollback := func() error {
		var errs []error
		for i := len(journal) - 1; i >= 0; i-- {
			if journal[i].undo == nil {
				continue
			}
			if err := journal[i].undo(); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
	fail := func(opErr error) (*ApplyResult, error) {
		if rbErr := rollback(); rbErr != nil {
			return nil, errors.Join(opErr, rbErr)
		}
		return nil, opErr
	}

	filesTouched := 0

	// 1. Creates — so a subsequent text edit can populate a new file.
	for _, op := range we.FileOps {
		if op.Kind != FileOpCreate {
			continue
		}
		target, err := resolveNewPathWithinWorkspace(op.Path, boundary)
		if err != nil {
			return fail(err)
		}
		c, err := commitCreate(target, op)
		if err != nil {
			return fail(fmt.Errorf("create %s: %w", op.Path, err))
		}
		if c != nil {
			journal = append(journal, *c)
			filesTouched++
		}
	}

	// 2. Text edits.
	fileEdits, err := resolveFileEdits(we, boundary)
	if err != nil {
		return fail(err)
	}
	seen := make(map[string]struct{}, len(fileEdits))
	for _, fe := range fileEdits {
		if _, ok := seen[fe.resolvedPath]; ok {
			return fail(fmt.Errorf("duplicate file edit for %s", fe.requestedPath))
		}
		seen[fe.resolvedPath] = struct{}{}

		content, err := os.ReadFile(fe.resolvedPath)
		if err != nil {
			return fail(fmt.Errorf("read %s: %w", fe.requestedPath, err))
		}
		info, err := os.Stat(fe.resolvedPath)
		if err != nil {
			return fail(fmt.Errorf("stat %s: %w", fe.requestedPath, err))
		}
		newContent, err := applyEdits(content, fe.edits)
		if err != nil {
			return fail(fmt.Errorf("apply edits to %s: %w", fe.requestedPath, err))
		}
		c, err := commitTextEdit(fe.resolvedPath, newContent, info.Mode())
		if err != nil {
			return fail(fmt.Errorf("write %s: %w", fe.requestedPath, err))
		}
		journal = append(journal, *c)
		filesTouched++
	}

	// 3. Renames.
	for _, op := range we.FileOps {
		if op.Kind != FileOpRename {
			continue
		}
		src, err := resolvePathWithinWorkspace(op.Path, boundary)
		if err != nil {
			return fail(err)
		}
		dst, err := resolveNewPathWithinWorkspace(op.NewPath, boundary)
		if err != nil {
			return fail(err)
		}
		c, err := commitRename(src, dst, op)
		if err != nil {
			return fail(fmt.Errorf("rename %s -> %s: %w", op.Path, op.NewPath, err))
		}
		if c != nil {
			journal = append(journal, *c)
			filesTouched++
		}
	}

	// 4. Deletes.
	for _, op := range we.FileOps {
		if op.Kind != FileOpDelete {
			continue
		}
		target, err := resolveNewPathWithinWorkspace(op.Path, boundary)
		if err != nil {
			return fail(err)
		}
		c, err := commitDelete(target, op)
		if err != nil {
			return fail(fmt.Errorf("delete %s: %w", op.Path, err))
		}
		if c != nil {
			journal = append(journal, *c)
			filesTouched++
		}
	}

	// All steps applied: discard backups. Surface (rather than silently drop)
	// any leftover backup that could not be removed.
	var finalizeErrs []error
	for _, c := range journal {
		if c.finalize == nil {
			continue
		}
		if err := c.finalize(); err != nil {
			finalizeErrs = append(finalizeErrs, err)
		}
	}
	return &ApplyResult{FilesModified: filesTouched}, errors.Join(finalizeErrs...)
}

// commitCreate creates an empty file at target. A nil committedOp means the
// operation was a no-op (the file already existed and IgnoreIfExists was set).
func commitCreate(target string, op FileOperation) (*committedOp, error) {
	_, statErr := os.Stat(target)
	exists := statErr == nil
	if exists {
		switch {
		case op.Overwrite:
			backup, err := reserveBackupPath(target)
			if err != nil {
				return nil, err
			}
			if err := os.Rename(target, backup); err != nil {
				return nil, err
			}
			if err := os.WriteFile(target, nil, 0o644); err != nil {
				_ = os.Rename(backup, target)
				return nil, err
			}
			return &committedOp{
				undo: func() error {
					_ = os.Remove(target)
					return os.Rename(backup, target)
				},
				finalize: func() error { return os.Remove(backup) },
			}, nil
		case op.IgnoreIfExists:
			// The create is skipped because the file already exists. Note that
			// the parser converts a same-batch edit to a create target against
			// an empty document; that assumption only holds for genuinely new
			// files (the gopls "extract to new file" shape), not for this
			// skipped-existing case. convertEditsForNewFile guards against
			// corruption by rejecting any edit referencing a line other than 0.
			return nil, nil
		default:
			return nil, fmt.Errorf("file already exists")
		}
	}

	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(target)
		return nil, err
	}
	return &committedOp{undo: func() error { return os.Remove(target) }}, nil
}

// commitTextEdit writes newContent to path via the same temp-then-rename with
// per-file backup used by the text-edits-only path, returning an undo that
// restores the original from the backup.
func commitTextEdit(path string, newContent []byte, mode os.FileMode) (*committedOp, error) {
	tmpPath, err := writeTempFile(path, newContent, mode)
	if err != nil {
		return nil, err
	}
	backupPath, err := reserveBackupPath(path)
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(path, backupPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backupPath, path)
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return &committedOp{
		undo: func() error {
			_ = os.Remove(path)
			return os.Rename(backupPath, path)
		},
		finalize: func() error { return os.Remove(backupPath) },
	}, nil
}

// commitRename moves src to dst. A nil committedOp means the rename was skipped
// (dst existed and IgnoreIfExists was set without Overwrite).
func commitRename(src, dst string, op FileOperation) (*committedOp, error) {
	_, statErr := os.Stat(dst)
	dstExists := statErr == nil
	var dstBackup string
	if dstExists {
		switch {
		case op.Overwrite:
			b, err := reserveBackupPath(dst)
			if err != nil {
				return nil, err
			}
			if err := os.Rename(dst, b); err != nil {
				return nil, err
			}
			dstBackup = b
		case op.IgnoreIfExists:
			return nil, nil
		default:
			return nil, fmt.Errorf("destination %s already exists", dst)
		}
	}

	if err := os.Rename(src, dst); err != nil {
		if dstBackup != "" {
			_ = os.Rename(dstBackup, dst)
		}
		return nil, err
	}
	return &committedOp{
		undo: func() error {
			var errs []error
			if err := os.Rename(dst, src); err != nil {
				errs = append(errs, err)
			}
			if dstBackup != "" {
				if err := os.Rename(dstBackup, dst); err != nil {
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		},
		finalize: func() error {
			if dstBackup != "" {
				return os.RemoveAll(dstBackup)
			}
			return nil
		},
	}, nil
}

// commitDelete removes target by moving it aside to a backup, so the delete can
// be undone on rollback. A nil committedOp means the delete was skipped (the
// file was absent and IgnoreIfNotExists was set).
func commitDelete(target string, op FileOperation) (*committedOp, error) {
	if _, statErr := os.Stat(target); statErr != nil {
		if os.IsNotExist(statErr) {
			if op.IgnoreIfNotExists {
				return nil, nil
			}
			return nil, fmt.Errorf("file does not exist")
		}
		return nil, statErr
	}
	backup, err := reserveBackupPath(target)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(target, backup); err != nil {
		return nil, err
	}
	return &committedOp{
		undo: func() error {
			_ = os.RemoveAll(target)
			return os.Rename(backup, target)
		},
		finalize: func() error { return os.RemoveAll(backup) },
	}, nil
}

// resolveNewPathWithinWorkspace validates a path that need not exist yet (a
// create or rename destination, or a possibly-absent delete target). It checks
// the lexical path is inside the workspace and that the parent directory
// resolves inside the workspace, guarding against symlinked parents escaping
// the boundary.
func resolveNewPathWithinWorkspace(path string, boundary workspaceBoundary) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file op path is empty; expected a path inside workspace %s", boundary.requestedRoot)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve file op path %s: %w", path, err)
	}
	absPath = filepath.Clean(absPath)
	if !isPathWithin(absPath, boundary.absRoot) && !isPathWithin(absPath, boundary.resolvedRoot) {
		return "", fmt.Errorf("file op path %s is outside workspace %s; refusing to write outside the workspace", path, boundary.requestedRoot)
	}

	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absPath))
	if err != nil {
		return "", fmt.Errorf("resolve parent of %s within workspace %s: %w", path, boundary.requestedRoot, err)
	}
	resolvedParent = filepath.Clean(resolvedParent)
	if !isPathWithin(resolvedParent, boundary.resolvedRoot) {
		return "", fmt.Errorf("file op path %s parent resolves to %s, outside workspace %s; refusing to follow symlink outside the workspace", path, resolvedParent, boundary.requestedRoot)
	}
	return filepath.Join(resolvedParent, filepath.Base(absPath)), nil
}
