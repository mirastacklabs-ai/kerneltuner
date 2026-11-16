package workload

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

// matcher identifies one canonical app. All non-empty conditions must hold:
// Comms matches the process comm or argv[0] basename (any of), Cmd matches a
// substring of the space-joined command line (any of).
type matcher struct {
	App   string
	Comms []string
	Cmd   []string
}

// matchers is checked in order; the first match wins for a process. Specific
// apps come before the generic jvm entry.
var matchers = []matcher{
	{App: "vllm", Cmd: []string{"vllm.entrypoints", "vllm serve"}},
	{App: "vllm", Comms: []string{"vllm"}},
	{App: "sglang", Cmd: []string{"sglang.launch_server", "sglang.srt"}},
	{App: "tgi", Comms: []string{"text-generation-launcher", "text-generation-router", "text-generation-server"}},
	{App: "triton", Comms: []string{"tritonserver"}},
	{App: "llama.cpp", Comms: []string{"llama-server", "llama-cli"}},
	{App: "ollama", Comms: []string{"ollama"}},
	{App: "pytorch", Comms: []string{"torchrun"}},
	// specific JVM apps before the generic jvm fallback
	{App: "cassandra", Comms: []string{"java"}, Cmd: []string{"org.apache.cassandra.service.CassandraDaemon"}},
	{App: "kafka", Comms: []string{"java"}, Cmd: []string{"kafka.Kafka"}},
	{App: "pulsar", Comms: []string{"java"}, Cmd: []string{"org.apache.pulsar"}},
	{App: "zookeeper", Comms: []string{"java"}, Cmd: []string{"org.apache.zookeeper.server"}},
	{App: "elasticsearch", Comms: []string{"java"}, Cmd: []string{"org.elasticsearch.bootstrap"}},
	{App: "opensearch", Comms: []string{"java"}, Cmd: []string{"org.opensearch.bootstrap"}},
	{App: "jvm", Comms: []string{"java"}},
	{App: "redis", Comms: []string{"redis-server"}},
	{App: "valkey", Comms: []string{"valkey-server"}},
	{App: "memcached", Comms: []string{"memcached"}},
	{App: "nginx", Comms: []string{"nginx"}},
	{App: "haproxy", Comms: []string{"haproxy"}},
	{App: "envoy", Comms: []string{"envoy"}},
	{App: "httpd", Comms: []string{"httpd", "apache2"}},
	{App: "postgres", Comms: []string{"postgres", "postmaster"}},
	// mariadb before mysql: mariadbd, or mysqld whose path/cmdline mentions mariadb
	{App: "mariadb", Comms: []string{"mariadbd"}},
	{App: "mariadb", Comms: []string{"mysqld"}, Cmd: []string{"mariadb"}},
	{App: "mysql", Comms: []string{"mysqld"}},
	{App: "clickhouse", Comms: []string{"clickhouse-server"}},
	{App: "clickhouse", Comms: []string{"clickhouse"}, Cmd: []string{" server"}},
	{App: "rabbitmq", Comms: []string{"beam.smp"}, Cmd: []string{"-s rabbit", "rabbit"}},
	{App: "nats", Comms: []string{"nats-server"}},
	{App: "kubelet", Comms: []string{"kubelet"}},
	{App: "etcd", Comms: []string{"etcd"}},
	{App: "containerd", Comms: []string{"containerd"}},
	{App: "cilium", Comms: []string{"cilium-agent"}},
	{App: "coredns", Comms: []string{"coredns"}},
	{App: "prometheus", Comms: []string{"prometheus"}},
	{App: "fluent-bit", Comms: []string{"fluent-bit", "td-agent-bit"}},
}

// KnownApps returns the canonical app ids in matcher-table order.
func KnownApps() []string {
	out := make([]string, len(appOrder))
	for n, i := range appOrder {
		out[i] = n
	}
	return out
}

// ParseDeclared validates declared workload names (trimmed, deduplicated,
// order kept). Unknown names yield an error listing the valid ids.
func ParseDeclared(names []string) ([]string, error) {
	known := map[string]bool{}
	for _, n := range KnownApps() {
		known[n] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		if !known[n] {
			return nil, fmt.Errorf("unknown workload %q; valid: %s", n, strings.Join(KnownApps(), ", "))
		}
		seen[n] = true
		out = append(out, n)
	}
	return out, nil
}

// Declare appends each named app not already present as App{Declared: true}.
func (w *Workload) Declare(names []string) {
	for _, n := range names {
		if !w.Has(n) {
			w.Apps = append(w.Apps, App{Name: n, Declared: true})
		}
	}
}

// appOrder fixes the order of Apps in the result (matcher table order).
var appOrder = func() map[string]int {
	m := map[string]int{}
	for _, mt := range matchers {
		if _, ok := m[mt.App]; !ok {
			m[mt.App] = len(m)
		}
	}
	return m
}()

func commMatches(p facts.Proc, names []string) bool {
	arg0 := ""
	if len(p.Cmdline) > 0 {
		arg0 = filepath.Base(p.Cmdline[0])
	}
	for _, n := range names {
		if p.Comm == n || arg0 == n {
			return true
		}
		// the kernel truncates comm to 15 bytes
		if len(p.Comm) == 15 && strings.HasPrefix(n, p.Comm) {
			return true
		}
	}
	return false
}

func classify(p facts.Proc) string {
	cmd := strings.Join(p.Cmdline, " ")
	for _, m := range matchers {
		if len(m.Comms) > 0 && !commMatches(p, m.Comms) {
			continue
		}
		if len(m.Cmd) > 0 {
			ok := false
			for _, s := range m.Cmd {
				if strings.Contains(cmd, s) {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		return m.App
	}
	return ""
}

// Detect finds applications and listening sockets. Missing files are not errors.
func Detect(r facts.Root, f *facts.Facts) (*Workload, error) {
	w := &Workload{}
	listen := readListeners(r.Proc)
	w.Listeners = len(listen)
	portSet := map[int]bool{}
	inodePort := map[string]int{}
	for _, l := range listen {
		portSet[l.port] = true
		inodePort[l.inode] = l.port
	}
	w.ListenPorts = sortedKeys(portSet)
	if f == nil {
		return w, nil
	}

	groups := map[string][]facts.Proc{}
	torchrun := false
	var pyTorch []facts.Proc
	for _, p := range f.Procs {
		app := classify(p)
		if app == "pytorch" {
			torchrun = true
		}
		if app == "" {
			if strings.HasPrefix(p.Comm, "python") && strings.Contains(strings.Join(p.Cmdline, " "), "torch") {
				pyTorch = append(pyTorch, p)
			}
			continue
		}
		groups[app] = append(groups[app], p)
	}
	if torchrun { // low confidence: python+torch only counts alongside torchrun
		groups["pytorch"] = append(groups["pytorch"], pyTorch...)
	}

	for name, procs := range groups {
		app := App{Name: name}
		ports := map[int]bool{}
		for i, p := range procs {
			app.PIDs = append(app.PIDs, p.PID)
			if app.Unit == "" {
				app.Unit = p.Unit
			}
			if app.Container == "" {
				app.Container = p.Container
			}
			if p.NoFileSoft > app.NoFileSoftMax {
				app.NoFileSoftMax = p.NoFileSoft
			}
			if p.NoFileSoft != 0 && (app.NoFileSoft == 0 || p.NoFileSoft < app.NoFileSoft) {
				app.NoFileSoft = p.NoFileSoft
			}
			if p.NoFileHard != 0 && (app.NoFileHard == 0 || p.NoFileHard < app.NoFileHard) {
				app.NoFileHard = p.NoFileHard
			}
			if i == 0 || p.MemlockSoft < app.MemlockSoft {
				app.MemlockSoft = p.MemlockSoft
			}
			if p.NProcSoft != 0 && (app.NProcSoft == 0 || p.NProcSoft < app.NProcSoft) {
				app.NProcSoft = p.NProcSoft
			}
			if i == 0 || p.OOMScoreAdj > app.OOMScoreAdj {
				app.OOMScoreAdj = p.OOMScoreAdj
			}
			if p.OpenFDs > app.MaxOpenFDs {
				app.MaxOpenFDs = p.OpenFDs
			}
			for _, in := range socketInodes(r.Proc, p.PID) {
				if port, ok := inodePort[in]; ok {
					ports[port] = true
				}
			}
		}
		sort.Ints(app.PIDs)
		app.Ports = sortedKeys(ports)
		w.Apps = append(w.Apps, app)
	}
	sort.Slice(w.Apps, func(i, j int) bool { return appOrder[w.Apps[i].Name] < appOrder[w.Apps[j].Name] })
	return w, nil
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

type listener struct {
	port  int
	inode string
}

// readListeners parses <proc>/net/tcp and tcp6, keeping state 0A (LISTEN).
func readListeners(proc string) []listener {
	var out []listener
	for _, name := range []string{"tcp", "tcp6"} {
		fh, err := os.Open(filepath.Join(proc, "net", name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Scan() // header
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 10 || fs[3] != "0A" {
				continue
			}
			i := strings.LastIndexByte(fs[1], ':')
			if i < 0 {
				continue
			}
			port, err := strconv.ParseUint(fs[1][i+1:], 16, 16)
			if err != nil {
				continue
			}
			out = append(out, listener{port: int(port), inode: fs[9]})
		}
		_ = fh.Close()
	}
	return out
}

// socketInodes returns socket inodes held by pid; empty when unreadable.
func socketInodes(proc string, pid int) []string {
	dir := filepath.Join(proc, strconv.Itoa(pid), "fd")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		t, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil || !strings.HasPrefix(t, "socket:[") || !strings.HasSuffix(t, "]") {
			continue
		}
		out = append(out, t[len("socket:["):len(t)-1])
	}
	return out
}
