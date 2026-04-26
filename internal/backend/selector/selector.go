package selector

import (
	"fmt"
	"path/filepath"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
)

// Selection describes the backend chosen for a file.
type Selection struct {
	Language    string
	LanguageID  string
	BackendName string
	Server      config.ServerConfig
	Backend     backend.RefactoringBackend
}

var (
	tsMorphAvailable  = tsmorph.Available
	newTSMorphBackend = func() backend.RefactoringBackend {
		return tsmorph.NewAdapter()
	}
	newLSPBackend = func(cfg config.ServerConfig, languageID string) backend.RefactoringBackend {
		return lsp.NewAdapter(cfg, languageID, nil)
	}
)

// ForFile selects a backend for the given file path while preserving current
// behavior for existing languages while allowing TypeScript/JavaScript to
// prefer ts-morph when it is available locally.
func ForFile(cfg *config.Config, workspaceRoot string, filePath string) (*Selection, error) {
	language := detectServerKey(filePath)
	languageID := detectLanguageID(filePath)

	if prefersTSMorph(language) && tsMorphAvailable() {
		return &Selection{
			Language:    language,
			LanguageID:  languageID,
			BackendName: "tsmorph",
			Backend:     newTSMorphBackend(),
		}, nil
	}

	serverCfg := cfg.ResolvedServer(language, workspaceRoot)
	if serverCfg.Command == "" {
		return nil, fmt.Errorf("no server configured for language %q", language)
	}

	return &Selection{
		Language:    language,
		LanguageID:  languageID,
		BackendName: "lsp",
		Server:      serverCfg,
		Backend:     newLSPBackend(serverCfg, languageID),
	}, nil
}

func prefersTSMorph(language string) bool {
	switch language {
	case "typescript", "javascript":
		return true
	default:
		return false
	}
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
