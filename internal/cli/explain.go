package cli

import "github.com/spf13/cobra"

func newExplainCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "explain <rule-id|param>",
		Short: "Explain a parameter or rule: rationale, sources, kernel notes",
		Args:  cobra.ExactArgs(1),
		RunE:  notImplemented,
	}
	return cmd
}
