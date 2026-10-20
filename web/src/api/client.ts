import type {
  AuthStatus,
  VersionInfo,
  Node,
  Link,
  ProbeResult,
  NodeStatus,
  SyncAction,
  SyncResult,
  SyncStatus,
  NetworkState,
  UpdateStateResponse,
  TaskMeta,
  TaskStatusResult,
  TaskRunResult,
  TaskSourceFile,
  OnlineSourcesResponse,
  OnlineManifest,
  TasksResponse,
  LookingGlassTask,
  LGRunRequest,
  LGRunResponse,
  GraphBlock,
  FleetLiveResponse,
  NodeMetricsResponse,
  NodeInterfaceTrafficSummary,
  CronTask,
  NodeCronTasksResultsResponse,
  ServerTask,
  DatabaseInfo,
  IntegrationsSettings,
  RevealLinkPskRequest,
  RevealLinkPskResponse,
  ResetLinkPskRequest,
  ResetLinkPskResponse,
  RevealLinkPrivateKeyRequest,
  RevealLinkPrivateKeyResponse,
  ResetLinkPrivateKeyRequest,
  ResetLinkPrivateKeyResponse,
  ConfigChange,
  ApplyPreviewResult,
  ApplyResult,
  ValidateApplyResponse,
} from "../types/api";

const API_BASE = "/api";

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options?.headers,
    },
    credentials: "include",
  });

  if (!response.ok) {
    let errorMsg = `HTTP Error ${response.status}`;
    try {
      const errJson = await response.json();
      if (errJson.error) {
        errorMsg = errJson.error;
      }
    } catch {
      // Fallback
    }
    const err = new Error(errorMsg) as Error & { status: number };
    err.status = response.status;
    throw err;
  }

  return response.json();
}

export const api = {
  // Version
  getVersion: () => request<VersionInfo>("/version"),

  // Auth
  getAuthStatus: () => request<AuthStatus>("/auth/status"),
  login: (password: string) =>
    request<AuthStatus>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ password }),
    }),
  unlock: (password: string) =>
    request<{ unlocked: boolean }>("/auth/unlock", {
      method: "POST",
      body: JSON.stringify({ password }),
    }),
  lock: () =>
    request<{ unlocked: boolean }>("/auth/lock", {
      method: "POST",
    }),
  logout: () =>
    request<{ logged_out: boolean }>("/auth/logout", {
      method: "POST",
    }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<{ success: boolean; message: string }>("/auth/change-password", {
      method: "POST",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),
  logoutAll: () =>
    request<{ logged_out: boolean; message: string }>("/auth/logout-all", {
      method: "POST",
    }),

  // Nodes
  getNodes: () => request<Node[]>("/nodes"),
  addNode: (node: Node) =>
    request<Node>("/nodes", {
      method: "POST",
      body: JSON.stringify(node),
    }),
  updateNode: (name: string, node: Node) =>
    request<Node>(`/nodes/${encodeURIComponent(name)}`, {
      method: "PUT",
      body: JSON.stringify(node),
    }),
  renameNode: (name: string, newName: string) =>
    request<Node>(`/nodes/${encodeURIComponent(name)}/rename`, {
      method: "POST",
      body: JSON.stringify({ new_name: newName }),
    }),
  updateNodePosition: (name: string, x: number, y: number) =>
    request<{ success: boolean; name: string; x: number; y: number }>(`/nodes/${encodeURIComponent(name)}/position`, {
      method: "PUT",
      body: JSON.stringify({ x, y }),
    }),
  deleteNode: (name: string) =>
    request<{ deleted: boolean }>(`/nodes/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  probeNode: (host: string, nodeName?: string) =>
    request<ProbeResult>("/nodes/probe", {
      method: "POST",
      body: JSON.stringify({ host, node_name: nodeName }),
    }),
  getNodeStatuses: () => request<Record<string, NodeStatus>>("/nodes/status"),
  refreshNodeStatus: (name: string) =>
    request<NodeStatus>(`/nodes/${encodeURIComponent(name)}/status`, {
      method: "POST",
    }),
  getNodeBirdConfig: (name: string) =>
    request<{ node: string; config: string }>(`/nodes/${encodeURIComponent(name)}/bird`),
  getNodeNftablesConfig: (name: string) =>
    request<{ node: string; config: string }>(`/nodes/${encodeURIComponent(name)}/nftables`),
  restartNodeWg: (name: string) =>
    request<{ success: boolean; message: string; output?: string }>(`/nodes/${encodeURIComponent(name)}/restart-wg`, {
      method: "POST",
    }),
  restartNodeBird: (name: string) =>
    request<{ success: boolean; message: string; output?: string }>(`/nodes/${encodeURIComponent(name)}/restart-bird`, {
      method: "POST",
    }),
  restartNodeInterface: (nodeName: string, iface: string) =>
    request<{ success: boolean; message: string; output?: string }>(
      `/nodes/${encodeURIComponent(nodeName)}/interfaces/${encodeURIComponent(iface)}/restart`,
      {
        method: "POST",
      },
    ),
  generateAgentToken: (name: string) =>
    request<{ node: string; token: string; server_url: string; install_command: string }>(
      `/nodes/${encodeURIComponent(name)}/agent-token`,
      {
        method: "POST",
      },
    ),
  getNodeAgent: (name: string) =>
    request<{
      node: string;
      mode: string;
      token: string;
      server_url: string;
      install_command: string;
      connected: boolean;
      last_seen?: string;
      agent_version?: string;
      telemetry?: any;
    }>(`/nodes/${encodeURIComponent(name)}/agent`),
  uninstallAgent: (name: string) =>
    request<{ success: boolean; message: string; node: string }>(
      `/nodes/${encodeURIComponent(name)}/agent/uninstall`,
      {
        method: "POST",
      },
    ),

  // Links
  getLinks: () => request<Link[]>("/links"),
  addLink: (data: {
    type?: string;
    from_node: string;
    to_node: string;
    from_port?: number;
    to_port?: number;
    from_mtu?: number;
    to_mtu?: number;
    from_use_ip?: boolean;
    to_use_ip?: boolean;
    from_policy?: string;
    to_policy?: string;
    from_routing_policy?: string;
    to_routing_policy?: string;
    from_cost?: number;
    to_cost?: number;
    from_fwmark?: string;
    to_fwmark?: string;
    from_preference?: number;
    to_preference?: number;
    from_mark?: string;
    to_mark?: string;
    from_note?: string;
    to_note?: string;
    mtu?: number;
    tags?: string[];
    assign_ipv4?: boolean;
    psk?: string;
    from?: Partial<Link["from"]>;
    to?: Partial<Link["to"]>;
  }) =>
    request<Link>("/links", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  createFullMesh: (nodes?: string[]) =>
    request<Link[]>("/links/mesh", {
      method: "POST",
      body: JSON.stringify({ nodes }),
    }),
  updateLink: (data: {
    type?: string;
    from_node: string;
    to_node: string;
    from_port?: number;
    to_port?: number;
    from_mtu?: number;
    to_mtu?: number;
    from_use_ip?: boolean;
    to_use_ip?: boolean;
    from_policy?: string;
    to_policy?: string;
    from_routing_policy?: string;
    to_routing_policy?: string;
    from_cost?: number;
    to_cost?: number;
    from_fwmark?: string;
    to_fwmark?: string;
    from_preference?: number;
    to_preference?: number;
    from_mark?: string;
    to_mark?: string;
    from_note?: string;
    to_note?: string;
    mtu?: number;
    tags?: string[];
    assign_ipv4?: boolean;
    psk?: string;
    from?: Partial<Link["from"]>;
    to?: Partial<Link["to"]>;
  }) =>
    request<Link>("/links", {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteLink: (fromNode: string, toNode: string, iface?: string) =>
    request<{ deleted: boolean }>(
      `/links?from=${encodeURIComponent(fromNode)}&to=${encodeURIComponent(toNode)}${iface ? `&interface=${encodeURIComponent(iface)}` : ""}`,
      {
        method: "DELETE",
      },
    ),
  revealLinkPsk: (data: RevealLinkPskRequest) =>
    request<RevealLinkPskResponse>("/links/psk/reveal", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  resetLinkPsk: (data: ResetLinkPskRequest) =>
    request<ResetLinkPskResponse>("/links/psk/reset", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  revealLinkPrivateKey: (data: RevealLinkPrivateKeyRequest) =>
    request<RevealLinkPrivateKeyResponse>("/links/private-key/reveal", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  resetLinkPrivateKey: (data: ResetLinkPrivateKeyRequest) =>
    request<ResetLinkPrivateKeyResponse>("/links/private-key/reset", {
      method: "POST",
      body: JSON.stringify(data),
    }),

  // Network Settings
  getNetworkSettings: () => request<import("../types/api").NetworkSettings>("/settings/network"),
  updateNetworkSettings: (settings: import("../types/api").NetworkSettings) =>
    request<import("../types/api").NetworkSettings>("/settings/network", {
      method: "PUT",
      body: JSON.stringify(settings),
    }),

  // DNS Settings
  getDNSConfig: () => request<import("../types/api").CloudflareDNSConfig>("/settings/dns"),
  updateDNSConfig: (config: import("../types/api").CloudflareDNSConfig) =>
    request<import("../types/api").CloudflareDNSConfig>("/settings/dns", {
      method: "PUT",
      body: JSON.stringify(config),
    }),
  syncDNS: (force?: boolean) =>
    request<import("../types/api").DNSSyncResult>("/settings/dns/sync", {
      method: "POST",
      body: JSON.stringify({ force: Boolean(force) }),
    }),

  // Server Background Tasks Settings
  getServerTasks: () => request<ServerTask[]>("/settings/tasks"),
  updateServerTask: (id: string, data: { cron_expr: string; enabled: boolean }) =>
    request<ServerTask>(`/settings/tasks/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  runServerTask: (id: string) =>
    request<ServerTask>(`/settings/tasks/${encodeURIComponent(id)}/run`, {
      method: "POST",
    }),

  // Database Settings & Maintenance
  getDatabaseInfo: () => request<DatabaseInfo>("/settings/database"),
  updateDatabaseSettings: (settings: { retention_days: number; raw_retention_days?: number }) =>
    request<DatabaseInfo>("/settings/database", {
      method: "PUT",
      body: JSON.stringify(settings),
    }),
  reclaimDatabase: () =>
    request<DatabaseInfo>("/settings/database/vacuum", {
      method: "POST",
    }),

  // Integrations & Prometheus / Telegram
  getIntegrationsSettings: () => request<IntegrationsSettings>("/settings/integrations"),
  updateIntegrationsSettings: (settings: {
    prometheus_enabled?: boolean;
    prometheus_auth_token?: string;
    telegram_enabled?: boolean;
    telegram_bot_token?: string;
    telegram_admin_id?: number | string;
    server_url?: string;
  }) =>
    request<IntegrationsSettings>("/settings/integrations", {
      method: "PUT",
      body: JSON.stringify(settings),
    }),
  resetPrometheusToken: () =>
    request<IntegrationsSettings>("/settings/integrations/reset-token", {
      method: "POST",
    }),
  testTelegramIntegration: (data: {
    telegram_bot_token?: string;
    telegram_admin_id?: number | string;
    server_url?: string;
  }) =>
    request<{ success: boolean; bot_username?: string; message?: string }>(
      "/settings/integrations/telegram/test",
      {
        method: "POST",
        body: JSON.stringify(data),
      }
    ),

  // Network Policies
  getNetworkPolicies: () => request<import("../types/api").NetworkPolicy[]>("/network-policies"),
  getNetworkPolicy: (id: string) =>
    request<import("../types/api").NetworkPolicy>(`/network-policies/${encodeURIComponent(id)}`),
  createNetworkPolicy: (data: Partial<import("../types/api").NetworkPolicy>) =>
    request<import("../types/api").NetworkPolicy>("/network-policies", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateNetworkPolicy: (id: string, data: Partial<import("../types/api").NetworkPolicy>) =>
    request<import("../types/api").NetworkPolicy>(`/network-policies/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteNetworkPolicy: (id: string) =>
    request<{ message: string }>(`/network-policies/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  refreshPolicyROA: (id: string) =>
    request<{ message: string }>(`/network-policies/${encodeURIComponent(id)}/refresh-roa`, {
      method: "POST",
    }),

  // Config Templates
  getTemplates: () => request<import("../types/api").ConfigTemplate[]>("/templates"),
  getTemplate: (id: string) => request<import("../types/api").ConfigTemplate>(`/templates/${encodeURIComponent(id)}`),
  createTemplate: (data: Partial<import("../types/api").ConfigTemplate>) =>
    request<import("../types/api").ConfigTemplate>("/templates", {
      method: "POST",
      body: JSON.stringify(data),
    }),
  updateTemplate: (id: string, data: Partial<import("../types/api").ConfigTemplate>) =>
    request<import("../types/api").ConfigTemplate>(`/templates/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    }),
  deleteTemplate: (id: string) =>
    request<{ message: string }>(`/templates/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  refreshAllROA: () =>
    request<{ message: string }>("/roa/refresh", {
      method: "POST",
    }),
  clearROACache: () =>
    request<{ message: string }>("/roa/clear-cache", {
      method: "POST",
    }),

  // Sync & State
  getSyncPreview: (nodeName?: string) => {
    const params = new URLSearchParams();
    if (nodeName) params.append("node", nodeName);
    const qs = params.toString();
    return request<SyncAction[]>(`/sync/preview${qs ? `?${qs}` : ""}`).then((res) => res || []);
  },
  executeSync: (force?: boolean, nodeName?: string) => {
    const params = new URLSearchParams();
    if (force) params.append("force", "true");
    if (nodeName) params.append("node", nodeName);
    const qs = params.toString();
    return request<SyncResult[]>(`/sync${qs ? `?${qs}` : ""}`, {
      method: "POST",
    }).then((res) => res || []);
  },
  getSyncStatus: () => request<SyncStatus>("/sync/status"),
  updateState: (nodeName?: string) =>
    request<UpdateStateResponse>(nodeName ? `/state/update?node=${encodeURIComponent(nodeName)}` : "/state/update", {
      method: "POST",
    }),
  getState: () => request<NetworkState>("/state"),

  // Helper Tasks
  getTasks: () => request<TaskMeta[]>("/tasks").then((res) => res || []),
  getTaskSources: (taskId: string) =>
    request<TaskSourceFile[]>(`/tasks/${encodeURIComponent(taskId)}/source`).then((res) => res || []),
  checkTaskStatus: (taskId: string, nodes?: string[]) =>
    request<Record<string, TaskStatusResult>>(`/tasks/${encodeURIComponent(taskId)}/status`, {
      method: "POST",
      body: JSON.stringify({ nodes }),
    }),
  runTask: (taskId: string, nodes: string[]) =>
    request<Record<string, TaskRunResult>>(`/tasks/${encodeURIComponent(taskId)}/run`, {
      method: "POST",
      body: JSON.stringify({ nodes }),
    }),
  getOnlineSources: () => request<OnlineSourcesResponse>("/tasks/online/sources"),
  getOnlineManifest: (source?: string) =>
    request<OnlineManifest>(`/tasks/online/manifest${source ? `?source=${encodeURIComponent(source)}` : ""}`),
  installOnlineTask: (source: string, taskId: string, files?: string[]) =>
    request<{ success: boolean; message: string; task_id: string }>("/tasks/online/install", {
      method: "POST",
      body: JSON.stringify({ source, task_id: taskId, files: files || [] }),
    }),
  deleteCustomTask: (taskId: string) =>
    request<{ success: boolean; message: string; task_id: string }>(`/tasks/custom/${encodeURIComponent(taskId)}`, {
      method: "DELETE",
    }),

  // Looking Glass
  getLookingGlassTasks: () => request<TasksResponse>("/looking-glass/tasks"),
  saveLookingGlassTask: (task: LookingGlassTask) =>
    request<LookingGlassTask>(
      task.id ? `/looking-glass/tasks/${encodeURIComponent(task.id)}` : "/looking-glass/tasks",
      {
        method: task.id ? "PUT" : "POST",
        body: JSON.stringify(task),
      },
    ),
  deleteLookingGlassTask: (taskId: string) =>
    request<{ deleted: boolean }>(`/looking-glass/tasks/${encodeURIComponent(taskId)}`, {
      method: "DELETE",
    }),
  runLookingGlass: (req: LGRunRequest) =>
    request<LGRunResponse>("/looking-glass/run", {
      method: "POST",
      body: JSON.stringify(req),
    }),

  // Blocks
  getBlocks: () => request<GraphBlock[]>("/blocks"),
  updateBlocks: (blocks: GraphBlock[]) =>
    request<GraphBlock[]>("/blocks", {
      method: "PUT",
      body: JSON.stringify(blocks),
    }),

  // Monitoring
  getFleetLiveStatus: (flush?: boolean) =>
    request<FleetLiveResponse>(flush ? "/nodes/live?flush=true" : "/nodes/live"),
  getNodeMetrics: (nodeName: string, range?: string, flush?: boolean, iface?: string) => {
    const params = new URLSearchParams();
    if (range) params.append("range", range);
    if (flush) params.append("flush", "true");
    if (iface) params.append("interface", iface);
    const qs = params.toString();
    return request<NodeMetricsResponse>(
      `/nodes/${encodeURIComponent(nodeName)}/metrics${qs ? `?${qs}` : ""}`,
    );
  },
  getNodeTraffic: (nodeName: string, iface?: string, range?: string, start?: number, end?: number) => {
    const params = new URLSearchParams();
    if (iface) params.append("interface", iface);
    if (range) params.append("range", range);
    if (start) params.append("start", start.toString());
    if (end) params.append("end", end.toString());
    const qs = params.toString();
    return request<NodeInterfaceTrafficSummary>(
      `/nodes/${encodeURIComponent(nodeName)}/traffic${qs ? `?${qs}` : ""}`,
    );
  },

  // Cron Tasks
  getCronTasks: () => request<CronTask[]>("/cron-tasks"),
  createCronTask: (task: Partial<CronTask>) =>
    request<CronTask>("/cron-tasks", {
      method: "POST",
      body: JSON.stringify(task),
    }),
  updateCronTask: (id: string, task: Partial<CronTask>) =>
    request<CronTask>(`/cron-tasks/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: JSON.stringify(task),
    }),
  deleteCronTask: (id: string) =>
    request<{ deleted: boolean }>(`/cron-tasks/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  toggleCronTask: (id: string) =>
    request<CronTask>(`/cron-tasks/${encodeURIComponent(id)}/toggle`, {
      method: "POST",
    }),
  getNodeCronTasksResults: (nodeName: string, taskId?: string, range?: string, limit?: number) => {
    const params = new URLSearchParams();
    if (taskId) params.append("task_id", taskId);
    if (range) params.append("range", range);
    if (limit) params.append("limit", limit.toString());
    const qs = params.toString();
    return request<NodeCronTasksResultsResponse>(
      `/nodes/${encodeURIComponent(nodeName)}/cron-tasks/results${qs ? `?${qs}` : ""}`,
    );
  },

  // Config Apply & Changes
  getConfigApplySchema: () => request<any>("/config/apply/schema"),
  validateConfigApply: (content: string) =>
    request<ValidateApplyResponse>("/config/apply/validate", {
      method: "POST",
      body: JSON.stringify({ content }),
    }),
  previewConfigApply: (content: string) =>
    request<ApplyPreviewResult>("/config/apply/preview", {
      method: "POST",
      body: JSON.stringify({ content }),
    }),
  executeConfigApply: (content: string, autoSync: boolean = false) =>
    request<ApplyResult>("/config/apply", {
      method: "POST",
      body: JSON.stringify({ content, auto_sync: autoSync }),
    }),
  listConfigChanges: () => request<ConfigChange[]>("/config/changes"),
  getConfigChange: (id: string) => request<ConfigChange>(`/config/changes/${encodeURIComponent(id)}`),
  saveConfigChange: (change: Partial<ConfigChange>) =>
    request<ConfigChange>(change.id ? `/config/changes/${encodeURIComponent(change.id)}` : "/config/changes", {
      method: change.id ? "PUT" : "POST",
      body: JSON.stringify(change),
    }),
  deleteConfigChange: (id: string) =>
    request<{ status: string }>(`/config/changes/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
  applySavedConfigChange: (id: string, autoSync: boolean = false) =>
    request<ApplyResult>(`/config/changes/${encodeURIComponent(id)}/apply`, {
      method: "POST",
      body: JSON.stringify({ auto_sync: autoSync }),
    }),
};

