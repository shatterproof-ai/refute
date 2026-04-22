package lsp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

// Compile-time interface check.
var _ backend.RefactoringBackend = (*Adapter)(nil)

// Adapter wraps the LSP Client to implement backend.RefactoringBackend.
type Adapter struct {
	cfg          config.ServerConfig
	languageID   string
	filePatterns []string
	client       *Client
}

// NewAdapter creates an Adapter that will use the given ServerConfig and
// language ID. filePatterns is a list of glob patterns identifying source
// files for this language (used for future operations).
func NewAdapter(cfg config.ServerConfig, languageID string, filePatterns []string) *Adapter {
	return &Adapter{
		cfg:          cfg,
		languageID:   languageID,
		filePatterns: filePatterns,
	}
}

// Initialize starts the LSP client with the given workspace root.
func (a *Adapter) Initialize(workspaceRoot string) error {
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("abs workspace root: %w", err)
	}

	client, err := StartClient(a.cfg.Command, a.cfg.Args, absRoot)
	if err != nil {
		return fmt.Errorf("start LSP client: %w", err)
	}

	a.client = client

	if isTSFamily(a.languageID) {
		_ = PrimeTSWorkspace(a.client, absRoot)
	}

	// Wait for the server to finish its initial indexing pass. LSP servers like
	// rust-analyzer emit $/progress notifications while indexing and cannot
	// reliably serve rename requests until indexing is complete.
	const indexingTimeout = 120 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), indexingTimeout)
	defer cancel()
	if err := client.WaitForIdle(ctx); err != nil {
		return fmt.Errorf("waiting for server ready: %w", err)
	}
	return nil
}

// Shutdown stops the LSP server.
func (a *Adapter) Shutdown() error {
	if a.client == nil {
		return nil
	}
	return a.client.Shutdown()
}

// DidOpen exposes the file-open notification for callers that need to prime
// the server before issuing FindSymbol or other queries.
func (a *Adapter) DidOpen(filePath string) error {
	if a.client == nil {
		return fmt.Errorf("adapter not initialized")
	}
	return a.client.DidOpen(filePath, a.languageID)
}

// FindSymbol resolves a Tier 1 qualified name via workspace/symbol.
// Supported forms:
//
//	"Name"               — bare symbol name
//	"pkg.Name"           — package-qualified (lowercase first component)
//	"Type.Method"        — type-qualified (uppercase first component)
//	"pkg.Type.Method"    — three-part: container must match Type
//
// Returns ErrSymbolNotFound when nothing matches.
func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	parts := parseQualifiedName(query.QualifiedName)
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return nil, fmt.Errorf("empty qualified name")
	}
	leaf := parts[len(parts)-1]

	syms, err := a.client.WorkspaceSymbol(leaf)
	if err != nil {
		return nil, err
	}

	var matches []symbol.Location
	for _, s := range syms {
		if s.Name != leaf {
			continue
		}
		if !qualifiedNameMatches(parts, s) {
			continue
		}
		if query.Kind != symbol.KindUnknown && lspKindToSymbolKind(s.Kind) != query.Kind {
			continue
		}
		matches = append(matches, symbol.Location{
			File:   uriToFile(s.Location.URI),
			Line:   s.Location.Range.Start.Line + 1,
			Column: s.Location.Range.Start.Character + 1,
			Name:   s.Name,
			Kind:   lspKindToSymbolKind(s.Kind),
		})
	}

	if len(matches) == 0 {
		return nil, backend.ErrSymbolNotFound
	}
	return matches, nil
}

func parseQualifiedName(name string) []string {
	if name == "" {
		return nil
	}
	return strings.Split(name, ".")
}

// qualifiedNameMatches reports whether workspace symbol s matches the
// qualified-name parts per the disambiguation rules documented on FindSymbol.
func qualifiedNameMatches(parts []string, s WorkspaceSymbolInfo) bool {
	switch len(parts) {
	case 1:
		return true
	case 2:
		first := parts[0]
		if startsUppercase(first) {
			return s.ContainerName == first
		}
		return s.ContainerName == first ||
			strings.HasSuffix(s.ContainerName, "/"+first)
	case 3:
		typeName := parts[1]
		return s.ContainerName == typeName ||
			strings.HasSuffix(s.ContainerName, "."+typeName) ||
			strings.HasSuffix(s.ContainerName, "/"+typeName)
	default:
		return false
	}
}

func startsUppercase(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsUpper([]rune(s)[0])
}

// lspKindToSymbolKind maps LSP SymbolKind integers to refute's SymbolKind.
func lspKindToSymbolKind(lspKind int) symbol.SymbolKind {
	switch lspKind {
	case 5: // Class
		return symbol.KindClass
	case 6: // Method
		return symbol.KindMethod
	case 7, 8: // Property, Field
		return symbol.KindField
	case 12: // Function
		return symbol.KindFunction
	case 13, 14: // Variable, Constant
		return symbol.KindVariable
	case 22: // EnumMember
		return symbol.KindField
	case 23: // Struct
		return symbol.KindType
	case 26: // TypeParameter
		return symbol.KindType
	default:
		return symbol.KindUnknown
	}
}

// Rename converts the 1-indexed Location to a 0-indexed LSP position, calls
// DidOpen on the file, then requests a rename from the server.
func (a *Adapter) Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}

	// Convert 1-indexed to 0-indexed for LSP.
	lspLine := loc.Line - 1
	lspCharacter := loc.Column - 1

	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, fmt.Errorf("DidOpen %s: %w", loc.File, err)
	}

	// Wait for any DidOpen-triggered analysis to settle before sending rename.
	const analysisTimeout = 30 * time.Second
	waitCtx, waitCancel := context.WithTimeout(context.Background(), analysisTimeout)
	defer waitCancel()
	if err := a.client.WaitForIdle(waitCtx); err != nil {
		return nil, fmt.Errorf("waiting for analysis: %w", err)
	}

	// Retry on ContentModified: servers like rust-analyzer cancel rename
	// requests when background salsa invalidation races with the request.
	const (
		renameMaxRetries = 5
		renameRetryDelay = 750 * time.Millisecond
	)
	var fileEdits []edit.FileEdit
	for attempt := 0; attempt < renameMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(renameRetryDelay)
		}
		var err error
		fileEdits, err = a.client.Rename(loc.File, lspLine, lspCharacter, newName)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrContentModified) {
			return nil, fmt.Errorf("rename: %w", err)
		}
		if attempt == renameMaxRetries-1 {
			return nil, fmt.Errorf("rename: server state did not settle after %d attempts: %w", renameMaxRetries, err)
		}
	}

	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

func (a *Adapter) ExtractFunction(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	we, placeholder, err := a.extractImpl(r, "function")
	if err != nil {
		return nil, err
	}
	if name != "" && placeholder != "" && placeholder != name {
		rewritePlaceholder(we, placeholder, name)
	}
	return we, nil
}

func (a *Adapter) ExtractVariable(r symbol.SourceRange, name string) (*edit.WorkspaceEdit, error) {
	we, placeholder, err := a.extractImpl(r, "variable")
	if err != nil {
		return nil, err
	}
	if name != "" && placeholder != "" && placeholder != name {
		rewritePlaceholder(we, placeholder, name)
	}
	return we, nil
}

func (a *Adapter) extractImpl(r symbol.SourceRange, kind string) (*edit.WorkspaceEdit, string, error) {
	if a.client == nil {
		return nil, "", fmt.Errorf("adapter not initialized")
	}
	if err := a.client.DidOpen(r.File, a.languageID); err != nil {
		return nil, "", fmt.Errorf("DidOpen %s: %w", r.File, err)
	}
	const analysisTimeout = 30 * time.Second
	waitCtx, waitCancel := context.WithTimeout(context.Background(), analysisTimeout)
	defer waitCancel()
	if err := a.client.WaitForIdle(waitCtx); err != nil {
		return nil, "", fmt.Errorf("waiting for analysis: %w", err)
	}
	actions, err := a.client.CodeActions(
		r.File,
		r.StartLine-1, r.StartCol-1,
		r.EndLine-1, r.EndCol-1,
		[]string{"refactor.extract"},
	)
	if err != nil {
		return nil, "", err
	}

	kindSuffix := "refactor.extract." + kind
	titleNeedle := kind

	for _, action := range actions {
		if !strings.HasPrefix(action.Kind, kindSuffix) &&
			!strings.Contains(strings.ToLower(action.Title), titleNeedle) {
			continue
		}
		we, err := a.resolveAction(action)
		if err != nil {
			return nil, "", err
		}
		placeholder := findExtractPlaceholder(we, kind)
		return we, placeholder, nil
	}
	return nil, "", backend.ErrUnsupported
}

func (a *Adapter) resolveAction(action CodeAction) (*edit.WorkspaceEdit, error) {
	var fileEdits []edit.FileEdit
	var err error
	if action.Edit != nil {
		fileEdits, err = parseWorkspaceEdit(*action.Edit)
	} else {
		fileEdits, err = a.client.ResolveCodeActionEdit(action)
	}
	if err != nil {
		return nil, err
	}
	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

func findExtractPlaceholder(we *edit.WorkspaceEdit, kind string) string {
	for _, fe := range we.FileEdits {
		for _, te := range fe.Edits {
			if te.NewText == "" {
				continue
			}
			var id string
			switch kind {
			case "function":
				id = matchIdentAfter(te.NewText, "func ")
			case "variable":
				id = matchIdentBefore(te.NewText, " :=")
				if id == "" {
					id = matchIdentBefore(te.NewText, ":=")
				}
			}
			if id != "" {
				return id
			}
		}
	}
	return ""
}

func matchIdentAfter(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	rest := s[i+len(needle):]
	end := 0
	for end < len(rest) && isIdentByte(rest[end]) {
		end++
	}
	if end == 0 {
		return ""
	}
	return rest[:end]
}

func matchIdentBefore(s, needle string) string {
	i := strings.Index(s, needle)
	if i <= 0 {
		return ""
	}
	start := i
	for start > 0 && isIdentByte(s[start-1]) {
		start--
	}
	if start == i {
		return ""
	}
	return s[start:i]
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_'
}

func rewritePlaceholder(we *edit.WorkspaceEdit, old, newID string) {
	for fi := range we.FileEdits {
		for ei := range we.FileEdits[fi].Edits {
			we.FileEdits[fi].Edits[ei].NewText = replaceWholeIdent(
				we.FileEdits[fi].Edits[ei].NewText, old, newID,
			)
		}
	}
}

func replaceWholeIdent(s, old, newID string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		j := strings.Index(s[i:], old)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		j += i
		leftOK := j == 0 || !isIdentByte(s[j-1])
		rightIdx := j + len(old)
		rightOK := rightIdx >= len(s) || !isIdentByte(s[rightIdx])
		b.WriteString(s[i:j])
		if leftOK && rightOK {
			b.WriteString(newID)
		} else {
			b.WriteString(old)
		}
		i = rightIdx
	}
	return b.String()
}

// ReplaceWholeIdentForTest is a test-only export of replaceWholeIdent.
func ReplaceWholeIdentForTest(s, old, newID string) string {
	return replaceWholeIdent(s, old, newID)
}

// InlineSymbol requests a refactor.inline code action over the symbol's
// identifier-width range. Gopls returns no actions for a zero-width range, so
// the request covers the whole identifier (min 1 char).
func (a *Adapter) InlineSymbol(loc symbol.Location) (*edit.WorkspaceEdit, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	if err := a.client.DidOpen(loc.File, a.languageID); err != nil {
		return nil, fmt.Errorf("DidOpen %s: %w", loc.File, err)
	}
	const analysisTimeout = 30 * time.Second
	waitCtx, waitCancel := context.WithTimeout(context.Background(), analysisTimeout)
	defer waitCancel()
	if err := a.client.WaitForIdle(waitCtx); err != nil {
		return nil, fmt.Errorf("waiting for analysis: %w", err)
	}

	startLine := loc.Line - 1
	startChar := loc.Column - 1
	endChar := startChar + max(len(loc.Name), 1)

	actions, err := a.client.CodeActions(
		loc.File,
		startLine, startChar, startLine, endChar,
		[]string{"refactor.inline"},
	)
	if err != nil {
		return nil, err
	}
	for _, action := range actions {
		if strings.HasPrefix(action.Kind, "refactor.inline") ||
			strings.Contains(strings.ToLower(action.Title), "inline") {
			return a.resolveAction(action)
		}
	}
	return nil, backend.ErrUnsupported
}

// MoveToFile returns ErrUnsupported — not yet implemented via LSP.
func (a *Adapter) MoveToFile(_ symbol.Location, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// Capabilities returns the list of operations this adapter supports.
func (a *Adapter) Capabilities() []backend.Capability {
	return []backend.Capability{
		{Operation: "rename"},
	}
}
