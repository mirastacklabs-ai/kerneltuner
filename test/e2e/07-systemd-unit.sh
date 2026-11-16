#!/bin/sh
# Boots the hardened unit under real systemd in a disposable container and
# checks the agent survives its own seccomp filter (SystemCallFilter).
# Safety: debian:12 with systemd as PID 1, --privileged with a private cgroup
# namespace, tmpfs /run, own netns, NO host mounts except the binary volume
# (read-only). Units that write kernel tunables (sysctl, binfmt, modules) are
# masked before boot so the VM's real sysctls are never touched.
. "$(dirname "$0")/lib.sh"
N=systemd-unit
C=kt-e2e-systemd-$$
IMG=${SYSTEMD_IMAGE:-debian:12}
cleanup() { docker rm -f "$C" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
log="$WORK/07.log"
: >"$log"
bad() { fail $N "$1 (log: $log)"; exit 1; }
dx() { docker exec "$C" "$@"; }

docker run -d --name "$C" --privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock \
  --network bridge -v "$BIN_VOL":/kt:ro "$IMG" sh -c '
    set -e
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq >/dev/null
    apt-get install -y -qq --no-install-recommends systemd >/dev/null
    mkdir -p /etc/systemd/system
    for u in systemd-sysctl systemd-binfmt systemd-modules-load; do ln -sf /dev/null /etc/systemd/system/$u.service; done
    echo BOOTING
    exec /lib/systemd/systemd' >>"$log" 2>&1 || bad "docker run failed"

# Wait (bounded) for systemd to come up; give up early if the container dies.
i=0
while [ $i -lt 90 ]; do
  [ "$(docker inspect -f '{{.State.Running}}' "$C" 2>/dev/null)" = true ] || { docker logs "$C" >>"$log" 2>&1; bad "container exited before systemd booted: $(docker logs "$C" 2>&1 | tail -3 | tr '\n' ' ')"; }
  st=$(dx systemctl is-system-running 2>/dev/null || true)
  case $st in running|degraded) break;; esac
  i=$((i+1)); sleep 2
done
case $st in running|degraded) ;; *) docker logs "$C" >>"$log" 2>&1; bad "systemd did not boot (is-system-running=$st)";; esac
echo "systemd up: $st" >>"$log"

dx install -m 0755 /kt/kerneltuner /usr/bin/kerneltuner || bad "install binary"
docker cp "$REPO/deploy/kerneltuner.service" "$C:/etc/systemd/system/kerneltuner.service" || bad "copy unit"
dx systemctl daemon-reload
dx systemctl start kerneltuner >>"$log" 2>&1 || { dx journalctl -u kerneltuner --no-pager >>"$log" 2>&1; bad "systemctl start failed"; }
sleep 20

dx systemctl status kerneltuner --no-pager >>"$log" 2>&1 || true
dx journalctl -u kerneltuner --no-pager >>"$log" 2>&1 || true
dx journalctl --no-pager -k >>"$log" 2>&1 || true

[ "$(dx systemctl is-active kerneltuner 2>/dev/null)" = active ] || bad "unit not active: $(dx systemctl is-active kerneltuner 2>&1)"
j=$(dx journalctl -u kerneltuner --no-pager 2>&1)
echo "$j" | grep -qiE 'SIGSYS|Bad system call|core dump|dumped core|status=31|code=killed' && bad "journal shows a seccomp kill or crash"
[ "$(dx systemctl show kerneltuner -p NRestarts --value)" = 0 ] || bad "unit restarted (NRestarts=$(dx systemctl show kerneltuner -p NRestarts --value))"
dx test -s /run/kerneltuner/state.json || bad "/run/kerneltuner/state.json missing or empty"
dx cat /run/kerneltuner/state.json | docker run --rm -i "$PYIMG" python3 -c 'import json,sys; json.load(sys.stdin)' >>"$log" 2>&1 \
  || bad "state.json does not parse"
dx kerneltuner status >>"$log" 2>&1 || bad "kerneltuner status failed"

dx systemctl stop kerneltuner >>"$log" 2>&1 || bad "systemctl stop failed"
pass $N
