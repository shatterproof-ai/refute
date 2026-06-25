package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type renameFlags struct {
	File    string
	Line    int
	Col     int
	Name    string
	NewName string
	Symbol  string
}

// renameAddressingHelp explains how to point rename at a symbol. The same text
// is appended to every rename command's Long so the addressing modes are
// discoverable from --help rather than only from an error.
const renameAddressingHelp = `Address the symbol in one of two ways:
  - by position: --file with --line, then either --col (exact column) or --name
    (the identifier to find on that line); or
  - by qualified name: --symbol (e.g. pkg.Func or Type.Method).
--name finds an identifier on a line; --symbol is a fully qualified name;
--new-name is the replacement identifier.`

func addRenameFlags(cmd *cobra.Command, flags *renameFlags) {
	cmd.Flags().StringVar(&flags.File, "file", "", "source file path (with --line)")
	cmd.Flags().IntVar(&flags.Line, "line", 0, "line number, 1-indexed (with --file)")
	cmd.Flags().IntVar(&flags.Col, "col", 0, "column number, 1-indexed (optional; alternative to --name)")
	cmd.Flags().StringVar(&flags.Name, "name", "", "identifier to find on the line (alternative to --col)")
	cmd.Flags().StringVar(&flags.NewName, "new-name", "", "new name for the symbol (required)")
	cmd.Flags().StringVar(&flags.Symbol, "symbol", "", "qualified symbol name, e.g. pkg.Func or Type.Method (alternative to --file/--line)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	_ = cmd.MarkFlagRequired("new-name")
}

func makeRenameCmd(use string, kind symbol.SymbolKind) *cobra.Command {
	flags := &renameFlags{}
	cmd := &cobra.Command{
		Use:   use,
		Short: fmt.Sprintf("Rename a %s (Go, Rust, TypeScript)", kind),
		Long: fmt.Sprintf("Rename a %s at the given location. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server).\n\n%s\n\nSee %s.",
			kind, renameAddressingHelp, supportMatrixURL),
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeRename, flags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(kind, flags)
		},
	}
	addRenameFlags(cmd, flags)
	return cmd
}

func init() {
	flags := &renameFlags{}
	renameCmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a symbol across the workspace (Go, Rust, TypeScript)",
		Long: "Rename a symbol at the given location. Supports Go (gopls), Rust (rust-analyzer), and TypeScript (typescript-language-server).\n\n" +
			renameAddressingHelp + "\n\nSee " + supportMatrixURL + ".",
		Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return validateLocationFlags(cmd, modeRename, flags)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRename(symbol.KindUnknown, flags)
		},
	}
	addRenameFlags(renameCmd, flags)

	RootCmd.AddCommand(renameCmd)
	RootCmd.AddCommand(makeRenameCmd("rename-function", symbol.KindFunction))
	RootCmd.AddCommand(makeRenameCmd("rename-class", symbol.KindClass))
	RootCmd.AddCommand(makeRenameCmd("rename-field", symbol.KindField))
	RootCmd.AddCommand(makeRenameCmd("rename-variable", symbol.KindVariable))
	RootCmd.AddCommand(makeRenameCmd("rename-parameter", symbol.KindParameter))
	RootCmd.AddCommand(makeRenameCmd("rename-type", symbol.KindType))
	RootCmd.AddCommand(makeRenameCmd("rename-method", symbol.KindMethod))
}

func runRename(kind symbol.SymbolKind, flags *renameFlags) error {
	ctx := jsonContext{Operation: "rename"}
	err := runRenameInner(kind, flags, &ctx)
	return routeOperationError(ctx, err)
}

// runRenameInner performs the rename and returns terminal errors for the shared
// wrapper to route. It updates *ctx as it resolves language/backend metadata so
// an envelope emitted by routeOperationError is fully attributed. Paths that
// produce a specialized status (invalid-position, backend setup) emit
// their envelope inline; the wrapper recognizes the resulting jsonEmitted error
// and passes it through without a second envelope.
func runRenameInner(kind symbol.SymbolKind, flags *renameFlags, ctx *jsonContext) error {
	telemetrySetContext(*ctx)
	query := symbol.Query{
		QualifiedName: flags.Symbol,
		File:          flags.File,
		Line:          flags.Line,
		Column:        flags.Col,
		Name:          flags.Name,
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
		return runRenameTier1(query, flags.NewName)
	}

	resolveDone := telemetryPhase("symbol-resolution")
	loc, err := symbol.Resolve(query)
	resolveDone()
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

	sel, workspaceRoot, err := buildBackend(loc.File, "rename")
	if err != nil {
		if flagJSON {
			*ctx = contextFromFile("rename", loc.File)
			if emitted, ok := emitLanguageUnsupportedError(*ctx, err); ok {
				return emitted
			}
			// Rename is supported by every current backend profile, but keep
			// future profile changes from falling through to backend setup
			// classification if a language ever lacks rename support.
			if errors.Is(err, selector.ErrOperationUnsupported) {
				return emitJSONOperationError(*ctx, err)
			}
			return emitJSONBackendSetupError(*ctx, err)
		}
		return err
	}
	defer func() { _ = sel.Backend.Shutdown() }()

	*ctx = contextFromSelection("rename", sel, workspaceRoot)
	return finishRename(sel.Backend, *ctx, loc, flags.NewName)
}

// buildBackend selects and initializes a refactoring backend for the given file
// and operation.
func buildBackend(filePath, operation string) (*selector.Selection, string, error) {
	selectDone := telemetryPhase("backend-selection")
	workspaceRoot, err := FindWorkspaceRootFromFile(filePath)
	if err != nil {
		selectDone()
		return nil, "", err
	}
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		selectDone()
		return nil, "", fmt.Errorf("loading config: %w", err)
	}
	sel, err := selector.SelectForOperation(commandContext(), cfg, workspaceRoot, filePath, operation)
	if err != nil {
		selectDone()
		return nil, "", err
	}
	if sel.BackendName == "lsp" && sel.Server.Command != "" {
		if _, lookErr := exec.LookPath(sel.Server.Command); lookErr != nil {
			selectDone()
			return nil, "", &ErrLSPServerMissing{
				Language:    sel.Language,
				Command:     sel.Server.Command,
				InstallHint: config.InstallHint(sel.Language),
			}
		}
	}
	selectDone()
	initDone := telemetryPhase("backend-initialization")
	if err := sel.Backend.Initialize(workspaceRoot); err != nil {
		initDone()
		return nil, "", NewBackendInitFailure(sel.BackendName, err)
	}
	initDone()
	return sel, workspaceRoot, nil
}

// finishRename requests the rename edit and routes it through the output pipeline.
func finishRename(b backend.RefactoringBackend, ctx jsonContext, loc symbol.Location, newName string) error {
	telemetrySetContext(ctx)
	refactorDone := telemetryPhase("backend-refactor-request")
	we, err := b.Rename(loc, newName)
	refactorDone()
	if err != nil {
		return translateRenameError(err, newName)
	}
	if len(we.FileEdits) == 0 {
		return NoEditsError()
	}
	return applyOrPreview(we, ctx)
}

// translateRenameError maps backend rename failures into refute's vocabulary so
// users see an actionable message instead of a stacked JSON-RPC chain like
// "rename failed: rename: rename request: JSON-RPC error 0: old and new names
// are the same: Foo". A rename to the symbol's existing name is treated as the
// documented exit-2 no-op rather than an error.
func translateRenameError(err error, newName string) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "old and new names are the same"):
		return &ExitCodeError{Code: noOpExitCode, Message: fmt.Sprintf("%q is already the symbol's name; nothing to rename", newName)}
	case strings.Contains(msg, "is not a valid identifier"), strings.Contains(msg, "invalid identifier"):
		return fmt.Errorf("%q is not a valid identifier", newName)
	default:
		return fmt.Errorf("rename failed: %s", cleanBackendErrorMessage(msg))
	}
}

// cleanBackendErrorMessage strips the LSP plumbing prefixes ("rename: ",
// "rename request: ", "JSON-RPC error N: ") that backends prepend, leaving the
// underlying human-readable message.
func cleanBackendErrorMessage(msg string) string {
	for _, prefix := range []string{"rename: ", "rename request: "} {
		msg = strings.TrimPrefix(msg, prefix)
	}
	if idx := strings.Index(msg, "JSON-RPC error "); idx != -1 {
		rest := msg[idx+len("JSON-RPC error "):]
		if colon := strings.Index(rest, ": "); colon != -1 {
			msg = msg[:idx] + rest[colon+2:]
		}
	}
	return msg
}

func runRenameTier1(query symbol.Query, newName string) error {
	setup, err := setupTier1RenameBackend(query)
	if err != nil {
		return err
	}
	defer func() { _ = setup.adapter.Shutdown() }()

	loc, err := resolveTier1Symbol(setup.adapter, setup.language, query)
	if err != nil {
		return handleTier1RenameError(setup.ctx, err)
	}
	return handleTier1RenameError(setup.ctx, finishRename(setup.adapter, setup.ctx, loc, newName))
}

// tier1WorkspaceRoot resolves the workspace root for a Tier 1 query.
// If --file is provided, walk up from it; otherwise walk up from cwd.
func tier1WorkspaceRoot(file string) (string, error) {
	if file != "" {
		abs, err := filepath.Abs(file)
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
	workspaceRoot, err := tier1WorkspaceRoot(query.File)
	if err != nil {
		return nil, err
	}
	language, err := inferTier1Language(query.QualifiedName, query.File)
	if err != nil {
		return nil, handleTier1BackendSetupError(tier1RenameContext("", workspaceRoot), err)
	}

	ctx := tier1RenameContext(language, workspaceRoot)
	telemetrySetContext(ctx)

	// Gate unsupported languages before any backend routing or construction,
	// mirroring selector.SelectForOperation for the Tier 2 path. A Tier 1
	// (--symbol) rename never reaches SelectForOperation, so without this check
	// a row marked
	// LevelUnsupported (e.g. Java/Kotlin) would fall through to backend setup and
	// report backend-missing instead of the documented unsupported-language
	// status (issue #124).
	if entry, ok := config.SupportFor(language); ok && entry.Level == config.LevelUnsupported {
		return nil, handleTier1BackendSetupError(ctx, &selector.ErrLanguageUnsupported{Language: language, Caveat: entry.Caveats})
	}

	// Tier-1 rename targets the whole module, so the languageID is the language
	// key itself (no file scope). setupLSPBackend re-sets the telemetry context
	// to the same value before emitting its phases.
	adapter, ctx, err := setupLSPBackend("rename", language, "", workspaceRoot)
	if err != nil {
		var setupErr *lspBackendSetupError
		if errors.As(err, &setupErr) {
			switch setupErr.phase {
			case "initialize":
				return nil, handleTier1BackendSetupError(ctx, NewBackendInitFailure("lsp", setupErr.err))
			case "prime":
				return nil, handleTier1WorkspacePrimeError(ctx, setupErr.err)
			}
			return nil, handleTier1BackendSetupError(ctx, setupErr.err)
		}
		return nil, handleTier1BackendSetupError(ctx, err)
	}
	return &tier1RenameBackend{adapter: adapter, language: language, ctx: ctx}, nil
}

// inferTier1Language determines which language backend a Tier 1 (--symbol)
// rename should target. With a --file it follows the file's extension. For a
// naked --symbol it infers from the qualified-name separator (Rust uses "::")
// or the workspace markers around the current directory, and errors rather than
// silently assuming Go — sending a Rust name like mod::Thing::run to gopls only
// fails late and confusingly.
func inferTier1Language(qualifiedName, file string) (string, error) {
	if file != "" {
		if language := DetectServerKey(file); language != "" {
			return language, nil
		}
		return "go", nil
	}
	if strings.Contains(qualifiedName, "::") {
		return "rust", nil
	}
	if cwd, err := os.Getwd(); err == nil {
		if language := DetectLanguageFromDir(cwd); language != "" {
			return language, nil
		}
	}
	return "", fmt.Errorf("cannot infer a language for --symbol %q without --file; "+
		"pass --file to point at the symbol, or run from inside the project "+
		"(go.mod or Cargo.toml). Rust qualified names use :: separators", qualifiedName)
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
	// A language the support matrix marks unsupported reports the documented
	// unsupported-language status, not a backend setup error, in both JSON
	// and human modes (matching the Tier 2 path in runRenameInner).
	var langUnsupported *selector.ErrLanguageUnsupported
	if errors.As(err, &langUnsupported) {
		if flagJSON {
			emitted, _ := emitLanguageUnsupportedError(ctx, err)
			return emitted
		}
		return err
	}
	if flagJSON {
		return emitJSONBackendSetupError(ctx, err)
	}
	var initFail *ErrBackendInitFailure
	if errors.As(err, &initFail) {
		return err
	}
	return fmt.Errorf("initializing backend: %w", err)
}

func handleTier1WorkspacePrimeError(ctx jsonContext, err error) error {
	if flagJSON {
		return emitJSONBackendSetupError(ctx, fmt.Errorf("priming workspace: %w", err))
	}
	return fmt.Errorf("priming workspace: %w", err)
}

// tier1Resolver is the slice of the LSP adapter that Tier 1 symbol resolution
// needs: raw workspace/symbol lookup plus Rust candidate narrowing. *lsp.Adapter
// satisfies it; the narrow interface keeps the CLI orchestration unit-testable
// without a live language server.
type tier1Resolver interface {
	FindSymbol(symbol.Query) ([]symbol.Location, error)
	FilterRustCandidates(infos []symbol.Location, modulePath []string, trait, name string) []symbol.Location
}

func resolveTier1Symbol(adapter tier1Resolver, language string, query symbol.Query) (symbol.Location, error) {
	done := telemetryPhase("symbol-resolution")
	defer done()
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

// resolveRustTier1Symbol parses a Rust qualified name and narrows the
// workspace/symbol matches via the LSP adapter's Rust candidate filter. The
// domain logic (container matching, trait resolution) lives in backend/lsp; the
// CLI only orchestrates and maps the outcome onto refute's error types.
func resolveRustTier1Symbol(adapter tier1Resolver, query symbol.Query) (symbol.Location, error) {
	modulePath, trait, name, err := symbol.ParseRustQualifiedName(query.QualifiedName)
	if err != nil {
		return symbol.Location{}, &tier1RustSymbolParseError{err: err}
	}
	infos, err := adapter.FindSymbol(symbol.Query{QualifiedName: name})
	if err != nil {
		return symbol.Location{}, &tier1SymbolResolutionError{err: err}
	}
	candidates := adapter.FilterRustCandidates(infos, modulePath, trait, name)
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
	telemetrySetContext(ctx)
	telemetrySetStatus(edit.StatusAmbiguous)
	telemetrySetError("ambiguous", "multiple candidates")
	if flagJSON {
		res := &edit.JSONResult{
			SchemaVersion:  edit.SchemaVersion,
			Status:         edit.StatusAmbiguous,
			Operation:      ctx.Operation,
			Language:       ctx.Language,
			Backend:        ctx.Backend,
			BackendVersion: ctx.BackendVersion,
			WorkspaceRoot:  ctx.WorkspaceRoot,
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
		// Mark the envelope as already written so routeOperationError does not
		// emit a second one when this bubbles up through runRenameInner.
		return &jsonEmitted{err: &ExitCodeError{Code: 1}}
	}
	msg := "Ambiguous — multiple candidates:\n"
	for _, l := range locs {
		msg += fmt.Sprintf("  %s:%d:%d  %s\n", l.File, l.Line, l.Column, l.Name)
	}
	msg += "Use --file and --line to narrow the selection."
	return &ExitCodeError{Code: 1, Message: msg}
}
