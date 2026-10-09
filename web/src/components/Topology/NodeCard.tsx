import React, { memo } from "react";
import { Handle, Position, NodeProps } from "@xyflow/react";
import { Box, Typography, Chip, IconButton, Tooltip, CircularProgress } from "@mui/material";
import {
  Server,
  MoreVertical,
  Globe,
  HardDrive,
  Tag,
  AlertTriangle,
  RefreshCw,
  Share2,
  CheckCircle2,
  Network,
  Cpu,
  Activity,
  Camera,
} from "lucide-react";
import { Node, NodeStatus } from "../../types/api";

const formatBytes = (bytes?: number): string => {
  if (!bytes || isNaN(bytes) || bytes <= 0) return "0B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val >= 10 || i === 0 ? val.toFixed(0) : val.toFixed(1)} ${units[i]}`;
};

const formatUptime = (seconds?: number): string => {
  if (!seconds || seconds <= 0) return "Just started";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
};

export interface NodeData {
  node: Node;
  status?: NodeStatus;
  blockName?: string;
  blockColor?: string;
  inBlock?: boolean;
  isBlockFullMesh?: boolean;
  isHealthy?: boolean;
  healthDetails?: {
    upLinks: number;
    totalLinks: number;
    downLinks: number;
    reason?: string;
  };
  isFocused?: boolean;
  onSelect?: (node: Node) => void;
  onRefreshNode?: (nodeName: string) => void;
  refreshingNodeName?: string | null;
  [key: string]: unknown;
}

export const NodeCard: React.FC<NodeProps> = memo(({ data }) => {
  const nodeData = data as unknown as NodeData;
  const {
    node,
    status,
    blockName,
    blockColor,
    inBlock,
    isBlockFullMesh,
    isHealthy = true,
    healthDetails,
    isFocused,
    onRefreshNode,
    refreshingNodeName,
  } = nodeData;
  const isOnline = status ? status.connected : true;
  const isExternal = Boolean(node.is_external);
  const isRefreshing = refreshingNodeName === node.name;
  const isAgentConfigured = node.mode === "agent" || Boolean(node.agent_token);
  const isAgentMode = status?.mode === "agent" || isAgentConfigured;
  const isAgentWorking = isAgentMode && isOnline && !isExternal;

  return (
    <Box
      sx={{
        width: 260,
        backgroundColor:
          !isOnline && !isExternal
            ? "#FFFDFD"
            : inBlock && !isHealthy
              ? "#FFFDFD"
              : inBlock && !isBlockFullMesh
                ? "#FAFBFD"
                : "#FFFFFF",
        border: isFocused
          ? "2px solid #4F46E5"
          : isExternal
            ? "2px dashed #8B5CF6"
            : inBlock
              ? isBlockFullMesh
                ? isHealthy
                  ? `2px solid ${blockColor || "#6366F1"}`
                  : "2px solid #EF4444"
                : isHealthy
                  ? "2px dashed #94A3B8"
                  : "2px dashed #EF4444"
              : !isOnline
                ? "1.5px solid #EF4444"
                : isAgentWorking
                  ? "1.5px solid #10B981"
                  : "1.5px solid #E2E8F0",
        borderTop:
          inBlock && isBlockFullMesh
            ? `4px solid ${isFocused ? "#4F46E5" : !isHealthy ? "#EF4444" : blockColor || "#6366F1"}`
            : isAgentWorking && (!inBlock || !isBlockFullMesh)
              ? "3px solid #10B981"
              : undefined,
        borderRadius: 2.5,
        boxShadow: isFocused
          ? "0 0 0 3px rgba(79, 70, 229, 0.25), 0 12px 28px rgba(79, 70, 229, 0.2)"
          : inBlock && isBlockFullMesh && isHealthy
            ? `0 4px 14px ${blockColor || "#6366F1"}25, 0 1px 3px rgba(0, 0, 0, 0.05)`
            : inBlock && !isHealthy
              ? "0 4px 14px rgba(239, 68, 68, 0.15), 0 1px 3px rgba(239, 68, 68, 0.08)"
              : isExternal
                ? "0 4px 6px -1px rgba(139, 92, 246, 0.08), 0 2px 4px -2px rgba(139, 92, 246, 0.05)"
                : !isOnline
                  ? "0 4px 10px rgba(239, 68, 68, 0.15), 0 2px 4px rgba(239, 68, 68, 0.1)"
                  : isAgentWorking
                    ? "0 4px 12px -1px rgba(16, 185, 129, 0.12), 0 2px 4px -2px rgba(16, 185, 129, 0.08)"
                    : "0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -2px rgba(0, 0, 0, 0.05)",
        overflow: "hidden",
        cursor: "pointer",
        transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
        "&:hover": {
          transform: "translateY(-2px)",
          borderColor: isFocused
            ? "#4F46E5"
            : inBlock && isBlockFullMesh
              ? blockColor || "#4F46E5"
              : isExternal
                ? "#7C3AED"
                : !isOnline
                  ? "#DC2626"
                  : isAgentWorking
                    ? "#059669"
                    : "#4F46E5",
          boxShadow: isFocused
            ? "0 0 0 3px rgba(79, 70, 229, 0.35), 0 16px 32px rgba(79, 70, 229, 0.25)"
            : inBlock && isBlockFullMesh
              ? `0 8px 20px ${blockColor || "#6366F1"}30`
              : isAgentWorking
                ? "0 10px 22px -3px rgba(16, 185, 129, 0.22), 0 4px 6px -4px rgba(16, 185, 129, 0.15)"
                : "0 10px 15px -3px rgba(79, 70, 229, 0.12), 0 4px 6px -4px rgba(79, 70, 229, 0.12)",
        },
      }}
    >
      {/* Handles for connections */}
      <Handle
        type="target"
        position={Position.Left}
        id="target"
        style={{
          width: 10,
          height: 10,
          backgroundColor: isExternal ? "#8B5CF6" : !isOnline ? "#EF4444" : "#4F46E5",
          border: "2px solid #FFFFFF",
        }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="source"
        style={{
          width: 10,
          height: 10,
          backgroundColor: isExternal ? "#8B5CF6" : !isOnline ? "#EF4444" : "#0891B2",
          border: "2px solid #FFFFFF",
        }}
      />

      {/* Header */}
      <Box
        sx={{
          p: 1.5,
          backgroundColor: isExternal ? "#FAF5FF" : !isOnline ? "#FEF2F2" : "#F8FAFC",
          borderBottom: "1px solid",
          borderColor: isExternal ? "rgba(139, 92, 246, 0.2)" : !isOnline ? "#FECDD3" : "#E2E8F0",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box
            sx={{
              width: 28,
              height: 28,
              borderRadius: 1.5,
              backgroundColor: isExternal
                ? "rgba(139, 92, 246, 0.15)"
                : !isOnline || (inBlock && !isHealthy)
                  ? "rgba(239, 68, 68, 0.15)"
                  : inBlock && isHealthy
                    ? "rgba(16, 185, 129, 0.12)"
                    : "rgba(79, 70, 229, 0.1)",
              color: isExternal
                ? "#7C3AED"
                : !isOnline || (inBlock && !isHealthy)
                  ? "#DC2626"
                  : inBlock && isHealthy
                    ? "#059669"
                    : "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              position: "relative",
            }}
          >
            {isExternal ? <Globe size={16} /> : <Server size={16} />}
            {inBlock && (
              <Box
                sx={{
                  position: "absolute",
                  bottom: -2,
                  right: -2,
                  width: 8,
                  height: 8,
                  borderRadius: "50%",
                  backgroundColor: isHealthy ? "#10B981" : "#EF4444",
                  border: "1.5px solid #FFFFFF",
                  boxShadow: isHealthy ? "0 0 4px #10B981" : "0 0 4px #EF4444",
                }}
              />
            )}
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, lineHeight: 1.2, color: "#0F172A" }}>
                {node.name}
              </Typography>
              {blockName && (
                <Chip
                  label={blockName}
                  size="small"
                  sx={{
                    height: 16,
                    fontSize: "0.55rem",
                    fontWeight: 700,
                    backgroundColor: `${blockColor || "#6366F1"}18`,
                    color: blockColor || "#6366F1",
                    border: `1px solid ${blockColor || "#6366F1"}40`,
                    letterSpacing: "0.3px",
                    px: 0.25,
                    maxWidth: 80,
                    "& .MuiChip-label": { px: 0.5, overflow: "hidden", textOverflow: "ellipsis" },
                  }}
                />
              )}
              {isExternal ? (
                <Chip
                  label="EXTERNAL"
                  size="small"
                  sx={{
                    height: 16,
                    fontSize: "0.55rem",
                    fontWeight: 800,
                    backgroundColor: "rgba(139, 92, 246, 0.15)",
                    color: "#7C3AED",
                    border: "1px solid rgba(139, 92, 246, 0.3)",
                    letterSpacing: "0.5px",
                    px: 0,
                  }}
                />
              ) : !isOnline ? (
                <Tooltip
                  title={
                    isAgentMode
                      ? `Agent offline or unreachable. Last seen: ${
                          status?.last_seen ? new Date(status.last_seen).toLocaleString() : node.agent_last_seen || "Unknown"
                        }`
                      : status?.error || "Device is offline or unreachable via SSH"
                  }
                >
                  <Chip
                    icon={<AlertTriangle size={10} color="#DC2626" />}
                    label={isAgentMode ? "AGENT OFFLINE" : "OFFLINE"}
                    size="small"
                    sx={{
                      height: 16,
                      fontSize: "0.55rem",
                      fontWeight: 800,
                      backgroundColor: "#FEE2E2",
                      color: "#DC2626",
                      border: "1px solid #FECDD3",
                      letterSpacing: "0.5px",
                      px: 0,
                    }}
                  />
                </Tooltip>
              ) : isAgentWorking ? (
                <Tooltip title="Agent connected & reporting live telemetry in real-time. Link states and node metrics are continuously up to date.">
                  <Chip
                    icon={
                      <Box
                        sx={{
                          width: 6,
                          height: 6,
                          borderRadius: "50%",
                          backgroundColor: "#10B981",
                          ml: 0.5,
                          boxShadow: "0 0 4px #10B981",
                        }}
                      />
                    }
                    label="LIVE"
                    size="small"
                    sx={{
                      height: 16,
                      fontSize: "0.55rem",
                      fontWeight: 800,
                      backgroundColor: "#ECFDF5",
                      color: "#059669",
                      border: "1px solid #A7F3D0",
                      letterSpacing: "0.5px",
                      px: 0,
                      "& .MuiChip-label": { px: 0.5 },
                    }}
                  />
                </Tooltip>
              ) : (
                <Tooltip
                  title={`SSH Snapshot (refreshed: ${
                    status?.last_seen ? new Date(status.last_seen).toLocaleTimeString() : "manual"
                  }). Not reporting live telemetry; for reference only.`}
                >
                  <Chip
                    icon={<Camera size={9} style={{ color: "#64748B", marginLeft: 4 }} />}
                    label="SNAPSHOT"
                    size="small"
                    sx={{
                      height: 16,
                      fontSize: "0.52rem",
                      fontWeight: 700,
                      backgroundColor: "#F1F5F9",
                      color: "#64748B",
                      border: "1px solid #CBD5E1",
                      letterSpacing: "0.3px",
                      px: 0,
                      "& .MuiChip-label": { px: 0.4 },
                    }}
                  />
                </Tooltip>
              )}
            </Box>
            <Typography
              variant="caption"
              sx={{ color: isExternal ? "#7C3AED" : !isOnline ? "#DC2626" : "#64748B", fontSize: "0.7rem" }}
            >
              {isExternal ? node.description || "Unmanaged Peer" : node.host || "No SSH Host"}
            </Typography>

            {/* In-Block State Badges: Full-Mesh vs Non-Full-Mesh & Healthy vs Unhealthy */}
            {inBlock && (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, mt: 0.5, flexWrap: "wrap" }}>
                {/* Full-Mesh / Non-Full-Mesh */}
                {isBlockFullMesh ? (
                  <Tooltip title="Full-Mesh Node: Directly connected to all peers in the block's maximum mesh core.">
                    <Chip
                      icon={<Share2 size={10} style={{ color: blockColor || "#6366F1", marginLeft: 4 }} />}
                      label="Full-Mesh"
                      size="small"
                      sx={{
                        height: 18,
                        fontSize: "0.62rem",
                        fontWeight: 800,
                        backgroundColor: `${blockColor || "#6366F1"}18`,
                        color: blockColor || "#6366F1",
                        border: `1px solid ${blockColor || "#6366F1"}50`,
                        letterSpacing: "0.2px",
                        "& .MuiChip-label": { px: 0.5 },
                      }}
                    />
                  </Tooltip>
                ) : (
                  <Tooltip title="Non-Full-Mesh Node: Incomplete mesh; lacks direct links to some nodes in this block.">
                    <Chip
                      icon={<Network size={10} style={{ color: "#64748B", marginLeft: 4 }} />}
                      label="Non-Mesh"
                      size="small"
                      sx={{
                        height: 18,
                        fontSize: "0.62rem",
                        fontWeight: 700,
                        backgroundColor: "#F1F5F9",
                        color: "#64748B",
                        border: "1px dashed #CBD5E1",
                        letterSpacing: "0.2px",
                        "& .MuiChip-label": { px: 0.5 },
                      }}
                    />
                  </Tooltip>
                )}

                {/* Healthy / Unhealthy */}
                {isHealthy ? (
                  <Tooltip title={`Healthy: All ${healthDetails?.totalLinks ?? 0} link(s) are active and normal.`}>
                    <Chip
                      icon={<CheckCircle2 size={10} style={{ color: "#059669", marginLeft: 4 }} />}
                      label="Healthy"
                      size="small"
                      sx={{
                        height: 18,
                        fontSize: "0.62rem",
                        fontWeight: 800,
                        backgroundColor: "#ECFDF5",
                        color: "#059669",
                        border: "1px solid #A7F3D0",
                        letterSpacing: "0.2px",
                        "& .MuiChip-label": { px: 0.5 },
                      }}
                    />
                  </Tooltip>
                ) : (
                  <Tooltip title={healthDetails?.reason || "Unhealthy: One or more links down or node offline."}>
                    <Chip
                      icon={<AlertTriangle size={10} style={{ color: "#DC2626", marginLeft: 4 }} />}
                      label={healthDetails?.downLinks ? `${healthDetails.downLinks} Down` : "Unhealthy"}
                      size="small"
                      sx={{
                        height: 18,
                        fontSize: "0.62rem",
                        fontWeight: 800,
                        backgroundColor: "#FEF2F2",
                        color: "#DC2626",
                        border: "1px solid #FECDD3",
                        letterSpacing: "0.2px",
                        "& .MuiChip-label": { px: 0.5 },
                      }}
                    />
                  </Tooltip>
                )}
              </Box>
            )}
          </Box>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.25 }}>
          {onRefreshNode && !isExternal && (
            <Tooltip title={`Refresh state & links for ${node.name}`}>
              <IconButton
                size="small"
                onClick={(e) => {
                  e.stopPropagation();
                  onRefreshNode(node.name);
                }}
                disabled={isRefreshing}
                sx={{
                  color: isRefreshing ? "#4F46E5" : "#94A3B8",
                  p: 0.5,
                  "&:hover": { color: "#0284C7", backgroundColor: "rgba(2, 132, 199, 0.08)" },
                }}
              >
                {isRefreshing ? <CircularProgress size={13} color="inherit" /> : <RefreshCw size={14} />}
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="Node Details">
            <IconButton size="small" sx={{ color: "#94A3B8", p: 0.5 }}>
              <MoreVertical size={15} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Body */}
      <Box sx={{ p: 1.5, display: "flex", flexDirection: "column", gap: 1 }}>
        {/* Main IP & Iface */}
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, minWidth: 0 }}>
            <Globe size={13} color="#64748B" />
            <Typography
              variant="body2"
              className="mono-font"
              noWrap
              sx={{ fontWeight: 600, fontSize: "0.8rem", color: isExternal ? "#7C3AED" : "#0891B2" }}
            >
              {node.ip || (isExternal ? "Unspecified IP" : "No IP")}
            </Typography>
          </Box>
          {!isExternal && node.interface && (
            <Chip
              icon={<HardDrive size={10} />}
              label={node.interface}
              size="small"
              sx={{
                height: 20,
                fontSize: "0.65rem",
                backgroundColor: "#F1F5F9",
                color: "#475569",
                border: "1px solid #E2E8F0",
              }}
            />
          )}
        </Box>

        {node.ip6 && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mt: -0.5 }}>
            <Globe size={11} color="#94A3B8" />
            <Typography
              variant="caption"
              className="mono-font"
              noWrap
              sx={{ fontWeight: 500, fontSize: "0.72rem", color: "#64748B" }}
              title={`Main IPv6: ${node.ip6}`}
            >
              {node.ip6}
            </Typography>
          </Box>
        )}

        {/* ASN & Entrypoints */}
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mt: 0.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Chip
              label={`AS${node.asn}`}
              size="small"
              sx={{
                height: 22,
                fontSize: "0.7rem",
                fontWeight: 700,
                backgroundColor: isExternal ? "rgba(139, 92, 246, 0.1)" : "rgba(79, 70, 229, 0.08)",
                color: isExternal ? "#7C3AED" : "#4338CA",
                border: isExternal ? "1px solid rgba(139, 92, 246, 0.3)" : "1px solid rgba(79, 70, 229, 0.2)",
              }}
            />
            {node.external_table && node.external_table !== (node.table ?? 254) ? (
              <Tooltip title={`External Table: ${node.external_table}`}>
                <Chip
                  label={`Ext T${node.external_table}`}
                  size="small"
                  sx={{
                    height: 22,
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    backgroundColor: "rgba(139, 92, 246, 0.1)",
                    color: "#7C3AED",
                    border: "1px solid rgba(139, 92, 246, 0.3)",
                  }}
                />
              </Tooltip>
            ) : null}
            {node.internet_table && node.internet_table !== (node.table ?? 254) ? (
              <Tooltip title={`Internet Table: ${node.internet_table}`}>
                <Chip
                  label={`Inet T${node.internet_table}`}
                  size="small"
                  sx={{
                    height: 22,
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    backgroundColor: "rgba(16, 185, 129, 0.1)",
                    color: "#059669",
                    border: "1px solid rgba(16, 185, 129, 0.3)",
                  }}
                />
              </Tooltip>
            ) : null}
            {node.metric !== undefined && node.metric !== null ? (
              <Tooltip title={`Kernel Route Metric: ${node.metric}`}>
                <Chip
                  label={`Metric ${node.metric}`}
                  size="small"
                  sx={{
                    height: 22,
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    backgroundColor: "rgba(59, 130, 246, 0.1)",
                    color: "#2563EB",
                    border: "1px solid rgba(59, 130, 246, 0.3)",
                  }}
                />
              </Tooltip>
            ) : null}
          </Box>

          <Typography
            variant="caption"
            sx={{ color: isExternal ? "#7C3AED" : "#64748B", fontSize: "0.7rem", fontWeight: isExternal ? 600 : 400 }}
          >
            {`${node.entrypoints?.filter((e) => e.ip && e.ip !== "").length || 0} Endpoints`}
          </Typography>
        </Box>

        {/* Node Tags */}
        {node.tags && node.tags.length > 0 && (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5, alignItems: "center" }}>
            <Tag size={11} color="#64748B" />
            {node.tags.map((tag) => (
              <Chip
                key={tag}
                label={`#${tag}`}
                size="small"
                sx={{
                  height: 18,
                  fontSize: "0.625rem",
                  fontWeight: 600,
                  backgroundColor: "rgba(8, 145, 178, 0.08)",
                  color: "#0891B2",
                  border: "1px solid rgba(8, 145, 178, 0.25)",
                  borderRadius: "4px",
                  px: 0.2,
                }}
              />
            ))}
          </Box>
        )}
      </Box>

      {/* Live State Bottom Line (Present when agent is working) */}
      {isAgentWorking && (
        <Tooltip
          arrow
          placement="bottom"
          title={
            <Box sx={{ p: 0.5 }}>
              <Typography
                variant="caption"
                sx={{ fontWeight: 800, display: "block", color: "#6EE7B7", mb: 0.5, letterSpacing: "0.3px" }}
              >
                LIVE NODE METRICS
              </Typography>
              <Box sx={{ display: "grid", gridTemplateColumns: "auto auto", gap: "3px 12px", fontSize: "0.72rem" }}>
                <span style={{ color: "#94A3B8" }}>CPU Usage:</span>
                <span style={{ fontWeight: 700, color: "#FFFFFF" }}>
                  {status?.metrics ? `${status.metrics.cpu_percent.toFixed(1)}%` : "Awaiting..."}
                </span>
                <span style={{ color: "#94A3B8" }}>RAM Usage:</span>
                <span style={{ fontWeight: 700, color: "#FFFFFF" }}>
                  {status?.metrics && status.metrics.memory_total_bytes > 0
                    ? `${formatBytes(status.metrics.memory_used_bytes)} / ${formatBytes(
                        status.metrics.memory_total_bytes,
                      )} (${Math.round((status.metrics.memory_used_bytes / status.metrics.memory_total_bytes) * 100)}%)`
                    : "Awaiting..."}
                </span>
                {status?.metrics?.load_avg && status.metrics.load_avg.length > 0 && (
                  <>
                    <span style={{ color: "#94A3B8" }}>Load Average:</span>
                    <span style={{ fontWeight: 700, color: "#FFFFFF" }}>
                      {status.metrics.load_avg.map((v) => v.toFixed(2)).join(", ")}
                    </span>
                  </>
                )}
                {status?.metrics?.uptime_seconds ? (
                  <>
                    <span style={{ color: "#94A3B8" }}>Uptime:</span>
                    <span style={{ fontWeight: 700, color: "#FFFFFF" }}>
                      {formatUptime(status.metrics.uptime_seconds)}
                    </span>
                  </>
                ) : null}
                {status?.agent_version && (
                  <>
                    <span style={{ color: "#94A3B8" }}>Agent:</span>
                    <span style={{ fontWeight: 700, color: "#FFFFFF" }}>{status.agent_version}</span>
                  </>
                )}
                <span style={{ color: "#94A3B8" }}>Status:</span>
                <span style={{ fontWeight: 700, color: "#10B981" }}>Continuous Real-Time</span>
              </Box>
            </Box>
          }
        >
          <Box
            sx={{
              px: 1.25,
              py: 0.6,
              backgroundColor: "rgba(16, 185, 129, 0.06)",
              borderTop: "1px solid rgba(16, 185, 129, 0.22)",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: 1,
            }}
          >
            {/* Live Indicator */}
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, flexShrink: 0 }}>
              <Box
                sx={{
                  width: 6,
                  height: 6,
                  borderRadius: "50%",
                  backgroundColor: "#10B981",
                  boxShadow: "0 0 0 0 rgba(16, 185, 129, 0.7)",
                  animation: "livePulse 2s infinite",
                  "@keyframes livePulse": {
                    "0%": { boxShadow: "0 0 0 0 rgba(16, 185, 129, 0.7)" },
                    "70%": { boxShadow: "0 0 0 4px rgba(16, 185, 129, 0)" },
                    "100%": { boxShadow: "0 0 0 0 rgba(16, 185, 129, 0)" },
                  },
                }}
              />
              <Typography
                variant="caption"
                sx={{
                  fontSize: "0.62rem",
                  fontWeight: 800,
                  color: "#059669",
                  letterSpacing: "0.5px",
                  lineHeight: 1,
                }}
              >
                LIVE
              </Typography>
            </Box>

            {/* Metrics Pills */}
            {status?.metrics ? (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, minWidth: 0 }}>
                {/* CPU Pill */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 0.35,
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    color:
                      status.metrics.cpu_percent > 85
                        ? "#DC2626"
                        : status.metrics.cpu_percent > 60
                          ? "#D97706"
                          : "#047857",
                    backgroundColor: "#FFFFFF",
                    px: 0.6,
                    py: 0.15,
                    borderRadius: 1,
                    border: "1px solid rgba(16, 185, 129, 0.25)",
                    lineHeight: 1.2,
                  }}
                >
                  <Cpu size={10} style={{ opacity: 0.8 }} />
                  <span className="mono-font">{Math.round(status.metrics.cpu_percent)}%</span>
                </Box>

                {/* RAM Pill */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 0.35,
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    color: "#047857",
                    backgroundColor: "#FFFFFF",
                    px: 0.6,
                    py: 0.15,
                    borderRadius: 1,
                    border: "1px solid rgba(16, 185, 129, 0.25)",
                    lineHeight: 1.2,
                  }}
                >
                  <Activity size={10} style={{ opacity: 0.8 }} />
                  <span className="mono-font">
                    {formatBytes(status.metrics.memory_used_bytes)}
                    {status.metrics.memory_total_bytes > 0 &&
                      ` (${Math.round((status.metrics.memory_used_bytes / status.metrics.memory_total_bytes) * 100)}%)`}
                  </span>
                </Box>
              </Box>
            ) : (
              <Typography
                variant="caption"
                sx={{
                  fontSize: "0.62rem",
                  color: "#059669",
                  fontWeight: 600,
                }}
              >
                reporting live
              </Typography>
            )}
          </Box>
        </Tooltip>
      )}

      {/* Offline Alert Strip */}
      {!isExternal && !isOnline && (
        <Box
          sx={{
            px: 1.5,
            py: 0.75,
            backgroundColor: "#FEF2F2",
            borderTop: "1px solid #FECDD3",
            display: "flex",
            alignItems: "center",
            gap: 0.75,
          }}
        >
          <AlertTriangle size={12} color="#DC2626" style={{ flexShrink: 0 }} />
          <Tooltip title={status?.error || "Device unreachable via SSH"}>
            <Typography
              variant="caption"
              sx={{
                color: "#B91C1C",
                fontSize: "0.68rem",
                fontWeight: 600,
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              {status?.error ? `Unreachable: ${status.error}` : "Unreachable via SSH"}
            </Typography>
          </Tooltip>
        </Box>
      )}
    </Box>
  );
});
