#!/bin/sh
. "$(dirname "$0")/lib.sh"
N=agent-alert
out=$(disposable "$PYIMG" sh /e2e/overflow.sh agent 2>&1)
echo "$out" | sed 's/^/  | /' >"$WORK/03.log"
if echo "$out" | grep -q RESULT-FAIL || ! echo "$out" | grep -q STDERR-FIRING || ! echo "$out" | grep -q STATE-FIRING || ! echo "$out" | grep -q STATUS-SHOWS; then
  fail $N "$(echo "$out" | grep -E 'RESULT-FAIL|SETUP-FAIL' | tr '\n' ';') (log: $WORK/03.log)"
else pass $N; fi
