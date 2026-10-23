#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

# Ensure standard PATH includes /usr/local/bin and /usr/local/sbin
case ":$PATH:" in
    *:/usr/local/bin:*) ;;
    *) PATH="/usr/local/bin:$PATH" ;;
esac
case ":$PATH:" in
    *:/usr/local/sbin:*) ;;
    *) PATH="/usr/local/sbin:$PATH" ;;
esac
export PATH

# 1. Check if wg-quick already exists (either native or third-party)
WG_QUICK_BIN=""
if command -v wg-quick >/dev/null 2>&1; then
    WG_QUICK_BIN="$(command -v wg-quick)"
elif [ -x /usr/bin/wg-quick ]; then
    WG_QUICK_BIN="/usr/bin/wg-quick"
elif [ -x /usr/local/bin/wg-quick ]; then
    WG_QUICK_BIN="/usr/local/bin/wg-quick"
fi

if [ -n "$WG_QUICK_BIN" ]; then
    # Probe version info if available
    VER_STR="$("$WG_QUICK_BIN" version 2>&1 | head -n1 || true)"
    if [ -n "$VER_STR" ] && ! echo "$VER_STR" | grep -qi "usage"; then
        echo "wg-quick is already installed ($VER_STR) at $WG_QUICK_BIN"
    else
        echo "wg-quick is already installed at $WG_QUICK_BIN"
    fi
    exit 10
fi

# 2. Check architecture compatibility for wg-quick-op
if command -v detect_arch >/dev/null 2>&1; then
    detect_arch
else
    SYSTEM_ARCH="$(uname -m)"
fi

case "$SYSTEM_ARCH" in
    x86_64|amd64)
        WG_OP_ARCH="x86_64"
        ;;
    i386|i686)
        WG_OP_ARCH="i386"
        ;;
    aarch64|arm64)
        WG_OP_ARCH="arm64"
        ;;
    armv7*|armhf|armv6*|arm)
        WG_OP_ARCH="armv6"
        ;;
    mips*el|mipsel|mipsle)
        WG_OP_ARCH="mipsle"
        ;;
    mips64)
        WG_OP_ARCH="mips64"
        ;;
    mips)
        WG_OP_ARCH="mips"
        ;;
    *)
        echo "Architecture '$SYSTEM_ARCH' is not supported by wg-quick-op"
        exit 20
        ;;
esac

echo "wg-quick is not installed; ready to install wg-quick-op ($WG_OP_ARCH)"
exit 0
