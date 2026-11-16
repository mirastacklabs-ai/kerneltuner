# Notes (fetched 2026-11-16, all verified by reading the page text)

- redis.conf and valkey.conf: `tcp-backlog 511` is silently truncated to net.core.somaxconn; raise somaxconn and tcp_max_syn_backlog.
- redis.conf/valkey.conf: maxclients defaults to 10000; if the process file limit cannot be raised, maxclients becomes the current limit minus 32. Cluster bus uses two connections per node (redis.conf).
- redis.conf/valkey.conf `disable-thp yes`: no effect when the kernel THP setting is madvise or never; with `always` the server disables THP for itself.
- Redis admin guide and valkey.io/topics/admin: `vm.overcommit_memory = 1` and `echo never > /sys/kernel/mm/transparent_hugepage/enabled`. Neither ties the advice to a kernel version.
- Valkey is detected as app `valkey` (valkey-server); the Redis rules apply to both.
