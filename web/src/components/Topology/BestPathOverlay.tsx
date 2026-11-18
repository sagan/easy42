import React, { useState } from "react";
import { Box, Chip, Typography } from "@mui/material";
import {
  ArrowLeft,
  ArrowLeftRight,
  ArrowRight,
  GitBranch,
  Info,
  X,
} from "lucide-react";
import {
  BestPathPair,
  DirectedLink,
  DirectedPath,
} from "../../utils/pathFinder";

export interface BestPathOverlayProps {
  pair: BestPathPair;
  adjacency: Map<string, DirectedLink[]>;
  /** Called when the user clicks a hop, to pin that node as the focus. */
  onFocusHop?: (nodeName: string) => void;
}

/** Emerald = focus → hover (forward), Indigo = hover → focus (reverse). */
export const BEST_PATH_FORWARD_COLOR = "#059669";
export const BEST_PATH_REVERSE_COLOR = "#4F46E5";
const FORWARD_COLOR = BEST_PATH_FORWARD_COLOR;
const REVERSE_COLOR = BEST_PATH_REVERSE_COLOR;
const NO_PATH_COLOR = "#EF4444";

const formatCost = (cost: number) => {
  const rounded = Math.round(cost * 100) / 100;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(2);
};

const RouteLine: React.FC<{
  path: DirectedPath;
  reverseIsPrimary: boolean;
  onFocusHop?: (nodeName: string) => void;
}> = ({ path, reverseIsPrimary, onFocusHop }) => {
  // Hops flagged `reversed` follow the path against a link's configured direction.
  const lastIdx = path.hops.length - 1;
  const Arrow = reverseIsPrimary ? ArrowLeft : ArrowRight;
  return (
    <Box
      sx={{
        display: "flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: 0.25,
        rowGap: 0.5,
      }}
    >
      {path.hops.map((hop, idx) => (
        <React.Fragment key={`${hop.node}-${idx}`}>
          {idx > 0 && (
            <Arrow size={11} color="#94A3B8" style={{ flexShrink: 0 }} />
          )}
          <Box
            component="span"
            onClick={() => onFocusHop?.(hop.node)}
            title={`${hop.node}${hop.viaCost !== undefined ? ` — reached with link cost ${formatCost(hop.viaCost)}` : ""}`}
            sx={{
              fontFamily: "monospace",
              fontSize: "0.68rem",
              fontWeight: idx === 0 || idx === lastIdx ? 800 : 600,
              color: "#1E293B",
              backgroundColor:
                idx === 0 || idx === lastIdx ? "#E2E8F0" : "transparent",
              px: idx === 0 || idx === lastIdx ? 0.4 : 0,
              borderRadius: 0.5,
              cursor: onFocusHop ? "pointer" : "default",
              "&:hover": { color: "#4F46E5" },
            }}
          >
            {hop.node}
          </Box>
          {hop.viaCost !== undefined && (
            <Typography
              component="span"
              sx={{ fontSize: "0.58rem", color: "#94A3B8", fontWeight: 600 }}
            >
              +{formatCost(hop.viaCost)}
            </Typography>
          )}
        </React.Fragment>
      ))}
    </Box>
  );
};

const DirectionRow: React.FC<{
  color: string;
  label: string;
  path: DirectedPath | null;
  reverseIsPrimary?: boolean;
  onFocusHop?: (nodeName: string) => void;
}> = ({ color, label, path, reverseIsPrimary = false, onFocusHop }) => {
  const Arrow = reverseIsPrimary ? ArrowLeft : ArrowRight;
  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.35 }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
        <Box
          sx={{
            width: 5,
            height: 12,
            borderRadius: 0.5,
            backgroundColor: path ? color : NO_PATH_COLOR,
          }}
        />
        <Arrow size={11} color={path ? color : NO_PATH_COLOR} />
        <Typography
          variant="caption"
          sx={{
            fontSize: "0.62rem",
            fontWeight: 700,
            letterSpacing: 0.3,
            color: "#64748B",
          }}
        >
          {label}
        </Typography>
        {path ? (
          <>
            <Chip
              size="small"
              label={`cost ${formatCost(path.totalCost)}`}
              sx={{
                height: 16,
                fontSize: "0.6rem",
                fontWeight: 800,
                color,
                backgroundColor: `${color}14`,
                border: `1px solid ${color}55`,
                borderRadius: 0.5,
                "& .MuiChip-label": { px: 0.5 },
              }}
            />
            <Typography
              variant="caption"
              sx={{ fontSize: "0.6rem", color: "#94A3B8", fontWeight: 600 }}
            >
              {path.hops.length - 1} hop{path.hops.length - 1 === 1 ? "" : "s"}
            </Typography>
          </>
        ) : (
          <Typography
            variant="caption"
            sx={{ fontSize: "0.6rem", color: NO_PATH_COLOR, fontWeight: 700 }}
          >
            unreachable
          </Typography>
        )}
      </Box>
      {path ? (
        <RouteLine
          path={path}
          reverseIsPrimary={reverseIsPrimary}
          onFocusHop={onFocusHop}
        />
      ) : (
        <Typography
          variant="caption"
          sx={{ fontSize: "0.65rem", color: "#94A3B8", fontStyle: "italic" }}
        >
          No link path between these nodes
        </Typography>
      )}
    </Box>
  );
};

/**
 * Floating summary of the best path(s) between the focused (pinned) node and the hovered
 * node. Rendered only in "best path mode"; can be collapsed into a compact chip so the
 * path stays drawn on the canvas while the user explores.
 */
export const BestPathOverlay: React.FC<BestPathOverlayProps> = ({
  pair,
  onFocusHop,
}) => {
  const [collapsed, setCollapsed] = useState(false);

  if (collapsed) {
    return (
      <Box
        onClick={() => setCollapsed(false)}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          backgroundColor: "rgba(255, 255, 255, 0.94)",
          backdropFilter: "blur(8px)",
          border: "1px solid #E2E8F0",
          borderRadius: 1,
          px: 1.1,
          py: 0.6,
          boxShadow: "0 4px 16px -2px rgba(15, 23, 42, 0.12)",
          cursor: "pointer",
        }}
      >
        <GitBranch
          size={13}
          color={pair.forward ? FORWARD_COLOR : NO_PATH_COLOR}
        />
        <Typography
          variant="caption"
          sx={{ fontWeight: 700, fontSize: "0.7rem", color: "#0F172A" }}
        >
          Best Path
        </Typography>
        <Typography
          variant="caption"
          className="mono-font"
          sx={{ fontSize: "0.65rem", color: "#64748B" }}
        >
          {pair.from} {pair.isAsymmetric ? "⇄" : "⇢"} {pair.to}
        </Typography>
        <Typography
          variant="caption"
          sx={{ fontSize: "0.65rem", fontWeight: 800, color: FORWARD_COLOR }}
        >
          {pair.forward ? formatCost(pair.forward.totalCost) : "unreachable"}
        </Typography>
      </Box>
    );
  }

  const endChip = (name: string, color: string) => (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 0.5,
        backgroundColor: "#F1F5F9",
        border: "1px solid #E2E8F0",
        borderRadius: 0.75,
        px: 0.6,
        py: 0.1,
      }}
    >
      <Box
        sx={{
          width: 6,
          height: 6,
          borderRadius: "50%",
          backgroundColor: color,
        }}
      />
      <Typography
        variant="caption"
        className="mono-font"
        sx={{ fontSize: "0.65rem", fontWeight: 700, color: "#334155" }}
      >
        {name}
      </Typography>
    </Box>
  );

  return (
    <Box
      sx={{
        backgroundColor: "rgba(255, 255, 255, 0.95)",
        backdropFilter: "blur(8px)",
        border: "1px solid #E2E8F0",
        borderRadius: 1,
        boxShadow:
          "0 4px 16px -2px rgba(15, 23, 42, 0.12), 0 2px 6px -1px rgba(15, 23, 42, 0.05)",
        px: 1.25,
        py: 1,
        minWidth: 260,
        maxWidth: 470,
        display: "flex",
        flexDirection: "column",
        gap: 0.75,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
        <GitBranch size={14} color="#4F46E5" />
        <Typography
          variant="caption"
          sx={{ fontSize: "0.68rem", fontWeight: 800, color: "#0F172A" }}
        >
          Best Path
        </Typography>
        {endChip(pair.from, FORWARD_COLOR)}
        <ArrowLeftRight size={11} color="#94A3B8" />
        {endChip(pair.to, "#4F46E5")}
        <Box sx={{ flex: 1 }} />
        <Box
          onClick={() => setCollapsed(true)}
          title="Collapse panel"
          sx={{
            display: "flex",
            color: "#94A3B8",
            cursor: "pointer",
            "&:hover": { color: "#475569" },
          }}
        >
          <X size={13} />
        </Box>
      </Box>

      <Box sx={{ height: 1, backgroundColor: "#E2E8F0" }} />

      <DirectionRow
        color={FORWARD_COLOR}
        label={`${pair.from} → ${pair.to}`}
        path={pair.forward}
        onFocusHop={onFocusHop}
      />

      {pair.isAsymmetric && (
        <>
          <Box sx={{ height: 1, borderTop: "1px dashed #E2E8F0" }} />
          <DirectionRow
            color={REVERSE_COLOR}
            label={`${pair.to} → ${pair.from}`}
            path={pair.reverse}
            reverseIsPrimary
            onFocusHop={onFocusHop}
          />
        </>
      )}

      <Box sx={{ display: "flex", alignItems: "flex-start", gap: 0.4 }}>
        <Info
          size={10}
          color="#94A3B8"
          style={{ marginTop: 2, flexShrink: 0 }}
        />
        <Typography
          variant="caption"
          sx={{ fontSize: "0.58rem", color: "#94A3B8", lineHeight: 1.35 }}
        >
          {pair.isAsymmetric
            ? "Asymmetric: directions prefer different links. Costs use per-link BIRD cost (local-pref); custom BIRD filters can override this."
            : "Symmetric: both directions share the same path. Preview only — custom BIRD filters can override it."}
        </Typography>
      </Box>
    </Box>
  );
};

export default BestPathOverlay;
