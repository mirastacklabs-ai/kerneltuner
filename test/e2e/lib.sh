# Shared helpers, sourced by every scenario. POSIX sh.
# Safety: never --net=host, never write host/VM sysctls. Apply/overflow
# scenarios run only in disposable --privileged containers with their own netns.
E2E_DIR=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$E2E_DIR/../.." && pwd)
BIN_VOL=${KT_E2E_BINVOL:-kt-e2e-bin}      # docker volume holding the linux binary
GO_IMAGE=${GO_IMAGE:-golang:1.27}
ALPINE=${ALPINE:-alpine:3}
PYIMG=${PYIMG:-python:3.12-slim}
WORK=${KT_E2E_TMP:-/private/tmp/claude-501/-Users-aarvee-repos-github-public-kerneltuner/3e43e5e7-efd7-49d2-9c6a-3034572ad155/scratchpad/e2e}
mkdir -p "$WORK"

pass() { echo "PASS $1"; }
fail() { echo "FAIL $1: $2"; }
skip() { echo "SKIP $1: $2"; }

# disposable <image> <args...>: privileged, own netns (default bridge), removed on exit.
disposable() { docker run --rm --privileged --network bridge \
  -v "$BIN_VOL":/kt:ro -v "$E2E_DIR/lib":/e2e:ro "$@"; }
