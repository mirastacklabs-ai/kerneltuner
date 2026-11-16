# Notes (fetched 2026-11-16)

- HAProxy 3.0 manual: global maxconn auto-adjusts the process soft fd limit; with no maxconn it is derived from the larger of the hard and soft limits (reduced by fd-hard-limit and memory); `ulimit-n` is auto-computed and not recommended, use `fd-hard-limit`; default fd-hard-limit 1048576.
- No sysctl guidance was found in the configuration manual, so no sysctl rule exists.
