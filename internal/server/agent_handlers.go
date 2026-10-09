package server

import (
	"fmt"
	"net/http"
	"strings"

	"easy42/internal/agent"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var agentWsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow agents connecting from any remote IP
	},
	ReadBufferSize:  64 * 1024,
	WriteBufferSize: 64 * 1024,
}

// handleAgentWebSocket handles incoming agent WebSocket connections
func (s *Server) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract token from header or query param
	token := ""
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		token = r.Header.Get("X-Easy42-Agent-Token")
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	token = strings.TrimSpace(token)
	if token == "" {
		http.Error(w, "missing agent authentication token", http.StatusUnauthorized)
		return
	}

	node := s.mgr.FindNodeByAgentToken(token)
	if node == nil {
		http.Error(w, "invalid agent authentication token", http.StatusUnauthorized)
		return
	}

	ws, err := agentWsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := agent.NewConnection(node.Name, ws, s.mgr.AgentHub())
	s.mgr.AgentHub().Register(conn)

	// Update node runtime status
	go func() {
		_, _ = s.mgr.RefreshNodeStatus(node.Name)
	}()

	// Run ReadLoop synchronously in this connection handler
	conn.ReadLoop()
}

// handleGenerateAgentToken generates a new agent authentication token for a node
func (s *Server) handleGenerateAgentToken(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	token, err := s.mgr.GenerateAgentToken(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	host := r.Host
	serverURL := fmt.Sprintf("%s://%s", scheme, host)

	installCmd := fmt.Sprintf("curl -fsSL %s/api/agent/install.sh | bash -s -- --server %s --token %s", serverURL, serverURL, token)

	writeJSON(w, http.StatusOK, map[string]any{
		"node":            name,
		"token":           token,
		"server_url":      serverURL,
		"install_command": installCmd,
	})
}

// handleGetNodeTelemetry returns the latest live telemetry reported by the node's agent
func (s *Server) handleGetNodeTelemetry(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	node := s.mgr.FindNode(name)
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	telemetry := s.mgr.AgentHub().GetTelemetry(name)
	connected := s.mgr.AgentHub().IsConnected(name)
	lastSeen := s.mgr.AgentHub().GetLastSeen(name)

	writeJSON(w, http.StatusOK, map[string]any{
		"node":      name,
		"connected": connected,
		"last_seen": lastSeen,
		"telemetry": telemetry,
	})
}

// handleAgentBinary serves the embedded agent binary for the requested architecture
func (s *Server) handleAgentBinary(w http.ResponseWriter, r *http.Request) {
	arch := chi.URLParam(r, "arch")
	data, err := agent.GetAgentBinary(arch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"easy42-agent-%s\"", agent.NormalizeArch(arch)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	_, _ = w.Write(data)
}

// handleAgentInstallScript serves the dynamic bash installation script for easy deployment
func (s *Server) handleAgentInstallScript(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	script := `#!/bin/sh
set -e

SERVER_URL="` + serverURL + `"
TOKEN=""

while [ $# -gt 0 ]; do
  case $1 in
    --server)
      SERVER_URL="$2"
      shift 2
      ;;
    --token)
      TOKEN="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

if [ -z "$TOKEN" ]; then
  echo "Error: --token <TOKEN> is required." >&2
  exit 1
fi

echo "[Easy42 Agent Installer] Starting installation..."

ARCH=$(uname -m)
case $ARCH in
  x86_64|amd64)
    TARGET_ARCH="x86_64"
    ;;
  aarch64|arm64)
    TARGET_ARCH="aarch64"
    ;;
  armv7*|armhf)
    TARGET_ARCH="armv7"
    ;;
  mips*el|mipsel)
    TARGET_ARCH="mipsel"
    ;;
  mips*)
    # OpenWrt and Linux MIPS kernels often report 'mips' even on little-endian (mipsel) hardware.
    # Detect endianness using hexdump, od, OpenWrt release info, or ELF header EI_DATA.
    IS_LE=""
    if (printf '\001\000' 2>/dev/null || echo -n -e '\x01\x00' 2>/dev/null) | hexdump -x 2>/dev/null | grep -q "0001"; then
      IS_LE=1
    elif printf '\001\000' 2>/dev/null | od -An -t x2 2>/dev/null | grep -q "0001"; then
      IS_LE=1
    elif [ -f /etc/openwrt_release ] && grep -qi "mipsel" /etc/openwrt_release; then
      IS_LE=1
    elif [ -f /etc/os-release ] && grep -qi "mipsel" /etc/os-release; then
      IS_LE=1
    elif od -j 5 -N 1 -An -t x1 /bin/sh 2>/dev/null | grep -q "01"; then
      IS_LE=1
    elif command -v lscpu >/dev/null 2>&1 && lscpu 2>/dev/null | grep -qi "Little Endian"; then
      IS_LE=1
    fi

    if [ "$IS_LE" = "1" ]; then
      TARGET_ARCH="mipsel"
    else
      echo "Unsupported architecture: $ARCH (big-endian MIPS is not currently supported; only mipsel is available)" >&2
      exit 1
    fi
    ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

echo "[Easy42 Agent Installer] Detected architecture: $TARGET_ARCH"

# Detect init system
INIT_SYSTEM=""
if [ -d /run/systemd/system ]; then
  INIT_SYSTEM="systemd"
elif [ -d /run/openrc ] || [ -f /run/openrc/softlevel ]; then
  INIT_SYSTEM="openrc"
elif [ -f /etc/rc.common ] || [ -f /sbin/procd ]; then
  INIT_SYSTEM="procd"
elif [ -f /sbin/openrc-run ] || (command -v rc-service >/dev/null 2>&1 && command -v rc-update >/dev/null 2>&1); then
  INIT_SYSTEM="openrc"
elif command -v systemctl >/dev/null 2>&1; then
  INIT_SYSTEM="systemd"
else
  INIT_SYSTEM="unknown"
fi

echo "[Easy42 Agent Installer] Detected init system: $INIT_SYSTEM"

mkdir -p /etc/easy42 /usr/local/bin

# Write configuration
cat <<EOF > /etc/easy42/agent.json
{"server":"${SERVER_URL}","token":"${TOKEN}","log_level":"info"}
EOF
chmod 600 /etc/easy42/agent.json

# Download binary from server embedded resources
BIN_URL="${SERVER_URL}/api/agent/bin/${TARGET_ARCH}"
echo "[Easy42 Agent Installer] Downloading bundled agent binary from ${BIN_URL}..."
if curl -fsSL -o /usr/local/bin/easy42-agent "${BIN_URL}"; then
  chmod 755 /usr/local/bin/easy42-agent
  echo "[Easy42 Agent Installer] Agent binary installed to /usr/local/bin/easy42-agent"
else
  echo "[Easy42 Agent Installer] Download failed from ${BIN_URL}" >&2
  exit 1
fi

# Stop any running agent service and clean up mismatched service files
case "$INIT_SYSTEM" in
  systemd)
    systemctl stop easy42-agent 2>/dev/null || true
    if [ -f /etc/init.d/easy42-agent ]; then
      rm -f /etc/init.d/easy42-agent 2>/dev/null || true
    fi
    ;;
  openrc)
    if command -v rc-service >/dev/null 2>&1; then
      rc-service easy42-agent stop 2>/dev/null || true
    elif [ -x /etc/init.d/easy42-agent ]; then
      /etc/init.d/easy42-agent stop 2>/dev/null || true
    fi
    if [ -f /etc/systemd/system/easy42-agent.service ]; then
      rm -f /etc/systemd/system/easy42-agent.service 2>/dev/null || true
    fi
    ;;
  procd)
    if [ -x /etc/init.d/easy42-agent ]; then
      /etc/init.d/easy42-agent stop 2>/dev/null || true
    fi
    if [ -f /etc/systemd/system/easy42-agent.service ]; then
      rm -f /etc/systemd/system/easy42-agent.service 2>/dev/null || true
    fi
    ;;
esac
killall easy42-agent 2>/dev/null || true

# Install service unit based on init system
case "$INIT_SYSTEM" in
  systemd)
    mkdir -p /etc/systemd/system
    cat <<EOF > /etc/systemd/system/easy42-agent.service
[Unit]
Description=Easy42 Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/easy42-agent --config /etc/easy42/agent.json
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    chmod 644 /etc/systemd/system/easy42-agent.service
    systemctl daemon-reload
    systemctl enable easy42-agent.service
    systemctl restart easy42-agent.service 2>/dev/null || systemctl start easy42-agent.service
    echo "[Easy42 Agent Installer] easy42-agent service enabled and started via systemd."
    ;;

  openrc)
    mkdir -p /etc/init.d
    cat <<EOF > /etc/init.d/easy42-agent
#!/sbin/openrc-run

name="easy42-agent"
description="Easy42 Node Agent"

command="/usr/local/bin/easy42-agent"
command_args="--config /etc/easy42/agent.json"
command_background=true
pidfile="/run/easy42-agent.pid"
output_log="/var/log/easy42-agent.log"
error_log="/var/log/easy42-agent.log"

depend() {
    need net
    after firewall
}
EOF
    chmod 755 /etc/init.d/easy42-agent
    if command -v rc-update >/dev/null 2>&1; then
      rc-update add easy42-agent default
    fi
    if command -v rc-service >/dev/null 2>&1; then
      rc-service easy42-agent restart 2>/dev/null || rc-service easy42-agent start
    elif [ -x /etc/init.d/easy42-agent ]; then
      /etc/init.d/easy42-agent restart 2>/dev/null || /etc/init.d/easy42-agent start
    fi
    echo "[Easy42 Agent Installer] easy42-agent service enabled and started via openrc."
    ;;

  procd)
    mkdir -p /etc/init.d
    cat <<EOF > /etc/init.d/easy42-agent
#!/bin/sh /etc/rc.common

START=95
STOP=10
USE_PROCD=1

start_service() {
    procd_open_instance
    procd_set_param command /usr/local/bin/easy42-agent --config /etc/easy42/agent.json
    procd_set_param respawn
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
EOF
    chmod 755 /etc/init.d/easy42-agent
    /etc/init.d/easy42-agent enable
    /etc/init.d/easy42-agent restart 2>/dev/null || /etc/init.d/easy42-agent start
    echo "[Easy42 Agent Installer] easy42-agent service enabled and started via procd."
    ;;

  *)
    echo "[Easy42 Agent Installer] Warning: unsupported init system '$INIT_SYSTEM'. Starting binary directly in background..."
    nohup /usr/local/bin/easy42-agent --config /etc/easy42/agent.json >/var/log/easy42-agent.log 2>&1 &
    ;;
esac

echo "[Easy42 Agent Installer] Installation completed successfully!"
`

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(script))
}

// handleGetNodeMetrics returns recorded historical metrics snapshots for a node
func (s *Server) handleGetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	node := s.mgr.FindNode(name)
	if node == nil {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}

	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "24h"
	}

	points, err := s.mgr.GetNodeMetricsHistory(name, rangeStr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"node":   name,
		"range":  rangeStr,
		"points": points,
	})
}

// handleGetFleetLiveStatus returns aggregate fleet metrics and live status for all nodes
func (s *Server) handleGetFleetLiveStatus(w http.ResponseWriter, r *http.Request) {
	summary, nodes, err := s.mgr.GetFleetLiveStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"summary": summary,
		"nodes":   nodes,
	})
}
