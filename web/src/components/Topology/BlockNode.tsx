import React, { memo, useState } from "react";
import { NodeProps, NodeResizer } from "@xyflow/react";
import {
  Box,
  Typography,
  IconButton,
  Tooltip,
  TextField,
  Chip,
  Menu,
  MenuItem,
} from "@mui/material";
import {
  Layers,
  Trash2,
  Edit3,
  Check,
  EyeOff,
  AlertTriangle,
  Palette,
  Share2,
  CheckCircle2,
} from "lucide-react";
import { GraphBlock } from "../../types/api";

export const BLOCK_PALETTE = [
  { name: "Indigo", color: "#6366F1", bg: "rgba(99, 102, 241, 0.04)", border: "#6366F1" },
  { name: "Cyan", color: "#06B6D4", bg: "rgba(6, 182, 212, 0.04)", border: "#06B6D4" },
  { name: "Emerald", color: "#10B981", bg: "rgba(16, 185, 129, 0.04)", border: "#10B981" },
  { name: "Amber", color: "#F59E0B", bg: "rgba(245, 158, 11, 0.04)", border: "#F59E0B" },
  { name: "Purple", color: "#A855F7", bg: "rgba(168, 85, 247, 0.04)", border: "#A855F7" },
  { name: "Rose", color: "#F43F5E", bg: "rgba(244, 63, 94, 0.04)", border: "#F43F5E" },
];

export interface BlockNodeData {
  block: GraphBlock;
  memberCount: number;
  hiddenLinkCount: number;
  brokenLinkCount: number;
  fullMeshCount?: number;
  healthyCount?: number;
  onRenameBlock: (id: string, newName: string) => void;
  onDeleteBlock: (id: string) => void;
  onChangeColor: (id: string, color: string) => void;
  [key: string]: unknown;
}

export const BlockNode: React.FC<NodeProps> = memo(({ data, selected }) => {
  const nodeData = data as unknown as BlockNodeData;
  const {
    block,
    memberCount = 0,
    hiddenLinkCount = 0,
    brokenLinkCount = 0,
    fullMeshCount = 0,
    healthyCount = 0,
    onRenameBlock,
    onDeleteBlock,
    onChangeColor,
  } = nodeData;

  const [isEditing, setIsEditing] = useState(false);
  const [nameValue, setNameValue] = useState(block?.name || "Block");
  const [paletteAnchor, setPaletteAnchor] = useState<null | HTMLElement>(null);

  const blockColor = block?.color || "#6366F1";
  const paletteMatch = BLOCK_PALETTE.find((p) => p.color === blockColor) || {
    name: "Custom",
    color: blockColor,
    bg: "rgba(99, 102, 241, 0.04)",
    border: blockColor,
  };

  const handleSaveName = () => {
    setIsEditing(false);
    const trimmed = nameValue.trim();
    if (trimmed && trimmed !== block.name) {
      onRenameBlock?.(block.id, trimmed);
    } else {
      setNameValue(block.name);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      handleSaveName();
    } else if (e.key === "Escape") {
      setNameValue(block.name);
      setIsEditing(false);
    }
  };

  return (
    <Box
      sx={{
        width: "100%",
        height: "100%",
        minWidth: 260,
        minHeight: 180,
        position: "relative",
        borderRadius: 3,
        border: selected ? `2px solid ${blockColor}` : `2px dashed ${blockColor}`,
        backgroundColor: paletteMatch.bg,
        boxShadow: selected
          ? `0 0 0 1px ${blockColor}, 0 10px 25px -5px rgba(0, 0, 0, 0.08)`
          : "0 4px 12px rgba(0, 0, 0, 0.02)",
        transition: "border-color 0.2s, box-shadow 0.2s",
        display: "flex",
        flexDirection: "column",
        userSelect: "none",
        pointerEvents: "none",
      }}
    >
      <NodeResizer
        color={blockColor}
        isVisible={Boolean(selected)}
        minWidth={260}
        minHeight={180}
        handleStyle={{ width: 8, height: 8, borderRadius: 2, pointerEvents: "all" }}
        lineStyle={{ borderColor: blockColor, pointerEvents: "all" }}
      />

      {/* Block Header */}
      <Box
        className="block-header"
        sx={{
          px: 1.75,
          py: 1.25,
          borderBottom: `1px solid ${blockColor}25`,
          backgroundColor: `${blockColor}12`,
          borderTopLeftRadius: 10,
          borderTopRightRadius: 10,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          cursor: "grab",
          pointerEvents: "all",
          "&:active": { cursor: "grabbing" },
        }}
      >
        {/* Left: Icon & Title */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
          <Box
            sx={{
              width: 24,
              height: 24,
              borderRadius: 1,
              backgroundColor: `${blockColor}22`,
              color: blockColor,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
          >
            <Layers size={14} />
          </Box>

          {isEditing ? (
            <TextField
              size="small"
              variant="standard"
              value={nameValue}
              onChange={(e) => setNameValue(e.target.value)}
              onBlur={handleSaveName}
              onKeyDown={handleKeyDown}
              autoFocus
              className="nodrag"
              sx={{
                "& .MuiInputBase-input": {
                  fontSize: "0.875rem",
                  fontWeight: 700,
                  color: "#0F172A",
                  py: 0,
                },
              }}
            />
          ) : (
            <Box
              onClick={(e) => {
                e.stopPropagation();
                setIsEditing(true);
              }}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 0.5,
                cursor: "pointer",
                "&:hover .edit-icon": { opacity: 1 },
              }}
            >
              <Typography
                variant="subtitle2"
                noWrap
                sx={{
                  fontWeight: 700,
                  color: "#0F172A",
                  fontSize: "0.875rem",
                  maxWidth: 180,
                }}
              >
                {block.name}
              </Typography>
              <Edit3
                size={12}
                className="edit-icon"
                style={{ opacity: 0.3, transition: "opacity 0.15s", color: "#64748B" }}
              />
            </Box>
          )}
        </Box>

        {/* Right: Badges & Controls */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexShrink: 0 }}>
          {/* Member node count */}
          <Chip
            label={`${memberCount} ${memberCount === 1 ? "node" : "nodes"}`}
            size="small"
            sx={{
              height: 20,
              fontSize: "0.68rem",
              fontWeight: 700,
              backgroundColor: `${blockColor}20`,
              color: blockColor,
              borderRadius: 1,
            }}
          />

          {/* Full-Mesh Core Badge */}
          {memberCount >= 2 && fullMeshCount > 0 && (
            <Tooltip
              title={`${fullMeshCount} of ${memberCount} node(s) form the maximum full-mesh interconnected core in this block.`}
            >
              <Chip
                icon={<Share2 size={11} color={blockColor} style={{ marginLeft: 4 }} />}
                label={fullMeshCount === memberCount ? "Full-Mesh" : `${fullMeshCount}/${memberCount} mesh`}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.68rem",
                  fontWeight: 700,
                  backgroundColor: `${blockColor}16`,
                  color: blockColor,
                  border: `1px solid ${blockColor}40`,
                  borderRadius: 1,
                }}
              />
            </Tooltip>
          )}

          {/* Member Health Badge */}
          {memberCount > 0 && (
            <Tooltip
              title={
                healthyCount === memberCount && brokenLinkCount === 0
                  ? "All member nodes are healthy with all links active"
                  : `${memberCount - healthyCount} node(s) degraded or offline`
              }
            >
              <Chip
                icon={
                  healthyCount === memberCount && brokenLinkCount === 0 ? (
                    <CheckCircle2 size={11} color="#059669" style={{ marginLeft: 4 }} />
                  ) : (
                    <AlertTriangle size={11} color="#DC2626" style={{ marginLeft: 4 }} />
                  )
                }
                label={healthyCount === memberCount && brokenLinkCount === 0 ? "Healthy" : `${memberCount - healthyCount} Degraded`}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.68rem",
                  fontWeight: 700,
                  backgroundColor: healthyCount === memberCount && brokenLinkCount === 0 ? "#ECFDF5" : "#FEF2F2",
                  color: healthyCount === memberCount && brokenLinkCount === 0 ? "#059669" : "#DC2626",
                  border: `1px solid ${healthyCount === memberCount && brokenLinkCount === 0 ? "#A7F3D0" : "#FECDD3"}`,
                  borderRadius: 1,
                }}
              />
            </Tooltip>
          )}

          {/* Hidden Links Badge */}
          {hiddenLinkCount > 0 && (
            <Tooltip title={`${hiddenLinkCount} intra-block mesh link(s) hidden to keep graph clean. Click any node to focus & inspect its links.`}>
              <Chip
                icon={<EyeOff size={11} color="#64748B" style={{ marginLeft: 4 }} />}
                label={`${hiddenLinkCount} hidden`}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.68rem",
                  fontWeight: 600,
                  backgroundColor: "#F1F5F9",
                  color: "#64748B",
                  borderRadius: 1,
                  border: "1px solid #E2E8F0",
                }}
              />
            </Tooltip>
          )}

          {/* Broken Links Alert */}
          {brokenLinkCount > 0 && (
            <Tooltip title={`${brokenLinkCount} intra-block link(s) down or unreachable!`}>
              <Chip
                icon={<AlertTriangle size={11} color="#DC2626" style={{ marginLeft: 4 }} />}
                label={`${brokenLinkCount} down`}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.68rem",
                  fontWeight: 700,
                  backgroundColor: "#FEE2E2",
                  color: "#DC2626",
                  borderRadius: 1,
                  border: "1px solid #FECDD3",
                }}
              />
            </Tooltip>
          )}

          {/* Palette Color Picker */}
          <Tooltip title="Change block color">
            <IconButton
              size="small"
              className="nodrag"
              onClick={(e) => {
                e.stopPropagation();
                setPaletteAnchor(e.currentTarget);
              }}
              sx={{
                width: 22,
                height: 22,
                p: 0,
                color: "#64748B",
                "&:hover": { color: blockColor },
              }}
            >
              <Palette size={13} />
            </IconButton>
          </Tooltip>

          <Menu
            anchorEl={paletteAnchor}
            open={Boolean(paletteAnchor)}
            onClose={() => setPaletteAnchor(null)}
            className="nodrag"
            PaperProps={{
              sx: {
                p: 0.5,
                borderRadius: 2,
                boxShadow: "0 8px 16px rgba(0,0,0,0.1)",
              },
            }}
          >
            {BLOCK_PALETTE.map((p) => (
              <MenuItem
                key={p.name}
                onClick={() => {
                  setPaletteAnchor(null);
                  onChangeColor?.(block.id, p.color);
                }}
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1.5,
                  py: 0.75,
                  px: 1.5,
                  borderRadius: 1,
                }}
              >
                <Box
                  sx={{
                    width: 14,
                    height: 14,
                    borderRadius: "50%",
                    backgroundColor: p.color,
                  }}
                />
                <Typography variant="body2" sx={{ fontSize: "0.8rem", fontWeight: 600 }}>
                  {p.name}
                </Typography>
                {p.color === blockColor && <Check size={14} color="#4F46E5" style={{ marginLeft: "auto" }} />}
              </MenuItem>
            ))}
          </Menu>

          {/* Delete Block */}
          <Tooltip title="Remove block (nodes remain on canvas)">
            <IconButton
              size="small"
              className="nodrag"
              onClick={(e) => {
                e.stopPropagation();
                onDeleteBlock?.(block.id);
              }}
              sx={{
                width: 22,
                height: 22,
                p: 0,
                color: "#94A3B8",
                "&:hover": { color: "#EF4444", backgroundColor: "#FEE2E2" },
              }}
            >
              <Trash2 size={13} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Block Body Area */}
      <Box
        sx={{
          flex: 1,
          p: 1.5,
          position: "relative",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          pointerEvents: "none",
        }}
      >
        {memberCount === 0 && (
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 0.5,
              opacity: 0.5,
            }}
          >
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, fontStyle: "italic" }}>
              Drag and drop nodes here to group
            </Typography>
            <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.68rem" }}>
              Mesh links inside this block will be auto-hidden
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  );
});

BlockNode.displayName = "BlockNode";
