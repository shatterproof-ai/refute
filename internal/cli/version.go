package cli

import (
	"encoding/json"
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

// versionInfo is the --json shape for the version command.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Long: `Print the refute version, the git commit it was built from, and the build
date. Pass --json to emit the same fields as a structured object for scripts.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := cmd.OutOrStdout()
		if flagJSON {
			data, err := json.MarshalIndent(versionInfo{
				Version:   Version,
				Commit:    Commit,
				BuildDate: BuildDate,
			}, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling version: %w", err)
			}
			fmt.Fprintln(w, string(data))
			return nil
		}
		fmt.Fprintf(w, "refute version %s\n", Version)
		fmt.Fprintf(w, "commit: %s\n", Commit)
		fmt.Fprintf(w, "built: %s\n", BuildDate)
		return nil
	},
}

func init() {
	versionCmd.Flags().BoolVar(&flagJSON, "json", false, "emit structured JSON instead of human-readable output")
}
