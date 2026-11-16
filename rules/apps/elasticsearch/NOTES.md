# Notes (fetched 2026-11-16, verified by reading the page text)

## Verified (elastic.co/guide/en/elasticsearch/reference/current/)
- vm-max-map-count.html: "If the default value is lower than 1048576, configure the vm.max_map_count parameter to 1048576." (older Elasticsearch docs said 262144; this page is the current one.)
- setup-configuration-memory.html: "Swapping is very bad for performance, for node stability, and should be avoided at all costs." Preferred: `swapoff -a`; else "vm.swappiness is set to 1"; else mlockall (`LimitMEMLOCK` infinity).
- file-descriptors.html: "increase the limit on the number of open files descriptors ... to 65,535 or higher"; RPM/Debian "already default" to 65535.
- system-config-tcpretries.html: Linux default of 15 retransmissions "take over 900 seconds"; `sysctl -w net.ipv4.tcp_retries2=5` (about 13 seconds); "Elasticsearch does not adjust this setting automatically".
- max-number-of-threads.html: "at least 4096" threads for the Elasticsearch user.

## Not said by the docs (no rule written)
- Thread limit (4096): the engine has no nproc limit kind and `Proc` carries no nproc limit. Recorded only.
- bootstrap.memory_lock / LimitMEMLOCK: only needed when mlockall is chosen, which kerneltuner cannot see, so no always-on memlock rule; it is mentioned in the swappiness rule's advice.
- No THP, I/O scheduler, read-ahead, file-system or NUMA guidance on the system-configuration pages.
