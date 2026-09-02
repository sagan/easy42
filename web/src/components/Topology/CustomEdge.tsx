import React from 'react';
import {
  BaseEdge,
  EdgeLabelRenderer,
  EdgeProps,
  getBezierPath,
} from '@xyflow/react';
import { Box, Typography } from '@mui/material';
import { Link } from '../../types/api';

export interface CustomEdgeData {
  link: Link;
  onSelect: (link: Link) => void;
  [key: string]: unknown;
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

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        style={{
          ...style,
          strokeWidth: 2,
          stroke: '#4F46E5',
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
            backgroundColor: '#FFFFFF',
            border: '1px solid #CBD5E1',
            borderRadius: 1.5,
            px: 1.2,
            py: 0.3,
            boxShadow: '0 2px 4px rgba(0, 0, 0, 0.08)',
            transition: 'all 0.15s ease',
            display: 'flex',
            alignItems: 'center',
            gap: 0.8,
            '&:hover': {
              backgroundColor: '#F8FAFC',
              borderColor: '#4F46E5',
              transform: `translate(-50%, -50%) translate(${labelX}px,${labelY}px) scale(1.05)`,
            },
          }}
        >
          {link ? (
            <>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{ fontSize: '0.65rem', fontWeight: 600, color: '#4F46E5' }}
              >
                {link.from.interface}:{link.from.listen_port}
              </Typography>
              <Typography variant="caption" sx={{ color: '#94A3B8', fontSize: '0.6rem' }}>
                ↔
              </Typography>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{ fontSize: '0.65rem', fontWeight: 600, color: '#0891B2' }}
              >
                {link.to.interface}:{link.to.listen_port}
              </Typography>
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
