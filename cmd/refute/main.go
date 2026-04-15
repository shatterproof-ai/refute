package main

import (
	"fmt"
	"os"
)

const version = "0.1.0-dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("refute %s\n", version)
		return
	}
	fmt.Fprintln(os.Stderr, "refute: automated source code refactoring")
	fmt.Fprintln(os.Stderr, "run 'refute version' to see version")
}
