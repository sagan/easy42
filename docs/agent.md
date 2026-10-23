# Node Agent (`easy42-agent`) & Telemetry

The **Easy42 Agent** (`easy42-agent`) is a high-performance, lightweight background daemon written in Rust. It enables real-time hardware telemetry, edge-executed network health probes, firewall/NAT traversal, and atomic configuration synchronization with lifecycle hooks.

---

- [Node Agent (`easy42-agent`) \& Telemetry](#node-agent-easy42-agent--telemetry)
  - [Agent Architecture](#agent-architecture)
  - [Automated Deployment via Device Helper](#automated-deployment-via-device-helper)
  - [Live Telemetry \& Monitoring](#live-telemetry--monitoring)
  - [Edge-Scheduled Cron Probes](#edge-scheduled-cron-probes)
  - [Agent Sync Hooks](#agent-sync-hooks)
    - [1. Configuration in `agent.json`](#1-configuration-in-agentjson)
    - [2. Dynamic Reloading](#2-dynamic-reloading)
    - [3. Execution Environment \& Variables](#3-execution-environment--variables)
    - [4. Safety \& Failure Semantics](#4-safety--failure-semantics)
    - [5. Example Hook Scripts](#5-example-hook-scripts)
  - [Service Management \& Troubleshooting](#service-management--troubleshooting)
    - [Systemd (Debian, Ubuntu, CentOS, Arch)](#systemd-debian-ubuntu-centos-arch)
    - [OpenRC (Alpine Linux)](#openrc-alpine-linux)
    - [Procd (OpenWrt)](#procd-openwrt)

## Agent Architecture

- **Static Binary**: Compiled against `musl libc` with zero external runtime dependencies. Binary sizes are typically under 3 MB. Supported targets include:
  - `x86_64-unknown-linux-musl`
  - `aarch64-unknown-linux-musl`
  - `armv7-unknown-linux-musleabihf`
  - `mipsel-unknown-linux-musl` (for MIPS OpenWrt routers)
- **Persistent WebSocket Tunnel**: Connects outbound to the controller's `/api/agent/ws` endpoint, using `wss://` for HTTPS servers and `ws://` for plain-HTTP servers. This eliminates the requirement for open inbound firewall ports or public IP addresses on managed nodes.
- **Protocol Buffers (Protobuf)**: All messages, telemetry reports, and remote commands are serialized via Protobuf (`proto/agent.proto`) for minimal bandwidth and CPU overhead.
- **Token Authentication**: The agent authenticates during WebSocket registration using a pre-shared bearer token configured in `agent.json`. Use `wss://` in production so the connection is also protected by TLS.

---

## Automated Deployment via Device Helper

Instead of manually compiling binaries and configuring service units, Easy42 provides a one-click automated deployment workflow via **[Device Helper](device-helper.md)** (`/helper`):

1. **Auto-Architecture Detection**: Queries the remote host (`uname -m`) to identify CPU architecture (`x86_64`, `aarch64`, etc.).
2. **Token Provisioning**: Generates a secure cryptographic agent authentication token if one has not already been assigned.
3. **Payload Injection**: Uploads the corresponding bundled `easy42-agent` binary. A generated `agent.json` is written only when `/etc/easy42/agent.json` does not already exist; redeploying preserves an existing agent configuration.
4. **Service Installation**: Detects the host's active init system (`systemd`, `openrc`, or `procd`), installs the service unit, and starts `easy42-agent`.
5. **Status Verification**: Refreshes node state on the controller once the agent connects over WebSockets.

---

## Live Telemetry & Monitoring

Every 10 seconds (configurable via `telemetry_interval_secs`), the agent samples and streams real-time system metrics to the controller:

1. **System Load & Compute**:
   - Total CPU utilization percentage (`cpu_percent`).
   - System load averages (`1m`, `5m`, `15m`).
   - Physical memory usage (`memory_used_bytes` / `memory_total_bytes`).
   - System uptime (`uptime_seconds`).
2. **Disk Utilization**:
   - Total, used, and free storage bytes for the root filesystem (`/`). The protocol carries a list for future mount points, but the current agent collects only `/`.
3. **Interface Traffic & Throughput**:
   - Link operational state (`is_up`).
   - Cumulative Rx and Tx bytes and packets.
   - Interface error counters.
   - Real-time rate computation (B/s throughput) and historical bandwidth charts.
4. **WireGuard Peer Telemetry**:
   - Timestamp of the last handshake (`last_handshake_time`).
   - Transfer counters (`rx_bytes`, `tx_bytes`) per peer public key.
   - Effective endpoint address and persistent keepalive interval.
5. **BIRD Routing Protocol Status**:
   - Status of all active BGP, Kernel, and Device protocols (e.g. `Established`, `Connect`, `up`, `down`).

---

## Edge-Scheduled Cron Probes

The agent executes scheduled network latency and connectivity probes directly from the edge host:

- **ICMP Ping** (default task type): Runs `ping -c <count> -W <timeout>` and records the raw output.
- **TCP Handshake Ping** (`task_type: "tcp_ping"`): Establishes TCP connections to `host:port` to measure latency without needing ICMP permissions.
- **Scheduling**: Supports standard 5-field cron expressions and 6-field expressions with a seconds field. The scheduler ticks every second and pushes new task lists from the controller without an agent restart.
- **Reporting**: Each run returns a Protobuf `CronTaskReport` containing the task ID, node name, execution timestamp, exit code, duration, raw stdout/stderr, and an error message. The controller persists results in SQLite for the UI and API.

---

## Agent Sync Hooks

The agent supports lifecycle **Sync Hooks** that run shell commands before and after configuration sync actions (such as updating WireGuard configs, reloading BIRD, applying nftables rules, or updating `/etc/hosts`).

### 1. Configuration in `agent.json`

Sync hooks are configured directly at the top level of `/etc/easy42/agent.json` using `pre_sync` and `post_sync`:

```json
{
  "server": "https://controller.mesh.example.com",
  "token": "e42_agent_secret_token_here",
  "log_level": "info",
  "telemetry_interval_secs": 10,
  "node_name": "node-frankfurt",
  "pre_sync": "/usr/local/bin/easy42-pre-sync.sh",
  "post_sync": "/usr/local/bin/easy42-post-sync.sh"
}
```

Inline shell commands are also supported:

```json
{
  "server": "https://controller.mesh.example.com",
  "token": "e42_agent_secret_token_here",
  "pre_sync": "logger -t easy42 'Pre-sync starting for $EASY42_SYNC_ACTION'",
  "post_sync": "logger -t easy42 'Post-sync completed with status $EASY42_SYNC_STATUS'"
}
```

### 2. Dynamic Reloading

The agent automatically reloads `agent.json` from disk whenever a sync action is received. You can update hook scripts or change commands **without restarting** the `easy42-agent` service.

### 3. Execution Environment & Variables

Hooks are executed with `sh -c "<command>"` under a 60-second timeout. The agent injects structured environment variables describing the sync action:

| Environment Variable      |   Available In   | Description                                                                                             |
| :------------------------ | :--------------: | :------------------------------------------------------------------------------------------------------ |
| `EASY42_SYNC_NODE`        |       Both       | The name of the node undergoing sync (e.g. `node-frankfurt`).                                           |
| `EASY42_SYNC_ACTION`      |       Both       | Action type: `sync_wg`, `sync_bird`, `sync_nftables`, `sync_roa`, `sync_hosts`, `delete_wg`, `down_wg`. |
| `EASY42_SYNC_INTERFACE`   |       Both       | Name of the network interface involved (e.g. `wg42lon`), if applicable.                                 |
| `EASY42_SYNC_TARGET_FILE` |       Both       | Target file path on disk (e.g. `/etc/wireguard/wg42lon.conf`).                                          |
| `EASY42_SYNC_DESCRIPTION` |       Both       | Human-readable description of the sync step.                                                            |
| `EASY42_SYNC_DIFF_STATUS` |       Both       | Status diff: `create`, `update`, `delete`, or `synced`.                                                 |
| `EASY42_SYNC_PHASE`       |       Both       | Current lifecycle phase: `pre` or `post`.                                                               |
| `EASY42_HOOK`             |       Both       | Name of the executing hook: `pre_sync` or `post_sync`.                                                  |
| `EASY42_SYNC_SUCCESS`     | `post_sync` only | `true` if the sync action succeeded, `false` otherwise.                                                 |
| `EASY42_SYNC_STATUS`      | `post_sync` only | `success` or `failed`.                                                                                  |
| `EASY42_SYNC_EXIT_CODE`   | `post_sync` only | Exit code of the sync action command (e.g. `0`).                                                        |
| `EASY42_SYNC_ERROR`       | `post_sync` only | Error message or stderr string if the sync action failed.                                               |

### 4. Safety & Failure Semantics

- **`pre_sync` Hook Blocking**: If the `pre_sync` hook exits with a non-zero exit code, the sync action is **immediately aborted**. No configuration files are written, no services are reloaded, and the failure is reported to the controller.
- **`post_sync` Hook Reporting**: The `post_sync` hook runs after the sync action finishes. If it fails, its exit code and stderr are logged and returned to the controller as an error.

### 5. Example Hook Scripts

**Pre-sync backup script (`/usr/local/bin/easy42-pre-sync.sh`)**:

```bash
#!/bin/sh
set -e
# Verify that disk space is sufficient before applying changes
AVAILABLE_KB=$(df -k /etc | awk 'NR==2 {print $4}')
if [ "$AVAILABLE_KB" -lt 5120 ]; then
  echo "Error: /etc has less than 5MB free space!" >&2
  exit 1
fi

# Backup existing configuration if target file exists
if [ -n "$EASY42_SYNC_TARGET_FILE" ] && [ -f "$EASY42_SYNC_TARGET_FILE" ]; then
  cp -p "$EASY42_SYNC_TARGET_FILE" "${EASY42_SYNC_TARGET_FILE}.bak"
fi
exit 0
```

**Post-sync notification script (`/usr/local/bin/easy42-post-sync.sh`)**:

```bash
#!/bin/sh
if [ "$EASY42_SYNC_STATUS" = "success" ]; then
  logger -t easy42 "Successfully applied $EASY42_SYNC_ACTION on $EASY42_SYNC_NODE"
else
  logger -p user.err -t easy42 "FAILED $EASY42_SYNC_ACTION on $EASY42_SYNC_NODE (code $EASY42_SYNC_EXIT_CODE): $EASY42_SYNC_ERROR"
fi
```

---

## Service Management & Troubleshooting

### Systemd (Debian, Ubuntu, CentOS, Arch)

```bash
# Start and enable the service
systemctl enable --now easy42-agent

# View live service logs
journalctl -u easy42-agent -f
```

### OpenRC (Alpine Linux)

```bash
# Check service status
rc-service easy42-agent status

# Restart the daemon
rc-service easy42-agent restart
```

### Procd (OpenWrt)

```bash
# Inspect agent output from logd
logread -e easy42-agent

# Restart service
/etc/init.d/easy42-agent restart
```
