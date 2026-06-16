package selector

import (
	"context"
	"fmt"
	"time"

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
	tsMorphAvailable = func(workspaceRoot, explicitPath string) bool {
		return tsmorph.AvailableAt(workspaceRoot, explicitPath)
	}
	newTSMorphBackend = func(ctx context.Context, adapterPath string) backend.RefactoringBackend {
		a := tsmorph.NewAdapterWithPath(adapterPath)
		a.SetContext(ctx)
		return a
	}
	newOpenRewriteBackend = func() backend.RefactoringBackend {
		return openrewrite.NewAdapter("")
	}
	newLSPBackend = func(ctx context.Context, cfg config.ServerConfig, languageID string, requestTimeout time.Duration) backend.RefactoringBackend {
		a := lsp.NewAdapter(cfg, languageID, nil)
		a.SetContext(ctx)
		a.SetRequestTimeout(requestTimeout)
		return a
	}
)

// ForFile selects a backend for the given file path while preserving current
// behavior for existing languages while allowing TypeScript/JavaScript to
// prefer ts-morph when it is available locally.
func ForFile(ctx context.Context, cfg *config.Config, workspaceRoot string, filePath string) (*Selection, error) {
	detected := language.Detect(filePath)
	language := detected.Language
	languageID := detected.LanguageID

	explicitAdapterPath := cfg.Tools.TSMorph.Adapter
	if prefersTSMorph(language) && tsMorphAvailable(workspaceRoot, explicitAdapterPath) {
		return &Selection{
			Language:    language,
			LanguageID:  languageID,
			BackendName: "tsmorph",
			Backend:     newTSMorphBackend(ctx, explicitAdapterPath),
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
		Backend:     newLSPBackend(ctx, serverCfg, languageID, cfg.RequestTimeout()),
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
