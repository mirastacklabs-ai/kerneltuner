# Pulsar / BookKeeper notes (fetched 2026-11-16)

No rules: the official Pulsar and BookKeeper documentation contains no host-level tuning guidance.

## Reviewed (all read, nothing kernel-related found)
- pulsar.apache.org/docs/next/deploy-bare-metal/, administration-zk-bk, performance-pulsar-perf, deploy-kubernetes, helm-deploy, concepts-architecture-overview.
- bookkeeper.apache.org/docs/reference/config, admin/bookies, admin/perf, getting-started/installation.
- Shipped config and scripts: Pulsar `conf/broker.conf`, `conf/bookkeeper.conf`, `conf/pulsar_env.sh`, `bin/pulsar`, `bin/pulsar-daemon`; BookKeeper `conf/bk_server.conf`.
- Searched for: ulimit, nofile, file descriptors, somaxconn, backlog, swap, swappiness, sysctl, vm., dirty, noatime, scheduler, readahead, memlock, huge/THP, max_map_count. Hits were unrelated ("swap" = BookKeeper ledger-cache swap, "backlog" = message backlog, `-XX:+UseTransparentHugePages` in a pulsar-perf example JVM flag, `openFileLimit` = a BookKeeper app setting).

## What the docs do say that is only app-level
- `journalSyncData=true`: fsync before ack; disabling risks data loss (page cache only). `journalRemoveFromPageCache=true`, `dbStorage_directIOEntryLogger` (O_DIRECT), `PULSAR_GC="-XX:+PerfDisableSharedMem -XX:+AlwaysPreTouch"`.
- Ledger storage advice (multiple HDDs, separate journal device) is hardware layout, not a kernel parameter.

## Gap
No vm.*, ulimit, or I/O scheduler advice exists in official Pulsar or BookKeeper docs. The detector matches Pulsar by `org.apache.pulsar` in a java cmdline; a standalone BookKeeper bookie (`org.apache.bookkeeper`) falls under the generic `jvm` app, not `pulsar`.
