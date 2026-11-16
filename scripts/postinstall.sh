#!/bin/sh
# Package postinstall: refresh systemd's view of the unit. Never enables or starts it.
if [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload >/dev/null 2>&1 || true
fi
exit 0
