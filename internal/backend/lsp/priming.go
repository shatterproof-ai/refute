package lsp

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// didOpener is the minimal interface the priming walker needs. Both *Client and
// test fakes satisfy it.
type didOpener interface {
	DidOpen(path, languageID string) error
}

// primeFiles is the single workspace-priming walk, parameterized by a
// language's primingProfile. It opens up to p.fileCap source files whose
// extension is registered in p.extensions (as the mapped languageID), skipping
// p.skipDirs and any dot-prefixed directory. It returns the number of files
// opened.
//
// Priming is best-effort: unreadable paths and individual DidOpen failures are
// skipped rather than aborting the walk. The caller is responsible for any
// post-priming sentinel round-trip (see primingProfile.drainWithSentinel).
func primeFiles(opener didOpener, root string, p primingProfile) int {
	if len(p.extensions) == 0 {
		return 0
	}
	opened := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil // skip unreadable paths silently
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base != "." && (strings.HasPrefix(base, ".") || p.skipDirs[base]) {
				return fs.SkipDir
			}
			return nil
		}
		if opened >= p.fileCap {
			return fs.SkipAll
		}
		langID, ok := p.extensions[strings.ToLower(filepath.Ext(path))]
		if !ok {
			return nil
		}
		if err := opener.DidOpen(path, langID); err == nil {
			opened++
		}
		return nil
	})
	return opened
}
