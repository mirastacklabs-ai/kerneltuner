package apply

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func TestApplyExpandsTargetsAndMergesDuplicates(t *testing.T) {
	root := facts.RootAt(t.TempDir())
	write(t, filepath.Join(root.Proc, "sys/vm/max_map_count"), "65530\n")
	for _, c := range []string{"cpu0", "cpu1", "cpu2"} {
		write(t, filepath.Join(root.Sys, "devices/system/cpu", c, "cpufreq/scaling_governor"), "powersave\n")
	}
	gov := rec("sysfs", "/sys/devices/system/cpu/{item}/cpufreq/scaling_governor", "cpu0-2 (3)", "performance")
	gov.Targets = []string{"cpu0", "cpu1", "cpu2", "cpu9"} // cpu9 is offline: no cpufreq dir
	a := rec("sysctl", "vm.max_map_count", "", "262144")
	a.RuleID, a.Compare = "app.es", "gte"
	b := rec("sysctl", "vm.max_map_count", "", "1048576")
	b.RuleID, b.Compare = "app.cassandra", "gte"

	p, err := BuildPlan([]advisor.Recommendation{gov, a, b}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ops) != 4 { // 3 cpus + 1 merged sysctl
		t.Fatalf("ops = %d: %+v", len(p.Ops), p.Ops)
	}
	var merged *Op
	for i := range p.Ops {
		if p.Ops[i].Kind == "sysctl" {
			merged = &p.Ops[i]
		}
	}
	if merged == nil || merged.Proposed != "1048576" || len(merged.Rules) != 2 {
		t.Errorf("merged op: %+v", merged)
	}
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "vm.max_map_count") {
		t.Errorf("warnings: %v", p.Warnings)
	}
	if _, err := Apply(p, Options{Now: fixed(), Version: "t"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"cpu0", "cpu1", "cpu2"} {
		if got := read(t, filepath.Join(root.Sys, "devices/system/cpu", c, "cpufreq/scaling_governor")); strings.TrimSpace(got) != "performance" {
			t.Errorf("%s governor = %q", c, got)
		}
	}
	if got := read(t, filepath.Join(root.Proc, "sys/vm/max_map_count")); strings.TrimSpace(got) != "1048576" {
		t.Errorf("max_map_count = %q", got)
	}

	// a Targets rule with no existing path is skipped
	none := rec("sysfs", "/sys/devices/system/cpu/{item}/cpufreq/scaling_governor", "cpu8-9 (2)", "performance")
	none.Targets = []string{"cpu8", "cpu9"}
	p, err = BuildPlan([]advisor.Recommendation{none}, root)
	if err != nil || len(p.Ops) != 0 || len(p.Skipped) != 1 {
		t.Errorf("skip: %+v %v", p, err)
	}
}
