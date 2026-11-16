#!/bin/sh
. "$(dirname "$0")/lib.sh"
N=evidence-listen-overflow
out=$(disposable "$PYIMG" sh /e2e/overflow.sh scan 2>&1)
echo "$out" | sed 's/^/  | /' >"$WORK/02.log"
echo "$out" | grep -q '^RESULT-OK' && pass $N || fail $N "$(echo "$out" | grep -E 'RESULT-FAIL|SETUP-FAIL|rule status|overflows' | tr '\n' ';')"
