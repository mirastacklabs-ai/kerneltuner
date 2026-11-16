# kerneltuner v1 — engine design (contract)

This is the shared contract for all packages. `docs/PLAN.md` explains the reasons behind it. If code and this file disagree, this file wins until it is amended.

## Principles
- **Read-only by default.** Only `apply`/`rollback` write, and only sysctl, sysfs and limits. `cmdline`, `nic`, `service`, `runtime` and `report` rules are only ever printed or generated.
- **Every path is relative to a root.** `facts.Root{Proc, Sys, Etc, Run, Var string}` defaults to `/proc`, `/sys`, `/etc`, `/run`, `/var`. The global `--root DIR` flag maps them to `DIR/proc` and so on. Tests and fixtures depend on this. Nothing may hard-code `/proc`.
- **Rules are data.** No rule-specific Go code. If a rule needs a new capability, add a generic helper to the expression environment.
- **Only verified facts.** Every rule cites ≥1 source URL listed in the `sources.yaml` of its pack directory, and has ≥1 golden test. Claims marked UNVERIFIED in `docs/research/` must not become rules.
- **No vendor CLIs.** Facts come from /proc and /sys. Vendor CLIs (nvidia-smi, rocm-smi, hl-smi, ethtool binary) are never required. Ring and channel sizes are read through the ethtool ioctl (`github.com/safchain/ethtool`), and its absence or failure is tolerated.
- **Missing data is not an error.** A collector that can't read a file leaves zero values and adds a note to `Facts.Warnings`.

## Dependencies (pinned in go.mod)
cobra, `github.com/prometheus/procfs`, `github.com/expr-lang/expr`, `github.com/coreos/go-systemd/v22/journal`, `github.com/safchain/ethtool`, `gopkg.in/yaml.v3`.

## Package ownership
Exactly one agent owns each path at a time.

| Path | Purpose |
|---|---|
| `internal/facts/**` | collectors, `Collect(Root) (*Facts, error)` |
| `internal/fixture` | fixture loader: materializes `testdata/fixtures/<name>/fixture.yaml` into a temp dir → `facts.Root` + injected rates |
| `internal/workload` | `Detect(Root, *facts.Facts) (*Workload, error)` |
| `internal/metrics` | `Sample(Root) (*Snapshot, error)`, `Rates(prev, cur) map[string]float64` |
| `internal/rules` | schema, loader (embedded FS + optional `--rules-dir`), validation, expression env |
| `internal/advisor` | `Evaluate(rules, Input) []Recommendation` |
| `internal/render` | text, json, sysctl, limits, systemd, tuned, cmdline, nic, runtime |
| `internal/agent` | sampling loop, alert state machine, journald, state file |
| `internal/apply` | snapshot, apply, verify, rollback |
| `internal/cli/<cmd>.go` | one file per subcommand; `cli.go` only wires them |
| `rules/` | package `rulesfs` (`embed.go`, `//go:embed`) plus YAML packs |
| `testdata/fixtures/` | fixture trees |
| `deploy/` | systemd unit |

## Core types (Go)

```go
// internal/facts
type Root struct{ Proc, Sys, Etc, Run, Var string }
func DefaultRoot() Root
func RootAt(dir string) Root // dir/proc, dir/sys, ...

type Facts struct {
    Kernel   Kernel
    Host     Host
    CPU      CPU
    NUMA     []NUMANode
    Memory   Memory
    NICs     []NIC
    Block    []BlockDev
    Accel    []Accelerator
    RDMA     []RDMADev
    IOMMU    IOMMU
    Sysctl   map[string]string // dotted name → value with whitespace normalised to single spaces
    Limits   Limits
    Modules  map[string]bool   // /proc/modules names
    Procs    []Proc            // all processes: PID, Comm, Cmdline (args), limits, fd count
    Tuned    Tuned
    Bpftune  bool              // a process with comm "bpftune" is running
    Irqbalance bool
    Warnings []string
}
type Kernel struct {
    Release string; Major, Minor, Patch int
    Cmdline string; CmdlineArgs map[string]string // "iommu"→"pt", flags → ""
    AvailableCongestion []string // net.ipv4.tcp_available_congestion_control
}
type Host struct { Virt string /* "none","kvm","vmware","xen","hyperv","unknown" */; Container bool; ContainerRuntime string; Cloud string /* "aws","gcp","azure","" from DMI */ }
type CPU struct {
    Vendor, Model string; Sockets, Cores, Threads int // Cores = physical
    SMTActive bool; SMTControl string
    Flags map[string]bool
    ScalingDriver, Governor, EPP, IdleDriver string
}
type NUMANode struct { ID int; CPUs []int; MemTotalBytes uint64 }
type Memory struct {
    TotalBytes, SwapTotalBytes uint64
    THPEnabled, THPDefrag string // bracketed active value, e.g. "madvise"
    HugePages map[string]uint64  // "2048kB"→count, "1048576kB"→count
}
type NIC struct {
    Name, Driver string; SpeedMbps int; MTU int; NUMANode int; Virtual bool
    RxQueues, TxQueues int
    Combined, CombinedMax, RxRing, RxRingMax, TxRing, TxRingMax int // 0 when unknown
}
type BlockDev struct { Name, Transport /* "nvme","sata","virtio","" */ string; Rotational bool; Scheduler string; Schedulers []string; ReadAheadKB, NrRequests int }
type Accelerator struct {
    Vendor string // "nvidia","amd","intel","habana"
    PCIAddr, VendorID, DeviceID, Class, Driver string
    NUMANode int // -1 when unknown
    LinkSpeed, LinkSpeedMax string; LinkWidth, LinkWidthMax int
    IOMMUGroup string
}
type RDMADev struct { Name string; Ports []RDMAPort }
type RDMAPort struct { Num int; State, LinkLayer, Rate, NetDev string }
type IOMMU struct { Enabled bool; Passthrough bool; Groups int }
type Limits struct { FileMax, NrOpen, FileNrAllocated uint64 }
type Proc struct { PID int; Comm string; Cmdline []string; NoFileSoft, NoFileHard, MemlockSoft, MemlockHard uint64 /* math.MaxUint64 = unlimited */; OpenFDs int; Unit string /* systemd unit from /proc/<pid>/cgroup, "" if none */ }
type Tuned struct { Active bool; Profile string } // /etc/tuned/active_profile + tuned process
```

```go
// internal/workload
type Workload struct { Apps []App; Listeners int; ListenPorts []int }
type App struct {
    Name string // canonical ids: nginx haproxy envoy redis valkey postgres mysql kafka elasticsearch
                // vllm sglang tgi triton llama.cpp ollama pytorch jvm
    PIDs []int; Ports []int; Unit string
    NoFileSoft, NoFileHard, MemlockSoft uint64; MaxOpenFDs int
}
func (w *Workload) Has(name string) bool
```
Detection works by matching comm/cmdline (`vllm serve`, `vllm.entrypoints`, `sglang.launch_server`, `text-generation-launcher`, `tritonserver`, `llama-server`/`llama-cli`, `ollama`). Listeners are read from `/proc/net/tcp{,6}` in state `0A`.

```go
// internal/metrics
type Snapshot struct { Time time.Time; Values map[string]float64 }
func Sample(r facts.Root) (*Snapshot, error)
func Rates(prev, cur *Snapshot) map[string]float64 // per second; counters only
func IsCounter(key string) bool
```
Metric keys must use exactly this naming:
- `TcpExt.<Field>` and `IpExt.<Field>` from `/proc/net/netstat`
- `Tcp.<Field>` and `Udp.<Field>` from `/proc/net/snmp`
- `softnet.dropped`, `softnet.time_squeeze` (summed over CPUs)
- `conntrack.count`, `conntrack.max`
- `fs.file_nr.allocated`, `fs.file_nr.max`
- `sockstat.tcp.tw`, `sockstat.tcp.orphan`, `sockstat.tcp.inuse`
- `psi.{cpu,memory,io}.{some,full}.avg10` (gauges)
- `vmstat.<field>` from `/proc/vmstat`
- `numa.<node>.<field>` from `/sys/devices/system/node/node*/numastat`
- `nic.<if>.<stat>` from `/sys/class/net/<if>/statistics/*`

Gauges: conntrack.\*, fs.file_nr.\*, sockstat.\*, psi.\*.

## Rules

### File layout
- `rules/<area>/<pack>/*.yaml`, where area is `kernel`, `ai`, `apps`, `antipatterns` or `ecosystem`. Each file holds a YAML list of rules.
- Every pack directory has a `sources.yaml`: a list of `{url, project, covers, last_reviewed}`. Every rule `sources` URL must appear in some `sources.yaml`.
- `rules/ecosystem/tuned.yaml` and `rules/ecosystem/bpftune.yaml` are data, not rules. They map `tuned` profile → `{sysctl: value}` and list the sysctls bpftune tunes. The advisor uses them for `managed` status.
- `rules/embed.go`: `package rulesfs; //go:embed all:kernel all:ai all:apps all:antipatterns all:ecosystem; var FS embed.FS`. Directories must exist, with `.gitkeep` if empty.

### Schema
```yaml
- id: net.core.somaxconn            # unique, dotted, stable forever
  title: Listen backlog cap
  kind: sysctl                      # sysctl | sysfs | limit | nic | cmdline | service | runtime | report | antipattern
  param: net.core.somaxconn         # sysctl name | sysfs path under /sys with {item} placeholder | limit name | cmdline key | unit | free text
  domain: network                   # network | limits | memory | cpu | storage | ai
  foreach: ""                       # "" | nic | block | accel | rdma | app | numa  → one recommendation per item; expr sees `item`
  filter: ""                        # expr over item; keep items where true
  applies:                          # all present conditions must hold; else status not-applicable
    kernel: ">=4.9"                 # space-separated constraints: ">=4.9 <6.18"
    when: "workload.Listeners > 0"  # expr → bool
  recommend: "max(4096, ...)"       # expr → number | string | list (lists render space-joined)
  compare: gte                      # eq | gte | lte | set (any non-empty current is OK) — decides ok vs recommend; per-field for lists
  detect: ""                        # antipattern/report only: expr → bool; true means firing
  evidence:
    - metric: TcpExt.ListenOverflows
      rate_gt: 0                    # or value_gt, or ratio_gt with `of: conntrack.max`
  namespaced: true                  # settable inside a container's netns/ns; false → status host-only when Facts.Host.Container
  reboot: false                     # true for cmdline
  risk: low                         # low | medium | high
  rationale: >-
    Why. Mention kernel-version history.
  advice: ""                        # runtime/service/report: text; may use {{.item.Name}}-style Go templates over env
  verify: "TcpExt.ListenOverflows rate stays 0"
  sources: [https://docs.kernel.org/networking/ip-sysctl.html]
  tests:
    - fixture: web-16c-64g-nginx
      expect: { status: ok }        # status, optional proposed, optional target (item name)
```

### Expression environment (expr-lang)
- **Variables:** `facts` (*facts.Facts), `workload` (*workload.Workload), `current` (number if it parses, else string; `[]float64` for multi-field), `item` (the foreach item or nil).
- **Functions:**
  - `kver(">=5.4")`, `exists("/proc/sys/...")` (resolved under Root)
  - `sysctl(name) string`, `sysctlNum(name) float64`
  - `metric(key)`, `rate(key)`, both 0 if absent
  - `hasApp(name)`, `cpuFlag(name)`, `accelCount(vendor)`, `module(name)`
  - `bytes("4M")`, `clamp(x, lo, hi)`, `max`, `min`, `pow2ceil(x)`, `contains(list, s)`
- Evaluation errors become status `error` with a message. They never panic.

### Advisor output
```go
type Status string // recommend | ok | not-applicable | managed | host-only | firing | error
type Recommendation struct {
    RuleID, Kind, Domain, Param, Target, Title string // Target = item name for foreach rules
    Current, Proposed string
    Status Status; Risk string; Reboot bool; Score float64
    Evidence []EvidenceHit // {Metric string; Value, Threshold float64}
    ManagedBy string        // "tuned:<profile>" | "bpftune"
    Rationale, Advice, Verify string; Sources []string
}
type Input struct { Facts *facts.Facts; Workload *workload.Workload; Values, Rates map[string]float64; Root facts.Root; Ecosystem rules.Ecosystem }
```
**Score:** risk weight (low 1.0, medium 0.7, high 0.4) × (1 + 2·hits). Antipatterns get ×1.5. Recommendations are sorted by score, descending.

**Managed status:**
- If the param is in the active tuned profile's set, status is `managed`, and the report shows tuned's value.
- If bpftune is running and the param is in its list, status is `managed`, with the warning "writing this sysctl disables the bpftune tuner".

## Fixtures
`testdata/fixtures/<name>/fixture.yaml`:
```yaml
description: 2-socket 8xH100 HGX, Ubuntu 24.04 kernel 6.8, vLLM
synthetic: true
extends: ""                 # optional base fixture name; files below override/add
files:                      # path (relative to fake root) → content
  proc/sys/net/core/somaxconn: "4096\n"
  sys/bus/pci/devices/0000:18:00.0/vendor: "0x10de\n"
symlinks: { "sys/class/net/eth0": "../../devices/pci0000:00/0000:00:03.0/net/eth0" }
values: { conntrack.count: 1200 }      # injected metric gauges
rates:  { TcpExt.ListenOverflows: 3.2 } # injected per-second rates
```
`fixture.Load(t, name) (facts.Root, values, rates map[string]float64)` materializes the fixture into `t.TempDir()`. Rule-pack agents only ADD fixtures, named `<pack>-<variant>`, and may `extends:` shared ones. Shared fixtures are never edited after wave 2.

## CLI
- **Global flags:** `--root`, `--rules-dir`, `--domain`, `--pack`, `--json`.
- `scan [--sample DUR] [--all]`: `--all` includes ok, not-applicable and managed results.
- `explain <rule-id|param>`
- `generate --format sysctl|limits|systemd|tuned|cmdline|nic|runtime|all [--out DIR]`: prints to stdout when `--out` is not given.
- `sources [--all]`
- `validate`: checks the schema, that sources are registered, and that tests reference existing fixtures.
- `capture -o FILE [--fixture]`: `--fixture` emits a fixture.yaml. Strips cmdline args beyond argv[0..1], environ, and any file not on the allowlist.
- `agent [--interval 15s] [--state /run/kerneltuner/state.json]`
- `status [--state ...]`
- `apply [--rule ID ...] [--dry-run] [--yes]`
- `rollback [ID] [--list]`

**Exit codes:** 0 ok, 1 error, 2 = scan found `recommend` or `firing` at risk ≥ the `--fail-on` level (default off; for CI).

## Agent
- Samples every interval. Rates are computed over the last two samples.
- A rule moves to `firing` after 2 consecutive ticks with an evidence hit, and to `resolved` after 4 clean ticks. The same rule re-alerts at most once per hour.
- Each transition goes to journald (MESSAGE plus fields `KT_RULE`, `KT_PARAM`, `KT_TARGET`, `KT_CURRENT`, `KT_PROPOSED`, `KT_EVIDENCE`, `KT_STATE`) when `journal.Enabled()`, otherwise a JSON line to stderr.
- The state file is written atomically (temp file + rename).
- The agent never writes to the system beyond its state file.

## Apply / rollback
- Only `sysctl`, `sysfs` and `limit` kinds, with status `recommend` or `firing`.
- **Snapshot:** `<Var>/lib/kerneltuner/snapshots/<UTC-ts>.json` holds every prior value and the prior contents of every file kerneltuner writes. `apply` then:
  - writes `<Etc>/sysctl.d/90-kerneltuner.conf` (merged with existing managed entries)
  - writes sysctl and sysfs values live under Root
  - writes `limit` changes as systemd drop-ins `<Etc>/systemd/system/<unit>.d/90-kerneltuner.conf` and prints the `systemctl daemon-reload && systemctl restart <unit>` hint. It never restarts anything itself.
  - reads every value back. On mismatch it reports the failure and auto-rolls back that apply.
- `--yes` is required unless `--dry-run` is given.
- `rollback` restores values and files exactly, including deleting files that did not exist before.
- Without `--root` it refuses unless it is running as root (uid 0).
