package edit

// Position in a text document. 0-indexed line and character, matching LSP convention.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// TextEdit is a single edit: replace the Range with NewText.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// FileEdit holds all edits for a single file.
type FileEdit struct {
	Path  string
	Edits []TextEdit
}

// WorkspaceEdit describes changes across multiple files.
type WorkspaceEdit struct {
	FileEdits []FileEdit
}
