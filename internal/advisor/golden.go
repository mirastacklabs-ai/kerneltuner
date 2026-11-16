package advisor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixture"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

// GoldenResult is the outcome of one rule test.
type GoldenResult struct {
	RuleID  string
	Fixture string
	Pass    bool
	Message string           // human-readable diff when !Pass
	Got     []Recommendation // what the rule produced on the fixture
}

// RunGolden runs every test of every rule: materialize the fixture, collect
// facts, detect the workload, inject the fixture's values and rates, evaluate
// the one rule and compare with expect. fixturesDir is the directory holding
// the fixtures; "" means fixture.FixturesDir().
//
// A test passes when some recommendation of the rule has the expected status
// and, if given, the expected target and proposed value. When expect.target
// is set, only the recommendation for that target is considered.
func RunGolden(rs []rules.Rule, eco rules.Ecosystem, fixturesDir string) []GoldenResult {
	var out []GoldenResult
	if len(rs) == 0 {
		return out
	}
	if fixturesDir == "" {
		dir, err := fixture.FixturesDir()
		if err != nil {
			for _, r := range rs {
				for _, t := range r.Tests {
					out = append(out, GoldenResult{RuleID: r.ID, Fixture: t.Fixture, Message: err.Error()})
				}
			}
			return out
		}
		fixturesDir = dir
	}
	cache := newFixtureCache(fixturesDir)
	defer cache.close()
	var names []string
	for _, r := range rs {
		for _, t := range r.Tests {
			names = append(names, t.Fixture)
		}
	}
	cache.preload(names)
	for _, r := range rs {
		for _, t := range r.Tests {
			out = append(out, runGoldenTest(r, t, eco, cache))
		}
	}
	return out
}

// goldenFixture is a fixture materialized and collected once per RunGolden.
type goldenFixture struct {
	root          facts.Root
	values, rates map[string]float64
	facts         *facts.Facts
	workload      *workload.Workload
	err           error
}

// fixtureCache materializes and collects each fixture once, keyed by name,
// under one temp directory removed by close.
type fixtureCache struct {
	fixturesDir string
	tmp         string
	byName      map[string]*goldenFixture
}

func newFixtureCache(fixturesDir string) *fixtureCache {
	return &fixtureCache{fixturesDir: fixturesDir, byName: map[string]*goldenFixture{}}
}

// close removes every materialized fixture, in parallel (the work is
// syscall-bound: thousands of tiny files).
func (c *fixtureCache) close() {
	if c.tmp == "" {
		return
	}
	ents, _ := os.ReadDir(c.tmp)
	var wg sync.WaitGroup
	sem := make(chan struct{}, ioWorkers)
	for _, e := range ents {
		wg.Add(1)
		sem <- struct{}{}
		go func(p string) {
			defer wg.Done()
			_ = os.RemoveAll(p)
			<-sem
		}(filepath.Join(c.tmp, e.Name()))
	}
	wg.Wait()
	_ = os.RemoveAll(c.tmp)
}

// ioWorkers bounds the parallel materialize/collect/remove workers.
const ioWorkers = 8

// preload materializes and collects every distinct fixture in names in
// parallel; get then only reads the cache.
func (c *fixtureCache) preload(names []string) {
	if c.tmp == "" {
		tmp, err := os.MkdirTemp("", "kt-golden-")
		if err != nil {
			return // get reports the error per fixture
		}
		c.tmp = tmp
	}
	var uniq []string
	seen := map[string]bool{}
	for _, n := range names {
		if !seen[n] {
			seen[n] = true
			uniq = append(uniq, n)
		}
	}
	res := make([]*goldenFixture, len(uniq))
	var wg sync.WaitGroup
	sem := make(chan struct{}, ioWorkers)
	for i, n := range uniq {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, n string) {
			defer wg.Done()
			res[i] = c.load(n)
			<-sem
		}(i, n)
	}
	wg.Wait()
	for i, n := range uniq {
		c.byName[n] = res[i]
	}
}

func (c *fixtureCache) get(name string) *goldenFixture {
	if g, ok := c.byName[name]; ok {
		return g
	}
	g := c.load(name)
	c.byName[name] = g
	return g
}

func (c *fixtureCache) load(name string) (g *goldenFixture) {
	g = &goldenFixture{}
	defer func() {
		if p := recover(); p != nil {
			g.err = fmt.Errorf("panic: %v", p)
		}
	}()
	spec, err := fixture.ParseIn(c.fixturesDir, name)
	if err != nil {
		g.err = err
		return g
	}
	if c.tmp == "" {
		if c.tmp, err = os.MkdirTemp("", "kt-golden-"); err != nil {
			g.err = err
			return g
		}
	}
	dir, err := os.MkdirTemp(c.tmp, "fx-")
	if err != nil {
		g.err = err
		return g
	}
	g.root, g.values, g.rates, err = fixture.MaterializeSpec(dir, spec)
	if err != nil {
		g.err = fmt.Errorf("materialize: %w", err)
		return g
	}
	if g.facts, err = facts.Collect(g.root); err != nil {
		g.err = fmt.Errorf("collect: %w", err)
		return g
	}
	if g.workload, err = workload.Detect(g.root, g.facts); err != nil {
		g.err = fmt.Errorf("detect: %w", err)
	}
	return g
}

func runGoldenTest(r rules.Rule, t rules.RuleTest, eco rules.Ecosystem, cache *fixtureCache) (res GoldenResult) {
	res = GoldenResult{RuleID: r.ID, Fixture: t.Fixture}
	fail := func(format string, a ...any) GoldenResult {
		res.Message = fmt.Sprintf(format, a...)
		return res
	}
	defer func() {
		if p := recover(); p != nil {
			res = fail("panic: %v", p)
		}
	}()
	g := cache.get(t.Fixture)
	if g.err != nil {
		return fail("%v", g.err)
	}
	recs := Evaluate([]rules.Rule{r}, Input{Facts: g.facts, Workload: g.workload, Values: g.values, Rates: g.rates, Root: g.root, Ecosystem: eco})
	res.Got = recs

	var cands []Recommendation
	for _, rec := range recs {
		if t.Expect.Target == "" || rec.Target == t.Expect.Target {
			cands = append(cands, rec)
		}
	}
	for _, rec := range cands {
		if string(rec.Status) == t.Expect.Status && (t.Expect.Proposed == "" || rec.Proposed == t.Expect.Proposed) {
			res.Pass = true
			return res
		}
	}
	var got []string
	for _, rec := range recs {
		g := fmt.Sprintf("status=%s target=%q current=%q proposed=%q", rec.Status, rec.Target, rec.Current, rec.Proposed)
		if rec.Status == StatusError {
			g += " " + rec.Advice
		}
		got = append(got, g)
	}
	if len(got) == 0 {
		got = []string{"(no recommendations)"}
	}
	return fail("want status=%s target=%q proposed=%q\n    got:  %s", t.Expect.Status, t.Expect.Target, t.Expect.Proposed, strings.Join(got, "\n          "))
}
