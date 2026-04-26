package selector

import (
	"fmt"
	"path/filepath"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
)

// Selection describes the backend chosen for a file.
type Selection struct {
	Language   string
	LanguageID string
	Server     config.ServerConfig
	Backend    backend.RefactoringBackend
}

// ForFile selects a backend for the given file path while preserving current
// behavior: all supported languages route through the generic LSP adapter.
func ForFile(cfg *config.Config, workspaceRoot string, filePath string) (*Selection, error) {
	language := detectServerKey(filePath)
	serverCfg := cfg.ResolvedServer(language, workspaceRoot)
	if serverCfg.Command == "" {
		return nil, fmt.Errorf("no server configured for language %q", language)
	}

	languageID := detectLanguageID(filePath)
	return &Selection{
		Language:   language,
		LanguageID: languageID,
		Server:     serverCfg,
		Backend:    lsp.NewAdapter(serverCfg, languageID, nil),
	}, nil
}

func detectServerKey(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
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

func detectLanguageID(filePath string) string {
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
