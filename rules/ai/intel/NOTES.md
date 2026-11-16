# Intel pack notes (2026-11-16)

Gaudi (Bare_Metal_Fresh_OS.html, redirects to Driver_Installation.html; Docker_Installation.html) re-fetched; text supports: IOMMU passthrough required only on Ubuntu 24.04.2/22.04.5 with kernel 6.8 (`iommu=pt intel_iommu=on`), governor performance on bare metal before starting containers, installer sets huge pages automatically, docker `--runtime=habana --cap-add=sys_nice --net=host --ipc=host`.

Not shipped:
- Gaudi 3 CPU settings (wrmsr 0x1b0/0x774, cpuidle state2 disable, energy_perf_bias, BIOS items): platform-specific MSR writes; verified in research but outside the planned coverage.
- Numeric hugepage/memlock rules: the installer owns them.
- Data Center GPU Max / Arc: https://dgpu-docs.intel.com/ (overview/introduction.html fetched, 200) has no host-tuning text (grep for iommu/hugepage/governor/numa/memlock/cmdline/grub empty). A detect-only report would add no guidance, so no rule. Gap: no vendor host-tuning guidance found for xe/i915 GPUs. intel-max-1550 is used only as a not-applicable fixture.

Distro detection: kerneltuner has no distro fact, so "Ubuntu" is approximated as kernel 6.8 with a release string ending in `-generic`. A real `Host.Distro` fact would make the Gaudi IOMMU rules exact.
