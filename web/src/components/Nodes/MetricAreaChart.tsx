import React, { useState, useRef, useMemo } from "react";
import { Box, Typography, Paper } from "@mui/material";

export interface DataPoint {
  timestamp: number;
  value: number;
  value2?: number; // optional second series (e.g. tx rate)
}

export function parseMetricRangeDuration(rangeStr?: string): number {
  if (!rangeStr) return 24 * 3600;
  const s = rangeStr.trim().toLowerCase();
  switch (s) {
    case "1h":
      return 3600;
    case "6h":
      return 6 * 3600;
    case "24h":
      return 24 * 3600;
    case "7d":
      return 7 * 86400;
    case "30d":
      return 30 * 86400;
  }
  const match = s.match(/^(\d+)([mhd])$/);
  if (match) {
    const val = parseInt(match[1], 10);
    const unit = match[2];
    if (unit === "m") return val * 60;
    if (unit === "h") return val * 3600;
    if (unit === "d") return val * 86400;
  }
  return 24 * 3600;
}

interface MetricAreaChartProps {
  title: string;
  data: DataPoint[];
  color?: string;
  color2?: string;
  unit?: string;
  label1?: string;
  label2?: string;
  height?: number;
  formatValue?: (val: number) => string;
  range?: string;
  startTime?: number;
  endTime?: number;
}

export const MetricAreaChart: React.FC<MetricAreaChartProps> = ({
  title,
  data,
  color = "#4F46E5",
  color2 = "#0891B2",
  unit = "%",
  label1 = "Value",
  label2 = "Value 2",
  height = 180,
  formatValue,
  range,
  startTime,
  endTime,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [hoverState, setHoverState] = useState<{
    targetTs: number;
    nearestIndex: number | null;
    isGap: boolean;
  } | null>(null);

  const hasDualSeries = data.some((d) => d.value2 !== undefined);

  // Compute min, max, avg
  const stats = useMemo(() => {
    if (data.length === 0) return { min: 0, max: 0, avg: 0, current: 0 };
    let min = Infinity;
    let max = -Infinity;
    let sum = 0;
    data.forEach((d) => {
      if (d.value < min) min = d.value;
      if (d.value > max) max = d.value;
      if (d.value2 !== undefined) {
        if (d.value2 < min) min = d.value2;
        if (d.value2 > max) max = d.value2;
      }
      sum += d.value;
    });
    if (min === Infinity) min = 0;
    if (max === -Infinity) max = 100;
    // Buffer max so lines don't hit very top
    max = max === 0 ? 10 : max * 1.15;
    return {
      min,
      max,
      avg: sum / data.length,
      current: data[data.length - 1].value,
      current2: hasDualSeries ? data[data.length - 1].value2 : undefined,
    };
  }, [data, hasDualSeries]);

  const defaultFormat = (v: number) => {
    if (formatValue) return formatValue(v);
    if (v >= 1024 * 1024 * 1024) return `${(v / (1024 * 1024 * 1024)).toFixed(1)} GB`;
    if (v >= 1024 * 1024) return `${(v / (1024 * 1024)).toFixed(1)} MB`;
    if (v >= 1024) return `${(v / 1024).toFixed(1)} KB`;
    return `${v.toFixed(1)}${unit}`;
  };

  // SVG coordinates calculation
  const padding = { top: 20, right: 12, bottom: 25, left: 12 };
  const width = 500; // viewBox coordinate space
  const plotWidth = width - padding.left - padding.right;
  const plotHeight = height - padding.top - padding.bottom;

  // Determine full linear time domain [minTs, maxTs]
  const { minTs, maxTs, timeSpan } = useMemo(() => {
    if (startTime !== undefined && endTime !== undefined && endTime > startTime) {
      return { minTs: startTime, maxTs: endTime, timeSpan: endTime - startTime };
    }

    if (range) {
      const dur = parseMetricRangeDuration(range);
      const latestDataTs = data.length > 0 ? data[data.length - 1].timestamp : 0;
      const end = endTime !== undefined ? endTime : Math.max(Math.floor(Date.now() / 1000), latestDataTs);
      const start = startTime !== undefined ? startTime : end - dur;
      return { minTs: start, maxTs: end, timeSpan: end - start };
    }

    if (data.length > 0) {
      const min = data[0].timestamp;
      const max = data[data.length - 1].timestamp;
      return { minTs: min, maxTs: max, timeSpan: max - min };
    }

    return { minTs: 0, maxTs: 0, timeSpan: 0 };
  }, [data, range, startTime, endTime]);

  const getX = (timestamp: number) => {
    if (timeSpan > 0) {
      const ratio = Math.max(0, Math.min(1, (timestamp - minTs) / timeSpan));
      return padding.left + ratio * plotWidth;
    }
    return padding.left;
  };

  const getY = (val: number) => {
    const normY = (val - 0) / Math.max(1, stats.max);
    return padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
  };

  // Threshold to identify empty data intervals (node offline, missed samples, etc.)
  const gapThreshold = useMemo(() => {
    const rangeDur = timeSpan > 0 ? timeSpan : (range ? parseMetricRangeDuration(range) : 86400);
    const baseGap = Math.max(60, rangeDur / 120);

    if (data.length < 2) {
      return baseGap;
    }

    const diffs: number[] = [];
    for (let i = 1; i < data.length; i++) {
      const diff = data[i].timestamp - data[i - 1].timestamp;
      if (diff > 0) diffs.push(diff);
    }
    if (diffs.length === 0) return baseGap;

    diffs.sort((a, b) => a - b);
    const medianDiff = diffs[Math.floor(diffs.length / 2)];

    return Math.max(medianDiff * 2.5, baseGap);
  }, [data, timeSpan, range]);

  // Group continuous points into segments, breaking on empty data gaps
  const segments = useMemo(() => {
    if (data.length === 0) return [];
    const segs: DataPoint[][] = [];
    let current: DataPoint[] = [];

    for (let i = 0; i < data.length; i++) {
      const pt = data[i];
      if (current.length > 0) {
        const prev = current[current.length - 1];
        if (pt.timestamp - prev.timestamp > gapThreshold) {
          segs.push(current);
          current = [];
        }
      }
      current.push(pt);
    }
    if (current.length > 0) {
      segs.push(current);
    }
    return segs;
  }, [data, gapThreshold]);

  // Precompute SVG path/polyline data per segment
  const segmentPaths = useMemo(() => {
    const bottomY = padding.top + plotHeight;
    return segments.map((seg, idx) => {
      if (seg.length === 1) {
        const pt = seg[0];
        const x = getX(pt.timestamp);
        const y1 = getY(pt.value);
        const y2 = hasDualSeries && pt.value2 !== undefined ? getY(pt.value2) : undefined;
        return {
          id: idx,
          isSingle: true,
          x,
          y1,
          y2,
          points1: "",
          area1: "",
          points2: "",
        };
      }

      const points1 = seg.map((p) => `${getX(p.timestamp)},${getY(p.value)}`).join(" ");
      const startX = getX(seg[0].timestamp);
      const endX = getX(seg[seg.length - 1].timestamp);
      const area1 = `M ${startX},${bottomY} L ${points1.replace(/ /g, " L ")} L ${endX},${bottomY} Z`;

      const points2 = hasDualSeries
        ? seg.map((p) => `${getX(p.timestamp)},${getY(p.value2 ?? 0)}`).join(" ")
        : "";

      return {
        id: idx,
        isSingle: false,
        x: startX,
        y1: 0,
        y2: undefined,
        points1,
        area1,
        points2,
      };
    });
  }, [segments, plotWidth, plotHeight, stats.max, padding.left, padding.top, timeSpan, minTs, hasDualSeries]);

  const handlePointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (data.length === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));

    if (timeSpan > 0) {
      const targetTs = minTs + ratio * timeSpan;
      let low = 0;
      let high = data.length - 1;
      while (low <= high) {
        const mid = (low + high) >> 1;
        if (data[mid].timestamp < targetTs) {
          low = mid + 1;
        } else {
          high = mid - 1;
        }
      }

      let bestIdx = 0;
      let bestDiff = Infinity;
      for (const idx of [low - 1, low, low + 1]) {
        if (idx >= 0 && idx < data.length) {
          const diff = Math.abs(data[idx].timestamp - targetTs);
          if (diff < bestDiff) {
            bestDiff = diff;
            bestIdx = idx;
          }
        }
      }

      // Check whether targetTs is close to data[bestIdx] or in an empty gap
      const pixelDiff = Math.abs(getX(data[bestIdx].timestamp) - (padding.left + ratio * plotWidth));
      const isClose = bestDiff <= gapThreshold * 0.6 || pixelDiff <= 18;

      setHoverState({
        targetTs,
        nearestIndex: isClose ? bestIdx : null,
        isGap: !isClose,
      });
    } else {
      const index = Math.round(ratio * (data.length - 1));
      setHoverState({
        targetTs: data[index]?.timestamp ?? 0,
        nearestIndex: index,
        isGap: false,
      });
    }
  };

  const handlePointerLeave = () => {
    setHoverState(null);
  };

  const activePoint = hoverState && hoverState.nearestIndex !== null ? data[hoverState.nearestIndex] : null;
  const gradientId = `grad-${title.replace(/[^a-zA-Z0-9]/g, "")}`;

  return (
    <Paper
      elevation={0}
      sx={{
        p: 2,
        borderRadius: 2.5,
        border: "1px solid #E2E8F0",
        bgcolor: "#FFFFFF",
        position: "relative",
        overflow: "hidden",
      }}
    >
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1 }}>
        <Box>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}>
            {title}
          </Typography>
          <Box sx={{ display: "flex", alignItems: "baseline", gap: 1 }}>
            <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "1.2rem" }}>
              {hoverState
                ? activePoint
                  ? defaultFormat(activePoint.value)
                  : "No data"
                : defaultFormat(stats.current)}
            </Typography>
            {hasDualSeries && (
              <Typography variant="body2" sx={{ fontWeight: 600, color: color2 }}>
                {hoverState
                  ? activePoint && activePoint.value2 !== undefined
                    ? `/ ${defaultFormat(activePoint.value2)}`
                    : activePoint
                      ? ""
                      : "/ —"
                  : stats.current2 !== undefined
                    ? `/ ${defaultFormat(stats.current2)}`
                    : ""}
              </Typography>
            )}
          </Box>
        </Box>

        <Box sx={{ textAlign: "right" }}>
          {hoverState ? (
            <Box>
              <Typography variant="caption" sx={{ color: "#475569", fontWeight: 600 }}>
                {new Date((activePoint ? activePoint.timestamp : hoverState.targetTs) * 1000).toISOString()}
              </Typography>
              {hoverState.isGap && (
                <Typography variant="caption" sx={{ color: "#94A3B8", display: "block", fontSize: "0.7rem" }}>
                  (no data recorded)
                </Typography>
              )}
            </Box>
          ) : (
            <Box sx={{ display: "flex", gap: 1 }}>
              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                Avg: <strong style={{ color: "#334155" }}>{defaultFormat(stats.avg)}</strong>
              </Typography>
            </Box>
          )}
        </Box>
      </Box>

      {/* SVG Chart */}
      <Box ref={containerRef} sx={{ width: "100%", height, touchAction: "none", cursor: "crosshair" }}>
        {data.length === 0 ? (
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", height: "100%" }}>
            <Typography variant="caption" sx={{ color: "#94A3B8" }}>
              No metric data recorded yet
            </Typography>
          </Box>
        ) : (
          <svg
            viewBox={`0 0 ${width} ${height}`}
            style={{ width: "100%", height: "100%", display: "block" }}
            onPointerMove={handlePointerMove}
            onPointerLeave={handlePointerLeave}
          >
            <defs>
              <linearGradient id={gradientId} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color} stopOpacity="0.25" />
                <stop offset="100%" stopColor={color} stopOpacity="0.0" />
              </linearGradient>
            </defs>

            {/* Subtle horizontal grid lines & Y-axis scale values */}
            {[0, 0.5, 1].map((pct) => {
              const y = padding.top + plotHeight * (1 - pct);
              const tickVal = pct * stats.max;
              return (
                <g key={pct}>
                  <line
                    x1={padding.left}
                    y1={y}
                    x2={padding.left + plotWidth}
                    y2={y}
                    stroke="#F1F5F9"
                    strokeWidth="1"
                    strokeDasharray="4 4"
                  />
                  {pct > 0 && (
                    <text x={padding.left + 4} y={y - 3} fontSize="9" fill="#94A3B8" fontFamily="sans-serif">
                      {defaultFormat(tickVal)}
                    </text>
                  )}
                </g>
              );
            })}

            {/* Area fill for each continuous segment (broken across empty data gaps) */}
            {segmentPaths.map(
              (sp) =>
                !sp.isSingle &&
                sp.area1 && <path key={`area-${sp.id}`} d={sp.area1} fill={`url(#${gradientId})`} />,
            )}

            {/* Series 1 Line and isolated points */}
            {segmentPaths.map((sp) =>
              sp.isSingle ? (
                <circle key={`pt1-${sp.id}`} cx={sp.x} cy={sp.y1} r="2.5" fill={color} />
              ) : (
                <polyline
                  key={`poly1-${sp.id}`}
                  fill="none"
                  stroke={color}
                  strokeWidth="2.2"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  points={sp.points1}
                />
              ),
            )}

            {/* Series 2 Line and isolated points (if dual) */}
            {hasDualSeries &&
              segmentPaths.map((sp) =>
                sp.isSingle && sp.y2 !== undefined ? (
                  <circle key={`pt2-${sp.id}`} cx={sp.x} cy={sp.y2} r="2.5" fill={color2} />
                ) : sp.points2 ? (
                  <polyline
                    key={`poly2-${sp.id}`}
                    fill="none"
                    stroke={color2}
                    strokeWidth="2.2"
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    points={sp.points2}
                  />
                ) : null,
              )}

            {/* Interactive hover crosshair */}
            {hoverState && (
              <g>
                <line
                  x1={
                    activePoint
                      ? getX(activePoint.timestamp)
                      : padding.left + ((hoverState.targetTs - minTs) / Math.max(1, timeSpan)) * plotWidth
                  }
                  y1={padding.top}
                  x2={
                    activePoint
                      ? getX(activePoint.timestamp)
                      : padding.left + ((hoverState.targetTs - minTs) / Math.max(1, timeSpan)) * plotWidth
                  }
                  y2={padding.top + plotHeight}
                  stroke={hoverState.isGap ? "#CBD5E1" : "#94A3B8"}
                  strokeWidth="1.2"
                  strokeDasharray={hoverState.isGap ? "3 3" : "2 2"}
                />
                {activePoint && (
                  <>
                    <circle
                      cx={getX(activePoint.timestamp)}
                      cy={getY(activePoint.value)}
                      r="4"
                      fill="#FFFFFF"
                      stroke={color}
                      strokeWidth="2.5"
                    />
                    {hasDualSeries && activePoint.value2 !== undefined && (
                      <circle
                        cx={getX(activePoint.timestamp)}
                        cy={getY(activePoint.value2)}
                        r="4"
                        fill="#FFFFFF"
                        stroke={color2}
                        strokeWidth="2.5"
                      />
                    )}
                  </>
                )}
              </g>
            )}

            {/* Time label bottom: linear full time domain */}
            {timeSpan > 0 && (
              <>
                <text x={padding.left} y={height - 5} fontSize="10" fill="#94A3B8" fontFamily="sans-serif">
                  {new Date(minTs * 1000).toISOString()}
                </text>
                <text
                  x={padding.left + plotWidth}
                  y={height - 5}
                  textAnchor="end"
                  fontSize="10"
                  fill="#94A3B8"
                  fontFamily="sans-serif"
                >
                  {new Date(maxTs * 1000).toISOString()}
                </text>
              </>
            )}
          </svg>
        )}
      </Box>

      {/* Legend if dual series */}
      {hasDualSeries && (
        <Box sx={{ display: "flex", gap: 2, mt: 1, justifyContent: "flex-end" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: color }} />
            <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
              {label1}
            </Typography>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Box sx={{ width: 10, height: 10, borderRadius: "50%", bgcolor: color2 }} />
            <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
              {label2}
            </Typography>
          </Box>
        </Box>
      )}
    </Paper>
  );
};
