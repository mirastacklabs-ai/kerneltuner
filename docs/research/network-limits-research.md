# kerneltuner rule research: network, limits, anti-patterns, app packs, ecosystem

Researched 2026-11-16. Tags: `verified-by-fetch` = page fetched with curl and text supports the claim. `UNVERIFIED` = not confirmed from a primary source fetched in this session (do not ship as fact).
Doc baseline = kernel master docs (docs.kernel.org / raw torvalds/linux master). Source files read: raw.githubusercontent.com/torvalds/linux/master/{net/core/sysctl_net_core.c, net/ipv4/sysctl_net_ipv4.c, net/ipv4/tcp_ipv4.c, net/ipv4/tcp.c, net/core/hotdata.c, net/core/net-procfs.c, fs/file_table.c, net/netfilter/nf_conntrack_core.c}; commit pages from git.kernel.org cgit.

## Namespace rule of thumb (verified by reading the sysctl tables)
- `net_core_table` (net/core) is registered in init_net only: netdev_max_backlog, netdev_budget, netdev_budget_usecs, default_qdisc, busy_poll. NOT settable in a container. verified-by-fetch
- `netns_core_table`: somaxconn, optmem_max, rps_default_mask, txrehash, tstamp_allow_data, plus rmem_max/wmem_max/rmem_default/wmem_default. In non-init netns the rmem/wmem group is READ-ONLY (mode &= ~0222 in netns_core_table_dup). Commit 19249c0724f2 "net: make net.core.{r,w}mem_{default,max} namespaced" (2024-05-31). verified-by-fetch
- `ipv4_table` (global only): tcp_max_orphans, tcp_mem. `ipv4_net_table` (per-netns, writable): tcp_rmem, tcp_wmem, tcp_congestion_control, tcp_tw_reuse, tcp_fin_timeout, tcp_slow_start_after_idle, tcp_mtu_probing, tcp_notsent_lowat, ip_local_port_range, tcp_max_syn_backlog, tcp_syncookies, tcp_timestamps, tcp_sack, tcp_max_tw_buckets, tcp_fastopen. verified-by-fetch
- Kernel-version caveat: namespacing commits (dates only; kernel versions UNVERIFIED): tcp_rmem/wmem 356d1833b638 (2017-11-07); tcp_congestion_control 6670e1524477 (2017-11-14); tcp_tw_reuse 56ab6b93007e (2016-12-25). tcp_sack/tcp_timestamps/tcp_window_scaling namespaced in 4.13 (kernelnewbies). verified-by-fetch
- Rules must key "settable in container" on kernel version; runtimes may also mount /proc/sys read-only.

## net.core.somaxconn
- Default 4096 since 5.4 (was 128). Docs: "Defaults to 4096. (Was 128 before linux-5.4)"; include/linux/socket.h `#define SOMAXCONN 4096`. verified-by-fetch
- Commit 19f92a030ca6 "net: increase SOMAXCONN to 4096" (2019-10-30; Google used 1024 then 4096; listen(fd,-1) apps; SYN-flood resilience). kernelnewbies 5.4 lists it plus tcp_max_syn_backlog bump. verified-by-fetch
- Caps listen() backlog: effective accept queue = min(app backlog, somaxconn).
- Helps when accept queue overflows. Counters: TcpExt ListenOverflows (accept queue full, SYN dropped) and ListenDrops (superset; also alloc failure). verified-by-fetch (docs.kernel.org snmp_counter)
- Risk: useless unless app backlog also raised (nginx 511, Redis 511); hides slow accept(), adds latency in overload.
- Netns: yes, writable. verified-by-fetch
- Sources: https://docs.kernel.org/networking/ip-sysctl.html ; https://kernelnewbies.org/Linux_5.4 ; https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=19f92a030ca6

## net.ipv4.tcp_max_syn_backlog
- Docs: per-listener limit of SYN_RECV request socks; "minimal value is 128 for low memory machines, and it will increase in proportion to the memory"; ~304 B per request sock. verified-by-fetch
- Code (tcp_ipv4.c): `max(128U, ehash_entries / 128)`. kernelnewbies 5.4 says default raised to 4096 then; the later scaled-formula commit/version is UNVERIFIED.
- Helps when SYN backlog fills under legit load (ListenDrops rising, "possible SYN flooding" in dmesg). Syncookie counter names beyond SyncookiesSent: UNVERIFIED.
- Pair with somaxconn (docs say so). Netns: yes. verified-by-fetch
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## net.core.netdev_max_backlog
- Docs: max packets queued on INPUT side when NIC delivers faster than kernel processes (per-CPU). Code default: `.max_backlog = 1000` (hotdata.c). NOTE bpftune docs say 1024; source says 1000. verified-by-fetch
- Counter: /proc/net/softnet_stat col 2 `dropped` (per CPU, hex). net-procfs.c fields: processed, dropped, time_squeeze, ..., received_rps, flow_limit_count, backlog len, cpu index. verified-by-fetch
- Mechanism limitation (applies to RPS/loopback/veth/tunnel path, NAPI-driver drops show up as NIC rx_missed): UNVERIFIED from docs.
- Risk: latency/memory; does not fix CPU-bound softirq. Netns: NO. verified-by-fetch
- Source: https://docs.kernel.org/admin-guide/sysctl/net.html ; hotdata.c ; net-procfs.c

## net.core.netdev_budget / netdev_budget_usecs
- Docs: budget = max packets per NAPI poll cycle across interfaces (round-robin), also bounded by usecs. verified-by-fetch
- Defaults (hotdata.c): netdev_budget=300; netdev_budget_usecs = 2*USEC_PER_SEC/HZ (2000 at HZ=1000; 8000 at HZ=250). verified-by-fetch
- netdev_budget_usecs added 4.12 ("Replace 2 jiffies with a sysctl to enable softirq tuning"). verified-by-fetch (kernelnewbies 4.12)
- Helps when softnet_stat col 3 (time_squeeze) increments.
- Risk: longer softirq runs hurt scheduler latency (bpftune backs off when wait/run ratio rises). verified-by-fetch
- Netns: NO. verified-by-fetch
- Sources: https://docs.kernel.org/admin-guide/sysctl/net.html ; https://kernelnewbies.org/Linux_4.12

## net.core.rmem_max / wmem_max (+ rmem_default/wmem_default)
- Cap SO_RCVBUF/SO_SNDBUF setsockopt values; do NOT cap TCP autotuning (tcp_rmem[2] does; setsockopt SO_RCVBUF disables autotuning for that socket). verified-by-fetch
- Defaults: historically 212992. Commit a6d4f25888b8 "net: set net.core.rmem_max and net.core.wmem_max to 4 MB" (Eric Dumazet, 2025-08-19): max=4194304, defaults stay 212992. In kernelnewbies Linux 6.18. Docs master "Default: 4194304". verified-by-fetch
- Rule: on >=6.18 "raise rmem_max to 16M" only matters for >4 MB buffers (UDP/QUIC); older kernels 212992 is small for UDP/QUIC.
- Counters for UDP drops (RcvbufErrors): UNVERIFIED.
- Risk: memory scales with socket count; counts toward tcp_mem/udp_mem.
- Netns: read-only inside netns. verified-by-fetch
- Sources: https://docs.kernel.org/admin-guide/sysctl/net.html ; https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=a6d4f25888b8 ; https://kernelnewbies.org/Linux_6.18

## net.ipv4.tcp_rmem / tcp_wmem (+ BDP sizing)
- Docs: rmem min 4096, default 131072, max "between 131072 and 32MB depending on RAM"; wmem min 4096, default 16384, max "between 64K and 4MB depending on RAM". verified-by-fetch
- tcp_init(): limit = nr_free_buffer_pages() << (PAGE_SHIFT-7); wmem max = max(64K, min(4MB,limit)); rmem max = max(128K, min(32MB,limit)). verified-by-fetch
- Netns: yes (tcp_mem global only). verified-by-fetch
- Helps when BDP (bandwidth x RTT) > tcp_rmem[2]/wmem[2] on high-BDP paths (`ss -tmi`). BDP sizing rule itself: no primary doc fetched, UNVERIFIED.
- Risk: memory = sockets x max; tcp_mem pressure; send-side bufferbloat. tuned network-throughput uses 16 MiB max; bpftune auto-tunes.
- Commit 83a945a529d6 "tcp: do not let tcp_rmem be set below 4096" (2026-09). tcp_adv_win_scale obsolete since 6.6 (docs). verified-by-fetch
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## tcp_congestion_control=bbr + default_qdisc=fq
- BBR added in 4.9 (released 2016-12-11). verified-by-fetch (kernelnewbies 4.9)
- Internal TCP pacing added in 4.13 (kernelnewbies 4.13 "Internal implementation for pacing"); tcp_bbr.c sets `sk_pacing_status = SK_PACING_NEEDED`. So pre-4.13 BBR needed sch_fq for pacing; >=4.13 works without fq (internal pacing). verified-by-fetch for the 4.13 entry + code; the commit text stating "pre-4.13 requires fq" UNVERIFIED.
- default_qdisc docs: default pfifo_fast; best with configless qdiscs (sfq, codel, fq_codel); mq root uses it for leaves; lo/veth ignore it (noqueue). verified-by-fetch
- Netns: tcp_congestion_control per-netns; allowed/available readable in non-init (commits 97684f0970f6, 9cb8e048e5d9). default_qdisc global only. verified-by-fetch
- Needs tcp_bbr module/CONFIG_TCP_CONG_BBR; check tcp_available_congestion_control.
- bpftune doc: CUBIC is Linux default; BBR helps where losses occur. verified-by-fetch. BBRv1 fairness drawbacks: UNVERIFIED.
- Sources: https://kernelnewbies.org/Linux_4.9 ; https://kernelnewbies.org/Linux_4.13 ; https://raw.githubusercontent.com/torvalds/linux/master/net/ipv4/tcp_bbr.c

## net.ipv4.ip_local_port_range
- Default 32768 60999; prefer different parity; must be >= ip_unprivileged_port_start; ip_local_reserved_ports excludes ports from auto-assign. verified-by-fetch
- Helps on EADDRNOTAVAIL from ephemeral exhaustion (many conns to one dst ip:port). Proof counter UNVERIFIED (use `ss -s`).
- Risk: overlap with listener ports (use reserved_ports); gain <2x; more source IPs/tw_reuse often better.
- Netns: yes.
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## tcp_tw_reuse
- Values 0/1/2 (2=loopback only). Docs "Default: 2"; code `sysctl_tcp_tw_reuse = 2`. verified-by-fetch
- Introduced as int with value 2 in commit 79e9fed46038 (2018-06-04); docs of v4.18 and v4.19 both say Default: 2, so default 2 since 4.18 (exact tag of commit UNVERIFIED; v4.18 doc check verified).
- tcp_tw_reuse_delay (ms, default 1000) added by commits ca6a6f93867a / 19ce8cd30465 (2024-12-09); kernel version UNVERIFIED. verified-by-fetch for existence/default.
- Value 1 is for clients with many outbound connections, needs timestamps (PAWS); docs: "should not be changed without advice of technical experts". Doesn't help inbound-server TIME-WAIT.
- Netns: yes.
- Source: https://docs.kernel.org/networking/ip-sysctl.html ; commit 79e9fed46038

## tcp_tw_recycle (REMOVED)
- Removed in 4.12: commit 4396e46187ca "tcp: remove tcp_tw_recycle" (2017-03-15): broken behind NAT (per-destination timestamp not monotonic across hosts); after timestamp offset randomisation (8a5bd45f6616) broken for all; PAWSPassive counter removed. kernelnewbies 4.12: "TCP: remove tcp_tw_recycle sysctl", "remove per-destination timestamp cache". verified-by-fetch
- Rule: flag if tcp_tw_recycle exists and is 1 (kernel <4.12): P0.
- Sources: https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/commit/?id=4396e46187ca ; https://kernelnewbies.org/Linux_4.12

## tcp_fin_timeout
- Default 60 s; applies to ORPHANED connections in FIN_WAIT_2, not TIME_WAIT (common folklore error). verified-by-fetch (docs wording)
- Helps with many FIN_WAIT_2 orphans (`ss -s`, see tcp_max_orphans). Netns: yes.
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## tcp_slow_start_after_idle
- Default 1 (RFC2861 cwnd decay after idle RTO); 0 keeps cwnd; tuned network-throughput sets 0. verified-by-fetch
- Helps persistent bursty connections; risk: line-rate burst into congested path. Netns: yes.
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## tcp_mtu_probing
- 0 disabled, 1 enabled on ICMP blackhole detection, 2 always (tcp_base_mss). tcp_mtu_probe_floor added 5.4. verified-by-fetch. Default value stated nowhere in fetched text: 0 UNVERIFIED.
- Helps PMTU blackholes (tunnels/VPN, filtered ICMP): hangs with large packets. Netns: yes.
- Sources: https://docs.kernel.org/networking/ip-sysctl.html ; https://kernelnewbies.org/Linux_5.4

## tcp_notsent_lowat
- Default UINT_MAX (off); limits unsent bytes in write queue; poll/epoll POLLOUT only below it; tuned network-throughput sets 131072. verified-by-fetch
- Helps latency (HTTP/2 prioritisation) and sender memory; risk: more wakeups, lower throughput if small. Netns: yes.
- Source: https://docs.kernel.org/networking/ip-sysctl.html

## nf_conntrack_max and hashsize
- nf_conntrack_buckets default: RAM/16384/sizeof(hlist_head), min 1024; 64-bit >1 GiB RAM: 65536; >4 GiB: 262144. Docs: "never fewer than 1024 and never more than 262144". verified-by-fetch
- nf_conntrack_max = buckets by default (docs); code max_factor=1 when auto-sized (8 when hashsize user-given). Entries inserted twice => avg chain 2. verified-by-fetch. Older kernels' factor 8 default: version UNVERIFIED.
- Helps on dmesg "nf_conntrack: table full, dropping packet"; count vs max in /proc/sys/net/netfilter. conntrack -S stat names UNVERIFIED.
- Risk: memory per entry (size UNVERIFIED); raise buckets with max. Buckets writable only in init netns (docs). nf_conntrack_max writability in container: UNVERIFIED.
- Source: https://docs.kernel.org/networking/nf_conntrack-sysctl.html ; nf_conntrack_core.c

## RPS / XPS
- scaling docs: per-rx-queue /sys/class/net/<dev>/queues/rx-N/rps_cpus; /proc/sys/net/core/rps_sock_flow_entries (RFS); RPS useful when fewer HW queues than CPUs; set to CPUs in same memory domain as the interrupting CPU. rps_default_mask netns. verified-by-fetch
- XPS details UNVERIFIED (not read). Counter: softnet_stat col 10 received_rps. verified-by-fetch
- Risk: IPIs/cache effects; no gain if RSS already spreads. Netns: sysfs per device; rps_default_mask per netns.
- Source: https://docs.kernel.org/networking/scaling.html

## NIC ring buffers / channels
- ethtool(8): `-g` show ring, `-G` set ring (rx/tx/rx-mini/rx-jumbo), `-L` set channels (rx/tx/other/combined). verified-by-fetch
- Counters: rx_missed_errors is summed into /proc/net/dev errs (net-procfs.c); driver `ethtool -S` names (rx_missed/rx_fifo/rx_no_buffer) UNVERIFIED. time_squeeze points to budget/IRQ spread, not ring size. verified-by-fetch
- Risk: bigger rings = memory + latency; channels > cores waste IRQs; change may bounce the link. Needs CAP_NET_ADMIN in device's netns.
- Sources: https://man7.org/linux/man-pages/man8/ethtool.8.html ; https://docs.kernel.org/networking/scaling.html

## fs.file-max
- Default (files_maxfiles_init): memreserve=(totalram-free)*3/2 (capped); n=((nr_pages-memreserve)*(PAGE_SIZE/1024))/10; max_files=max(n, NR_FILE) i.e. ~10% of memory at ~1 KB/file. NR_FILE value UNVERIFIED. verified-by-fetch for formula
- Docs: "VFS: file-max limit <n> reached" in kernel log. Global only. systemd>=240 bumps to max.
- Source: https://docs.kernel.org/admin-guide/sysctl/fs.html ; fs/file_table.c

## fs.nr_open
- Default 1048576; per-process ceiling for RLIMIT_NOFILE. verified-by-fetch
- Raising LimitNOFILE above it fails; raise nr_open first. Global only.
- Source: https://docs.kernel.org/admin-guide/sysctl/fs.html

## systemd LimitNOFILE
- NEWS "CHANGES WITH 240": kernel default 1024 soft / 4096 hard; systemd raises hard to 512K; soft stays 1024 because select() can't handle fds >=1024 (apps opt in by raising own soft limit); 1G hard is problematic (Java allocates arrays per fd); fs.nr_open and fs.file-max auto-bumped to max since memcg accounts fds (build opts -Dbump-proc-sys-fs-file-max=false / -Dbump-proc-sys-fs-nr-open=false revert). verified-by-fetch
- Lennart blog: "don't use select() + bump the RLIMIT_NOFILE soft limit to hard limit in your modern programs". verified-by-fetch
- systemd.exec(5) page fetched; LimitNOFILE= per-unit override; its exact default text UNVERIFIED. https://systemd.io/ULIMIT/ returned 404, not used.
- Sources: https://raw.githubusercontent.com/systemd/systemd/main/NEWS ; https://0pointer.net/blog/file-descriptor-limits.html ; https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html

## Anti-patterns
- tcp_timestamps=0: docs default 1 (random offsets), 2 = no offsets; netns since 4.13. Disabling breaks PAWS and tcp_tw_reuse safety (tw_reuse_delay docs reference PAWS). verified-by-fetch
- tcp_sack=0: default 1; harm to loss recovery is general knowledge, UNVERIFIED in a primary source.
- Oversized tcp_mem/rmem/wmem: tcp_mem computed at boot from RAM; pressure mode; tuned comment: "do not account for hugepage allocation. Ensure you do not oversubscribe system memory". verified-by-fetch
- tcp_tw_recycle: removed 4.12 (above).
- Disabling syncookies: default 1; "fallback facility", not for legit load; 2 = always for testing; they degrade TCP extensions; docs say tune tcp_max_syn_backlog / tcp_synack_retries / tcp_abort_on_overflow instead. verified-by-fetch
- Lowering tcp_max_tw_buckets: docs say raise, never lower artificially. tcp_abort_on_overflow=1: only if listener can't be tuned. verified-by-fetch

## App pack: nginx
- worker_rlimit_nofile changes RLIMIT_NOFILE for workers; core docs tie max open files to it (worker_connections section). verified-by-fetch
- listen backlog=number: default -1 on FreeBSD/DragonFly/macOS, 511 elsewhere. verified-by-fetch
- Rule: effective = min(backlog, somaxconn); with somaxconn=128 (<5.4) nginx's 511 is truncated; set backlog= explicitly.
- Sources: https://nginx.org/en/docs/http/ngx_http_core_module.html#listen ; https://nginx.org/en/docs/ngx_core_module.html#worker_rlimit_nofile

## App pack: HAProxy
- maxconn: per-process connections; "process soft file descriptor limit is automatically adjusted according to provided maxconn"; ulimit-n auto-computed, "recommended not to use" (prefer fd-hard-limit); select poller limited to ~1024 fds. verified-by-fetch
- HAProxy-official sysctl recommendations: none found in the configuration manual; management/starter guides not fetched => UNVERIFIED.
- Source: https://docs.haproxy.org/3.0/configuration.html (maxconn, ulimit-n, fd-hard-limit)

## App pack: Redis
- redis.conf `tcp-backlog 511`: kernel silently truncates to somaxconn; raise both somaxconn and tcp_max_syn_backlog. verified-by-fetch (network domain)
- MEMORY-DOMAIN (not network): admin docs say vm.overcommit_memory=1 (fork for background save) and disable THP (`echo never > /sys/kernel/mm/transparent_hugepage/enabled`); redis.conf `disable-thp yes` is process-level, ineffective if kernel is madvise/never. verified-by-fetch
- Sources: https://redis.io/docs/latest/operate/oss_and_stack/management/admin/ ; https://raw.githubusercontent.com/redis/redis/unstable/redis.conf

## Ecosystem: tuned (master profiles)
- throughput-performance: cpu governor/EPB/min_perf_pct performance; vm dirty_bytes=40%, dirty_background_bytes=10%; readahead >4096; vm.swappiness=10; net.core.somaxconn=>2048 (comment cites 5.4 default 4096); thunderx: THP never, numa_balancing=0. verified-by-fetch
- network-latency (include latency-performance): THP never; net.core.busy_read=50; busy_poll=50; tcp_fastopen=3; numa_balancing=0; hung_task_timeout_secs 600; nmi_watchdog 0; vm.stat_interval 10; timer_migration 0; cmdline skew_tick=1 tsc=reliable rcupdate.rcu_normal_after_boot=1 rcutree.nohz_full_patience_delay=1000. latency-performance: force_latency cstate, governor performance, dirty_bytes=10%, dirty_background_bytes=3%, swappiness 10. verified-by-fetch
- network-throughput (include throughput-performance): tcp_notsent_lowat=131072; tcp_slow_start_after_idle=0; tcp_rmem="4096 131072 16777216"; tcp_wmem="4096 16384 16777216". verified-by-fetch
- Overlap: check `tuned-adm active`; defer on somaxconn floor, busy_poll/read, tcp_fastopen, notsent_lowat, slow_start_after_idle, tcp_rmem/wmem, swappiness, dirty_*, THP. Profiles vary by tuned version/distro.
- Source: https://github.com/redhat-performance/tuned/tree/master/profiles

## Ecosystem: bpftune
- Tuners (README): TCP connection (congestion control; BBR on loss >~1.5%), IP fragmentation, neighbour table, sysctl (disables a tuner if admin sets a colliding sysctl), TCP buffer (tcp_rmem/tcp_wmem/tcp_mem), net buffer, netns, UDP buffer. verified-by-fetch
- net-buffer tunables: netdev_max_backlog (doc says default 1024), flow_limit_cpu_bitmap, netdev_budget, netdev_budget_usecs (from time_squeezed; backs off on wait/run ratio). UDP buffer: rmem_default, rmem_max (README list). verified-by-fetch
- Overlap: detect bpftune and avoid: tcp_rmem/wmem/mem, netdev_max_backlog, netdev_budget(_usecs), flow_limit_cpu_bitmap, rmem/wmem defaults/max, congestion control, ipfrag, neigh gc_thresh. Manual sysctl writes silently disable the colliding tuner until bpftune restart. verified-by-fetch
- Source: https://github.com/oracle/bpftune (README.md, docs/bpftune-*.rst)

## Open UNVERIFIED list
- Kernel versions: tcp_tw_reuse_delay, scaled tcp_max_syn_backlog formula, netns-ification commits, rmem_max read-only-in-netns.
- BDP sizing primary doc; tcp_sack=0 harm; port-exhaustion counter; ethtool -S rx_missed names; conntrack -S names; XPS; HAProxy sysctl guidance; NR_FILE value; systemd.exec default text; tcp_mtu_probing default.
