# ZooKeeper notes (fetched 2026-11-16, zookeeperAdmin.html, verified by reading the page text)

## Verified
- "In order for ZooKeeper to function with any sort of timeliness, it simply cannot be allowed to swap. Therefore, make certain that the maximum heap size given to ZooKeeper is not bigger than the amount of real memory available to ZooKeeper." -> `app.zookeeper.swapping` (report on observed swap I/O) and `app.zookeeper.heap_exceeds_ram` (report when -Xmx > MemTotal).
- "DON'T SWAP. Be conservative ... if you have 4G of RAM, do not set the Java max heap size to 6G or even 4G. ... a 3G heap for a 4G machine."
- `clientPortListenBacklog`: "By default, this value is unset (-1) which, on Linux, uses a backlog of 50." Connections beyond it "receive a network timeout (30s) which may cause ZooKeeper session expiry issues." The kernel cap by somaxconn comes from docs.kernel.org (ip-sysctl). -> `app.zookeeper.listen_backlog` (runtime, gated on ListenOverflows).
- `maxClientCnxns` (default 60) mentions file descriptor exhaustion as a DoS class; no limit value.

## The docs do NOT say
- Any vm.swappiness value or advice to disable swap; only "don't swap" via heap sizing.
- Any ulimit / LimitNOFILE value, sysctl, THP, scheduler or filesystem/mount option. The guide's disk advice (dedicated transaction log device, no sharing) is not a kernel setting and is not covered.
- A recommended clientPortListenBacklog value.

## Engine gaps seen
- The heap check parses `-Xmx` from the process cmdline; heap set only via environment (no environ fact) or containers with a cgroup memory limit are not seen.
