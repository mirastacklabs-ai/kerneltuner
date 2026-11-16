// Package cli wires the kerneltuner command tree.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
)

// Set at build time via -ldflags -X.
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// globalOpts holds the persistent flags shared by every subcommand.
type globalOpts struct {
	RootDir  string
	RulesDir string
	Domain   string
	Pack     string
	JSON     bool
	Workload []string

	// Fixture is the hidden dev flag --fixture NAME (scan, generate, apply):
	// materialize testdata/fixtures/NAME into a temp dir and use it as the root,
	// with its injected values and rates.
	Fixture string
	// FixturesDir is the hidden --fixtures-dir flag (else $KERNELTUNER_FIXTURES).
	FixturesDir string
	// fixRoot is set by loadPipeline once the fixture is materialized.
	fixRoot *facts.Root
}

// Root resolves the filesystem root: DefaultRoot, or RootAt(--root).
func (g *globalOpts) Root() facts.Root {
	if g.fixRoot != nil {
		return *g.fixRoot
	}
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
	root.PersistentPreRun = func(*cobra.Command, []string) {
		if g.FixturesDir != "" {
			fixture.SetDir(g.FixturesDir)
		}
	}
	pf := root.PersistentFlags()
	pf.StringVar(&g.RootDir, "root", "", "treat DIR as the filesystem root (DIR/proc, DIR/sys, ...)")
	pf.StringVar(&g.RulesDir, "rules-dir", "", "load rule packs from DIR (the directory that holds kernel/, ai/, apps/, antipatterns/) instead of the embedded ones")
	pf.StringVar(&g.Domain, "domain", "", "only rules in this domain (network|limits|memory|cpu|storage|ai)")
	pf.StringVar(&g.Pack, "pack", "", "only rules in this pack (e.g. kernel/network)")
	pf.BoolVar(&g.JSON, "json", false, "machine-readable JSON output")
	pf.StringSliceVar(&g.Workload, "workload", nil, "declare workloads NAME[,NAME...] as present even if not running, to tune a host before installing the software (unknown names are rejected)")
	pf.StringVar(&g.FixturesDir, "fixtures-dir", "", "dev: directory holding fixtures (default $KERNELTUNER_FIXTURES)")
	_ = pf.MarkHidden("fixtures-dir")
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
