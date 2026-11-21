import React from "react";
import { Box, Typography, Paper, LinearProgress, Chip, Tooltip } from "@mui/material";
import { ArrowDown, ArrowUp, Zap, ShieldAlert, CheckCircle2, XCircle, Globe, Clock, Info } from "lucide-react";
import { HttpResult, HttpProbe } from "../../types/api";

interface HttpViewProps {
  http: HttpResult;
}

/** Color coding for an HTTP status code: 2xx/3xx ok-ish, 4xx/5xx still "received". */
const statusChipSx = (code: number) => {
  if (code >= 200 && code < 300) {
    return { bgcolor: "#DCFCE7", color: "#15803D", border: "1px solid #A7F3D0" };
  }
  if (code >= 300 && code < 400) {
    return { bgcolor: "#EFF6FF", color: "#1D4ED8", border: "1px solid #BFDBFE" };
  }
  if (code >= 400 && code < 500) {
    return { bgcolor: "#FFFBEB", color: "#B45309", border: "1px solid #FDE68A" };
  }
  if (code >= 500) {
    return { bgcolor: "#FEF2F2", color: "#B91C1C", border: "1px solid #FECACA" };
  }
  return { bgcolor: "#F1F5F9", color: "#475569", border: "1px solid #E2E8F0" };
};

const statusLabel = (code: number): string => {
  if (code >= 200 && code < 300) return "Success";
  if (code >= 300 && code < 400) return "Redirect (not followed)";
  if (code >= 400 && code < 500) return "Client Error";
  if (code >= 500) return "Server Error";
  return `Status ${code}`;
};

export const HttpView: React.FC<HttpViewProps> = ({ http }) => {
  const lossPct = http.packet_loss_pct ?? 0;
  const isReachable = (http.packets_received ?? 0) > 0;
  const probes: HttpProbe[] = http.probes ?? [];

  const lossColor =
    lossPct === 0
      ? { bg: "#ECFDF5", text: "#065F46", border: "#A7F3D0" }
      : lossPct <= 20
        ? { bg: "#FFFBEB", text: "#92400E", border: "#FDE68A" }
        : { bg: "#FEF2F2", text: "#991B1B", border: "#FECACA" };

  const maxScale = Math.max((http.max_ttfb_ms || 0) * 1.25, 20);

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {/* Target & Status Summary Banner */}
      <Paper
        elevation={0}
        sx={{
          p: 1,
          borderRadius: 1,
          border: "1px solid #E2E8F0",
          backgroundColor: "#F8FAFC",
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", minWidth: 0 }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: 1,
              flexShrink: 0,
              backgroundColor: isReachable ? "#DCFCE7" : "#FEE2E2",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: isReachable ? "#16A34A" : "#DC2626",
            }}
          >
            <Globe size={20} />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Target URL
            </Typography>
            <Typography
              variant="subtitle1"
              title={http.target}
              sx={{
                fontWeight: 700,
                fontFamily: "monospace",
                color: "#0F172A",
                wordBreak: "break-all",
                fontSize: "0.9rem",
              }}
            >
              {http.target}
            </Typography>
          </Box>
          {http.resolved_addr && http.resolved_addr !== http.target && (
            <Chip
              label={`Resolved: ${http.resolved_addr}`}
              size="small"
              sx={{ fontFamily: "monospace", fontSize: "0.75rem", backgroundColor: "#E2E8F0", color: "#334155" }}
            />
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
          {(http.status_codes ?? []).map((code) => (
            <Chip
              key={code}
              label={`${code} · ${statusLabel(code)}`}
              size="small"
              sx={{ fontWeight: 700, fontSize: "0.78rem", ...statusChipSx(code) }}
            />
          ))}
          <Chip
            icon={isReachable ? <CheckCircle2 size={16} /> : <XCircle size={16} />}
            label={isReachable ? "Response Received" : "Request Failed"}
            sx={{
              fontWeight: 700,
              fontSize: "0.8rem",
              backgroundColor: isReachable ? "#DCFCE7" : "#FEE2E2",
              color: isReachable ? "#15803D" : "#B91C1C",
              "& .MuiChip-icon": { color: isReachable ? "#15803D" : "#B91C1C" },
            }}
          />
        </Box>
      </Paper>

      {/* KPI Cards Grid */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "repeat(2, 1fr)", sm: "repeat(4, 1fr)" },
          gap: 1,
        }}
      >
        {/* Success rate / failures */}
        <Paper
          elevation={0}
          sx={{
            p: 1,
            borderRadius: 1,
            border: `1px solid ${lossColor.border}`,
            backgroundColor: lossColor.bg,
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: lossColor.text, fontWeight: 600, textTransform: "uppercase" }}>
              Failed Requests
            </Typography>
            <ShieldAlert size={16} color={lossColor.text} />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: lossColor.text }}>
            {lossPct.toFixed(1)}%
          </Typography>
          <Typography variant="caption" sx={{ color: lossColor.text }}>
            {http.packets_received ?? 0} / {http.packets_sent ?? 0} succeeded
          </Typography>
        </Paper>

        {/* Min TTFB */}
        <Paper
          elevation={0}
          sx={{
            p: 1,
            borderRadius: 1,
            border: "1px solid #E2E8F0",
            backgroundColor: "#FFFFFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Min TTFB
            </Typography>
            <ArrowDown size={16} color="#10B981" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
            {isReachable && http.min_ttfb_ms > 0 ? `${http.min_ttfb_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            Fastest first byte
          </Typography>
        </Paper>

        {/* Avg TTFB */}
        <Paper
          elevation={0}
          sx={{
            p: 1,
            borderRadius: 1,
            border: "1.5px solid #6366F1",
            backgroundColor: "#FAFBFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#4F46E5", fontWeight: 700, textTransform: "uppercase" }}>
              Avg TTFB
            </Typography>
            <Zap size={16} color="#4F46E5" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#4F46E5", fontFamily: "monospace" }}>
            {isReachable && http.avg_ttfb_ms > 0 ? `${http.avg_ttfb_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#6366F1" }}>
            Mean time to first byte
          </Typography>
        </Paper>

        {/* Max TTFB */}
        <Paper
          elevation={0}
          sx={{
            p: 1,
            borderRadius: 1,
            border: "1px solid #E2E8F0",
            backgroundColor: "#FFFFFF",
            display: "flex",
            flexDirection: "column",
            gap: 0.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
              Max TTFB
            </Typography>
            <ArrowUp size={16} color="#F59E0B" />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A", fontFamily: "monospace" }}>
            {isReachable && http.max_ttfb_ms > 0 ? `${http.max_ttfb_ms.toFixed(2)} ms` : "-"}
          </Typography>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            mdev: {isReachable && http.mdev_ttfb_ms > 0 ? `${http.mdev_ttfb_ms.toFixed(2)} ms` : "-"}
          </Typography>
        </Paper>
      </Box>

      {/* Explanation banner */}
      <Box
        sx={{
          p: 1,
          borderRadius: 1,
          backgroundColor: "#F8FAFC",
          border: "1px solid #E2E8F0",
          display: "flex",
          alignItems: "flex-start",
          gap: 1.25,
        }}
      >
        <Info size={18} color="#64748B" style={{ marginTop: 2, flexShrink: 0 }} />
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
          <Typography variant="caption" sx={{ fontWeight: 700, color: "#334155" }}>
            GET request with no redirects followed; response body is not read
          </Typography>
          <Typography variant="caption" sx={{ color: "#475569", lineHeight: 1.4 }}>
            A probe counts as successful once the HTTP response header is received. 4xx and 5xx status codes are still
            considered successful responses and are reported above. TTFB measures elapsed time until the response header
            arrives.
          </Typography>
        </Box>
      </Box>

      {/* Per-probe list */}
      {probes.length > 0 && (
        <Paper elevation={0} sx={{ p: 1, border: "1px solid #E2E8F0", borderRadius: 2 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B", mb: 1 }}>
            Individual HTTP Probes
          </Typography>

          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            {probes.map((probe, idx) => {
              const failed = Boolean(probe.error) || probe.status_code === 0;
              const pct = Math.min(100, (probe.ttfb_ms / maxScale) * 100);

              return (
                <Box
                  key={idx}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    p: 1,
                    borderRadius: 1,
                    backgroundColor: failed ? "#FEF2F2" : "#F8FAFC",
                  }}
                >
                  <Typography variant="caption" sx={{ color: "#64748B", fontFamily: "monospace", width: 32 }}>
                    #{idx + 1}
                  </Typography>

                  <Chip
                    label={failed ? "ERROR" : probe.status_code}
                    size="small"
                    sx={{
                      fontFamily: "monospace",
                      fontWeight: 700,
                      fontSize: "0.7rem",
                      height: 22,
                      width: 64,
                      ...(failed ? { bgcolor: "#FEE2E2", color: "#B91C1C" } : statusChipSx(probe.status_code)),
                    }}
                  />

                  <Box sx={{ flex: 1, minWidth: 60 }}>
                    {failed ? (
                      <Typography
                        variant="caption"
                        title={probe.error}
                        sx={{
                          color: "#B91C1C",
                          fontFamily: "monospace",
                          display: "block",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {probe.error || "request failed"}
                      </Typography>
                    ) : (
                      <LinearProgress
                        variant="determinate"
                        value={pct}
                        sx={{
                          height: 8,
                          borderRadius: 4,
                          backgroundColor: "#E2E8F0",
                          "& .MuiLinearProgress-bar": {
                            backgroundColor:
                              probe.ttfb_ms < 100 ? "#10B981" : probe.ttfb_ms < 300 ? "#F59E0B" : "#8B5CF6",
                            borderRadius: 4,
                          },
                        }}
                      />
                    )}
                  </Box>

                  {!failed && probe.status && (
                    <Tooltip title={probe.status}>
                      <Typography
                        variant="caption"
                        sx={{
                          color: "#64748B",
                          fontFamily: "monospace",
                          maxWidth: 140,
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                      >
                        {probe.status}
                      </Typography>
                    </Tooltip>
                  )}

                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, width: 90, justifyContent: "flex-end" }}>
                    <Clock size={13} color="#94A3B8" />
                    <Typography
                      variant="body2"
                      sx={{ fontFamily: "monospace", fontWeight: 600, color: failed ? "#94A3B8" : "#0F172A" }}
                    >
                      {failed ? "-" : `${probe.ttfb_ms.toFixed(2)} ms`}
                    </Typography>
                  </Box>
                </Box>
              );
            })}
          </Box>
        </Paper>
      )}
    </Box>
  );
};
