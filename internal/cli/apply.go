package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/apply"
)

// geteuid is a variable so tests can simulate non-root.
var geteuid = os.Geteuid

func newApplyCmd(g *globalOpts) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply recommendations with a snapshot (requires root)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ruleIDs, _ := cmd.Flags().GetStringArray("rule")
			dry, _ := cmd.Flags().GetBool("dry-run")
			yes, _ := cmd.Flags().GetBool("yes")
			// Guards run before anything reads or writes the system.
			if !dry {
				if err := requireRoot(g); err != nil {
					return err
				}
				if !yes {
					return errors.New("apply changes the system: re-run with --yes to confirm, or --dry-run to preview")
				}
			}
			p, err := loadPipeline(g)
			if err != nil {
				return err
			}
			defer p.Close()
			values, rates, err := p.sample(cmd.Context(), 0)
			if err != nil {
				return err
			}
			return runApply(cmd.OutOrStdout(), cmd.ErrOrStderr(), g, p.evaluate(values, rates), ruleIDs, dry)
		},
	}
	addFixtureFlag(cmd, g)
	cmd.Flags().StringArray("rule", nil, "apply only this rule ID (repeatable)")
	cmd.Flags().Bool("dry-run", false, "show what would change without writing")
	cmd.Flags().Bool("yes", false, "confirm writing (required unless --dry-run)")
	return cmd
}

// requireRoot refuses unless running as uid 0 or --root was given explicitly.
func requireRoot(g *globalOpts) error {
	if g.RootDir != "" || geteuid() == 0 {
		return nil
	}
	return errors.New("refusing to touch the real system: run as root, or pass --root DIR to work on a copy")
}

// runApply filters recs, plans, shows the plan and (unless dry) applies it.
func runApply(out, errOut io.Writer, g *globalOpts, recs []advisor.Recommendation, ruleIDs []string, dry bool) error {
	if len(ruleIDs) > 0 {
		want := map[string]bool{}
		for _, id := range ruleIDs {
			want[id] = true
		}
		var kept []advisor.Recommendation
		for _, r := range recs {
			if want[r.RuleID] {
				kept = append(kept, r)
				delete(want, r.RuleID)
			}
		}
		for id := range want {
			_, _ = fmt.Fprintf(errOut, "warning: rule %s produced no recommendation\n", id)
		}
		recs = kept
	}
	plan, err := apply.BuildPlan(recs, g.Root())
	if err != nil {
		return err
	}
	if g.JSON {
		return runApplyJSON(out, plan, dry)
	}
	printPlan(out, plan)
	if len(plan.Ops) == 0 {
		_, _ = fmt.Fprintln(out, "nothing to apply")
		return nil
	}
	res, err := apply.Apply(plan, apply.Options{DryRun: dry, Version: Version})
	if err != nil {
		return err
	}
	return report(out, res, dry)
}

func runApplyJSON(out io.Writer, plan *apply.Plan, dry bool) error {
	res, err := apply.Apply(plan, apply.Options{DryRun: dry, Version: Version})
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		*apply.Result
		Skipped []apply.Skip `json:"skipped,omitempty"`
	}{res, plan.Skipped})
}

func report(out io.Writer, res *apply.Result, dry bool) error {
	if dry {
		_, _ = fmt.Fprintln(out, "dry run: nothing was written")
		return nil
	}
	_, _ = fmt.Fprintf(out, "applied and verified %d change(s)\nsnapshot: %s\n", len(res.Ops), res.Snapshot)
	for _, h := range res.Hints {
		_, _ = fmt.Fprintln(out, h)
	}
	_, _ = fmt.Fprintf(out, "undo with: kerneltuner rollback %s\n", res.ID)
	return nil
}

func printPlan(out io.Writer, plan *apply.Plan) {
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "RULE\tKIND\tTARGET\tNEW VALUE\tWRITES")
	for _, op := range plan.Ops {
		name := op.Param
		if op.Target != "" {
			name += " [" + op.Target + "]"
		}
		w := op.LivePath
		if op.File != "" {
			if w != "" {
				w += " + "
			}
			w += op.File
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", op.RuleID, op.Kind, name, op.Proposed, w)
	}
	_ = tw.Flush()
	for _, s := range plan.Skipped {
		_, _ = fmt.Fprintf(out, "skipped %s: %s\n", s.RuleID, s.Reason)
	}
	for _, w := range plan.Warnings {
		_, _ = fmt.Fprintf(out, "warning: merged duplicates: %s\n", w)
	}
}
