package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

func newValidateCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Check rule schema, source registration and test fixtures",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if g.RulesDir != "" {
				if err := rules.CheckPacks(rulesFS(g)); err != nil {
					return fmt.Errorf("load rules (--rules-dir %s): %w", g.RulesDir, err)
				}
			}
			rs, eco, srcs, err := loadRules(g)
			if err != nil {
				return err
			}
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			problems := 0
			report := func(format string, a ...any) {
				problems++
				_, _ = fmt.Fprintf(out, format+"\n", a...)
			}

			fixturesDir, derr := fixture.FixturesDir()
			if derr != nil {
				_, _ = fmt.Fprintf(errOut, "warning: %v; skipping fixture and golden checks\n", derr)
				fixturesDir = ""
			}
			for _, e := range rules.ValidateWith(rs, srcs, fixturesDir) {
				report("invalid: %v", e)
			}
			tests := 0
			for _, r := range rs {
				tests += len(r.Tests)
			}
			if problems == 0 && fixturesDir != "" {
				for _, res := range advisor.RunGolden(rs, eco, fixturesDir) {
					if !res.Pass {
						report("golden: %s [fixture %s]: %s", res.RuleID, res.Fixture, res.Message)
					}
				}
			}
			if problems > 0 {
				return &ExitError{Code: 1, Msg: fmt.Sprintf("validate: %d problem(s)", problems)}
			}
			_, err = fmt.Fprintf(out, "OK: %d rules, %d sources, %d tests\n", len(rs), len(srcs), tests)
			return err
		},
	}
	return cmd
}
