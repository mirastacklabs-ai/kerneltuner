# Memory pack notes

Newly verified by fetch on 2026-11-16 (beyond docs/research/cpu-mem-research.md):

- vm.html swappiness: "values beyond 100 can be considered" for zram/zswap; 2x faster swap IO gives 133 (x + 2x = 200).
- vm.html watermark_scale_factor: names `allocstall` and `kswapd_low_wmark_hit_quickly` as the signals. The doc gives no numeric threshold, so the 1 event/s screening threshold in the rule is ours, not the kernel's.
- vm.html zone_reclaim_mode: disabled by default; leave off for file servers / cache-heavy workloads; enable only for per-node partitioned workloads.
- vm.html min_free_kbytes: below 1024KB the system is "subtly broken, and prone to deadlock".
- vm.html overcommit_memory: mode 2 limit is swap + overcommit_ratio% of RAM; admin_reserve_kbytes should be raised under mode 2.
- transhuge.html: `compact_stall`, `thp_fault_fallback` definitions; defrag=always makes the requesting app stall and directly reclaim/compact.
- psi.html: example trigger "some 150000 1000000" (150 ms of stall in a 1 s window); that 15 % figure is reused as a screening threshold.

Skipped (UNVERIFIED or no engine fact):

- vm.max_map_count: only the default (65530) and the Fedora 1048576 bump are verified; the workload that needs it (Elasticsearch, 262144) is UNVERIFIED here and belongs to the apps pack.
- kernel.numa_balancing generic rule: facts carry no per-process CPU pinning, so "NUMA > 1 AND pinned" cannot be detected; the AI packs own the GPU case.
- The suffixed vmstat names `allocstall_normal` / `allocstall_movable` come from /proc/vmstat naming; the doc names only `allocstall`.
