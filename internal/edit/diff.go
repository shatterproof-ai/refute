package edit

import (
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// RenderDiff computes a unified diff for each FileEdit in we, plus summary
// lines for any create/rename/delete file operations, and returns them joined
// with newlines. Returns an empty string when we contains neither edits nor
// file operations.
//
// Because file operations are not applied in dry-run mode, a text edit that
// targets a file scheduled for creation is diffed against empty content rather
// than read from disk.
func RenderDiff(we *WorkspaceEdit) (string, error) {
	if len(we.FileEdits) == 0 && len(we.FileOps) == 0 {
		return "", nil
	}

	createTargets := make(map[string]struct{}, len(we.FileOps))
	for _, op := range we.FileOps {
		if op.Kind == FileOpCreate {
			createTargets[op.Path] = struct{}{}
		}
	}

	var parts []string

	for _, fe := range orderedFileEdits(we.FileEdits) {
		var original []byte
		fromFile := fe.Path
		if _, isNew := createTargets[fe.Path]; isNew {
			fromFile = "/dev/null"
		} else {
			data, err := os.ReadFile(fe.Path)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", fe.Path, err)
			}
			original = data
		}

		modified, err := applyEdits(original, fe.Edits)
		if err != nil {
			return "", fmt.Errorf("compute diff for %s: %w", fe.Path, err)
		}

		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(original)),
			B:        difflib.SplitLines(string(modified)),
			FromFile: fromFile,
			ToFile:   fe.Path,
			Context:  3,
		}

		text, err := difflib.GetUnifiedDiffString(diff)
		if err != nil {
			return "", fmt.Errorf("generate unified diff for %s: %w", fe.Path, err)
		}

		if text != "" {
			parts = append(parts, text)
		}
	}

	if ops := renderFileOps(we.FileOps); ops != "" {
		parts = append(parts, ops)
	}

	return strings.Join(parts, "\n"), nil
}

// renderFileOps produces a short human-readable summary of the create/rename/
// delete operations in a WorkspaceEdit. A create is usually also shown as a
// /dev/null diff above; the summary line makes the operation explicit.
func renderFileOps(ops []FileOperation) string {
	if len(ops) == 0 {
		return ""
	}
	var lines []string
	for _, op := range ops {
		switch op.Kind {
		case FileOpCreate:
			lines = append(lines, fmt.Sprintf("create %s", op.Path))
		case FileOpRename:
			lines = append(lines, fmt.Sprintf("rename %s -> %s", op.Path, op.NewPath))
		case FileOpDelete:
			lines = append(lines, fmt.Sprintf("delete %s", op.Path))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}
