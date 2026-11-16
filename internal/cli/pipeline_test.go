package cli

import (
	"context"
	"testing"
	"time"
)

func TestPipelineStubs(t *testing.T) {
	g := &globalOpts{RootDir: t.TempDir(), RulesDir: t.TempDir(), Domain: "network", Pack: "kernel/network"}
	p, err := loadPipeline(g)
	if err != nil {
		t.Fatal(err)
	}
	if p.Facts == nil || p.Workload == nil || p.Root.Proc != g.RootDir+"/proc" {
		t.Fatalf("incomplete pipeline: %+v", p)
	}
	for _, d := range []time.Duration{0, time.Millisecond} {
		v, r, err := p.sample(context.Background(), d)
		if err != nil || v == nil || r == nil {
			t.Fatalf("sample(%v) = %v %v %v", d, v, r, err)
		}
	}
	if len(p.Facts.Warnings) == 0 {
		t.Error("expected warnings from stubs")
	}
	if recs := p.evaluate(map[string]float64{}, map[string]float64{}); len(recs) != 0 {
		t.Errorf("recs = %v", recs)
	}
}
