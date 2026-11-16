// Command kerneltuner is a Linux kernel tuning advisor and observe-only agent.
package main

import (
	"fmt"
	"os"

	"github.com/mirastacklabs-ai/kerneltuner/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
