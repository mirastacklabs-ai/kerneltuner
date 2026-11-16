package cli

import "github.com/spf13/cobra"

func newValidateCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check rule schema, source registration and test fixtures",
		RunE:  notImplemented,
	}
	return cmd
}
