# kerneltuner research: memory, CPU, storage, CPU-only inference rule packs

Fetched 2026-11-16. Tags: `verified-by-fetch` = page fetched and text supports the claim; `UNVERIFIED` = not confirmed from a fetched primary source.
Doc base: https://docs.kernel.org/ (current docs = latest kernel).
Raw fetches: scratchpad dl/ and fetch/cm/ (untrusted data, text-stripped).

Container note (all host sysctl/sysfs items): settable-in-container is UNVERIFIED as a general rule (Docker run page fetched; its --sysctl text did not state namespacing). Working assumption: vm.*, /sys/kernel/mm, /sys/devices/system/cpu, /sys/block are host-global, need host write access; advisor should emit host-side commands. Verified-by-fetch: `docker run --ulimit memlock=...`, `--cpuset-cpus`, `--cpuset-mems` exist; Docker says setting ulimits needs privileges not in default container so use --ulimit. https://docs.docker.com/reference/cli/docker/container/run/

## MEMORY

### vm.swappiness
- Default 60, range 0-200 (verified-by-fetch; mm/vmscan.c `vm_swappiness = 60` also seen). >100 for zram/zswap/fast swap; 133 example when swap IO 2x faster than fs; 0 = no swap until free+file pages below high watermark.
- History: range raised to 200 and reclaim balancing made refault/IO-cost based in 5.8 (verified-by-fetch kernelnewbies 5.8).
- Helps when swap/refault thrash. Evidence: /proc/pressure/memory some/full avg10 (verified-by-fetch psi doc); /proc/vmstat pswpin/pswpout, workingset_refault (semantics UNVERIFIED).
- Risks: low + no headroom => OOM instead of swap; high on slow swap => latency.
- Container: host sysctl (UNVERIFIED); per-cgroup swap.max is the container lever (UNVERIFIED here).
- Sources: https://docs.kernel.org/admin-guide/sysctl/vm.html ; https://kernelnewbies.org/Linux_5.8

### vm.dirty_ratio / dirty_background_ratio vs *_bytes
- Defaults 20 / 10 (verified-by-fetch from mm/page-writeback.c source; sysctl doc does not state values). Percent of available memory (free+reclaimable), not total RAM (verified-by-fetch).
- *_bytes counterpart; only one active, writing one makes other read 0; dirty_bytes min 2 pages (verified-by-fetch).
- Helps: huge-RAM hosts with write bursts (long flush stalls). Evidence: PSI io; meminfo Dirty/Writeback (UNVERIFIED in fetched docs). Irrelevant for read-mostly model loading.
- Risks: too low throttles writers; too high = long stalls, bigger crash-loss window.
- Source: https://docs.kernel.org/admin-guide/sysctl/vm.html

### vm.overcommit_memory / overcommit_ratio
- overcommit_memory default 0 heuristic; 1 always; 2 never (verified-by-fetch). Mode 2 limit = swap + overcommit_ratio% of RAM; user_reserve_kbytes applies. overcommit_ratio default 50: UNVERIFIED (not in fetched text).
- Helps when mode 0 refuses huge allocations (ENOMEM). Evidence: meminfo CommitLimit vs Committed_AS (UNVERIFIED semantics in fetched docs).
- Risks: mode 1 => OOM killer instead of ENOMEM; mode 2 breaks mmap/fork-heavy apps. Do not recommend 2 for inference hosts.
- Source: https://docs.kernel.org/admin-guide/sysctl/vm.html

### vm.max_map_count
- Kernel default 65530 (verified-by-fetch: sysctl doc; include/linux/mm.h DEFAULT_MAX_MAP_COUNT = USHRT_MAX - 5). Doc: most apps <1000 maps; malloc debuggers 1-2 per allocation.
- Distro bump to 1048576: Fedora 39 via /usr/lib/sysctl.d, rationale Wine/Steam games (DayZ, Hogwarts Legacy, CS2) (verified-by-fetch). Arch (filesystem 2024.04.07-1), SteamOS, NixOS, Debian linux-sysctl-defaults MR: seen only in search-result snippets, UNVERIFIED (not fetched). Ubuntu: nothing found.
- Elasticsearch needs >=262144: UNVERIFIED (Elastic page not fetched).
- mmap'd model weights: one file normally = few VMAs, so llama.cpp rarely needs a bump; many-shard models/allocators can. Evidence: `wc -l /proc/PID/maps` vs limit; mmap ENOMEM with free RAM. (reasoning, UNVERIFIED)
- Risks: slight kernel memory per VMA; low.
- Sources: https://docs.kernel.org/admin-guide/sysctl/vm.html ; https://fedoraproject.org/wiki/Changes/IncreaseVmMaxMapCount

### THP enabled / defrag / khugepaged
- enabled: always|madvise|never; per-size hugepages-<size>kB/enabled adds inherit; PMD size default inherit, other sizes never (verified-by-fetch). Boot: transparent_hugepage=, thp_anon= (verified-by-fetch). Compile-time default of top-level enabled varies by distro: UNVERIFIED which. 6.8 added CONFIG_TRANSPARENT_HUGEPAGE_NEVER (verified-by-fetch kernelnewbies 6.8).
- defrag: always (stall, direct reclaim+compact), defer (wake kswapd/kcompactd; khugepaged installs later), defer+madvise (direct only for MADV_HUGEPAGE regions, others defer), madvise (default), never. MADV_COLLAPSE works even under never (verified-by-fetch).
- khugepaged: auto-starts when any THP size enabled; knobs pages_to_scan, scan_sleep_millisecs, alloc_sleep_millisecs, max_ptes_none/swap/shared, defrag; stats pages_collapsed, full_scans (verified-by-fetch). /proc/vmstat: thp_fault_alloc, thp_fault_fallback, thp_collapse_alloc, thp_collapse_alloc_failed (verified-by-fetch).
- History: MADV_COLLAPSE in 6.1; multi-size THP anon in 6.8 (verified-by-fetch changelogs). shrink_underused knob in current doc, intro version UNVERIFIED.
- Helps: anon heaps TLB-bound. Signals: high thp_fault_fallback, low thp_fault_alloc. Risks: defrag=always => alloc stalls/compaction CPU; memory bloat.
- Container: sysfs host-global; per-process prctl(PR_SET_THP_DISABLE) works in container (verified-by-fetch).
- Sources: https://docs.kernel.org/admin-guide/mm/transhuge.html ; https://kernelnewbies.org/Linux_6.8 ; https://kernelnewbies.org/Linux_6.1

### Static hugetlb (2M vs 1G)
- Pool: /proc/sys/vm/nr_hugepages, per-size /sys/kernel/mm/hugepages/hugepages-<size>kB/nr_hugepages, nr_overcommit_hugepages, nr_hugepages_mempolicy; meminfo HugePages_Total/Free/Rsvd/Surp (verified-by-fetch).
- Boot: hugepagesz=2M hugepages=512 default_hugepagesz=..; per-node hugepages=0:1,1:2 (verified-by-fetch). Doc notes runtime allocation needs contiguous memory, best at boot; 1G (gigantic) reliably at boot. Exact 1G runtime/CMA conditions UNVERIFIED.
- Use via MAP_HUGETLB, hugetlbfs, SysV shm (hugetlb_shm_group) (verified-by-fetch). No fetched llama.cpp/PyTorch/OpenVINO doc uses hugetlb: UNVERIFIED.
- Risks: reserved memory unavailable to others; NUMA-uneven. Evidence of misuse: HugePages_Free == Total.
- Source: https://docs.kernel.org/admin-guide/mm/hugetlbpage.html

### vm.zone_reclaim_mode
- Default 0 (verified-by-fetch). Bits 1 on, 2 write dirty, 4 swap. Enable only if workload partitioned per NUMA node and remote access hurts; leave off for cache-heavy hosts.
- Evidence: numastat numa_miss/numa_foreign/other_node (definitions verified-by-fetch).
- Risk: latency, hurts page cache. Not for inference.
- Sources: https://docs.kernel.org/admin-guide/sysctl/vm.html ; https://man7.org/linux/man-pages/man8/numastat.8.html

### kernel.numa_balancing
- 0 disabled, 1 normal, 2 memory tiering; doc: disable if workload already NUMA-bound; boot numa_balancing=enable|disable (verified-by-fetch). Default depends on CONFIG: UNVERIFIED.
- Evidence: numa_hint_faults, numa_pages_migrated in /proc/vmstat (names UNVERIFIED in fetched docs).
- Risk of disabling: loses auto-locality for unpinned threads.
- Source: https://docs.kernel.org/admin-guide/sysctl/kernel.html

### vm.min_free_kbytes
- Computed from lowmem at boot/hotplug; kernel may raise for THP; once written, no auto-adjust; <1024KB breaks system; too high OOMs (verified-by-fetch). No numeric default in doc.
- Prefer watermark_scale_factor first. Risk: pins value, lowers usable RAM.
- Source: https://docs.kernel.org/admin-guide/sysctl/vm.html

### vm.watermark_scale_factor / watermark_boost_factor
- scale default 10 (0.1%), max 3000 (verified-by-fetch; mm/page_alloc.c `= 10`). Doc signals: high allocstall or kswapd_low_wmark_hit_quickly in /proc/vmstat => raise.
- boost default 15000, 0 disables (verified-by-fetch).
- Risk: more free memory held, kswapd CPU, smaller cache.
- Source: https://docs.kernel.org/admin-guide/sysctl/vm.html

### MGLRU (lru_gen)
- Merged 6.1 (verified-by-fetch). Needs CONFIG_LRU_GEN; default per CONFIG_LRU_GEN_ENABLED; /sys/kernel/mm/lru_gen/enabled bitmask 0x0001 main, 0x0002, 0x0004 (y => 0x0007); min_ttl_ms thrash prevention (verified-by-fetch). Check file existence for availability.
- Sources: https://docs.kernel.org/admin-guide/mm/multigen_lru.html ; https://kernelnewbies.org/Linux_6.1

### PSI
- /proc/pressure/{cpu,memory,io}; some/full avg10/avg60/avg300/total; CPU full undefined system-wide; triggers by writing "some 150000 1000000" (verified-by-fetch). Per-cgroup PSI + IRQ PSI: 6.1 changelog summary (verified-by-fetch; earlier cgroup2 PSI history UNVERIFIED). Absent if CONFIG_PSI=n; psi=0 boot UNVERIFIED.
- Source: https://docs.kernel.org/accounting/psi.html

## CPU

### cpufreq governors
- performance = highest freq within scaling_max_freq; powersave = lowest; schedutil uses PELT, f = 1.25*f0*util/max, RT/DL go to max; successor to ondemand/conservative (verified-by-fetch). Boot cpufreq.default_governor=. With intel_pstate active only performance/powersave exist.
- Evidence: scaling_cur_freq vs cpuinfo_max_freq under load. Risks: power/thermal; small gain on HWP/EPP systems.
- Source: https://docs.kernel.org/admin-guide/pm/cpufreq.html

### intel_pstate EPP
- Active default for HWP CPUs, passive otherwise; intel_pstate=no_hwp|passive|active|disable. HWP+performance forces EPP 0 and rejects sysfs EPP writes; HWP+powersave honors energy_performance_preference (string or 0-255); "default" = firmware; set same hint on all CPUs (verified-by-fetch). Also no_turbo.
- Source: https://docs.kernel.org/admin-guide/pm/intel_pstate.html

### amd-pstate EPP
- Modes active (amd_pstate_epp; EPP 0x0 perf .. 0xff efficiency), passive, guided; amd_pstate=disable|passive|active|guided; sysfs status; preferred core; "dynamic" EPP (verified-by-fetch, dynamic intro version UNVERIFIED). EPP driver merged 6.3 (verified-by-fetch kernelnewbies 6.3). Default mode per kernel/CPU gen: UNVERIFIED.
- Sources: https://docs.kernel.org/admin-guide/pm/amd-pstate.html ; https://kernelnewbies.org/Linux_6.3

### C-states / cpuidle
- Governors menu (default tickless), TEO, ladder, haltpoll (doc says four); /sys/devices/system/cpu/cpuidle/current_governor; per-state cpuN/cpuidle/stateM/{disable(RW),latency,residency,usage,time}; governor honors PM QoS (verified-by-fetch).
- Boot: intel_idle.max_cstate=N (kernel-parameters: 0 falls back to acpi_idle; intel_idle doc: 0 makes init fail; wording conflicts, both verified-by-fetch), processor.max_cstate=N, cpuidle.off=1.
- Runtime alternative: /dev/cpu_dma_latency PM QoS, process-scoped, no reboot (verified-by-fetch).
- Helps wake jitter; risks power/thermals/turbo headroom; little gain for batch inference (reasoning, UNVERIFIED).
- Sources: https://docs.kernel.org/admin-guide/pm/cpuidle.html ; https://docs.kernel.org/admin-guide/pm/intel_idle.html ; https://docs.kernel.org/power/pm_qos_interface.html

### SMT control
- /sys/devices/system/cpu/smt/{active,control}; values on, off, <N>, forceoff, notsupported, notimplemented (verified-by-fetch). Boot nosmt / nosmt=force (verified-by-fetch l1tf doc).
- Off may help compute-bound GEMM; halves logical CPUs; also a mitigation lever (report only).
- Sources: https://docs.kernel.org/admin-guide/abi-testing.html ; https://docs.kernel.org/admin-guide/hw-vuln/l1tf.html

### isolcpus / nohz_full / rcu_nocbs vs cpuset
- isolcpus flags nohz (= nohz_full; 1Hz residual tick needs workqueue cpumask affined), domain (default; irreversible at runtime, "use cpusets for dynamic"), managed_irq (verified-by-fetch). nohz_full=: no default; rcu_nocbs= offloads RCU callbacks, nohz_full CPUs offloaded too (verified-by-fetch no_hz).
- cgroup cpuset isolated partitions: UNVERIFIED (cgroup-v2 doc downloaded, section not extracted). Docker --cpuset-cpus/--cpuset-mems verified-by-fetch.
- Needs reboot; reduces capacity; rarely worthwhile for throughput inference.
- Sources: https://docs.kernel.org/admin-guide/kernel-parameters.html ; https://docs.kernel.org/timers/no_hz.html

### irqbalance
- Kernel: /proc/irq/N/smp_affinity(_list); managed IRQs not changeable via /proc/irq (verified-by-fetch). irqbalance daemon docs not fetched: UNVERIFIED.
- Source: https://docs.kernel.org/core-api/irq/irq-affinity.html

### sched autogroup
- /proc/sys/kernel/sched_autogroup_enabled, needs CONFIG_SCHED_AUTOGROUP, groups per session (verified-by-fetch sched(7)). Not in sysctl/kernel doc. Low relevance.
- Source: https://man7.org/linux/man-pages/man7/sched.7.html

### CPU vulnerability mitigations (REPORT ONLY)
- Status: /sys/devices/system/cpu/vulnerabilities/* (hw-vuln index fetched; path from ABI). mitigations=off: "improves system performance, but may expose users to several CPU vulnerabilities"; needs CONFIG_CPU_MITIGATIONS=y (verified-by-fetch). Never recommend silently; explicit opt-in only.
- Sources: https://docs.kernel.org/admin-guide/hw-vuln/index.html ; https://docs.kernel.org/admin-guide/kernel-parameters.html

## STORAGE
- https://docs.kernel.org/block/queue-sysfs.html returned 404: not a source. Use ABI text (verified-by-fetch): https://docs.kernel.org/admin-guide/abi-stable.html
- scheduler: /sys/block/<disk>/queue/scheduler shows/sets; module autoload (verified-by-fetch). NVMe default none, SATA mq-deadline: UNVERIFIED.
- read_ahead_kb: max readahead per device; may exceed for MADV_HUGEPAGE (verified-by-fetch). Default 128: UNVERIFIED. Raise (1-4MB) for sequential mmap model load; evidence PSI io, pgmajfault rate. Risk: waste on random IO; per-device, non-persistent (udev). Alternative: pre-read file / MADV_WILLNEED.
- nr_requests: requests per blk_mq_tags instance; actual depends on hw queues, elevator, shared tags (verified-by-fetch). Low value for model loading (UNVERIFIED).
- Container: /sys read-only by default (UNVERIFIED).

## CPU-ONLY INFERENCE

### ISA and backends
- cpuinfo flags avx2, avx512f/bw/vl/dq, avx512_vnni, avx512_bf16, avx_vnni, amx_tile, amx_bf16, amx_int8: names UNVERIFIED (no cpufeatures page fetched).
- oneDNN: JIT picks best ISA; ONEDNN_MAX_CPU_ISA caps: AVX2, AVX2_VNNI, AVX512_CORE, AVX512_CORE_VNNI, AVX512_CORE_BF16, AVX512_CORE_AMX, AVX512_CORE_AMX_FP16, AVX10_x (verified-by-fetch). https://uxlfoundation.github.io/oneDNN/dev_guide_cpu_dispatcher_control.html
- OpenVINO: bf16/f16 uses AMX vs AVX512/AVX2; bf16 simulation on AVX-512 without native avx512_bf16 is dev-only; AMX needs e.g. Sapphire Rapids (verified-by-fetch). https://docs.openvino.ai/2025/openvino-workflow/running-inference/inference-devices-and-modes/cpu-device.html
- llama.cpp: BLAS options; oneAPI build gives avx_vnni on CPUs without avx512_vnni; AMD: ZenDNN backend recommended (verified-by-fetch). Runtime kernel-to-ISA mapping UNVERIFIED. https://raw.githubusercontent.com/ggml-org/llama.cpp/master/docs/build.md
- PyTorch tuning guide: no AMX mention in fetched text: UNVERIFIED.
- Ollama FAQ: no CPU tuning guidance found: UNVERIFIED.

### AMX kernel requirement
- Linux 5.16 added AMX; needs arch_prctl ARCH_GET_XCOMP_SUPP / ARCH_GET_XCOMP_PERM / ARCH_REQ_XCOMP_PERM; TILE_DATA = XSTATE component 18 (verified-by-fetch). Libraries request it themselves. SIGILL-on-no-permission wording and container seccomp effect: UNVERIFIED.
- Sources: https://docs.kernel.org/arch/x86/xstate.html ; https://kernelnewbies.org/Linux_5.16

### Threads
- llama.cpp: -t default -1 (auto), -tb, -C/--cpu-mask, --cpu-range, --cpu-strict, --prio, --poll (default 50) (verified-by-fetch). "Use physical cores" guidance: UNVERIFIED (not in fetched official docs).
- OpenVINO: ov::hint::enable_hyper_threading, num_streams, inference_num_threads, enable_cpu_pinning (verified-by-fetch; defaults UNVERIFIED).
- PyTorch: OMP_NUM_THREADS; GOMP_CPU_AFFINITY="N-M"; OMP_PROC_BIND=CLOSE; KMP_AFFINITY=granularity=fine,compact,1,0; KMP_BLOCKTIME 0/1; jemalloc/tcmalloc LD_PRELOAD (verified-by-fetch). https://docs.pytorch.org/tutorials/recipes/recipes/tuning_guide.html
- GNU OpenMP vars OMP_NUM_THREADS, OMP_PROC_BIND, OMP_PLACES, GOMP_CPU_AFFINITY (verified-by-fetch). https://gcc.gnu.org/onlinedocs/libgomp/Environment-Variables.html
- Intel KMP_AFFINITY page fetched but value semantics not extracted: UNVERIFIED. https://www.intel.com/content/www/us/en/docs/dpcpp-cpp-compiler/developer-guide-reference/2025-0/thread-affinity-interface.html

### NUMA binding
- PyTorch: `numactl --cpunodebind=N --membind=N` avoids cross-socket access (verified-by-fetch).
- llama.cpp --numa distribute | isolate | numactl; drop page cache before first use, issue 1437; env LLAMA_ARG_NUMA (verified-by-fetch). https://raw.githubusercontent.com/ggml-org/llama.cpp/master/tools/server/README.md
- OpenVINO auto NUMA handling; streams pinned to physical-core groups per node; LATENCY hint reduces cross-node transfer (verified-by-fetch).
- --interleave vs --cpunodebind specifics: UNVERIFIED (numa_memory_policy doc downloaded, not extracted). Disable numa_balancing when pinned (verified-by-fetch).

### mlock / mmap
- Current llama.cpp README: -lm/--load-mode auto|none|mmap|mlock|mmap+mlock|dio, env LLAMA_ARG_LOAD_MODE; mlock "keeps model in RAM rather than swapping or compressing" (verified-by-fetch). Older --mlock/--no-mmap not in this README: advisor must be version-aware, old names UNVERIFIED.
- RLIMIT_MEMLOCK >= model size needed: UNVERIFIED (doc silent). Container: --ulimit memlock=-1:-1 (verified-by-fetch); CAP_IPC_LOCK UNVERIFIED.

### Hugepages for weights
- No fetched official doc recommends hugetlb for weights: UNVERIFIED. tmpfs huge= policies always/within_size/advise/never, default never (verified-by-fetch transhuge).

## Gaps
Intel/AMD tuning guides not fetched; queue-sysfs 404; NVMe default scheduler; read_ahead_kb default; overcommit_ratio=50; Elasticsearch 262144; Arch/SteamOS/Debian max_map_count specifics; per-distro THP default; amd-pstate default mode; cpuinfo flag names; irqbalance docs; container writability; Ollama CPU docs; AMX SIGILL; cpuset partitions.
