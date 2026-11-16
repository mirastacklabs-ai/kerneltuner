// Package cli wires the kerneltuner command tree.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// Set at build time via -ldflags -X.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var errNotImplemented = errors.New("not implemented yet")

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kerneltuner",
		Short:         "Linux kernel tuning advisor with cited, version-aware rules",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		stub("scan", "Report recommended kernel settings for this host"),
		stub("explain", "Explain a parameter or rule: rationale, sources, kernel notes"),
		stub("generate", "Write sysctl.d, limits.d, systemd or tuned files (touches nothing)"),
		stub("sources", "List upstream docs behind the active recommendations"),
		stub("agent", "Run the observe-only sampling daemon that alerts via journald"),
		stub("status", "Show firing rules and trends from the running agent"),
		stub("apply", "Apply recommendations with a snapshot (requires root)"),
		stub("rollback", "Restore settings from an apply snapshot"),
		stub("capture", "Archive /proc and /sys data for fixtures and bug reports"),
		versionCmd(),
	)
	return root
}

func stub(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("%s: %w", cmd.Name(), errNotImplemented)
		},
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Version:   %s\nGitCommit: %s\nBuildDate: %s\n", Version, GitCommit, BuildDate)
		},
	}
}
