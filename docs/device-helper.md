# Device Helper & Remote Host Automation

**Device Helper** is Easy42's integrated remote provisioning and host lifecycle automation subsystem. It enables operators to verify prerequisites, install packages, tune kernel parameters, configure daemon integration points, apply persistent node addresses, and deploy services across all managed nodes in a cluster without requiring external configuration management tooling such as Ansible, Puppet, Salt, or Terraform.

---

## Overview & Architecture

When deploying dynamic WireGuard overlay networks with BIRD 2 routing, target Linux hosts typically require foundational system preparation:
- Installing the WireGuard kernel module/tools and the BIRD 2 routing daemon.
- Enabling IPv4/IPv6 packet forwarding and configuring reverse path filtering (`rp_filter`) to prevent asymmetric routing drops.
- Persisting node overlay/external IPv4 and IPv6 addresses (including dummy interfaces) through the host's native network manager.
- Injecting Easy42 configuration includes into `/etc/bird/bird.conf` and `/etc/nftables.conf`.
- Installing interface autostart boot units (`wg42*`).
- Provisioning and starting the `easy42-agent` binary and system service.

Device Helper packages these operational workflows into **embedded tasks**. Each task contains non-destructive status inspection scripts (`status.sh`) and idempotent execution scripts (`run.sh`) that run natively on target nodes.

```
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                         easy42 Controller / Web UI                          │
 │                                                                             │
 │    ┌───────────────────────────┐         ┌─────────────────────────────┐    │
 │    │ Device Helper UI (/helper)│         │     REST API (/api/tasks)   │    │
 │    └─────────────┬─────────────┘         └──────────────┬──────────────┘    │
 └──────────────────┼──────────────────────────────────────┼───────────────────┘
                    │                                      │
                    ▼                                      ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                      Task Execution Engine (tasks.go)                       │
 │    - Dynamic payload injection (agent binary/agent.json, node_ip.env)       │
 │    - Optional custom_network.sh payload from node configuration             │
 │    - Ephemeral SFTP upload to /tmp/.easy42_task_<id>_<rand>                 │
 │    - SSH remote execution through `sh` with root/sudo privilege detection   │
 │    - Automatic deferred remote cleanup (rm -rf)                             │
 └──────────────────────────────────────┬──────────────────────────────────────┘
                                        │
           ┌────────────────────────────┼────────────────────────────┐
           │ (SSH / SFTP)               │ (SSH / SFTP)               │ (SSH / SFTP)
           ▼                            ▼                            ▼
 ┌───────────────────┐        ┌───────────────────┐        ┌───────────────────┐
 │   Debian/Ubuntu   │        │   Alpine Linux    │        │      OpenWrt      │
 │  (systemd + apt)  │        │ (openrc + apk)    │        │  (procd + opkg)   │
 └───────────────────┘        └───────────────────┘        └───────────────────┘
```

### Key Architectural Characteristics

1. **Zero External Dependencies**: All scripts and manifests are embedded directly into the Easy42 binary via Go's `embed.FS`. Adding or editing a task requires rebuilding the binary.
2. **Ephemeral & Safe Execution**: Scripts are uploaded via SFTP to a uniquely named remote directory (`/tmp/.easy42_task_<task_id>_<randHex>`) with `0755` permissions, executed over SSH, and removed by a deferred cleanup handler.
3. **Privilege Elevation**: The shared `common.sh` detects whether the SSH session runs as `root`. If not, it uses `sudo` and fails with a clear error when `sudo` is unavailable.
4. **Targeted Fleet Filtering**: Nodes can be filtered by user-defined tags (`#cloud`, `#edge`, `#eu`, etc.) or selected individually for precise, progressive rollouts. External nodes are never eligible for helper tasks.
5. **Non-Destructive Status Inspection**: Before applying any change, Device Helper queries target nodes to determine whether a task is already satisfied, ready to apply, or incompatible.
6. **Payload-Aware Tasks**: `deploy_agent` and `configure_network` receive generated files at execution time (`easy42-agent`, `agent.json`, `node_ip.env`, and optionally `custom_network.sh`) rather than relying on static scripts alone.

### SSH Authentication & Host Key Verification

Device Helper communicates with remote managed nodes strictly over SSH and SFTP. It **directly and exclusively uses the current system OS user's OpenSSH configuration and files** on the controller host:

- **Client Authentication**: Reads private keys from `~/.ssh/id_ed25519` (with fallback to `~/.ssh/id_rsa`, `~/.ssh/id_ecdsa`, or an active SSH Agent via `$SSH_AUTH_SOCK`). Easy42 does not prompt for passwords or store SSH private keys within its encrypted database.
- **Server Host Key Verification**: Strictly verifies remote host keys against `~/.ssh/known_hosts` (and `/etc/ssh/ssh_known_hosts`). Connections to unknown hosts or mismatched keys are rejected immediately to protect against man-in-the-middle attacks.
- **Host Configuration (`~/.ssh/config`)**: Transparently respects `HostName`, `User`, `Port`, `IdentityFile`, and `HostKeyAlgorithms` directives defined in the user's `~/.ssh/config`.

**Setup for Target Nodes**:
1. Copy the controller host user's public key (`~/.ssh/id_ed25519.pub`) to target nodes:
   ```bash
   ssh-copy-id -i ~/.ssh/id_ed25519.pub root@<node-host>
   ```
2. Scan and trust the target host's public key in the controller's `~/.ssh/known_hosts`:
   ```bash
   ssh-keyscan -H <node-host> >> ~/.ssh/known_hosts
   ```

---

## Embedded Task Catalog

Tasks are sorted by weight and categorized into logical operational domains:

| Task ID | Category | Title | Weight | Actions Performed |
| :--- | :--- | :--- | :---: | :--- |
| `deploy_agent` | Services | **Deploy Agent** | `5` | Detects remote CPU architecture, obtains or reuses the node agent token, uploads the bundled `easy42-agent` binary, writes `agent.json` only when none exists, creates and enables a system service (`systemd`, `openrc`, or `procd`), and refreshes node status. |
| `configure_network` | System & Network | **Configure Network** | `6` | Persists the node's main/external IPv4 and IPv6 addresses (and optional custom network script) through netplan, `/etc/network/interfaces`, systemd-networkd, or OpenWrt UCI. See [Network Configuration Task](#network-configuration-task). |
| `install_wireguard` | Packages | **Install WireGuard** | `10` | Installs WireGuard and `wireguard-tools` using native package managers (`apt`, `apk`, `pacman`, `dnf`/`yum`, `opkg`). |
| `install_bird` | Packages | **Install BIRD** | `20` | Installs BIRD 2 (`bird2` or `bird`) via the native package manager and enables the system service. |
| `config_bird` | Configuration | **Configure BIRD Main Config** | `30` | Finds `/etc/bird/bird.conf`, `/etc/bird.conf`, or `/etc/bird2/bird.conf` (creating `/etc/bird/bird.conf` when none exists), creates a fallback placeholder `/etc/bird_easy42.conf`, appends `include "/etc/bird_easy42.conf";`, validates syntax with `bird -p`, and issues `birdc configure` if the daemon is active. |
| `config_nftables` | Configuration | **Configure Nftables Autostart** | `35` | Creates placeholder `/etc/easy42.nft`, hooks `include "/etc/easy42.nft"` into `/etc/nftables.conf`, or installs the standalone `easy42-nftables` service unit on systems without standard nftables loaders. |
| `sysctl_params` | System & Network | **Configure Sysctl Parameters** | `40` | Deploys `/etc/sysctl.d/99-easy42.conf` enabling `net.ipv4.ip_forward=1`, `net.ipv6.conf.all.forwarding=1`, and loose reverse path filtering (`net.ipv4.conf.all.rp_filter=0`, `net.ipv4.conf.default.rp_filter=0`) so asymmetric dynamic routing works without kernel packet drops. |
| `autostart_interfaces` | System & Network | **WireGuard Autostart Service** | `50` | Generates and enables an autostart daemon (`easy42-autostart`) under `systemd`, `openrc`, or `procd` to bring up all `wg42*` interfaces automatically on boot. |

The task list API returns the manifest fields exactly as stored in each `task.json`, sorted by ascending weight and then by task ID.

`deploy_agent` does not overwrite an existing `/etc/easy42/agent.json`. If the node's agent token or controller URL changes, update that file manually (or remove it before redeploying) and restart `easy42-agent`.

---

## Network Configuration Task

`configure_network` is the only task that changes the node's persistent interface addressing. It is intended to make the addresses managed by Easy42 survive a reboot rather than only existing in the live kernel.

### Inputs

The controller generates a `node_ip.env` file from the node configuration and injects it into both `status.sh` and `run.sh`:

| Variable | Source | Notes |
| :--- | :--- | :--- |
| `NODE_NAME` | `node.name` | Used in messages only. |
| `NODE_INTERFACE` | `node.interface` | Must match the interface whitelist. |
| `NODE_IP` | `node.ip` | CIDR suffixes are stripped before use. |
| `NODE_IP6` | `node.ip6` | CIDR suffixes are stripped before use. |
| `NODE_EXTERNAL_IP` | `node.external_ip` | Added only when different from `NODE_IP`. |
| `NODE_EXTERNAL_IP6` | `node.external_ip6` | Added only when different from `NODE_IP6`. |
| `HAS_CUSTOM_SCRIPT` | `node.custom_network_script` | `1` when a non-empty custom script is configured, otherwise `0`. |

If a custom script is configured, it is uploaded as `custom_network.sh` and embedded into the generated network hook.

### Interface whitelist

For safety, the task only accepts interfaces matching:

```
lo       dummy*   eth*     ens*     enp*     eno*     wl*      wlan*
```

Any other interface produces `Not Supported` / exit code `20` during status checks and exit code `20` during execution. The task refuses to run when the node has no addresses and no custom script.

### Address handling

- CIDR masks are stripped from configured addresses and re-applied as `/32` (IPv4) or `/128` (IPv6).
- Main and external addresses are deduplicated.
- For `lo`, the task additionally ensures `127.0.0.1/8` is present.
- For `dummy*`, the dummy kernel module is loaded and the interface is created/raised when missing.
- After applying, the task verifies every desired address is present on the host and exits non-zero if verification fails.

### Network manager selection

The task probes native network managers in this order:

1. **netplan** — `netplan` exists and `/etc/netplan` is present.
2. **interfaces** — `/etc/network/interfaces` exists, or the OS is Alpine.
3. **systemd-networkd** — `/etc/systemd/network` exists and `systemd-networkd` is running or enabled.
4. **OpenWrt UCI** — `/etc/config/network` exists and `uci` is available.
5. **systemd-networkd fallback** — any systemd/systemctl host.
6. Otherwise the task fails with exit code `1`.

Persistence by backend:

| Backend | Persistent configuration |
| :--- | :--- |
| netplan | `/etc/netplan/99-easy42.yaml` (`renderer: networkd`), validated with `netplan generate`, then `netplan apply`. |
| interfaces | `### EASY42_START` / `### EASY42_END` block in `/etc/network/interfaces`, replacing any previous Easy42 block. |
| systemd-networkd | `99-easy42-<iface>.netdev` (dummy, non-`lo`) and `99-easy42-<iface>.network` in `/etc/systemd/network`, followed by `networkctl reload`. |
| OpenWrt | `network.easy42_node` UCI interface with `proto=static`, committed and applied via `/etc/init.d/network restart`. |

Existing managed files are backed up first with a timestamped suffix:

```
<file>.easy42.bak.YYYYMMDDHHMMSS
```

### Custom network script

`node.custom_network_script` lets operators append arbitrary commands (for example policy-routing or mark rules) that run when the target interface becomes routable.

- **systemd hosts**: written to `/etc/networkd-dispatcher/routable.d/99-easy42.sh`, guarded by an interface match, plus a `networkd-dispatcher.service` drop-in at `/etc/systemd/system/networkd-dispatcher.service.d/override.conf`. The package is installed automatically when possible.
- **interfaces-based hosts / Alpine**: written to `/etc/network/easy42.sh` and referenced from the generated interface block with `up /etc/network/easy42.sh`.
- Scripts are wrapped in `### EASY42_CUSTOM_START` / `### EASY42_CUSTOM_END` markers. Removing the custom script from the node configuration removes the generated dispatcher/hook on the next run.
- Status checks compare the marker contents with the desired script and report drift instead of silently accepting an outdated script.

### Status semantics

`configure_network` reports `done` when all desired addresses already exist on **any** interface and the custom script (if any) matches the expected markers and target interface. It reports `ready` with a human-readable list of missing addresses and/or script problems otherwise.

---

## Status Protocol & State Machine

Each task's `status.sh` script executes a series of non-destructive checks and terminates with standardized exit codes that map to UI status badges:

```
                  ┌───────────────────────────────┐
                  │      Check Task Status        │
                  │       (status.sh)             │
                  └──────────────┬────────────────┘
                                 │
         ┌───────────────────────┼───────────────────────┐
         │ Exit 0                │ Exit 10               │ Exit 20
         ▼                       ▼                       ▼
┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐
│ Needs Execution  │    │Already Configured│    │  Not Applicable  │
│     (ready)      │    │      (done)      │    │  (incompatible)  │
└────────┬─────────┘    └──────────────────┘    └──────────────────┘
         │
         │ Run Task (run.sh)
         ▼
┌──────────────────┐
│   Exit 0: OK     │
│ Exit != 0: Error │
└──────────────────┘
```

| Exit Code | Status Code | UI Badge | Description |
| :---: | :--- | :--- | :--- |
| **`0`** | `ready` | **Needs Execution** | The target host is compatible and missing the configuration or package. Ready to run. |
| **`10`** | `done` | **Already Configured** | The package, file include, service, sysctl tuning, addresses, or custom network script are already present and active. |
| **`20`** | `incompatible` | **Not Applicable** | The OS distribution, init system, interface name, or node configuration does not match the task prerequisites. |
| **`Other`** | `error` | **Check Failed** | Script execution failed, SSH connection dropped, or root elevation failed. |

When a task is executed (`run.sh`), an exit code of `0` denotes success, while any non-zero exit code denotes failure. The full stdout and stderr output stream is captured, returned in the API response, and displayed in the UI terminal log viewer. For `deploy_agent` and `configure_network`, a successful run also triggers an asynchronous node status refresh.

---

## Multi-Platform Support

All helper tasks leverage a unified POSIX-compliant helper library (`tasks/_common/common.sh`), supporting heterogeneous Linux environments:

### Supported Operating Systems
- **Debian / Ubuntu / Raspbian / Armbian**: `apt-get` with `DEBIAN_FRONTEND=noninteractive`.
- **Alpine Linux**: `apk update && apk add`.
- **Arch Linux / Manjaro**: `pacman -Sy --noconfirm`.
- **CentOS / RHEL / Rocky / Alma / Fedora**: `dnf` or `yum`.
- **OpenWrt**: `opkg update && opkg install`.

### Supported Init Systems
- **systemd**: Detected via `/run/systemd/system` (or a `systemctl` fallback). Supports unit generation under `/etc/systemd/system/`, `systemctl daemon-reload`, enable/start/restart, and `networkctl reload`.
- **OpenRC**: Detected via `/run/openrc`, `/run/openrc/softlevel`, or `rc-service`/`rc-update`. Supports `/etc/init.d/` scripts and runlevel management via `rc-update add <service> default`.
- **procd (OpenWrt)**: Detected via `/etc/rc.common`, `/sbin/procd`, or `OS_ID=openwrt`. Generates procd-compliant `/etc/init.d/` startup scripts and calls `/etc/init.d/<service> enable`.

---

## Using Device Helper in the Web UI

Device Helper is integrated directly into the Easy42 Web UI:

### 1. Navigating to Device Helper
- **Global Header**: Click **Helper** in the top navigation bar (route `/helper`).
- **Node Detail Page**: Click the **Device Helper** button in the header actions bar of any managed node (`/helper?node=<node_name>`).
- **Direct Task Links / URL Routing**: Each task is routable via URL query parameters (`/helper?task=<task_id>` or `/helper?node=<node_name>&task=<task_id>`). Selecting a task syncs the selected task ID in the address bar for shareable deep links.

### 2. Fleet Tag Filtering & Selection
- **Tag Filter**: Use the tag selector dropdown to filter devices by node tags (`#core`, `#edge`, `#cloud`, `#eu`).
- **Batch Selection**: Use the table header checkbox to select or deselect all nodes matching the active tag filter.
- **Individual Selection**: Toggle specific devices using row checkboxes.

### 3. Checking Prerequisites & Drift Detection
- Click **Check Status** to inspect the selected task on all checked devices simultaneously.
- Status badges update in real time (`Already Configured`, `Needs Execution`, `Not Applicable`, or `Check Failed`).
- Hover over the **Details** column to view exact check messages, timestamps, and exit codes.
- Click **Auto Select** (enabled once statuses are fetched) to automatically select only the nodes whose status is `Needs Execution` and deselect all others for targeted remediation.

### 4. Executing Tasks & Viewing Real-Time Logs
- Click **Execute Task (N)** to run the task across all selected devices, or click the individual **Play** icon on a specific device row.
- Device Helper automatically opens the **Execution Output** terminal view at the bottom of the page.
- Select device tabs (`node-lon`, `node-fra`) to review full console stdout/stderr logs and execution durations.
- Once a task finishes successfully, Device Helper automatically triggers a re-check to ensure the node transitions to `Already Configured`.

---

## REST API Reference

All Device Helper actions are available under the authenticated `/api` router. Unless a reverse proxy or the API client library adds a prefix, the paths below are relative to the Easy42 server root.

### 1. List Available Tasks
```http
GET /api/tasks
```
**Response** (`200 OK`):
```json
[
  {
    "id": "deploy_agent",
    "title": "Deploy Agent",
    "description": "Deploys or upgrades the bundled Easy42 node agent binary and system service unit.",
    "category": "Services",
    "weight": 5
  },
  {
    "id": "configure_network",
    "title": "Configure Network",
    "description": "Configures permanent node IP addresses and custom network rules (e.g. policy routing) using native network managers (systemd-networkd/networkd-dispatcher, /etc/network/interfaces).",
    "category": "System & Network",
    "weight": 6
  },
  {
    "id": "install_wireguard",
    "title": "Install WireGuard",
    "description": "Installs WireGuard and wireguard-tools on the device using the native package manager.",
    "category": "Packages",
    "weight": 10
  }
]
```

### 2. Read Task Source Files
```http
GET /api/tasks/{id}/source
GET /api/tasks/{id}/sources
```
Both routes return the same payload: the task's own embedded files followed by shared files from `tasks/_common`.

**Response** (`200 OK`):
```json
[
  {
    "name": "run.sh",
    "path": "configure_network/run.sh",
    "content": "#!/bin/sh\n...",
    "shared": false
  },
  {
    "name": "status.sh",
    "path": "configure_network/status.sh",
    "content": "#!/bin/sh\n...",
    "shared": false
  },
  {
    "name": "task.json",
    "path": "configure_network/task.json",
    "content": "{\n  \"id\": \"configure_network\",\n...",
    "shared": false
  },
  {
    "name": "common.sh",
    "path": "_common/common.sh",
    "content": "#!/bin/sh\n...",
    "shared": true
  }
]
```

Task files are ordered as `run.sh`, `status.sh`, `source.sh`, other `.sh` files, then remaining files. Shared files are appended and marked with `"shared": true`. An unknown task ID returns `400 Bad Request`.

### 3. Check Task Status on Nodes
```http
POST /api/tasks/{id}/status
Content-Type: application/json

{
  "nodes": ["node-fra", "node-lon"]
}
```
*Note: If `"nodes"` is empty or omitted, status is checked on all managed nodes. External nodes are always excluded.*

**Response** (`200 OK`):
```json
{
  "node-fra": {
    "task_id": "sysctl_params",
    "node_name": "node-fra",
    "status": "done",
    "message": "Sysctl parameters are already configured and active",
    "exit_code": 10,
    "duration_ms": 342
  },
  "node-lon": {
    "task_id": "sysctl_params",
    "node_name": "node-lon",
    "status": "ready",
    "message": "Sysctl config missing or incomplete",
    "exit_code": 0,
    "duration_ms": 289
  }
}
```

`status` is one of `ready`, `done`, `incompatible`, or `error`. `exit_code` is `-1` when the SSH connection itself failed.

### 4. Execute Task on Nodes
```http
POST /api/tasks/{id}/run
Content-Type: application/json

{
  "nodes": ["node-lon"]
}
```
The `nodes` array is **required** for execution; an empty list returns `400 Bad Request` (`no valid target nodes specified`). External nodes and unknown node names are ignored.

Before running, the controller derives the agent callback URL from the request (`X-Forwarded-Proto` is honored, and the host must pass the server's safe-host check). This URL is embedded in `agent.json` for `deploy_agent`.

**Response** (`200 OK`):
```json
{
  "node-lon": {
    "task_id": "sysctl_params",
    "node_name": "node-lon",
    "success": true,
    "output": "Applied /etc/sysctl.d/99-easy42.conf successfully.\nnet.ipv4.ip_forward = 1\nnet.ipv6.conf.all.forwarding = 1\nnet.ipv4.conf.all.rp_filter = 0\nnet.ipv4.conf.default.rp_filter = 0",
    "exit_code": 0,
    "duration_ms": 612
  }
}
```

---

## Authoring Custom Tasks

Tasks are compiled into the binary with `//go:embed all:*`, so a new task is picked up only after rebuilding Easy42. To add one, create a new subdirectory under `tasks/` containing three files:

```
tasks/
├── _common/
│   └── common.sh
├── my_custom_task/
│   ├── task.json
│   ├── status.sh
│   └── run.sh
```

Directories whose names begin with `_` are skipped. Directories without a valid `task.json` are ignored.

### 1. Task Manifest (`task.json`)
```json
{
  "id": "my_custom_task",
  "title": "Setup Custom MTU Daemon",
  "description": "Installs and tunes custom interface MTU optimization service.",
  "category": "System & Network",
  "weight": 60
}
```

`id` is used in API paths and must be unique. `weight` controls ordering (lower runs earlier in the UI).

### 2. Status Script (`status.sh`)
```bash
#!/bin/sh
. "$(dirname "$0")/common.sh"

if [ -f /etc/my-mtu-daemon.conf ]; then
    echo "Custom MTU daemon is already installed"
    exit 10  # done
fi

echo "Custom MTU daemon is not installed"
exit 0  # ready
```

### 3. Execution Script (`run.sh`)
```bash
#!/bin/sh
. "$(dirname "$0")/common.sh"

echo "Deploying custom MTU configuration..."
cat << 'EOF' | $SUDO tee /etc/my-mtu-daemon.conf >/dev/null
# Generated by Easy42 Device Helper
ENABLED=1
EOF

echo "Done."
exit 0
```

`common.sh` provides `$SUDO`, OS/init detection (`OS_ID`, `INIT_SYSTEM`, `has_systemd`, `has_openrc`, `has_procd`), service helpers (`is_service_installed`, `is_service_enabled`, `is_service_running`, `enable_service`, `start_service`, `restart_service`, `reload_service`), `install_packages`, and timestamped `backup_file`. Note that it enables `set -e`, so custom scripts should handle expected non-zero commands explicitly.

---

## Filesystem Custom Tasks

In addition to built-in embedded tasks, Easy42 allows operators to define **custom tasks** directly on the server's filesystem without rebuilding the binary.

Custom tasks are stored in:
```
<easy42-data-dir>/tasks/<task_id>/
```
*(Default: `~/.config/easy42/tasks/<task_id>/`, or overridden by the `--data-dir` flag)*

### Directory Structure & File Specifications

```
~/.config/easy42/tasks/
└── <task_id>/
    ├── run.sh          # [Required, mode 0755] The execution script
    ├── status.sh       # [Optional, mode 0755] Probe script (0=Ready, 10=Done, 20=Incompatible)
    └── task.json       # [Optional] Metadata (title, description, category, weight)
```

1. **`run.sh`** (*Required*): Executable shell script run natively on target nodes as root or with sudo. Must exit `0` on success or non-zero on error.
2. **`status.sh`** (*Optional*): Probes remote node state. If omitted, Device Helper automatically marks the task as **Ready to Execute** (`0`).
3. **`task.json`** (*Optional*): Metadata defining `title`, `description`, `category`, and `weight`. If omitted, the title is automatically derived from the directory name (e.g. `install_bird` becomes `"Install Bird"`), category defaults to `"Custom"`, and weight defaults to `500`.
4. **Access to `common.sh`**:
   Custom tasks automatically have access to Easy42's shared `common.sh` helper library in their working directory:
   ```sh
   SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
   [ -f "$SCRIPT_DIR/common.sh" ] && . "$SCRIPT_DIR/common.sh"
   ```

### UI Integration

- **Separated Sidebar Section**: In the Device Helper Web UI, custom tasks are grouped in their own **Custom Tasks** section below the built-in tasks.
- **"+ Task" Guide & AI Prompt Page**: A dedicated "+ Task" item provides step-by-step instructions and an interactive **Ready-to-Use AI Prompt Generator** that can be copied into any LLM (ChatGPT, Claude, Gemini, DeepSeek) to generate production-ready `run.sh` and `status.sh` scripts.
- **View Source**: Operators can click "View Source" on any custom task to inspect its scripts and the shared `common.sh`.

### Payload-based tasks

Tasks that need controller-generated files can be supported by extending the execution engine, which already special-cases `deploy_agent` and `configure_network`. Uploaded payload files are written into the same temporary directory as the scripts; only `easy42-agent`, `status.sh`, and `run.sh` receive `0755`, `agent.json` receives `0600`, and other files default to `0644`. To generate dynamic status checks, override `status.sh` in the payload as the built-in payload tasks do.

---

## Online Tasks Repository

The **Online Tasks Repository** allows operators to discover, download, install, and update custom tasks directly from online sources into their local Easy42 instance via the Web UI. Installed tasks are written directly into `<easy42-data-dir>/tasks/<task_id>/` and immediately become available in the Device Helper task list alongside built-in tasks.

### Default Repository Source

Easy42 includes a built-in default repository pointing to the official community tasks catalog:
```
https://raw.githubusercontent.com/sagan/easy42-tasks/refs/heads/master/
```

The Easy42 backend proxies all requests to online sources to bypass browser Cross-Origin Resource Sharing (CORS) restrictions.

### Custom Online Sources (`sources.txt`)

Operators can configure custom or private online task sources (e.g. internal GitHub/GitLab repositories, enterprise artifact mirrors, or raw HTTP file servers).

To add custom sources, create a text file:
```
<easy42-data-dir>/tasks/sources.txt
```
*(Default: `~/.config/easy42/tasks/sources.txt`)*

Add one base URL per line:
```text
# Internal company tasks repository
https://raw.githubusercontent.com/my-org/easy42-internal-tasks/main/

# Local network testing repository
http://192.168.1.50:8080/easy42-tasks/
```

- Blank lines and lines starting with `#` are ignored.
- The default official repository is always included at the top of the sources list.
- Operators can switch between configured sources dynamically via the dropdown in the Web UI.

### Repository Layout & Manifest Specification (`manifest.json`)

An online tasks repository must host a `manifest.json` file at its root. Each task's files reside in a directory named after its `id`:

```
<repository-root>/
├── manifest.json
└── <task-id>/
    ├── run.sh
    ├── status.sh
    └── task.json
```

#### Manifest Schema

```json
{
  "name": "Community Tasks Catalog",
  "description": "Curated collection of host automation tasks for Easy42",
  "version": "1.0",
  "tasks": [
    {
      "id": "install_bind",
      "title": "Install BIND DNS server",
      "description": "Installs and enables BIND 9 DNS server daemon across Linux distributions.",
      "category": "Packages",
      "weight": 20,
      "files": ["run.sh", "status.sh", "task.json"]
    },
    {
      "id": "setup_prometheus_node_exporter",
      "title": "Setup Node Exporter",
      "description": "Installs and enables Prometheus Node Exporter system service.",
      "category": "Monitoring",
      "weight": 80,
      "files": ["run.sh", "status.sh"]
    }
  ]
}
```

- `id`: Unique task identifier (alphanumeric, dashes, underscores). Path traversal characters (`..`, `/`, `\`) are strictly forbidden.
- `title`: Human-readable task name displayed in the UI.
- `description`: Detailed description of the task's actions.
- `category`: Grouping category (e.g. `Packages`, `Configuration`, `System & Network`, `Monitoring`).
- `weight`: Display order weight (lower numbers appear earlier).
- `files`: Array of relative filenames to download into `<data-dir>/tasks/<task-id>/`. Must include `run.sh`.

### Installation & Atomicity Guarantees

When a user initiates task installation:
1. **Validation**: Easy42 validates task identifiers and filenames to prevent path traversal attacks.
2. **Atomic Staging**: Files are downloaded to an ephemeral temporary directory: `<data-dir>/tasks/.download-<task-id>-<randHex>/`.
3. **Permissions**: Shell scripts (`.sh`) are automatically given executable permissions (`0755`), while configuration and manifest files receive `0644`.
4. **Manifest Synchronization**: If `task.json` is not provided in the download file list, Easy42 generates one automatically using the manifest's `title`, `description`, `category`, and `weight`.
5. **Atomic Promotion**: Once all files are downloaded successfully, any existing directory at `<data-dir>/tasks/<task-id>/` is cleanly replaced with the newly downloaded directory. If download fails, the temporary folder is deleted and the existing task remains intact.

### Deleting Custom & Online-Installed Tasks

Online-installed and custom filesystem tasks can be deleted directly from the Web UI:
- In the task header, custom tasks feature a **Delete Task** button.
- A confirmation dialog ensures accidental deletions are prevented.
- Deletions are securely restricted to directories within `<data-dir>/tasks/`. Built-in embedded tasks cannot be deleted.

### REST API Endpoints

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/tasks/online/sources` | Returns the list of available online sources (default + `sources.txt`). |
| `GET` | `/api/tasks/online/manifest?source=<url>` | Fetches and returns the parsed `manifest.json` from the specified source. |
| `POST` | `/api/tasks/online/install` | Downloads and installs a task. Body: `{"source": "<url>", "task": {...}}`. |
| `DELETE` | `/api/tasks/custom/{id}` | Deletes a custom task directory from the server's filesystem. |

