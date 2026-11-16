#!/bin/sh
# kerneltuner installer: downloads a release tarball, verifies its sha256, installs the binary.
# Usage: install.sh [-v VERSION] [-d INSTALL_DIR]
#   VERSION defaults to the latest release (e.g. v1.0.0); INSTALL_DIR defaults to /usr/local/bin.
# Env: KERNELTUNER_VERSION, KERNELTUNER_INSTALL_DIR override the defaults.
set -eu

REPO="mirastacklabs-ai/kerneltuner"
VERSION="${KERNELTUNER_VERSION:-latest}"
DEST="${KERNELTUNER_INSTALL_DIR:-/usr/local/bin}"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }

while getopts "v:d:h" opt; do
    case "$opt" in
        v) VERSION="$OPTARG" ;;
        d) DEST="$OPTARG" ;;
        *) printf 'usage: %s [-v VERSION] [-d INSTALL_DIR]\n' "$0" >&2; exit 2 ;;
    esac
done

[ "$(uname -s)" = "Linux" ] || die "kerneltuner supports Linux only (found $(uname -s))"

case "$(uname -m)" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) die "unsupported architecture: $(uname -m) (supported: x86_64, aarch64)" ;;
esac

if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL --retry 3 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -q -O "$2" "$1"; }
else
    die "need curl or wget"
fi

if command -v sha256sum >/dev/null 2>&1; then
    sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
    sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
    die "need sha256sum or shasum"
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT INT TERM

if [ "$VERSION" = "latest" ]; then
    BASE="https://github.com/$REPO/releases/latest/download"
else
    case "$VERSION" in v*) ;; *) VERSION="v$VERSION" ;; esac
    BASE="https://github.com/$REPO/releases/download/$VERSION"
fi

fetch "$BASE/checksums.txt" "$WORK/checksums.txt" || die "download failed: $BASE/checksums.txt"

# checksums.txt is signed keylessly (cosign, OIDC). Verify the signature when
# cosign is available; otherwise say so.
if command -v cosign >/dev/null 2>&1; then
    fetch "$BASE/checksums.txt.sig" "$WORK/checksums.txt.sig" || die "download failed: $BASE/checksums.txt.sig"
    fetch "$BASE/checksums.txt.pem" "$WORK/checksums.txt.pem" || die "download failed: $BASE/checksums.txt.pem"
    cosign verify-blob \
        --certificate "$WORK/checksums.txt.pem" \
        --signature "$WORK/checksums.txt.sig" \
        --certificate-identity-regexp "^https://github[.]com/$REPO/[.]github/workflows/release[.]yml@refs/tags/v.+" \
        --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
        "$WORK/checksums.txt" >/dev/null 2>&1 || die "cosign signature verification of checksums.txt failed"
    info "cosign signature verified"
else
    info "note: cosign not found; the checksums.txt signature was not verified (install cosign to verify releases)"
fi

# Archive names embed the version, so look the name up in checksums.txt.
ARCHIVE="$(grep -F "_linux_${ARCH}.tar.gz" "$WORK/checksums.txt" | awk '{print $2}' | grep -E '\.tar\.gz$' | head -n 1)"
[ -n "$ARCHIVE" ] || die "no linux/$ARCH archive listed in checksums.txt ($VERSION)"
# The name comes from a downloaded file: allow plain file names only.
case "$ARCHIVE" in
    *[!A-Za-z0-9._-]* | .*) die "unexpected archive name in checksums.txt: $ARCHIVE" ;;
esac

info "Downloading $ARCHIVE ($VERSION, linux/$ARCH)"
fetch "$BASE/$ARCHIVE" "$WORK/$ARCHIVE" || die "download failed: $BASE/$ARCHIVE"

WANT="$(grep -F "$ARCHIVE" "$WORK/checksums.txt" | awk -v a="$ARCHIVE" '$2 == a {print $1; exit}')"
[ -n "$WANT" ] || die "no checksum for $ARCHIVE in checksums.txt"
GOT="$(sha256 "$WORK/$ARCHIVE")"
[ "$WANT" = "$GOT" ] || die "sha256 mismatch for $ARCHIVE (expected $WANT, got $GOT)"
info "sha256 verified"

tar -xzf "$WORK/$ARCHIVE" -C "$WORK" kerneltuner || die "kerneltuner not found in archive"

if [ ! -d "$DEST" ]; then
    mkdir -p "$DEST" 2>/dev/null || die "cannot create $DEST; rerun with sufficient privileges or pass -d DIR"
fi
[ -w "$DEST" ] || die "$DEST is not writable; rerun with sufficient privileges (e.g. as root) or pass -d DIR"
install -m 0755 "$WORK/kerneltuner" "$DEST/kerneltuner" || die "install to $DEST failed"

info "Installed $DEST/kerneltuner"
"$DEST/kerneltuner" version 2>/dev/null || true
