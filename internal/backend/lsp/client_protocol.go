package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// DidOpen notifies the server a file is open (reads file content, sends textDocument/didOpen).
func (c *Client) DidOpen(filePath string, languageID string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	type textDocumentItem struct {
		URI        string `json:"uri"`
		LanguageID string `json:"languageId"`
		Version    int    `json:"version"`
		Text       string `json:"text"`
	}
	type didOpenParams struct {
		TextDocument textDocumentItem `json:"textDocument"`
	}

	return c.notify("textDocument/didOpen", didOpenParams{
		TextDocument: textDocumentItem{
			URI:        fileToURI(absPath),
			LanguageID: languageID,
			Version:    1,
			Text:       string(content),
		},
	})
}

// Rename sends textDocument/rename and returns file edits.
// line and character are 0-indexed (LSP convention).
func (c *Client) Rename(filePath string, line, character int, newName string) ([]edit.FileEdit, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	type position struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type textDocumentIdentifier struct {
		URI string `json:"uri"`
	}
	type renameParams struct {
		TextDocument textDocumentIdentifier `json:"textDocument"`
		Position     position               `json:"position"`
		NewName      string                 `json:"newName"`
	}

	result, err := c.request("textDocument/rename", renameParams{
		TextDocument: textDocumentIdentifier{URI: fileToURI(absPath)},
		Position:     position{Line: line, Character: character},
		NewName:      newName,
	})
	if err != nil {
		if isLSPError(err, lspContentModified) {
			return nil, fmt.Errorf("rename request: %w", ErrContentModified)
		}
		if isRetryableRenameError(err) {
			return nil, fmt.Errorf("rename request: %w", ErrRenamePositionUnavailable)
		}
		return nil, fmt.Errorf("rename request: %w", err)
	}

	// A rename only ever produces text edits, never file ops, so return the
	// text edits directly to preserve the adapter's retry loop.
	we, err := parseWorkspaceEdit(result)
	if err != nil {
		return nil, err
	}
	if we == nil {
		return nil, nil
	}
	return we.FileEdits, nil
}

// CodeAction is an LSP code action (refactoring, quick fix, etc.).
type CodeAction struct {
	Title   string           `json:"title"`
	Kind    string           `json:"kind,omitempty"`
	Edit    *json.RawMessage `json:"edit,omitempty"`
	Data    *json.RawMessage `json:"data,omitempty"`
	Command *json.RawMessage `json:"command,omitempty"`
}

// WorkspaceSymbolInfo is a single result from workspace/symbol.
type WorkspaceSymbolInfo struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	} `json:"location"`
}

// CodeActions requests code actions for a range. kinds filters by action kind
// prefix (e.g., []string{"refactor.extract"} returns only extract actions).
// All positions are 0-indexed (LSP convention).
func (c *Client) CodeActions(filePath string, startLine, startChar, endLine, endChar int, kinds []string) ([]CodeAction, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": fileToURI(absPath)},
		"range": map[string]any{
			"start": map[string]any{"line": startLine, "character": startChar},
			"end":   map[string]any{"line": endLine, "character": endChar},
		},
		"context": map[string]any{
			"diagnostics": []any{},
			"only":        kinds,
		},
	}
	result, err := c.request("textDocument/codeAction", params)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var actions []CodeAction
	if err := json.Unmarshal(result, &actions); err != nil {
		return nil, fmt.Errorf("parsing code actions: %w", err)
	}
	return actions, nil
}

// ResolveCodeActionEdit resolves a code action to its file edits. Use when the
// action returned by CodeActions has no Edit field attached.
func (c *Client) ResolveCodeActionEdit(action CodeAction) (*edit.WorkspaceEdit, error) {
	result, err := c.request("codeAction/resolve", action)
	if err != nil {
		return nil, err
	}
	var resolved CodeAction
	if err := json.Unmarshal(result, &resolved); err != nil {
		return nil, fmt.Errorf("parsing resolved code action: %w", err)
	}
	if resolved.Edit == nil {
		return nil, fmt.Errorf("resolved code action %q has no edit", resolved.Title)
	}
	return parseWorkspaceEdit(*resolved.Edit)
}

// WorkspaceSymbol queries the server for symbols matching query. Results are
// limited to packages the server has already loaded — callers that need broad
// coverage should prime the workspace first.
func (c *Client) WorkspaceSymbol(query string) ([]WorkspaceSymbolInfo, error) {
	result, err := c.request("workspace/symbol", map[string]any{"query": query})
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || string(result) == "null" {
		return nil, nil
	}
	var syms []WorkspaceSymbolInfo
	if err := json.Unmarshal(result, &syms); err != nil {
		return nil, fmt.Errorf("parsing workspace symbols: %w", err)
	}
	return syms, nil
}

// DocumentSymbol holds a hierarchical symbol entry from textDocument/documentSymbol.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// Range mirrors the LSP Range type (0-indexed).
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position mirrors the LSP Position type (0-indexed).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// DocumentSymbol requests hierarchical document symbols for a file. If
// rust-analyzer returns the flat SymbolInformation form instead, callers
// must fall back to the cheap branch for that file.
func (c *Client) DocumentSymbol(path string) ([]DocumentSymbol, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("abs path: %w", err)
	}
	params := map[string]any{
		"textDocument": map[string]any{"uri": fileToURI(absPath)},
	}
	raw, err := c.request("textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var result []DocumentSymbol
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parsing document symbols: %w", err)
	}
	return result, nil
}
