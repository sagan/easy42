#!/bin/sh
set -e
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
if [ -f "$SCRIPT_DIR/common.sh" ]; then
    . "$SCRIPT_DIR/common.sh"
fi

mkdir -p /etc/easy42 /usr/local/bin /etc/systemd/system

# 1. Install or upgrade agent binary
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
    cp -f "$SCRIPT_DIR/agent.toml" /etc/easy42/agent.toml
    chmod 600 /etc/easy42/agent.toml
    echo "Wrote /etc/easy42/agent.toml"
fi

# 3. Install systemd service unit
cat << 'EOF' > /etc/systemd/system/easy42-agent.service
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

# 4. Enable and start/restart service
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload
    systemctl enable easy42-agent.service
    systemctl restart easy42-agent.service
    echo "easy42-agent service successfully enabled and started via systemd"
elif command -v service >/dev/null 2>&1; then
    service easy42-agent restart || true
    echo "easy42-agent service started via sysvinit"
else
    echo "Warning: no system service manager found. Starting binary directly..."
    killall easy42-agent 2>/dev/null || true
    nohup /usr/local/bin/easy42-agent --config /etc/easy42/agent.toml >/var/log/easy42-agent.log 2>&1 &
fi

exit 0
