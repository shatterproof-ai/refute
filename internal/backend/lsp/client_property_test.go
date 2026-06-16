package lsp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
	"pgregory.net/rapid"
)

const (
	maxWorkspaceEditRawBytes = 512
	maxWorkspaceEditEntries  = 4
	maxWorkspaceTextEdits    = 4
)

func TestParseWorkspaceEditArbitraryInputProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rapid.SliceOfN(rapid.Byte(), 0, maxWorkspaceEditRawBytes).Draw(t, "raw json")

		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("parseWorkspaceEdit panicked for %q: %v", raw, recovered)
			}
		}()

		we, err := parseWorkspaceEdit(json.RawMessage(raw))
		if err == nil && we != nil {
			assertNonEmptyFileEditPaths(t, we.FileEdits)
		}
	})
}

func TestParseWorkspaceEditDeterminismProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := drawWorkspaceEditJSON(t)

		first, firstErr := parseWorkspaceEdit(raw)
		second, secondErr := parseWorkspaceEdit(raw)

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("parseWorkspaceEdit is nondeterministic for %s:\nfirst:  %+v\nsecond: %+v", string(raw), first, second)
		}
		if errorString(firstErr) != errorString(secondErr) {
			t.Fatalf("parseWorkspaceEdit error is nondeterministic for %s:\nfirst:  %v\nsecond: %v", string(raw), firstErr, secondErr)
		}
		if firstErr == nil && first != nil {
			assertNonEmptyFileEditPaths(t, first.FileEdits)
		}
	})
}

func drawWorkspaceEditJSON(t *rapid.T) json.RawMessage {
	dir, err := os.MkdirTemp("", "refute-rapid-workspace-edit-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	shape := rapid.SampledFrom([]string{"changes", "documentChanges"}).Draw(t, "workspace edit shape")
	entries := rapid.IntRange(0, maxWorkspaceEditEntries).Draw(t, "workspace edit entries")
	validPaths := make([]string, 0, entries)
	for i := 0; i < entries; i++ {
		path := filepath.Join(dir, fmt.Sprintf("file-%d.go", i))
		if err := os.WriteFile(path, []byte("0123456789\nabcdefghij\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		validPaths = append(validPaths, path)
	}

	switch shape {
	case "changes":
		changes := make(map[string][]lspPropertyTextEdit, entries)
		for i := 0; i < entries; i++ {
			uri := drawWorkspaceEditURI(t, validPaths[i], i)
			changes[uri] = drawLSPPropertyTextEdits(t, i)
		}
		raw, err := json.Marshal(struct {
			Changes map[string][]lspPropertyTextEdit `json:"changes"`
		}{Changes: changes})
		if err != nil {
			t.Fatalf("marshal changes workspace edit: %v", err)
		}
		return raw
	default:
		documentChanges := make([]lspPropertyDocumentChange, 0, entries)
		for i := 0; i < entries; i++ {
			documentChanges = append(documentChanges, lspPropertyDocumentChange{
				TextDocument: lspPropertyTextDocument{URI: drawWorkspaceEditURI(t, validPaths[i], i)},
				Edits:        drawLSPPropertyTextEdits(t, i),
			})
		}
		raw, err := json.Marshal(struct {
			DocumentChanges []lspPropertyDocumentChange `json:"documentChanges"`
		}{DocumentChanges: documentChanges})
		if err != nil {
			t.Fatalf("marshal documentChanges workspace edit: %v", err)
		}
		return raw
	}
}

func drawWorkspaceEditURI(t *rapid.T, validPath string, entry int) string {
	kind := rapid.SampledFrom([]string{"valid", "empty", "missing-file"}).Draw(t, fmt.Sprintf("entry %d uri kind", entry))
	switch kind {
	case "empty":
		return ""
	case "missing-file":
		return fileToURI(validPath + ".missing")
	default:
		return fileToURI(validPath)
	}
}

func drawLSPPropertyTextEdits(t *rapid.T, entry int) []lspPropertyTextEdit {
	count := rapid.IntRange(0, maxWorkspaceTextEdits).Draw(t, fmt.Sprintf("entry %d edit count", entry))
	edits := make([]lspPropertyTextEdit, 0, count)
	for i := 0; i < count; i++ {
		start := rapid.IntRange(0, len("0123456789")).Draw(t, fmt.Sprintf("entry %d edit %d start", entry, i))
		end := rapid.IntRange(start, len("0123456789")).Draw(t, fmt.Sprintf("entry %d edit %d end", entry, i))
		newText := rapid.StringN(0, 8, 8).Draw(t, fmt.Sprintf("entry %d edit %d new text", entry, i))
		edits = append(edits, lspPropertyTextEdit{
			Range: lspPropertyRange{
				Start: lspPropertyPosition{Line: 0, Character: start},
				End:   lspPropertyPosition{Line: 0, Character: end},
			},
			NewText: newText,
		})
	}
	return edits
}

func assertNonEmptyFileEditPaths(t *rapid.T, fileEdits []edit.FileEdit) {
	t.Helper()

	for _, fe := range fileEdits {
		if fe.Path == "" {
			t.Fatalf("parseWorkspaceEdit returned an empty file path: %+v", fileEdits)
		}
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type lspPropertyPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspPropertyRange struct {
	Start lspPropertyPosition `json:"start"`
	End   lspPropertyPosition `json:"end"`
}

type lspPropertyTextEdit struct {
	Range   lspPropertyRange `json:"range"`
	NewText string           `json:"newText"`
}

type lspPropertyTextDocument struct {
	URI string `json:"uri"`
}

type lspPropertyDocumentChange struct {
	TextDocument lspPropertyTextDocument `json:"textDocument"`
	Edits        []lspPropertyTextEdit   `json:"edits"`
}
