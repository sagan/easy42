export interface PortSpec {
  port?: number;
  external_port?: number;
  range?: string;
}

export interface Entrypoint {
  ip?: string;
  ports?: PortSpec[];
  tags?: string[];
  mtu?: number;
}

export interface Node {
  name: string;
  host: string;
  ip: string;
  interface: string;
  asn: number;
  entrypoints?: Entrypoint[];
  tags?: string[];
  x?: number;
  y?: number;
}

export interface LinkEnd {
  name: string;
  interface: string;
  address: string;
  listen_port: number;
  endpoint?: string;
  private_key?: string;
  public_key: string;
  persistent_keepalive: number;
  mtu?: number;
}

export interface Link {
  from: LinkEnd;
  to: LinkEnd;
  tags?: string[];
}

export interface InterfaceInfo {
  name: string;
  addresses: string[];
  up: boolean;
  type?: string;
  mtu?: number;
}

export interface WgPeerStatus {
  public_key: string;
  endpoint: string;
  allowed_ips: string[];
  latest_handshake: string;
  transfer_rx_bytes: number;
  transfer_tx_bytes: number;
  persistent_keepalive: number;
}

export interface WgInterfaceStatus {
  name: string;
  public_key: string;
  listen_port: number;
  peers: WgPeerStatus[];
}

export interface NodeStatus {
  name: string;
  host: string;
  connected: boolean;
  last_seen: string;
  hostname: string;
  interfaces?: InterfaceInfo[];
  wg_interfaces?: WgInterfaceStatus[];
  error?: string;
}

export interface ProbeResult {
  hostname: string;
  suggested_name: string;
  suggested_ip: string;
  suggested_interface: string;
  suggested_asn: number;
  interfaces: InterfaceInfo[];
  detected_entrypoints: Entrypoint[];
}

export interface AuthStatus {
  authenticated: boolean;
  unlocked: boolean;
  has_config: boolean;
}

export interface SyncAction {
  node_name: string;
  host: string;
  type: string;
  interface: string;
  target_file: string;
  file_content?: string;
  diff?: string;
  description: string;
}

export interface SyncResult {
  node_name: string;
  action: string;
  success: boolean;
  error?: string;
  output?: string;
  duration_ms: number;
}

export interface SyncStatus {
  last_sync: string;
  results: SyncResult[];
}
