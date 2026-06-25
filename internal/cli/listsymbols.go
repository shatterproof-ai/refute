package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

type listSymbolsFlags struct {
	Query string
	File  string
	Kind  string
	Lang  string
}

// listStatusOK is the JSON status for a successful listing. list-symbols is a
// read-only discovery query rather than an edit, so the edit-oriented status
// constants (applied/dry-run/no-op) do not fit; an empty result is still a
// successful "ok" listing, not an error.
const listStatusOK = "ok"

// listSymbol is one discovery candidate. It carries everything an agent needs
// to feed a Tier-3 operation without guessing: exact file/line/column plus the
// symbol kind and qualified name.
type listSymbol struct {
	File          string `json:"file"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Container     string `json:"container,omitempty"`
	QualifiedName string `json:"qualifiedName"`
}

// listSymbolsResult is the `refute list-symbols --json` envelope. It mirrors
// the shared edit.JSONResult metadata fields but carries a symbols array
// instead of edits. Consumers branch on schemaVersion and status.
type listSymbolsResult struct {
	SchemaVersion  string          `json:"schemaVersion"`
	Status         string          `json:"status"`
	Operation      string          `json:"operation,omitempty"`
	Language       string          `json:"language,omitempty"`
	Backend        string          `json:"backend,omitempty"`
	BackendVersion string          `json:"backendVersion,omitempty"`
	WorkspaceRoot  string          `json:"workspaceRoot,omitempty"`
	Query          string          `json:"query,omitempty"`
	Symbols        []listSymbol    `json:"symbols"`
	Warnings       []string        `json:"warnings,omitempty"`
	Error          *edit.JSONError `json:"error,omitempty"`
}

func init() {
	flags := &listSymbolsFlags{}
	cmd := &cobra.Command{
		Use:   "list-symbols",
		Short: "Discover candidate symbols before refactoring (Go, Rust, TypeScript)",
		Long: `List workspace symbols so agents can discover exact file/line/column
candidates before requesting a refactoring.

Symbols are resolved via the LSP workspace/symbol request. Each result includes
the file, 1-indexed line and column, kind, and qualified name — enough to feed
a Tier-3 operation directly.

The target language is taken from --lang, else detected from --file, else
defaults to Go. Languages without an LSP backend (e.g. Java, Kotlin) return a
structured unsupported result. See ` + supportMatrixURL + `.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runListSymbols(flags)
		},
	}
	cmd.Flags().StringVar(&flags.Query, "query", "", "symbol name query passed to workspace/symbol (empty lists all primed symbols)")
	cmd.Flags().StringVar(&flags.File, "file", "", "limit results to symbols declared in this file (also selects the language)")
	cmd.Flags().StringVar(&flags.Kind, "kind", "", "filter by kind: function, class, method, field, variable, type, parameter")
	cmd.Flags().StringVar(&flags.Lang, "lang", "", "language to query (overrides detection from --file; defaults to go)")
	cmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
	RootCmd.AddCommand(cmd)
}

func runListSymbols(flags *listSymbolsFlags) error {
	telemetrySetContext(jsonContext{Operation: "list-symbols"})

	kind, ok := parseSymbolKindFilter(flags.Kind)
	if !ok {
		return fmt.Errorf("invalid --kind %q (want one of: function, class, method, field, variable, type, parameter)", flags.Kind)
	}

	var fileScope string
	if flags.File != "" {
		abs, err := filepath.Abs(flags.File)
		if err != nil {
			return fmt.Errorf("resolving file path: %w", err)
		}
		// Resolve symlinks so the scope matches the on-disk paths LSP servers
		// report; otherwise an exact-string file filter can silently drop every
		// candidate. Best-effort: fall back to the absolute path if the file
		// does not exist yet or cannot be resolved.
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		fileScope = abs
	}

	lang, err := resolveListLanguage(fileScope, flags)
	if err != nil {
		return err
	}

	workspaceRoot, err := tier1WorkspaceRoot("")
	if err != nil {
		return err
	}

	ctx := jsonContext{Operation: "list-symbols", Language: lang, Backend: "lsp", WorkspaceRoot: workspaceRoot}

	if support, ok := config.SupportFor(lang); !ok || support.Level == config.LevelUnsupported {
		return unsupportedListLanguage(ctx, lang, support)
	}

	adapter, ctx, err := setupListSymbolsBackend(lang, fileScope, workspaceRoot)
	if err != nil {
		if flagJSON {
			return emitJSONBackendSetupError(ctx, err)
		}
		return err
	}
	defer func() { _ = adapter.Shutdown() }()

	queryDone := telemetryPhase("symbol-discovery")
	syms, err := adapter.ListSymbols(flags.Query)
	queryDone()
	if err != nil {
		if flagJSON {
			return emitJSONError(ctx, edit.StatusBackendFailed, "operation-failed", err.Error(), "")
		}
		return fmt.Errorf("listing symbols: %w", err)
	}

	filtered := filterListSymbols(syms, fileScope, kind)

	// A file scope that removes every candidate from a non-empty result set is
	// almost always a path the server reports differently (or a typo), not a
	// genuinely symbol-free file. Surface it rather than returning a silent
	// empty listing, which a caller cannot distinguish from "no such symbol".
	var warnings []string
	if fileScope != "" && len(syms) > 0 && len(filterListSymbols(syms, fileScope, symbol.KindUnknown)) == 0 {
		warnings = append(warnings, fmt.Sprintf(
			"no symbols matched file scope %s, though the server reported %d symbol(s) elsewhere; check the --file path",
			flags.File, len(syms)))
	}

	return emitListSymbols(ctx, filtered, warnings, flags)
}

// resolveListLanguage picks the language key: explicit --lang wins, then
// detection from the scoped file. It falls back to the Go primary target only
// when neither --lang nor --file is given; an unrecognized --file extension is
// an explicit error so the caller is not silently switched to Go.
func resolveListLanguage(fileScope string, flags *listSymbolsFlags) (string, error) {
	if flags.Lang != "" {
		return flags.Lang, nil
	}
	if fileScope != "" {
		key := DetectServerKey(fileScope)
		if key == "" {
			return "", fmt.Errorf("cannot determine language for %q; pass --lang explicitly", flags.File)
		}
		return key, nil
	}
	return "go", nil
}

func setupListSymbolsBackend(lang, fileScope, workspaceRoot string) (*lsp.Adapter, jsonContext, error) {
	adapter, ctx, err := setupLSPBackend("list-symbols", lang, fileScope, workspaceRoot)
	if err != nil {
		// Preserve list-symbols' phase-specific error wrapping. Selection-phase
		// errors already carry their final form and are returned as-is.
		var setupErr *lspBackendSetupError
		if errors.As(err, &setupErr) {
			switch setupErr.phase {
			case "initialize":
				return nil, ctx, NewBackendInitFailure("lsp", setupErr.err)
			case "prime":
				return nil, ctx, fmt.Errorf("priming workspace: %w", setupErr.err)
			}
			return nil, ctx, setupErr.err
		}
		return nil, ctx, err
	}
	return adapter, ctx, nil
}

// unsupportedListLanguage reports a language refute cannot drive via LSP as a
// structured unsupported result so agents get a deterministic answer instead of
// a backend failure.
func unsupportedListLanguage(ctx jsonContext, lang string, support config.LanguageSupport) error {
	if support.Backend != "" {
		ctx.Backend = support.Backend
	}
	supported := supportedListLanguages()
	msg := fmt.Sprintf("list-symbols does not support language %q", lang)
	if support.Backend != "" {
		msg += fmt.Sprintf(" (backend: %s)", support.Backend)
	}
	hint := fmt.Sprintf("Supported languages: %s.", strings.Join(supported, ", "))
	if flagJSON {
		return emitJSONError(ctx, edit.StatusUnsupported, "unsupported-language", msg, hint)
	}
	return &ExitCodeError{Code: 1, Message: msg + "\n" + hint}
}

// supportedListLanguages lists the LSP-backed languages list-symbols can query,
// in support-matrix order.
func supportedListLanguages() []string {
	var out []string
	for _, row := range config.SupportMatrix {
		if row.Level != config.LevelUnsupported {
			out = append(out, row.Language)
		}
	}
	return out
}

// parseSymbolKindFilter maps a --kind flag value to a SymbolKind. An empty
// string means "no filter" and returns (KindUnknown, true). Unknown values
// return (KindUnknown, false).
func parseSymbolKindFilter(s string) (symbol.SymbolKind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return symbol.KindUnknown, true
	case "function":
		return symbol.KindFunction, true
	case "class":
		return symbol.KindClass, true
	case "field":
		return symbol.KindField, true
	case "variable":
		return symbol.KindVariable, true
	case "parameter":
		return symbol.KindParameter, true
	case "type":
		return symbol.KindType, true
	case "method":
		return symbol.KindMethod, true
	default:
		return symbol.KindUnknown, false
	}
}

// filterListSymbols narrows discovered symbols to a file scope and/or kind.
// A zero-value scope or KindUnknown disables that filter.
func filterListSymbols(syms []symbol.Location, fileScope string, kind symbol.SymbolKind) []symbol.Location {
	out := make([]symbol.Location, 0, len(syms))
	for _, s := range syms {
		if fileScope != "" && s.File != fileScope {
			continue
		}
		if kind != symbol.KindUnknown && s.Kind != kind {
			continue
		}
		out = append(out, s)
	}
	return out
}

// qualifiedSymbolName joins a symbol's container and name into a dotted
// qualified name, falling back to the bare name when no container is reported.
func qualifiedSymbolName(loc symbol.Location) string {
	if loc.Container == "" {
		return loc.Name
	}
	return loc.Container + "." + loc.Name
}

func emitListSymbols(ctx jsonContext, syms []symbol.Location, warnings []string, flags *listSymbolsFlags) error {
	telemetrySetContext(ctx)
	telemetrySetStatus(listStatusOK)
	if flagJSON {
		data, err := renderListSymbolsJSON(ctx, syms, warnings, flags.Query)
		if err != nil {
			return fmt.Errorf("marshalling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	for _, w := range warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if len(syms) == 0 {
		fmt.Fprintln(os.Stderr, "No symbols found.")
		return nil
	}
	for _, s := range syms {
		fmt.Printf("%s:%d:%d\t%s\t%s\n", s.File, s.Line, s.Column, s.Kind, qualifiedSymbolName(s))
	}
	return nil
}

func renderListSymbolsJSON(ctx jsonContext, syms []symbol.Location, warnings []string, query string) ([]byte, error) {
	sorted := append([]symbol.Location(nil), syms...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].Column < sorted[j].Column
	})

	res := listSymbolsResult{
		SchemaVersion:  edit.SchemaVersion,
		Status:         listStatusOK,
		Operation:      ctx.Operation,
		Language:       ctx.Language,
		Backend:        ctx.Backend,
		BackendVersion: ctx.BackendVersion,
		WorkspaceRoot:  ctx.WorkspaceRoot,
		Query:          query,
		Symbols:        make([]listSymbol, 0, len(sorted)),
		Warnings:       warnings,
	}
	for _, s := range sorted {
		res.Symbols = append(res.Symbols, listSymbol{
			File:          s.File,
			Line:          s.Line,
			Column:        s.Column,
			Kind:          s.Kind.String(),
			Name:          s.Name,
			Container:     s.Container,
			QualifiedName: qualifiedSymbolName(s),
		})
	}
	return json.MarshalIndent(res, "", "  ")
}
