#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

case "$INIT_SYSTEM" in
    systemd|openrc|procd)
        ;;
    *)
        echo "Not applicable: unsupported init system '$INIT_SYSTEM' (supported: systemd, openrc, procd)"
        exit 20
        ;;
esac

SERVICE_NAME="easy42-wg-autostart"
SERVICE_FILE="$(get_service_file_path "$SERVICE_NAME")"

if is_service_installed "$SERVICE_NAME"; then
    if is_service_enabled "$SERVICE_NAME"; then
        echo "WireGuard autostart service ($INIT_SYSTEM) is already installed and enabled"
        exit 10
    else
        echo "WireGuard autostart service file exists ($SERVICE_FILE) but is not enabled"
        exit 0
    fi
fi

echo "WireGuard autostart service ($INIT_SYSTEM) is not installed"
exit 0
