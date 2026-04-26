package tsmorph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

var _ backend.RefactoringBackend = (*Adapter)(nil)

type Adapter struct {
	workspaceRoot string
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

func Available() bool {
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	paths := wrapperPaths()
	if _, err := os.Stat(paths.script); err != nil {
		return false
	}
	if _, err := os.Stat(paths.moduleDir); err != nil {
		return false
	}
	return true
}

func (a *Adapter) Initialize(workspaceRoot string) error {
	if !Available() {
		return fmt.Errorf("ts-morph backend not installed; run npm install in adapters/tsmorph")
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return fmt.Errorf("abs workspace root: %w", err)
	}
	a.workspaceRoot = absRoot
	return nil
}

func (a *Adapter) Shutdown() error {
	return nil
}

func (a *Adapter) FindSymbol(_ symbol.Query) ([]symbol.Location, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error) {
	if a.workspaceRoot == "" {
		return nil, fmt.Errorf("adapter not initialized")
	}

	req := renameRequest{
		WorkspaceRoot: a.workspaceRoot,
		File:          loc.File,
		Line:          loc.Line,
		Column:        loc.Column,
		NewName:       newName,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	paths := wrapperPaths()
	cmd := exec.Command("node", paths.script)
	cmd.Dir = a.workspaceRoot
	cmd.Stdin = bytes.NewReader(data)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ts-morph rename: %s", msg)
	}

	var resp renameResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parse ts-morph output: %w", err)
	}

	fileEdits := make([]edit.FileEdit, 0, len(resp.FileEdits))
	for _, fe := range resp.FileEdits {
		edits := make([]edit.TextEdit, 0, len(fe.Edits))
		for _, e := range fe.Edits {
			edits = append(edits, edit.TextEdit{
				Range: edit.Range{
					Start: edit.Position{Line: e.Range.Start.Line, Character: e.Range.Start.Character},
					End:   edit.Position{Line: e.Range.End.Line, Character: e.Range.End.Character},
				},
				NewText: e.NewText,
			})
		}
		fileEdits = append(fileEdits, edit.FileEdit{
			Path:  fe.Path,
			Edits: edits,
		})
	}
	return &edit.WorkspaceEdit{FileEdits: fileEdits}, nil
}

func (a *Adapter) ExtractFunction(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) ExtractVariable(_ symbol.SourceRange, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) InlineSymbol(_ symbol.Location) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) MoveToFile(_ symbol.Location, _ string) (*edit.WorkspaceEdit, error) {
	return nil, backend.ErrUnsupported
}

func (a *Adapter) Capabilities() []backend.Capability {
	return []backend.Capability{
		{Operation: "rename"},
	}
}

type renameRequest struct {
	WorkspaceRoot string `json:"workspaceRoot"`
	File          string `json:"file"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	NewName       string `json:"newName"`
}

type renameResponse struct {
	FileEdits []struct {
		Path  string `json:"path"`
		Edits []struct {
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
				End struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"end"`
			} `json:"range"`
			NewText string `json:"newText"`
		} `json:"edits"`
	} `json:"fileEdits"`
}

type paths struct {
	script    string
	moduleDir string
}

func wrapperPaths() paths {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	return paths{
		script:    filepath.Join(repoRoot, "adapters", "tsmorph", "rename.cjs"),
		moduleDir: filepath.Join(repoRoot, "adapters", "tsmorph", "node_modules", "ts-morph"),
	}
}
