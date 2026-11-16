package facts_test

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
	"github.com/mirastacklabs-ai/kerneltuner/internal/fixturetest"
)

func collect(t *testing.T, name string) *facts.Facts {
	t.Helper()
	root, _, _ := fixturetest.Load(t, name)
	f, err := facts.Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func proc(f *facts.Facts, comm string) *facts.Proc {
	for i := range f.Procs {
		if f.Procs[i].Comm == comm {
			return &f.Procs[i]
		}
	}
	return nil
}

func TestEmptyRootWarnsWithoutError(t *testing.T) {
	f, err := facts.Collect(facts.RootAt(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Warnings) == 0 {
		t.Fatal("expected warnings for missing files")
	}
	if f.Kernel.Major != 0 || f.Sysctl == nil || f.Modules == nil || f.CPU.Flags == nil {
		t.Fatalf("zero values / non-nil maps expected: %+v", f.Kernel)
	}
}

func TestKernelAndCmdline(t *testing.T) {
	f := collect(t, "gpu-8xh100")
	k := f.Kernel
	if k.Release != "6.8.0-45-generic" || k.Major != 6 || k.Minor != 8 || k.Patch != 0 {
		t.Errorf("kernel = %+v", k)
	}
	if v, ok := k.CmdlineArgs["intel_iommu"]; !ok || v != "on" {
		t.Errorf("cmdline args = %v", k.CmdlineArgs)
	}
	if v, ok := k.CmdlineArgs["quiet"]; !ok || v != "" {
		t.Errorf("flag quiet = %q,%v", v, ok)
	}
	web := collect(t, "legacy-4.9-c10k")
	if web.Kernel.Major != 4 || web.Kernel.Minor != 9 || web.Kernel.Patch != 0 {
		t.Errorf("legacy kernel = %+v", web.Kernel)
	}
	if !reflect.DeepEqual(web.Kernel.AvailableCongestion, []string{"cubic", "reno"}) {
		t.Errorf("congestion = %v", web.Kernel.AvailableCongestion)
	}
}

func TestHost(t *testing.T) {
	cases := []struct {
		fixture   string
		virt      string
		container bool
		runtime   string
		cloud     string
	}{
		{"web-16c-64g-nginx", "none", false, "", ""},
		{"container-k8s-redis", "kvm", true, "containerd", "aws"},
		{"gpu-8xh100", "none", false, "", ""},
	}
	for _, tc := range cases {
		h := collect(t, tc.fixture).Host
		if h.Virt != tc.virt || h.Container != tc.container || h.ContainerRuntime != tc.runtime || h.Cloud != tc.cloud {
			t.Errorf("%s: host = %+v", tc.fixture, h)
		}
	}
}

func TestHostContainerSignals(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("proc/1/environ", "PATH=/bin\x00container=lxc\x00")
	if h := mustCollect(t, dir).Host; !h.Container || h.ContainerRuntime != "lxc" {
		t.Errorf("environ: %+v", h)
	}
	_ = os.Remove(filepath.Join(dir, "proc/1/environ"))
	write("run/.containerenv", "")
	if h := mustCollect(t, dir).Host; !h.Container || h.ContainerRuntime != "podman" {
		t.Errorf(".containerenv: %+v", h)
	}
	_ = os.Remove(filepath.Join(dir, "run/.containerenv"))
	write(".dockerenv", "")
	if h := mustCollect(t, dir).Host; !h.Container || h.ContainerRuntime != "docker" {
		t.Errorf(".dockerenv: %+v", h)
	}
	write("proc/1/cgroup", "12:cpu:/docker/abc\n")
	_ = os.Remove(filepath.Join(dir, ".dockerenv"))
	if h := mustCollect(t, dir).Host; !h.Container || h.ContainerRuntime != "docker" {
		t.Errorf("cgroup: %+v", h)
	}
}

func mustCollect(t *testing.T, dir string) *facts.Facts {
	t.Helper()
	f, err := facts.Collect(facts.RootAt(dir))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCPUAndNUMA(t *testing.T) {
	f := collect(t, "gpu-8xh100")
	c := f.CPU
	if c.Sockets != 2 || c.Cores != 64 || c.Threads != 128 || !c.SMTActive || c.SMTControl != "on" {
		t.Errorf("cpu = %+v", c)
	}
	if c.Vendor != "GenuineIntel" || !strings.Contains(c.Model, "8462Y+") || !c.Flags["amx_tile"] || !c.Flags["avx512f"] {
		t.Errorf("cpu id = %q %q", c.Vendor, c.Model)
	}
	if c.ScalingDriver != "intel_pstate" || c.Governor != "performance" || c.EPP != "performance" || c.IdleDriver != "intel_idle" {
		t.Errorf("power = %+v", c)
	}
	if len(f.NUMA) != 2 {
		t.Fatalf("numa = %+v", f.NUMA)
	}
	n0 := f.NUMA[0]
	if n0.ID != 0 || len(n0.CPUs) != 64 || n0.CPUs[0] != 0 || n0.CPUs[31] != 31 || n0.CPUs[32] != 64 || n0.MemTotalBytes != 1056878080*1024 {
		t.Errorf("node0 = %+v", n0)
	}
	total := 0
	for _, n := range f.NUMA {
		total += len(n.CPUs)
	}
	if total != c.Threads {
		t.Errorf("numa cpus %d != threads %d", total, c.Threads)
	}
	if g := collect(t, "cpu-amx-2s").CPU; g.Governor != "powersave" || g.EPP != "balance_power" || g.ScalingDriver != "intel_pstate" || !g.Flags["amx_bf16"] || !g.Flags["amx_int8"] || !g.Flags["avx512_bf16"] {
		t.Errorf("amx cpu = %+v", g)
	}
	if g := collect(t, "gaudi3-8x").CPU.Governor; g != "schedutil" {
		t.Errorf("gaudi governor = %q", g)
	}
	if v := collect(t, "legacy-4.9-c10k").CPU; v.Sockets != 1 || v.Cores != 4 || v.Threads != 8 {
		t.Errorf("legacy cpu = %+v", v)
	}
}

func TestCPUFallbackToCPUInfo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proc"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := "processor\t: 0\nvendor_id\t: X\nphysical id\t: 0\ncpu cores\t: 2\nflags\t\t: fpu sse\n\nprocessor\t: 1\nvendor_id\t: X\nphysical id\t: 0\ncpu cores\t: 2\n\n"
	if err := os.WriteFile(filepath.Join(dir, "proc/cpuinfo"), []byte(info), 0o644); err != nil {
		t.Fatal(err)
	}
	c := mustCollect(t, dir).CPU
	if c.Threads != 2 || c.Sockets != 1 || c.Cores != 2 || !c.Flags["sse"] {
		t.Errorf("cpu = %+v", c)
	}
}

func TestMemory(t *testing.T) {
	m := collect(t, "gpu-8xh100").Memory
	if m.TotalBytes != 2113756160*1024 || m.THPEnabled != "always" || m.THPDefrag != "madvise" {
		t.Errorf("memory = %+v", m)
	}
	if v, ok := m.HugePages["2048kB"]; !ok || v != 0 {
		t.Errorf("hugepages = %v", m.HugePages)
	}
	if m := collect(t, "container-k8s-redis").Memory; m.THPEnabled != "always" || m.SwapTotalBytes != 0 {
		t.Errorf("redis memory = %+v", m)
	}
	if m := collect(t, "web-16c-64g-nginx").Memory; m.THPEnabled != "madvise" || m.SwapTotalBytes != 8388604*1024 {
		t.Errorf("web memory = %+v", m)
	}
}

func TestNICs(t *testing.T) {
	f := collect(t, "web-16c-64g-nginx")
	if len(f.NICs) != 1 {
		t.Fatalf("nics = %+v", f.NICs)
	}
	n := f.NICs[0]
	want := facts.NIC{Name: "eth0", Driver: "mlx5_core", SpeedMbps: 25000, MTU: 1500, NUMANode: 0, RxQueues: 16, TxQueues: 16,
		Combined: 16, CombinedMax: 63, RxRing: 1024, RxRingMax: 8192, TxRing: 1024, TxRingMax: 8192}
	if n != want {
		t.Errorf("nic = %+v\nwant  %+v", n, want)
	}
	v := collect(t, "container-k8s-redis").NICs[0]
	if !v.Virtual || v.Driver != "" || v.MTU != 8951 || v.RxRing != 0 || v.NUMANode != -1 {
		t.Errorf("veth = %+v", v)
	}
	l := collect(t, "legacy-4.9-c10k").NICs[0]
	if l.Driver != "ixgbe" || l.SpeedMbps != 10000 || l.Virtual {
		t.Errorf("legacy nic = %+v", l)
	}
}

func TestNICSkipsLoopback(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"lo", "eth1"} {
		if err := os.MkdirAll(filepath.Join(dir, "sys/class/net", n), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := mustCollect(t, dir)
	if len(f.NICs) != 1 || f.NICs[0].Name != "eth1" || !f.NICs[0].Virtual {
		t.Errorf("nics = %+v", f.NICs)
	}
}

func TestBlock(t *testing.T) {
	b := collect(t, "cpu-amx-2s").Block
	if len(b) != 1 {
		t.Fatalf("block = %+v", b)
	}
	d := b[0]
	if d.Name != "nvme0n1" || d.Transport != "nvme" || d.Rotational || d.Scheduler != "none" ||
		!reflect.DeepEqual(d.Schedulers, []string{"none", "mq-deadline"}) || d.ReadAheadKB != 128 || d.NrRequests != 1023 {
		t.Errorf("block = %+v", d)
	}
	if l := collect(t, "legacy-4.9-c10k").Block[0]; l.Scheduler != "cfq" || l.Transport != "sata" || len(l.Schedulers) != 3 {
		t.Errorf("legacy block = %+v", l)
	}
}

func TestBlockSkipsVirtual(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"loop0", "ram0", "zram0", "dm-0", "vda", "nvme1n1"} {
		if err := os.MkdirAll(filepath.Join(dir, "sys/block", n, "queue"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := mustCollect(t, dir)
	var names []string
	for _, b := range f.Block {
		names = append(names, b.Name+":"+b.Transport)
	}
	if !reflect.DeepEqual(names, []string{"nvme1n1:nvme", "vda:virtio"}) {
		t.Errorf("block = %v", names)
	}
}

func TestAccel(t *testing.T) {
	f := collect(t, "gpu-8xh100")
	if len(f.Accel) != 8 {
		t.Fatalf("accel = %d", len(f.Accel))
	}
	narrow := 0
	for i, a := range f.Accel {
		if a.Vendor != "nvidia" || a.VendorID != "0x10de" || a.Driver != "nvidia" || a.Class != "0x030200" || a.NUMANode != i/4 {
			t.Errorf("gpu %d = %+v", i, a)
		}
		if a.LinkSpeed != "32.0 GT/s PCIe" || a.LinkWidthMax != 16 || a.IOMMUGroup == "" {
			t.Errorf("gpu %d link = %+v", i, a)
		}
		if a.LinkWidth == 8 {
			narrow++
			if a.PCIAddr != "0000:ab:00.0" {
				t.Errorf("narrow gpu = %s", a.PCIAddr)
			}
		}
	}
	if narrow != 1 {
		t.Errorf("narrow gpus = %d", narrow)
	}
	for name, want := range map[string]struct {
		vendor string
		n      int
		driver string
	}{
		"gpu-8xmi300x":   {"amd", 8, "amdgpu"},
		"gaudi3-8x":      {"habana", 8, "habanalabs"},
		"intel-max-1550": {"intel", 2, "i915"},
	} {
		got := collect(t, name).Accel
		if len(got) != want.n || got[0].Vendor != want.vendor || got[0].Driver != want.driver {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if a := collect(t, "web-16c-64g-nginx").Accel; len(a) != 0 {
		t.Errorf("web accel = %+v", a)
	}
}

func TestRDMAAndIOMMU(t *testing.T) {
	f := collect(t, "gpu-8xh100")
	if len(f.RDMA) != 8 {
		t.Fatalf("rdma = %d", len(f.RDMA))
	}
	p := f.RDMA[0].Ports[0]
	if f.RDMA[0].Name != "mlx5_0" || p.Num != 1 || p.State != "4: ACTIVE" || p.LinkLayer != "InfiniBand" || p.Rate != "400 Gb/sec (4X NDR)" || p.NetDev != "ibp24s0" {
		t.Errorf("rdma0 = %+v", f.RDMA[0])
	}
	if !f.IOMMU.Enabled || f.IOMMU.Passthrough || f.IOMMU.Groups != 16 {
		t.Errorf("iommu = %+v", f.IOMMU)
	}
	if f.Modules["nvidia_peermem"] || !f.Modules["nvidia"] {
		t.Errorf("modules = %v", f.Modules)
	}
	if f := collect(t, "web-16c-64g-nginx"); f.IOMMU.Enabled || len(f.RDMA) != 0 {
		t.Errorf("web iommu/rdma = %+v %+v", f.IOMMU, f.RDMA)
	}
}

func TestIOMMUPassthrough(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, s string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("proc/cmdline", "ro iommu=pt\n")
	if m := mustCollect(t, dir).IOMMU; !m.Passthrough {
		t.Errorf("cmdline pt: %+v", m)
	}
	write("proc/cmdline", "ro\n")
	write("sys/kernel/iommu_groups/0/type", "identity\n")
	write("sys/kernel/iommu_groups/1/type", "identity\n")
	if m := mustCollect(t, dir).IOMMU; !m.Passthrough || m.Groups != 2 || !m.Enabled {
		t.Errorf("identity groups: %+v", m)
	}
}

func TestSysctlAndLimits(t *testing.T) {
	f := collect(t, "legacy-4.9-c10k")
	if f.Sysctl["net.core.somaxconn"] != "128" || f.Sysctl["net.ipv4.tcp_tw_recycle"] != "1" || f.Sysctl["net.ipv4.tcp_timestamps"] != "0" {
		t.Errorf("legacy sysctl: %v", f.Sysctl["net.core.somaxconn"])
	}
	if f.Sysctl["net.ipv4.tcp_rmem"] != "4096 131072 6291456" {
		t.Errorf("tcp_rmem = %q", f.Sysctl["net.ipv4.tcp_rmem"])
	}
	if f.Sysctl["net.netfilter.nf_conntrack_max"] != "65536" || f.Sysctl["net.ipv4.conf.all.rp_filter"] != "2" {
		t.Errorf("netfilter/conf: %v", f.Sysctl)
	}
	if _, ok := f.Sysctl["net.core.netdev_budget_usecs"]; ok {
		t.Error("4.9 must not have netdev_budget_usecs")
	}
	if f.Sysctl["fs.file-max"] != "6553600" || f.Limits.FileMax != 6553600 || f.Limits.NrOpen != 1048576 || f.Limits.FileNrAllocated != 31800 {
		t.Errorf("limits = %+v", f.Limits)
	}
	if collect(t, "gpu-8xh100").Sysctl["kernel.numa_balancing"] != "1" {
		t.Error("h100 numa_balancing")
	}
}

func TestSysctlNormalisesWhitespace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "proc/sys/net/ipv4")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "tcp_mem"), []byte("1\t2   3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "write_only"), []byte("x"), 0o200); err != nil {
		t.Fatal(err)
	}
	f := mustCollect(t, dir)
	if f.Sysctl["net.ipv4.tcp_mem"] != "1 2 3" {
		t.Errorf("tcp_mem = %q", f.Sysctl["net.ipv4.tcp_mem"])
	}
	if _, ok := f.Sysctl["net.ipv4.write_only"]; ok {
		t.Error("write-only file must be skipped")
	}
}

func TestProcs(t *testing.T) {
	f := collect(t, "web-16c-64g-nginx")
	nginx := proc(f, "nginx")
	if nginx == nil || nginx.PID != 1203 || nginx.Unit != "nginx.service" || nginx.NoFileSoft != 1024 || nginx.NoFileHard != 524288 {
		t.Fatalf("nginx = %+v", nginx)
	}
	if nginx.OpenFDs != 9 || nginx.MemlockSoft != 8388608 || len(nginx.Cmdline) != 1 {
		t.Errorf("nginx = %+v", nginx)
	}
	workers := 0
	for _, p := range f.Procs {
		if p.Comm == "nginx" {
			workers++
		}
	}
	if workers != 17 {
		t.Errorf("nginx procs = %d", workers)
	}
	if sd := proc(f, "systemd"); sd == nil || sd.PID != 1 || sd.Unit != "" {
		t.Errorf("systemd = %+v", sd)
	}

	v := proc(collect(t, "gpu-8xh100"), "python3")
	if v == nil || v.Unit != "vllm.service" || v.MemlockSoft != 8388608 || v.NoFileSoft != 1048576 {
		t.Fatalf("vllm = %+v", v)
	}
	if got := strings.Join(v.Cmdline, " "); !strings.HasPrefix(got, "python3 -m vllm.entrypoints.openai.api_server") {
		t.Errorf("cmdline = %q", got)
	}
	if sg := proc(collect(t, "gpu-8xmi300x"), "python3"); sg == nil || sg.MemlockSoft != math.MaxUint64 || sg.Cmdline[2] != "sglang.launch_server" {
		t.Errorf("sglang = %+v", sg)
	}
	if r := proc(collect(t, "container-k8s-redis"), "redis-server"); r == nil || r.PID != 1 || r.Cmdline[1] != "*:6379" || r.Unit != "" {
		t.Errorf("redis = %+v", r)
	}
}

func TestUnitFromNestedCgroup(t *testing.T) {
	dir := t.TempDir()
	d := filepath.Join(dir, "proc/42")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"comm":    "worker\n",
		"cmdline": "/bin/worker\x00--flag\x00",
		"cgroup":  "1:name=systemd:/system.slice/app.service/sub\n0::/system.slice/app.service/sub\n",
		"limits":  "Limit Soft Limit Hard Limit Units\nMax open files            unlimited            unlimited            files\nMax locked memory         65536                unlimited            bytes\n",
	}
	for k, v := range files {
		if err := os.WriteFile(filepath.Join(d, k), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := mustCollect(t, dir)
	p := proc(f, "worker")
	if p == nil || p.Unit != "app.service" || p.NoFileSoft != math.MaxUint64 || p.MemlockHard != math.MaxUint64 || p.MemlockSoft != 65536 || p.OpenFDs != 0 {
		t.Errorf("proc = %+v", p)
	}
}

func TestEcosystem(t *testing.T) {
	f := collect(t, "bpftune-tuned-host")
	if !f.Tuned.Active || f.Tuned.Profile != "throughput-performance" || !f.Bpftune || f.Irqbalance {
		t.Errorf("tuned=%+v bpftune=%v irqbalance=%v", f.Tuned, f.Bpftune, f.Irqbalance)
	}
	if f.Kernel.Major != 6 || f.Kernel.Minor != 6 {
		t.Errorf("kernel = %+v", f.Kernel)
	}
	g := collect(t, "gpu-8xh100")
	if g.Tuned.Active || g.Tuned.Profile != "" || g.Bpftune || !g.Irqbalance {
		t.Errorf("h100 ecosystem: %+v %v %v", g.Tuned, g.Bpftune, g.Irqbalance)
	}
	if proc(g, "nvidia-persistenced") != nil {
		t.Error("nvidia-persistenced must not run")
	}
}

func TestOtherFixturesCollectCleanly(t *testing.T) {
	for _, name := range []string{"gpu-8xmi300x", "gaudi3-8x", "intel-max-1550", "cpu-amx-2s", "bpftune-tuned-host", "container-k8s-redis"} {
		f := collect(t, name)
		if f.Kernel.Major == 0 || f.CPU.Threads == 0 || f.Memory.TotalBytes == 0 || len(f.NUMA) == 0 {
			t.Errorf("%s: incomplete facts %+v", name, f.CPU)
		}
		if f.CPU.Threads != totalNUMACPUs(f) {
			t.Errorf("%s: threads %d vs numa %d", name, f.CPU.Threads, totalNUMACPUs(f))
		}
	}
}

// colima-vm is a real capture (kerneltuner capture --fixture) of the Colima
// development VM, so it exercises the collectors on real kernel formats.
func TestColimaVMRealCapture(t *testing.T) {
	f := collect(t, "colima-vm")
	if f.Kernel.Major <= 0 {
		t.Errorf("kernel = %+v", f.Kernel)
	}
	if f.CPU.Threads <= 0 || f.CPU.Sockets <= 0 || f.CPU.Cores <= 0 {
		t.Errorf("cpu = %+v", f.CPU)
	}
	if f.Memory.TotalBytes == 0 || f.Memory.THPEnabled == "" {
		t.Errorf("memory = %+v", f.Memory)
	}
	if f.Host.Virt == "none" || f.Host.Container {
		t.Errorf("host = %+v", f.Host)
	}
	if len(f.Sysctl) < 100 || len(f.Block) == 0 || len(f.Procs) == 0 || len(f.NICs) == 0 {
		t.Errorf("sysctl=%d block=%d procs=%d nics=%d", len(f.Sysctl), len(f.Block), len(f.Procs), len(f.NICs))
	}
	if p := proc(f, "systemd"); p == nil || p.PID != 1 {
		t.Errorf("systemd = %+v", p)
	}
}

func totalNUMACPUs(f *facts.Facts) int {
	n := 0
	for _, x := range f.NUMA {
		n += len(x.CPUs)
	}
	return n
}
