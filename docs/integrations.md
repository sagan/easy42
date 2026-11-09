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
  - [8. Events](#8-events)
  - [9. Workload Apps (`apps`)](#9-workload-apps-apps)
  - [10. MCP Server](#10-mcp-server)
  - [11. Scheduled Tasks](#11-scheduled-tasks)
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

## 3. WireGuard PSK (Preshared Key) Support

Easy42 supports WireGuard **preshared keys** for an additional symmetric layer on top of Curve25519.

### Key Capabilities

- **Per-link PSK**: `links[].psk` (or a legacy endpoint-level `psk`).
- **Encrypted storage**: PSKs and private keys are encrypted with the DEK using XAES-256-GCM in `config.json`.
- **Generation**: PSKs are generated automatically for new WireGuard links when the vault is unlocked, or can be supplied through the UI/API.
- **Rotation**: Rotate from the Link dialog or the `/api/links/psk/reveal` / `/api/links/psk/reset` endpoints (private keys: `/api/links/private-key/reveal` / `/api/links/private-key/reset`), then run `easy42 sync --scope full` (or `easy42 sync` on a TTY) to deploy keys to both peers.

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
| `/pingall <target> [count]`            | ICMP ping to target from all managed nodes (aggregated).      |
| `/tcp_ping [node] <host:port> [count]` | TCP handshake probe (agent required).                         |

If no node is given to `/ping` or `/tcp_ping`, Easy42 uses the first available managed node (`tcp_ping` requires an active agent). Node names may be abbreviated when unambiguous. `/pingall` fans out to all managed nodes concurrently (sorted, 30s context) and returns one aggregated report, split into `(cont.)` follow-up messages when the result exceeds ~3500 characters. Node online/offline alerts are debounced (90s grace, debounce threshold 2, 10s monitor tick) and also recorded as fleet events (see [Events](#8-events)).

---

## 7. Config Templates Engine & Hooks

Easy42 compiles WireGuard, BIRD, and nftables configuration with Go's `text/template` engine.

- **Built-in templates**: `default_wg`, `default_bird`, `default_nft` (read-only).
- **Custom templates**: stored in `config.json` under `templates`, managed from **Settings -> Templates** or `/api/templates`.
- **Per-node selection**: `nodes[].wg_template`, `bird_template`, and `nft_template` accept a template ID or name. Empty (or `default`) uses the built-in template.

### Referencing Note JSON in Templates (`.data`)

The markdown `note` field on **Node**, **Link**, **LinkEnd**, and **App** objects can embed structured configuration by including a ` ``` ` or ` ```json ` code block. Easy42 automatically extracts and parses the JSON from the first found code fence (a block with any other language is skipped) and exposes it as the `.data` map in template contexts (best-effort; if the first matching block is invalid JSON, extraction stops and yields an empty map without breaking template rendering).

For example, if a node's Note contains:
```markdown
# Edge Gateway
```json
{
  "type": "server",
  "zone": "us-east"
}
```
```

You can reference these properties in custom BIRD, WireGuard, and nftables templates:
- **Node level** (BIRD, nftables, WireGuard root): `{{ .data.type }}`
- **WireGuard context**: `{{ .self_node.data.type }}`, `{{ .peer_node.data.type }}`, `{{ .link.data.type }}`, `{{ .self_end.data.prop }}`, `{{ .peer_end.data.prop }}`
- **BIRD links iteration**: `{{ range .links }}{{ .data.carrier }} {{ .link.data.carrier }} {{ .local.data.prop }} {{ .remote.data.prop }} {{ .remote_node.data.role }}{{ end }}`

### Looking Glass custom tasks

Built-in Looking Glass tasks are `ping`, `tcp_ping`, `traceroute`, `mtr`, `bird_show_protocols`, `bird_show_route`, `bird_show_route_where`, `bird_show_status`, `ip_route_get`, and `whois_dn42`. Custom tasks can be created/edited via `GET/POST /api/looking-glass/tasks` and `PUT/DELETE /api/looking-glass/tasks/{id}` (stored in `looking_glass_tasks`), and ad-hoc runs use `POST /api/looking-glass/run` (timeout clamped to `1`–`60s`; SSH fallback for non-agent nodes, while `tcp_ping` requires an agent).

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

## 8. Events

Easy42 records fleet events in SQLite (`data.db`) and broadcasts them over the client WebSocket (`event_created`) and to Telegram via the bot's notify path.

| Field | Type | Description |
| :---- | :--- | :---------- |
| `type` | `string` | `node_offline`, `node_online`, or `cron_task_failure`. |
| `node_name` | `string` | Affected node. |
| `task_id` | `string` | Cron task ID for `cron_task_failure`. |
| `title`, `message` | `string` | Human-readable summary and detail. |
| `metadata` | `object` | Structured extra data. |
| `created_at`, `is_read` | — | Timestamp and read flag. |

Producers: node online/offline transitions from the Telegram monitor (90s grace, debounce 2), and cron-task failures (debounced to at most one event per task/node per 24h). API: `GET /api/events` (with `limit`/`offset`/`unread_only`/`type`/`node_name` filters, returning `events`, `total`, `unread_count`), `GET /api/events/unread-count`, `POST /api/events/{id}/read`, `POST /api/events/read-all`, `DELETE /api/events/{id}`, `DELETE /api/events`.

## 9. Workload Apps (`apps`)

Workload Apps are declarative units of work deployed to agent-managed nodes from the Web UI or `/api/apps`. External nodes are never targets, and nodes whose agent reports `disable_apps: true` refuse app sync/actions.

| App type | Agent destination | Notes |
| :------- | :---------------- | :---- |
| `docker` | `/etc/ezappc/<name>/compose.yaml` + `.env` (`0600`) | `docker compose up` runs asynchronously (900s timeout); status is probed. |
| `systemd` | `/etc/systemd/system/ezapp-<name>.service` + `.env` (plus `EnvironmentFile=/etc/systemd/system/ezapp-<name>.env`) | Transpiled to OpenRC (`/etc/init.d/ezapp-*` + `/etc/conf.d/ezapp-*`) or procd on Alpine/OpenWrt; sandboxing/cgroup directives are dropped with `# NOTE:` headers. |
| `cron` | System crontab between `### EASY42_START`/`### EASY42_END` markers | Sorted entries; disabled apps are `#`-prefixed. Distinct from cron **probes** below. |

Auxiliary `files` are written to `/etc/ezapp-data/<name>/`. Orphaned files from deleted/renamed apps are garbage-collected via a `.easy42-managed.json` manifest (user bind-mounts are preserved). Deployment is hash-based (`ComputeAppHash` / `ComputeAppSpecFilesHash`) and skipped entirely when `is_apps_disabled` on the node.

- **Selectors**: `apps[].selector` uses the shared `name`/`tag`/`env:<NAME>` semantics (AND; empty matches all). See [Task Selector](device-helper.md#task-selector).
- **Variables**: Global `app_variables` (`key`, `value`, `description`, `is_secret`) are combined with per-node context variables (plus `APP_NAME`). Interpolation supports `{{VAR}}`/`{{.VAR}}` and `${VAR}` in `url`, `env_content`, `files`, and cron-type `main_content` only (Docker/systemd manifests are not interpolated). Preview with `POST /api/apps/preview-interpolate` and `POST /api/apps/preview-transpile`.
- **Lifecycle API**: `GET/POST /api/apps`, `PUT/DELETE /api/apps/{id}`, `POST …/toggle`, `POST …/toggle-published`, `POST/PUT …/rename`, `POST/PUT …/duplicate` (forces `published: false`, `enabled: false` and auto-increments trailing digits), `GET/POST /api/apps/variables`, `DELETE /api/apps/variables/{key}`, `GET /api/nodes/{name}/apps`, `POST /api/nodes/{name}/apps/{appName}/action` (`start`/`stop`/`restart`/`logs`, default tail `100`).
- **Note JSON**: An App's `note` may carry a fenced `json` block exposed as `.data` in its templates, like node/link notes.

## 10. MCP Server

Easy42 ships a Model Context Protocol server so AI tools and automation can inspect (and, with a full-control token, modify) the cluster.

- **Transports**: Streamable HTTP at `/mcp` and `/api/mcp`; SSE at `/mcp/sse` + `/mcp/message` (and `/api/mcp/sse` + `/api/mcp/message`). A stdio CLI is also available: `easy42 mcp [--role full_control|read_only]`.
- **Auth**: Bearer token (`Authorization: Bearer <token>` or `?token=<token>`) with constant-time comparison. The **read-only** token permits inspection; the **full-control** token permits modifications. OAuth 2.0/2.1 is supported for clients such as Gemini: discovery at `/.well-known/oauth-protected-resource`, `/.well-known/oauth-authorization-server`, and `/.well-known/openid-configuration`, plus `GET /oauth/authorize`, `POST /oauth/token`, and `POST /oauth/register` (with `mcp.oauth_client_id`/`mcp.oauth_client_secret`).
- **Tools & resources**: ~50 tools covering cluster info, nodes, links, network policies, templates, config apply, sync, tasks, Looking Glass, cron tasks, database, and integrations (see `internal/mcp/tools_*.go`). Resources: `easy42://cluster/summary`, `easy42://cluster/nodes`, `easy42://cluster/links`. Prompts: `cluster_health_audit`, `add_node_wizard`.
- **Settings**: `mcp.enabled`, `mcp.read_only_token`, `mcp.full_control_token`, `mcp.oauth_client_id`, `mcp.oauth_client_secret` in `config.json` (see [Configuration](configuration.md#mcp)). Manage via `GET/PUT /api/settings/mcp` and `POST /api/settings/mcp/reset-token`.

## 11. Scheduled Tasks

Easy42 has two scheduler layers.

### Server tasks (`server_tasks`)

Four fixed tasks run on the Easy42 server (scheduler ticks every second, 60s slots, 10-minute execution timeout; each run records `ServerTaskState`):

| Task            | Default schedule | Purpose                                   |
| :-------------- | :--------------- | :---------------------------------------- |
| `update_roa`    | `0 0 * * *`      | Refresh policy ROA files.                 |
| `sync`          | `30 0 * * *`     | Push compiled configuration to all nodes. |
| `sync_dns`      | `0 1 * * *`      | Reconcile Cloudflare DNS records.         |
| `backup_config` | `0 2 * * *`      | Backup `config.json` (at most 100 backups preserved). |

Schedules use 5- or 6-field cron syntax and can be changed from **Settings -> Tasks** or the `/api/settings/tasks` API (`GET /api/settings/tasks`, `PUT /api/settings/tasks/{id}`). `POST /api/settings/tasks/{id}/run` executes one immediately. The apply schema accepts a `server_tasks` section, but the current apply engine does not merge it.

### Cron probes (`cron_tasks`)

User-defined `ping` or `tcp_ping` probes run on selector-matched node agents on a cron schedule (`task_type` is only `ping`/`tcp_ping`, not arbitrary Looking Glass IDs). `params.count` defaults to `4` (`1`–`20`), `timeout_sec` defaults to `15` (capped at `60`). Results are stored in `data.db` (with parsed RTT/loss and `ParsedJSON`), rolled up into 5-minute summaries (`ok`/`degraded`/`failed`), shown on the node's Cron Tasks page, and repeated failures record a fleet event. Cron tasks are managed from **Cron Tasks** or `/api/cron-tasks` (`GET/POST /api/cron-tasks`, `PUT/DELETE /api/cron-tasks/{id}`, `POST …/toggle`, `GET /api/nodes/{name}/cron-tasks/results`); the apply schema accepts `cron_tasks`, but the current apply engine does not merge that section. A task whose selector matches no managed node is rejected on save.

### Ping Table

The Looking Glass **Ping Table** measures latency between every pair of agent nodes: `e2e` (end-to-end via the overlay) or `link` (direct link interface/address when a direct link exists, otherwise derived IPv6 link-local). One run fans out to `N*(N-1)` directed pings (`ping` count `3`, 4s timeout, 5s context) with at most 25 concurrent executions and per-source-node serialization; statuses include `pending`/`running`/`success`/`failed`/`unreachable`/`no_agent`/`no_link`/`self`/`cancelled`/`completed`. API: `POST /api/looking-glass/ping-table/run|cancel`, `GET …/current|/history|/tasks/{id}|/stream` (SSE), `DELETE …/tasks/{id}`. History is persisted in `data.db` (`ping_table_tasks`).
