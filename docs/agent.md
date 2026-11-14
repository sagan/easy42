# Node Agent (`easy42-agent`) & Telemetry

The **Easy42 Agent** (`easy42-agent`) is a high-performance, lightweight background daemon written in Rust. It enables real-time hardware telemetry, edge-executed network health probes, firewall/NAT traversal, atomic configuration synchronization with lifecycle hooks, workload-App management (Docker/systemd/cron), and Phantun obfuscator supervision.

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
  - [Workload Apps, Obfuscators \& `disable_apps`](#workload-apps-obfuscators--disable_apps)
  - [Service Management \& Troubleshooting](#service-management--troubleshooting)
    - [Systemd (Debian, Ubuntu, CentOS, Arch)](#systemd-debian-ubuntu-centos-arch)
    - [OpenRC (Alpine Linux)](#openrc-alpine-linux)
    - [Procd (OpenWrt)](#procd-openwrt)

## Agent Architecture

- **Static Binary**: Compiled against `musl libc` with zero external runtime dependencies. Binary sizes are typically under 3 MB. Embedded targets are `x86_64`, `aarch64`, `armv7`, and `mipsel` (big-endian `mips` is explicitly rejected). Architecture detection uses a 6-step shell probe (`uname -m` plus MIPS endianness checks via `hexdump`/`od`, `lscpu`, and OS-release files), not just `uname -m`.
- **Persistent WebSocket Tunnel**: Connects outbound to the controller's `/api/agent/ws` endpoint, using `wss://` for HTTPS servers and `ws://` for plain-HTTP servers. This eliminates the requirement for open inbound firewall ports or public IP addresses on managed nodes. Reconnect backoff grows from `2s` to `60s`; read/write timeouts are `45s`/`10s`.
- **Protocol Buffers (Protobuf)**: All messages, telemetry reports, and remote commands are serialized via Protobuf (`proto/agent.proto`) for minimal bandwidth and CPU overhead.
- **Token Authentication**: The agent authenticates during WebSocket registration using a pre-shared bearer token configured in `agent.json`, sent as both `Authorization: Bearer <token>` and `X-Easy42-Agent-Token`. The registration payload includes `node_name`, `agent_version`, `os_info`, `hostname`, `hosts_hash`, and `disable_apps`. Use `wss://` in production so the connection is also protected by TLS.

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

Every report also carries `hosts_hash`, the current workload-App statuses, and `disable_apps`.

1. **System Load & Compute**:
   - Total CPU utilization percentage (`cpu_percent`; the first sample is `0%`).
   - System load averages (`1m`, `5m`, `15m`).
   - Physical memory usage (`memory_used_bytes` / `memory_total_bytes`), where used is `MemTotal - MemAvailable`.
   - System uptime (`uptime_seconds`).
2. **Disk Utilization**:
   - Total, used, and free storage bytes for the root filesystem (`/`). The protocol carries a list for future mount points, but the current agent collects only `/`.
3. **Interface Traffic & Throughput**:
   - Only physical, tunnel, and point-to-point interfaces are reported (`lo`, `bridge*`, `veth*` are excluded), with addresses and `primary`/`physical` flags.
   - Link operational state (`is_up`).
   - Cumulative Rx and Tx bytes and packets, plus error counters. The agent sends cumulative counters only; B/s rates and historical charts are computed server-side.
4. **WireGuard Peer Telemetry**:
   - Timestamp of the last handshake (`last_handshake_time`).
   - Transfer counters (`rx_bytes`, `tx_bytes`) per peer public key.
   - Effective endpoint address and persistent keepalive interval.
5. **BIRD Routing Protocol Status**:
   - Status of all active BGP, Kernel, and Device protocols (e.g. `Established`, `Connect`, `up`, `down`).

---

## Edge-Scheduled Cron Probes

The agent executes scheduled network latency and connectivity probes directly from the edge host:

- **ICMP Ping** (default task type): Runs `ping -c <count> -W <timeout>` (default `count=4`, `timeout=15s`) and records the raw output.
- **TCP Handshake Ping** (`task_type: "tcp_ping"`): Establishes TCP connections to `host:port` to measure latency without needing ICMP permissions; returns JSON with `local_mss`/`remote_mss`/`tcp_timestamps` parsed server-side.
- **Scheduling**: Supports standard 5-field cron expressions and 6-field expressions with a seconds field. The scheduler ticks every second and pushes new task lists from the controller without an agent restart. Tasks are selector-targeted (`name`/`tag`/`env:<NAME>`); a task matching no managed node is rejected on save.
- **Reporting**: Each run returns a Protobuf `CronTaskReport` containing the task ID, node name, execution timestamp, exit code, duration, raw stdout/stderr, and an error message. The controller persists results in SQLite (with parsed RTT/loss/min/avg/max/mdev and `ok`/`degraded`/`failed` summaries) for the UI and API, and records a fleet event on repeated failures.
- **Looking Glass allowlist**: Ad-hoc Looking Glass commands are restricted to `ping`, `traceroute`, `birdc`, `ip`, `wg`, `mtr`, `whois`, and `dig` (`Custom` category), with host/port validation and ping-count clamps (`1–30` for ping, `1–20` for `tcp_ping`).

> **Note:** The agent also runs **cron-type workload Apps** (system crontab entries installed via the Apps feature, delimited by `### EASY42_START`/`### EASY42_END` markers). That system is separate from cron probes and has no `count`/`timeout` semantics — see [Workload Apps](integrations.md#9-workload-apps-apps).

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

The agent automatically reloads `agent.json` from disk whenever a sync action is received (re-reading `pre_sync`/`post_sync`/`disable_apps`/`node_name` on every sync action, including the `disable_apps` check). You can update hook scripts or change commands **without restarting** the `easy42-agent` service.

Sync actions are restricted to an allowlist of paths (`/etc/easy42.nft`, `/etc/bird_easy42.conf`, `/etc/easy42/agent.json`, `/etc/wireguard/wg42*.conf`, `/etc/easy42_*_roa[46].conf`, and `/tmp/*`). `sync_roa` and `sync_bird` both end with `birdc configure`; deletions also run `wg-quick down` and `ip link del`.

### 3. Execution Environment & Variables

Hooks are executed with `sh -c "<command>"` under a 60-second timeout. The agent injects structured environment variables describing the sync action:

| Environment Variable      |   Available In   | Description                                                                                                                             |
| :------------------------ | :--------------: | :-------------------------------------------------------------------------------------------------------------------------------------- |
| `EASY42_SYNC_NODE`        |       Both       | The name of the node undergoing sync (e.g. `node-frankfurt`).                                                                           |
| `EASY42_SYNC_ACTION`      |       Both       | Action type: `sync_wg`, `sync_bird`, `sync_nftables`, `sync_roa`, `sync_hosts`, `delete_config`, `delete_file`, `delete_wg`, `down_wg`. |
| `EASY42_SYNC_INTERFACE`   |       Both       | Name of the network interface involved (e.g. `wg42lon`), if applicable.                                                                 |
| `EASY42_SYNC_TARGET_FILE` |       Both       | Target file path on disk (e.g. `/etc/wireguard/wg42lon.conf`).                                                                          |
| `EASY42_SYNC_DESCRIPTION` |       Both       | Human-readable description of the sync step.                                                                                            |
| `EASY42_SYNC_DIFF_STATUS` |       Both       | Status diff: `create`, `update`, `delete`, or `synced`.                                                                                 |
| `EASY42_SYNC_PHASE`       |       Both       | Current lifecycle phase: `pre` or `post`.                                                                                               |
| `EASY42_HOOK`             |       Both       | Name of the executing hook: `pre_sync` or `post_sync`.                                                                                  |
| `EASY42_SYNC_SUCCESS`     | `post_sync` only | `true` if the sync action succeeded, `false` otherwise.                                                                                 |
| `EASY42_SYNC_STATUS`      | `post_sync` only | `success` or `failed`.                                                                                                                  |
| `EASY42_SYNC_EXIT_CODE`   | `post_sync` only | Exit code of the sync action command (e.g. `0`).                                                                                        |
| `EASY42_SYNC_ERROR`       | `post_sync` only | Error message or stderr string if the sync action failed.                                                                               |

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

## Workload Apps, Obfuscators & `disable_apps`

- **Apps**: The agent declaratively syncs Docker Compose apps to `/etc/ezappc/ezapp-<name>/compose.yaml` + `.env` (`0600`), systemd apps to `/etc/systemd/system/ezapp-<name>.service` + `.env` (or `/etc/init.d/ezapp-*` + `/etc/conf.d/ezapp-*` on OpenRC/procd), auxiliary files and artifacts to `/etc/ezapp/<name>/`, and cron apps into the system crontab. Orphaned files are garbage-collected via a `.easy42-managed.json` manifest (preserving user bind-mounts). Actions `start`/`stop`/`restart`/`logs` are supported (cron apps reject actions). See [Workload Apps](integrations.md#9-workload-apps-apps).
- **Platform reporting**: On registration the agent reports `os_info` as `<os> <arch> [<endian>]`, using `uname -m` for the architecture rather than the compile-time value (a `mipsel` binary reports `mips` on every MIPS host otherwise). MIPS hosts additionally report `little`/`big`, which the server uses to pick the correct upstream artifact. See [Artifacts](integrations.md#9-workload-apps-apps).
- **Init system reporting**: The agent also reports `init_system` (`systemd`, `procd` for OpenWrt, or `openrc` for Alpine), detected from `/run/systemd/system`, `/etc/openwrt_release`/`/sbin/procd`, and `/sbin/openrc-run`/`/run/openrc`. The server uses this to transpile systemd units into procd/OpenRC init scripts. Agents older than this field are handled with an `os_info` heuristic, which cannot detect OpenWrt (since `os_info` never contains `openwrt`), so OpenWrt nodes require an agent that reports `init_system`.
- **Obfuscators**: The agent installs and supervises one Phantun service per obfuscated link endpoint (`<interface>-obfuscator`). See [WireGuard Obfuscation](architecture.md#wireguard-obfuscation-phantun).
- **`disable_apps`**: Setting `"disable_apps": true` in `agent.json` makes the agent refuse `SyncApps`/`AppAction` requests and omit app statuses from telemetry. The `install.sh` agent installer supports configuring this flag (`--disable-apps`) in a new or existing `agent.json`.
- **`install.sh` Custom Node Options**: The `/api/agent/install.sh` installer supports `--pre-sync <cmd>` and `--post-sync <cmd>` (configured via node Note data `agent_pre_sync` and `agent_post_sync`), `--disable-apps` (`agent_disable_apps`), `--node-name <name>`, and `--overwrite-config`. When installing or upgrading, any passed hooks or flags are written or updated in `/etc/easy42/agent.json`.

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
