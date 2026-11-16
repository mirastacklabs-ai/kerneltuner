# Storage pack notes

Newly verified by fetch on 2026-11-16 (beyond docs/research/cpu-mem-research.md):

- block/elevator.c (master): `elevator_set_default()` uses mq-deadline for single-queue (or shared-tag) devices and "none" when there are multiple hardware queues or mq-deadline is unavailable. This verifies the previously UNVERIFIED "NVMe default is none". The code on master only; older-kernel version history is not claimed.
- tuned `throughput-performance` profile: `[disk] readahead=>4096` with the comment that the default unit is KiB. This is the source for the 4096 KiB read_ahead_kb value. tuned applies it broadly, not specifically to model loading; the gating on AI hosts is our inference.
- abi-stable: read_ahead_kb is the maximum read-ahead in KB for filesystems on the device and may be exceeded for MADV_HUGEPAGE regions; writing to `scheduler` switches the scheduler and may load its module.
- https://docs.kernel.org/block/queue-sysfs.html is a 404; not used. https://docs.kernel.org/block/switching-sched.html fetched but is mostly navigation text; not used.

Skipped:

- nr_requests: only the ABI definition is verified and no source gives a recommended value; research marks the benefit for model loading UNVERIFIED.
- Default read_ahead_kb of 128 is not claimed anywhere; the rule only compares against the 4096 target.
