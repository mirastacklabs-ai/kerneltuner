package cli

import "github.com/spf13/cobra"

func newStatusCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show firing rules and trends from the running agent",
		RunE:  notImplemented,
	}
	cmd.Flags().String("state", "/run/kerneltuner/state.json", "state file path")
	return cmd
}
