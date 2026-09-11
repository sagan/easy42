import React, { memo } from "react";
import { Handle, Position, NodeProps } from "@xyflow/react";
import { Box, Typography, Chip, IconButton, Tooltip, CircularProgress } from "@mui/material";
import { Server, MoreVertical, Globe, HardDrive, Tag, AlertTriangle, RefreshCw } from "lucide-react";
import { Node, NodeStatus } from "../../types/api";

export interface NodeData {
  node: Node;
  status?: NodeStatus;
  onSelect: (node: Node) => void;
  onRefreshNode?: (nodeName: string) => void;
  refreshingNodeName?: string | null;
  [key: string]: unknown;
}

export const NodeCard: React.FC<NodeProps> = memo(({ data }) => {
  const nodeData = data as unknown as NodeData;
  const { node, status, onSelect, onRefreshNode, refreshingNodeName } = nodeData;
  const isOnline = status ? status.connected : true;
  const isExternal = Boolean(node.is_external);
  const isRefreshing = refreshingNodeName === node.name;

  return (
    <Box
      onClick={() => onSelect(node)}
      sx={{
        width: 260,
        backgroundColor: !isOnline && !isExternal ? "#FFFDFD" : "#FFFFFF",
        border: isExternal ? "2px dashed #8B5CF6" : "1.5px solid",
        borderColor: isExternal ? "#8B5CF6" : isOnline ? "#E2E8F0" : "#EF4444",
        borderRadius: 2.5,
        boxShadow: isExternal
          ? "0 4px 6px -1px rgba(139, 92, 246, 0.08), 0 2px 4px -2px rgba(139, 92, 246, 0.05)"
          : !isOnline
          ? "0 4px 10px rgba(239, 68, 68, 0.15), 0 2px 4px rgba(239, 68, 68, 0.1)"
          : "0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -2px rgba(0, 0, 0, 0.05)",
        overflow: "hidden",
        cursor: "pointer",
        transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
        "&:hover": {
          transform: "translateY(-2px)",
          borderColor: isExternal ? "#7C3AED" : !isOnline ? "#DC2626" : "#4F46E5",
          boxShadow: isExternal
            ? "0 10px 15px -3px rgba(139, 92, 246, 0.2), 0 4px 6px -4px rgba(139, 92, 246, 0.15)"
            : !isOnline
            ? "0 10px 15px -3px rgba(239, 68, 68, 0.25)"
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
                : !isOnline
                ? "rgba(239, 68, 68, 0.15)"
                : "rgba(79, 70, 229, 0.1)",
              color: isExternal ? "#7C3AED" : !isOnline ? "#DC2626" : "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            {isExternal ? <Globe size={16} /> : <Server size={16} />}
          </Box>
          <Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, lineHeight: 1.2, color: "#0F172A" }}>
                {node.name}
              </Typography>
              {isExternal && (
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
              )}
              {!isExternal && !isOnline && (
                <Tooltip title={status?.error || "Device is offline or unreachable via SSH"}>
                  <Chip
                    icon={<AlertTriangle size={10} color="#DC2626" />}
                    label="OFFLINE"
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
              )}
            </Box>
            <Typography variant="caption" sx={{ color: isExternal ? "#7C3AED" : !isOnline ? "#DC2626" : "#64748B", fontSize: "0.7rem" }}>
              {isExternal ? node.description || "Unmanaged Peer" : node.host || "No SSH Host"}
            </Typography>
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
          </Box>

          <Typography variant="caption" sx={{ color: isExternal ? "#7C3AED" : "#64748B", fontSize: "0.7rem", fontWeight: isExternal ? 600 : 400 }}>
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
