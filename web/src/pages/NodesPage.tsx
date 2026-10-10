import React, { useState, useMemo, useEffect, useCallback } from "react";
import {
  Box,
  Typography,
  Paper,
  Grid,
  TextField,
  InputAdornment,
  Chip,
  IconButton,
  Button,
  ToggleButtonGroup,
  ToggleButton,
  LinearProgress,
  Tooltip,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TableSortLabel,
  Select,
  MenuItem,
  FormControl,
} from "@mui/material";
import {
  Server,
  Search,
  LayoutGrid,
  List,
  RefreshCw,
  Activity,
  ArrowDown,
  ArrowUp,
  Cpu,
  HardDrive,
  Compass,
  Info,
  Zap,
  Layers,
  X,
} from "lucide-react";
import { useMesh } from "../context/MeshContext";
import { NodeLiveStatus } from "../types/api";
import { NodeMetricsDrawer } from "../components/Nodes/NodeMetricsDrawer";
import { NodeDetailDrawer } from "../components/Topology/NodeDetailDrawer";
import { useNavigate, useSearchParams } from "react-router-dom";

export type SortField = "name" | "status" | "block" | "host" | "uptime" | "cpu" | "memory" | "disk" | "network";

export const NodesPage: React.FC = () => {
  const {
    nodes,
    nodeStatuses,
    fleetLive,
    refreshFleetLive,
    selectedTag,
    setSelectedTag,
    selectedNode,
    setSelectedNode,
    setNodeToEdit,
    setAddNodeOpen,
    setNodeToRename,
    setRenameModalOpen,
    setSyncTargetNode,
    setSyncOpen,
    loadData,
    handleUpdateState,
    updatingState,
    blocks,
  } = useMesh();

  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  // Read filter and sort state from URL query parameters
  const urlSearch = searchParams.get("search") || "";
  const urlTag = searchParams.get("tag") || "All";
  const urlBlock = searchParams.get("block") || "All";
  const urlView = (searchParams.get("view") === "table" ? "table" : "cards") as "cards" | "table";
  const sortField = (searchParams.get("sort") || "name") as SortField;
  const sortOrder = (searchParams.get("order") === "desc" ? "desc" : "asc") as "asc" | "desc";

  const handleRequestSort = useCallback(
    (field: SortField) => {
      const isAsc = sortField === field && sortOrder === "asc";
      const newOrder = isAsc ? "desc" : "asc";
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("sort", field);
          next.set("order", newOrder);
          return next;
        },
        { replace: true }
      );
    },
    [sortField, sortOrder, setSearchParams]
  );

  // Local state for search term with instant UI feedback & URL synchronization
  const [searchTerm, setSearchTerm] = useState(urlSearch);
  const [selectedMetricsNode, setSelectedMetricsNode] = useState<string | null>(null);

  // Synchronize local search term if URL search changes externally
  useEffect(() => {
    setSearchTerm(urlSearch);
  }, [urlSearch]);

  // Synchronize selected tag in context if URL tag is provided
  useEffect(() => {
    if (urlTag && urlTag !== selectedTag) {
      setSelectedTag(urlTag);
    }
  }, [urlTag]);

  // Debounce search term updates to URL
  useEffect(() => {
    const timer = setTimeout(() => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          const trimmed = searchTerm.trim();
          if (trimmed) {
            if (next.get("search") !== trimmed) next.set("search", trimmed);
          } else {
            next.delete("search");
          }
          return next;
        },
        { replace: true }
      );
    }, 250);
    return () => clearTimeout(timer);
  }, [searchTerm, setSearchParams]);

  // Helper to update URL query parameters
  const updateQueryParam = useCallback(
    (updates: { tag?: string; block?: string; view?: "cards" | "table" }) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (updates.tag !== undefined) {
            if (updates.tag && updates.tag !== "All") {
              next.set("tag", updates.tag);
            } else {
              next.delete("tag");
            }
          }
          if (updates.block !== undefined) {
            if (updates.block && updates.block !== "All") {
              next.set("block", updates.block);
            } else {
              next.delete("block");
            }
          }
          if (updates.view !== undefined) {
            if (updates.view && updates.view !== "cards") {
              next.set("view", updates.view);
            } else {
              next.delete("view");
            }
          }
          return next;
        },
        { replace: true }
      );
    },
    [setSearchParams]
  );

  const handleSelectTag = (tag: string) => {
    setSelectedTag(tag);
    updateQueryParam({ tag });
  };

  const handleSelectBlock = (block: string) => {
    updateQueryParam({ block });
  };

  const handleToggleView = (view: "cards" | "table") => {
    updateQueryParam({ view });
  };

  const handleClearFilters = () => {
    setSearchTerm("");
    setSelectedTag("All");
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.delete("search");
        next.delete("tag");
        next.delete("block");
        return next;
      },
      { replace: true }
    );
  };

  // Requirement 2: Completely exclude external nodes from Nodes monitoring page
  const managedNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external);
  }, [nodes]);

  // Map each node to its belonging blocks (excluding external-only blocks or blocks named "external")
  const nodeBlocksMap = useMemo(() => {
    const map = new Map<string, string[]>();
    (blocks || []).forEach((b) => {
      if (b.name && b.name.toLowerCase() !== "external" && b.nodes && Array.isArray(b.nodes)) {
        b.nodes.forEach((nodeName) => {
          const list = map.get(nodeName) || [];
          if (!list.includes(b.name)) {
            list.push(b.name);
          }
          map.set(nodeName, list);
        });
      }
    });
    return map;
  }, [blocks]);

  // Unique block names that belong to at least one managed node, excluding "external"
  const managedBlocks = useMemo(() => {
    const set = new Set<string>();
    const managedNames = new Set(managedNodes.map((n) => n.name));
    (blocks || []).forEach((b) => {
      if (
        b.name &&
        b.name.toLowerCase() !== "external" &&
        b.nodes &&
        Array.isArray(b.nodes) &&
        b.nodes.some((name) => managedNames.has(name))
      ) {
        set.add(b.name);
      }
    });
    return Array.from(set).sort();
  }, [blocks, managedNodes]);

  // Unique tags from managed (non-external) nodes only, excluding "external"
  const managedTags = useMemo(() => {
    const set = new Set<string>();
    managedNodes.forEach((n) => {
      n.tags?.forEach((t) => {
        const trimmed = t.trim();
        if (trimmed && trimmed.toLowerCase() !== "external") {
          set.add(trimmed);
        }
      });
    });
    return Array.from(set).sort();
  }, [managedNodes]);

  // Requirement 3: Filter nodes by tag, block, and search term
  const filteredNodes = useMemo(() => {
    return managedNodes.filter((n) => {
      const matchTag =
        urlTag === "All" || !managedTags.includes(urlTag) || (n.tags && n.tags.includes(urlTag));
      const nBlocks = nodeBlocksMap.get(n.name) || [];
      const matchBlock =
        urlBlock === "All" || !managedBlocks.includes(urlBlock) || nBlocks.includes(urlBlock);
      const q = searchTerm.toLowerCase().trim();
      const matchSearch =
        !q ||
        n.name.toLowerCase().includes(q) ||
        (n.host || "").toLowerCase().includes(q) ||
        (n.tags && n.tags.some((t) => t.toLowerCase().includes(q))) ||
        nBlocks.some((b) => b.toLowerCase().includes(q));
      return matchTag && matchBlock && matchSearch;
    });
  }, [managedNodes, urlTag, urlBlock, searchTerm, nodeBlocksMap, managedTags, managedBlocks]);

  // Map node name to live telemetry status
  const liveMap = useMemo(() => {
    const map = new Map<string, NodeLiveStatus>();
    if (fleetLive?.nodes) {
      fleetLive.nodes.forEach((nl) => map.set(nl.name, nl));
    }
    return map;
  }, [fleetLive]);

  const formatNetRate = (bytesPerSec?: number) => {
    if (!bytesPerSec) return "0 B/s";
    if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`;
    if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(0)} KB/s`;
    return `${bytesPerSec.toFixed(0)} B/s`;
  };

  const formatUptime = (secs?: number) => {
    if (!secs) return "Offline";
    const d = Math.floor(secs / 86400);
    const h = Math.floor((secs % 86400) / 3600);
    const m = Math.floor((secs % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  };

  const getCpuColor = (cpu?: number) => {
    if (!cpu || cpu < 60) return "#059669"; // green
    if (cpu < 85) return "#D97706"; // orange
    return "#E11D48"; // red
  };

  const selectedNodeLive = selectedMetricsNode ? liveMap.get(selectedMetricsNode) : null;
  const isFiltering = Boolean(
    searchTerm.trim() ||
    (urlTag !== "All" && managedTags.includes(urlTag)) ||
    (urlBlock !== "All" && managedBlocks.includes(urlBlock))
  );

  // Sort displayed nodes by active column and order
  const sortedNodes = useMemo(() => {
    const list = [...filteredNodes];
    list.sort((a, b) => {
      const liveA = liveMap.get(a.name);
      const liveB = liveMap.get(b.name);
      const statusA = nodeStatuses[a.name];
      const statusB = nodeStatuses[b.name];

      const isOnlineA = Boolean(liveA?.connected ?? statusA?.connected);
      const isOnlineB = Boolean(liveB?.connected ?? statusB?.connected);
      const agentA = Boolean(
        liveA?.agent_installed ?? (
          statusA?.connected ||
          Boolean(statusA?.agent_version) ||
          Boolean(statusA?.metrics && statusA.metrics.memory_total_bytes > 0)
        )
      );
      const agentB = Boolean(
        liveB?.agent_installed ?? (
          statusB?.connected ||
          Boolean(statusB?.agent_version) ||
          Boolean(statusB?.metrics && statusB.metrics.memory_total_bytes > 0)
        )
      );

      let comparison = 0;

      switch (sortField) {
        case "name":
          comparison = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
          break;

        case "status": {
          const rankA = isOnlineA ? 2 : agentA ? 1 : 0;
          const rankB = isOnlineB ? 2 : agentB ? 1 : 0;
          comparison = rankA - rankB;
          if (comparison === 0) {
            comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
          }
          break;
        }

        case "block": {
          const blockA = (nodeBlocksMap.get(a.name) || [])[0] || "";
          const blockB = (nodeBlocksMap.get(b.name) || [])[0] || "";
          comparison = blockA.localeCompare(blockB, undefined, { numeric: true });
          if (comparison === 0) {
            comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
          }
          break;
        }

        case "host": {
          const hostA = a.host || "";
          const hostB = b.host || "";
          comparison = hostA.localeCompare(hostB, undefined, { numeric: true });
          break;
        }

        case "uptime": {
          const uptimeA = isOnlineA ? (liveA?.uptime_seconds ?? statusA?.metrics?.uptime_seconds ?? 0) : 0;
          const uptimeB = isOnlineB ? (liveB?.uptime_seconds ?? statusB?.metrics?.uptime_seconds ?? 0) : 0;
          comparison = uptimeA - uptimeB;
          break;
        }

        case "cpu": {
          const cpuA = agentA ? (liveA?.metrics?.cpu_percent ?? statusA?.metrics?.cpu_percent ?? 0) : -1;
          const cpuB = agentB ? (liveB?.metrics?.cpu_percent ?? statusB?.metrics?.cpu_percent ?? 0) : -1;
          comparison = cpuA - cpuB;
          break;
        }

        case "memory": {
          const memA = agentA ? (liveA?.metrics?.memory_used_bytes ?? statusA?.metrics?.memory_used_bytes ?? 0) : -1;
          const memB = agentB ? (liveB?.metrics?.memory_used_bytes ?? statusB?.metrics?.memory_used_bytes ?? 0) : -1;
          comparison = memA - memB;
          break;
        }

        case "disk": {
          const disksA = statusA?.metrics?.disks || statusA?.disks || [];
          const disksB = statusB?.metrics?.disks || statusB?.disks || [];
          const diskA = disksA.find((d) => d.path === "/") || disksA[0];
          const diskB = disksB.find((d) => d.path === "/") || disksB[0];
          const usedA = agentA && diskA ? diskA.used_bytes : -1;
          const usedB = agentB && diskB ? diskB.used_bytes : -1;
          comparison = usedA - usedB;
          break;
        }

        case "network": {
          const netA = agentA ? ((liveA?.metrics?.net_rx_rate ?? 0) + (liveA?.metrics?.net_tx_rate ?? 0)) : -1;
          const netB = agentB ? ((liveB?.metrics?.net_rx_rate ?? 0) + (liveB?.metrics?.net_tx_rate ?? 0)) : -1;
          comparison = netA - netB;
          break;
        }

        default:
          comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
      }

      return sortOrder === "asc" ? comparison : -comparison;
    });
    return list;
  }, [filteredNodes, sortField, sortOrder, liveMap, nodeStatuses, nodeBlocksMap]);

  return (
    <Box sx={{ p: { xs: 1.5, sm: 2.5, md: 3 }, height: "100%", overflowY: "auto", display: "flex", flexDirection: "column", gap: 2.5 }}>
      {/* Fleet Overview Top Summary Banner */}
      <Grid container spacing={{ xs: 1.5, sm: 2 }}>
        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: { xs: 1.5, sm: 2 },
              borderRadius: 2.5,
              border: "1px solid #E2E8F0",
              bgcolor: "#FFFFFF",
              display: "flex",
              alignItems: "center",
              gap: { xs: 1.2, sm: 2 },
            }}
          >
            <Box
              sx={{
                width: { xs: 38, sm: 44 },
                height: { xs: 38, sm: 44 },
                borderRadius: 2,
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
              }}
            >
              <Server size={20} />
            </Box>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontSize: { xs: "0.65rem", sm: "0.75rem" } }}>
                SERVERS FLEET
              </Typography>
              <Box sx={{ display: "flex", alignItems: "baseline", gap: 0.8 }}>
                <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", fontSize: { xs: "1.2rem", sm: "1.5rem" } }}>
                  {fleetLive?.summary.online_nodes ?? 0}
                </Typography>
                <Typography variant="body2" sx={{ color: "#64748B", fontSize: { xs: "0.7rem", sm: "0.875rem" } }}>
                  / {managedNodes.length} Online
                </Typography>
              </Box>
              <Box sx={{ display: "flex", gap: 0.8, mt: 0.3, alignItems: "center", flexWrap: "wrap" }}>
                <Typography variant="caption" sx={{ color: "#E11D48", fontWeight: 600, fontSize: "0.7rem" }}>
                  {fleetLive?.summary.offline_nodes ?? 0} Offline
                </Typography>
                <Typography variant="caption" sx={{ color: "#CBD5E1", fontSize: "0.7rem" }}>
                  •
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontSize: "0.7rem" }}>
                  {fleetLive?.summary.no_agent_nodes ?? 0} No Agent
                </Typography>
              </Box>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: { xs: 1.5, sm: 2 },
              borderRadius: 2.5,
              border: "1px solid #E2E8F0",
              bgcolor: "#FFFFFF",
              display: "flex",
              alignItems: "center",
              gap: { xs: 1.2, sm: 2 },
            }}
          >
            <Box
              sx={{
                width: { xs: 38, sm: 44 },
                height: { xs: 38, sm: 44 },
                borderRadius: 2,
                bgcolor: "#ECFDF5",
                color: "#059669",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
              }}
            >
              <Activity size={20} />
            </Box>
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontSize: { xs: "0.65rem", sm: "0.75rem" } }}>
                REALTIME TRAFFIC
              </Typography>
              <Box sx={{ display: "flex", flexDirection: { xs: "column", sm: "row" }, alignItems: { xs: "flex-start", sm: "center" }, gap: { xs: 0.2, sm: 1 } }}>
                <Typography variant="body2" sx={{ fontWeight: 700, color: "#059669", display: "flex", alignItems: "center", gap: 0.2, fontSize: { xs: "0.75rem", sm: "0.875rem" } }}>
                  <ArrowDown size={13} /> {formatNetRate(fleetLive?.summary.total_rx_rate)}
                </Typography>
                <Typography variant="body2" sx={{ fontWeight: 700, color: "#0891B2", display: "flex", alignItems: "center", gap: 0.2, fontSize: { xs: "0.75rem", sm: "0.875rem" } }}>
                  <ArrowUp size={13} /> {formatNetRate(fleetLive?.summary.total_tx_rate)}
                </Typography>
              </Box>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: { xs: 1.5, sm: 2 },
              borderRadius: 2.5,
              border: "1px solid #E2E8F0",
              bgcolor: "#FFFFFF",
              display: "flex",
              alignItems: "center",
              gap: { xs: 1.2, sm: 2 },
            }}
          >
            <Box
              sx={{
                width: { xs: 38, sm: 44 },
                height: { xs: 38, sm: 44 },
                borderRadius: 2,
                bgcolor: "#FEF3C7",
                color: "#D97706",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
              }}
            >
              <Cpu size={20} />
            </Box>
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontSize: { xs: "0.65rem", sm: "0.75rem" } }}>
                AVERAGE CPU
              </Typography>
              <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", fontSize: { xs: "1.2rem", sm: "1.5rem" } }}>
                {(fleetLive?.summary.average_cpu ?? 0).toFixed(1)}%
              </Typography>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: { xs: 1.5, sm: 2 },
              borderRadius: 2.5,
              border: "1px solid #E2E8F0",
              bgcolor: "#FFFFFF",
              display: "flex",
              alignItems: "center",
              gap: { xs: 1.2, sm: 2 },
            }}
          >
            <Box
              sx={{
                width: { xs: 38, sm: 44 },
                height: { xs: 38, sm: 44 },
                borderRadius: 2,
                bgcolor: "#F0FDF4",
                color: "#16A34A",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
              }}
            >
              <HardDrive size={20} />
            </Box>
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontSize: { xs: "0.65rem", sm: "0.75rem" } }}>
                AVERAGE MEMORY
              </Typography>
              <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", fontSize: { xs: "1.2rem", sm: "1.5rem" } }}>
                {(fleetLive?.summary.average_mem_perc ?? 0).toFixed(1)}%
              </Typography>
            </Box>
          </Paper>
        </Grid>
      </Grid>

      {/* Control Bar: Search, Block Filter, Tags, View Mode, Refresh */}
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1.5,
          bgcolor: "#FFFFFF",
          p: 1.5,
          borderRadius: 2.5,
          border: "1px solid #E2E8F0",
        }}
      >
        {/* Search input */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flex: { xs: "1 1 100%", sm: "0 1 240px" } }}>
          <TextField
            size="small"
            placeholder="Search servers, IP, tags..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            fullWidth
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={16} color="#94A3B8" />
                </InputAdornment>
              ),
              sx: { borderRadius: 2, bgcolor: "#F8FAFC", fontSize: "0.875rem" },
            }}
          />
        </Box>

        {/* Block Filter Dropdown (rendered only if managed nodes belong to blocks) */}
        {managedBlocks.length > 0 && (
          <FormControl size="small" sx={{ minWidth: { xs: "100%", sm: 150 } }}>
            <Select
              value={managedBlocks.includes(urlBlock) ? urlBlock : "All"}
              onChange={(e) => handleSelectBlock(e.target.value)}
              displayEmpty
              sx={{
                height: 36,
                fontSize: "0.85rem",
                borderRadius: 2,
                bgcolor: "#F8FAFC",
              }}
              startAdornment={
                <InputAdornment position="start" sx={{ mr: 0.5 }}>
                  <Layers size={15} color="#64748B" />
                </InputAdornment>
              }
            >
              <MenuItem value="All">All Blocks</MenuItem>
              {managedBlocks.map((bName) => (
                <MenuItem key={bName} value={bName}>
                  {bName}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        )}

        {/* Tag Filters (rendered only if managed nodes have tags) */}
        {managedTags.length > 0 && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 0.8,
              overflowX: "auto",
              maxWidth: { xs: "100%", md: "420px" },
              pb: { xs: 0.5, md: 0 },
              "-webkit-overflow-scrolling": "touch",
            }}
          >
            {["All", ...managedTags].map((tag) => (
              <Chip
                key={tag}
                label={tag}
                size="small"
                onClick={() => handleSelectTag(tag)}
                sx={{
                  fontWeight: (managedTags.includes(urlTag) ? urlTag : "All") === tag ? 700 : 500,
                  bgcolor: (managedTags.includes(urlTag) ? urlTag : "All") === tag ? "#4F46E5" : "#F1F5F9",
                  color: (managedTags.includes(urlTag) ? urlTag : "All") === tag ? "#FFFFFF" : "#475569",
                  borderRadius: 1.5,
                  "&:hover": {
                    bgcolor: (managedTags.includes(urlTag) ? urlTag : "All") === tag ? "#4338CA" : "#E2E8F0",
                  },
                }}
              />
            ))}
          </Box>
        )}

        {/* Clear filter indicator if filtering */}
        {isFiltering && (
          <Button
            size="small"
            variant="text"
            onClick={handleClearFilters}
            startIcon={<X size={14} />}
            sx={{ color: "#64748B", fontSize: "0.75rem", textTransform: "none", py: 0.4 }}
          >
            Clear ({filteredNodes.length}/{managedNodes.length})
          </Button>
        )}

        {/* Actions & View Toggle */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, ml: "auto" }}>
          <ToggleButtonGroup
            value={urlView}
            exclusive
            onChange={(_, val) => val && handleToggleView(val)}
            size="small"
            sx={{ height: 32 }}
          >
            <ToggleButton value="cards" sx={{ px: 1.5 }}>
              <LayoutGrid size={16} />
            </ToggleButton>
            <ToggleButton value="table" sx={{ px: 1.5 }}>
              <List size={16} />
            </ToggleButton>
          </ToggleButtonGroup>

          <Tooltip title="Refresh live telemetry (flush agent cache)">
            <IconButton
              size="small"
              onClick={() => refreshFleetLive(true)}
              sx={{
                bgcolor: "#F8FAFC",
                border: "1px solid #E2E8F0",
                borderRadius: 2,
                p: 0.8,
              }}
            >
              <RefreshCw size={16} color="#64748B" />
            </IconButton>
          </Tooltip>

          <Button
            size="small"
            variant="outlined"
            onClick={() => handleUpdateState()}
            disabled={updatingState}
            startIcon={<Zap size={14} />}
            sx={{ fontSize: "0.8rem", borderRadius: 2 }}
          >
            Probe All
          </Button>
        </Box>
      </Box>

      {/* Nodes Cards View */}
      {urlView === "cards" && (
        <Grid container spacing={2}>
          {sortedNodes.length === 0 && (
            <Grid item xs={12}>
              <Paper
                elevation={0}
                sx={{
                  p: 4,
                  textAlign: "center",
                  borderRadius: 3,
                  border: "1px dashed #CBD5E1",
                  bgcolor: "#FFFFFF",
                }}
              >
                <Server size={32} color="#94A3B8" />
                <Typography variant="body1" sx={{ mt: 1, fontWeight: 600, color: "#475569" }}>
                  No servers match the selected filters
                </Typography>
                <Typography variant="caption" sx={{ color: "#94A3B8", display: "block", mt: 0.5 }}>
                  Try resetting tags or changing your search criteria.
                </Typography>
                {isFiltering && (
                  <Button size="small" variant="outlined" onClick={handleClearFilters} sx={{ mt: 2, borderRadius: 2 }}>
                    Reset Filters
                  </Button>
                )}
              </Paper>
            </Grid>
          )}

          {sortedNodes.map((n) => {
            const live = liveMap.get(n.name);
            const status = nodeStatuses[n.name];
            const isOnline = Boolean(live?.connected ?? status?.connected);
            const agentInstalled = Boolean(
              live?.agent_installed ?? (
                status?.connected ||
                Boolean(status?.agent_version && status.agent_version.length > 0) ||
                Boolean(status?.metrics && status.metrics.memory_total_bytes > 0)
              )
            );

            const metrics = live?.metrics ?? status?.metrics;
            const cpuPercent = metrics?.cpu_percent ?? 0;
            const memUsedGB = metrics ? metrics.memory_used_bytes / (1024 * 1024 * 1024) : 0;
            const memTotalGB = metrics ? metrics.memory_total_bytes / (1024 * 1024 * 1024) : 0;
            const memPercent = memTotalGB > 0 ? (memUsedGB / memTotalGB) * 100 : 0;
            const disks = status?.metrics?.disks || status?.disks || [];
            const primaryDisk = disks.find((d) => d.path === "/") || disks[0];
            const diskUsedGB = primaryDisk ? primaryDisk.used_bytes / (1024 * 1024 * 1024) : 0;
            const diskTotalGB = primaryDisk ? primaryDisk.total_bytes / (1024 * 1024 * 1024) : 0;
            const diskPercent = diskTotalGB > 0 ? (diskUsedGB / diskTotalGB) * 100 : 0;
            const primaryIface =
              status?.primary_interface ||
              live?.primary_interface ||
              status?.interfaces?.find((i) => ((i.flags ?? 0) & 1) !== 0)?.name;
            const rxRate = live?.metrics?.net_rx_rate ?? 0;
            const txRate = live?.metrics?.net_tx_rate ?? 0;
            const nodeBlocks = nodeBlocksMap.get(n.name) || [];

            return (
              <Grid item xs={12} sm={6} lg={4} xl={3} key={n.name}>
                <Paper
                  elevation={0}
                  sx={{
                    p: 2.2,
                    borderRadius: 3,
                    border: "1px solid #E2E8F0",
                    bgcolor: "#FFFFFF",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.8,
                    transition: "all 0.2s ease-in-out",
                    "&:hover": {
                      boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.08)",
                      borderColor: "#CBD5E1",
                      transform: "translateY(-2px)",
                    },
                  }}
                >
                  {/* Card Header */}
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
                      {/* Pulse Status Dot */}
                      <Box
                        sx={{
                          width: 12,
                          height: 12,
                          borderRadius: "50%",
                          bgcolor: isOnline ? "#10B981" : agentInstalled ? "#EF4444" : "#94A3B8",
                          boxShadow: isOnline ? "0 0 0 3px rgba(16, 185, 129, 0.2)" : "none",
                        }}
                      />
                      <Box>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
                            {n.name}
                          </Typography>
                          {nodeBlocks.map((bName) => (
                            <Chip
                              key={bName}
                              icon={<Layers size={11} />}
                              label={bName}
                              size="small"
                              sx={{
                                height: 18,
                                fontSize: "0.65rem",
                                fontWeight: 600,
                                bgcolor: "#EEF2FF",
                                color: "#4F46E5",
                                "& .MuiChip-icon": { color: "#4F46E5", ml: 0.5 },
                              }}
                            />
                          ))}
                        </Box>
                        <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                          {n.host}
                        </Typography>
                      </Box>
                    </Box>

                    {/* Requirement 2: Differentiated status chip */}
                    <Box sx={{ textAlign: "right" }}>
                      <Chip
                        label={isOnline ? "Online" : agentInstalled ? "Offline" : "No Agent"}
                        size="small"
                        sx={{
                          bgcolor: isOnline ? "#ECFDF5" : agentInstalled ? "#FEF2F2" : "#F1F5F9",
                          color: isOnline ? "#059669" : agentInstalled ? "#E11D48" : "#64748B",
                          fontWeight: 700,
                          fontSize: "0.7rem",
                          height: 22,
                          border: !agentInstalled ? "1px solid #E2E8F0" : "none",
                        }}
                      />
                      <Typography variant="caption" sx={{ display: "block", color: "#94A3B8", mt: 0.3, fontSize: "0.7rem" }}>
                        {isOnline
                          ? formatUptime(live?.uptime_seconds ?? metrics?.uptime_seconds)
                          : agentInstalled
                          ? "Offline"
                          : "Not Monitored"}
                      </Typography>
                    </Box>
                  </Box>

                  {/* Resource Gauges or No Agent notice */}
                  {agentInstalled ? (
                    <>
                      <Box sx={{ display: "flex", flexDirection: "column", gap: 1.2 }}>
                        {/* CPU */}
                        <Box>
                          <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5 }}>
                            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                              CPU
                            </Typography>
                            <Typography variant="caption" sx={{ fontWeight: 700, color: getCpuColor(cpuPercent) }}>
                              {cpuPercent.toFixed(1)}%
                            </Typography>
                          </Box>
                          <LinearProgress
                            variant="determinate"
                            value={Math.min(100, Math.max(0, cpuPercent))}
                            sx={{
                              height: 6,
                              borderRadius: 3,
                              bgcolor: "#F1F5F9",
                              "& .MuiLinearProgress-bar": {
                                bgcolor: getCpuColor(cpuPercent),
                                borderRadius: 3,
                              },
                            }}
                          />
                        </Box>

                        {/* RAM */}
                        <Box>
                          <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5 }}>
                            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                              RAM
                            </Typography>
                            <Typography variant="caption" sx={{ fontWeight: 700, color: "#334155" }}>
                              {memUsedGB.toFixed(1)} / {memTotalGB.toFixed(1)} GB ({memPercent.toFixed(0)}%)
                            </Typography>
                          </Box>
                          <LinearProgress
                            variant="determinate"
                            value={Math.min(100, Math.max(0, memPercent))}
                            sx={{
                              height: 6,
                              borderRadius: 3,
                              bgcolor: "#F1F5F9",
                              "& .MuiLinearProgress-bar": {
                                bgcolor: memPercent > 85 ? "#E11D48" : "#4F46E5",
                                borderRadius: 3,
                              },
                            }}
                          />
                        </Box>

                        {/* Disk */}
                        {primaryDisk && diskTotalGB > 0 && (
                          <Box>
                            <Box sx={{ display: "flex", justifyContent: "space-between", mb: 0.5 }}>
                              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                                Disk ({primaryDisk.path})
                              </Typography>
                              <Typography
                                variant="caption"
                                sx={{
                                  fontWeight: 700,
                                  color: diskPercent > 85 ? "#E11D48" : diskPercent > 70 ? "#F59E0B" : "#334155",
                                }}
                              >
                                {diskUsedGB.toFixed(1)} / {diskTotalGB.toFixed(1)} GB ({diskPercent.toFixed(0)}%)
                              </Typography>
                            </Box>
                            <LinearProgress
                              variant="determinate"
                              value={Math.min(100, Math.max(0, diskPercent))}
                              sx={{
                                height: 6,
                                borderRadius: 3,
                                bgcolor: "#F1F5F9",
                                "& .MuiLinearProgress-bar": {
                                  bgcolor: diskPercent > 85 ? "#E11D48" : diskPercent > 70 ? "#F59E0B" : "#10B981",
                                  borderRadius: 3,
                                },
                              }}
                            />
                          </Box>
                        )}
                      </Box>

                      {/* Network Transfer & Load */}
                      <Box
                        sx={{
                          p: 1.2,
                          bgcolor: "#F8FAFC",
                          borderRadius: 2,
                          border: "1px solid #F1F5F9",
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                        }}
                      >
                        <Tooltip title={primaryIface ? `Primary Interface: ${primaryIface}` : "Primary Interface Bandwidth"}>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                            {primaryIface && (
                              <Chip
                                label={primaryIface}
                                size="small"
                                sx={{
                                  height: 18,
                                  fontSize: "0.62rem",
                                  bgcolor: "#EFF6FF",
                                  color: "#2563EB",
                                  fontWeight: 700,
                                  borderRadius: 1,
                                  "& .MuiChip-label": { px: 0.6 },
                                }}
                              />
                            )}
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.3, color: "#059669" }}>
                              <ArrowDown size={14} />
                              <Typography variant="caption" sx={{ fontWeight: 700 }}>
                                {formatNetRate(rxRate)}
                              </Typography>
                            </Box>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.3, color: "#0891B2" }}>
                              <ArrowUp size={14} />
                              <Typography variant="caption" sx={{ fontWeight: 700 }}>
                                {formatNetRate(txRate)}
                              </Typography>
                            </Box>
                          </Box>
                        </Tooltip>

                        <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
                          Load:{" "}
                          <strong>
                            {live?.metrics?.load_1m !== undefined
                              ? live.metrics.load_1m.toFixed(2)
                              : status?.metrics?.load_avg?.[0] !== undefined
                              ? status.metrics.load_avg[0].toFixed(2)
                              : "-"}
                          </strong>
                        </Typography>
                      </Box>
                    </>
                  ) : (
                    /* Distinct No Agent banner */
                    <Box
                      sx={{
                        p: 2,
                        bgcolor: "#F8FAFC",
                        borderRadius: 2.5,
                        border: "1px dashed #CBD5E1",
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        justifyContent: "center",
                        textAlign: "center",
                        gap: 0.6,
                        my: 0.5,
                        minHeight: 120,
                      }}
                    >
                      <Server size={22} color="#94A3B8" />
                      <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                        Agent Not Installed
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.7rem", maxWidth: 220 }}>
                        Deploy the easy42 agent to stream realtime CPU, memory, and telemetry.
                      </Typography>
                    </Box>
                  )}

                  {/* OS / Agent Info Footer & Actions */}
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", pt: 0.5 }}>
                    <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.7rem" }}>
                      {agentInstalled
                        ? `${live?.os_info || "Linux"} ${live?.agent_version ? `• v${live.agent_version}` : ""}`
                        : "Mesh Managed Node"}
                    </Typography>

                    {/* Actions */}
                    <Box sx={{ display: "flex", gap: 0.8 }}>
                      <Tooltip title={agentInstalled ? "Historical Charts & Telemetry" : "Agent not installed"}>
                        <span>
                          <IconButton
                            size="small"
                            onClick={() => setSelectedMetricsNode(n.name)}
                            disabled={!agentInstalled}
                            sx={{
                              color: "#4F46E5",
                              bgcolor: "#EEF2FF",
                              "&:hover": { bgcolor: "#E0E7FF" },
                              "&.Mui-disabled": { bgcolor: "#F1F5F9", color: "#CBD5E1" },
                              p: 0.7,
                            }}
                          >
                            <Activity size={15} />
                          </IconButton>
                        </span>
                      </Tooltip>

                      <Tooltip title="Looking Glass">
                        <IconButton
                          size="small"
                          onClick={() => navigate(`/looking-glass?node=${encodeURIComponent(n.name)}`)}
                          sx={{
                            color: "#0891B2",
                            bgcolor: "#ECFEFF",
                            "&:hover": { bgcolor: "#CFFAFE" },
                            p: 0.7,
                          }}
                        >
                          <Compass size={15} />
                        </IconButton>
                      </Tooltip>

                      {/* Requirement 1: Opens NodeDetailDrawer */}
                      <Tooltip title="Node Topology Details">
                        <IconButton
                          size="small"
                          onClick={() => setSelectedNode(n)}
                          sx={{
                            color: "#64748B",
                            bgcolor: "#F1F5F9",
                            "&:hover": { bgcolor: "#E2E8F0" },
                            p: 0.7,
                          }}
                        >
                          <Info size={15} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </Box>
                </Paper>
              </Grid>
            );
          })}
        </Grid>
      )}

      {/* Nodes Table View */}
      {urlView === "table" && (
        <TableContainer
          component={Paper}
          elevation={0}
          sx={{
            borderRadius: 3,
            border: "1px solid #E2E8F0",
            overflowX: "auto",
          }}
        >
          <Table size="small">
            <TableHead sx={{ bgcolor: "#F8FAFC" }}>
              <TableRow>
                <TableCell
                  sortDirection={sortField === "name" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "name"}
                    direction={sortField === "name" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("name")}
                  >
                    Node
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "status" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "status"}
                    direction={sortField === "status" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("status")}
                  >
                    Status
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "block" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "block"}
                    direction={sortField === "block" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("block")}
                  >
                    Block
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "host" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "host"}
                    direction={sortField === "host" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("host")}
                  >
                    Host IP
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "uptime" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "uptime"}
                    direction={sortField === "uptime" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("uptime")}
                  >
                    Uptime
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "cpu" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "cpu"}
                    direction={sortField === "cpu" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("cpu")}
                  >
                    CPU
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "memory" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "memory"}
                    direction={sortField === "memory" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("memory")}
                  >
                    Memory
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "disk" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "disk"}
                    direction={sortField === "disk" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("disk")}
                  >
                    Disk
                  </TableSortLabel>
                </TableCell>
                <TableCell
                  sortDirection={sortField === "network" ? sortOrder : false}
                  sx={{ fontWeight: 700, color: "#475569" }}
                >
                  <TableSortLabel
                    active={sortField === "network"}
                    direction={sortField === "network" ? sortOrder : "asc"}
                    onClick={() => handleRequestSort("network")}
                  >
                    Network Rx / Tx
                  </TableSortLabel>
                </TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }} align="right">
                  Actions
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {sortedNodes.length === 0 && (
                <TableRow>
                  <TableCell colSpan={10} align="center" sx={{ py: 4, color: "#64748B" }}>
                    No servers match the selected filters.
                  </TableCell>
                </TableRow>
              )}

              {sortedNodes.map((n) => {
                const live = liveMap.get(n.name);
                const status = nodeStatuses[n.name];
                const isOnline = Boolean(live?.connected ?? status?.connected);
                const agentInstalled = Boolean(
                  live?.agent_installed ?? (
                    status?.connected ||
                    Boolean(status?.agent_version && status.agent_version.length > 0) ||
                    Boolean(status?.metrics && status.metrics.memory_total_bytes > 0)
                  )
                );

                const metrics = live?.metrics ?? status?.metrics;
                const cpuPercent = metrics?.cpu_percent ?? 0;
                const memUsedGB = metrics ? metrics.memory_used_bytes / (1024 * 1024 * 1024) : 0;
                const memTotalGB = metrics ? metrics.memory_total_bytes / (1024 * 1024 * 1024) : 0;
                const disks = status?.metrics?.disks || status?.disks || [];
                const primaryDisk = disks.find((d) => d.path === "/") || disks[0];
                const diskUsedGB = primaryDisk ? primaryDisk.used_bytes / (1024 * 1024 * 1024) : 0;
                const diskTotalGB = primaryDisk ? primaryDisk.total_bytes / (1024 * 1024 * 1024) : 0;
                const diskPercent = diskTotalGB > 0 ? (diskUsedGB / diskTotalGB) * 100 : 0;
                const primaryIface =
                  status?.primary_interface ||
                  live?.primary_interface ||
                  status?.interfaces?.find((i) => ((i.flags ?? 0) & 1) !== 0)?.name;
                const rxRate = live?.metrics?.net_rx_rate ?? 0;
                const txRate = live?.metrics?.net_tx_rate ?? 0;
                const nodeBlocks = nodeBlocksMap.get(n.name) || [];

                return (
                  <TableRow key={n.name} hover>
                    <TableCell>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Box
                          sx={{
                            width: 8,
                            height: 8,
                            borderRadius: "50%",
                            bgcolor: isOnline ? "#10B981" : agentInstalled ? "#EF4444" : "#94A3B8",
                          }}
                        />
                        <Typography variant="body2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                          {n.name}
                        </Typography>
                      </Box>
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={isOnline ? "Online" : agentInstalled ? "Offline" : "No Agent"}
                        size="small"
                        sx={{
                          bgcolor: isOnline ? "#ECFDF5" : agentInstalled ? "#FEF2F2" : "#F1F5F9",
                          color: isOnline ? "#059669" : agentInstalled ? "#E11D48" : "#64748B",
                          fontWeight: 700,
                          fontSize: "0.7rem",
                          height: 20,
                          border: !agentInstalled ? "1px solid #E2E8F0" : "none",
                        }}
                      />
                    </TableCell>
                    <TableCell>
                      {nodeBlocks.length > 0 ? (
                        nodeBlocks.map((b) => (
                          <Chip
                            key={b}
                            label={b}
                            size="small"
                            sx={{ height: 20, fontSize: "0.65rem", fontWeight: 600, bgcolor: "#EEF2FF", color: "#4F46E5", mr: 0.5 }}
                          />
                        ))
                      ) : (
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          —
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell sx={{ color: "#64748B", fontSize: "0.85rem" }}>{n.host}</TableCell>
                    <TableCell sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                      {isOnline
                        ? formatUptime(live?.uptime_seconds ?? metrics?.uptime_seconds)
                        : agentInstalled
                        ? "Offline"
                        : "—"}
                    </TableCell>
                    <TableCell>
                      {agentInstalled ? (
                        <Typography variant="body2" sx={{ fontWeight: 700, color: getCpuColor(cpuPercent) }}>
                          {cpuPercent.toFixed(1)}%
                        </Typography>
                      ) : (
                        <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                          —
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell sx={{ color: "#334155", fontSize: "0.85rem" }}>
                      {agentInstalled && memTotalGB > 0 ? (
                        `${memUsedGB.toFixed(1)} / ${memTotalGB.toFixed(1)} GB`
                      ) : (
                        <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                          —
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell sx={{ color: "#334155", fontSize: "0.85rem" }}>
                      {agentInstalled && primaryDisk && diskTotalGB > 0 ? (
                        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.3, minWidth: 95 }}>
                          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                            <Typography variant="caption" sx={{ fontWeight: 600, color: "#334155", fontSize: "0.75rem" }}>
                              {diskUsedGB.toFixed(1)} / {diskTotalGB.toFixed(1)} GB
                            </Typography>
                            <Typography
                              variant="caption"
                              sx={{
                                fontWeight: 700,
                                fontSize: "0.7rem",
                                color: diskPercent > 85 ? "#E11D48" : diskPercent > 70 ? "#F59E0B" : "#059669",
                              }}
                            >
                              {diskPercent.toFixed(0)}%
                            </Typography>
                          </Box>
                          <LinearProgress
                            variant="determinate"
                            value={Math.min(100, Math.max(0, diskPercent))}
                            sx={{
                              height: 4,
                              borderRadius: 2,
                              bgcolor: "#F1F5F9",
                              "& .MuiLinearProgress-bar": {
                                bgcolor: diskPercent > 85 ? "#E11D48" : diskPercent > 70 ? "#F59E0B" : "#10B981",
                                borderRadius: 2,
                              },
                            }}
                          />
                        </Box>
                      ) : (
                        <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                          —
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell>
                      {agentInstalled ? (
                        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.3 }}>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                            <Typography variant="caption" sx={{ color: "#059669", fontWeight: 700 }}>
                              ↓ {formatNetRate(rxRate)}
                            </Typography>
                            <Typography variant="caption" sx={{ color: "#0891B2", fontWeight: 700 }}>
                              ↑ {formatNetRate(txRate)}
                            </Typography>
                          </Box>
                          {primaryIface && (
                            <Typography variant="caption" className="mono-font" sx={{ color: "#64748B", fontSize: "0.68rem" }}>
                              {primaryIface} (primary)
                            </Typography>
                          )}
                        </Box>
                      ) : (
                        <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                          —
                        </Typography>
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title={agentInstalled ? "Historical Charts" : "Agent not installed"}>
                        <span>
                          <IconButton
                            size="small"
                            onClick={() => setSelectedMetricsNode(n.name)}
                            disabled={!agentInstalled}
                            sx={{
                              color: "#4F46E5",
                              "&.Mui-disabled": { color: "#CBD5E1" },
                            }}
                          >
                            <Activity size={16} />
                          </IconButton>
                        </span>
                      </Tooltip>
                      <Tooltip title="Looking Glass">
                        <IconButton
                          size="small"
                          onClick={() => navigate(`/looking-glass?node=${encodeURIComponent(n.name)}`)}
                          sx={{ color: "#0891B2" }}
                        >
                          <Compass size={16} />
                        </IconButton>
                      </Tooltip>
                      {/* Requirement 1: Opens NodeDetailDrawer */}
                      <Tooltip title="Node Topology Details">
                        <IconButton size="small" onClick={() => setSelectedNode(n)} sx={{ color: "#64748B" }}>
                          <Info size={16} />
                        </IconButton>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {/* Historical Telemetry Charts Drawer */}
      <NodeMetricsDrawer
        open={Boolean(selectedMetricsNode)}
        onClose={() => setSelectedMetricsNode(null)}
        nodeName={selectedMetricsNode}
        nodeLive={selectedNodeLive}
      />

      {/* Requirement 1: Mounted Topology Node Details Drawer with full action handlers */}
      <NodeDetailDrawer
        node={selectedNode}
        status={selectedNode ? nodeStatuses[selectedNode.name] : undefined}
        open={Boolean(selectedNode)}
        onClose={() => setSelectedNode(null)}
        onEditNode={(node) => {
          setNodeToEdit(node);
          setAddNodeOpen(true);
        }}
        onRenameNode={(node) => {
          setNodeToRename(node);
          setRenameModalOpen(true);
        }}
        onNodeDeleted={() => {
          setSelectedNode(null);
          loadData();
          refreshFleetLive();
        }}
        onStatusRefreshed={() => {
          loadData();
          refreshFleetLive();
        }}
        onOpenHelper={(name) => navigate(`/helper?node=${encodeURIComponent(name)}`)}
        onOpenLookingGlass={(name) => navigate(`/looking-glass?node=${encodeURIComponent(name)}`)}
        onUpdateNodeState={(name) => handleUpdateState(name)}
        onSyncNode={(name) => {
          setSyncTargetNode(name);
          setSyncOpen(true);
        }}
        onNodeUpdated={() => {
          loadData();
          refreshFleetLive();
        }}
      />
    </Box>
  );
};
