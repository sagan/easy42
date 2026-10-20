# System Architecture & Technical Design

This document details the internal architecture, link-local addressing schemes, interface management, and cryptographic security model of **Easy42**.

---

## High-Level Architecture

Easy42 is structured around a central orchestrator controller (providing both a modern Web UI and a scriptable CLI) that configures managed nodes either **agent-lessly** over SSH/SFTP or **agent-powered** via `easy42-agent` over persistent WebSockets.

```
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                              easy42 Controller                              │
 │                                                                             │
 │   ┌──────────────────────┐      ┌───────────────────────┐                   │
 │   │  Web UI (React/Vite) │      │      CLI (Cobra)      │                   │
 │   └──────────┬───────────┘      └───────────┬───────────┘                   │
 │              │                              │                               │
 │              ▼                              ▼                               │
 │   ┌─────────────────────────────────────────────────────────────────────┐   │
 │   │                           Engine & State                            │   │
 │   │      - Node & Link Topology Graph       - WebSocket Agent Hub       │   │
 │   │      - Network Policies & Routing       - SQLite Metrics & Cron     │   │
 │   │      - Cloudflare DNS Sync Engine       - ROA Cache Manager         │   │
 │   │      - Visual Blocks & Layouts          - Prometheus Exporter       │   │
 │   │      - /etc/hosts Sync Manager          - Device Helper Task Engine │   │
 │   │      - Encrypted State Store            - Config Apply & Backups    │   │
 │   └──────────────────────────────────┬──────────────────────────────────┘   │
 │                                      │                                      │
 │                                      ▼                                      │
 │   ┌─────────────────────────────────────────────────────────────────────┐   │
 │   │                    Core Compiler & Templates                        │   │
 │   │      - WireGuard Configs (wg42*.conf)                               │   │
 │   │      - BIRD 2 Dynamic Routing (/etc/bird_easy42.conf)               │   │
 │   │      - nftables Ruleset (/etc/easy42.nft)                           │   │
 │   │      - Config Hooks & Custom Template Overrides                     │   │
 │   └──────────────────────────────────┬──────────────────────────────────┘   │
 └──────────────────────┬───────────────┴───────────────┬──────────────────────┘
                        │                               │
             SSH / SFTP │ (Agent-Less)       WebSocket  │ (Agent Mode)
                        │                     (TLS/WSS) │
                        ▼                               ▼
       ┌─────────────────────────────┐   ┌─────────────────────────────┐
       │     Remote Node A (SSH)     │   │    Remote Node B (Agent)    │
       │  ┌───────────────────────┐  │   │  ┌───────────────────────┐  │
       │  │ Linux Kernel WireGuard│  │   │  │   easy42-agent (Rust) │  │
       │  │ /etc/wireguard/wg42B  │  │   │  │   Telemetry & Cron    │  │
       │  └───────────────────────┘  │   │  └───────────┬───────────┘  │
       │  ┌───────────────────────┐  │   │  ┌───────────▼───────────┐  │
       │  │ BIRD 2 (MP-BGP + BFD) │  │   │  │ Linux Kernel WireGuard│  │
       │  └───────────────────────┘  │   │  │ /etc/wireguard/wg42A  │  │
       │  ┌───────────────────────┐  │   │  └───────────────────────┘  │
       │  │ nftables (easy42 tbl) │  │   │  ┌───────────────────────┐  │
       │  └───────────────────────┘  │   │  │ BIRD 2 (MP-BGP + BFD) │  │
       └──────────────┬──────────────┘   │  └───────────────────────┘  │
                      │                  │  ┌───────────────────────┐  │
                      │   P2P WireGuard  │  │ nftables (easy42 tbl) │  │
                      └────── Tunnel ────┴──┴───────────────────────┘  │
                         (IPv6 Link-Local / IPv4 Link-Local)
```

---

## Data Plane & Dual-Stack Link-Local Addressing

Every WireGuard link created between two nodes uses a dedicated **point-to-point WireGuard interface** named `wg42<peer>` for internal mesh peers or `wg42-<peer>` for external unmanaged peers. Manual links use an existing interface instead.

### 1. Deterministic IPv6 Link-Local (`fe80::/64`)

Instead of requiring dedicated subnets for tunnel interconnects, Easy42 assigns an RFC 4291 compliant IPv6 link-local address to the tunnel interface, derived deterministically from the node's main IPv4 address:

$$\text{IPv4: } 192.168.100.10 \implies \text{IPv6 Link-Local: } \texttt{fe80::c0a8:640a/64}$$

Both ends of the link use their respective link-local addresses. Because link-local traffic is scoped strictly to the interface, the same link-local subnet can exist across multiple interfaces without collision.

### 2. Deterministic IPv4 Link-Local (`169.254.X.X/32`, RFC 3927)

For integration with third-party software or routing appliances that do not support RFC 8950 Extended Next Hop IPv4 over IPv6, Easy42 can optionally assign a deterministic pair of IPv4 link-local addresses (`169.254.X.X/32`) to the WireGuard interface:

- The IP addresses are derived by hashing the sorted pair of node main IPs plus the link index (`"<ipLow>-<ipHigh>#<linkIndex>"`), producing a deterministic consecutive `/30` address pair (`4*k + 1` and `4*k + 2`) within `169.254.1.1`–`169.254.254.254`.
- Both endpoints receive dedicated IPv4 link-local addresses, enabling native IPv4 next-hop peering over the tunnel. This is opt-in per link (`assign_ipv4`).

### 3. Multiprotocol BGP over IPv6 Link-Local (RFC 8950 / RFC 5549)

BIRD establishes a single BGP peering session over the IPv6 link-local address bound to the interface (`neighbor fe80::... % 'wg42peer'`). By enabling **Extended Next Hop**, BIRD transmits both IPv4 and IPv6 routing tables over this single session:

```bird
protocol bgp 'easy42_peer_nodeB' from easy42_peer {
    interface "wg42nodeB";
    local fe80::c0a8:640a as SELF_AS;
    neighbor fe80::c0a8:640b % 'wg42nodeB' as 4224420002;
}
```

This eliminates dual-stack tunnel configuration and separate IPv4 BGP sessions.

### 4. Safe Interface Naming (`IFNAMSIZ` 15-Character Compliance)

Linux network interface names are restricted to 15 characters plus a null terminator (`IFNAMSIZ = 16`). Easy42 enforces this transparently:

- Node names can be arbitrarily long and descriptive (e.g. `frankfurt-edge-router-01`).
- The interface compiler truncates the peer portion to 11 characters for internal peers (`wg42<peer>`) and 10 characters for external peers (`wg42-<peer>`) so the total never exceeds 15 characters. Trailing `-`/`_` characters are trimmed, and `GetInterfaceNameWithSuffix` reserves room for numeric disambiguation suffixes (`wg421`, `wg422`, …) when multiple links exist between the same pair.

---

## WireGuard & Manual Links

For each WireGuard link, Easy42 compiles `/etc/wireguard/wg42<peer>.conf`:

```ini
# Auto-generated by easy42. Do not edit manually.

[Interface]
Address = fe80::c0a8:640a/64,169.254.120.1/32
ListenPort = 24192
PrivateKey = <unencrypted_node_private_key>
MTU = 1420
FwMark = 0x2a
# Some OS (debian) wg-quick seems to force set wg rp_filter to 2 on start
PostUp = sysctl -w net.ipv4.conf.%i.forwarding=1; sysctl -w net.ipv4.conf.%i.rp_filter=0; ip route add 169.254.120.2 dev %i; ip -6 route add fe80::c0a8:640b dev %i || true
Table = off

[Peer]
PublicKey = 0Yx+7eQ6gIqXy...=
PresharedKey = <optional_preshared_key>
AllowedIPs = fe80::c0a8:640b/128, 169.254.120.2/32, 0.0.0.0/0, ::/0
Endpoint = 198.51.100.2:25831
PersistentKeepalive = 25
```

The `169.254.120.1/32` address, IPv4 route, and `FwMark` are present only when the link has `assign_ipv4` enabled and/or a policy/link `fwmark` is configured. The IPv6 `Address` always carries `/64` (the derived link-local prefix), while an explicit address may be supplied through the link endpoint.

### Parameter Rationale

- **`Table = off`**: **Crucial.** Prevents `wg-quick` from inserting default routes (`0.0.0.0/0`) into the kernel routing table and hijacking internet connectivity. Routing decisions are handled strictly by BIRD.
- **`AllowedIPs = <peer_link_locals>, 0.0.0.0/0, ::/0`**: In WireGuard, `AllowedIPs` acts as a cryptokey-routing filter. To allow the interface to forward transit packets for any network prefix advertised by BGP, `AllowedIPs` must accept wildcard traffic (`0.0.0.0/0` and `::/0`).
- **`ListenPort = 2XXXX`**: Calculated deterministically using a 32-bit FNV-1a hash of the peer's IP (`20000 + hash(peer_ip) % 10000`). This avoids port collisions while maintaining stable port assignments across restarts.
- **`PostUp`**: Runs three things on interface creation: enables IPv4 forwarding on the interface (`net.ipv4.conf.%i.forwarding=1`), disables strict Reverse Path Filtering (`net.ipv4.conf.%i.rp_filter=0`), and installs a host route to the peer's IPv4 link-local (when IPv4 link-local addressing is in use) plus a best-effort IPv6 route to the peer's link-local. In multi-path mesh networks, return packets frequently enter via a different interface than outbound traffic (asymmetric routing); without `rp_filter=0`, the Linux kernel silently drops valid transit packets.
- **`MTU = 1420`**: The default, chosen to leave room for WireGuard and IPv6 headers inside a 1500-byte path. It can be overridden per link endpoint.
- **`FwMark`**: Optional firewall mark emitted only when set on the link or its effective network policy. It lets policy routing and nftables rules steer WireGuard-encapsulated traffic.
- **`PersistentKeepalive = 25`**: Keeps stateful firewall entries and NAT mappings alive on intermediate routers. Automatically enabled only when an endpoint is resolvable for the peer; it is left at `0` (omitted) for unidirectional/NAT-only links.
- **`PresharedKey` (PSK)**: Optional post-quantum symmetric encryption key shared between peers.

### Manual Links (Non-WireGuard Tunnels & Existing Interfaces)

In addition to WireGuard, Easy42 supports **Manual Links** for integrating pre-existing interfaces into the dynamic routing mesh (such as GRE, SIT, IPIP, VXLAN, physical links, or existing tunnel software):

- Manual links bypass WireGuard interface generation and configure BIRD 2 BGP peering directly over the specified interface and neighbor addresses.
- Policies, link costs, BFD, and firewall rules apply seamlessly to manual links.

---

## Entrypoint Selection Algorithm

When generating links between Node A and Node B:

1. Easy42 compares `entrypoints` tags on both nodes (for example `lan`, `cloud`, `ipv6`). Two `none` endpoints are never matched with each other.
2. If no entrypoint tag matches, the source node's own `tags` are compared against the destination node's entrypoint tags. This lets a node-level tag select a specific reachable entrypoint.
3. If neither match produces a candidate, the first non-`none` entrypoint on the destination node is used.
4. If every destination entrypoint is marked `none` (strictly behind a NAT/firewall without port forwarding), no `Endpoint = ...` is emitted for that direction. The link becomes unidirectional: only the side with a reachable endpoint gets an `Endpoint`, while the NATed side initiates the connection and maintains it via `PersistentKeepalive = 25`.
5. Endpoint ports come from the selected entrypoint's explicit port or port range, then fall back to the requested listen port or a peer-IP-derived port. `ExternalPort` takes precedence over `Port` when present.

---

## Security & Encryption Architecture

1. **Encrypted Key Storage at Rest**:
   WireGuard private keys and preshared keys stored in `config.json` are encrypted using **XAES-256-GCM** with a random 24-byte nonce. Each ciphertext is `base64(nonce + ciphertext + tag)`. The controller decrypts them only in memory while the vault is unlocked; deployed node configurations necessarily contain the plaintext keys that WireGuard requires.
2. **Key Derivation (Argon2id)**:
   The Data Encryption Key (DEK) is encrypted with a Key Encryption Key (KEK) derived from the administrator password using **Argon2id** (memory: 64 MB, time: 3 iterations, parallelism: 2). The stored DEK blob is `base64(salt_16B + nonce_24B + ciphertext)`; the 16-byte salt is generated per encryption.
3. **In-Memory Protection (`memguard`)**:
   In-flight plaintext private keys, PSKs, and the decrypted DEK are held in volatile memory protected by [`awnumar/memguard`](https://github.com/awnumar/memguard). Memory pages are locked against swapping to disk, surrounded by guard pages, and wiped on exit.
4. **Agent Token Authentication**:
   Agents authenticate to the controller with a pre-shared bearer token verified during WebSocket registration. Use `wss://` in production so the transport is also protected by TLS.
5. **Stateless Session Cookies**:
   Frontend Web UI authentication uses an HMAC-SHA256-signed, HTTP-only, `SameSite=Lax` session cookie. The signing secret is generated randomly on first boot, and tokens embed an expiry plus a fingerprint of the password hash so that changing the admin password invalidates existing sessions. The cookie is also marked `Secure` when the request arrives over HTTPS.
6. **SSH Node Management & Device Helper (OpenSSH Native Trust Model)**:
   For agent-less node synchronization, configuration deployment, and Device Helper host automation, Easy42 directly and exclusively leverages the local OS user's OpenSSH environment:
   - **Authentication**: Authenticates using `~/.ssh/id_ed25519` (with fallback to `~/.ssh/id_rsa`, `~/.ssh/id_ecdsa`, or an active SSH Agent `$SSH_AUTH_SOCK`). Easy42 does not store or manage SSH passwords or separate SSH private keys in its configuration database.
   - **Host Key Verification**: Enforces strict server host key verification against `~/.ssh/known_hosts`. Connections to unknown or mismatched host keys are rejected immediately to protect against man-in-the-middle attacks. Standard `~/.ssh/config` overrides (hosts, users, ports, identities) are seamlessly respected.
