package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/spf13/cobra"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/metrics"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
	rulesfs "github.com/mirastacklabs-ai/kerneltuner/rules"
)

// pipeline is the shared state of scan, generate, explain, agent and apply.
type pipeline struct {
	Root     facts.Root
	Facts    *facts.Facts
	Workload *workload.Workload
	Rules    []rules.Rule // after --domain/--pack filtering
	Eco      rules.Ecosystem
	Sources  []rules.Source

	fxValues, fxRates map[string]float64 // injected by --fixture
	tmpDir            string             // materialized --fixture, removed by Close
}

// Close removes the temp directory of a --fixture run. Safe to call always.
func (p *pipeline) Close() {
	if p != nil && p.tmpDir != "" {
		_ = os.RemoveAll(p.tmpDir)
		p.tmpDir = ""
	}
}

// addFixtureFlag registers the hidden --fixture NAME dev flag on cmd.
func addFixtureFlag(cmd *cobra.Command, g *globalOpts) {
	cmd.Flags().StringVar(&g.Fixture, "fixture", "", "dev: use testdata/fixtures/NAME (with its values and rates) as the root")
	_ = cmd.Flags().MarkHidden("fixture")
}

// materializeFixture builds the root for --fixture into a temp dir.
func (p *pipeline) materializeFixture(g *globalOpts) error {
	spec, err := fixture.Parse(g.Fixture)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "kt-fixture-")
	if err != nil {
		return err
	}
	root, values, rates, err := fixture.MaterializeSpec(dir, spec)
	if err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	p.tmpDir, p.fxValues, p.fxRates = dir, values, rates
	p.Root = root
	g.fixRoot = &root
	return nil
}

// loadPipeline collects facts, detects the workload and loads the rules
// (embedded, or --rules-dir when set), filtered by --domain and --pack.
func loadPipeline(g *globalOpts) (*pipeline, error) {
	p := &pipeline{Root: g.Root()}
	declared, err := workload.ParseDeclared(g.Workload)
	if err != nil {
		return nil, fmt.Errorf("--workload: %w", err)
	}
	if g.Fixture != "" {
		if err := p.materializeFixture(g); err != nil {
			return nil, fmt.Errorf("--fixture: %w", err)
		}
	}
	if p.Facts, err = facts.Collect(p.Root); err != nil {
		p.Close()
		return nil, fmt.Errorf("collect facts: %w", err)
	}
	if p.Workload, err = workload.Detect(p.Root, p.Facts); err != nil {
		p.Close()
		return nil, fmt.Errorf("detect workload: %w", err)
	}
	p.Workload.Declare(declared)
	all, eco, srcs, err := loadRules(g)
	if err != nil {
		p.Close()
		return nil, err
	}
	p.Eco, p.Sources = eco, srcs
	for _, r := range all {
		if g.Domain != "" && r.Domain != g.Domain {
			continue
		}
		if g.Pack != "" && r.Pack != g.Pack && path.Base(r.Pack) != g.Pack {
			continue
		}
		p.Rules = append(p.Rules, r)
	}
	return p, nil
}

// rulesFS is the embedded rule tree, or the --rules-dir directory when set.
func rulesFS(g *globalOpts) fs.FS {
	if g.RulesDir != "" {
		return os.DirFS(g.RulesDir)
	}
	return rulesfs.FS
}

// loadRules loads every rule pack: the embedded ones, or --rules-dir when set.
// It applies no --domain/--pack filtering.
func loadRules(g *globalOpts) ([]rules.Rule, rules.Ecosystem, []rules.Source, error) {
	fsys := rulesFS(g)
	rs, eco, srcs, err := rules.Load(fsys)
	if err != nil {
		if g.RulesDir != "" {
			return nil, eco, nil, fmt.Errorf("load rules (--rules-dir %s): %w", g.RulesDir, err)
		}
		return nil, eco, nil, fmt.Errorf("load rules: %w", err)
	}
	return rs, eco, srcs, nil
}

// sample reads metrics once (d <= 0: values only, no rates) or twice, d apart.
// Failures are not fatal: empty maps are returned and a warning is recorded.
func (p *pipeline) sample(ctx context.Context, d time.Duration) (values, rates map[string]float64, err error) {
	values, rates, err = p.sampleLive(ctx, d)
	for k, v := range p.fxValues { // --fixture: injected values and rates win
		values[k] = v
	}
	for k, v := range p.fxRates {
		rates[k] = v
	}
	return values, rates, err
}

func (p *pipeline) sampleLive(ctx context.Context, d time.Duration) (values, rates map[string]float64, err error) {
	values, rates = map[string]float64{}, map[string]float64{}
	first, serr := metrics.Sample(p.Root)
	if serr != nil {
		p.warn("metrics sample failed: %v", serr)
		return values, rates, nil
	}
	if d <= 0 {
		return copyMap(first.Values), rates, nil
	}
	select {
	case <-ctx.Done():
		return values, rates, ctx.Err()
	case <-time.After(d):
	}
	second, serr := metrics.Sample(p.Root)
	if serr != nil {
		p.warn("metrics sample failed: %v", serr)
		return copyMap(first.Values), rates, nil
	}
	return copyMap(second.Values), metrics.Rates(first, second), nil
}

// evaluate runs the loaded rules against the facts and the given metrics.
func (p *pipeline) evaluate(values, rates map[string]float64) []advisor.Recommendation {
	return advisor.Evaluate(p.Rules, advisor.Input{
		Facts:     p.Facts,
		Workload:  p.Workload,
		Values:    values,
		Rates:     rates,
		Root:      p.Root,
		Ecosystem: p.Eco,
	})
}

func (p *pipeline) warn(format string, a ...any) {
	p.Facts.Warnings = append(p.Facts.Warnings, fmt.Sprintf(format, a...))
}

func copyMap(m map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
