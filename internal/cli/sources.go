package cli

import "github.com/spf13/cobra"

func newSourcesCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "List upstream docs behind the active recommendations",
		RunE:  notImplemented,
	}
	cmd.Flags().Bool("all", false, "list every registered source, not only those behind active recommendations")
	return cmd
}
