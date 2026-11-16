# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Documentation: `docs/COVERAGE.md`, `docs/RULE-AUTHORING.md`, `CONTRIBUTING.md`, `SECURITY.md` and `NOTICE`.

## [1.0.0] - 2026-11-16

First release. kerneltuner is a Linux kernel tuning advisor and an observe-only agent, shipped as one static Go binary for linux/amd64 and linux/arm64. Its main asset is a knowledge base of YAML rules, each cited to the project's own official documentation.

### Added

- Commands: `scan`, `explain`, `generate`, `sources`, `validate`, `capture`, `agent`, `status`, `apply`, `rollback` and `version`.
- Facts layer reading `/proc` and `/sys` only, with no vendor CLIs: kernel, CPU, memory, swap and zram, NICs (ring and channel sizes through the ethtool ioctl), block devices, mounts, kernel config, per-process limits and OOM score, containers, distro, accelerators (NVIDIA, AMD, Intel Gaudi) and RDMA.
- Workload detection for 34 canonical app ids, plus a `--workload` flag to declare apps on a host where they are not yet installed.
- Rules engine with a strict YAML schema, an expression language for conditions and values, `foreach` over NICs, block devices, accelerators, RDMA devices, apps, NUMA nodes and CPUs, runtime `detect`, `require_evidence`, and live evidence from kernel counters.
- Advisor: statuses `recommend`, `ok`, `not-applicable`, `managed`, `host-only`, `firing` and `error`; risk-weighted scoring; reconciliation of several rules for one setting, with a `CONFLICT` block when proposals disagree.
- Ecosystem awareness: parameters managed by an active `tuned` profile or a running `bpftune` are reported as `managed`, not re-recommended. `generate` can emit a `tuned` profile.
- Output formats from `generate`: sysctl, limits, systemd, tuned, cmdline, NIC, runtime advice and a combined set.
- First-class citations: rule sources can carry a verbatim quote and section, `scan` prints footnotes, there is a Markdown report, and `sources --check` verifies that registered pages still exist and still contain the cited quotes. A weekly CI job runs the check.
- `capture` with a redacted fixture mode for contributing real-host fixtures.
- Rule knowledge base: 137 rules, 398 golden tests, 145 source entries (121 distinct URLs) and 112 fixtures:
  - kernel (36 rules): network 17, memory 9, CPU 5, limits 3, storage 2
  - AI inference (25 rules): AMD ROCm 10, NVIDIA 5, Intel Gaudi 4, CPU inference 3, RDMA 3
  - applications (72 rules): RabbitMQ, Cassandra, ClickHouse, Elasticsearch, OpenSearch, PostgreSQL, MariaDB, MySQL, Kafka, ZooKeeper, Redis and Valkey, memcached, nginx, HAProxy, Envoy, Apache httpd, kubelet, etcd, containerd, Cilium, Fluent Bit, vLLM, SGLang, TGI, llama.cpp and PyTorch
  - antipatterns (4 rules): removed or harmful network settings
  - data for the `tuned` and `bpftune` ecosystems
  - notes without rules for Pulsar, NATS, CoreDNS and Prometheus, whose upstream docs have no host-level guidance
- `docs/COVERAGE.md` records per-pack depth, documented gaps and what kerneltuner does not do.
- Release engineering: GoReleaser builds tar.gz archives, deb and rpm packages, a sha256 `checksums.txt` and an SPDX SBOM per archive; GitHub Actions for CI and releases; `scripts/install.sh`, which verifies the checksum.

### Security

- The agent is observe-only. Its only file write is `/run/kerneltuner/state.json`. The shipped systemd unit sets `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `NoNewPrivileges`, `ProtectKernelTunables`, `ProtectKernelModules`, `ProtectControlGroups`, `RestrictSUIDSGID`, `LockPersonality` and `MemoryDenyWriteExecute`, and limits capabilities to `CAP_DAC_READ_SEARCH`, `CAP_SYS_PTRACE` and `CAP_NET_ADMIN`, all used for reading.
- `apply` takes a snapshot, verifies every value by reading it back, rolls back automatically on a mismatch, and requires root unless `--root` is given. It applies only sysctl, sysfs and limit rules, and never restarts a service.
- `capture` reads an allowlist only, never reads `/proc/<pid>/environ` (except the `container=` entry of PID 1), keeps `argv[0..1]` with secrets and email addresses redacted, and replaces the host name.
- The only network access is the opt-in `sources --check`, which sends no host data.
- Vulnerabilities are reported through GitHub private vulnerability reporting; see `SECURITY.md`.

### Known limitations

- Rules are sourced from documentation, not benchmarks.
- No application config files and no process environment are read.
- GPU, Gaudi and RDMA fixtures are synthetic.
- No existing rule carries a verbatim citation quote yet; `validate` warns about this.
- Pulsar, NATS, CoreDNS, Prometheus and Intel Data Center GPU Max and Arc have no rules.

[Unreleased]: https://github.com/mirastacklabs-ai/kerneltuner/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/mirastacklabs-ai/kerneltuner/releases/tag/v1.0.0
