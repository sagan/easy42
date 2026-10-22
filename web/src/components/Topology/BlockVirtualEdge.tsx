import React, { memo, useState } from "react";
import { BaseEdge, EdgeLabelRenderer, EdgeProps, useInternalNode } from "@xyflow/react";
import {
  Box,
  Typography,
  Popover,
  Button,
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableCell,
  Chip,
} from "@mui/material";
import { Layers, UnfoldVertical, FoldVertical, ExternalLink } from "lucide-react";
import { GraphBlock, Link } from "../../types/api";

export interface BlockVirtualEdgeData {
  pairKey: string;
  sourceBlock: GraphBlock;
  targetBlock: GraphBlock;
  links: Link[];
  workingCount: number;
  downCount: number;
  unknownCount: number;
  isExpanded?: boolean;
  onToggleExpandBundle?: (pairKey: string) => void;
  onSelectLink?: (link: Link) => void;
  [key: string]: unknown;
}

/**
 * Calculates the exact intersection point where the ray from fromCenter
 * pointing to toCenter hits the rectangular bounding box border.
 */
export function getRectBorderIntersection(
  fromCenter: { x: number; y: number },
  toCenter: { x: number; y: number },
  rect: { x: number; y: number; width: number; height: number },
): { x: number; y: number } {
  const dx = toCenter.x - fromCenter.x;
  const dy = toCenter.y - fromCenter.y;

  if (dx === 0 && dy === 0) {
    return fromCenter;
  }

  const hw = Math.max(10, rect.width / 2);
  const hh = Math.max(10, rect.height / 2);

  const tx = dx !== 0 ? hw / Math.abs(dx) : Infinity;
  const ty = dy !== 0 ? hh / Math.abs(dy) : Infinity;
  const t = Math.min(tx, ty);

  return {
    x: fromCenter.x + dx * t,
    y: fromCenter.y + dy * t,
  };
}

export const BlockVirtualEdge: React.FC<EdgeProps> = memo(
  ({ id, source, target, sourceX, sourceY, targetX, targetY, style = {}, data }) => {
    const edgeData = data as unknown as BlockVirtualEdgeData;
    const pairKey = edgeData?.pairKey || id;
    const sourceBlock = edgeData?.sourceBlock;
    const targetBlock = edgeData?.targetBlock;
    const links = edgeData?.links || [];
    const workingCount = edgeData?.workingCount || 0;
    const downCount = edgeData?.downCount || 0;
    const isExpanded = Boolean(edgeData?.isExpanded);
    const onToggleExpandBundle = edgeData?.onToggleExpandBundle;
    const onSelectLink = edgeData?.onSelectLink;

    const [popoverAnchor, setPopoverAnchor] = useState<HTMLElement | null>(null);

    const sourceNode = useInternalNode(source);
    const targetNode = useInternalNode(target);

    // Compute bounding boxes of source and target blocks
    const sPos = sourceNode?.internals?.positionAbsolute ??
      sourceNode?.position ?? {
        x: sourceX - (sourceBlock?.width || 440) / 2,
        y: sourceY - (sourceBlock?.height || 320) / 2,
      };
    const sW = sourceNode?.measured?.width ?? (sourceNode?.width as number | undefined) ?? sourceBlock?.width ?? 440;
    const sH = sourceNode?.measured?.height ?? (sourceNode?.height as number | undefined) ?? sourceBlock?.height ?? 320;

    const tPos = targetNode?.internals?.positionAbsolute ??
      targetNode?.position ?? {
        x: targetX - (targetBlock?.width || 440) / 2,
        y: targetY - (targetBlock?.height || 320) / 2,
      };
    const tW = targetNode?.measured?.width ?? (targetNode?.width as number | undefined) ?? targetBlock?.width ?? 440;
    const tH = targetNode?.measured?.height ?? (targetNode?.height as number | undefined) ?? targetBlock?.height ?? 320;

    const c1 = { x: sPos.x + sW / 2, y: sPos.y + sH / 2 };
    const c2 = { x: tPos.x + tW / 2, y: tPos.y + tH / 2 };

    const startPoint = getRectBorderIntersection(c1, c2, { x: sPos.x, y: sPos.y, width: sW, height: sH });
    const endPoint = getRectBorderIntersection(c2, c1, { x: tPos.x, y: tPos.y, width: tW, height: tH });

    // Fallback if rectangles overlap closely
    const dist = Math.hypot(endPoint.x - startPoint.x, endPoint.y - startPoint.y);
    const sx = dist > 5 ? startPoint.x : c1.x;
    const sy = dist > 5 ? startPoint.y : c1.y;
    const tx = dist > 5 ? endPoint.x : c2.x;
    const ty = dist > 5 ? endPoint.y : c2.y;

    const edgePath = `M ${sx} ${sy} L ${tx} ${ty}`;
    const labelX = (sx + tx) / 2;
    const labelY = (sy + ty) / 2;

    const color1 = sourceBlock?.color || "#6366F1";
    const color2 = targetBlock?.color || "#06B6D4";
    const gradientId = `block-virtual-grad-${id.replace(/[^a-zA-Z0-9_-]/g, "_")}`;

    const dotColor = downCount > 0 ? "#EF4444" : workingCount > 0 ? "#10B981" : "#94A3B8";

    // Weighted stroke width scaled logarithmically: 1 link -> ~2.2px, 14 links -> ~6px, 50 links -> 8px
    const linkCount = Math.max(1, links.length);
    const weightedStrokeWidth = Math.min(8, Math.max(2, 1.8 + Math.log2(linkCount) * 1.3));

    const handleBadgeClick = (e: React.MouseEvent<HTMLElement>) => {
      e.stopPropagation();
      setPopoverAnchor(e.currentTarget);
    };

    const handleClosePopover = () => {
      setPopoverAnchor(null);
    };

    return (
      <>
        {/* SVG Gradient Definition */}
        <svg style={{ position: "absolute", top: 0, left: 0, width: 0, height: 0, pointerEvents: "none" }}>
          <defs>
            <linearGradient id={gradientId} x1={sx} y1={sy} x2={tx} y2={ty} gradientUnits="userSpaceOnUse">
              <stop offset="0%" stopColor={color1} stopOpacity={isExpanded ? 0.4 : 0.85} />
              <stop offset="100%" stopColor={color2} stopOpacity={isExpanded ? 0.4 : 0.85} />
            </linearGradient>
          </defs>
        </svg>

        <BaseEdge
          id={id}
          path={edgePath}
          style={{
            ...style,
            stroke: `url(#${gradientId})`,
            strokeWidth: weightedStrokeWidth,
            strokeDasharray: isExpanded ? "4 4" : "7 5",
            strokeLinecap: "round",
            cursor: "pointer",
            opacity: isExpanded ? 0.45 : 1,
            transition: "stroke-width 0.2s ease, opacity 0.2s ease",
          }}
        />

        <EdgeLabelRenderer>
          <Box
            onClick={handleBadgeClick}
            style={{
              position: "absolute",
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
              pointerEvents: "all",
              zIndex: 50,
            }}
            sx={{
              cursor: "pointer",
              backgroundColor: isExpanded ? "rgba(248, 250, 252, 0.95)" : "rgba(255, 255, 255, 0.96)",
              backdropFilter: "blur(6px)",
              border: "1.5px solid",
              borderColor: isExpanded ? `${color1}60` : `${color1}90`,
              borderRadius: 2,
              px: 1.25,
              py: 0.45,
              boxShadow: isExpanded ? "0 1px 4px rgba(0, 0, 0, 0.05)" : "0 2px 8px rgba(0, 0, 0, 0.08)",
              display: "flex",
              alignItems: "center",
              gap: 0.75,
              transition: "all 0.15s ease",
              "&:hover": {
                borderColor: color1,
                transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px) scale(1.06)`,
                boxShadow: "0 4px 14px rgba(0, 0, 0, 0.14)",
              },
            }}
          >
            <Box
              sx={{
                width: 7,
                height: 7,
                borderRadius: "50%",
                backgroundColor: dotColor,
                boxShadow: workingCount > 0 && downCount === 0 ? "0 0 6px #10B981" : undefined,
                flexShrink: 0,
              }}
            />
            <Layers size={13} color={color1} />
            <Typography
              variant="caption"
              sx={{
                fontSize: "0.72rem",
                fontWeight: 700,
                color: "#1E293B",
                letterSpacing: "0.01em",
              }}
            >
              {links.length} {links.length === 1 ? "Link" : "Links"}
            </Typography>
            <Box
              sx={{
                fontSize: "0.62rem",
                fontWeight: 700,
                color: isExpanded ? "#0284C7" : color1,
                backgroundColor: isExpanded ? "#E0F2FE" : `${color1}18`,
                borderRadius: 0.75,
                px: 0.6,
                py: 0.1,
                lineHeight: 1.2,
              }}
            >
              {isExpanded ? "Expanded" : "Virtual"}
            </Box>
          </Box>
          {/* Interactive Click-to-Expand Popover Card */}
          <Popover
            open={Boolean(popoverAnchor)}
            anchorEl={popoverAnchor}
            onClose={handleClosePopover}
            anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
            transformOrigin={{ vertical: "top", horizontal: "center" }}
            slotProps={{
              paper: {
                sx: {
                  width: 380,
                  maxHeight: 420,
                  borderRadius: 2.5,
                  boxShadow: "0 12px 32px rgba(0,0,0,0.18)",
                  border: "1px solid #E2E8F0",
                  overflow: "hidden",
                  display: "flex",
                  flexDirection: "column",
                  mt: 0.75,
                },
              },
            }}
          >
            {/* Header */}
            <Box
              sx={{
                p: 1.5,
                backgroundColor: "#F8FAFC",
                borderBottom: "1px solid #E2E8F0",
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
                    backgroundColor: `${color1}15`,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Layers size={15} color={color1} />
                </Box>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
                    {sourceBlock?.name || "Group A"} ↔ {targetBlock?.name || "Group B"}
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
                    {links.length} cross-group links ({workingCount} active{downCount > 0 ? `, ${downCount} down` : ""})
                  </Typography>
                </Box>
              </Box>

              {/* Toggle Canvas Expansion */}
              {onToggleExpandBundle && (
                <Button
                  size="small"
                  variant={isExpanded ? "outlined" : "contained"}
                  startIcon={isExpanded ? <FoldVertical size={13} /> : <UnfoldVertical size={13} />}
                  onClick={() => {
                    onToggleExpandBundle(pairKey);
                  }}
                  sx={{
                    fontSize: "0.7rem",
                    fontWeight: 700,
                    textTransform: "none",
                    borderRadius: 1.5,
                    py: 0.4,
                    px: 1,
                    boxShadow: "none",
                    backgroundColor: isExpanded ? "transparent" : "#4F46E5",
                    borderColor: isExpanded ? "#CBD5E1" : undefined,
                    color: isExpanded ? "#475569" : "#FFFFFF",
                    "&:hover": {
                      backgroundColor: isExpanded ? "#F1F5F9" : "#4338CA",
                      boxShadow: "none",
                    },
                  }}
                >
                  {isExpanded ? "Collapse" : "Expand Lines"}
                </Button>
              )}
            </Box>

            {/* List of underlying links */}
            <Box sx={{ overflowY: "auto", flex: 1, p: 0.5 }}>
              <Table size="small" stickyHeader>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#64748B", py: 0.75 }}>
                      Source Node
                    </TableCell>
                    <TableCell sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#64748B", py: 0.75 }}>
                      Target Node
                    </TableCell>
                    <TableCell align="right" sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#64748B", py: 0.75 }}>
                      Action
                    </TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {links.map((link, idx) => {
                    return (
                      <TableRow
                        key={idx}
                        hover
                        sx={{
                          cursor: "pointer",
                          "&:last-child td, &:last-child th": { border: 0 },
                        }}
                        onClick={() => {
                          handleClosePopover();
                          onSelectLink?.(link);
                        }}
                      >
                        <TableCell sx={{ py: 0.75 }}>
                          <Typography variant="caption" sx={{ fontWeight: 600, color: "#1E293B", display: "block" }}>
                            {link.from.name}
                          </Typography>
                          <Typography
                            variant="caption"
                            className="mono-font"
                            sx={{ fontSize: "0.65rem", color: "#64748B" }}
                          >
                            {link.from.interface}
                          </Typography>
                        </TableCell>
                        <TableCell sx={{ py: 0.75 }}>
                          <Typography variant="caption" sx={{ fontWeight: 600, color: "#1E293B", display: "block" }}>
                            {link.to.name}
                          </Typography>
                          <Typography
                            variant="caption"
                            className="mono-font"
                            sx={{ fontSize: "0.65rem", color: "#64748B" }}
                          >
                            {link.to.interface}
                          </Typography>
                        </TableCell>
                        <TableCell align="right" sx={{ py: 0.75 }}>
                          <Chip
                            size="small"
                            label="View Link"
                            icon={<ExternalLink size={10} />}
                            sx={{
                              height: 20,
                              fontSize: "0.65rem",
                              fontWeight: 600,
                              cursor: "pointer",
                              backgroundColor: "#F1F5F9",
                              "&:hover": { backgroundColor: "#E2E8F0" },
                            }}
                          />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </Box>
          </Popover>
        </EdgeLabelRenderer>
      </>
    );
  },
);

BlockVirtualEdge.displayName = "BlockVirtualEdge";
