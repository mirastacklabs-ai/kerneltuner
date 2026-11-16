package cli

import "github.com/spf13/cobra"

func newRollbackCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "rollback [ID]",
		Short: "Restore settings from an apply snapshot",
		Args:  cobra.MaximumNArgs(1),
		RunE:  notImplemented,
	}
	cmd.Flags().Bool("list", false, "list available snapshots")
	return cmd
}
