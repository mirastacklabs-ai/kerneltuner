package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(g *globalOpts) *cobra.Command {
	_ = g
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Version:   %s\nGitCommit: %s\nBuildDate: %s\n", Version, GitCommit, BuildDate)
		},
	}
}
