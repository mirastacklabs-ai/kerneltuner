# Runs inside the disposable alpine container (own netns, own /etc and /var).
set -u
echo 16 > /proc/sys/net/core/somaxconn || { echo "RESULT-FAIL setup"; exit 0; }
nc -l -p 9999 >/dev/null 2>&1 &   # a listener so workload.Listeners > 0
sleep 1
snap() { sysctl -a 2>/dev/null | grep '^net\.ipv4\.' | sort; }
snap > /tmp/before.txt
SC=$(cat /proc/sys/net/core/somaxconn)
ls /etc/sysctl.d > /tmp/etc.before 2>/dev/null
/kt/kerneltuner apply --rule net.core.somaxconn --yes --root / ; rc=$?
NEW=$(cat /proc/sys/net/core/somaxconn)
echo "apply rc=$rc somaxconn $SC -> $NEW"
ls /etc/sysctl.d > /tmp/etc.mid 2>/dev/null
[ "$rc" = 0 ] || { echo "RESULT-FAIL apply exit $rc"; exit 0; }
[ "$NEW" != "$SC" ] || { echo "RESULT-FAIL value unchanged"; exit 0; }
F=/etc/sysctl.d/90-kerneltuner.conf
[ -e $F ] || { echo "RESULT-FAIL sysctl.d file not created"; exit 0; }
echo "created: $F"
/kt/kerneltuner rollback --yes --root / ; rc=$?
[ "$rc" = 0 ] || { echo "RESULT-FAIL rollback exit $rc"; exit 0; }
BACK=$(cat /proc/sys/net/core/somaxconn)
[ "$BACK" = "$SC" ] || { echo "RESULT-FAIL somaxconn $BACK != $SC after rollback"; exit 0; }
snap > /tmp/after.txt
diff /tmp/before.txt /tmp/after.txt > /tmp/d.txt || { echo "RESULT-FAIL net.ipv4 diff:"; cat /tmp/d.txt; exit 0; }
[ ! -e $F ] || { echo "RESULT-FAIL sysctl.d file still present"; exit 0; }
echo "RESULT-OK"
