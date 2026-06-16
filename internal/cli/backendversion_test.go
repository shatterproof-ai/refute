package cli

import (
	"testing"

	"github.com/shatterproof-ai/refute/internal/backend/selector"
	"github.com/shatterproof-ai/refute/internal/backend/tsmorph"
	"github.com/shatterproof-ai/refute/internal/config"
)

func TestFirstNonEmptyLine(t *testing.T) {
	cases := map[string]string{
		"golang.org/x/tools/gopls v0.22.0\n  extra\n": "golang.org/x/tools/gopls v0.22.0",
		"\n\n  rust-analyzer 1.0.0  \n":               "rust-analyzer 1.0.0",
		"":                                            "",
		"   \n\t\n":                                   "",
	}
	for in, want := range cases {
		if got := firstNonEmptyLine(in); got != want {
			t.Errorf("firstNonEmptyLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbeBinaryVersion_EmptyInputs(t *testing.T) {
	if got := probeBinaryVersion("", []string{"version"}); got != "" {
		t.Errorf("empty command: got %q, want empty", got)
	}
	if got := probeBinaryVersion("gopls", nil); got != "" {
		t.Errorf("nil args: got %q, want empty", got)
	}
}

func TestBackendVersionForSelection(t *testing.T) {
	origProbe := versionProbeFn
	t.Cleanup(func() { versionProbeFn = origProbe })

	var gotCommand string
	var gotArgs []string
	versionProbeFn = func(command string, args []string) string {
		gotCommand = command
		gotArgs = args
		return "gopls v9.9.9"
	}

	t.Run("nil selection", func(t *testing.T) {
		if got := backendVersionForSelection(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("lsp probes server command with matrix args", func(t *testing.T) {
		sel := &selector.Selection{
			Language:    "go",
			BackendName: "lsp",
			Server:      config.ServerConfig{Command: "gopls", Args: []string{"serve"}},
		}
		if got := backendVersionForSelection(sel); got != "gopls v9.9.9" {
			t.Errorf("got %q, want %q", got, "gopls v9.9.9")
		}
		if gotCommand != "gopls" {
			t.Errorf("probed command = %q, want gopls", gotCommand)
		}
		// gopls uses the "version" subcommand, not "--version".
		if len(gotArgs) != 1 || gotArgs[0] != "version" {
			t.Errorf("probed args = %v, want [version]", gotArgs)
		}
	})

	t.Run("tsmorph reports stamped package version", func(t *testing.T) {
		sel := &selector.Selection{Language: "typescript", BackendName: "tsmorph"}
		if got := backendVersionForSelection(sel); got != tsmorph.AdapterPackageVersion {
			t.Errorf("got %q, want %q", got, tsmorph.AdapterPackageVersion)
		}
	})

	t.Run("unknown backend yields empty", func(t *testing.T) {
		sel := &selector.Selection{Language: "java", BackendName: "openrewrite"}
		if got := backendVersionForSelection(sel); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
