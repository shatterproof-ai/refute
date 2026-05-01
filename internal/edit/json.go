package edit

import (
	"encoding/json"
)

// SchemaVersion identifies the JSON envelope shape. Bump when removing or
// renaming a field. Adding new optional fields with `omitempty` does not
// require a bump.
const SchemaVersion = "1"

// Status values for JSONResult.Status. Success-path statuses describe the
// outcome of a refactoring; error-path statuses describe why no edit was
// produced.
const (
	StatusApplied         = "applied"
	StatusDryRun          = "dry-run"
	StatusNoOp            = "no-op"
	StatusAmbiguous       = "ambiguous"
	StatusUnsupported     = "unsupported"
	StatusBackendMissing  = "backend-missing"
	StatusBackendFailed   = "backend-failed"
	StatusInvalidPosition = "invalid-position"
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

// JSONError describes why an operation could not produce edits.
type JSONError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// JSONResult is the refute JSON output envelope. Consumers should branch on
// SchemaVersion and Status; all other fields are optional and may be absent
// for a given status.
type JSONResult struct {
	SchemaVersion string          `json:"schemaVersion"`
	Status        string          `json:"status"`
	Operation     string          `json:"operation,omitempty"`
	Language      string          `json:"language,omitempty"`
	Backend       string          `json:"backend,omitempty"`
	WorkspaceRoot string          `json:"workspaceRoot,omitempty"`
	FilesModified int             `json:"filesModified"`
	Edits         []JSONFileEdit  `json:"edits,omitempty"`
	NewSymbol     *JSONSymbolLoc  `json:"newSymbol,omitempty"`
	Candidates    []JSONSymbolLoc `json:"candidates,omitempty"`
	Warnings      []string        `json:"warnings,omitempty"`
	Error         *JSONError      `json:"error,omitempty"`
}

// RenderJSON converts a WorkspaceEdit into a JSONResult envelope with the
// given status. Positions are converted from LSP's 0-indexed to refute's
// 1-indexed convention. Caller is expected to populate Operation, Language,
// Backend, and WorkspaceRoot before marshalling.
func RenderJSON(we *WorkspaceEdit, status string) *JSONResult {
	res := &JSONResult{SchemaVersion: SchemaVersion, Status: status}
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
