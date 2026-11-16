package cli

import "github.com/spf13/cobra"

func newGenerateCmd(g *globalOpts) *cobra.Command {
	_ = g
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Write sysctl.d, limits.d, systemd or tuned files (touches nothing)",
		RunE:  notImplemented,
	}
	cmd.Flags().String("format", "all", "sysctl|limits|systemd|tuned|cmdline|nic|runtime|all")
	cmd.Flags().String("out", "", "write files into DIR instead of stdout")
	return cmd
}
