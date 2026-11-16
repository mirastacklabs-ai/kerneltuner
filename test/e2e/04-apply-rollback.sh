#!/bin/sh
# CLI --root maps proc/sys/etc/run/var all under DIR (facts.RootAt); it cannot
# split live /proc from temp /etc, so apply runs with --root / inside a
# disposable container whose /etc and /var are its own.
. "$(dirname "$0")/lib.sh"
N=apply-rollback
out=$(disposable "$ALPINE" sh /e2e/applyrb.sh 2>&1)
echo "$out" | sed 's/^/  | /' >"$WORK/04.log"
echo "$out" | grep -q '^RESULT-OK' && pass $N || fail $N "$(echo "$out" | grep -E 'RESULT-FAIL|apply rc|^[<>+-]' | tr '\n' ';')"
