package lsp

import (
	"io/fs"
	"path/filepath"
	"strings"
)

const maxPrimedFiles = 10

// isTSFamily reports whether the given LSP language ID belongs to the
// TypeScript/JavaScript family served by typescript-language-server.
func isTSFamily(languageID string) bool {
	switch languageID {
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return true
	}
	return false
}

// PrimeTSWorkspace opens up to maxPrimedFiles TypeScript source files in the
// workspace so typescript-language-server initialises its project graph before
// the first rename request arrives. Failures are non-fatal.
func PrimeTSWorkspace(client *Client, root string) error {
	var opened int
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if opened >= maxPrimedFiles {
			return filepath.SkipAll
		}
		var langID string
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ts":
			langID = "typescript"
		case ".tsx":
			langID = "typescriptreact"
		}
		if langID != "" {
			if openErr := client.DidOpen(path, langID); openErr == nil {
				opened++
			}
		}
		return nil
	})
}
