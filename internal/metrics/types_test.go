package metrics

import (
	"testing"
	"time"
)

func TestIsCounter(t *testing.T) {
	for k, want := range map[string]bool{
		"TcpExt.ListenOverflows": true, "softnet.dropped": true, "vmstat.pgfault": true,
		"conntrack.count": false, "fs.file_nr.allocated": false, "sockstat.tcp.tw": false, "psi.cpu.some.avg10": false,
	} {
		if IsCounter(k) != want {
			t.Errorf("IsCounter(%q) != %v", k, want)
		}
	}
}

func TestRates(t *testing.T) {
	t0 := time.Unix(100, 0)
	prev := &Snapshot{Time: t0, Values: map[string]float64{"a": 10, "g": 1}}
	cur := &Snapshot{Time: t0.Add(5 * time.Second), Values: map[string]float64{"a": 20, "conntrack.count": 9, "new": 1}}
	r := Rates(prev, cur)
	if r["a"] != 2 || len(r) != 1 {
		t.Fatalf("got %v", r)
	}
	if len(Rates(nil, cur)) != 0 {
		t.Fatal("nil prev")
	}
}
