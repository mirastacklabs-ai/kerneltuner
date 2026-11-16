# CPU pack notes

Newly verified by fetch on 2026-11-16 (beyond docs/research/cpu-mem-research.md):

- kernel-parameters: `intel_idle.max_cstate=0` "disables intel_idle and fall back on acpi_idle"; intel_idle.html says 0 "causes the intel_idle initialization to fail". The docs conflict, so no rule recommends the boot parameter; the runtime /dev/cpu_dma_latency route is advised instead.
- cpuidle.html / pm_qos_interface.html: the PM QoS request lives only as long as the open file descriptor on /dev/cpu_dma_latency; idle governors must not pick states whose exit latency exceeds the effective (minimum) limit.
- intel_pstate.html: HWP + performance writes EPP 0 and rejects any non-zero EPP written via sysfs.
- amd-pstate.html: usable with all generic scaling governors.
- kernel-parameters `mitigations=off`: "improves system performance, but it may also expose users to several CPU vulnerabilities"; supported only with CPU_MITIGATIONS=y. `isolcpus=` `domain` isolation is irreversible at runtime ("Use cpusets for a dynamic configuration"); `nohz` flag equals nohz_full, and the residual 1 Hz tick needs the workqueue cpumask affined to housekeeping CPUs.
- no_hz.html: at least one non-adaptive-tick CPU must remain; NO_HZ_FULL is normally not wanted unless realtime or certain HPC workloads.

Skipped:

- cmdline-kind isolcpus/nohz_full rules: nothing in the facts says a host is a realtime/HPC isolation target, and a CPU list cannot be derived. The report rule only fires when the operator already set these parameters.
- Vulnerability state: the engine has no collector for /sys/devices/system/cpu/vulnerabilities/*, so the mitigation report keys off `mitigations=off` on the kernel command line only.
- Per-CPU governor/EPP as a `sysfs` rule: `{item}` is a foreach item name and there is no per-CPU foreach, so a sysfs rule could only address cpu0. It is a `runtime` rule instead.
