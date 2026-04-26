package lsp

import (
	"io/fs"
	"path/filepath"
	"strings"
)

const maxPrimedFiles = 10

func shouldPrimeWorkspace(languageID string) bool {
	switch languageID {
	case "typescript", "typescriptreact", "javascript", "javascriptreact":
		return true
	case "rust":
		return true
	}
	return false
}

// PrimeWorkspace opens up to maxPrimedFiles source files for languages that
// benefit from a warmer project graph before the first rename request arrives.
// Failures are non-fatal.
func PrimeWorkspace(client *Client, root string, languageID string) error {
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
		case ".js":
			langID = "javascript"
		case ".jsx":
			langID = "javascriptreact"
		case ".rs":
			langID = "rust"
		}
		if langID == languageID {
			if openErr := client.DidOpen(path, langID); openErr == nil {
				opened++
			}
		}
		return nil
	})
}
