#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

echo "=== Installing BIRD on $OS_ID ==="

case "$OS_ID" in
    ubuntu|debian|raspbian|armbian)
        # Debian/Ubuntu usually packages BIRD 2 as bird2
        if ! install_packages bird2; then
            echo "bird2 package failed, trying bird package..."
            install_packages bird
        fi
        ;;
    centos|rhel|rocky|almalinux|fedora)
        # EPEL repository might be needed for RHEL/CentOS
        if [ "$OS_ID" = "centos" ] || [ "$OS_ID" = "rhel" ] || [ "$OS_ID" = "rocky" ] || [ "$OS_ID" = "almalinux" ]; then
            $SUDO dnf install -y -q epel-release 2>/dev/null || $SUDO yum install -y -q epel-release 2>/dev/null || true
        fi
        install_packages bird
        ;;
    alpine)
        install_packages bird
        ;;
    arch|manjaro)
        install_packages bird
        ;;
    *)
        install_packages bird2 || install_packages bird
        ;;
esac

# Enable and start service if supported init system is present
for srv in bird bird2; do
    if is_service_installed "$srv"; then
        enable_service "$srv" >/dev/null 2>&1 || true
        start_service "$srv" >/dev/null 2>&1 || true
        echo "Service $srv enabled"
        break
    fi
done

if command -v bird >/dev/null 2>&1 || command -v bird2 >/dev/null 2>&1; then
    BIRD_CMD="bird"
    command -v bird >/dev/null 2>&1 || BIRD_CMD="bird2"
    echo "BIRD successfully installed: $($BIRD_CMD --version 2>&1 | head -n1)"
    exit 0
else
    echo "Error: BIRD installation finished but 'bird' binary was not found" >&2
    exit 1
fi
