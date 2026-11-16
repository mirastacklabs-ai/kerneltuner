package render

import (
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func hostile(kind, param, target, proposed string) advisor.Recommendation {
	return advisor.Recommendation{RuleID: "h", Kind: kind, Param: param, Target: target, Proposed: proposed,
		Status: advisor.StatusRecommend, Title: "t"}
}

// Q03: limits.d lines get only safe, whitespace-free values and fixed items.
func TestLimitsFallbackValueChecks(t *testing.T) {
	for _, v := range []string{"1 2", "1\n* hard nofile 1", "1\t2", "\x00", "a b"} {
		_, fb := Limits([]advisor.Recommendation{hostile("limit", "LimitNOFILE", "nginx", v)}, Meta{})
		if strings.Contains(fb, "\n*\t") || !strings.Contains(fb, "# skipped") {
			t.Errorf("value %q not skipped:\n%s", v, fb)
		}
	}
	_, fb := Limits([]advisor.Recommendation{hostile("limit", "LimitNOFILE", "nginx", "1048576")}, Meta{})
	if !strings.Contains(fb, "*\tsoft\tnofile\t1048576") {
		t.Errorf("good value dropped:\n%s", fb)
	}
}

// Q04: tuned.conf never carries injected lines or sections.
func TestTunedSkipsUnsafeEntries(t *testing.T) {
	recs := []advisor.Recommendation{
		hostile("sysctl", "net.core.somaxconn", "", "1\n[script]\nscript=/x"),
		hostile("sysctl", "bad\nname", "", "1"),
		hostile("sysctl", "net.core.ok", "", "5"),
		hostile("sysfs", "/sys/block/{item}/queue/x", "sd\na", "1"),
		hostile("sysfs", "/sys/block/{item}/queue/y", "sda", "2"),
	}
	m := Meta{Facts: &facts.Facts{}}
	m.Facts.Tuned.Active, m.Facts.Tuned.Profile = true, "evil\n[script]"
	out := Tuned(recs, m)
	for _, ln := range strings.Split(out, "\n") {
		if ln == "[script]" || strings.HasPrefix(ln, "script=") || strings.HasPrefix(ln, "include=evil") {
			t.Errorf("injected line %q in:\n%s", ln, out)
		}
	}
	for _, want := range []string{"include=throughput-performance", "net.core.ok=5", "/sys/block/sda/queue/y=2", "# skipped"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

// Q05: no shell-active character reaches GRUB_CMDLINE_LINUX or the tuned cmdline.
func TestCmdlineTokensAreWhitelisted(t *testing.T) {
	var recs []advisor.Recommendation
	for _, v := range []string{`x"; rm -rf /; "`, "$(id)", "`id`", `a\b`, "a;b", "a b", "ok_1.2,3:4+5/6-7"} {
		recs = append(recs, hostile("cmdline", "param", "", v))
	}
	recs = append(recs, hostile("cmdline", `bad"param`, "", ""), hostile("cmdline", "transparent_hugepage", "", "madvise"))
	for name, out := range map[string]string{"grub": Cmdline(recs, Meta{}), "tuned": Tuned(recs, Meta{})} {
		for _, ln := range strings.Split(out, "\n") {
			if strings.HasPrefix(ln, "#") {
				continue
			}
			ln = strings.ReplaceAll(ln, "$GRUB_CMDLINE_LINUX ", "") // the fixed prefix
			if strings.ContainsAny(ln, "$`\\;") || strings.Count(ln, `"`) > 2 {
				t.Errorf("%s: unsafe line %q", name, ln)
			}
		}
		if !strings.Contains(out, "param=ok_1.2,3:4+5/6-7") || !strings.Contains(out, "transparent_hugepage=madvise") {
			t.Errorf("%s: valid tokens missing:\n%s", name, out)
		}
		if strings.Count(out, "# skipped") != 7 {
			t.Errorf("%s: want 7 skip comments:\n%s", name, out)
		}
	}
	// Nothing valid: no GRUB line at all.
	if out := Cmdline([]advisor.Recommendation{hostile("cmdline", "a;b", "", "")}, Meta{}); strings.Contains(out, "GRUB_CMDLINE_LINUX") {
		t.Errorf("emitted a GRUB line for an invalid token:\n%s", out)
	}
}

// Q12: an interface name cannot start with "-" (it would read as an ethtool option).
func TestIfNameRejectsLeadingDash(t *testing.T) {
	for _, n := range []string{"-s", "--version", "-", "-eth0"} {
		if ifNameRe.MatchString(n) {
			t.Errorf("%q accepted", n)
		}
	}
	for _, n := range []string{"eth0", "enp1s0f0", "bond0.100", "veth@if3", "_x"} {
		if !ifNameRe.MatchString(n) {
			t.Errorf("%q rejected", n)
		}
	}
}
