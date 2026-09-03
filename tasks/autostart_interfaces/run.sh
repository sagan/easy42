#!/bin/sh
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

case "$INIT_SYSTEM" in
    systemd|openrc|procd)
        ;;
    *)
        echo "Error: unsupported init system '$INIT_SYSTEM' (supported: systemd, openrc, procd)" >&2
        exit 1
        ;;
esac

SERVICE_NAME="easy42-wg-autostart"
SERVICE_FILE="$(get_service_file_path "$SERVICE_NAME")"

echo "=== Installing easy42 WireGuard autostart service ($INIT_SYSTEM) ==="

case "$INIT_SYSTEM" in
    systemd)
        cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
[Unit]
Description=easy42 WireGuard interfaces autostart
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/bin/sh -c 'for conf in /etc/wireguard/wg42*.conf; do [ -f "$$conf" ] || continue; iface=$${conf##*/}; iface=$${iface%.conf}; wg-quick up "$$iface" || true; done'
ExecStop=/bin/sh -c 'for conf in /etc/wireguard/wg42*.conf; do [ -f "$$conf" ] || continue; iface=$${conf##*/}; iface=$${iface%.conf}; wg-quick down "$$iface" || true; done'

[Install]
WantedBy=multi-user.target
EOF
        $SUDO chmod 644 "$SERVICE_FILE"
        ;;

    openrc)
        $SUDO mkdir -p /etc/init.d
        cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
#!/sbin/openrc-run

description="easy42 WireGuard interfaces autostart"

depend() {
    need net
    after firewall
}

start() {
    ebegin "Starting easy42 WireGuard interfaces"
    for conf in /etc/wireguard/wg42*.conf; do
        [ -f "$conf" ] || continue
        iface="${conf##*/}"
        iface="${iface%.conf}"
        wg-quick up "$iface" || true
    done
    eend 0
}

stop() {
    ebegin "Stopping easy42 WireGuard interfaces"
    for conf in /etc/wireguard/wg42*.conf; do
        [ -f "$conf" ] || continue
        iface="${conf##*/}"
        iface="${iface%.conf}"
        wg-quick down "$iface" || true
    done
    eend 0
}
EOF
        $SUDO chmod 755 "$SERVICE_FILE"
        ;;

    procd)
        $SUDO mkdir -p /etc/init.d
        cat << 'EOF' | $SUDO tee "$SERVICE_FILE" >/dev/null
#!/bin/sh /etc/rc.common

START=95
STOP=10

start() {
    for conf in /etc/wireguard/wg42*.conf; do
        [ -f "$conf" ] || continue
        iface="${conf##*/}"
        iface="${iface%.conf}"
        wg-quick up "$iface" || true
    done
}

stop() {
    for conf in /etc/wireguard/wg42*.conf; do
        [ -f "$conf" ] || continue
        iface="${conf##*/}"
        iface="${iface%.conf}"
        wg-quick down "$iface" || true
    done
}
EOF
        $SUDO chmod 755 "$SERVICE_FILE"
        ;;
esac

enable_service "$SERVICE_NAME"

echo "easy42 WireGuard autostart service ($INIT_SYSTEM) successfully installed and enabled"
exit 0
