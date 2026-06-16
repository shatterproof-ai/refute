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

// FileOpKind identifies a non-text file operation in a WorkspaceEdit, mirroring
// the LSP CreateFile/RenameFile/DeleteFile documentChanges entries.
type FileOpKind string

const (
	FileOpCreate FileOpKind = "create"
	FileOpRename FileOpKind = "rename"
	FileOpDelete FileOpKind = "delete"
)

// FileOperation is a create, rename, or delete of a file. Per the LSP spec a
// CreateFile produces an empty file (its contents arrive via a subsequent
// TextDocumentEdit), so FileOperation carries no content of its own.
//
// Path is the create/delete target and the rename source. NewPath is the
// rename destination and is empty for create and delete.
type FileOperation struct {
	Kind    FileOpKind
	Path    string
	NewPath string
	// Overwrite applies to create and rename: replace an existing destination.
	Overwrite bool
	// IgnoreIfExists applies to create and rename: skip when the destination
	// already exists (Overwrite takes precedence).
	IgnoreIfExists bool
	// Recursive and IgnoreIfNotExists apply to delete.
	Recursive         bool
	IgnoreIfNotExists bool
}

// WorkspaceEdit describes changes across multiple files: text edits plus
// optional create/rename/delete file operations.
//
// When both are present, file operations are applied in a fixed order relative
// to text edits: creates first (so a subsequent text edit can populate a new
// file), then text edits, then renames, then deletes. This covers the
// "extract to new file" shape (create + edits). The whole batch is applied
// atomically: any failure rolls every applied step back.
type WorkspaceEdit struct {
	FileEdits      []FileEdit
	FileOps        []FileOperation
	FromCodeAction bool // true for extract/inline edits; enables snippet-placeholder stripping
}
