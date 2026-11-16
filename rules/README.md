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

- `kind: limit` uses `param: LimitNOFILE|LimitMEMLOCK` and `foreach: app`. `kind: nic` uses `param: combined|rx_ring|tx_ring` and `foreach: nic`. `kind: sysfs` uses an absolute `/sys/...` path with an optional `{item}` placeholder (the foreach item's name).
- `kind: cmdline`: `param` is the key; `recommend` is the value, or empty for a bare flag; `reboot: true` is required.
- `kind: service`: `recommend` is `"running"` or `"stopped"`.
- `kind: runtime`: only `advice` (a Go template over `item`, `facts`, `workload`, `target`) is used; the result is `recommend` whenever `applies` holds.
- `kind: antipattern|report`: `detect` decides `firing` vs `ok`.
- `evidence.metric` and `of` may contain `{item}`, replaced by the foreach item's name.
- In expressions, `contains(list, s)` tests list membership; `x in list` also works.
- A test's `expect.target` selects the recommendation of that foreach item.
