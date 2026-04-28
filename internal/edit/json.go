package edit

import (
	"encoding/json"
)

// JSONChange is a single text edit in JSON output. All positions are 1-indexed.
type JSONChange struct {
	StartLine int    `json:"startLine"`
	StartCol  int    `json:"startCol"`
	EndLine   int    `json:"endLine"`
	EndCol    int    `json:"endCol"`
	NewText   string `json:"newText"`
}

// JSONFileEdit groups changes by file.
type JSONFileEdit struct {
	File    string       `json:"file"`
	Changes []JSONChange `json:"changes"`
}

// JSONSymbolLoc is a 1-indexed symbol location used for the newSymbol field.
type JSONSymbolLoc struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Name   string `json:"name"`
}

// JSONResult is the full refute JSON output envelope.
type JSONResult struct {
	Status        string          `json:"status"` // "applied" | "dry-run" | "no-op" | "ambiguous"
	FilesModified int             `json:"filesModified"`
	Edits         []JSONFileEdit  `json:"edits,omitempty"`
	NewSymbol     *JSONSymbolLoc  `json:"newSymbol,omitempty"`
	Candidates    []JSONSymbolLoc `json:"candidates,omitempty"`
}

// RenderJSON converts a WorkspaceEdit into the JSONResult envelope with the
// given status. Positions are converted from LSP's 0-indexed to refute's
// 1-indexed convention.
func RenderJSON(we *WorkspaceEdit, status string) *JSONResult {
	res := &JSONResult{Status: status}
	if we == nil {
		return res
	}
	for _, fe := range we.FileEdits {
		if len(fe.Edits) == 0 {
			continue
		}
		jfe := JSONFileEdit{File: fe.Path}
		for _, te := range fe.Edits {
			jfe.Changes = append(jfe.Changes, JSONChange{
				StartLine: te.Range.Start.Line + 1,
				StartCol:  te.Range.Start.Character + 1,
				EndLine:   te.Range.End.Line + 1,
				EndCol:    te.Range.End.Character + 1,
				NewText:   te.NewText,
			})
		}
		res.Edits = append(res.Edits, jfe)
	}
	res.FilesModified = len(res.Edits)
	return res
}

// Marshal returns indented JSON suitable for stdout.
func (r *JSONResult) Marshal() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
