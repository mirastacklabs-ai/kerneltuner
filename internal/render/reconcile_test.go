package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
)

func dupRecs() []advisor.Recommendation {
	mk := func(id, param, target, cur, prop, cmp string, score float64) advisor.Recommendation {
		kind := "sysctl"
		if strings.HasPrefix(param, "Limit") {
			kind = "limit"
		}
		return advisor.Recommendation{RuleID: id, Kind: kind, Domain: "memory", Param: param, Target: target, Title: "T " + id,
			Current: cur, Proposed: prop, Compare: cmp, Status: advisor.StatusRecommend, Risk: "low", Score: score,
			Sources: []string{"https://example.org/" + id}}
	}
	return []advisor.Recommendation{
		mk("app.elasticsearch.map", "vm.max_map_count", "", "65530", "262144", "gte", 1),
		mk("app.cassandra.map", "vm.max_map_count", "", "65530", "1048576", "gte", 1),
		mk("app.vllm.ipc-lock", "LimitMEMLOCK", "vllm.service", "8388608", "infinity", "gte", 1),
		mk("ai.rdma.memlock", "LimitMEMLOCK", "vllm.service", "8388608", "134217728", "gte", 1),
		mk("a.cc", "net.ipv4.tcp_congestion_control", "", "cubic", "bbr", "eq", 2),
		mk("b.cc", "net.ipv4.tcp_congestion_control", "", "cubic", "reno", "eq", 1),
	}
}

func TestGenerateMergesDuplicates(t *testing.T) {
	recs := dupRecs()
	files, err := Generate("all", recs, testMeta())
	if err != nil {
		t.Fatal(err)
	}
	sysctl := files[FileSysctl]
	if strings.Count(sysctl, "vm.max_map_count =") != 1 || !strings.Contains(sysctl, "vm.max_map_count = 1048576") {
		t.Errorf("max_map_count not merged to the max:\n%s", sysctl)
	}
	if !strings.Contains(sysctl, "app.elasticsearch.map, app.cassandra.map") && !strings.Contains(sysctl, "app.cassandra.map, app.elasticsearch.map") {
		t.Errorf("comment lacks contributing ids:\n%s", sysctl)
	}
	if strings.Count(sysctl, "tcp_congestion_control =") != 1 || !strings.Contains(sysctl, "tcp_congestion_control = bbr") || !strings.Contains(sysctl, "CONFLICT") {
		t.Errorf("eq conflict not resolved by score with a warning:\n%s", sysctl)
	}
	dropin := files["systemd/vllm.service.d/90-kerneltuner.conf"]
	if strings.Count(dropin, "LimitMEMLOCK=") != 1 || !strings.Contains(dropin, "LimitMEMLOCK=infinity") ||
		!strings.Contains(dropin, "app.vllm.ipc-lock") || !strings.Contains(dropin, "ai.rdma.memlock") {
		t.Errorf("limit drop-in:\n%s", dropin)
	}
	if len(recs) != 6 {
		t.Errorf("input slice must be untouched")
	}
}

func TestTextShowsConflictsLine(t *testing.T) {
	var b bytes.Buffer
	if err := Text(&b, dupRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "CONFLICT: 1 setting has") || !strings.Contains(out, "a.cc proposes bbr (chosen)") ||
		strings.Index(out, "CONFLICT:") > strings.Index(out, "== MEMORY ==") {
		t.Errorf("no conflicts line:\n%s", out)
	}
	// every recommendation is still listed
	if strings.Count(out, "app.elasticsearch.map") < 1 || strings.Count(out, "app.cassandra.map") < 1 {
		t.Errorf("duplicates hidden in text:\n%s", out)
	}
	b.Reset()
	if err := Text(&b, testRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "CONFLICT") {
		t.Error("conflicts line without conflicts")
	}
}

func TestJSONKeepsEveryRecommendation(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, dupRecs(), testMeta(), false); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(b.String(), `"rule_id"`); n != 6 {
		t.Errorf("json has %d recommendations, want 6", n)
	}
}

func TestTunedExpandsSummarizedTargets(t *testing.T) {
	r := advisor.Recommendation{RuleID: "cpu.gov", Kind: "sysfs", Param: "/sys/devices/system/cpu/{item}/cpufreq/scaling_governor",
		Target: "cpu0-1 (2)", Targets: []string{"cpu0", "cpu1"}, Proposed: "performance", Status: advisor.StatusRecommend, Title: "Governor"}
	got := Tuned([]advisor.Recommendation{r}, testMeta())
	for _, want := range []string{"/sys/devices/system/cpu/cpu0/cpufreq/scaling_governor=performance", "/sys/devices/system/cpu/cpu1/cpufreq/scaling_governor=performance"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
