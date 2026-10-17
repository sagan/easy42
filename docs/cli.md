# Command-Line Interface (CLI) Reference

Easy42 includes a comprehensive command-line interface built with [Cobra](https://github.com/spf13/cobra). The CLI can be run interactively or automated within CI/CD pipelines.

---

## Cheat Sheet

Easy42 includes a full CLI for scripting and head-less environments:

```bash
# Node operations
easy42 node list
easy42 node probe user@vps-fra.example.com
easy42 node add --name node-fra --ip 192.168.100.1 --ip6 fd42::1 --asn 4224420001
easy42 node bird node-fra   # Preview generated BIRD config
easy42 node nft node-fra    # Preview generated nftables rules

# Link operations
easy42 link list
easy42 link add node-fra node-lon
easy42 link add node-fra node-lon --assign-ipv4 --psk "<base64_psk>"

# State inspection and synchronization
easy42 status --live        # Real-time tunnel status & byte counters
easy42 sync --dry-run       # Preview diffs across all nodes
easy42 sync                 # Apply configuration across all nodes
easy42 sync --node node-fra # Synchronize a single node
```

## Global Flags

All commands accept the following persistent flag:

- `-d, --data-dir <path>`: Specifies the directory where `config.json`, the encrypted key store, and `easy42.db` are kept. Defaults to `~/.config/easy42`.

---

## Command Reference

### 1. `easy42 serve`

Starts the HTTP/WebSocket backend server and hosts the embedded Web UI.

```bash
# Listen on default 127.0.0.1:4242
easy42 serve

# Listen on a custom port
easy42 serve --listen 0.0.0.0:8080

# Bind to multiple IP addresses or interfaces simultaneously
easy42 serve --listen 127.0.0.1:4242,192.168.1.100:4242

# Specify custom data directory
easy42 serve --data-dir /var/lib/easy42
```

---

### 2. `easy42 node`

Manage nodes in the overlay mesh network.

#### `easy42 node list`

Lists all registered nodes, their connection mode (SSH or agent), overlay IPs, and ASNs.

```bash
easy42 node list
```

#### `easy42 node probe <host>`

Probes a remote host over SSH to inspect existing network interfaces, IP addresses, and routing tables.

```bash
easy42 node probe user@vps-fra.example.com
```

#### `easy42 node add`

Registers a new node in the network topology.

```bash
easy42 node add \
  --name "node-fra" \
  --host "vps-fra.example.com" \
  --mode "ssh" \
  --ip "192.168.100.1" \
  --ip6 "fd42:a159:f9f0::1" \
  --external-ip "172.20.150.1" \
  --interface "lo" \
  --asn 4224420001
```

#### `easy42 node rename <old-name> <new-name>`

Renames an existing node. Automatically updates all links, policies, and block memberships referencing the node.

```bash
easy42 node rename node-fra node-frankfurt
```

#### `easy42 node remove <name>`

Removes a node and tears down all connected links.

```bash
easy42 node remove node-frankfurt
```

#### `easy42 node bird <name>`

Compiles and previews the generated BIRD 2 routing configuration (`/etc/bird_easy42.conf`) for a node.

```bash
easy42 node bird node-frankfurt
```

#### `easy42 node nft <name>`

Compiles and previews the generated nftables firewall ruleset (`/etc/easy42.nft`) for a node.

```bash
easy42 node nft node-frankfurt
```

---

### 3. `easy42 link`

Manage point-to-point tunnels between nodes.

#### `easy42 link list`

Displays all configured links, interface names, link-local addresses, and costs.

```bash
easy42 link list
```

#### `easy42 link add <from-node> <to-node>`

Establishes a point-to-point WireGuard or manual link between two nodes.

```bash
# Standard WireGuard link
easy42 link add node-fra node-lon

# WireGuard link with deterministic IPv4 link-local (169.254.X.X/32)
easy42 link add node-fra node-lon --assign-ipv4

# WireGuard link with Preshared Key (PSK)
easy42 link add node-fra node-lon --psk "c2VjcmV0LWtleS12YWx1ZQ=="

# Manual link (over an existing GRE, SIT, or physical interface)
easy42 link add node-fra node-ext \
  --type manual \
  --from-iface "gre0" \
  --to-iface "gre0" \
  --from-addr "172.20.1.1" \
  --to-addr "172.20.1.2"
```

#### `easy42 link remove <from-node> <to-node>`

Deletes the link between two nodes.

```bash
easy42 link remove node-fra node-lon
```

---

### 4. `easy42 status`

Inspect topology health, daemon status, and live interface counters.

```bash
# View configuration and local node summaries
easy42 status

# Query live node statuses, active WireGuard handshakes, and transfer bytes
easy42 status --live
```

---

### 5. `easy42 sync`

Synchronizes compiled configuration state to managed nodes.

```bash
# Dry run: calculate and preview diffs across all nodes without applying
easy42 sync --dry-run

# Synchronize all nodes in parallel
easy42 sync

# Synchronize a specific target node only
easy42 sync --node node-frankfurt
```
