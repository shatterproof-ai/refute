package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

var workspaceMarkers = []string{
	"go.mod", "go.work", "Cargo.toml", "Cargo.lock",
	"package.json", "tsconfig.json", "pyproject.toml", "setup.py",
}

// FindWorkspaceRootFromDir walks up from dir to find a directory containing
// any workspaceMarker. Returns dir if no marker is found before the filesystem
// root.
func FindWorkspaceRootFromDir(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", dir, err)
	}
	cur := absDir
	for {
		for _, m := range workspaceMarkers {
			if _, err := os.Stat(filepath.Join(cur, m)); err == nil {
				return cur, nil
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return absDir, nil
		}
		cur = parent
	}
}

// FindWorkspaceRootFromFile starts the walk from the directory containing
// filePath.
func FindWorkspaceRootFromFile(filePath string) (string, error) {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", filePath, err)
	}
	return FindWorkspaceRootFromDir(filepath.Dir(abs))
}

// DetectServerKey returns the server config key for a file based on its
// extension. Used to look up the language server in the config.
func DetectServerKey(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}

// DetectLanguageID returns the LSP language ID for a file based on its
// extension. Passed to the LSP server's textDocument/didOpen notification.
func DetectLanguageID(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	case ".rs":
		return "rust"
	case ".cs":
		return "csharp"
	default:
		return ""
	}
}
