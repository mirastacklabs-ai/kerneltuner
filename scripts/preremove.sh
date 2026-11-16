#!/bin/sh
# Package preremove: stop and disable the agent when the package is really
# being removed. deb passes "remove" (also "upgrade", "deconfigure"); rpm passes
# the number of instances left after this operation (0 = full removal, 1 = upgrade).
case "${1:-}" in
    remove | 0)
        if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
            systemctl disable --now kerneltuner >/dev/null 2>&1 || true
        fi
        ;;
esac
exit 0
