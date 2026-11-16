package advisor

import (
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

func goldenRule(tests ...rules.RuleTest) rules.Rule {
	r := sysctlRule("g.somaxconn", "net.core.somaxconn", "4096", "gte")
	r.Tests = tests
	return r
}

func TestRunGolden(t *testing.T) {
	rs := []rules.Rule{goldenRule(
		rules.RuleTest{Fixture: "low-backlog", Expect: rules.Expect{Status: "recommend", Proposed: "4096"}},
		rules.RuleTest{Fixture: "high-backlog", Expect: rules.Expect{Status: "ok"}},
		rules.RuleTest{Fixture: "low-backlog", Expect: rules.Expect{Status: "ok"}},     // deliberate mismatch
		rules.RuleTest{Fixture: "no-such-fixture", Expect: rules.Expect{Status: "ok"}}, // missing fixture
	)}
	res := RunGolden(rs, rules.Ecosystem{}, "testdata/fixtures")
	if len(res) != 4 {
		t.Fatalf("results = %d", len(res))
	}
	for i, wantPass := range []bool{true, true, false, false} {
		if res[i].Pass != wantPass {
			t.Errorf("result %d (%s): pass=%v msg=%q", i, res[i].Fixture, res[i].Pass, res[i].Message)
		}
	}
	if !strings.Contains(res[2].Message, "want status=ok") || !strings.Contains(res[2].Message, "status=recommend") {
		t.Errorf("diff not readable: %q", res[2].Message)
	}
}

func TestRunGoldenEmpty(t *testing.T) {
	if got := RunGolden(nil, rules.Ecosystem{}, ""); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}
