package cli

import (
	"github.com/spf13/cobra"
)

var (
	flagConfig  string
	flagDryRun  bool
	flagVerbose bool
)

// RootCmd is the top-level CLI command.
var RootCmd = &cobra.Command{
	Use:   "refute",
	Short: "Automated source code refactoring",
	Long:  "refute orchestrates existing refactoring engines to provide IDE-quality refactoring from the command line.",
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "path to config file")
	RootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show diff without applying changes")
	RootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "show detailed output")
	RootCmd.AddCommand(versionCmd)
}
