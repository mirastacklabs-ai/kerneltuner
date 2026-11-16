// Package cli wires the kerneltuner command tree.
package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// Set at build time via -ldflags -X.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

var errNotImplemented = errors.New("not implemented yet")

// globalOpts holds the persistent flags shared by every subcommand.
type globalOpts struct {
	RootDir  string
	RulesDir string
	Domain   string
	Pack     string
	JSON     bool
}

// Root resolves the filesystem root: DefaultRoot, or RootAt(--root).
func (g *globalOpts) Root() facts.Root {
	if g.RootDir == "" {
		return facts.DefaultRoot()
	}
	return facts.RootAt(g.RootDir)
}

// NewRootCmd builds the root command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	g := &globalOpts{}
	root := &cobra.Command{
		Use:           "kerneltuner",
		Short:         "Linux kernel tuning advisor with cited, version-aware rules",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.RootDir, "root", "", "treat DIR as the filesystem root (DIR/proc, DIR/sys, ...)")
	pf.StringVar(&g.RulesDir, "rules-dir", "", "load rule packs from DIR in addition to the embedded ones")
	pf.StringVar(&g.Domain, "domain", "", "only rules in this domain (network|limits|memory|cpu|storage|ai)")
	pf.StringVar(&g.Pack, "pack", "", "only rules in this pack (e.g. kernel/network)")
	pf.BoolVar(&g.JSON, "json", false, "machine-readable JSON output")
	root.AddCommand(
		newScanCmd(g),
		newExplainCmd(g),
		newGenerateCmd(g),
		newSourcesCmd(g),
		newValidateCmd(g),
		newCaptureCmd(g),
		newAgentCmd(g),
		newStatusCmd(g),
		newApplyCmd(g),
		newRollbackCmd(g),
		newVersionCmd(g),
	)
	return root
}

// notImplemented is the RunE body of every stub subcommand.
func notImplemented(cmd *cobra.Command, _ []string) error {
	return fmt.Errorf("%s: %w", cmd.Name(), errNotImplemented)
}
