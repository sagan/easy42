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

SERVICE_NAME="easy42-agent"

if command -v easy42-agent >/dev/null 2>&1; then
    VER="$(easy42-agent --version 2>&1 | head -n1)"
    if is_service_installed "$SERVICE_NAME"; then
        if is_service_running "$SERVICE_NAME"; then
            echo "Easy42 Agent is active and running ($VER, $INIT_SYSTEM)"
            exit 10
        elif is_service_enabled "$SERVICE_NAME"; then
            echo "Easy42 Agent binary installed ($VER) and service enabled ($INIT_SYSTEM), but service is stopped"
            exit 0
        else
            echo "Easy42 Agent binary installed ($VER) but service is not enabled/running ($INIT_SYSTEM)"
            exit 0
        fi
    else
        echo "Easy42 Agent binary installed ($VER) but service is not installed ($INIT_SYSTEM)"
        exit 0
    fi
fi

echo "Easy42 Agent is not installed"
exit 0
