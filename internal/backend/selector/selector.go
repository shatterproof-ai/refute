package selector

import (
	"fmt"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/openrewrite"
	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/language"
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
	newOpenRewriteBackend = func() backend.RefactoringBackend {
		return openrewrite.NewAdapter("")
	}
	newLSPBackend = func(cfg config.ServerConfig, languageID string) backend.RefactoringBackend {
		return lsp.NewAdapter(cfg, languageID, nil)
	}
)

// ForFile selects a backend for the given file path while preserving current
// behavior for existing languages while allowing TypeScript/JavaScript to
// prefer ts-morph when it is available locally.
func ForFile(cfg *config.Config, workspaceRoot string, filePath string) (*Selection, error) {
	detected := language.Detect(filePath)
	language := detected.Language
	languageID := detected.LanguageID

	if prefersTSMorph(language) && tsMorphAvailable() {
		return &Selection{
			Language:    language,
			LanguageID:  languageID,
			BackendName: "tsmorph",
			Backend:     newTSMorphBackend(),
		}, nil
	}

	if prefersOpenRewrite(language) {
		return &Selection{
			Language:    language,
			LanguageID:  languageID,
			BackendName: "openrewrite",
			Backend:     newOpenRewriteBackend(),
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

func prefersOpenRewrite(language string) bool {
	switch language {
	case "java", "kotlin":
		return true
	default:
		return false
	}
}
