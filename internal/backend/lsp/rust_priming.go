package lsp

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// didOpener is the minimal interface PrimeRustWorkspace needs. Both *Client
// and test fakes satisfy it.
type didOpener interface {
	DidOpen(path, languageID string) error
}

// PrimeRustWorkspace opens up to maxPrimedFiles Rust source files in the
// workspace so rust-analyzer begins indexing before the first rename or code-
// action request arrives. Failures to open individual files are non-fatal.
func PrimeRustWorkspace(client didOpener, root string) error {
	var opened int
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "target" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if opened >= maxPrimedFiles {
			return filepath.SkipAll
		}
		if strings.ToLower(filepath.Ext(path)) != ".rs" {
			return nil
		}
		if openErr := client.DidOpen(path, "rust"); openErr == nil {
			opened++
		}
		return nil
	})
}
