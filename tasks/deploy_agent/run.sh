#!/bin/sh
set -e
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

SERVICE_NAME="easy42-agent"
SERVICE_FILE="$(get_service_file_path "$SERVICE_NAME")"

# 1. Install or upgrade agent binary
mkdir -p /usr/local/bin
if [ -f "$SCRIPT_DIR/easy42-agent" ]; then
    cp -f "$SCRIPT_DIR/easy42-agent" /usr/local/bin/easy42-agent
    chmod 755 /usr/local/bin/easy42-agent
    echo "Installed bundled easy42-agent binary to /usr/local/bin/easy42-agent"
elif [ -n "$SERVER_URL" ] && [ -n "$TARGET_ARCH" ]; then
    echo "Downloading binary from $SERVER_URL/api/agent/bin/$TARGET_ARCH..."
    curl -fsSL -o /usr/local/bin/easy42-agent "$SERVER_URL/api/agent/bin/$TARGET_ARCH"
    chmod 755 /usr/local/bin/easy42-agent
    echo "Downloaded and installed easy42-agent binary to /usr/local/bin/easy42-agent"
else
    echo "Error: easy42-agent binary not found in task payload" >&2
    exit 1
fi

# 2. Install agent configuration if provided
if [ -f "$SCRIPT_DIR/agent.toml" ]; then
    mkdir -p /etc/easy42
    cp -f "$SCRIPT_DIR/agent.toml" /etc/easy42/agent.toml
    chmod 600 /etc/easy42/agent.toml
    echo "Wrote /etc/easy42/agent.toml"
fi

# 3. Stop old service and clean up stale service files from mismatched init system
stop_service "$SERVICE_NAME" 2>/dev/null || true
if [ "$INIT_SYSTEM" != "systemd" ] && [ -f /etc/systemd/system/easy42-agent.service ]; then
    rm -f /etc/systemd/system/easy42-agent.service 2>/dev/null || true
fi
if [ "$INIT_SYSTEM" = "systemd" ] && [ -f /etc/init.d/easy42-agent ]; then
    rm -f /etc/init.d/easy42-agent 2>/dev/null || true
fi
killall easy42-agent 2>/dev/null || true

echo "=== Installing easy42-agent service ($INIT_SYSTEM) ==="

# 4. Install service unit based on init system
case "$INIT_SYSTEM" in
    systemd)
        mkdir -p /etc/systemd/system
        cat << 'EOF' > "$SERVICE_FILE"
[Unit]
Description=Easy42 Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/easy42-agent --config /etc/easy42/agent.toml
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
        chmod 644 "$SERVICE_FILE"
        ;;

    openrc)
        mkdir -p /etc/init.d
        cat << 'EOF' > "$SERVICE_FILE"
#!/sbin/openrc-run

name="easy42-agent"
description="Easy42 Node Agent"

command="/usr/local/bin/easy42-agent"
command_args="--config /etc/easy42/agent.toml"
command_background=true
pidfile="/run/easy42-agent.pid"
output_log="/var/log/easy42-agent.log"
error_log="/var/log/easy42-agent.log"

depend() {
    need net
    after firewall
}
EOF
        chmod 755 "$SERVICE_FILE"
        ;;

    procd)
        mkdir -p /etc/init.d
        cat << 'EOF' > "$SERVICE_FILE"
#!/bin/sh /etc/rc.common

START=95
STOP=10
USE_PROCD=1

start_service() {
    procd_open_instance
    procd_set_param command /usr/local/bin/easy42-agent --config /etc/easy42/agent.toml
    procd_set_param respawn
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
        chmod 755 "$SERVICE_FILE"
        ;;
esac

# 5. Enable and start service
enable_service "$SERVICE_NAME"
start_service "$SERVICE_NAME"

echo "easy42-agent service successfully installed, enabled and started via $INIT_SYSTEM"
exit 0
