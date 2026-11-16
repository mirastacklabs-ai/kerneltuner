# Notes (fetched 2026-11-16)

Verified:
- resource_limits FAQ ("How does Envoy prevent file descriptor exhaustion?"): "On Unix-based systems, it is recommended to keep the sum of all connection limits less than half of the system's file descriptor limit to account for upstream connections, files, and other usage of file descriptors."
- Overload manager: "The connection limit is recommended to be less than half of the system's file descriptor limit"; with no limit set Envoy warns at startup.
- listener.proto: "tcp_backlog_size ... If no value is provided net.core.somaxconn will be used on Linux and 128 otherwise."
- kernel ip-sysctl: "somaxconn ... Defaults to 4096. (Was 128 before linux-5.4)".

Rule notes:
- app.envoy.fd_limit gates on soft limit <= 1024. That threshold is our inference (the half-of-limit rule gives under 512 connections at 1024), not an Envoy statement. No LimitNOFILE value is proposed because the docs give none.

Docs do NOT say (no rule written):
- Any recommended LimitNOFILE / ulimit number, or any sysctl value (the `faq/configuration/limits` URL is 404; the edge-proxy best-practices page has no host-level tuning, only Envoy config).
- net.ipv4.tcp_fastopen: listener.proto says the sysctl "must include flag 0x2" to enable TCP_FASTOPEN, but whether a listener uses tcp_fast_open_queue_length needs Envoy config parsing, so no rule.
- THP, swap, I/O scheduler, readahead, mount options, memlock, kernel cmdline: not covered.
