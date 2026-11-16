# Engine issues reported by pack agents

Status after the engine fix pass. Behaviour is documented in `docs/DESIGN.md` ("Engine amendments") and `rules/README.md`.

- [nvidia] serviceRunning matched `Proc.Comm == name` only. **FIXED**: comm (with the 15-char truncation prefix), argv[0] basename and unit name.
- [nvidia] workload.App.Unit only from .service cgroup; no container flag. **FIXED**: `Proc.Container` / `App.Container` (docker, containerd, crio, podman) from the cgroup path, plus `appContainer(name)`. `App.Unit` is deliberately still service-only.
- [nvidia] document quoted string `recommend` values. **FIXED**: `rules/README.md`.
- [nvidia] accel.Classify drops 0x0680 bridges (NVSwitch undetectable). **FIXED**: `Accelerator.Kind` (`gpu`, `nvswitch`, `npu`), `accelCount` counts no NVSwitch, new `accelKindCount(vendor, kind)`.
- [limits] float formatting of large integers. **FIXED**: proposed values and integer comparisons are exact.
- [limits] App.NoFileSoft = min across PIDs misleads. **FIXED** (additive): `App.NoFileSoftMax` is the max across PIDs; `NoFileSoft` stays the min so existing rules are unchanged. Rules that care about workers should switch to the new field.
- [limits] pow2ceil requires float arg. **FIXED**: `pow2ceil`, `clamp`, `bytes` accept ints and floats; `max`/`min` already did.
- [cpu-inf] Facts.Procs has no environ. **WONTFIX**: by design (privacy); documented in `rules/README.md`.
- [cpu-inf] runtime rules always `recommend`. **FIXED**: optional `detect` on `runtime` and `service` (true = recommend, false = ok); no `detect` keeps today's behaviour.
- [cpu-inf] "cannot scan PCI devices" noise on fixtures. **FIXED**: only real read errors warn.
- [overlap] duplicate recommendations for one (kind, param, target). **FIXED**: `advisor.Reconcile` merges them in `generate` and `apply` (gte max, lte min, eq/string conflict warns and picks the highest score; every rule id in the comment); `scan` and JSON keep all of them; the text report adds a `conflicts:` line.
- [perf] golden tests 57-140s. **FIXED**: fixtures are materialized and collected once per run, in parallel; fixture validation is memoized (rules package about 35s to about 11s on a loaded host).
- [memory/cpu] no `foreach: cpu`. **FIXED**: `foreach: cpu` plus `foreach_summary: true` (Target `cpu0-63 (64)`, `Targets` list, apply writes each CPU).
- [memory/cpu] no facts for CPU vulnerabilities, /proc/swaps. **FIXED**: `CPU.Vulnerabilities`, `Facts.Swaps` (zram detection), also `CPU.IDs`, `fs.inotify.*` and `net.bridge.*` sysctls. Per-process CPU affinity: **WONTFIX** for now (needs `/proc/<pid>/status` parsing per process; no rule needs it yet).
- [memory/cpu] evidence never changes status. **FIXED**: documented, plus optional `require_evidence: true`.
- [memory/cpu] `scan --root` cannot inject fixture values/rates. **FIXED**: hidden `--fixture NAME` on `scan`, `generate`, `apply`.
- [amd] no Host.Distro. **FIXED**: `Host.Distro{ID, VersionID}`. The Gaudi rule can switch from the release-suffix approximation; that is a rule change, not done here.
- [amd] cmdline comma lists. **FIXED**: set comparison for comma values and `cmdlineHas(key, item)`.
- [amd] RoCE netdevs only under infiniband/.../device/net missing from NICs. **FIXED**.
- [network] rps_cpus readback format (zero-padded hex mask) unverified. **WONTFIX** here: needs a real kernel; apply's verify compares whitespace-normalised text and rolls back on mismatch, so a wrong guess fails safe. Revisit with a real host capture.
- [network] stock 10G+ hosts get 4 medium-risk recs from tuned values. **FIXED** in the engine: `require_evidence: true` is available; adding it to those rules is a pack change (not done here, rule YAML is out of scope for this pass).

## Reported by app-pack authors after the fix pass (engine fix pass 2)
Details are in `docs/DESIGN.md` ("Engine amendments (fix pass 2)") and `rules/README.md`.

- [envoy/httpd] No mount table / fstype facts (NFS sendfile advice impossible). **FIXED**: `Facts.Mounts`, `mountOf`, `fsType`, `mountHasOpt`, `blockOf`.
- [envoy/httpd] No app config parsing; rules gate on host symptoms only. **WONTFIX**: by design for v1 (reading arbitrary application configs is out of scope and a privacy risk); gate on host facts, mounts and process limits instead.
- [postgres] Need per-process oom_score_adj fact. **FIXED**: `Proc.OOMScoreAdj`, `App.OOMScoreAdj` (max across PIDs).
- [postgres] Document values()/all() expr builtins in rules/README.md. **FIXED**: "Expression builtins and new helpers"; every example is a test.
- [conflict] app.postgres.overcommit_memory=2 vs app.redis.overcommit_memory=1 must warn loudly. **FIXED in the engine**: `advisor.Conflicts` now also reports a recommendation that contradicts an already-satisfied `eq` rule; `advisor.ConflictBlock` formats the CONFLICT block. The text renderer still has to print it at the top of the report (citations/render owner).
- [cncf] sysctl()/sysctlNum() lack the Root.Proc/sys fallback. **FIXED**: `rules.SysctlValue`.
- [cncf] `validate --rules-dir DIR` reported 0 rules on a copy of rules/. **FIXED**: root cause was `Load` treating a directory without `kernel/`, `ai/`, `apps/` or `antipatterns/` as an empty rule set (a missing DIR, or `cp -r rules DEST`, which nests the packs under `DEST/rules`). `validate` now fails with a message naming the problem.
- [cncf] No facts: kernel config, /proc/mounts, process ionice, oom_score_adj, NPROC/CORE limits. **FIXED**: `Kernel.Config` + `kconfig()`, `Facts.Mounts`, `Proc.OOMScoreAdj`, `Proc.NProcSoft/NProcHard/CoreSoft`, `App.NProcSoft`. **WONTFIX**: process ionice (`Proc.IOPrio`): the kernel offers it only through the `ioprio_get(2)` syscall, never via `/proc`, and issuing it for other processes is outside the observe-only read model. `LimitCORE` as a `limit` param is not done (optional).
- [messaging] expr rejects uint64 literals > int64. **FIXED**: `unlimited` constant and `isUnlimited(x)`.
- [nosql] No nproc limit kind. **FIXED**: `LimitNPROC` in the `limit` kind (advisor, validate, apply drop-in). The limits.d fallback of `generate --format limits` still labels it `nofile`; the renderer needs an `nproc` item (render owner).
- [nosql] sources.yaml YAML errors should name the file. **FIXED**: loader errors are `<path>:<line>: <message>`.
- [cassandra] read_ahead per-device cannot tell which disk holds data. **FIXED**: `blockOf(path)` and `Mount.Disk` (dm/partition resolved to the parent disk), e.g. `filter: item.Name == blockOf("/var/lib/cassandra")` on `foreach: block`.
