# Notes (fetched 2026-11-16, all verified by reading the page text)

Rules: `app.cilium.min_kernel` (report).

## Verified
- system_requirements: "Linux kernel >= 5.10 or equivalent (e.g., 4.18 on RHEL 8.10)"; "it is recommended to use kernel 5.10 or later"; newer kernels enable extra features that Cilium detects at start.
- Required kernel config listed (CONFIG_BPF, BPF_SYSCALL, BPF_JIT, CGROUP_BPF, DEBUG_INFO_BTF, ...), plus extras for iptables/ipset, VXLAN/GENEVE, L7 proxy, IPsec.
- performance tuning page: Bandwidth Manager sets up FQ qdiscs and "optimal server-grade sysctl settings" itself; netkit needs kernel >= 6.8. App/Helm-level, no host rule.

## Not documented / not ruled (gaps)
- rp_filter: not mentioned on the system requirements, troubleshooting or tuning pages fetched; no rule.
- BPF JIT is given only as `CONFIG_BPF_JIT=y`, not as the `net.core.bpf_jit_enable` sysctl; no rule.
- Kernel config options cannot be checked: facts do not read /proc/config.gz or /boot/config-*. Engine gap.
- No LimitNOFILE, memlock or inotify guidance.
