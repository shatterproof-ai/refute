package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version, Commit, and BuildDate are populated at build time via -ldflags:
//
//	go build -ldflags "-X github.com/shatterproof-ai/refute/internal/cli.Version=v0.1.0 \
//	                   -X github.com/shatterproof-ai/refute/internal/cli.Commit=$(git rev-parse --short HEAD) \
//	                   -X github.com/shatterproof-ai/refute/internal/cli.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
//
// Defaults below identify a local development build.
var (
	Version   = "0.1.0-dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Run: func(cmd *cobra.Command, args []string) {
		w := cmd.OutOrStdout()
		fmt.Fprintf(w, "refute version %s\n", Version)
		fmt.Fprintf(w, "commit: %s\n", Commit)
		fmt.Fprintf(w, "built:  %s\n", BuildDate)
	},
}
