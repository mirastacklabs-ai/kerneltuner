# kerneltuner — plan

## Context
Linux kernel defaults are sized for the smallest machine. Admins of bigger servers tune them using old c10k/c100k blog posts, and a lot of that advice is obsolete or harmful on modern kernels (`tcp_tw_recycle` was removed in 4.12, `somaxconn` defaults to 4096 since 5.4, and so on). kerneltuner is a single Go binary that:
- looks at the hardware, the kernel and the workload running on the server
- watches live counters for evidence of real bottlenecks
- tells the admin what to tune, why, how risky it is, and what to leave alone

**Positioning: it complements existing tools, it does not compete with them.** Tuning knowledge is scattered across kernel docs, vendor guides (nginx, HAProxy, Redis, Postgres, Kafka, Elasticsearch…), distro guides, and tools like `tuned` and `bpftune`. kerneltuner gathers that knowledge into one curated, version-aware knowledge base that cites its sources, and presents it in one tool. It works alongside the existing ecosystem:
- Every rule links back to its upstream source, so credit and authority stay with the original project.
- `generate` can output a `tuned` profile, so admins already using `tuned` keep their workflow.
- If `tuned` or `bpftune` is active, the report says which parameters that tool already manages and does not recommend changing them.
- The rules knowledge base is the main asset. It is published as plain YAML so other tools can reuse it.

**Decisions made with the user**
- Continuous agent that only observes and alerts. Every change goes through a manual `apply` command with rollback.
- **v1 covers everything**: network, limits, memory/VM, CPU, storage, plus **AI inference** for NVIDIA GPUs, AMD GPUs (ROCm), Intel accelerators (Gaudi and Data Center GPU Max, with depth limited by what docs exist), CPU-only inference (llama.cpp, Ollama), and multi-node over RDMA/NCCL. This changed from the original "network + limits first" scope.
- Written in Go.
- Recommendations come from static facts and are ranked by live counter evidence.
- Output goes to the CLI and journald. No Prometheus endpoint or HTTP API in v1.

The directory is empty, so everything is built from scratch.

## Architecture
```
facts (static)  ──┐
workload detect ──┼─► advisor (rules engine) ─► recommendations ─► report / generate / agent alerts
metrics (rates) ──┘          ▲                                       │
                     rules/*.yaml (embedded)                   apply / rollback (manual)
```

### Layout
```
cmd/kerneltuner/main.go          # cobra commands
internal/facts/      # CPU (model, flags incl. avx512*/amx_*, SMT, governor/EPP, C-states), NUMA topology, RAM/hugepages/THP, kernel release+features+cmdline, virt/container, NICs (speed, queues, rings, driver), block devices (nvme/rotational, scheduler, readahead), current sysctls, limits
internal/facts/accel/ # accelerators via sysfs PCI (vendor 0x10de NVIDIA, 0x1002 AMD, 0x8086 class 0x12/0x03 Intel, 0x1da3 Habana): NUMA node, PCIe link speed/width vs max, IOMMU groups, driver; /proc/driver/nvidia, /sys/class/kfd, /sys/class/accel; RDMA via /sys/class/infiniband. Vendor CLIs (nvidia-smi, rocm-smi, hl-smi) used only when present, never required
internal/workload/   # listening sockets (/proc/net/tcp*), process comm/cmdline, systemd units → known apps (nginx, haproxy, envoy, postgres, redis, kafka, jvm…)
internal/metrics/    # samplers + rate calculation over a sliding window
internal/rules/      # YAML schema, loader (go:embed), expression evaluation
internal/advisor/    # evaluates rules → Recommendation{current, proposed, risk, evidence, score}
internal/render/     # text report, sysctl.d file, limits.d + systemd LimitNOFILE drop-ins, JSON (internal / tests)
internal/agent/      # sampling loop, alert state machine (ok→firing→resolved), cooldown, journald
internal/apply/      # snapshot → write /etc/sysctl.d/90-kerneltuner.conf → sysctl -w; rollback by snapshot id
rules/kernel/{network,limits,memory,cpu,storage}/*.yaml   # generic kernel rules
rules/ai/{nvidia,amd,intel,cpu-inference,rdma}/*.yaml     # AI-inference host rules per platform
rules/apps/<oss-project>/*.yaml        # one pack per OSS project (nginx, haproxy, redis…), each rule citing that project's docs
rules/antipatterns/*.yaml
rules/sources.yaml                     # registry of upstream docs: url, project, kernel/app versions covered, last-reviewed date
testdata/fixtures/<name>/{proc,sys,meta.json}
deploy/kerneltuner.service
```

### Libraries (reused, not rewritten)
- `github.com/prometheus/procfs` (+ `sysfs`) parses /proc and /sys. It takes a root path, so tests can point it at fixtures.
- `github.com/expr-lang/expr`: sandboxed expressions for rule conditions and value formulas.
- `github.com/coreos/go-systemd/v22/journal`: structured journald fields.
- `github.com/safchain/ethtool`: NIC rings, channels and stats without shelling out.
- `github.com/spf13/cobra`: the CLI.

### Rule schema (data, not code)
```yaml
id: net.listen-backlog
kind: sysctl                     # sysctl | sysfs | limit | nic | cmdline (needs reboot) | service | runtime (advice for app flags/env, e.g. numactl, OMP_NUM_THREADS) | antipattern
param: net.core.somaxconn
domain: network
applies:                         # prefer feature probes over version numbers (distros backport)
  kernel: "<5.4"                 # optional semver-ish range
  probe: "exists('/proc/sys/net/core/somaxconn')"
  when: "workload.listeners > 0"
recommend: "max(current, 4096)"  # expr over facts.*, workload.*, current
evidence:                        # raises priority when observed
  - metric: TcpExt.ListenOverflows
    rate_gt: 0
risk: low                        # low | medium | high (high = security/stability trade-off)
rationale: "..."
sources: ["https://..."]
verify: "TcpExt.ListenOverflows rate stays 0"
tests:                           # golden cases run by `go test`
  - fixture: ubuntu-22.04-16c-64g-nginx
    expect: { proposed: 4096 }
```
Score = base priority × risk penalty × evidence boost. Rules whose proposed value equals the current one are reported as "already OK". Anti-pattern rules flag harmful values that are already set (`tcp_tw_recycle`, `tcp_timestamps=0`, `tcp_sack=0`, oversized buffers).

### MVP rule set (about 25 rules)
- **Network**: `somaxconn`, `tcp_max_syn_backlog`, `netdev_max_backlog`, `netdev_budget(_usecs)`, `rmem_max`/`wmem_max`/`tcp_rmem`/`tcp_wmem` (sized from bandwidth-delay product using link speed), BBR + `fq` (only if the kernel supports them), `ip_local_port_range`, `tcp_tw_reuse`, `tcp_fin_timeout`, `tcp_slow_start_after_idle`, `tcp_mtu_probing`, `tcp_notsent_lowat`, `nf_conntrack_max` + hashsize (only if conntrack is loaded), NIC ring sizes and channel count, RPS/XPS when NIC queues < cores, whether irqbalance is present.
- **Limits**: `fs.file-max`, `fs.nr_open`, per-service `LimitNOFILE` (compares `/proc/<pid>/limits` with the open file count).
- **Evidence counters**:
  - `/proc/net/netstat`: ListenOverflows/ListenDrops, TCPBacklogDrop, TCPRcvQDrop
  - `/proc/net/snmp`: retransmits
  - `/proc/net/softnet_stat`: dropped, time_squeeze
  - conntrack count vs max
  - `/proc/sys/fs/file-nr`
  - `sockstat` TIME_WAIT/orphans
  - NIC `rx_missed`/`no_buffer`
  - PSI
- **Memory/CPU/storage**:
  - memory: swappiness, dirty ratios, overcommit, `max_map_count`, THP mode/defrag, static hugepages, `zone_reclaim_mode`, `numa_balancing`, `min_free_kbytes`
  - CPU: governor/EPP, C-state limits, SMT
  - storage: NVMe scheduler `none`, readahead
  - evidence: PSI, THP compaction stalls (`/proc/vmstat`), major page faults, `numastat` misses, throttling
- **AI inference**:
  - memlock ulimit for pinned host memory
  - `iommu=pt` and other boot parameters
  - PCIe link running below its rated speed or width (a real-world fault)
  - GPU-to-NUMA affinity, with matching `numactl` runtime advice
  - persistence mode (NVIDIA) and its equivalents on AMD and Intel
  - hugepages and THP for model weights; readahead and page cache for loading weights
  - CPU inference: AVX-512/AMX detection → backend advice, SMT, `OMP_NUM_THREADS` and thread pinning
  - RDMA: memlock, MTU, PFC/ECN prerequisites for RoCE, NCCL environment advice
  - app packs: vLLM, SGLang, TGI, Triton, llama.cpp, Ollama, PyTorch, citing vendor guides (NVIDIA, AMD ROCm, Intel Gaudi)
- **Context awareness**: inside a container, only namespaced sysctls are recommended. On cloud NICs (ena, virtio, gve), driver limits are respected.

## Commands
- `scan`: one-shot report (current value, proposed value, risk, evidence, reason). `--sample 30s` samples counters before reporting. `--json` for machine-readable output.
- `explain <param|rule-id>`: rationale, sources, kernel notes.
- `generate [--format sysctl|tuned|systemd]`: writes sysctl.d / limits.d / systemd drop-in files, or a `tuned` profile, to stdout or a directory. Touches nothing on the system.
- `sources`: lists the upstream docs the active recommendations draw on, with their last-reviewed dates.
- `agent`: the daemon. Samples every 15s and evaluates rules against rates. When a rule changes state (ok → firing → resolved) it sends a journald alert with fields KT_RULE, KT_PARAM, KT_CURRENT, KT_PROPOSED, KT_EVIDENCE, with a per-rule cooldown. It writes `/run/kerneltuner/state.json`. Never writes to the system.
- `status`: reads state.json and shows firing rules and trends.
- `apply [--rule id…]`: requires root. Saves current values to `/var/lib/kerneltuner/snapshots/<ts>.json`, writes `90-kerneltuner.conf`, applies live and checks the values took effect. Prints the matching `rollback` command.
- `rollback <snapshot>`: restores values and removes or rewrites the managed file.
- `capture`: tars the relevant /proc and /sys files for fixtures and bug reports. Secrets and cmdline arguments are stripped.

The systemd unit for `agent` is hardened: `ProtectSystem=strict` and `NoNewPrivileges`. It gets only `CAP_DAC_READ_SEARCH` + `CAP_SYS_PTRACE` (needed to read other processes' fd counts and limits). `CAP_NET_ADMIN` turned out to be unnecessary (ethtool get-queries are unprivileged) and is not granted.

## Phases
1. **Skeleton**: `git init`, go.mod, Makefile with Docker targets (`make test`, `make build`, `make lint` using `golang` / `golangci-lint` images; nothing installed on the host), project CLAUDE.md, README.
2. **Engine**: facts (including accelerators and RDMA), workload detection, rules engine, and the `scan`/`explain`/`generate`/`sources` commands. `generate` gains a `cmdline` output (GRUB snippet with a reboot warning). Detects `tuned` and `bpftune` so their parameters are reported as managed. Ships with 2–3 seed rules per kind to prove the schema works. Fixtures:
   - one captured from Colima
   - synthetic GPU servers: 2-socket with 8× H100, MI300X, Gaudi
   - a 2-socket CPU-inference box with AMX
   - an RDMA node
3. **Rule packs, written in parallel** by several agents working from research notes. Each pack must meet the bar: a source URL plus a golden test. Packs:
   - network + limits + antipatterns + nginx/HAProxy/Redis
   - memory/CPU/storage
   - NVIDIA
   - AMD/ROCm
   - Intel
   - CPU inference
   - RDMA/NCCL
   - **OSS app packs (added to v1).** Every rule is gated on the app being detected or declared (`--workload`).
     - Databases: PostgreSQL, MySQL, MariaDB, Cassandra, ClickHouse
     - Caches: Redis, Valkey, Memcached
     - Messaging: Kafka, ZooKeeper, Pulsar, RabbitMQ, NATS
     - Search: Elasticsearch, OpenSearch
     - Web and proxy: nginx, HAProxy, Envoy, Apache httpd
     - CNCF and node: Kubernetes node (kubelet), etcd, containerd, Cilium, CoreDNS, Prometheus, Fluent Bit
     - MongoDB and ScyllaDB are excluded because their licenses are not OSI open source.
4. **Metrics + evidence ranking + `capture`**.
5. **`agent` + journald + `status` + systemd unit**.
6. **`apply` / `rollback`**, covering only sysctl/sysfs/limits. Boot-parameter and runtime rules are never applied, only generated.
7. **Later**: more app packs on request, a Prometheus exporter.


## Verification
- `make test` (Docker `golang` image): unit tests on the procfs/sysfs fixtures, every rule's golden `tests:` cases, and a check that `generate` output parses as valid sysctl.d/systemd syntax.
- Real kernel: run `scan` inside a privileged container on Colima (`docker run --privileged --net=host -v /proc:/host/proc:ro …`) and compare its facts with `sysctl -a` / `ethtool`.
- Evidence path: in that container, start a tiny listener with `somaxconn=128`, flood it with a load tool (e.g. `wrk` in a container) until ListenOverflows increases, and confirm `scan --sample` ranks the backlog rule first and `agent` logs a journald alert (`journalctl -t kerneltuner`).
- `apply`/`rollback`: run in a disposable VM or container, never on the Mac host. Values change, `rollback` restores them exactly (diff of `sysctl -a`).
