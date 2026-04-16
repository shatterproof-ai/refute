package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "0.1.0-dev"

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

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("refute %s\n", version)
	},
}

func init() {
	RootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "path to config file")
	RootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show diff without applying changes")
	RootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "show detailed output")
	RootCmd.AddCommand(versionCmd)
}
