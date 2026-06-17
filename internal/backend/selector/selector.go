package selector

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/openrewrite"
	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/language"
)

// ErrLanguageUnsupported is returned by the selector entry points (ForFile,
// CandidatesFor, SelectForOperation) when the detected language is marked
// LevelUnsupported in the support matrix. Selection short-circuits with
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

// Candidate is one rung of the backend ladder for a (file, operation), in
// preference order, annotated with whether that backend supports the operation.
type Candidate struct {
	Selection
	// Operation is the refactoring operation the candidate was evaluated for.
	Operation string
	// Supported reports whether the backend advertises Operation in its
	// Capabilities(). Callers try candidates in order, skipping unsupported
	// ones, and refuse explicitly when none support the operation.
	Supported bool
}

// ErrOperationUnsupported is returned by SelectForOperation when a backend is
// configured for the file's language but none of the ladder's rungs support the
// requested operation. Callers refuse rather than attempt-and-fail mid-flight.
var ErrOperationUnsupported = errors.New("operation not supported by any configured backend for this language")

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

// ForFile selects the preferred backend for the given file path, preserving the
// existing per-file behavior: TypeScript/JavaScript prefer ts-morph when it is
// available locally, falling back to the LSP server; Java/Kotlin use
// OpenRewrite; everything else uses its configured LSP server. It returns the
// first rung of the backend ladder (see CandidatesFor).
func ForFile(ctx context.Context, cfg *config.Config, workspaceRoot string, filePath string) (*Selection, error) {
	ladder, err := backendLadder(ctx, cfg, workspaceRoot, filePath)
	if err != nil {
		return nil, err
	}
	sel := ladder[0]
	return &sel, nil
}

// CandidatesFor returns the ordered (backend, operation) candidates for a file:
// the same preference-ordered backend ladder ForFile draws from, each annotated
// with whether it supports the requested operation. The first supported
// candidate is the one to run; if none are supported the operation should be
// refused. Use SelectForOperation for that fall-through-or-refuse decision.
func CandidatesFor(ctx context.Context, cfg *config.Config, workspaceRoot, filePath, operation string) ([]Candidate, error) {
	ladder, err := backendLadder(ctx, cfg, workspaceRoot, filePath)
	if err != nil {
		return nil, err
	}
	cands := make([]Candidate, 0, len(ladder))
	for _, sel := range ladder {
		cands = append(cands, Candidate{
			Selection: sel,
			Operation: operation,
			Supported: backendSupports(sel.Backend, operation),
		})
	}
	return cands, nil
}

// SelectForOperation returns the first backend in the ladder that supports the
// operation. It returns ErrOperationUnsupported when a backend exists for the
// language but none supports the operation, so the caller can refuse up front
// instead of attempting an operation that will fail at request time.
func SelectForOperation(ctx context.Context, cfg *config.Config, workspaceRoot, filePath, operation string) (*Selection, error) {
	cands, err := CandidatesFor(ctx, cfg, workspaceRoot, filePath, operation)
	if err != nil {
		return nil, err
	}
	for i := range cands {
		if cands[i].Supported {
			sel := cands[i].Selection
			return &sel, nil
		}
	}
	return nil, fmt.Errorf("%q: %w", operation, ErrOperationUnsupported)
}

// backendLadder builds the preference-ordered backend candidates for a file.
// The order encodes the documented backend strategy: a preferred backend with
// fall-through rungs behind it. It returns an error only when no backend is
// configured for the language at all.
func backendLadder(ctx context.Context, cfg *config.Config, workspaceRoot, filePath string) ([]Selection, error) {
	detected := language.Detect(filePath)
	lang := detected.Language
	languageID := detected.LanguageID

	// Gate unsupported languages before any backend routing or construction.
	// The support matrix is the single source of truth for what refute claims;
	// a row marked LevelUnsupported must never reach a backend (e.g. Java/Kotlin
	// must not reach OpenRewrite setup), so callers can report the documented
	// unsupported status instead of a backend-missing/backend-unavailable error.
	// Gating here in the shared ladder builder covers ForFile, CandidatesFor, and
	// SelectForOperation alike.
	if entry, ok := config.SupportFor(lang); ok && entry.Level == config.LevelUnsupported {
		return nil, &ErrLanguageUnsupported{Language: lang, Caveat: entry.Caveats}
	}

	var ladder []Selection

	// Java/Kotlin route exclusively through OpenRewrite; there is no LSP rung.
	if prefersOpenRewrite(lang) {
		return []Selection{{
			Language:    lang,
			LanguageID:  languageID,
			BackendName: "openrewrite",
			Backend:     newOpenRewriteBackend(),
		}}, nil
	}

	// TypeScript/JavaScript prefer a locally available ts-morph adapter, then
	// fall through to the LSP server.
	explicitAdapterPath := cfg.Tools.TSMorph.Adapter
	if prefersTSMorph(lang) && tsMorphAvailable(workspaceRoot, explicitAdapterPath) {
		ladder = append(ladder, Selection{
			Language:    lang,
			LanguageID:  languageID,
			BackendName: "tsmorph",
			Backend:     newTSMorphBackend(ctx, explicitAdapterPath),
		})
	}

	if serverCfg := cfg.ResolvedServer(lang, workspaceRoot); serverCfg.Command != "" {
		ladder = append(ladder, Selection{
			Language:    lang,
			LanguageID:  languageID,
			BackendName: "lsp",
			Server:      serverCfg,
			Backend:     newLSPBackend(ctx, serverCfg, languageID, cfg.RequestTimeout()),
		})
	}

	if len(ladder) == 0 {
		return nil, fmt.Errorf("no server configured for language %q", lang)
	}
	return ladder, nil
}

// backendSupports reports whether a backend advertises the operation.
func backendSupports(b backend.RefactoringBackend, operation string) bool {
	for _, c := range b.Capabilities() {
		if c.Operation == operation {
			return true
		}
	}
	return false
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
