import React from "react";
import { BaseEdge, EdgeLabelRenderer, EdgeProps, getBezierPath } from "@xyflow/react";
import { Box, Typography } from "@mui/material";
import { Link } from "../../types/api";
import { getEffectiveLinkCost } from "../../utils/cost";

export type LinkWorkingState = "working" | "not_working" | "unknown";

export interface CustomEdgeData {
  link: Link;
  workingState?: LinkWorkingState;
  latestHandshake?: string;
  transferRxBytes?: number;
  transferTxBytes?: number;
  isExternal?: boolean;
  fromCost?: number;
  toCost?: number;
  onSelect: (link: Link) => void;
  [key: string]: unknown;
}

function formatHandshakeAgo(dateStr?: string): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  const diffSec = Math.floor((Date.now() - date.getTime()) / 1000);
  if (diffSec < 0) return "now";
  if (diffSec < 60) return `${diffSec}s`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m`;
  const diffHours = Math.floor(diffMin / 60);
  return `${diffHours}h`;
}

export const CustomEdge: React.FC<EdgeProps> = ({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style = {},
  markerEnd,
  data,
}) => {
  const edgeData = data as unknown as CustomEdgeData;
  const link = edgeData?.link;
  const workingState: LinkWorkingState = edgeData?.workingState || "unknown";
  const latestHandshake = edgeData?.latestHandshake;
  const isExternal = Boolean(edgeData?.isExternal);
  const isManual = link?.type === "manual" || link?.from?.type === "manual" || link?.to?.type === "manual";
  const totalLinksInPair = (edgeData?.totalLinksInPair as number) || 1;
  const linkIndexInPair = (edgeData?.linkIndexInPair as number) || 0;

  const fromCost =
    typeof edgeData?.fromCost === "number"
      ? edgeData.fromCost
      : getEffectiveLinkCost(link?.from, isExternal);
  const toCost =
    typeof edgeData?.toCost === "number"
      ? edgeData.toCost
      : getEffectiveLinkCost(link?.to, isExternal);

  const isFromCostOverridden = Boolean(link?.from?.cost && link.from.cost !== 0);
  const isToCostOverridden = Boolean(link?.to?.cost && link.to.cost !== 0);

  // In WireGuard links, interface names are named after the peer (wg42<peer>),
  // so link.from.interface (e.g. wg42foo) is on node bar pointing to node foo,
  // which users associate with the "foo" end of the link.
  // The egress cost from the node represented on the left towards the right node is leftCost (pointing →),
  // and the egress cost from the node represented on the right towards the left node is rightCost (pointing ←).
  const fromIface = (link?.from?.interface || "").toLowerCase();
  const toName = (link?.to?.name || "").toLowerCase();
  const fromName = (link?.from?.name || "").toLowerCase();

  const leftRepresentsTo =
    Boolean(toName && fromIface.includes(toName.slice(0, 8))) ||
    Boolean(fromIface.startsWith("wg42") && !fromIface.includes(fromName.slice(0, 8)));

  const leftCost = leftRepresentsTo ? toCost : fromCost;
  const rightCost = leftRepresentsTo ? fromCost : toCost;
  const isLeftCostOverridden = leftRepresentsTo ? isToCostOverridden : isFromCostOverridden;
  const isRightCostOverridden = leftRepresentsTo ? isFromCostOverridden : isToCostOverridden;
  const leftNodeName = leftRepresentsTo ? (link?.to?.name || "Left") : (link?.from?.name || "Left");
  const rightNodeName = leftRepresentsTo ? (link?.from?.name || "Right") : (link?.to?.name || "Right");

  let edgePath: string;
  let labelX: number;
  let labelY: number;

  if (totalLinksInPair > 1) {
    const midX = (sourceX + targetX) / 2;
    const midY = (sourceY + targetY) / 2;
    const dx = targetX - sourceX;
    const dy = targetY - sourceY;
    const len = Math.hypot(dx, dy) || 1;
    const nx = -dy / len;
    const ny = dx / len;
    const step = 38;
    const offset = (linkIndexInPair - (totalLinksInPair - 1) / 2) * step;
    const cx = midX + nx * offset;
    const cy = midY + ny * offset;

    edgePath = `M ${sourceX} ${sourceY} Q ${cx} ${cy} ${targetX} ${targetY}`;
    labelX = (sourceX + 2 * cx + targetX) / 4;
    labelY = (sourceY + 2 * cy + targetY) / 4;
  } else {
    const [bezierPath, bLabelX, bLabelY] = getBezierPath({
      sourceX,
      sourceY,
      sourcePosition,
      targetPosition,
      targetX,
      targetY,
    });
    edgePath = bezierPath;
    labelX = bLabelX;
    labelY = bLabelY;
  }

  // Determine styles according to derived working state and link type
  let strokeColor = "#94A3B8";
  let strokeWidth = 2;
  let strokeDasharray: string | undefined = isExternal ? "6 4" : "4 4";
  let pillBorderColor = "#CBD5E1";
  let pillBgColor = "#FFFFFF";
  let dotColor = "#94A3B8";
  let statusText = isExternal ? "External" : "Idle";

  if (isManual) {
    strokeColor = "#7C3AED";
    strokeDasharray = "5 3";
    strokeWidth = 2.2;
    pillBorderColor = "#DDD6FE";
    pillBgColor = "#FAF5FF";
    dotColor = "#7C3AED";
    statusText = "Manual";
  } else if (isExternal) {
    strokeColor = "#8B5CF6";
    strokeDasharray = "6 4";
    if (workingState === "working") {
      strokeWidth = 2.5;
      pillBorderColor = "#C4B5FD";
      pillBgColor = "#F5F3FF";
      dotColor = "#8B5CF6";
      statusText = (latestHandshake ? formatHandshakeAgo(latestHandshake) : "Active") + " (Ext)";
    } else if (workingState === "not_working") {
      strokeColor = "#EF4444";
      pillBorderColor = "#FCA5A5";
      pillBgColor = "#FEF2F2";
      dotColor = "#EF4444";
      statusText = "Down (Ext)";
    } else {
      pillBorderColor = "#DDD6FE";
      pillBgColor = "#FAF5FF";
      dotColor = "#8B5CF6";
      statusText = "External";
    }
  } else if (workingState === "working") {
    strokeColor = "#10B981";
    strokeWidth = 2.5;
    strokeDasharray = undefined;
    pillBorderColor = "#6EE7B7";
    pillBgColor = "#F0FDF4";
    dotColor = "#10B981";
    statusText = latestHandshake ? formatHandshakeAgo(latestHandshake) : "Active";
  } else if (workingState === "not_working") {
    strokeColor = "#EF4444";
    strokeWidth = 2;
    strokeDasharray = "6 4";
    pillBorderColor = "#FCA5A5";
    pillBgColor = "#FEF2F2";
    dotColor = "#EF4444";
    statusText = "Down";
  }

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{
          ...style,
          strokeWidth,
          stroke: strokeColor,
          strokeDasharray,
          cursor: "pointer",
          pointerEvents: "all",
          transition: "stroke 0.2s ease, stroke-width 0.2s ease",
        }}
      />
      <EdgeLabelRenderer>
        <Box
          onClick={(e) => {
            e.stopPropagation();
            if (link && edgeData.onSelect) {
              edgeData.onSelect(link);
            }
          }}
          style={{
            position: "absolute",
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: "all",
            zIndex: 1000,
          }}
          sx={{
            cursor: "pointer",
            backgroundColor: pillBgColor,
            border: "1px solid",
            borderColor: pillBorderColor,
            borderRadius: 2,
            px: 1.2,
            py: 0.4,
            boxShadow: "0 2px 5px rgba(0, 0, 0, 0.06)",
            transition: "all 0.15s ease",
            display: "flex",
            alignItems: "center",
            gap: 1,
            "&:hover": {
              borderColor: strokeColor,
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px) scale(1.06)`,
              boxShadow: "0 4px 10px rgba(0, 0, 0, 0.12)",
            },
          }}
        >
          {link ? (
            <>
              {/* Working State Status Dot */}
              <Box
                sx={{
                  width: 7,
                  height: 7,
                  borderRadius: "50%",
                  backgroundColor: dotColor,
                  boxShadow: workingState === "working" ? "0 0 6px #10B981" : undefined,
                  flexShrink: 0,
                }}
              />

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                {/* Interface names: wg42foo <-> wg42bar */}
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, lineHeight: 1.2 }}>
                  <Typography
                    variant="caption"
                    className="mono-font"
                    sx={{ fontSize: "0.66rem", fontWeight: 600, color: "#1E293B", lineHeight: 1.2 }}
                  >
                    {link.from.interface}
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontSize: "0.55rem", lineHeight: 1.2 }}>
                    ↔
                  </Typography>
                  <Typography
                    variant="caption"
                    className="mono-font"
                    sx={{ fontSize: "0.66rem", fontWeight: 600, color: "#1E293B", lineHeight: 1.2 }}
                  >
                    {link.to.interface}
                  </Typography>
                </Box>

                {/* Link Cost display below the wg42foo <-> wg42bar label */}
                <Box
                  data-testid="link-cost"
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 0.35,
                    mt: 0.15,
                    lineHeight: 1,
                  }}
                  title={
                    leftCost === rightCost
                      ? `Cost: ${leftCost}`
                      : `Cost: ${leftNodeName} → ${rightNodeName}: ${leftCost}, ${rightNodeName} → ${leftNodeName}: ${rightCost}`
                  }
                >
                  {leftCost === rightCost ? (
                    <Typography
                      variant="caption"
                      className="mono-font"
                      sx={{
                        fontSize: "0.60rem",
                        fontWeight: 600,
                        color: isLeftCostOverridden || isRightCostOverridden ? "#D97706" : "#64748B",
                        lineHeight: 1,
                      }}
                    >
                      {leftCost}
                    </Typography>
                  ) : (
                    <>
                      <Typography
                        variant="caption"
                        className="mono-font"
                        sx={{
                          fontSize: "0.60rem",
                          fontWeight: 600,
                          color: isLeftCostOverridden ? "#D97706" : "#64748B",
                          lineHeight: 1,
                        }}
                      >
                        {leftCost}
                      </Typography>
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: "0.55rem",
                          fontWeight: 700,
                          color: "#94A3B8",
                          lineHeight: 1,
                          userSelect: "none",
                        }}
                      >
                        →
                      </Typography>
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: "0.55rem",
                          fontWeight: 700,
                          color: "#94A3B8",
                          lineHeight: 1,
                          userSelect: "none",
                        }}
                      >
                        ←
                      </Typography>
                      <Typography
                        variant="caption"
                        className="mono-font"
                        sx={{
                          fontSize: "0.60rem",
                          fontWeight: 600,
                          color: isRightCostOverridden ? "#D97706" : "#64748B",
                          lineHeight: 1,
                        }}
                      >
                        {rightCost}
                      </Typography>
                    </>
                  )}
                </Box>
              </Box>

              {/* Status Badge */}
              <Box
                sx={{
                  fontSize: "0.62rem",
                  fontWeight: 700,
                  color: dotColor,
                  borderRadius: 1,
                  px: 0.5,
                  py: 0.1,
                  backgroundColor: "rgba(0,0,0,0.03)",
                  display: "flex",
                  alignItems: "center",
                  gap: 0.3,
                }}
              >
                {workingState === "working" && !isManual && "⚡"}
                {statusText}
              </Box>
            </>
          ) : (
            <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
              WG Link
            </Typography>
          )}
        </Box>
      </EdgeLabelRenderer>
    </>
  );
};
