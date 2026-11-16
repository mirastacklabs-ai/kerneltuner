package advisor

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
)

func TestExactIntegerFormatting(t *testing.T) {
	c := newTC(t)
	c.f.Sysctl["kernel.shmmax"] = "9223372036854775806"
	cases := []struct{ rec, want string }{
		{"9223372036854775807", "9223372036854775807"},
		{`"9223372036854775807"`, "9223372036854775807"},
		{"max(9223372036854775807, 5)", "9223372036854775807"},
		{"clamp(9223372036854775807, 1, 9223372036854775807)", "9223372036854775807"},
		{"pow2ceil(4097)", "8192"},
		{"pow2ceil(4097.5)", "8192"},
		{"[4096, 87380, 9223372036854775807]", "4096 87380 9223372036854775807"},
		{`bytes("4M")`, "4194304"},
		{"min(3, 2.5)", "2.5"},
	}
	for _, tt := range cases {
		r := sysctlRule("x", "kernel.shmmax", tt.rec, "eq")
		if got := one(t, c.eval(r)); got.Proposed != tt.want {
			t.Errorf("%s: proposed %q (advice %q), want %q", tt.rec, got.Proposed, got.Advice, tt.want)
		}
	}
	// current one below proposed: exact compare must not call them equal.
	r := one(t, c.eval(sysctlRule("x", "kernel.shmmax", "9223372036854775807", "gte")))
	if r.Status != StatusRecommend || r.Current != "9223372036854775806" {
		t.Errorf("exact compare: %+v", r)
	}
	r = one(t, c.eval(sysctlRule("x", "kernel.shmmax", "9223372036854775806", "gte")))
	if r.Status != StatusOK {
		t.Errorf("equal huge ints: %+v", r)
	}
}

func TestNumericHelpersAcceptInts(t *testing.T) {
	c := newTC(t)
	c.f.CPU.Threads = 12
	for _, rec := range []string{
		"pow2ceil(facts.CPU.Threads)", "clamp(facts.CPU.Threads, 1, 8)", "max(facts.CPU.Threads, 4)", "min(facts.CPU.Threads, 4.5)",
	} {
		got := one(t, c.eval(sysctlRule("x", "vm.swappiness", rec, "eq")))
		if got.Status == StatusError {
			t.Errorf("%s: %s", rec, got.Advice)
		}
	}
	if got := one(t, c.eval(sysctlRule("x", "vm.swappiness", "pow2ceil(facts.CPU.Threads)", "eq"))); got.Proposed != "16" {
		t.Errorf("pow2ceil(12) = %q", got.Proposed)
	}
}

func TestServiceRunningMatching(t *testing.T) {
	c := newTC(t)
	c.f.Procs = []facts.Proc{
		{PID: 1, Comm: "nvidia-persiste", Cmdline: []string{"/usr/bin/nvidia-persistenced", "--verbose"}},
		{PID: 2, Comm: "nginx", Cmdline: []string{"nginx: master process /usr/sbin/nginx"}},
		{PID: 3, Comm: "worker", Cmdline: []string{"/opt/bin/chronyd"}},
		{PID: 4, Comm: "x", Unit: "foo.service"},
		{PID: 5, Comm: "short"},
	}
	for name, want := range map[string]bool{
		"nvidia-persistenced.service": true, // comm prefix of a 15-char truncation
		"nginx":                       true,
		"chronyd":                     true, // argv[0] basename
		"foo.service":                 true, // unit name
		"nvidia-persistent":           true,
		"nvidia-fabricmanager":        false,
		"shortcut":                    false, // short comm: no prefix match
		"missing":                     false,
	} {
		r := rules.Rule{ID: "s", Kind: "service", Param: name, Recommend: `"running"`, Domain: "ai", Risk: "low", Title: "s"}
		got := one(t, c.eval(r))
		if running := got.Current == "running"; running != want {
			t.Errorf("%s: current=%q, want running=%v", name, got.Current, want)
		}
	}
}

func TestRuntimeAndServiceDetect(t *testing.T) {
	c := newTC(t)
	rt := rules.Rule{ID: "r", Kind: "runtime", Domain: "cpu", Risk: "low", Title: "r", Advice: "do it", Detect: "facts.CPU.Threads > 4"}
	if got := one(t, c.eval(rt)); got.Status != StatusOK {
		t.Errorf("detect false: %s", got.Status)
	}
	c.f.CPU.Threads = 8
	if got := one(t, c.eval(rt)); got.Status != StatusRecommend {
		t.Errorf("detect true: %s", got.Status)
	}
	rt.Detect = ""
	c.f.CPU.Threads = 1
	if got := one(t, c.eval(rt)); got.Status != StatusRecommend {
		t.Errorf("no detect keeps recommend: %s", got.Status)
	}

	svc := rules.Rule{ID: "s", Kind: "service", Param: "irqbalance", Recommend: `"stopped"`, Domain: "cpu", Risk: "low", Title: "s"}
	if got := one(t, c.eval(svc)); got.Status != StatusRecommend { // running, want stopped
		t.Errorf("service without detect: %s", got.Status)
	}
	svc.Detect = `current == "running" && facts.CPU.Threads > 4`
	if got := one(t, c.eval(svc)); got.Status != StatusOK {
		t.Errorf("service detect false: %s", got.Status)
	}
	c.f.CPU.Threads = 16
	if got := one(t, c.eval(svc)); got.Status != StatusRecommend || got.Proposed != "stopped" {
		t.Errorf("service detect true: %+v", got)
	}
}

func TestRequireEvidence(t *testing.T) {
	c := newTC(t)
	r := sysctlRule("e", "net.core.somaxconn", "4096", "gte")
	r.Evidence = []rules.Evidence{{Metric: "TcpExt.ListenOverflows", RateGT: fp(0)}}
	if got := one(t, c.eval(r)); got.Status != StatusRecommend {
		t.Errorf("default: %s", got.Status)
	}
	r.RequireEvidence = true
	if got := one(t, c.eval(r)); got.Status != StatusOK {
		t.Errorf("require_evidence without hit: %s", got.Status)
	}
	c.rates = map[string]float64{"TcpExt.ListenOverflows": 3}
	if got := one(t, c.eval(r)); got.Status != StatusRecommend || len(got.Evidence) != 1 {
		t.Errorf("require_evidence with hit: %+v", got)
	}

	ap := rules.Rule{ID: "ap", Kind: "antipattern", Domain: "network", Risk: "low", Title: "ap", Detect: "true",
		Evidence: []rules.Evidence{{Metric: "m", RateGT: fp(1)}}, RequireEvidence: true}
	c.rates = nil
	if got := one(t, c.eval(ap)); got.Status != StatusOK {
		t.Errorf("antipattern without evidence: %s", got.Status)
	}
}

func TestCmdlineCommaLists(t *testing.T) {
	c := newTC(t)
	c.f.Kernel.CmdlineArgs["pci"] = "realloc=off,nommconf"
	mk := func(rec string) rules.Rule {
		return rules.Rule{ID: "c", Kind: "cmdline", Param: "pci", Recommend: rec, Reboot: true, Domain: "ai", Risk: "low", Title: "c"}
	}
	for rec, want := range map[string]Status{
		`"realloc=off"`:               StatusOK,
		`"nommconf"`:                  StatusOK,
		`"nommconf,realloc=off"`:      StatusOK,
		`"realloc=off,nommconf,pcie"`: StatusRecommend,
		`"pcie"`:                      StatusRecommend,
	} {
		if got := one(t, c.eval(mk(rec))); got.Status != want {
			t.Errorf("%s: %s, want %s", rec, got.Status, want)
		}
	}
	applies := mk(`"x"`)
	applies.Kind = "antipattern"
	applies.Detect = `cmdlineHas("pci", "realloc=off") && !cmdlineHas("pci", "pcie") && !cmdlineHas("iommu", "x")`
	applies.Reboot = false
	if got := one(t, c.eval(applies)); got.Status != StatusFiring {
		t.Errorf("cmdlineHas: %s (%s)", got.Status, got.Advice)
	}
}

func TestForeachCPUSummary(t *testing.T) {
	c := newTC(t)
	c.f.CPU.IDs = []int{0, 1, 2, 3, 4, 5, 6, 7}
	for _, id := range c.f.CPU.IDs {
		gov := "powersave"
		if id >= 6 {
			gov = "performance"
		}
		writeSys(t, c.root.Sys, "devices/system/cpu/cpu"+strconv.Itoa(id)+"/cpufreq/scaling_governor", gov+"\n")
	}
	r := rules.Rule{ID: "g", Kind: "sysfs", Param: "/sys/devices/system/cpu/{item}/cpufreq/scaling_governor",
		Recommend: `"performance"`, Foreach: "cpu", Domain: "cpu", Risk: "low", Title: "g"}
	recs := c.eval(r)
	if len(recs) != 8 || recs[0].Target != "cpu0" {
		t.Fatalf("no summary: %d recs, first %+v", len(recs), recs[0])
	}
	r.ForeachSummary = true
	recs = c.eval(r)
	if len(recs) != 2 {
		t.Fatalf("summary: %d recs: %+v", len(recs), recs)
	}
	var rec, ok Recommendation
	for _, x := range recs {
		if x.Status == StatusRecommend {
			rec = x
		} else {
			ok = x
		}
	}
	if rec.Target != "cpu0-5 (6)" || len(rec.Targets) != 6 || rec.Targets[5] != "cpu5" {
		t.Errorf("recommend group: %q %v", rec.Target, rec.Targets)
	}
	if ok.Status != StatusOK || ok.Target != "cpu6-7 (2)" {
		t.Errorf("ok group: %+v", ok)
	}
	c.f.CPU.IDs = []int{0, 1, 2, 4}
	if got := summaryTarget([]string{"cpu0", "cpu1", "cpu2", "cpu4"}); got != "cpu0-2,4 (4)" {
		t.Errorf("gaps: %q", got)
	}
	if got := summaryTarget([]string{"eth0", "eth1"}); got != "eth0,eth1 (2)" {
		t.Errorf("non-cpu: %q", got)
	}
}

func TestAccelKindEnv(t *testing.T) {
	c := newTC(t)
	c.f.Accel = []facts.Accelerator{
		{Vendor: "nvidia", Kind: "gpu"}, {Vendor: "nvidia", Kind: "gpu"},
		{Vendor: "nvidia", Kind: "nvswitch"}, {Vendor: "habana", Kind: "npu"},
	}
	for expr, want := range map[string]string{
		`accelCount("nvidia")`:                 "2",
		`accelKindCount("nvidia", "nvswitch")`: "1",
		`accelKindCount("nvidia", "gpu")`:      "2",
		`accelKindCount("habana", "npu")`:      "1",
		`accelCount("habana")`:                 "1",
	} {
		if got := one(t, c.eval(sysctlRule("k", "vm.swappiness", expr, "eq"))); got.Proposed != want {
			t.Errorf("%s = %q, want %s", expr, got.Proposed, want)
		}
	}
}

func TestEngineFixtureEndToEnd(t *testing.T) {
	// hosts with strings in advice stay printable
	c := newTC(t)
	r := rules.Rule{ID: "a", Kind: "runtime", Domain: "ai", Risk: "low", Title: "a", Advice: "{{.facts.Host.Distro.ID}}"}
	c.f.Host.Distro = facts.Distro{ID: "ubuntu", VersionID: "24.04"}
	if got := one(t, c.eval(r)); !strings.Contains(got.Advice, "ubuntu") {
		t.Errorf("advice: %q", got.Advice)
	}
}
