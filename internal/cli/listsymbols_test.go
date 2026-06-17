package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestParseSymbolKindFilter(t *testing.T) {
	cases := map[string]struct {
		want symbol.SymbolKind
		ok   bool
	}{
		"":          {symbol.KindUnknown, true},
		"function":  {symbol.KindFunction, true},
		"Function":  {symbol.KindFunction, true},
		"class":     {symbol.KindClass, true},
		"method":    {symbol.KindMethod, true},
		"field":     {symbol.KindField, true},
		"variable":  {symbol.KindVariable, true},
		"type":      {symbol.KindType, true},
		"parameter": {symbol.KindParameter, true},
		"bogus":     {symbol.KindUnknown, false},
	}
	for in, want := range cases {
		got, ok := parseSymbolKindFilter(in)
		if ok != want.ok || got != want.want {
			t.Errorf("parseSymbolKindFilter(%q) = (%v, %v), want (%v, %v)", in, got, ok, want.want, want.ok)
		}
	}
}

func sampleSymbols() []symbol.Location {
	return []symbol.Location{
		{File: "/ws/a.go", Line: 10, Column: 6, Name: "Alpha", Kind: symbol.KindFunction, Container: "pkg"},
		{File: "/ws/a.go", Line: 20, Column: 6, Name: "Beta", Kind: symbol.KindType, Container: "pkg"},
		{File: "/ws/b.go", Line: 5, Column: 2, Name: "Alpha", Kind: symbol.KindMethod, Container: "Widget"},
	}
}

func TestFilterListSymbols_Empty(t *testing.T) {
	got := filterListSymbols(nil, "", symbol.KindUnknown)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}

func TestFilterListSymbols_SingleByFile(t *testing.T) {
	got := filterListSymbols(sampleSymbols(), "/ws/b.go", symbol.KindUnknown)
	if len(got) != 1 {
		t.Fatalf("expected 1 symbol scoped to b.go, got %d", len(got))
	}
	if got[0].Name != "Alpha" || got[0].File != "/ws/b.go" {
		t.Errorf("unexpected symbol %+v", got[0])
	}
}

func TestFilterListSymbols_SingleByKind(t *testing.T) {
	got := filterListSymbols(sampleSymbols(), "", symbol.KindType)
	if len(got) != 1 {
		t.Fatalf("expected 1 type symbol, got %d", len(got))
	}
	if got[0].Name != "Beta" {
		t.Errorf("unexpected symbol %+v", got[0])
	}
}

func TestFilterListSymbols_Ambiguous(t *testing.T) {
	// Same leaf name across two files is the ambiguous discovery case agents
	// must be able to disambiguate by file/line/column.
	got := filterListSymbols(sampleSymbols(), "", symbol.KindUnknown)
	if len(got) != 3 {
		t.Fatalf("expected all 3 symbols, got %d", len(got))
	}
	var alphas int
	for _, s := range got {
		if s.Name == "Alpha" {
			alphas++
		}
	}
	if alphas != 2 {
		t.Errorf("expected 2 ambiguous Alpha candidates, got %d", alphas)
	}
}

func unmarshalListResult(t *testing.T, data []byte) listSymbolsResult {
	t.Helper()
	var res listSymbolsResult
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", err, data)
	}
	return res
}

func TestRenderListSymbolsJSON_Empty(t *testing.T) {
	ctx := jsonContext{Operation: "list-symbols", Language: "go", Backend: "lsp", WorkspaceRoot: "/ws"}
	data, err := renderListSymbolsJSON(ctx, nil, nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	res := unmarshalListResult(t, data)
	if res.SchemaVersion != edit.SchemaVersion {
		t.Errorf("schemaVersion = %q, want %q", res.SchemaVersion, edit.SchemaVersion)
	}
	if res.Status != listStatusOK {
		t.Errorf("status = %q, want %q", res.Status, listStatusOK)
	}
	if res.Symbols == nil {
		t.Error("symbols must be a present (possibly empty) array, not null")
	}
	if len(res.Symbols) != 0 {
		t.Errorf("expected 0 symbols, got %d", len(res.Symbols))
	}
}

func TestRenderListSymbolsJSON_Single(t *testing.T) {
	ctx := jsonContext{Operation: "list-symbols", Language: "go", Backend: "lsp", WorkspaceRoot: "/ws"}
	data, err := renderListSymbolsJSON(ctx, sampleSymbols()[1:2], nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	res := unmarshalListResult(t, data)
	if len(res.Symbols) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(res.Symbols))
	}
	s := res.Symbols[0]
	if s.File != "/ws/a.go" || s.Line != 20 || s.Column != 6 {
		t.Errorf("Tier-3 coordinates wrong: %+v", s)
	}
	if s.Kind != "type" {
		t.Errorf("kind = %q, want type", s.Kind)
	}
	if s.QualifiedName != "pkg.Beta" {
		t.Errorf("qualifiedName = %q, want pkg.Beta", s.QualifiedName)
	}
}

func TestRenderListSymbolsJSON_Ambiguous(t *testing.T) {
	ctx := jsonContext{Operation: "list-symbols", Language: "go", Backend: "lsp", WorkspaceRoot: "/ws"}
	data, err := renderListSymbolsJSON(ctx, sampleSymbols(), nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	res := unmarshalListResult(t, data)
	if len(res.Symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d", len(res.Symbols))
	}
}

func resetListFlagsForTest(t *testing.T) {
	t.Helper()
	flagJSON = false
	flagFile = ""
	flagListQuery = ""
	flagListKind = ""
	flagListLang = ""
	flagConfig = ""
	t.Cleanup(func() {
		flagJSON = false
		flagFile = ""
		flagListQuery = ""
		flagListKind = ""
		flagListLang = ""
		flagConfig = ""
	})
}

func TestListSymbolsCommand_UnsupportedLanguageJSON(t *testing.T) {
	resetListFlagsForTest(t)
	flagListLang = "java"
	flagJSON = true

	var runErr error
	out := captureStdout(t, func() {
		runErr = runListSymbols()
	})
	var ec *ExitCodeError
	if !errors.As(runErr, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", runErr)
	}

	var res edit.JSONResult
	if jErr := json.Unmarshal([]byte(out), &res); jErr != nil {
		t.Fatalf("unmarshal: %v\nraw:\n%s", jErr, out)
	}
	if res.Status != edit.StatusUnsupported {
		t.Errorf("status = %q, want %q", res.Status, edit.StatusUnsupported)
	}
	if res.Error == nil || res.Error.Code != "unsupported-language" {
		t.Errorf("expected unsupported-language error, got %+v", res.Error)
	}
	if res.Language != "java" {
		t.Errorf("language = %q, want java", res.Language)
	}
	if res.Error != nil && res.Error.Hint == "" {
		t.Error("expected non-empty hint listing supported languages")
	}
}

func TestListSymbolsCommand_UnsupportedLanguageHuman(t *testing.T) {
	resetListFlagsForTest(t)
	flagListLang = "kotlin"

	err := runListSymbols()
	var ec *ExitCodeError
	if !errors.As(err, &ec) || ec.Code == 0 {
		t.Fatalf("expected non-zero ExitCodeError, got %#v", err)
	}
	if !strings.Contains(ec.Message, "kotlin") {
		t.Errorf("expected message to mention kotlin, got %q", ec.Message)
	}
}

func TestResolveListLanguage(t *testing.T) {
	t.Run("explicit lang wins", func(t *testing.T) {
		resetListFlagsForTest(t)
		flagListLang = "rust"
		got, err := resolveListLanguage("/ws/main.go")
		if err != nil || got != "rust" {
			t.Fatalf("got (%q, %v), want (rust, nil)", got, err)
		}
	})
	t.Run("detect from file", func(t *testing.T) {
		resetListFlagsForTest(t)
		got, err := resolveListLanguage("/ws/main.go")
		if err != nil || got != "go" {
			t.Fatalf("got (%q, %v), want (go, nil)", got, err)
		}
	})
	t.Run("default go without file or lang", func(t *testing.T) {
		resetListFlagsForTest(t)
		got, err := resolveListLanguage("")
		if err != nil || got != "go" {
			t.Fatalf("got (%q, %v), want (go, nil)", got, err)
		}
	})
	t.Run("undetected extension errors instead of defaulting to go", func(t *testing.T) {
		resetListFlagsForTest(t)
		flagFile = "/ws/notes.txt"
		if _, err := resolveListLanguage("/ws/notes.txt"); err == nil {
			t.Fatal("expected error for undetected file extension, got nil")
		}
	})
}

func TestListSymbolsCommand_UndetectedFileExtension(t *testing.T) {
	resetListFlagsForTest(t)
	flagFile = "/ws/notes.txt"

	if err := runListSymbols(); err == nil {
		t.Fatal("expected error for --file with undetected extension")
	}
}

func TestListSymbolsCommand_InvalidKind(t *testing.T) {
	resetListFlagsForTest(t)
	flagListLang = "go"
	flagListKind = "bogus"

	if err := runListSymbols(); err == nil {
		t.Fatal("expected error for invalid --kind")
	}
}
