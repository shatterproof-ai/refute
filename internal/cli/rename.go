package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
		Short: fmt.Sprintf("Rename a %s (Go, Rust, TypeScript)", kind),
		Long:  fmt.Sprintf("Rename a %s at the given location. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server). See docs/support-matrix.md.", kind),
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
		Short: "Rename a symbol across the workspace (Go, Rust, TypeScript)",
		Long:  "Rename a symbol at the given location. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server). See docs/support-matrix.md.",
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
		if flagJSON {
			return emitJSONError(
				contextFromFile("rename", query.File),
				edit.StatusInvalidPosition,
				"invalid-position",
				err.Error(),
				"Check --file, --line, --col, and --name.",
			)
		}
		return fmt.Errorf("symbol resolution: %w", err)
	}

	sel, workspaceRoot, err := buildBackend(loc.File)
	if err != nil {
		if flagJSON {
			ctx := contextFromFile("rename", loc.File)
			return emitJSONError(ctx, backendErrorStatus(err), "backend-unavailable", err.Error(), "Run `refute doctor` for backend setup details.")
		}
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	ctx := contextFromSelection("rename", sel, workspaceRoot)
	if err := finishRename(sel.Backend, ctx, loc, flagNewName); err != nil {
		if flagJSON {
			return emitJSONOperationError(ctx, err)
		}
		return err
	}
	return nil
}

// buildBackend selects and initializes a refactoring backend for the given file.
func buildBackend(filePath string) (*selector.Selection, string, error) {
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
	if sel.BackendName == "lsp" && sel.Server.Command != "" {
		if _, lookErr := exec.LookPath(sel.Server.Command); lookErr != nil {
			return nil, "", &ErrLSPServerMissing{
				Language:    sel.Language,
				Command:     sel.Server.Command,
				InstallHint: config.InstallHint(sel.Language),
			}
		}
	}
	if err := sel.Backend.Initialize(workspaceRoot); err != nil {
		return nil, "", fmt.Errorf("initializing backend: %w", err)
	}
	return sel, workspaceRoot, nil
}

// finishRename requests the rename edit and routes it through the output pipeline.
func finishRename(b backend.RefactoringBackend, ctx jsonContext, loc symbol.Location, newName string) error {
	we, err := b.Rename(loc, newName)
	if err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(we, ctx)
}

// applyOrPreview emits the result per --dry-run/--json/default flags.
func applyOrPreview(we *edit.WorkspaceEdit, ctx jsonContext) error {
	if flagJSON {
		return emitJSON(we, ctx, statusForFlags())
	}
	if flagDryRun {
		diff, err := edit.RenderDiff(we)
		if err != nil {
			return fmt.Errorf("rendering diff: %w", err)
		}
		fmt.Print(diff)
		return nil
	}
	result, err := edit.ApplyWithin(we, ctx.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("applying edits: %w", err)
	}
	green := color.New(color.FgGreen).SprintFunc()
	fmt.Fprintf(os.Stderr, "%s Modified %d file(s):", green("ok"), result.FilesModified)
	for _, fe := range we.FileEdits {
		rel, _ := filepath.Rel(ctx.WorkspaceRoot, fe.Path)
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

func emitJSON(we *edit.WorkspaceEdit, ctx jsonContext, status string) error {
	res := edit.RenderJSON(we, status)
	res.Operation = ctx.Operation
	res.Language = ctx.Language
	res.Backend = ctx.Backend
	res.WorkspaceRoot = ctx.WorkspaceRoot
	if !flagDryRun {
		if _, err := edit.ApplyWithin(we, ctx.WorkspaceRoot); err != nil {
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
	setup, err := setupTier1RenameBackend(query)
	if err != nil {
		return err
	}
	defer func() { _ = setup.adapter.Shutdown() }()

	loc, err := resolveTier1Symbol(setup.adapter, setup.language, query)
	if err != nil {
		return handleTier1RenameError(setup.ctx, err)
	}
	return handleTier1RenameError(setup.ctx, finishRename(setup.adapter, setup.ctx, loc, flagNewName))
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

type tier1RenameBackend struct {
	adapter  *lsp.Adapter
	language string
	ctx      jsonContext
}

func setupTier1RenameBackend(query symbol.Query) (*tier1RenameBackend, error) {
	workspaceRoot, err := tier1WorkspaceRoot()
	if err != nil {
		return nil, err
	}
	language := tier1Language(query)
	ctx := tier1RenameContext(language, workspaceRoot)

	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		return nil, handleTier1BackendSetupError(ctx, fmt.Errorf("loading config: %w", err))
	}
	serverCfg := cfg.Server(language)
	if serverCfg.Command == "" {
		return nil, handleTier1BackendSetupError(ctx, fmt.Errorf("no server configured for language %q", language))
	}
	if _, lookErr := exec.LookPath(serverCfg.Command); lookErr != nil {
		return nil, handleTier1BackendSetupError(ctx, &ErrLSPServerMissing{
			Language:    language,
			Command:     serverCfg.Command,
			InstallHint: config.InstallHint(language),
		})
	}

	adapter := lsp.NewAdapter(serverCfg, language, nil)
	if err := adapter.Initialize(workspaceRoot); err != nil {
		return nil, handleTier1BackendSetupError(ctx, err)
	}
	// Prime so workspace/symbol sees the whole module.
	if _, err := adapter.PrimeWorkspace(workspaceRoot); err != nil {
		_ = adapter.Shutdown()
		return nil, handleTier1WorkspacePrimeError(ctx, err)
	}
	return &tier1RenameBackend{adapter: adapter, language: language, ctx: ctx}, nil
}

func tier1Language(query symbol.Query) string {
	if query.File == "" {
		return "go" // fallback for naked --symbol without --file
	}
	if language := DetectServerKey(query.File); language != "" {
		return language
	}
	return "go"
}

func tier1RenameContext(language, workspaceRoot string) jsonContext {
	return jsonContext{
		Operation:     "rename",
		Language:      language,
		Backend:       "lsp",
		WorkspaceRoot: workspaceRoot,
	}
}

func handleTier1BackendSetupError(ctx jsonContext, err error) error {
	if flagJSON {
		return emitJSONError(ctx, backendErrorStatus(err), "backend-unavailable", err.Error(), "Run `refute doctor` for backend setup details.")
	}
	return fmt.Errorf("initializing backend: %w", err)
}

func handleTier1WorkspacePrimeError(ctx jsonContext, err error) error {
	if flagJSON {
		return emitJSONError(ctx, edit.StatusBackendFailed, "backend-failed", err.Error(), "")
	}
	return fmt.Errorf("priming workspace: %w", err)
}

func resolveTier1Symbol(adapter backend.RefactoringBackend, language string, query symbol.Query) (symbol.Location, error) {
	if language == "rust" {
		return resolveRustTier1Symbol(adapter, query)
	}
	locs, err := adapter.FindSymbol(query)
	if err != nil {
		return symbol.Location{}, &tier1SymbolResolutionError{err: err}
	}
	if len(locs) > 1 {
		return symbol.Location{}, &backend.ErrAmbiguous{Candidates: locs}
	}
	if len(locs) == 0 {
		return symbol.Location{}, &tier1SymbolResolutionError{err: backend.ErrSymbolNotFound}
	}
	return locs[0], nil
}

func resolveRustTier1Symbol(adapter backend.RefactoringBackend, query symbol.Query) (symbol.Location, error) {
	modulePath, trait, name, err := ParseRustQualifiedName(query.QualifiedName)
	if err != nil {
		return symbol.Location{}, &tier1RustSymbolParseError{err: err}
	}
	infos, err := adapter.FindSymbol(symbol.Query{QualifiedName: name})
	if err != nil {
		return symbol.Location{}, &tier1SymbolResolutionError{err: err}
	}
	candidates := filterRustCandidates(infos, modulePath, trait, name, adapter)
	switch len(candidates) {
	case 0:
		return symbol.Location{}, &ErrSymbolNotFound{
			Language:   "rust",
			Input:      query.QualifiedName,
			ModulePath: modulePath,
			Trait:      trait,
			Name:       name,
		}
	case 1:
		return candidates[0], nil
	default:
		return symbol.Location{}, &backend.ErrAmbiguous{Candidates: candidates}
	}
}

type tier1RustSymbolParseError struct {
	err error
}

func (e *tier1RustSymbolParseError) Error() string {
	return fmt.Sprintf("parse --symbol: %v", e.err)
}

func (e *tier1RustSymbolParseError) Unwrap() error {
	return e.err
}

type tier1SymbolResolutionError struct {
	err error
}

func (e *tier1SymbolResolutionError) Error() string {
	return fmt.Sprintf("symbol resolution: %v", e.err)
}

func (e *tier1SymbolResolutionError) Unwrap() error {
	return e.err
}

func handleTier1RenameError(ctx jsonContext, err error) error {
	if err == nil {
		return nil
	}
	var ambiguous *backend.ErrAmbiguous
	if errors.As(err, &ambiguous) {
		return ambiguousError(ctx, ambiguous.Candidates)
	}
	var parseErr *tier1RustSymbolParseError
	if errors.As(err, &parseErr) {
		return err
	}
	var resolutionErr *tier1SymbolResolutionError
	if errors.As(err, &resolutionErr) {
		if flagJSON {
			return emitJSONOperationError(ctx, resolutionErr.err)
		}
		return err
	}
	if flagJSON {
		return emitJSONOperationError(ctx, err)
	}
	return err
}

// ambiguousError formats a Tier 1 ambiguity result. In JSON mode, emit a
// structured candidates list; otherwise print a human-readable message.
func ambiguousError(ctx jsonContext, locs []symbol.Location) error {
	if flagJSON {
		res := &edit.JSONResult{
			SchemaVersion: edit.SchemaVersion,
			Status:        edit.StatusAmbiguous,
			Operation:     ctx.Operation,
			Language:      ctx.Language,
			Backend:       ctx.Backend,
			WorkspaceRoot: ctx.WorkspaceRoot,
		}
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

// filterRustCandidates narrows workspace/symbol results by module path and,
// for trait-qualified queries, by enclosing trait. The cheap branch matches
// the trait via parseRustContainer. The expensive branch falls back to
// DocumentSymbol when the container parser cannot identify the trait.
func filterRustCandidates(
	infos []symbol.Location,
	modulePath []string,
	trait, name string,
	adapter backend.RefactoringBackend,
) []symbol.Location {
	out := make([]symbol.Location, 0, len(infos))
	for _, info := range infos {
		if info.Name != name {
			continue
		}
		infoMod, infoType, infoTrait := lsp.ParseRustContainer(info.Container)
		if !moduleMatches(modulePath, infoMod, infoType) {
			continue
		}
		if trait != "" {
			resolved := infoTrait
			if resolved == "" {
				resolved = resolveTraitByDocumentSymbol(adapter, info)
			}
			if resolved != trait {
				continue
			}
		}
		out = append(out, info)
	}
	return out
}

// moduleMatches returns true when expected is a suffix of actual (actual may
// have extra leading segments). An expected of ["Greeter"] matches any
// container ending in Greeter; ["greet", "Greeter"] requires both.
func moduleMatches(expected, actualMod []string, actualType string) bool {
	full := append([]string{}, actualMod...)
	if actualType != "" {
		full = append(full, actualType)
	}
	if len(expected) == 0 {
		return true
	}
	if len(full) == 0 {
		// rust-analyzer omits containerName for some module-level functions;
		// accept any name match when no container info is available.
		return true
	}
	// Use the shorter length for suffix matching: rust-analyzer may return an
	// abbreviated container (e.g., "util" instead of "greet::util").
	n := len(full)
	if len(expected) < n {
		n = len(expected)
	}
	for i := 0; i < n; i++ {
		if full[len(full)-n+i] != expected[len(expected)-n+i] {
			return false
		}
	}
	return true
}

// resolveTraitByDocumentSymbol is populated only on the expensive branch.
// On the cheap branch it is a no-op that returns "". See Task 6.
func resolveTraitByDocumentSymbol(adapter backend.RefactoringBackend, info symbol.Location) string {
	a, ok := adapter.(*lsp.Adapter)
	if !ok {
		return ""
	}
	symbols, err := a.DocumentSymbols(info.File)
	if err != nil {
		return ""
	}
	return lsp.FindEnclosingImplTrait(symbols, info.Line-1) // info.Line is 1-indexed; DocSymbol is 0-indexed
}
