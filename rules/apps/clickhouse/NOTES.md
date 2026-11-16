# Notes (fetched 2026-11-16, verified by reading the page or file text)

## Verified (clickhouse.com/docs/operations/tips)
- "Always use the performance scaling governor."
- "Do not disable overcommit. The value cat /proc/sys/vm/overcommit_memory should be 0 or 1."
- "Always set transparent huge pages to madvise." ClickHouse "still warns at startup if THP is set to always".
- "For HDD, choose the mq-deadline or CFQ scheduler, and for SSD, choose noop. Don't reduce the 'readahead' setting."
- "Ext4 is the most reliable option. Set the mount options noatime. XFS works well too."; no compressed or encrypted filesystems; NFS "not the best idea".
- packages/clickhouse-server.service (ClickHouse repo): `LimitNOFILE=500000`.

## Not said by the docs (no rule written)
- SSD scheduler: the page says "noop", which is the legacy single-queue name (the blk-mq equivalent is `none`); not mapped without a source, so no rule. CFQ no longer exists, so the HDD rule checks only mq-deadline.
- readahead: "Don't reduce" with no value and no default, so no rule.
- No swap guidance, no vm.max_map_count, no file-descriptor limit on the tips page (the nofile rule cites the shipped unit).
- RAID stripe_cache_size (`/sys/block/md2/md/stripe_cache_size` 4096) is documented but device-specific; not written.
- fstrim for NVMe/SSD and the mount options have no fact source; covered only by the always-on runtime advice rule.
