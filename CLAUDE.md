# kerneltuner

Linux kernel tuning advisor + observe-only agent (Go). Complements `tuned`/`bpftune`/vendor guides by aggregating cited tuning knowledge. v1 scope: network, limits, memory, CPU, storage + AI inference (NVIDIA, AMD, Intel, CPU-only, RDMA). Full design: [docs/PLAN.md](docs/PLAN.md).

## Layout
- `cmd/kerneltuner/main.go`: tiny entrypoint; command wiring lives in `internal/cli/`.
- `internal/facts/accel`: accelerator + RDMA detection via sysfs PCI.
- `internal/{facts,workload,metrics,rules,advisor,render,agent,apply}`: see each `doc.go`.
- `rules/`: YAML knowledge base (`kernel/{network,limits,memory,cpu,storage}`, `ai/{nvidia,amd,intel,cpu-inference,rdma}`, `apps/<project>`, `antipatterns`, `sources.yaml`).
- `testdata/fixtures/<name>/{proc,sys,meta.json}`: captured host fixtures.

## Commands (all run in Docker, Go cache in volume `kerneltuner-gocache`)
- `make build`: static linux binary to `bin/kerneltuner`
- `make test`: `go test -race ./...`
- `make lint`: golangci-lint (pinned image)
- `make fmt`, `make tidy`

## Rules
- Install nothing on the host (macOS): no brew, no `go install`. Docker only.
- Rules are data: every rule must cite a source URL (registered in `rules/sources.yaml`) and have a golden test.
- `apply`/`rollback` tests must NEVER touch the Mac host or the Colima VM's real sysctls. Use disposable containers or fixtures only.
- Never commit secrets; use gitignored `.env.local`.
- Do not commit unless asked; stage files by name.

## Status
Phase 1 (skeleton) done; later phases per docs/PLAN.md.
