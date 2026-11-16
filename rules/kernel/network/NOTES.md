# Network pack notes

Facts verified 2026-11-16 beyond docs/research/network-limits-research.md:

- scaling.html mentions irqbalance once, only to say it "dynamically optimizes IRQ assignments" and "may override any manual settings". It does not recommend running it, so no irqbalance rule was written.
- tcp_bbr.c header: "BBR might be used with the fq qdisc with pacing enabled, otherwise TCP stack falls back to an internal pacing using one high resolution timer per TCP socket and may use more resources." Used for net.core.default_qdisc. The claim that pre-4.13 BBR requires fq is not in the fetched text and is not used.
- snmp_counter.html covers TcpExtListenOverflows, TcpExtListenDrops and TcpOutSegs. It does not cover UdpRcvbufErrors, UdpSndbufErrors or TcpRetransSegs.
- tuned master [sysctl] sections: throughput-performance (vm.swappiness=10, net.core.somaxconn=>2048), latency-performance (vm.swappiness=10), network-latency (include latency-performance; busy_read/busy_poll 50, tcp_fastopen 3, kernel.numa_balancing 0, hung_task_timeout_secs 600, nmi_watchdog 0, vm.stat_interval 10, timer_migration 0), network-throughput (include throughput-performance; tcp_notsent_lowat 131072, tcp_slow_start_after_idle 0, tcp_rmem "4096 131072 16777216", tcp_wmem "4096 16384 16777216").

Skipped (UNVERIFIED): rmem_max/wmem_max (no verified UDP drop counter to gate on), tcp_fin_timeout (no sourced value), tcp_sack=0 antipattern (harm unsourced), irqbalance, BBR+fq guidance for kernels 4.9 to 4.12, ipfrag/neighbour bpftune sysctl names.
