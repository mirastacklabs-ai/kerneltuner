#!/bin/sh
# Read-only scan of the Colima VM kernel: / mounted ro, --pid=host, no host net.
. "$(dirname "$0")/lib.sh"
N=real-kernel-scan
OUT=$WORK/scan.json; : > "$OUT"
docker run --rm -v /:/host:ro --pid=host -v "$BIN_VOL":/kt:ro "$ALPINE" \
  sh -c '/kt/kerneltuner scan --root /host --all >/dev/null && /kt/kerneltuner scan --root /host --all --format json' > "$OUT" 2>"$WORK/scan.err"
rc=$?
[ $rc -eq 0 ] || { fail $N "exit code $rc: $(head -c 200 "$WORK/scan.err")"; exit 1; }
res=$(docker run --rm -i "$ALPINE" sh -c 'apk add -q --no-progress jq >/dev/null 2>&1 && jq -r "
  [(.kernel|capture(\"^(?<m>[0-9]+)\").m|tonumber),
   ([.recommendations[]|select(.status==\"ok\")]|length),
   ([.recommendations[]|select(.status==\"error\")]|length)]|@tsv"' < "$OUT") \
  || { fail $N "JSON did not parse (jq)"; exit 1; }
set -- $res
[ "$1" -gt 0 ] || { fail $N "kernel version $1 not > 0"; exit 1; }
[ "$2" -ge 1 ] || { fail $N "no rule with status ok"; exit 1; }
[ "$3" -eq 0 ] || { fail $N "$3 rules with status error"; exit 1; }
pass $N
