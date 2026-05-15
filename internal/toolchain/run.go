package toolchain

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type RunOptions struct {
	ProjectRoot string
	Args        []string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
}

func Run(ctx context.Context, opts RunOptions) error {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	binary := filepath.Join(root, ActiveBinPath)
	if _, err := os.Stat(binary); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s is missing; run `refute-tool sync` first", ActiveBinPath)
		}
		return err
	}
	cmd := exec.CommandContext(ctx, binary, opts.Args...)
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return cmd.Run()
}

type DoctorOptions struct {
	ProjectRoot string
	Stdout      io.Writer
	Stderr      io.Writer
}

func Doctor(ctx context.Context, opts DoctorOptions) error {
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	out := opts.Stdout
	if out == nil {
		out = io.Discard
	}
	lockPath := filepath.Join(root, LockfileName)
	if _, err := os.Stat(lockPath); err != nil {
		fmt.Fprintf(out, "lockfile: missing (%s)\n", LockfileName)
	} else {
		fmt.Fprintf(out, "lockfile: present (%s)\n", LockfileName)
	}
	binary := filepath.Join(root, ActiveBinPath)
	if _, err := os.Stat(binary); err != nil {
		fmt.Fprintf(out, "binary: missing (%s)\n", ActiveBinPath)
		return nil
	}
	fmt.Fprintf(out, "binary: present (%s)\n", ActiveBinPath)
	fmt.Fprintln(out, "refute doctor:")
	return Run(ctx, RunOptions{
		ProjectRoot: root,
		Args:        []string{"doctor"},
		Stdout:      opts.Stdout,
		Stderr:      opts.Stderr,
	})
}
