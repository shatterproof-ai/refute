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

// DetectLanguageFromDir walks up from dir looking for a language-defining
// workspace marker and returns the server key it implies ("go" or "rust"), or
// "" when no marker is found or the nearest marker-bearing directory is
// ambiguous (both Go and Rust markers present). Used to route a naked --symbol
// that carries no file or :: separator; an ambiguous result deliberately yields
// "" so the caller asks for --file rather than guessing.
func DetectLanguageFromDir(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	cur := absDir
	for {
		hasGo := fileExists(filepath.Join(cur, "go.mod")) || fileExists(filepath.Join(cur, "go.work"))
		hasRust := fileExists(filepath.Join(cur, "Cargo.toml"))
		switch {
		case hasGo && hasRust:
			return "" // ambiguous: do not guess
		case hasGo:
			return "go"
		case hasRust:
			return "rust"
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
