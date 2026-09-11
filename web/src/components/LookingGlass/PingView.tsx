import React from "react";
import { Box, Typography, Paper, LinearProgress } from "@mui/material";
import { ArrowDown, ArrowUp, Zap, ShieldAlert } from "lucide-react";
import { PingResult } from "../../types/api";

interface PingViewProps {
  ping: PingResult;
}

export const PingView: React.FC<PingViewProps> = ({ ping }) => {
  const lossColor =
    ping.packet_loss_pct === 0
      ? { bg: "#ECFDF5", text: "#065F46", border: "#A7F3D0" }
      : ping.packet_loss_pct <= 20
      ? { bg: "#FFFBEB", text: "#92400E", border: "#FDE68A" }
      : { bg: "#FEF2F2", text: "#991B1B", border: "#FECACA" };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
      {/* KPI Cards Grid */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", sm: "repeat(4, 1fr)" },
          gap: 1.5,
        }}
      >
        {/* Packet Loss Card */}
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
            {ping.packet_loss_pct.toFixed(1)}%
          </Typography>
          <Typography variant="caption" sx={{ color: lossColor.text }}>
            {ping.packets_received} / {ping.packets_sent} packets received
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
            {ping.min_rtt_ms ? `${ping.min_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            Fastest round-trip
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
            {ping.avg_rtt_ms ? `${ping.avg_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#6366F1" }}>
            Mean RTT over {ping.packets_received} replies
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
            {ping.max_rtt_ms ? `${ping.max_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            mdev: {ping.mdev_rtt_ms ? `${ping.mdev_rtt_ms.toFixed(2)} ms` : "-"}
          </Typography>
        </Paper>
      </Box>

      {/* Per-Packet Table */}
      {ping.packets && ping.packets.length > 0 && (
        <Paper elevation={0} sx={{ p: 2, border: "1px solid #E2E8F0", borderRadius: 2 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B", mb: 1.5 }}>
            Individual Packet Probes
          </Typography>

          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            {ping.packets.map((pkt, idx) => {
              const maxScale = (ping.max_rtt_ms || 50) * 1.2;
              const pct = Math.min(100, (pkt.time_ms / maxScale) * 100);

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
                  <Typography variant="caption" sx={{ color: "#64748B", fontFamily: "monospace", width: 60 }}>
                    #{pkt.seq}
                  </Typography>

                  <Typography variant="caption" sx={{ color: "#334155", width: 60, fontFamily: "monospace" }}>
                    ttl={pkt.ttl}
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
                          backgroundColor:
                            pkt.time_ms < 30 ? "#10B981" : pkt.time_ms < 80 ? "#F59E0B" : "#8B5CF6",
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
                    {pkt.time_ms.toFixed(2)} ms
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
