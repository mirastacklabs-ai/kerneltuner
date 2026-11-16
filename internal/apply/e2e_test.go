package apply

import (
	"os"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// TestRealProcNamespaced exercises the real procfs. Only run it inside a
// disposable container with its own network namespace (never --net=host):
//
//	KT_E2E_DISPOSABLE=1 go test ./internal/apply -run RealProc
func TestRealProcNamespaced(t *testing.T) {
	if os.Getenv("KT_E2E_DISPOSABLE") != "1" {
		t.Skip("set KT_E2E_DISPOSABLE=1 inside a disposable container")
	}
	const live = "/proc/sys/net/ipv4/tcp_fin_timeout"
	root := facts.Root{Proc: "/proc", Sys: "/sys", Etc: t.TempDir(), Var: t.TempDir()}
	before, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	p, err := BuildPlan([]advisor.Recommendation{{RuleID: "e2e", Kind: "sysctl",
		Param: "net.ipv4.tcp_fin_timeout", Proposed: "17", Status: advisor.StatusRecommend}}, root)
	if err != nil || len(p.Ops) != 1 {
		t.Fatalf("plan: %+v %v", p, err)
	}
	res, err := Apply(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	mid, _ := os.ReadFile(live)
	t.Logf("before=%q after-apply=%q", strings.TrimSpace(string(before)), strings.TrimSpace(string(mid)))
	if strings.TrimSpace(string(mid)) != "17" {
		t.Fatalf("not applied: %q", mid)
	}
	if err := Rollback(root, res.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(live)
	t.Logf("after-rollback=%q", strings.TrimSpace(string(after)))
	if string(after) != string(before) {
		t.Fatalf("not restored: %q != %q", after, before)
	}
}
