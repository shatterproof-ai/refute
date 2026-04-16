package lsp

import (
	"fmt"
	"path/filepath"

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
	return nil
}

// Shutdown stops the LSP server.
func (a *Adapter) Shutdown() error {
	if a.client == nil {
		return nil
	}
	return a.client.Shutdown()
}

// FindSymbol returns ErrUnsupported — not yet implemented via LSP.
func (a *Adapter) FindSymbol(_ symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
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

	fileEdits, err := a.client.Rename(loc.File, lspLine, lspCharacter, newName)
	if err != nil {
		return nil, fmt.Errorf("rename: %w", err)
	}

	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

// ExtractFunction returns ErrUnsupported — not yet implemented via LSP.
func (a *Adapter) ExtractFunction(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// ExtractVariable returns ErrUnsupported — not yet implemented via LSP.
func (a *Adapter) ExtractVariable(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

// InlineSymbol returns ErrUnsupported — not yet implemented via LSP.
func (a *Adapter) InlineSymbol(_ symbol.Location) (*edit.WorkspaceEdit, error) {
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
