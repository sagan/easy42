import React, { memo } from "react";
import { BaseEdge, EdgeLabelRenderer, EdgeProps, useInternalNode } from "@xyflow/react";
import { Box, Typography, Tooltip } from "@mui/material";
import { Layers } from "lucide-react";
import { GraphBlock, Link } from "../../types/api";

export interface BlockVirtualEdgeData {
  sourceBlock: GraphBlock;
  targetBlock: GraphBlock;
  links: Link[];
  workingCount: number;
  downCount: number;
  unknownCount: number;
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
  ({
    id,
    source,
    target,
    sourceX,
    sourceY,
    targetX,
    targetY,
    style = {},
    data,
  }) => {
    const edgeData = data as unknown as BlockVirtualEdgeData;
    const sourceBlock = edgeData?.sourceBlock;
    const targetBlock = edgeData?.targetBlock;
    const links = edgeData?.links || [];
    const workingCount = edgeData?.workingCount || 0;
    const downCount = edgeData?.downCount || 0;

    const sourceNode = useInternalNode(source);
    const targetNode = useInternalNode(target);

    // Compute bounding boxes of source and target blocks
    const sPos = sourceNode?.internals?.positionAbsolute ?? sourceNode?.position ?? {
      x: sourceX - (sourceBlock?.width || 440) / 2,
      y: sourceY - (sourceBlock?.height || 320) / 2,
    };
    const sW =
      sourceNode?.measured?.width ?? (sourceNode?.width as number | undefined) ?? sourceBlock?.width ?? 440;
    const sH =
      sourceNode?.measured?.height ?? (sourceNode?.height as number | undefined) ?? sourceBlock?.height ?? 320;

    const tPos = targetNode?.internals?.positionAbsolute ?? targetNode?.position ?? {
      x: targetX - (targetBlock?.width || 440) / 2,
      y: targetY - (targetBlock?.height || 320) / 2,
    };
    const tW =
      targetNode?.measured?.width ?? (targetNode?.width as number | undefined) ?? targetBlock?.width ?? 440;
    const tH =
      targetNode?.measured?.height ?? (targetNode?.height as number | undefined) ?? targetBlock?.height ?? 320;

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

    return (
      <>
        {/* SVG Gradient Definition */}
        <svg style={{ position: "absolute", top: 0, left: 0, width: 0, height: 0, pointerEvents: "none" }}>
          <defs>
            <linearGradient
              id={gradientId}
              x1={sx}
              y1={sy}
              x2={tx}
              y2={ty}
              gradientUnits="userSpaceOnUse"
            >
              <stop offset="0%" stopColor={color1} stopOpacity={0.85} />
              <stop offset="100%" stopColor={color2} stopOpacity={0.85} />
            </linearGradient>
          </defs>
        </svg>

        <BaseEdge
          id={id}
          path={edgePath}
          style={{
            ...style,
            stroke: `url(#${gradientId})`,
            strokeWidth: 2.5,
            strokeDasharray: "7 5",
            strokeLinecap: "round",
            cursor: "default",
          }}
        />

        <EdgeLabelRenderer>
          <Tooltip
            title={
              <Box sx={{ p: 0.5 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#FFFFFF", mb: 0.25 }}>
                  Virtual Block Connection
                </Typography>
                <Typography variant="body2" sx={{ color: "#E2E8F0", fontSize: "0.75rem", mb: 0.5 }}>
                  {sourceBlock?.name || "Block"} ↔ {targetBlock?.name || "Block"}
                </Typography>
                <Typography variant="caption" sx={{ color: "#94A3B8", display: "block" }}>
                  {links.length} {links.length === 1 ? "link" : "links"} ({workingCount} active
                  {downCount > 0 ? `, ${downCount} down` : ""})
                </Typography>
                <Typography variant="caption" sx={{ color: "#818CF8", display: "block", mt: 0.5 }}>
                  Click any node in either block to inspect its individual links.
                </Typography>
              </Box>
            }
            arrow
          >
            <Box
              style={{
                position: "absolute",
                transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
                pointerEvents: "all",
                zIndex: 50,
              }}
              sx={{
                cursor: "pointer",
                backgroundColor: "rgba(255, 255, 255, 0.96)",
                backdropFilter: "blur(6px)",
                border: "1.5px dashed",
                borderColor: `${color1}90`,
                borderRadius: 2,
                px: 1.25,
                py: 0.45,
                boxShadow: "0 2px 8px rgba(0, 0, 0, 0.08)",
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
                  color: color1,
                  backgroundColor: `${color1}18`,
                  borderRadius: 0.75,
                  px: 0.6,
                  py: 0.1,
                  lineHeight: 1.2,
                }}
              >
                Virtual
              </Box>
            </Box>
          </Tooltip>
        </EdgeLabelRenderer>
      </>
    );
  },
);
