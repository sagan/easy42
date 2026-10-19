# Device Helper & Remote Host Automation

**Device Helper** is Easy42's integrated remote provisioning and host lifecycle automation subsystem. It enables operators to verify prerequisites, install packages, tune kernel parameters, configure daemon integration points, and deploy services across all managed nodes in a cluster without requiring external configuration management tooling such as Ansible, Puppet, Salt, or Terraform.

---

## Overview & Architecture

When deploying dynamic WireGuard overlay networks with BIRD 2 routing, target Linux hosts typically require foundational system preparation:
- Installing WireGuard kernel module/tools and BIRD 2 routing daemon.
- Enabling IPv4/IPv6 packet forwarding and configuring reverse path filtering (`rp_filter`) to prevent asymmetric routing drops.
- Injecting Easy42 configuration includes into `/etc/bird/bird.conf` and `/etc/nftables.conf`.
- Installing interface autostart boot units (`wg42*`).
- Provisioning and starting the `easy42-agent` binary and system service.

Device Helper packages these operational workflows into **embedded tasks**. Each task contains non-destructive status inspection scripts (`status.sh`) and idempotent execution scripts (`run.sh`) that run natively on target nodes.

```
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                         easy42 Controller / Web UI                          │
 │                                                                             │
 │    ┌───────────────────────────┐         ┌─────────────────────────────┐    │
 │    │ Device Helper UI (/helper)│         │   REST API (/api/v1/tasks)  │    │
 │    └─────────────┬─────────────┘         └──────────────┬──────────────┘    │
 └──────────────────┼──────────────────────────────────────┼───────────────────┘
                    │                                      │
                    ▼                                      ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                      Task Execution Engine (tasks.go)                       │
 │    - Dynamic payload injection (agent binaries, agent.json)                 │
 │    - Ephemeral SFTP upload to /tmp/.easy42_task_<id>_<rand>                 │
 │    - SSH remote execution with sudo privilege detection                     │
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

1. **Zero External Dependencies**: All scripts and manifests are embedded directly into the Easy42 binary via Go's `embed.FS`.
2. **Ephemeral & Safe Execution**: Scripts are uploaded via SFTP to a uniquely named remote directory (`/tmp/.easy42_task_<task_id>_<randHex>`) with `0755` permissions, executed over SSH, and immediately deleted via deferred cleanup handlers.
3. **Privilege Elevation**: Automatically detects whether the SSH session runs as `root` or an unprivileged user, prefixing commands with `sudo` when necessary.
4. **Targeted Fleet Filtering**: Nodes can be filtered by user-defined tags (`#cloud`, `#edge`, `#eu`, etc.) or selected individually for precise, progressive rollouts.
5. **Non-Destructive Status Inspection**: Before applying any change, Device Helper queries target nodes to determine whether a task is already satisfied, ready to apply, or incompatible.

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
| `deploy_agent` | Services | **Deploy Agent** | `5` | Detects remote CPU architecture (`x86_64`, `aarch64`, etc.), generates node agent token, uploads bundled `easy42-agent` binary and `agent.json`, creates and enables system service (`systemd`, `openrc`, or `procd`), and refreshes node status. |
| `install_wireguard` | Packages | **Install WireGuard** | `10` | Installs `wireguard` and `wireguard-tools` using native package managers (`apt`, `apk`, `pacman`, `dnf`/`yum`, `opkg`). |
| `install_bird` | Packages | **Install BIRD** | `20` | Installs BIRD 2 (`bird2` or `bird`) via native package manager and enables the system service. |
| `config_bird` | Configuration | **Configure BIRD Main Config** | `30` | Validates `/etc/bird/bird.conf` (or `/etc/bird.conf`), creates a fallback placeholder `/etc/bird_easy42.conf`, appends `include "/etc/bird_easy42.conf";`, validates configuration syntax with `bird -p`, and issues `birdc configure` if the daemon is active. |
| `config_nftables` | Configuration | **Configure Nftables Autostart** | `35` | Creates placeholder `/etc/easy42.nft`, hooks `include "/etc/easy42.nft"` into `/etc/nftables.conf`, or installs the standalone `easy42-nftables` service unit on systems without standard nftables loaders. |
| `sysctl_params` | System & Network | **Configure Sysctl Parameters** | `40` | Deploys `/etc/sysctl.d/99-easy42.conf` enabling `net.ipv4.ip_forward=1`, `net.ipv6.conf.all.forwarding=1`, and sets loose reverse path filtering (`net.ipv4.conf.all.rp_filter=0`, `net.ipv4.conf.default.rp_filter=0`) to ensure asymmetric dynamic routing functions without kernel packet drops. |
| `autostart_interfaces` | System & Network | **WireGuard Autostart Service** | `50` | Generates and enables an autostart daemon (`easy42-autostart`) under `systemd`, `openrc`, or `procd` to bring up all `wg42*` interfaces automatically upon system reboot. |

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
| **`10`** | `done` | **Already Configured** | The package, file include, service, or sysctl tuning is already present and active. No action needed. |
| **`20`** | `incompatible` | **Not Applicable** | The OS distribution or init system does not match the task prerequisites. |
| **`Other`** | `error` | **Check Failed** | Script execution failed, SSH connection dropped, or root elevation failed. |

When a task is executed (`run.sh`), an exit code of `0` denotes success, while any non-zero exit code denotes failure. The full stdout and stderr output stream is captured, stored, and displayed in the UI terminal log viewer.

---

## Multi-Platform Support

All helper tasks leverage a unified POSIX-compliant helper library (`tasks/_common/common.sh`), supporting heterogeneous Linux environments:

### Supported Operating Systems
- **Debian / Ubuntu**: Uses `apt-get` with non-interactive frontend flags (`DEBIAN_FRONTEND=noninteractive`).
- **Alpine Linux**: Uses `apk` with `--no-cache`.
- **Arch Linux**: Uses `pacman -Sy --noconfirm --needed`.
- **CentOS / RHEL / Rocky / Alma / Fedora**: Uses `dnf` or `yum`.
- **OpenWrt**: Uses `opkg update && opkg install`.

### Supported Init Systems
- **systemd**: Full support for `systemctl daemon-reload`, unit file generation (`/etc/systemd/system/`), and timer/service enablement.
- **OpenRC**: Supports `/etc/init.d/` service scripts and runlevel management via `rc-update add <service> default`.
- **procd (OpenWrt)**: Generates procd-compliant `/etc/init.d/` startup scripts with `START=90` priority and calls `/etc/init.d/<service> enable`.

---

## Using Device Helper in the Web UI

Device Helper is integrated directly into the Easy42 Web UI:

### 1. Navigating to Device Helper
- **Global Header**: Click **Device Helper** in the top navigation bar (route `/helper`).
- **Node Detail Page**: Click the **Device Helper** button in the header actions bar of any managed node (`/helper?node=<node_name>`).
- **Direct Task Links / URL Routing**: Each task in the list is fully routable via URL query parameters (`/helper?task=<task_id>` or `/helper?node=<node_name>&task=<task_id>`). Selecting any task dynamically syncs and persists the selected task ID in the address bar for shareable deep links.

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

All Device Helper actions can be automated via Easy42's HTTP REST API:

### 1. List Available Tasks
```http
GET /api/v1/tasks
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
    "id": "install_wireguard",
    "title": "Install WireGuard",
    "description": "Installs WireGuard and wireguard-tools on the device using the native package manager.",
    "category": "Packages",
    "weight": 10
  },
  {
    "id": "sysctl_params",
    "title": "Configure Sysctl Parameters",
    "description": "Configures IP forwarding and loose reverse path filtering (rp_filter=0) in /etc/sysctl.d/99-easy42.conf for asymmetric routing.",
    "category": "System & Network",
    "weight": 40
  }
]
```

### 2. Check Task Status on Nodes
```http
POST /api/v1/tasks/{id}/status
Content-Type: application/json

{
  "nodes": ["node-fra", "node-lon"]
}
```
*Note: If `"nodes"` is empty or omitted, status is checked on all managed nodes.*

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

### 3. Execute Task on Nodes
```http
POST /api/v1/tasks/{id}/run
Content-Type: application/json

{
  "nodes": ["node-lon"]
}
```

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

To add custom helper tasks to Easy42, create a new subdirectory under `tasks/` containing three files:

```
tasks/
├── _common/
│   └── common.sh
├── my_custom_task/
│   ├── task.json
│   ├── status.sh
│   └── run.sh
```

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

### 2. Status Script (`status.sh`)
```bash
#!/bin/sh
. "$(dirname "$0")/common.sh"

if [ -f /etc/my-mtu-daemon.conf ]; then
    echo "Custom MTU daemon is already installed"
    exit 10  # done
fi

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
