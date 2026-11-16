# Notes (fetched 2026-11-16)

- docs.kernel.org fs.html: file-max is the max number of file handles the kernel allocates; "VFS: file-max limit <number> reached" is logged; file-nr = allocated, unused (always 0 on 2.6+), max. nr_open default 1048576 and is the per-process ceiling.
- systemd NEWS v240: kernel default RLIMIT_NOFILE 1024 soft / 4096 hard; systemd raises hard to 512K, soft stays 1024 (select()); 1G hard limits break Java-like per-fd arrays.
- limit rules are `namespaced: true` because RLIMIT_NOFILE is per process (set by the container runtime), unlike the global fs.* sysctls.
