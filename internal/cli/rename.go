package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/openrewrite"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

var (
	flagFile    string
	flagLine    int
	flagCol     int
	flagName    string
	flagNewName string
	flagSymbol  string
)

func addRenameFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	cmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed, optional)")
	cmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	cmd.Flags().StringVar(&flagNewName, "new-name", "", "new name for the symbol")
	cmd.Flags().StringVar(&flagSymbol, "symbol", "", "qualified symbol name (e.g., ClassName.method)")
	cmd.MarkFlagRequired("new-name")
}

func makeRenameCmd(use string, kind symbol.SymbolKind) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Rename a %s", kind),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(kind)
		},
	}
	addRenameFlags(cmd)
	return cmd
}

func init() {
	// Generic rename (requires exact position).
	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a symbol (kind-agnostic, requires --file --line --col)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(symbol.KindUnknown)
		},
	}
	addRenameFlags(renameCmd)

	RootCmd.AddCommand(renameCmd)
	RootCmd.AddCommand(makeRenameCmd("rename-function", symbol.KindFunction))
	RootCmd.AddCommand(makeRenameCmd("rename-class", symbol.KindClass))
	RootCmd.AddCommand(makeRenameCmd("rename-field", symbol.KindField))
	RootCmd.AddCommand(makeRenameCmd("rename-variable", symbol.KindVariable))
	RootCmd.AddCommand(makeRenameCmd("rename-parameter", symbol.KindParameter))
	RootCmd.AddCommand(makeRenameCmd("rename-type", symbol.KindType))
	RootCmd.AddCommand(makeRenameCmd("rename-method", symbol.KindMethod))
}

func runRename(kind symbol.SymbolKind) error {
	// Build the symbol query from flags.
	query := symbol.Query{
		QualifiedName: flagSymbol,
		File:          flagFile,
		Line:          flagLine,
		Column:        flagCol,
		Name:          flagName,
		Kind:          kind,
	}

	// Resolve file path to absolute.
	if query.File != "" {
		abs, err := filepath.Abs(query.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query.File = abs
	}

	// Resolve the symbol to a concrete location.
	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	// Determine workspace root (walk up to find go.mod or similar).
	workspaceRoot, err := findWorkspaceRoot(loc.File)
	if err != nil {
		return err
	}

	// Load config.
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Select and initialize the best available backend for the target language.
	adapter, err := selectBackend(loc.File, workspaceRoot, cfg)
	if err != nil {
		return err
	}
	defer adapter.Shutdown()

	// Perform the rename.
	we, err := adapter.Rename(loc, flagNewName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}

	if len(we.FileEdits) == 0 {
		fmt.Fprintln(os.Stderr, "No changes produced.")
		os.Exit(2)
	}

	// Dry-run: show diff and exit.
	if flagDryRun {
		diff, err := edit.RenderDiff(we)
		if err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}

	// Apply edits.
	result, err := edit.Apply(we)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}

	// Print summary.
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s Modified %d file(s):", green("ok"), result.FilesModified)
	for _, fe := range we.FileEdits {
		rel, _ := filepath.Rel(workspaceRoot, fe.Path)
		if rel == "" {
			rel = fe.Path
		}
		fmt.Fprintf(os.Stderr, " %s", rel)
	}
	fmt.Fprintln(os.Stderr)

	if flagVerbose {
		diff, err := edit.RenderDiff(we)
		if err == nil && diff != "" {
			fmt.Print(diff)
		}
	}

	return nil
}

// findWorkspaceRoot walks up from the file to find a directory with go.mod,
// package.json, or similar project markers.
func findWorkspaceRoot(filePath string) (string, error) {
	dir := filepath.Dir(filePath)
	markers := []string{"Cargo.toml", "Cargo.lock", "go.mod", "go.work", "package.json", "tsconfig.json", "pyproject.toml", "setup.py", "pom.xml", "build.gradle", "build.gradle.kts"}

	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root without finding a marker.
			return filepath.Dir(filePath), nil
		}
		dir = parent
	}
}

// detectServerKey returns the server config key for a file based on its extension.
// This is used to look up the language server in the config.
func detectServerKey(filePath string) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return "go"
	case ".ts", ".tsx", ".js", ".jsx":
		return "typescript"
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

// detectLanguageID returns the LSP language ID for a file based on its extension.
// This is passed to the LSP server's textDocument/didOpen notification.
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

// selectBackend picks the best available backend for the file's language,
// initializes it, and returns it ready to use.
//
// For Java and Kotlin:
//  1. Try OpenRewrite (requires the fat JAR built from adapters/openrewrite/).
//  2. Fall back to the jdtls / kotlin-language-server LSP adapter.
//
// For all other languages the generic LSP adapter is used directly.
func selectBackend(filePath, workspaceRoot string, cfg *config.Config) (backend.RefactoringBackend, error) {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".java", ".kt":
		or := openrewrite.NewAdapter("")
		if err := or.Initialize(workspaceRoot); err == nil {
			return or, nil
		}
		// OpenRewrite unavailable — fall through to LSP.
		serverKey := detectServerKey(filePath)
		serverCfg := cfg.Server(serverKey)
		if serverCfg.Command == "" {
			return nil, fmt.Errorf("no server configured for language %q and OpenRewrite JAR not found", serverKey)
		}
		languageID := detectLanguageID(filePath)
		a := lsp.NewAdapter(serverCfg, languageID, nil)
		if err := a.Initialize(workspaceRoot); err != nil {
			return nil, fmt.Errorf("initializing LSP fallback for %s: %w", serverKey, err)
		}
		return a, nil

	default:
		serverKey := detectServerKey(filePath)
		serverCfg := cfg.Server(serverKey)
		if serverCfg.Command == "" {
			return nil, fmt.Errorf("no server configured for language %q", serverKey)
		}
		languageID := detectLanguageID(filePath)
		a := lsp.NewAdapter(serverCfg, languageID, nil)
		if err := a.Initialize(workspaceRoot); err != nil {
			return nil, fmt.Errorf("initializing backend: %w", err)
		}
		return a, nil
	}
}
