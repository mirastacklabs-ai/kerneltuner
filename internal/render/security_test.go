package render

import (
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
)

func nicRec(target, param, val string) advisor.Recommendation {
	return advisor.Recommendation{RuleID: "nic." + param, Kind: "nic", Param: param, Target: target, Proposed: val,
		Status: advisor.StatusRecommend, Title: "t"}
}

// Q19: interface names are validated and quoted in nic.sh.
func TestNICScriptSkipsAndQuotesInterfaceNames(t *testing.T) {
	out := NIC([]advisor.Recommendation{
		nicRec("eth0", "rx_ring", "4096"),
		nicRec("eth0; rm -rf /", "rx_ring", "4096"),
		nicRec("$(id)", "combined", "8"),
		nicRec("a\nb", "tx_ring", "2"),
		nicRec("0123456789abcdef", "rx_ring", "1"), // 16 chars: too long
	}, Meta{})
	if !strings.Contains(out, "ethtool -G eth0 rx 4096\n") {
		t.Errorf("valid NIC missing:\n%s", out)
	}
	for _, bad := range []string{"rm -rf", "$(id)", "0123456789abcdef"} {
		for _, ln := range strings.Split(out, "\n") {
			if strings.Contains(ln, bad) && !strings.HasPrefix(ln, "# skipped interface") {
				t.Errorf("hostile name reached the script: %q", ln)
			}
		}
	}
	if strings.Count(out, "# skipped interface") != 4 {
		t.Errorf("want 4 skip notes:\n%s", out)
	}
	if shq("a b") != "'a b'" || shq("it's") != `'it'\''s'` || shq("eth0") != "eth0" {
		t.Error("shq")
	}
}

// Q01: systemd hint and drop-in names for hostile targets.
func TestLimitsRejectsHostileUnitNames(t *testing.T) {
	r := advisor.Recommendation{RuleID: "l", Kind: "limit", Param: "LimitNOFILE", Target: "x\nExecStart=/bin/sh;.service",
		Proposed: "1\nBad=1", Status: advisor.StatusRecommend, Title: "t"}
	drop, fallback := Limits([]advisor.Recommendation{r}, Meta{})
	if len(drop) != 0 {
		t.Fatalf("hostile unit produced a drop-in: %v", drop)
	}
	for _, ln := range strings.Split(fallback, "\n") {
		if strings.HasPrefix(ln, "ExecStart") || strings.HasPrefix(ln, "Bad=") {
			t.Errorf("injected line: %q", ln)
		}
	}
}
