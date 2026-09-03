#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

if command -v wg >/dev/null 2>&1 && command -v wg-quick >/dev/null 2>&1; then
    WG_VER="$(wg --version 2>&1 | head -n1)"
    echo "WireGuard is already installed ($WG_VER)"
    exit 10
fi

echo "WireGuard is not installed"
exit 0
