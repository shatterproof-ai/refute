package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/shatterproof-ai/refute/internal/edit"

	"github.com/spf13/cobra"
)

var (
	flagConfig  string
	flagDryRun  bool
	flagVerbose bool
)

// RootCmd is the top-level CLI command.
//
// SilenceUsage and SilenceErrors make cli.Run the single error printer:
// cobra neither echoes the error nor dumps the flags/usage block for runtime
// failures, so each error reaches stderr exactly once. Execute restores usage
// output for command-line syntax errors only.
var RootCmd = &cobra.Command{
	Use:   "refute",
	Short: "Automated source code refactoring",
	Long: `refute orchestrates existing refactoring engines to provide IDE-quality
refactoring from the command line.

EXIT CODES
  0  command succeeded
  1  general failure, invalid request, unsupported operation, ambiguous match, or backend failure
  2  no edits or no matching symbol
  3  required backend missing`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	RootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &usageError{err: err}
	})
	RootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "path to config file")
	RootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "show diff without applying changes")
	RootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "show detailed output")
	RootCmd.AddCommand(versionCmd)
}

type usageError struct {
	err error
}

func (e *usageError) Error() string {
	return e.err.Error()
}

func (e *usageError) Unwrap() error {
	return e.err
}

// commandCtx is the SIGINT/SIGTERM-cancellable context established by Execute
// and propagated to long-running backend operations (e.g. LSP requests) so that
// Ctrl-C aborts an in-flight refactoring promptly.
var commandCtx context.Context

// commandContext returns the cancellable context for the current invocation,
// falling back to context.Background() before Execute has run (e.g. in tests).
func commandContext() context.Context {
	if commandCtx != nil {
		return commandCtx
	}
	return context.Background()
}

// Execute runs the root command and prints usage only for CLI syntax errors.
// Runtime errors remain quiet here so Run can print a single concise message.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	commandCtx = ctx
	RootCmd.SetContext(ctx)

	cmd, err := RootCmd.ExecuteC()
	if err == nil {
		return nil
	}
	if shouldShowUsage(cmd, err) {
		if cmd == nil {
			cmd = RootCmd
		}
		_ = cmd.Usage()
	} else if flagJSON && !hasExitCode(err) {
		return emitJSONError(contextFromCommand(cmd), edit.StatusBackendFailed, "invalid-request", err.Error(), "")
	}
	return err
}

func hasExitCode(err error) bool {
	var ec exitCoder
	return errors.As(err, &ec)
}

func shouldShowUsage(cmd *cobra.Command, err error) bool {
	var usageErr *usageError
	return errors.As(err, &usageErr) || isRootUnknownCommand(cmd, err)
}

func isRootUnknownCommand(cmd *cobra.Command, err error) bool {
	return cmd == RootCmd && strings.HasPrefix(err.Error(), "unknown command ")
}

func contextFromCommand(cmd *cobra.Command) jsonContext {
	ctx := jsonContext{}
	if cmd != nil {
		ctx.Operation = cmd.Name()
	}
	return ctx
}
