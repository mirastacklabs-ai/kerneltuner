package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
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
}

// loadPipeline collects facts, detects the workload and loads the rules
// (embedded, or --rules-dir when set), filtered by --domain and --pack.
func loadPipeline(g *globalOpts) (*pipeline, error) {
	p := &pipeline{Root: g.Root()}
	var err error
	if p.Facts, err = facts.Collect(p.Root); err != nil {
		return nil, fmt.Errorf("collect facts: %w", err)
	}
	if p.Workload, err = workload.Detect(p.Root, p.Facts); err != nil {
		return nil, fmt.Errorf("detect workload: %w", err)
	}
	all, eco, srcs, err := loadRules(g)
	if err != nil {
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

// loadRules loads every rule pack: the embedded ones, or --rules-dir when set.
// It applies no --domain/--pack filtering.
func loadRules(g *globalOpts) ([]rules.Rule, rules.Ecosystem, []rules.Source, error) {
	var fsys fs.FS = rulesfs.FS
	if g.RulesDir != "" {
		fsys = os.DirFS(g.RulesDir)
	}
	rs, eco, srcs, err := rules.Load(fsys)
	if err != nil {
		return nil, eco, nil, fmt.Errorf("load rules: %w", err)
	}
	return rs, eco, srcs, nil
}

// sample reads metrics once (d <= 0: values only, no rates) or twice, d apart.
// Failures are not fatal: empty maps are returned and a warning is recorded.
func (p *pipeline) sample(ctx context.Context, d time.Duration) (values, rates map[string]float64, err error) {
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
