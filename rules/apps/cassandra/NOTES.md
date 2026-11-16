# Notes (fetched 2026-11-16, verified by reading the page or file text)

## Verified
- Production recommendations (cassandra.apache.org/doc/latest/cassandra/getting-started/production.html): read ahead "Spinning Disks 64KB, SSD 4KB" as an "Initial Recommendation"; "Read ahead is definitely not useful ... Solid state drives (SSDs)"; up to "5x latency and throughput performance penalty". Units: `blockdev --setra 8` is 4KB.
- debian/cassandra-sysctl.conf (trunk): `vm.max_map_count = 1048575` and `net.ipv4.tcp_keepalive_time=300`.
- debian/cassandra.postinst: "it is highly recommended that you set vm.max_map_count to 1048575 in the host."
- debian/cassandra.conf and redhat/cassandra.conf: `cassandra - memlock unlimited`, `cassandra - nofile 100000` (nproc 8096 on Debian, 32768 on RPM; `as unlimited`).
- conf/cassandra.yaml, row_cache_size comment: "Do never allow your system to swap."

## Not said by the docs (no rule written)
- The production page has no swap, vm.max_map_count, ulimit, THP, zone_reclaim or clock-sync section in the current docs (the 4.1 copy is the same). The max_map_count, keepalive and limits values come from the shipped package files, which give no rationale.
- THP: jvm-server.options notes only that AlwaysPreTouch "makes transparent hugepage allocation more effective"; no THP setting is recommended.
- cassandra.yaml `internode_tcp_user_timeout` comment mentions `net.ipv4.tcp_retries2` of about 8 only when the app timeout is set to 0, an app-config condition kerneltuner cannot see, so no rule.
- nproc: both packages ship a limit but they differ (8096 vs 32768) and the engine has no nproc limit kind.
- The read-ahead rule cannot tell which block devices hold data_file_directories.
