package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// primeFilesCap bounds how many files we'll open during priming.
const primeFilesCap = 500

// skipDirs are directories we never recurse into during Go workspace priming.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	".svn":         true,
	".hg":          true,
}

// PrimeGoWorkspace opens every *.go file under workspaceRoot (skipping
// vendor/node_modules/.git) via DidOpen so gopls can index their packages.
// Stops after primeFilesCap files. Returns the number of files opened and
// any fatal walk error.
//
// DidOpen is fire-and-forget; gopls indexes asynchronously. This function
// issues a zero-result WorkspaceSymbol call at the end to force a round-trip
// and drain the client's notification queue.
func (c *Client) PrimeGoWorkspace(workspaceRoot string) (int, error) {
	opened := 0
	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return nil // skip unreadable paths silently
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if skipDirs[base] || (strings.HasPrefix(base, ".") && base != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if opened >= primeFilesCap {
			return filepath.SkipAll
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		if err := c.DidOpen(path, "go"); err != nil {
			return fmt.Errorf("DidOpen %s: %w", path, err)
		}
		opened++
		return nil
	})
	if err != nil {
		return opened, err
	}
	// Round-trip to drain the notification queue.
	_, _ = c.WorkspaceSymbol("__refute_prime_sentinel__")
	return opened, nil
}
