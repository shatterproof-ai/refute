package main

import "github.com/shatterproof-ai/refute/internal/cli"

func main() {
	cli.Run(cli.RootCmd.Execute)
}
