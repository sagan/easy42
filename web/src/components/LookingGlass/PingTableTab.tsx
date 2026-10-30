import React, { useState, useEffect, useRef, useMemo } from "react";
import {
  Box,
  Typography,
  Button,
  Paper,
  CircularProgress,
  LinearProgress,
  Chip,
  Tooltip,
  IconButton,
  Menu,
  MenuItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  ToggleButton,
  ToggleButtonGroup,
  Alert,
} from "@mui/material";
import {
  Play,
  Square,
  History,
  Trash2,
  Zap,
  Network,
  Download,
  ChevronDown,
  Check,
} from "lucide-react";
import { api } from "../../api/client";
import { Node, PingTableTask, PingTableTaskSummary, PingTableItem } from "../../types/api";
import { ExportPingTableModal } from "./ExportPingTableModal";

interface PingTableTabProps {
  nodes: Node[];
}

export const PingTableTab: React.FC<PingTableTabProps> = ({ nodes }) => {
  const [latencyType, setLatencyType] = useState<"e2e" | "link">("e2e");
  const [currentTask, setCurrentTask] = useState<PingTableTask | null>(null);
  const [loading, setLoading] = useState<boolean>(false);
  const [actionLoading, setActionLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  // History state
  const [historyList, setHistoryList] = useState<PingTableTaskSummary[]>([]);
  const [historyAnchorEl, setHistoryAnchorEl] = useState<null | HTMLElement>(null);

  // In-memory session cache for fetched ping table task details
  const taskCacheRef = useRef<Map<string, PingTableTask>>(new Map());

  // Export modal state
  const [exportModalOpen, setExportModalOpen] = useState<boolean>(false);

  // Managed nodes (internal nodes only)
  const internalNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external).sort((a, b) => a.name.localeCompare(b.name));
  }, [nodes]);

  // Load history list
  const loadHistory = async () => {
    try {
      const list = await api.getPingTableHistory(50);
      setHistoryList(list || []);
      return list || [];
    } catch (err: any) {
      console.error("Failed to load ping table history:", err);
      return [];
    }
  };

  // Filter history to current latency type
  const filteredHistory = useMemo(() => {
    return historyList.filter((item) => item.latency_type === latencyType);
  }, [historyList, latencyType]);

  const isViewingLatest = useMemo(() => {
    if (!currentTask) return true;
    if (currentTask.status === "running") return true;
    if (filteredHistory.length === 0) return true;
    return currentTask.id === filteredHistory[0].id;
  }, [currentTask, filteredHistory]);

  const historyButtonLabel = useMemo(() => {
    if (!currentTask || isViewingLatest) {
      return "Latest";
    }
    return new Date(currentTask.created_at * 1000).toLocaleString([], {
      dateStyle: "short",
      timeStyle: "short",
    });
  }, [currentTask, isViewingLatest]);

  // Load initial task and history on mount
  useEffect(() => {
    let cancelled = false;
    const init = async () => {
      try {
        setLoading(true);
        const [active, history] = await Promise.all([
          api.getCurrentPingTable().catch(() => null),
          api.getPingTableHistory(50).catch(() => []),
        ]);
        if (cancelled) return;
        setHistoryList(history || []);

        let initialType: "e2e" | "link" = "e2e";
        if (active && active.status === "running") {
          initialType = active.latency_type;
          setLatencyType(initialType);
          taskCacheRef.current.set(active.id, active);
          setCurrentTask(active);
        } else {
          // Default to latest task of initial latencyType ("e2e")
          const matching = (history || []).find((h) => h.latency_type === "e2e");
          if (matching) {
            const detail = await api.getPingTableTask(matching.id);
            taskCacheRef.current.set(matching.id, detail);
            if (!cancelled) setCurrentTask(detail);
          } else if (active) {
            initialType = active.latency_type;
            setLatencyType(initialType);
            taskCacheRef.current.set(active.id, active);
            setCurrentTask(active);
          } else if (history && history.length > 0) {
            initialType = history[0].latency_type;
            setLatencyType(initialType);
            const detail = await api.getPingTableTask(history[0].id);
            taskCacheRef.current.set(history[0].id, detail);
            if (!cancelled) setCurrentTask(detail);
          }
        }

        // Prefetch latest task of the other latency type in background for instantaneous switching
        const otherType: "e2e" | "link" = initialType === "e2e" ? "link" : "e2e";
        const otherLatest = (history || []).find((h) => h.latency_type === otherType);
        if (otherLatest && !taskCacheRef.current.has(otherLatest.id)) {
          api.getPingTableTask(otherLatest.id).then((otherDetail) => {
            if (otherDetail && !cancelled) {
              taskCacheRef.current.set(otherLatest.id, otherDetail);
            }
          }).catch(() => {});
        }
      } catch (err: any) {
        if (!cancelled) {
          console.error("Failed to load initial ping table data:", err);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    init();
    return () => {
      cancelled = true;
    };
  }, []);

  // SSE stream for progressive real-time updates when running
  const sseRef = useRef<EventSource | null>(null);
  const isRunning = currentTask?.status === "running";

  useEffect(() => {
    if (!isRunning) {
      if (sseRef.current) {
        sseRef.current.close();
        sseRef.current = null;
      }
      return;
    }

    // Connect to SSE stream
    const eventSource = new EventSource("/api/looking-glass/ping-table/stream");
    sseRef.current = eventSource;

    eventSource.onmessage = (event) => {
      try {
        const updatedTask: PingTableTask = JSON.parse(event.data);
        if (updatedTask && updatedTask.id) {
          taskCacheRef.current.set(updatedTask.id, updatedTask);
          setCurrentTask((prev) => {
            // Only update if it's the same task or more recent
            if (!prev || prev.id === updatedTask.id) {
              return updatedTask;
            }
            return prev;
          });
        }
      } catch (e) {
        console.error("Error parsing SSE ping table event:", e);
      }
    };

    eventSource.onerror = () => {
      // EventSource reconnects automatically
    };

    // Backup polling every 1.5s while running
    const pollInterval = setInterval(async () => {
      try {
        const res = await api.getCurrentPingTable();
        if (res && res.id === currentTask?.id) {
          taskCacheRef.current.set(res.id, res);
          setCurrentTask(res);
        }
      } catch {
        // Ignore polling errors
      }
    }, 1500);

    return () => {
      eventSource.close();
      clearInterval(pollInterval);
      sseRef.current = null;
    };
  }, [isRunning, currentTask?.id]);

  // Handle Run
  const handleRun = async () => {
    try {
      setActionLoading(true);
      setError(null);
      const res = await api.runPingTable(latencyType);
      taskCacheRef.current.set(res.id, res);
      setCurrentTask(res);
      loadHistory();
    } catch (err: any) {
      console.error("Failed to start ping table task:", err);
      setError(err?.message || "Failed to start ping table task");
    } finally {
      setActionLoading(false);
    }
  };

  // Handle Cancel
  const handleCancel = async () => {
    try {
      setActionLoading(true);
      await api.cancelPingTable();
      const res = await api.getCurrentPingTable();
      if (res) {
        taskCacheRef.current.set(res.id, res);
        setCurrentTask(res);
      }
      loadHistory();
    } catch (err: any) {
      console.error("Failed to cancel ping table task:", err);
      setError(err?.message || "Failed to cancel ping table task");
    } finally {
      setActionLoading(false);
    }
  };

  // Handle Latency Type Switch
  const handleTypeChange = async (newType: "e2e" | "link") => {
    if (newType === latencyType) return;
    setLatencyType(newType);
    setError(null);

    // If active running task matches newType, display it directly
    if (currentTask && currentTask.status === "running" && currentTask.latency_type === newType) {
      return;
    }

    // Check if latest task for newType is already cached in memory
    const latestSummary = historyList.find((h) => h.latency_type === newType);
    if (latestSummary && taskCacheRef.current.has(latestSummary.id)) {
      // Instant switch from in-memory cache without spinner
      setCurrentTask(taskCacheRef.current.get(latestSummary.id)!);
      return;
    }

    // Cache miss: fetch from server
    try {
      setLoading(true);
      const history = await api.getPingTableHistory(50).catch(() => []);
      setHistoryList(history || []);

      const active = await api.getCurrentPingTable().catch(() => null);
      if (active && active.status === "running" && active.latency_type === newType) {
        taskCacheRef.current.set(active.id, active);
        setCurrentTask(active);
        return;
      }

      const latest = (history || []).find((h) => h.latency_type === newType);
      if (latest) {
        if (taskCacheRef.current.has(latest.id)) {
          setCurrentTask(taskCacheRef.current.get(latest.id)!);
        } else {
          const detail = await api.getPingTableTask(latest.id);
          taskCacheRef.current.set(latest.id, detail);
          setCurrentTask(detail);
        }
      } else {
        setCurrentTask(null);
      }
    } catch (err: any) {
      console.error("Failed to switch ping table type:", err);
    } finally {
      setLoading(false);
    }
  };

  const handleSelectHistoryItem = async (id: string) => {
    setHistoryAnchorEl(null);

    // Instant switch if task already exists in memory cache
    const cached = taskCacheRef.current.get(id);
    if (cached) {
      setCurrentTask(cached);
      return;
    }

    // Cache miss: load from server
    try {
      setLoading(true);
      const item = await api.getPingTableTask(id);
      if (item) {
        taskCacheRef.current.set(item.id, item);
        setCurrentTask(item);
      }
    } catch (err: any) {
      console.error("Failed to view history task:", err);
      setError(err?.message || "Failed to load historical task");
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteHistoryItem = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    try {
      await api.deletePingTableTask(id);
      taskCacheRef.current.delete(id);
      const updatedList = historyList.filter((item) => item.id !== id);
      setHistoryList(updatedList);
      if (currentTask?.id === id) {
        const remainingForType = updatedList.filter((h) => h.latency_type === latencyType);
        if (remainingForType.length > 0) {
          const nextId = remainingForType[0].id;
          if (taskCacheRef.current.has(nextId)) {
            setCurrentTask(taskCacheRef.current.get(nextId)!);
          } else {
            const nextTask = await api.getPingTableTask(nextId);
            taskCacheRef.current.set(nextId, nextTask);
            setCurrentTask(nextTask);
          }
        } else {
          setCurrentTask(null);
        }
      }
    } catch (err: any) {
      console.error("Failed to delete historical task:", err);
    }
  };


  // Matrix and metrics calculation
  const matrixNodes = currentTask?.nodes && currentTask.nodes.length > 0
    ? currentTask.nodes
    : internalNodes.map((n) => n.name);

  const stats = useMemo(() => {
    if (!currentTask || !currentTask.results) {
      return { totalValid: 0, avgLat: null as number | null, minLat: null as number | null, maxLat: null as number | null };
    }
    const latencies: number[] = [];

    for (const u of matrixNodes) {
      for (const v of matrixNodes) {
        if (u === v) continue;
        const item = currentTask.results[u]?.[v];
        if (item && item.status === "success" && item.latency_ms != null) {
          latencies.push(item.latency_ms);
        }
      }
    }

    if (latencies.length === 0) {
      return { totalValid: 0, avgLat: null, minLat: null, maxLat: null };
    }

    const sum = latencies.reduce((acc, val) => acc + val, 0);
    const avg = sum / latencies.length;
    const min = Math.min(...latencies);
    const max = Math.max(...latencies);

    return {
      totalValid: latencies.length,
      avgLat: avg,
      minLat: min,
      maxLat: max,
    };
  }, [currentTask, matrixNodes]);

  const progressPercent = currentTask?.total_pairs && currentTask.total_pairs > 0
    ? Math.round((currentTask.completed_pairs / currentTask.total_pairs) * 100)
    : 0;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
      {/* 1. Control & Configuration Card */}
      <Paper
        elevation={0}
        sx={{
          p: 2.5,
          borderRadius: 2,
          border: "1px solid #E2E8F0",
          backgroundColor: "#FFFFFF",
          display: "flex",
          flexDirection: "column",
          gap: 2,
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", md: "row" },
            alignItems: { xs: "flex-start", md: "center" },
            justifyContent: "space-between",
            gap: 2,
          }}
        >
          {/* Latency Type Selector */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
              Latency Measurement Type:
            </Typography>
            <ToggleButtonGroup
              value={latencyType}
              exclusive
              disabled={isRunning}
              onChange={(_, val) => {
                if (val) handleTypeChange(val as "e2e" | "link");
              }}
              size="small"
              sx={{
                "& .MuiToggleButton-root": {
                  textTransform: "none",
                  fontWeight: 600,
                  px: 2,
                  py: 0.8,
                  fontSize: "0.85rem",
                  borderColor: "#CBD5E1",
                },
                "& .Mui-selected": {
                  backgroundColor: "#4F46E5 !important",
                  color: "#FFFFFF !important",
                },
              }}
            >
              <ToggleButton value="e2e">
                <Zap size={15} style={{ marginRight: 6 }} />
                End-to-End Latency (Default)
              </ToggleButton>
              <ToggleButton value="link">
                <Network size={15} style={{ marginRight: 6 }} />
                Direct Link Latency
              </ToggleButton>
            </ToggleButtonGroup>
            <Typography variant="caption" sx={{ color: "#64748B", maxWidth: 640 }}>
              {latencyType === "e2e"
                ? "Pings each internal node's main IPv4. Traffic routes across the Easy42 mesh; packets may be dynamically relayed based on link metrics."
                : "Pings directly connected neighbor nodes via link-local private IPv6 address over WireGuard/manual interface. Only direct links are measured."}
            </Typography>
          </Box>

          {/* Action Buttons */}
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap", width: { xs: "100%", md: "auto" } }}>
            {/* History Dropdown Button */}
            <Button
              variant="outlined"
              size="small"
              onClick={(e) => {
                setHistoryAnchorEl(e.currentTarget);
                loadHistory();
              }}
              startIcon={<History size={15} />}
              endIcon={<ChevronDown size={14} />}
              sx={{
                textTransform: "none",
                color: "#334155",
                borderColor: "#CBD5E1",
                fontWeight: 600,
              }}
            >
              {historyButtonLabel}
            </Button>

            {/* History Dropdown Menu */}
            <Menu
              anchorEl={historyAnchorEl}
              open={Boolean(historyAnchorEl)}
              onClose={() => setHistoryAnchorEl(null)}
              PaperProps={{
                sx: {
                  minWidth: 260,
                  maxWidth: 380,
                  maxHeight: 380,
                  borderRadius: 2,
                  boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1)",
                  border: "1px solid #E2E8F0",
                  py: 0.5,
                },
              }}
              transformOrigin={{ horizontal: "left", vertical: "top" }}
              anchorOrigin={{ horizontal: "left", vertical: "bottom" }}
            >
              {filteredHistory.length === 0 ? (
                <MenuItem disabled sx={{ fontSize: "0.82rem", py: 1.5 }}>
                  <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                    No {latencyType === "link" ? "Direct Link" : "End-to-End"} history yet
                  </Typography>
                </MenuItem>
              ) : (
                filteredHistory.map((item, idx) => {
                  const isSelected = currentTask?.id === item.id;
                  const isLatest = idx === 0;
                  const dateText = new Date(item.created_at * 1000).toLocaleString();

                  return (
                    <MenuItem
                      key={item.id}
                      onClick={() => handleSelectHistoryItem(item.id)}
                      selected={isSelected}
                      sx={{
                        py: 0.85,
                        px: 1.5,
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        gap: 1.5,
                        "&.Mui-selected": {
                          backgroundColor: "#EEF2FF",
                          "&:hover": {
                            backgroundColor: "#E0E7FF",
                          },
                        },
                      }}
                    >
                      <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25, minWidth: 0, flex: 1 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                          {isSelected && <Check size={14} color="#4F46E5" />}
                          <Typography
                            sx={{
                              fontSize: "0.82rem",
                              fontWeight: isSelected || isLatest ? 700 : 500,
                              color: isSelected ? "#4F46E5" : "#1E293B",
                              lineHeight: 1.2,
                            }}
                          >
                            {isLatest ? "Latest" : dateText}
                          </Typography>
                          {isLatest && (
                            <Chip
                              label="LATEST"
                              size="small"
                              sx={{
                                height: 16,
                                fontSize: "0.62rem",
                                fontWeight: 700,
                                bgcolor: "#EEF2FF",
                                color: "#4F46E5",
                                border: "1px solid #C7D2FE",
                              }}
                            />
                          )}
                        </Box>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                          <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                            {isLatest ? dateText : `${item.node_count} nodes · ${item.completed_pairs}/${item.total_pairs} pairs`}
                          </Typography>
                          {item.status !== "completed" && (
                            <Chip
                              label={item.status.toUpperCase()}
                              size="small"
                              sx={{
                                height: 15,
                                fontSize: "0.6rem",
                                fontWeight: 700,
                                bgcolor: item.status === "running" ? "#E0F2FE" : "#FEF2F2",
                                color: item.status === "running" ? "#0369A1" : "#B91C1C",
                              }}
                            />
                          )}
                        </Box>
                      </Box>

                      <IconButton
                        size="small"
                        onClick={(e) => handleDeleteHistoryItem(e, item.id)}
                        sx={{
                          color: "#94A3B8",
                          p: 0.5,
                          "&:hover": { color: "#EF4444", bgcolor: "#FEF2F2" },
                        }}
                      >
                        <Trash2 size={13} />
                      </IconButton>
                    </MenuItem>
                  );
                })
              )}
            </Menu>

            {/* Export Diagram Button */}
            {currentTask && (
              <Button
                variant="outlined"
                size="small"
                onClick={() => setExportModalOpen(true)}
                startIcon={<Download size={15} />}
                sx={{
                  textTransform: "none",
                  fontWeight: 600,
                  color: "#0F766E",
                  borderColor: "#99F6E4",
                  bgcolor: "#F0FDFA",
                  "&:hover": {
                    borderColor: "#5EEAD4",
                    bgcolor: "#CCFBF1",
                  },
                }}
              >
                Export Diagram
              </Button>
            )}

            {isRunning ? (
              <Button
                variant="contained"
                color="error"
                size="medium"
                disabled={actionLoading}
                onClick={handleCancel}
                startIcon={actionLoading ? <CircularProgress size={16} color="inherit" /> : <Square size={16} />}
                sx={{
                  textTransform: "none",
                  fontWeight: 600,
                  px: 3,
                  backgroundColor: "#DC2626",
                  "&:hover": { backgroundColor: "#B91C1C" },
                }}
              >
                Cancel Task
              </Button>
            ) : (
              <Button
                variant="contained"
                color="primary"
                size="medium"
                disabled={actionLoading || internalNodes.length < 2}
                onClick={handleRun}
                startIcon={actionLoading ? <CircularProgress size={16} color="inherit" /> : <Play size={16} />}
                sx={{
                  textTransform: "none",
                  fontWeight: 600,
                  px: 3,
                  backgroundColor: "#4F46E5",
                  "&:hover": { backgroundColor: "#4338CA" },
                }}
              >
                Run Ping Table
              </Button>
            )}
          </Box>
        </Box>

        {/* Progress & Status Indicator */}
        {isRunning && (
          <Box sx={{ mt: 1, p: 2, borderRadius: 2, backgroundColor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <CircularProgress size={16} sx={{ color: "#4F46E5" }} />
                <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Measuring cluster ping latencies...
                </Typography>
                <Chip
                  label="RUNNING"
                  size="small"
                  sx={{
                    fontSize: "0.68rem",
                    height: 20,
                    fontWeight: 700,
                    backgroundColor: "#EEF2FF",
                    color: "#4F46E5",
                    border: "1px solid #C7D2FE",
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ fontWeight: 600, color: "#475569" }}>
                {currentTask?.completed_pairs} / {currentTask?.total_pairs} pairs ({progressPercent}%)
              </Typography>
            </Box>
            <LinearProgress
              variant="determinate"
              value={progressPercent}
              sx={{
                height: 8,
                borderRadius: 4,
                backgroundColor: "#E2E8F0",
                "& .MuiLinearProgress-bar": {
                  backgroundColor: "#4F46E5",
                  borderRadius: 4,
                },
              }}
            />
          </Box>
        )}


        {error && (
          <Alert severity="error" onClose={() => setError(null)} sx={{ mt: 1 }}>
            {error}
          </Alert>
        )}

        {!currentTask && !loading && (
          <Alert
            severity="info"
            variant="outlined"
            sx={{
              mt: 1,
              borderRadius: 2,
              backgroundColor: "#F8FAFC",
              borderColor: "#CBD5E1",
              "& .MuiAlert-icon": { color: "#4F46E5" },
            }}
          >
            No ping table measurements recorded yet. Select latency measurement type above and click <strong>Run Ping Table</strong> to measure cluster latencies in real time.
          </Alert>
        )}
      </Paper>

      {/* 2. Metrics & Cluster Summary */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", md: "repeat(4, 1fr)" },
          gap: 2,
        }}
      >
        <Paper elevation={0} sx={{ p: 2, borderRadius: 2, border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 500 }}>
            Cluster Internal Nodes
          </Typography>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
            {matrixNodes.length}
          </Typography>
        </Paper>

        <Paper elevation={0} sx={{ p: 2, borderRadius: 2, border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 500 }}>
            Completed / Total Pairs
          </Typography>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
            {currentTask ? `${currentTask.completed_pairs} / ${currentTask.total_pairs}` : `0 / ${matrixNodes.length >= 2 ? (matrixNodes.length * (matrixNodes.length - 1)) / 2 : 0}`}
          </Typography>
        </Paper>

        <Paper elevation={0} sx={{ p: 2, borderRadius: 2, border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 500 }}>
            Avg Latency (Mesh)
          </Typography>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#4F46E5", mt: 0.5 }}>
            {stats.avgLat != null ? `${stats.avgLat.toFixed(1)} ms` : "—"}
          </Typography>
        </Paper>

        <Paper elevation={0} sx={{ p: 2, borderRadius: 2, border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 500 }}>
            Min / Max Latency
          </Typography>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.5 }}>
            {stats.minLat != null && stats.maxLat != null
              ? `${stats.minLat.toFixed(1)} / ${stats.maxLat.toFixed(1)} ms`
              : "—"}
          </Typography>
        </Paper>
      </Box>

      {/* 3. Two-Dimensional Ping Matrix Table */}
      <Paper
        elevation={0}
        sx={{
          borderRadius: 2,
          border: "1px solid #E2E8F0",
          backgroundColor: "#FFFFFF",
          overflow: "hidden",
        }}
      >
        <Box sx={{ p: 2, borderBottom: "1px solid #E2E8F0", display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
              {currentTask?.latency_type === "link" ? "Direct Link Latency Matrix" : "End-to-End Latency Matrix"}
            </Typography>
            {currentTask && (
              <Chip
                label={currentTask.status.toUpperCase()}
                size="small"
                color={
                  currentTask.status === "completed"
                    ? "success"
                    : currentTask.status === "running"
                    ? "primary"
                    : currentTask.status === "cancelled"
                    ? "warning"
                    : "default"
                }
                variant="outlined"
                sx={{ height: 22, fontSize: "0.72rem", fontWeight: 700 }}
              />
            )}
          </Box>

          {/* Color Scale Legend */}
          <Box sx={{ display: { xs: "none", lg: "flex" }, alignItems: "center", gap: 1.5 }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              Legend:
            </Typography>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Box sx={{ width: 12, height: 12, borderRadius: 0.5, bgcolor: "#ECFDF5", border: "1px solid #A7F3D0" }} />
              <Typography variant="caption" sx={{ color: "#065F46", fontSize: "0.72rem" }}>&lt;30ms</Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Box sx={{ width: 12, height: 12, borderRadius: 0.5, bgcolor: "#F0F9FF", border: "1px solid #BAE6FD" }} />
              <Typography variant="caption" sx={{ color: "#0369A1", fontSize: "0.72rem" }}>30-70ms</Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Box sx={{ width: 12, height: 12, borderRadius: 0.5, bgcolor: "#FFFBEB", border: "1px solid #FDE68A" }} />
              <Typography variant="caption" sx={{ color: "#B45309", fontSize: "0.72rem" }}>70-150ms</Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Box sx={{ width: 12, height: 12, borderRadius: 0.5, bgcolor: "#FEF2F2", border: "1px solid #FECACA" }} />
              <Typography variant="caption" sx={{ color: "#B91C1C", fontSize: "0.72rem" }}>&gt;150ms</Typography>
            </Box>
          </Box>
        </Box>

        {loading ? (
          <Box sx={{ p: 8, display: "flex", justifyContent: "center", alignItems: "center" }}>
            <CircularProgress size={32} />
          </Box>
        ) : matrixNodes.length < 2 ? (
          <Box sx={{ p: 6, textAlign: "center" }}>
            <Typography variant="body2" sx={{ color: "#64748B" }}>
              At least 2 internal nodes are required to measure a ping table matrix.
            </Typography>
          </Box>
        ) : (
          <TableContainer sx={{ maxHeight: 600, overflowX: "auto" }}>
            <Table stickyHeader size="small">
              <TableHead>
                <TableRow>
                  <TableCell
                    sx={{
                      backgroundColor: "#F8FAFC",
                      fontWeight: 700,
                      color: "#475569",
                      fontSize: "0.8rem",
                      zIndex: 3,
                      position: "sticky",
                      left: 0,
                      minWidth: 120,
                      borderRight: "2px solid #E2E8F0",
                    }}
                  >
                    From \ To
                  </TableCell>
                  {matrixNodes.map((nodeName) => (
                    <TableCell
                      key={nodeName}
                      align="center"
                      sx={{
                        backgroundColor: "#F8FAFC",
                        fontWeight: 700,
                        color: "#1E293B",
                        fontSize: "0.8rem",
                        minWidth: 100,
                        whiteSpace: "nowrap",
                        borderRight: "1px solid #E2E8F0",
                      }}
                    >
                      {nodeName}
                    </TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {matrixNodes.map((rowNode) => (
                  <TableRow key={rowNode} hover>
                    {/* Sticky Row Header */}
                    <TableCell
                      sx={{
                        backgroundColor: "#F8FAFC",
                        fontWeight: 700,
                        color: "#1E293B",
                        fontSize: "0.8rem",
                        position: "sticky",
                        left: 0,
                        zIndex: 2,
                        borderRight: "2px solid #E2E8F0",
                        whiteSpace: "nowrap",
                      }}
                    >
                      {rowNode}
                    </TableCell>

                    {/* Matrix Cells */}
                    {matrixNodes.map((colNode) => {
                      if (rowNode === colNode) {
                        return (
                          <TableCell
                            key={colNode}
                            align="center"
                            sx={{
                              backgroundColor: "#F1F5F9",
                              color: "#94A3B8",
                              fontWeight: 600,
                              fontSize: "0.85rem",
                              borderRight: "1px solid #E2E8F0",
                            }}
                          >
                            —
                          </TableCell>
                        );
                      }

                      const item: PingTableItem | undefined = currentTask?.results?.[rowNode]?.[colNode];

                      return (
                        <MatrixCell
                          key={colNode}
                          fromNode={rowNode}
                          toNode={colNode}
                          item={item}
                          latencyType={currentTask?.latency_type || latencyType}
                          isRunning={isRunning}
                        />
                      );
                    })}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Paper>

      {/* Export Diagram Modal */}
      <ExportPingTableModal
        open={exportModalOpen}
        onClose={() => setExportModalOpen(false)}
        task={currentTask}
        nodes={nodes}
      />
    </Box>
  );
};

interface MatrixCellProps {
  fromNode: string;
  toNode: string;
  item?: PingTableItem;
  latencyType: "e2e" | "link";
  isRunning: boolean;
}

const MatrixCell: React.FC<MatrixCellProps> = ({ fromNode, toNode, item, latencyType, isRunning }) => {
  if (!item || item.status === "pending") {
    return (
      <TableCell
        align="center"
        sx={{
          backgroundColor: "#FFFFFF",
          color: "#94A3B8",
          fontSize: "0.85rem",
          fontWeight: 500,
          borderRight: "1px solid #E2E8F0",
        }}
      >
        {isRunning ? (
          <CircularProgress size={12} sx={{ color: "#4F46E5" }} />
        ) : (
          "—"
        )}
      </TableCell>
    );
  }

  let bg = "#FFFFFF";
  let color = "#1E293B";
  let text = "—";
  let tooltipTitle = `${fromNode} → ${toNode}`;

  if (item.status === "success" && item.latency_ms != null) {
    const lat = item.latency_ms;
    text = `${lat.toFixed(1)} ms`;
    if (lat < 30) {
      bg = "#ECFDF5";
      color = "#065F46";
    } else if (lat < 70) {
      bg = "#F0F9FF";
      color = "#0369A1";
    } else if (lat < 150) {
      bg = "#FFFBEB";
      color = "#B45309";
    } else {
      bg = "#FEF2F2";
      color = "#B91C1C";
    }
  } else if (item.status === "no_link") {
    bg = "#F8FAFC";
    color = "#94A3B8";
    text = "N/A";
  } else if (item.status === "no_agent") {
    bg = "#FFFBEB";
    color = "#D97706";
    text = "No agent";
  } else if (item.status === "unreachable" || item.status === "failed") {
    bg = "#FEF2F2";
    color = "#DC2626";
    text = "Timeout";
  }

  const tooltipContent = (
    <Box sx={{ p: 0.5, fontSize: "0.75rem" }}>
      <Typography variant="caption" sx={{ fontWeight: 700, display: "block", color: "#FFFFFF" }}>
        {tooltipTitle}
      </Typography>
      <Box sx={{ mt: 0.5, color: "#E2E8F0" }}>
        <div>Type: <strong>{latencyType === "link" ? "Direct Link" : "End-to-End"}</strong></div>
        <div>Status: <strong>{item.status}</strong></div>
        {item.latency_ms != null && <div>Latency: <strong>{item.latency_ms.toFixed(2)} ms</strong></div>}
        {item.packet_loss_pct != null && <div>Packet Loss: <strong>{item.packet_loss_pct}%</strong></div>}
        {item.target_ip && <div>Target IP: <code>{item.target_ip}</code></div>}
        {item.interface && <div>Interface: <code>{item.interface}</code></div>}
        {item.from_node && <div>Measured From: <strong>{item.from_node}</strong></div>}
        {item.error && <div style={{ color: "#FCA5A5", marginTop: 4 }}>Error: {item.error}</div>}
      </Box>
    </Box>
  );

  return (
    <Tooltip title={tooltipContent} arrow placement="top">
      <TableCell
        align="center"
        sx={{
          backgroundColor: bg,
          color: color,
          fontWeight: 600,
          fontSize: "0.8rem",
          cursor: "default",
          borderRight: "1px solid #E2E8F0",
          transition: "background-color 0.2s ease",
          "&:hover": {
            filter: "brightness(0.96)",
          },
        }}
      >
        {text}
      </TableCell>
    </Tooltip>
  );
};
