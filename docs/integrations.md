# Integrations & Extended Features

This document describes Easy42's third-party integrations and advanced management subsystems: the Prometheus exporter, `/etc/hosts` synchronization, WireGuard preshared keys, database retention, Cloudflare DNS automation, the Telegram bot, fleet events, config templates/hooks, scheduled tasks, workload Apps, and the MCP server.

---

- [Integrations \& Extended Features](#integrations--extended-features)
  - [1. Prometheus Metrics Exporter](#1-prometheus-metrics-exporter)
    - [Endpoint \& Authentication](#endpoint--authentication)
    - [Prometheus Configuration Example](#prometheus-configuration-example)
    - [Sample Grafana Dashboards](#sample-grafana-dashboards)
    - [Exported Metrics Catalog](#exported-metrics-catalog)
      - [A. Easy42 Cluster State \& Topology](#a-easy42-cluster-state--topology)
      - [B. WireGuard Telemetry (MindFlavor Compatible)](#b-wireguard-telemetry-mindflavor-compatible)
      - [C. BIRD / BGP Dynamic Routing (bird\_exporter Compatible)](#c-bird--bgp-dynamic-routing-bird_exporter-compatible)
      - [D. Linux Host System (node\_exporter Compatible)](#d-linux-host-system-node_exporter-compatible)
  - [2. `/etc/hosts` Automatic Management](#2-etchosts-automatic-management)
    - [How It Works](#how-it-works)
  - [3. SSH Key Management (agent mode)](#3-ssh-key-management-agent-mode)
    - [1. Public key collection](#1-public-key-collection)
    - [2. Distribution rules](#2-distribution-rules)
    - [3. Writing the key files](#3-writing-the-key-files)
    - [4. Scheduling and idempotency](#4-scheduling-and-idempotency)
  - [4. WireGuard PSK (Preshared Key) Support](#4-wireguard-psk-preshared-key-support)
    - [Key Capabilities](#key-capabilities)
  - [4. Database Management \& Data Retention](#4-database-management--data-retention)
    - [Retention Settings](#retention-settings)
    - [Cleanup \& Rollups](#cleanup--rollups)
    - [Maintenance](#maintenance)
  - [5. Cloudflare DNS Automation](#5-cloudflare-dns-automation)
    - [Configuration](#configuration)
    - [Generated DNS Records](#generated-dns-records)
    - [Sync Modes](#sync-modes)
  - [6. Telegram Bot](#6-telegram-bot)
    - [Configuration](#configuration-1)
    - [Commands](#commands)
  - [7. Config Templates Engine \& Hooks](#7-config-templates-engine--hooks)
    - [Referencing Note JSON in Templates (`.data`)](#referencing-note-json-in-templates-data)
    - [Looking Glass custom tasks](#looking-glass-custom-tasks)
    - [Node config hooks](#node-config-hooks)
  - [8. Events](#8-events)
  - [9. Workload Apps (`apps`)](#9-workload-apps-apps)
  - [10. MCP Server](#10-mcp-server)
  - [11. Scheduled Tasks](#11-scheduled-tasks)
    - [Server tasks (`server_tasks`)](#server-tasks-server_tasks)
    - [Cron probes (`cron_tasks`)](#cron-probes-cron_tasks)
    - [Ping Table](#ping-table)
    - [Server tasks (`server_tasks`)](#server-tasks-server_tasks)
    - [Cron probes (`cron_tasks`)](#cron-probes-cron_tasks)
    - [Ping Table](#ping-table)

## 1. Prometheus Metrics Exporter

Easy42 exposes cluster telemetry in Prometheus text format (`text/plain; version=0.0.4`).

### Endpoint & Authentication

- **Endpoint**: `GET /metrics`
- **Enablement**: `integrations.prometheus_enabled` (enabled by default).
- **Authentication**: Bearer token from **Settings -> Integrations**, passed either as `Authorization: Bearer <token>` or `?token=<token>`.
- **Disabled/unconfigured behavior**: returns HTTP `403` when Prometheus is disabled; returns `500` when no token is configured server-side; returns `401` (with `WWW-Authenticate`) when the supplied token is missing or mismatched.
- **Caching**: rendered metrics are cached in memory for 2 seconds to absorb high-frequency scrapes.

### Prometheus Configuration Example

```yaml
scrape_configs:
  - job_name: "easy42"
    scrape_interval: 15s
    metrics_path: "/metrics"
    scheme: "http" # or https
    static_configs:
      - targets: ["127.0.0.1:4242"]
    authorization:
      type: Bearer
      credentials: "e42_prom_your_auth_token_here"
```

The token can be rotated from the UI or `POST /api/settings/integrations/reset-token`.

### Sample Grafana Dashboards

Pre-built dashboards are in [`docs/grafana/`](grafana/):

- **[easy42 - Cluster Overview](grafana/easy42-cluster-overview.json)**: fleet health, node inventory, link connectivity, and resource highlights.
- **[easy42 - WireGuard & BGP Mesh](grafana/easy42-wireguard-mesh.json)**: handshake tracking, Rx/Tx rates, and BIRD session state.
- **[easy42 - Node System Metrics](grafana/easy42-node-system.json)**: CPU, load, memory, root-filesystem usage, and reported network interfaces.

See the [Grafana Dashboards Guide](grafana/README.md) for import instructions.

### Exported Metrics Catalog

Only nodes running `easy42-agent` report telemetry. SSH-only nodes still appear in `easy42_node_info` and `easy42_node_up`, but they have no `easy42_node_last_seen_timestamp_seconds` and no WireGuard, BIRD, or host metrics (the runtime metadata labels are empty).

#### A. Easy42 Cluster State & Topology

| Metric Name                               | Type  | Description                                                       | Labels                                                                               |
| :---------------------------------------- | :---: | :---------------------------------------------------------------- | :----------------------------------------------------------------------------------- |
| `easy42_exporter_build_info`              | Gauge | Build and version information                                     | `version`, `commit`                                                                  |
| `easy42_scrape_duration_seconds`          | Gauge | Metrics collection duration                                       | —                                                                                    |
| `easy42_total_nodes`                      | Gauge | Configured nodes                                                  | —                                                                                    |
| `easy42_connected_nodes`                  | Gauge | Nodes with a connected agent                                      | —                                                                                    |
| `easy42_total_links`                      | Gauge | Configured links                                                  | —                                                                                    |
| `easy42_node_info`                        | Gauge | Static node metadata                                              | `node`, `hostname`, `agent_version`, `os`, `asn`, `ip`, `ip6`, `mode`, `is_external` |
| `easy42_node_up`                          | Gauge | Agent connection status (`1` connected, `0` offline)              | `node`                                                                               |
| `easy42_node_last_seen_timestamp_seconds` | Gauge | Unix timestamp of last agent contact                              | `node`                                                                               |
| `easy42_link_up`                          | Gauge | `1` if either endpoint reported a WireGuard handshake within 180s | `from_node`, `to_node`, `from_interface`, `to_interface`                             |

`is_external` is rendered as `0` or `1`. `easy42_link_up` is derived from agent WireGuard peer state, not from BGP session state.

#### B. WireGuard Telemetry (MindFlavor Compatible)

| Metric Name                                            |  Type   | Description                                            | Labels                                                                      |
| :----------------------------------------------------- | :-----: | :----------------------------------------------------- | :-------------------------------------------------------------------------- |
| `wireguard_peer_connected`                             |  Gauge  | `1` if the peer handshaked within 180s                 | `node`, `interface`, `public_key`, `peer_node`, `friendly_name`             |
| `wireguard_latest_handshake_seconds`                   |  Gauge  | Unix timestamp of the latest handshake                 | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_latest_handshake_delay_seconds`             |  Gauge  | Seconds since the latest handshake                     | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_received_bytes_total`                       | Counter | Bytes received from the peer                           | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_sent_bytes_total`                           | Counter | Bytes sent to the peer                                 | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_interface_up`                               |  Gauge  | `1` if a `wg*` interface is up                         | `node`, `interface`                                                         |
| `wireguard_peer_persistent_keepalive_interval_seconds` |  Gauge  | Keepalive interval, emitted only when greater than `0` | `node`, `interface`, `public_key`, `peer_node`, `friendly_name`             |

`peer_node` and `friendly_name` are resolved from configured links by peer public key or interface name. Both are empty for unmanaged peers.

#### C. BIRD / BGP Dynamic Routing (bird_exporter Compatible)

| Metric Name                    | Type  | Description                                   | Labels                               |
| :----------------------------- | :---: | :-------------------------------------------- | :----------------------------------- |
| `bird_protocol_up`             | Gauge | `1` if the protocol state is `up`             | `node`, `name`, `proto`, `peer_node` |
| `bird_protocol_state`          | Gauge | Value `1` for the current state               | `node`, `name`, `proto`, `state`     |
| `bird_bgp_session_established` | Gauge | `1` if the BGP protocol reports `established` | `node`, `name`, `peer_node`          |

#### D. Linux Host System (node_exporter Compatible)

| Metric Name                               |  Type   | Description                                               | Labels               |
| :---------------------------------------- | :-----: | :-------------------------------------------------------- | :------------------- |
| `node_cpu_percent`                        |  Gauge  | CPU utilization percentage (0-100)                        | `node`               |
| `node_cpu_utilization_ratio`              |  Gauge  | CPU utilization ratio (0-1)                               | `node`               |
| `node_load1`, `node_load5`, `node_load15` |  Gauge  | Load averages                                             | `node`               |
| `node_memory_MemTotal_bytes`              |  Gauge  | Total memory                                              | `node`               |
| `node_memory_used_bytes`                  |  Gauge  | Used memory                                               | `node`               |
| `node_memory_MemAvailable_bytes`          |  Gauge  | Currently rendered as `total - used`                      | `node`               |
| `node_memory_MemFree_bytes`               |  Gauge  | Currently rendered as `total - used`                      | `node`               |
| `node_uptime_seconds`                     |  Gauge  | Host uptime                                               | `node`               |
| `node_boot_time_seconds`                  |  Gauge  | Boot time (now minus uptime)                              | `node`               |
| `node_filesystem_size_bytes`              |  Gauge  | Filesystem total size                                     | `node`, `mountpoint` |
| `node_filesystem_used_bytes`              |  Gauge  | Used space                                                | `node`, `mountpoint` |
| `node_filesystem_free_bytes`              |  Gauge  | Free space                                                | `node`, `mountpoint` |
| `node_filesystem_avail_bytes`             |  Gauge  | Currently rendered as free bytes (not non-root available) | `node`, `mountpoint` |
| `node_network_receive_bytes_total`        | Counter | Received bytes                                            | `node`, `device`     |
| `node_network_transmit_bytes_total`       | Counter | Transmitted bytes                                         | `node`, `device`     |
| `node_network_receive_packets_total`      | Counter | Received packets                                          | `node`, `device`     |
| `node_network_transmit_packets_total`     | Counter | Transmitted packets                                       | `node`, `device`     |
| `node_network_receive_errs_total`         | Counter | Receive errors                                            | `node`, `device`     |
| `node_network_transmit_errs_total`        | Counter | Transmit errors                                           | `node`, `device`     |
| `node_network_up`                         |  Gauge  | `1` if the device is up                                   | `node`, `device`     |

---

## 2. `/etc/hosts` Automatic Management

Easy42 can keep node names resolvable on managed nodes without running internal DNS. This feature is **agent-only** and is controlled per node with `update_hosts: true`.

### How It Works

- Only non-external nodes that have a main `ip` are included. External peers and IPv6-only records are skipped.
- Records are sorted by node name and written as `IP name name.internal`.
- The managed block uses fixed markers:

```
### EASY42_START
192.168.100.1 node-fra node-fra.internal
192.168.100.2 node-lon node-lon.internal
### EASY42_END
```

- Everything outside the markers is preserved.
- The server computes a SHA-256 hash of the normalized block. On agent registration, on every telemetry report, and on heartbeat `hosts_hash` change, a mismatch triggers a push of the current records. Only nodes with `update_hosts` enabled in agent mode are updated.
- A successful apply also triggers a background resync, and the block is synchronized on normal node sync.

Hosts sync requires agent mode; SSH-only nodes are not modified.

---

## 3. SSH Key Management (agent mode)

Easy42 can make root SSH between cluster nodes password-less and free of host-key prompts. It is **agent-only**, requires agent **v0.1.18+**, and is disabled on nodes whose agent config sets `disable_apps: true`.

### 1. Public key collection

Each supporting agent reports **two kinds of public keys**, tagged with a `kind`:

| Kind   | Files                                              | Purpose                                                                 |
| :----- | :------------------------------------------------- | :---------------------------------------------------------------------- |
| `user` | `~/.ssh/id_rsa.pub`, `id_ecdsa.pub`, `id_ecdsa_sk.pub`, `id_ed25519.pub`, `id_ed25519_sk.pub`, `id_dsa.pub` | Root **login** identities used by the node's SSH *client*. Installed into `authorized_keys` so this node can log in elsewhere. |
| `host` | `/etc/ssh/ssh_host_*_key.pub` (OpenSSH) and `/etc/dropbear/dropbear_*_host_key` (Dropbear/OpenWrt) | SSH **server** host keys presented by the node's SSH server. Installed into `known_hosts` so other nodes can verify this node when connecting to it. |

The distinction matters: `~/.ssh/id_*` keys are *client* keys and have nothing to do with the identity the node's SSH server presents. Only the `host` keys belong in `known_hosts`. Host key collection is agent **v0.1.19+**.

Both server layouts are detected automatically: OpenSSH `.pub` files and the Dropbear single-file key format used on OpenWrt (`dropbear_ed25519_host_key`, `dropbear_rsa_host_key`, `dropbear_ecdsa_host_key`, `dropbear_dss_host_key`). For Dropbear the public key is extracted from the binary key file; there is no `.pub` companion to verify against.

Dropbear writes the SSH private-key structure (`string keytype` followed by the private parameters: RSA `e, n, d, p, q`, ECDSA `curve, Q, d`, Ed25519 `string(private scalar ‖ public point)`). Only the leading public fields are published, and the Ed25519 point is truncated to its 32-byte public half. A key is only reported when its blob is **exactly** one public key (algorithm plus its public fields, nothing after them), both in the agent and in the controller; anything else is treated as unreadable instead of being written to `known_hosts`. This matters because OpenSSH compares the stored blob structurally with the key the server presents, so an over-long blob (private material included) would never match and every connection would prompt for host key confirmation.

- When the matching private key can be parsed (unencrypted OpenSSH, PEM RSA, or PEM EC), the agent verifies the `.pub` file actually belongs to it and ignores stale or hand-edited files. Non-default identity files (`id_custom.pub`, keys generated with `-f`) are never collected.
- Keys are reported in telemetry and can be refreshed on demand. Files are re-read at most once per minute.

### 2. Distribution rules

Keys are **never** distributed implicitly. A rule maps a **source selector** (the nodes whose keys are distributed) onto a **destination selector** (the nodes whose SSH key files are updated) with one of three types:

| Type              | Target file                          | Keys installed                                  |
| :---------------- | :----------------------------------- | :---------------------------------------------- |
| `known_hosts`     | `/root/.ssh/known_hosts` (mode 0644) | The source nodes' SSH **server host keys**, so the destination can verify them when connecting to those nodes. |
| `authorized_keys` | `/root/.ssh/authorized_keys` (0600)  | The source nodes' root **login keys**, so they can log in to the destination as root. |
| `all`             | Both files                           | Each file receives only the matching key kind.  |

`known_hosts` never receives login identities and `authorized_keys` never receives host keys; keys of the wrong kind are skipped even if a rule targets both files.

A source node that matches a rule but contributes nothing (no keys collected yet, SSH key management unavailable because it is not in agent mode, runs an older agent, or has `disable_apps: true`) is silently left out of the block. The sync action description stays short and stable (`Update SSH known_hosts (…) on node`); the diff shows exactly which keys changed, so a node that cannot contribute never turns the description into a wall of text.

**External peers are never sources.** DN42/IXP peers (`is_external`) have no agent and can never publish keys: they are not reported as "skipped" even though they match an "all nodes" selector. You can therefore select "all nodes" as the source without the 25+ external peers polluting every sync entry.

When several nodes publish the **same** host key (cloned VM images, a shared key), the host name lists are merged into one `known_hosts` line so no node is left untrusted.

Selectors use the same node selector format as workload apps and cron probes (`name`, `tag`, `env:<NAME>`, or `{"all":"1"}` for every node). Neither selector may be empty: an empty selector matches no node. Rules are managed from the **SSH** tab of the Apps page (`/apps/ssh`), the REST API (`/api/ssh-keys/rules`), or MCP (`list_ssh_keys`, `create_ssh_key_rule`, `update_ssh_key_rule`, `delete_ssh_key_rule`).

### 3. Writing the key files

`/root/.ssh/known_hosts` and `/root/.ssh/authorized_keys` are managed exactly like `/etc/hosts`: the desired lines are assembled into a deterministic block, every other line is preserved, and the file is replaced atomically (`fsync` + `rename`) only when its content actually changes.

```
### EASY42_START
node-fra,node-fra.internal,node-fra.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5... root@node-fra
### EASY42_END
```

`known_hosts` lines follow the OpenSSH format `hostnames keytype base64-key [comment]`. Each source key is written once per source node with the host names that node is reachable under, in this order: the main IPv4/IPv6 addresses, the external IPv4/IPv6 addresses, `<name>`, `<name>.internal` and, when a **Base Domain** is set on the DNS settings page, `<name>.<base domain>`. Missing values are skipped. A key without any host name is left out of `known_hosts` (such a line would be invalid), while it is still written to `authorized_keys`.

Entries written to `authorized_keys` are prefixed with the `easy42` comment marker so cluster-managed keys can be told apart from manually added ones. Removing every rule that granted a key removes the block (and its surrounding separator line) again, restoring the previous file contents byte for byte.

The controller sends the rendered block in the `sync_ssh_keys` action's `file_content`, and the agent writes it verbatim (host name lists in `known_hosts` cannot be rebuilt from the key list alone). The agent only rebuilds a block from the `ssh_keys` field when no content is supplied, which keeps older controllers working.

### 4. Scheduling and idempotency

- The sync plan contains one `sync_ssh_keys` action per destination node and target file, compared against the hash of the block last applied (state store) and the hash last reported by the agent. Synced files are not re-sent.
- When a node agent registers or a source key changes, the latest applied state is re-pushed automatically, mirroring the `/etc/hosts` auto-remediation. Changes that were never synced are never deployed silently.
- A source key rotation invalidates every plan that includes it on the next sync.

---

## 4. WireGuard PSK (Preshared Key) Support

Easy42 supports WireGuard **preshared keys** for an additional symmetric layer on top of Curve25519.

### Key Capabilities

- **Per-link PSK**: `links[].psk` (or a legacy endpoint-level `psk`).
- **Encrypted storage**: PSKs and private keys are encrypted with the DEK using XAES-256-GCM in `config.json`.
- **Generation**: PSKs are generated automatically for new WireGuard links when the vault is unlocked, or can be supplied through the UI/API.
- **Rotation**: Rotate from the Link dialog or the `/api/links/psk/reveal` / `/api/links/psk/reset` endpoints (private keys: `/api/links/private-key/reveal` / `/api/links/private-key/reset`), then run `easy42 sync --scope full` (or `easy42 sync` on a TTY) to deploy keys to both peers.

There is **no `--psk` flag** on `easy42 link add`. Declarative patches can set `links[].psk` to an encrypted value; omitted values are generated if the vault is unlocked.

---

## 5. Database Management & Data Retention

Easy42 stores telemetry in an embedded SQLite database at `<data-dir>/data.db`.

### Retention Settings

- `database.retention_days`: summarized metrics, 5-minute rollups, and cron results. Default `47`.
- `database.raw_retention_days`: high-resolution raw telemetry and probe logs. Default `2`.
- API/UI limits: summarized retention up to `3650` days; raw retention up to `30` days and never greater than the summarized retention. The config apply engine only enforces positive values, so a patch can exceed the API caps.

### Cleanup & Rollups

- An initial cleanup runs 15 seconds after server startup, then every hour.
- Raw rows older than `raw_retention_days` are aggregated into 5-minute statistical windows before being pruned.
- Summaries older than `retention_days` are deleted.
- Interface traffic statistics are refreshed every 30 seconds.

### Maintenance

- **Vacuum**: `POST /api/settings/database/vacuum` runs a rollup/clean pass, then `VACUUM`.
- **Size monitoring**: `GET /api/settings/database` returns database stats and current retention values.
- Retention changes apply immediately and trigger rollup/cleanup.

---

## 6. Cloudflare DNS Automation

Easy42 can publish overlay addresses to Cloudflare. DNS sync is skipped unless `zone_id`, `api_token`, and `base_domain` are all configured.

### Configuration

```json
{
  "dns": {
    "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
    "api_token": "cf-token-...",
    "base_domain": "mesh.example.com",
    "publish_ipv6_own_name": false
  }
}
```

### Generated DNS Records

For each node with a valid main IPv4:

- `A <node>.<base_domain>` -> node IPv4
- `A *.<node>.<base_domain>` -> node IPv4

For IPv6, behavior depends on `publish_ipv6_own_name`:

- **`false` (default)**: `AAAA <node>.<base_domain>` and `AAAA *.<node>.<base_domain>` -> node IPv6.
- **`true`**: only `AAAA <node>6.<base_domain>` -> node IPv6. In this mode the node's own name and wildcard are **not** given AAAA records.

All records are created with automatic TTL and are not proxied. Easy42 manages A/AAAA records it owns and deletes duplicates.

### Sync Modes

- A normal DNS sync reconciles expected records and removes stale IPv6 records when the `publish_ipv6_own_name` mode changes.
- Force sync (`POST /api/settings/dns/sync` with force) additionally deletes orphaned `A`, `AAAA`, and `CNAME` records under `*.<base_domain>` that do not correspond to a current node.
- Full DNS synchronization runs from the **Settings -> DNS** sync action, `POST /api/settings/dns/sync`, or the `sync_dns` server task. In addition, adding or deleting a node, or changing a node's name/IP through the UI/API, triggers a background per-node Cloudflare record update or deletion when DNS is configured. Node configuration sync (`easy42 sync`) does **not** reconcile DNS records.

---

## 7. Telegram Bot

The Telegram integration provides read-only cluster queries and on-demand probes from a chat.

### Configuration

Set these under `integrations` (or **Settings -> Integrations**):

| Field                   | Description                                                                          |
| :---------------------- | :----------------------------------------------------------------------------------- |
| `telegram_enabled`      | Starts the bot and accepts webhook updates.                                          |
| `telegram_bot_token`    | Token from @BotFather.                                                               |
| `telegram_admin_id`     | Numeric Telegram user ID allowed to use the bot.                                     |
| `telegram_secret_token` | Secret validated against `X-Telegram-Bot-Api-Secret-Token`. Auto-generated if empty. |
| `server_url`            | Public HTTPS URL; the webhook is registered at `<server_url>/api/telegram/webhook`.  |

Use `POST /api/settings/integrations/telegram/test` to verify the token and send a test message. The webhook route itself is public but requires the secret header; all commands are restricted to `telegram_admin_id`.

### Commands

| Command                                | Description                                                   |
| :------------------------------------- | :------------------------------------------------------------ |
| `/start`                               | Welcome message and dashboard link.                           |
| `/help`                                | List commands.                                                |
| `/status`                              | Cluster status summary.                                       |
| `/nodes`                               | List managed nodes and connection state.                      |
| `/node [name-or-prefix]`               | Node details; without an argument shows a selection keyboard. |
| `/ping [node] <target> [count]`        | ICMP ping from a node (count 1-20, default 4).                |
| `/pingall <target> [count]`            | ICMP ping to target from all managed nodes (aggregated).      |
| `/tcp_ping [node] <host:port> [count]` | TCP handshake probe (agent required).                         |

If no node is given to `/ping` or `/tcp_ping`, Easy42 uses the first available managed node (`tcp_ping` requires an active agent). Node names may be abbreviated when unambiguous. `/pingall` fans out to all managed nodes concurrently (sorted, 30s context) and returns one aggregated report, split into `(cont.)` follow-up messages when the result exceeds ~3500 characters. Node online/offline alerts are debounced (90s grace, debounce threshold 2, 10s monitor tick) and also recorded as fleet events (see [Events](#8-events)).

---

## 8. Config Templates Engine & Hooks

Easy42 compiles WireGuard, BIRD, and nftables configuration with Go's `text/template` engine.

- **Built-in templates**: `default_wg`, `default_bird`, `default_nft` (read-only).
- **Custom templates**: stored in `config.json` under `templates`, managed from **Settings -> Templates** or `/api/templates`.
- **Per-node selection**: `nodes[].wg_template`, `bird_template`, and `nft_template` accept a template ID or name. Empty (or `default`) uses the built-in template.

### Referencing Note JSON in Templates (`.data`)

The markdown `note` field on **Node**, **Link**, **LinkEnd**, and **App** objects can embed structured configuration by including a ` ``` ` or ` ```json ` code block. Easy42 automatically extracts and parses the JSON from the first found code fence (a block with any other language is skipped) and exposes it as the `.data` map in template contexts (best-effort; if the first matching block is invalid JSON, extraction stops and yields an empty map without breaking template rendering).

For example, if a node's Note contains:

````markdown
# Edge Gateway

```json
{
  "type": "server",
  "zone": "us-east"
}
```
````

You can reference these properties in custom BIRD, WireGuard, and nftables templates:

- **Node level** (BIRD, nftables, WireGuard root): `{{ .data.type }}`
- **WireGuard context**: `{{ .self_node.data.type }}`, `{{ .peer_node.data.type }}`, `{{ .link.data.type }}`, `{{ .self_end.data.prop }}`, `{{ .peer_end.data.prop }}`
- **BIRD links iteration**: `{{ range .links }}{{ .data.carrier }} {{ .link.data.carrier }} {{ .local.data.prop }} {{ .remote.data.prop }} {{ .remote_node.data.role }}{{ end }}`

### Looking Glass custom tasks

Built-in Looking Glass tasks are `ping`, `tcp_ping`, `http`, `traceroute`, `mtr`, `bird_show_protocols`, `bird_show_route`, `bird_show_route_where`, `bird_show_status`, `ip_route_get`, and `whois_dn42`. Custom tasks can be created/edited via `GET/POST /api/looking-glass/tasks` and `PUT/DELETE /api/looking-glass/tasks/{id}` (stored in `looking_glass_tasks`), and ad-hoc runs use `POST /api/looking-glass/run` (timeout clamped to `1`–`60s`; SSH fallback for non-agent nodes, while `tcp_ping` and `http` require an agent).

The `http` task and the ad-hoc `http <url> [count]` command run natively inside the agent: a single `GET` with no redirects, stopping at the response header (the body is never read). Latency is TTFB, a probe succeeds as soon as a response header is received (`4xx`/`5xx` are recorded, not failed), `count` defaults to `1` (max `5`), and results render in the `HTTP Metrics` visualizer with per-probe status codes.

### Node config hooks

`nodes[].config_hooks` injects snippets into generated files. Each snippet is itself a Go `text/template` rendered with the same context and functions as the config template it is injected into (e.g. `define hook_ip = {{ .ip }};`), then included by the config template's hook section:

| Hook type                                   | Injection point                      |
| :------------------------------------------ | :----------------------------------- |
| `bird.pre`                                  | Before BIRD protocol declarations.   |
| `bird` (aliases `bird.post`, `bird.global`) | End of the BIRD configuration.       |
| `wg.interface`                              | Inside the `[Interface]` section.    |
| `wg.peer`                                   | Inside each `[Peer]` section.        |
| `wg.post` (alias `wg`)                      | End of the WireGuard interface file. |
| `nft.pre`                                   | Before nftables table declarations.  |
| `nft.table`                                 | Inside `table inet easy42 { ... }`.  |
| `nft` (aliases `nft.post`, `nft.global`)    | End of the nftables file.            |

For `wg.peer`, an optional `target` limits the snippet to a matching interface or peer name. Hooks exist only on `nodes[].config_hooks` (links/endpoints have none). Hooks are compiled from the server: invalid template syntax fails config generation with a `hook template` error, while syntactically valid templates may still produce invalid node configuration — validate with `easy42 node bird <name>` or `easy42 node nft <name>` before syncing.

---

## 9. Events

Easy42 records fleet events in SQLite (`data.db`) and broadcasts them over the client WebSocket (`event_created`) and to Telegram via the bot's notify path.

| Field                   | Type     | Description                                            |
| :---------------------- | :------- | :----------------------------------------------------- |
| `type`                  | `string` | `node_offline`, `node_online`, or `cron_task_failure`. |
| `node_name`             | `string` | Affected node.                                         |
| `task_id`               | `string` | Cron task ID for `cron_task_failure`.                  |
| `title`, `message`      | `string` | Human-readable summary and detail.                     |
| `metadata`              | `object` | Structured extra data.                                 |
| `created_at`, `is_read` | —        | Timestamp and read flag.                               |

Producers: node online/offline transitions from the Telegram monitor (90s grace, debounce 2), and cron-task failures (debounced to at most one event per task/node per 24h). API: `GET /api/events` (with `limit`/`offset`/`unread_only`/`type`/`node_name` filters, returning `events`, `total`, `unread_count`), `GET /api/events/unread-count`, `POST /api/events/{id}/read`, `POST /api/events/read-all`, `DELETE /api/events/{id}`, `DELETE /api/events`.

## 10. Workload Apps (`apps`)

Workload Apps are declarative units of work deployed to agent-managed nodes from the Web UI or `/api/apps`. External nodes are never targets, and nodes whose agent reports `disable_apps: true` refuse app sync/actions.

| App type  | Agent destination                                                                                                 | Notes                                                                                                                                                             |
| :-------- | :---------------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `docker`  | `/etc/ezappc/ezapp-<name>/compose.yaml` + `.env` (`0600`)                                                         | `docker compose up` runs asynchronously (900s timeout); status is probed.                                                                                         |
| `systemd` | `/etc/systemd/system/ezapp-<name>.service` + `.env` (plus `EnvironmentFile=/etc/systemd/system/ezapp-<name>.env`) | Transpiled to OpenRC (`/etc/init.d/ezapp-*` + `/etc/conf.d/ezapp-*`) or procd on Alpine/OpenWrt; sandboxing/cgroup directives are dropped with `# NOTE:` headers. |
| `cron`    | System crontab between `### EASY42_START`/`### EASY42_END` markers                                                | Sorted entries; disabled apps are `#`-prefixed. Distinct from cron **probes** below.                                                                              |
| `script`  | `/etc/ezscript/<name>` (`0755`)                                                                                   | Standalone executable script; there is no service unit or daemon.                                                                                                 |

A disabled or deleted app is removed from the next sync (`Remove unassigned …`). Script apps are cleaned up by removing `/etc/ezscript/<name>`; their type is resolved from the agent probe or the reported app status, so the plan converges even after the file is gone and never falls back to a nonexistent service unit.

Auxiliary `files` and artifacts are written to `/etc/ezapp/<name>/`. Orphaned files from deleted/renamed apps are garbage-collected via a `.easy42-managed.json` manifest (user bind-mounts are preserved). Deployment is hash-based (`ComputeAppHash` / `ComputeAppSpecFilesHash`) and skipped entirely when `is_apps_disabled` on the node.

The target init system is taken from the `init_system` value the agent reports on registration (`systemd`, `procd`, or `openrc`). Only agents that report `procd`/`openrc` get their systemd units transpiled; older agents fall back to matching `os_info`, which cannot identify OpenWrt.

- **Selectors**: `apps[].selector` uses the shared `name`/`tag`/`env:<NAME>` semantics (AND). Use `{"all":"1"}` for every node; an empty selector matches no node. See [Task Selector](device-helper.md#task-selector).
- **Artifacts**: `apps[].artifact` downloads a binary from a URL template or a GitHub Release and serves it to the agent, which installs it on the node. The server resolves one artifact per node architecture, so the template must be architecture-aware.
  - **Architecture variables**: `${ARCH}` (Go: `amd64`/`arm64`/`armv7`/`mipsel`), `${ARCH_GNU}` (`x86_64`/`aarch64`/`armv7l`/`mipsel`), `${ARCH_RUST}` (`x86_64`/`aarch64`/`armv7`/`mipsel`), `${ARCH_DEB}` (`amd64`/`arm64`/`armhf`), `${OS}`, and `${APP_NAME}`.
  - **MIPS endianness**: MIPS kernels often report a bare `mips` for both byte orders (common on OpenWrt/MT7621). Agents therefore report `os_info` as `<os> <arch> <endian>` — e.g. `linux mips little` — and the server resolves the endianness before expanding the template, so `mips` + `little` becomes `mipsel` (and `mips64` + `little` becomes `mips64le`). Use `${ARCH_RUST}` in Rust-style asset names: `wg-watcher-${ARCH_RUST}-unknown-linux-musl` yields `wg-watcher-mipsel-unknown-linux-musl`.
  - **Automatic asset selection**: When the GitHub resolver has no `asset_pattern`, it matches assets on architecture token boundaries, so `mips` never matches a `mipsel` asset and `arm` never matches `arm64`. If no asset matches the reported endianness, resolution fails instead of installing the wrong binary.
  - **Overrides**: `arch_mapping` (`nodeArch -> upstreamArch`) and `checksums` (`arch -> sha256`) keys are matched against the resolved architecture (e.g. `mipsel`).
- **Variables**: Global `app_variables` (`key`, `value`, `description`, `is_secret`) are combined with per-node context variables (plus `APP_NAME`). Interpolation supports `{{VAR}}`/`{{.VAR}}` and `${VAR}` in `url`, `env_content`, `files`, and cron-type `main_content` only (Docker/systemd manifests are not interpolated). Preview with `POST /api/apps/preview-interpolate` and `POST /api/apps/preview-transpile`.
- **Lifecycle API**: `GET/POST /api/apps`, `PUT/DELETE /api/apps/{id}`, `POST …/toggle`, `POST …/toggle-published`, `POST/PUT …/rename`, `POST/PUT …/duplicate` (forces `published: false`, `enabled: false` and auto-increments trailing digits), `GET/POST /api/apps/variables`, `DELETE /api/apps/variables/{key}`, `GET /api/nodes/{name}/apps`, `POST /api/nodes/{name}/apps/{appName}/action` (`start`/`stop`/`restart`/`logs`, default tail `100`).
- **Note JSON**: An App's `note` may carry a fenced `json` block exposed as `.data` in its templates, like node/link notes.

## 11. MCP Server

Easy42 ships a Model Context Protocol server so AI tools and automation can inspect (and, with a full-control token, modify) the cluster.

- **Transports**: Streamable HTTP at `/mcp` and `/api/mcp`; SSE at `/mcp/sse` + `/mcp/message` (and `/api/mcp/sse` + `/api/mcp/message`). A stdio CLI is also available: `easy42 mcp [--role full_control|read_only]`.
- **Auth**: Bearer token (`Authorization: Bearer <token>` or `?token=<token>`) with constant-time comparison. The **read-only** token permits inspection; the **full-control** token permits modifications. OAuth 2.0/2.1 is supported for clients such as Gemini: discovery at `/.well-known/oauth-protected-resource`, `/.well-known/oauth-authorization-server`, and `/.well-known/openid-configuration`, plus `GET /oauth/authorize`, `POST /oauth/token`, and `POST /oauth/register` (with `mcp.oauth_client_id`/`mcp.oauth_client_secret`).
- **Tools & resources**: ~50 tools covering cluster info, nodes, links, network policies, templates, config apply, sync, tasks, Looking Glass, cron tasks, database, and integrations (see `internal/mcp/tools_*.go`). Resources: `easy42://cluster/summary`, `easy42://cluster/nodes`, `easy42://cluster/links`. Prompts: `cluster_health_audit`, `add_node_wizard`.
- **Settings**: `mcp.enabled`, `mcp.read_only_token`, `mcp.full_control_token`, `mcp.oauth_client_id`, `mcp.oauth_client_secret` in `config.json` (see [Configuration](configuration.md#mcp)). Manage via `GET/PUT /api/settings/mcp` and `POST /api/settings/mcp/reset-token`.

## 12. Scheduled Tasks

Easy42 has two scheduler layers.

### Server tasks (`server_tasks`)

Four fixed tasks run on the Easy42 server (scheduler ticks every second, 60s slots, 10-minute execution timeout; each run records `ServerTaskState`):

| Task            | Default schedule | Purpose                                               |
| :-------------- | :--------------- | :---------------------------------------------------- |
| `update_roa`    | `0 0 * * *`      | Refresh policy ROA files.                             |
| `sync`          | `30 0 * * *`     | Push compiled configuration to all nodes.             |
| `sync_dns`      | `0 1 * * *`      | Reconcile Cloudflare DNS records.                     |
| `backup_config` | `0 2 * * *`      | Backup `config.json` (at most 100 backups preserved). |

Schedules use 5- or 6-field cron syntax and can be changed from **Settings -> Tasks** or the `/api/settings/tasks` API (`GET /api/settings/tasks`, `PUT /api/settings/tasks/{id}`). `POST /api/settings/tasks/{id}/run` executes one immediately. The apply schema accepts a `server_tasks` section, but the current apply engine does not merge it.

### Cron probes (`cron_tasks`)

User-defined `ping`, `tcp_ping`, or `http` probes run on selector-matched node agents on a cron schedule (`task_type` is only `ping`/`tcp_ping`/`http`, not arbitrary Looking Glass IDs). `params.count` defaults to `4` (`1`–`20`); for `http` tasks it defaults to `1` (max `5`) and `target` is a validated `http(s)://` URL, `timeout_sec` defaults to `15` (capped at `60`). Results are stored in `data.db` (with parsed RTT/loss and `ParsedJSON`), rolled up into 5-minute summaries (`ok`/`degraded`/`failed`), shown on the node's Cron Tasks page, and repeated failures record a fleet event. Cron tasks are managed from **Cron Tasks** or `/api/cron-tasks` (`GET/POST /api/cron-tasks`, `PUT/DELETE /api/cron-tasks/{id}`, `POST …/toggle`, `GET /api/nodes/{name}/cron-tasks/results`); the apply schema accepts `cron_tasks`, but the current apply engine does not merge that section. A task whose selector matches no managed node is rejected on save.

### Ping Table

The Looking Glass **Ping Table** measures latency between every pair of agent nodes: `e2e` (end-to-end via the overlay) or `link` (direct link interface/address when a direct link exists, otherwise derived IPv6 link-local). One run fans out to `N*(N-1)` directed pings (`ping` count `3`, 4s timeout, 5s context) with at most 25 concurrent executions and per-source-node serialization; statuses include `pending`/`running`/`success`/`failed`/`unreachable`/`no_agent`/`no_link`/`self`/`cancelled`/`completed`. API: `POST /api/looking-glass/ping-table/run|cancel`, `GET …/current|/history|/tasks/{id}|/stream` (SSE), `DELETE …/tasks/{id}`. History is persisted in `data.db` (`ping_table_tasks`).
