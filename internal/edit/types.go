package edit

// Position in a text document. Line and Character are 0-indexed; Character is
// a byte offset within the line. LSP adapters convert to/from UTF-16 at the
// protocol boundary.
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
	FileEdits     []FileEdit
	FromCodeAction bool // true for extract/inline edits; enables snippet-placeholder stripping
}
