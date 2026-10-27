import React, { useState, useEffect } from "react";
import {
  Box,
  Typography,
  Button,
  IconButton,
  Paper,
  TextField,
  InputAdornment,
  Chip,
  Switch,
  Tabs,
  Tab,
  Alert,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Tooltip,
} from "@mui/material";
import {
  Bot,
  Sparkles,
  Copy,
  Check,
  Eye,
  EyeOff,
  RefreshCw,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Terminal,
  ExternalLink,
  ChevronDown,
  CheckCircle2,
  AlertCircle,
  Network,
  Sliders,
  Code,
  Globe,
  Radio,
  FileCode,
  KeyRound,
} from "lucide-react";
import { api } from "../../api/client";
import type { MCPSettingsResponse, ResetMCPTokenRequest } from "../../types/api";

type ClientType = "claude" | "cursor" | "cline" | "windsurf" | "gemini" | "stdio" | "curl";
type TokenSelect = "full" | "ro";

interface ToolInfo {
  name: string;
  category: string;
  desc: string;
  isWrite: boolean;
  args?: string;
}

const MCP_TOOLS: ToolInfo[] = [
  // Cluster
  {
    name: "get_cluster_info",
    category: "Cluster",
    desc: "Get high-level mesh network summary, ASN, node count, link count",
    isWrite: false,
  },
  {
    name: "get_cluster_state",
    category: "Cluster",
    desc: "Get runtime cluster state, active sessions, and WireGuard interfaces",
    isWrite: false,
  },
  {
    name: "get_fleet_live_status",
    category: "Cluster",
    desc: "Live ping reachability and agent metrics across all nodes",
    isWrite: false,
  },

  // Nodes
  {
    name: "list_nodes",
    category: "Nodes",
    desc: "List all nodes with IP allocations, tags, listen ports, and status",
    isWrite: false,
    args: "tag?: string",
  },
  {
    name: "get_node",
    category: "Nodes",
    desc: "Get node details, WireGuard config, BIRD BGP routing config, nftables rules",
    isWrite: false,
    args: "name: string, include_bird?: bool, include_nftables?: bool",
  },
  {
    name: "add_node",
    category: "Nodes",
    desc: "Add a new node to the cluster control plane",
    isWrite: true,
    args: "name: string, host?: string, ip?: string, ip6?: string, asn?: int, tags?: string[]",
  },
  {
    name: "update_node",
    category: "Nodes",
    desc: "Update existing node properties, tags, and routing options",
    isWrite: true,
    args: "name: string, host?: string, ip?: string, tags?: string[]",
  },
  {
    name: "rename_node",
    category: "Nodes",
    desc: "Rename node safely across all links, policies, and tasks",
    isWrite: true,
    args: "old_name: string, new_name: string",
  },
  {
    name: "delete_node",
    category: "Nodes",
    desc: "Remove node and tear down its peer link connections",
    isWrite: true,
    args: "name: string",
  },
  {
    name: "probe_node",
    category: "Nodes",
    desc: "Probe host latency and reachability before adding",
    isWrite: false,
    args: "host: string, node_name?: string",
  },
  {
    name: "refresh_node_status",
    category: "Nodes",
    desc: "Trigger active ping / reachability probe for a node",
    isWrite: true,
    args: "name: string",
  },
  {
    name: "restart_node_service",
    category: "Nodes",
    desc: "Restart WireGuard, BIRD BGP, or interface on node",
    isWrite: true,
    args: "name: string, service: 'wireguard'|'bird'|'interface'",
  },
  {
    name: "get_node_telemetry",
    category: "Nodes",
    desc: "Get recent CPU, memory, load, and interface traffic",
    isWrite: false,
    args: "name: string",
  },

  // Links
  {
    name: "list_links",
    category: "Links",
    desc: "List all WireGuard peer links with interfaces, addresses, and costs",
    isWrite: false,
    args: "node?: string",
  },
  {
    name: "get_link",
    category: "Links",
    desc: "Get connection details for link between two specific nodes",
    isWrite: false,
    args: "from: string, to: string",
  },
  {
    name: "add_link",
    category: "Links",
    desc: "Create WireGuard point-to-point link with key generation",
    isWrite: true,
    args: "from: string, to: string, from_port?: int, to_port?: int, cost?: int, mtu?: int",
  },
  {
    name: "create_mesh_links",
    category: "Links",
    desc: "Generate full mesh links among specified nodes",
    isWrite: true,
    args: "nodes?: string[]",
  },
  {
    name: "update_link",
    category: "Links",
    desc: "Update MTU, routing cost, or policy for an existing link",
    isWrite: true,
    args: "from: string, to: string, cost?: int, mtu?: int, policy?: string",
  },
  {
    name: "delete_link",
    category: "Links",
    desc: "Remove link between two nodes",
    isWrite: true,
    args: "from: string, to: string, interface?: string",
  },
  {
    name: "reset_link_psk",
    category: "Links",
    desc: "Rotate or set WireGuard pre-shared key (PSK)",
    isWrite: true,
    args: "from: string, to: string, psk?: string",
  },
  {
    name: "reset_link_private_key",
    category: "Links",
    desc: "Regenerate WireGuard private/public keypair on link endpoint",
    isWrite: true,
    args: "from: string, to: string, node: string",
  },

  // Network & DNS
  {
    name: "get_network_settings",
    category: "Network",
    desc: "Get network name, public ASN, permitted IP prefixes, and FlapAlerted config",
    isWrite: false,
  },
  {
    name: "update_network_settings",
    category: "Network",
    desc: "Update global cluster network parameters",
    isWrite: true,
    args: "name?: string, public_asn?: int, prefixes?: string[]",
  },
  { name: "get_ip_blocks", category: "Network", desc: "Get visual topology groupings on graph", isWrite: false },
  {
    name: "update_ip_blocks",
    category: "Network",
    desc: "Update visual topology grouping blocks",
    isWrite: true,
    args: "blocks_json: string",
  },
  { name: "get_dns_config", category: "DNS", desc: "Get Cloudflare DNS integration configuration", isWrite: false },
  {
    name: "update_dns_config",
    category: "DNS",
    desc: "Update Cloudflare DNS zone, base domain, or tokens",
    isWrite: true,
    args: "zone_id?: string, api_token?: string, base_domain?: string",
  },
  {
    name: "sync_dns",
    category: "DNS",
    desc: "Synchronize all node IP records to Cloudflare DNS",
    isWrite: true,
    args: "force?: bool",
  },

  // Policies & Templates
  {
    name: "list_network_policies",
    category: "Policies",
    desc: "List BGP routing, firewall, and ROA validation policies",
    isWrite: false,
  },
  {
    name: "get_network_policy",
    category: "Policies",
    desc: "Get policy details by ID",
    isWrite: false,
    args: "id: string",
  },
  {
    name: "create_network_policy",
    category: "Policies",
    desc: "Create a network policy for routing, firewall rules, and ROA",
    isWrite: true,
    args: "id: string, name: string, cost?: int, policy_json?: string",
  },
  {
    name: "update_network_policy",
    category: "Policies",
    desc: "Update an existing network policy",
    isWrite: true,
    args: "id: string, name?: string, policy_json?: string",
  },
  {
    name: "delete_network_policy",
    category: "Policies",
    desc: "Delete a custom network policy",
    isWrite: true,
    args: "id: string",
  },
  {
    name: "refresh_policy_roa",
    category: "Policies",
    desc: "Refresh Route Origin Authorization (ROA) tables",
    isWrite: true,
    args: "policy_id?: string",
  },
  {
    name: "list_templates",
    category: "Templates",
    desc: "List configuration templates for WireGuard, BIRD, nftables",
    isWrite: false,
  },
  {
    name: "get_template",
    category: "Templates",
    desc: "Get template source code by ID",
    isWrite: false,
    args: "id: string",
  },
  {
    name: "create_template",
    category: "Templates",
    desc: "Create a custom configuration template",
    isWrite: true,
    args: "id: string, name: string, type: 'bird'|'wg'|'nft', content: string",
  },
  {
    name: "update_template",
    category: "Templates",
    desc: "Update custom template source code",
    isWrite: true,
    args: "id: string, content?: string",
  },
  { name: "delete_template", category: "Templates", desc: "Delete custom template", isWrite: true, args: "id: string" },

  // Config Apply & Changes
  {
    name: "preview_config_apply",
    category: "Config Apply",
    desc: "Dry-run declarative configuration changes (YAML/JSON) computing diff",
    isWrite: false,
    args: "content: string",
  },
  {
    name: "execute_config_apply",
    category: "Config Apply",
    desc: "Atomically apply declarative configuration patch to cluster",
    isWrite: true,
    args: "content: string, auto_sync?: bool",
  },
  {
    name: "list_config_changes",
    category: "Config Apply",
    desc: "List saved configuration change patches",
    isWrite: false,
  },
  {
    name: "save_config_change",
    category: "Config Apply",
    desc: "Save or update a named configuration change snippet",
    isWrite: true,
    args: "name: string, content: string",
  },
  {
    name: "apply_saved_config_change",
    category: "Config Apply",
    desc: "Execute a saved configuration change snippet by ID",
    isWrite: true,
    args: "id: string, auto_sync?: bool",
  },
  {
    name: "delete_config_change",
    category: "Config Apply",
    desc: "Delete a saved configuration snippet",
    isWrite: true,
    args: "id: string",
  },

  // Sync & Tasks
  {
    name: "preview_sync",
    category: "Sync",
    desc: "Preview planned WireGuard, BIRD, and firewall push diff across nodes",
    isWrite: false,
    args: "nodes?: string[], scope?: 'all'|'wireguard'|'bird'",
  },
  {
    name: "execute_sync",
    category: "Sync",
    desc: "Deploy WireGuard, BIRD, nftables, and hosts configs to remote nodes",
    isWrite: true,
    args: "nodes?: string[], scope?: string, force?: bool",
  },
  {
    name: "get_sync_status",
    category: "Sync",
    desc: "Get synchronization status and check for out-of-sync nodes",
    isWrite: false,
  },
  { name: "list_tasks", category: "Tasks", desc: "List device helper automation scripts", isWrite: false },
  {
    name: "run_task",
    category: "Tasks",
    desc: "Execute a device helper task on target nodes",
    isWrite: true,
    args: "id: string, nodes?: string[]",
  },
  {
    name: "check_task_status",
    category: "Tasks",
    desc: "Query task readiness and results on nodes",
    isWrite: false,
    args: "id: string, nodes?: string[]",
  },

  // Diagnostics & Cron
  {
    name: "list_looking_glass_tasks",
    category: "Looking Glass",
    desc: "List diagnostic commands (ping, traceroute, BIRD show route, mtr)",
    isWrite: false,
  },
  {
    name: "run_looking_glass",
    category: "Looking Glass",
    desc: "Execute Looking Glass diagnosis from nodes to target",
    isWrite: true,
    args: "nodes: string[], task_id?: string, target?: string",
  },
  { name: "list_cron_tasks", category: "Cron", desc: "List scheduled automated tasks across nodes", isWrite: false },
  {
    name: "create_cron_task",
    category: "Cron",
    desc: "Schedule recurring latency or ping task on nodes",
    isWrite: true,
    args: "id: string, name: string, task_type: 'ping'|'tcp_ping', cron_expr: string, nodes: string[], target: string",
  },
  {
    name: "toggle_cron_task",
    category: "Cron",
    desc: "Enable or disable a scheduled cron task",
    isWrite: true,
    args: "id: string",
  },
  {
    name: "get_node_cron_task_results",
    category: "Cron",
    desc: "Get latency metrics and history records for a node",
    isWrite: false,
    args: "node: string, task_id?: string, range?: string",
  },

  // Maintenance & Integrations
  {
    name: "get_database_info",
    category: "Database",
    desc: "Get SQLite database size and retention configuration",
    isWrite: false,
  },
  {
    name: "reclaim_database",
    category: "Database",
    desc: "Run VACUUM to reclaim disk space and optimize DB",
    isWrite: true,
  },
  {
    name: "get_integrations_settings",
    category: "Integrations",
    desc: "Get Prometheus scraping and Telegram Bot settings",
    isWrite: false,
  },
  {
    name: "reset_prometheus_token",
    category: "Integrations",
    desc: "Rotate Prometheus scraping Bearer token",
    isWrite: true,
  },
];

export const MCPSettingsTab: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [data, setData] = useState<MCPSettingsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);

  // Visibility toggles for credentials
  const [showFullToken, setShowFullToken] = useState(false);
  const [showRoToken, setShowRoToken] = useState(false);
  const [showOAuthSecret, setShowOAuthSecret] = useState(false);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Client guide selection
  const [selectedClient, setSelectedClient] = useState<ClientType>("claude");
  const [selectedToken, setSelectedToken] = useState<TokenSelect>("full");

  // Tools catalog filter
  const [toolSearch, setToolSearch] = useState("");
  const [toolCategory, setToolCategory] = useState("All");
  const [showToolsCatalog, setShowToolsCatalog] = useState(false);

  // Reset token confirmation dialog
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [tokenToReset, setTokenToReset] = useState<ResetMCPTokenRequest["token_type"]>("full_control");
  const [resetting, setResetting] = useState(false);

  const loadData = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await api.getMCPSettings();
      setData(res);
    } catch (err: any) {
      setError(err.message || "Failed to load MCP server settings.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleToggleEnabled = async (checked: boolean) => {
    if (!data) return;
    try {
      setSaving(true);
      setError(null);
      setSuccess(null);
      const updated = await api.updateMCPSettings({ enabled: checked });
      setData(updated);
      setSuccess(checked ? "MCP server enabled successfully!" : "MCP server disabled.");
      setTimeout(() => setSuccess(null), 4000);
    } catch (err: any) {
      setError(err.message || "Failed to update MCP server status.");
    } finally {
      setSaving(false);
    }
  };

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => {
      setCopiedKey(null);
    }, 2500);
  };

  const handleOpenResetDialog = (type: ResetMCPTokenRequest["token_type"]) => {
    setTokenToReset(type);
    setResetDialogOpen(true);
  };

  const handleConfirmReset = async () => {
    try {
      setResetting(true);
      setError(null);
      const updated = await api.resetMCPToken(tokenToReset);
      setData(updated);
      setResetDialogOpen(false);
      setSuccess(
        tokenToReset === "full_control"
          ? "Full-Control token rotated successfully!"
          : tokenToReset === "read_only"
            ? "Read-Only token rotated successfully!"
            : "Both MCP tokens rotated successfully!",
      );
      setTimeout(() => setSuccess(null), 4000);
    } catch (err: any) {
      setError(err.message || "Failed to reset token.");
    } finally {
      setResetting(false);
    }
  };

  const activeTokenValue =
    selectedToken === "full"
      ? data?.full_control_token || "YOUR_FULL_CONTROL_TOKEN"
      : data?.read_only_token || "YOUR_READ_ONLY_TOKEN";

  const getSnippets = () => {
    const url = data?.server_url || "http://127.0.0.1:4242/mcp";
    const sseUrl = data?.sse_url || "http://127.0.0.1:4242/mcp/sse";
    const token = activeTokenValue;

    switch (selectedClient) {
      case "claude":
        return {
          filename: "claude_desktop_config.json",
          pathNote:
            "macOS: ~/Library/Application Support/Claude/claude_desktop_config.json\nWindows: %APPDATA%\\Claude\\claude_desktop_config.json\nLinux: ~/.config/Claude/claude_desktop_config.json",
          code: JSON.stringify(
            {
              mcpServers: {
                easy42: {
                  url: `${url}?token=${token}`,
                  headers: {
                    Authorization: `Bearer ${token}`,
                  },
                },
              },
            },
            null,
            2,
          ),
        };
      case "cursor":
        return {
          filename: "Cursor MCP Settings (cursor_mcp.json or Settings > Features > MCP)",
          pathNote: "Cursor: Settings > Features > MCP > Add New MCP Server\nType: SSE / Streamable HTTP\nURL: " + url,
          code: JSON.stringify(
            {
              mcpServers: {
                easy42: {
                  url: `${url}?token=${token}`,
                  type: "sse",
                  headers: {
                    Authorization: `Bearer ${token}`,
                  },
                },
              },
            },
            null,
            2,
          ),
        };
      case "cline":
        return {
          filename: "cline_mcp_settings.json",
          pathNote: "VS Code: Extensions > Cline / Roo Code > MCP Server Settings",
          code: JSON.stringify(
            {
              mcpServers: {
                easy42: {
                  url: `${sseUrl}?token=${token}`,
                  headers: {
                    Authorization: `Bearer ${token}`,
                  },
                },
              },
            },
            null,
            2,
          ),
        };
      case "gemini":
        return {
          filename: "Google Gemini Web App (Settings > Personal Intelligence > Connected apps > + Custom)",
          pathNote:
            "1. Open gemini.google.com > Settings > Personal Intelligence > Connected apps > + Custom, open Connect to an MCP server\n2. Fill in the MCP Server URL, OAuth Client ID, and OAuth Client Secret shown below\n3. Click Connect to authorize Easy42 MCP with Gemini",
          code: JSON.stringify(
            {
              mcp_server_url: url,
              oauth_client_id: data?.oauth_client_id || "easy42-mcp",
              oauth_client_secret: data?.oauth_client_secret || "",
              authorization_endpoint: data?.authorize_url || `${url.replace(/\/mcp$/, "")}/oauth/authorize`,
              token_endpoint: data?.token_url || `${url.replace(/\/mcp$/, "")}/oauth/token`,
            },
            null,
            2,
          ),
        };
      case "windsurf":
        return {
          filename: "mcp_config.json",
          pathNote: "Windsurf: ~/.codeium/windsurf/mcp_config.json",
          code: JSON.stringify(
            {
              mcpServers: {
                easy42: {
                  serverUrl: `${url}?token=${token}`,
                  headers: {
                    Authorization: `Bearer ${token}`,
                  },
                },
              },
            },
            null,
            2,
          ),
        };
      case "stdio":
        return {
          filename: "Local Stdio Command (claude_desktop_config.json / Cursor)",
          pathNote: "Runs directly on the host using the local easy42 binary without HTTP transport",
          code: JSON.stringify(
            {
              mcpServers: {
                easy42: {
                  command: "easy42",
                  args: ["mcp", "--role", selectedToken === "full" ? "full_control" : "read_only"],
                },
              },
            },
            null,
            2,
          ),
        };
      case "curl":
        return {
          filename: "cURL Test Command",
          pathNote: "Send an MCP JSON-RPC ping / initialize request via cURL",
          code: `curl -i -X POST "${url}" \\\n  -H "Authorization: Bearer ${token}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test-client","version":"1.0"}}}'`,
        };
    }
  };

  const currentSnippet = getSnippets();

  const categories = ["All", ...Array.from(new Set(MCP_TOOLS.map((t) => t.category)))];
  const filteredTools = MCP_TOOLS.filter((t) => {
    const matchesCat = toolCategory === "All" || t.category === toolCategory;
    const matchesSearch =
      toolSearch === "" ||
      t.name.toLowerCase().includes(toolSearch.toLowerCase()) ||
      t.desc.toLowerCase().includes(toolSearch.toLowerCase());
    return matchesCat && matchesSearch;
  });

  return (
    <>
      <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
        {/* HEADER DESCRIPTION */}
        <Box
          sx={{
            p: 2.5,
            borderRadius: 2.5,
            bgcolor: "#F8FAFC",
            border: "1px solid #E2E8F0",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 2,
            flexWrap: { xs: "wrap", md: "nowrap" },
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
            <Box
              sx={{
                width: 46,
                height: 46,
                borderRadius: 2,
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
                border: "1px solid #E0E7FF",
              }}
            >
              <Bot size={26} />
            </Box>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "1.05rem" }}>
                  Model Context Protocol (MCP) Server
                </Typography>
                <Chip
                  size="small"
                  icon={<Sparkles size={12} color="#D97706" />}
                  label="AI Agent Control Plane"
                  sx={{
                    bgcolor: "#FEF3C7",
                    color: "#92400E",
                    fontWeight: 700,
                    fontSize: "0.72rem",
                    border: "1px solid #FDE68A",
                    height: 22,
                  }}
                />
              </Box>
              <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.25 }}>
                Allow AI agents (Claude, Cursor, Cline, Windsurf, Roo Code) to query, configure, and operate the entire
                Easy42 mesh control plane.
              </Typography>
            </Box>
          </Box>

          <Box sx={{ display: "flex", alignItems: "center", gap: 2, flexShrink: 0 }}>
            <Box sx={{ textAlign: "right" }}>
              <Typography
                variant="caption"
                sx={{ color: "#64748B", display: "block", fontWeight: 600, fontSize: "0.75rem" }}
              >
                MCP Service
              </Typography>
              <Chip
                size="small"
                icon={<Radio size={12} color={data?.enabled ? "#16A34A" : "#64748B"} />}
                label={data?.enabled ? "Active" : "Disabled"}
                sx={{
                  bgcolor: data?.enabled ? "#DCFCE7" : "#F1F5F9",
                  color: data?.enabled ? "#15803D" : "#64748B",
                  fontWeight: 700,
                  fontSize: "0.75rem",
                  border: `1px solid ${data?.enabled ? "#BBF7D0" : "#CBD5E1"}`,
                  height: 22,
                }}
              />
            </Box>
            <Switch
              checked={data?.enabled ?? true}
              onChange={(e) => handleToggleEnabled(e.target.checked)}
              disabled={loading || saving}
              color="primary"
            />
            <Button
              size="small"
              variant="outlined"
              startIcon={<RefreshCw size={14} className={loading ? "spin" : ""} />}
              onClick={loadData}
              disabled={loading || saving}
              sx={{
                textTransform: "none",
                fontWeight: 600,
                borderColor: "#CBD5E1",
                color: "#334155",
                borderRadius: 2,
                "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
              }}
            >
              Refresh
            </Button>
          </Box>
        </Box>

        {success && (
          <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
            {success}
          </Alert>
        )}

        {error && (
          <Alert icon={<AlertCircle size={18} />} severity="error" sx={{ borderRadius: 2 }}>
            {error}
          </Alert>
        )}

        {loading && !data ? (
          <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", py: 8 }}>
            <CircularProgress size={36} />
          </Box>
        ) : (
          <>
            {/* CONNECTION ENDPOINTS */}
            <Paper
              elevation={0}
              sx={{
                p: 3,
                borderRadius: 2.5,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                boxShadow: "0 1px 3px rgba(0, 0, 0, 0.05)",
              }}
            >
              <Typography
                variant="subtitle1"
                sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}
              >
                <Globe size={18} color="#4F46E5" /> Connection Endpoints
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.5, mb: 2 }}>
                Configure these endpoints in your AI client. Streamable HTTP is the modern MCP specification standard.
              </Typography>

              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
                {/* Streamable HTTP URL */}
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                  }}
                >
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                    <Typography
                      variant="caption"
                      sx={{ fontWeight: 700, color: "#334155", textTransform: "uppercase", letterSpacing: "0.04em" }}
                    >
                      Streamable HTTP URL (Standard)
                    </Typography>
                    <Chip
                      size="small"
                      label="Recommended"
                      color="primary"
                      sx={{ height: 20, fontSize: "0.68rem", fontWeight: 700 }}
                    />
                  </Box>
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <TextField
                      fullWidth
                      size="small"
                      value={data?.server_url || ""}
                      slotProps={{ input: { readOnly: true } }}
                      sx={{
                        "& .MuiInputBase-input": {
                          fontFamily: "monospace",
                          fontSize: "0.82rem",
                          bgcolor: "#FFFFFF",
                        },
                      }}
                    />
                    <Tooltip title={copiedKey === "url" ? "Copied!" : "Copy URL"}>
                      <Button
                        variant="outlined"
                        onClick={() => handleCopy(data?.server_url || "", "url")}
                        sx={{ minWidth: 42, px: 1, borderColor: "#CBD5E1" }}
                      >
                        {copiedKey === "url" ? <Check size={16} color="#16A34A" /> : <Copy size={16} />}
                      </Button>
                    </Tooltip>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.8 }}>
                    Used by Claude Desktop, Cursor, and Windsurf via HTTP POST/SSE.
                  </Typography>
                </Box>

                {/* Classic SSE Endpoint */}
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                  }}
                >
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                    <Typography
                      variant="caption"
                      sx={{ fontWeight: 700, color: "#334155", textTransform: "uppercase", letterSpacing: "0.04em" }}
                    >
                      Classic SSE Endpoint
                    </Typography>
                    <Chip size="small" label="SSE Stream" sx={{ height: 20, fontSize: "0.68rem", fontWeight: 700 }} />
                  </Box>
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <TextField
                      fullWidth
                      size="small"
                      value={data?.sse_url || ""}
                      slotProps={{ input: { readOnly: true } }}
                      sx={{
                        "& .MuiInputBase-input": {
                          fontFamily: "monospace",
                          fontSize: "0.82rem",
                          bgcolor: "#FFFFFF",
                        },
                      }}
                    />
                    <Tooltip title={copiedKey === "sse" ? "Copied!" : "Copy SSE URL"}>
                      <Button
                        variant="outlined"
                        onClick={() => handleCopy(data?.sse_url || "", "sse")}
                        sx={{ minWidth: 42, px: 1, borderColor: "#CBD5E1" }}
                      >
                        {copiedKey === "sse" ? <Check size={16} color="#16A34A" /> : <Copy size={16} />}
                      </Button>
                    </Tooltip>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.8 }}>
                    Message POST endpoint: <code style={{ color: "#4F46E5" }}>{data?.endpoint}/message</code>
                  </Typography>
                </Box>
              </Box>
            </Paper>

            {/* CREDENTIALS / TOKENS */}
            <Paper
              elevation={0}
              sx={{
                p: 3,
                borderRadius: 2.5,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                boxShadow: "0 1px 3px rgba(0, 0, 0, 0.05)",
              }}
            >
              <Typography
                variant="subtitle1"
                sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}
              >
                <Shield size={18} color="#4F46E5" /> Access Tokens & Credentials
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.5, mb: 2 }}>
                Easy42 enforces role-based security with two distinct token tiers. Pass via Bearer Authorization header
                or <code>?token=</code> query parameter.
              </Typography>

              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2.5 }}>
                {/* 1. Full-Control Token */}
                <Box
                  sx={{
                    p: 2.5,
                    borderRadius: 2,
                    bgcolor: "rgba(239, 68, 68, 0.03)",
                    border: "1px solid #FECACA",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.5,
                  }}
                >
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Box
                        sx={{
                          width: 32,
                          height: 32,
                          borderRadius: 1.5,
                          bgcolor: "#FEE2E2",
                          color: "#DC2626",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                        }}
                      >
                        <ShieldAlert size={18} />
                      </Box>
                      <Box>
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#991B1B" }}>
                          Full-Control Token
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#B91C1C", fontWeight: 600 }}>
                          Read & Write (Full Admin Control)
                        </Typography>
                      </Box>
                    </Box>
                    <Chip
                      size="small"
                      label="Admin"
                      color="error"
                      sx={{ height: 20, fontSize: "0.68rem", fontWeight: 700 }}
                    />
                  </Box>

                  <Typography variant="body2" sx={{ color: "#7F1D1D", fontSize: "0.82rem", lineHeight: 1.4 }}>
                    Allows the AI agent to mutate cluster state: add/remove nodes, create wireguard links, restart
                    BIRD/WireGuard, apply declarative config, and run device tasks.
                  </Typography>

                  <TextField
                    fullWidth
                    size="small"
                    type={showFullToken ? "text" : "password"}
                    value={data?.full_control_token || ""}
                    slotProps={{
                      input: {
                        readOnly: true,
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton size="small" onClick={() => setShowFullToken(!showFullToken)}>
                              {showFullToken ? <EyeOff size={16} /> : <Eye size={16} />}
                            </IconButton>
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={{
                      "& .MuiInputBase-input": {
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        bgcolor: "#FFFFFF",
                      },
                    }}
                  />

                  <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
                    <Button
                      fullWidth
                      variant="contained"
                      color="error"
                      size="small"
                      startIcon={copiedKey === "full_tok" ? <Check size={14} /> : <Copy size={14} />}
                      onClick={() => handleCopy(data?.full_control_token || "", "full_tok")}
                      sx={{ textTransform: "none", fontWeight: 600, borderRadius: 1.5 }}
                    >
                      {copiedKey === "full_tok" ? "Copied!" : "Copy Full-Control Token"}
                    </Button>
                    <Button
                      variant="outlined"
                      color="inherit"
                      size="small"
                      onClick={() => handleOpenResetDialog("full_control")}
                      sx={{
                        textTransform: "none",
                        fontWeight: 600,
                        borderRadius: 1.5,
                        borderColor: "#FCA5A5",
                        color: "#B91C1C",
                      }}
                    >
                      Rotate
                    </Button>
                  </Box>
                </Box>

                {/* 2. Read-Only Token */}
                <Box
                  sx={{
                    p: 2.5,
                    borderRadius: 2,
                    bgcolor: "rgba(14, 165, 233, 0.03)",
                    border: "1px solid #BAE6FD",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.5,
                  }}
                >
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Box
                        sx={{
                          width: 32,
                          height: 32,
                          borderRadius: 1.5,
                          bgcolor: "#E0F2FE",
                          color: "#0284C7",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                        }}
                      >
                        <ShieldCheck size={18} />
                      </Box>
                      <Box>
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0369A1" }}>
                          Read-Only Token
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#0284C7", fontWeight: 600 }}>
                          Safe Audit & Inspection Only
                        </Typography>
                      </Box>
                    </Box>
                    <Chip
                      size="small"
                      label="Safe / Read-Only"
                      color="info"
                      sx={{ height: 20, fontSize: "0.68rem", fontWeight: 700 }}
                    />
                  </Box>

                  <Typography variant="body2" sx={{ color: "#075985", fontSize: "0.82rem", lineHeight: 1.4 }}>
                    Allows the AI agent to inspect nodes, links, policies, and health metrics. Any mutation attempts
                    return an explicit permission-denied error.
                  </Typography>

                  <TextField
                    fullWidth
                    size="small"
                    type={showRoToken ? "text" : "password"}
                    value={data?.read_only_token || ""}
                    slotProps={{
                      input: {
                        readOnly: true,
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton size="small" onClick={() => setShowRoToken(!showRoToken)}>
                              {showRoToken ? <EyeOff size={16} /> : <Eye size={16} />}
                            </IconButton>
                          </InputAdornment>
                        ),
                      },
                    }}
                    sx={{
                      "& .MuiInputBase-input": {
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        bgcolor: "#FFFFFF",
                      },
                    }}
                  />

                  <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
                    <Button
                      fullWidth
                      variant="contained"
                      color="info"
                      size="small"
                      startIcon={copiedKey === "ro_tok" ? <Check size={14} /> : <Copy size={14} />}
                      onClick={() => handleCopy(data?.read_only_token || "", "ro_tok")}
                      sx={{ textTransform: "none", fontWeight: 600, borderRadius: 1.5 }}
                    >
                      {copiedKey === "ro_tok" ? "Copied!" : "Copy Read-Only Token"}
                    </Button>
                    <Button
                      variant="outlined"
                      color="inherit"
                      size="small"
                      onClick={() => handleOpenResetDialog("read_only")}
                      sx={{
                        textTransform: "none",
                        fontWeight: 600,
                        borderRadius: 1.5,
                        borderColor: "#7DD3FC",
                        color: "#0369A1",
                      }}
                    >
                      Rotate
                    </Button>
                  </Box>
                </Box>
              </Box>
            </Paper>

            {/* OAUTH 2.0 CREDENTIALS (GOOGLE GEMINI & ENTERPRISE) */}
            <Paper
              elevation={0}
              sx={{
                p: 3,
                borderRadius: 2.5,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                boxShadow: "0 1px 3px rgba(0, 0, 0, 0.05)",
              }}
            >
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "flex-start",
                  flexWrap: "wrap",
                  gap: 1.5,
                }}
              >
                <Box>
                  <Typography
                    variant="subtitle1"
                    sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}
                  >
                    <KeyRound size={18} color="#4F46E5" /> OAuth 2.0 Client Credentials
                  </Typography>
                  <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.5, mb: 2 }}>
                    Used by AI platforms requiring standard OAuth 2.0 authentication such as{" "}
                    <strong>Google Gemini Web App</strong> (Settings &gt; Personal Intelligence &gt; Connect to an MCP
                    server).
                  </Typography>
                </Box>
                <Chip
                  size="small"
                  label="RFC 8414 & RFC 9728"
                  sx={{ bgcolor: "#EEF2FF", color: "#4F46E5", fontWeight: 700, fontSize: "0.72rem" }}
                />
              </Box>

              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
                {/* OAuth Client ID */}
                <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 700,
                      color: "#334155",
                      textTransform: "uppercase",
                      letterSpacing: "0.04em",
                      display: "block",
                      mb: 1,
                    }}
                  >
                    OAuth Client ID
                  </Typography>
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <TextField
                      fullWidth
                      size="small"
                      value={data?.oauth_client_id || "easy42-mcp"}
                      slotProps={{ input: { readOnly: true } }}
                      sx={{
                        "& .MuiInputBase-input": { fontFamily: "monospace", fontSize: "0.82rem", bgcolor: "#FFFFFF" },
                      }}
                    />
                    <Tooltip title={copiedKey === "oauth_cid" ? "Copied!" : "Copy Client ID"}>
                      <Button
                        variant="outlined"
                        onClick={() => handleCopy(data?.oauth_client_id || "easy42-mcp", "oauth_cid")}
                        sx={{ minWidth: 42, px: 1, borderColor: "#CBD5E1" }}
                      >
                        {copiedKey === "oauth_cid" ? <Check size={16} color="#16A34A" /> : <Copy size={16} />}
                      </Button>
                    </Tooltip>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.8 }}>
                    Enter this into Gemini's "OAuth client ID" field.
                  </Typography>
                </Box>

                {/* OAuth Client Secret */}
                <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                    <Typography
                      variant="caption"
                      sx={{ fontWeight: 700, color: "#334155", textTransform: "uppercase", letterSpacing: "0.04em" }}
                    >
                      OAuth Client Secret
                    </Typography>
                    <Button
                      size="small"
                      onClick={() => handleOpenResetDialog("oauth_secret")}
                      sx={{ textTransform: "none", fontSize: "0.72rem", p: 0, minWidth: "auto", color: "#64748B" }}
                    >
                      Regenerate
                    </Button>
                  </Box>
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <TextField
                      fullWidth
                      size="small"
                      type={showOAuthSecret ? "text" : "password"}
                      value={data?.oauth_client_secret || ""}
                      slotProps={{
                        input: {
                          readOnly: true,
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton size="small" onClick={() => setShowOAuthSecret(!showOAuthSecret)}>
                                {showOAuthSecret ? <EyeOff size={16} /> : <Eye size={16} />}
                              </IconButton>
                            </InputAdornment>
                          ),
                        },
                      }}
                      sx={{
                        "& .MuiInputBase-input": { fontFamily: "monospace", fontSize: "0.82rem", bgcolor: "#FFFFFF" },
                      }}
                    />
                    <Tooltip title={copiedKey === "oauth_sec" ? "Copied!" : "Copy Client Secret"}>
                      <Button
                        variant="outlined"
                        onClick={() => handleCopy(data?.oauth_client_secret || "", "oauth_sec")}
                        sx={{ minWidth: 42, px: 1, borderColor: "#CBD5E1" }}
                      >
                        {copiedKey === "oauth_sec" ? <Check size={16} color="#16A34A" /> : <Copy size={16} />}
                      </Button>
                    </Tooltip>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.8 }}>
                    Enter this into Gemini's "OAuth client secret" field.
                  </Typography>
                </Box>
              </Box>
            </Paper>

            {/* AI TOOLS INTEGRATION GUIDES */}
            <Paper
              elevation={0}
              sx={{
                p: 3,
                borderRadius: 2.5,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                boxShadow: "0 1px 3px rgba(0, 0, 0, 0.05)",
              }}
            >
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "flex-start",
                  flexWrap: "wrap",
                  gap: 2,
                  mb: 2,
                }}
              >
                <Box>
                  <Typography
                    variant="subtitle1"
                    sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}
                  >
                    <Code size={18} color="#4F46E5" /> AI Agent Setup Guides & Snippets
                  </Typography>
                  <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.5 }}>
                    Select your AI assistant to generate copy-ready configuration snippets.
                  </Typography>
                </Box>

                {/* Interactive Token Switcher */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    p: 0.5,
                    borderRadius: 2,
                    bgcolor: "#F1F5F9",
                    border: "1px solid #E2E8F0",
                  }}
                >
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569", px: 1 }}>
                    Embed Token:
                  </Typography>
                  <Button
                    size="small"
                    variant={selectedToken === "full" ? "contained" : "text"}
                    color="error"
                    onClick={() => setSelectedToken("full")}
                    sx={{
                      textTransform: "none",
                      fontWeight: 700,
                      fontSize: "0.75rem",
                      py: 0.4,
                      px: 1.2,
                      borderRadius: 1.5,
                    }}
                  >
                    Full Control
                  </Button>
                  <Button
                    size="small"
                    variant={selectedToken === "ro" ? "contained" : "text"}
                    color="info"
                    onClick={() => setSelectedToken("ro")}
                    sx={{
                      textTransform: "none",
                      fontWeight: 700,
                      fontSize: "0.75rem",
                      py: 0.4,
                      px: 1.2,
                      borderRadius: 1.5,
                    }}
                  >
                    Read Only
                  </Button>
                </Box>
              </Box>

              <Tabs
                value={selectedClient}
                onChange={(_, v) => setSelectedClient(v)}
                sx={{
                  borderBottom: 1,
                  borderColor: "divider",
                  "& .MuiTab-root": { textTransform: "none", fontWeight: 600, minHeight: 40, py: 1 },
                }}
              >
                <Tab value="claude" label="Claude Desktop" icon={<Bot size={15} />} iconPosition="start" />
                <Tab value="cursor" label="Cursor IDE" icon={<Terminal size={15} />} iconPosition="start" />
                <Tab value="cline" label="Cline / Roo Code" icon={<Code size={15} />} iconPosition="start" />
                <Tab value="windsurf" label="Windsurf" icon={<Sliders size={15} />} iconPosition="start" />
                <Tab value="gemini" label="Google Gemini" icon={<Globe size={15} />} iconPosition="start" />
                <Tab value="stdio" label="CLI / Stdio" icon={<Network size={15} />} iconPosition="start" />
                <Tab value="curl" label="cURL Test" icon={<ExternalLink size={15} />} iconPosition="start" />
              </Tabs>

              {/* Special Guide Card for Google Gemini Web App */}
              {selectedClient === "gemini" && (
                <Box
                  sx={{
                    p: 2.5,
                    mt: 2,
                    borderRadius: 2,
                    bgcolor: "rgba(79, 70, 229, 0.04)",
                    border: "1px solid #C7D2FE",
                    display: "flex",
                    flexDirection: "column",
                    gap: 2,
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                    <Box
                      sx={{
                        width: 36,
                        height: 36,
                        borderRadius: 1.5,
                        bgcolor: "#EEF2FF",
                        color: "#4F46E5",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                      }}
                    >
                      <Globe size={20} />
                    </Box>
                    <Box>
                      <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E1B4B" }}>
                        Connect to Google Gemini Web App
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#4338CA" }}>
                        In Gemini Web, open <strong>Settings</strong> &gt; <strong>Personal Intelligence</strong> &gt;{" "}
                        <strong>Connect to an MCP server</strong> and paste these credentials:
                      </Typography>
                    </Box>
                  </Box>

                  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr 1fr" }, gap: 1.5 }}>
                    <Box>
                      <Typography
                        variant="caption"
                        sx={{ fontWeight: 700, color: "#312E81", display: "block", mb: 0.5 }}
                      >
                        1. MCP Server URL
                      </Typography>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <TextField
                          fullWidth
                          size="small"
                          value={data?.server_url || ""}
                          slotProps={{ input: { readOnly: true } }}
                          sx={{
                            "& .MuiInputBase-input": {
                              fontFamily: "monospace",
                              fontSize: "0.78rem",
                              bgcolor: "#FFFFFF",
                            },
                          }}
                        />
                        <Tooltip title={copiedKey === "gem_url" ? "Copied!" : "Copy"}>
                          <Button
                            variant="outlined"
                            size="small"
                            onClick={() => handleCopy(data?.server_url || "", "gem_url")}
                            sx={{ minWidth: 36, px: 0.5, borderColor: "#CBD5E1" }}
                          >
                            {copiedKey === "gem_url" ? <Check size={14} color="#16A34A" /> : <Copy size={14} />}
                          </Button>
                        </Tooltip>
                      </Box>
                    </Box>

                    <Box>
                      <Typography
                        variant="caption"
                        sx={{ fontWeight: 700, color: "#312E81", display: "block", mb: 0.5 }}
                      >
                        2. OAuth Client ID
                      </Typography>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <TextField
                          fullWidth
                          size="small"
                          value={data?.oauth_client_id || "easy42-mcp"}
                          slotProps={{ input: { readOnly: true } }}
                          sx={{
                            "& .MuiInputBase-input": {
                              fontFamily: "monospace",
                              fontSize: "0.78rem",
                              bgcolor: "#FFFFFF",
                            },
                          }}
                        />
                        <Tooltip title={copiedKey === "gem_cid" ? "Copied!" : "Copy"}>
                          <Button
                            variant="outlined"
                            size="small"
                            onClick={() => handleCopy(data?.oauth_client_id || "easy42-mcp", "gem_cid")}
                            sx={{ minWidth: 36, px: 0.5, borderColor: "#CBD5E1" }}
                          >
                            {copiedKey === "gem_cid" ? <Check size={14} color="#16A34A" /> : <Copy size={14} />}
                          </Button>
                        </Tooltip>
                      </Box>
                    </Box>

                    <Box>
                      <Typography
                        variant="caption"
                        sx={{ fontWeight: 700, color: "#312E81", display: "block", mb: 0.5 }}
                      >
                        3. OAuth Client Secret
                      </Typography>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <TextField
                          fullWidth
                          size="small"
                          type={showOAuthSecret ? "text" : "password"}
                          value={data?.oauth_client_secret || ""}
                          slotProps={{
                            input: {
                              readOnly: true,
                              endAdornment: (
                                <InputAdornment position="end">
                                  <IconButton size="small" onClick={() => setShowOAuthSecret(!showOAuthSecret)}>
                                    {showOAuthSecret ? <EyeOff size={14} /> : <Eye size={14} />}
                                  </IconButton>
                                </InputAdornment>
                              ),
                            },
                          }}
                          sx={{
                            "& .MuiInputBase-input": {
                              fontFamily: "monospace",
                              fontSize: "0.78rem",
                              bgcolor: "#FFFFFF",
                            },
                          }}
                        />
                        <Tooltip title={copiedKey === "gem_sec" ? "Copied!" : "Copy"}>
                          <Button
                            variant="outlined"
                            size="small"
                            onClick={() => handleCopy(data?.oauth_client_secret || "", "gem_sec")}
                            sx={{ minWidth: 36, px: 0.5, borderColor: "#CBD5E1" }}
                          >
                            {copiedKey === "gem_sec" ? <Check size={14} color="#16A34A" /> : <Copy size={14} />}
                          </Button>
                        </Tooltip>
                      </Box>
                    </Box>
                  </Box>
                </Box>
              )}

              <Box sx={{ mt: 2.5 }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                    {currentSnippet.filename}
                  </Typography>
                  <Button
                    size="small"
                    variant="contained"
                    startIcon={copiedKey === "snippet" ? <Check size={14} /> : <Copy size={14} />}
                    onClick={() => handleCopy(currentSnippet.code, "snippet")}
                    sx={{ textTransform: "none", fontWeight: 600, borderRadius: 1.5, py: 0.5 }}
                  >
                    {copiedKey === "snippet" ? "Copied!" : "Copy Snippet"}
                  </Button>
                </Box>

                {currentSnippet.pathNote && (
                  <Paper
                    elevation={0}
                    sx={{
                      p: 1.5,
                      mb: 1.5,
                      borderRadius: 1.5,
                      bgcolor: "#F8FAFC",
                      border: "1px solid #E2E8F0",
                      fontFamily: "monospace",
                      fontSize: "0.75rem",
                      color: "#475569",
                      whiteSpace: "pre-wrap",
                    }}
                  >
                    {currentSnippet.pathNote}
                  </Paper>
                )}

                <Paper
                  elevation={0}
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    bgcolor: "#0F172A",
                    color: "#F8FAFC",
                    fontFamily: "monospace",
                    fontSize: "0.82rem",
                    overflowX: "auto",
                    border: "1px solid #334155",
                    maxHeight: 280,
                  }}
                >
                  <pre style={{ margin: 0 }}>{currentSnippet.code}</pre>
                </Paper>
              </Box>
            </Paper>

            {/* MCP TOOLS DIRECTORY / CAPABILITIES */}
            <Paper
              elevation={0}
              sx={{
                p: 3,
                borderRadius: 2.5,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                boxShadow: "0 1px 3px rgba(0, 0, 0, 0.05)",
              }}
            >
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  cursor: "pointer",
                }}
                onClick={() => setShowToolsCatalog(!showToolsCatalog)}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <Box
                    sx={{
                      width: 36,
                      height: 36,
                      borderRadius: 2,
                      bgcolor: "#EEF2FF",
                      color: "#4F46E5",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                    }}
                  >
                    <FileCode size={20} />
                  </Box>
                  <Box>
                    <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                      Available MCP Tools & Actions ({MCP_TOOLS.length} Tools)
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.82rem" }}>
                      Explore the complete capabilities exposed to AI agents for cluster automation
                    </Typography>
                  </Box>
                </Box>
                <IconButton
                  size="small"
                  sx={{
                    transform: showToolsCatalog ? "rotate(180deg)" : "rotate(0deg)",
                    transition: "transform 0.2s",
                  }}
                >
                  <ChevronDown size={20} />
                </IconButton>
              </Box>

              {showToolsCatalog && (
                <Box sx={{ mt: 3, pt: 2, borderTop: "1px solid #E2E8F0" }}>
                  <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", mb: 2 }}>
                    <TextField
                      size="small"
                      placeholder="Search tools..."
                      value={toolSearch}
                      onChange={(e) => setToolSearch(e.target.value)}
                      sx={{ width: { xs: "100%", sm: 260 } }}
                    />
                    <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                      {categories.map((c) => (
                        <Chip
                          key={c}
                          label={c}
                          clickable
                          onClick={() => setToolCategory(c)}
                          color={toolCategory === c ? "primary" : "default"}
                          variant={toolCategory === c ? "filled" : "outlined"}
                          size="small"
                          sx={{ fontWeight: 600, fontSize: "0.75rem" }}
                        />
                      ))}
                    </Box>
                  </Box>

                  <Box
                    sx={{
                      display: "grid",
                      gridTemplateColumns: { xs: "1fr", sm: "repeat(auto-fill, minmax(320px, 1fr))" },
                      gap: 1.5,
                      maxHeight: 380,
                      overflowY: "auto",
                      pr: 0.5,
                    }}
                  >
                    {filteredTools.map((t) => (
                      <Paper
                        key={t.name}
                        elevation={0}
                        sx={{
                          p: 1.5,
                          borderRadius: 2,
                          bgcolor: "#F8FAFC",
                          border: "1px solid #E2E8F0",
                          display: "flex",
                          flexDirection: "column",
                          gap: 0.8,
                          "&:hover": { borderColor: "#CBD5E1", bgcolor: "#F1F5F9" },
                        }}
                      >
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                          <Typography
                            variant="subtitle2"
                            sx={{ fontFamily: "monospace", fontWeight: 700, color: "#1E293B", fontSize: "0.82rem" }}
                          >
                            {t.name}
                          </Typography>
                          <Chip
                            size="small"
                            label={t.isWrite ? "Full Control" : "Read Only"}
                            color={t.isWrite ? "warning" : "info"}
                            sx={{ height: 18, fontSize: "0.62rem", fontWeight: 700 }}
                          />
                        </Box>
                        <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.78rem", lineHeight: 1.35 }}>
                          {t.desc}
                        </Typography>
                        {t.args && (
                          <Typography
                            variant="caption"
                            sx={{
                              fontFamily: "monospace",
                              fontSize: "0.7rem",
                              color: "#4F46E5",
                              bgcolor: "#EEF2FF",
                              px: 0.8,
                              py: 0.3,
                              borderRadius: 1,
                              display: "inline-block",
                            }}
                          >
                            args: {t.args}
                          </Typography>
                        )}
                      </Paper>
                    ))}
                  </Box>
                </Box>
              )}
            </Paper>
          </>
        )}
      </Box>

      {/* ROTATE TOKEN CONFIRMATION DIALOG */}
      <Dialog open={resetDialogOpen} onClose={() => setResetDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}>
          <ShieldAlert size={20} color="#DC2626" /> Rotate Access Token
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: "#475569", lineHeight: 1.5 }}>
            Are you sure you want to rotate the{" "}
            <strong>
              {tokenToReset === "full_control"
                ? "Full-Control"
                : tokenToReset === "read_only"
                  ? "Read-Only"
                  : tokenToReset === "oauth_secret"
                    ? "OAuth Client Secret"
                    : "both"}
            </strong>{" "}
            credential?
          </Typography>
          <Alert severity="warning" sx={{ mt: 2, borderRadius: 2, fontSize: "0.82rem" }}>
            Any AI tools or background agents currently using this token will lose access until updated with the new
            token.
          </Alert>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button
            onClick={() => setResetDialogOpen(false)}
            disabled={resetting}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleConfirmReset}
            disabled={resetting}
            startIcon={<RefreshCw size={14} className={resetting ? "spin" : ""} />}
            sx={{ textTransform: "none", fontWeight: 700, borderRadius: 2 }}
          >
            {resetting ? "Rotating..." : "Rotate Token"}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};
