package config

// Release-support tiers. These describe what refute claims for a language in
// the current release, independent of whether the backend is installed on the
// local host.
const (
	LevelSupported    = "supported"
	LevelExperimental = "experimental"
	LevelPlanned      = "planned"
	LevelUnsupported  = "unsupported"
)

// Backend keys identify the driver behind a support-matrix row. Use these
// constants instead of bare string literals so the runtime comparison in
// `refute doctor` and the matrix definition cannot drift apart.
const (
	// BackendTSLanguageServer is the LSP fallback backend shared by the
	// TypeScript and JavaScript rows.
	BackendTSLanguageServer = "lsp/typescript-language-server"
)

// LanguageSupport is one row of refute's support matrix. SupportMatrix is the
// single source of truth that feeds `refute doctor` rows, install hints, and
// missing-server errors, so the runtime can never drift from the documented
// matrix in docs/support-matrix.md.
type LanguageSupport struct {
	// Language is the refute language key (matches language detection keys).
	Language string
	// Backend identifies the driver, e.g. "lsp/gopls" or "openrewrite".
	Backend string
	// Binary is the language-server executable probed on PATH. Empty for
	// unsupported rows, which are reported without a host probe.
	Binary string
	// VersionArgs are the arguments that make Binary print its version on
	// stdout (e.g. {"version"} for gopls, {"--version"} for the others). Empty
	// when no version probe is defined for the backend.
	VersionArgs []string
	// Level is the release-support tier (one of the Level* constants).
	Level string
	// InstallHint is the command a user runs to install the backend.
	InstallHint string
	// Operations lists the refactorings routed through the backend.
	Operations []string
	// Caveats is human-readable context shown by doctor and the matrix doc.
	Caveats string
}

var (
	renameOnly     = []string{"rename"}
	fullOperations = []string{"rename", "extract-function", "extract-variable", "inline"}
)

// SupportMatrix is the ordered support matrix. Order is the order doctor
// reports languages. Each language appears exactly once (the ts-morph adapter
// is a separate doctor-only concern layered on top of the TypeScript row).
var SupportMatrix = []LanguageSupport{
	{
		Language:    "go",
		Backend:     "lsp/gopls",
		Binary:      "gopls",
		VersionArgs: []string{"version"},
		Level:       LevelSupported,
		InstallHint: "go install golang.org/x/tools/gopls@latest",
		Operations:  fullOperations,
		Caveats:     "Primary v0.1 target.",
	},
	{
		Language:    "typescript",
		Backend:     BackendTSLanguageServer,
		Binary:      "typescript-language-server",
		VersionArgs: []string{"--version"},
		Level:       LevelExperimental,
		InstallHint: "npm install -g typescript-language-server typescript",
		Operations:  renameOnly,
		Caveats:     "Fallback for TypeScript/JavaScript when ts-morph adapter is unavailable.",
	},
	{
		Language:    "javascript",
		Backend:     BackendTSLanguageServer,
		Binary:      "typescript-language-server",
		VersionArgs: []string{"--version"},
		Level:       LevelExperimental,
		InstallHint: "npm install -g typescript-language-server typescript",
		Operations:  renameOnly,
		Caveats:     "JavaScript support is experimental for v0.1.",
	},
	{
		Language:    "rust",
		Backend:     "lsp/rust-analyzer",
		Binary:      "rust-analyzer",
		VersionArgs: []string{"--version"},
		Level:       LevelExperimental,
		InstallHint: "rustup component add rust-analyzer",
		Operations:  fullOperations,
		Caveats:     "Rust support is experimental for v0.1; extract and inline operations depend on rust-analyzer assists and inline is single-call-site only.",
	},
	{
		Language:    "python",
		Backend:     "lsp/pyright",
		Binary:      "pyright-langserver",
		VersionArgs: []string{"--version"},
		Level:       LevelPlanned,
		InstallHint: "npm install -g pyright",
		Operations:  renameOnly,
		Caveats:     "Python support is planned, not yet covered by integration tests.",
	},
	{
		Language: "java",
		Backend:  "openrewrite",
		Level:    LevelUnsupported,
		Caveats:  "Java/OpenRewrite support is not claimed for v0.1.",
	},
	{
		Language: "kotlin",
		Backend:  "openrewrite",
		Level:    LevelUnsupported,
		Caveats:  "Kotlin/OpenRewrite support is not claimed for v0.1.",
	},
}

// supportByLanguage indexes SupportMatrix by language for O(1) lookup.
var supportByLanguage = func() map[string]LanguageSupport {
	m := make(map[string]LanguageSupport, len(SupportMatrix))
	for _, e := range SupportMatrix {
		m[e.Language] = e
	}
	return m
}()

// SupportFor returns the support-matrix row for a language and whether it
// exists.
func SupportFor(language string) (LanguageSupport, bool) {
	e, ok := supportByLanguage[language]
	return e, ok
}
