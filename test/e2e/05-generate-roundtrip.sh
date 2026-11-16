#!/bin/sh
. "$(dirname "$0")/lib.sh"
N=generate-roundtrip
G=$WORK/gen
# Outputs are produced and validated in disposable containers; nothing lands on the host.
# Generate into a volume-less tmp inside one container chain via stdout tar.
docker run --rm -v "$BIN_VOL":/kt:ro -v "$REPO":/src:ro -w /src "$ALPINE" sh -c '
  /kt/kerneltuner generate --fixture web-16c-64g-nginx --format all --out /tmp/out >/dev/null &&
  /kt/kerneltuner generate --fixture cassandra-stock --format all --out /tmp/out2 >/dev/null &&
  tar -C /tmp -cf - out out2' > "$WORK/gen.tar" 2>"$WORK/gen.err" || { fail $N "generate failed: $(head -c 200 "$WORK/gen.err")"; exit 1; }
# 1. sysctl keys exist under /proc/sys in a fresh container; 2. tuned parses as INI
res=$(docker run --rm -i "$PYIMG" sh -c '
  mkdir /w && tar -C /w -xf - || exit 2
  python3 - <<PY
import re,configparser,os,sys
bad=[]
txt=open("/w/out/90-kerneltuner.conf").read()
keys=[m.group(1) for m in re.finditer(r"^\s*([a-z0-9_.\-/]+)\s*=\s*\S", txt, re.M)]
if not keys: bad.append("no sysctl keys parsed")
for k in keys:
    if not os.path.exists("/proc/sys/"+k.replace(".","/")): bad.append("missing /proc/sys key "+k)
cp=configparser.ConfigParser(delimiters=("=",),interpolation=None,strict=False); cp.optionxform=str
try: cp.read_string(open("/w/out/tuned.conf").read())
except Exception as e: bad.append("tuned.conf not INI: %s"%e)
else:
    if "main" not in cp or "sysctl" not in cp: bad.append("tuned.conf lacks [main]/[sysctl]")
print("BAD:"+"; ".join(bad) if bad else "OK keys=%d"%len(keys))
PY' < "$WORK/gen.tar" 2>&1)
case $res in OK*) ;; *) fail $N "$res"; exit 1;; esac
# 3. systemd-analyze verify of the generated drop-ins (cassandra-stock emits one;
#    the nginx fixture emits none).
res=$(docker run --rm -i debian:bookworm-slim sh -c '
  apt-get update -qq >/dev/null 2>&1 && apt-get install -y -qq --no-install-recommends systemd >/dev/null 2>&1 || { echo "BAD: apt install systemd failed"; exit 0; }
  mkdir /w && tar -C /w -xf - || exit 2
  d=$(find /w/out /w/out2 -path "*.service.d/*.conf" | head -1); [ -n "$d" ] || { echo "BAD: no drop-in generated"; exit 0; }
  svc=$(basename "$(dirname "$d")" .d)
  mkdir -p /etc/systemd/system/$svc.d && cp "$d" /etc/systemd/system/$svc.d/
  printf "[Service]\nExecStart=/bin/true\n" > /etc/systemd/system/$svc
  o=$(systemd-analyze verify /etc/systemd/system/$svc 2>&1); rc=$?
  echo "$o" | grep -iE "unknown|invalid|error|failed" | grep -v "Failed to connect\|bus" && { echo "BAD: $o"; exit 0; }
  [ $rc -eq 0 ] && echo "OK $svc" || echo "BAD rc=$rc: $o"' < "$WORK/gen.tar" 2>&1 | tail -3)
rm -f "$WORK/gen.tar"
case $res in *OK*) pass $N;; *) fail $N "systemd-analyze: $res";; esac
