# Command-Line Interface (CLI) Reference

Easy42 includes a comprehensive command-line interface built with [Cobra](https://github.com/spf13/cobra). The CLI can be run interactively or automated within CI/CD pipelines.

---

- [Command-Line Interface (CLI) Reference](#command-line-interface-cli-reference)
  - [Cheat Sheet](#cheat-sheet)
  - [Global Flags](#global-flags)
  - [Command Reference](#command-reference)
    - [1. `easy42 serve`](#1-easy42-serve)
    - [2. `easy42 node`](#2-easy42-node)
      - [`easy42 node list`](#easy42-node-list)
      - [`easy42 node probe <host>`](#easy42-node-probe-host)
      - [`easy42 node add`](#easy42-node-add)
      - [`easy42 node rename <old-name> <new-name>`](#easy42-node-rename-old-name-new-name)
      - [`easy42 node remove <name>`](#easy42-node-remove-name)
      - [`easy42 node bird <name>`](#easy42-node-bird-name)
      - [`easy42 node nft <name>`](#easy42-node-nft-name)
    - [3. `easy42 link`](#3-easy42-link)
      - [`easy42 link list`](#easy42-link-list)
      - [`easy42 link add <from-node> <to-node>`](#easy42-link-add-from-node-to-node)
      - [`easy42 link remove <from-node> <to-node>`](#easy42-link-remove-from-node-to-node)
    - [4. `easy42 status`](#4-easy42-status)
    - [5. `easy42 sync`](#5-easy42-sync)
    - [6. `easy42 apply`](#6-easy42-apply)
    - [7. `easy42 version`](#7-easy42-version)
  - [Common Workflows](#common-workflows)
    - [Bootstrap an SSH-managed mesh](#bootstrap-an-ssh-managed-mesh)
    - [Apply a declarative patch in CI](#apply-a-declarative-patch-in-ci)

## Cheat Sheet

Easy42 includes a full CLI for scripting and head-less environments:

```bash
# Node operations
easy42 node list
easy42 node probe user@vps-fra.example.com
easy42 node add --name node-fra --host vps-fra.example.com \
  --ip 192.168.100.1 --ip6 fd42::1 --asn 4224420001
easy42 node bird node-fra   # Preview generated BIRD config
easy42 node nft node-fra    # Preview generated nftables rules

# Link operations
easy42 link list
easy42 link add node-fra node-lon
easy42 link add node-fra node-lon --assign-ipv4 --from-port 21001 --to-port 21002

# Declarative configuration changes
easy42 apply --dry-run patch.yaml
easy42 apply patch.yaml --sync

# State inspection and synchronization
easy42 status --live        # Probe live node connectivity and interface count
easy42 sync --dry-run       # Preview diffs across all nodes
easy42 sync                 # Apply configuration across all nodes
easy42 sync --node node-fra # Synchronize a single node
easy42 version              # Print version and bundled agent information
```

## Global Flags

All commands accept the following persistent flag:

- `-d, --data-dir <path>`: Directory containing `config.json` and `data.db`. Defaults to `~/.config/easy42`.

The data directory contains:

| File          | Description                                                                                                      |
| :------------ | :--------------------------------------------------------------------------------------------------------------- |
| `config.json` | Main configuration, encrypted secrets, and session secret (mode `0600`).                                         |
| `data.db`     | SQLite database for telemetry history, interface traffic, cron results, and applied-state hashes.                |
| `server.url`  | Transient URL file written by `easy42 serve` so CLI commands can locate the running server. Removed on shutdown. |

---

## Command Reference

### 1. `easy42 serve`

Starts the HTTP/WebSocket backend server and hosts the embedded Web UI. On first launch, it initializes `config.json` and prints a generated admin password.

```bash
# Listen on default 127.0.0.1:4242
easy42 serve

# Listen on a custom address
easy42 serve --listen 0.0.0.0:8080

# Bind to multiple addresses simultaneously (comma-separated or repeat the flag)
easy42 serve --listen 127.0.0.1:4242,192.168.1.100:4242
easy42 serve --listen 127.0.0.1:4242 --listen [::1]:4242

# Specify a custom data directory
easy42 serve --data-dir /var/lib/easy42
```

| Flag           | Default          | Description                                             |
| :------------- | :--------------- | :------------------------------------------------------ |
| `-l, --listen` | `127.0.0.1:4242` | Listen address(es). May be comma-separated or repeated. |

`serve` also starts the built-in server background-task scheduler and the hourly ROA cache refresher.

---

### 2. `easy42 node`

Manage nodes in the overlay mesh network.

#### `easy42 node list`

Lists all registered nodes with host, main IPv4, interface, ASN, and entrypoint count.

```bash
easy42 node list
```

#### `easy42 node probe <host>`

Probes a remote host over SSH and prints its interfaces, addresses, and routing information as JSON.

```bash
easy42 node probe user@vps-fra.example.com
```

#### `easy42 node add`

Registers a new SSH-managed node in the topology.

```bash
easy42 node add \
  --name "node-fra" \
  --host "vps-fra.example.com" \
  --ip "192.168.100.1" \
  --ip6 "fd42:a159:f9f0::1" \
  --external-ip "172.20.150.1" \
  --external-ip6 "fd42:a159:f9f0::d" \
  --iface "lo" \
  --asn 4224420001
```

| Flag                      | Default      | Required | Description                                                    |
| :------------------------ | :----------- | :------: | :------------------------------------------------------------- |
| `-n, --name`              | —            |   Yes    | Unique node name.                                              |
| `-H, --host`              | —            |   Yes    | SSH host, alias, or IP.                                        |
| `-i, --ip`                | —            |   Yes    | Primary overlay IPv4 address.                                  |
| `--ip6`                   | —            |    No    | Primary overlay IPv6 address.                                  |
| `--external-ip`           | —            |    No    | External/DN42 IPv4 address.                                    |
| `--external-ip6`          | —            |    No    | External/DN42 IPv6 address.                                    |
| `--iface`                 | `lo`         |    No    | Interface holding the primary IP.                              |
| `-a, --asn`               | `4224420001` |    No    | Private or external AS number.                                 |
| `--custom-network-script` | —            |    No    | Shell script persisted by the `configure_network` helper task. |

The CLI creates SSH-mode nodes. To use agent mode, generate an agent token and deploy the agent from the Web UI or API; the node then reports its mode as `agent`.

#### `easy42 node rename <old-name> <new-name>`

Renames a node and updates connected links, interface names, tags, blocks, runtime state, and DNS records. Aliases: `mv`.

```bash
easy42 node rename node-fra node-frankfurt
```

#### `easy42 node remove <name>`

Removes a node and all connected links. Aliases: `rm`, `delete`.

```bash
easy42 node remove node-frankfurt
```

#### `easy42 node bird <name>`

Compiles and prints the generated BIRD 2 configuration (`/etc/bird_easy42.conf`) for a node.

```bash
easy42 node bird node-frankfurt
```

#### `easy42 node nft <name>`

Compiles and prints the generated nftables ruleset (`/etc/easy42.nft`) for a node.

```bash
easy42 node nft node-frankfurt
```

---

### 3. `easy42 link`

Manage point-to-point tunnels between nodes.

#### `easy42 link list`

Displays link type, endpoint nodes, interface names, addresses, and listen ports.

```bash
easy42 link list
```

#### `easy42 link add <from-node> <to-node>`

Creates a WireGuard or manual link. WireGuard link creation requires the easy42 admin password to encrypt newly generated private keys. The command reads the password from `--password`, the `EASY42_PASSWORD` environment variable, or an interactive prompt.

```bash
# Standard WireGuard link (deterministic ports)
easy42 link add node-fra node-lon

# Deterministic IPv4 link-local (169.254.X.X/32) addresses and custom ports
easy42 link add node-fra node-lon --assign-ipv4 --from-port 21001 --to-port 21002

# Manual link over an existing GRE, SIT, VXLAN, or physical interface
easy42 link add node-fra node-ext \
  --type manual \
  --from-iface "gre0" \
  --to-iface "gre0" \
  --from-addr "172.20.1.1" \
  --to-addr "172.20.1.2"
```

| Flag             | Default     | Description                                                     |
| :--------------- | :---------- | :-------------------------------------------------------------- |
| `-t, --type`     | `wireguard` | Link type: `wireguard` or `manual`.                             |
| `-p, --password` | —           | easy42 admin password for WireGuard key encryption.             |
| `--from-port`    | derived     | Custom WireGuard listen port on the first node.                 |
| `--to-port`      | derived     | Custom WireGuard listen port on the second node.                |
| `--assign-ipv4`  | `false`     | Assign a deterministic IPv4 link-local (`169.254.X.X/32`) pair. |
| `--from-iface`   | —           | Interface on the first node (required for manual links).        |
| `--to-iface`     | —           | Interface on the second node (required for manual links).       |
| `--from-addr`    | —           | Local address on the first node (required for manual links).    |
| `--to-addr`      | —           | Local address on the second node (required for manual links).   |

> **Note:** There is no `--psk` flag on `link add`. Preshared keys are generated and rotated through the Web UI, the `/api/links/psk/*` endpoints, or declarative config (`links[].psk`).

#### `easy42 link remove <from-node> <to-node>`

Deletes the link between two nodes. If more than one link exists between the same pair, disambiguate with `-i, --interface <iface>`.

```bash
easy42 link remove node-fra node-lon
easy42 link remove node-fra node-lon --interface wg42lon
```

---

### 4. `easy42 status`

Inspect the configured topology and, with `--live`, probe node reachability and WireGuard interface counts.

```bash
# View configured nodes and links
easy42 status

# Probe live node status (SSH or agent) and WireGuard interfaces
easy42 status --live
```

| Flag         | Default | Description                                                  |
| :----------- | :------ | :----------------------------------------------------------- |
| `-r, --live` | `false` | Query live node status instead of showing local config only. |

---

### 5. `easy42 sync`

Synchronizes compiled configuration state (WireGuard, BIRD, nftables, ROA, and `/etc/hosts` where applicable) to managed nodes.

```bash
# Dry run: calculate and preview actions across all nodes
easy42 sync --dry-run

# Synchronize all nodes in parallel (public material only by default in non-interactive shells)
easy42 sync

# Full sync, including encrypted key material (prompts for the admin password)
easy42 sync --scope full

# Synchronize a specific target node only
easy42 sync --node node-frankfurt

# Explicit public-only synchronization
easy42 sync --scope public
```

| Flag             | Default   | Description                                                                                          |
| :--------------- | :-------- | :--------------------------------------------------------------------------------------------------- |
| `-s, --scope`    | `auto`    | `auto`, `public`, or `full`. `auto` resolves to `full` on an interactive TTY and `public` otherwise. |
| `-n, --dry-run`  | `false`   | Preview planned actions without applying changes.                                                    |
| `--node`         | all nodes | Restrict synchronization to one node.                                                                |
| `-p, --password` | —         | Admin password for `full` scope (also reads `EASY42_PASSWORD`).                                      |

**Scope semantics:**

- `public` synchronizes non-secret configuration and is safe without unlocking the encrypted key store.
- `full` unlocks the encrypted DEK and deploys WireGuard private keys/PSKs; it requires the admin password.
- `auto` selects `full` when stdin is a terminal, otherwise `public` for non-interactive/CI use.

If a server is running, the CLI connects to it using `EASY42_SERVER_URL`, `<data-dir>/server.url`, or the default `http://127.0.0.1:4242` endpoint. Otherwise it performs the sync directly from the data directory.

---

### 6. `easy42 apply`

Applies a partial declarative config change (JSON or YAML) to the easy42 configuration. Changes are validated against a strict JSON Schema before preview or execution.

```bash
# Preview a patch from a file
easy42 apply --dry-run patch.yaml

# Apply a patch and print the affected nodes/links
easy42 apply patch.json

# Read from stdin and trigger a sync of affected nodes afterwards
cat patch.yaml | easy42 apply - --sync

# Machine-readable preview
easy42 apply --dry-run --format json patch.yaml
```

| Flag         | Default | Description                                                            |
| :----------- | :------ | :--------------------------------------------------------------------- |
| `-f, --file` | —       | Path to a config change file (alternative to the positional argument). |
| `--dry-run`  | `false` | Validate and preview the planned changes without writing.              |
| `-s, --sync` | `false` | Trigger a sync of affected nodes after a successful apply.             |
| `--format`   | `text`  | Preview format: `text` or `json`.                                      |

The patch may contain `network_settings`, `network_policies`, `nodes`, `links`, `blocks`, `dns`, `templates`, `server_tasks`, `cron_tasks`, `database`, and `integrations`. Unknown fields are rejected. See [Configuration Specification](configuration.md#declarative-config-apply) for details and examples.

When a server is running, the CLI sends the apply request to `/api/config/apply` (or `/api/config/apply/preview`); otherwise it applies the change directly to the data directory. Successful applies create a timestamped backup of `config.json`.

---

### 7. `easy42 version`

Prints the easy42 version, Git commit, build timestamp, builder, Go version, OS/architecture, bundled agent version, and project URL.

```bash
easy42 version
```

---

## Common Workflows

### Bootstrap an SSH-managed mesh

```bash
easy42 node add --name node-fra --host vps-fra.example.com --ip 192.168.100.1
easy42 node add --name node-lon --host vps-lon.example.com --ip 192.168.100.2
easy42 link add node-fra node-lon
easy42 sync --dry-run
easy42 sync --scope full
```

### Apply a declarative patch in CI

```bash
export EASY42_PASSWORD="$(cat /run/secrets/easy42-password)"
easy42 apply --dry-run infra/patch.yaml
easy42 apply infra/patch.yaml --sync
```
