package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// resolveGoParamIndex parses a Go source file and returns the 0-based position,
// within its function's parameter list, of the parameter identified by the
// 1-indexed line/col. Parameters are counted individually, so `a, b int`
// contributes two positions (indices n and n+1); a receiver and type parameters
// are not parameters and are not counted.
//
// change-signature is Go-only in this release, so the reference CLI resolves the
// index here to populate ParamEdit.FromIndex with the real original position
// (per the SignatureParams contract) instead of a placeholder. The backend still
// re-derives the target parameter from Target.Location, so this never changes
// which parameter is removed.
//
// The resolver is deliberately no stricter than the backend it feeds: gopls's
// removeUnusedParam offers the action only when the position is on a parameter
// identifier, so a position this resolver cannot map to a parameter name is one
// gopls would also decline. Parsing is best-effort — a syntax error elsewhere in
// the file does not abort resolution when the target function still parses.
func resolveGoParamIndex(file string, line, col int) (int, error) {
	fset := token.NewFileSet()
	// A partial AST is still useful: ParseFile recovers from syntax errors and
	// returns the declarations it could parse, so an unrelated broken function
	// does not block resolving a well-formed one. Only bail if nothing parsed.
	f, parseErr := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
	if f == nil {
		return 0, fmt.Errorf("parsing %s to resolve the parameter position: %w", file, parseErr)
	}

	// within reports whether the 1-indexed (line, col) falls inside node's
	// [Pos, End) source span. token positions are 1-indexed byte columns, and
	// End is exclusive, matching the CLI's --line/--col convention.
	within := func(n ast.Node) bool {
		start := fset.Position(n.Pos())
		end := fset.Position(n.End())
		if line < start.Line || line > end.Line {
			return false
		}
		if line == start.Line && col < start.Column {
			return false
		}
		if line == end.Line && col >= end.Column {
			return false
		}
		return true
	}

	// matchParams returns the 0-based index of the parameter at the target
	// position within params, or -1 if the target is not on one of its
	// parameters. A single-name field matches on its whole span (name or type);
	// a multi-name field (`a, b int`) matches only on an individual name, since
	// the shared type region cannot be attributed to one parameter (and gopls
	// offers no action there either).
	matchParams := func(params *ast.FieldList) int {
		if params == nil {
			return -1
		}
		idx := 0
		for _, field := range params.List {
			if len(field.Names) == 0 {
				if within(field) {
					return idx
				}
				idx++
				continue
			}
			single := len(field.Names) == 1
			for _, name := range field.Names {
				if within(name) || (single && within(field)) {
					return idx
				}
				idx++
			}
		}
		return -1
	}

	found := -1
	ast.Inspect(f, func(n ast.Node) bool {
		if found >= 0 {
			return false
		}
		// *ast.FuncType covers the parameter list of every function form — top
		// level (*ast.FuncDecl), literals (*ast.FuncLit), and interface-method /
		// func-typed signatures — so a single case handles them all. A receiver
		// and type parameters live outside FuncType.Params and are never counted.
		if ft, ok := n.(*ast.FuncType); ok {
			if idx := matchParams(ft.Params); idx >= 0 {
				found = idx
				return false
			}
		}
		return true
	})

	if found < 0 {
		if parseErr != nil {
			return 0, fmt.Errorf("no function parameter found at %s:%d:%d (the file has syntax errors: %w); point --line/--col at the parameter's name", file, line, col, parseErr)
		}
		return 0, fmt.Errorf("no function parameter found at %s:%d:%d; point --line/--col at the parameter's name", file, line, col)
	}
	return found, nil
}
