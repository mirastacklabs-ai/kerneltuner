package cli

import "github.com/spf13/cobra"

func newCaptureCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "capture",
		Short: "Archive /proc and /sys data for fixtures and bug reports",
		RunE:  notImplemented,
	}
	cmd.Flags().StringP("output", "o", "", "output file")
	cmd.Flags().Bool("fixture", false, "emit a fixture.yaml instead of an archive")
	return cmd
}
