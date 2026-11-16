# Security policy

## Reporting a vulnerability

Report vulnerabilities through GitHub private vulnerability reporting: open the repository's **Security** tab and choose **Report a vulnerability**. Do not open a public issue for a security problem.

Include the version (`kerneltuner version`), what you did, what you expected and what happened. If the problem involves a rule that recommends something harmful, include the rule id and the source it cites. We will acknowledge the report, work on a fix in the private advisory, and credit you in the release notes unless you prefer otherwise.

## Supported versions

| Version | Supported |
|---|---|
| 1.x | Yes |
| 0.x and earlier | No |

Fixes go into the latest 1.x release.

## Scope and security model

### `apply` writes system settings

`apply` and `rollback` change the host: they write sysctl values, sysfs values and systemd drop-ins, and they need root. Without `--root` they refuse to run unless the user is uid 0. `apply` takes a snapshot first, reads every value back, and rolls back automatically on a mismatch; `--yes` is required unless you use `--dry-run`. It never restarts a service itself. Only `sysctl`, `sysfs` and `limit` rules can be applied. A recommendation is advice from the project's docs, not a guarantee that it is safe for your workload: review the output of `scan` or `apply --dry-run` first.

Vulnerabilities in scope include: `apply` writing outside the paths it documents, a failed rollback that leaves a changed system, and a rule expression that can run code or read files outside the facts layer.

### The agent is observe-only

`kerneltuner agent` samples counters, evaluates rules and logs state changes to journald (or JSON lines on stderr). Its only file write is its state file, `/run/kerneltuner/state.json`. The shipped systemd unit ([deploy/kerneltuner.service](deploy/kerneltuner.service)) enforces this:

- `ProtectSystem=strict` with `ReadWritePaths=/run/kerneltuner`, `ProtectHome=yes`, `PrivateTmp=yes`
- `ProtectKernelTunables=yes`, `ProtectKernelModules=yes`, `ProtectControlGroups=yes`
- `NoNewPrivileges=yes`, `RestrictSUIDSGID=yes`, `LockPersonality=yes`, `MemoryDenyWriteExecute=yes`

It runs with three capabilities and no others (`CapabilityBoundingSet` and `AmbientCapabilities` are the same list):

| Capability | Why |
|---|---|
| `CAP_DAC_READ_SEARCH` | Read files owned by other users under `/proc/<pid>` (socket-to-process mapping, per-process limits) |
| `CAP_SYS_PTRACE` | The kernel requires it to read `/proc/<pid>/fd` links and `/proc/<pid>/limits` of other users' processes |
| `CAP_NET_ADMIN` | Read NIC ring and channel sizes through the ethtool ioctl |

All three are used for reading. Without them the agent works with less complete process and NIC facts; missing data is never an error. The agent does not apply changes, and it has no network listener.

### `capture` redaction

`kerneltuner capture` reads an explicit allowlist of `/proc`, `/sys` and `/etc` paths and writes an archive or a fixture file. Guarantees:

- It never reads `/proc/<pid>/environ`, with one exception: the `container=` entry of PID 1. It never reads file descriptor targets, serial numbers, or MAC and IP addresses.
- It keeps the process name and `argv[0..1]` from each command line. Arguments that look like secrets (names containing `pass`, `secret`, `token`, `key`, `auth` or `cred`) and email addresses are replaced with `[redacted]`.
- The host name is replaced with `redacted-host` wherever it appears. The `kernel.hostname`, `domainname` and `tcp_fastopen_key` sysctls are skipped.
- Remote mounts are recorded as `[remote]`, and mount credentials and overlay paths are dropped. Kernel config is limited to a fixed list of options.

Redaction is pattern-based and not a proof of anonymity. Review a capture before you share it. A path, argument or value that leaks something it should not is a bug we want to hear about.

### `sources --check` and the network

`kerneltuner sources --check` is the only command that uses the network, and only when you run it. It sends one HTTP GET per registered documentation URL, with a `kerneltuner-sources-check/<version>` User-Agent and no cookies, and compares the page text with the quotes in the rules. It sends no host data: no facts, no host name, no rule results. The weekly `sources-check` workflow runs it in CI, not on your machines. `scan`, `explain`, `generate`, `agent`, `capture`, `apply` and `rollback` make no network requests.

## Supply chain

Releases are built by GoReleaser in GitHub Actions from a tag, and ship with a `checksums.txt` (sha256) and an SPDX SBOM for each archive. `scripts/install.sh` verifies the sha256 before installing. GitHub Actions in the workflows are pinned by commit SHA, and Dependabot tracks Go modules.
