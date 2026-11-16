# Deploying the kerneltuner agent

The agent is observe-only. It samples kernel counters, evaluates the rules and
logs `firing` / `resolved` transitions. Its only file write is
`/run/kerneltuner/state.json`.

## Install

```sh
sudo install -m 0755 bin/kerneltuner /usr/bin/kerneltuner
sudo install -m 0644 deploy/kerneltuner.service /etc/systemd/system/kerneltuner.service
sudo systemctl daemon-reload
sudo systemctl enable --now kerneltuner
```

## Use

```sh
kerneltuner status                 # firing rules with duration, last 10 events
kerneltuner status --json          # raw state file
journalctl -t kerneltuner -f       # follow alerts
journalctl -t kerneltuner -p warning   # firing alerts only (resolved is INFO)
journalctl -t kerneltuner KT_RULE=net.core.somaxconn -o verbose
```

Journal fields on every entry: `KT_RULE`, `KT_PARAM`, `KT_TARGET`,
`KT_CURRENT`, `KT_PROPOSED`, `KT_EVIDENCE`, `KT_STATE` (`firing` | `resolved`).
Without journald the agent prints the same data as JSON lines on stderr.

## Notes

- A rule fires after 2 consecutive ticks with evidence and resolves after 4
  clean ticks. The same rule re-alerts at most once per hour.
- The unit runs as a transient unprivileged user (`DynamicUser=yes`, state in
  `RuntimeDirectory=kerneltuner`) with only `CAP_DAC_READ_SEARCH` and
  `CAP_SYS_PTRACE`. The latter is what the kernel requires to read other users'
  `/proc/<pid>/fd` (open-fd counts, socket-to-port mapping); `/proc/<pid>/limits`
  is world-readable and needs neither. `CAP_NET_ADMIN` is not granted: the
  ethtool ring/channel queries are unprivileged. Address families are limited to
  `AF_UNIX AF_NETLINK AF_INET AF_INET6`, system calls to `@system-service` minus
  `@privileged @resources`, and devices, namespaces, kernel logs, the clock and
  the host name are off limits. Capabilities are explained in the unit file.
  Missing data is never an error; without them, process and NIC facts are
  simply less complete.
- Review the sandbox with `systemd-analyze security kerneltuner.service`
  (exposure 2.2, rated OK, on systemd 252).
- Override flags with `systemctl edit kerneltuner` (`ExecStart=` then
  `ExecStart=/usr/bin/kerneltuner agent --interval 30s`).
- Validate the unit without installing systemd on the host:

```sh
docker run --rm -v "$PWD/deploy:/d:ro" debian:bookworm-slim sh -c \
  'apt-get update -qq && apt-get install -y -qq systemd >/dev/null && \
   cp /d/kerneltuner.service /etc/systemd/system/ && touch /usr/bin/kerneltuner && chmod +x /usr/bin/kerneltuner && \
   systemd-analyze verify /etc/systemd/system/kerneltuner.service && \
   systemd-analyze security --offline=true kerneltuner.service --no-pager | tail -1'
```
