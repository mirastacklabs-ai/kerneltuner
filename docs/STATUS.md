# Status — 2026-11-16

## Done
- **Plan** (`docs/PLAN.md`). v1 scope: network, limits, memory, CPU, storage, and AI inference on NVIDIA, AMD, Intel, CPU-only and RDMA.
- **Phase 1 skeleton, verified.**
  - `make tidy build test lint` all pass in Docker. golangci-lint is pinned to v2.15.0, because it needs a version built with Go ≥1.27.
  - The `version` and stub commands run in an alpine container.
  - Contents: Go module `github.com/mirastacklabs-ai/kerneltuner`, cobra CLI stubs plus a test, package stubs, and the `rules/` tree including `kernel/{network,limits,memory,cpu,storage}` and `ai/{nvidia,amd,intel,cpu-inference,rdma}`.
- **Research** in `docs/research/`: `network-limits-research.md`, `cpu-mem-research.md`, `ai-accel-research.md`. Every claim is tagged `verified-by-fetch` (taken from a downloaded primary source) or UNVERIFIED. Rules may only be built from verified claims.

## Next
1. Code review pass on the phase 1 skeleton.
2. Phase 2 (engine) from `docs/PLAN.md`: facts (including accelerators and RDMA), workload detection, the rules engine, and `scan`/`explain`/`generate`/`sources`.
3. Phase 3: rule packs written from the research files.

## Open research gaps (each needs a primary source before it can become a rule)
- **AMD:** the THP, governor and NPS guidance moved or is gone (the old tuning-guide URL now redirects to the Customer Acceptance Guide).
- **PCIe:** the `current_link_speed`/`max_link_speed` sysfs files are not in the documented kernel ABI.
- **Intel:** no host-tuning docs for Data Center GPU Max/Arc, so only detection is possible. There is no numeric memlock or hugepage guidance for Gaudi.
- **Network:** no source yet for buffer sizing from bandwidth-delay product, HAProxy's sysctl guidance, NIC drop-counter names (they differ by driver), or the `tcp_mtu_probing` default.
- **CPU:** no official source for "use physical cores, not SMT threads". The kernel docs give conflicting descriptions of `intel_idle.max_cstate=0`.
