# Notes (fetched 2026-11-16, all verified by reading the page text)

Rules: `app.kubelet.ip_forward`, `app.kubelet.swap_enabled`.

## Verified
- container-runtimes page: "By default, the Linux kernel does not allow IPv4 packets to be routed between interfaces. Most Kubernetes cluster networking implementations will change this setting (if needed), but some might expect the administrator to do it for them." Gives `net.ipv4.ip_forward = 1` in /etc/sysctl.d/k8s.conf.
- install-kubeadm page: "The default behavior of a kubelet is to fail to start if swap memory is detected on a node"; tolerate with `failSwapOn: false`; "even if failSwapOn: false is provided, workloads wouldn't have swap access by default"; set a `swapBehavior` other than NoSwap to use it; disable with `swapoff -a` plus /etc/fstab or systemd.swap.
- swap-memory-management page: behaviors NoSwap (default) and LimitedSwap; with NoSwap and failSwapOn false, "systemd services (and even the kubelet itself!) can utilize swap"; recommends `memory.swap.max=0` for the system slice and prioritising its I/O; kernel supports the `noswap` tmpfs option from 6.3.
- Swap history, from the official Kubernetes 1.28 blog: "The 1.22 release introduced Alpha support"; in 1.28 swap support "graduated to Beta", enabled by the NodeSwap feature gate. The concept pages above do not state version history themselves.
- sysctl-cluster page: safe vs unsafe sysctls; `net.core.somaxconn` is unsafe and needs `--allowed-unsafe-sysctls`. Context only, no rule (pod-level configuration with no host-state check).

## Not documented (gaps, no rules)
- br_netfilter / net.bridge.bridge-nf-call-iptables: the current container-runtimes page no longer mentions them; only ip_forward is given. No rule.
- No kubelet LimitNOFILE, inotify (fs.inotify.*), vm.* or THP guidance in the fetched pages.
- failSwapOn / swapBehavior live in the kubelet config file, which kerneltuner cannot read; the swap rule therefore only reports swap presence.
