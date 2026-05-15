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
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stdout, `usage: refute-shared [sync|doctor|-- <refute args>]

Go helper for polyglot repositories using refute.lock.json and .refute/bin/refute.`)
		os.Exit(0)
	}
	ctx := context.Background()
	switch args[0] {
	case "sync":
		_, err := toolchain.Sync(ctx, toolchain.SyncOptions{ProjectRoot: "."})
		exit(err)
	case "doctor":
		exit(toolchain.Doctor(ctx, toolchain.DoctorOptions{ProjectRoot: ".", Stdout: os.Stdout, Stderr: os.Stderr}))
	case "--":
		exit(toolchain.Run(ctx, toolchain.RunOptions{ProjectRoot: ".", Args: args[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}))
	default:
		exit(toolchain.Run(ctx, toolchain.RunOptions{ProjectRoot: ".", Args: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}))
	}
}

func exit(err error) {
	if err == nil {
		os.Exit(0)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
