package advisor

import (
	"math"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/rules"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

func TestLimitNPROC(t *testing.T) {
	c := newTC(t)
	c.w.Apps = []workload.App{
		{Name: "elasticsearch", Unit: "elasticsearch.service", NProcSoft: 4096},
		{Name: "cassandra", Unit: "cassandra.service", NProcSoft: 65536},
		{Name: "opensearch", Unit: "opensearch.service", NProcSoft: math.MaxUint64},
	}
	r := rules.Rule{ID: "p", Kind: "limit", Param: "LimitNPROC", Foreach: "app", Recommend: "max(4096, 65536)", Compare: "gte", Domain: "limits", Risk: "low", Title: "t", Namespaced: true}
	by := map[string]Recommendation{}
	for _, g := range c.eval(r) {
		by[g.Target] = g
	}
	if g := by["elasticsearch.service"]; g.Status != StatusRecommend || g.Current != "4096" || g.Proposed != "65536" {
		t.Errorf("elasticsearch: %+v", g)
	}
	if g := by["cassandra.service"]; g.Status != StatusOK {
		t.Errorf("cassandra: %+v", g)
	}
	if g := by["opensearch.service"]; g.Status != StatusOK || g.Current != "unlimited" {
		t.Errorf("opensearch: %+v", g)
	}
}

func TestSysctlHelperFallbackAndFactsInRules(t *testing.T) {
	c := newTC(t)
	writeSys(t, c.root.Proc, "sys/user/max_user_namespaces", "15000\n")
	c.f.Mounts = []facts.Mount{{Device: "/dev/sda1", MountPoint: "/", FSType: "ext4", Options: []string{"rw"}, Disk: "sda"}}
	c.f.Block = []facts.BlockDev{{Name: "sda", ReadAheadKB: 128}}
	c.f.Kernel.Config = map[string]string{"CONFIG_BPF_JIT": "y"}
	c.w.Apps = []workload.App{{Name: "postgres", OOMScoreAdj: -900}}
	r := rules.Rule{ID: "a", Kind: "antipattern", Domain: "memory", Risk: "low", Title: "t",
		Detect: `sysctlNum("user.max_user_namespaces") == 15000 && fsType("/var/lib/pg") == "ext4" && ` +
			`kconfig("CONFIG_BPF_JIT") == "y" && blockOf("/var/lib/pg") == "sda" && ` +
			`workload.Apps[0].OOMScoreAdj < -500 && !mountHasOpt("/", "noatime")`}
	if got := one(t, c.eval(r)); got.Status != StatusFiring {
		t.Errorf("detect must fire: %+v", got)
	}
}

func TestConflictsEqDisagreeAndBlock(t *testing.T) {
	recs := []Recommendation{
		mk("app.postgres.overcommit_memory", "sysctl", "vm.overcommit_memory", "", "2", "eq", 2),
		mk("app.redis.overcommit_memory", "sysctl", "vm.overcommit_memory", "", "1", "eq", 1),
		mk("other", "sysctl", "vm.swappiness", "", "10", "lte", 1),
	}
	cs := Conflicts(recs)
	if len(cs) != 1 || cs[0].Resolved || !cs[0].Genuine() || cs[0].Winner != "app.postgres.overcommit_memory" {
		t.Fatalf("conflicts = %+v", cs)
	}
	b := ConflictBlock(cs)
	for _, want := range []string{"CONFLICT: 1 setting has rules that disagree", "vm.overcommit_memory",
		"app.postgres.overcommit_memory proposes 2 (chosen)", "app.redis.overcommit_memory proposes 1"} {
		if !strings.Contains(b, want) {
			t.Errorf("block missing %q:\n%s", want, b)
		}
	}
}

func TestConflictsRecommendVersusSatisfiedRule(t *testing.T) {
	ok := mk("app.redis.overcommit_memory", "sysctl", "vm.overcommit_memory", "", "1", "eq", 0)
	ok.Status = StatusOK
	recs := []Recommendation{mk("app.postgres.overcommit_memory", "sysctl", "vm.overcommit_memory", "", "2", "eq", 2), ok}
	cs := Conflicts(recs)
	if len(cs) != 1 || !cs[0].Genuine() || len(cs[0].Kept) != 1 || cs[0].Kept[0] != "app.redis.overcommit_memory=1" {
		t.Fatalf("conflicts = %+v", cs)
	}
	if b := ConflictBlock(cs); !strings.Contains(b, "app.redis.overcommit_memory wants 1 is already satisfied and would be broken") {
		t.Errorf("block:\n%s", b)
	}
	// Reconcile itself is unchanged: one recommendation, no merge conflict.
	if out, rc := Reconcile(recs); len(rc) != 0 || len(out) != 2 {
		t.Errorf("Reconcile changed: %+v %+v", out, rc)
	}
	// Agreeing values, or a gte ok rule, are not conflicts.
	ok.Proposed = "2"
	if cs := Conflicts([]Recommendation{recs[0], ok}); len(cs) != 0 {
		t.Errorf("agreeing: %+v", cs)
	}
	ok.Proposed, ok.Compare = "1", "gte"
	if cs := Conflicts([]Recommendation{recs[0], ok}); len(cs) != 0 {
		t.Errorf("gte ok rule: %+v", cs)
	}
}

func TestConflictBlockSkipsResolved(t *testing.T) {
	recs := []Recommendation{
		mk("a", "sysctl", "vm.max_map_count", "", "262144", "gte", 1),
		mk("b", "sysctl", "vm.max_map_count", "", "1048576", "gte", 1),
	}
	cs := Conflicts(recs)
	if len(cs) != 1 || !cs[0].Resolved {
		t.Fatalf("conflicts = %+v", cs)
	}
	if b := ConflictBlock(cs); b != "" {
		t.Errorf("resolved merges must not shout: %q", b)
	}
}
