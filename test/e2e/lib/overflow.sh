# Runs inside the disposable container: $1 = scenario (scan|agent)
set -u
echo 16 > /proc/sys/net/core/somaxconn || { echo "SETUP-FAIL somaxconn"; exit 90; }
ovf() { python3 - <<'PY'
h=v=None
for l in open('/proc/net/netstat'):
    p=l.split()
    if p[0]=='TcpExt:':
        if h is None: h=p[1:]
        else: v=p[1:]
print(int(v[h.index('ListenOverflows')]))
PY
}
case "$1" in
scan)
  python3 /e2e/flood.py 20 & sleep 2
  B=$(ovf)
  /kt/kerneltuner scan --sample 5s --format json > /tmp/s.json; rc=$?
  A=$(ovf)
  echo "scan rc=$rc overflows before=$B after=$A"
  [ "$rc" = 0 ] || { echo "RESULT-FAIL scan exit $rc"; exit 0; }
  [ "$A" -gt "$B" ] || { echo "RESULT-FAIL ListenOverflows did not increase"; exit 0; }
  python3 - <<'PY'
import json
d=json.load(open('/tmp/s.json'))
r=[x for x in d['recommendations'] if x['rule_id']=='net.core.somaxconn']
if not r: print('RESULT-FAIL no somaxconn rule in output'); raise SystemExit
r=r[0]
print('rule status=%s current=%s proposed=%s evidence=%s'%(r['status'],r['current'],r['proposed'],json.dumps(r['evidence'])))
ok=r['status']=='recommend' and any('ListenOverflows' in json.dumps(e) for e in r['evidence'])
print('RESULT-OK' if ok else 'RESULT-FAIL status/evidence not as expected')
PY
  ;;
agent)
  python3 /e2e/flood.py 30 & sleep 1
  timeout 14 /kt/kerneltuner agent --interval 2s --state /tmp/state.json 2>/tmp/agent.err >/tmp/agent.out
  echo "--- stderr"; cat /tmp/agent.err
  grep -q '"firing"\|firing' /tmp/agent.err && echo "STDERR-FIRING" || echo "RESULT-FAIL no firing on stderr"
  grep -q 'firing' /tmp/state.json 2>/dev/null && echo "STATE-FIRING" || echo "RESULT-FAIL no firing in state file"
  /kt/kerneltuner status --state /tmp/state.json | tee /tmp/status.out
  grep -q 'somaxconn' /tmp/status.out && echo "STATUS-SHOWS" || echo "RESULT-FAIL status lacks somaxconn"
  ;;
esac
