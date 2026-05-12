package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shatterproof-ai/refute/internal/language"
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
	return language.Detect(filePath).CLIConfigKey
}
