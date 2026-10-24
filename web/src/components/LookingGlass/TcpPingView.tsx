import React from "react";
import { Box, Typography, Paper, LinearProgress, Chip, Tooltip } from "@mui/material";
import {
  ArrowDown,
  ArrowUp,
  Zap,
  ShieldAlert,
  CheckCircle2,
  XCircle,
  Globe,
  Layers,
  Server,
  ArrowLeftRight,
  Info,
} from "lucide-react";
import { TcpPingResult } from "../../types/api";

interface TcpPingViewProps {
  tcpPing: TcpPingResult;
}

export const TcpPingView: React.FC<TcpPingViewProps> = ({ tcpPing }) => {
  const lossPct = tcpPing.packet_loss_pct ?? 0;
  const isReachable = (tcpPing.packets_received ?? 0) > 0;
  const hasMssInfo = tcpPing.local_mss !== undefined || tcpPing.remote_mss !== undefined;

  const lossColor =
    lossPct === 0
      ? { bg: "#ECFDF5", text: "#065F46", border: "#A7F3D0" }
      : lossPct <= 20
        ? { bg: "#FFFBEB", text: "#92400E", border: "#FDE68A" }
        : { bg: "#FEF2F2", text: "#991B1B", border: "#FECACA" };

  const isIPv6 = Boolean(tcpPing.resolved_addr?.includes(":") && !tcpPing.resolved_addr?.includes("."));
  const tsOverhead = tcpPing.tcp_timestamps ? 12 : 0;
  // IP (20 B for IPv4, 40 B for IPv6) + TCP base header (20 B)
  const ipHeaderSize = isIPv6 ? 40 : 20;
  const tcpBaseHeaderSize = 20;
  const baseOverhead = ipHeaderSize + tcpBaseHeaderSize;
  const totalOverhead = baseOverhead + tsOverhead;

  const localWireMss = tcpPing.local_mss ? tcpPing.local_mss + tsOverhead : undefined;
  const localEstMtu = tcpPing.local_mss ? tcpPing.local_mss + totalOverhead : undefined;

  const remoteWireMss = tcpPing.remote_mss ? tcpPing.remote_mss + tsOverhead : undefined;
  const remoteEstMtu = tcpPing.remote_mss ? tcpPing.remote_mss + totalOverhead : undefined;

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

      {/* TCP MSS & Handshake MTU Negotiation Section */}
      {hasMssInfo && (
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
          {/* Header */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
              <Box
                sx={{
                  width: 34,
                  height: 34,
                  borderRadius: 1.5,
                  backgroundColor: "#F0F9FF",
                  border: "1px solid #BAE6FD",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  color: "#0284C7",
                }}
              >
                <Layers size={18} />
              </Box>
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}>
                  TCP MSS Negotiation & MTU Analysis
                  <Tooltip title="Maximum Segment Size (MSS) negotiated during the TCP 3-way handshake. Helps identify Path MTU issues, tunnel encapsulation overhead, and firewall/router MSS clamping.">
                    <Info size={14} color="#94A3B8" style={{ cursor: "pointer" }} />
                  </Tooltip>
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Segment size parameters exchanged in SYN and SYN+ACK packets
                </Typography>
              </Box>
            </Box>

            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
              {tcpPing.tcp_timestamps && (
                <Chip
                  label="TCP Timestamps Active (−12 B TSopt)"
                  size="small"
                  sx={{
                    fontWeight: 600,
                    fontSize: "0.72rem",
                    backgroundColor: "#EFF6FF",
                    color: "#1D4ED8",
                    border: "1px solid #BFDBFE",
                  }}
                />
              )}
              {tcpPing.local_mss && tcpPing.remote_mss && (
                <Chip
                  icon={<ArrowLeftRight size={14} />}
                  label={`Effective Send MSS: ${Math.min(tcpPing.local_mss, tcpPing.remote_mss)} B`}
                  sx={{
                    fontWeight: 700,
                    fontSize: "0.78rem",
                    backgroundColor: "#F0FDF4",
                    color: "#166534",
                    border: "1px solid #BBF7D0",
                    "& .MuiChip-icon": { color: "#166534" },
                  }}
                />
              )}
            </Box>
          </Box>

          {/* Cards: Local MSS vs Target Host MSS */}
          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)" },
              gap: 2,
            }}
          >
            {/* Local Agent Node MSS */}
            <Paper
              elevation={0}
              sx={{
                p: 2,
                borderRadius: 2,
                border: "1px solid #E0F2FE",
                backgroundColor: "#F8FAFC",
                display: "flex",
                flexDirection: "column",
                gap: 1,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Server size={16} color="#0284C7" />
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#0369A1", textTransform: "uppercase" }}>
                    Local Node MSS (Agent)
                  </Typography>
                </Box>
                <Chip
                  label="SYN Option"
                  size="small"
                  sx={{
                    fontSize: "0.68rem",
                    fontWeight: 700,
                    height: 20,
                    backgroundColor: "#E0F2FE",
                    color: "#0369A1",
                  }}
                />
              </Box>

              <Box sx={{ display: "flex", alignItems: "baseline", gap: 1 }}>
                <Typography variant="h4" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
                  {localWireMss ? `${localWireMss} B` : "-"}
                </Typography>
                {tcpPing.tcp_timestamps && tcpPing.local_mss && (
                  <Typography variant="body2" sx={{ color: "#64748B", fontFamily: "monospace" }}>
                    ({tcpPing.local_mss} B payload)
                  </Typography>
                )}
              </Box>

              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  {tcpPing.tcp_timestamps
                    ? `Wire SYN MSS: ${localWireMss} B | Usable payload: ${tcpPing.local_mss} B (−12 B TSopt)`
                    : "Advertised by agent node network stack in SYN packet"}
                </Typography>
                {localEstMtu && (
                  <Typography variant="caption" sx={{ color: "#0369A1", fontWeight: 600 }}>
                    Est. Interface MTU: ~{localEstMtu} B ({isIPv6 ? "IPv6" : "IPv4"})
                  </Typography>
                )}
              </Box>
            </Paper>

            {/* Target Host MSS */}
            <Paper
              elevation={0}
              sx={{
                p: 2,
                borderRadius: 2,
                border: "1px solid #F3E8FF",
                backgroundColor: "#FAF5FF",
                display: "flex",
                flexDirection: "column",
                gap: 1,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Globe size={16} color="#7C3AED" />
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#7C3AED", textTransform: "uppercase" }}>
                    Target Host MSS (Remote)
                  </Typography>
                </Box>
                <Chip
                  label="SYN+ACK Option"
                  size="small"
                  sx={{
                    fontSize: "0.68rem",
                    fontWeight: 700,
                    height: 20,
                    backgroundColor: "#F3E8FF",
                    color: "#7C3AED",
                  }}
                />
              </Box>

              <Box sx={{ display: "flex", alignItems: "baseline", gap: 1 }}>
                <Typography variant="h4" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
                  {remoteWireMss ? `${remoteWireMss} B` : "-"}
                </Typography>
                {tcpPing.tcp_timestamps && tcpPing.remote_mss && (
                  <Typography variant="body2" sx={{ color: "#64748B", fontFamily: "monospace" }}>
                    ({tcpPing.remote_mss} B payload)
                  </Typography>
                )}
              </Box>

              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  {tcpPing.tcp_timestamps
                    ? `Wire SYN+ACK MSS: ${remoteWireMss} B | Usable payload: ${tcpPing.remote_mss} B`
                    : "Offered by target host or path clamp (snd_mss)"}
                </Typography>
                {remoteEstMtu && (
                  <Typography variant="caption" sx={{ color: "#7C3AED", fontWeight: 600 }}>
                    Est. Path MTU: ~{remoteEstMtu} B ({isIPv6 ? "IPv6" : "IPv4"})
                  </Typography>
                )}
              </Box>
            </Paper>
          </Box>

          {/* Diagnostic MTU Insight Banner */}
          {tcpPing.local_mss && tcpPing.remote_mss && (
            <Box
              sx={{
                p: 1.5,
                borderRadius: 1.5,
                backgroundColor:
                  tcpPing.remote_mss === tcpPing.local_mss
                    ? "#F0FDF4"
                    : tcpPing.remote_mss < tcpPing.local_mss
                      ? "#FFFBEB"
                      : "#F8FAFC",
                border: `1px solid ${
                  tcpPing.remote_mss === tcpPing.local_mss
                    ? "#BBF7D0"
                    : tcpPing.remote_mss < tcpPing.local_mss
                      ? "#FDE68A"
                      : "#E2E8F0"
                }`,
                display: "flex",
                alignItems: "flex-start",
                gap: 1.25,
              }}
            >
              <Info
                size={18}
                color={
                  tcpPing.remote_mss === tcpPing.local_mss
                    ? "#16A34A"
                    : tcpPing.remote_mss < tcpPing.local_mss
                      ? "#D97706"
                      : "#64748B"
                }
                style={{ marginTop: 2, flexShrink: 0 }}
              />
              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
                <Typography
                  variant="caption"
                  sx={{
                    fontWeight: 700,
                    color:
                      tcpPing.remote_mss === tcpPing.local_mss
                        ? "#15803D"
                        : tcpPing.remote_mss < tcpPing.local_mss
                          ? "#B45309"
                          : "#334155",
                  }}
                >
                  {tcpPing.remote_mss === tcpPing.local_mss
                    ? "Symmetric MTU Alignment"
                    : tcpPing.remote_mss < tcpPing.local_mss
                      ? `Path / Target MSS Clamped (Δ ${tcpPing.local_mss - tcpPing.remote_mss} B)`
                      : `Local Interface MSS Bound (Δ ${tcpPing.remote_mss - tcpPing.local_mss} B)`}
                </Typography>
                <Typography variant="caption" sx={{ color: "#475569", lineHeight: 1.4 }}>
                  {tcpPing.remote_mss === tcpPing.local_mss
                    ? `Both the local agent and target host agree on an effective payload MSS of ${tcpPing.local_mss} B (wire MSS ${localWireMss} B). No middlebox MSS clamping detected.`
                    : tcpPing.remote_mss < tcpPing.local_mss
                      ? `Target host or network middlebox (e.g. VPN tunnel, PPPoE, or firewall) clamped the MSS down to ${tcpPing.remote_mss} B. Data transmissions will respect ${tcpPing.remote_mss} B payload to avoid Path MTU packet drops.`
                      : `Local node interface restricts outgoing segments to ${tcpPing.local_mss} B even though remote host accepts up to ${tcpPing.remote_mss} B.`}
                </Typography>
              </Box>
            </Box>
          )}
        </Paper>
      )}

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
