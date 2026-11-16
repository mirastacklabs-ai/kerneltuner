# Status — 2026-11-16 (v1 in progress)

## Done (on `main`, pushed, CI green)
- Engine: facts (including accelerators and RDMA), workload detection (34 app ids plus `--workload`), metrics, rules engine and validator, advisor, all renderers, the commands `scan`/`explain`/`generate`/`sources`/`validate`/`capture`, the observe-only `agent` with journald plus `status`, and `apply`/`rollback` with snapshot and verify.
- Release engineering: goreleaser (tar.gz, deb, rpm, checksums, SBOM), CI and release workflows, `scripts/install.sh`, and the hardened systemd unit.
- Rule packs (80 rules, 87 sources, 234 golden tests):
  - kernel: network, limits, memory, CPU, storage
  - antipatterns
  - ecosystem data for tuned and bpftune
  - AI: NVIDIA, RDMA, AMD, Intel/Gaudi, CPU inference
  - apps: nginx, HAProxy, Redis/Valkey, vLLM, SGLang, TGI, llama.cpp, PyTorch

## In progress (parallel agent worktrees under `.claude/worktrees/`)
- App packs:
  - PostgreSQL, MySQL, MariaDB
  - Kafka, ZooKeeper, Pulsar, RabbitMQ, NATS
  - Cassandra, ClickHouse, Elasticsearch, OpenSearch, Memcached
  - kubelet, etcd, containerd, Cilium, CoreDNS, Prometheus, Fluent Bit
  - Envoy, Apache httpd
- Engine fix pass for the items in `docs/ENGINE-ISSUES.md`: duplicate reconciliation in generate/apply, exact integers, container-aware apps, distro/vulnerabilities/swaps facts, `foreach: cpu`, runtime `detect`, `require_evidence`, `scan --fixture`, faster golden tests, NVSwitch detection.

## Remaining to v1.0.0
1. Merge the app packs and the engine fixes, then run the full gate.
2. Docs: README usage, rule-authoring guide, CONTRIBUTING, SECURITY, NOTICE, CHANGELOG.
3. End-to-end tests in disposable containers: a real-kernel scan, evidence (ListenOverflows), an agent alert, and apply→rollback.
4. Read-only code and security review, followed by fixes.
5. Tag `v1.0.0`, then verify the release assets and `install.sh`.

## If interrupted
Unmerged agent work lives on local `worktree-agent-*` branches. Check them with `git worktree list` and `git log main..<branch>`. Merge the committed branches, and inspect `git -C <worktree> status` for uncommitted work.
