package facts_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mirastacklabs-ai/kerneltuner/internal/facts"
)

func TestEngineFactsFixture(t *testing.T) {
	f := collect(t, "engine-facts")

	if d := f.Host.Distro; d.ID != "ubuntu" || d.VersionID != "24.04" {
		t.Errorf("distro = %+v", d)
	}
	if v := f.CPU.Vulnerabilities["spectre_v2"]; !strings.HasPrefix(v, "Mitigation: Enhanced") || f.CPU.Vulnerabilities["meltdown"] != "Not affected" {
		t.Errorf("vulnerabilities = %v", f.CPU.Vulnerabilities)
	}
	if len(f.CPU.IDs) != 8 || f.CPU.IDs[7] != 7 {
		t.Errorf("cpu ids = %v", f.CPU.IDs)
	}

	if len(f.Swaps) != 2 {
		t.Fatalf("swaps = %+v", f.Swaps)
	}
	if z := f.Swaps[0]; z.Path != "/dev/zram0" || !z.Zram || z.Type != "partition" || z.SizeKB != 4194300 || z.Priority != 100 {
		t.Errorf("zram swap = %+v", z)
	}
	if s := f.Swaps[1]; s.Zram || s.Type != "file" || s.Priority != -2 {
		t.Errorf("file swap = %+v", s)
	}

	if f.Sysctl["fs.inotify.max_user_watches"] != "8192" || f.Sysctl["net.bridge.bridge-nf-call-iptables"] != "1" {
		t.Errorf("inotify/bridge sysctls missing: %v", f.Sysctl)
	}

	// RoCE netdev exists only under infiniband/mlx5_0/device/net/roce0.
	var roce *facts.NIC
	for i := range f.NICs {
		if f.NICs[i].Name == "roce0" {
			roce = &f.NICs[i]
		}
	}
	if roce == nil || roce.MTU != 4200 || roce.Virtual {
		t.Errorf("roce0 = %+v (nics %+v)", roce, f.NICs)
	}

	var gpus, switches int
	for _, a := range f.Accel {
		switch a.Kind {
		case "gpu":
			gpus++
		case "nvswitch":
			switches++
			if a.Vendor != "nvidia" || a.Class != "0x068000" {
				t.Errorf("nvswitch = %+v", a)
			}
		}
	}
	if gpus != 2 || switches != 2 {
		t.Errorf("gpus=%d nvswitch=%d (%+v)", gpus, switches, f.Accel)
	}

	if v := f.Kernel.CmdlineArgs["pci"]; v != "realloc=off,nommconf" {
		t.Errorf("pci cmdline = %q", v)
	}
	if p := proc(f, "python3"); p == nil || p.Container != "docker" {
		t.Errorf("docker proc = %+v", p)
	}
	if p := proc(f, "ollama"); p == nil || p.Container != "containerd" {
		t.Errorf("containerd proc = %+v", p)
	}
	if p := proc(f, "sshd"); p == nil || p.Container != "" {
		t.Errorf("sshd container = %+v", p)
	}
	for _, w := range f.Warnings {
		if strings.Contains(w, "PCI") {
			t.Errorf("unexpected PCI warning: %q", w)
		}
	}
}

func TestNoPCIDirIsNotAWarning(t *testing.T) {
	f, err := facts.Collect(facts.RootAt(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range f.Warnings {
		if strings.Contains(w, "cannot scan PCI") {
			t.Errorf("noise warning: %q", w)
		}
	}
}

func TestDistroFallbackToUsrLib(t *testing.T) {
	dir := t.TempDir()
	write(t, dir+"/usr/lib/os-release", "NAME=\"Rocky Linux\"\nID=\"rocky\"\nVERSION_ID=\"9.4\"\n")
	f, err := facts.Collect(facts.RootAt(dir))
	if err != nil {
		t.Fatal(err)
	}
	if d := f.Host.Distro; d.ID != "rocky" || d.VersionID != "9.4" {
		t.Errorf("distro = %+v", d)
	}
	write(t, dir+"/etc/os-release", "ID=debian\nVERSION_ID=12\n")
	if f, _ = facts.Collect(facts.RootAt(dir)); f.Host.Distro.ID != "debian" {
		t.Errorf("etc must win: %+v", f.Host.Distro)
	}
}

func TestContainerRuntimes(t *testing.T) {
	dir := t.TempDir()
	cases := map[int]struct{ cg, want string }{
		10: {"0::/system.slice/docker-3f2a9c.scope\n", "docker"},
		11: {"0::/kubepods.slice/kubepods-burstable.slice/cri-containerd-aa11.scope\n", "containerd"},
		12: {"0::/kubepods.slice/crio-bb22.scope\n", "crio"},
		13: {"0::/machine.slice/libpod-cc33.scope\n", "podman"},
		14: {"12:cpu:/docker/0123456789abcdef\n", "docker"},
		15: {"0::/system.slice/nginx.service\n", ""},
		16: {"0::/system.slice/docker.service\n", ""},
	}
	for pid, c := range cases {
		p := dir + "/proc/" + itoa(pid)
		write(t, p+"/comm", "app\n")
		write(t, p+"/cgroup", c.cg)
	}
	f, err := facts.Collect(facts.RootAt(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := map[int]string{}
	for _, p := range f.Procs {
		got[p.PID] = p.Container
	}
	for pid, c := range cases {
		if got[pid] != c.want {
			t.Errorf("pid %d (%s): container %q, want %q", pid, strings.TrimSpace(c.cg), got[pid], c.want)
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
