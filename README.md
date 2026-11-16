# kerneltuner

[![CI](https://github.com/mirastacklabs-ai/kerneltuner/actions/workflows/ci.yml/badge.svg)](https://github.com/mirastacklabs-ai/kerneltuner/actions/workflows/ci.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

kerneltuner is a Linux kernel tuning advisor and an observe-only agent, shipped as one static Go binary. It reads the hardware, kernel and running workloads of a host, checks live counters for real bottlenecks, and reports what to change, why, how risky it is, and what to leave alone.

Host tuning guidance is scattered across the Linux kernel docs and the official docs of 26+ OSS and CNCF projects plus GPU vendors. kerneltuner collects it into one tool. Every recommendation cites the upstream page and quotes it, so credit stays with the source and you can check the claim yourself. It complements `tuned`, `bpftune` and vendor guides instead of replacing them: it defers to settings they manage, and it can emit a `tuned` profile.

It is an advisor whose rules come from official documentation. It is not benchmark-validated. A recommendation says what the upstream docs advise, not what is fastest on your hardware. See [docs/COVERAGE.md](docs/COVERAGE.md) for the depth of each pack and for what kerneltuner does not do.

## Install

Linux only, amd64 and arm64.

The install script downloads the release archive for your architecture, checks its sha256 against the release's `checksums.txt`, and installs to `/usr/local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/mirastacklabs-ai/kerneltuner/main/scripts/install.sh | sudo sh
```

Options: `-v VERSION` (default: latest release) and `-d INSTALL_DIR`. To read the script first, download it and run `sh install.sh`.

`.deb` and `.rpm` packages are attached to each [GitHub release](https://github.com/mirastacklabs-ai/kerneltuner/releases). They install the binary to `/usr/bin` and the systemd unit for the agent.

To build from source, you need only Docker. Nothing is installed on the host:

```sh
git clone https://github.com/mirastacklabs-ai/kerneltuner.git
cd kerneltuner
make build    # static linux binary at bin/kerneltuner
make test
```

## Quick start

```sh
# What should I change on this host?
sudo kerneltuner scan

# Tune a host before the software is installed
kerneltuner scan --workload kafka,postgres

# Why? Rationale, risk and sources for one rule or parameter
kerneltuner explain net.core.somaxconn

# Write sysctl.d files to ./out; touches nothing else
kerneltuner generate --format sysctl --out ./out

# Preview, apply, undo
sudo kerneltuner apply --dry-run
sudo kerneltuner apply --yes
sudo kerneltuner rollback --yes
```

`scan` shows only `recommend` and `firing` results by default. Add `--all` to include `ok`, `not-applicable`, `managed` and `host-only`. Other useful flags: `--format text|json|md`, `--sample 10s` (live counter rates), `--domain network|limits|memory|cpu|storage|ai`, `--pack kernel/network`, `--fail-on low|medium|high` (exit 2, for CI), `--no-citations`. `generate --format` takes `sysctl`, `limits`, `systemd`, `tuned`, `cmdline`, `nic`, `runtime` or `all`. `rollback --list` shows snapshots.

## Example output

Real output from the bundled fixtures, which are synthetic `/proc` and `/sys` trees used by the tests. Output is trimmed: lines are shortened, repeated rows and most References are cut, and `...` marks the cuts. Each reference also carries a `quote` field with the upstream text where the rule has one; quote backfill is in progress, see [docs/COVERAGE.md](docs/COVERAGE.md).

A legacy 4.9 kernel web host (`scan --fixture legacy-4.9-c10k`):

```text
kerneltuner dev | kernel 4.9.0-19-amd64
CPU Intel(R) Xeon(R) CPU E3-1270 v6 @ 3.80GHz (1 sockets, 4 cores, 8 threads) | RAM 31 GiB
accelerators: none | Warnings: 1

== NETWORK ==
STATUS     RULE                         CURRENT -> PROPOSED    RISK    EVIDENCE
recommend  net.core.somaxconn           128 -> 4096            low     TcpExt.ListenOverflows=38.5 (> 0) [1,2,3,4]
    The accept queue of every listening socket is min(the application's listen() backlog,
    somaxconn).
recommend  net.ipv4.ip_local_port_range 32768 60999 -> 1024 65535  medium  sockstat.tcp.tw=2.15e+04 (> 2e+04) [1]
firing     antipattern.tcp_tw_recycle   -                      high    [7,8]
    tcp_tw_recycle was removed in Linux 4.12 (commit 4396e46187ca) because it breaks clients behind
    NAT: ...
...
References
[1] linux (reviewed 2026-11-16)
    https://docs.kernel.org/networking/ip-sysctl.html
...
126 more result(s) hidden (ok, not-applicable, managed, host-only); use --all to show them.
```

A GPU host (`scan --fixture gpu-8xh100`), AI section:

```text
kerneltuner dev | kernel 6.8.0-45-generic
CPU Intel(R) Xeon(R) Platinum 8462Y+ (2 sockets, 64 cores, 128 threads) | RAM 2016 GiB
accelerators: 8x nvidia | Warnings: 1

== AI ==
STATUS     RULE                       TARGET        CURRENT -> PROPOSED  RISK    EVIDENCE
firing     ai.nvidia.gpudirect-iommu                -                    medium  [8,9]
    [reboot required] GPUDirect RDMA relies on all physical addresses being the same from the GPU's
    and the NIC's point of view, so it is incompatible with any IOMMU translation o...
recommend  ai.nvidia.persistenced                   stopped -> running   low     [10]
    Without persistence, the driver unloads GPU state when the last client closes the device file,
    so the next CUDA start pays the initialisation cost again.
...
[10] nvidia (reviewed 2026-11-16)
     https://docs.nvidia.com/deploy/driver-persistence/persistence-daemon.html
```

The shareable Markdown report (`scan --format md`), same legacy fixture:

```markdown
# kerneltuner report

Generated by kerneltuner dev on 2026-11-16. Review before applying: these are recommendations, not verified for your workload.

### NETWORK

| Status | Rule | Target | Change | Risk | Why | Refs |
|---|---|---|---|---|---|---|
| recommend | `net.core.somaxconn` |  | `128 -> 4096` | low | The accept queue of every listening socket is min(the application's listen() backlog, somaxconn). Evidence: TcpExt.ListenOverflows=38.5 (> 0) | [1,2,3,4] |
...
## References

- **[1]** linux (reviewed 2026-11-16)
  <https://docs.kernel.org/networking/ip-sysctl.html>
```

## What it covers

- **Domains:** network, limits (fds, ports, conntrack), memory, CPU, storage, and AI inference hosts: NVIDIA, AMD ROCm, Intel Gaudi, CPU-only inference (AMX), and RDMA/NCCL.
- **Apps** (the ids `--workload` accepts, and that detection recognises): vllm, sglang, tgi, triton, llama.cpp, ollama, pytorch, cassandra, kafka, pulsar, zookeeper, elasticsearch, opensearch, jvm, redis, valkey, memcached, nginx, haproxy, envoy, httpd, postgres, mariadb, mysql, clickhouse, rabbitmq, nats, kubelet, etcd, containerd, cilium, coredns, prometheus, fluent-bit.
- **Antipatterns:** settings that are removed or harmful on the running kernel, such as `tcp_tw_recycle` or disabled TCP timestamps.

Depth varies a lot between packs, and a few apps are detected but have no rules because their docs give no host-level guidance. [docs/COVERAGE.md](docs/COVERAGE.md) has the per-pack matrix.

## How it works

kerneltuner reads facts from `/proc` and `/sys` (kernel, CPU, memory, NICs, block devices, accelerators, processes and their limits, container status). Workload detection matches running processes to app ids, or takes `--workload`. Live counters, such as listen overflows or allocation stalls, serve as evidence that a bottleneck is real, and they raise or lower a rule's score. Rules are cited YAML data under [rules/](rules/README.md), embedded in the binary and tested against fixtures.

Each rule yields one status: `recommend` (change it), `ok` (already fine), `managed` (an active `tuned` profile or `bpftune` owns the parameter), `host-only` (cannot be set from inside a container), `firing` (an antipattern or report check detected a problem), plus `not-applicable`. Recommendations from different rules for the same setting are reconciled into one value before `generate` or `apply`. When a recommendation contradicts a rule that is already satisfied at another value, it is reported as a conflict. Design details are in [docs/DESIGN.md](docs/DESIGN.md).

## Safety model

- Read-only by default. `scan`, `explain`, `generate`, `sources` and `capture` change nothing on the host.
- `apply` touches only sysctl, sysfs and limits settings. It needs root, takes a snapshot first, verifies the result after writing, and rolls back automatically if verification fails. `--dry-run` shows the plan, and `--yes` is required to write. Boot parameters (`cmdline`) and runtime advice are only ever generated or printed, never applied.
- The agent is observe-only. Its one file write is `/run/kerneltuner/state.json`. The shipped systemd unit is hardened, and alerts go through journald.
- Container aware: settings that cannot be changed from inside a container are reported as `host-only`.
- `capture` archives an explicit allowlist of `/proc`, `/sys` and `/etc` paths for bug reports and fixtures. It keeps only the process name and the first two argv entries, with secrets and host names redacted. It never reads environ, fd targets, host names, serials, or MAC and IP addresses.

## Agent

```sh
sudo systemctl enable --now kerneltuner
journalctl -t kerneltuner -f
kerneltuner status
```

The agent samples counters every 15 seconds by default (`--interval`), logs `firing` and `resolved` transitions, and never changes the system. `kerneltuner status --json` prints the raw state. See [deploy/README.md](deploy/README.md) for journal fields, thresholds and overriding flags.

## Citations and source drift

```sh
kerneltuner sources              # upstream docs behind the active recommendations
kerneltuner sources --all        # every registered source
kerneltuner sources --check      # fetch each URL and verify the cited quotes still appear
```

`sources --check` is opt-in and sends no host data. A weekly CI job ([sources-check.yml](.github/workflows/sources-check.yml)) runs it and opens a `sources-drift` issue when an upstream page moves or its text changes.

## Contributing

Rules need a registered upstream source, a verbatim quote that supports the claim, and golden tests against fixtures. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup and the rule bar, and [docs/RULE-AUTHORING.md](docs/RULE-AUTHORING.md) for the schema. Security reports: [SECURITY.md](SECURITY.md). Fixtures captured from real GPU, Gaudi and RDMA hosts are especially welcome.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
