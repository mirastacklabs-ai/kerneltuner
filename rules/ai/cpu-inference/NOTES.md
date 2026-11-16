# Wave 3 notes: cpu-inference and CPU/serving app packs

Facts newly verified by fetch on 2026-11-16 (raw pages under the session scratchpad `fetch-wave3/`):

- AMX landed upstream in Linux 5.16: https://kernelnewbies.org/Linux_5.16 ("Add support for AMX instructions"). The kernel XSTATE doc (https://docs.kernel.org/arch/x86/xstate.html) covers TILE_DATA / component 18 but does not name a version.
- /proc/cpuinfo flag names `amx_tile`, `amx_bf16`, `amx_int8`, `avx512_bf16`, `avx512_vnni`, `avx_vnni` come from `arch/x86/include/asm/cpufeatures.h` (quoted flag strings).
- oneDNN: ONEDNN_MAX_CPU_ISA table includes `AVX512_CORE_AMX` (alias of AVX10_1_512_AMX); `DEFAULT` = no restriction.
- OpenVINO CPU device: bf16/f16 inference precision enables AMX on 4th-gen and newer Xeon.
- PyTorch tuning guide: one instance per socket, `numactl --cpunodebind=N --membind=N`, OMP_NUM_THREADS, OMP_PROC_BIND=CLOSE, GOMP_CPU_AFFINITY, KMP_AFFINITY=granularity=fine,compact,1,0, KMP_BLOCKTIME 0 or 1. No AMX text.
- llama.cpp server README at tags b4000 (examples/server) and b6000 (tools/server) documents `--mlock`, `--no-mmap`, `--numa`; master documents `-lm/--load-mode` (auto|none|mmap|mlock|mmap+mlock|dio) and `--numa`, and no longer lists `--mlock`/`--no-mmap`. The tag URLs are registered in `rules/apps/llama.cpp/sources.yaml`.
- mlock(2): RLIMIT_MEMLOCK soft limit bounds unprivileged processes, CAP_IPC_LOCK is exempt (Linux 2.6.9+); ENOMEM when the limit is exceeded.
- vLLM docker page: "either the ipc=host flag or --shm-size flag" (official wording, replaces the mirror text).

Skipped, still UNVERIFIED:

- docker's default /dev/shm size (64 MB) was not found on the fetched docker run page; not claimed.
- TGI docs give no rationale for `--shm-size 1g`; NVIDIA-side SGLang container flags; CAP_IPC_LOCK requirement for llama.cpp mlock beyond the mlock(2) man page.
- llama.cpp thread-count ("physical cores") guidance, OpenVINO thread/stream defaults, jemalloc/tcmalloc LD_PRELOAD (guide-documented but undetectable, left out to avoid noise), `clearcpuid=` cmdline handling for AMX.
- Environment variables (OMP_*, ONEDNN_*, LLAMA_ARG_*) and docker IPC settings are not visible to kerneltuner (no /proc/PID/environ, no container config), so those rules are advice (`runtime`) or conservative detectors that say so.
- PyTorch NUMA binding lives in `ai.cpu.numa-binding` (not duplicated in the pytorch pack); llama.cpp NUMA in `app.llama_cpp.numa`; SGLang host cmdline (`pci=realloc=off iommu=pt`) and numa_balancing=0 belong to ai.amd.
