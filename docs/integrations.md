# Integrations & Extended Features

This document describes Easy42's third-party integrations and advanced management subsystems: the Prometheus exporter, `/etc/hosts` synchronization, WireGuard preshared keys, database retention, Cloudflare DNS automation, the Telegram bot, config templates/hooks, and scheduled tasks.

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
  - [3. WireGuard PSK (Preshared Key) Support](#3-wireguard-psk-preshared-key-support)
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
    - [Node config hooks](#node-config-hooks)
  - [8. Scheduled Tasks](#8-scheduled-tasks)
    - [Server tasks (`server_tasks`)](#server-tasks-server_tasks)
    - [Cron probes (`cron_tasks`)](#cron-probes-cron_tasks)

## 1. Prometheus Metrics Exporter

Easy42 exposes cluster telemetry in Prometheus text format (`text/plain; version=0.0.4`).

### Endpoint & Authentication

- **Endpoint**: `GET /metrics`
- **Enablement**: `integrations.prometheus_enabled` (enabled by default).
- **Authentication**: Bearer token from **Settings -> Integrations**, passed either as `Authorization: Bearer <token>` or `?token=<token>`.
- **Disabled behavior**: returns HTTP `403`; a missing token returns `500`; a bad token returns `401` with `WWW-Authenticate`.
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
- The server computes a SHA-256 hash of the normalized block. On agent registration and on every telemetry report, a mismatch triggers a push of the current records.
- A successful apply also triggers a background resync, and the block is synchronized on normal node sync.

Hosts sync requires agent mode; SSH-only nodes are not modified.

---

## 3. WireGuard PSK (Preshared Key) Support

Easy42 supports WireGuard **preshared keys** for an additional symmetric layer on top of Curve25519.

### Key Capabilities

- **Per-link PSK**: `links[].psk` (or a legacy endpoint-level `psk`).
- **Encrypted storage**: PSKs and private keys are encrypted with the DEK using XAES-256-GCM in `config.json`.
- **Generation**: PSKs are generated automatically for new WireGuard links when the vault is unlocked, or can be supplied through the UI/API.
- **Rotation**: Rotate from the Link dialog or the `/api/links/psk/*` endpoints, then run `easy42 sync --scope full` (or `easy42 sync` on a TTY) to deploy keys to both peers.

There is **no `--psk` flag** on `easy42 link add`. Declarative patches can set `links[].psk` to an encrypted value; omitted values are generated if the vault is unlocked.

---

## 4. Database Management & Data Retention

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

## 5. Cloudflare DNS Automation

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

## 6. Telegram Bot

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
| `/tcp_ping [node] <host:port> [count]` | TCP handshake probe (agent required).                         |

If no node is given, Easy42 uses the first available managed node (`tcp_ping` requires an active agent). Node names may be abbreviated when unambiguous.

---

## 7. Config Templates Engine & Hooks

Easy42 compiles WireGuard, BIRD, and nftables configuration with Go's `text/template` engine.

- **Built-in templates**: `default_wg`, `default_bird`, `default_nft` (read-only).
- **Custom templates**: stored in `config.json` under `templates`, managed from **Settings -> Templates** or `/api/templates`.
- **Per-node selection**: `nodes[].wg_template`, `bird_template`, and `nft_template` accept a template ID or name. Empty (or `default`) uses the built-in template.

### Node config hooks

`nodes[].config_hooks` injects snippets into generated files:

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

For `wg.peer`, an optional `target` limits the snippet to a named peer. Hooks are compiled from the server; malformed snippets produce invalid node configuration and should be validated with `easy42 node bird <name>` or `easy42 node nft <name>` before syncing.

---

## 8. Scheduled Tasks

Easy42 has two scheduler layers.

### Server tasks (`server_tasks`)

Three fixed tasks run on the Easy42 server:

| Task         | Default schedule | Purpose                                   |
| :----------- | :--------------- | :---------------------------------------- |
| `update_roa` | `0 0 * * *`      | Refresh policy ROA files.                 |
| `sync`       | `30 0 * * *`     | Push compiled configuration to all nodes. |
| `sync_dns`   | `0 1 * * *`      | Reconcile Cloudflare DNS records.         |

Schedules use 5- or 6-field cron syntax and can be changed from **Settings -> Tasks** or `PUT /api/settings/tasks/{id}`. `POST /api/settings/tasks/{id}/run` executes one immediately. The apply schema accepts a `server_tasks` section, but the current apply engine does not merge it.

### Cron probes (`cron_tasks`)

User-defined `ping` or `tcp_ping` probes run on selected node agents on a cron schedule. Each run is stored in `data.db`, rolled up into 5-minute summaries, and shown on the node's Cron Tasks page. Cron tasks are managed from **Cron Tasks** or `/api/cron-tasks`; the apply schema accepts `cron_tasks`, but the current apply engine does not merge that section.
