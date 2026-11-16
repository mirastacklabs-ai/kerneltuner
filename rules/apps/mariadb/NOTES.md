# Notes (fetched 2026-11-16 from mariadb.com/kb/en, verified by reading the page text)

Verified and turned into rules:
- configuring-linux-for-mariadb: "we recommend using the none (previously called noop) scheduler. Recommended schedulers are none, for SSDs and NVMes, and mq-deadline (previously called deadline) for hard disks" -> `app.mariadb.io_scheduler`.
- Same page: "On many systems, both the soft and hard limit default to 1024. On an active database server, it is very easy to exceed 1024 open file descriptors"; example `mysql soft nofile 65535` / `mysql hard nofile 65535` -> `app.mariadb.nofile` (65535 is the KB's example value).
- server-system-variables, open_files_limit: "MariaDB cannot set this to exceed the hard limit imposed by the operating system"; 0 means MAX(max_connections*5, max_connections + table_open_cache*2).
- configuring-swappiness: "For MariaDB databases, it is recommended to set swappiness to a value of 1"; the page text also shows `vm.swappiness = 0` examples and a summary saying "ideally to 1"; the explicit recommendation is 1 -> `app.mariadb.swappiness`. "it can be prudent to leave some swap space".

Read but not turned into rules:
- server-system-variables, large_pages: Linux needs kernel.shmmax larger than the allocation, vm.nr_hugepages larger than the usage, and "ulimit for locked memory ... (systemd LimitMEMLOCK)"; allocation otherwise "falls back to conventional memory" with a log warning. No numbers, and large_pages is a my.cnf option invisible in facts (off by default), so no rule.
- innodb-system-variables, innodb_use_native_aio: MariaDB 10.6.5/10.7.1+ switches it to 0 itself on Linux 5.11-5.15 (io-uring bug MDEV-26674) and treats 5.15.3+ as fixed. The server handles this automatically and distro kernel versions (Ubuntu 5.15.0-NN) do not map onto upstream patch numbers, so no rule.
- configuring-mariadb-for-optimal-performance: pure app tuning (innodb_buffer_pool_size about 80% of memory, innodb_flush_method), no host angle.
- Core file size (`mysql soft core unlimited`) is debugging, not performance tuning.

What the docs do NOT say:
- No THP guidance, no vm.overcommit, no dirty_* sysctls, no read-ahead, no filesystem or mount options, no kernel command-line parameters (other than naming hugepages/hugepagesz for large_pages), no fs.aio-max-nr, and no NUMA advice on the pages fetched.
- The swappiness KB page is inconsistent between its 0 example and its final value-of-1 recommendation; the rule uses the explicit recommendation.
