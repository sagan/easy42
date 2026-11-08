#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

case "$SYSTEM_ARCH" in
    x86_64|i386|arm64|arm|armv6|armv7|mips|mipsle|mips64)
        ;;
    *)
        echo "Not applicable: unsupported architecture '$SYSTEM_ARCH' for the Phantun binary task"
        exit 20
        ;;
esac

if command -v phantun_client >/dev/null 2>&1 && command -v phantun_server >/dev/null 2>&1; then
    echo "Phantun is already installed: $(command -v phantun_client) and $(command -v phantun_server)"
    exit 10
fi

echo "Phantun is not installed"
exit 0
