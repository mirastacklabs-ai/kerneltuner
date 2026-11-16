package main

import (
	"fmt"
	"strconv"
)

// osRelease writes <etc>/os-release (the Debian/Ubuntu subset).
func (f *fx) osRelease(id, version string) {
	f.w("etc/os-release", fmt.Sprintf("NAME=%q\nVERSION_ID=%q\nID=%s\nID_LIKE=debian\n", id, version, id))
}

// vulns writes /sys/devices/system/cpu/vulnerabilities/<name>.
func (f *fx) vulns(kv map[string]string) {
	for k, v := range kv {
		f.wl("sys/devices/system/cpu/vulnerabilities/"+k, v)
	}
}

// governors writes scaling_governor for every cpu (cpufreq only writes cpu0).
func (f *fx) governors(n int, governor string) {
	for i := 0; i < n; i++ {
		f.wl(fmt.Sprintf("sys/devices/system/cpu/cpu%d/cpufreq/scaling_governor", i), governor)
	}
}

// cgroup overrides the cgroup file of an already added process.
func (f *fx) cgroup(pid int, path string) {
	f.wl(fmt.Sprintf("proc/%d/cgroup", pid), "0::"+path)
}

// engineFacts exercises the engine fact collectors: distro, CPU
// vulnerabilities, zram swap, inotify and bridge sysctls, a RoCE netdev that
// exists only under infiniband/, an NVSwitch bridge, containerised and
// comm-truncated processes, a comma-list cmdline value and per-cpu governors.
func engineFacts() *fx {
	f := newFx("engine-facts", "Engine fact coverage: Ubuntu 24.04, 8 cpus with mixed governors, zram swap, 2 H100 + 2 NVSwitch, RoCE netdev only under infiniband, dockerised vllm, nvidia-persistenced, pci=realloc=off,nommconf", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro quiet pci=realloc=off,nommconf")
	f.osRelease("ubuntu", "24.04")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) Platinum 8462Y+", family: 6, modelNum: 143, step: 8,
		sockets: 1, cores: 4, tpc: 2, flags: flagsBase + flagsAVX512, mhz: "2800.000", cacheKB: 61440}
	f.dmi("Supermicro", "SYS-821GE-TNHR", "American Megatrends International, LLC.")
	f.cpu(c)
	f.cpufreq("intel_pstate", "powersave", "balance_performance", "intel_idle")
	f.governors(8, "powersave")
	f.wl("sys/devices/system/cpu/cpu6/cpufreq/scaling_governor", "performance")
	f.wl("sys/devices/system/cpu/cpu7/cpufreq/scaling_governor", "performance")
	f.numa(c, 1, 65536000)
	f.mem(65536000, 4194300, "madvise", "madvise", 0)
	f.vulns(map[string]string{
		"spectre_v2": "Mitigation: Enhanced / Automatic IBRS; IBPB: conditional; RSB filling",
		"meltdown":   "Not affected",
		"mds":        "Not affected",
	})
	f.w("proc/swaps", "Filename\t\t\t\tType\t\tSize\t\tUsed\t\tPriority\n/dev/zram0                              partition\t4194300\t\t0\t\t100\n/swapfile                               file\t\t2097148\t\t0\t\t-2\n")
	f.sysctl(map[string]string{
		"fs.inotify.max_user_watches":        "8192",
		"fs.inotify.max_user_instances":      "128",
		"net.bridge.bridge-nf-call-iptables": "1",
	})
	// Two GPUs and two NVSwitch bridges.
	for i, a := range []string{"0000:18:00.0", "0000:2a:00.0"} {
		f.pci(pciSpec{addr: a, vendor: "0x10de", device: "0x2330", class: "0x030200", driver: "nvidia", numa: 0,
			speed: "32.0 GT/s PCIe", maxSpeed: "32.0 GT/s PCIe", width: 16, maxWidth: 16, group: 40 + i})
	}
	for _, a := range []string{"0000:05:00.0", "0000:06:00.0"} {
		f.pci(pciSpec{addr: a, vendor: "0x10de", device: "0x22a3", class: "0x068000", driver: "nvidia", numa: 0})
	}
	// RoCE netdev reachable only through infiniband/mlx5_0/device/net/roce0.
	f.rdma("mlx5_0", "4: ACTIVE", "Ethernet", "100 Gb/sec (4X EDR)", "roce0")
	f.nic(nicSpec{name: "eth0", driver: "ice", vendor: "0x8086", speed: 25000, mtu: 1500, numa: 0, queues: 4})
	f.modules("nvidia", "nvidia_uvm", "mlx5_ib", "mlx5_core", "ib_core", "nvme")
	f.proc(procSpec{pid: 910, comm: "nvidia-persiste", argv: []string{"/usr/bin/nvidia-persistenced", "--verbose"},
		nofile: "1024", nofileHard: "524288", unit: "nvidia-persistenced.service", fds: 4})
	f.proc(procSpec{pid: 4120, comm: "python3",
		argv:   []string{"python3", "-m", "vllm.entrypoints.openai.api_server", "--model", "m"},
		nofile: "1048576", nofileHard: "1048576", fds: 8})
	f.cgroup(4120, "/system.slice/docker-"+"3f2a9c1d7b6e4a58b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f2a3"+".scope")
	f.proc(procSpec{pid: 4200, comm: "ollama", argv: []string{"/usr/bin/ollama", "serve"}, nofile: "1024", fds: 4})
	f.cgroup(4200, "/kubepods.slice/kubepods-burstable.slice/cri-containerd-"+strconv.Itoa(1234567)+".scope")
	f.listeners(22, 8000, 11434)
	return f
}
