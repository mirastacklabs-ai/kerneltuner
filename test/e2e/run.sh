#!/bin/sh
# kerneltuner e2e runner. Exits non-zero on any FAIL. Usage: run.sh [scenario-number...]
D=$(cd "$(dirname "$0")" && pwd)
sh "$D/00-build.sh" || exit 1
rc=0
for s in "$D"/0[1-6]-*.sh; do
  [ $# -gt 0 ] && { n=$(basename "$s" | cut -c2); case " $* " in *" $n "*) ;; *) continue;; esac; }
  out=$(sh "$s" 2>&1); echo "$out"
  echo "$out" | grep -q '^FAIL' && rc=1
done
[ $rc -eq 0 ] && echo "E2E: all passed" || echo "E2E: FAILURES"
exit $rc
