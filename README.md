# kerneltuner

A Linux kernel tuning advisor and observe-only agent, shipped as a single Go binary. It looks at the hardware, kernel and workload on a server, watches live counters for real bottlenecks, and tells you what to tune, why, how risky it is, and what to leave alone.

## Positioning: complements, not competes

Tuning knowledge is scattered across kernel docs, vendor guides (nginx, HAProxy, Redis, Postgres, Kafka...), distro guides, and tools like `tuned` and `bpftune`. kerneltuner gathers it into one curated, version-aware knowledge base that cites its sources.

- Every rule links to its upstream source; credit stays with the original project.
- `generate` can emit a `tuned` profile, so existing `tuned` workflows keep working.
- If `tuned` or `bpftune` is active, parameters they manage are reported, not re-recommended. kerneltuner complements both: it adds cited, workload-aware rules and an advisory layer, not another auto-tuner.
- The rules are plain YAML under `rules/`, reusable by other tools.

## Scope (v1)

- Kernel: network, limits (fds, ports, conntrack), memory/VM, CPU, storage.
- AI inference hosts: NVIDIA GPUs, AMD GPUs (ROCm), Intel accelerators (Gaudi, Data Center GPU Max; depth limited by available upstream docs), CPU-only inference (llama.cpp, Ollama), and multi-node RDMA/NCCL.
- App packs cite their upstream guides (nginx, HAProxy, Redis, vLLM, SGLang, TGI, Triton, PyTorch...).

## Status

Early development. Phase 1 (skeleton) only, nothing is committed yet: all commands except `version` are stubs. See [docs/PLAN.md](docs/PLAN.md).

## Planned commands

`scan`, `explain`, `generate`, `sources`, `agent`, `status`, `apply`, `rollback`, `capture`, `version`.

## Build

Everything runs in Docker; nothing is installed on the host.

```
make build   # bin/kerneltuner (linux, static)
make test
make lint
make fmt
make tidy
```

## License

Apache-2.0, see [LICENSE](LICENSE).
