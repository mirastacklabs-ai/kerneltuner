package main

import (
	"fmt"
	"strconv"
)

var stockSysctl = map[string]string{
	"net.core.somaxconn":                        "4096",
	"net.core.netdev_max_backlog":               "1000",
	"net.core.netdev_budget":                    "300",
	"net.core.rmem_default":                     "212992",
	"net.core.rmem_max":                         "212992",
	"net.core.wmem_default":                     "212992",
	"net.core.wmem_max":                         "212992",
	"net.core.optmem_max":                       "20480",
	"net.core.busy_poll":                        "0",
	"net.core.busy_read":                        "0",
	"net.core.default_qdisc":                    "fq_codel",
	"net.ipv4.tcp_congestion_control":           "cubic",
	"net.ipv4.tcp_available_congestion_control": "reno cubic",
	"net.ipv4.tcp_rmem":                         "4096 131072 6291456",
	"net.ipv4.tcp_wmem":                         "4096 16384 4194304",
	"net.ipv4.tcp_max_syn_backlog":              "1024",
	"net.ipv4.tcp_syncookies":                   "1",
	"net.ipv4.tcp_tw_reuse":                     "2",
	"net.ipv4.tcp_fin_timeout":                  "60",
	"net.ipv4.tcp_slow_start_after_idle":        "1",
	"net.ipv4.tcp_timestamps":                   "1",
	"net.ipv4.tcp_sack":                         "1",
	"net.ipv4.tcp_window_scaling":               "1",
	"net.ipv4.tcp_keepalive_time":               "7200",
	"net.ipv4.tcp_keepalive_intvl":              "75",
	"net.ipv4.tcp_keepalive_probes":             "9",
	"net.ipv4.tcp_max_tw_buckets":               "262144",
	"net.ipv4.tcp_mtu_probing":                  "0",
	"net.ipv4.tcp_fastopen":                     "1",
	"net.ipv4.tcp_notsent_lowat":                "4294967295",
	"net.ipv4.tcp_retries2":                     "15",
	"net.ipv4.tcp_syn_retries":                  "6",
	"net.ipv4.tcp_synack_retries":               "5",
	"net.ipv4.tcp_abort_on_overflow":            "0",
	"net.ipv4.tcp_moderate_rcvbuf":              "1",
	"net.ipv4.tcp_max_orphans":                  "65536",
	"net.ipv4.ip_local_port_range":              "32768 60999",
	"net.ipv4.ip_forward":                       "0",
	"net.ipv4.conf.all.rp_filter":               "2",
	"vm.swappiness":                             "60",
	"vm.dirty_ratio":                            "20",
	"vm.dirty_background_ratio":                 "10",
	"vm.dirty_expire_centisecs":                 "3000",
	"vm.dirty_writeback_centisecs":              "500",
	"vm.overcommit_memory":                      "0",
	"vm.overcommit_ratio":                       "50",
	"vm.max_map_count":                          "65530",
	"vm.min_free_kbytes":                        "67584",
	"vm.vfs_cache_pressure":                     "100",
	"vm.nr_hugepages":                           "0",
	"vm.zone_reclaim_mode":                      "0",
	"vm.watermark_scale_factor":                 "10",
	"kernel.pid_max":                            "4194304",
	"kernel.threads-max":                        "1028760",
	"kernel.sched_autogroup_enabled":            "1",
	"kernel.randomize_va_space":                 "2",
	"kernel.perf_event_paranoid":                "4",
	"kernel.sysrq":                              "176",
	"kernel.shmmax":                             "18446744073692774399",
	"kernel.shmall":                             "18446744073692774399",
	"fs.nr_open":                                "1048576",
	"fs.aio-max-nr":                             "65536",
	"fs.pipe-max-size":                          "1048576",
	"fs.suid_dumpable":                          "2",
}

func base() *fx {
	f := newFx("base-linux", "Shared base: stock kernel 5.15 sysctl defaults, systemd and sshd only", "")
	f.kernel("5.15.0-91-generic", "BOOT_IMAGE=/boot/vmlinuz-5.15.0-91-generic root=/dev/nvme0n1p2 ro quiet splash")
	f.sysctl(stockSysctl)
	f.fileNr(2144, 9223372036854775807)
	return f
}

// host adds the processes and listener every non-container host has, plus the
// sysctls that exist on kernels newer than 4.9.
func (f *fx) host(modern bool) {
	f.proc(systemd(1))
	f.proc(sshd(701))
	f.listeners(22)
	f.w("proc/net/tcp6", "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	if modern {
		f.sysctl(map[string]string{"net.core.netdev_budget_usecs": "2000", "kernel.numa_balancing": "0"})
	}
}

func web() *fx {
	f := newFx("web-16c-64g-nginx", "Bare-metal web tier: 8c/16t Xeon, 64 GiB, 25G mlx5, Ubuntu 22.04 kernel 5.15, nginx 16 workers on 80/443, stock sysctls", "base-linux")
	f.host(true)
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) E-2388G CPU @ 3.20GHz", family: 6, modelNum: 167, step: 1,
		sockets: 1, cores: 8, tpc: 2, flags: flagsBase + flagsAVX512, mhz: "3200.000", cacheKB: 16384}
	f.dmi("Dell Inc.", "PowerEdge R250", "Dell Inc.")
	f.cpu(c)
	f.cpufreq("intel_pstate", "powersave", "balance_performance", "intel_idle")
	f.numa(c, 1, 65536000)
	f.mem(65536000, 8388604, "madvise", "madvise", 0)
	f.nic(nicSpec{name: "eth0", driver: "mlx5_core", vendor: "0x15b3", speed: 25000, mtu: 1500, numa: 0, queues: 16,
		ring: &ringSpec{comb: 16, combMax: 63, rx: 1024, rxMax: 8192, tx: 1024, txMax: 8192}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	f.modules("nf_conntrack", "nf_defrag_ipv4", "mlx5_core", "mlx5_ib", "ib_core", "nvme", "xfs")
	f.sysctl(map[string]string{
		"net.netfilter.nf_conntrack_max":                     "262144",
		"net.netfilter.nf_conntrack_buckets":                 "65536",
		"net.netfilter.nf_conntrack_tcp_timeout_established": "432000",
		"net.netfilter.nf_conntrack_count":                   "1830",
	})
	f.fileNr(5120, 9223372036854775807)
	f.proc(procSpec{pid: 1203, comm: "nginx", argv: []string{"nginx: master process /usr/sbin/nginx -g daemon on; master_process on;"},
		nofile: "1024", nofileHard: "524288", unit: "nginx.service", fds: 9})
	for i := 0; i < 16; i++ {
		f.proc(procSpec{pid: 1204 + i, comm: "nginx", argv: []string{"nginx: worker process"},
			nofile: "1024", nofileHard: "524288", unit: "nginx.service", fds: 14})
	}
	f.listeners(22, 80, 443)
	f.values["conntrack.count"] = 1830
	f.values["conntrack.max"] = 262144
	f.values["sockstat.tcp.tw"] = 410
	return f
}

func legacy() *fx {
	f := newFx("legacy-4.9-c10k", "Debian 9 kernel 4.9 C10K proxy: HAProxy, ixgbe 10G, somaxconn 128, tcp_tw_recycle=1, tcp_timestamps=0, cubic only", "base-linux")
	f.host(false)
	f.kernel("4.9.0-19-amd64", "BOOT_IMAGE=/boot/vmlinuz-4.9.0-19-amd64 root=/dev/sda1 ro quiet")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) CPU E3-1270 v6 @ 3.80GHz", family: 6, modelNum: 158, step: 9,
		sockets: 1, cores: 4, tpc: 2, flags: flagsBase, mhz: "3800.000", cacheKB: 8192}
	f.dmi("Supermicro", "X11SSH-F", "American Megatrends Inc.")
	f.cpu(c)
	f.cpufreq("intel_pstate", "powersave", "", "intel_idle")
	f.numa(c, 1, 32768000)
	f.mem(32768000, 4194300, "always", "always", 0)
	f.nic(nicSpec{name: "eth0", driver: "ixgbe", vendor: "0x8086", speed: 10000, mtu: 1500, numa: 0, queues: 8,
		ring: &ringSpec{comb: 8, combMax: 63, rx: 512, rxMax: 8192, tx: 512, txMax: 8192}})
	f.block("sda", false, "cfq", schedLegacy, 128, 128)
	f.modules("nf_conntrack", "nf_conntrack_ipv4", "ixgbe", "ahci", "ext4")
	f.sysctl(map[string]string{
		"net.core.somaxconn":                                 "128",
		"net.core.netdev_max_backlog":                        "1000",
		"net.ipv4.tcp_tw_recycle":                            "1",
		"net.ipv4.tcp_tw_reuse":                              "0",
		"net.ipv4.tcp_timestamps":                            "0",
		"net.ipv4.tcp_max_syn_backlog":                       "512",
		"net.ipv4.tcp_available_congestion_control":          "cubic reno",
		"net.ipv4.tcp_congestion_control":                    "cubic",
		"net.ipv4.tcp_adv_win_scale":                         "1",
		"net.netfilter.nf_conntrack_max":                     "65536",
		"net.netfilter.nf_conntrack_buckets":                 "16384",
		"net.netfilter.nf_conntrack_count":                   "48210",
		"net.netfilter.nf_conntrack_tcp_timeout_established": "432000",
	})
	f.fileNr(31800, 6553600)
	f.proc(procSpec{pid: 902, comm: "haproxy", argv: []string{"/usr/sbin/haproxy", "-Ws", "-f", "/etc/haproxy/haproxy.cfg", "-p", "/run/haproxy.pid"},
		nofile: "4096", nofileHard: "4096", unit: "haproxy.service", fds: 40})
	f.proc(procSpec{pid: 904, comm: "haproxy", argv: []string{"/usr/sbin/haproxy", "-Ws", "-f", "/etc/haproxy/haproxy.cfg", "-p", "/run/haproxy.pid"},
		nofile: "4096", nofileHard: "4096", unit: "haproxy.service", fds: 96})
	f.listeners(22, 80, 443)
	f.values["conntrack.count"] = 48210
	f.values["conntrack.max"] = 65536
	f.values["sockstat.tcp.tw"] = 21500
	f.rates["TcpExt.ListenOverflows"] = 38.5
	f.rates["TcpExt.ListenDrops"] = 38.5
	return f
}

func k8sRedis() *fx {
	f := newFx("container-k8s-redis", "Kubernetes pod on EKS (containerd, cgroup v2): redis-server as pid 1, kernel 6.1, THP always, veth eth0", "base-linux")
	f.kernel("6.1.102-108.177.amzn2023.x86_64", "BOOT_IMAGE=(hd0,gpt1)/boot/vmlinuz-6.1.102-108.177.amzn2023.x86_64 root=UUID=00000000-0000-0000-0000-000000000000 ro console=tty0 console=ttyS0,115200n8 nvme_core.io_timeout=4294967295 selinux=1 security=selinux")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) Platinum 8375C CPU @ 2.90GHz", family: 6, modelNum: 106, step: 6,
		sockets: 1, cores: 4, tpc: 2, flags: flagsBase + flagsAVX512, mhz: "2899.998", cacheKB: 55296, hyper: true}
	f.dmi("Amazon EC2", "m6i.2xlarge", "Amazon EC2")
	f.cpu(c)
	f.numa(c, 1, 32768000)
	f.mem(32768000, 0, "always", "madvise", 0)
	f.nic(nicSpec{name: "eth0", mtu: 8951, queues: 1, speed: 10000})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	f.sysctl(map[string]string{"net.core.somaxconn": "4096", "net.ipv4.tcp_max_syn_backlog": "4096", "vm.overcommit_memory": "0"})
	f.fileNr(960, 9223372036854775807)
	// The container sees only its own processes: redis is pid 1.
	f.w("proc/1/comm", "redis-server\n")
	f.w("proc/1/cmdline", "redis-server\x00*:6379\x00")
	f.w("proc/1/limits", `Limit                     Soft Limit           Hard Limit           Units
Max cpu time              unlimited            unlimited            seconds
Max file size             unlimited            unlimited            bytes
Max stack size            8388608              unlimited            bytes
Max processes             unlimited            unlimited            processes
Max open files            1048576              1048576              files
Max locked memory         65536                65536                bytes
`)
	f.w("proc/1/cgroup", "0::/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod5e0f1b8a_3c2d_4f1e_9a7b_0c1d2e3f4a5b.slice/cri-containerd-8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a.scope\n")
	for i := 0; i < 24; i++ {
		f.w(fmt.Sprintf("proc/1/fd/%d", i), "")
	}
	f.w("proc/1/environ", "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\x00")
	f.listeners(6379)
	f.w("proc/net/tcp6", "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n")
	return f
}

func iommuGroupsFor(start, n int) []int {
	var g []int
	for i := 0; i < n; i++ {
		g = append(g, start+i)
	}
	return g
}

func h100() *fx {
	f := newFx("gpu-8xh100", "2-socket Xeon 8462Y+ HGX, 8x H100 SXM5 (one at x8), 8x ConnectX-7 NDR IB, Ubuntu 24.04 kernel 6.8, vLLM, no iommu=pt, numa_balancing=1, THP always, no nvidia_peermem", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro intel_iommu=on quiet splash")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) Platinum 8462Y+", family: 6, modelNum: 143, step: 8,
		sockets: 2, cores: 32, tpc: 2, flags: flagsBase + flagsAVX512 + flagsBF16 + flagsAMX, mhz: "2800.000", cacheKB: 61440}
	f.dmi("Supermicro", "SYS-821GE-TNHR", "American Megatrends International, LLC.")
	f.cpu(c)
	f.cpufreq("intel_pstate", "performance", "performance", "intel_idle")
	f.numa(c, 2, 2113756160)
	f.mem(2113756160, 0, "always", "madvise", 0)
	f.sysctl(map[string]string{"kernel.numa_balancing": "1"})
	f.nic(nicSpec{name: "eno1np0", driver: "ice", vendor: "0x8086", speed: 25000, mtu: 1500, numa: 0, queues: 32,
		ring: &ringSpec{comb: 32, combMax: 64, rx: 512, rxMax: 8160, tx: 512, txMax: 8160}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	for i := 1; i <= 4; i++ {
		f.block("nvme"+strconv.Itoa(i)+"n1", false, "none", schedMQ, 128, 1023)
	}
	gpuAddrs := []string{"0000:18:00.0", "0000:2a:00.0", "0000:3a:00.0", "0000:5d:00.0", "0000:9a:00.0", "0000:ab:00.0", "0000:ba:00.0", "0000:db:00.0"}
	for i, a := range gpuAddrs {
		node := i / 4
		w := 16
		if i == 5 {
			w = 8
		}
		f.pci(pciSpec{addr: a, vendor: "0x10de", device: "0x2330", class: "0x030200", driver: "nvidia", numa: node,
			speed: "32.0 GT/s PCIe", maxSpeed: "32.0 GT/s PCIe", width: w, maxWidth: 16, group: 40 + i})
	}
	f.iommu("dmar0", append(iommuGroupsFor(40, 8), iommuGroupsFor(60, 8)...), "DMA-FQ")
	ibNet := []string{"ibp24s0", "ibp41s0", "ibp58s0", "ibp94s0", "ibp154s0", "ibp171s0", "ibp186s0", "ibp219s0"}
	for i := 0; i < 8; i++ {
		f.rdma("mlx5_"+strconv.Itoa(i), "4: ACTIVE", "InfiniBand", "400 Gb/sec (4X NDR)", ibNet[i])
		f.nic(nicSpec{name: ibNet[i], driver: "mlx5_core", vendor: "0x15b3", noSpeed: true, mtu: 2044, numa: i / 4, queues: 8})
	}
	f.modules("nvidia", "nvidia_uvm", "nvidia_modeset", "nvidia_drm", "mlx5_ib", "mlx5_core", "ib_uverbs", "ib_core", "rdma_cm", "ib_ipoib", "nvme")
	f.proc(procSpec{pid: 755, comm: "irqbalance", argv: []string{"/usr/sbin/irqbalance", "--foreground"}, nofile: "1024", nofileHard: "524288", unit: "irqbalance.service", fds: 6})
	f.proc(procSpec{pid: 4120, comm: "python3",
		argv:   []string{"python3", "-m", "vllm.entrypoints.openai.api_server", "--model", "meta-llama/Llama-3.1-70B-Instruct", "--tensor-parallel-size", "8"},
		nofile: "1048576", nofileHard: "1048576", memlock: "8388608", memlockHard: "8388608", unit: "vllm.service", fds: 96})
	f.listeners(22, 8000)
	f.fileNr(4096, 9223372036854775807)
	return f
}

func mi300x() *fx {
	f := newFx("gpu-8xmi300x", "2-socket EPYC 9534, 8x MI300X (amdgpu, KFD), 8 RoCE ports, kernel 6.8, sglang, no iommu=pt, numa_balancing=1", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro amd_iommu=on quiet splash")
	c := cpuSpec{vendor: "AuthenticAMD", model: "AMD EPYC 9534 64-Core Processor", family: 25, modelNum: 17, step: 1,
		sockets: 2, cores: 64, tpc: 2, flags: flagsAMD, mhz: "2450.000", cacheKB: 1024}
	f.dmi("Supermicro", "AS -8125GS-TNMR2", "American Megatrends International, LLC.")
	f.cpu(c)
	f.cpufreq("acpi-cpufreq", "schedutil", "", "")
	f.numa(c, 2, 2113756160)
	f.mem(2113756160, 0, "madvise", "madvise", 0)
	f.sysctl(map[string]string{"kernel.numa_balancing": "1"})
	f.nic(nicSpec{name: "enp1s0f0", driver: "i40e", vendor: "0x8086", speed: 10000, mtu: 1500, numa: 0, queues: 16,
		ring: &ringSpec{comb: 16, combMax: 64, rx: 512, rxMax: 8160, tx: 512, txMax: 8160}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	gpus := []string{"0000:05:00.0", "0000:27:00.0", "0000:47:00.0", "0000:65:00.0", "0000:85:00.0", "0000:a7:00.0", "0000:c7:00.0", "0000:e5:00.0"}
	for i, a := range gpus {
		f.pci(pciSpec{addr: a, vendor: "0x1002", device: "0x74a1", class: "0x038000", driver: "amdgpu", numa: i / 4,
			speed: "32.0 GT/s PCIe", maxSpeed: "32.0 GT/s PCIe", width: 16, maxWidth: 16, group: 30 + i})
	}
	f.iommu("ivhd0", iommuGroupsFor(30, 8), "DMA")
	// KFD topology: two CPU nodes then eight GPU nodes.
	for n := 0; n < 10; n++ {
		d := fmt.Sprintf("sys/class/kfd/kfd/topology/nodes/%d/properties", n)
		if n < 2 {
			f.w(d, fmt.Sprintf("cpu_cores_count %d\nsimd_count 0\nvendor_id 0\ndevice_id 0\n", c.cores*c.tpc))
		} else {
			f.w(d, "cpu_cores_count 0\nsimd_count 1216\nvendor_id 4098\ndevice_id 29857\nlocation_id "+strconv.Itoa(n)+"\n")
		}
	}
	for i := 0; i < 8; i++ {
		nd := fmt.Sprintf("ens%df0np0", 10+i)
		f.rdma("mlx5_"+strconv.Itoa(i), "4: ACTIVE", "Ethernet", "400 Gb/sec (4X NDR)", nd)
	}
	f.modules("amdgpu", "amd_iommu_v2", "drm_ttm_helper", "ttm", "mlx5_ib", "mlx5_core", "ib_core", "nvme")
	f.proc(procSpec{pid: 3310, comm: "python3",
		argv:   []string{"python3", "-m", "sglang.launch_server", "--model-path", "meta-llama/Llama-3.1-70B-Instruct", "--tp", "8"},
		nofile: "1048576", nofileHard: "1048576", memlock: "unlimited", memlockHard: "unlimited", unit: "sglang.service", fds: 80})
	f.listeners(22, 30000)
	return f
}

func gaudi3() *fx {
	f := newFx("gaudi3-8x", "2-socket Xeon Platinum 8580, 8x Intel Gaudi 3 (habanalabs), Ubuntu kernel 6.8, schedutil governor", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro intel_iommu=on quiet")
	c := cpuSpec{vendor: "GenuineIntel", model: "INTEL(R) XEON(R) PLATINUM 8580", family: 6, modelNum: 207, step: 2,
		sockets: 2, cores: 60, tpc: 2, flags: flagsBase + flagsAVX512 + flagsBF16 + flagsAMX, mhz: "2000.000", cacheKB: 307200}
	f.dmi("Supermicro", "SYS-822GA-NGR3", "American Megatrends International, LLC.")
	f.cpu(c)
	f.cpufreq("intel_cpufreq", "schedutil", "", "intel_idle")
	f.numa(c, 2, 2113756160)
	f.mem(2113756160, 0, "madvise", "madvise", 0)
	f.sysctl(map[string]string{"kernel.numa_balancing": "1"})
	f.nic(nicSpec{name: "eno1", driver: "ice", vendor: "0x8086", speed: 25000, mtu: 1500, numa: 0, queues: 32,
		ring: &ringSpec{comb: 32, combMax: 64, rx: 512, rxMax: 8160, tx: 512, txMax: 8160}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	addrs := []string{"0000:19:00.0", "0000:2b:00.0", "0000:3d:00.0", "0000:5f:00.0", "0000:9d:00.0", "0000:af:00.0", "0000:bf:00.0", "0000:dd:00.0"}
	for i, a := range addrs {
		f.pci(pciSpec{addr: a, vendor: "0x1da3", device: "0x1060", class: "0x120000", driver: "habanalabs", numa: i / 4,
			speed: "32.0 GT/s PCIe", maxSpeed: "32.0 GT/s PCIe", width: 16, maxWidth: 16, group: 50 + i})
		f.wl(fmt.Sprintf("sys/class/accel/accel%d/device/vendor", i), "0x1da3")
		f.wl(fmt.Sprintf("sys/class/accel/accel%d/device/device", i), "0x1060")
	}
	f.iommu("dmar0", iommuGroupsFor(50, 8), "DMA-FQ")
	f.modules("habanalabs", "habanalabs_en", "habanalabs_cn", "ib_core", "nvme")
	f.listeners(22)
	return f
}

func intelMax() *fx {
	f := newFx("intel-max-1550", "2-socket Xeon Max 9480, 2x Intel Data Center GPU Max 1550 (i915), kernel 6.8", "base-linux")
	f.host(true)
	f.kernel("6.8.0-45-generic", "BOOT_IMAGE=/boot/vmlinuz-6.8.0-45-generic root=/dev/nvme0n1p2 ro intel_iommu=on quiet")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) CPU Max 9480", family: 6, modelNum: 143, step: 8,
		sockets: 2, cores: 56, tpc: 2, flags: flagsBase + flagsAVX512 + flagsBF16 + flagsAMX, mhz: "1900.000", cacheKB: 112640}
	f.dmi("Intel Corporation", "M50FCP2SBSTD", "Intel Corporation")
	f.cpu(c)
	f.cpufreq("intel_pstate", "performance", "performance", "intel_idle")
	f.numa(c, 2, 1056000000)
	f.mem(1056000000, 0, "madvise", "madvise", 0)
	f.sysctl(map[string]string{"kernel.numa_balancing": "1"})
	f.nic(nicSpec{name: "eno1", driver: "ice", vendor: "0x8086", speed: 25000, mtu: 1500, numa: 0, queues: 32,
		ring: &ringSpec{comb: 32, combMax: 64, rx: 512, rxMax: 8160, tx: 512, txMax: 8160}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	for i, a := range []string{"0000:29:00.0", "0000:bd:00.0"} {
		f.pci(pciSpec{addr: a, vendor: "0x8086", device: "0x0bd5", class: "0x038000", driver: "i915", numa: i,
			speed: "16.0 GT/s PCIe", maxSpeed: "16.0 GT/s PCIe", width: 16, maxWidth: 16, group: 70 + i})
	}
	f.iommu("dmar0", iommuGroupsFor(70, 2), "DMA-FQ")
	f.modules("i915", "drm_buddy", "ttm", "nvme")
	f.listeners(22)
	return f
}

func amx() *fx {
	f := newFx("cpu-amx-2s", "2-socket Sapphire Rapids Gold 6430 with AMX, kernel 6.5, llama-server CPU inference, THP always, numa_balancing=1, powersave/balance_power", "base-linux")
	f.host(true)
	f.kernel("6.5.0-44-generic", "BOOT_IMAGE=/boot/vmlinuz-6.5.0-44-generic root=/dev/nvme0n1p2 ro quiet splash")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) Gold 6430", family: 6, modelNum: 143, step: 8,
		sockets: 2, cores: 32, tpc: 2, flags: flagsBase + flagsAVX512 + flagsBF16 + flagsAMX, mhz: "2100.000", cacheKB: 61440}
	f.dmi("Dell Inc.", "PowerEdge R760", "Dell Inc.")
	f.cpu(c)
	f.cpufreq("intel_pstate", "powersave", "balance_power", "intel_idle")
	f.numa(c, 2, 528000000)
	f.mem(528000000, 8388604, "always", "madvise", 0)
	f.sysctl(map[string]string{"kernel.numa_balancing": "1"})
	f.nic(nicSpec{name: "eno1np0", driver: "bnxt_en", vendor: "0x14e4", speed: 25000, mtu: 1500, numa: 0, queues: 16,
		ring: &ringSpec{comb: 16, combMax: 74, rx: 511, rxMax: 8191, tx: 511, txMax: 8191}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	f.modules("bnxt_en", "nvme", "xfs")
	f.proc(procSpec{pid: 2210, comm: "llama-server",
		argv:   []string{"/opt/llama.cpp/build/bin/llama-server", "-m", "/models/llama-3.1-70b-q4_k_m.gguf", "--port", "8080", "-t", "64"},
		nofile: "1024", nofileHard: "524288", memlock: "8388608", memlockHard: "8388608", unit: "llama-server.service", fds: 20})
	f.listeners(22, 8080)
	return f
}

func bpftuneTuned() *fx {
	f := newFx("bpftune-tuned-host", "Kernel 6.6 host running bpftune and tuned (profile throughput-performance)", "base-linux")
	f.host(true)
	f.kernel("6.6.30-1.el9.elrepo.x86_64", "BOOT_IMAGE=(hd0,msdos1)/vmlinuz-6.6.30-1.el9.elrepo.x86_64 root=/dev/mapper/rl-root ro crashkernel=1G-4G:192M resume=/dev/mapper/rl-swap rd.lvm.lv=rl/root rd.lvm.lv=rl/swap")
	c := cpuSpec{vendor: "GenuineIntel", model: "Intel(R) Xeon(R) Silver 4410Y", family: 6, modelNum: 143, step: 8,
		sockets: 1, cores: 12, tpc: 2, flags: flagsBase + flagsAVX512 + flagsBF16 + flagsAMX, mhz: "2000.000", cacheKB: 30720}
	f.dmi("HPE", "ProLiant DL360 Gen11", "HPE")
	f.cpu(c)
	f.cpufreq("intel_pstate", "performance", "performance", "intel_idle")
	f.numa(c, 1, 131072000)
	f.mem(131072000, 4194300, "madvise", "madvise", 0)
	f.nic(nicSpec{name: "ens1f0", driver: "ice", vendor: "0x8086", speed: 25000, mtu: 1500, numa: 0, queues: 24,
		ring: &ringSpec{comb: 24, combMax: 64, rx: 512, rxMax: 8160, tx: 512, txMax: 8160}})
	f.block("nvme0n1", false, "none", schedMQ, 128, 1023)
	f.modules("ice", "nvme", "xfs", "tcp_bbr")
	f.sysctl(map[string]string{"net.ipv4.tcp_available_congestion_control": "reno cubic bbr"})
	f.wl("etc/tuned/active_profile", "throughput-performance")
	f.proc(procSpec{pid: 880, comm: "tuned", argv: []string{"/usr/bin/python3", "-Es", "/usr/sbin/tuned", "-l", "-P"}, unit: "tuned.service", fds: 8})
	f.proc(procSpec{pid: 901, comm: "bpftune", argv: []string{"/usr/sbin/bpftune", "-s"}, unit: "bpftune.service", fds: 30})
	f.listeners(22)
	return f
}
