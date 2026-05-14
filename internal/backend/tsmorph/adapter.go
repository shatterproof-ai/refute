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
	adapterPath   string // explicit override; empty means auto-discover
}

func NewAdapter() *Adapter {
	return &Adapter{}
}

// NewAdapterWithPath creates an adapter that uses explicitPath as the rename.cjs
// script location instead of the auto-discovery chain.
func NewAdapterWithPath(explicitPath string) *Adapter {
	return &Adapter{adapterPath: explicitPath}
}

// Available reports whether the ts-morph adapter can be located using the
// repo-relative development path or global npm. Use AvailableAt when the
// workspace root is known to also check workspace-local node_modules.
func Available() bool {
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	_, ok := resolveAdapterPaths("", "")
	return ok
}

// AvailableAt reports whether the ts-morph adapter can be located given the
// workspace root and an optional explicit path override.
func AvailableAt(workspaceRoot, explicitPath string) bool {
	if _, err := exec.LookPath("node"); err != nil {
		return false
	}
	_, ok := resolveAdapterPaths(workspaceRoot, explicitPath)
	return ok
}

func (a *Adapter) Initialize(workspaceRoot string) error {
	if !AvailableAt(workspaceRoot, a.adapterPath) {
		return fmt.Errorf("ts-morph adapter not found; install with: npm install -g @shatterproof-ai/refute-ts-adapter")
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

func (a *Adapter) FindSymbol(query symbol.Query) ([]symbol.Location, error) {
	if a.workspaceRoot == "" {
		return nil, fmt.Errorf("adapter not initialized")
	}

	req := findSymbolRequest{
		Operation:     "findSymbol",
		WorkspaceRoot: a.workspaceRoot,
		File:          query.File,
		QualifiedName: query.QualifiedName,
		Kind:          query.Kind.String(),
	}

	var resp findSymbolResponse
	if err := a.run(req, &resp); err != nil {
		return nil, err
	}

	locs := make([]symbol.Location, 0, len(resp.Candidates))
	for _, candidate := range resp.Candidates {
		locs = append(locs, symbol.Location{
			File:   candidate.File,
			Line:   candidate.Line,
			Column: candidate.Column,
			Name:   candidate.Name,
			Kind:   parseKind(candidate.Kind),
		})
	}

	switch len(locs) {
	case 0:
		return nil, backend.ErrSymbolNotFound
	case 1:
		return locs, nil
	default:
		return locs, &backend.ErrAmbiguous{Candidates: locs}
	}
}

func (a *Adapter) Rename(loc symbol.Location, newName string) (*edit.WorkspaceEdit, error) {
	if a.workspaceRoot == "" {
		return nil, fmt.Errorf("adapter not initialized")
	}

	req := renameRequest{
		Operation:     "rename",
		WorkspaceRoot: a.workspaceRoot,
		File:          loc.File,
		Line:          loc.Line,
		Column:        loc.Column,
		NewName:       newName,
	}

	var resp renameResponse
	if err := a.run(req, &resp); err != nil {
		return nil, err
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
	Operation     string `json:"operation"`
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

type findSymbolRequest struct {
	Operation     string `json:"operation"`
	WorkspaceRoot string `json:"workspaceRoot"`
	File          string `json:"file,omitempty"`
	QualifiedName string `json:"qualifiedName"`
	Kind          string `json:"kind,omitempty"`
}

type findSymbolResponse struct {
	Candidates []struct {
		File   string `json:"file"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
	} `json:"candidates"`
}

type paths struct {
	script    string
	moduleDir string
}

func (a *Adapter) run(req any, resp any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	p, ok := resolveAdapterPaths(a.workspaceRoot, a.adapterPath)
	if !ok {
		return fmt.Errorf("ts-morph adapter not found; install with: npm install -g @shatterproof-ai/refute-ts-adapter")
	}
	cmd := exec.Command("node", p.script)
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
		return fmt.Errorf("ts-morph operation failed: %s", msg)
	}

	if err := json.Unmarshal(stdout.Bytes(), resp); err != nil {
		return fmt.Errorf("parse ts-morph output: %w", err)
	}
	return nil
}

func parseKind(name string) symbol.SymbolKind {
	switch name {
	case "function":
		return symbol.KindFunction
	case "class":
		return symbol.KindClass
	case "field":
		return symbol.KindField
	case "variable":
		return symbol.KindVariable
	case "parameter":
		return symbol.KindParameter
	case "type":
		return symbol.KindType
	case "method":
		return symbol.KindMethod
	default:
		return symbol.KindUnknown
	}
}

// resolveAdapterPaths returns the script and ts-morph module directory by
// walking the discovery chain: explicit override → workspace node_modules →
// global npm root → repo-relative development fallback.
func resolveAdapterPaths(workspaceRoot, explicitPath string) (paths, bool) {
	// 1. Explicit config override.
	if explicitPath != "" {
		dir := filepath.Dir(explicitPath)
		p := paths{
			script:    explicitPath,
			moduleDir: filepath.Join(dir, "node_modules", "ts-morph"),
		}
		if pathsExist(p) {
			return p, true
		}
		// ts-morph may be hoisted to workspace node_modules.
		if workspaceRoot != "" {
			p.moduleDir = filepath.Join(workspaceRoot, "node_modules", "ts-morph")
			if pathsExist(p) {
				return p, true
			}
		}
	}

	// 2. Workspace node_modules/@shatterproof-ai/refute-ts-adapter.
	if workspaceRoot != "" {
		pkgDir := filepath.Join(workspaceRoot, "node_modules", "@shatterproof-ai", "refute-ts-adapter")
		script := filepath.Join(pkgDir, "rename.cjs")
		// Prefer bundled ts-morph; fall back to hoisted.
		for _, modDir := range []string{
			filepath.Join(pkgDir, "node_modules", "ts-morph"),
			filepath.Join(workspaceRoot, "node_modules", "ts-morph"),
		} {
			p := paths{script: script, moduleDir: modDir}
			if pathsExist(p) {
				return p, true
			}
		}
	}

	// 3. Global npm root.
	if root := globalNpmRoot(); root != "" {
		pkgDir := filepath.Join(root, "@shatterproof-ai", "refute-ts-adapter")
		p := paths{
			script:    filepath.Join(pkgDir, "rename.cjs"),
			moduleDir: filepath.Join(pkgDir, "node_modules", "ts-morph"),
		}
		if pathsExist(p) {
			return p, true
		}
	}

	// 4. Repo-relative (development fallback using compile-time source path).
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	p := paths{
		script:    filepath.Join(repoRoot, "adapters", "tsmorph", "rename.cjs"),
		moduleDir: filepath.Join(repoRoot, "adapters", "tsmorph", "node_modules", "ts-morph"),
	}
	if pathsExist(p) {
		return p, true
	}

	return paths{}, false
}

func pathsExist(p paths) bool {
	if _, err := os.Stat(p.script); err != nil {
		return false
	}
	if _, err := os.Stat(p.moduleDir); err != nil {
		return false
	}
	return true
}

func globalNpmRoot() string {
	out, err := exec.Command("npm", "root", "-g").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
