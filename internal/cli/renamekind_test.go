package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/edit"
	"github.com/shatterproof-ai/refute/internal/symbol"
)

func TestValidateResolvedKind(t *testing.T) {
	cases := []struct {
		name        string
		language    string
		requested   symbol.SymbolKind
		actual      symbol.SymbolKind
		wantErr     bool
		wantActual  symbol.SymbolKind
		wantLangInv bool
	}{
		{name: "plain rename never validates", language: "go", requested: symbol.KindUnknown, actual: symbol.KindFunction, wantErr: false},
		{name: "correct kind passes", language: "go", requested: symbol.KindFunction, actual: symbol.KindFunction, wantErr: false},
		{name: "mismatch in-language", language: "go", requested: symbol.KindMethod, actual: symbol.KindFunction, wantErr: true, wantActual: symbol.KindFunction, wantLangInv: false},
		{name: "class on go function names actual and is lang-invalid", language: "go", requested: symbol.KindClass, actual: symbol.KindFunction, wantErr: true, wantActual: symbol.KindFunction, wantLangInv: true},
		{name: "class on go with unknown actual still rejects", language: "go", requested: symbol.KindClass, actual: symbol.KindUnknown, wantErr: true, wantActual: symbol.KindUnknown, wantLangInv: true},
		{name: "unknown actual for valid kind is permissive", language: "go", requested: symbol.KindFunction, actual: symbol.KindUnknown, wantErr: false},
		{name: "class is valid in typescript", language: "typescript", requested: symbol.KindClass, actual: symbol.KindClass, wantErr: false},
		{name: "unknown language is permissive", language: "elvish", requested: symbol.KindClass, actual: symbol.KindUnknown, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResolvedKind(tc.language, tc.requested, tc.actual, "Sym")
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateResolvedKind err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr {
				return
			}
			var km *ErrKindMismatch
			if !errors.As(err, &km) {
				t.Fatalf("err = %#v, want *ErrKindMismatch", err)
			}
			if km.Actual != tc.wantActual {
				t.Errorf("Actual = %v, want %v", km.Actual, tc.wantActual)
			}
			if km.LangInvalid != tc.wantLangInv {
				t.Errorf("LangInvalid = %v, want %v", km.LangInvalid, tc.wantLangInv)
			}
			if km.ExitCode() != 1 {
				t.Errorf("ExitCode = %d, want 1", km.ExitCode())
			}
		})
	}
}

func TestErrKindMismatchMessages(t *testing.T) {
	cases := []struct {
		name      string
		err       *ErrKindMismatch
		wantParts []string
	}{
		{
			name:      "lang-invalid with known actual names actual and variant",
			err:       &ErrKindMismatch{SymbolName: "FormatGreeting", Language: "go", Requested: symbol.KindClass, Actual: symbol.KindFunction, LangInvalid: true},
			wantParts: []string{"go has no class symbols", `"FormatGreeting" is a function`, "`rename`", "`rename-function`"},
		},
		{
			name:      "lang-invalid with unknown actual",
			err:       &ErrKindMismatch{SymbolName: "Foo", Language: "go", Requested: symbol.KindClass, Actual: symbol.KindUnknown, LangInvalid: true},
			wantParts: []string{"go has no class symbols", "`rename`"},
		},
		{
			name:      "in-language mismatch",
			err:       &ErrKindMismatch{SymbolName: "Foo", Language: "go", Requested: symbol.KindMethod, Actual: symbol.KindFunction},
			wantParts: []string{`"Foo" is a function, not a method`, "`rename-function`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.err.Error()
			for _, part := range tc.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("message %q missing %q", msg, part)
				}
			}
		})
	}
}

func TestSelectTier1Candidate(t *testing.T) {
	fn := symbol.Location{Name: "Foo", Kind: symbol.KindFunction}
	method := symbol.Location{Name: "Foo", Kind: symbol.KindMethod}

	t.Run("not found", func(t *testing.T) {
		_, err := selectTier1Candidate(nil, symbol.KindFunction)
		if !errors.Is(err, backend.ErrSymbolNotFound) {
			t.Fatalf("err = %v, want ErrSymbolNotFound", err)
		}
	})
	t.Run("unique kind match disambiguates", func(t *testing.T) {
		loc, err := selectTier1Candidate([]symbol.Location{fn, method}, symbol.KindMethod)
		if err != nil || loc.Kind != symbol.KindMethod {
			t.Fatalf("loc = %+v, err = %v; want the method", loc, err)
		}
	})
	t.Run("single wrong-kind symbol is returned for validation", func(t *testing.T) {
		loc, err := selectTier1Candidate([]symbol.Location{fn}, symbol.KindClass)
		if err != nil || loc.Kind != symbol.KindFunction {
			t.Fatalf("loc = %+v, err = %v; want the function returned for mismatch reporting", loc, err)
		}
	})
	t.Run("multiple kind matches are ambiguous", func(t *testing.T) {
		_, err := selectTier1Candidate([]symbol.Location{method, method}, symbol.KindMethod)
		var amb *backend.ErrAmbiguous
		if !errors.As(err, &amb) {
			t.Fatalf("err = %v, want ErrAmbiguous", err)
		}
	})
	t.Run("plain rename single match", func(t *testing.T) {
		loc, err := selectTier1Candidate([]symbol.Location{fn}, symbol.KindUnknown)
		if err != nil || loc.Kind != symbol.KindFunction {
			t.Fatalf("loc = %+v, err = %v", loc, err)
		}
	})
	t.Run("plain rename ambiguous", func(t *testing.T) {
		_, err := selectTier1Candidate([]symbol.Location{fn, method}, symbol.KindUnknown)
		var amb *backend.ErrAmbiguous
		if !errors.As(err, &amb) {
			t.Fatalf("err = %v, want ErrAmbiguous", err)
		}
	})
}

// kindStubBackend is a stubBackend that also satisfies backend.KindResolver so
// the probe path can be exercised without a live language server.
type kindStubBackend struct {
	stubBackend
	kind    symbol.SymbolKind
	kindErr error
}

func (k *kindStubBackend) ResolveKind(symbol.Location) (symbol.SymbolKind, error) {
	return k.kind, k.kindErr
}

func TestProbeSymbolKind(t *testing.T) {
	t.Run("backend without KindResolver yields unknown", func(t *testing.T) {
		if got := probeSymbolKind(&stubBackend{}, symbol.Location{}); got != symbol.KindUnknown {
			t.Fatalf("got %v, want KindUnknown", got)
		}
	})
	t.Run("resolver error yields unknown", func(t *testing.T) {
		b := &kindStubBackend{kind: symbol.KindFunction, kindErr: errors.New("boom")}
		if got := probeSymbolKind(b, symbol.Location{}); got != symbol.KindUnknown {
			t.Fatalf("got %v, want KindUnknown on error", got)
		}
	})
	t.Run("resolver kind is returned", func(t *testing.T) {
		b := &kindStubBackend{kind: symbol.KindMethod}
		if got := probeSymbolKind(b, symbol.Location{}); got != symbol.KindMethod {
			t.Fatalf("got %v, want KindMethod", got)
		}
	})
}

func TestEmitJSONOperationErrorKindMismatch(t *testing.T) {
	reset := func() { flagJSON = false }
	reset()
	t.Cleanup(reset)
	flagJSON = true

	ctx := jsonContext{Operation: "rename", Language: "go", Backend: "lsp", WorkspaceRoot: "/workspace"}
	kindErr := &ErrKindMismatch{SymbolName: "FormatGreeting", Language: "go", Requested: symbol.KindClass, Actual: symbol.KindFunction, LangInvalid: true}

	out := captureStdout(t, func() {
		runErr := emitJSONOperationError(ctx, kindErr)
		var ec *ExitCodeError
		if !errors.As(runErr, &ec) || ec.Code != 1 {
			t.Fatalf("runErr = %#v, want exit code 1", runErr)
		}
	})

	var got edit.JSONResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal JSON: %v\nraw:\n%s", err, out)
	}
	if got.Status != edit.StatusKindMismatch {
		t.Fatalf("status = %q, want %q", got.Status, edit.StatusKindMismatch)
	}
	if got.Error == nil || got.Error.Code != "kind-mismatch" {
		t.Fatalf("error = %+v, want code kind-mismatch", got.Error)
	}
	if !strings.Contains(got.Error.Message, "function") {
		t.Errorf("message %q should name the actual kind", got.Error.Message)
	}
}
