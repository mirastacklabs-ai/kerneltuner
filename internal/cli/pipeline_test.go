package cli

import (
	"context"
	"strings"
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
	if recs := p.evaluate(map[string]float64{}, map[string]float64{}); len(recs) != 0 {
		t.Errorf("recs = %v", recs)
	}
}

func TestPipelineDeclaredWorkload(t *testing.T) {
	g := &globalOpts{RootDir: t.TempDir(), RulesDir: t.TempDir(), Workload: []string{"kafka", "nginx"}}
	p, err := loadPipeline(g)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Workload.Has("kafka") || !p.Workload.Has("nginx") || !p.Workload.Apps[0].Declared {
		t.Fatalf("apps: %+v", p.Workload.Apps)
	}
}

func TestWorkloadFlagCLI(t *testing.T) {
	if _, _, err := run(t, "--workload", "kafka,redis", "scan"); err != nil {
		t.Fatal(err)
	}
	_, _, err := run(t, "--workload", "bogus", "scan")
	if err == nil || !strings.Contains(err.Error(), "unknown workload") || !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("err = %v", err)
	}
}
