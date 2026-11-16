# Notes (fetched 2026-11-16)

No rules. Pages read: prometheus.io/docs/prometheus/latest/storage/, /command-line/prometheus/, and introduction/faq.

## Verified
- storage page: "Non-POSIX compliant filesystems are not supported for Prometheus' local storage as unrecoverable corruptions may happen. NFS filesystems (including AWS's EFS) are not supported." Real host guidance, but kerneltuner facts do not expose mounts or the TSDB path, so it cannot be detected; no rule.

## Not documented (gaps)
- No file descriptor / LimitNOFILE, vm.max_map_count, swap, THP or inotify guidance on the fetched pages.
