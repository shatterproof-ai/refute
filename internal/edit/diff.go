package edit

import (
	"fmt"
	"os"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// RenderDiff computes a unified diff for each FileEdit in we and returns all
// diffs joined with newlines. Returns an empty string when we contains no
// FileEdits.
func RenderDiff(we *WorkspaceEdit) (string, error) {
	if len(we.FileEdits) == 0 {
		return "", nil
	}

	var parts []string

	for _, fe := range we.FileEdits {
		original, err := os.ReadFile(fe.Path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", fe.Path, err)
		}

		modified, err := applyEdits(original, fe.Edits)
		if err != nil {
			return "", fmt.Errorf("compute diff for %s: %w", fe.Path, err)
		}

		diff := difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(original)),
			B:        difflib.SplitLines(string(modified)),
			FromFile: fe.Path,
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

	return strings.Join(parts, "\n"), nil
}
