import React from "react";
import { Box, BoxProps } from "@mui/material";

export interface Easy42LogoProps extends Omit<BoxProps, "children"> {
  size?: number | string;
  variant?: "mark" | "full";
  badge?: boolean;
}

/**
 * Easy42Logo represents the official branding mark for Easy42.
 * It features an interconnected mesh topology badge embodying DN42 and WireGuard overlay tunnels,
 * forming the iconic "42" overlay graph with node vertices.
 */
export const Easy42Logo: React.FC<Easy42LogoProps> = ({
  size = 32,
  variant = "mark",
  badge = true,
  sx,
  ...rest
}) => {
  return (
    <Box
      component="span"
      sx={{
        display: "inline-flex",
        alignItems: "center",
        justifyContent: "center",
        width: size,
        height: size,
        flexShrink: 0,
        verticalAlign: "middle",
        ...sx,
      }}
      {...rest}
    >
      <svg
        viewBox="0 0 512 512"
        width="100%"
        height="100%"
        style={{ display: "block", overflow: "visible" }}
      >
        <defs>
          <linearGradient id="easy42-bg-grad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#3730A3" />
            <stop offset="50%" stopColor="#4F46E5" />
            <stop offset="100%" stopColor="#0891B2" />
          </linearGradient>

          <radialGradient id="easy42-glow-grad" cx="30%" cy="20%" r="65%">
            <stop offset="0%" stopColor="#A5B4FC" stopOpacity="0.35" />
            <stop offset="100%" stopColor="#312E81" stopOpacity="0" />
          </radialGradient>

          <linearGradient id="easy42-link-grad" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#FFFFFF" />
            <stop offset="60%" stopColor="#E0E7FF" />
            <stop offset="100%" stopColor="#67E8F9" />
          </linearGradient>

          <linearGradient id="easy42-tunnel-grad" x1="0%" y1="50%" x2="100%" y2="50%">
            <stop offset="0%" stopColor="#38BDF8" stopOpacity="0.9" />
            <stop offset="100%" stopColor="#A78BFA" stopOpacity="0.9" />
          </linearGradient>

          <filter id="easy42-shadow" x="-20%" y="-20%" width="140%" height="140%">
            <feDropShadow dx="0" dy="4" stdDeviation="6" floodColor="#0F172A" floodOpacity="0.35" />
          </filter>

          <filter id="easy42-node-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feDropShadow dx="0" dy="2" stdDeviation="4" floodColor="#06B6D4" floodOpacity="0.5" />
          </filter>
        </defs>

        {badge && (
          <>
            <rect width="512" height="512" rx="116" fill="url(#easy42-bg-grad)" />
            <rect width="512" height="512" rx="116" fill="url(#easy42-glow-grad)" />
            <rect
              x="8"
              y="8"
              width="496"
              height="496"
              rx="110"
              fill="none"
              stroke="rgba(255, 255, 255, 0.22)"
              strokeWidth="5"
            />
          </>
        )}

        <g>
          {/* Overlay Mesh Interconnect Links (Background topology) */}
          <g
            stroke="url(#easy42-tunnel-grad)"
            strokeWidth="12"
            strokeLinecap="round"
            strokeDasharray="14 10"
            opacity="0.75"
          >
            <line x1="250" y1="265" x2="335" y2="305" />
            <line
              x1="165"
              y1="125"
              x2="355"
              y2="125"
              opacity="0.4"
              strokeDasharray="8 8"
              strokeWidth="8"
            />
            <line
              x1="230"
              y1="395"
              x2="290"
              y2="395"
              strokeDasharray="none"
              strokeWidth="14"
              opacity="0.9"
            />
          </g>

          {/* Main WireGuard Topology Paths */}
          <g filter="url(#easy42-shadow)">
            {/* Numeral 4 Structure */}
            <line
              x1="230"
              y1="145"
              x2="230"
              y2="395"
              stroke="url(#easy42-link-grad)"
              strokeWidth="28"
              strokeLinecap="round"
            />
            <polyline
              points="165,125 100,265 260,265"
              fill="none"
              stroke="url(#easy42-link-grad)"
              strokeWidth="28"
              strokeLinecap="round"
              strokeLinejoin="round"
            />

            {/* Numeral 2 Structure */}
            <path
              d="M 292,185 C 292,130 330,115 365,115 C 405,115 425,140 425,180 C 425,235 370,295 292,395 L 425,395"
              fill="none"
              stroke="url(#easy42-link-grad)"
              strokeWidth="28"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>

          {/* Network Nodes (Vertices) */}
          <g filter="url(#easy42-node-glow)">
            {/* Node 4A */}
            <circle cx="165" cy="125" r="14" fill="rgba(255,255,255,0.3)" />
            <circle cx="165" cy="125" r="9" fill="#FFFFFF" />

            {/* Node 4B */}
            <circle cx="100" cy="265" r="14" fill="rgba(255,255,255,0.3)" />
            <circle cx="100" cy="265" r="9" fill="#FFFFFF" />

            {/* Node 4C */}
            <circle cx="230" cy="265" r="16" fill="rgba(103, 232, 249, 0.4)" />
            <circle cx="230" cy="265" r="10" fill="#67E8F9" />

            {/* Node 4D */}
            <circle cx="230" cy="395" r="15" fill="rgba(255,255,255,0.3)" />
            <circle cx="230" cy="395" r="9" fill="#FFFFFF" />

            {/* Node 2A */}
            <circle cx="365" cy="115" r="14" fill="rgba(255,255,255,0.3)" />
            <circle cx="365" cy="115" r="9" fill="#FFFFFF" />

            {/* Node 2B */}
            <circle cx="425" cy="180" r="14" fill="rgba(255,255,255,0.3)" />
            <circle cx="425" cy="180" r="9" fill="#FFFFFF" />

            {/* Node 2C */}
            <circle cx="335" cy="305" r="12" fill="rgba(103, 232, 249, 0.4)" />
            <circle cx="335" cy="305" r="7" fill="#67E8F9" />

            {/* Node 2D */}
            <circle cx="292" cy="395" r="15" fill="rgba(255,255,255,0.3)" />
            <circle cx="292" cy="395" r="9" fill="#FFFFFF" />

            {/* Node 2E */}
            <circle cx="425" cy="395" r="15" fill="rgba(103, 232, 249, 0.4)" />
            <circle cx="425" cy="395" r="9" fill="#67E8F9" />
          </g>
        </g>
      </svg>
    </Box>
  );
};
