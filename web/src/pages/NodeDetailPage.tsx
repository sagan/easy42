import React, { useState, useEffect, useMemo, useCallback } from "react";
import { useParams, useNavigate, useSearchParams } from "react-router-dom";
import {
  Box,
  Paper,
  Typography,
  Button,
  IconButton,
  Chip,
  Divider,
  Tabs,
  Tab,
  Grid,
  TextField,
  Tooltip,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  LinearProgress,
  useTheme,
  useMediaQuery,
  Drawer,
  Menu,
  MenuItem,
  InputAdornment,
  ButtonGroup,
} from "@mui/material";
import {
  Server,
  ArrowLeft,
  RefreshCw,
  Activity,
  Network,
  Shield,
  FileText,
  Globe,
  Cpu,
  Radio,
  Wrench,
  Compass,
  Edit2,
  Tag,
  Trash2,
  RotateCcw,
  Copy,
  Check,
  Layers,
  Search,
  ArrowUp,
  ArrowDown,
  FileCode,
  Menu as MenuIcon,
  X,
  MoreVertical,
  Sliders,
  ArrowRightLeft,
  Zap,
} from "lucide-react";
import { useMesh } from "../context/MeshContext";
import { api } from "../api/client";
import { Node, NodeMetricPoint, GraphBlock } from "../types/api";
import { MetricAreaChart } from "../components/Nodes/MetricAreaChart";
import { MarkdownView } from "../components/Common/MarkdownView";

const formatBytes = (bytes?: number): string => {
  if (!bytes || isNaN(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val >= 10 || i === 0 ? val.toFixed(0) : val.toFixed(1)} ${units[i]}`;
};

const formatNetRate = (bytesPerSec?: number): string => {
  if (!bytesPerSec || isNaN(bytesPerSec) || bytesPerSec <= 0) return "0 B/s";
  if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(2)} MB/s`;
  if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`;
  return `${bytesPerSec.toFixed(0)} B/s`;
};

const formatUptime = (seconds?: number): string => {
  if (!seconds || seconds <= 0) return "Unknown";
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
};

export const NodeDetailPage: React.FC = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const navigate = useNavigate();
  const { nodeName } = useParams<{ nodeName: string }>();
  const [searchParams, setSearchParams] = useSearchParams();

  const {
    nodes,
    blocks,
    links,
    nodeStatuses,
    fleetLive,
    loadData,
    refreshFleetLive,
    handleUpdateState,
    updatingState,
    setNodeToEdit,
    setAddNodeOpen,
    setNodeToRename,
    setRenameModalOpen,
    setSyncTargetNode,
    setSyncOpen,
    setStateToast,
  } = useMesh();

  const decodedNodeName = useMemo(() => {
    return nodeName ? decodeURIComponent(nodeName) : "";
  }, [nodeName]);

  const node = useMemo(() => {
    return nodes.find((n) => n.name === decodedNodeName) || null;
  }, [nodes, decodedNodeName]);

  const status = nodeStatuses[decodedNodeName];
  const live = fleetLive?.nodes?.find((n) => n.name === decodedNodeName);

  const isOnline = live?.connected ?? status?.connected ?? false;
  const agentInstalled = Boolean(
    live?.agent_installed ?? (status?.agent_version != null || status?.mode === "agent" || node?.mode === "agent"),
  );

  // Tabs state synced with query params
  const tabParam = searchParams.get("tab") || "overview";
  const validTabs = ["overview", "metrics", "telemetry", "networking", "routing", "firewall"];
  const currentTab = validTabs.includes(tabParam)
    ? tabParam === "telemetry"
      ? "metrics"
      : tabParam
    : "overview";

  const handleTabChange = (_: React.SyntheticEvent, newValue: string) => {
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("tab", newValue);
        return next;
      },
      { replace: true },
    );
  };

  // Sidebar states
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
  const [sidebarSearch, setSidebarSearch] = useState("");

  // Telemetry state
  const [range, setRange] = useState<string>("24h");
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [points, setPoints] = useState<NodeMetricPoint[]>([]);

  // Action states
  const [probing, setProbing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [restartingWg, setRestartingWg] = useState(false);
  const [restartingBird, setRestartingBird] = useState(false);
  const [moreMenuAnchor, setMoreMenuAnchor] = useState<null | HTMLElement>(null);

  // Notes state
  const [isEditingNote, setIsEditingNote] = useState(false);
  const [noteDraft, setNoteDraft] = useState("");
  const [savingNote, setSavingNote] = useState(false);
  const [noteTab, setNoteTab] = useState<"write" | "preview">("write");

  // Config Viewers
  const [viewingBird, setViewingBird] = useState(false);
  const [birdConfig, setBirdConfig] = useState<string | null>(null);
  const [loadingBird, setLoadingBird] = useState(false);

  const [viewingNftables, setViewingNftables] = useState(false);
  const [nftablesConfig, setNftablesConfig] = useState<string | null>(null);
  const [loadingNftables, setLoadingNftables] = useState(false);

  // Copied text tooltip state
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  // Reset & load notes
  useEffect(() => {
    if (node) {
      setNoteDraft(node.note || "");
      setIsEditingNote(false);
      setNoteTab("write");
    }
  }, [node?.name, node?.note]);

  // Fetch telemetry
  const fetchMetrics = useCallback(
    async (flush = false) => {
      if (!decodedNodeName) return;
      setMetricsLoading(true);
      try {
        const res = await api.getNodeMetrics(decodedNodeName, range, flush);
        setPoints(res.points || []);
      } catch {
        setPoints([]);
      } finally {
        setMetricsLoading(false);
      }
    },
    [decodedNodeName, range],
  );

  useEffect(() => {
    if (decodedNodeName) {
      fetchMetrics();
    }
  }, [decodedNodeName, range, fetchMetrics]);

  // Grouped nodes for the sidebar
  // Grouped by node belonging block and ordered by name inside each block
  const groupedSidebarNodes = useMemo(() => {
    const q = sidebarSearch.trim().toLowerCase();

    // Set of node names attached to any block
    const assignedNodeNames = new Set<string>();

    const blockGroups: Array<{
      block: GraphBlock | { id: string; name: string; color: string };
      nodes: Node[];
    }> = [];

    (blocks || []).forEach((b) => {
      if (!b.name || b.name.toLowerCase() === "external") return;
      const bNodeNames = b.nodes || [];
      const matchingNodes = nodes
        .filter((n) => bNodeNames.includes(n.name))
        .filter((n) => {
          if (!q) return true;
          return n.name.toLowerCase().includes(q) || (n.host && n.host.toLowerCase().includes(q));
        })
        .sort((a, b) => a.name.localeCompare(b.name));

      bNodeNames.forEach((name) => assignedNodeNames.add(name));

      if (matchingNodes.length > 0) {
        blockGroups.push({
          block: b,
          nodes: matchingNodes,
        });
      }
    });

    // Ungrouped / Other Nodes
    const ungroupedNodes = nodes
      .filter((n) => !assignedNodeNames.has(n.name))
      .filter((n) => {
        if (!q) return true;
        return n.name.toLowerCase().includes(q) || (n.host && n.host.toLowerCase().includes(q));
      })
      .sort((a, b) => a.name.localeCompare(b.name));

    if (ungroupedNodes.length > 0) {
      blockGroups.push({
        block: { id: "ungrouped", name: "Ungrouped", color: "#64748B" },
        nodes: ungroupedNodes,
      });
    }

    return blockGroups;
  }, [blocks, nodes, sidebarSearch]);

  // Belonging blocks for current node
  const currentNodeBlocks = useMemo(() => {
    if (!node) return [];
    return (blocks || []).filter(
      (b) => b.name && b.name.toLowerCase() !== "external" && b.nodes && b.nodes.includes(node.name),
    );
  }, [blocks, node]);

  // Links connected to this node
  const nodeLinks = useMemo(() => {
    if (!node) return [];
    return links.filter((l) => l.from.name === node.name || l.to.name === node.name);
  }, [links, node]);

  // Actions
  const handleProbe = async () => {
    if (!node) return;
    setProbing(true);
    try {
      await api.refreshNodeStatus(node.name);
      await loadData();
      await refreshFleetLive();
      setStateToast({ message: `Node "${node.name}" status refreshed.`, severity: "success" });
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to probe node.", severity: "error" });
    } finally {
      setProbing(false);
    }
  };

  const handleDelete = async () => {
    if (!node) return;
    if (!window.confirm(`Are you sure you want to delete node "${node.name}"? This cannot be undone.`)) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteNode(node.name);
      await loadData();
      await refreshFleetLive();
      setStateToast({ message: `Node "${node.name}" deleted.`, severity: "info" });
      navigate("/nodes");
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to delete node.", severity: "error" });
      setDeleting(false);
    }
  };

  const handleRestartAllWg = async () => {
    if (!node) return;
    if (
      !window.confirm(
        `Restart all WireGuard interfaces on node "${node.name}"? This may temporarily disrupt traffic.`,
      )
    ) {
      return;
    }
    setRestartingWg(true);
    try {
      const res = await api.restartNodeWg(node.name);
      setStateToast({
        message: res.message || "WireGuard interfaces restarted successfully.",
        severity: "success",
      });
      await api.refreshNodeStatus(node.name);
      await loadData();
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to restart WireGuard.", severity: "error" });
    } finally {
      setRestartingWg(false);
    }
  };

  const handleRestartBird = async () => {
    if (!node) return;
    if (!window.confirm(`Restart BIRD routing daemon on node "${node.name}"?`)) {
      return;
    }
    setRestartingBird(true);
    try {
      const res = await api.restartNodeBird(node.name);
      setStateToast({ message: res.message || "BIRD service restarted successfully.", severity: "success" });
      await api.refreshNodeStatus(node.name);
      await loadData();
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to restart BIRD.", severity: "error" });
    } finally {
      setRestartingBird(false);
    }
  };

  const handleSaveNote = async () => {
    if (!node) return;
    setSavingNote(true);
    try {
      const updated = { ...node, note: noteDraft };
      await api.updateNode(node.name, updated);
      await loadData();
      setIsEditingNote(false);
      setStateToast({ message: "Note saved successfully.", severity: "success" });
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to save note.", severity: "error" });
    } finally {
      setSavingNote(false);
    }
  };

  const handleOpenBirdConfig = async () => {
    if (!node) return;
    setViewingBird(true);
    setLoadingBird(true);
    try {
      const res = await api.getNodeBirdConfig(node.name);
      setBirdConfig(res.config);
    } catch {
      setBirdConfig("# Failed to load BIRD configuration from node");
    } finally {
      setLoadingBird(false);
    }
  };

  const handleOpenNftablesConfig = async () => {
    if (!node) return;
    setViewingNftables(true);
    setLoadingNftables(true);
    try {
      const res = await api.getNodeNftablesConfig(node.name);
      setNftablesConfig(res.config);
    } catch {
      setNftablesConfig("# Failed to load Nftables configuration from node");
    } finally {
      setLoadingNftables(false);
    }
  };

  // Transform metrics data for charts
  const cpuData = useMemo(() => points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent })), [points]);
  const memData = useMemo(
    () =>
      points.map((p) => ({
        timestamp: p.timestamp,
        value: p.memory_used_bytes / (1024 * 1024 * 1024),
        value2: p.memory_total_bytes / (1024 * 1024 * 1024),
      })),
    [points],
  );
  const netData = useMemo(
    () =>
      points.map((p) => ({
        timestamp: p.timestamp,
        value: p.net_rx_rate,
        value2: p.net_tx_rate,
      })),
    [points],
  );
  const loadDataPoints = useMemo(
    () =>
      points.map((p) => ({
        timestamp: p.timestamp,
        value: p.load_1m,
        value2: p.load_5m,
      })),
    [points],
  );

  // Live quick metrics stats
  const metrics = live?.metrics || status?.metrics;
  const cpuPercent = metrics?.cpu_percent ?? 0;
  const memUsedGB = metrics ? metrics.memory_used_bytes / (1024 * 1024 * 1024) : 0;
  const memTotalGB = metrics ? metrics.memory_total_bytes / (1024 * 1024 * 1024) : 0;
  const memPercent = memTotalGB > 0 ? (memUsedGB / memTotalGB) * 100 : 0;
  const rxRate = live?.metrics?.net_rx_rate ?? 0;
  const txRate = live?.metrics?.net_tx_rate ?? 0;
  const disks = status?.metrics?.disks || status?.disks || [];
  const primaryDisk = disks.find((d) => d.path === "/") || disks[0];
  const diskUsedGB = primaryDisk ? primaryDisk.used_bytes / (1024 * 1024 * 1024) : 0;
  const diskTotalGB = primaryDisk ? primaryDisk.total_bytes / (1024 * 1024 * 1024) : 0;
  const diskPercent = diskTotalGB > 0 ? (diskUsedGB / diskTotalGB) * 100 : 0;

  // Sidebar list JSX component (shared between desktop sticky bar and mobile drawer)
  const sidebarContent = (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%", width: "100%" }}>
      {/* Sidebar Header & Search */}
      <Box sx={{ p: 2, borderBottom: "1px solid #E2E8F0" }}>
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Server size={18} color="#4F46E5" />
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
              Nodes Explorer
            </Typography>
          </Box>
          <Chip
            label={`${nodes.length} nodes`}
            size="small"
            sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#EEF2FF", color: "#4F46E5" }}
          />
        </Box>
        <TextField
          size="small"
          fullWidth
          placeholder="Filter nodes..."
          value={sidebarSearch}
          onChange={(e) => setSidebarSearch(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search size={15} color="#94A3B8" />
              </InputAdornment>
            ),
            sx: { height: 34, fontSize: "0.8rem", bgcolor: "#F8FAFC" },
          }}
        />
      </Box>

      {/* Grouped Nodes List */}
      <Box sx={{ flex: 1, overflowY: "auto", p: 1.5, display: "flex", flexDirection: "column", gap: 2 }}>
        {groupedSidebarNodes.length === 0 ? (
          <Box sx={{ p: 3, textAlign: "center" }}>
            <Typography variant="caption" sx={{ color: "#94A3B8" }}>
              No nodes match search
            </Typography>
          </Box>
        ) : (
          groupedSidebarNodes.map((group) => {
            const blockColor = (group.block as any).color || "#4F46E5";
            return (
              <Box key={group.block.id || group.block.name}>
                {/* Block Group Header */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 1,
                    py: 0.6,
                    mb: 0.6,
                    borderRadius: 1.5,
                    bgcolor: "rgba(241, 245, 249, 0.6)",
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                    <Box
                      sx={{
                        width: 8,
                        height: 8,
                        borderRadius: "50%",
                        bgcolor: blockColor,
                        boxShadow: `0 0 0 2px ${blockColor}22`,
                      }}
                    />
                    <Typography
                      variant="caption"
                      sx={{
                        fontWeight: 700,
                        color: "#334155",
                        letterSpacing: "0.3px",
                        fontSize: "0.72rem",
                        textTransform: "uppercase",
                      }}
                    >
                      {group.block.name}
                    </Typography>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.7rem", fontWeight: 600 }}>
                    {group.nodes.length}
                  </Typography>
                </Box>

                {/* Node Items in this block */}
                <Box sx={{ display: "flex", flexDirection: "column", gap: 0.4 }}>
                  {group.nodes.map((n) => {
                    const isSelected = n.name === decodedNodeName;
                    const nLive = fleetLive?.nodes?.find((item) => item.name === n.name);
                    const nStatus = nodeStatuses[n.name];
                    const nOnline = nLive?.connected ?? nStatus?.connected ?? false;
                    const nHasAgent = Boolean(
                      nLive?.agent_installed ?? (nStatus?.agent_version != null || nStatus?.mode === "agent"),
                    );

                    return (
                      <Paper
                        key={n.name}
                        elevation={0}
                        onClick={() => {
                          navigate(`/nodes/${encodeURIComponent(n.name)}${location.search}`);
                          if (isMobile) setMobileDrawerOpen(false);
                        }}
                        sx={{
                          p: 1.2,
                          px: 1.4,
                          borderRadius: 2,
                          cursor: "pointer",
                          transition: "all 0.15s ease",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          bgcolor: isSelected ? "#EEF2FF" : "#FFFFFF",
                          border: "1px solid",
                          borderColor: isSelected ? "#C7D2FE" : "#E2E8F0",
                          borderLeft: isSelected ? `4px solid ${blockColor}` : "1px solid #E2E8F0",
                          "&:hover": {
                            bgcolor: isSelected ? "#EEF2FF" : "#F8FAFC",
                            borderColor: isSelected ? "#A5B4FC" : "#CBD5E1",
                            transform: "translateX(2px)",
                          },
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1.2, minWidth: 0 }}>
                          {/* Dot indicator */}
                          <Box
                            sx={{
                              width: 8,
                              height: 8,
                              borderRadius: "50%",
                              flexShrink: 0,
                              bgcolor: nOnline ? "#10B981" : nHasAgent ? "#EF4444" : "#94A3B8",
                              boxShadow: nOnline ? "0 0 0 2px rgba(16, 185, 129, 0.25)" : "none",
                            }}
                          />
                          <Box sx={{ minWidth: 0 }}>
                            <Typography
                              variant="body2"
                              sx={{
                                fontWeight: isSelected ? 700 : 600,
                                color: isSelected ? "#3730A3" : "#0F172A",
                                fontSize: "0.82rem",
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {n.name}
                            </Typography>
                            <Typography
                              variant="caption"
                              className="mono-font"
                              sx={{
                                color: isSelected ? "#6366F1" : "#64748B",
                                fontSize: "0.68rem",
                                display: "block",
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {n.host || n.ip || "No IP"}
                            </Typography>
                          </Box>
                        </Box>

                        {/* Badges / indicators */}
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, flexShrink: 0 }}>
                          {n.is_external && (
                            <Chip
                              label="Ext"
                              size="small"
                              sx={{ height: 16, fontSize: "0.6rem", bgcolor: "#FEF3C7", color: "#B45309" }}
                            />
                          )}
                        </Box>
                      </Paper>
                    );
                  })}
                </Box>
              </Box>
            );
          })
        )}
      </Box>

      {/* Sidebar Footer Link */}
      <Box sx={{ p: 1.5, borderTop: "1px solid #E2E8F0", bgcolor: "#F8FAFC" }}>
        <Button
          fullWidth
          size="small"
          variant="outlined"
          startIcon={<ArrowLeft size={14} />}
          onClick={() => navigate("/nodes")}
          sx={{
            py: 0.7,
            fontSize: "0.78rem",
            fontWeight: 600,
            borderColor: "#CBD5E1",
            color: "#475569",
            bgcolor: "#FFFFFF",
          }}
        >
          All Nodes View
        </Button>
      </Box>
    </Box>
  );

  // If node not found
  if (!node && nodes.length > 0) {
    return (
      <Box sx={{ display: "flex", height: "calc(100vh - 64px)", bgcolor: "#F8FAFC" }}>
        {/* Desktop Sidebar */}
        {!isMobile && (
          <Box
            sx={{
              width: 300,
              flexShrink: 0,
              borderRight: "1px solid #E2E8F0",
              bgcolor: "#FFFFFF",
              display: "flex",
              flexDirection: "column",
            }}
          >
            {sidebarContent}
          </Box>
        )}

        <Box sx={{ flex: 1, p: 4, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <Paper
            elevation={0}
            sx={{
              p: 5,
              textAlign: "center",
              maxWidth: 480,
              borderRadius: 4,
              border: "1px dashed #CBD5E1",
              bgcolor: "#FFFFFF",
            }}
          >
            <Server size={42} color="#94A3B8" style={{ marginBottom: 16 }} />
            <Typography variant="h5" sx={{ fontWeight: 700, mb: 1, color: "#0F172A" }}>
              Node Not Found
            </Typography>
            <Typography variant="body2" sx={{ color: "#64748B", mb: 3 }}>
              Node <strong>"{decodedNodeName}"</strong> was not found in the current Easy42 mesh topology.
            </Typography>
            <Button
              variant="contained"
              startIcon={<ArrowLeft size={16} />}
              onClick={() => navigate("/nodes")}
            >
              Return to Nodes List
            </Button>
          </Paper>
        </Box>
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", height: "calc(100vh - 64px)", overflow: "hidden", bgcolor: "#F8FAFC" }}>
      {/* 1. Desktop Persistent Sidebar */}
      {!isMobile && (
        <Box
          sx={{
            width: 300,
            flexShrink: 0,
            borderRight: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            display: "flex",
            flexDirection: "column",
            height: "100%",
          }}
        >
          {sidebarContent}
        </Box>
      )}

      {/* 2. Mobile Collapsible Drawer */}
      <Drawer
        anchor="left"
        open={mobileDrawerOpen}
        onClose={() => setMobileDrawerOpen(false)}
        PaperProps={{ sx: { width: 310, bgcolor: "#FFFFFF" } }}
      >
        {sidebarContent}
      </Drawer>

      {/* 3. Main Details Area */}
      <Box
        sx={{
          flex: 1,
          display: "flex",
          flexDirection: "column",
          overflowY: "auto",
          p: { xs: 2, sm: 3, md: 4 },
          gap: 2.5,
        }}
      >
        {/* Navigation Breadcrumbs & Mobile Bar */}
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {isMobile && (
              <Button
                size="small"
                variant="outlined"
                startIcon={<MenuIcon size={16} />}
                onClick={() => setMobileDrawerOpen(true)}
                sx={{
                  mr: 1,
                  px: 1.2,
                  py: 0.5,
                  fontSize: "0.75rem",
                  borderColor: "#CBD5E1",
                  bgcolor: "#FFFFFF",
                }}
              >
                Nodes List
              </Button>
            )}
            <Button
              size="small"
              variant="text"
              startIcon={<ArrowLeft size={15} />}
              onClick={() => navigate("/nodes")}
              sx={{ color: "#64748B", fontWeight: 600, fontSize: "0.8rem", px: 1 }}
            >
              Back to Nodes
            </Button>
            <Typography variant="body2" sx={{ color: "#CBD5E1" }}>
              /
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 700, color: "#334155" }}>
              {node?.name}
            </Typography>
          </Box>

          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Tooltip title="Refresh node runtime telemetry & status">
              <Button
                size="small"
                variant="outlined"
                startIcon={<RefreshCw size={14} className={probing || metricsLoading ? "spin" : ""} />}
                onClick={() => {
                  handleProbe();
                  fetchMetrics(true);
                }}
                disabled={probing || metricsLoading}
                sx={{
                  bgcolor: "#FFFFFF",
                  color: "#475569",
                  borderColor: "#CBD5E1",
                  fontSize: "0.78rem",
                  py: 0.5,
                  px: 1.4,
                }}
              >
                {probing || metricsLoading ? "Refreshing..." : "Refresh Live"}
              </Button>
            </Tooltip>

            {!node?.is_external && (
              <Button
                size="small"
                variant="contained"
                startIcon={<Zap size={14} />}
                onClick={() => {
                  if (node) {
                    setSyncTargetNode(node.name);
                    setSyncOpen(true);
                  }
                }}
                sx={{
                  fontSize: "0.78rem",
                  py: 0.5,
                  px: 1.6,
                  bgcolor: "#4F46E5",
                  "&:hover": { bgcolor: "#4338CA" },
                }}
              >
                Sync Device
              </Button>
            )}

            <IconButton
              size="small"
              onClick={(e) => setMoreMenuAnchor(e.currentTarget)}
              sx={{ color: "#64748B", bgcolor: "#FFFFFF", border: "1px solid #CBD5E1", p: 0.7 }}
            >
              <MoreVertical size={16} />
            </IconButton>

            {/* More Actions Dropdown Menu */}
            <Menu
              anchorEl={moreMenuAnchor}
              open={Boolean(moreMenuAnchor)}
              onClose={() => setMoreMenuAnchor(null)}
              PaperProps={{ sx: { minWidth: 200, borderRadius: 2, p: 0.5, boxShadow: "0 10px 25px rgba(0,0,0,0.1)" } }}
            >
              <MenuItem
                onClick={() => {
                  setMoreMenuAnchor(null);
                  if (node) {
                    setNodeToEdit(node);
                    setAddNodeOpen(true);
                  }
                }}
                sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
              >
                <Edit2 size={16} color="#4F46E5" /> Edit Configuration
              </MenuItem>
              <MenuItem
                onClick={() => {
                  setMoreMenuAnchor(null);
                  if (node) {
                    setNodeToRename(node);
                    setRenameModalOpen(true);
                  }
                }}
                sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
              >
                <Tag size={16} color="#4F46E5" /> Rename Node
              </MenuItem>
              {!node?.is_external && (
                <MenuItem
                  onClick={() => {
                    setMoreMenuAnchor(null);
                    if (node) handleUpdateState(node.name);
                  }}
                  disabled={updatingState}
                  sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
                >
                  <Radio size={16} color="#0284C7" /> Update State & WireGuard
                </MenuItem>
              )}
              {!node?.is_external && (
                <MenuItem
                  onClick={() => {
                    setMoreMenuAnchor(null);
                    handleRestartAllWg();
                  }}
                  disabled={restartingWg}
                  sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
                >
                  <RotateCcw size={16} color="#0284C7" /> Restart All WireGuard
                </MenuItem>
              )}
              {!node?.is_external && (
                <MenuItem
                  onClick={() => {
                    setMoreMenuAnchor(null);
                    handleRestartBird();
                  }}
                  disabled={restartingBird}
                  sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
                >
                  <RotateCcw size={16} color="#D97706" /> Restart BIRD Service
                </MenuItem>
              )}
              <MenuItem
                onClick={() => {
                  setMoreMenuAnchor(null);
                  navigate(`/looking-glass?node=${encodeURIComponent(node?.name || "")}`);
                }}
                sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
              >
                <Compass size={16} color="#0891B2" /> Looking Glass
              </MenuItem>
              <MenuItem
                onClick={() => {
                  setMoreMenuAnchor(null);
                  navigate(`/helper?node=${encodeURIComponent(node?.name || "")}`);
                }}
                sx={{ fontSize: "0.82rem", gap: 1.2, py: 1 }}
              >
                <Wrench size={16} color="#475569" /> Device Config Helper
              </MenuItem>
              <Divider sx={{ my: 0.5 }} />
              <MenuItem
                onClick={() => {
                  setMoreMenuAnchor(null);
                  handleDelete();
                }}
                disabled={deleting}
                sx={{ fontSize: "0.82rem", gap: 1.2, py: 1, color: "#E11D48" }}
              >
                <Trash2 size={16} /> Delete Node
              </MenuItem>
            </Menu>
          </Box>
        </Box>

        {/* Hero Node Profile Card */}
        <Paper
          elevation={0}
          sx={{
            p: { xs: 2.5, sm: 3 },
            borderRadius: 3.5,
            bgcolor: "#FFFFFF",
            border: "1px solid #E2E8F0",
            display: "flex",
            flexDirection: "column",
            gap: 2.5,
            boxShadow: "0 4px 20px -2px rgba(15, 23, 42, 0.04)",
          }}
        >
          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "flex-start",
              flexWrap: "wrap",
              gap: 2,
            }}
          >
            {/* Identity and Badges */}
            <Box sx={{ display: "flex", alignItems: "flex-start", gap: 2 }}>
              <Box
                sx={{
                  p: 1.8,
                  borderRadius: 3,
                  bgcolor: "#EEF2FF",
                  color: "#4F46E5",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  border: "1px solid #C7D2FE",
                  boxShadow: "0 4px 12px rgba(79, 70, 229, 0.12)",
                }}
              >
                <Server size={32} />
              </Box>

              <Box>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.2, flexWrap: "wrap", mb: 0.5 }}>
                  <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", letterSpacing: "-0.5px" }}>
                    {node?.name}
                  </Typography>

                  {/* Status chip */}
                  <Chip
                    label={isOnline ? "Online" : agentInstalled ? "Offline" : "No Agent"}
                    size="small"
                    sx={{
                      bgcolor: isOnline ? "#ECFDF5" : agentInstalled ? "#FEF2F2" : "#F1F5F9",
                      color: isOnline ? "#059669" : agentInstalled ? "#E11D48" : "#64748B",
                      fontWeight: 700,
                      fontSize: "0.75rem",
                      height: 24,
                      px: 0.5,
                      border: !agentInstalled ? "1px solid #E2E8F0" : "none",
                    }}
                  />

                  {/* Belonging block chips */}
                  {currentNodeBlocks.map((b) => (
                    <Chip
                      key={b.id || b.name}
                      icon={<Layers size={13} style={{ color: b.color || "#4F46E5" }} />}
                      label={b.name}
                      size="small"
                      sx={{
                        height: 24,
                        fontSize: "0.72rem",
                        fontWeight: 700,
                        bgcolor: `${b.color || "#4F46E5"}15`,
                        color: b.color || "#4F46E5",
                        border: `1px solid ${b.color || "#4F46E5"}30`,
                      }}
                    />
                  ))}

                  {node?.is_external && (
                    <Chip
                      label="External Node"
                      size="small"
                      sx={{
                        height: 24,
                        fontSize: "0.72rem",
                        fontWeight: 700,
                        bgcolor: "#FEF3C7",
                        color: "#92400E",
                      }}
                    />
                  )}
                </Box>

                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
                  <Typography
                    variant="body2"
                    className="mono-font"
                    sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.6 }}
                  >
                    <Globe size={14} color="#94A3B8" /> {node?.host || "No host set"}
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#CBD5E1" }}>
                    •
                  </Typography>
                  <Typography variant="body2" sx={{ color: "#64748B" }}>
                    ASN: <strong style={{ color: "#0F172A" }}>{node?.asn || 424242}</strong>
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#CBD5E1" }}>
                    •
                  </Typography>
                  <Typography variant="body2" sx={{ color: "#64748B" }}>
                    Uptime:{" "}
                    <strong style={{ color: isOnline ? "#059669" : "#64748B" }}>
                      {formatUptime(live?.uptime_seconds ?? status?.metrics?.uptime_seconds)}
                    </strong>
                  </Typography>
                </Box>
              </Box>
            </Box>

            {/* Quick action buttons row */}
            <Box sx={{ display: "flex", gap: 1 }}>
              <Button
                variant="outlined"
                size="small"
                startIcon={<Compass size={15} />}
                onClick={() => navigate(`/looking-glass?node=${encodeURIComponent(node?.name || "")}`)}
                sx={{
                  color: "#0891B2",
                  borderColor: "#CFFAFE",
                  bgcolor: "#ECFEFF",
                  fontWeight: 600,
                  fontSize: "0.78rem",
                  "&:hover": { bgcolor: "#CFFAFE", borderColor: "#A5F3FC" },
                }}
              >
                Looking Glass
              </Button>
              <Button
                variant="outlined"
                size="small"
                startIcon={<Wrench size={15} />}
                onClick={() => navigate(`/helper?node=${encodeURIComponent(node?.name || "")}`)}
                sx={{
                  color: "#475569",
                  borderColor: "#E2E8F0",
                  bgcolor: "#F8FAFC",
                  fontWeight: 600,
                  fontSize: "0.78rem",
                  "&:hover": { bgcolor: "#F1F5F9", borderColor: "#CBD5E1" },
                }}
              >
                Device Helper
              </Button>
            </Box>
          </Box>

          {/* Quick Realtime Metrics Bar */}
          {agentInstalled && (
            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(4, 1fr)" },
                gap: 1.5,
                pt: 1,
                borderTop: "1px dashed #E2E8F0",
              }}
            >
              <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2.5, border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    CPU Load
                  </Typography>
                  <Cpu size={14} color="#4F46E5" />
                </Box>
                <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", fontSize: "1.15rem" }}>
                  {cpuPercent.toFixed(1)}%
                </Typography>
                <LinearProgress
                  variant="determinate"
                  value={Math.min(100, Math.max(0, cpuPercent))}
                  sx={{
                    height: 5,
                    borderRadius: 3,
                    mt: 1,
                    bgcolor: "#E2E8F0",
                    "& .MuiLinearProgress-bar": {
                      bgcolor: cpuPercent > 80 ? "#E11D48" : cpuPercent > 50 ? "#D97706" : "#4F46E5",
                    },
                  }}
                />
              </Box>

              <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2.5, border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    RAM Usage
                  </Typography>
                  <Activity size={14} color="#059669" />
                </Box>
                <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", fontSize: "1.15rem" }}>
                  {memUsedGB.toFixed(1)}{" "}
                  <span style={{ fontSize: "0.8rem", color: "#64748B", fontWeight: 500 }}>
                    / {memTotalGB.toFixed(1)} GB
                  </span>
                </Typography>
                <LinearProgress
                  variant="determinate"
                  value={Math.min(100, Math.max(0, memPercent))}
                  sx={{
                    height: 5,
                    borderRadius: 3,
                    mt: 1,
                    bgcolor: "#E2E8F0",
                    "& .MuiLinearProgress-bar": {
                      bgcolor: memPercent > 85 ? "#E11D48" : memPercent > 70 ? "#D97706" : "#059669",
                    },
                  }}
                />
              </Box>

              <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2.5, border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", fontWeight: 600, display: "flex", alignItems: "center", gap: 0.4 }}
                  >
                    <ArrowDown size={13} color="#059669" /> Inbound Rx
                  </Typography>
                </Box>
                <Typography variant="h6" sx={{ fontWeight: 800, color: "#059669", fontSize: "1.15rem" }}>
                  {formatNetRate(rxRate)}
                </Typography>
                <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.7rem", mt: 0.5, display: "block" }}>
                  {live?.primary_interface ? `via ${live.primary_interface}` : "Primary iface"}
                </Typography>
              </Box>

              <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2.5, border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", fontWeight: 600, display: "flex", alignItems: "center", gap: 0.4 }}
                  >
                    <ArrowUp size={13} color="#0891B2" /> Outbound Tx
                  </Typography>
                </Box>
                <Typography variant="h6" sx={{ fontWeight: 800, color: "#0891B2", fontSize: "1.15rem" }}>
                  {formatNetRate(txRate)}
                </Typography>
                <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.7rem", mt: 0.5, display: "block" }}>
                  {live?.primary_interface ? `via ${live.primary_interface}` : "Primary iface"}
                </Typography>
              </Box>
            </Box>
          )}
        </Paper>

        {/* Tab Controls */}
        <Box sx={{ borderBottom: "1px solid #E2E8F0", bgcolor: "#FFFFFF", borderRadius: 2.5, px: 2 }}>
          <Tabs
            value={currentTab}
            onChange={handleTabChange}
            variant="scrollable"
            scrollButtons="auto"
            sx={{
              minHeight: 48,
              "& .MuiTab-root": {
                fontWeight: 600,
                fontSize: "0.85rem",
                textTransform: "none",
                minHeight: 48,
                color: "#64748B",
                gap: 1,
                "&.Mui-selected": {
                  color: "#4F46E5",
                  fontWeight: 700,
                },
              },
              "& .MuiTabs-indicator": {
                bgcolor: "#4F46E5",
                height: 3,
                borderRadius: "3px 3px 0 0",
              },
            }}
          >
            <Tab value="overview" icon={<Sliders size={16} />} iconPosition="start" label="Overview & Specs" />
            <Tab
              value="metrics"
              icon={<Activity size={16} />}
              iconPosition="start"
              label="Telemetry & Historical Charts"
            />
            <Tab
              value="networking"
              icon={<Network size={16} />}
              iconPosition="start"
              label={`Network & Links (${nodeLinks.length})`}
            />
            <Tab value="routing" icon={<Radio size={16} />} iconPosition="start" label="Routing & BIRD" />
            <Tab value="firewall" icon={<Shield size={16} />} iconPosition="start" label="Firewall & Security" />
          </Tabs>
        </Box>

        {/* Tab 1: Overview & Specs */}
        {currentTab === "overview" && node && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <Grid container spacing={3}>
              {/* Configuration Specifications */}
              <Grid item xs={12} md={7}>
                <Paper
                  elevation={0}
                  sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0", height: "100%" }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", mb: 2 }}>
                    Network Addressing & Routing Tables
                  </Typography>

                  <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                    <Box sx={{ p: 1.8, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                          Main IPv4 Address
                        </Typography>
                        {node.ip && (
                          <Tooltip title={copiedKey === "ip4" ? "Copied!" : "Copy IP"}>
                            <IconButton size="small" onClick={() => handleCopy("ip4", node.ip!)}>
                              {copiedKey === "ip4" ? <Check size={14} color="#059669" /> : <Copy size={14} />}
                            </IconButton>
                          </Tooltip>
                        )}
                      </Box>
                      <Typography variant="body1" className="mono-font" sx={{ fontWeight: 700, color: "#0891B2", mt: 0.5 }}>
                        {node.ip || "—"}
                      </Typography>
                    </Box>

                    <Box sx={{ p: 1.8, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                          Main IPv6 Address
                        </Typography>
                        {node.ip6 && (
                          <Tooltip title={copiedKey === "ip6" ? "Copied!" : "Copy IP"}>
                            <IconButton size="small" onClick={() => handleCopy("ip6", node.ip6!)}>
                              {copiedKey === "ip6" ? <Check size={14} color="#059669" /> : <Copy size={14} />}
                            </IconButton>
                          </Tooltip>
                        )}
                      </Box>
                      <Typography
                        variant="body1"
                        className="mono-font"
                        sx={{
                          fontWeight: 700,
                          color: "#7C3AED",
                          mt: 0.5,
                          fontSize: "0.88rem",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                        }}
                      >
                        {node.ip6 || "—"}
                      </Typography>
                    </Box>

                    <Box sx={{ p: 1.8, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                        Autonomous System Number (ASN)
                      </Typography>
                      <Typography variant="body1" className="mono-font" sx={{ fontWeight: 700, color: "#4F46E5", mt: 0.5 }}>
                        AS{node.asn}
                      </Typography>
                    </Box>

                    <Box sx={{ p: 1.8, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                        Kernel Export Table
                      </Typography>
                      <Typography variant="body1" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
                        Table {node.table ?? 254}
                      </Typography>
                    </Box>
                  </Box>

                  {/* External IPs if configured */}
                  {(node.external_ip || node.external_ip6) && (
                    <Box sx={{ mt: 2.5, p: 2, bgcolor: "#FFFBEB", borderRadius: 2, border: "1px solid #FDE68A" }}>
                      <Typography variant="caption" sx={{ color: "#B45309", fontWeight: 700, display: "block", mb: 1 }}>
                        EXTERNAL CLEARED IPS (PUBLIC INTERNET)
                      </Typography>
                      <Box sx={{ display: "flex", gap: 3, flexWrap: "wrap" }}>
                        {node.external_ip && (
                          <Typography variant="body2" className="mono-font" sx={{ color: "#92400E" }}>
                            v4: <strong>{node.external_ip}</strong>
                          </Typography>
                        )}
                        {node.external_ip6 && (
                          <Typography variant="body2" className="mono-font" sx={{ color: "#92400E" }}>
                            v6: <strong>{node.external_ip6}</strong>
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  )}

                  {/* Node Tags */}
                  <Box sx={{ mt: 3 }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "block", mb: 1 }}>
                      NODE TAGS
                    </Typography>
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.8 }}>
                      {node.tags && node.tags.length > 0 ? (
                        node.tags.map((t) => (
                          <Chip
                            key={t}
                            label={t}
                            size="small"
                            sx={{ bgcolor: "#F1F5F9", color: "#334155", fontWeight: 600, fontSize: "0.75rem" }}
                          />
                        ))
                      ) : (
                        <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                          No tags assigned
                        </Typography>
                      )}
                    </Box>
                  </Box>
                </Paper>
              </Grid>

              {/* Host & Agent Details */}
              <Grid item xs={12} md={5}>
                <Paper
                  elevation={0}
                  sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0", height: "100%" }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", mb: 2 }}>
                    Host & Management Details
                  </Typography>

                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="body2" sx={{ color: "#64748B" }}>
                        Node Hostname:
                      </Typography>
                      <Typography variant="body2" className="mono-font" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        {live?.hostname || status?.hostname || "—"}
                      </Typography>
                    </Box>
                    <Divider />

                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="body2" sx={{ color: "#64748B" }}>
                        Operating System:
                      </Typography>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        {live?.os_info || "Linux"}
                      </Typography>
                    </Box>
                    <Divider />

                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="body2" sx={{ color: "#64748B" }}>
                        Management Mode:
                      </Typography>
                      <Chip
                        label={node.mode ? node.mode.toUpperCase() : "AGENT"}
                        size="small"
                        sx={{
                          height: 20,
                          fontSize: "0.68rem",
                          fontWeight: 700,
                          bgcolor: node.mode === "ssh" ? "#F1F5F9" : "#EEF2FF",
                          color: node.mode === "ssh" ? "#475569" : "#4F46E5",
                        }}
                      />
                    </Box>
                    <Divider />

                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="body2" sx={{ color: "#64748B" }}>
                        easy42 Agent Version:
                      </Typography>
                      <Typography variant="body2" className="mono-font" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        {live?.agent_version || status?.agent_version ? `v${live?.agent_version || status?.agent_version}` : "Not reported"}
                      </Typography>
                    </Box>
                    <Divider />

                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="body2" sx={{ color: "#64748B" }}>
                        Last Communication:
                      </Typography>
                      <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.8rem" }}>
                        {live?.last_seen || status?.last_seen || "Unknown"}
                      </Typography>
                    </Box>
                  </Box>

                  {/* Primary Disk info */}
                  {primaryDisk && (
                    <Box sx={{ mt: 3, p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.5 }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700 }}>
                          ROOT STORAGE (/)
                        </Typography>
                        <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                          {diskPercent.toFixed(1)}% Used
                        </Typography>
                      </Box>
                      <LinearProgress
                        variant="determinate"
                        value={diskPercent}
                        sx={{
                          height: 6,
                          borderRadius: 3,
                          my: 1,
                          bgcolor: "#E2E8F0",
                          "& .MuiLinearProgress-bar": {
                            bgcolor: diskPercent > 90 ? "#E11D48" : diskPercent > 75 ? "#D97706" : "#4F46E5",
                          },
                        }}
                      />
                      <Box sx={{ display: "flex", justifyContent: "space-between", mt: 0.5 }}>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Used: {diskUsedGB.toFixed(1)} GB
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Total: {diskTotalGB.toFixed(1)} GB
                        </Typography>
                      </Box>
                    </Box>
                  )}
                </Paper>
              </Grid>
            </Grid>

            {/* Node Documentation & Notes */}
            <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <FileText size={18} color="#4F46E5" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Node Notes & Documentation
                  </Typography>
                </Box>

                {!isEditingNote ? (
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<Edit2 size={13} />}
                    onClick={() => {
                      setNoteDraft(node.note || "");
                      setIsEditingNote(true);
                      setNoteTab("write");
                    }}
                    sx={{ fontSize: "0.78rem", py: 0.4, px: 1.5, borderColor: "#CBD5E1", color: "#4F46E5" }}
                  >
                    {node.note ? "Edit Notes" : "Add Notes"}
                  </Button>
                ) : (
                  <Box sx={{ display: "flex", gap: 1 }}>
                    <ButtonGroup size="small" variant="outlined">
                      <Button
                        variant={noteTab === "write" ? "contained" : "outlined"}
                        onClick={() => setNoteTab("write")}
                        sx={{ fontSize: "0.75rem", py: 0.3 }}
                      >
                        Write
                      </Button>
                      <Button
                        variant={noteTab === "preview" ? "contained" : "outlined"}
                        onClick={() => setNoteTab("preview")}
                        sx={{ fontSize: "0.75rem", py: 0.3 }}
                      >
                        Preview
                      </Button>
                    </ButtonGroup>
                  </Box>
                )}
              </Box>

              {!isEditingNote ? (
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    minHeight: 100,
                  }}
                >
                  <MarkdownView
                    content={node.note}
                    emptyText="No documentation recorded for this node yet. Click 'Add Notes' to add network notes or maintenance instructions."
                  />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                  {noteTab === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={4}
                      maxRows={12}
                      placeholder="Write markdown documentation, contact info, IP plans, maintenance notes..."
                      value={noteDraft}
                      onChange={(e) => setNoteDraft(e.target.value)}
                      disabled={savingNote}
                      sx={{
                        bgcolor: "#F8FAFC",
                        "& .MuiInputBase-root": {
                          fontFamily: "monospace",
                          fontSize: "0.85rem",
                        },
                      }}
                    />
                  ) : (
                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        bgcolor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
                        minHeight: 120,
                      }}
                    >
                      <MarkdownView content={noteDraft} emptyText="No content written yet." />
                    </Box>
                  )}
                  <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
                    <Button
                      size="small"
                      onClick={() => setIsEditingNote(false)}
                      disabled={savingNote}
                      sx={{ color: "#64748B" }}
                    >
                      Cancel
                    </Button>
                    <Button
                      size="small"
                      variant="contained"
                      onClick={handleSaveNote}
                      disabled={savingNote}
                      startIcon={savingNote ? <CircularProgress size={14} color="inherit" /> : <Check size={14} />}
                    >
                      {savingNote ? "Saving..." : "Save Documentation"}
                    </Button>
                  </Box>
                </Box>
              )}
            </Paper>
          </Box>
        )}

        {/* Tab 2: Telemetry & Historical Charts */}
        {currentTab === "metrics" && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {/* Range and controls */}
            <Paper
              elevation={0}
              sx={{
                p: 2,
                borderRadius: 3,
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                flexWrap: "wrap",
                gap: 2,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                <Activity size={20} color="#4F46E5" />
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Telemetry History ({points.length} samples collected)
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Realtime metrics aggregated and stored by easy42 time-series storage
                  </Typography>
                </Box>
              </Box>

              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                <ButtonGroup size="small" variant="outlined" sx={{ bgcolor: "#F8FAFC" }}>
                  {["1h", "6h", "24h", "7d"].map((r) => (
                    <Button
                      key={r}
                      onClick={() => setRange(r)}
                      sx={{
                        fontWeight: range === r ? 700 : 500,
                        bgcolor: range === r ? "#4F46E5" : "transparent",
                        color: range === r ? "#FFFFFF" : "#64748B",
                        borderColor: "#CBD5E1",
                        "&:hover": { bgcolor: range === r ? "#4338CA" : "#F1F5F9" },
                      }}
                    >
                      {r.toUpperCase()}
                    </Button>
                  ))}
                </ButtonGroup>

                <Tooltip title="Flush cached samples and refresh telemetry immediately">
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<RefreshCw size={14} className={metricsLoading ? "spin" : ""} />}
                    onClick={() => fetchMetrics(true)}
                    disabled={metricsLoading}
                    sx={{ borderColor: "#CBD5E1", color: "#475569" }}
                  >
                    Flush & Refresh
                  </Button>
                </Tooltip>
              </Box>
            </Paper>

            {metricsLoading && points.length === 0 ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 12 }}>
                <CircularProgress size={36} />
              </Box>
            ) : points.length === 0 ? (
              <Paper
                elevation={0}
                sx={{
                  p: 6,
                  textAlign: "center",
                  borderRadius: 3,
                  bgcolor: "#FFFFFF",
                  border: "1px dashed #CBD5E1",
                }}
              >
                <Activity size={36} color="#94A3B8" style={{ marginBottom: 12 }} />
                <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#334155" }}>
                  No Historical Telemetry Recorded Yet
                </Typography>
                <Typography variant="body2" sx={{ color: "#64748B", mt: 0.5, maxWidth: 450, mx: "auto" }}>
                  Telemetry points are pushed periodically by the easy42 background agent. Once the agent runs for a few
                  minutes, historical charts will populate here.
                </Typography>
              </Paper>
            ) : (
              <Grid container spacing={2.5}>
                {/* 1. CPU Utilization */}
                <Grid item xs={12} lg={6}>
                  <MetricAreaChart
                    title="CPU Utilization"
                    data={cpuData}
                    color="#4F46E5"
                    unit="%"
                    height={210}
                    formatValue={(v) => `${v.toFixed(1)}%`}
                  />
                </Grid>

                {/* 2. Memory Utilization */}
                <Grid item xs={12} lg={6}>
                  <MetricAreaChart
                    title="Memory Utilization (Used / Total)"
                    data={memData}
                    color="#059669"
                    color2="#94A3B8"
                    label1="Used RAM"
                    label2="Total RAM"
                    height={210}
                    formatValue={(v) => `${v.toFixed(2)} GB`}
                  />
                </Grid>

                {/* 3. Network Bandwidth (Traffic Rates) */}
                <Grid item xs={12} lg={6}>
                  <MetricAreaChart
                    title={
                      live?.primary_interface
                        ? `Network Bandwidth (${live.primary_interface})`
                        : "Network Bandwidth"
                    }
                    data={netData}
                    color="#059669"
                    color2="#0891B2"
                    label1="Inbound (Rx)"
                    label2="Outbound (Tx)"
                    height={210}
                    formatValue={formatNetRate}
                  />
                </Grid>

                {/* 4. System Load Average */}
                <Grid item xs={12} lg={6}>
                  <MetricAreaChart
                    title="System Load Average"
                    data={loadDataPoints}
                    color="#D97706"
                    color2="#64748B"
                    label1="Load 1 min"
                    label2="Load 5 min"
                    height={210}
                    formatValue={(v) => v.toFixed(2)}
                  />
                </Grid>
              </Grid>
            )}
          </Box>
        )}

        {/* Tab 3: Network & Links */}
        {currentTab === "networking" && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            {/* Connected Links Table */}
            <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Network size={18} color="#4F46E5" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Connected Peer Links ({nodeLinks.length})
                  </Typography>
                </Box>
              </Box>

              {nodeLinks.length === 0 ? (
                <Box sx={{ p: 4, textAlign: "center", bgcolor: "#F8FAFC", borderRadius: 2 }}>
                  <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                    No mesh links connected to this node yet. Connect nodes via the Topology Canvas or Add Link modal.
                  </Typography>
                </Box>
              ) : (
                <TableContainer>
                  <Table size="small">
                    <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                      <TableRow>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Peer Node</TableCell>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Interface</TableCell>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Tunnel Address</TableCell>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Listen Port</TableCell>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Keepalive</TableCell>
                        <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Cost / Policy</TableCell>
                        <TableCell align="right" sx={{ fontWeight: 700, color: "#475569" }}>
                          Actions
                        </TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {nodeLinks.map((link, idx) => {
                        const isFrom = link.from.name === node?.name;
                        const peerEnd = isFrom ? link.to : link.from;
                        const selfEnd = isFrom ? link.from : link.to;

                        return (
                          <TableRow key={idx} hover>
                            <TableCell>
                              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                                <ArrowRightLeft size={14} color="#64748B" />
                                <Typography
                                  variant="body2"
                                  onClick={() => navigate(`/nodes/${encodeURIComponent(peerEnd.name)}`)}
                                  sx={{
                                    fontWeight: 700,
                                    color: "#4F46E5",
                                    cursor: "pointer",
                                    "&:hover": { textDecoration: "underline" },
                                  }}
                                >
                                  {peerEnd.name}
                                </Typography>
                              </Box>
                            </TableCell>
                            <TableCell>
                              <Typography variant="body2" className="mono-font" sx={{ color: "#334155" }}>
                                {selfEnd.interface || "—"}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Typography variant="body2" className="mono-font" sx={{ color: "#0891B2" }}>
                                {selfEnd.address || selfEnd.address4 || "—"}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Typography variant="body2" className="mono-font">
                                {selfEnd.listen_port || "—"}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Typography variant="body2">
                                {selfEnd.persistent_keepalive ? `${selfEnd.persistent_keepalive}s` : "Default (25s)"}
                              </Typography>
                            </TableCell>
                            <TableCell>
                              <Chip
                                label={selfEnd.policy || "Default Mesh"}
                                size="small"
                                sx={{ height: 20, fontSize: "0.7rem", bgcolor: "#F1F5F9" }}
                              />
                            </TableCell>
                            <TableCell align="right">
                              <Button
                                size="small"
                                variant="text"
                                onClick={() => navigate(`/nodes/${encodeURIComponent(peerEnd.name)}`)}
                                sx={{ fontSize: "0.75rem", p: 0.5 }}
                              >
                                View Peer
                              </Button>
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </Paper>

            {/* WireGuard Interfaces & Handshake Status */}
            {status?.wg_interfaces && status.wg_interfaces.length > 0 && (
              <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    WireGuard Kernel Interfaces Status
                  </Typography>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<RotateCcw size={13} />}
                    onClick={handleRestartAllWg}
                    disabled={restartingWg}
                    sx={{ fontSize: "0.75rem", color: "#0284C7", borderColor: "#BAE6FD" }}
                  >
                    Restart WireGuard
                  </Button>
                </Box>

                <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  {status.wg_interfaces.map((wg) => (
                    <Box key={wg.name} sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.5 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Chip
                            label={wg.name}
                            size="small"
                            sx={{ fontWeight: 700, bgcolor: "#EEF2FF", color: "#4F46E5" }}
                          />
                          <Typography variant="caption" className="mono-font" sx={{ color: "#64748B" }}>
                            Listen Port: {wg.listen_port}
                          </Typography>
                        </Box>
                        <Typography variant="caption" className="mono-font" sx={{ color: "#64748B" }}>
                          Pubkey: {wg.public_key ? `${wg.public_key.substring(0, 14)}...` : "—"}
                        </Typography>
                      </Box>

                      {wg.peers && wg.peers.length > 0 && (
                        <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                          {wg.peers.map((p, pIdx) => (
                            <Box
                              key={pIdx}
                              sx={{
                                p: 1.2,
                                bgcolor: "#FFFFFF",
                                borderRadius: 1.5,
                                border: "1px solid #E2E8F0",
                                display: "flex",
                                justifyContent: "space-between",
                                alignItems: "center",
                                flexWrap: "wrap",
                                gap: 1,
                              }}
                            >
                              <Box>
                                <Typography variant="caption" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
                                  Peer: {p.public_key ? `${p.public_key.substring(0, 16)}...` : "—"}
                                </Typography>
                                <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                                  Endpoint: {p.endpoint || "Roaming"}
                                </Typography>
                              </Box>
                              <Box sx={{ textAlign: "right" }}>
                                <Typography variant="caption" sx={{ color: "#059669", fontWeight: 600, display: "block" }}>
                                  Rx: {formatBytes(p.transfer_rx_bytes)} • Tx: {formatBytes(p.transfer_tx_bytes)}
                                </Typography>
                                <Typography variant="caption" sx={{ color: "#64748B" }}>
                                  Handshake: {p.latest_handshake || "None"}
                                </Typography>
                              </Box>
                            </Box>
                          ))}
                        </Box>
                      )}
                    </Box>
                  ))}
                </Box>
              </Paper>
            )}

            {/* Network Interfaces on Host */}
            {status?.interfaces && status.interfaces.length > 0 && (
              <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", mb: 2 }}>
                  Host Physical & Virtual Interfaces ({status.interfaces.length})
                </Typography>
                <Grid container spacing={1.5}>
                  {status.interfaces.map((inf) => {
                    const isPrimary = ((inf.flags ?? 0) & 1) !== 0;
                    const isPhysical = ((inf.flags ?? 0) & 2) !== 0;

                    return (
                      <Grid item xs={12} sm={6} md={4} key={inf.name}>
                        <Box
                          sx={{
                            p: 1.5,
                            borderRadius: 2,
                            bgcolor: isPrimary ? "#EFF6FF" : "#F8FAFC",
                            border: "1px solid",
                            borderColor: isPrimary ? "#BFDBFE" : "#E2E8F0",
                          }}
                        >
                          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.8 }}>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                              <Typography variant="body2" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
                                {inf.name}
                              </Typography>
                              {isPrimary && (
                                <Chip
                                  label="Primary"
                                  size="small"
                                  sx={{ height: 16, fontSize: "0.6rem", bgcolor: "#DBEAFE", color: "#1D4ED8", fontWeight: 700 }}
                                />
                              )}
                              {isPhysical && (
                                <Chip
                                  label="Physical"
                                  size="small"
                                  sx={{ height: 16, fontSize: "0.6rem", bgcolor: "#E0E7FF", color: "#4338CA", fontWeight: 600 }}
                                />
                              )}
                            </Box>
                            <Chip
                              label={inf.up ? "UP" : "DOWN"}
                              size="small"
                              sx={{
                                height: 18,
                                fontSize: "0.65rem",
                                fontWeight: 700,
                                bgcolor: inf.up ? "#ECFDF5" : "#FEF2F2",
                                color: inf.up ? "#059669" : "#E11D48",
                              }}
                            />
                          </Box>
                          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.2 }}>
                            {inf.addresses?.map((addr, aIdx) => (
                              <Typography key={aIdx} variant="caption" className="mono-font" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
                                {addr}
                              </Typography>
                            ))}
                          </Box>
                        </Box>
                      </Grid>
                    );
                  })}
                </Grid>
              </Paper>
            )}
          </Box>
        )}

        {/* Tab 4: Routing & BIRD */}
        {currentTab === "routing" && node && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Radio size={18} color="#4F46E5" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    BIRD Internet Routing Daemon
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<FileCode size={14} />}
                    onClick={handleOpenBirdConfig}
                    sx={{ fontSize: "0.78rem", color: "#4F46E5", borderColor: "#C7D2FE" }}
                  >
                    View Generated BIRD Config
                  </Button>
                  {!node.is_external && (
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={<RotateCcw size={14} />}
                      onClick={handleRestartBird}
                      disabled={restartingBird}
                      sx={{ fontSize: "0.78rem", color: "#D97706", borderColor: "#FDE68A" }}
                    >
                      Restart BIRD
                    </Button>
                  )}
                </Box>
              </Box>

              <Grid container spacing={2}>
                <Grid item xs={12} sm={4}>
                  <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      Export Table
                    </Typography>
                    <Typography variant="h6" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
                      Table {node.table ?? 254}
                    </Typography>
                  </Box>
                </Grid>

                <Grid item xs={12} sm={4}>
                  <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      External Export Table
                    </Typography>
                    <Typography variant="h6" className="mono-font" sx={{ fontWeight: 700, color: "#7C3AED", mt: 0.5 }}>
                      {node.external_table ? `Table ${node.external_table}` : "None (uses main)"}
                    </Typography>
                  </Box>
                </Grid>

                <Grid item xs={12} sm={4}>
                  <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      Target Config Path
                    </Typography>
                    <Typography variant="h6" className="mono-font" sx={{ fontWeight: 700, color: "#0891B2", mt: 0.5 }}>
                      /etc/bird_easy42.conf
                    </Typography>
                  </Box>
                </Grid>
              </Grid>

              {/* Kernel Route Imports */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "block", mb: 1 }}>
                  KERNEL ROUTE IMPORTS ({node.routes?.length || 0})
                </Typography>
                {node.routes && node.routes.length > 0 ? (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                    {node.routes.map((rule, idx) => (
                      <Box
                        key={idx}
                        sx={{
                          p: 1.5,
                          borderRadius: 2,
                          bgcolor: "#F8FAFC",
                          border: "1px solid #E2E8F0",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                        }}
                      >
                        <Chip
                          label={`Table ${rule.table}`}
                          size="small"
                          sx={{ fontWeight: 700, bgcolor: "#EEF2FF", color: "#4F46E5" }}
                        />
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {rule.prefixes?.map((p) => (
                            <Chip
                              key={p}
                              label={p}
                              size="small"
                              variant="outlined"
                              className="mono-font"
                              sx={{ height: 22, fontSize: "0.7rem" }}
                            />
                          ))}
                        </Box>
                      </Box>
                    ))}
                  </Box>
                ) : (
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                    No kernel table imports configured for this node.
                  </Typography>
                )}
              </Box>

              {/* Static routes */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "block", mb: 1 }}>
                  STATIC ROUTES ({node.static_routes?.length || 0})
                </Typography>
                {node.static_routes && node.static_routes.length > 0 ? (
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.8 }}>
                    {node.static_routes.map((sr) => (
                      <Chip
                        key={sr}
                        label={sr}
                        size="small"
                        className="mono-font"
                        sx={{ bgcolor: "#EDE9FE", color: "#6D28D9", fontWeight: 600 }}
                      />
                    ))}
                  </Box>
                ) : (
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                    No static routes configured.
                  </Typography>
                )}
              </Box>
            </Paper>
          </Box>
        )}

        {/* Tab 5: Firewall & Security */}
        {currentTab === "firewall" && node && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
            <Paper elevation={0} sx={{ p: 3, borderRadius: 3, bgcolor: "#FFFFFF", border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Shield size={18} color="#E11D48" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Nftables Firewall & NAT Engine
                  </Typography>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<FileCode size={14} />}
                  onClick={handleOpenNftablesConfig}
                  sx={{ fontSize: "0.78rem", color: "#E11D48", borderColor: "#FECDD3" }}
                >
                  View Nftables Rules
                </Button>
              </Box>

              <Grid container spacing={2}>
                <Grid item xs={12} sm={6}>
                  <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      Target Rules File
                    </Typography>
                    <Typography variant="h6" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
                      /etc/easy42.nft
                    </Typography>
                  </Box>
                </Grid>

                <Grid item xs={12} sm={6}>
                  <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      Firewall Backend Status
                    </Typography>
                    <Typography variant="h6" sx={{ fontWeight: 700, color: "#059669", mt: 0.5 }}>
                      Active (nftables Linux kernel)
                    </Typography>
                  </Box>
                </Grid>
              </Grid>

              {/* Entrypoints & NAT summary */}
              <Box sx={{ mt: 3 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "block", mb: 1 }}>
                  PORT FORWARDING & NAT ENTRYPOINTS ({node.entrypoints?.length || 0})
                </Typography>
                {node.entrypoints && node.entrypoints.length > 0 ? (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                    {node.entrypoints.map((ep, idx) => (
                      <Box
                        key={idx}
                        sx={{
                          p: 1.5,
                          borderRadius: 2,
                          bgcolor: "#F8FAFC",
                          border: "1px solid #E2E8F0",
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                          flexWrap: "wrap",
                          gap: 1,
                        }}
                      >
                        <Typography variant="body2" className="mono-font" sx={{ fontWeight: 700, color: "#0891B2" }}>
                          {ep.ip || "Outbound SNAT (Strict)"}
                        </Typography>
                        <Box sx={{ display: "flex", gap: 0.8, alignItems: "center" }}>
                          {ep.ports?.map((p, pIdx) => (
                            <Chip
                              key={pIdx}
                              label={`Port ${p.range || p.external_port || p.port}`}
                              size="small"
                              sx={{ height: 20, fontSize: "0.68rem" }}
                            />
                          ))}
                          {ep.mtu && (
                            <Chip label={`MTU ${ep.mtu}`} size="small" sx={{ height: 20, fontSize: "0.68rem" }} />
                          )}
                          <Chip
                            label={ep.tags?.join(", ") || "default"}
                            size="small"
                            sx={{ height: 20, fontSize: "0.68rem", bgcolor: "#EEF2FF", color: "#4F46E5" }}
                          />
                        </Box>
                      </Box>
                    ))}
                  </Box>
                ) : (
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                    No external entrypoints or port redirects configured.
                  </Typography>
                )}
              </Box>
            </Paper>
          </Box>
        )}
      </Box>

      {/* BIRD Config Dialog */}
      <Dialog open={viewingBird} onClose={() => setViewingBird(false)} maxWidth="md" fullWidth>
        <DialogTitle sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid #E2E8F0" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <FileCode size={20} color="#4F46E5" />
            <Typography variant="h6" sx={{ fontWeight: 700 }}>
              BIRD Configuration: {node?.name}
            </Typography>
          </Box>
          <IconButton size="small" onClick={() => setViewingBird(false)}>
            <X size={18} />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ pt: 2.5 }}>
          <Alert severity="info" sx={{ mb: 2, borderRadius: 2 }}>
            Deployed to <code>/etc/bird_easy42.conf</code>. Ensure your main BIRD config includes:
            <Box
              component="pre"
              sx={{ m: 0.5, p: 0.8, bgcolor: "#EEF2FF", color: "#4338CA", borderRadius: 1, fontFamily: "monospace", fontSize: "0.75rem" }}
            >
              include "/etc/bird_easy42.conf";
            </Box>
          </Alert>
          {loadingBird ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={32} />
            </Box>
          ) : (
            <Box
              component="pre"
              className="mono-font"
              sx={{
                p: 2,
                borderRadius: 2,
                bgcolor: "#0F172A",
                color: "#38BDF8",
                fontSize: "0.8rem",
                maxHeight: "55vh",
                overflow: "auto",
              }}
            >
              {birdConfig || "# No config available"}
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, py: 1.5, borderTop: "1px solid #E2E8F0", bgcolor: "#F8FAFC" }}>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              if (birdConfig) navigator.clipboard.writeText(birdConfig);
            }}
            disabled={!birdConfig || loadingBird}
          >
            Copy Config
          </Button>
          <Button size="small" variant="contained" onClick={() => setViewingBird(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* Nftables Config Dialog */}
      <Dialog open={viewingNftables} onClose={() => setViewingNftables(false)} maxWidth="md" fullWidth>
        <DialogTitle sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid #E2E8F0" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Shield size={20} color="#E11D48" />
            <Typography variant="h6" sx={{ fontWeight: 700 }}>
              Nftables Firewall Rules: {node?.name}
            </Typography>
          </Box>
          <IconButton size="small" onClick={() => setViewingNftables(false)}>
            <X size={18} />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ pt: 2.5 }}>
          <Alert severity="info" sx={{ mb: 2, borderRadius: 2 }}>
            Deployed to <code>/etc/easy42.nft</code> during sync. Executed via <code>nft -f /etc/easy42.nft</code>.
          </Alert>
          {loadingNftables ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={32} />
            </Box>
          ) : (
            <Box
              component="pre"
              className="mono-font"
              sx={{
                p: 2,
                borderRadius: 2,
                bgcolor: "#0F172A",
                color: "#F43F5E",
                fontSize: "0.8rem",
                maxHeight: "55vh",
                overflow: "auto",
              }}
            >
              {nftablesConfig || "# No config available"}
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, py: 1.5, borderTop: "1px solid #E2E8F0", bgcolor: "#F8FAFC" }}>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              if (nftablesConfig) navigator.clipboard.writeText(nftablesConfig);
            }}
            disabled={!nftablesConfig || loadingNftables}
          >
            Copy Config
          </Button>
          <Button size="small" variant="contained" onClick={() => setViewingNftables(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
