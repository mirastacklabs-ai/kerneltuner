package advisor

import "testing"

func TestReconcileMixedGteLteRespectsBounds(t *testing.T) {
	// gte floor 100 and lte ceiling 500: the chosen value must lie inside.
	out, c := Reconcile([]Recommendation{
		mk("lo", "sysctl", "vm.x", "", "100", "gte", 0.5),
		mk("hi", "sysctl", "vm.x", "", "500", "lte", 1),
	})
	if len(out) != 1 || len(c) != 1 || !c[0].Resolved || out[0].Proposed != "100" {
		t.Fatalf("compatible mixed group: %+v %+v", out, c)
	}
	// Incompatible: floor above ceiling is a genuine conflict, never a value
	// below the floor or above the ceiling silently "resolved".
	_, c = Reconcile([]Recommendation{
		mk("lo", "sysctl", "vm.x", "", "1000", "gte", 0.5),
		mk("hi", "sysctl", "vm.x", "", "500", "lte", 1),
	})
	if len(c) != 1 || c[0].Resolved || !c[0].Genuine() {
		t.Fatalf("incompatible mixed group must be a genuine conflict: %+v", c)
	}
	// eq value below the gte floor conflicts.
	_, c = Reconcile([]Recommendation{
		mk("eq", "sysctl", "vm.x", "", "50", "eq", 1),
		mk("lo", "sysctl", "vm.x", "", "100", "gte", 1),
	})
	if len(c) != 1 || c[0].Resolved {
		t.Fatalf("eq below gte floor must conflict: %+v", c)
	}
}

func TestConflictsKeptIncludesGteLteOkRules(t *testing.T) {
	ok := func(id, cmp, val string) Recommendation {
		return Recommendation{RuleID: id, Kind: "sysctl", Param: "vm.x", Proposed: val, Compare: cmp, Status: StatusOK}
	}
	rec := mk("want", "sysctl", "vm.x", "", "10", "eq", 1)
	cs := Conflicts([]Recommendation{rec, ok("floor", "gte", "100")})
	if len(cs) != 1 || len(cs[0].Kept) != 1 || cs[0].Kept[0] != "floor=100" {
		t.Fatalf("gte ok rule broken by 10 must be kept: %+v", cs)
	}
	cs = Conflicts([]Recommendation{rec, ok("ceil", "lte", "5")})
	if len(cs) != 1 || len(cs[0].Kept) != 1 {
		t.Fatalf("lte ok rule broken by 10: %+v", cs)
	}
	if cs := Conflicts([]Recommendation{rec, ok("floor", "gte", "5")}); len(cs) != 0 {
		t.Fatalf("satisfied gte must not conflict: %+v", cs)
	}
}

func TestReconcileFoldsSysctlSpellings(t *testing.T) {
	out, c := Reconcile([]Recommendation{
		mk("a", "sysctl", "net.core.somaxconn", "", "1024", "gte", 1),
		mk("b", "sysctl", "net/core/somaxconn", "", "4096", "gte", 1),
	})
	if len(out) != 1 || len(c) != 1 || out[0].Proposed != "4096" {
		t.Fatalf("dot and slash spellings must merge: %+v %+v", out, c)
	}
}
