package rules

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadSeedPacks(t *testing.T) {
	rs, eco, srcs, err := Load(os.DirFS("testdata/packs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || len(srcs) != 1 {
		t.Fatalf("rules=%d sources=%d", len(rs), len(srcs))
	}
	packs := map[string]string{}
	for _, r := range rs {
		packs[r.ID] = r.Pack
	}
	if packs["net.core.somaxconn"] != "kernel/network" || packs["anti.swap-on-nvme"] != "antipatterns" {
		t.Errorf("packs = %v", packs)
	}
	nt := eco.Tuned["network-throughput"].Sysctl
	if nt["vm.swappiness"] != "10" || nt["net.core.somaxconn"] != "4096" || nt["net.core.rmem_max"] != "16777216" {
		t.Errorf("include chain not resolved: %v", nt)
	}
	if eco.Tuned["throughput-performance"].Sysctl["net.core.somaxconn"] != "2048" {
		t.Error("parent mutated by child")
	}
	if eco.Tuned["accelerator-performance"].Sysctl["kernel.numa_balancing"] != "0" {
		t.Error("unknown include should end the chain")
	}
	if len(eco.Bpftune) != 2 {
		t.Errorf("bpftune = %v", eco.Bpftune)
	}
	if errs := ValidateWith(rs, srcs, "testdata/fixtures"); len(errs) != 0 {
		t.Errorf("seed packs invalid: %v", errs)
	}
}

func TestLoadEmptyAndMissing(t *testing.T) {
	rs, eco, srcs, err := Load(fstest.MapFS{
		"kernel/network/.gitkeep":   {},
		"kernel/network/empty.yaml": {Data: []byte("")},
		"ecosystem/tuned.yaml":      {Data: []byte("")},
		"ecosystem/bpftune.yaml":    {Data: []byte("sysctls: []\n")},
	})
	if err != nil || len(rs) != 0 || len(srcs) != 0 || len(eco.Tuned) != 0 || len(eco.Bpftune) != 0 {
		t.Fatalf("%v %v %v %v %v", rs, eco, srcs, err, len(srcs))
	}
	if _, _, _, err := Load(fstest.MapFS{}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStrict(t *testing.T) {
	for name, fsys := range map[string]fstest.MapFS{
		"unknown rule field":    {"kernel/x/a.yaml": {Data: []byte("- id: a\n  bogus: 1\n")}},
		"unknown source field":  {"kernel/x/sources.yaml": {Data: []byte("- url: u\n  bogus: 1\n")}},
		"unknown tuned field":   {"ecosystem/tuned.yaml": {Data: []byte("profiles:\n  a:\n    nope: 1\n")}},
		"unknown bpftune field": {"ecosystem/bpftune.yaml": {Data: []byte("nope: []\n")}},
		"include cycle":         {"ecosystem/tuned.yaml": {Data: []byte("profiles:\n  a: {include: b}\n  b: {include: a}\n")}},
		"bad yaml":              {"kernel/x/a.yaml": {Data: []byte("- id: [\n")}},
	} {
		if _, _, _, err := Load(fsys); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestValidateReportsAllProblems(t *testing.T) {
	good, _, srcs, err := Load(os.DirFS("testdata/packs"))
	if err != nil {
		t.Fatal(err)
	}
	mut := func(f func(r *Rule)) []Rule {
		r := good[0]
		r.Evidence = append([]Evidence(nil), r.Evidence...)
		r.Tests = append([]RuleTest(nil), r.Tests...)
		f(&r)
		return []Rule{r}
	}
	cases := []struct {
		name string
		rs   []Rule
		want string
	}{
		{"bad id", mut(func(r *Rule) { r.ID = "Bad ID" }), "id"},
		{"bad kind", mut(func(r *Rule) { r.Kind = "magic" }), "kind"},
		{"bad domain", mut(func(r *Rule) { r.Domain = "x" }), "domain"},
		{"bad risk", mut(func(r *Rule) { r.Risk = "x" }), "risk"},
		{"bad compare", mut(func(r *Rule) { r.Compare = "x" }), "compare"},
		{"no recommend", mut(func(r *Rule) { r.Recommend = "" }), "recommend is required"},
		{"no param", mut(func(r *Rule) { r.Param = "" }), "param is required"},
		{"bad expr", mut(func(r *Rule) { r.Recommend = "1 +" }), "recommend does not compile"},
		{"unknown fn", mut(func(r *Rule) { r.Applies.When = "nope()" }), "applies.when"},
		{"bad kernel", mut(func(r *Rule) { r.Applies.Kernel = "5" }), "applies.kernel"},
		{"unregistered source", mut(func(r *Rule) { r.Sources = []string{"https://nope"} }), "not registered"},
		{"no source", mut(func(r *Rule) { r.Sources = nil }), "source"},
		{"no tests", mut(func(r *Rule) { r.Tests = nil }), "golden test"},
		{"missing fixture", mut(func(r *Rule) { r.Tests[0].Fixture = "nope" }), "nope"},
		{"bad expect status", mut(func(r *Rule) { r.Tests[0].Expect.Status = "great" }), "expect.status"},
		{"cmdline w/o reboot", mut(func(r *Rule) { r.Kind, r.Param, r.Reboot = "cmdline", "iommu", false }), "reboot"},
		{"antipattern w/o detect", mut(func(r *Rule) { r.Kind, r.Detect = "antipattern", "" }), "detect is required"},
		{"report w/o detect", mut(func(r *Rule) { r.Kind, r.Detect = "report", "" }), "detect is required"},
		{"runtime w/o advice", mut(func(r *Rule) { r.Kind, r.Advice = "runtime", "" }), "advice is required"},
		{"limit w/o foreach", mut(func(r *Rule) { r.Kind, r.Param = "limit", "LimitNOFILE" }), "foreach: app"},
		{"limit bad param", mut(func(r *Rule) { r.Kind, r.Param, r.Foreach = "limit", "nofile", "app" }), "limit param"},
		{"nic bad param", mut(func(r *Rule) { r.Kind, r.Param, r.Foreach = "nic", "mtu", "nic" }), "nic param"},
		{"sysfs bad path", mut(func(r *Rule) { r.Kind, r.Param = "sysfs", "/proc/x" }), "sysfs param"},
		{"filter w/o foreach", mut(func(r *Rule) { r.Filter = "true" }), "filter requires foreach"},
		{"bad evidence", mut(func(r *Rule) { r.Evidence = []Evidence{{Metric: "m"}} }), "evidence"},
		{"bad template", mut(func(r *Rule) { r.Advice = "{{.x" }), "advice template"},
		{"duplicate id", []Rule{good[0], good[0]}, "duplicate id"},
	}
	for _, c := range cases {
		errs := ValidateWith(c.rs, srcs, "testdata/fixtures")
		if len(errs) == 0 {
			t.Errorf("%s: no problems reported", c.name)
			continue
		}
		var joined []string
		for _, e := range errs {
			joined = append(joined, e.Error())
		}
		if !strings.Contains(strings.Join(joined, "\n"), c.want) {
			t.Errorf("%s: want %q in %v", c.name, c.want, joined)
		}
	}
	// Several problems at once are all returned.
	multi := mut(func(r *Rule) { r.Kind, r.Domain, r.Risk, r.Sources, r.Tests = "x", "y", "z", nil, nil })
	if errs := ValidateWith(multi, srcs, ""); len(errs) < 5 {
		t.Errorf("want >=5 problems, got %v", errs)
	}
}
