package cli

import (
	"fmt"
	"os/exec"

	"github.com/shatterproof-ai/refute/internal/backend/lsp"
	"github.com/shatterproof-ai/refute/internal/config"
	"github.com/shatterproof-ai/refute/internal/language"
)

// lspBackendSetupError tags an LSP backend setup failure with the phase in which
// it occurred so each caller can apply its own error wrapping. The wrapped error
// already carries the final selection-phase form (ErrLSPServerMissing, the
// "loading config" wrap, or "no server configured"); the initialize and prime
// phases carry the raw backend error so callers can wrap it as they did before
// this helper was extracted.
type lspBackendSetupError struct {
	phase string // "selection", "initialize", or "prime"
	err   error
}

func (e *lspBackendSetupError) Error() string { return e.err.Error() }

func (e *lspBackendSetupError) Unwrap() error { return e.err }

// setupLSPBackend performs the LSP backend bring-up shared by whole-workspace
// operations (Tier-1 rename and list-symbols): it loads config, verifies the
// language server binary, constructs and initializes the adapter, and primes the
// workspace, emitting the "backend-selection", "backend-initialization", and
// "workspace-priming" telemetry phases in order.
//
// On success it returns the primed adapter and the populated jsonContext. On
// failure it returns the context built so far and an *lspBackendSetupError whose
// phase lets the caller reproduce its historical error wrapping; the adapter is
// shut down before a priming failure is returned.
//
// fileScope, when non-empty, refines the LSP languageID from the file's
// extension (e.g. for a specific source file); an empty fileScope uses lang as
// the languageID.
func setupLSPBackend(operation, lang, fileScope, workspaceRoot string) (*lsp.Adapter, jsonContext, error) {
	ctx := jsonContext{Operation: operation, Language: lang, Backend: "lsp", WorkspaceRoot: workspaceRoot}
	telemetrySetContext(ctx)

	selectDone := telemetryPhase("backend-selection")
	cfg, err := config.Load(flagConfig, workspaceRoot)
	if err != nil {
		selectDone()
		return nil, ctx, &lspBackendSetupError{phase: "selection", err: fmt.Errorf("loading config: %w", err)}
	}
	serverCfg := cfg.Server(lang)
	if serverCfg.Command == "" {
		selectDone()
		return nil, ctx, &lspBackendSetupError{phase: "selection", err: fmt.Errorf("no server configured for language %q", lang)}
	}
	if _, lookErr := exec.LookPath(serverCfg.Command); lookErr != nil {
		selectDone()
		return nil, ctx, &lspBackendSetupError{phase: "selection", err: &ErrLSPServerMissing{
			Language:    lang,
			Command:     serverCfg.Command,
			InstallHint: config.InstallHint(lang),
		}}
	}
	selectDone()

	languageID := lang
	if fileScope != "" {
		if id := language.Detect(fileScope).LanguageID; id != "" {
			languageID = id
		}
	}

	adapter := lsp.NewAdapter(serverCfg, languageID, nil)
	adapter.SetContext(commandContext())
	adapter.SetRequestTimeout(cfg.RequestTimeout())
	ctx.BackendVersion = backendVersionForLanguageServer(lang, serverCfg.Command)

	initDone := telemetryPhase("backend-initialization")
	if err := adapter.Initialize(workspaceRoot); err != nil {
		initDone()
		return nil, ctx, &lspBackendSetupError{phase: "initialize", err: err}
	}
	initDone()

	// Prime so workspace/symbol sees the whole module rather than only files the
	// server has already opened.
	primeDone := telemetryPhase("workspace-priming")
	if _, err := adapter.PrimeWorkspace(workspaceRoot); err != nil {
		primeDone()
		_ = adapter.Shutdown()
		return nil, ctx, &lspBackendSetupError{phase: "prime", err: err}
	}
	primeDone()

	return adapter, ctx, nil
}
