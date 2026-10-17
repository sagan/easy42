# Configuration Specification (`config.json`)

This document provides the full reference specification for Easy42's state file, stored in `<data-dir>/config.json` (default: `~/.config/easy42/config.json`).

---

## Annotated Configuration Example

```json
{
  "password_hash": "$argon2id$v=19$m=65536,t=3,p=4$...",
  "dek_encrypted": "base64_encoded_encrypted_dek",
  "salt": "base64_encoded_salt",
  "network_settings": {
    "public_asn": 4242421234,
    "prefixes": [
      "172.20.150.0/24",
      "fd42:a159:f9f0::/48"
    ],
    "disallowed_dn42_networks": [
      "172.20.0.0/14",
      "fd00::/8"
    ],
    "flapalerted_address": "fd42:a159:f9f0:8000::c8",
    "flapalerted_port": 1790,
    "flapalerted_asn": 4224420000,
    "flapalerted_url": "http://[fd42:a159:f9f0:8000::c8]:8699"
  },
  "dns": {
    "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
    "api_token": "cf-token-...",
    "base_domain": "mesh.example.com",
    "publish_ipv6_own_name": false
  },
  "database": {
    "retention_days": 47
  },
  "integrations": {
    "prometheus_enabled": true,
    "prometheus_auth_token": "e42_prom_a1b2c3d4e5f6..."
  },
  "blocks": [
    {
      "id": "blk-eu",
      "name": "Europe Region",
      "color": "#3B82F6",
      "x": 100,
      "y": 150,
      "nodes": ["node-fra", "node-lon"]
    }
  ],
  "nodes": [
    {
      "name": "node-fra",
      "host": "vps-fra.example.com",
      "mode": "ssh",
      "ip": "192.168.100.1",
      "ip6": "fd42:a159:f9f0::1",
      "external_ip": "172.20.150.1",
      "interface": "lo",
      "asn": 4224420001,
      "entrypoints": [
        {
          "ip": "198.51.100.1",
          "ports": [51820, "20000-29999"],
          "tags": ["public", "fra"]
        }
      ],
      "table": 254,
      "external_table": 100,
      "internet_table": 200,
      "metric": 50,
      "static_routes": ["172.20.150.0/24"],
      "routes": [
        {
          "table": 100,
          "prefixes": ["10.200.0.0/16"]
        }
      ],
      "enable_fa": true,
      "update_hosts": true,
      "tags": ["cloud", "eu"]
    }
  ],
  "links": [
    {
      "type": "wireguard",
      "assign_ipv4": true,
      "psk": "encrypted_base64_wireguard_psk",
      "from": {
        "name": "node-fra",
        "interface": "wg42lon",
        "address": "fe80::c0a8:6401/64",
        "address4": "169.254.120.1/32",
        "listen_port": 21820,
        "public_key": "vS5Vz...",
        "policy": "default",
        "cost": -10
      },
      "to": {
        "name": "node-lon",
        "interface": "wg42fra",
        "address": "fe80::c0a8:6402/64",
        "address4": "169.254.120.2/32",
        "listen_port": 21821,
        "public_key": "k9L1p...",
        "policy": "default",
        "cost": -10
      }
    }
  ]
}
```

---

## Schema Reference

### `network_settings`

| Field | Type | Description |
| :--- | :--- | :--- |
| `public_asn` | `integer` | Your public or DN42 ASN (e.g. `4242421234`). |
| `prefixes` | `string[]` | IP prefixes owned by your ASN advertised into the mesh. |
| `disallowed_dn42_networks`| `string[]` | CIDR blocks strictly filtered from export to external peers. |
| `flapalerted_address` | `string` | IP address of external FlapAlerted service for route flap tracking. |
| `flapalerted_port` | `integer` | BGP port for FlapAlerted session (default: `1790`). |
| `flapalerted_asn` | `integer` | BGP ASN of the FlapAlerted server. |
| `flapalerted_url` | `string` | Web URL to the FlapAlerted dashboard. |

### `nodes`

| Field | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | Unique identifier and hostname of the node. |
| `host` | `string` | Hostname or IP used for SSH connectivity. |
| `mode` | `string` | `"ssh"` (agent-less) or `"agent"` (`easy42-agent`). |
| `ip` | `string` | Primary overlay IPv4 address (bound to loopback). |
| `ip6` | `string` | Primary overlay IPv6 address. |
| `external_ip` | `string` | Public/External IP used for SNAT egress. |
| `interface` | `string` | Local interface holding the primary IP (default: `"lo"`). |
| `asn` | `integer` | Private BGP AS number (e.g. `4224420001`). |
| `entrypoints` | `object[]` | List of public/reachable listening endpoints. |
| `table` | `integer` | Kernel FIB routing table (default: `254` - main). |
| `external_table` | `integer` | Optional kernel table for policy-routed external traffic. |
| `internet_table` | `integer` | Optional kernel table for upstream default transit. |
| `metric` | `integer` | Kernel route metric (default: `100`). |
| `static_routes` | `string[]` | Subnets unconditionally advertised into BGP. |
| `routes` | `object[]` | Kernel table route redistribution rules. |
| `enable_fa` | `boolean` | Enable BGP telemetry session to FlapAlerted. |
| `update_hosts` | `boolean` | Automatically synchronize `/etc/hosts` on this node. |
| `tags` | `string[]` | Metadata tags for grouping and endpoint matching. |

### `links`

| Field | Type | Description |
| :--- | :--- | :--- |
| `type` | `string` | `"wireguard"` or `"manual"`. |
| `assign_ipv4` | `boolean` | Assign deterministic `169.254.X.X/32` IPv4 link-local address. |
| `psk` | `string` | Encrypted base64 WireGuard preshared key. |
| `from`, `to` | `object` | Endpoints of the link (`name`, `interface`, `address`, `cost`, `policy`, `public_key`, `listen_port`). |

### `database`

| Field | Type | Description |
| :--- | :--- | :--- |
| `retention_days` | `integer` | Number of days to retain metrics and probe logs before pruning (default: `47`). |

### `integrations`

| Field | Type | Description |
| :--- | :--- | :--- |
| `prometheus_enabled` | `boolean` | Enable or disable the `/metrics` export endpoint. |
| `prometheus_auth_token`| `string` | Bearer token required to scrape Prometheus metrics. |
