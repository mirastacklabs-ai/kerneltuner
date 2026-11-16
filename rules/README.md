# Rule packs

Rules are data. Layout: `rules/<area>/<pack>/*.yaml`, where area is `kernel`, `ai`, `apps`, `antipatterns` or `ecosystem`. Each file holds a YAML list of rules.

## Sources registry

Every pack directory has its own `sources.yaml`: a list of `{url, project, covers, last_reviewed}`. Every URL in a rule's `sources` must appear in the `sources.yaml` of some pack (`kerneltuner validate` checks this). There is no global registry.

## Ecosystem data

`rules/ecosystem/tuned.yaml` and `rules/ecosystem/bpftune.yaml` are data, not rules. They drive the `managed` status.

## Schema

The rule schema, expression environment and test format are defined in [docs/DESIGN.md](../docs/DESIGN.md). The Go package `rulesfs` (`embed.go`) embeds this tree.
