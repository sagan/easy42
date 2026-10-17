# easy42 Grafana Dashboards

This directory contains pre-configured, production-ready sample Grafana dashboards (in JSON format) designed to visualize metrics exported by the **easy42** `/metrics` Prometheus endpoint.

## Available Dashboards

| Dashboard File | UID | Description | Target Use Case |
| :--- | :--- | :--- | :--- |
| [`easy42-cluster-overview.json`](file:///root/projects/easy42/docs/grafana/easy42-cluster-overview.json) | `easy42-cluster-overview` | Executive and operational overview of the entire easy42 mesh: fleet status, node reachability, topology links, WireGuard bandwidth, BGP peering, and system load highlights. | NOC / Executive / Daily Cluster Operations |
| [`easy42-wireguard-mesh.json`](file:///root/projects/easy42/docs/grafana/easy42-wireguard-mesh.json) | `easy42-wireguard-mesh` | In-depth networking telemetry focusing on WireGuard tunnels (peer handshakes, keepalive, bidirectional Rx/Tx rates) and BIRD BGP session states. | Network Engineering & Mesh Peering |
| [`easy42-node-system.json`](file:///root/projects/easy42/docs/grafana/easy42-node-system.json) | `easy42-node-system` | Detailed host OS & hardware telemetry (node_exporter compatible): multi-core CPU %, load averages (1m/5m/15m), memory allocations, disk partition usage, and physical network interfaces. | Infrastructure & Host Performance Troubleshooting |

---

## Prerequisites

1. **Prometheus Scraper Configured**: Ensure Prometheus or VictoriaMetrics is scraping easy42 `/metrics` with the configured Bearer token:

   ```yaml
   scrape_configs:
     - job_name: 'easy42'
       scrape_interval: 15s
       metrics_path: '/metrics'
       scheme: 'http' # or https
       bearer_token: 'YOUR_PROMETHEUS_AUTH_TOKEN'
       static_configs:
         - targets: ['127.0.0.1:4242']
   ```

2. **Grafana**: Version 9.0+ or 10.0+ (dashboards use `schemaVersion: 38`, compatible with Grafana 9, 10, and 11).

---

## How to Import into Grafana

### Option A: Via Grafana Web UI

1. Open your Grafana web interface.
2. Navigate to **Dashboards** $\to$ **New** $\to$ **Import** (or visit `/dashboard/import`).
3. Click **Upload dashboard JSON file** and select one of the JSON files from this directory, or paste the raw JSON contents into the text area.
4. When prompted:
   - Select your Prometheus data source from the dropdown.
   - Click **Import**.

### Option B: Automated Provisioning

You can mount these files into your Grafana dashboard provisioning directory (e.g. `/etc/grafana/provisioning/dashboards`):

```yaml
# /etc/grafana/provisioning/dashboards/easy42.yaml
apiVersion: 1

providers:
  - name: 'easy42'
    orgId: 1
    folder: 'easy42 Mesh'
    type: file
    disableDeletion: false
    editable: true
    options:
      path: /etc/grafana/dashboards/easy42
```

Copy the `.json` files into `/etc/grafana/dashboards/easy42/`.

### Option C: Via Grafana REST API

```bash
GRAFANA_URL="http://localhost:3000"
GRAFANA_API_KEY="eyJrIjoi..." # or service account token

for f in easy42-cluster-overview.json easy42-wireguard-mesh.json easy42-node-system.json; do
  curl -s -X POST \
    -H "Authorization: Bearer ${GRAFANA_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"dashboard\": $(cat $f), \"overwrite\": true}" \
    "${GRAFANA_URL}/api/dashboards/db" | jq .
done
```

---

## Dashboard Details & Features

### 1. easy42 Cluster Overview (`easy42-cluster-overview.json`)

- **Header KPIs**:
  - Total Configured Nodes vs. Connected Agents.
  - Overall Agent Connectivity Health Ratio (%).
  - Configured Mesh Links vs. Operational Links.
  - Active WireGuard Peers & Established BGP Sessions count.
  - Real-time Total Mesh Bandwidth (sum of WireGuard Rx and Tx rates).
- **Fleet Inventory & Status Table**:
  - Node Name, Connection State (Online / Offline badge), Overlay IPv4, Overlay IPv6, ASN, Mode, Agent Version, OS, and Host FQDN.
- **Node Connectivity Timeline**:
  - Historical state-timeline visualizing connection uptime and disconnect events per node.
- **Mesh Links Status**:
  - Table of configured mesh links showing source node, destination node, interfaces, and operational UP/DOWN status.
- **WireGuard Peer Traffic & Handshakes**:
  - Bidirectional throughput graphs per peer.
  - Handshake delay tracking with warning threshold lines at 180s and critical at 300s.
  - Peer status table with endpoint IP:Port and public keys.
- **BIRD / BGP Telemetry**:
  - BGP session status history and BIRD protocol inventory.
- **Host Resource Highlights**:
  - Comparative timeseries for CPU %, Memory %, Load 1m, and interface bandwidth across all nodes.

### 2. easy42 WireGuard & BGP Mesh (`easy42-wireguard-mesh.json`)

- **Interactive Variables**:
  - `$datasource`: Select Prometheus instance.
  - `$node`: Filter by source node.
  - `$interface`: Filter by WireGuard interface name (e.g. `wg42peer1`).
  - `$peer`: Filter by peer node friendly name.
- **Handshake Quality & Delay**:
  - Exact time elapsed since last handshake for each peer.
  - Automatic detection of stale tunnels (> 180s without completed handshake).
- **Throughput & Bandwidth Rates**:
  - High-resolution Rx and Tx throughput graphs.
  - Identification of top talkers in the mesh.
- **Dynamic Routing (BIRD)**:
  - BGP session uptime and state transitions.
  - BIRD protocol health monitoring.

### 3. easy42 Node System Metrics (`easy42-node-system.json`)

- **Compatibility**: Uses Prometheus metrics exposed with standard `node_exporter` naming conventions (`node_load*`, `node_cpu_*`, `node_memory_*`, `node_filesystem_*`, `node_network_*`).
- **Interactive Variables**:
  - `$node`: Multi-select or single-select node filter.
- **Hardware Telemetry**:
  - Multi-series CPU utilization % and 1m / 5m / 15m load averages.
  - Memory breakdown (Total, Used, Available, Free) and utilization gauge.
  - Storage partitions: disk space used/free and percentage bar gauge per mount point.
  - Network devices: per-interface bandwidth (Rx/Tx bytes/s), packet rates (pps), and error rates.

---

## Metrics Reference

The dashboards rely on the following metrics exposed by easy42:

| Metric Family | Metric Name | Type | Description |
| :--- | :--- | :---: | :--- |
| **Cluster State** | `easy42_total_nodes` | Gauge | Total nodes configured in easy42 cluster |
| | `easy42_connected_nodes` | Gauge | Number of active connected node agents |
| | `easy42_total_links` | Gauge | Total configured node-to-node links |
| | `easy42_node_up` | Gauge | Connection status of node agent (`1` = up, `0` = down) |
| | `easy42_node_info` | Gauge | Node metadata labels (`hostname`, `agent_version`, `os`, `asn`, `ip`, `ip6`, `mode`) |
| | `easy42_link_up` | Gauge | Operational status of link (`1` = up, `0` = down) |
| | `easy42_scrape_duration_seconds` | Gauge | Scrape duration in seconds |
| **WireGuard** | `wireguard_peer_connected` | Gauge | `1` if peer completed handshake within 180s, `0` otherwise |
| | `wireguard_latest_handshake_seconds` | Gauge | Unix timestamp of latest WireGuard handshake |
| | `wireguard_latest_handshake_delay_seconds` | Gauge | Elapsed seconds since last handshake |
| | `wireguard_received_bytes_total` | Counter | Total bytes received from WireGuard peer |
| | `wireguard_sent_bytes_total` | Counter | Total bytes sent to WireGuard peer |
| | `wireguard_interface_up` | Gauge | `1` if WireGuard interface is up, `0` otherwise |
| | `wireguard_peer_persistent_keepalive_interval_seconds` | Gauge | Persistent keepalive interval in seconds |
| **BIRD / BGP** | `bird_protocol_up` | Gauge | `1` if BIRD protocol is up, `0` otherwise |
| | `bird_protocol_state` | Gauge | State gauge per protocol (`up`, `down`, `start`, etc.) |
| | `bird_bgp_session_established` | Gauge | `1` if BGP session is in Established state, `0` otherwise |
| **System** | `node_load1`, `node_load5`, `node_load15` | Gauge | Linux system load averages |
| | `node_cpu_percent`, `node_cpu_utilization_ratio` | Gauge | CPU utilization percentage and ratio |
| | `node_memory_MemTotal_bytes`, `node_memory_used_bytes`, `node_memory_MemAvailable_bytes`, `node_memory_MemFree_bytes` | Gauge | System physical memory statistics in bytes |
| | `node_uptime_seconds`, `node_boot_time_seconds` | Gauge | Host uptime and boot timestamp |
| | `node_filesystem_size_bytes`, `node_filesystem_used_bytes`, `node_filesystem_free_bytes`, `node_filesystem_avail_bytes` | Gauge | Partition disk space usage |
| | `node_network_receive_bytes_total`, `node_network_transmit_bytes_total` | Counter | Network interface throughput counters |
| | `node_network_receive_packets_total`, `node_network_transmit_packets_total` | Counter | Network interface packet counters |
| | `node_network_receive_errs_total`, `node_network_transmit_errs_total` | Counter | Network interface error counters |
| | `node_network_up` | Gauge | `1` if network device is up, `0` otherwise |
