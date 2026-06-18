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

// ErrLanguageUnsupported is returned by ForFile when the detected language is
// marked LevelUnsupported in the support matrix. Selection short-circuits with
// this error before constructing any backend, so an operation on an unsupported
// language reports the documented unsupported status instead of reaching a
// backend that is not claimed for this release (issue #110).
type ErrLanguageUnsupported struct {
	// Language is the refute language key that is unsupported.
	Language string
	// Caveat is the support-matrix explanation shown to the user.
	Caveat string
}

func (e *ErrLanguageUnsupported) Error() string {
	if e.Caveat == "" {
		return fmt.Sprintf("%s is not supported in this release", e.Language)
	}
	return fmt.Sprintf("%s is not supported in this release: %s", e.Language, e.Caveat)
}

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

	// Gate unsupported languages before any backend routing or construction.
	// The support matrix is the single source of truth for what refute claims;
	// a row marked LevelUnsupported must never reach a backend (e.g. Java/Kotlin
	// must not reach OpenRewrite setup), so callers can report the documented
	// unsupported status instead of a backend-missing/backend-unavailable error.
	if entry, ok := config.SupportFor(language); ok && entry.Level == config.LevelUnsupported {
		return nil, &ErrLanguageUnsupported{Language: language, Caveat: entry.Caveats}
	}

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
