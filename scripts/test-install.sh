#!/bin/sh
# Regression tests for scripts/install.sh using a stubbed curl (no network).
# Run on Linux, e.g.: docker run --rm -v "$PWD:/src:ro" -w /src debian:bookworm-slim sh scripts/test-install.sh
# shellcheck disable=SC2015
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
fail=0
ok() { printf 'ok   %s\n' "$1"; }
bad() { printf 'FAIL %s\n' "$1"; fail=1; }

ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; esac
mkdir -p "$T/bin" "$T/srv" "$T/pkg"
printf '#!/bin/sh\necho fake kerneltuner\n' >"$T/pkg/kerneltuner"
chmod +x "$T/pkg/kerneltuner"
NAME="kerneltuner_1.0.0_linux_${ARCH}.tar.gz"
tar -czf "$T/srv/$NAME" -C "$T/pkg" kerneltuner
SUM=$(sha256sum "$T/srv/$NAME" | cut -d' ' -f1)
printf 'x\n' >"$T/srv/checksums.txt.sig"
printf 'x\n' >"$T/srv/checksums.txt.pem"

# curl stub: copy the requested basename from $T/srv.
cat >"$T/bin/curl" <<EOF
#!/bin/sh
while [ \$# -gt 0 ]; do
    case "\$1" in -o) out=\$2; shift 2 ;; -*) shift ;; *) url=\$1; shift ;; esac
done
f="$T/srv/\$(basename "\$url")"
[ -f "\$f" ] && cp "\$f" "\$out"
EOF
chmod +x "$T/bin/curl"

run() { PATH="$T/bin:$PATH" sh "$HERE/install.sh" -v v1.0.0 -d "$T/dest" 2>&1; }

# Decoy lines that merely contain the archive name must not be picked or used
# as the checksum (grep -F, exact name match).
cat >"$T/srv/checksums.txt" <<EOF
0000000000000000000000000000000000000000000000000000000000000000  ${NAME}.sbom.json
${SUM}  ${NAME}
EOF
out=$(run)
if [ -x "$T/dest/kerneltuner" ] && printf '%s' "$out" | grep -q 'sha256 verified'; then ok "decoy sbom line ignored"; else bad "decoy sbom line ignored: $out"; fi
printf '%s' "$out" | grep -q 'cosign not found' && ok "note printed without cosign" || bad "no cosign note: $out"

# A name with regex metacharacters is matched literally, not as a pattern.
printf '%s  %s\n' "$SUM" "kerneltuner_1.0.0_linux_${ARCH}.tar.gz" >"$T/srv/checksums.txt"
rm -rf "$T/dest"
run >/dev/null && ok "literal match" || bad "literal match"

# Path traversal in the archive name is refused.
printf '%s  ../evil_linux_%s.tar.gz\n' "$SUM" "$ARCH" >"$T/srv/checksums.txt"
rm -rf "$T/dest"
out=$(run); printf '%s' "$out" | grep -q 'unexpected archive name' && ok "traversal name refused" || bad "traversal name: $out"

# Checksum mismatch.
printf '%s  %s\n' "0000000000000000000000000000000000000000000000000000000000000000" "$NAME" >"$T/srv/checksums.txt"
rm -rf "$T/dest"
out=$(run); printf '%s' "$out" | grep -q 'sha256 mismatch' && ok "mismatch refused" || bad "mismatch: $out"

# cosign present: a failing verification aborts, a passing one is reported.
printf '%s  %s\n' "$SUM" "$NAME" >"$T/srv/checksums.txt"
printf '#!/bin/sh\nexit 1\n' >"$T/bin/cosign"; chmod +x "$T/bin/cosign"
rm -rf "$T/dest"
out=$(run); printf '%s' "$out" | grep -q 'signature verification of checksums.txt failed' && [ ! -e "$T/dest/kerneltuner" ] && ok "bad signature refused" || bad "bad signature: $out"
printf '#!/bin/sh\nexit 0\n' >"$T/bin/cosign"
rm -rf "$T/dest"
out=$(run); printf '%s' "$out" | grep -q 'cosign signature verified' && [ -x "$T/dest/kerneltuner" ] && ok "good signature accepted" || bad "good signature: $out"

exit $fail
