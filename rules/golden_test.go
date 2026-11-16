package rulesfs_test

import (
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	rulesfs "github.com/mirastacklabs-ai/kerneltuner/rules"
)

func load(t *testing.T) ([]rules.Rule, rules.Ecosystem, []rules.Source) {
	t.Helper()
	rs, eco, srcs, err := rules.Load(rulesfs.FS)
	if err != nil {
		t.Fatalf("load embedded rules: %v", err)
	}
	return rs, eco, srcs
}

// TestRulesValid fails with every schema problem in the embedded packs.
func TestRulesValid(t *testing.T) {
	rs, _, srcs := load(t)
	for _, err := range rules.Validate(rs, srcs) {
		t.Error(err)
	}
}

// TestGolden runs every rule test against its fixture. It passes vacuously
// while there are no rules.
func TestGolden(t *testing.T) {
	rs, eco, _ := load(t)
	for _, res := range advisor.RunGolden(rs, eco, "") {
		if !res.Pass {
			t.Errorf("%s [fixture %s]\n    %s", res.RuleID, res.Fixture, res.Message)
		}
	}
}
