package lsp

// This file is the single place where per-language LSP behavior is registered.
// The shared adapter consults a languageProfile instead of switching on a
// language-ID string, so adding or adjusting a language touches only the
// languageProfiles registry below — not the adapter, priming, or capability
// code.

// refactorEngine selects how a language drives extract/inline through LSP code
// actions.
type refactorEngine int

const (
	// engineTitleMatch resolves extract/inline by requesting refactor.extract /
	// refactor.inline code actions and matching them by kind prefix and a
	// title substring. Used by gopls and typescript-language-server. Inline
	// requests cover the symbol's identifier-width range (servers return no
	// action for a zero-width range).
	engineTitleMatch refactorEngine = iota
	// engineAssist resolves extract/inline through rust-analyzer's assist
	// actions, matched by the rustActionPatterns table, and inlines the single
	// call site over the identifier-width range.
	engineAssist
)

// primingProfile parameterizes the workspace-priming file walk. Priming opens a
// bounded set of source files via textDocument/didOpen so the server begins
// indexing before the first request arrives.
type primingProfile struct {
	// extensions maps a lowercase source-file extension (".go") to the LSP
	// languageID the file is opened as. Empty disables priming.
	extensions map[string]string
	// skipDirs are directory base names never recursed into. Dot-prefixed
	// directories are always skipped regardless of this set.
	skipDirs map[string]bool
	// fileCap bounds how many files are opened.
	//
	// Go primes broadly (500): gopls answers workspace/symbol against the set
	// of loaded packages, so a Tier-1 qualified-name query needs the whole
	// module opened. rust-analyzer and typescript-language-server index
	// transitively from a few entry files, so they prime lightly (10) to warm
	// the graph without opening the world.
	fileCap int
	// drainWithSentinel issues a final zero-result workspace/symbol query after
	// priming to force the server to process queued didOpen notifications
	// before the first real query. gopls needs this; the others do not.
	drainWithSentinel bool
	// onInitialize primes during Initialize (rust, typescript, javascript).
	// When false the language is primed only on an explicit PrimeWorkspace call
	// (go, whose broad prime is driven by the Tier-1 rename path).
	onInitialize bool
}

// languageProfile is the per-language behavior the shared LSP adapter consults.
//
// Note: language-specific symbol grammar now lives outside this profile —
// qualified-name parsing in internal/symbol (rust.go) and Rust trait/impl
// candidate filtering in this package (rust_resolve.go, Adapter.FilterRustCandidates).
// This profile owns the LSP-backend behaviors only.
type languageProfile struct {
	languageID string
	engine     refactorEngine
	priming    primingProfile
	// operations is the list of refactoring operations this language's backend
	// actually supports, in the same vocabulary as backend.Capability.Operation.
	// Adapter.Capabilities() derives from this, so a language never advertises an
	// operation its engine cannot perform. These must agree with the language's
	// config.SupportMatrix Operations (guarded by a test).
	operations []string
}

// Operation-support lists, named so they read at the registry call site. The
// strings match backend.Capability.Operation and config.SupportMatrix.
var (
	fullOperations = []string{"rename", "extract-function", "extract-variable", "inline"}
	renameOnlyOps  = []string{"rename"}
)

// goSkipDirs and friends are the per-language directory skip sets, named so the
// rationale stays close to the registry.
var (
	goSkipDirs   = map[string]bool{"vendor": true, "node_modules": true, ".git": true, ".svn": true, ".hg": true}
	rustSkipDirs = map[string]bool{"target": true, "node_modules": true}
	webSkipDirs  = map[string]bool{"node_modules": true, "dist": true}
)

// languageProfiles is the registry. Adding a language means adding one entry
// here; no other file in the LSP backend needs to change.
var languageProfiles = map[string]languageProfile{
	"go": {
		languageID: "go",
		engine:     engineTitleMatch,
		operations: fullOperations,
		priming: primingProfile{
			extensions:        map[string]string{".go": "go"},
			skipDirs:          goSkipDirs,
			fileCap:           500,
			drainWithSentinel: true,
			onInitialize:      false,
		},
	},
	"rust": {
		languageID: "rust",
		engine:     engineAssist,
		operations: fullOperations,
		priming: primingProfile{
			extensions:   map[string]string{".rs": "rust"},
			skipDirs:     rustSkipDirs,
			fileCap:      10,
			onInitialize: true,
		},
	},
	"typescript": {
		languageID: "typescript",
		engine:     engineTitleMatch,
		operations: renameOnlyOps,
		priming: primingProfile{
			extensions:   map[string]string{".ts": "typescript"},
			skipDirs:     webSkipDirs,
			fileCap:      10,
			onInitialize: true,
		},
	},
	"typescriptreact": {
		languageID: "typescriptreact",
		engine:     engineTitleMatch,
		operations: renameOnlyOps,
		priming: primingProfile{
			extensions:   map[string]string{".tsx": "typescriptreact"},
			skipDirs:     webSkipDirs,
			fileCap:      10,
			onInitialize: true,
		},
	},
	"javascript": {
		languageID: "javascript",
		engine:     engineTitleMatch,
		operations: renameOnlyOps,
		priming: primingProfile{
			extensions:   map[string]string{".js": "javascript"},
			skipDirs:     webSkipDirs,
			fileCap:      10,
			onInitialize: true,
		},
	},
	"javascriptreact": {
		languageID: "javascriptreact",
		engine:     engineTitleMatch,
		operations: renameOnlyOps,
		priming: primingProfile{
			extensions:   map[string]string{".jsx": "javascriptreact"},
			skipDirs:     webSkipDirs,
			fileCap:      10,
			onInitialize: true,
		},
	},
	// python is a skeleton: rename only (no assist engine, no priming) until a
	// pyright integration lands. It demonstrates that adding a language is a
	// registry-only change.
	"python": {
		languageID: "python",
		engine:     engineTitleMatch,
		operations: renameOnlyOps,
		priming:    primingProfile{},
	},
}

// profileFor returns the registered profile for a languageID, or a conservative
// default (title-match engine, no priming, rename-only) for an unregistered
// language. Rename-only is the safe baseline: every LSP server implements
// textDocument/rename, so an unknown language advertises rename without
// claiming extract/inline it cannot perform.
func profileFor(languageID string) languageProfile {
	if p, ok := languageProfiles[languageID]; ok {
		return p
	}
	return languageProfile{languageID: languageID, engine: engineTitleMatch, operations: renameOnlyOps}
}
