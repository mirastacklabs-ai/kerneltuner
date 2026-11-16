package advisor

import (
	"fmt"
	"os"
	"strings"

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
	for _, r := range rs {
		for _, t := range r.Tests {
			out = append(out, runGoldenTest(r, t, eco, fixturesDir))
		}
	}
	return out
}

func runGoldenTest(r rules.Rule, t rules.RuleTest, eco rules.Ecosystem, fixturesDir string) (res GoldenResult) {
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
	spec, err := fixture.ParseIn(fixturesDir, t.Fixture)
	if err != nil {
		return fail("%v", err)
	}
	dir, err := os.MkdirTemp("", "kt-golden-")
	if err != nil {
		return fail("%v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	root, values, rates, err := fixture.MaterializeSpec(dir, spec)
	if err != nil {
		return fail("materialize: %v", err)
	}
	f, err := facts.Collect(root)
	if err != nil {
		return fail("collect: %v", err)
	}
	w, err := workload.Detect(root, f)
	if err != nil {
		return fail("detect: %v", err)
	}
	recs := Evaluate([]rules.Rule{r}, Input{Facts: f, Workload: w, Values: values, Rates: rates, Root: root, Ecosystem: eco})
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
