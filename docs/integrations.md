# Integrations & Extended Features

This document describes the third-party integrations and advanced management subsystems available in **Easy42**, including Prometheus monitoring, `/etc/hosts` management, WireGuard PSK support, database retention, and Cloudflare DNS automation.

---

## 1. Prometheus Metrics Exporter

Easy42 includes a built-in Prometheus exposition endpoint serving cluster-wide telemetry in standard Prometheus text format (version 0.0.4).

### Endpoint & Authentication

- **Endpoint**: `/metrics`
- **Authentication**: Requires a bearer token configured in **Settings -> Integrations**.
- **Auth Header**: `Authorization: Bearer <token>` or URL parameter `?token=<token>`
- **Caching**: State is cached in memory for 2 seconds with double-checked locking to prevent CPU spikes under high-frequency scraping.

### Prometheus Configuration Example

Add a scrape job to `/etc/prometheus/prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'easy42'
    scrape_interval: 15s
    metrics_path: '/metrics'
    scheme: 'http' # or https
    static_configs:
      - targets: ['127.0.0.1:4242']
    bearer_token: 'e42_prom_your_auth_token_here'
```

### Sample Grafana Dashboards

Easy42 provides pre-built, production-ready sample Grafana dashboards located in [`docs/grafana/`](file:///root/projects/easy42/docs/grafana/):

- **[easy42 - Cluster Overview](file:///root/projects/easy42/docs/grafana/easy42-cluster-overview.json)** (`easy42-cluster-overview`): High-level cluster health, node fleet inventory, topology link connectivity, WireGuard throughput, and resource highlights.
- **[easy42 - WireGuard & BGP Mesh](file:///root/projects/easy42/docs/grafana/easy42-wireguard-mesh.json)** (`easy42-wireguard-mesh`): In-depth tunnel latency, handshake tracking (> 180s alert threshold), bidirectional Rx/Tx rates, and BIRD dynamic routing state transitions.
- **[easy42 - Node System Metrics](file:///root/projects/easy42/docs/grafana/easy42-node-system.json)** (`easy42-node-system`): Host-level OS and hardware performance (CPU %, 1m/5m/15m load, memory allocation, disk partitions, and physical network interfaces).

For step-by-step import instructions and provisioning configurations, see [Grafana Dashboards Guide](file:///root/projects/easy42/docs/grafana/README.md).

### Exported Metrics Catalog

Easy42 exports Prometheus metrics organized into four major functional domains:

#### A. easy42 Cluster State & Topology

| Metric Name | Type | Description | Labels |
| :--- | :---: | :--- | :--- |
| `easy42_exporter_build_info` | Gauge | easy42 build and version information | `version`, `commit` |
| `easy42_scrape_duration_seconds` | Gauge | Duration of metrics collection in seconds | — |
| `easy42_total_nodes` | Gauge | Total configured nodes in the cluster | — |
| `easy42_connected_nodes` | Gauge | Number of active connected WebSocket agents | — |
| `easy42_total_links` | Gauge | Total configured node-to-node links | — |
| `easy42_node_info` | Gauge | Static node metadata | `node`, `hostname`, `agent_version`, `os`, `asn`, `ip`, `ip6`, `mode`, `is_external` |
| `easy42_node_up` | Gauge | Agent connection status (`1` = connected, `0` = offline) | `node` |
| `easy42_node_last_seen_timestamp_seconds` | Gauge | Unix timestamp when node agent was last seen | `node` |
| `easy42_link_up` | Gauge | Operational status of link (`1` = up, `0` = down) | `from_node`, `to_node`, `from_interface`, `to_interface` |

#### B. WireGuard Telemetry (MindFlavor Compatible)

| Metric Name | Type | Description | Labels |
| :--- | :---: | :--- | :--- |
| `wireguard_peer_connected` | Gauge | `1` if peer completed handshake within 180s, `0` otherwise | `node`, `interface`, `public_key`, `peer_node`, `friendly_name` |
| `wireguard_latest_handshake_seconds` | Gauge | Unix timestamp of latest WireGuard handshake | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_latest_handshake_delay_seconds` | Gauge | Elapsed seconds since last handshake | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_received_bytes_total` | Counter | Total bytes received from WireGuard peer | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_sent_bytes_total` | Counter | Total bytes sent to WireGuard peer | `node`, `interface`, `public_key`, `endpoint`, `peer_node`, `friendly_name` |
| `wireguard_interface_up` | Gauge | `1` if WireGuard interface is UP, `0` otherwise | `node`, `interface` |
| `wireguard_peer_persistent_keepalive_interval_seconds` | Gauge | Persistent keepalive interval in seconds | `node`, `interface`, `public_key`, `peer_node`, `friendly_name` |

#### C. BIRD / BGP Dynamic Routing (bird_exporter Compatible)

| Metric Name | Type | Description | Labels |
| :--- | :---: | :--- | :--- |
| `bird_protocol_up` | Gauge | `1` if BIRD protocol is up, `0` otherwise | `node`, `name`, `proto`, `peer_node` |
| `bird_protocol_state` | Gauge | Protocol state gauge (`1` on matching `state`) | `node`, `name`, `proto`, `state` |
| `bird_bgp_session_established` | Gauge | `1` if BGP session is in Established state, `0` otherwise | `node`, `name`, `peer_node` |

#### D. Linux Host System (node_exporter Compatible)

| Metric Name | Type | Description | Labels |
| :--- | :---: | :--- | :--- |
| `node_cpu_percent` | Gauge | CPU utilization percentage (0.0 to 100.0) | `node` |
| `node_cpu_utilization_ratio` | Gauge | CPU utilization ratio (0.0 to 1.0) | `node` |
| `node_load1`, `node_load5`, `node_load15` | Gauge | 1, 5, and 15-minute load averages | `node` |
| `node_memory_MemTotal_bytes` | Gauge | Total physical memory in bytes | `node` |
| `node_memory_used_bytes` | Gauge | Memory currently used in bytes | `node` |
| `node_memory_MemAvailable_bytes` | Gauge | Available memory in bytes | `node` |
| `node_memory_MemFree_bytes` | Gauge | Free memory in bytes | `node` |
| `node_uptime_seconds` | Gauge | Host uptime in seconds | `node` |
| `node_boot_time_seconds` | Gauge | Node boot time in Unix epoch seconds | `node` |
| `node_filesystem_size_bytes` | Gauge | Partition total size in bytes | `node`, `mountpoint` |
| `node_filesystem_used_bytes` | Gauge | Partition used space in bytes | `node`, `mountpoint` |
| `node_filesystem_free_bytes` | Gauge | Partition free space in bytes | `node`, `mountpoint` |
| `node_filesystem_avail_bytes` | Gauge | Partition space available to non-root users | `node`, `mountpoint` |
| `node_network_receive_bytes_total` | Counter | Network device received bytes | `node`, `device` |
| `node_network_transmit_bytes_total` | Counter | Network device transmitted bytes | `node`, `device` |
| `node_network_receive_packets_total` | Counter | Network device received packets | `node`, `device` |
| `node_network_transmit_packets_total` | Counter | Network device transmitted packets | `node`, `device` |
| `node_network_receive_errs_total` | Counter | Network device receive errors | `node`, `device` |
| `node_network_transmit_errs_total` | Counter | Network device transmit errors | `node`, `device` |
| `node_network_up` | Gauge | Network device operational status (`1` = up, `0` = down) | `node`, `device` |

---

## 2. /etc/hosts Automatic Management

Easy42 can automatically synchronize mesh node hostnames and IP addresses to `/etc/hosts` on all managed nodes, eliminating the need to run an internal DNS server for node-to-node name resolution.

### How It Works

- Enabled per node by toggling `update_hosts: true` (or via the **Update /etc/hosts** checkbox in node settings).
- During configuration sync, Easy42 injects or updates a managed block in `/etc/hosts`:

```
# BEGIN easy42 managed hosts
192.168.100.1    node-frankfurt
fd42:a159:f9f0::1 node-frankfurt
192.168.100.2    node-london
fd42:a159:f9f0::2 node-london
# END easy42 managed hosts
```

- Content outside the `# BEGIN` and `# END` markers is preserved untouched.
- Applied safely over SSH or via the agent's atomic file write protocol.

---

## 3. WireGuard PSK (Preshared Key) Support

Easy42 supports WireGuard **Preshared Keys (PSK)** to provide an additional layer of post-quantum symmetric encryption on top of the standard Curve25519 key exchange.

### Key Capabilities

- **Per-Link PSK**: Configure PSK on any WireGuard link between two nodes.
- **Encrypted Storage**: PSKs are encrypted at rest using **XAES-256-GCM** in `config.json` alongside private keys.
- **Easy Generation**: Generate secure base64 256-bit PSKs with a single click in the Web UI or via CLI:
  ```bash
  easy42 link add node-fra node-lon --psk "<base64_psk>"
  ```
- **Zero-Downtime Key Rotation**: Update PSKs on active links and apply changes across both peers simultaneously via `easy42 sync`.

---

## 4. Database Management & Data Retention

Easy42 uses an embedded SQLite database (`easy42.db`) located in the data directory to store time-series metrics, historical ping results, and interface traffic statistics.

### Configurable Retention Period

- Configure `retention_days` in **Settings -> Database** (default: `47` days).
- A background maintenance task cleans data older than the retention threshold every 24 hours.

### Downsampling & Storage Optimization

- **Recent Data (Full Fidelity)**: Raw per-second/minute telemetry and probe points are preserved for recent periods.
- **Older Data (Statistical Aggregation)**: Data approaching the retention horizon is automatically rolled up into hourly statistical aggregates (min, avg, max, sample count), reducing database growth by up to 90%.
- **Database Maintenance Actions**:
  - **Vacuum**: Reclaims unused disk space and defragments SQLite pages.
  - **Database Size Monitoring**: Live database size tracking displayed directly in the Web UI.

---

## 5. Cloudflare DNS Automation

Easy42 features native Cloudflare DNS integration to publish overlay IP addresses to public or private DNS zones.

### Configuration

Set up DNS parameters in `config.json` or through **Settings -> Cloudflare DNS**:

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

- `A` record: `<node>.<base_domain>` $\to$ Node overlay IPv4.
- `AAAA` record: `<node>.<base_domain>` $\to$ Node overlay IPv6.
- Wildcard record: `*.<node>.<base_domain>` $\to$ Node overlay IPv4 and IPv6 (useful for routing wildcard subdomains or services).
- Dedicated IPv6 record (optional): `<node>6.<base_domain>` $\to$ Node overlay IPv6.

---

## 6. Config Templates Engine & Hooks

Easy42 compiles configuration files using Go's `text/template` engine, enabling full customization:

- **Built-in Templates**: `default_wg` (WireGuard), `default_bird` (BIRD 2), `default_nft` (nftables).
- **Custom Templates**: Define user templates in Web UI or API and attach them globally or to specific nodes.
- **Node Lifecycle Hooks**: Inject raw configuration blocks into specific compilation hooks:
  - `bird.pre`: Injected before all BIRD protocol declarations.
  - `bird`: Appended to the end of the BIRD configuration.
  - `wg.interface`: Injected into the `[Interface]` section of WireGuard configs.
  - `wg.peer`: Injected into the `[Peer]` section of WireGuard configs.
  - `wg.post`: Appended to the end of WireGuard interface files.
  - `nft.pre`: Injected before table declarations in `/etc/easy42.nft`.
  - `nft.table`: Injected inside `table inet easy42 { ... }`.
