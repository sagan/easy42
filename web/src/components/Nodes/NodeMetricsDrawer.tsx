import React, { useState, useEffect, useCallback } from "react";
import {
  Drawer,
  Dialog,
  DialogContent,
  Box,
  Typography,
  IconButton,
  Button,
  ButtonGroup,
  Chip,
  CircularProgress,
  useTheme,
  useMediaQuery,
  Grid,
} from "@mui/material";
import { X, RefreshCw, Server, ArrowDown, ArrowUp } from "lucide-react";
import { api } from "../../api/client";
import { NodeMetricPoint, NodeLiveStatus } from "../../types/api";
import { MetricAreaChart } from "./MetricAreaChart";

interface NodeMetricsDrawerProps {
  open: boolean;
  onClose: () => void;
  nodeName: string | null;
  nodeLive?: NodeLiveStatus | null;
}

export const NodeMetricsDrawer: React.FC<NodeMetricsDrawerProps> = ({
  open,
  onClose,
  nodeName,
  nodeLive,
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));
  const [range, setRange] = useState<string>("24h");
  const [loading, setLoading] = useState<boolean>(false);
  const [points, setPoints] = useState<NodeMetricPoint[]>([]);

  const fetchMetrics = useCallback(async () => {
    if (!nodeName) return;
    setLoading(true);
    try {
      const res = await api.getNodeMetrics(nodeName, range);
      setPoints(res.points || []);
    } catch {
      setPoints([]);
    } finally {
      setLoading(false);
    }
  }, [nodeName, range]);

  useEffect(() => {
    if (open && nodeName) {
      fetchMetrics();
    }
  }, [open, nodeName, fetchMetrics]);

  // Transform data for charts
  const cpuData = points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent }));
  const memData = points.map((p) => ({
    timestamp: p.timestamp,
    value: p.memory_used_bytes / (1024 * 1024 * 1024), // GB
    value2: p.memory_total_bytes / (1024 * 1024 * 1024),
  }));
  const netData = points.map((p) => ({
    timestamp: p.timestamp,
    value: p.net_rx_rate,
    value2: p.net_tx_rate,
  }));
  const loadData = points.map((p) => ({
    timestamp: p.timestamp,
    value: p.load_1m,
    value2: p.load_5m,
  }));

  const formatNetRate = (bytesPerSec: number) => {
    if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(2)} MB/s`;
    if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`;
    return `${bytesPerSec.toFixed(0)} B/s`;
  };

  const formatUptime = (secs?: number) => {
    if (!secs) return "Unknown";
    const d = Math.floor(secs / 86400);
    const h = Math.floor((secs % 86400) / 3600);
    const m = Math.floor((secs % 3600) / 60);
    if (d > 0) return `${d}d ${h}h ${m}m`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  };

  const content = (
    <Box sx={{ p: { xs: 2, sm: 3 }, height: "100%", display: "flex", flexDirection: "column" }}>
      {/* Header */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              p: 1,
              bgcolor: "#EEF2FF",
              color: "#4F46E5",
              borderRadius: 2,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Server size={22} />
          </Box>
          <Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {nodeName}
              </Typography>
              <Chip
                label={nodeLive?.connected ? "Online" : "Offline"}
                size="small"
                sx={{
                  bgcolor: nodeLive?.connected ? "#ECFDF5" : "#FEF2F2",
                  color: nodeLive?.connected ? "#059669" : "#E11D48",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                  height: 22,
                }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              {nodeLive?.hostname || "Unknown Host"} • Uptime: {formatUptime(nodeLive?.uptime_seconds)}
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <IconButton size="small" onClick={fetchMetrics} disabled={loading} sx={{ color: "#64748B" }}>
            <RefreshCw size={18} className={loading ? "spin" : ""} />
          </IconButton>
          <IconButton size="small" onClick={onClose} sx={{ color: "#64748B" }}>
            <X size={20} />
          </IconButton>
        </Box>
      </Box>

      {/* Range Filter Buttons */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2.5, flexWrap: "wrap", gap: 1 }}>
        <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155" }}>
          Telemetry Range
        </Typography>
        <ButtonGroup size="small" variant="outlined" sx={{ bgcolor: "#F8FAFC" }}>
          {["1h", "6h", "24h", "7d"].map((r) => (
            <Button
              key={r}
              onClick={() => setRange(r)}
              sx={{
                fontWeight: range === r ? 700 : 500,
                bgcolor: range === r ? "#4F46E5" : "transparent",
                color: range === r ? "#FFFFFF" : "#64748B",
                borderColor: "#E2E8F0",
                "&:hover": {
                  bgcolor: range === r ? "#4338CA" : "#F1F5F9",
                },
              }}
            >
              {r.toUpperCase()}
            </Button>
          ))}
        </ButtonGroup>
      </Box>

      {/* Metrics Quick Bar */}
      {nodeLive?.metrics && (
        <Grid container spacing={1.5} sx={{ mb: 2.5 }}>
          <Grid item xs={6} sm={3}>
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                CPU Load
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {nodeLive.metrics.cpu_percent.toFixed(1)}%
              </Typography>
            </Box>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                RAM Usage
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {(nodeLive.metrics.memory_used_bytes / (1024 * 1024 * 1024)).toFixed(1)} GB
              </Typography>
            </Box>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <ArrowDown size={14} color="#059669" /> Inbound
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {formatNetRate(nodeLive.metrics.net_rx_rate)}
              </Typography>
            </Box>
          </Grid>
          <Grid item xs={6} sm={3}>
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <ArrowUp size={14} color="#0891B2" /> Outbound
              </Typography>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {formatNetRate(nodeLive.metrics.net_tx_rate)}
              </Typography>
            </Box>
          </Grid>
        </Grid>
      )}

      {/* Charts Section */}
      <Box sx={{ flex: 1, overflowY: "auto", pr: 0.5, display: "flex", flexDirection: "column", gap: 2 }}>
        {loading && points.length === 0 ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
            <CircularProgress size={32} />
          </Box>
        ) : (
          <>
            <MetricAreaChart
              title="CPU Usage"
              data={cpuData}
              color="#4F46E5"
              unit="%"
              height={170}
              formatValue={(v) => `${v.toFixed(1)}%`}
            />

            <MetricAreaChart
              title="Memory Utilization (Used / Total)"
              data={memData}
              color="#059669"
              color2="#94A3B8"
              label1="Used"
              label2="Total"
              height={170}
              formatValue={(v) => `${v.toFixed(2)} GB`}
            />

            <MetricAreaChart
              title="Network Bandwidth"
              data={netData}
              color="#059669"
              color2="#0891B2"
              label1="Inbound (Rx)"
              label2="Outbound (Tx)"
              height={170}
              formatValue={formatNetRate}
            />

            <MetricAreaChart
              title="System Load Average"
              data={loadData}
              color="#D97706"
              color2="#64748B"
              label1="1 min"
              label2="5 min"
              height={170}
              formatValue={(v) => v.toFixed(2)}
            />
          </>
        )}
      </Box>
    </Box>
  );

  if (isMobile) {
    return (
      <Dialog fullScreen open={open} onClose={onClose}>
        <DialogContent sx={{ p: 0, bgcolor: "#F8FAFC" }}>{content}</DialogContent>
      </Dialog>
    );
  }

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { sm: 550, md: 620, lg: 680 },
          bgcolor: "#F8FAFC",
          boxShadow: "0 20px 25px -5px rgba(0, 0, 0, 0.1)",
        },
      }}
    >
      {content}
    </Drawer>
  );
};
