# AMD pack notes (2026-11-16)

Re-fetched by curl; text supports the cited claims:
- MI300X acceptance guide (redirects to instinct.docs.amd.com/projects/system-acceptance/.../gpus/mi300x.html): "Pass: Contains pci=realloc=off, amd_iommu=on or intel_iommu=on, and iommu=pt".
- AMD workload-optimization page (docs-7.0.0): `cat /proc/sys/kernel/numa_balancing`, `sudo sysctl kernel.numa_balancing=0`; "Disable ACS for multi-node RCCL" (`lspci -vvv | grep -i acsctl`, SrcValid+, disable_acs.sh).
- Troubleshooting page: memlock unlimited / nofile 1048576 in limits.conf; MTU 9000 on host; ACS on bare metal; numa_balancing=0 in GRUB_CMDLINE_LINUX_DEFAULT.
- The latest rocm.docs.amd.com workload page (ai-ecosystem project) repeats numa_balancing and ACS text; no THP/governor text.

Still UNVERIFIED, no rule shipped: THP, AMD CPU governor/C-states, NPS modes (BIOS; the latest page only lists NPS in a partition table), amdgpu module params, MaxReadReq/MaxPayload (runtime setpci; needs lspci data kerneltuner does not collect).

Engine/fixture notes:
- `recommend` is an expression, so string values must be quoted inside the YAML string (`'"pt"'`).
- `pci=realloc=off` is parsed as key `pci`, value `realloc=off`; a host with `pci=realloc=off,nommconf` would be flagged. Documented in the rule.
- The shared gpu-8xmi300x fixture puts RoCE MTU under infiniband/.../device/net only, with no /sys/class/net entry, so facts.NICs lacks them; ai-amd-roce-mtu1500 and ai-amd-tuned add the netdevs.
