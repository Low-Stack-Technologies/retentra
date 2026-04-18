package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"retentra/internal/retentra"
)

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		printHelp(stdout)
		return 0
	}

	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: retentra config.yaml")
		fmt.Fprintln(stderr, "run 'retentra --help' for more information")
		return 2
	}

	if err := retentra.Run(context.Background(), args[0], stdout); err != nil {
		fmt.Fprintf(stderr, "retentra: %v\n", err)
		return 1
	}
	return 0
}

func printHelp(out io.Writer) {
	fmt.Fprint(out, `retentra creates backup archives from a YAML configuration.

Usage:
  retentra config.yaml

Arguments:
  config.yaml    Path to the retentra YAML configuration file.

Options:
  -h, --help     Show this help message.

Examples:
  retentra config.yaml
  retentra /etc/retentra/nightly.yaml
`)
}
