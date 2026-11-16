package apply

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/advisor"
)

func TestApplyLimitNPROCDropin(t *testing.T) {
	root := setup(t)
	recs := []advisor.Recommendation{
		rec("limit", "LimitNPROC", "elasticsearch.service", "65536"),
		rec("limit", "nproc", "cassandra", "infinity"),
	}
	p, err := BuildPlan(recs, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Ops) != 2 || len(p.Skipped) != 0 {
		t.Fatalf("ops=%d skipped=%+v", len(p.Ops), p.Skipped)
	}
	if _, err := Apply(p, Options{Now: fixed()}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(root.Etc, "systemd/system/elasticsearch.service.d/90-kerneltuner.conf")); !strings.Contains(got, "[Service]\nLimitNPROC=65536\n") {
		t.Errorf("elasticsearch dropin = %q", got)
	}
	if got := read(t, filepath.Join(root.Etc, "systemd/system/cassandra.service.d/90-kerneltuner.conf")); !strings.Contains(got, "LimitNPROC=infinity\n") {
		t.Errorf("cassandra dropin = %q", got)
	}
}
