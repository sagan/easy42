import React, { memo } from "react";
import { Handle, Position, NodeProps } from "@xyflow/react";
import { Box, Typography, Chip, IconButton, Tooltip } from "@mui/material";
import { Server, MoreVertical, Globe, HardDrive, Tag } from "lucide-react";
import { Node, NodeStatus } from "../../types/api";

export interface NodeData {
  node: Node;
  status?: NodeStatus;
  onSelect: (node: Node) => void;
  [key: string]: unknown;
}

export const NodeCard: React.FC<NodeProps> = memo(({ data }) => {
  const nodeData = data as unknown as NodeData;
  const { node, status, onSelect } = nodeData;
  const isOnline = status ? status.connected : true;
  const isExternal = Boolean(node.is_external);

  return (
    <Box
      onClick={() => onSelect(node)}
      sx={{
        width: 260,
        backgroundColor: "#FFFFFF",
        border: isExternal ? "2px dashed #8B5CF6" : "1px solid",
        borderColor: isExternal ? "#8B5CF6" : isOnline ? "#E2E8F0" : "rgba(225, 29, 72, 0.4)",
        borderRadius: 2.5,
        boxShadow: isExternal
          ? "0 4px 6px -1px rgba(139, 92, 246, 0.08), 0 2px 4px -2px rgba(139, 92, 246, 0.05)"
          : "0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -2px rgba(0, 0, 0, 0.05)",
        overflow: "hidden",
        cursor: "pointer",
        transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
        "&:hover": {
          transform: "translateY(-2px)",
          borderColor: isExternal ? "#7C3AED" : "#4F46E5",
          boxShadow: isExternal
            ? "0 10px 15px -3px rgba(139, 92, 246, 0.2), 0 4px 6px -4px rgba(139, 92, 246, 0.15)"
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
          backgroundColor: isExternal ? "#8B5CF6" : "#4F46E5",
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
          backgroundColor: isExternal ? "#8B5CF6" : "#0891B2",
          border: "2px solid #FFFFFF",
        }}
      />

      {/* Header */}
      <Box
        sx={{
          p: 1.5,
          backgroundColor: isExternal ? "#FAF5FF" : "#F8FAFC",
          borderBottom: "1px solid",
          borderColor: isExternal ? "rgba(139, 92, 246, 0.2)" : "#E2E8F0",
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
              backgroundColor: isExternal ? "rgba(139, 92, 246, 0.15)" : "rgba(79, 70, 229, 0.1)",
              color: isExternal ? "#7C3AED" : "#4F46E5",
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
            </Box>
            <Typography variant="caption" sx={{ color: isExternal ? "#7C3AED" : "#64748B", fontSize: "0.7rem" }}>
              {isExternal ? node.description || "Unmanaged Peer" : node.host || "No SSH Host"}
            </Typography>
          </Box>
        </Box>

        <Tooltip title="Node Details">
          <IconButton size="small" sx={{ color: "#94A3B8" }}>
            <MoreVertical size={16} />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Body */}
      <Box sx={{ p: 1.5, display: "flex", flexDirection: "column", gap: 1 }}>
        {/* Main IP & Iface */}
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
            <Globe size={13} color="#64748B" />
            <Typography
              variant="body2"
              className="mono-font"
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

        {/* ASN & Entrypoints */}
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mt: 0.5 }}>
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

          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
            {isExternal ? "BGP Peer" : `${node.entrypoints?.filter((e) => e.ip && e.ip !== "").length || 0} Endpoints`}
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
    </Box>
  );
});
