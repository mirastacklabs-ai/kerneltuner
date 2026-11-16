package cli

import "github.com/spf13/cobra"

func newApplyCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply recommendations with a snapshot (requires root)",
		RunE:  notImplemented,
	}
	cmd.Flags().StringArray("rule", nil, "apply only this rule ID (repeatable)")
	cmd.Flags().Bool("dry-run", false, "show what would change without writing")
	cmd.Flags().Bool("yes", false, "confirm writing (required unless --dry-run)")
	return cmd
}
