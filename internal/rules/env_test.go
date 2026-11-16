package rules

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/workload"
)

func testEnv(t *testing.T, current, item any) Env {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proc", "sys"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proc", "marker"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f := &facts.Facts{
		Kernel:  facts.Kernel{Release: "6.8.0", Major: 6, Minor: 8, Patch: 0},
		Sysctl:  map[string]string{"net.core.somaxconn": "4096", "net.ipv4.tcp_rmem": "4096 131072 6291456"},
		Modules: map[string]bool{"nvidia": true},
		CPU:     facts.CPU{Flags: map[string]bool{"avx2": true}},
		Memory:  facts.Memory{TotalBytes: 64 << 30},
		Accel:   []facts.Accelerator{{Vendor: "nvidia"}, {Vendor: "nvidia"}, {Vendor: "amd"}},
	}
	w := &workload.Workload{Apps: []workload.App{{Name: "nginx"}}, Listeners: 3}
	return NewEnv(EnvInput{
		Facts: f, Workload: w, Root: facts.RootAt(dir),
		Values: map[string]float64{"conntrack.count": 10},
		Rates:  map[string]float64{"TcpExt.ListenOverflows": 2.5},
	}, current, item)
}

func TestEnvExpressions(t *testing.T) {
	cases := []struct {
		src  string
		cur  any
		want any
	}{
		{`kver(">=5.4")`, nil, true},
		{`kver(">=6.9")`, nil, false},
		{`kver(">=6.0 <6.8")`, nil, false},
		{`kver(">=6.0 <=6.8")`, nil, true},
		{`kver("==6.8")`, nil, true},
		{`exists("/proc/marker")`, nil, true},
		{`exists("/proc/../etc/passwd")`, nil, false},
		{`exists("/proc/nope")`, nil, false},
		{`sysctl("net.core.somaxconn")`, nil, "4096"},
		{`sysctlNum("net.core.somaxconn") * 2`, nil, float64(8192)},
		{`sysctlNum("missing")`, nil, float64(0)},
		{`metric("conntrack.count")`, nil, float64(10)},
		{`metric("nope")`, nil, float64(0)},
		{`rate("TcpExt.ListenOverflows") > 0`, nil, true},
		{`hasApp("nginx")`, nil, true},
		{`hasApp("redis")`, nil, false},
		{`cpuFlag("avx2")`, nil, true},
		{`accelCount("nvidia")`, nil, 2},
		{`module("nvidia")`, nil, true},
		{`bytes("4M")`, nil, float64(4 << 20)},
		{`bytes("1g")`, nil, float64(1 << 30)},
		{`bytes("64K")`, nil, float64(64 << 10)},
		{`clamp(5, 1, 3)`, nil, float64(3)},
		{`clamp(0, 1, 3)`, nil, float64(1)},
		{`pow2ceil(1000)`, nil, float64(1024)},
		{`pow2ceil(1024)`, nil, float64(1024)},
		{`max(4096, 100)`, nil, 4096},
		{`min(4096, 100)`, nil, 100},
		{`contains(facts.Kernel.AvailableCongestion, "bbr")`, nil, false},
		{`contains(["a","b"], "b")`, nil, true},
		{`workload.Listeners > 0 && workload.Has("nginx")`, nil, true},
		{`facts.Memory.TotalBytes / 1024 / 1024 / 1024`, nil, float64(64)},
		{`current >= 4096`, float64(4096), true},
		{`current[1]`, []float64{1, 2}, float64(2)},
		{`item.Name`, facts.NIC{Name: "eth0"}, "eth0"},
		{`[4096, 87380, max(6291456, bytes("4M"))]`, nil, []any{4096, 87380, 6291456}},
	}
	for _, c := range cases {
		var env Env
		if c.src == `item.Name` {
			env = testEnv(t, nil, c.cur)
		} else {
			env = testEnv(t, c.cur, nil)
		}
		got, err := Eval(c.src, env)
		if err != nil {
			t.Errorf("%s: %v", c.src, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s = %#v (%T), want %#v", c.src, got, got, c.want)
		}
	}
}

func TestEnvCompileErrors(t *testing.T) {
	for _, src := range []string{`nope(1)`, `facts.Nope`, `1 +`, `kver(`} {
		if _, err := Compile(src); err == nil {
			t.Errorf("%q compiled", src)
		}
	}
	if _, err := Eval(`kver("bogus")`, testEnv(t, nil, nil)); err == nil {
		t.Error("bad constraint accepted")
	}
	if _, err := EvalBool(`1 + 1`, testEnv(t, nil, nil)); err == nil {
		t.Error("EvalBool accepted a number")
	}
}

func TestTypedValue(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"4096\n", float64(4096)},
		{"4096  131072\t6291456", []float64{4096, 131072, 6291456}},
		{"bbr", "bbr"},
		{"1 two", "1 two"},
		{"", ""},
	}
	for _, c := range cases {
		if got := TypedValue(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("TypedValue(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestKernelConstraint(t *testing.T) {
	cases := []struct {
		c       string
		k       [3]int
		want    bool
		wantErr bool
	}{
		{">=4.9", [3]int{4, 9, 0}, true, false},
		{">=4.9", [3]int{4, 8, 99}, false, false},
		{">=4.9 <6.18", [3]int{6, 17, 5}, true, false},
		{">=4.9 <6.18", [3]int{6, 18, 0}, false, false},
		{">5.4.1", [3]int{5, 4, 1}, false, false},
		{">5.4.1", [3]int{5, 4, 2}, true, false},
		{"<=6.8", [3]int{6, 8, 7}, true, false},
		{"==5.15", [3]int{5, 15, 160}, true, false},
		{"==5.15.2", [3]int{5, 15, 3}, false, false},
		{"5.4", [3]int{}, false, true},
		{">=5", [3]int{}, false, true},
		{">=a.b", [3]int{}, false, true},
		{"", [3]int{}, false, true},
	}
	for _, c := range cases {
		kc, err := ParseKernelConstraint(c.c)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v", c.c, err)
			continue
		}
		if err == nil && kc.Match(c.k[0], c.k[1], c.k[2]) != c.want {
			t.Errorf("%q on %v = %v, want %v", c.c, c.k, !c.want, c.want)
		}
	}
}

func TestResolveUnderNoEscape(t *testing.T) {
	r := facts.RootAt("/fake")
	if p, ok := ResolveUnder(r, "/sys/class/../../proc/x"); !ok || p != "/fake/proc/x" {
		t.Errorf("got %q %v", p, ok)
	}
	if _, ok := ResolveUnder(r, "/usr/bin/x"); ok {
		t.Error("unknown prefix resolved")
	}
	if p, _ := ResolveUnder(r, "/proc/../../../etc/shadow"); p != "/fake/etc/shadow" {
		t.Errorf("escape: %q", p)
	}
}
