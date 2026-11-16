# AI accelerator host tuning: research for kerneltuner rule packs
Date 2026-11-16. Verification pass done by curl; raw pages under the session scratchpad fetch/ dir.
Legend: `verified-by-fetch` = page fetched HTTP 200 and text supports the claim (URL listed). UNVERIFIED = not confirmed from a fetched page; do not cite until fetched.

## 0. Generic detection (all vendors)
- PCI scan of /sys/bus/pci/devices/*/{vendor,device,class,numa_node,local_cpulist,iommu_group}.
  - numa_node semantics: verified-by-fetch https://raw.githubusercontent.com/torvalds/linux/master/Documentation/ABI/testing/sysfs-bus-pci (also https://docs.kernel.org/admin-guide/abi-testing-files.html). vendor/class/local_cpus attrs: verified-by-fetch https://docs.kernel.org/PCI/sysfs-pci.html
  - current_link_speed/max_link_speed/current_link_width/max_link_width: NOT found in fetched ABI docs (grep empty). UNVERIFIED as documented ABI. Idle-downtrain caveat UNVERIFIED.
  - AMD documents a link check: `lspci -d 1002:74a1 -vvv | grep -e DevSta -e LnkSta`, pass = 32GT/s, x16, no FatalErr+. verified-by-fetch https://rocm.docs.amd.com/en/latest/how-to/system-optimization/mi300x.html
  - Vendor IDs (0x10de, 0x1002, 0x8086, 0x1da3) and class codes (0x0300/0x0302/0x0380/0x1200): UNVERIFIED.
- numa_balancing sysctl: verified-by-fetch https://docs.kernel.org/admin-guide/sysctl/kernel.html
- THP: verified-by-fetch https://docs.kernel.org/admin-guide/mm/transhuge.html (system-wide or madvise-restricted). sysfs path text itself not grepped: UNVERIFIED.

## 1. NVIDIA GPU hosts
### 1.1 Persistence  verified-by-fetch
- Without persistence, GPU state unloads when all clients close the device file; nvidia-persistenced holds device files open. Legacy persistence mode will eventually be deprecated/removed; nvidia-smi uses the daemon RPC if running, else legacy. Minimal overhead once the daemon runs.
- Rule: prefer nvidia-persistenced service over `nvidia-smi -pm 1`. Detect: systemctl is-active / process, or nvidia-smi -q (optional CLI).
- Daemon default start mode: UNVERIFIED (man pages conflicted).
- Source: https://docs.nvidia.com/deploy/driver-persistence/persistence-daemon.html
### 1.2 IOMMU / GPUDirect RDMA  verified-by-fetch
- GPUDirect RDMA needs identical physical addresses across PCI devices; incompatible with IOMMU translation other than 1:1, so IOMMU must be disabled or in pass-through. Detect /proc/cmdline, /sys/class/iommu. Risk: less DMA isolation.
- Source: https://docs.nvidia.com/cuda/gpudirect-rdma/index.html (section "IOMMUs")
- ACS: no ACS text on NVIDIA page. AMD says disable ACS for multi-node RCCL, check `lspci -vvv | grep -i acsctl` (verified-by-fetch https://rocm.docs.amd.com/en/docs-7.0.0/how-to/rocm-for-ai/inference-optimization/workload.html). NVIDIA ACS guidance UNVERIFIED. BIOS/setpci: report-only.
### 1.3 nvidia-peermem  verified-by-fetch
- Ships with GPU driver; load manually (`modprobe nvidia-peermem`); no service autoloads it. Needs kernel RDMA peer-memory support via MLNX_OFED (or patched kernel); if GPU driver installed before MLNX_OFED, reinstall the GPU driver. Replaces nv_peer_mem (stop/unload it first).
- Detect: lsmod / /proc/modules. Source: https://docs.nvidia.com/cuda/gpudirect-rdma/index.html (section 4.7)
### 1.4 Fabric Manager  verified-by-fetch (partial)
- `systemctl enable|start|status nvidia-fabricmanager`. For driver releases before 620 the FM package version must match the driver package; from 620 FM is versioned standalone. HGX H100 needs driver 525.xx+; HGX B200/B300 need OFED/MOFED.
- Failure symptom (CUDA error 802) and NVSwitch PCI class detection: UNVERIFIED.
- Source: https://docs.nvidia.com/datacenter/tesla/fabric-manager-user-guide/index.html
### 1.5 UNVERIFIED this pass
- PCIe link degradation (see 0), `nvidia-smi topo -m`, NVIDIA-specific memlock text, THP/hugepage guidance, NVIDIA-side numa_balancing, MIG/clocks, sysfs GPU detection by vendor/class.

## 2. AMD ROCm (MI200/MI300X)
- The fetched MI300X "system-optimization" page is now the Customer Acceptance Guide; old tuning-guide URLs redirect to it. It has NO THP, governor, NPS or amdgpu module-param guidance. Those items are UNVERIFIED.
- Kernel cmdline  verified-by-fetch: /proc/cmdline must contain `pci=realloc=off`, `amd_iommu=on` or `intel_iommu=on`, and `iommu=pt`. Also: no amdgpu errors in dmesg, >=1.5T memory, all 8 GPUs in lspci. Source: https://rocm.docs.amd.com/en/latest/how-to/system-optimization/mi300x.html
- SGLang AMD doc agrees: GRUB_CMDLINE_LINUX `pci=realloc=off iommu=pt`, update-grub, reboot  verified-by-fetch https://docs.sglang.io/docs/hardware-platforms/amd_gpu.md
- numa_balancing=0  verified-by-fetch: check /proc/sys/kernel/numa_balancing; `sudo sysctl kernel.numa_balancing=0`; permanent via `numa_balancing=0` in GRUB_CMDLINE_LINUX_DEFAULT (AMD wording). Reason: less variability / RCCL hangs. Sources: https://rocm.docs.amd.com/en/docs-7.0.0/how-to/rocm-for-ai/inference-optimization/workload.html ; https://instinct.docs.amd.com/projects/gpu-cluster-networking/en/latest/how-to/troubleshooting.html
- memlock/nofile  verified-by-fetch: defaults can be too small (ib_write_bw "failed to create QP", RCCL hangs/segfaults, hipMalloc fails). /etc/security/limits.conf: `* soft/hard memlock unlimited`, `* soft/hard nofile 1048576`, re-login. Source: troubleshooting URL above.
- MTU  verified-by-fetch: 1500 default limits performance; host 9000 (supports RDMA 4096 + TCP jumbo), switch max. Same URL.
- MaxReadReq/MaxPayload to max; MaxReadReq changeable at runtime via setpci  verified-by-fetch https://instinct.docs.amd.com/projects/gpu-cluster-networking/en/latest/how-to/single-node-config.html
- NPS modes (BIOS, report-only), AMD CPU governor/C-states, THP, amdgpu module params, /sys/class/kfd topology: UNVERIFIED.
- Detection: lspci lists "Processing accelerators ... [Instinct MI300X]"  verified-by-fetch (MI300X page).

## 3. Intel
### 3.1 Gaudi  verified-by-fetch unless noted
- `habanalabs-installer.sh install --type base`; "The installation sets the number of huge pages automatically" (do not add a manual nr_hugepages rule). Source: https://docs.habana.ai/en/latest/Installation_Guide/Bare_Metal_Fresh_OS.html (same text in Driver_Installation.html)
- IOMMU passthrough required only on Ubuntu 24.04.2/22.04.5 with kernel 6.8: `iommu=pt intel_iommu=on`. Same page.
- CPU governor `performance` via /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor, set on bare metal before starting containers; BIOS must expose ACPI cpufreq. Same page.
- Gaudi 3 CPU settings: Sapphire/Emerald Rapids `wrmsr -a 0x1b0 0x0`, `wrmsr -a 0x774 0x2708`; Granite Rapids: `echo 1 | tee /sys/devices/system/cpu/cpu*/cpuidle/state2/disable` (check cpuidle/state*/name), `echo 0 | sudo tee /sys/devices/system/cpu/cpu*/power/energy_perf_bias`. BIOS items (SNC disabled, C1-to-C1E off, ACPI C6x off, package C-state C0/C1, Latency Optimized Mode, Workload Profile off) = report-only. Risk: MSR writes platform-specific; power cost. Same page.
- Docker: `--runtime=habana --cap-add=sys_nice --net=host --ipc=host` (ipc=host required for distributed training). Source: https://docs.habana.ai/en/latest/Installation_Guide/Additional_Installation/Docker_Installation.html
- Interfaces appear as accel0..accel7 (index may change on reboot/driver reload): Driver_Installation.html. /sys/class/accel, /dev/accel, memlock for Gaudi, vendor id 0x1da3: UNVERIFIED.
### 3.2 Intel DC GPU Max / Arc (xe/i915): not fetched; SPARSE, UNVERIFIED. Candidate: https://dgpu-docs.intel.com/

## 4. Multi-node / RDMA
- memlock/nofile and MTU 9000: section 2 (verified-by-fetch). GPUDirect RDMA/IOMMU/peermem: 1.2-1.3.
- NCCL env docs  verified-by-fetch https://docs.nvidia.com/deeplearning/nccl/user-guide/docs/env.html : NCCL_SOCKET_IFNAME, NCCL_IB_HCA, and `NCCL_NET_GDR_LEVEL (formerly NCCL_IB_GDR_LEVEL)` (renamed 2.4.0) are documented. Finer semantics (prefix lists, 32 HCA cap) from older archived snippets; re-grep fetch/nccl before quoting. Launcher env vars: report, don't set.
- vLLM: confirm GDR with `NCCL_DEBUG=TRACE`, look for `[send] via NET/IB/GDRDMA`  verified-by-fetch https://docs.vllm.ai/en/stable/serving/parallelism_scaling/
- RoCE PFC/ECN/DSCP host prerequisites: not in fetched AMD single-node page (grep empty). UNVERIFIED; switch-side report-only. /sys/class/infiniband layout UNVERIFIED.

## 5. Serving frameworks
- vLLM  verified-by-fetch: official docker example uses `--ipc=host` (https://docs.vllm.ai/en/stable/deployment/docker/); for GPUDirect RDMA add IPC_LOCK capability and mount /dev/shm; docker example `--ipc=host --shm-size=16G -v /dev/shm:/dev/shm` (https://docs.vllm.ai/en/stable/serving/parallelism_scaling/). "ipc=host required for TP" wording came from a mirror: UNVERIFIED on official text.
- SGLang  verified-by-fetch https://docs.sglang.io/docs/hardware-platforms/amd_gpu.md : AMD container examples `--ipc=host --shm-size 16g`; host config `pci=realloc=off iommu=pt` + numa_balancing=0.
- TGI  verified-by-fetch https://huggingface.co/docs/text-generation-inference/quicktour : example `docker run --gpus all --shm-size 1g`; no rationale text.
- Triton, PyTorch shm, NUMA binding / CPU offload host guidance: not fetched, UNVERIFIED.

## Fetch failures
- instinct.docs.amd.com .../amdgpu-docs/en/develop/system-optimization/mi300x.html : 404.
- rocm.docs.amd.com tuning-guides/mi300x/* and docs-6.2.2 system.html: 200 but redirect to acceptance guide.
- Not fetched: RoCE PFC/ECN docs, Intel dgpu-docs, Triton, kfd topology.
