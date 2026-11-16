package metrics

import (
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func TestSample(t *testing.T) {
	s, err := Sample(facts.Root{Proc: "testdata/proc", Sys: "testdata/sys"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"TcpExt.ListenOverflows": 7, "TcpExt.ListenDrops": 9, "IpExt.InOctets": 5000,
		"Tcp.RetransSegs": 100, "Tcp.CurrEstab": 42, "Tcp.MaxConn": -1, "Udp.RcvbufErrors": 2,
		"softnet.dropped": 11, "softnet.time_squeeze": 13,
		"conntrack.count": 1234, "conntrack.max": 262144,
		"fs.file_nr.allocated": 2048, "fs.file_nr.max": 9223372,
		"sockstat.tcp.inuse": 12, "sockstat.tcp.orphan": 3, "sockstat.tcp.tw": 45,
		"psi.cpu.some.avg10": 1.5, "psi.memory.some.avg10": 0, "psi.memory.full.avg10": 2.25,
		"vmstat.nr_free_pages": 1000, "vmstat.pgfault": 5555,
		"numa.0.numa_hit": 100, "numa.1.numa_miss": 6,
		"nic.eth0.rx_dropped": 999, "nic.eth0.rx_packets": 888,
	}
	for k, w := range want {
		if g, ok := s.Values[k]; !ok || g != w {
			t.Errorf("%s: got %v (present=%v) want %v", k, g, ok, w)
		}
	}
	if _, ok := s.Values["MPTcpExt.Foo"]; ok {
		t.Error("MPTcpExt must be ignored")
	}
	if _, ok := s.Values["Ip.Forwarding"]; ok {
		t.Error("Ip must be ignored")
	}
}

func TestSampleMissingRoot(t *testing.T) {
	s, err := Sample(facts.Root{Proc: "testdata/none", Sys: "testdata/none"})
	if err != nil || s == nil || len(s.Values) != 0 {
		t.Fatalf("%v %v", s, err)
	}
}

func TestGaugesNotRated(t *testing.T) {
	for _, k := range []string{"Tcp.CurrEstab", "vmstat.nr_free_pages", "Tcp.MaxConn"} {
		if IsCounter(k) {
			t.Errorf("%s should be a gauge", k)
		}
	}
	for _, k := range []string{"Tcp.RetransSegs", "nic.eth0.rx_dropped", "numa.0.numa_hit", "TcpExt.ListenDrops"} {
		if !IsCounter(k) {
			t.Errorf("%s should be a counter", k)
		}
	}
}
