# Notes (fetched 2026-11-16, verified by reading the page text)

Verified and turned into rules:
- kernel-resources.html (18.4.4 Linux Memory Overcommit): "The default virtual memory behavior on Linux is not optimal for PostgreSQL ... the kernel might terminate a PostgreSQL server process, including the postmaster". "selecting strict overcommit mode via sysctl: sysctl -w vm.overcommit_memory=2" -> `app.postgres.overcommit_memory`.
- Same section: "set the process-specific OOM score adjustment value for the postmaster process to -1000 ... echo -1000 > /proc/self/oom_score_adj", plus `PG_OOM_ADJUST_FILE=/proc/self/oom_score_adj` and `PG_OOM_ADJUST_VALUE=0` for children -> `app.postgres.oom_score_adj` (runtime).
- kernel-resources.html (18.4.5 Linux Huge Pages): "Using huge pages reduces overhead when using large contiguous chunks of memory ... shared_memory_size_in_huge_pages ... sysctl -w vm.nr_hugepages=3170 ... hugepagesz=2M hugepages=3170" -> `app.postgres.huge_pages_unreserved` (report, no fixed value: depends on shared_buffers).
- runtime-config-resource.html: "huge_pages ... Valid values are try (the default), on, and off"; THP "has been known to cause performance degradation with PostgreSQL for some users on some Linux versions, so its use is currently discouraged" -> `app.postgres.transparent_hugepage` (report; fires only on `always`).

What the docs do NOT say (no rule written):
- No fixed vm.overcommit_ratio value, only "you might also wish to modify" it.
- No LimitNOFILE value. The page says raise the open-files limit on dedicated servers and tune max_files_per_process, but gives no number.
- No vm.swappiness, I/O scheduler, read-ahead, dirty_* or filesystem/mount-option guidance in the kernel-resources or resource-consumption pages.
- No THP replacement value (only "discouraged"); no statement that `madvise` is safe or unsafe.
- LimitMEMLOCK / `ulimit -l` is mentioned for huge pages ("It may also be necessary ... give permission to lock memory") but with no value.
- systemd RemoveIPC=no (logind.conf) is documented for source installs and non-system users; the host config is not visible in facts and a rule would be noisy and always on, so it is not a rule.
- SysV SHMMAX/SHMALL/SEMMNI tables apply only with shared_memory_type=sysv; the default mmap needs none of them, so no rule.
