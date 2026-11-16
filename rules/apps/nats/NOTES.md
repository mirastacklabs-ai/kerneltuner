# NATS notes (fetched 2026-11-16)

No rules: the official NATS documentation contains no host-level tuning guidance.

## Reviewed (all read, nothing kernel-related found)
- docs.nats.io: installation, nats_admin, slow_consumers, configuration, signals pages.
- The whole `nats-io/nats.docs` repo (tarball) searched for: ulimit, nofile, file descriptor, somaxconn, sysctl, swappiness, LimitNOFILE, max_map, tcp_, transparent. The only relevant hits are in `legacy/stan` (NATS Streaming, deprecated): the file store's `file_descriptors_limit` setting notes that "the need for file descriptors increase with the number of channels". That is an app setting for a legacy product with no OS value.

## Gap
No ulimit, sysctl, THP, or filesystem recommendations for nats-server or JetStream in the official docs. Not invented here.
