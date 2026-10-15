import React, { useState, useEffect, useMemo } from "react";
import {
  Box,
  Typography,
  Paper,
  Button,
  ButtonGroup,
  Chip,
  Alert,
  CircularProgress,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  TableContainer,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Grid,
} from "@mui/material";
import {
  Clock,
  Activity,
  RefreshCw,
  Terminal,
  CheckCircle2,
  AlertTriangle,
  Copy,
  ExternalLink,
  ArrowUpRight,
} from "lucide-react";
import { Link as RouterLink, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import type { CronTaskResultRecord, NodeCronTasksResultsResponse } from "../../types/api";

interface NodeCronTasksTabProps {
  nodeName: string;
}

export const NodeCronTasksTab: React.FC<NodeCronTasksTabProps> = ({ nodeName }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [range, setRange] = useState<string>("24h");
  const [selectedTask, setSelectedTask] = useState<string>(
    () => searchParams.get("task") || searchParams.get("taskId") || "",
  );
  const [viewMode, setViewMode] = useState<"both" | "charts" | "table">("both");

  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<NodeCronTasksResultsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Raw output modal state
  const [inspectRecord, setInspectRecord] = useState<CronTaskResultRecord | null>(null);
  const [copied, setCopied] = useState(false);

  // Sync selectedTask if URL query params change (e.g. browser back/forward or external navigation)
  useEffect(() => {
    const paramTask = searchParams.get("task") || searchParams.get("taskId") || "";
    if (paramTask !== selectedTask) {
      setSelectedTask(paramTask);
    }
  }, [searchParams]);

  const handleTaskChange = (taskId: string) => {
    setSelectedTask(taskId);
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (taskId) {
          next.set("task", taskId);
        } else {
          next.delete("task");
          next.delete("taskId");
        }
        return next;
      },
      { replace: true },
    );
  };

  const fetchData = async () => {
    setLoading(true);
    try {
      const res = await api.getNodeCronTasksResults(nodeName, selectedTask || undefined, range, 200);
      setData(res);
      setError(null);
    } catch (err: any) {
      setError(err?.message || "Failed to load cron task results");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [nodeName, range, selectedTask]);

  const results = data?.results || [];
  const assignedTasks = data?.tasks || [];

  // Overall KPIs
  const overallKPIs = useMemo(() => {
    if (results.length === 0) {
      return { totalRuns: 0, avgRTT: 0, avgLoss: 0, successCount: 0 };
    }
    let sumRTT = 0;
    let sumLoss = 0;
    let success = 0;
    results.forEach((r) => {
      sumRTT += r.avg_rtt_ms;
      sumLoss += r.packet_loss_pct;
      if (r.exit_code === 0 && r.packet_loss_pct < 100) {
        success++;
      }
    });
    return {
      totalRuns: results.length,
      avgRTT: sumRTT / results.length,
      avgLoss: sumLoss / results.length,
      successCount: success,
    };
  }, [results]);

  // Chronological data sorted by time ASC for charts
  const chartPoints = useMemo(() => {
    return [...results].sort((a, b) => a.executed_at - b.executed_at);
  }, [results]);

  // Copy raw output
  const handleCopyRaw = () => {
    if (inspectRecord?.raw_output) {
      navigator.clipboard.writeText(inspectRecord.raw_output);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Top Filter and Controls Bar */}
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
        <Box sx={{ display: "flex", alignItems: "center", gap: 2, flexWrap: "wrap" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Clock size={20} color="#4F46E5" />
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
              Cron Probes on {nodeName}
            </Typography>
          </Box>

          {/* Task Filter */}
          <FormControl size="small" sx={{ minWidth: 200 }}>
            <InputLabel id="task-filter-select-label">Filter by Task</InputLabel>
            <Select
              labelId="task-filter-select-label"
              value={selectedTask}
              label="Filter by Task"
              onChange={(e) => handleTaskChange(e.target.value)}
              sx={{ bgcolor: "#F8FAFC" }}
            >
              <MenuItem value="">
                <em>All Tasks ({assignedTasks.length})</em>
              </MenuItem>
              {assignedTasks.map((t) => (
                <MenuItem key={t.id} value={t.id}>
                  {t.name} ({t.task_type === "tcp_ping" ? "TCP" : "ICMP"})
                </MenuItem>
              ))}
              {selectedTask && !assignedTasks.some((t) => t.id === selectedTask) && (
                <MenuItem value={selectedTask}>
                  <em>Task ({selectedTask})</em>
                </MenuItem>
              )}
            </Select>
          </FormControl>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          {/* View mode buttons */}
          <ButtonGroup size="small" variant="outlined" sx={{ bgcolor: "#F8FAFC" }}>
            <Button
              onClick={() => setViewMode("both")}
              sx={{
                fontWeight: viewMode === "both" ? 700 : 500,
                bgcolor: viewMode === "both" ? "#EEF2FF" : "transparent",
                color: viewMode === "both" ? "#4F46E5" : "#64748B",
                borderColor: "#CBD5E1",
              }}
            >
              Both
            </Button>
            <Button
              onClick={() => setViewMode("charts")}
              sx={{
                fontWeight: viewMode === "charts" ? 700 : 500,
                bgcolor: viewMode === "charts" ? "#EEF2FF" : "transparent",
                color: viewMode === "charts" ? "#4F46E5" : "#64748B",
                borderColor: "#CBD5E1",
              }}
            >
              Charts
            </Button>
            <Button
              onClick={() => setViewMode("table")}
              sx={{
                fontWeight: viewMode === "table" ? 700 : 500,
                bgcolor: viewMode === "table" ? "#EEF2FF" : "transparent",
                color: viewMode === "table" ? "#4F46E5" : "#64748B",
                borderColor: "#CBD5E1",
              }}
            >
              Table
            </Button>
          </ButtonGroup>

          {/* Time range buttons */}
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

          <Button
            size="small"
            variant="outlined"
            startIcon={<RefreshCw size={14} className={loading ? "spin" : ""} />}
            onClick={fetchData}
            disabled={loading}
            sx={{ borderColor: "#CBD5E1", color: "#475569" }}
          >
            Refresh
          </Button>

          <Button
            component={RouterLink}
            to="/looking-glass/cron"
            size="small"
            variant="outlined"
            startIcon={<ExternalLink size={14} />}
            sx={{ borderColor: "#CBD5E1", color: "#475569" }}
          >
            Manage Tasks
          </Button>
        </Box>
      </Paper>

      {error && (
        <Alert severity="error" onClose={() => setError(null)} sx={{ borderRadius: 2.5 }}>
          {error}
        </Alert>
      )}

      {/* KPI Cards */}
      <Grid container spacing={2}>
        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: 2.5,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
              display: "flex",
              alignItems: "center",
              gap: 2,
            }}
          >
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Clock size={20} />
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                ASSIGNED TASKS
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                {assignedTasks.length}
              </Typography>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: 2.5,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
              display: "flex",
              alignItems: "center",
              gap: 2,
            }}
          >
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: "#ECFDF5",
                color: "#059669",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Activity size={20} />
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                PROBE RUNS ({range})
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                {overallKPIs.totalRuns}
              </Typography>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: 2.5,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
              display: "flex",
              alignItems: "center",
              gap: 2,
            }}
          >
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: "#F0F9FF",
                color: "#0284C7",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <ArrowUpRight size={20} />
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                AVG LATENCY (RTT)
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                {results.length > 0 ? `${overallKPIs.avgRTT.toFixed(2)} ms` : "—"}
              </Typography>
            </Box>
          </Paper>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: 2.5,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
              display: "flex",
              alignItems: "center",
              gap: 2,
            }}
          >
            <Box
              sx={{
                p: 1.25,
                borderRadius: 2,
                bgcolor: overallKPIs.avgLoss === 0 ? "#F0FDF4" : "#FEF2F2",
                color: overallKPIs.avgLoss === 0 ? "#16A34A" : "#DC2626",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              {overallKPIs.avgLoss === 0 ? <CheckCircle2 size={20} /> : <AlertTriangle size={20} />}
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                AVG PACKET LOSS
              </Typography>
              <Typography
                variant="h6"
                sx={{
                  fontWeight: 800,
                  color: overallKPIs.avgLoss === 0 ? "#16A34A" : "#DC2626",
                  lineHeight: 1.2,
                }}
              >
                {results.length > 0 ? `${overallKPIs.avgLoss.toFixed(1)}%` : "—"}
              </Typography>
            </Box>
          </Paper>
        </Grid>
      </Grid>

      {/* Main Body */}
      {loading && results.length === 0 ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}>
          <CircularProgress size={36} />
        </Box>
      ) : results.length === 0 ? (
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
          <Clock size={40} color="#94A3B8" style={{ marginBottom: 12 }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#334155" }}>
            No Cron Task Results Recorded for {nodeName}
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", mt: 0.5, maxWidth: 500, mx: "auto", mb: 2 }}>
            {assignedTasks.length === 0
              ? `There are currently no cron tasks assigned to node "${nodeName}". Visit Looking Glass -> Cron Tasks to assign recurring ping tests.`
              : `Cron tasks are assigned to this node, but no reports have been collected in the past ${range}. Verify that easy42-agent is running on the node.`}
          </Typography>
          <Button
            component={RouterLink}
            to="/looking-glass/cron"
            variant="contained"
            sx={{ bgcolor: "#4F46E5", textTransform: "none", fontWeight: 600 }}
          >
            Configure Cron Tasks in Looking Glass
          </Button>
        </Paper>
      ) : (
        <>
          {/* Charts Section */}
          {viewMode !== "table" && (
            <Grid container spacing={2.5}>
              {/* Latency Chart */}
              <Grid item xs={12} lg={6}>
                <CronLatencyChart
                  title="Ping / TCP Handshake Latency (RTT ms)"
                  points={chartPoints}
                  color="#4F46E5"
                  color2="#0891B2"
                />
              </Grid>

              {/* Packet Loss Chart */}
              <Grid item xs={12} lg={6}>
                <CronPacketLossChart title="Packet Loss Rate (%)" points={chartPoints} color="#DC2626" />
              </Grid>
            </Grid>
          )}

          {/* Results Table Section */}
          {viewMode !== "charts" && (
            <Paper
              elevation={0}
              sx={{
                borderRadius: 3,
                border: "1px solid #E2E8F0",
                bgcolor: "#FFFFFF",
                overflow: "hidden",
              }}
            >
              <Box
                sx={{
                  p: 2,
                  bgcolor: "#F8FAFC",
                  borderBottom: "1px solid #E2E8F0",
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                  Execution History ({results.length} samples)
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Chronological logs from agent probes
                </Typography>
              </Box>

              <TableContainer sx={{ maxHeight: 500 }}>
                <Table stickyHeader size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Time</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Task</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Target</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Packets</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Loss %</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Avg RTT</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Min / Max</TableCell>
                      <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Status</TableCell>
                      <TableCell align="right" sx={{ fontWeight: 700, color: "#475569" }}>
                        Raw Output
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {results.map((r) => {
                      const isSuccess = r.exit_code === 0 && r.packet_loss_pct < 100;
                      return (
                        <TableRow key={r.id} hover>
                          <TableCell sx={{ whiteSpace: "nowrap", color: "#64748B", fontSize: "0.8rem" }}>
                            {new Date(r.executed_at * 1000).toISOString()}
                          </TableCell>
                          <TableCell>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                              <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                                {r.task_name}
                              </Typography>
                              <Chip
                                size="small"
                                label={r.task_type === "tcp_ping" ? "TCP" : "ICMP"}
                                sx={{
                                  fontSize: "0.65rem",
                                  height: 18,
                                  bgcolor: r.task_type === "tcp_ping" ? "#F5F3FF" : "#EFF6FF",
                                  color: r.task_type === "tcp_ping" ? "#7C3AED" : "#2563EB",
                                }}
                              />
                            </Box>
                          </TableCell>
                          <TableCell sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#334155" }}>
                            {r.target || "—"}
                          </TableCell>
                          <TableCell sx={{ fontSize: "0.8rem" }}>
                            {r.packets_received} / {r.packets_sent}
                          </TableCell>
                          <TableCell>
                            <Chip
                              size="small"
                              label={`${r.packet_loss_pct.toFixed(0)}%`}
                              sx={{
                                fontSize: "0.75rem",
                                fontWeight: 700,
                                height: 22,
                                bgcolor: r.packet_loss_pct === 0 ? "#F0FDF4" : "#FEF2F2",
                                color: r.packet_loss_pct === 0 ? "#16A34A" : "#DC2626",
                                border: `1px solid ${r.packet_loss_pct === 0 ? "#BBF7D0" : "#FECACA"}`,
                              }}
                            />
                          </TableCell>
                          <TableCell sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.8rem" }}>
                            {r.avg_rtt_ms > 0 ? `${r.avg_rtt_ms.toFixed(2)} ms` : "—"}
                          </TableCell>
                          <TableCell sx={{ color: "#64748B", fontSize: "0.75rem" }}>
                            {r.min_rtt_ms > 0 ? `${r.min_rtt_ms.toFixed(1)} / ${r.max_rtt_ms.toFixed(1)} ms` : "—"}
                          </TableCell>
                          <TableCell>
                            {isSuccess ? (
                              <Chip
                                size="small"
                                label="OK"
                                sx={{
                                  bgcolor: "#F0FDF4",
                                  color: "#16A34A",
                                  fontWeight: 700,
                                  fontSize: "0.7rem",
                                  height: 20,
                                }}
                              />
                            ) : (
                              <Chip
                                size="small"
                                label={r.error ? "Error" : "Fail"}
                                sx={{
                                  bgcolor: "#FEF2F2",
                                  color: "#DC2626",
                                  fontWeight: 700,
                                  fontSize: "0.7rem",
                                  height: 20,
                                }}
                              />
                            )}
                          </TableCell>
                          <TableCell align="right">
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<Terminal size={12} />}
                              onClick={() => setInspectRecord(r)}
                              sx={{
                                textTransform: "none",
                                fontSize: "0.75rem",
                                py: 0.25,
                                px: 1,
                                borderColor: "#CBD5E1",
                                color: "#475569",
                              }}
                            >
                              View Output
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
            </Paper>
          )}
        </>
      )}

      {/* Raw Output Inspection Modal */}
      <Dialog
        open={Boolean(inspectRecord)}
        onClose={() => setInspectRecord(null)}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { borderRadius: 3, p: 1 } }}
      >
        <DialogTitle
          sx={{
            fontWeight: 700,
            color: "#0F172A",
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Terminal size={20} color="#4F46E5" />
            <span>
              {inspectRecord?.task_name} Output ({inspectRecord?.target})
            </span>
          </Box>
          <Button size="small" startIcon={<Copy size={14} />} onClick={handleCopyRaw} sx={{ textTransform: "none" }}>
            {copied ? "Copied!" : "Copy"}
          </Button>
        </DialogTitle>
        <DialogContent>
          <Box sx={{ display: "flex", gap: 2, mb: 1.5, flexWrap: "wrap" }}>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Executed: <strong>{inspectRecord ? new Date(inspectRecord.executed_at * 1000).toISOString() : ""}</strong>
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Duration: <strong>{inspectRecord?.duration_ms} ms</strong>
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Exit code: <strong>{inspectRecord?.exit_code}</strong>
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Loss: <strong>{inspectRecord?.packet_loss_pct}%</strong>
            </Typography>
          </Box>

          <Paper
            elevation={0}
            sx={{
              p: 2,
              bgcolor: "#0F172A",
              color: "#F8FAFC",
              borderRadius: 2,
              fontFamily: "monospace",
              fontSize: "0.85rem",
              whiteSpace: "pre-wrap",
              wordBreak: "break-all",
              maxHeight: 380,
              overflowY: "auto",
            }}
          >
            {inspectRecord?.raw_output || inspectRecord?.error || "No stdout produced"}
          </Paper>
        </DialogContent>
        <DialogActions sx={{ p: 2, pt: 1 }}>
          <Button onClick={() => setInspectRecord(null)} sx={{ color: "#475569" }}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

// -------------------------------------------------------------
// Interactive SVG Chart: Latency (RTT ms) over Time
// -------------------------------------------------------------

interface CronLatencyChartProps {
  title: string;
  points: CronTaskResultRecord[];
  color?: string;
  color2?: string;
}

const CronLatencyChart: React.FC<CronLatencyChartProps> = ({
  title,
  points,
  color = "#4F46E5",
  color2 = "#0891B2",
}) => {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

  const stats = useMemo(() => {
    if (points.length === 0) return { min: 0, max: 10, avg: 0, current: 0 };
    let min = Infinity;
    let max = -Infinity;
    let sum = 0;
    points.forEach((p) => {
      if (p.avg_rtt_ms < min) min = p.avg_rtt_ms;
      if (p.max_rtt_ms > max) max = p.max_rtt_ms;
      sum += p.avg_rtt_ms;
    });
    if (min === Infinity) min = 0;
    if (max === -Infinity || max === 0) max = 10;
    max = max * 1.15; // padding top
    return {
      min,
      max,
      avg: sum / points.length,
      current: points[points.length - 1].avg_rtt_ms,
    };
  }, [points]);

  const height = 200;
  const padding = { top: 20, right: 15, bottom: 25, left: 15 };
  const width = 500;
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;

  const pointsAvg = useMemo(() => {
    if (points.length === 0) return "";
    return points
      .map((d, i) => {
        const x = padding.left + (i / Math.max(1, points.length - 1)) * plotWidth;
        const normY = (d.avg_rtt_ms - 0) / Math.max(0.1, stats.max);
        const y = padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
        return `${x},${y}`;
      })
      .join(" ");
  }, [points, plotWidth, plotHeight, stats.max]);

  const pointsMax = useMemo(() => {
    if (points.length === 0) return "";
    return points
      .map((d, i) => {
        const x = padding.left + (i / Math.max(1, points.length - 1)) * plotWidth;
        const normY = (d.max_rtt_ms - 0) / Math.max(0.1, stats.max);
        const y = padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
        return `${x},${y}`;
      })
      .join(" ");
  }, [points, plotWidth, plotHeight, stats.max]);

  const areaAvg = useMemo(() => {
    if (!pointsAvg || points.length === 0) return "";
    const startX = padding.left;
    const endX = padding.left + plotWidth;
    const bottomY = padding.top + plotHeight;
    return `M ${startX},${bottomY} L ${pointsAvg.replace(/ /g, " L ")} L ${endX},${bottomY} Z`;
  }, [pointsAvg, points.length, plotWidth, plotHeight]);

  const handlePointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (points.length === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    const index = Math.round(ratio * (points.length - 1));
    setHoverIndex(index);
  };

  const activePoint = hoverIndex !== null && points[hoverIndex] ? points[hoverIndex] : null;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 2,
        borderRadius: 2.5,
        border: "1px solid #E2E8F0",
        bgcolor: "#FFFFFF",
      }}
    >
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1 }}>
        <Box>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, textTransform: "uppercase" }}>
            {title}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "baseline", gap: 1 }}>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", fontSize: "1.2rem" }}>
              {activePoint ? `${activePoint.avg_rtt_ms.toFixed(2)} ms` : `${stats.current.toFixed(2)} ms`}
            </Typography>
            {activePoint && (
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                (min: {activePoint.min_rtt_ms.toFixed(1)} / max: {activePoint.max_rtt_ms.toFixed(1)} ms)
              </Typography>
            )}
          </Box>
        </Box>

        <Box sx={{ textAlign: "right" }}>
          {activePoint ? (
            <Typography variant="caption" sx={{ color: "#475569", fontWeight: 600 }}>
              {new Date(activePoint.executed_at * 1000).toISOString()}
            </Typography>
          ) : (
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Avg: <strong>{stats.avg.toFixed(2)} ms</strong>
            </Typography>
          )}
        </Box>
      </Box>

      {/* SVG Container */}
      <Box sx={{ width: "100%", height, touchAction: "none", cursor: "crosshair" }}>
        <svg
          viewBox={`0 0 ${width} ${height}`}
          style={{ width: "100%", height: "100%", display: "block" }}
          onPointerMove={handlePointerMove}
          onPointerLeave={() => setHoverIndex(null)}
        >
          <defs>
            <linearGradient id="grad-cron-latency" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.25" />
              <stop offset="100%" stopColor={color} stopOpacity="0.0" />
            </linearGradient>
          </defs>

          {/* Grid lines */}
          {[0, 0.5, 1].map((pct) => {
            const y = padding.top + plotHeight * (1 - pct);
            const val = pct * stats.max;
            return (
              <g key={pct}>
                <line
                  x1={padding.left}
                  y1={y}
                  x2={padding.left + plotWidth}
                  y2={y}
                  stroke="#F1F5F9"
                  strokeWidth="1"
                  strokeDasharray="4 4"
                />
                {pct > 0 && (
                  <text x={padding.left + 4} y={y - 3} fontSize="9" fill="#94A3B8" fontFamily="sans-serif">
                    {val.toFixed(1)} ms
                  </text>
                )}
              </g>
            );
          })}

          {/* Area fill */}
          {areaAvg && <path d={areaAvg} fill="url(#grad-cron-latency)" />}

          {/* Max RTT Line (dashed cyan) */}
          {pointsMax && (
            <polyline
              fill="none"
              stroke={color2}
              strokeWidth="1.5"
              strokeDasharray="3 3"
              strokeLinecap="round"
              strokeLinejoin="round"
              points={pointsMax}
            />
          )}

          {/* Avg RTT Line (solid indigo) */}
          {pointsAvg && (
            <polyline
              fill="none"
              stroke={color}
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
              points={pointsAvg}
            />
          )}

          {/* Hover crosshair */}
          {hoverIndex !== null && points[hoverIndex] && (
            <g>
              <line
                x1={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                y1={padding.top}
                x2={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                y2={padding.top + plotHeight}
                stroke="#94A3B8"
                strokeWidth="1.2"
                strokeDasharray="2 2"
              />
              <circle
                cx={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                cy={
                  padding.top +
                  plotHeight -
                  Math.max(0, Math.min(plotHeight, (points[hoverIndex].avg_rtt_ms / stats.max) * plotHeight))
                }
                r="4"
                fill="#FFFFFF"
                stroke={color}
                strokeWidth="2.5"
              />
            </g>
          )}

          {/* Time axis */}
          {points.length > 1 && (
            <>
              <text x={padding.left} y={height - 5} fontSize="9" fill="#94A3B8" fontFamily="sans-serif">
                {new Date(points[0].executed_at * 1000).toISOString()}
              </text>
              <text
                x={padding.left + plotWidth}
                y={height - 5}
                textAnchor="end"
                fontSize="9"
                fill="#94A3B8"
                fontFamily="sans-serif"
              >
                {new Date(points[points.length - 1].executed_at * 1000).toISOString()}
              </text>
            </>
          )}
        </svg>
      </Box>

      {/* Legend */}
      <Box sx={{ display: "flex", gap: 2, mt: 0.5, justifyContent: "flex-end" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: color }} />
          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
            Avg Latency
          </Typography>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: color2 }} />
          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
            Max Latency
          </Typography>
        </Box>
      </Box>
    </Paper>
  );
};

// -------------------------------------------------------------
// Interactive SVG Chart: Packet Loss Rate (%) over Time
// -------------------------------------------------------------

interface CronPacketLossChartProps {
  title: string;
  points: CronTaskResultRecord[];
  color?: string;
}

const CronPacketLossChart: React.FC<CronPacketLossChartProps> = ({ title, points, color = "#DC2626" }) => {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

  const stats = useMemo(() => {
    if (points.length === 0) return { max: 100, current: 0, avg: 0 };
    let sum = 0;
    points.forEach((p) => {
      sum += p.packet_loss_pct;
    });
    return {
      max: 100,
      current: points[points.length - 1].packet_loss_pct,
      avg: sum / points.length,
    };
  }, [points]);

  const height = 200;
  const padding = { top: 20, right: 15, bottom: 25, left: 15 };
  const width = 500;
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;

  const pointsLoss = useMemo(() => {
    if (points.length === 0) return "";
    return points
      .map((d, i) => {
        const x = padding.left + (i / Math.max(1, points.length - 1)) * plotWidth;
        const normY = d.packet_loss_pct / 100.0;
        const y = padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
        return `${x},${y}`;
      })
      .join(" ");
  }, [points, plotWidth, plotHeight]);

  const areaLoss = useMemo(() => {
    if (!pointsLoss || points.length === 0) return "";
    const startX = padding.left;
    const endX = padding.left + plotWidth;
    const bottomY = padding.top + plotHeight;
    return `M ${startX},${bottomY} L ${pointsLoss.replace(/ /g, " L ")} L ${endX},${bottomY} Z`;
  }, [pointsLoss, points.length, plotWidth, plotHeight]);

  const handlePointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (points.length === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
    const index = Math.round(ratio * (points.length - 1));
    setHoverIndex(index);
  };

  const activePoint = hoverIndex !== null && points[hoverIndex] ? points[hoverIndex] : null;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 2,
        borderRadius: 2.5,
        border: "1px solid #E2E8F0",
        bgcolor: "#FFFFFF",
      }}
    >
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1 }}>
        <Box>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, textTransform: "uppercase" }}>
            {title}
          </Typography>
          <Typography
            variant="h6"
            sx={{
              fontWeight: 800,
              fontSize: "1.2rem",
              color: (activePoint ? activePoint.packet_loss_pct : stats.current) === 0 ? "#16A34A" : "#DC2626",
            }}
          >
            {activePoint ? `${activePoint.packet_loss_pct.toFixed(1)}%` : `${stats.current.toFixed(1)}%`}
          </Typography>
        </Box>

        <Box sx={{ textAlign: "right" }}>
          {activePoint ? (
            <Typography variant="caption" sx={{ color: "#475569", fontWeight: 600 }}>
              {new Date(activePoint.executed_at * 1000).toISOString()}
            </Typography>
          ) : (
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Avg Loss: <strong>{stats.avg.toFixed(1)}%</strong>
            </Typography>
          )}
        </Box>
      </Box>

      {/* SVG Container */}
      <Box sx={{ width: "100%", height, touchAction: "none", cursor: "crosshair" }}>
        <svg
          viewBox={`0 0 ${width} ${height}`}
          style={{ width: "100%", height: "100%", display: "block" }}
          onPointerMove={handlePointerMove}
          onPointerLeave={() => setHoverIndex(null)}
        >
          <defs>
            <linearGradient id="grad-cron-loss" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={color} stopOpacity="0.3" />
              <stop offset="100%" stopColor={color} stopOpacity="0.0" />
            </linearGradient>
          </defs>

          {/* Grid lines: 0%, 50%, 100% */}
          {[0, 0.5, 1].map((pct) => {
            const y = padding.top + plotHeight * (1 - pct);
            const val = pct * 100;
            return (
              <g key={pct}>
                <line
                  x1={padding.left}
                  y1={y}
                  x2={padding.left + plotWidth}
                  y2={y}
                  stroke="#F1F5F9"
                  strokeWidth="1"
                  strokeDasharray="4 4"
                />
                <text x={padding.left + 4} y={y - 3} fontSize="9" fill="#94A3B8" fontFamily="sans-serif">
                  {val.toFixed(0)}%
                </text>
              </g>
            );
          })}

          {/* Area fill */}
          {areaLoss && <path d={areaLoss} fill="url(#grad-cron-loss)" />}

          {/* Loss Line */}
          {pointsLoss && (
            <polyline
              fill="none"
              stroke={color}
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
              points={pointsLoss}
            />
          )}

          {/* Hover crosshair */}
          {hoverIndex !== null && points[hoverIndex] && (
            <g>
              <line
                x1={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                y1={padding.top}
                x2={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                y2={padding.top + plotHeight}
                stroke="#94A3B8"
                strokeWidth="1.2"
                strokeDasharray="2 2"
              />
              <circle
                cx={padding.left + (hoverIndex / Math.max(1, points.length - 1)) * plotWidth}
                cy={
                  padding.top +
                  plotHeight -
                  Math.max(0, Math.min(plotHeight, (points[hoverIndex].packet_loss_pct / 100) * plotHeight))
                }
                r="4"
                fill="#FFFFFF"
                stroke={color}
                strokeWidth="2.5"
              />
            </g>
          )}

          {/* Time axis */}
          {points.length > 1 && (
            <>
              <text x={padding.left} y={height - 5} fontSize="9" fill="#94A3B8" fontFamily="sans-serif">
                {new Date(points[0].executed_at * 1000).toISOString()}
              </text>
              <text
                x={padding.left + plotWidth}
                y={height - 5}
                textAnchor="end"
                fontSize="9"
                fill="#94A3B8"
                fontFamily="sans-serif"
              >
                {new Date(points[points.length - 1].executed_at * 1000).toISOString()}
              </text>
            </>
          )}
        </svg>
      </Box>

      {/* Legend */}
      <Box sx={{ display: "flex", gap: 2, mt: 0.5, justifyContent: "flex-end" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: color }} />
          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
            Packet Loss %
          </Typography>
        </Box>
      </Box>
    </Paper>
  );
};
