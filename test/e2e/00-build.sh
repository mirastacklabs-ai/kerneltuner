#!/bin/sh
# Build the linux binary (host arch) into docker volume $BIN_VOL (never bin/).
# A volume is used because the Colima VM does not share the scratchpad path.
. "$(dirname "$0")/lib.sh"
docker volume create "$BIN_VOL" >/dev/null
docker run --rm -v "$REPO":/src:ro -v kerneltuner-gocache:/go -v "$BIN_VOL":/out -w /src \
  -e GOCACHE=/go/cache -e CGO_ENABLED=0 -e GOOS=linux "$GO_IMAGE" \
  go build -trimpath -o /out/kerneltuner ./cmd/kerneltuner || { fail build "go build failed"; exit 1; }
pass build
