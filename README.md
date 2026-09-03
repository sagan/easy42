# Easy42

It's a new project `easy42`. We're going to create a overlay software networking tool that's a little similar to zerotier, tailscale, EasyTier, netbird, netmaker, or the mix / combination or lightweight alternative of these tools.

The primary design goal is to be used in DN42 networks to help create & manage the wg mesh network of one AS's internal servers. But it can also be used in the one's personal private networks without DN42 involved.

The software provide both CLI and Web UI interface for managing networks.

## Design Principles

- Linux and WireGuard only. The data plane uses Linux kernel wireguard exclusively.
- It generates standard /etc/wireguard/wg*.conf files and use standard `wg` & `wg-quick` tools to manage wireguard interfaces. ipv6 link only addresses are used on wg interfaces by default.
- NAT traversal (like STUN server) is not supported at this time (we may implement this feature in the future).
- It supports flexible topology, like hub-and-spoke, full mesh, or a custom topology.
- It also provides Bird configuration auto generation for the connected peers so you can run BGP (or other IGP protocols like OSPF) over WireGuard easily.
- In the first phrase we will use an agent-less approach. The server uses standard SSH / SFTP to manage the peers' configurations. In the next phase we may consider to deploy agents on nodes to handle more complex tasks like NAT traversal.

```
Topology / Peer Spec (YAML/JSON)
        │
        ▼
┌───────────────────────────────┐
│     easy42 Core Compiler      │
│  - Computes wg peer configs   │
│  - Computes bird peer configs │
│  - Generates diffs / actions  │
└───────────────┬───────────────┘
                │
        ┌───────┴───────┐
        ▼               ▼
 ┌─────────────┐ ┌─────────────┐
 │ SSH Applier │ │ Local Agent │ (Future / Optional)
 │ (Phase 1)   │ │ (Phase 2)   │
 └─────────────┘ └─────────────┘
```


## Software Design

The `easy42` is running as a Go web app. The frontend project uses TypeScript, Vite, React and Material UI and the dist files are embedded in the binary.

The frontend core UI is a visual Topology editor. User can add new node, edit node info, and create relations (wg links) between nodes.

All configurations are stored in server as plain text json file. The json file stored all nodes & wg links info but some sensitive info (like wg interface private keys) are encrypted by an automatically generated encryption key which is encrypted by user provided password.

As a supplement, the software also provides a CLI interface to do some common actions like applying topology configs to devices, checking wireguard interface status on devices, etc. We use https://github.com/spf13/cobra for CLI. A standalone sub-command is used to start backend as web app.

## Topology / Peer Spec

The `node` object represents a device / node:

- `name`: the global unique name of the node, e.g. "router-gw1", "server-main-1". The name must be a valid hostname of at most 11 chars. By default it uses the device hostname as name, if device hostname is too long or duplicate with other devices, trim it and / or add numeric suffix.
  - We create wg links using `wg42<name>` as interface name where `<name>` is the peer's name. Since in Linux the interface name max length is 15 chars, the peer name should be at most 11 chars.
- `host`: the device ssh host. Can be an alias in `~/.ssh/config` or an direct ip address / hostname.
- `ip`: the "main" IPv4 address of the device / node. It must be a existing static & stable ipv4 address on any interface of the device. The address should be globally unique. If the device is a lan device / router it's recommended to use it's primary lan ip (such as `192.168.100.1` ); If the device is a VPS / Internet server it's recommanded to config a private ipv4 as main IP on the loopback or a dummy interface. The main ip MUST NOT be configured on easy42 managed `wg42*` interface, but it should be OK to use a ip on other `wg*` or other type tunnel interface as long as the interface & ip is stable. It's the user's responsibility to have this ip configured on the device for now. Though not recommended, it's able to use the device's public ipv4 as main ip (if the device has it's public ipv4 configured on it's main nic). The generated bird config will broadcast this ip (as /32) to other nodes so it will be globally routable in the user's network.
- `interface`: the main IP interface name, e.g. "lo", "dn42", "eth0".
- `asn`: AS Number of this node (must be a valid private AS number). By default we assign a unique private ASN in 4299420000-4299429999 range automatically to each device. User can also choose to use a same ASN (like their DN42 ASN) for all or some of their devices.
- `entrypoints`: The external entrypoints of the device. A device can has multiple entrypoints. A entrypoint is usually the public ipv4/ipv6 address or domain name that can reach the device from the Internet. e.g. `{ip: "1.2.3.4"}`. We support device which is behind NAT but some port forwardings (DNAT) are configured on the device's gateways so it's able to reach those devices from specific port numbers. In this case, the port number should also be provided, e.g. `{ip: "[IP_ADDRESS]", ports: [{external_port: 51820, port: 51820}]}`. All possible fields of the entrypoint object:
  - `ip` : the ip (v4 or v6), or a domain.
  - `ports` : (optional) The available ports array for wg listening or peering. Each element could be a port number, or a string represents port range (e.g. `2000-2999`) or a object `{port: Port, external_port:? Port}`. The `external_port` is optional which is used to specify different external port for devices behind NAT.
  - `tags` : The string[] tag array.
  - One specific "none" endpoint is automatically inserted as the last element of `endpoints` array when the node is created. This element only has `tags` field but no other fields. This endpoint means the device is strictly behind NAT / firewall so it can't be accessed from other nodes actively. The user can edit this "none" entrypoint's tags but can't delete it in UI.

The required and global unique fields: `name`, `ip`.
The required fields: `host`.

The `link` object represent a wg link between two devices:

- `from`: the one end node object: `{name, ...}`
  - `name` : the node name.
  - `interface` : the self node wg interface name. Automatically generated by default.
  - `address` : the node's wg interface `Address`. By default we use ipv6 link-local address that's derived from the self node main ip. E.g. if the node `ip` is `192.168.100.10`, the ipv6 link-local address is `fe80::192:168:100:10/64`.
  - `listen_port` : The local device wg listening port. By default we use `20000 + hash(other_end_peer_ip) % 10000` (`2XXXX`) as port.
  - `endpoint` : the self node wg port external access endpoint. optional. By default it's derived automatically (see below).
  - `private_key` : the encrypted self wg private key. Generated automatically when the link is created.
  - `public_key` : the self wg public key.
  - `persistent_keepalive` : The wg `[Peer]` section's `PersistentKeepalive` interval in seconds. By default it's set to `25` if the other node `endpoint` exists, else `0` (mean disabled).
- `to`: the other end node object: `{name, ...}`
  - The orders or `from` and `to` is arbitrary but in program UI we follow a determined order: the node with smaller name (alphabetic order) comes first (as `from`) when creating links.
- `tags`: string[], the link's tags.

When creating wg.conf on devices, the `[Peer]` section is derived automatically since we only support peer-2-peer wg links that each wg interface has only one peer:

- `AllowedIPs` : Set to `<peer_ipv6>/128,0.0.0.0/0,::/0`. Note we set `Table = off` in wg.conf for all interfaces.
- `Endpoint` : By default the endpoint is derived automatically from the the two peering node's `endpoints` field in a deterministic way. The logic:
  1. First check the `endpoints` of thw two nodes and check if we can find a pair of endpoints which share as a same `tag`. If we find one, we use it, otherwise use the first endpoint in each node's `endpoints`.
  2. The node's found `endpoint` is used as the other node's wg peer `Endpoint`, with some caveats:
    - If the endpoint `ports` is not set, it means we can use all ports. We use the peer node's WireGuard listening port (which defaults to the self node's ASN last 5 digits `2XXXX`) as the self wg conf `[Peer]` section `Endpoint` port, joined with the peer node's `ip` field.
    - If the endpoint `ports` is defined, we select a (unused in other links from this node) port for the peer randomly. If 'external_port' is set, we use that, else use 'port'.
    - If the endpoint is "none" endpoint, we don't configure the other node's wg peer `Endpoint`.

## Node Configuration Push

When the user changes the network topology in UI and "Save", the backend will connect to involved nodes and update / create wg or bird configurations. This process uses standard SSH (for executing command on devices) / SFTP (for update device config file content or query it's current content) protocol with public key authentication. We automatically use default OpenSSH files `~/.ssh/config`, `~/.ssh/id_ed25519` and `~/.ssh/known_hosts` for server ssh addressing, host verification and authentication. It's the user's responsibility to ensure the backend's public key is authorized on all nodes and host keys exist in backend known_hosts file.

The backend should cache the nodes' current status info and persist them to a separate node status file.

We use standard on-device config file pathes and CLI tool pathes.

- `/etc/bird` for bird configuration
- `/etc/wireguard/wg*.conf` for wg configuration
- `wg / wg-quick` for managing wg interfaces
- `birdc` for managing bird.

It's the user's responsibility to have wireguard / bird installed on devices for now.

All device config updations or command executions must follow a idempotent & atomic way. For example, when updating a device config file, it must first fetch the current on-device file, compare with the new desired file content (ignoring comments), only if the contents are different it updates the file and executes the corresponding command to reload the configuration. We try best to keep the on-device config file existing comments and file structure if possible. When updating device file through SFTP, we first upload the file to a temporary name in the same directory and then rename it to the final target name to ensure atomicity.

## Data persistence

All data are stored in `Data Dir` which defaults to `~/.config/easy42` and can be customized by cmdline flag. For now we store all info in `config.json` file inside `Data Dir` as:

```json
{
  "password_hash": "", // the Argon2id hash of user password      
  "encrypted_dek": "", // the encrypted-by-password data encryption key
  "session_secret": "",// the random secret used in frontend authenticated session token signing / verification
  "nodes": [],
  "links": []  
}
```

Some fields are stored as encrypted base64 string (with salt embedded): the `nodes` element `private_key`. The encryption key is derived from user password and stored as `encrypted_dek`. The encryption altorighm is xaes-256-gcm ( filippo.io/xaes256gcm ). The xaes-256-gcm nonce is 24 bytes long and is embedded in the encrypted string so the final encrypted field is: `base64(nonce + ciphertext)`.

When the server starts the first time (config.json doesn't exist), generate a cryptographically random strong password of `[a-zA-Z0-9]{22}` format, initialize the config.json file with auto-generated password_hash, session_secret, and encrylted_dek fields, print the initial password to stderr.

The frontend session is authorized by a stateless http-only cookie which is derived from password_hash and session_secret. After the server started or restarted all authed browser sessions remain valid, but user will need to provide the password in the browser when it does any action that need the plaintext of encrypted fields (like wg private key). The inputed app password plaintext remains in backend memory until the server restarts or user manually "lock" it. We use github.com/awnumar/memguard to protect the in-memory app password and other credentials.

In frontend login page it only needs to input the password (no username).

## Frontend Topology editor

### Add New Node procedures

1. Enter new node ssh host.
2. The backend uses ssh to connect to the node, fetch is current status (ip(s) on all interfaces, hostname, etc) and present them to user in a form.
3. User checks and adjusts the fields, e.g. select the main ip & interface, provide asn, name etc, then submit the form.
4. The backend will create a new `node` object and store it to the config file, and update the frontend state.

### Add New wg link procedure

The user drag-and-drop from one node to the other to create a new link. The UI will show a dialog to let user select the `endpoint` settings (if the node has multiple endpoints), confirm `interface` name (auto generated, editable), `listen_port` (auto generated, editable), etc. After confirm, the backend will create a new `link` object and store it to the config file, and update the frontend state.

### Sync Button

In topology UI, clicking "Sync" button will trigger the backend to sync settings (wg link configs) to devices as described in "Node Configuration Push" chapter.

The UI displays "unsynced" links in graph as different style.

## Development Phrases

- Phrase 1 (current phrase): backend, frontend, topology editing UI (nodes, links), wg management functions. In this phrase it only update device `/etc/wireguard/wg*.conf` files and run `wg / wg-quick` to spawn interfaces or udpate / sync wg conf.
- Phrase 2: node service persistence (use systemd to auto-start wg interface on startup). bird / BGP related functions. We will design details later.
