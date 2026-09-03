#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

echo "=== Installing WireGuard on $OS_ID ==="

case "$OS_ID" in
    ubuntu|debian|raspbian|armbian)
        install_packages wireguard wireguard-tools
        ;;
    alpine)
        install_packages wireguard-tools
        ;;
    centos|rhel|rocky|almalinux|fedora)
        install_packages wireguard-tools
        ;;
    arch|manjaro)
        install_packages wireguard-tools
        ;;
    *)
        install_packages wireguard wireguard-tools
        ;;
esac

# Attempt to load wireguard kernel module (non-fatal if userspace or built-in)
$SUDO modprobe wireguard >/dev/null 2>&1 || true

# Ensure wireguard directory exists with proper permissions
$SUDO mkdir -p /etc/wireguard
$SUDO chmod 700 /etc/wireguard

if command -v wg >/dev/null 2>&1; then
    echo "WireGuard successfully installed: $(wg --version 2>&1 | head -n1)"
    exit 0
else
    echo "Error: WireGuard installation finished but 'wg' binary was not found" >&2
    exit 1
fi
