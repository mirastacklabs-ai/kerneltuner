package advisor

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

func fp(v float64) *float64 { return &v }

type tc struct {
	f     *facts.Facts
	w     *workload.Workload
	vals  map[string]float64
	rates map[string]float64
	eco   rules.Ecosystem
	root  facts.Root
}

func newTC(t *testing.T) *tc {
	dir := t.TempDir()
	return &tc{
		f: &facts.Facts{
			Kernel: facts.Kernel{Release: "6.8.0", Major: 6, Minor: 8, CmdlineArgs: map[string]string{"quiet": "", "iommu": "pt"}},
			Sysctl: map[string]string{
				"net.core.somaxconn":              "128",
				"net.ipv4.tcp_rmem":               "4096 131072 6291456",
				"net.ipv4.tcp_congestion_control": "cubic",
				"vm.swappiness":                   "10",
			},
			Procs: []facts.Proc{{PID: 1, Comm: "irqbalance"}},
		},
		w:    &workload.Workload{},
		root: facts.RootAt(dir),
	}
}

func (c *tc) eval(rs ...rules.Rule) []Recommendation {
	return Evaluate(rs, Input{Facts: c.f, Workload: c.w, Values: c.vals, Rates: c.rates, Root: c.root, Ecosystem: c.eco})
}

func one(t *testing.T, recs []Recommendation) Recommendation {
	t.Helper()
	if len(recs) != 1 {
		t.Fatalf("got %d recs: %+v", len(recs), recs)
	}
	return recs[0]
}

func sysctlRule(id, param, rec, cmp string) rules.Rule {
	return rules.Rule{ID: id, Kind: "sysctl", Param: param, Recommend: rec, Compare: cmp, Domain: "network", Risk: "low", Namespaced: true, Title: id}
}

func TestSysctlCompareModes(t *testing.T) {
	c := newTC(t)
	cases := []struct {
		name   string
		r      rules.Rule
		status Status
		prop   string
	}{
		{"gte recommend", sysctlRule("a", "net.core.somaxconn", "4096", "gte"), StatusRecommend, "4096"},
		{"gte ok", sysctlRule("a", "net.core.somaxconn", "100", "gte"), StatusOK, "100"},
		{"lte ok", sysctlRule("a", "vm.swappiness", "10", "lte"), StatusOK, "10"},
		{"lte recommend", sysctlRule("a", "vm.swappiness", "1", "lte"), StatusRecommend, "1"},
		{"eq default recommend", sysctlRule("a", "vm.swappiness", "1", ""), StatusRecommend, "1"},
		{"eq string ok", sysctlRule("a", "net.ipv4.tcp_congestion_control", `"cubic"`, "eq"), StatusOK, "cubic"},
		{"eq string recommend", sysctlRule("a", "net.ipv4.tcp_congestion_control", `"bbr"`, "eq"), StatusRecommend, "bbr"},
		{"set ok", sysctlRule("a", "net.ipv4.tcp_congestion_control", `"bbr"`, "set"), StatusOK, "bbr"},
		{"list per-field gte recommend", sysctlRule("a", "net.ipv4.tcp_rmem", "[4096, 131072, bytes(\"16M\")]", "gte"), StatusRecommend, "4096 131072 16777216"},
		{"list per-field gte ok", sysctlRule("a", "net.ipv4.tcp_rmem", "[4096, 87380, 6291456]", "gte"), StatusOK, "4096 87380 6291456"},
		{"list string form", sysctlRule("a", "net.ipv4.tcp_rmem", `"4096 131072 6291456"`, "eq"), StatusOK, "4096 131072 6291456"},
		{"list length mismatch", sysctlRule("a", "net.ipv4.tcp_rmem", "4096", "gte"), StatusRecommend, "4096"},
		{"uses current", sysctlRule("a", "net.core.somaxconn", "max(4096, current*2)", "gte"), StatusRecommend, "4096"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := one(t, c.eval(tt.r))
			if r.Status != tt.status || r.Proposed != tt.prop {
				t.Errorf("status=%s proposed=%q (advice %q), want %s %q", r.Status, r.Proposed, r.Advice, tt.status, tt.prop)
			}
		})
	}
}

func TestSysctlMissingIsNotApplicableAndProcFallback(t *testing.T) {
	c := newTC(t)
	r := sysctlRule("a", "net.core.nope", "1", "eq")
	if got := one(t, c.eval(r)); got.Status != StatusNotApplicable {
		t.Errorf("status = %s", got.Status)
	}
	p := filepath.Join(c.root.Proc, "sys/net/core/rmem_max")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("212992\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := one(t, c.eval(sysctlRule("b", "net.core.rmem_max", "1048576", "gte")))
	if got.Status != StatusRecommend || got.Current != "212992" {
		t.Errorf("fallback: %+v", got)
	}
}

func TestApplies(t *testing.T) {
	c := newTC(t)
	r := sysctlRule("a", "net.core.somaxconn", "4096", "gte")
	r.Applies.Kernel = ">=6.9"
	if got := one(t, c.eval(r)); got.Status != StatusNotApplicable {
		t.Errorf("kernel: %s", got.Status)
	}
	r.Applies.Kernel = ">=4.9 <6.18"
	if got := one(t, c.eval(r)); got.Status != StatusRecommend {
		t.Errorf("kernel ok: %s", got.Status)
	}
	r.Applies.When = "workload.Listeners > 0"
	if got := one(t, c.eval(r)); got.Status != StatusNotApplicable {
		t.Errorf("when: %s", got.Status)
	}
	c.w.Listeners = 2
	if got := one(t, c.eval(r)); got.Status != StatusRecommend {
		t.Errorf("when true: %s", got.Status)
	}
}

func TestEvidenceScoreAndSort(t *testing.T) {
	c := newTC(t)
	c.rates = map[string]float64{"TcpExt.ListenOverflows": 3}
	c.vals = map[string]float64{"conntrack.count": 90, "conntrack.max": 100, "x.gauge": 5}
	hot := sysctlRule("hot", "net.core.somaxconn", "4096", "gte")
	hot.Evidence = []rules.Evidence{
		{Metric: "TcpExt.ListenOverflows", RateGT: fp(0)},
		{Metric: "conntrack.count", RatioGT: fp(0.8), Of: "conntrack.max"},
		{Metric: "x.gauge", ValueGT: fp(10)}, // no hit
	}
	cold := sysctlRule("cold", "vm.swappiness", "1", "eq")
	got := c.eval(cold, hot)
	if len(got) != 2 || got[0].RuleID != "hot" {
		t.Fatalf("order: %+v", got)
	}
	if len(got[0].Evidence) != 2 || got[0].Score != 1*(1+2*2) {
		t.Errorf("evidence=%v score=%v", got[0].Evidence, got[0].Score)
	}
	if got[0].Evidence[1].Value != 0.9 {
		t.Errorf("ratio = %v", got[0].Evidence[1].Value)
	}
	if got[1].Score != 1 {
		t.Errorf("cold score = %v", got[1].Score)
	}
	hot.Risk = "high"
	if s := c.eval(hot)[0].Score; math.Abs(s-0.4*5) > 1e-9 {
		t.Errorf("high-risk score = %v", s)
	}
}

func TestAntipatternAndReport(t *testing.T) {
	c := newTC(t)
	ap := rules.Rule{ID: "ap", Kind: "antipattern", Detect: `sysctlNum("vm.swappiness") < 20`, Domain: "memory", Risk: "medium", Title: "t",
		Advice: "swappiness check on {{.facts.Kernel.Release}}"}
	got := one(t, c.eval(ap))
	if got.Status != StatusFiring || math.Abs(got.Score-1.05) > 1e-9 {
		t.Errorf("antipattern: %+v", got)
	}
	if !strings.Contains(got.Advice, "6.8.0") {
		t.Errorf("advice not templated: %q", got.Advice)
	}
	rep := ap
	rep.Kind, rep.Detect = "report", "false"
	if got := one(t, c.eval(rep)); got.Status != StatusOK {
		t.Errorf("report: %s", got.Status)
	}
}

func TestSysfsForeachBlockAndFilter(t *testing.T) {
	c := newTC(t)
	c.f.Block = []facts.BlockDev{{Name: "nvme0n1", Transport: "nvme"}, {Name: "sda", Rotational: true}}
	for name, v := range map[string]string{"nvme0n1": "[none] mq-deadline\n", "sda": "mq-deadline kyber [bfq]\n"} {
		p := filepath.Join(c.root.Sys, "block", name, "queue", "scheduler")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := rules.Rule{ID: "sched", Kind: "sysfs", Param: "/sys/block/{item}/queue/scheduler", Foreach: "block",
		Filter: `item.Transport == "nvme" || item.Rotational`, Recommend: `item.Rotational ? "mq-deadline" : "none"`,
		Compare: "eq", Domain: "storage", Risk: "low", Title: "t", Namespaced: true}
	got := c.eval(r)
	if len(got) != 2 {
		t.Fatalf("recs = %+v", got)
	}
	by := map[string]Recommendation{}
	for _, g := range got {
		by[g.Target] = g
	}
	if by["nvme0n1"].Status != StatusOK || by["nvme0n1"].Current != "none" {
		t.Errorf("nvme: %+v", by["nvme0n1"])
	}
	if by["sda"].Status != StatusRecommend || by["sda"].Current != "bfq" {
		t.Errorf("sda: %+v", by["sda"])
	}
	r.Filter = "false"
	if got := one(t, c.eval(r)); got.Status != StatusNotApplicable {
		t.Errorf("empty foreach: %s", got.Status)
	}
	r.Filter = ""
	r.Param = "/sys/block/{item}/../../../etc/passwd"
	for _, g := range c.eval(r) {
		if g.Status != StatusNotApplicable && g.Status != StatusError {
			t.Errorf("path traversal not contained: %+v", g)
		}
	}
}

func TestCmdline(t *testing.T) {
	c := newTC(t)
	mk := func(key, rec string) rules.Rule {
		return rules.Rule{ID: "c", Kind: "cmdline", Param: key, Recommend: rec, Compare: "eq", Reboot: true, Domain: "ai", Risk: "medium", Title: "t", Namespaced: true}
	}
	if got := one(t, c.eval(mk("iommu", `"pt"`))); got.Status != StatusOK {
		t.Errorf("value ok: %+v", got)
	}
	if got := one(t, c.eval(mk("iommu", `"on"`))); got.Status != StatusRecommend || !got.Reboot {
		t.Errorf("value differs: %+v", got)
	}
	if got := one(t, c.eval(mk("quiet", ""))); got.Status != StatusOK {
		t.Errorf("flag present: %+v", got)
	}
	if got := one(t, c.eval(mk("nohz_full", ""))); got.Status != StatusRecommend {
		t.Errorf("flag absent: %+v", got)
	}
}

func TestLimit(t *testing.T) {
	c := newTC(t)
	c.w.Apps = []workload.App{
		{Name: "nginx", Unit: "nginx.service", NoFileSoft: 1024, MemlockSoft: math.MaxUint64},
		{Name: "redis", NoFileSoft: 100000},
	}
	r := rules.Rule{ID: "l", Kind: "limit", Param: "LimitNOFILE", Foreach: "app", Recommend: "65536", Compare: "gte", Domain: "limits", Risk: "low", Title: "t", Namespaced: true}
	got := c.eval(r)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	by := map[string]Recommendation{}
	for _, g := range got {
		by[g.Target] = g
	}
	if by["nginx.service"].Status != StatusRecommend || by["nginx.service"].Current != "1024" {
		t.Errorf("nginx: %+v", by["nginx.service"])
	}
	if by["redis"].Status != StatusOK {
		t.Errorf("redis: %+v", by["redis"])
	}
	r.Param, r.Recommend = "LimitMEMLOCK", `"infinity"`
	g := c.eval(r)
	for _, x := range g {
		if x.Target == "nginx.service" && (x.Status != StatusOK || x.Current != "unlimited") {
			t.Errorf("memlock: %+v", x)
		}
		if x.Target == "redis" && (x.Status != StatusRecommend || x.Proposed != "infinity") {
			t.Errorf("redis memlock: %+v", x)
		}
	}
}

func TestServiceAndNIC(t *testing.T) {
	c := newTC(t)
	svc := rules.Rule{ID: "s", Kind: "service", Param: "irqbalance.service", Recommend: `"stopped"`, Compare: "eq", Domain: "cpu", Risk: "low", Title: "t"}
	if got := one(t, c.eval(svc)); got.Status != StatusRecommend || got.Current != "running" {
		t.Errorf("service: %+v", got)
	}
	c.f.NICs = []facts.NIC{{Name: "eth0", Combined: 4, CombinedMax: 32, RxRing: 256, RxRingMax: 4096}, {Name: "lo"}}
	nic := rules.Rule{ID: "n", Kind: "nic", Param: "rx_ring", Foreach: "nic", Recommend: "item.RxRingMax", Compare: "gte", Domain: "network", Risk: "low", Title: "t", Namespaced: true}
	got := c.eval(nic)
	by := map[string]Recommendation{}
	for _, g := range got {
		by[g.Target] = g
	}
	if by["eth0"].Status != StatusRecommend || by["eth0"].Proposed != "4096" || by["eth0"].Current != "256" {
		t.Errorf("eth0: %+v", by["eth0"])
	}
	if by["lo"].Status != StatusNotApplicable {
		t.Errorf("lo: %+v", by["lo"])
	}
}

func TestRuntime(t *testing.T) {
	c := newTC(t)
	c.w.Apps = []workload.App{{Name: "vllm"}}
	r := rules.Rule{ID: "rt", Kind: "runtime", Param: "vllm", Foreach: "app", Filter: `item.Name == "vllm"`, Advice: "Set --max-num-seqs for {{.item.Name}}", Domain: "ai", Risk: "low", Title: "t"}
	got := one(t, c.eval(r))
	if got.Status != StatusRecommend || got.Advice != "Set --max-num-seqs for vllm" || got.Target != "vllm" {
		t.Errorf("%+v", got)
	}
}

func TestManagedAndHostOnly(t *testing.T) {
	c := newTC(t)
	c.eco = rules.Ecosystem{
		Tuned:   map[string]rules.TunedProfile{"throughput-performance": {Sysctl: map[string]string{"vm.swappiness": "10", "net.core.somaxconn": "2048"}}},
		Bpftune: []string{"net.ipv4.tcp_rmem"},
	}
	c.f.Tuned = facts.Tuned{Active: true, Profile: "throughput-performance"}
	got := one(t, c.eval(sysctlRule("m", "net.core.somaxconn", "4096", "gte")))
	if got.Status != StatusManaged || got.ManagedBy != "tuned:throughput-performance" || !strings.Contains(got.Advice, "2048") {
		t.Errorf("tuned: %+v", got)
	}
	// Already satisfied: ok wins over managed.
	if got := one(t, c.eval(sysctlRule("m", "vm.swappiness", "10", "eq"))); got.Status != StatusOK {
		t.Errorf("ok-managed: %+v", got)
	}
	c.f.Tuned = facts.Tuned{}
	if got := one(t, c.eval(sysctlRule("m", "net.ipv4.tcp_rmem", "[4096, 131072, 16777216]", "gte"))); got.Status != StatusRecommend {
		t.Errorf("bpftune not running: %+v", got)
	}
	c.f.Bpftune = true
	got = one(t, c.eval(sysctlRule("m", "net.ipv4.tcp_rmem", "[4096, 131072, 16777216]", "gte")))
	if got.Status != StatusManaged || got.ManagedBy != "bpftune" || !strings.Contains(got.Advice, "disables the bpftune") {
		t.Errorf("bpftune: %+v", got)
	}
	// Container: host-only unless namespaced.
	c.f.Bpftune = false
	c.f.Host.Container = true
	r := sysctlRule("h", "vm.swappiness", "1", "eq")
	r.Namespaced = false
	if got := one(t, c.eval(r)); got.Status != StatusHostOnly {
		t.Errorf("host-only: %+v", got)
	}
	r.Namespaced = true
	if got := one(t, c.eval(r)); got.Status != StatusRecommend {
		t.Errorf("namespaced: %+v", got)
	}
}

func TestErrorsNeverPanic(t *testing.T) {
	c := newTC(t)
	bad := []rules.Rule{
		sysctlRule("e1", "vm.swappiness", "nosuchfn(1)", "eq"),
		sysctlRule("e2", "vm.swappiness", "1 / 0 + item.X.Y", "eq"),
		{ID: "e3", Kind: "antipattern", Detect: "1 + 1", Domain: "cpu", Risk: "low", Title: "t"},
		{ID: "e4", Kind: "sysctl", Param: "vm.swappiness", Recommend: "1", Applies: rules.Applies{Kernel: "garbage"}, Domain: "cpu", Risk: "low", Title: "t"},
		{ID: "e5", Kind: "limit", Param: "LimitNOFILE", Recommend: "1", Domain: "cpu", Risk: "low", Title: "t"},
	}
	for _, r := range bad {
		got := one(t, c.eval(r))
		if got.Status != StatusError || got.Advice == "" {
			t.Errorf("%s: %+v", r.ID, got)
		}
	}
	// Nil facts and workload must not panic either.
	_ = Evaluate(bad[:1], Input{})
}
