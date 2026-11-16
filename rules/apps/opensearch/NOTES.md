# Notes (fetched 2026-11-16, verified by reading the page text)

## Verified (opensearch.org/docs/latest/install-and-configure/install-opensearch/)
- index/ "Important settings": "make sure the Linux setting vm.max_map_count is set to at least 262144", also for the Docker image (set on the host); `vm.max_map_count=262144` in /etc/sysctl.conf.
- index/: `bootstrap.memory_lock=true` "Disables swapping (along with memlock). Swapping can dramatically decrease performance and stability, so you should ensure it is disabled on production clusters."
- index/: "nofile 65536 - Sets a limit of 65536 open files for the OpenSearch user."
- tar/ and docker/ pages: "Disable memory paging and swapping performance on the host to improve performance. sudo swapoff -a".
- The sample systemd unit on the tarball page sets `LimitNOFILE=65535` while the Important settings text says 65536; the rule uses the 65536 from the important-settings text.

## Not said by the docs (no rule written)
- The docs give no tcp_retries2, swappiness value, THP, scheduler or read-ahead advice, and no explanation of why 262144 or 65536 were chosen.
- memlock unlimited appears only in the sample docker-compose files together with bootstrap.memory_lock; since that setting is invisible to kerneltuner there is no memlock rule.
- The tarball unit's `LimitNPROC=4096` has no engine rule kind.
