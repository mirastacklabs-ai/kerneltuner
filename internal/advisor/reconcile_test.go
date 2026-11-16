package advisor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSys(t *testing.T, base, rel, content string) {
	t.Helper()
	p := filepath.Join(base, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mk(id, kind, param, target, prop, cmp string, score float64) Recommendation {
	return Recommendation{RuleID: id, Kind: kind, Param: param, Target: target, Proposed: prop, Compare: cmp,
		Status: StatusRecommend, Score: score, Sources: []string{"https://x/" + id}}
}

func TestReconcileGTEMax(t *testing.T) {
	recs := []Recommendation{
		mk("es", "sysctl", "vm.max_map_count", "", "262144", "gte", 1),
		mk("other", "sysctl", "vm.swappiness", "", "10", "lte", 1),
		mk("cass", "sysctl", "vm.max_map_count", "", "1048576", "gte", 0.7),
	}
	out, conflicts := Reconcile(recs)
	if len(out) != 2 || len(conflicts) != 1 {
		t.Fatalf("out=%d conflicts=%d: %+v", len(out), len(conflicts), out)
	}
	m := out[0]
	if m.Param != "vm.max_map_count" || m.Proposed != "1048576" || strings.Join(m.Contributors, ",") != "es,cass" {
		t.Errorf("merged: %+v", m)
	}
	if !conflicts[0].Resolved || conflicts[0].Method != "max" {
		t.Errorf("conflict: %+v", conflicts[0])
	}
	if len(m.Sources) != 2 {
		t.Errorf("sources not unioned: %v", m.Sources)
	}
	if len(recs) != 3 || recs[0].Proposed != "262144" || recs[0].Contributors != nil {
		t.Errorf("input mutated: %+v", recs)
	}
}

func TestReconcileLTEMinAndExactInts(t *testing.T) {
	out, c := Reconcile([]Recommendation{
		mk("a", "sysctl", "vm.swappiness", "", "10", "lte", 1),
		mk("b", "sysctl", "vm.swappiness", "", "1", "lte", 1),
	})
	if len(out) != 1 || out[0].Proposed != "1" || !c[0].Resolved || c[0].Method != "min" {
		t.Errorf("lte: %+v %+v", out, c)
	}
	out, _ = Reconcile([]Recommendation{
		mk("a", "sysctl", "kernel.shmmax", "", "9223372036854775806", "gte", 1),
		mk("b", "sysctl", "kernel.shmmax", "", "9223372036854775807", "gte", 1),
	})
	if out[0].Proposed != "9223372036854775807" {
		t.Errorf("exact max: %q", out[0].Proposed)
	}
}

func TestReconcileMultiFieldAndLimits(t *testing.T) {
	out, _ := Reconcile([]Recommendation{
		mk("a", "sysctl", "net.ipv4.tcp_rmem", "", "4096 131072 6291456", "gte", 1),
		mk("b", "sysctl", "net.ipv4.tcp_rmem", "", "4096 87380 16777216", "gte", 1),
	})
	if out[0].Proposed != "4096 131072 16777216" {
		t.Errorf("per-field max: %q", out[0].Proposed)
	}
	out, _ = Reconcile([]Recommendation{
		mk("ipc", "limit", "LimitMEMLOCK", "vllm.service", "infinity", "gte", 1),
		mk("rdma", "limit", "LIMITMEMLOCK", "vllm.service", "67108864", "gte", 1),
		mk("nofile", "limit", "LimitNOFILE", "vllm.service", "1048576", "gte", 1),
	})
	if len(out) != 2 || out[0].Proposed != "infinity" || len(out[0].Contributors) != 2 {
		t.Errorf("limits: %+v", out)
	}
}

func TestReconcileEqConflictPicksHighestScore(t *testing.T) {
	out, c := Reconcile([]Recommendation{
		mk("low", "sysctl", "net.ipv4.tcp_congestion_control", "", "cubic", "eq", 0.7),
		mk("high", "sysctl", "net.ipv4.tcp_congestion_control", "", "bbr", "eq", 2.1),
	})
	if len(out) != 1 || out[0].Proposed != "bbr" || out[0].RuleID != "high" {
		t.Errorf("out: %+v", out)
	}
	if len(c) != 1 || c[0].Resolved || c[0].Winner != "high" {
		t.Errorf("conflict: %+v", c)
	}
	if !strings.HasPrefix(out[0].Note, "CONFLICT") || !strings.Contains(c[0].String(), "rules disagree") {
		t.Errorf("note %q / %q", out[0].Note, c[0])
	}
	// mixed gte and eq on numbers is also a conflict, not a merge
	_, c = Reconcile([]Recommendation{
		mk("a", "sysctl", "vm.swappiness", "", "10", "gte", 1),
		mk("b", "sysctl", "vm.swappiness", "", "20", "eq", 1),
	})
	if len(c) != 1 || c[0].Resolved {
		t.Errorf("mixed modes: %+v", c)
	}
}

func TestReconcileAgreeLeavesNoConflictAndIgnoresOthers(t *testing.T) {
	ok := mk("ok", "sysctl", "vm.max_map_count", "", "1", "gte", 0)
	ok.Status = StatusOK
	recs := []Recommendation{
		mk("a", "sysctl", "vm.max_map_count", "", "262144", "gte", 1),
		mk("b", "sysctl", "vm.max_map_count", "", "262144", "gte", 1),
		ok,
		mk("t1", "sysctl", "vm.max_map_count", "t", "5", "gte", 1), // different target
		mk("rt", "runtime", "free text", "", "", "", 1),
		mk("rt2", "runtime", "free text", "", "", "", 1),
	}
	out, c := Reconcile(recs)
	if len(c) != 0 {
		t.Errorf("conflicts: %+v", c)
	}
	if len(out) != 5 {
		t.Fatalf("len = %d: %+v", len(out), out)
	}
	if got := strings.Join(out[0].Contributors, ","); got != "a,b" {
		t.Errorf("contributors %q", got)
	}
	// no group of two: input slice returned untouched
	one := recs[3:4]
	if o, c := Reconcile(one); len(o) != 1 || c != nil {
		t.Errorf("single: %+v %v", o, c)
	}
}

func TestReconcileKeepsSummarizedTargets(t *testing.T) {
	a := mk("a", "sysfs", "/sys/devices/system/cpu/{item}/cpufreq/scaling_governor", "cpu0-1 (2)", "performance", "eq", 1)
	a.Targets = []string{"cpu0", "cpu1"}
	b := a
	b.RuleID = "b"
	out, c := Reconcile([]Recommendation{a, b})
	if len(out) != 1 || len(out[0].Targets) != 2 || len(c) != 0 {
		t.Errorf("%+v %v", out, c)
	}
}
