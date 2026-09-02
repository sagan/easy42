import React, { memo } from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { Box, Typography, Chip, IconButton, Tooltip } from '@mui/material';
import { Server, MoreVertical, Globe, HardDrive } from 'lucide-react';
import { Node, NodeStatus } from '../../types/api';

export interface NodeData {
  node: Node;
  status?: NodeStatus;
  onSelect: (node: Node) => void;
  [key: string]: unknown;
}

export const NodeCard: React.FC<NodeProps> = memo(({ data }) => {
  const nodeData = data as unknown as NodeData;
  const { node, status, onSelect } = nodeData;
  const isOnline = status ? status.connected : true;

  return (
    <Box
      onClick={() => onSelect(node)}
      sx={{
        width: 260,
        backgroundColor: '#FFFFFF',
        border: '1px solid',
        borderColor: isOnline ? '#E2E8F0' : 'rgba(225, 29, 72, 0.4)',
        borderRadius: 2.5,
        boxShadow: '0 4px 6px -1px rgba(0, 0, 0, 0.05), 0 2px 4px -2px rgba(0, 0, 0, 0.05)',
        overflow: 'hidden',
        cursor: 'pointer',
        transition: 'all 0.2s cubic-bezier(0.4, 0, 0.2, 1)',
        '&:hover': {
          transform: 'translateY(-2px)',
          borderColor: '#4F46E5',
          boxShadow: '0 10px 15px -3px rgba(79, 70, 229, 0.12), 0 4px 6px -4px rgba(79, 70, 229, 0.12)',
        },
      }}
    >
      {/* Handles for connections */}
      <Handle
        type="target"
        position={Position.Left}
        id="target"
        style={{
          width: 10,
          height: 10,
          backgroundColor: '#4F46E5',
          border: '2px solid #FFFFFF',
        }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="source"
        style={{
          width: 10,
          height: 10,
          backgroundColor: '#0891B2',
          border: '2px solid #FFFFFF',
        }}
      />

      {/* Header */}
      <Box
        sx={{
          p: 1.5,
          backgroundColor: '#F8FAFC',
          borderBottom: '1px solid #E2E8F0',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box
            sx={{
              width: 28,
              height: 28,
              borderRadius: 1.5,
              backgroundColor: 'rgba(79, 70, 229, 0.1)',
              color: '#4F46E5',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Server size={16} />
          </Box>
          <Box>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, lineHeight: 1.2, color: '#0F172A' }}>
              {node.name}
            </Typography>
            <Typography variant="caption" sx={{ color: '#64748B', fontSize: '0.7rem' }}>
              {node.host}
            </Typography>
          </Box>
        </Box>

        <Tooltip title="Node Details">
          <IconButton size="small" sx={{ color: '#94A3B8' }}>
            <MoreVertical size={16} />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Body */}
      <Box sx={{ p: 1.5, display: 'flex', flexDirection: 'column', gap: 1 }}>
        {/* Main IP & Iface */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
            <Globe size={13} color="#64748B" />
            <Typography variant="body2" className="mono-font" sx={{ fontWeight: 600, fontSize: '0.8rem', color: '#0891B2' }}>
              {node.ip}
            </Typography>
          </Box>
          <Chip
            icon={<HardDrive size={10} />}
            label={node.interface}
            size="small"
            sx={{
              height: 20,
              fontSize: '0.65rem',
              backgroundColor: '#F1F5F9',
              color: '#475569',
              border: '1px solid #E2E8F0',
            }}
          />
        </Box>

        {/* ASN & Entrypoints */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mt: 0.5 }}>
          <Chip
            label={`AS${node.asn}`}
            size="small"
            sx={{
              height: 22,
              fontSize: '0.7rem',
              fontWeight: 700,
              backgroundColor: 'rgba(79, 70, 229, 0.08)',
              color: '#4338CA',
              border: '1px solid rgba(79, 70, 229, 0.2)',
            }}
          />

          <Typography variant="caption" sx={{ color: '#64748B', fontSize: '0.7rem' }}>
            {node.entrypoints?.filter(e => e.ip && e.ip !== '').length || 0} Endpoints
          </Typography>
        </Box>
      </Box>
    </Box>
  );
});
