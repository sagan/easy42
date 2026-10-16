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

export interface ConfigHook {
  type: string;
  target?: string;
  content: string;
}

export interface NetworkSettings {
  public_asn?: number;
  confed_members?: string;
  prefixes?: string[];
  local_dn42_networks?: string[];
  disallowed_dn42_networks?: string[];
  disallowed_dn42_cidrs?: string[];
  flapalerted_address?: string;
  flapalerted_port?: number;
  flapalerted_asn?: number;
  fa_address?: string;
  fa_port?: number;
  fa_asn?: number;
  flapalerted_url?: string;
  fa_url?: string;
}

export interface FlapAlertedMetrics {
  ActiveFlapCount: number;
  ActiveFlapTotalPathChangeCount: number;
  TotalPathChangeCount: number;
  AverageRouteChanges90: string | number;
  Sessions: number;
}

export interface CloudflareDNSConfig {
  zone_id?: string;
  api_token?: string;
  base_domain?: string;
  publish_ipv6_own_name?: boolean;
}

export interface DNSSyncResult {
  created: number;
  updated: number;
  deleted: number;
  ignored: number;
  errors?: string[];
  message?: string;
}

export interface SNATConfig {
  enabled: boolean;
  condition?: string;
  target?: string;
}

export interface NetworkPolicy {
  id: string;
  name: string;
  description?: string;
  is_internal?: boolean;
  cost?: number;
  local_networks?: string[];
  allowed_dst_cidrs?: string[];
  allowed_src_cidrs?: string[];
  disallowed_dst_cidrs?: string[];
  disallowed_src_cidrs?: string[];
  reject_internet?: boolean;
  filter_forward?: boolean;
  filter_input?: boolean;
  input_allow_icmp?: boolean;
  input_allow_icmp6?: boolean;
  input_tcp_ports?: string[];
  input_udp_ports?: string[];
  snat?: SNATConfig;
  forward_snat?: boolean;
  forward_snat_target?: string;
  dscp_ingress?: number;
  dscp_egress?: number;
  block_ingress_new?: string;
  ingress_new_whitelist?: string[];
  roa4?: string;
  roa6?: string;
  roa_strict?: boolean;
  fwmark?: string;
  preference?: number;
  mark?: string;
  routing_policy?: string;
  bird_import_filter?: string;
  bird_export_filter?: string;
  bird_export_fillter?: string;
}

export interface ConfigTemplate {
  id: string;
  name: string;
  type: "wg" | "bird" | "nft" | string;
  description?: string;
  content: string;
  is_builtin?: boolean;
  modified_at?: string;
}

export interface Node {
  name: string;
  host?: string;
  is_external?: boolean;
  description?: string;
  note?: string;
  ip?: string;
  ip6?: string;
  external_ip?: string;
  external_ip6?: string;
  interface?: string;
  asn: number;
  entrypoints?: Entrypoint[];
  tags?: string[];
  table?: number;
  external_table?: number;
  internet_table?: number;
  metric?: number;
  static_routes?: string[];
  routes?: KernelRouteRule[];
  config_hooks?: ConfigHook[];
  wg_template?: string;
  bird_template?: string;
  nft_template?: string;
  x?: number;
  y?: number;
  modified_at?: string;
  mode?: "ssh" | "agent" | string;
  agent_token?: string;
  agent_version?: string;
  agent_last_seen?: string;
  enable_fa?: boolean;
  update_hosts?: boolean;
}

export interface LinkEnd {
  name: string;
  type?: "wireguard" | "manual" | string;
  interface: string;
  address: string;
  address4?: string;
  neighbor_address?: string;
  listen_port: number;
  endpoint?: string;
  private_key?: string;
  public_key: string;
  psk?: string;
  persistent_keepalive: number;
  mtu?: number;
  use_ip?: boolean;
  resolved_endpoint?: string;
  policy?: string;
  routing_policy?: string;
  cost?: number;
  fwmark?: string;
  preference?: number;
  mark?: string;
  note?: string;
}

export interface Link {
  type?: "wireguard" | "manual" | string;
  from: LinkEnd;
  to: LinkEnd;
  tags?: string[];
  assign_ipv4?: boolean;
  psk?: string;
  modified_at?: string;
}

export interface RevealLinkPskRequest {
  from: string;
  to: string;
  interface?: string;
  password: string;
}

export interface RevealLinkPskResponse {
  from: string;
  to: string;
  interface?: string;
  psk: string;
}

export interface ResetLinkPskRequest {
  from: string;
  to: string;
  interface?: string;
  psk?: string;
}

export interface ResetLinkPskResponse {
  from: string;
  to: string;
  interface?: string;
  psk: string;
  message: string;
}

export interface RevealLinkPrivateKeyRequest {
  from: string;
  to: string;
  node?: string;
  interface?: string;
  password: string;
}

export interface RevealLinkPrivateKeyResponse {
  from: string;
  to: string;
  node: string;
  interface?: string;
  private_key: string;
  public_key: string;
}

export interface ResetLinkPrivateKeyRequest {
  from: string;
  to: string;
  node?: string;
  interface?: string;
  private_key?: string;
}

export interface ResetLinkPrivateKeyResponse {
  from: string;
  to: string;
  node: string;
  interface?: string;
  private_key: string;
  public_key: string;
  message: string;
}

export interface InterfaceInfo {
  name: string;
  addresses: string[];
  up: boolean;
  type?: string;
  mtu?: number;
  flags?: number;
  rx_bytes?: number;
  tx_bytes?: number;
}

export const IFACE_FLAG_PRIMARY = 1 << 0;
export const IFACE_FLAG_PHYSICAL = 1 << 1;

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

export interface DiskMetrics {
  path: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
}

export interface SystemMetrics {
  cpu_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  uptime_seconds?: number;
  load_avg?: number[];
  disks?: DiskMetrics[];
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
  mode?: "ssh" | "agent" | string;
  agent_version?: string;
  primary_interface?: string;
  metrics?: SystemMetrics;
  disks?: DiskMetrics[];
}

export interface ProbeResult {
  hostname: string;
  suggested_name: string;
  suggested_ip: string;
  suggested_ip6?: string;
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
  nftables_config_hash?: string;
  nftables_applied_at?: string;
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
  failed_nodes?: Record<string, string>;
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

// Looking Glass Types
export type ParameterType = "string" | "ip_or_cidr" | "number" | "select" | "boolean" | "host_port";

export interface TaskParam {
  key: string;
  label: string;
  type: ParameterType;
  description?: string;
  default_value?: string;
  required: boolean;
  options?: string[];
  regex?: string;
}

export interface LookingGlassTask {
  id: string;
  name: string;
  description?: string;
  category: string;
  command_tmpl: string;
  parser: string;
  params: TaskParam[];
  is_builtin?: boolean;
  timeout_sec?: number;
}

export interface TasksResponse {
  builtin: LookingGlassTask[];
  custom: LookingGlassTask[];
}

export interface LGRunRequest {
  nodes: string[];
  task_id?: string;
  ad_hoc?: boolean;
  custom_command?: string;
  custom_parser?: string;
  params?: Record<string, string>;
  timeout_sec?: number;
}

export interface BirdProtocolEntry {
  name: string;
  proto: string;
  table: string;
  state: string;
  since: string;
  info: string;
  connected: boolean;
}

export interface BirdRouteEntry {
  network: string;
  best: boolean;
  from_proto: string;
  since: string;
  metric?: string;
  via?: string;
  interface?: string;
  next_hop?: string;
  as_path?: string[];
  communities?: string[];
  local_pref?: string;
  origin?: string;
}

export interface BirdRouteResult {
  target?: string;
  routes: BirdRouteEntry[];
}

export interface PingPacket {
  seq: number;
  ttl: number;
  time_ms: number;
  host?: string;
}

export interface PingResult {
  host: string;
  ip?: string;
  packets_sent: number;
  packets_received: number;
  packet_loss_pct: number;
  min_rtt_ms: number;
  avg_rtt_ms: number;
  max_rtt_ms: number;
  mdev_rtt_ms: number;
  packets?: PingPacket[];
}

export interface TcpPingResult {
  target: string;
  resolved_addr: string;
  packets_sent: number;
  packets_received: number;
  packet_loss_pct: number;
  min_rtt_ms: number;
  avg_rtt_ms: number;
  max_rtt_ms: number;
  mdev_rtt_ms: number;
  rtts: number[];
}

export interface TracerouteHop {
  hop: number;
  host: string;
  ip?: string;
  asn?: string;
  times_ms: number[];
  loss_pct?: number;
  timed_out: boolean;
}

export interface TracerouteResult {
  target: string;
  hops: TracerouteHop[];
}

export interface MTRHop {
  hop: number;
  host: string;
  ip?: string;
  loss_pct: number;
  sent: number;
  last_ms: number;
  avg_ms: number;
  best_ms: number;
  worst_ms: number;
  stdev_ms: number;
}

export interface MTRResult {
  target: string;
  hops: MTRHop[];
}

export interface LGNodeResult {
  node_name: string;
  task_id?: string;
  command: string;
  raw_output: string;
  exit_code: number;
  duration_ms: number;
  error?: string;
  parser: string;
  parsed?: any;
}

export interface LGRunResponse {
  task_id?: string;
  command: string;
  results: Record<string, LGNodeResult>;
}

export interface GraphBlock {
  id: string;
  name: string;
  color: string;
  x: number;
  y: number;
  width: number;
  height: number;
  nodes?: string[];
}

export interface NodeMetricPoint {
  timestamp: number;
  cpu_percent: number;
  memory_used_bytes: number;
  memory_total_bytes: number;
  uptime_seconds: number;
  load_1m: number;
  load_5m: number;
  load_15m: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
  net_rx_rate: number;
  net_tx_rate: number;
  traffic_rx_bytes?: number;
  traffic_tx_bytes?: number;
}

export interface NodeMetricsResponse {
  node: string;
  range: string;
  interface?: string;
  interfaces?: string[];
  points: NodeMetricPoint[];
}

export interface InterfaceTrafficPoint {
  timestamp: number;
  raw_rx_bytes: number;
  raw_tx_bytes: number;
  accum_rx_bytes: number;
  accum_tx_bytes: number;
  traffic_rx_bytes: number;
  traffic_tx_bytes: number;
  rx_rate: number;
  tx_rate: number;
  reset_detected: boolean;
}

export interface NodeInterfaceTrafficSummary {
  node_name: string;
  interface_name: string;
  start_time: number;
  end_time: number;
  total_rx_bytes: number;
  total_tx_bytes: number;
  points: InterfaceTrafficPoint[];
}

export interface FleetMetricsSummary {
  total_nodes: number;
  online_nodes: number;
  offline_nodes: number;
  no_agent_nodes?: number;
  total_rx_rate: number;
  total_tx_rate: number;
  average_cpu: number;
  average_mem_perc: number;
}

export interface NodeLiveStatus {
  name: string;
  hostname: string;
  os_info: string;
  agent_version: string;
  uptime_seconds: number;
  connected: boolean;
  agent_installed?: boolean;
  last_seen: string;
  primary_interface?: string;
  metrics?: NodeMetricPoint;
}

export interface FleetLiveResponse {
  summary: FleetMetricsSummary;
  nodes: NodeLiveStatus[];
}

export interface CronTask {
  id: string;
  name: string;
  description?: string;
  task_type: "ping" | "tcp_ping";
  cron_expr: string;
  nodes: string[];
  params: Record<string, string>;
  timeout_sec?: number;
  enabled: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface CronTaskResultRecord {
  id: number;
  task_id: string;
  task_name: string;
  task_type: string;
  node_name: string;
  executed_at: number;
  duration_ms: number;
  exit_code: number;
  error?: string;
  target: string;
  packets_sent: number;
  packets_received: number;
  packet_loss_pct: number;
  min_rtt_ms: number;
  avg_rtt_ms: number;
  max_rtt_ms: number;
  mdev_rtt_ms: number;
  raw_output: string;
  parsed_json?: string;
}

export interface CronTaskStatsSummary {
  task_id: string;
  task_name: string;
  task_type: string;
  target: string;
  total_runs: number;
  latest_run_time: number;
  latest_avg_rtt_ms: number;
  latest_loss_pct: number;
  overall_avg_rtt_ms: number;
  overall_loss_pct: number;
  last_status: "ok" | "degraded" | "failed" | "unknown";
}

export interface NodeCronTasksResultsResponse {
  results: CronTaskResultRecord[];
  summary: CronTaskStatsSummary[];
  tasks: CronTask[];
}

export interface ServerTask {
  id: string;
  name: string;
  description: string;
  cron_expr: string;
  enabled: boolean;
  next_run?: string;
  last_run?: string;
  last_status: "never_run" | "running" | "success" | "error";
  last_duration_ms: number;
  last_message: string;
}

export interface DatabaseInfo {
  retention_days: number;
  file_path: string;
  total_size_bytes: number;
  total_size_human: string;
  page_size?: number;
  page_count?: number;
  freelist_count?: number;
  wal_size_bytes?: number;
  metrics_count?: number;
  interface_metrics_count?: number;
  cron_results_count?: number;
}


