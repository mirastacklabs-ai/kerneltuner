package cli

import (
	"time"

	"github.com/spf13/cobra"
)

func newAgentCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run the observe-only sampling daemon that alerts via journald",
		RunE:  notImplemented,
	}
	cmd.Flags().Duration("interval", 15*time.Second, "sampling interval")
	cmd.Flags().String("state", "/run/kerneltuner/state.json", "state file path")
	return cmd
}
