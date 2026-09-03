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
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@mui/material';
import { X, Trash2, RefreshCw, Server, Globe, HardDrive, Shield, Activity, Edit2, Tag, Network, FileCode, Wrench } from 'lucide-react';
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
  onOpenHelper?: (nodeName: string) => void;
}

export const NodeDetailDrawer: React.FC<NodeDetailDrawerProps> = ({
  node,
  status,
  open,
  onClose,
  onEditNode,
  onNodeDeleted,
  onStatusRefreshed,
  onOpenHelper,
}) => {
  const [refreshing, setRefreshing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [viewingBird, setViewingBird] = useState(false);
  const [birdConfig, setBirdConfig] = useState<string | null>(null);
  const [loadingBird, setLoadingBird] = useState(false);

  if (!node) return null;

  const handleOpenBirdConfig = async () => {
    setViewingBird(true);
    setLoadingBird(true);
    try {
      const res = await api.getNodeBirdConfig(node.name);
      setBirdConfig(res.config);
    } catch (err: unknown) {
      const e = err as Error;
      setBirdConfig(`# Failed to load BIRD config: ${e.message}`);
    } finally {
      setLoadingBird(false);
    }
  };

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

        {/* BIRD / BGP Routing */}
        <Box>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
            <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, letterSpacing: '0.5px', display: 'flex', alignItems: 'center', gap: 0.8 }}>
              <Network size={14} color="#4F46E5" /> ROUTING (BIRD & BGP)
            </Typography>
            <Button
              size="small"
              variant="text"
              startIcon={<FileCode size={13} />}
              onClick={handleOpenBirdConfig}
              sx={{ fontSize: '0.7rem', py: 0.2, px: 1, color: '#4F46E5', fontWeight: 600 }}
            >
              View Config
            </Button>
          </Box>

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            {/* Kernel Routing Table */}
            <Box sx={{ p: 1.2, borderRadius: 1.5, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Export Table:
              </Typography>
              <Chip
                label={`Table ${node.table ?? 254}`}
                size="small"
                sx={{ height: 20, fontSize: '0.7rem', fontWeight: 600, backgroundColor: 'rgba(79, 70, 229, 0.08)', color: '#4F46E5' }}
              />
            </Box>

            {/* Static Routes */}
            <Box sx={{ p: 1.2, borderRadius: 1.5, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
              <Typography variant="caption" sx={{ color: '#64748B', display: 'block', mb: 0.5 }}>
                Static BGP Prefixes:
              </Typography>
              {node.static_routes && node.static_routes.length > 0 ? (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
                  {node.static_routes.map((sr) => (
                    <Chip
                      key={sr}
                      label={sr}
                      size="small"
                      className="mono-font"
                      sx={{ height: 20, fontSize: '0.65rem', backgroundColor: '#EDE9FE', color: '#6D28D9' }}
                    />
                  ))}
                </Box>
              ) : (
                <Typography variant="caption" sx={{ color: '#94A3B8', fontStyle: 'italic' }}>
                  None configured
                </Typography>
              )}
            </Box>

            {/* Kernel Route Imports */}
            <Box sx={{ p: 1.2, borderRadius: 1.5, backgroundColor: '#F8FAFC', border: '1px solid #E2E8F0' }}>
              <Typography variant="caption" sx={{ color: '#64748B', display: 'block', mb: 0.5 }}>
                Kernel Table Imports ({node.routes?.length || 0}):
              </Typography>
              {node.routes && node.routes.length > 0 ? (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.8 }}>
                  {node.routes.map((rule, idx) => (
                    <Box key={idx} sx={{ p: 0.8, borderRadius: 1, backgroundColor: '#FFFFFF', border: '1px solid #CBD5E1' }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.4 }}>
                        <Chip label={`Table ${rule.table}`} size="small" sx={{ height: 18, fontSize: '0.65rem', fontWeight: 600 }} />
                      </Box>
                      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.4 }}>
                        {rule.prefixes?.map((p) => (
                          <Chip key={p} label={p} size="small" variant="outlined" className="mono-font" sx={{ height: 18, fontSize: '0.62rem' }} />
                        ))}
                      </Box>
                    </Box>
                  ))}
                </Box>
              ) : (
                <Typography variant="caption" sx={{ color: '#94A3B8', fontStyle: 'italic' }}>
                  None configured
                </Typography>
              )}
            </Box>
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
        {onOpenHelper && (
          <Button
            fullWidth
            variant="outlined"
            startIcon={<Wrench size={16} />}
            onClick={() => onOpenHelper(node.name)}
            disabled={refreshing || deleting}
            sx={{
              borderColor: '#4F46E5',
              color: '#4F46E5',
              fontWeight: 600,
              '&:hover': {
                borderColor: '#3730A3',
                backgroundColor: 'rgba(79, 70, 229, 0.06)',
              },
            }}
          >
            Device Config Helper
          </Button>
        )}
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

      {/* BIRD Config Dialog */}
      <Dialog open={viewingBird} onClose={() => setViewingBird(false)} maxWidth="md" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', pb: 1, borderBottom: '1px solid #E2E8F0' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <FileCode size={20} color="#4F46E5" />
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: '1.05rem' }}>
              BIRD Configuration: {node.name}
            </Typography>
          </Box>
          <IconButton size="small" onClick={() => setViewingBird(false)}>
            <X size={18} />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ pt: 2, pb: 1 }}>
          <Alert severity="info" sx={{ mb: 2, fontSize: '0.8rem', borderRadius: 2 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 0.5 }}>
              Standard Device Path: <code>/etc/bird_easy42.conf</code>
            </Typography>
            <Typography variant="body2" sx={{ fontSize: '0.78rem', color: '#334155', mb: 0.5 }}>
              easy42 automatically deploys this configuration to <code>/etc/bird_easy42.conf</code> and executes <code>birdc configure</code> on the node during synchronization.
            </Typography>
            <Typography variant="body2" sx={{ fontSize: '0.78rem', color: '#334155' }}>
              Ensure your main BIRD config (<code>/etc/bird/bird.conf</code> or <code>/etc/bird.conf</code>) removes any conflicting default skeleton protocols and includes:
            </Typography>
            <Box component="code" sx={{ display: 'block', mt: 0.5, p: 0.8, bgcolor: '#EEF2FF', borderRadius: 1, fontFamily: 'monospace', color: '#4338CA', fontSize: '0.75rem', fontWeight: 600 }}>
              include "/etc/bird_easy42.conf";
            </Box>
          </Alert>

          {loadingBird ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
              <CircularProgress size={32} />
            </Box>
          ) : (
            <Box
              component="pre"
              className="mono-font"
              sx={{
                m: 0,
                p: 2,
                borderRadius: 2,
                backgroundColor: '#0F172A',
                color: '#38BDF8',
                fontSize: '0.78rem',
                lineHeight: 1.5,
                maxHeight: '60vh',
                overflow: 'auto',
                border: '1px solid #334155',
              }}
            >
              {birdConfig || '# No config generated'}
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, py: 1.5, borderTop: '1px solid #E2E8F0', backgroundColor: '#F8FAFC' }}>
          <Button
            size="small"
            variant="outlined"
            onClick={() => {
              if (birdConfig) {
                navigator.clipboard.writeText(birdConfig);
              }
            }}
            disabled={!birdConfig || loadingBird}
          >
            Copy Config
          </Button>
          <Button size="small" variant="contained" onClick={() => setViewingBird(false)}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Drawer>
  );
};

