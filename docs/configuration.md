# Configuration Specification (`config.json`)

This document is the reference for Easy42's state file, stored in `<data-dir>/config.json` (default: `~/.config/easy42/config.json`, mode `0600`). The file is managed by the server and CLI, but it is plain JSON and can be edited declaratively with [`easy42 apply`](#declarative-config-apply).

> **WARNING:** `password_hash`, `encrypted_dek`, and `session_secret` are security-critical. Back up `config.json` before manual edits and never commit it to version control. Declarative apply deliberately forbids changing these fields. Other credentials are also stored in plaintext in `config.json` (see [Security Fields](#security-fields)); protect the file (mode `0600`) and any backups.

---

- [Configuration Specification (`config.json`)](#configuration-specification-configjson)
  - [Top-Level Structure](#top-level-structure)
  - [Annotated Example](#annotated-example)
  - [Schema Reference](#schema-reference)
    - [Security Fields](#security-fields)
    - [`network_settings`](#network_settings)
    - [`network_policies`](#network_policies)
    - [`nodes`](#nodes)
      - [Entrypoints](#entrypoints)
      - [Kernel route rules (`routes`)](#kernel-route-rules-routes)
      - [Config hooks (`config_hooks`)](#config-hooks-config_hooks)
    - [`links`](#links)
    - [`blocks`](#blocks)
    - [`dns`](#dns)
    - [`templates`](#templates)
    - [`looking_glass_tasks`](#looking_glass_tasks)
    - [`cron_tasks`](#cron_tasks)
    - [`server_tasks`](#server_tasks)
    - [`database`](#database)
    - [`integrations`](#integrations)
    - [`mcp`](#mcp)
    - [`apps` and `app_variables`](#apps-and-app_variables)
    - [`config_changes`](#config_changes)
  - [Declarative Config Apply](#declarative-config-apply)
    - [Allowed patch sections](#allowed-patch-sections)
    - [Forbidden fields](#forbidden-fields)
    - [Merge and validation rules](#merge-and-validation-rules)
    - [Side effects](#side-effects)
    - [Example patch](#example-patch)
    - [Related API endpoints](#related-api-endpoints)
  - [Data Files](#data-files)

## Top-Level Structure

| Field                 | Type       | Description                                                                                                           |
| :-------------------- | :--------- | :-------------------------------------------------------------------------------------------------------------------- |
| `password_hash`       | `string`   | Argon2id hash of the admin password (`m=65536,t=3,p=2`).                                                              |
| `encrypted_dek`       | `string`   | Base64 of `salt(16B) + nonce(24B) + ciphertext`, an XAES-256-GCM encryption of the 32-byte data encryption key (DEK). |
| `session_secret`      | `string`   | Random secret used to sign browser sessions and short-lived CLI session tokens.                                       |
| `network_settings`    | `object`   | Global ASN, peering prefixes, DN42 filters, and FlapAlerted settings.                                                 |
| `network_policies`    | `object[]` | User-defined routing/firewall policies. Built-in policies are always added at runtime.                                |
| `nodes`               | `object[]` | Managed servers and external peers.                                                                                   |
| `links`               | `object[]` | WireGuard or manual point-to-point links between nodes.                                                               |
| `looking_glass_tasks` | `object[]` | Custom Looking Glass tasks (built-ins are added at runtime).                                                          |
| `cron_tasks`          | `object[]` | Recurring ping / TCP-ping probes executed by agents.                                                                  |
| `apps`                | `object[]` | Workload apps (Docker Compose, systemd, cron) deployed to agent nodes. See [Apps](#apps-and-app_variables).            |
| `app_variables`       | `object[]` | Global variables interpolated into app manifests/env files. See [Apps](#apps-and-app_variables).                         |
| `blocks`              | `object[]` | Visual grouping rectangles on the topology graph.                                                                     |
| `dns`                 | `object`   | Cloudflare DNS synchronization settings.                                                                              |
| `templates`           | `object[]` | Custom WireGuard, BIRD, and nftables Go templates.                                                                    |
| `server_tasks`        | `object[]` | Schedules for the four fixed server background tasks.                                                                 |
| `database`            | `object`   | Summarized and raw telemetry retention.                                                                               |
| `integrations`        | `object`   | Prometheus and Telegram Bot settings.                                                                                 |
| `mcp`                 | `object`   | Model Context Protocol server settings and tokens. See [`mcp`](#mcp).                                                  |
| `config_changes`      | `object[]` | Saved declarative config snippets displayed in the UI.                                                                |

---

## Annotated Example

```json
{
  "password_hash": "$argon2id$v=19$m=65536,t=3,p=2$...",
  "encrypted_dek": "base64(salt + nonce + ciphertext)",
  "session_secret": "base64_random_secret",
  "network_settings": {
    "public_asn": 4242421234,
    "prefixes": ["172.20.0.0/14{21,29}", "fd00::/8{44,64}"],
    "disallowed_dn42_networks": ["172.20.0.0/16", "fd00:dead:beef::/48"],
    "flapalerted_address": "fd42:a159:f9f0:8000::c8",
    "flapalerted_port": 1790,
    "flapalerted_asn": 4224420000,
    "flapalerted_url": "http://[fd42:a159:f9f0:8000::c8]:8699"
  },
  "network_policies": [
    {
      "id": "transit",
      "name": "Transit Customer",
      "description": "Receive all routes, advertise only local prefixes",
      "cost": 150,
      "routing_policy": "advertise_only",
      "reject_internet": true,
      "filter_forward": true,
      "filter_input": true,
      "input_allow_icmp": true,
      "input_allow_icmp6": true,
      "input_tcp_ports": ["22", "443"],
      "roa4": "https://dn42.burble.com/roa/dn42_roa_bird2_4.conf",
      "roa6": "https://dn42.burble.com/roa/dn42_roa_bird2_6.conf",
      "roa_strict": true
    }
  ],
  "nodes": [
    {
      "name": "node-fra",
      "host": "vps-fra.example.com",
      "caption": "Frankfurt edge",
      "description": "Contact: noc@example.com",
      "note": "Managed by **Ansible**",
      "ip": "192.168.100.1",
      "ip6": "fd42:a159:f9f0::1",
      "external_ip": "172.20.150.1",
      "external_ip6": "fd42:a159:f9f0::d",
      "interface": "lo",
      "asn": 4224420001,
      "mode": "ssh",
      "entrypoints": [
        {
          "ip": "198.51.100.1",
          "ports": [51820, "20000-29999", { "port": 443, "external_port": 8443 }],
          "tags": ["public", "fra"],
          "mtu": 1420
        }
      ],
      "table": 254,
      "external_table": 100,
      "internet_table": 200,
      "metric": null,
      "static_routes": ["172.20.150.0/24"],
      "routes": [{ "table": 100, "prefixes": ["10.200.0.0/16"] }],
      "config_hooks": [{ "type": "bird", "content": "include \"/etc/bird/custom.conf\";" }],
      "enable_fa": true,
      "update_hosts": true,
      "tags": ["cloud", "eu"],
      "x": 120,
      "y": 80
    }
  ],
  "links": [
    {
      "type": "wireguard",
      "assign_ipv4": true,
      "disable_bfd": false,
      "psk": "base64_encrypted_preshared_key",
      "tags": ["backbone"],
      "from": {
        "name": "node-fra",
        "interface": "wg42lon",
        "address": "fe80::c0a8:6401/64",
        "address4": "169.254.120.1/32",
        "listen_port": 21820,
        "public_key": "vS5Vz...",
        "policy": "default",
        "cost": 10,
        "persistent_keepalive": 25,
        "mtu": 1420
      },
      "to": {
        "name": "node-lon",
        "interface": "wg42fra",
        "address": "fe80::c0a8:6402/64",
        "address4": "169.254.120.2/32",
        "listen_port": 21821,
        "public_key": "k9L1p...",
        "policy": "default",
        "cost": 10,
        "persistent_keepalive": 25,
        "mtu": 1420
      }
    }
  ],
  "blocks": [
    {
      "id": "blk-eu",
      "name": "Europe",
      "color": "#3B82F6",
      "x": 100,
      "y": 150,
      "width": 260,
      "height": 180,
      "nodes": ["node-fra", "node-lon"]
    }
  ],
  "dns": {
    "zone_id": "023e105f4ecef8ad9ca31a8372d0c353",
    "api_token": "cf-token-...",
    "base_domain": "mesh.example.com",
    "publish_ipv6_own_name": false
  },
  "templates": [
    {
      "id": "custom_wg_edge",
      "name": "Edge WireGuard",
      "type": "wg",
      "description": "Adds a per-interface PostUp rule",
      "content": "{{/* Go text/template */}}",
      "is_builtin": false
    }
  ],
  "server_tasks": [
    { "id": "update_roa", "cron_expr": "0 0 * * *", "enabled": true },
    { "id": "sync", "cron_expr": "30 0 * * *", "enabled": true },
    { "id": "sync_dns", "cron_expr": "0 1 * * *", "enabled": true },
    { "id": "backup_config", "cron_expr": "0 2 * * *", "enabled": true }
  ],
  "cron_tasks": [
    {
      "id": "probe-core",
      "name": "Probe core",
      "description": "ICMP reachability to the core router",
      "task_type": "ping",
      "cron_expr": "*/5 * * * *",
      "selector": { "name": "node-fra" },
      "params": { "target": "172.20.0.53", "count": "4" },
      "timeout_sec": 20,
      "enabled": true
    }
  ],
  "database": {
    "retention_days": 47,
    "raw_retention_days": 2
  },
  "integrations": {
    "prometheus_enabled": true,
    "prometheus_auth_token": "e42_prom_...",
    "telegram_enabled": false,
    "telegram_bot_token": "",
    "telegram_admin_id": 0,
    "telegram_secret_token": "",
    "server_url": ""
  },
  "mcp": {
    "enabled": true,
    "read_only_token": "e42_mcp_ro_...",
    "full_control_token": "e42_mcp_full_...",
    "oauth_client_id": "easy42-mcp",
    "oauth_client_secret": "e42_sec_..."
  },
  "apps": [
    {
      "id": "app-uuid",
      "name": "my-service",
      "description": "Example Docker Compose app",
      "note": "",
      "url": "https://example.com/my-service",
      "app_type": "docker",
      "main_content": "services:\n  web:\n    image: nginx:latest\n",
      "env_content": "APP_ENV=production\n",
      "files": {},
      "selector": {},
      "published": true,
      "enabled": true,
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-01T00:00:00Z"
    }
  ],
  "app_variables": [
    { "key": "APP_DOMAIN", "value": "example.com", "description": "", "is_secret": false, "updated_at": "2026-01-01T00:00:00Z" }
  ],
  "config_changes": [
    {
      "id": "0b0e...",
      "name": "Add Frankfurt edge",
      "description": "Declarative onboarding snippet",
      "content": "nodes:\n  - name: node-fra\n    host: vps-fra.example.com\n",
      "format": "yaml",
      "created_at": "2026-01-01T00:00:00Z",
      "modified_at": "2026-01-01T00:00:00Z"
    }
  ]
}
```

---

## Schema Reference

### Security Fields

| Field            | Type     | Description                                                                                                        |
| :--------------- | :------- | :----------------------------------------------------------------------------------------------------------------- |
| `password_hash`  | `string` | Argon2id encoded hash (`$argon2id$v=19$m=65536,t=3,p=2$salt$hash`).                                                |
| `encrypted_dek`  | `string` | Base64 `salt(16) + nonce(24) + XAES-256-GCM ciphertext`. The KEK is derived from the admin password with Argon2id. |
| `session_secret` | `string` | Random signing key for browser sessions and CLI-to-server session tokens.                                          |

WireGuard private keys and preshared keys are encrypted with the DEK (XAES-256-GCM, 24-byte nonce, stored as base64). They are decrypted only while the in-memory vault is unlocked with the admin password.

Other credentials are **not** vault-encrypted: `nodes[].agent_token`, `dns.api_token`, `integrations.prometheus_auth_token`, `integrations.telegram_bot_token`, `integrations.telegram_secret_token`, `mcp.read_only_token`, `mcp.full_control_token`, `mcp.oauth_client_secret`, and `app_variables[].value` (even when `is_secret` is set, the value is stored in plaintext in `config.json`) are stored as plaintext in `config.json`. Protect the file (mode `0600`) and any backups, and avoid committing them to version control.

### `network_settings`

| Field                      | Type       | Description                                                                                                                                          |
| :------------------------- | :--------- | :--------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`                     | `string`   | Optional network name. Defaults to `Easy42`.                                                                                                          |
| `public_asn`               | `integer`  | Your public or DN42 ASN (for example `4242421234`).                                                                                                  |
| `prefixes`                 | `string[]` | DN42/mesh prefix space used for peering filters and the built-in `dn42` policy allow-lists. Supports BIRD-style sets such as `172.20.0.0/14{21,29}`. |
| `disallowed_dn42_networks` | `string[]` | CIDRs excluded from the built-in DN42 policy's import/export and firewall allow-lists.                                                               |
| `disallowed_dn42_cidrs`    | `string[]` | Legacy alias for `disallowed_dn42_networks`. Both are normalized to the same value on load.                                                          |
| `flapalerted_address`      | `string`   | IP address of the FlapAlerted service. Enables the FlapAlerted BGP session on nodes with `enable_fa`.                                                |
| `flapalerted_port`         | `integer`  | FlapAlerted BGP port. Effective default `1790` (via getter; only persisted in `config.json` when `flapalerted_address` is set). |
| `flapalerted_asn`          | `integer`  | FlapAlerted BGP ASN. Effective default `4224420000` (same persistence note as above).                                          |
| `flapalerted_url`          | `string`   | Base URL of the FlapAlerted web UI. If omitted, derived from `flapalerted_address` and port `8699`.                                                  |

### `network_policies`

Easy42 always provides three read-only built-in policies (`is_internal: true`):

| ID        | Name                    | Behavior                                                                                                                                                                           |
| :-------- | :---------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `default` | Default (Internal)      | Full routing, rejects Internet route leaks, no ingress filtering. Cost `100`.                                                                                                      |
| `dn42`    | DN42 / External Peering | Full routing with DN42 prefix allow-lists and disallowed-CIDR filtering, forward/input firewall, ICMP/ICMPv6 and UDP/53 allowed, SNAT to `external_ip`, DSCP ingress/egress `0`, DN42 ROA URLs. Cost `100`. |
| `none`    | None (Unrestricted)     | Full routing, no filters, no Internet-leak protection. Cost `100`.                                                                                                                 |

User-defined policies with the same ID as a built-in override that built-in at lookup time.

| Field                   | Type       | Description                                                                                 |
| :---------------------- | :--------- | :------------------------------------------------------------------------------------------ | ---------- |
| `id`                    | `string`   | Policy identifier referenced by links.                                                      |
| `name`                  | `string`   | Display name.                                                                               |
| `description`           | `string`   | Optional description.                                                                       |
| `is_internal`           | `boolean`  | `true` for built-ins; user policies leave this unset/false.                                 |
| `cost`                  | `integer`  | Cost subtracted from BGP local preference. Default `100`; a link-level `cost` overrides it. |
| `local_networks`        | `string[]` | Prefixes considered local for this policy.                                                  |
| `allowed_dst_cidrs`     | `string[]` | Destination prefixes permitted on the link.                                                 |
| `allowed_src_cidrs`     | `string[]` | Source prefixes permitted on the link.                                                      |
| `disallowed_dst_cidrs`  | `string[]` | Destination prefixes rejected/blocked.                                                      |
| `disallowed_src_cidrs`  | `string[]` | Source prefixes rejected/blocked.                                                           |
| `reject_internet`       | `boolean`  | Reject Internet routes learned from the peer (route-leak protection).                       |
| `filter_forward`        | `boolean`  | Enable the nftables forward filter.                                                         |
| `filter_input`          | `boolean`  | Enable the nftables input filter.                                                           |
| `input_allow_icmp`      | `boolean`  | Allow ICMP in the input filter.                                                             |
| `input_allow_icmp6`     | `boolean`  | Allow ICMPv6 in the input filter.                                                           |
| `input_tcp_ports`       | `string[]` | Allowed TCP ports/ranges (`"22"`, `"8000-8080"`, `"all"`, `"*"`).                           |
| `input_udp_ports`       | `string[]` | Allowed UDP ports/ranges.                                                                   |
| `snat`                  | `object`   | Egress SNAT configuration (see below).                                                      |
| `forward_snat`          | `boolean`  | Enable SNAT for forwarded traffic.                                                          |
| `forward_snat_target`   | `string`   | `masquerade`, `external_ip`, `main_ip`, or a literal IP.                                    |
| `dscp_ingress`          | `string`   | Rewrite DSCP on packets received from the peer. Supports `value` or `value/mask` (decimal or hex, both 0-63, mask non-zero). |
| `dscp_egress`           | `string`   | Rewrite DSCP on packets sent to the peer. Supports `value` or `value/mask` (decimal or hex, both 0-63, mask non-zero).      |
| `block_ingress_new`     | `string`   | Block new/invalid ingress connections: `""` (disabled), `"all"`, or `"forward"`.            |
| `ingress_new_whitelist` | `string[]` | Exceptions in `ip:port` or `ip:port/tcp                                                     | udp` form. |
| `roa4`                  | `string`   | IPv4 ROA URL used for BIRD validation.                                                      |
| `roa6`                  | `string`   | IPv6 ROA URL.                                                                               |
| `roa_strict`            | `boolean`  | Reject routes without a valid ROA.                                                          |
| `fwmark`                | `string`   | Default WireGuard `[Interface] FwMark` for links using this policy.                         |
| `preference`            | `integer`  | Default BIRD BGP protocol preference.                                                       |
| `mark`                  | `string`   | Default netfilter mark applied to packets received from the peer (supports `mark` or `mark/bitmask`). |
| `routing_policy`        | `string`   | `full`, `stub`, `receive_only`, or `advertise_only` (aliases are normalized).               |
| `add_paths`             | `string`   | BIRD BGP ADD-PATH capability (`off` [default], `on`, `rx`, or `tx`).                        |
| `bird_import_filter`    | `string`   | Custom BIRD snippet appended to the generated import filter.                                |
| `bird_export_filter`    | `string`   | Custom BIRD snippet appended to the generated export filter.                                |
| `bird_export_fillter`   | `string`   | Legacy alias for `bird_export_filter` (note the historical spelling). Used only when `bird_export_filter` is empty. |
| `bird_pre_snippet`      | `string`   | Custom BIRD snippet inserted early in `bird_easy42.conf` (before `bird_hooks_pre`).         |
| `bird_post_snippet`     | `string`   | Custom BIRD snippet appended at the end of `bird_easy42.conf` (before `bird_hooks`).         |

`snat` object:

| Field       | Type      | Description                                                                                                               |
| :---------- | :-------- | :------------------------------------------------------------------------------------------------------------------------ |
| `enabled`   | `boolean` | Enable SNAT.                                                                                                              |
| `condition` | `string`  | `not_dst` (SNAT traffic whose source is outside the allowed destination CIDR set, or inside the disallowed set) or `all`. |
| `target`    | `string`  | `external_ip`, `masquerade`, `main_ip`, or a literal IP.                                                                  |

`block_ingress_new` accepts aliases such as `ingress`, `all_ingress`/`all-ingress`, `forwarding`, `forward_only`/`forward-only`, `none`, `off`, `disabled`, `false`. Unrecognized values normalize to disabled.

### `nodes`

| Field                   | Type            | Description                                                                                                                                                            |
| :---------------------- | :-------------- | :--------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `name`                  | `string`        | Unique node name and hostname. Alphanumeric, `-`, and `_` only. WireGuard interface names are derived from it and truncated to 15 characters.                          |
| `host`                  | `string`        | SSH host, alias, or IP. Omitted for external peers.                                                                                                                    |
| `is_external`           | `boolean`       | Marks an unmanaged external peer (for example DN42).                                                                                                                   |
| `caption`               | `string`        | Optional short description (plain text, one short sentence). Shown in the UI.                                                                                              |
| `description`           | `string`        | Optional description / contact info.                                                                                                                                     |
| `note`                  | `string`        | Markdown note shown in the UI.                                                                                                                                         |
| `ip`                    | `string`        | Primary overlay IPv4 address (normally bound to `interface`).                                                                                                          |
| `ip6`                   | `string`        | Primary overlay IPv6 address.                                                                                                                                          |
| `external_ip`           | `string`        | External/DN42 IPv4. Falls back to `ip` when empty.                                                                                                                     |
| `external_ip6`          | `string`        | External/DN42 IPv6. Falls back to `ip6` when empty.                                                                                                                    |
| `interface`             | `string`        | Interface holding the primary IP. Default `lo`.                                                                                                                        |
| `asn`                   | `integer`       | Private BGP ASN (`4224420000`-`4224429999`) or external ASN.                                                                                                           |
| `entrypoints`           | `object[]`      | Public reachable endpoints (see below).                                                                                                                                |
| `tags`                  | `string[]`      | Free-form tags used by the UI and endpoint matching.                                                                                                                   |
| `table`                 | `integer`       | Kernel FIB table for BIRD-exported routes. Default `254`.                                                                                                              |
| `external_table`        | `integer`       | Optional table for routes learned from external peers.                                                                                                                 |
| `internet_table`        | `integer`       | Optional table for Internet routes learned from peers.                                                                                                                 |
| `metric`                | `integer\|null` | Optional kernel route metric. When `null`, the template/OS default applies.                                                                                            |
| `static_routes`         | `string[]`      | Prefixes unconditionally advertised into BGP. Prefix with `@` (e.g. `@192.168.100.0/24`) to advertise without exporting the unreachable route to the kernel FIB. |
| `routes`                | `object[]`      | Kernel routes imported into BGP (see below).                                                                                                                           |
| `config_hooks`          | `object[]`      | Custom config snippets injected into generated files (see below).                                                                                                      |
| `wg_template`           | `string`        | Custom WireGuard template ID or name. Empty uses `default_wg`.                                                                                                         |
| `bird_template`         | `string`        | Custom BIRD template ID or name. Empty uses `default_bird`.                                                                                                            |
| `nft_template`          | `string`        | Custom nftables template ID or name. Empty uses `default_nft`.                                                                                                         |
| `x`, `y`                | `number\|null`  | Topology graph coordinates.                                                                                                                                            |
| `modified_at`           | `timestamp`     | System-managed last modification time.                                                                                                                                 |
| `mode`                  | `string`        | `ssh` (default) or `agent`. A non-empty `agent_token` also implies agent mode.                                                                                         |
| `agent_token`           | `string`        | Static shared token used by `easy42-agent`.                                                                                                                            |
| `agent_version`         | `string`        | Agent version reported at runtime.                                                                                                                                     |
| `agent_last_seen`       | `timestamp`     | Last agent heartbeat/report.                                                                                                                                           |
| `enable_fa`             | `boolean`       | Enable the FlapAlerted BGP telemetry session.                                                                                                                          |
| `update_hosts`          | `boolean`       | Manage the `/etc/hosts` Easy42 block on this node (agent mode only).                                                                                                   |
| `custom_network_script` | `string`        | Shell script persisted and applied by the `configure_network` Device Helper task (for example policy-routing rules).                                                   |
| `route_reflector`       | `boolean`       | When true, marks the node as a BGP Route Reflector for internal iBGP peers, appending `rr client;` to internal BGP peer blocks that share the same ASN in `bird.conf`. |

#### Entrypoints

An entrypoint describes how a node can be reached from outside.

| Field   | Type       | Description                                                                                                                                                       |
| :------ | :--------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ip`    | `string`   | Public IP. An empty `ip` is treated as the `none` entrypoint (strictly behind NAT).                                                                               |
| `ports` | `array`    | Ports exposed at this entrypoint. Each item may be an integer (`51820`), a range string (`"20000-29999"`), or an object `{ "port": 443, "external_port": 8443 }`. |
| `tags`  | `string[]` | Tags used to match link endpoints to entrypoints.                                                                                                                 |
| `mtu`   | `integer`  | Optional MTU associated with this endpoint (0-9000).                                                                                                              |

#### Kernel route rules (`routes`)

| Field      | Type       | Description                                   |
| :--------- | :--------- | :-------------------------------------------- |
| `table`    | `integer`  | Kernel table to read routes from.             |
| `prefixes` | `string[]` | Prefixes to import from that table into BIRD. |

#### Config hooks (`config_hooks`)

| Field     | Type     | Description                                                                                                                                                                   |
| :-------- | :------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `type`    | `string` | Hook location: `bird`, `bird.pre`, `wg.interface`, `wg.peer`, `wg.post`, `nft.table`, `nft.pre`, or `nft`. Aliases: `bird.post`/`bird.global`, `wg`, `nft.post`/`nft.global`. |
| `target`  | `string` | Optional interface or peer filter for WireGuard hooks.                                                                                                                        |
| `content` | `string` | Snippet rendered as a Go `text/template` (same context and functions as the config template, e.g. `{{.ip}}`) and injected into the generated configuration.                   |

Hook content is loaded into the Go template together with the config template and pulled in via the `include` function: hook sections in config templates render each hook with `{{ include . $ }}`, where `$` is the full template context.

### `links`

| Field         | Type        | Description                                                                                |
| :------------ | :---------- | :----------------------------------------------------------------------------------------- |
| `type`        | `string`    | `wireguard` (default) or `manual`.                                                         |
| `from`, `to`  | `object`    | The two link endpoints. Order is not significant.                                          |
| `tags`        | `string[]`  | Free-form tags.                                                                            |
| `assign_ipv4` | `boolean`   | Assign deterministic `169.254.X.X/32` IPv4 link-local addresses.                           |
| `disable_bfd` | `boolean`   | Emit `bfd no;` for this link's BGP session, overriding the template's `bfd yes;`.          |
| `psk`         | `string`    | Encrypted base64 WireGuard preshared key. Omit to generate one when the vault is unlocked. |
| `note`        | `string`    | Markdown note for the link.                                                                |
| `modified_at` | `timestamp` | System-managed last modification time.                                                     |

Each endpoint (`from`, `to`):

| Field                  | Type      | Description                                                                                              |
| :--------------------- | :-------- | :------------------------------------------------------------------------------------------------------- |
| `name`                 | `string`  | Node name. Required.                                                                                     |
| `type`                 | `string`  | `wireguard` (default) or `manual`.                                                                       |
| `interface`            | `string`  | Interface name. Auto-derived as `wg42<peer>` (`wg42-<peer>` for external peers) when empty.              |
| `address`              | `string`  | IPv6 link-local address, for example `fe80::c0a8:6401/64`. Derived from the node's main IPv4 when empty. |
| `address4`             | `string`  | Optional IPv4 link-local address, for example `169.254.X.X/32`.                                          |
| `neighbor_address`     | `string`  | Peer address for manual links. Defaults to the opposite endpoint's address.                              |
| `listen_port`          | `integer` | Local WireGuard listen port. Derived from the peer's IPv4 when `0`.                                      |
| `endpoint`             | `string`  | Public `host:port` of the peer. Auto-resolved from the peer's entrypoints when empty.                    |
| `private_key`          | `string`  | Encrypted base64 WireGuard private key (server-managed).                                                 |
| `public_key`           | `string`  | WireGuard public key.                                                                                    |
| `persistent_keepalive` | `integer` | Keepalive interval. `0` becomes `25` when an endpoint is present, otherwise `0`.                         |
| `mtu`                  | `integer` | Link MTU. Default `1420`.                                                                                |
| `use_ip`               | `boolean` | Resolve the peer's endpoint hostname to an IP on the server.                                             |
| `resolved_endpoint`    | `string`  | System-derived endpoint actually used. Not accepted in apply patches.                                    |
| `policy`               | `string`  | Policy ID. External peers default to `dn42`; internal peers default to `default`.                        |
| `cost`                 | `integer` | Link cost override. When non-zero, overrides the policy cost.                                            |
| `fwmark`               | `string`  | WireGuard `FwMark` override for this endpoint.                                                           |
| `preference`           | `integer` | BIRD BGP preference override.                                                                            |
| `mark`                 | `string`  | Netfilter mark override for received packets (supports `mark` or `mark/bitmask`).                        |
| `routing_policy`       | `string`  | BGP routing policy override.                                                                             |
| `add_paths`            | `string`  | BIRD BGP add paths override (`off`, `on`, `rx`, `tx`, or `inherit` to revert to policy).                 |
| `psk`                  | `string`  | Endpoint-level encrypted PSK (legacy; link-level `psk` is preferred).                                    |
| `note`                 | `string`  | Markdown note.                                                                                           |
| `obfuscator`           | `string`  | Agent-mode WireGuard UDP-over-TCP obfuscation: `none`, `phantun_client`, or `phantun_server`. Both endpoints must be complementary. |
| `obfuscator_parameters` | `object` | Generated Phantun parameters (`local_port`, `server_port`, `wireguard_port`, `tun_name`, `tun_local`, `tun_peer`, optional `extra_args`). Generated on first enable and editable thereafter. |

When a link is obfuscated, the client-side WireGuard peer endpoint becomes `127.0.0.1:<local_port>` and the server-side WireGuard peer endpoint is unset. The Easy42 agent installs and supervises one service per endpoint named `<interface>-obfuscator` (for example `wg42peer-obfuscator`). Generated Phantun ports are random in `10000...19999`; generated TUN pairs use a random `198.51.100.X` with peer `X+1`. Default generation avoids ports and TUN addresses already used by obfuscated links on the node. Both generated WireGuard interface configurations use an MTU that is reduced by `12` bytes for Phantun encapsulation overhead. Sync fails if an obfuscated link must be deployed to a node without a running Easy42 agent.

### `blocks`

| Field             | Type       | Description                        |
| :---------------- | :--------- | :--------------------------------- |
| `id`              | `string`   | Unique block ID.                   |
| `name`            | `string`   | Display name.                      |
| `color`           | `string`   | Hex color.                         |
| `x`, `y`          | `number`   | Top-left position on the graph.    |
| `width`, `height` | `number`   | Optional size.                     |
| `nodes`           | `string[]` | Node names contained in the block. |

### `dns`

| Field                   | Type      | Description                                                         |
| :---------------------- | :-------- | :------------------------------------------------------------------ |
| `zone_id`               | `string`  | Cloudflare zone ID.                                                 |
| `api_token`             | `string`  | Cloudflare API token with DNS edit permission.                      |
| `base_domain`           | `string`  | Base domain for generated records (for example `mesh.example.com`). |
| `publish_ipv6_own_name` | `boolean` | Also publish the node's main IPv6 address on its own name.          |

All three of `zone_id`, `api_token`, and `base_domain` must be set for DNS sync to run. Legacy aliases `cloudflare_zone_id`, `cloudflare_api_token`, and `publish_main_ipv6_to_own_name` are accepted on load.

### `templates`

| Field         | Type        | Description                                                                                     |
| :------------ | :---------- | :---------------------------------------------------------------------------------------------- |
| `id`          | `string`    | Template ID referenced by `nodes[].wg_template` / `bird_template` / `nft_template`, or by name. |
| `name`        | `string`    | Display name.                                                                                   |
| `type`        | `string`    | `wg`, `bird`, or `nft`.                                                                         |
| `description` | `string`    | Optional description.                                                                           |
| `content`     | `string`    | Go `text/template` source.                                                                      |
| `is_builtin`  | `boolean`   | `true` for read-only system templates.                                                          |
| `modified_at` | `timestamp` | Last modification time.                                                                         |

Built-in template IDs are `default_wg`, `default_bird`, and `default_nft`. Selecting `default` also resolves to the built-in template.

### `looking_glass_tasks`

| Field          | Type       | Description                                                                                                     |
| :------------- | :--------- | :-------------------------------------------------------------------------------------------------------------- |
| `id`           | `string`   | Unique task ID.                                                                                                 |
| `name`         | `string`   | Display name.                                                                                                   |
| `description`  | `string`   | Optional description.                                                                                           |
| `category`     | `string`   | `BGP / Routing`, `Connectivity`, `Diagnostics`, or `Custom`.                                                    |
| `command_tmpl` | `string`   | Command template. Parameters are substituted as `{{key}}` and escaped by the server.                            |
| `parser`       | `string`   | `bird_route`, `bird_protocols`, `ping`, `tcp_ping`, `traceroute`, `mtr`, or `raw`.                              |
| `params`       | `object[]` | Parameter definitions (`key`, `label`, `type`, `description`, `default_value`, `required`, `options`, `regex`). |
| `is_builtin`   | `boolean`  | `true` for read-only built-ins.                                                                                 |
| `timeout_sec`  | `integer`  | Execution timeout in seconds.                                                                                   |

Built-in tasks include `ping`, `tcp_ping`, `traceroute`, `mtr`, `bird_show_protocols`, `bird_show_route`, `bird_show_route_where`, `bird_show_status`, `ip_route_get`, and `whois_dn42`.

### `cron_tasks`

Scheduled `ping`/`tcp_ping` probes executed by node agents on a selector-matched set of nodes. This is distinct from per-node Looking Glass tasks (which are run manually from the Looking Glass UI) and from cron-type workload Apps (see [Apps](#apps-and-app_variables)).

| Field                      | Type        | Description                                                                                             |
| :------------------------- | :---------- | :------------------------------------------------------------------------------------------------------ |
| `id`                       | `string`    | Unique task ID.                                                                                         |
| `name`                     | `string`    | Display name.                                                                                           |
| `description`              | `string`    | Optional description.                                                                                   |
| `task_type`                | `string`    | `ping` (ICMP) or `tcp_ping`.                                                                            |
| `cron_expr`                | `string`    | 5-field (`min hour dom mon dow`) or 6-field (`sec min hour dom mon dow`) cron expression.               |
| `selector`                 | `object`    | Node selector (`name`, `tag`, `env:<NAME>`; AND). Empty matches every managed node.                     |
| `params`                   | `object`    | Probe parameters. `ping` uses `target` and `count`; `tcp_ping` uses `target` (`host:port`) and `count`. |
| `timeout_sec`              | `integer`   | Optional timeout in seconds (default `15`, capped at `60`).                                              |
| `enabled`                  | `boolean`   | Whether the schedule is active.                                                                         |
| `created_at`, `updated_at` | `timestamp` | Timestamps managed by the server.                                                                       |

`params.count` defaults to `4` (clamped to `1`–`20`; invalid values fall back to `4`). A cron task that matches no managed node is rejected on save. Results are stored in `data.db` and presented in the node's Cron Tasks view; summarized status is `ok`, `degraded`, or `failed`, and repeated failures record a fleet event (see [Events](integrations.md#8-events)).

### `server_tasks`

Only the predefined server tasks are valid. Missing entries are recreated with defaults on load.

| ID              | Default cron | Description                                                                                 |
| :-------------- | :----------- | :------------------------------------------------------------------------------------------ |
| `update_roa`    | `0 0 * * *`  | Download/update ROA files for all policy ROA URLs.                                          |
| `sync`          | `30 0 * * *` | Synchronize topology, WireGuard, BIRD, nftables, ROA, and hosts configuration to all nodes. |
| `sync_dns`      | `0 1 * * *`  | Synchronize node records to Cloudflare DNS.                                                 |
| `backup_config` | `0 2 * * *`  | Backup easy42 configuration file (config.json) preserving at most 100 backups.              |

| Field       | Type      | Description                                                |
| :---------- | :-------- | :--------------------------------------------------------- |
| `id`        | `string`  | One of `update_roa`, `sync`, `sync_dns`, `backup_config`.   |
| `cron_expr` | `string`  | 5- or 6-field cron expression. Invalid values fall back to the default. |
| `enabled`   | `boolean` | Enable the schedule.                                       |

All tasks are enabled by default. Schedules can be changed from the UI or the `/api/settings/tasks` API (`GET /api/settings/tasks`, `PUT /api/settings/tasks/{id}`, `POST /api/settings/tasks/{id}/run`). The apply schema accepts `server_tasks`, but the current apply engine does not merge that section.

### `database`

| Field                | Type      | Description                                                                  |
| :------------------- | :-------- | :--------------------------------------------------------------------------- |
| `retention_days`     | `integer` | Retention for summarized telemetry, rollups, and cron results. Default `47`. |
| `raw_retention_days` | `integer` | Retention for high-resolution raw telemetry and logs. Default `2`.           |

Cleanup runs hourly after an initial pass, and telemetry is rolled up into 5-minute buckets. Values `<= 0` are treated as the defaults (`47` / `2` via getters; a fresh `config.json` may omit `raw_retention_days` until it is explicitly set).

### `integrations`

| Field                   | Type      | Description                                                                                                      |
| :---------------------- | :-------- | :--------------------------------------------------------------------------------------------------------------- |
| `prometheus_enabled`    | `boolean` | Enable the `/metrics` endpoint.                                                                                  |
| `prometheus_auth_token` | `string`  | Bearer token required to scrape `/metrics`. Auto-generated if empty.                                             |
| `telegram_enabled`      | `boolean` | Enable the Telegram Bot integration.                                                                             |
| `telegram_bot_token`    | `string`  | Bot token from @BotFather.                                                                                       |
| `telegram_admin_id`     | `integer` | Numeric Telegram user ID allowed to issue commands.                                                              |
| `telegram_secret_token` | `string`  | Secret token sent in the `X-Telegram-Bot-Api-Secret-Token` header. Auto-generated if empty.                      |
| `server_url`            | `string`  | Public HTTPS base URL of this Easy42 instance, used to register the Telegram webhook at `/api/telegram/webhook`. |

### `mcp`

Model Context Protocol (MCP) server settings. The MCP server is enabled by default and exposes cluster inspection/mutation tools over Streamable HTTP (`/mcp`, `/api/mcp`) and SSE (`/mcp/sse`, `/mcp/message`), with OAuth 2.0/2.1 discovery under `/.well-known/*` and `/oauth/*`. There is also a stdio CLI (`easy42 mcp --role full_control|read_only`). See [MCP Server](integrations.md#10-mcp-server).

| Field                   | Type     | Description                                                                                   |
| :---------------------- | :------- | :---------------------------------------------------------------------------------------------- |
| `enabled`               | `boolean` | Enable the MCP server. Enabled by default.                                                    |
| `read_only_token`       | `string`  | Bearer token for inspection-only tools. Auto-generated on first load if empty.                  |
| `full_control_token`    | `string`  | Bearer token for tools that may modify configuration. Auto-generated; must differ from the read-only token. |
| `oauth_client_id`       | `string`  | OAuth client ID for MCP OAuth flow. Default `easy42-mcp`.                                      |
| `oauth_client_secret`   | `string`  | OAuth client secret. Auto-generated if empty.                                                  |

Tokens are accepted as `Authorization: Bearer <token>` (or `?token=<token>`). Manage them from **Settings -> MCP** or `GET/PUT /api/settings/mcp` and `POST /api/settings/mcp/reset-token`. The apply schema does not merge `mcp`; manage it through the UI/API.

### `apps` and `app_variables`

Workload apps deployed to agent-managed nodes (see [Workload Apps](integrations.md#9-workload-apps-apps)). Apps are **not** part of the declarative apply schema; manage them through the UI/API (`/api/apps`, `/api/apps/variables`).

| Field (`apps[]`) | Type | Description |
| :--------------- | :--- | :---------- |
| `id` | `string` | Server-generated UUID. |
| `name` | `string` | App name, `^[a-zA-Z0-9_-]{1,64}$`. Used as the Docker/project and systemd unit suffix (`ezapp-<name>`). |
| `description` | `string` | Optional description. |
| `note` | `string` | Markdown note. A fenced ` ```json ` block is exposed as `.data` in templates, like node/link notes. |
| `url` | `string` | Optional URL associated with the app. Interpolated like `env_content`. |
| `app_type` | `string` | `docker`, `systemd`, or `cron`. |
| `main_content` | `string` | Docker Compose YAML, systemd unit content, or cron expression + command. Required. |
| `env_content` | `string` | `.env` content (`KEY=VAL` lines). Interpolated. |
| `files` | `object` | Map of relative filename to content for auxiliary config files. Paths must not contain `..` or start with `/`. |
| `selector` | `object` | Node selector (`name`, `tag`, `env:<NAME>`; AND). Empty matches every node. See [Selectors](device-helper.md#task-selector). |
| `published` | `boolean` | Whether the app is published/visible. Duplicating an app forces `published: false`. |
| `enabled` | `boolean` | Whether the app is enabled (deployed). Disabled apps are not synced. |
| `created_at`, `updated_at` | `timestamp` | Server-managed timestamps. |

| Field (`app_variables[]`) | Type | Description |
| :------------------------ | :--- | :---------- |
| `key` | `string` | Variable name, `^[a-zA-Z_][a-zA-Z0-9_]*$`. |
| `value` | `string` | Variable value (stored in plaintext in `config.json`). |
| `description` | `string` | Optional description. |
| `is_secret` | `boolean` | Marks the variable as secret in the UI (does not encrypt the stored value). |
| `updated_at` | `timestamp` | Server-managed timestamp. |

Interpolation uses `{{VAR}}` / `{{.VAR}}` and `${VAR}` syntax in `url`, `env_content`, `files`, and cron-type `main_content` (Docker/systemd manifests are not interpolated). Global variables are combined with per-node context variables (plus `APP_NAME`).

### `config_changes`

Saved snippets shown in the Config Apply UI. They are validated before being stored but are not applied until explicitly run.

| Field                       | Type        | Description                                                    |
| :-------------------------- | :---------- | :------------------------------------------------------------- |
| `id`                        | `string`    | UUID generated by the server.                                  |
| `name`                      | `string`    | Display name. If empty when saving through the API, the server generates `Snippet <date>`. Duplicate names are de-duplicated. |
| `description`               | `string`    | Optional description.                                          |
| `content`                   | `string`    | JSON or YAML patch content. Must pass strict apply validation. |
| `format`                    | `string`    | `json` or `yaml`.                                              |
| `created_at`, `modified_at` | `timestamp` | Timestamps managed by the server.                              |

---

## Declarative Config Apply

`easy42 apply` (and the Config Apply UI) accepts a **partial patch** rather than a complete `config.json`. Patches are parsed as JSON when they start with `{` or `[`, otherwise as YAML, then decoded with unknown-field rejection.

### Allowed patch sections

| Section            | Apply behavior                                                                                                                                                                   |
| :----------------- | :------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `nodes`            | Create or partially update nodes, matched case-insensitively by `name`.                                                                                                          |
| `links`            | Create or partially update links, matched by unordered endpoint pair. New links require both endpoint nodes to exist in the existing configuration or earlier in the same patch. |
| `network_settings` | Merge individual fields.                                                                                                                                                         |
| `database`         | Merge retention fields.                                                                                                                                                          |
| `dns`              | Merge Cloudflare fields.                                                                                                                                                         |
| `integrations`     | Merge Prometheus and Telegram fields.                                                                                                                                            |
| `network_policies` | Accepted by the schema. The current apply engine does not merge this section; manage policies through the UI/API.                                                                |
| `blocks`           | Accepted by the schema. The current apply engine does not merge this section; manage blocks through the UI/API.                                                                  |
| `templates`        | Accepted by the schema. The current apply engine does not merge this section; manage templates through the UI/API.                                                               |
| `server_tasks`     | Accepted by the schema. The current apply engine does not merge this section; manage schedules through the UI/API.                                                               |
| `cron_tasks`       | Accepted by the schema. The current apply engine does not merge this section; manage cron tasks through the UI/API.                                                              |
| `apps`, `app_variables`, `mcp`, `looking_glass_tasks` | **Not** in the apply schema at all; manage through the UI/API. |

Unknown-field rejection is strict at the top level and for `nodes[]`/`links[]`/link endpoints, but the advertised schema defines `network_policies`, `blocks`, `templates`, `server_tasks`, and `cron_tasks` as permissive objects, so unknown keys inside those sections are not rejected by the schema (and are ignored by the engine since those sections are not merged).

For a section that is merged, omitted fields are left unchanged. To clear an optional string or list field, set it to an empty string or an empty list. A JSON `null` is treated like an omitted field by the patch decoder — pointer fields (for example `metric`, `x`/`y`, `preference`) therefore cannot be cleared via apply. Lists such as `entrypoints`, `routes`, and `config_hooks` are replaced wholesale when present.

### Delta operations

For `static_routes` and `tags`, apply patches also support incremental delta operations:
- **`nodes[].static_routes_add`**: Append static routes to the node's existing routes. If the route already exists, it is ignored (idempotent).
- **`nodes[].static_routes_del`**: Remove specified static routes from the node. If a route does not exist, it is ignored.
- **`nodes[].tags_add` / `nodes[].tags_del`**: Add or remove tags from nodes incrementally.
- **`links[].tags_add` / `links[].tags_del`**: Add or remove tags from links incrementally.

If an attribute already contains the added items or does not contain the deleted items, applying the delta results in a no-op with 0 changes recorded.


### Forbidden fields

The following fields are rejected even though they exist in `config.json`:

| Location  | Forbidden fields                                   |
| :-------- | :------------------------------------------------- |
| Root      | `password_hash`, `encrypted_dek`, `session_secret` |
| `nodes[]` | `modified_at`, `agent_version`, `agent_last_seen`  |
| `links[]` | `modified_at`                                      |
| `links[].from/to` | `resolved_endpoint` (system-derived; rejected as an unknown field in the endpoint patch) |

`nodes[].agent_token`, `links[].from/to.private_key`, `links[].from/to.public_key`, and `links[].from/to.psk` are not part of the apply schema and cannot be set declaratively (only the link-level `links[].psk` is accepted). Use the UI/API to manage agent tokens and WireGuard keys — apply cannot supply them either when the vault is locked.

### Merge and validation rules

- Node names must match `^[a-zA-Z0-9_-]+$` and be unique (case-insensitive).
- Non-external nodes must not share the same main IPv6 address.
- Every link endpoint must reference an existing node.
- `ip` and `external_ip` must be valid IPv4 addresses *without* a CIDR suffix; `ip6` and `external_ip6` must be valid IPv6 addresses (a CIDR suffix is stripped/ignored for IPv6 validation only).
- `asn` must be greater than zero. Tables must be non-negative. Entrypoint/link MTUs must be within `0-9000`.
- The advertised apply schema declares link `cost >= 0`, but the Go patch validator does not enforce non-negativity, so a negative `cost` in a patch currently passes and merges. `config.json` and the UI/API accept negative costs for preference boosting.
- A link patch cannot use the same node for both endpoints.
- New WireGuard links receive generated keys and (if absent) a PSK only while the encryption vault is unlocked. With a locked vault, apply leaves those fields empty; use the UI/API (after unlocking) to supply/rotate keys.

### Side effects

A successful apply:

1. Writes a timestamped backup to `<data-dir>/config.json.bak.<unix>` and a `config.json.bak` copy, then prunes old backups to at most 100.
2. Saves the merged configuration atomically.
3. Triggers an `/etc/hosts` resynchronization for agent nodes in the background, and re-syncs Phantun obfuscator services to agents.
4. Optionally triggers a sync of affected nodes (`--sync` on the CLI, `auto_sync: true` on the API).

Applying does not automatically roll back if a later node sync fails. To recover, restore one of the backups and run a sync.

### Example patch

```yaml
network_settings:
  flapalerted_address: fd42:a159:f9f0:8000::c8
  flapalerted_url: http://[fd42:a159:f9f0:8000::c8]:8699

nodes:
  - name: node-fra
    host: vps-fra.example.com
    ip: 192.168.100.1
    ip6: fd42:a159:f9f0::1
    asn: 4224420001
    update_hosts: true
    entrypoints:
      - ip: 198.51.100.1
        ports: [51820, "20000-29999"]
        tags: [public]

links:
  - from:
      name: node-fra
      policy: default
    to:
      name: node-lon
      policy: default
    assign_ipv4: true

database:
  retention_days: 60
  raw_retention_days: 3

integrations:
  prometheus_enabled: true
  telegram_enabled: false
```

Preview and apply it with:

```bash
easy42 apply --dry-run patch.yaml
easy42 apply patch.yaml --sync
```

### Related API endpoints

The apply endpoints below are under `/api` and require an authenticated session. (The server also exposes non-`/api` endpoints such as `/metrics`, `/mcp`, `/oauth/*`, and `/.well-known/*`; see `internal/server/server.go`.)

| Method   | Path                         | Purpose                                                |
| :------- | :--------------------------- | :----------------------------------------------------- |
| `GET`    | `/config/apply/schema`       | Return the strict JSON Schema for patches.             |
| `POST`   | `/config/apply/validate`     | Validate syntax/schema and report the detected format. |
| `POST`   | `/config/apply/preview`      | Validate, simulate, and return the diff/summary.       |
| `POST`   | `/config/apply`              | Execute a patch. Accepts optional `auto_sync`.         |
| `GET`    | `/config/changes`            | List saved snippets.                                   |
| `POST`   | `/config/changes`            | Create a saved snippet.                                |
| `GET`    | `/config/changes/{id}`       | Read a snippet.                                        |
| `PUT`    | `/config/changes/{id}`       | Update a snippet.                                      |
| `DELETE` | `/config/changes/{id}`       | Delete a snippet.                                      |
| `POST`   | `/config/changes/{id}/apply` | Apply a saved snippet. Accepts optional `auto_sync`.   |

The preview response includes `valid`, `format`, a summary (`added_nodes`, `modified_nodes`, `added_links`, `modified_links`, `total_changes`), per-field diffs, and a unified text diff. The unified diff is computed from a filtered view (network ASN/prefixes, node name/host/IPs/ASN/static routes/tags, link endpoints/cost), not a full byte-for-byte diff of `config.json`. Other API areas (nodes, links, policies, templates, apps, events, tasks, Looking Glass, settings) are documented in their respective chapters.

---

## Data Files

| Path                                | Description                                                                                           |
| :---------------------------------- | :---------------------------------------------------------------------------------------------------- |
| `<data-dir>/config.json`            | This file. Created with mode `0600`.                                                                  |
| `<data-dir>/config.json.bak`        | Most recent apply backup.                                                                             |
| `<data-dir>/config.json.bak.<unix>` | Timestamped backups written by each apply.                                                            |
| `<data-dir>/data.db`                | SQLite database for telemetry, interface traffic, cron results, fleet events, ping-table tasks, node state (including obfuscator state), and applied-config hashes. |
| `<data-dir>/server.url`             | Transient URL used by the CLI to find a running `easy42 serve`; removed on shutdown.                  |

The database is not part of `config.json`. Deleting `data.db` resets telemetry and applied-state tracking but leaves the network configuration intact.
