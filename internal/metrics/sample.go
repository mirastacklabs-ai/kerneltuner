package metrics

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// Sample reads every metric source under r. Missing or unparsable sources are
// skipped silently; the error is always nil.
func Sample(r facts.Root) (*Snapshot, error) {
	v := map[string]float64{}
	pairs(filepath.Join(r.Proc, "net/netstat"), v, "TcpExt", "IpExt")
	pairs(filepath.Join(r.Proc, "net/snmp"), v, "Tcp", "Udp")
	softnet(filepath.Join(r.Proc, "net/softnet_stat"), v)
	single(filepath.Join(r.Proc, "sys/net/netfilter/nf_conntrack_count"), "conntrack.count", v)
	single(filepath.Join(r.Proc, "sys/net/netfilter/nf_conntrack_max"), "conntrack.max", v)
	fileNr(filepath.Join(r.Proc, "sys/fs/file-nr"), v)
	sockstat(filepath.Join(r.Proc, "net/sockstat"), v)
	for _, res := range []string{"cpu", "memory", "io"} {
		psi(filepath.Join(r.Proc, "pressure", res), res, v)
	}
	keyVals(filepath.Join(r.Proc, "vmstat"), "vmstat.", v)
	numa(r.Sys, v)
	nics(r.Sys, v)
	return &Snapshot{Time: time.Now(), Values: v}, nil
}

func readLines(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

func num(s string) (float64, bool) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// pairs parses /proc/net/{netstat,snmp}: a header line and a value line per
// "Prefix:" group. Only the wanted prefixes are kept.
func pairs(path string, out map[string]float64, want ...string) {
	lines := readLines(path)
	for i := 0; i+1 < len(lines); i += 2 {
		h, vals := strings.Fields(lines[i]), strings.Fields(lines[i+1])
		if len(h) == 0 || len(h) != len(vals) || h[0] != vals[0] || !strings.HasSuffix(h[0], ":") {
			continue
		}
		prefix := strings.TrimSuffix(h[0], ":")
		keep := false
		for _, w := range want {
			keep = keep || w == prefix
		}
		if !keep {
			continue
		}
		for j := 1; j < len(h); j++ {
			if f, ok := num(vals[j]); ok {
				out[prefix+"."+h[j]] = f
			}
		}
	}
}

// softnet sums dropped (column 2) and time_squeeze (column 3) over CPUs.
func softnet(path string, out map[string]float64) {
	var dropped, squeeze float64
	seen := false
	for _, l := range readLines(path) {
		f := strings.Fields(l)
		if len(f) < 3 {
			continue
		}
		d, e1 := strconv.ParseUint(f[1], 16, 64)
		s, e2 := strconv.ParseUint(f[2], 16, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		dropped += float64(d)
		squeeze += float64(s)
		seen = true
	}
	if seen {
		out["softnet.dropped"] = dropped
		out["softnet.time_squeeze"] = squeeze
	}
}

func single(path, key string, out map[string]float64) {
	if l := readLines(path); len(l) > 0 {
		if f, ok := num(strings.TrimSpace(l[0])); ok {
			out[key] = f
		}
	}
}

func fileNr(path string, out map[string]float64) {
	l := readLines(path)
	if len(l) == 0 {
		return
	}
	f := strings.Fields(l[0])
	if len(f) < 3 {
		return
	}
	if a, ok := num(f[0]); ok {
		out["fs.file_nr.allocated"] = a
	}
	if m, ok := num(f[2]); ok {
		out["fs.file_nr.max"] = m
	}
}

func sockstat(path string, out map[string]float64) {
	for _, l := range readLines(path) {
		f := strings.Fields(l)
		if len(f) < 3 || f[0] != "TCP:" {
			continue
		}
		for i := 1; i+1 < len(f); i += 2 {
			switch f[i] {
			case "inuse", "orphan", "tw":
				if v, ok := num(f[i+1]); ok {
					out["sockstat.tcp."+f[i]] = v
				}
			}
		}
	}
}

// psi parses "some avg10=0.12 avg60=... total=..." lines, keeping avg10.
func psi(path, res string, out map[string]float64) {
	for _, l := range readLines(path) {
		f := strings.Fields(l)
		if len(f) < 2 || (f[0] != "some" && f[0] != "full") {
			continue
		}
		for _, kv := range f[1:] {
			if v, ok := strings.CutPrefix(kv, "avg10="); ok {
				if x, ok := num(v); ok {
					out["psi."+res+"."+f[0]+".avg10"] = x
				}
			}
		}
	}
}

// keyVals parses "name value" lines.
func keyVals(path, prefix string, out map[string]float64) {
	for _, l := range readLines(path) {
		f := strings.Fields(l)
		if len(f) != 2 {
			continue
		}
		if v, ok := num(f[1]); ok {
			out[prefix+f[0]] = v
		}
	}
}

func numa(sys string, out map[string]float64) {
	files, _ := filepath.Glob(filepath.Join(sys, "devices/system/node/node*/numastat"))
	for _, p := range files {
		node := strings.TrimPrefix(filepath.Base(filepath.Dir(p)), "node")
		if _, err := strconv.Atoi(node); err != nil {
			continue
		}
		keyVals(p, "numa."+node+".", out)
	}
}

func nics(sys string, out map[string]float64) {
	files, _ := filepath.Glob(filepath.Join(sys, "class/net/*/statistics/*"))
	for _, p := range files {
		ifc := filepath.Base(filepath.Dir(filepath.Dir(p)))
		if l := readLines(p); len(l) > 0 {
			if v, ok := num(strings.TrimSpace(l[0])); ok {
				out["nic."+ifc+"."+filepath.Base(p)] = v
			}
		}
	}
}
