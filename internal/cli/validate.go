package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

// fixturesDirFn is fixture.FixturesDir; tests replace it.
var fixturesDirFn = fixture.FixturesDir

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// checkoutFixtures finds testdata/fixtures of a source checkout without
// searching upwards: it looks only at the parent of an explicit --rules-dir,
// or at the working directory itself (which must hold rules/ and testdata/).
func checkoutFixtures(rulesDir, cwd string) string {
	if rulesDir != "" {
		if abs, err := filepath.Abs(rulesDir); err == nil {
			if d := filepath.Join(filepath.Dir(abs), "testdata", "fixtures"); isDir(d) {
				return d
			}
		}
		return ""
	}
	if cwd != "" && isDir(filepath.Join(cwd, "rules")) {
		if d := filepath.Join(cwd, "testdata", "fixtures"); isDir(d) {
			return d
		}
	}
	return ""
}

// resolveFixtures returns the fixtures directory for validate. An empty
// result means golden checks are skipped (only with --skip-golden).
func resolveFixtures(g *globalOpts, skipGolden bool) (string, error) {
	if skipGolden {
		return "", nil
	}
	dir, err := fixturesDirFn()
	if err == nil {
		return dir, nil
	}
	cwd, _ := os.Getwd()
	if d := checkoutFixtures(g.RulesDir, cwd); d != "" {
		return d, nil
	}
	return "", errors.New("validate: no fixtures directory found, so the golden tests cannot run. " +
		"Run it from a source checkout (the directory holding rules/ and testdata/), or pass --fixtures-dir DIR " +
		"(or set KERNELTUNER_FIXTURES), or pass --skip-golden to check only the rule schema and sources")
}

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
			if g.Pack != "" {
				var in []rules.Rule
				for _, r := range rs {
					if r.Pack == g.Pack || path.Base(r.Pack) == g.Pack {
						in = append(in, r)
					}
				}
				if len(in) == 0 {
					return fmt.Errorf("--pack %q: no rules in that pack", g.Pack)
				}
				rs = in
			}
			out, errOut := cmd.OutOrStdout(), cmd.ErrOrStderr()
			problems := 0
			report := func(format string, a ...any) {
				problems++
				_, _ = fmt.Fprintf(out, format+"\n", a...)
			}

			skip, _ := cmd.Flags().GetBool("skip-golden")
			fixturesDir, derr := resolveFixtures(g, skip)
			if derr != nil {
				return &ExitError{Code: 1, Msg: derr.Error()}
			}
			if skip {
				_, _ = fmt.Fprintln(errOut, "warning: --skip-golden: skipping fixture and golden checks")
			}
			for _, e := range rules.ValidateWith(rs, srcs, fixturesDir) {
				report("invalid: %v", e)
			}
			// A rule source without a verbatim quote is an error.
			for _, pr := range rules.CheckCitations(rs).Problems {
				report("invalid: %s", pr)
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
	cmd.Flags().Bool("skip-golden", false, "do not run fixture checks and golden tests (when no fixtures directory is available)")
	cmd.Flags().Bool("strict-citations", true, "deprecated no-op: a rule source without a verbatim quote is always an error")
	_ = cmd.Flags().MarkHidden("strict-citations")
	return cmd
}
