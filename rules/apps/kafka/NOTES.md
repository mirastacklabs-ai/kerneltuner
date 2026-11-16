# Kafka notes (fetched 2026-11-16, verified by reading the page text)

kafka.apache.org/documentation is a JS-rendered site, and `/documentation/#hwandos` fetches as an index only. The same text is the docs source in the Apache repo (`docs/operations/hardware-and-os.md`, `docs/configuration/`), which is what the rules cite.

## Verified
- File descriptors: "We recommend at least 100000 allowed file descriptors for the broker processes as a starting point." Segments plus connections need descriptors; mmap keeps an extra reference after close(). -> `app.kafka.nofile`.
- vm.max_map_count: "By default, on a number of Linux systems, the value of vm.max_map_count is somewhere around 65535. ... each log segment uses 2 map areas ... creating 50000 partitions on a broker will result allocation of 100000 map areas and likely cause broker crash with OutOfMemoryError (Map failed)". A formula, no value. -> `app.kafka.max_map_count` (runtime advice only).
- Mount options: "the following options are recommended ... at mount time: noatime ... Kafka does not rely on the atime attributes at all, so it is safe to disable this." XFS "does not require any change in the default settings"; XFS Request Local Time 160 ms vs 250 ms+ for best EXT4. -> `app.kafka.data_dir_mount` (runtime advice).
- `SocketServerConfigs.java` (socket.listen.backlog.size, default 50): "In Linux, you may also need to configure somaxconn and tcp_max_syn_backlog kernel parameters accordingly". -> `app.kafka.listen_backlog` (runtime, gated on ListenOverflows).

## The docs do NOT say
- Any vm.swappiness value (the current OS page has no swap section at all).
- Any vm.dirty_* values. The flush section says only to keep the defaults (no application fsync) and rely on the OS background flush; it describes pdflush behaviour and links an archived article, with no numbers.
- A recommended vm.max_map_count value, or max socket buffer value ("can be increased to enable high-performance data transfer between data centers", linking an external TCP-tuning page).
- I/O scheduler, readahead, THP, or LimitMEMLOCK guidance.
- EXT4 mount options (data=writeback, commit=, delalloc) are described as unsafe in multi-failure scenarios; deliberately not turned into rules.

## Engine gaps seen
- No mount facts (/proc/mounts), so noatime/XFS cannot be checked; the mount rule is unconditional runtime advice whenever Kafka is detected.
- Kafka is detected only when java's cmdline contains `kafka.Kafka`; Kafka run under another launcher class (e.g. containerized `kafka.docker`) is not matched.
