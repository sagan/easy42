import React from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
} from '@xyflow/react';
import { Box, Typography } from '@mui/material';
import { Link } from '../../types/api';

export type LinkWorkingState = 'working' | 'not_working' | 'unknown';

export interface CustomEdgeData {
  link: Link;
  workingState?: LinkWorkingState;
  latestHandshake?: string;
  transferRxBytes?: number;
  transferTxBytes?: number;
  onSelect: (link: Link) => void;
  [key: string]: unknown;
}

function formatHandshakeAgo(dateStr?: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const diffSec = Math.floor((Date.now() - date.getTime()) / 1000);
  if (diffSec < 0) return 'now';
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
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetPosition,
    targetX,
    targetY,
  });

  const edgeData = data as unknown as CustomEdgeData;
  const link = edgeData?.link;
  const workingState: LinkWorkingState = edgeData?.workingState || 'unknown';
  const latestHandshake = edgeData?.latestHandshake;

  // Determine styles according to derived working state
  let strokeColor = '#94A3B8';
  let strokeWidth = 2;
  let strokeDasharray: string | undefined = '4 4';
  let pillBorderColor = '#CBD5E1';
  let pillBgColor = '#FFFFFF';
  let dotColor = '#94A3B8';
  let statusText = 'Idle';

  if (workingState === 'working') {
    strokeColor = '#10B981';
    strokeWidth = 2.5;
    strokeDasharray = undefined;
    pillBorderColor = '#6EE7B7';
    pillBgColor = '#F0FDF4';
    dotColor = '#10B981';
    statusText = latestHandshake ? formatHandshakeAgo(latestHandshake) : 'Active';
  } else if (workingState === 'not_working') {
    strokeColor = '#EF4444';
    strokeWidth = 2;
    strokeDasharray = '6 4';
    pillBorderColor = '#FCA5A5';
    pillBgColor = '#FEF2F2';
    dotColor = '#EF4444';
    statusText = 'Down';
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
          transition: 'stroke 0.2s ease, stroke-width 0.2s ease',
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
            position: 'absolute',
            transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px)`,
            pointerEvents: 'all',
          }}
          sx={{
            cursor: 'pointer',
            backgroundColor: pillBgColor,
            border: '1px solid',
            borderColor: pillBorderColor,
            borderRadius: 2,
            px: 1.2,
            py: 0.4,
            boxShadow: '0 2px 5px rgba(0, 0, 0, 0.06)',
            transition: 'all 0.15s ease',
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            '&:hover': {
              borderColor: strokeColor,
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px) scale(1.06)`,
              boxShadow: '0 4px 10px rgba(0, 0, 0, 0.12)',
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
                  borderRadius: '50%',
                  backgroundColor: dotColor,
                  boxShadow: workingState === 'working' ? '0 0 6px #10B981' : undefined,
                  flexShrink: 0,
                }}
              />

              <Typography
                variant="caption"
                className="mono-font"
                sx={{ fontSize: '0.66rem', fontWeight: 600, color: '#1E293B' }}
              >
                {link.from.interface}
              </Typography>
              <Typography variant="caption" sx={{ color: '#94A3B8', fontSize: '0.55rem' }}>
                ↔
              </Typography>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{ fontSize: '0.66rem', fontWeight: 600, color: '#1E293B' }}
              >
                {link.to.interface}
              </Typography>

              {/* Status Badge */}
              <Box
                sx={{
                  fontSize: '0.62rem',
                  fontWeight: 700,
                  color: dotColor,
                  borderRadius: 1,
                  px: 0.5,
                  py: 0.1,
                  backgroundColor: 'rgba(0,0,0,0.03)',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.3,
                }}
              >
                {workingState === 'working' && '⚡'}
                {statusText}
              </Box>
            </>
          ) : (
            <Typography variant="caption" sx={{ fontSize: '0.7rem', color: '#64748B' }}>
              WG Link
            </Typography>
          )}
        </Box>
      </EdgeLabelRenderer>
    </>
  );
};
