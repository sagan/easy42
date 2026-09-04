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

export interface KernelRouteRule {
  table: number;
  prefixes: string[];
}

export interface NetworkSettings {
  public_asn?: number;
  confed_members?: string;
  export_prefixes?: string[];
  import_prefixes?: string[];
}

export interface Node {
  name: string;
  host?: string;
  is_external?: boolean;
  description?: string;
  ip?: string;
  interface?: string;
  asn: number;
  entrypoints?: Entrypoint[];
  tags?: string[];
  table?: number;
  static_routes?: string[];
  routes?: KernelRouteRule[];
  x?: number;
  y?: number;
  modified_at?: string;
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
  modified_at?: string;
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
  command?: string;
  description: string;
  needs_apply?: boolean;
  status?: "pending" | "synced";
  diff_status?: "create" | "update" | "delete" | "synced";
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

export interface StateInterface {
  name: string;
  target_file: string;
  config_hash: string;
  peer_node?: string;
  peer_pub_key?: string;
  listen_port?: number;
  address?: string;
  status: string;
  latest_handshake?: string;
  working_state?: "working" | "not_working" | "unknown";
  transfer_rx_bytes?: number;
  transfer_tx_bytes?: number;
  applied_at?: string;
}

export interface StateNode {
  name: string;
  host: string;
  last_seen?: string;
  bird_config_hash?: string;
  bird_applied_at?: string;
  interfaces: Record<string, StateInterface>;
}

export interface NetworkState {
  version: number;
  updated_at: string;
  nodes: Record<string, StateNode>;
}

export interface UpdateStateResponse {
  success: boolean;
  state: NetworkState;
  warnings?: string[];
}

export interface TaskMeta {
  id: string;
  title: string;
  description: string;
  category: string;
  weight: number;
}

export type TaskCheckStatus = "ready" | "done" | "incompatible" | "error";

export interface TaskStatusResult {
  task_id: string;
  node_name: string;
  status: TaskCheckStatus;
  message: string;
  exit_code: number;
  duration_ms: number;
}

export interface TaskRunResult {
  task_id: string;
  node_name: string;
  success: boolean;
  output: string;
  exit_code: number;
  duration_ms: number;
}
