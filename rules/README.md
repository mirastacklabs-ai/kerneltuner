# Rule packs

Rules are data. Layout: `rules/<area>/<pack>/*.yaml`, where area is `kernel`, `ai`, `apps`, `antipatterns` or `ecosystem`. Each file holds a YAML list of rules. YAML is strict: unknown fields are an error.

## Sources registry

Every pack directory has its own `sources.yaml`: a list of `{url, project, title, covers, last_reviewed}` (`title` is optional: the page's human title, shown in reports). Every URL in a rule's `sources` must appear in the `sources.yaml` of some pack (`kerneltuner validate` checks this). There is no global registry.

## Citations

A rule's `sources` list takes two forms, which can be mixed:

```yaml
sources:
  - https://docs.kernel.org/networking/ip-sysctl.html          # URL only: no quote yet
  - url: https://docs.kernel.org/networking/ip-sysctl.html
    section: somaxconn                                          # optional heading or anchor text
    quote: >-
      somaxconn - INTEGER: Limit of socket listen() backlog.    # at most 300 characters
```

**The quote must be verbatim from the cited page and must support the claim the rule makes.** Copy it from the page; do not paraphrase, shorten with `...`, merge sentences from different places, or quote text that merely sits near the claim. Reports print it under the rule as evidence, and `kerneltuner sources --check` fetches the page weekly and fails if the quote is gone, so a paraphrase breaks the check.

Rules of thumb:
- One quote per source entry. If a rule rests on two passages, list the URL twice with a quote each.
- Prefer the sentence that carries the number or the behaviour (the default, the version, the unit), not the page introduction.
- Plain text only: no markdown or HTML. Whitespace and typographic quotes or dashes do not matter to the check; wording and case do.
- `section` is the nearest heading or the anchor (`#tcp_rmem`), so a reader can find the passage.
- Bump `last_reviewed` in `sources.yaml` when you re-read the page.

`kerneltuner validate` rejects a quote over 300 characters and warns about sources without one; `kerneltuner validate --strict-citations` makes that an error (it will become the default once quotes are backfilled). `kerneltuner sources --check [--only <pack>]` is the opt-in network check.

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

- `kind: limit` uses `param: LimitNOFILE|LimitMEMLOCK` and `foreach: app`. `kind: nic` uses `param: combined|rx_ring|tx_ring` and `foreach: nic`. `kind: sysfs` uses an absolute `/sys/...` path with an optional `{item}` placeholder (the foreach item's name).
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

### Kinds and status
- `kind: service` is `running` when a process matches the unit (`.service` suffix optional) by comm (including the kernel's 15-character truncation, so `nvidia-persistenced` matches comm `nvidia-persiste`), by argv[0] basename, or by systemd unit name.
- `kind: runtime` without `detect` is always `recommend` while `applies` holds (there is no ok state). With `detect`, true means the advice is needed (`recommend`) and false means `ok`. `kind: service` takes the same optional `detect` (it sees `current` as `running` or `stopped`): true means `recommend`, false `ok`; without it the status compares current with `recommend`.
- Evidence only raises the score (risk weight x (1 + 2 x hits)); it never changes the status. Set `require_evidence: true` (needs at least one `evidence` entry) so that `recommend` or `firing` needs at least one evidence hit, and otherwise reports `ok`. Gate on metrics that the status must depend on with `applies.when` or `detect` instead.
- `foreach: cpu` iterates logical CPU ids; the item is the int id and `{item}` is `cpu0`, `cpu1`, ... so `param: /sys/devices/system/cpu/{item}/cpufreq/scaling_governor` works. With `foreach_summary: true` (any `foreach`), results identical apart from the item are collapsed into one recommendation. For CPUs the Target is a range such as `cpu0-63 (64)` (`cpu0-3,8 (5)` with gaps); other kinds join names (`eth0,eth1 (2)`). The recommendation's `Targets` lists every item and `apply` writes each one (absent paths such as offline CPUs are skipped). A test's `expect.target` must use the collapsed name. Results that differ (a different current value, or advice that mentions `{{.item}}`) stay separate rows.

### Duplicates
Several rules may target the same setting `(kind, param, target)`: for example `vm.max_map_count` from two apps, or `LimitMEMLOCK` on `vllm.service`. `scan` and `--json` show every recommendation. `generate` and `apply` merge them into one setting: `compare: gte` takes the max (per field), `lte` the min; for `eq`, `set` or mixed modes with different values the highest-scored recommendation wins and a warning is printed. The generated comment lists every contributing rule id, and the text report adds a `conflicts:` line when proposals disagree. Give rules that share a setting the same `compare` mode and compatible thresholds so they merge instead of conflicting.

### Testing a rule against a fixture
`kerneltuner scan --fixture NAME` is a hidden developer flag (also accepted by `generate` and `apply`). It materializes `testdata/fixtures/NAME` into a temp dir, uses it as `--root` and injects the fixture's `values` and `rates`, so you can see what your rule does without writing a golden test first:

```
kerneltuner --rules-dir rules --pack apps/nginx --json scan --all --fixture nginx-raised-nofile
```

Run it from inside the repository (the fixtures are found by walking up to `go.mod`). `--rules-dir` replaces the embedded packs (point it at a rules tree such as `rules`); `--pack` limits the run to one pack. `kerneltuner validate` runs every golden test; each fixture is materialized and collected once per run.
