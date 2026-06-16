package cli

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
)

// versionProbeTimeout bounds a single version subprocess. Version capture is
// best-effort metadata, so a backend whose version command wedges degrades to
// "" rather than hanging the operation it annotates.
const versionProbeTimeout = 5 * time.Second

// versionProbeFn runs a backend binary's version command and returns the first
// non-empty output line. It is a package var so tests can inject a deterministic
// probe instead of shelling out to a real language server.
var versionProbeFn = probeBinaryVersion

// probeBinaryVersion runs `command args...` and returns the first non-empty,
// trimmed line of stdout. Any failure (binary missing, non-zero exit, empty
// output, or exceeding versionProbeTimeout) yields the empty string: version
// capture is best-effort and never blocks the operation it annotates.
func probeBinaryVersion(command string, args []string) string {
	if command == "" || len(args) == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, command, args...).Output()
	if err != nil {
		return ""
	}
	return firstNonEmptyLine(string(out))
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// backendVersionForSelection reports the version string for a resolved backend
// selection, or "" when it cannot be determined. LSP backends are probed by
// running their configured server command with the matrix VersionArgs; the
// ts-morph adapter reports its stamped package version.
func backendVersionForSelection(sel *selector.Selection) string {
	if sel == nil {
		return ""
	}
	if sel.BackendName == "tsmorph" {
		return tsmorph.AdapterPackageVersion
	}
	if sel.BackendName == "lsp" {
		return backendVersionForLanguageServer(sel.Language, sel.Server.Command)
	}
	return ""
}

// backendVersionForLanguageServer probes an LSP server command for its version
// using the VersionArgs registered for the language in the support matrix.
func backendVersionForLanguageServer(language, command string) string {
	if command == "" {
		return ""
	}
	entry, ok := config.SupportFor(language)
	if !ok || len(entry.VersionArgs) == 0 {
		return ""
	}
	return versionProbeFn(command, entry.VersionArgs)
}
