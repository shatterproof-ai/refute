package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shatterproof-ai/refute/internal/edit"
)

// parseWorkspaceEdit converts an LSP WorkspaceEdit JSON result into an
// *edit.WorkspaceEdit. It handles both the `changes` map format and the
// `documentChanges` array format, including the CreateFile/RenameFile/
// DeleteFile resource operations that may appear in documentChanges (e.g. a
// gopls "extract to new file" action). Returns nil when the result carries no
// edits.
func parseWorkspaceEdit(raw json.RawMessage) (*edit.WorkspaceEdit, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	type lspPosition struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspRange struct {
		Start lspPosition `json:"start"`
		End   lspPosition `json:"end"`
	}
	type lspTextEdit struct {
		Range   lspRange `json:"range"`
		NewText string   `json:"newText"`
	}
	type lspTextDocumentEdit struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Edits []lspTextEdit `json:"edits"`
	}
	type lspWorkspaceEdit struct {
		Changes         map[string][]lspTextEdit `json:"changes"`
		DocumentChanges []json.RawMessage        `json:"documentChanges"`
	}

	var we lspWorkspaceEdit
	if err := json.Unmarshal(raw, &we); err != nil {
		return nil, fmt.Errorf("parse workspace edit: %w", err)
	}

	convertEdits := func(path string, lspEdits []lspTextEdit) ([]edit.TextEdit, error) {
		out := make([]edit.TextEdit, 0, len(lspEdits))
		for _, e := range lspEdits {
			startCharacter, err := utf16CharacterToByteColumnInFile(path, e.Range.Start.Line, e.Range.Start.Character)
			if err != nil {
				return nil, err
			}
			endCharacter, err := utf16CharacterToByteColumnInFile(path, e.Range.End.Line, e.Range.End.Character)
			if err != nil {
				return nil, err
			}
			out = append(out, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: startCharacter - 1},
					End:   edit.Position{Line: e.Range.End.Line, Character: endCharacter - 1},
				},
				NewText: e.NewText,
			})
		}
		return out, nil
	}

	// convertEditsForNewFile converts edits that target a file being created in
	// the same WorkspaceEdit. The file does not exist on disk yet, so columns
	// are converted against an empty document rather than read from the file.
	// A CreateFile yields an empty file, so servers insert content at the start
	// (line 0, character 0).
	convertEditsForNewFile := func(lspEdits []lspTextEdit) ([]edit.TextEdit, error) {
		out := make([]edit.TextEdit, 0, len(lspEdits))
		for _, e := range lspEdits {
			if e.Range.Start.Line != 0 || e.Range.End.Line != 0 {
				return nil, fmt.Errorf("edit on newly-created file references line %d-%d, but a created file starts empty", e.Range.Start.Line, e.Range.End.Line)
			}
			startByte, err := utf16CharacterToByteCharacter("", e.Range.Start.Character)
			if err != nil {
				return nil, fmt.Errorf("edit on newly-created file: %w", err)
			}
			endByte, err := utf16CharacterToByteCharacter("", e.Range.End.Character)
			if err != nil {
				return nil, fmt.Errorf("edit on newly-created file: %w", err)
			}
			out = append(out, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: 0, Character: startByte},
					End:   edit.Position{Line: 0, Character: endByte},
				},
				NewText: e.NewText,
			})
		}
		return out, nil
	}

	// Prefer documentChanges when present. Each entry is either a
	// TextDocumentEdit (no "kind") or a CreateFile/RenameFile/DeleteFile
	// resource operation (with "kind"). Text edits and file ops are collected
	// separately; the applier orders file ops relative to edits.
	if len(we.DocumentChanges) > 0 {
		result := &edit.WorkspaceEdit{}

		// First pass: collect the paths being created so text edits targeting
		// them are converted against an empty document rather than read from
		// disk (the file does not exist yet).
		createTargets := make(map[string]struct{})
		for _, rawDC := range we.DocumentChanges {
			var probe struct {
				Kind string `json:"kind"`
				URI  string `json:"uri"`
			}
			if err := json.Unmarshal(rawDC, &probe); err != nil {
				return nil, fmt.Errorf("parse documentChanges entry: %w", err)
			}
			if probe.Kind == "create" {
				if p := uriToFile(probe.URI); p != "" {
					createTargets[p] = struct{}{}
				}
			}
		}

		for _, rawDC := range we.DocumentChanges {
			var probe struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(rawDC, &probe); err != nil {
				return nil, fmt.Errorf("parse documentChanges entry: %w", err)
			}
			switch probe.Kind {
			case "":
				var dc lspTextDocumentEdit
				if err := json.Unmarshal(rawDC, &dc); err != nil {
					return nil, fmt.Errorf("parse textDocument edit: %w", err)
				}
				path := uriToFile(dc.TextDocument.URI)
				if path == "" {
					return nil, fmt.Errorf("documentChanges entry has empty or missing textDocument.uri")
				}
				var edits []edit.TextEdit
				var err error
				if _, isNew := createTargets[path]; isNew {
					edits, err = convertEditsForNewFile(dc.Edits)
				} else {
					edits, err = convertEdits(path, dc.Edits)
				}
				if err != nil {
					return nil, err
				}
				result.FileEdits = append(result.FileEdits, edit.FileEdit{Path: path, Edits: edits})
			case "create":
				var op struct {
					URI     string `json:"uri"`
					Options struct {
						Overwrite      bool `json:"overwrite"`
						IgnoreIfExists bool `json:"ignoreIfExists"`
					} `json:"options"`
				}
				if err := json.Unmarshal(rawDC, &op); err != nil {
					return nil, fmt.Errorf("parse create file op: %w", err)
				}
				path := uriToFile(op.URI)
				if path == "" {
					return nil, fmt.Errorf("create file op has empty or missing uri")
				}
				result.FileOps = append(result.FileOps, edit.FileOperation{
					Kind:           edit.FileOpCreate,
					Path:           path,
					Overwrite:      op.Options.Overwrite,
					IgnoreIfExists: op.Options.IgnoreIfExists,
				})
			case "rename":
				var op struct {
					OldURI  string `json:"oldUri"`
					NewURI  string `json:"newUri"`
					Options struct {
						Overwrite      bool `json:"overwrite"`
						IgnoreIfExists bool `json:"ignoreIfExists"`
					} `json:"options"`
				}
				if err := json.Unmarshal(rawDC, &op); err != nil {
					return nil, fmt.Errorf("parse rename file op: %w", err)
				}
				oldPath := uriToFile(op.OldURI)
				newPath := uriToFile(op.NewURI)
				if oldPath == "" || newPath == "" {
					return nil, fmt.Errorf("rename file op has empty or missing oldUri/newUri")
				}
				result.FileOps = append(result.FileOps, edit.FileOperation{
					Kind:           edit.FileOpRename,
					Path:           oldPath,
					NewPath:        newPath,
					Overwrite:      op.Options.Overwrite,
					IgnoreIfExists: op.Options.IgnoreIfExists,
				})
			case "delete":
				var op struct {
					URI     string `json:"uri"`
					Options struct {
						Recursive         bool `json:"recursive"`
						IgnoreIfNotExists bool `json:"ignoreIfNotExists"`
					} `json:"options"`
				}
				if err := json.Unmarshal(rawDC, &op); err != nil {
					return nil, fmt.Errorf("parse delete file op: %w", err)
				}
				path := uriToFile(op.URI)
				if path == "" {
					return nil, fmt.Errorf("delete file op has empty or missing uri")
				}
				result.FileOps = append(result.FileOps, edit.FileOperation{
					Kind:              edit.FileOpDelete,
					Path:              path,
					Recursive:         op.Options.Recursive,
					IgnoreIfNotExists: op.Options.IgnoreIfNotExists,
				})
			default:
				return nil, fmt.Errorf("unsupported documentChanges kind %q", probe.Kind)
			}
		}
		sortFileEditsByPath(result.FileEdits)
		return result, nil
	}

	// Fall back to changes map (text edits only).
	if len(we.Changes) > 0 {
		result := &edit.WorkspaceEdit{}
		uris := make([]string, 0, len(we.Changes))
		for uri := range we.Changes {
			uris = append(uris, uri)
		}
		sort.Strings(uris)

		for _, uri := range uris {
			path := uriToFile(uri)
			if path == "" {
				return nil, fmt.Errorf("changes entry has empty uri")
			}
			lspEdits := we.Changes[uri]
			edits, err := convertEdits(path, lspEdits)
			if err != nil {
				return nil, err
			}
			result.FileEdits = append(result.FileEdits, edit.FileEdit{Path: path, Edits: edits})
		}
		sortFileEditsByPath(result.FileEdits)
		return result, nil
	}

	return nil, nil
}

func sortFileEditsByPath(fileEdits []edit.FileEdit) {
	sort.SliceStable(fileEdits, func(i, j int) bool {
		return fileEdits[i].Path < fileEdits[j].Path
	})
}

// fileToURI converts an absolute file path to a file:// URI.
func fileToURI(path string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}
	return u.String()
}

// uriToFile converts a file:// URI to an absolute file path.
func uriToFile(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	path := u.Path
	// On Windows, url.Path starts with /C:/... — trim leading slash.
	if strings.HasPrefix(path, "/") && len(path) > 2 && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}
