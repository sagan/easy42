# Dynamic Routing with BIRD 2 & Network Policies

This document explains how **Easy42** configures **BIRD 2** for automated dynamic routing, sub-second failover, traffic engineering, and route filtering.

---

## BIRD 2 Configuration Architecture

Easy42 generates `/etc/bird_easy42.conf` on each managed node and links it into the node's main BIRD configuration (`/etc/bird/bird.conf`) via:

```bird
include "/etc/bird_easy42.conf";
```

### 1. BGP Confederations & Autonomous Systems

Easy42 supports two main BGP architectures:

1. **Flat Private ASN / iBGP Mesh**: All nodes share the same ASN or use private AS numbers.
2. **BGP Confederation (RFC 5065)**:
   Each internal node is assigned a unique private ASN within `4224420000`–`4224429999`, grouped under a single parent Confederation AS (e.g. your public DN42 ASN `424242xxxx`):

   ```bird
   confederation CONFED_AS;
   confederation member yes;  # Internal peers
   ```

   To external peers (e.g., outside DN42 peerings):

   ```bird
   confederation CONFED_AS;
   confederation member no;   # External eBGP peers
   ```

   External peers see your entire network originating from your public ASN, hiding internal topology while eliminating iBGP full-mesh requirements.

### 2. Sub-Second Fast Convergence with BFD

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

Internal peer protocols enable `bfd yes;`, ensuring instant route withdrawal when an overlay path degrades or drops.

### 3. Route Origination & Loopback Announcement

Each node has a primary stable IP (e.g., `192.168.100.1` on `lo`). To originate this route into BGP without creating a routing loop on the local host:

```bird
protocol static static_self {
    ipv4;
    route 192.168.100.1/32 reject;
}
```

In BIRD, a `reject` route is purely local to the routing daemon; it injects the `/32` prefix into BIRD's routing table (`master4`) for export to peers, while the local kernel routes packets destined for `192.168.100.1` to the local loopback interface.

### 4. Multi-Table Kernel FIB Synchronization & Metrics

Learned BGP routes are exported to the Linux kernel FIB:

- **Main Routing Table** (`table 254` by default, or user-configured):
  ```bird
  protocol kernel kernel_v4 {
      metric 100;
      ipv4 {
          export filter {
              krt_prefsrc = SELF_IP;
              if source ~ [ RTS_BGP ] then accept;
              reject;
          };
          import none;
      };
      kernel table TABLE;
  }
  ```
  Setting `krt_prefsrc = SELF_IP` ensures local applications use the overlay IP when originating mesh traffic.
- **External Routing Table (`external_table`)**: Directs external routes (marked with BGP Large Community `(CONFED_AS, 1, 1)`) into an isolated routing table for Policy-Based Routing (PBR).
- **Internet Routing Table (`internet_table`)**: Isolates upstream transit or default routes into a dedicated kernel table.

### 5. Dynamic Kernel Route Import & Static Route Broadcasting

Nodes can dynamically redistribute existing routes into BGP:

- `routes`: Imports prefixes directly from specific Linux kernel routing tables (e.g., table 100 subnets) into BIRD using kernel protocol `learn`.
- `static_routes`: Injects static prefix announcements directly into BIRD's static protocol block for unconditional BGP advertisement.

### 6. Route Leak Protection

To prevent accidental leaks of default routes or public internet subnets into private overlays or DN42:

```bird
define INTERNET = [ 0.0.0.0/0, 0.0.0.0/1, 128.0.0.0/1 ];
define INTERNET6 = [ ::/0, ::/1, 8000::/1 ];

import filter {
    if net ~ INTERNET then reject;
    # ...
};
```

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

| Policy        | Target Usage             | Internet Leak Protection |      Forward Filter       |     Input Filter     | Default SNAT  | ROA Enabled  |
| :------------ | :----------------------- | :----------------------: | :-----------------------: | :------------------: | :------------ | :----------: |
| **`default`** | Internal mesh nodes      |           Yes            |            No             |          No          | Disabled      |   Optional   |
| **`dn42`**    | External DN42 peers      |           Yes            | Yes (172.20/14, fd00::/8) | Yes (BGP, DNS, ICMP) | `external_ip` | Yes (Burble) |
| **`none`**    | Unrestricted tunnels     |            No            |            No             |          No          | Disabled      |      No      |
| _Custom_      | Custom peering / transit |       Configurable       |    Configurable CIDRs     |  Configurable Ports  | Configurable  |  Custom URL  |

### Link Costs, Negative Costs & Latency Steering

Each policy defines a base `cost` (default: 100), which can be overridden on individual links:

- During BGP import, BIRD calculates: `bgp_local_pref = DEFAULT_LOCAL_PREF(10000) − cost`
- **Negative Link Costs**: Easy42 fully supports negative costs (e.g. `-50`), which increase `bgp_local_pref` above 10000, allowing explicit preference of high-bandwidth or primary transit paths over default routes.

### Routing Policy Modes

Each policy or link can enforce a BGP routing policy:

- **`full`** (default): Normal transit mesh routing (imports and exports all valid routes).
- **`stub`** (`originate_only`): Advertises only the node's local loopback routes and imports full routes from peers.
- **`receive_only`**: Imports routes from the peer but never exports any routes.
- **`advertise_only`**: Exports local routes to the peer but rejects all incoming route announcements.

---

## Route Origin Authorization (ROA)

Route Origin Authorization verifies that the announcing AS is authorized to originate a given IP prefix. Easy42 includes an automatic ROA caching engine.

- When configured with a ROA URL (e.g., `https://dn42.burble.com/roa/dn42_roa_bird2_4.conf`), the controller periodically downloads and caches the ROA file locally in the data directory.
- During node sync, the cached ROA tables are deployed to `/etc/bird_roa_<policy>_4.conf`.
- BIRD checks incoming routes using native static ROA tables:

```bird
roa4 table roa_table_dn42_4;
protocol static roa_proto_dn42_4 {
    roa4 { table roa_table_dn42_4; };
    include "/etc/bird_roa_dn42_4.conf";
}

function roa_check_dn42() {
    if roa_check(roa_table_dn42_4, net, bgp_path.last) = ROA_VALID then return true;
    if roa_check(roa_table_dn42_4, net, bgp_path.last) = ROA_UNKNOWN then return true; # Non-strict mode
    return false; # ROA_INVALID
}
```
