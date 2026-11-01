import React, { useState, useRef, useMemo } from "react";
import { Box, Typography, Paper } from "@mui/material";

export interface DataPoint {
  timestamp: number;
  value: number;
  value2?: number; // optional second series (e.g. tx rate)
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
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);

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

  const { minTs, timeSpan } = useMemo(() => {
    if (data.length === 0) return { minTs: 0, timeSpan: 0 };
    const min = data[0].timestamp;
    const max = data[data.length - 1].timestamp;
    return { minTs: min, timeSpan: max - min };
  }, [data]);

  const getX = (d: DataPoint, i: number) => {
    if (timeSpan > 0) {
      return padding.left + Math.max(0, Math.min(1, (d.timestamp - minTs) / timeSpan)) * plotWidth;
    }
    return padding.left + (i / Math.max(1, data.length - 1)) * plotWidth;
  };

  const points1 = useMemo(() => {
    if (data.length === 0) return "";
    return data
      .map((d, i) => {
        const x = getX(d, i);
        const normY = (d.value - 0) / Math.max(1, stats.max);
        const y = padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
        return `${x},${y}`;
      })
      .join(" ");
  }, [data, plotWidth, plotHeight, stats.max, padding.left, padding.top, timeSpan, minTs]);

  const area1 = useMemo(() => {
    if (!points1 || data.length === 0) return "";
    const startX = getX(data[0], 0);
    const endX = getX(data[data.length - 1], data.length - 1);
    const bottomY = padding.top + plotHeight;
    return `M ${startX},${bottomY} L ${points1.replace(/ /g, " L ")} L ${endX},${bottomY} Z`;
  }, [points1, data, padding.left, padding.top, plotWidth, plotHeight, timeSpan, minTs]);

  const points2 = useMemo(() => {
    if (!hasDualSeries || data.length === 0) return "";
    return data
      .map((d, i) => {
        const val = d.value2 || 0;
        const x = getX(d, i);
        const normY = (val - 0) / Math.max(1, stats.max);
        const y = padding.top + plotHeight - Math.max(0, Math.min(plotHeight, normY * plotHeight));
        return `${x},${y}`;
      })
      .join(" ");
  }, [data, hasDualSeries, plotWidth, plotHeight, stats.max, padding.left, padding.top, timeSpan, minTs]);

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
      setHoverIndex(bestIdx);
    } else {
      const index = Math.round(ratio * (data.length - 1));
      setHoverIndex(index);
    }
  };

  const handlePointerLeave = () => {
    setHoverIndex(null);
  };

  const activePoint = hoverIndex !== null && data[hoverIndex] ? data[hoverIndex] : null;

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
              {activePoint ? defaultFormat(activePoint.value) : defaultFormat(stats.current)}
            </Typography>
            {hasDualSeries && (
              <Typography variant="body2" sx={{ fontWeight: 600, color: color2 }}>
                /{" "}
                {activePoint && activePoint.value2 !== undefined
                  ? defaultFormat(activePoint.value2)
                  : stats.current2 !== undefined
                    ? defaultFormat(stats.current2)
                    : ""}
              </Typography>
            )}
          </Box>
        </Box>

        <Box sx={{ textAlign: "right" }}>
          {activePoint ? (
            <Typography variant="caption" sx={{ color: "#475569", fontWeight: 600 }}>
              {new Date(activePoint.timestamp * 1000).toISOString()}
            </Typography>
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
              <linearGradient id={`grad-${title.replace(/\s+/g, "")}`} x1="0" y1="0" x2="0" y2="1">
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

            {/* Area fill */}
            {area1 && <path d={area1} fill={`url(#grad-${title.replace(/\s+/g, "")})`} />}

            {/* Series 1 Line */}
            {points1 && (
              <polyline
                fill="none"
                stroke={color}
                strokeWidth="2.2"
                strokeLinecap="round"
                strokeLinejoin="round"
                points={points1}
              />
            )}

            {/* Series 2 Line (if dual) */}
            {hasDualSeries && points2 && (
              <polyline
                fill="none"
                stroke={color2}
                strokeWidth="2.2"
                strokeLinecap="round"
                strokeLinejoin="round"
                points={points2}
              />
            )}

            {/* Interactive hover crosshair */}
            {hoverIndex !== null && data[hoverIndex] && (
              <g>
                <line
                  x1={getX(data[hoverIndex], hoverIndex)}
                  y1={padding.top}
                  x2={getX(data[hoverIndex], hoverIndex)}
                  y2={padding.top + plotHeight}
                  stroke="#94A3B8"
                  strokeWidth="1.2"
                  strokeDasharray="2 2"
                />
                <circle
                  cx={getX(data[hoverIndex], hoverIndex)}
                  cy={
                    padding.top +
                    plotHeight -
                    Math.max(0, Math.min(plotHeight, (data[hoverIndex].value / Math.max(1, stats.max)) * plotHeight))
                  }
                  r="4"
                  fill="#FFFFFF"
                  stroke={color}
                  strokeWidth="2.5"
                />
              </g>
            )}

            {/* Time label bottom */}
            {data.length > 1 && (
              <>
                <text x={padding.left} y={height - 5} fontSize="10" fill="#94A3B8" fontFamily="sans-serif">
                  {new Date(data[0].timestamp * 1000).toISOString()}
                </text>
                <text
                  x={padding.left + plotWidth}
                  y={height - 5}
                  textAnchor="end"
                  fontSize="10"
                  fill="#94A3B8"
                  fontFamily="sans-serif"
                >
                  {new Date(data[data.length - 1].timestamp * 1000).toISOString()}
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
