# Dynamic Routing with BIRD 2 & Network Policies

This document explains how **Easy42** configures **BIRD 2** for automated dynamic routing, BFD-based failure detection, traffic engineering, and route filtering.

---

## BIRD 2 Configuration Architecture

Easy42 generates `/etc/bird_easy42.conf` on each managed node and links it into the node's main BIRD configuration (`/etc/bird/bird.conf`) via:

```bird
include "/etc/bird_easy42.conf";
```

### 1. BGP Confederations & Autonomous Systems

Easy42 supports two main BGP architectures:

1. **Flat Private ASN / iBGP Mesh**: All nodes share the same ASN or use private AS numbers. No confederation directives are emitted.
2. **BGP Confederation (RFC 5065)**: When a confederation ASN is configured globally, each internal node is assigned a unique private ASN within `4224420000`–`4224429999`, grouped under a single parent Confederation AS (for example the public DN42 ASN `424242xxxx`):

   ```bird
   define CONFED_AS = 424242xxxx;
   define COMM_EXTERNAL = (CONFED_AS, 1, 1);

   template bgp easy42_peer {
       confederation CONFED_AS;
       confederation member yes;  # Internal peers
       default bgp_local_pref DEFAULT_LOCAL_PREF;
       ...
   }

   template bgp external_peer {
       confederation CONFED_AS;
       confederation member no;   # External eBGP peers
       ...
   }
   ```

   External peers see the network originating from the public ASN, hiding internal topology while eliminating an iBGP full mesh. Internal routes are tagged with the large community `COMM_EXTERNAL` when imported from external peers so they can be steered into the external kernel table.

### 2. Fast Failure Detection with BFD

Easy42 injects a BIRD 2 BFD protocol block on every node:

```bird
protocol bfd {
    interface "*" {
        min rx interval 2000ms;
        min tx interval 2000ms;
        multiplier 5;
    };
    multihop {
        min rx interval 2000ms;
        min tx interval 2000ms;
        multiplier 5;
    };
}
```

Internal peer protocols enable `bfd yes;`, so BGP sessions are torn down when the overlay path stops responding instead of waiting for the BGP hold timer. With the generated `2000ms` intervals and multiplier `5`, detection takes roughly 10 seconds. Faster detection requires overriding the BIRD template (for example with `bird_template`).

### 3. Route Origination & Loopback Announcement

Each node has a primary stable IP (e.g., `192.168.100.1` on `lo`). To originate this route into BGP without creating a routing loop on the local host:

```bird
protocol static static_self {
    ipv4;
    route 192.168.100.1/32 reject;
    route 198.51.100.10/32 reject;   # external_ip, when configured
}

protocol static static_self_v6 {
    ipv6;
    route fd42::10/128 reject;       # ip6, when configured
    route 2001:db8::10/128 reject;   # external_ip6, when configured and different
}
```

In BIRD, a `reject` route is purely local to the routing daemon; it injects the prefix into BIRD's routing table (`master4`/`master6`) for export to peers, while the local kernel routes the local address through the loopback path. User-configured `static_routes` are emitted the same way in `static_v4`/`static_v6`, with `/32` or `/128` appended when no prefix length is given.

### 4. Multi-Table Kernel FIB Synchronization & Metrics

Learned BGP routes are exported to the Linux kernel FIB:

- **Main Routing Table** (`table 254` by default, or the node's `table`):
  ```bird
  protocol kernel kernel_v4 {
      metric 100;
      ipv4 {
          export filter {
              krt_prefsrc = SELF_IP;
              if (COMM_EXTERNAL ~ bgp_large_community) then reject;
              if (source ~ [ RTS_BGP ]) && (net ~ INTERNET) then reject;
              if source ~ [ RTS_BGP ] then accept;
              if source ~ [ RTS_STATIC ] && (net ~ PRIVATE) then accept;
              reject;
          };
          import none;
      };
      kernel table TABLE;
  }
  ```
  Setting `krt_prefsrc = SELF_IP` ensures local applications use the overlay IP when originating mesh traffic. Static `reject` routes are only installed for private prefixes, and Internet routes learned over BGP are not installed in the main table when a separate internet table is configured. `kernel_v6` mirrors this with `INTERNET6` and `PRIVATE6`.
- **External Routing Table (`external_table`)**: A pipe copies routes carrying the `COMM_EXTERNAL` large community from `master4`/`master6` into an isolated table, and a dedicated kernel protocol installs them with `krt_prefsrc` set to `external_ip`/`external_ip6` (falling back to `ip`/`ip6`). This enables Policy-Based Routing (PBR) for external peer routes.
- **Internet Routing Table (`internet_table`)**: A separate pipe/kernel pair isolates upstream transit or default routes (`INTERNET`/`INTERNET6`) into another kernel table. If `external_table` and `internet_table` are equal, external routes and Internet routes share that table. If `internet_table` differs, `COMM_EXTERNAL` routes are excluded from the internet pipe.

The metric is emitted only when configured on the node, and kernel `learn` is enabled only when the node has `routes` entries for that table.

### 5. Dynamic Kernel Route Import & Static Route Broadcasting

Nodes can dynamically redistribute existing routes into BGP:

- `routes`: Imports prefixes directly from specific Linux kernel routing tables (e.g., table 100 subnets) into BIRD using kernel protocol `learn`.
- `static_routes`: Injects static prefix announcements directly into BIRD's static protocol block for unconditional BGP advertisement.

### 6. Route Leak Protection

To prevent accidental leaks of default routes or public internet subnets into private overlays or DN42:

```bird
define PRIVATE  = [ 192.168.0.0/16+, 10.0.0.0/8+, 172.16.0.0/12+, 198.18.0.0/15+ ];
define PRIVATE6 = [ fd00::/8+ ];
define INTERNET  = [ 0.0.0.0/0, 0.0.0.0/1, 128.0.0.0/1 ];
define INTERNET6 = [ ::/0, ::/1, 8000::/1 ];

import filter {
    if net ~ INTERNET then reject;
    # policy-specific allowed/disallowed prefixes follow
    accept;
};
```

Policies with `reject_internet` enabled (the built-in `default` and `dn42` policies) reject both `INTERNET` and `INTERNET6`. Custom policies can set `/0` import and export prefix lists plus disallowed lists; entries in a disallowed list are checked before the allowed list.

### 7. FlapAlerted BGP Peering & Alerting Integration

When configured globally, Easy42 can peer with a FlapAlerted instance (`enable_fa: true` per node):

```bird
protocol bgp flapalerted {
    local LOCAL_IP as FA_ASN;
    neighbor FA_ADDR as FA_ASN port FA_PORT;
    multihop;
    ipv4 {
        add paths on;
        export all;
        import none;
        extended next hop on;
    };
    ipv6 {
        add paths on;
        export all;
        import none;
        extended next hop on;
    };
}
```

This streams all routing state to FlapAlerted without accepting inbound route injection.

---

## Network Policies

Every link endpoint in Easy42 is assigned a **Network Policy**, governing routing, firewall filters, and NAT.

| Policy | Target Usage | Internet Leak Protection | Forward Filter | Input Filter | Default SNAT | ROA |
| :--- | :--- | :---: | :--- | :--- | :--- | :--- |
| **`default`** | Internal mesh nodes | Yes (`reject_internet`) | Disabled | Disabled | Disabled | None |
| **`dn42`** | External / DN42 peers | Yes (`reject_internet`) | Enabled against allowed/disallowed DN42 CIDRs | Enabled; TCP 179 always, UDP 53, ICMP/ICMPv6 | `external_ip`, when the source is outside the allowed CIDR set | Burble IPv4 + IPv6 URLs |
| **`none`** | Unrestricted tunnels | No | Disabled | Disabled | Disabled | None |
| _Custom_ | Custom peering / transit | Configurable | Configurable CIDRs | Configurable ports | Configurable | Custom URLs |

Policy defaults are resolved at compile time:

- An endpoint without an explicit policy uses `dn42` when the **remote** endpoint is external, and `default` otherwise.
- Built-in policies are always present and cannot be edited. Custom policies with the same ID take precedence over built-ins.
- The default DN42 prefix list comes from global `network_settings.prefixes` when set; otherwise it uses the standard DN42/ChaosVPN/Freifunk/NeoNetwork ranges. `disallowed_dn42_networks` / `disallowed_dn42_cidrs` add exclusions.
- If neither `external_ip`/`external_ip6` is configured for a SNAT target, no corresponding SNAT rule is emitted for that family.

### Link Costs, Negative Costs & Latency Steering

Each policy defines a base `cost` (default: 100), which can be overridden by `cost` on an individual link endpoint:

- **In confederation mode only**, BIRD adjusts `bgp_local_pref` on import. Without a confederation ASN, the cost is still stored and displayed but does not alter `bgp_local_pref`.
- Positive cost: `bgp_local_pref = DEFAULT_LOCAL_PREF(10000) − cost` (floored at 1).
- **Negative link costs** (e.g. `-50`) increase preference above 10000: `bgp_local_pref = 10000 + |cost|`, clamped to the 32-bit maximum. This allows explicitly preferring high-bandwidth or primary transit paths.
- An endpoint `cost` of `0` means "inherit the policy cost"; it is not a literal zero cost.

### Routing Policy Modes

Each policy or link endpoint can enforce a BGP routing policy. Aliases are normalized by the configuration layer:

| Mode | Aliases | Import behavior | Export behavior |
| :--- | :--- | :--- | :--- |
| `full` (default) | `transit`, `mesh`, empty | Accepts valid routes after leak/ROA/filter checks. | Exports all valid routes after leak/filter checks. |
| `stub` | `originate_only`, `local_only`, `transit_client` | Accepts routes from the peer normally. | Exports only `RTS_STATIC`, `RTS_DEVICE`, and `RTS_INHERIT` routes (local origination). |
| `receive_only` | `import_only` | Accepts routes from the peer normally. | Rejects all exports. |
| `advertise_only` | `export_only` | Rejects all imports. | Exports local-originated routes (`RTS_STATIC`, `RTS_DEVICE`, `RTS_INHERIT`). |

Unrecognized or empty values normalize to `full`.

---

## Route Origin Authorization (ROA)

Route Origin Authorization verifies that the announcing AS is authorized to originate a given IP prefix. Easy42 includes an automatic ROA caching engine.

- Policies can define separate IPv4 (`roa4`) and IPv6 (`roa6`) ROA URLs. The built-in `dn42` policy points at `https://dn42.burble.com/roa/dn42_roa_bird2_4.conf` and `..._6.conf`.
- The controller downloads and caches each file locally in the data directory. A refresh endpoint (`POST /api/network-policies/{id}/refresh-roa`) forces an update, and `POST /api/roa/refresh` refreshes all policies.
- During node sync, cached tables are deployed to `/etc/easy42_<policy>_roa4.conf` and `/etc/easy42_<policy>_roa6.conf` (for example `/etc/easy42_dn42_roa4.conf`).
- BIRD checks incoming routes using native static ROA tables. The generated names follow `<policy>_roa4` / `<policy>_roa6` for tables, `<policy>_roa4_static` / `<policy>_roa6_static` for protocols, and `<policy>_roa_check` for the function:

```bird
roa4 table dn42_roa4;
protocol static dn42_roa4_static {
    roa4 { table dn42_roa4; };
    include "/etc/easy42_dn42_roa4.conf";
}

roa6 table dn42_roa6;
protocol static dn42_roa6_static {
    roa6 { table dn42_roa6; };
    include "/etc/easy42_dn42_roa6.conf";
}

function dn42_roa_check() {
    if net.type = NET_IP4 then {
        if roa_check(dn42_roa4, net, bgp_path.last) = ROA_VALID then return true;
        if roa_check(dn42_roa4, net, bgp_path.last) = ROA_UNKNOWN then return true; # Non-strict mode
        return false; # ROA_INVALID
    } else {
        if roa_check(dn42_roa6, net, bgp_path.last) = ROA_VALID then return true;
        if roa_check(dn42_roa6, net, bgp_path.last) = ROA_UNKNOWN then return true; # Non-strict mode
        return false; # ROA_INVALID
    }
}
```

With `roa_strict: true`, `ROA_UNKNOWN` is rejected as well, leaving only `ROA_VALID`. If a family has no ROA URL, that family's check returns true (no restriction). Custom policies can reuse the same mechanism with their own URLs.
