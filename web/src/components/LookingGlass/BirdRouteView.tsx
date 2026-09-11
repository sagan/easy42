import React from "react";
import {
  Box,
  Typography,
  Chip,
  Paper,
  Divider,
  Tooltip,
  IconButton,
} from "@mui/material";
import {
  Star,
  ArrowRight,
  Network,
  Copy,
  Check,
} from "lucide-react";
import { BirdRouteResult } from "../../types/api";

interface BirdRouteViewProps {
  routeResult: BirdRouteResult;
  onQueryASN?: (asn: string) => void;
}

export const BirdRouteView: React.FC<BirdRouteViewProps> = ({
  routeResult,
  onQueryASN,
}) => {
  const [copiedText, setCopiedText] = React.useState<string | null>(null);

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedText(text);
    setTimeout(() => setCopiedText(null), 1500);
  };

  const routes = routeResult.routes || [];

  if (routes.length === 0) {
    return (
      <Paper
        elevation={0}
        sx={{
          p: 4,
          textAlign: "center",
          backgroundColor: "#F8FAFC",
          border: "1px dashed #CBD5E1",
          borderRadius: 2,
        }}
      >
        <Network size={32} color="#94A3B8" style={{ marginBottom: 8 }} />
        <Typography variant="body1" sx={{ color: "#475569", fontWeight: 600 }}>
          No BGP routes found for {routeResult.target || "the specified target"}
        </Typography>
        <Typography variant="body2" sx={{ color: "#94A3B8", mt: 0.5 }}>
          The target prefix may not be in the routing table, or was filtered out.
        </Typography>
      </Paper>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {/* Route Count Header */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          p: 1.5,
          borderRadius: 2,
          backgroundColor: "#F8FAFC",
          border: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Network size={18} color="#4F46E5" />
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
            Query Target:{" "}
            <span style={{ fontFamily: "monospace", color: "#4F46E5" }}>
              {routeResult.target || "All matching routes"}
            </span>
          </Typography>
          <Chip
            label={`${routes.length} Route Candidates`}
            size="small"
            sx={{
              backgroundColor: "#EEF2FF",
              color: "#4338CA",
              fontWeight: 600,
              fontSize: "0.72rem",
            }}
          />
        </Box>
      </Box>

      {/* Route Cards */}
      {routes.map((route, idx) => (
        <Paper
          key={idx}
          elevation={0}
          sx={{
            p: 2.5,
            borderRadius: 2,
            border: route.best ? "1.5px solid #6366F1" : "1px solid #E2E8F0",
            backgroundColor: route.best ? "#FAFBFF" : "#FFFFFF",
            boxShadow: route.best ? "0 4px 14px rgba(99, 102, 241, 0.08)" : "none",
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
          }}
        >
          {/* Top Bar: Prefix, Best Badge, Protocol, Metric */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
              <Typography
                variant="h6"
                sx={{
                  fontFamily: "monospace",
                  fontWeight: 700,
                  fontSize: "1.05rem",
                  color: "#0F172A",
                }}
              >
                {route.network}
              </Typography>

              {route.best && (
                <Chip
                  icon={<Star size={12} color="#FFFFFF" fill="#FFFFFF" />}
                  label="BEST ROUTE"
                  size="small"
                  sx={{
                    backgroundColor: "#4F46E5",
                    color: "#FFFFFF",
                    fontWeight: 700,
                    fontSize: "0.68rem",
                    letterSpacing: 0.5,
                    height: 22,
                  }}
                />
              )}

              <Chip
                label={route.from_proto}
                size="small"
                variant="outlined"
                sx={{
                  borderColor: "#CBD5E1",
                  color: "#475569",
                  fontWeight: 600,
                  fontSize: "0.72rem",
                  fontFamily: "monospace",
                  height: 22,
                }}
              />
            </Box>

            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              {route.metric && (
                <Chip
                  label={`Metric: ${route.metric}`}
                  size="small"
                  sx={{
                    backgroundColor: "#F1F5F9",
                    color: "#475569",
                    fontSize: "0.72rem",
                  }}
                />
              )}
              {route.local_pref && (
                <Chip
                  label={`LocalPref: ${route.local_pref}`}
                  size="small"
                  sx={{
                    backgroundColor: "#ECFDF5",
                    color: "#065F46",
                    fontSize: "0.72rem",
                    fontWeight: 600,
                  }}
                />
              )}
              {route.since && (
                <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                  Learned: {route.since}
                </Typography>
              )}
            </Box>
          </Box>

          <Divider sx={{ my: 0.5 }} />

          {/* Middle: Next-Hop & Interface */}
          <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 3 }}>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block", textTransform: "uppercase", fontSize: "0.68rem", fontWeight: 600 }}>
                Next Hop
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                <Typography variant="body2" sx={{ fontFamily: "monospace", fontWeight: 600, color: "#1E293B" }}>
                  {route.next_hop || route.via || "Direct / Local"}
                </Typography>
                {(route.next_hop || route.via) && (
                  <Tooltip title={copiedText === (route.next_hop || route.via) ? "Copied!" : "Copy IP"}>
                    <IconButton size="small" onClick={() => handleCopy(route.next_hop || route.via || "")}>
                      {copiedText === (route.next_hop || route.via) ? <Check size={12} color="#10B981" /> : <Copy size={12} />}
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
            </Box>

            {route.interface && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", display: "block", textTransform: "uppercase", fontSize: "0.68rem", fontWeight: 600 }}>
                  Interface
                </Typography>
                <Chip
                  label={route.interface}
                  size="small"
                  sx={{
                    backgroundColor: "#F1F5F9",
                    color: "#0F172A",
                    fontFamily: "monospace",
                    fontWeight: 600,
                    fontSize: "0.72rem",
                    height: 22,
                  }}
                />
              </Box>
            )}

            {route.origin && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", display: "block", textTransform: "uppercase", fontSize: "0.68rem", fontWeight: 600 }}>
                  Origin
                </Typography>
                <Typography variant="body2" sx={{ color: "#334155", fontWeight: 500, fontSize: "0.825rem" }}>
                  {route.origin}
                </Typography>
              </Box>
            )}
          </Box>

          {/* AS-Path Chain */}
          {route.as_path && route.as_path.length > 0 && (
            <Box sx={{ mt: 0.5 }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block", textTransform: "uppercase", fontSize: "0.68rem", fontWeight: 600, mb: 0.5 }}>
                AS-Path Sequence ({route.as_path.length} Hops)
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 1 }}>
                {route.as_path.map((asn, aIdx) => (
                  <React.Fragment key={`${asn}-${aIdx}`}>
                    <Chip
                      label={`AS${asn}`}
                      size="small"
                      onClick={() => onQueryASN ? onQueryASN(`AS${asn}`) : handleCopy(`AS${asn}`)}
                      sx={{
                        backgroundColor: aIdx === route.as_path!.length - 1 ? "#EEF2FF" : "#F8FAFC",
                        color: aIdx === route.as_path!.length - 1 ? "#4338CA" : "#334155",
                        border: aIdx === route.as_path!.length - 1 ? "1px solid #C7D2FE" : "1px solid #E2E8F0",
                        fontWeight: 600,
                        fontFamily: "monospace",
                        fontSize: "0.75rem",
                        cursor: "pointer",
                        "&:hover": {
                          backgroundColor: "#E0E7FF",
                        },
                      }}
                    />
                    {aIdx < route.as_path!.length - 1 && (
                      <ArrowRight size={14} color="#94A3B8" />
                    )}
                  </React.Fragment>
                ))}
              </Box>
            </Box>
          )}

          {/* BGP Communities */}
          {route.communities && route.communities.length > 0 && (
            <Box sx={{ mt: 0.5 }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block", textTransform: "uppercase", fontSize: "0.68rem", fontWeight: 600, mb: 0.5 }}>
                BGP Communities ({route.communities.length})
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 0.8 }}>
                {route.communities.map((comm, cIdx) => (
                  <Chip
                    key={cIdx}
                    label={comm}
                    size="small"
                    sx={{
                      backgroundColor: "#F3F4F6",
                      color: "#374151",
                      fontSize: "0.72rem",
                      fontFamily: "monospace",
                      height: 22,
                    }}
                  />
                ))}
              </Box>
            </Box>
          )}
        </Paper>
      ))}
    </Box>
  );
};
