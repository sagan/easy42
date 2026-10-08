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

	script := `#!/bin/bash
set -e

SERVER_URL="` + serverURL + `"
TOKEN=""

while [[ $# -gt 0 ]]; do
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
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

echo "[Easy42 Agent Installer] Detected architecture: $TARGET_ARCH"

mkdir -p /etc/easy42 /usr/local/bin

# Write configuration
cat <<EOF > /etc/easy42/agent.toml
# Easy42 Agent Configuration
server = "${SERVER_URL}"
token = "${TOKEN}"
log_level = "info"
EOF
chmod 600 /etc/easy42/agent.toml

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

# Install systemd service
cat <<EOF > /etc/systemd/system/easy42-agent.service
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

if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  systemctl enable --now easy42-agent.service || true
  echo "[Easy42 Agent Installer] easy42-agent service enabled and started."
fi

echo "[Easy42 Agent Installer] Installation completed successfully!"
`

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	_, _ = w.Write([]byte(script))
}
