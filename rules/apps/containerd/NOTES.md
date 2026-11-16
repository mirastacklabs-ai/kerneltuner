# Notes (fetched 2026-11-16, all verified by reading the text)

Rules: `app.containerd.overlay_module` (report).

## Verified
- containerd.service (shipped unit): `ExecStartPre=-/sbin/modprobe overlay`; `LimitNPROC=infinity`, `LimitCORE=infinity` with the comment "Having non-zero Limit*s causes performance problems due to accounting overhead in the kernel. We recommend using cgroups to do container-local accounting."; `TasksMax=infinity`; `OOMScoreAdjust=-999`; `Delegate=yes`.
- docs/ops.md: same `modprobe overlay` in the example unit; `oom_score = -999` so "containers are killed before the containerd daemon gets into an out of memory condition".
- docs/cri/config.md: default snapshotter is `overlayfs`.

## Not documented / not ruled (gaps)
- No sysctl, THP, inotify or LimitNOFILE guidance (the shipped unit sets no LimitNOFILE).
- LimitNPROC/LimitCORE=infinity and OOMScoreAdjust=-999 are documented, but facts carry neither process NPROC/CORE limits nor oom_score_adj, so they cannot be checked; no rule.
- ip_forward and bridge settings are Kubernetes prerequisites (see the kubelet pack), not containerd documentation.
