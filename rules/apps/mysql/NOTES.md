# Notes (fetched 2026-11-16 from dev.mysql.com/doc/refman/8.4, verified by reading the page text)

Verified and turned into rules:
- server-system-variables.html, open_files_limit: "The effective open_files_limit value is based on ... 10 + max_connections + (table_open_cache * 2). Using the defaults for these variables yields 8161." and "On Linux systems using systemd, the value cannot be set greater than LimitNOFILE (this is DefaultLimitNOFILE, if LimitNOFILE is not set)" -> `app.mysql.nofile` (8161 is the default-config value only).
- optimizing-innodb-diskio.html: "Use a noop or deadline I/O scheduler with native AIO on Linux ... Generally, noop and deadline I/O schedulers are recommended. Conduct benchmarks to determine which I/O scheduler provides the best results" -> `app.mysql.io_scheduler` (noop = none, deadline = mq-deadline on blk-mq kernels; either accepted).

Read but not turned into rules:
- large-page-support.html: gives the recipe (`vm.nr_hugepages` = innodb_buffer_pool_size / Hugepagesize, then `large-pages=ON`; large pages are "disabled by default") but does not recommend enabling them, and the page does not mention memlock/LimitMEMLOCK. Whether large-pages is on lives in my.cnf, not in facts, so no rule.
- innodb-linux-native-aio.html: needs libaio; `innodb_use_native_aio=0` fallback. No fs.aio-max-nr guidance, so no rule.
- optimizing-innodb-diskio.html also covers O_DIRECT / fsync and `innodb_flush_method`: app tuning without a host angle.
- memory-use.html only says an oversized buffer pool "may cause swapping due to competition for memory".

What the docs do NOT say:
- No vm.swappiness value, no THP guidance, no read-ahead, no dirty_* sysctls, no filesystem or mount options (the diskio page only discusses InnoDB flush methods and schedulers), no vm.overcommit guidance, no NUMA interleave advice on these pages.
- No LimitNOFILE value other than the derived 8161 for defaults.
