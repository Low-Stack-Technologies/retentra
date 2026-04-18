package main

import (
	"context"
	"fmt"
	"os"

	"retentra/internal/retentra"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: retentra config.yaml")
		os.Exit(2)
	}

	if err := retentra.Run(context.Background(), os.Args[1], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "retentra: %v\n", err)
		os.Exit(1)
	}
}
