# Coverage

What the v1.0.0 knowledge base covers, how deep it goes, and what it does not do. The numbers come from the YAML under `rules/` and the `NOTES.md` file in each pack; see [How the numbers were counted](#how-the-numbers-were-counted).

## Headline numbers

| Item | Count |
|---|---|
| Rules | 137 |
| Golden tests (`tests:` entries) | 398 |
| Source entries in `sources.yaml` files | 145 (121 distinct URLs) |
| Fixtures | 112 (111 synthetic, 1 captured from a real VM) |
| Packs with rules | 37 |
| Packs with notes but no rules | 4 (Pulsar, NATS, CoreDNS, Prometheus) |
| Canonical workload ids | 34 |

Rules by area: kernel 36, AI 25, apps 72, antipatterns 4.

Rules by kind: sysctl 36, runtime 33, report 27, limit 16, sysfs 9, antipattern 7, cmdline 6, nic 2, service 1.

`sysctl`, `sysfs` and `limit` rules can be applied. `runtime` and `report` rules produce advice or a finding only, because the setting is not readable from `/proc` and `/sys` or cannot be changed safely from here. `cmdline` rules need a reboot and are never applied.

## Depth ratings

Each pack gets one rating. The criteria are:

- **Strong**: the upstream project publishes rich host-level guidance (kernel parameters, limits, mounts, schedulers), and the pack turns it into several rules, most of them gated on evidence from live counters or host facts.
- **Moderate**: upstream guidance exists but is partial, or the pack covers the part of it that the facts layer can see. Some rules are advice only.
- **Thin**: upstream docs give little host-level guidance. The pack holds one to three rules, usually a single limit, a listen-backlog check or an advice item.

Ratings are a maintainer judgement made from the pack's `NOTES.md`, not a measured score. A Thin rating means the upstream documentation is thin, not that the pack is incomplete.

## Matrix

Kinds: S sysctl, F sysfs, L limit, N nic, C cmdline, V service, R runtime, P report, A antipattern. Gaps are the "not said by the docs" and "not ruled" items in each `NOTES.md`; the full text is there.

### Kernel

| Pack | Rules (by kind) | Tests | Covered | Documented gaps | Depth |
|---|---|---|---|---|---|
| `kernel/network` | 17 (S14, N2, F1) | 55 | Listen and SYN backlog, TCP buffer ceilings, BBR and fq, ephemeral ports, `tcp_tw_reuse`, slow start after idle, `tcp_notsent_lowat`, conntrack size, softnet budget and backlog, NIC ring and channel counts, RPS | `rmem_max`/`wmem_max` (no sourced UDP drop gate), `tcp_fin_timeout`, `tcp_sack=0` harm, irqbalance, BBR with fq on kernels 4.9 to 4.12 | Strong |
| `kernel/limits` | 3 (S2, L1) | 7 | `fs.file-max`, `fs.nr_open`, per-app open-file headroom | None recorded beyond what other packs own | Moderate |
| `kernel/memory` | 9 (P5, S2, A2) | 20 | Swappiness for zram, dirty-page window, strict overcommit on inference hosts, `min_free_kbytes`, watermark scale, `zone_reclaim_mode`, THP defrag and compaction stalls, PSI memory stalls | `vm.max_map_count` (apps own it), generic `numa_balancing` (no per-process pinning fact) | Strong |
| `kernel/cpu` | 5 (P3, R2) | 13 | Performance governor, C-state wakeup latency, SMT state, isolation boot parameters in use, `mitigations=off` | Boot-parameter rules for `isolcpus`/`nohz_full` (cannot derive a CPU list), per-process affinity | Moderate |
| `kernel/storage` | 2 (F2) | 6 | NVMe scheduler `none`, `read_ahead_kb` for model loading | `nr_requests` (no sourced value), default `read_ahead_kb` not claimed | Thin |

### AI inference

| Pack | Rules (by kind) | Tests | Covered | Documented gaps | Depth |
|---|---|---|---|---|---|
| `ai/amd` | 10 (C4, L2, R2, S1, P1) | 29 | MI300X boot parameters (`pci=realloc=off`, IOMMU), `numa_balancing`, memlock and nofile, RoCE MTU, ACS, SGLang container flags | THP, AMD governor and C-states, NPS modes, amdgpu module parameters, MaxReadReq/MaxPayload | Strong |
| `ai/nvidia` | 5 (P2, V1, A1, R1) | 14 | `nvidia-persistenced`, PCIe link width, container IPC and shared memory, peermem and IOMMU reports | Fabric Manager (now detectable, rule not written), GPU-to-NUMA binding, NVIDIA memlock, ACS, RoCE PFC/ECN | Moderate |
| `ai/rdma` | 3 (L1, R1, P1) | 8 | Memlock, RoCE MTU, NCCL HCA selection | No NOTES.md of its own; shares `ai/nvidia` notes. RoCE PFC/ECN host prerequisites are not covered | Moderate |
| `ai/intel` | 4 (C2, R1, P1) | 13 | Gaudi 3: IOMMU passthrough on Ubuntu 6.8 kernels, governor, container flags | Gaudi 3 MSR and cpuidle settings, hugepage and memlock numbers (the installer owns them), Data Center GPU Max and Arc (see below) | Moderate |
| `ai/cpu-inference` | 3 (R2, P1) | 9 | AMX availability and kernel version, NUMA binding for CPU inference | `OMP_*`/`ONEDNN_*` variables (invisible), docker `/dev/shm` default, thread-count guidance, allocator preload | Moderate |

### Apps

| Pack | Rules (by kind) | Tests | Covered | Documented gaps | Depth |
|---|---|---|---|---|---|
| `apps/rabbitmq` | 9 (S6, R2, L1) | 24 | nofile, somaxconn, SYN backlog, FIN timeout, keepalive, `ERL_MAX_PORTS`, listen backlog | `fs.file-max`, port range and buffer values are not in the docs; swappiness and THP not covered; `tcp_tw_reuse` only for client hosts | Strong |
| `apps/cassandra` | 6 (S2, L2, F1, P1) | 18 | `vm.max_map_count`, keepalive, memlock and nofile, read-ahead, swap | The production page has no swap, max_map_count or limits section; values come from packaging files without rationale. THP, nproc differences between packages | Moderate |
| `apps/clickhouse` | 6 (F2, S1, L1, R1, P1) | 16 | Governor, overcommit, THP `madvise`, scheduler, nofile, mount advice | SSD `noop` mapping, read-ahead value, swap, RAID `stripe_cache_size` | Strong |
| `apps/elasticsearch` | 4 (S3, L1) | 12 | `vm.max_map_count`, swappiness, `tcp_retries2`, nofile | Thread limit rule not written, `bootstrap.memory_lock` invisible, no THP, scheduler or NUMA guidance | Strong |
| `apps/opensearch` | 3 (S1, L1, P1) | 8 | `vm.max_map_count`, nofile, swap report | `tcp_retries2`, swappiness value, THP, scheduler; `LimitNPROC=4096` sample not ruled | Moderate |
| `apps/postgres` | 4 (S1, P2, R1) | 12 | Overcommit, postmaster `oom_score_adj`, huge pages, THP | Fixed `overcommit_ratio`, LimitNOFILE value, swappiness, scheduler, `LimitMEMLOCK` value, SysV shm limits | Moderate |
| `apps/mariadb` | 3 (S1, F1, L1) | 11 | Scheduler, nofile, swappiness | `large_pages`, `innodb_use_native_aio` (server handles it), THP, overcommit, `fs.aio-max-nr`, NUMA | Moderate |
| `apps/mysql` | 2 (F1, L1) | 6 | Scheduler, nofile (default-config value 8161 only) | Swappiness, THP, read-ahead, mount options, `large-pages` (my.cnf only), `fs.aio-max-nr` | Thin |
| `apps/kafka` | 4 (R3, L1) | 9 | nofile, `vm.max_map_count` formula, data-dir mount options, listen backlog | No swappiness or `vm.dirty_*` values, no `max_map_count` value, no scheduler, THP or memlock guidance | Moderate |
| `apps/zookeeper` | 3 (P2, R1) | 8 | Swapping, heap larger than RAM, listen backlog | No swappiness value, no ulimit, sysctl, THP or scheduler guidance | Thin |
| `apps/redis` | 4 (S1, F1, L1, R1) | 17 | `tcp-backlog` against somaxconn, overcommit, THP, nofile against `maxclients` | Applies to Valkey too. `maxclients` and `tcp-backlog` values are in redis.conf and not parsed | Moderate |
| `apps/memcached` | 3 (R2, P1) | 9 | Backlog, swap and NUMA advice | No sysctl, THP, ulimit or `max_map_count` guidance; custom `-b`/`-c` invisible | Thin |
| `apps/nginx` | 2 (R2) | 6 | `worker_rlimit_nofile` against worker limits, backlog | Defaults only; no sysctl or THP guidance in the nginx docs | Thin |
| `apps/haproxy` | 1 (R1) | 3 | fd limit handling | No sysctl guidance in the configuration manual | Thin |
| `apps/envoy` | 2 (R2) | 7 | fd limit against connection limits, listen backlog | No recommended `LimitNOFILE` or sysctl value; `tcp_fastopen` needs config parsing; THP, swap, scheduler absent | Thin |
| `apps/httpd` | 2 (R2) | 6 | `ListenBackLog` against somaxconn, swapping | No sysctl, ulimit, THP or scheduler value; `EnableSendfile` on NFS | Thin |
| `apps/kubelet` | 2 (S1, P1) | 6 | `ip_forward`, swap enabled | `br_netfilter` no longer documented, no inotify, `LimitNOFILE`, vm.* or THP guidance; `failSwapOn` lives in the kubelet config file | Thin |
| `apps/etcd` | 1 (R1) | 3 | Disk priority advice when sharing a host | No sysctl, THP, scheduler or LimitNOFILE guidance; `ionice` is not readable | Thin |
| `apps/containerd` | 1 (P1) | 3 | `overlay` module | No sysctl, THP, inotify or LimitNOFILE guidance; `LimitNPROC`/`OOMScoreAdjust` documented but not ruled | Thin |
| `apps/cilium` | 1 (P1) | 4 | Minimum kernel version | `rp_filter` and `bpf_jit_enable` not documented as sysctls; no limits or inotify guidance | Thin |
| `apps/fluent-bit` | 1 (R1) | 2 | inotify and nofile advice for the tail input | Docs use placeholders, no values; shipped unit not fetched | Thin |
| `apps/vllm` | 2 (L1, R1) | 6 | Container shared memory, memlock for GPUDirect RDMA | Container flags are not visible in facts | Thin |
| `apps/sglang` | 1 (R1) | 3 | Shared memory advice | Host cmdline and `numa_balancing` belong to `ai/amd`; NVIDIA container flags unverified | Thin |
| `apps/tgi` | 1 (R1) | 2 | `--shm-size` advice | The docs give no rationale for `--shm-size 1g` | Thin |
| `apps/llama.cpp` | 3 (L1, P2) | 11 | `--numa`, `mlock`, memlock limit | `CAP_IPC_LOCK` requirement, thread-count guidance, `LLAMA_ARG_*` variables | Moderate |
| `apps/pytorch` | 1 (R1) | 3 | CPU threading advice | NUMA binding lives in `ai/cpu-inference`; environment variables invisible | Thin |

### Antipatterns, ecosystem and unruled packs

| Pack | Rules (by kind) | Tests | Covered | Documented gaps | Depth |
|---|---|---|---|---|---|
| `antipatterns` | 4 (A4) | 9 | Network antipatterns (removed or harmful sysctls, oversized TCP memory) | Only network antipatterns so far | Moderate |
| `ecosystem` | data only | 0 | `tuned` profiles and `bpftune` sysctls, used for the `managed` status | Hand-curated from the tuned master branch; not generated | n/a |
| `apps/pulsar`, `apps/nats`, `apps/coredns`, `apps/prometheus` | 0 | 0 | Workload detection only | See [Packs with no rules](#packs-with-no-rules) | none |

Several gaps in the older `NOTES.md` files (no kernel config facts, no mount table, no nproc or `oom_score_adj` facts, no distro fact) were closed by the engine fix passes recorded in [ENGINE-ISSUES.md](ENGINE-ISSUES.md). The mount table, kernel config and nproc/`oom_score_adj` gaps in the `NOTES.md` files are therefore closed: those facts now exist and rules can use them. The matching rules were not all written afterwards, so some of those gaps still show as unruled in the matrix above.

## What kerneltuner does not do

- **No benchmarking.** Rules are sourced from official documentation, not measured. kerneltuner never runs a workload, and a recommendation says what the upstream docs advise, not what is fastest on your hardware. Use the `verify` field of each rule to check the effect.
- **No parsing of application config files.** It does not read `redis.conf`, `nginx.conf`, `postgresql.conf`, kubelet config or similar. Rules gate on host facts, process limits and mounts instead, so a rule cannot know that `maxclients` or `shared_buffers` was changed. Several packs say so in their notes. This is intentional: reading arbitrary application configs is out of scope and a privacy risk.
- **No reading of process environment.** `/proc/<pid>/environ` is never read, so variables such as `OMP_NUM_THREADS`, `ONEDNN_MAX_CPU_ISA`, `NCCL_*` and `ERL_MAX_PORTS` are invisible. Rules that depend on them are written as `runtime` advice. (`capture` reads one entry only: `container=` from PID 1.)
- **GPU, Gaudi and RDMA fixtures are synthetic.** 111 of the 112 fixtures are hand-built trees of fake `/proc` and `/sys` files, including every NVIDIA, AMD, Intel and RDMA fixture. They test the rule logic, not that a real driver exposes those paths in that format. The one captured fixture is `colima-vm`. Real-host captures are welcome; see [RULE-AUTHORING.md](RULE-AUTHORING.md).
- **Some thresholds are heuristics.** Where the upstream docs give no number, the rule states its own screening threshold and says so in its `rationale` (search the YAML for `heuristic`, `screening threshold` or `our inference`). Examples: the 1 event/s `allocstall` threshold in `kernel/memory`, the doubling formula in `limits.file_max`, the soft limit of 1024 in `app.envoy.fd_limit`. These are starting points, not tuned optima.
- **Quotes are verified, not endorsements.** All 137 rules carry verbatim quotes in their `sources` entries, and `validate` rejects a source without one. `sources --check` fetches every registered page and confirms each quote is still there; it runs weekly in CI. A quote shows that the upstream text says what the rule cites, not that the advice is optimal for your hardware.
- **No per-process I/O priority, CPU affinity or SMT topology tuning.** `ionice` is only available through a syscall, and affinity needs per-process parsing no rule has needed yet.
- **No automatic changes.** The agent only observes. Changes go through `apply`, which supports `sysctl`, `sysfs` and `limit` rules only.
- **No Windows, BSD or macOS support.** Linux only, amd64 and arm64.

## Packs with no rules

These four packs have a `NOTES.md` and no rules, because the upstream documentation contains no host-level tuning guidance to cite. Their app ids are still detected, so other rules (for example the generic `jvm` or `kernel/*` packs) can see them. Rules are not invented where the source is silent.

| Pack | Why there are no rules |
|---|---|
| Pulsar and BookKeeper | The Pulsar and BookKeeper docs, shipped config and scripts were searched for `ulimit`, `nofile`, `somaxconn`, swap, `sysctl`, `vm.`, `dirty`, `noatime`, scheduler, readahead, `memlock`, THP and `max_map_count`. Hits were app-level (`journalSyncData`, ledger storage layout) or unrelated. A standalone bookie is detected as the generic `jvm`, not `pulsar`. |
| NATS | The NATS docs and the whole `nats-io/nats.docs` repository were searched for the same terms. The only hit is a `file_descriptors_limit` setting for the deprecated NATS Streaming, an app setting with no OS value. |
| CoreDNS | The manual is configuration only. No guidance on file descriptors, ulimits, sysctls, UDP buffers, conntrack or inotify. The `loop` plugin page describes a DNS forwarding loop, not a kernel setting. |
| Prometheus | One real host statement exists: NFS and other non-POSIX filesystems are not supported for local storage. kerneltuner cannot detect it, because the TSDB path is a command-line flag it does not parse. No fd, `max_map_count`, swap, THP or inotify guidance. |

Intel Data Center GPU Max and Arc also have no rules. The Intel dGPU documentation at `dgpu-docs.intel.com` has no host-tuning text (searches for `iommu`, `hugepage`, `governor`, `numa`, `memlock`, `cmdline` and `grub` came back empty), so a detect-only report would add no guidance. The `intel-max-1550` fixture exists only to test that the Gaudi rules are not applicable on such a host. If Intel publishes host guidance for xe or i915 GPUs, a pack can be added with [RULE-AUTHORING.md](RULE-AUTHORING.md).

## How the numbers were counted

From the repository root, with `rules/` as the working directory:

```sh
# rules per pack and kind, tests, sources
for f in $(find kernel ai apps antipatterns -name '*.yaml' ! -name sources.yaml); do cat "$f"; done \
  | awk '/^- id:/{r++} /^  kind:/{k[$2]++} /^    - fixture:/{t++} END{print r" rules, "t" tests"; for(x in k) print x, k[x]}'
find . -name sources.yaml | xargs cat | grep -c '^- url'
```

Per-pack rows use the same counts run on each pack directory. Fixtures are the directories under `testdata/fixtures`; synthetic ones carry `synthetic: true`. Counts are for the v1.0.0 tree and will drift as packs grow; `kerneltuner validate` is the source of truth for whether the tree is consistent.
