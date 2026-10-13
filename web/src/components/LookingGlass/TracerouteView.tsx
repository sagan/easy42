import React, { useMemo, useContext } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Tooltip,
} from "@mui/material";
import { Server } from "lucide-react";
import { TracerouteResult, MTRResult, Node } from "../../types/api";
import { MeshContext } from "../../context/MeshContext";

interface TracerouteViewProps {
  traceResult?: TracerouteResult;
  mtrResult?: MTRResult;
  nodes?: Node[];
}

function normalizeIP(ipStr: string): string {
  if (!ipStr) return "";
  let clean = ipStr.trim();
  if (clean.includes("/")) {
    clean = clean.split("/")[0].trim();
  }
  if (clean.startsWith("[") && clean.endsWith("]")) {
    clean = clean.slice(1, -1).trim();
  }
  clean = clean.toLowerCase();

  if (clean.includes(":")) {
    try {
      const parts = clean.split("::");
      if (parts.length === 2) {
        const left = parts[0] ? parts[0].split(":") : [];
        const right = parts[1] ? parts[1].split(":") : [];
        const missing = 8 - (left.length + right.length);
        if (missing > 0) {
          const middle = new Array(missing).fill("0");
          const full = [...left, ...middle, ...right].map((seg) => seg.padStart(4, "0").toLowerCase());
          return full.join(":");
        }
      } else if (parts.length === 1) {
        const segs = clean.split(":");
        if (segs.length === 8) {
          return segs.map((seg) => seg.padStart(4, "0").toLowerCase()).join(":");
        }
      }
    } catch {
      // fallback to clean string
    }
  }
  return clean;
}

export const TracerouteView: React.FC<TracerouteViewProps> = ({ traceResult, mtrResult, nodes }) => {
  const navigate = useNavigate();
  const meshContext = useContext(MeshContext);
  const effectiveNodes: Node[] = nodes || meshContext?.nodes || [];

  // Map of normalized & raw lowercase IPs to Node
  const nodeIpMap = useMemo(() => {
    const map = new Map<string, Node>();
    (effectiveNodes || []).forEach((n: Node) => {
      const ips = [n.ip, n.ip6, n.external_ip, n.external_ip6];
      ips.forEach((rawIp) => {
        if (!rawIp) return;
        const trimmed = rawIp.trim();
        if (!trimmed) return;
        const noCidr = trimmed.split("/")[0].trim().toLowerCase();
        map.set(noCidr, n);
        const normalized = normalizeIP(trimmed);
        if (normalized) {
          map.set(normalized, n);
        }
      });
    });
    return map;
  }, [effectiveNodes]);

  const getNodeForIp = (ipStr?: string): Node | undefined => {
    if (!ipStr) return undefined;
    const trimmed = ipStr.trim();
    if (!trimmed || trimmed === "*") return undefined;
    const noCidr = trimmed.split("/")[0].trim().toLowerCase();
    const direct = nodeIpMap.get(noCidr);
    if (direct) return direct;
    const normalized = normalizeIP(trimmed);
    if (normalized) {
      return nodeIpMap.get(normalized);
    }
    return undefined;
  };

  if (mtrResult && mtrResult.hops && mtrResult.hops.length > 0) {
    return (
      <TableContainer component={Paper} elevation={0} sx={{ border: "1px solid #E2E8F0", borderRadius: 2 }}>
        <Table size="small">
          <TableHead sx={{ backgroundColor: "#F1F5F9" }}>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, color: "#475569", width: 60 }}>HOP</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569" }}>HOST / IP</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                LOSS %
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                SENT
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                LAST
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                AVG
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                BEST
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                WRST
              </TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>
                STDEV
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {mtrResult.hops.map((hop) => {
              const hasLoss = hop.loss_pct > 0;
              const matchedNode = getNodeForIp(hop.ip) || getNodeForIp(hop.host);
              const originalIp = hop.ip || hop.host;

              return (
                <TableRow
                  key={hop.hop}
                  sx={{
                    "&:hover": { backgroundColor: "#F8FAFC" },
                    backgroundColor: hop.loss_pct >= 50 ? "rgba(239, 68, 68, 0.05)" : "inherit",
                  }}
                >
                  <TableCell sx={{ fontWeight: 700, fontFamily: "monospace", color: "#64748B" }}>{hop.hop}</TableCell>
                  <TableCell>
                    {matchedNode ? (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Tooltip title={`easy42 node: ${matchedNode.name} (click to view)`} arrow>
                          <Chip
                            icon={<Server size={12} />}
                            label={matchedNode.name}
                            size="small"
                            onClick={() => navigate(`/nodes/${encodeURIComponent(matchedNode.name)}`)}
                            sx={{
                              backgroundColor: "#EEF2FF",
                              color: "#4338CA",
                              border: "1px solid #C7D2FE",
                              fontWeight: 700,
                              fontSize: "0.75rem",
                              height: 22,
                              cursor: "pointer",
                              "&:hover": { backgroundColor: "#E0E7FF" },
                              "& .MuiChip-icon": { color: "#4F46E5", ml: 0.5 },
                            }}
                          />
                        </Tooltip>
                        <Typography
                          sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A", fontSize: "0.85rem" }}
                        >
                          {originalIp}
                        </Typography>
                        {hop.host && hop.host !== originalIp && hop.host !== matchedNode.name && (
                          <Typography variant="caption" sx={{ fontFamily: "monospace", color: "#64748B" }}>
                            ({hop.host})
                          </Typography>
                        )}
                      </Box>
                    ) : (
                      <Typography sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A" }}>
                        {hop.host}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell align="right">
                    <Chip
                      label={`${hop.loss_pct.toFixed(1)}%`}
                      size="small"
                      sx={{
                        backgroundColor: !hasLoss ? "#ECFDF5" : hop.loss_pct < 20 ? "#FFFBEB" : "#FEF2F2",
                        color: !hasLoss ? "#065F46" : hop.loss_pct < 20 ? "#92400E" : "#991B1B",
                        fontWeight: 600,
                        fontSize: "0.7rem",
                        height: 20,
                      }}
                    />
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                    {hop.sent}
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                    {hop.last_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell
                    align="right"
                    sx={{ fontFamily: "monospace", fontWeight: 600, fontSize: "0.8rem", color: "#4F46E5" }}
                  >
                    {hop.avg_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#10B981" }}>
                    {hop.best_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#F59E0B" }}>
                    {hop.worst_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#64748B" }}>
                    {hop.stdev_ms.toFixed(1)}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
    );
  }

  const hops = traceResult?.hops || [];

  return (
    <TableContainer component={Paper} elevation={0} sx={{ border: "1px solid #E2E8F0", borderRadius: 2 }}>
      <Table size="small">
        <TableHead sx={{ backgroundColor: "#F1F5F9" }}>
          <TableRow>
            <TableCell sx={{ fontWeight: 600, color: "#475569", width: 60 }}>HOP</TableCell>
            <TableCell sx={{ fontWeight: 600, color: "#475569" }}>HOST / IP</TableCell>
            <TableCell sx={{ fontWeight: 600, color: "#475569" }}>PROBES / LATENCY</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {hops.length === 0 ? (
            <TableRow>
              <TableCell colSpan={3} align="center" sx={{ py: 3, color: "#94A3B8" }}>
                No hops recorded
              </TableCell>
            </TableRow>
          ) : (
            hops.map((hop) => {
              const isHostUnknown =
                (hop.timed_out || !hop.host || hop.host === "*") &&
                (!hop.host || hop.host === "*") &&
                (!hop.ip || hop.ip === "*");
              const matchedNode = !isHostUnknown ? getNodeForIp(hop.ip) || getNodeForIp(hop.host) : undefined;
              const originalIp = hop.ip && hop.ip !== "*" ? hop.ip : hop.host && hop.host !== "*" ? hop.host : "";

              return (
                <TableRow key={hop.hop} sx={{ "&:hover": { backgroundColor: "#F8FAFC" } }}>
                  <TableCell sx={{ fontWeight: 700, fontFamily: "monospace", color: "#64748B" }}>{hop.hop}</TableCell>
                  <TableCell>
                    {isHostUnknown ? (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Typography sx={{ fontFamily: "monospace", color: "#94A3B8", fontStyle: "italic" }}>
                          * * * (Request timed out)
                        </Typography>
                      </Box>
                    ) : matchedNode ? (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Tooltip title={`easy42 node: ${matchedNode.name} (click to view)`} arrow>
                          <Chip
                            icon={<Server size={12} />}
                            label={matchedNode.name}
                            size="small"
                            onClick={() => navigate(`/nodes/${encodeURIComponent(matchedNode.name)}`)}
                            sx={{
                              backgroundColor: "#EEF2FF",
                              color: "#4338CA",
                              border: "1px solid #C7D2FE",
                              fontWeight: 700,
                              fontSize: "0.75rem",
                              height: 22,
                              cursor: "pointer",
                              "&:hover": { backgroundColor: "#E0E7FF" },
                              "& .MuiChip-icon": { color: "#4F46E5", ml: 0.5 },
                            }}
                          />
                        </Tooltip>
                        <Typography sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A" }}>
                          {originalIp}
                        </Typography>
                        {hop.host && hop.host !== originalIp && hop.host !== matchedNode.name && (
                          <Typography variant="caption" sx={{ fontFamily: "monospace", color: "#64748B" }}>
                            ({hop.host})
                          </Typography>
                        )}
                      </Box>
                    ) : (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Typography sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A" }}>
                          {hop.host}
                        </Typography>
                        {hop.ip && hop.ip !== hop.host && (
                          <Typography variant="caption" sx={{ fontFamily: "monospace", color: "#64748B" }}>
                            ({hop.ip})
                          </Typography>
                        )}
                      </Box>
                    )}
                  </TableCell>
                  <TableCell>
                    {hop.timed_out || !hop.times_ms || hop.times_ms.length === 0 ? (
                      <Chip
                        label="Timeout"
                        size="small"
                        sx={{
                          backgroundColor: "#FEF2F2",
                          color: "#991B1B",
                          fontSize: "0.7rem",
                          height: 20,
                        }}
                      />
                    ) : (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                        {hop.times_ms.map((t, tIdx) => (
                          <Chip
                            key={tIdx}
                            label={`${t.toFixed(2)} ms`}
                            size="small"
                            sx={{
                              backgroundColor: t < 25 ? "#ECFDF5" : t < 70 ? "#FFFBEB" : "#F5F3FF",
                              color: t < 25 ? "#065F46" : t < 70 ? "#92400E" : "#6D28D9",
                              border: t < 25 ? "1px solid #A7F3D0" : t < 70 ? "1px solid #FDE68A" : "1px solid #DDD6FE",
                              fontFamily: "monospace",
                              fontWeight: 600,
                              fontSize: "0.72rem",
                              height: 22,
                            }}
                          />
                        ))}
                      </Box>
                    )}
                  </TableCell>
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
