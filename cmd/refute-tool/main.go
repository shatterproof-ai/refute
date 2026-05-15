package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/shatterproof-ai/refute/internal/toolchain"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		usage(os.Stdout)
		return 0
	}
	ctx := context.Background()
	switch args[0] {
	case "sync":
		result, err := toolchain.Sync(ctx, toolchain.SyncOptions{
			ProjectRoot: ".",
			Stdout:      os.Stdout,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "refute-tool sync: %v\n", err)
			return 1
		}
		if result.Installed {
			fmt.Fprintf(os.Stdout, "installed %s\n", toolchain.ActiveBinPath)
		} else {
			fmt.Fprintf(os.Stdout, "%s is already current\n", toolchain.ActiveBinPath)
		}
		return 0
	case "run":
		runArgs := args[1:]
		if len(runArgs) > 0 && runArgs[0] == "--" {
			runArgs = runArgs[1:]
		}
		return exitCode(toolchain.Run(ctx, toolchain.RunOptions{
			ProjectRoot: ".",
			Args:        runArgs,
			Stdin:       os.Stdin,
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}))
	case "doctor":
		return exitCode(toolchain.Doctor(ctx, toolchain.DoctorOptions{
			ProjectRoot: ".",
			Stdout:      os.Stdout,
			Stderr:      os.Stderr,
		}))
	default:
		fmt.Fprintf(os.Stderr, "unknown refute-tool command %q\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

func usage(out *os.File) {
	fmt.Fprintln(out, `usage: refute-tool <command>

Commands:
  sync          install or refresh .refute/bin/refute from refute.lock.json
  run -- <args> run .refute/bin/refute with args and preserve its exit code
  doctor        report shared toolchain state and delegate to refute doctor`)
}
