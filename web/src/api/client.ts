import { AuthStatus, Node, Link, ProbeResult, NodeStatus, SyncAction, SyncResult, SyncStatus } from '../types/api';

const API_BASE = '/api';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
    credentials: 'include',
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
  // Auth
  getAuthStatus: () => request<AuthStatus>('/auth/status'),
  login: (password: string) => request<AuthStatus>('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  }),
  unlock: (password: string) => request<{ unlocked: boolean }>('/auth/unlock', {
    method: 'POST',
    body: JSON.stringify({ password }),
  }),
  lock: () => request<{ unlocked: boolean }>('/auth/lock', {
    method: 'POST',
  }),
  logout: () => request<{ logged_out: boolean }>('/auth/logout', {
    method: 'POST',
  }),
  changePassword: (currentPassword: string, newPassword: string) =>
    request<{ success: boolean; message: string }>('/auth/change-password', {
      method: 'POST',
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),
  logoutAll: () =>
    request<{ logged_out: boolean; message: string }>('/auth/logout-all', {
      method: 'POST',
    }),

  // Nodes
  getNodes: () => request<Node[]>('/nodes'),
  addNode: (node: Node) => request<Node>('/nodes', {
    method: 'POST',
    body: JSON.stringify(node),
  }),
  updateNode: (name: string, node: Node) => request<Node>(`/nodes/${encodeURIComponent(name)}`, {
    method: 'PUT',
    body: JSON.stringify(node),
  }),
  updateNodePosition: (name: string, x: number, y: number) =>
    request<{ success: boolean; name: string; x: number; y: number }>(`/nodes/${encodeURIComponent(name)}/position`, {
      method: 'PUT',
      body: JSON.stringify({ x, y }),
    }),
  deleteNode: (name: string) => request<{ deleted: boolean }>(`/nodes/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  }),
  probeNode: (host: string) => request<ProbeResult>('/nodes/probe', {
    method: 'POST',
    body: JSON.stringify({ host }),
  }),
  getNodeStatuses: () => request<Record<string, NodeStatus>>('/nodes/status'),
  refreshNodeStatus: (name: string) => request<NodeStatus>(`/nodes/${encodeURIComponent(name)}/status`, {
    method: 'POST',
  }),

  // Links
  getLinks: () => request<Link[]>('/links'),
  addLink: (data: {
    from_node: string;
    to_node: string;
    from_port?: number;
    to_port?: number;
    from_mtu?: number;
    to_mtu?: number;
    mtu?: number;
    tags?: string[];
  }) =>
    request<Link>('/links', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateLink: (data: {
    from_node: string;
    to_node: string;
    from_port?: number;
    to_port?: number;
    from_mtu?: number;
    to_mtu?: number;
    mtu?: number;
    tags?: string[];
  }) =>
    request<Link>('/links', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteLink: (fromNode: string, toNode: string) =>
    request<{ deleted: boolean }>(`/links?from=${encodeURIComponent(fromNode)}&to=${encodeURIComponent(toNode)}`, {
      method: 'DELETE',
    }),

  // Sync
  getSyncPreview: () => request<SyncAction[]>('/sync/preview').then((res) => res || []),
  executeSync: () => request<SyncResult[]>('/sync', {
    method: 'POST',
  }).then((res) => res || []),
  getSyncStatus: () => request<SyncStatus>('/sync/status'),
};
