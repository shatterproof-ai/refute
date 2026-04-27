package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/selector"
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
	flagJSON    bool
)

func addRenameFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flagFile, "file", "", "source file path")
	cmd.Flags().IntVar(&flagLine, "line", 0, "line number (1-indexed)")
	cmd.Flags().IntVar(&flagCol, "col", 0, "column number (1-indexed, optional)")
	cmd.Flags().StringVar(&flagName, "name", "", "symbol name to find on the line")
	cmd.Flags().StringVar(&flagNewName, "new-name", "", "new name for the symbol")
	cmd.Flags().StringVar(&flagSymbol, "symbol", "", "qualified symbol name (e.g., pkg.Func or Type.Method)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	_ = cmd.MarkFlagRequired("new-name")
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
	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a symbol (kind-agnostic)",
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
	query := symbol.Query{
		QualifiedName: flagSymbol,
		File:          flagFile,
		Line:          flagLine,
		Column:        flagCol,
		Name:          flagName,
		Kind:          kind,
	}

	if query.File != "" {
		abs, err := filepath.Abs(query.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		query.File = abs
	}

	if query.Tier() == 1 {
		return runRenameTier1(query)
	}

	loc, err := symbol.Resolve(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}

	b, workspaceRoot, err := buildBackend(loc.File)
	if err != nil {
		return err
	}
	defer b.Shutdown()

	return finishRename(b, workspaceRoot, loc, flagNewName)
}

// buildBackend selects and initializes a refactoring backend for the given file.
func buildBackend(filePath string) (backend.RefactoringBackend, string, error) {
	workspaceRoot, err := FindWorkspaceRootFromFile(filePath)
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	sel, err := selector.ForFile(cfg, workspaceRoot, filePath)
	if err != nil {
		return nil, "", err
	}
	if err := sel.Backend.Initialize(workspaceRoot); err != nil {
		return nil, "", fmt.Errorf("initializing backend: %w", err)
	}
	return sel.Backend, workspaceRoot, nil
}

// finishRename requests the rename edit and routes it through the output pipeline.
func finishRename(b backend.RefactoringBackend, workspaceRoot string, loc symbol.Location, newName string) error {
	we, err := b.Rename(loc, newName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(we, workspaceRoot)
}

// applyOrPreview emits the result per --dry-run/--json/default flags.
func applyOrPreview(we *edit.WorkspaceEdit, workspaceRoot string) error {
	if flagJSON {
		return emitJSON(we, statusForFlags())
	}
	if flagDryRun {
		diff, err := edit.RenderDiff(we)
		if err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}
	result, err := edit.Apply(we)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}
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
		if diff, err := edit.RenderDiff(we); err == nil && diff != "" {
			fmt.Print(diff)
		}
	}
	return nil
}

func statusForFlags() string {
	if flagDryRun {
		return "dry-run"
	}
	return "applied"
}

func emitJSON(we *edit.WorkspaceEdit, status string) error {
	res := edit.RenderJSON(we, status)
	if !flagDryRun {
		if _, err := edit.Apply(we); err != nil {
			return fmt.Errorf("applying edits: %w", err)
		}
	}
	data, err := res.Marshal()
	if err != nil {
		return fmt.Errorf("marshalling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func runRenameTier1(query symbol.Query) error {
	workspaceRoot, err := tier1WorkspaceRoot()
	if err != nil {
		return err
	}

	language := "go"
	if query.File != "" {
		language = DetectServerKey(query.File)
	}
	if language == "" {
		language = "go" // fallback for naked --symbol without --file
	}

	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	serverCfg := cfg.Server(language)
	if serverCfg.Command == "" {
		return fmt.Errorf("no server configured for language %q", language)
	}

	adapter := lsp.NewAdapter(serverCfg, language, nil)
	if err := adapter.Initialize(workspaceRoot); err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer adapter.Shutdown()

	// Prime so workspace/symbol sees the whole module.
	if _, err := adapter.PrimeWorkspace(workspaceRoot); err != nil {
		return fmt.Errorf("priming workspace: %w", err)
	}

	locs, err := adapter.FindSymbol(query)
	if err != nil {
		return fmt.Errorf("symbol resolution: %w", err)
	}
	if len(locs) > 1 {
		return ambiguousError(locs)
	}
	return finishRename(adapter, workspaceRoot, locs[0], flagNewName)
}

// tier1WorkspaceRoot resolves the workspace root for a Tier 1 query.
// If --file is provided, walk up from it; otherwise walk up from cwd.
func tier1WorkspaceRoot() (string, error) {
	if flagFile != "" {
		abs, err := filepath.Abs(flagFile)
		if err != nil {
			return "", err
		}
		return FindWorkspaceRootFromDir(filepath.Dir(abs))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}
	return FindWorkspaceRootFromDir(cwd)
}

// ambiguousError formats a Tier 1 ambiguity result. In JSON mode, emit a
// structured candidates list; otherwise print a human-readable message.
func ambiguousError(locs []symbol.Location) error {
	if flagJSON {
		res := &edit.JSONResult{Status: "ambiguous"}
		for _, l := range locs {
			res.Candidates = append(res.Candidates, edit.JSONSymbolLoc{
				File:   l.File,
				Line:   l.Line,
				Column: l.Column,
				Name:   l.Name,
			})
		}
		data, _ := res.Marshal()
		fmt.Println(string(data))
		return &ExitCodeError{Code: 1}
	}
	msg := "Ambiguous — multiple candidates:\n"
	for _, l := range locs {
		msg += fmt.Sprintf("  %s:%d:%d  %s\n", l.File, l.Line, l.Column, l.Name)
	}
	msg += "Use --file and --line to narrow the selection."
	return &ExitCodeError{Code: 1, Message: msg}
}
