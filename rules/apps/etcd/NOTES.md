# Notes (fetched 2026-11-16, all verified by reading the page text)

Rules: `app.etcd.disk_priority` (runtime).

## Verified
- etcd.io/docs/latest/tuning: "An etcd cluster is very sensitive to disk latencies... disk activity from other processes may cause long fsync latencies... An etcd server can sometimes stably run alongside these processes when given a high disk priority. On Linux, etcd's disk priority can be configured with ionice: `sudo ionice -c2 -n0 -p $(pgrep etcd)`" (best effort, highest priority).
- Same page, Network: prioritise peer traffic (port 2380) over client traffic (2379) with `tc qdisc ... prio` when followers log "sending buffer is full". Symptom-triggered and not detectable from /proc, so no rule.
- op-guide/hardware: "etcd is very sensitive to disk write latency. Typically 50 sequential IOPS (e.g., a 7200 RPM disk) is required. For heavily loaded clusters, 500 sequential IOPS ... is recommended." Not a kernel setting, and kerneltuner does not measure IOPS; no rule.
- FAQ: the 2GB default storage quota; "To avoid swapping or running out of memory, the machine should have at least as much RAM to cover the quota". Sizing guidance, no host setting.

## Not documented (gaps)
- No sysctl, THP, I/O scheduler, readahead, LimitNOFILE or swap guidance on the fetched pages.
- The ionice rule cannot read the current I/O priority (not in facts), so it is `runtime` advice gated on etcd sharing the host with another detected app.
