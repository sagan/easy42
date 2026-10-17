# Operations, Bootstrap Tasks & Troubleshooting

This document covers operational verification, troubleshooting tools, device bootstrap automation, and solutions for common network gotchas.

---

## Operational Verification

### 1. Verifying WireGuard Interfaces

```bash
# Check interface status, latest handshake timestamps, and transfer bytes
wg show

# Verify that Table = off kept the kernel routing table clean
ip route show dev wg42lon

# Verify IPv6 and IPv4 link-local address assignment
ip -6 addr show dev wg42lon
ip -4 addr show dev wg42lon
```

### 2. Inspecting BIRD 2 Dynamic Routing

```bash
# Check status of BGP peering sessions and BFD states
birdc show protocols
birdc show bfd sessions

# View all learned BGP routes, active preferences, and paths
birdc show route all

# Check routes exported from BIRD to the Linux kernel FIB
birdc show route export kernel_v4
birdc show route export kernel_v6
```

### 3. Debugging nftables Rules

```bash
# View active ruleset in the easy42 table
nft list table inet easy42

# Check packet and byte counters on the filter chains
nft list chain inet easy42 filter_input
nft list chain inet easy42 filter_forward
```

### 4. Inspecting Node Agent Logs

```bash
# Systemd (Debian, Ubuntu, CentOS, Arch)
journalctl -u easy42-agent -f

# OpenRC (Alpine Linux)
rc-service easy42-agent status

# OpenWrt procd
logread -e easy42-agent
```

---

## Device Helper (Bootstrap & Host Automation)

Easy42 includes an embedded task execution engine to bootstrap prerequisites, install software packages, tune kernel sysctl parameters, and configure daemons on remote nodes over SSH without requiring external automation tools (such as Ansible or Terraform):

| Task ID | Category | Description |
| :--- | :--- | :--- |
| `deploy_agent` | Services | Automatically detects remote architecture, deploys/upgrades the bundled `easy42-agent` binary, provisions token configs, and starts the system service unit (`systemd`, `openrc`, `procd`). |
| `install_wireguard` | Packages | Installs `wireguard` and `wireguard-tools` across all supported Linux distributions (Debian, Ubuntu, Alpine, Arch, RHEL/CentOS, OpenWrt). |
| `install_bird` | Packages | Installs BIRD 2 and enables the system service. |
| `config_bird` | Configuration | Backs up `/etc/bird/bird.conf`, inserts `include "/etc/bird_easy42.conf";`, validates syntax with `bird -p`, and reloads `birdc`. |
| `config_nftables` | Configuration | Configures `/etc/nftables.conf` (or OpenWrt / Alpine init systems) to include `/etc/easy42.nft` and enables the service. |
| `sysctl_params` | System & Network | Deploys `/etc/sysctl.d/99-easy42.conf` enabling IPv4/IPv6 forwarding and setting `rp_filter=0`. |
| `autostart_interfaces` | System & Network | Installs systemd, OpenRC, or procd units to bring up `wg42*` interfaces on boot. |

Inspect, verify, and execute these tasks directly from the **Device Helper** page (`/helper`) in the top navigation bar, or via the **Device Helper** button in any node's detail view. You can filter devices by tag (`#core`, `#edge`), batch check prerequisites, run tasks, and view live execution logs.

For comprehensive task details, state machine exit codes, and REST API usage, see **[Device Helper & Remote Host Automation](device-helper.md)**.

---

## Common Gotchas & Fixes

### 1. Asymmetric Routing Packet Drops

- **Symptom**: WireGuard handshakes succeed, but TCP connections stall or `ping` fails in one direction.
- **Root Cause**: Linux enables strict Reverse Path Filtering (`rp_filter=1`) by default. In a multi-path dynamic mesh, return traffic frequently arrives via a different interface than outbound traffic, causing the kernel to drop legitimate packets.
- **Fix**: Set Reverse Path Filtering to loose (`2`) or disabled (`0`):
  ```bash
  sysctl -w net.ipv4.conf.all.rp_filter=0
  sysctl -w net.ipv4.conf.default.rp_filter=0
  sysctl -w net.ipv4.conf.wg42*.rp_filter=0
  ```
  Persist this across reboots by executing the **sysctl_params** task in **[Device Helper](device-helper.md)** (or manually writing `/etc/sysctl.d/99-easy42.conf`).

### 2. Missing IPv6 Packet Forwarding

- **Symptom**: IPv4 routes and traffic work correctly across nodes, but IPv6 fails to transit.
- **Fix**: Ensure IPv6 forwarding is enabled in the Linux kernel:
  ```bash
  sysctl -w net.ipv6.conf.all.forwarding=1
  ```

### 3. SSH Authentication & Host Key Verification Failures

- **Symptom**: Controller reports `ssh: handshake failed: knownhosts: key is unknown`, `known_hosts mismatch`, or `no SSH authentication methods found`.
- **Root Cause & Security Model**: Easy42's SSH node management and Device Helper features **directly and exclusively use the current system OS user's OpenSSH files** for authentication and server host key verification:
  - **Authentication**: `~/.ssh/id_ed25519` (with fallback to `~/.ssh/id_rsa`, `~/.ssh/id_ecdsa`, or an active SSH Agent via `$SSH_AUTH_SOCK`). Easy42 does not store or manage SSH passwords or separate SSH private keys.
  - **Host Key Verification**: `~/.ssh/known_hosts`. Easy42 enforces strict host key verification. Connections to unknown or mismatched host keys are rejected immediately to protect against man-in-the-middle attacks.
- **Fix**:
  1. **Authorize Public Key**: Ensure the public key of the controller system user (`~/.ssh/id_ed25519.pub`) is installed into the remote host's `~/.ssh/authorized_keys`:
     ```bash
     ssh-copy-id -i ~/.ssh/id_ed25519.pub root@<node-host>
     ```
  2. **Trust Remote Host Key**: Connect once manually or scan and append the remote host's key to `~/.ssh/known_hosts`:
     ```bash
     ssh-keyscan -H <node-host> >> ~/.ssh/known_hosts
     ```
  3. **Service User Context**: If running Easy42 as a systemd service (e.g., under user `root`), ensure these files exist in that specific user's home directory (`/root/.ssh/id_ed25519` and `/root/.ssh/known_hosts`).

### 4. Agent WebSocket Disconnections Behind Reverse Proxies

- **Symptom**: `easy42-agent` fails to connect or disconnects periodically when routed through Nginx, Caddy, or Cloudflare.
- **Fix**: Ensure `Upgrade` and `Connection` headers are forwarded in your reverse proxy config:
  ```nginx
  location /api/agent/ws {
      proxy_pass http://127.0.0.1:4242;
      proxy_http_version 1.1;
      proxy_set_header Upgrade $http_upgrade;
      proxy_set_header Connection "upgrade";
      proxy_read_timeout 86400s;
      proxy_send_timeout 86400s;
  }
  ```

### 5. MTU Blackholing on External Transit

- **Symptom**: WireGuard handshakes succeed and ICMP pings work, but HTTPS or large file transfers hang.
- **Fix**: Verify that nftables MSS clamping is active in `table inet easy42`:
  ```bash
  nft list chain inet easy42 mangle_postrouting
  ```
  Ensure the rule `tcp flags syn tcp option maxseg size set rt mtu` is present.
