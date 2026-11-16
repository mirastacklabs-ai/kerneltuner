# Notes (fetched 2026-11-16)

- nginx core docs: worker_connections default 512, counts all connections including proxied ones, and "cannot exceed the current limit on the maximum number of open files, which can be changed by worker_rlimit_nofile". worker_rlimit_nofile: "used to increase the limit without restarting the main process".
- The worker_rlimit_nofile rule only checks worker processes (cmdline "worker process"); the master keeps the unit's LimitNOFILE.
