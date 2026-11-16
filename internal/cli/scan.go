package cli

import "github.com/spf13/cobra"

func newScanCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Report recommended kernel settings for this host",
		RunE:  notImplemented,
	}
	cmd.Flags().Duration("sample", 0, "sample live counters for this long to compute rates")
	cmd.Flags().Bool("all", false, "include ok, not-applicable and managed results")
	cmd.Flags().String("fail-on", "", "exit 2 if recommend/firing at risk >= LEVEL (low|medium|high); off by default")
	return cmd
}
