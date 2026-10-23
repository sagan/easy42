#!/bin/sh
set -e

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

echo "=== Installing wg-quick (wg-quick-op) on ${OS_ID:-system} ==="

# 1. Check if wg-quick already exists (either native or third-party version)
WG_QUICK_BIN=""
if command -v wg-quick >/dev/null 2>&1; then
    WG_QUICK_BIN="$(command -v wg-quick)"
elif [ -x /usr/bin/wg-quick ]; then
    WG_QUICK_BIN="/usr/bin/wg-quick"
elif [ -x /usr/local/bin/wg-quick ]; then
    WG_QUICK_BIN="/usr/local/bin/wg-quick"
fi

if [ -n "$WG_QUICK_BIN" ]; then
    echo "wg-quick is already installed at $WG_QUICK_BIN; skipping download."
    exit 0
fi

# 2. Detect machine architecture
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
        echo "Error: architecture '$SYSTEM_ARCH' is not supported by wg-quick-op releases" >&2
        exit 1
        ;;
esac

# 3. Ensure prerequisites: tar and curl/wget
if ! command -v tar >/dev/null 2>&1; then
    echo "Installing tar..."
    install_packages tar || true
fi
if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    echo "Installing curl/wget..."
    install_packages curl || install_packages wget || true
fi

# 4. Download wg-quick-op release
VERSION="${WG_QUICK_OP_VERSION:-0.5.0}"
VERSION="${VERSION#v}"

DOWNLOAD_URL="https://github.com/sagan/wg-quick-op/releases/download/v${VERSION}/wg-quick-op_Linux_${WG_OP_ARCH}.tar.gz"
echo "Downloading wg-quick-op v${VERSION} (${WG_OP_ARCH}) from ${DOWNLOAD_URL}..."

if command -v mktemp >/dev/null 2>&1; then
    TMP_DIR="$(mktemp -d /tmp/wgquick.XXXXXX 2>/dev/null || mktemp -d)"
else
    TMP_DIR="/tmp/wgquick_install_$$"
    mkdir -p "$TMP_DIR"
fi

cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

TAR_FILE="$TMP_DIR/wg-quick-op.tar.gz"

if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$TAR_FILE" "$DOWNLOAD_URL" || curl -kfsSL -o "$TAR_FILE" "$DOWNLOAD_URL"
elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$TAR_FILE" "$DOWNLOAD_URL" || wget --no-check-certificate -q -O "$TAR_FILE" "$DOWNLOAD_URL" || wget -O "$TAR_FILE" "$DOWNLOAD_URL"
else
    echo "Error: neither curl nor wget is available to download $DOWNLOAD_URL" >&2
    exit 1
fi

echo "Extracting wg-quick-op archive..."
(cd "$TMP_DIR" && tar -xzf "$TAR_FILE")

if [ ! -f "$TMP_DIR/wg-quick-op" ]; then
    echo "Error: extracted archive does not contain wg-quick-op binary" >&2
    exit 1
fi

# 5. Install to /usr/bin/wg-quick
$SUDO mkdir -p /usr/bin
$SUDO cp -f "$TMP_DIR/wg-quick-op" /usr/bin/wg-quick
$SUDO chmod 755 /usr/bin/wg-quick

# 6. Verify installation
if [ -x /usr/bin/wg-quick ]; then
    INSTALLED_VER="$(/usr/bin/wg-quick version 2>&1 | head -n1 || echo "wg-quick-op")"
    echo "Successfully installed wg-quick to /usr/bin/wg-quick ($INSTALLED_VER)"
    if ! command -v wg >/dev/null 2>&1; then
        echo "Notice: 'wg' binary was not found in PATH; ensure wireguard-tools package is installed."
    fi
    exit 0
else
    echo "Error: /usr/bin/wg-quick installation failed" >&2
    exit 1
fi
