#!/bin/sh
# kerneltuner e2e runner. Exits non-zero on any FAIL. Usage: run.sh [scenario-number...]
D=$(cd "$(dirname "$0")" && pwd)
sh "$D/00-build.sh" || exit 1
rc=0
for s in "$D"/0[1-7]-*.sh; do
  [ $# -gt 0 ] && { n=$(basename "$s" | cut -c2); case " $* " in *" $n "*) ;; *) continue;; esac; }
  out=$(sh "$s" 2>&1); echo "$out"
  echo "$out" | grep -q '^FAIL' && rc=1
done
docker volume rm "${KT_E2E_BINVOL:-kt-e2e-bin}" >/dev/null 2>&1 || true
[ $rc -eq 0 ] && echo "E2E: all passed" || echo "E2E: FAILURES"
exit $rc
