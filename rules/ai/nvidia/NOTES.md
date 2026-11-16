# NVIDIA / RDMA pack notes (2026-11-16)

Facts verified by fetch in this pass (beyond docs/research/ai-accel-research.md):

- PCI sysfs `current_link_speed`, `current_link_width`, `max_link_speed`, `max_link_width` exist as device attributes:
  `drivers/pci/pci-sysfs.c` in the kernel tree (source, not ABI docs). They are NOT in Documentation/ABI/testing/sysfs-bus-pci
  or the admin-guide ABI listing (grepped, empty), so no kernel-version history is claimed.
- NVIDIA DCGM diagnostics page (docs.nvidia.com/datacenter/dcgm/latest/user-guide/dcgm-diagnostics.html): example H100
  PCIe config sets `pcie: h2d_d2h_single_pinned: min_pci_generation: 3.0, min_pci_width: 16.0`. Basis for the width-only
  link rule. Idle speed downtraining is still UNVERIFIED, so speed is not evaluated.
- vLLM Docker page: "either the ipc=host flag or --shm-size flag" lets the container use host shared memory; vLLM uses
  PyTorch shared memory between processes, particularly for tensor-parallel inference. This resolves the earlier
  "UNVERIFIED on official text" note for the ipc=host rationale.
- NCCL env page: NCCL_IB_HCA (comma list `<hca>[:<port>...]`, `^` exclude, `=` exact), NCCL_SOCKET_IFNAME (prefix list),
  NCCL_NET_GDR_LEVEL (LOC/PIX/PXB/PHB/SYS; auto-selected when unset).
- GPUDirect RDMA page: IOMMU must be disabled or in pass-through; nvidia-peermem gives Mellanox IB HCAs direct access to
  GPU memory; loaded by `modprobe nvidia-peermem`.

Engine observations (not patched):

- `service` rules match `Proc.Comm == name || Unit == unit`. Kernel comm is truncated to 15 chars, so
  `nvidia-persistenced` shows as `nvidia-persiste`; only the systemd unit match works.
- `workload.App.Unit` is only set from a `.service` cgroup element, so containerised apps (docker/containerd scopes)
  have an empty Unit. `ai.nvidia.vllm-container-ipc` uses "no unit" as a proxy for container or ad-hoc launch.

Skipped (UNVERIFIED): Fabric Manager rule (NVSwitch PCI class detection unverified; Classify drops 0x0680 bridges),
GPU-to-NUMA numactl binding advice (no fetched source recommends it), NVIDIA-specific memlock text, SGLang/TGI shm
(SGLang example is AMD-only; TGI quickstart gives `--shm-size 1g` with no rationale), ACS guidance for NVIDIA, RoCE
PFC/ECN host prerequisites.
