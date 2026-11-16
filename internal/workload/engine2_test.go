package workload

import (
	"math"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func TestDetectNProcAndOOM(t *testing.T) {
	f := &facts.Facts{Procs: []facts.Proc{
		{PID: 10, Comm: "postgres", NProcSoft: 8192, OOMScoreAdj: -900},
		{PID: 11, Comm: "postgres", NProcSoft: 4096, OOMScoreAdj: 0},
		{PID: 12, Comm: "postgres", NProcSoft: 0, OOMScoreAdj: -1000},
		{PID: 20, Comm: "redis-server", NProcSoft: math.MaxUint64, OOMScoreAdj: -500},
	}}
	w, err := Detect(root(), f)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]App{}
	for _, a := range w.Apps {
		got[a.Name] = a
	}
	if pg := got["postgres"]; pg.NProcSoft != 4096 || pg.OOMScoreAdj != 0 {
		t.Errorf("postgres = %+v (min nonzero nproc, max oom_score_adj)", pg)
	}
	if rd := got["redis"]; rd.NProcSoft != math.MaxUint64 || rd.OOMScoreAdj != -500 {
		t.Errorf("redis = %+v", rd)
	}
}
