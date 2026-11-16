# Rule packs

Rules are data. Layout: `rules/<area>/<pack>/*.yaml`, where area is `kernel`, `ai`, `apps`, `antipatterns` or `ecosystem`. Each file holds a YAML list of rules. YAML is strict: unknown fields are an error.

## Sources registry

Every pack directory has its own `sources.yaml`: a list of `{url, project, covers, last_reviewed}`. Every URL in a rule's `sources` must appear in the `sources.yaml` of some pack (`kerneltuner validate` checks this). There is no global registry.

## Ecosystem data

`rules/ecosystem/tuned.yaml` and `rules/ecosystem/bpftune.yaml` are data, not rules. They drive the `managed` status. Either file may be empty or missing.

`tuned.yaml`: tuned profiles and the sysctls they set. `include` chains are resolved at load time (the child overrides the parent; an `include` naming a profile not listed here ends the chain). Sysctl names are dotted; values are scalars.

```yaml
profiles:
  throughput-performance:
    sysctl:
      vm.swappiness: 10
      net.core.somaxconn: 2048
  network-throughput:
    include: throughput-performance
    sysctl:
      net.core.rmem_max: 16777216
```

`bpftune.yaml`: the sysctls bpftune tunes at runtime.

```yaml
sysctls:
  - net.ipv4.tcp_rmem
  - net.core.somaxconn
```

A `recommend` result is reported as `managed` (instead of `recommend`) when the param is in the active tuned profile's set or, if `bpftune` is running, in the bpftune list. A param that already satisfies the rule stays `ok`.

## Schema

The rule schema, expression environment and test format are defined in [docs/DESIGN.md](../docs/DESIGN.md). The Go package `rulesfs` (`embed.go`) embeds this tree.

Conventions the validator and advisor enforce:

- `kind: limit` uses `param: LimitNOFILE|LimitMEMLOCK|LimitNPROC` and `foreach: app` (`LimitNPROC` compares `App.NProcSoft`, the smallest non-zero soft "Max processes" of the app's PIDs). `kind: nic` uses `param: combined|rx_ring|tx_ring` and `foreach: nic`. `kind: sysfs` uses an absolute `/sys/...` path with an optional `{item}` placeholder (the foreach item's name).
- `kind: cmdline`: `param` is the key; `recommend` is the value, or empty for a bare flag; `reboot: true` is required.
- `kind: service`: `recommend` is `"running"` or `"stopped"`.
- `kind: runtime`: only `advice` (a Go template over `item`, `facts`, `workload`, `target`) is used; the result is `recommend` whenever `applies` holds.
- `kind: antipattern|report`: `detect` decides `firing` vs `ok`.
- `evidence.metric` and `of` may contain `{item}`, replaced by the foreach item's name.
- In expressions, `contains(list, s)` tests list membership; `x in list` also works.
- A test's `expect.target` selects the recommendation of that foreach item.

## Engine behaviour for rule authors

### Expressions
- `recommend` is an expr expression, so a string value must be a quoted expr string. In YAML that is `recommend: '"running"'` (or `recommend: "\"running\""`). A bare `recommend: running` is an identifier, not a string, and fails to compile.
- Numeric helpers `pow2ceil`, `clamp`, `bytes`, `max` and `min` take ints or floats. When every argument is an int the result is an exact int, so `9223372036854775807` stays exact. Proposed and current values that are integers are never formatted through float64. `sysctlNum`, `metric` and `rate` return floats; `current` is a float64 for numeric values.
- Facts you can read: `facts.Host.Distro.ID` / `.VersionID` (from `<etc>/os-release`, falling back to `usr/lib/os-release`), `facts.CPU.Vulnerabilities` (map `spectre_v2` -> text), `facts.CPU.IDs`, `facts.Swaps` (`Path`, `Type`, `SizeKB`, `Priority`, `Zram`), `facts.Sysctl` now also holds `fs.inotify.*` and `net.bridge.*`. RoCE netdevs that only appear under `/sys/class/infiniband/*/device/net/*` are in `facts.NICs`.
- Accelerators have `Kind`: `gpu`, `nvswitch` (NVIDIA PCI class 0x0680) or `npu` (Habana). `accelCount(vendor)` counts everything except NVSwitch bridges; `accelKindCount(vendor, kind)` counts one kind (`""` matches any vendor or kind). `foreach: accel` also iterates NVSwitch bridges, so add `filter: item.Kind == "gpu"` where only GPUs matter.
- Containers: `facts.Procs[].Container` and `workload` apps' `Container` hold the runtime (`docker`, `containerd`, `crio`, `podman`, or `""`), derived from the cgroup path (`docker-<id>.scope`, `cri-containerd-<id>.scope`, `crio-<id>.scope`, `libpod-<id>.scope`, `/docker/<id>`). `appContainer("vllm")` returns it by app name. A containerised app has no `Unit` unless a `.service` is in its cgroup path, so its `foreach: app` Target is the app name. `App.NoFileSoft` is the minimum across the app's PIDs and `App.NoFileSoftMax` the maximum (a low-limit master with raised workers).
- Facts has no process environ, by design (privacy): rules cannot see `OMP_*` or `ONEDNN_*` variables.
- `cmdlineHas(key, item)` tests a comma-separated cmdline value: with `pci=realloc=off,nommconf`, `cmdlineHas("pci", "realloc=off")` is true. For `kind: cmdline`, a current or proposed value containing a comma is compared as a set: the rule is `ok` when every proposed item is among the current items. An empty item tests only that the key exists.

### Expression builtins and new helpers
- **expr builtins** (from expr-lang, usable anywhere an expression is). Collections: `all(list, pred)`, `any`, `none`, `one`, `count(list, pred)`, `filter(list, pred)`, `map(list, fn)`, `sum`, `len`, `first`, `last`, `sort`, `uniq`, `join`. Maps: `keys(m)` and `values(m)` (a map such as `facts.Sysctl`, `facts.CPU.Vulnerabilities` or `facts.Kernel.Config`). Strings: `upper`, `lower`, `trim`, `split`, `hasPrefix`, `hasSuffix` and the operators `startsWith`, `endsWith`, `matches`, `contains`, `in`. Other: `let x = ...; expr`, `a ?? b`, `a ? b : c`. Inside a predicate `#` is the element and `.Field` reads its field. Examples:
  - `all(values(facts.CPU.Vulnerabilities), {# == "Not affected" or # startsWith "Mitigation"})`: every CPU vulnerability is mitigated.
  - `none(facts.Block, {.Rotational})`, `count(facts.Block, {.Rotational}) > 0`
  - `len(filter(facts.Mounts, {.FSType startsWith "nfs"})) > 0`
  - `"vm.swappiness" in keys(facts.Sysctl)`
  - `any(workload.Apps, {.Name == "redis" and .OOMScoreAdj > -500})`
  
  Note that `contains(list, s)` (function form) is the engine's list test; the `contains` operator is expr's string test.
- `sysctl(name)` and `sysctlNum(name)` read `facts.Sysctl` and fall back to `<Root.Proc>/sys/<name with dots as slashes>`, the way `kind: sysctl` does, so any readable sysctl works (`user.max_user_namespaces`, `kernel.*` outside the collected set). An unreadable name gives `""` and `0`.
- **Unlimited.** `unlimited` is `math.MaxUint64` (a `uint64`; the expr parser rejects integer literals above `9223372036854775807`, so never write `18446744073709551615`). `isUnlimited(x)` is true for `unlimited` as uint64 or float64 (`current` of a limit rule is the float64 of it) and for the strings `"unlimited"` and `"infinity"`. Write `isUnlimited(current) or current >= bytes("64G")` instead of `current < 1e18` hacks. The same value is `workload` apps' `NoFileSoft`, `MemlockSoft` and `NProcSoft` when the limit is unlimited, and ``Proc` limits likewise.
- **Mounts.** `facts.Mounts` is the table from `/proc/self/mounts` (else `/proc/mounts`): `Device`, `MountPoint`, `FSType`, `Options` (list), `Disk` (parent whole-disk name, `nvme0n1`, resolved through `/sys/class/block`; `dm`/`md` devices go to their first slave; `""` for tmpfs, network and unresolvable devices). Helpers take a path anywhere on the host, which is matched to the longest mount point holding it: `mountOf(path)` (a `Mount`; the zero value with an empty `MountPoint` when there is no table), `fsType(path)`, `mountHasOpt(path, opt)` (`opt` is `noatime` or `key=value`; a bare key also matches `key=value`), `blockOf(path)` (the disk name, so `filter: item.Name == blockOf("/var/lib/cassandra")` on a `foreach: block` rule). Paths are host paths, not under `--root`.
- **Kernel config.** `facts.Kernel.Config` is `CONFIG_X` -> `y`, `m`, `n` (`# CONFIG_X is not set`), a number or an unquoted string, read from `<Proc>/config.gz` or `<root>/boot/config-<release>`. It is nil when neither exists, so `kconfig("CONFIG_BPF_JIT")` returns `""` (unknown) as well as for an option the kernel does not have: test `== "y"` rather than `!= "n"`. `capture` records a fixed list of options only.
- **Processes.** `facts.Procs[]` gains `OOMScoreAdj`, `NProcSoft`, `NProcHard` and `CoreSoft` (unlimited is MaxUint64). `workload.Apps[]` gains `NProcSoft` (minimum non-zero across PIDs) and `OOMScoreAdj` (the maximum across PIDs, so the least protected process decides). I/O priority (`ionice`) is not a fact: the kernel only exposes it through the `ioprio_get` syscall, not `/proc`.

### Kinds and status
- `kind: service` is `running` when a process matches the unit (`.service` suffix optional) by comm (including the kernel's 15-character truncation, so `nvidia-persistenced` matches comm `nvidia-persiste`), by argv[0] basename, or by systemd unit name.
- `kind: runtime` without `detect` is always `recommend` while `applies` holds (there is no ok state). With `detect`, true means the advice is needed (`recommend`) and false means `ok`. `kind: service` takes the same optional `detect` (it sees `current` as `running` or `stopped`): true means `recommend`, false `ok`; without it the status compares current with `recommend`.
- Evidence only raises the score (risk weight x (1 + 2 x hits)); it never changes the status. Set `require_evidence: true` (needs at least one `evidence` entry) so that `recommend` or `firing` needs at least one evidence hit, and otherwise reports `ok`. Gate on metrics that the status must depend on with `applies.when` or `detect` instead.
- `foreach: cpu` iterates logical CPU ids; the item is the int id and `{item}` is `cpu0`, `cpu1`, ... so `param: /sys/devices/system/cpu/{item}/cpufreq/scaling_governor` works. With `foreach_summary: true` (any `foreach`), results identical apart from the item are collapsed into one recommendation. For CPUs the Target is a range such as `cpu0-63 (64)` (`cpu0-3,8 (5)` with gaps); other kinds join names (`eth0,eth1 (2)`). The recommendation's `Targets` lists every item and `apply` writes each one (absent paths such as offline CPUs are skipped). A test's `expect.target` must use the collapsed name. Results that differ (a different current value, or advice that mentions `{{.item}}`) stay separate rows.

### Duplicates
Several rules may target the same setting `(kind, param, target)`: for example `vm.max_map_count` from two apps, or `LimitMEMLOCK` on `vllm.service`. `scan` and `--json` show every recommendation. `generate` and `apply` merge them into one setting: `compare: gte` takes the max (per field), `lte` the min; for `eq`, `set` or mixed modes with different values the highest-scored recommendation wins and a warning is printed. The generated comment lists every contributing rule id, and the text report adds a `conflicts:` line when proposals disagree. A rule that is already satisfied (`ok`, `compare: eq`) at a different value than a recommended rule also counts (`app.redis.overcommit_memory` at 1 against `app.postgres.overcommit_memory` wanting 2): `advisor.Conflicts` reports it and `advisor.ConflictBlock` formats the `CONFLICT:` block. Give rules that share a setting the same `compare` mode and compatible thresholds so they merge instead of conflicting.

### Testing a rule against a fixture
`kerneltuner scan --fixture NAME` is a hidden developer flag (also accepted by `generate` and `apply`). It materializes `testdata/fixtures/NAME` into a temp dir, uses it as `--root` and injects the fixture's `values` and `rates`, so you can see what your rule does without writing a golden test first:

```
kerneltuner --rules-dir rules --pack apps/nginx --json scan --all --fixture nginx-raised-nofile
```

Run it from inside the repository (the fixtures are found by walking up to `go.mod`). `--rules-dir` replaces the embedded packs (point it at a rules tree such as `rules`); `--pack` limits the run to one pack. `kerneltuner validate` runs every golden test; each fixture is materialized and collected once per run.
