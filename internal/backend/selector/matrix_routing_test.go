package selector

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend"
	"github.com/shatterproof-ai/refute/internal/config"
)

// matrixLanguageFiles maps each support-matrix language key to a representative
// file whose extension language.Detect resolves back to that key. The map is
// exhaustive over SupportMatrix; a new language row with no mapping fails the
// test loudly, so the guardrail cannot silently skip a language.
var matrixLanguageFiles = map[string]string{
	"go":         "/tmp/main.go",
	"typescript": "/tmp/app.ts",
	"javascript": "/tmp/app.js",
	"rust":       "/tmp/main.rs",
	"python":     "/tmp/main.py",
	"java":       "/tmp/Main.java",
	"kotlin":     "/tmp/Main.kt",
}

// TestSupportMatrixLevelMatchesBackendRouting is the issue #119 drift guardrail
// for the support matrix <-> backend-selection boundary. It enforces the
// invariant both directions, keyed off SupportMatrix so the data and the
// routing cannot drift apart:
//
//   - Every LevelUnsupported row is gated with ErrLanguageUnsupported before any
//     backend constructor runs (an unsupported language must never reach active
//     backend setup, e.g. Java/Kotlin must not reach OpenRewrite).
//   - Every non-unsupported row is routable: the selector returns a backend
//     selection for it and does NOT gate it as unsupported (a claimed language
//     that the selector cannot reach is the converse drift).
//
// The complementary TestForFile_UnsupportedLanguageGatedBeforeBackendSetup
// focuses on constructor-spying for the unsupported rows; this test adds the
// matrix-wide bidirectional equivalence.
func TestSupportMatrixLevelMatchesBackendRouting(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load("", dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	// Make ts-morph deterministically unavailable so TypeScript/JavaScript route
	// through their configured LSP rung rather than depending on dev-local
	// adapter assets (see #128). Stub every constructor so no real backend
	// process is created and so the unsupported branch can assert none ran.
	var constructed bool
	oldAvail := tsMorphAvailable
	oldOR := newOpenRewriteBackend
	oldTS := newTSMorphBackend
	oldLSP := newLSPBackend
	tsMorphAvailable = func(_, _ string) bool { return false }
	newOpenRewriteBackend = func() backend.RefactoringBackend { constructed = true; return fakeBackend{} }
	newTSMorphBackend = func(context.Context, string) backend.RefactoringBackend { constructed = true; return fakeBackend{} }
	newLSPBackend = func(context.Context, config.ServerConfig, string, time.Duration) backend.RefactoringBackend {
		constructed = true
		return fakeBackend{}
	}
	t.Cleanup(func() {
		tsMorphAvailable = oldAvail
		newOpenRewriteBackend = oldOR
		newTSMorphBackend = oldTS
		newLSPBackend = oldLSP
	})

	if len(config.SupportMatrix) == 0 {
		t.Fatal("SupportMatrix is empty; nothing to check")
	}

	for _, entry := range config.SupportMatrix {
		path, ok := matrixLanguageFiles[entry.Language]
		if !ok {
			t.Fatalf("no representative file mapped for support-matrix language %q; add it to matrixLanguageFiles", entry.Language)
		}

		constructed = false
		sel, err := ForFile(context.Background(), cfg, dir, filepath.Join(dir, filepath.Base(path)))

		var unsupported *ErrLanguageUnsupported
		gated := errors.As(err, &unsupported)

		if entry.Level == config.LevelUnsupported {
			if !gated {
				t.Errorf("%s: Level=unsupported but selection was not gated (err=%v, sel=%#v); "+
					"an unsupported matrix row must be routed to ErrLanguageUnsupported, not a backend", entry.Language, err, sel)
				continue
			}
			if sel != nil {
				t.Errorf("%s: gated unsupported language returned a non-nil selection %#v", entry.Language, sel)
			}
			if constructed {
				t.Errorf("%s: a backend constructor ran for an unsupported language before gating", entry.Language)
			}
			if unsupported.Language != entry.Language {
				t.Errorf("%s: gate reported language %q", entry.Language, unsupported.Language)
			}
			continue
		}

		// Non-unsupported rows must be reachable, not gated as unsupported.
		if gated {
			t.Errorf("%s: Level=%q but selection was gated as unsupported; a claimed language must be routable", entry.Language, entry.Level)
			continue
		}
		if err != nil {
			t.Errorf("%s: Level=%q but ForFile returned %v; expected a routable backend selection", entry.Language, entry.Level, err)
			continue
		}
		if sel == nil || sel.Language != entry.Language {
			t.Errorf("%s: expected a selection for language %q, got %#v", entry.Language, entry.Language, sel)
		}
	}
}
