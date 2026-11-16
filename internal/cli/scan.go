package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/render"
)

// ExitError makes a command exit with a specific code. An empty Msg prints
// nothing. Use ExitCode in main to map an error to a process exit code.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }

// ExitCode returns the process exit code for err: 0 for nil, the code of an
// ExitError, otherwise 1.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}

var riskRank = map[string]int{"low": 1, "medium": 2, "high": 3}

func (p *pipeline) meta() render.Meta {
	return render.Meta{Version: Version, Facts: p.Facts, Workload: p.Workload}
}

func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func newScanCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Report recommended kernel settings for this host",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sampleDur, _ := cmd.Flags().GetDuration("sample")
			all, _ := cmd.Flags().GetBool("all")
			failOn, _ := cmd.Flags().GetString("fail-on")
			if failOn != "" && riskRank[failOn] == 0 {
				return fmt.Errorf("--fail-on %q: want low, medium or high", failOn)
			}
			p, err := loadPipeline(g)
			if err != nil {
				return err
			}
			defer p.Close()
			values, rates, err := p.sample(cmdContext(cmd), sampleDur)
			if err != nil {
				return err
			}
			recs := p.evaluate(values, rates)
			if g.JSON {
				err = render.JSON(cmd.OutOrStdout(), recs, p.meta(), all)
			} else {
				err = render.Text(cmd.OutOrStdout(), recs, p.meta(), all)
			}
			if err != nil {
				return err
			}
			if failOn != "" && exceeds(recs, failOn) {
				return &ExitError{Code: 2}
			}
			return nil
		},
	}
	addFixtureFlag(cmd, g)
	cmd.Flags().Duration("sample", 0, "sample live counters for this long to compute rates")
	cmd.Flags().Bool("all", false, "include ok, not-applicable and managed results")
	cmd.Flags().String("fail-on", "", "exit 2 if recommend/firing at risk >= LEVEL (low|medium|high); off by default")
	return cmd
}

// exceeds reports whether any recommend/firing result has risk >= level.
func exceeds(recs []advisor.Recommendation, level string) bool {
	for _, r := range recs {
		if (r.Status == advisor.StatusRecommend || r.Status == advisor.StatusFiring) && riskRank[r.Risk] >= riskRank[level] {
			return true
		}
	}
	return false
}
