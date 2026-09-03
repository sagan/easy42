import React, { useState } from 'react';
import {
  Drawer,
  Box,
  Typography,
  IconButton,
  Button,
  Chip,
  Divider,
  CircularProgress,
  Alert,
  Tooltip,
} from '@mui/material';
import { X, Trash2, RefreshCw, Server, Globe, HardDrive, Shield, Activity, Edit2, Tag } from 'lucide-react';
import { api } from '../../api/client';
import { Node, NodeStatus } from '../../types/api';

interface NodeDetailDrawerProps {
  node: Node | null;
  status?: NodeStatus;
  open: boolean;
  onClose: () => void;
  onEditNode: (node: Node) => void;
  onNodeDeleted: (name: string) => void;
  onStatusRefreshed: (status: NodeStatus) => void;
}

export const NodeDetailDrawer: React.FC<NodeDetailDrawerProps> = ({
  node,
  status,
  open,
  onClose,
  onEditNode,
  onNodeDeleted,
  onStatusRefreshed,
}) => {
  const [refreshing, setRefreshing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!node) return null;

  const handleRefresh = async () => {
    setRefreshing(true);
    setError(null);
    try {
      const updated = await api.refreshNodeStatus(node.name);
      onStatusRefreshed(updated);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || 'Failed to refresh status');
    } finally {
      setRefreshing(false);
    }
  };

  const handleDelete = async () => {
    if (!window.confirm(`Are you sure you want to remove node "${node.name}" and all its connected links?`)) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteNode(node.name);
      onNodeDeleted(node.name);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || 'Failed to delete node');
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: '100%', sm: 420 },
          backgroundColor: '#FFFFFF',
          borderLeft: '1px solid #E2E8F0',
          p: 3,
        },
      }}
    >
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              backgroundColor: 'rgba(79, 70, 229, 0.1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#4F46E5',
            }}
          >
            <Server size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, color: '#0F172A' }}>
              {node.name}
            </Typography>
            <Typography variant="caption" className="mono-font" sx={{ color: '#64748B' }}>
              {node.host}
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Tooltip title="Edit Node">
            <IconButton
              size="small"
              onClick={() => onEditNode(node)}
              sx={{ color: '#4F46E5', '&:hover': { backgroundColor: 'rgba(79, 70, 229, 0.08)' } }}
            >
              <Edit2 size={18} />
            </IconButton>
          </Tooltip>
          <IconButton size="small" onClick={onClose} sx={{ color: '#94A3B8' }}>
            <X size={20} />
          </IconButton>
        </Box>
      </Box>

      <Divider sx={{ borderColor: '#E2E8F0', mb: 2.5 }} />

      {/* Quick Specs */}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, letterSpacing: '0.5px' }}>
          NODE CONFIGURATION
        </Typography>

        <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 1.5 }}>
          <Box sx={{ p: 1.5, borderRadius: 2, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
            <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Globe size={12} /> Main IPv4
            </Typography>
            <Typography variant="body2" className="mono-font" sx={{ fontWeight: 600, color: '#0891B2', mt: 0.5 }}>
              {node.ip}
            </Typography>
          </Box>

          <Box sx={{ p: 1.5, borderRadius: 2, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
            <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <HardDrive size={12} /> Interface
            </Typography>
            <Typography variant="body2" className="mono-font" sx={{ fontWeight: 600, color: '#0F172A', mt: 0.5 }}>
              {node.interface}
            </Typography>
          </Box>
        </Box>

        <Box sx={{ p: 1.5, borderRadius: 2, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
          <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5 }}>
            <Shield size={12} /> AS Number
          </Typography>
          <Typography variant="body2" className="mono-font" sx={{ fontWeight: 700, color: '#4F46E5', mt: 0.5 }}>
            AS{node.asn}
          </Typography>
        </Box>

        {/* Node Tags */}
        <Box sx={{ p: 1.5, borderRadius: 2, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
          <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5, mb: 0.5 }}>
            <Tag size={12} /> Tags
          </Typography>
          {node.tags && node.tags.length > 0 ? (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.5 }}>
              {node.tags.map((tag) => (
                <Chip
                  key={tag}
                  label={`#${tag}`}
                  size="small"
                  sx={{
                    height: 22,
                    fontSize: '0.7rem',
                    fontWeight: 600,
                    backgroundColor: 'rgba(8, 145, 178, 0.08)',
                    color: '#0891B2',
                    border: '1px solid rgba(8, 145, 178, 0.25)',
                  }}
                />
              ))}
            </Box>
          ) : (
            <Typography variant="caption" sx={{ color: '#94A3B8', fontStyle: 'italic', display: 'block', mt: 0.5 }}>
              No tags assigned
            </Typography>
          )}
        </Box>

        {/* External Entrypoints */}
        <Box>
          <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, mb: 1, display: 'block', letterSpacing: '0.5px' }}>
            ENTRYPOINTS ({node.entrypoints?.length || 0})
          </Typography>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            {node.entrypoints?.map((ep, i) => (
              <Box
                key={i}
                sx={{
                  p: 1.2,
                  borderRadius: 1.5,
                  backgroundColor: '#F8FAFC',
                  border: '1px solid #E2E8F0',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                }}
              >
                <Typography variant="caption" className="mono-font" sx={{ color: ep.ip ? '#0891B2' : '#D97706', fontWeight: 600 }}>
                  {ep.ip || 'Strictly NAT (Outbound only)'}
                </Typography>
                <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                  {ep.mtu && <Chip label={`MTU ${ep.mtu}`} size="small" variant="outlined" sx={{ fontSize: '0.65rem', height: 20 }} />}
                  <Chip label={ep.tags?.join(', ') || 'default'} size="small" sx={{ fontSize: '0.65rem' }} />
                </Box>
              </Box>
            ))}
          </Box>
        </Box>

        {/* Runtime Status */}
        {status && (
          <Box sx={{ mt: 1 }}>
            <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, mb: 1, display: 'block', letterSpacing: '0.5px' }}>
              RUNTIME STATUS
            </Typography>
            <Box
              sx={{
                p: 1.5,
                borderRadius: 2,
                backgroundColor: '#F8FAFC',
                border: '1px solid #E2E8F0',
                display: 'flex',
                flexDirection: 'column',
                gap: 1,
              }}
            >
              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Typography variant="caption" sx={{ color: '#64748B' }}>
                  Connectivity:
                </Typography>
                <Chip
                  icon={<Activity size={12} />}
                  label={status.connected ? 'Online' : 'Unreachable'}
                  size="small"
                  color={status.connected ? 'success' : 'error'}
                  sx={{ height: 20, fontSize: '0.7rem' }}
                />
              </Box>
              {status.hostname && (
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <Typography variant="caption" sx={{ color: '#64748B' }}>
                    Remote Hostname:
                  </Typography>
                  <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A' }}>
                    {status.hostname}
                  </Typography>
                </Box>
              )}
              {status.wg_interfaces && status.wg_interfaces.length > 0 && (
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                  <Typography variant="caption" sx={{ color: '#64748B' }}>
                    WireGuard Interfaces:
                  </Typography>
                  <Typography variant="caption" className="mono-font" sx={{ color: '#059669', fontWeight: 600 }}>
                    {status.wg_interfaces.map((w) => w.name).join(', ')}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        )}

        {error && (
          <Alert severity="error" sx={{ borderRadius: 2 }}>
            {error}
          </Alert>
        )}
      </Box>

      {/* Footer Actions */}
      <Box sx={{ mt: 'auto', pt: 3, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
        <Button
          fullWidth
          variant="contained"
          color="primary"
          startIcon={<Edit2 size={16} />}
          onClick={() => onEditNode(node)}
          disabled={refreshing || deleting}
        >
          Edit Node
        </Button>
        <Box sx={{ display: 'flex', gap: 1.5 }}>
          <Button
            fullWidth
            variant="outlined"
            startIcon={refreshing ? <CircularProgress size={16} color="inherit" /> : <RefreshCw size={16} />}
            onClick={handleRefresh}
            disabled={refreshing || deleting}
          >
            {refreshing ? 'Probing...' : 'Probe Live'}
          </Button>
          <Button
            fullWidth
            variant="outlined"
            color="error"
            startIcon={deleting ? <CircularProgress size={16} color="inherit" /> : <Trash2 size={16} />}
            onClick={handleDelete}
            disabled={refreshing || deleting}
          >
            {deleting ? 'Removing...' : 'Delete Node'}
          </Button>
        </Box>
      </Box>
    </Drawer>
  );
};
