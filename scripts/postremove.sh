#!/bin/sh
# Package postremove: refresh systemd's view after the unit file is gone.
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi
exit 0
