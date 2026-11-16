# Notes (fetched 2026-11-16)

Verified:
- mpm_common ListenBackLog: "Default: ListenBackLog 511"; "Generally no tuning is needed or desired; however on some systems, it is desirable to increase this when under a TCP SYN flood attack"; "This will often be limited to a smaller number by the operating system."
- perf-tuning: "A webserver should never ever have to swap, as swapping increases the latency of each request ... You can, and should, control the MaxRequestWorkers setting so that your server does not spawn so many children that it starts swapping."
- kernel ip-sysctl: "somaxconn ... Defaults to 4096. (Was 128 before linux-5.4)".

Docs do NOT say (no rule written):
- Any sysctl, ulimit/LimitNOFILE, THP, I/O scheduler or readahead value. The perf-tuning page has no open-files advice (only the ServerLimit/MaxRequestWorkers sizing).
- EnableSendfile off / EnableMMAP off are advised for NFS-mounted content ("With an NFS-mounted filesystem, the kernel may be unable to reliably serve the network file through its own cache"). That is host-adjacent, but the facts layer has no mount table, so it cannot be detected; no rule.
- ListenCoresBucketsRatio needs SO_REUSEPORT (Linux 3.9+); httpd config is not parsed, so no rule.
