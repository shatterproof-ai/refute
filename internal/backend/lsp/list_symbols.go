package lsp

import (
	"fmt"

	"github.com/shatterproof-ai/refute/internal/symbol"
)

// ListSymbols queries workspace/symbol for every symbol matching query and
// returns them as 1-indexed locations for symbol discovery.
//
// Unlike FindSymbol, results are not narrowed to an exact qualified-name match:
// callers receive each symbol the server reports so they can present discovery
// candidates and disambiguate by file/line/column. An empty query asks the
// server for its full symbol set, which is server-dependent (gopls returns
// everything in the primed workspace; some servers return nothing).
//
// Column conversion is best-effort: when a result's source file cannot be read
// to map UTF-16 to a byte column, the 1-indexed UTF-16 character is reported
// instead of failing the whole listing.
func (a *Adapter) ListSymbols(query string) ([]symbol.Location, error) {
	if a.client == nil {
		return nil, fmt.Errorf("adapter not initialized")
	}
	syms, err := a.client.WorkspaceSymbol(query)
	if err != nil {
		return nil, err
	}
	out := make([]symbol.Location, 0, len(syms))
	for _, s := range syms {
		file := uriToFile(s.Location.URI)
		zeroLine := s.Location.Range.Start.Line
		character := s.Location.Range.Start.Character
		column, err := utf16CharacterToByteColumnInFile(file, zeroLine, character)
		if err != nil {
			column = character + 1 // best-effort when the file is unreadable
		}
		out = append(out, symbol.Location{
			File:      file,
			Line:      zeroLine + 1,
			Column:    column,
			Name:      s.Name,
			Kind:      lspKindToSymbolKind(s.Kind),
			Container: s.ContainerName,
		})
	}
	return out, nil
}
