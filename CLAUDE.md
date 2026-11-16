# kerneltuner

Linux kernel tuning advisor + observe-only agent (Go). Complements `tuned`/`bpftune`/vendor guides by aggregating cited tuning knowledge. v1 scope: network, limits, memory, CPU, storage + AI inference (NVIDIA, AMD, Intel, CPU-only, RDMA). Plan: [docs/PLAN.md](docs/PLAN.md). **Engine contract (types, rule schema, ownership): [docs/DESIGN.md](docs/DESIGN.md)**, which wins over everything else.

## Layout
- `cmd/kerneltuner/main.go`: tiny entrypoint; command wiring lives in `internal/cli/`.
- `internal/facts/accel`: accelerator + RDMA detection via sysfs PCI.
- `internal/{facts,workload,metrics,rules,advisor,render,agent,apply}`: see each `doc.go`.
- `rules/`: YAML knowledge base (`kernel/{network,limits,memory,cpu,storage}`, `ai/{nvidia,amd,intel,cpu-inference,rdma}`, `apps/<project>`, `antipatterns`, `ecosystem`). Each pack dir has its own `sources.yaml`; `rules/embed.go` embeds them.
- `testdata/fixtures/<name>/fixture.yaml`: fake /proc + /sys trees (supports `extends`), loaded by `internal/fixture`.

## Commands (all run in Docker, Go cache in volume `kerneltuner-gocache`)
- `make build`: static linux binary to `bin/kerneltuner`
- `make test`: `go test -race ./...`
- `make lint`: golangci-lint (pinned image)
- `make fmt`, `make tidy`

## Rules
- Install nothing on the host (macOS): no brew, no `go install`. Docker only.
- Rules are data: every rule must cite a source URL (registered in its pack's `sources.yaml`) and have a golden test. Only `verified-by-fetch` claims from `docs/research/` may become rules.
- `apply`/`rollback` tests must NEVER touch the Mac host or the Colima VM's real sysctls. Use disposable containers or fixtures only.
- Never commit secrets; use gitignored `.env.local`.
- Do not commit unless asked; stage files by name.

## Status
See [docs/STATUS.md](docs/STATUS.md).
