#!/bin/sh
. "$(dirname "$0")/lib.sh"
N=package-install
cd "$REPO" || exit 1
had_dist=0; [ -d dist ] && had_dist=1
trap '[ $had_dist -eq 0 ] && rm -rf "$REPO/dist"' EXIT
make release-snapshot >"$WORK/release.log" 2>&1 || { fail $N "make release-snapshot failed (see $WORK/release.log): $(tail -3 "$WORK/release.log" | tr '\n' ' ')"; exit 1; }
case $(uname -m) in arm64|aarch64) A_DEB=arm64; A_RPM=arm64;; *) A_DEB=amd64; A_RPM=amd64;; esac
deb=$(ls dist/*_linux_$A_DEB.deb 2>/dev/null | head -1); rpm=$(ls dist/*_linux_$A_RPM.rpm 2>/dev/null | head -1)
[ -n "$deb" ] && [ -n "$rpm" ] || { fail $N "missing deb/rpm in dist: $(ls dist | tr '\n' ' ')"; exit 1; }
d=$(docker run --rm -v "$REPO/dist":/d:ro debian:12 sh -c "dpkg -i /d/$(basename $deb) >/dev/null 2>&1; kerneltuner version && ls -l /usr/lib/systemd/system/kerneltuner.service" 2>&1) \
  || { fail $N "deb: $d"; exit 1; }
r=$(docker run --rm -v "$REPO/dist":/d:ro rockylinux:9 sh -c "rpm -i --nodeps /d/$(basename $rpm) >/dev/null 2>&1; kerneltuner version && ls -l /usr/lib/systemd/system/kerneltuner.service" 2>&1) \
  || { fail $N "rpm: $r"; exit 1; }
rm -rf "$REPO/dist"; had_dist=1
pass $N
