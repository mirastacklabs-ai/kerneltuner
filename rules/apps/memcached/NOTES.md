# Notes (fetched 2026-11-16, verified by reading the page text)

## Verified
- doc/memcached.1 (man page): `-b, --listen-backlog=<num>` "Set the backlog queue limit to <num> connections. The default is 1024."; `-c` default 1024 connections; `-k` "somewhat dangerous option with large caches"; `-L` "Try to use large memory pages (if available)".
- README.md: "the -k (mlockall) option to memcached might be dangerous when using a large cache. Just make sure the memcached machines don't swap."
- wiki/Hardware: "Do not over-allocate memory and expect swap to save you"; "Often it's a good idea to run hosts with very little swap, or no swap at all"; NUMA: "run one memcached instance per NUMA node and bind the instances via numactl".
- docs.kernel.org ip-sysctl: net.core.somaxconn is the "Limit of socket listen() backlog" (default 4096, 128 before 5.4). The memcached pages do not themselves say the kernel truncates -b.

## Not said by the docs (no rule written)
- No sysctl, THP, ulimit or max_map_count guidance anywhere in the wiki or man page.
- `-c` connections vs the open-files limit: no formula for extra descriptors, so no nofile rule. The shipped scripts/memcached.service sets no LimitNOFILE.
- `-L` large pages: no mention of vm.nr_hugepages or THP, so nothing host-level to check.
- -k and RLIMIT_MEMLOCK: not documented, so no memlock rule.
- Custom `-b` / `-c` values are invisible (no cmdline parsing in the rule), so the backlog rule assumes the default.
