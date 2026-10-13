import React from "react";
import { Box, Typography, Paper, LinearProgress, Chip } from "@mui/material";
import { ArrowDown, ArrowUp, Zap, ShieldAlert, CheckCircle2, XCircle, Globe } from "lucide-react";
import { TcpPingResult } from "../../types/api";

interface TcpPingViewProps {
  tcpPing: TcpPingResult;
}

export const TcpPingView: React.FC<TcpPingViewProps> = ({ tcpPing }) => {
  const lossPct = tcpPing.packet_loss_pct ?? 0;
  const isReachable = (tcpPing.packets_received ?? 0) > 0;

  const lossColor =
    lossPct === 0
      ? { bg: "#ECFDF5", text: "#065F46", border: "#A7F3D0" }
      : lossPct <= 20
        ? { bg: "#FFFBEB", text: "#92400E", border: "#FDE68A" }
        : { bg: "#FEF2F2", text: "#991B1B", border: "#FECACA" };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
      {/* Target & Resolution Summary Banner */}
      <Paper
        elevation={0}
        sx={{
          p: 2,
          borderRadius: 2,
          border: "1px solid #E2E8F0",
          backgroundColor: "#F8FAFC",
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1.5,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: 1.5,
              backgroundColor: isReachable ? "#DCFCE7" : "#FEE2E2",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: isReachable ? "#16A34A" : "#DC2626",
            }}
          >
            <Globe size={20} />
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Target Host & Port
            </Typography>
            <Typography variant="subtitle1" sx={{ fontWeight: 700, fontFamily: "monospace", color: "#0F172A" }}>
              {tcpPing.target}
            </Typography>
          </Box>
          {tcpPing.resolved_addr && tcpPing.resolved_addr !== tcpPing.target && (
            <Chip
              label={`Resolved: ${tcpPing.resolved_addr}`}
              size="small"
              sx={{
                fontFamily: "monospace",
                fontSize: "0.75rem",
                backgroundColor: "#E2E8F0",
                color: "#334155",
              }}
            />
          )}
        </Box>

        <Chip
          icon={isReachable ? <CheckCircle2 size={16} /> : <XCircle size={16} />}
          label={isReachable ? "Port Reachable" : "Connection Failed"}
          sx={{
            fontWeight: 700,
            fontSize: "0.8rem",
            backgroundColor: isReachable ? "#DCFCE7" : "#FEE2E2",
            color: isReachable ? "#15803D" : "#B91C1C",
            "& .MuiChip-icon": {
              color: isReachable ? "#15803D" : "#B91C1C",
            },
          }}
        />
      </Paper>

      {/* KPI Cards Grid */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", sm: "repeat(4, 1fr)" },
          gap: 1.5,
        }}
      >
        {/* Packet / Connection Loss */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: 2,
            border: `1px solid ${lossColor.border}`,
            backgroundColor: lossColor.bg,
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: lossColor.text, fontWeight: 600, textTransform: "uppercase" }}>
              Packet Loss
            </Typography>
            <ShieldAlert size={16} color={lossColor.text} />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: lossColor.text }}>
            {lossPct.toFixed(1)}%
          </Typography>
          <Typography variant="caption" sx={{ color: lossColor.text }}>
            {tcpPing.packets_received ?? 0} / {tcpPing.packets_sent ?? 0} connected
          </Typography>
        </Paper>

        {/* Min RTT */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: 2,
            border: "1px solid #E2E8F0",
            backgroundColor: "#FFFFFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Min Latency
            </Typography>
            <ArrowDown size={16} color="#10B981" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
            {tcpPing.min_rtt_ms && isReachable ? `${tcpPing.min_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            Fastest handshake
          </Typography>
        </Paper>

        {/* Avg RTT */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: 2,
            border: "1.5px solid #6366F1",
            backgroundColor: "#FAFBFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#4F46E5", fontWeight: 700, textTransform: "uppercase" }}>
              Avg Latency
            </Typography>
            <Zap size={16} color="#4F46E5" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#4F46E5", fontFamily: "monospace" }}>
            {tcpPing.avg_rtt_ms && isReachable ? `${tcpPing.avg_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#6366F1" }}>
            Mean TCP connect time
          </Typography>
        </Paper>

        {/* Max RTT */}
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: 2,
            border: "1px solid #E2E8F0",
            backgroundColor: "#FFFFFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Max Latency
            </Typography>
            <ArrowUp size={16} color="#F59E0B" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
            {tcpPing.max_rtt_ms && isReachable ? `${tcpPing.max_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            mdev: {tcpPing.mdev_rtt_ms && isReachable ? `${tcpPing.mdev_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
        </Paper>
      </Box>

      {/* Per-Probe Latency Chart */}
      {tcpPing.rtts && tcpPing.rtts.length > 0 && (
        <Paper elevation={0} sx={{ p: 2, border: "1px solid #E2E8F0", borderRadius: 2 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B", mb: 1.5 }}>
            Individual TCP Probes
          </Typography>

          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            {tcpPing.rtts.map((rtt, idx) => {
              const maxScale = Math.max(tcpPing.max_rtt_ms * 1.25, 20);
              const pct = Math.min(100, (rtt / maxScale) * 100);

              return (
                <Box
                  key={idx}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 2,
                    p: 1,
                    borderRadius: 1.5,
                    backgroundColor: "#F8FAFC",
                  }}
                >
                  <Typography variant="caption" sx={{ color: "#64748B", fontFamily: "monospace", width: 50 }}>
                    #{idx + 1}
                  </Typography>

                  <Box sx={{ flex: 1 }}>
                    <LinearProgress
                      variant="determinate"
                      value={pct}
                      sx={{
                        height: 8,
                        borderRadius: 4,
                        backgroundColor: "#E2E8F0",
                        "& .MuiLinearProgress-bar": {
                          backgroundColor: rtt < 30 ? "#10B981" : rtt < 80 ? "#F59E0B" : "#8B5CF6",
                          borderRadius: 4,
                        },
                      }}
                    />
                  </Box>

                  <Typography
                    variant="body2"
                    sx={{
                      fontFamily: "monospace",
                      fontWeight: 600,
                      width: 80,
                      textAlign: "right",
                      color: "#0F172A",
                    }}
                  >
                    {rtt.toFixed(2)} ms
                  </Typography>
                </Box>
              );
            })}
          </Box>
        </Paper>
      )}
    </Box>
  );
};
