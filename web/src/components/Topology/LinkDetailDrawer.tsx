import React, { useState } from 'react';
import {
  Drawer,
  Box,
  Typography,
  IconButton,
  Button,
  Divider,
  CircularProgress,
  Alert,
  Tooltip,
  Chip,
} from '@mui/material';
import { X, Trash2, Link as LinkIcon, Key, ArrowRightLeft, Edit2, Activity } from 'lucide-react';
import { api } from '../../api/client';
import { Link, NetworkState } from '../../types/api';

interface LinkDetailDrawerProps {
  link: Link | null;
  networkState?: NetworkState | null;
  open: boolean;
  onClose: () => void;
  onEditLink: (link: Link) => void;
  onLinkDeleted: (from: string, to: string) => void;
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
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

export const LinkDetailDrawer: React.FC<LinkDetailDrawerProps> = ({
  link,
  networkState,
  open,
  onClose,
  onEditLink,
  onLinkDeleted,
}) => {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!link) return null;

  const handleDelete = async () => {
    if (!window.confirm(`Are you sure you want to delete the WireGuard link between "${link.from.name}" and "${link.to.name}"?`)) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteLink(link.from.name, link.to.name);
      onLinkDeleted(link.from.name, link.to.name);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || 'Failed to delete link');
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
          width: { xs: '100%', sm: 440 },
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
              backgroundColor: 'rgba(8, 145, 178, 0.1)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#0891B2',
            }}
          >
            <LinkIcon size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, color: '#0F172A' }}>
              WireGuard Link
            </Typography>
            <Typography variant="caption" className="mono-font" sx={{ color: '#64748B' }}>
              {link.from.name} ↔ {link.to.name}
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Tooltip title="Edit Link">
            <IconButton
              size="small"
              onClick={() => onEditLink(link)}
              sx={{ color: '#0891B2', '&:hover': { backgroundColor: 'rgba(8, 145, 178, 0.08)' } }}
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

      {/* Live Connection Health Card */}
      {(() => {
        const fromIface = networkState?.nodes?.[link.from.name]?.interfaces?.[link.from.interface];
        const toIface = networkState?.nodes?.[link.to.name]?.interfaces?.[link.to.interface];

        let workingState: 'working' | 'not_working' | 'unknown' = 'unknown';
        if (fromIface?.working_state === 'working' || toIface?.working_state === 'working') {
          workingState = 'working';
        } else if (fromIface?.working_state === 'not_working' || toIface?.working_state === 'not_working') {
          workingState = 'not_working';
        }

        const hsFrom = fromIface?.latest_handshake ? new Date(fromIface.latest_handshake) : null;
        const hsTo = toIface?.latest_handshake ? new Date(toIface.latest_handshake) : null;
        let newestHandshake: Date | null = null;
        if (hsFrom && hsTo) {
          newestHandshake = hsFrom >= hsTo ? hsFrom : hsTo;
        } else {
          newestHandshake = hsFrom || hsTo;
        }

        const totalRx = (fromIface?.transfer_rx_bytes || 0) + (toIface?.transfer_rx_bytes || 0);
        const totalTx = (fromIface?.transfer_tx_bytes || 0) + (toIface?.transfer_tx_bytes || 0);

        return (
          <Box
            sx={{
              p: 2,
              mb: 2.5,
              borderRadius: 2,
              backgroundColor:
                workingState === 'working'
                  ? '#ECFDF5'
                  : workingState === 'not_working'
                  ? '#FEF2F2'
                  : '#F8FAFC',
              border: '1px solid',
              borderColor:
                workingState === 'working'
                  ? '#A7F3D0'
                  : workingState === 'not_working'
                  ? '#FECDD3'
                  : '#E2E8F0',
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.5 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: '#1E293B', display: 'flex', alignItems: 'center', gap: 1 }}>
                <Activity size={16} color={workingState === 'working' ? '#059669' : workingState === 'not_working' ? '#DC2626' : '#64748B'} />
                Tunnel Health
              </Typography>
              <Chip
                label={workingState === 'working' ? 'WORKING' : workingState === 'not_working' ? 'NOT WORKING' : 'UNKNOWN'}
                size="small"
                sx={{
                  fontWeight: 700,
                  fontSize: '0.65rem',
                  bgcolor: workingState === 'working' ? '#10B981' : workingState === 'not_working' ? '#EF4444' : '#94A3B8',
                  color: '#FFFFFF',
                }}
              />
            </Box>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.8 }}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                <Typography variant="caption" sx={{ color: '#64748B' }}>Latest Handshake:</Typography>
                <Typography variant="caption" sx={{ fontWeight: 600, color: '#1E293B' }}>
                  {newestHandshake ? `${newestHandshake.toLocaleTimeString()} (${formatHandshakeAgo(newestHandshake.toISOString())} ago)` : 'None observed'}
                </Typography>
              </Box>
              {(totalRx > 0 || totalTx > 0) && (
                <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Typography variant="caption" sx={{ color: '#64748B' }}>Observed Transfer:</Typography>
                  <Typography variant="caption" sx={{ fontWeight: 600, color: '#1E293B' }}>
                    ↓ {formatBytes(totalRx)} / ↑ {formatBytes(totalTx)}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        );
      })()}

      {/* Specifications */}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2.5 }}>
        {/* End 1 */}
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: '#EEF2FF',
            border: '1px solid #C7D2FE',
          }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: '#3730A3', mb: 1.5 }}>
            Node 1: {link.from.name}
          </Typography>

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Interface:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.from.interface}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Link-Local IPv6:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0891B2', fontWeight: 600 }}>
                {link.from.address}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Listen Port:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.from.listen_port}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                MTU:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.from.mtu || 1420}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Peer Endpoint:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#D97706', fontWeight: 600 }}>
                {link.from.endpoint || 'Dynamic / None'}
              </Typography>
            </Box>

            <Box sx={{ mt: 0.5 }}>
              <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Key size={11} /> Public Key:
              </Typography>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{
                  display: 'block',
                  mt: 0.2,
                  p: 0.8,
                  backgroundColor: '#FFFFFF',
                  border: '1px solid #CBD5E1',
                  borderRadius: 1,
                  fontSize: '0.68rem',
                  wordBreak: 'break-all',
                  color: '#475569',
                }}
              >
                {link.from.public_key}
              </Typography>
            </Box>
          </Box>
        </Box>

        {/* Center Divider Icon */}
        <Box sx={{ display: 'flex', justifyContent: 'center', my: -1 }}>
          <Box
            sx={{
              p: 0.7,
              borderRadius: '50%',
              backgroundColor: '#FFFFFF',
              border: '1px solid #CBD5E1',
              color: '#64748B',
            }}
          >
            <ArrowRightLeft size={16} />
          </Box>
        </Box>

        {/* End 2 */}
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: '#ECFEFF',
            border: '1px solid #A5F3FC',
          }}
        >
          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: '#0E7490', mb: 1.5 }}>
            Node 2: {link.to.name}
          </Typography>

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Interface:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.to.interface}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Link-Local IPv6:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0891B2', fontWeight: 600 }}>
                {link.to.address}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Listen Port:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.to.listen_port}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                MTU:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#0F172A', fontWeight: 600 }}>
                {link.to.mtu || 1420}
              </Typography>
            </Box>

            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="caption" sx={{ color: '#64748B' }}>
                Peer Endpoint:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: '#D97706', fontWeight: 600 }}>
                {link.to.endpoint || 'Dynamic / None'}
              </Typography>
            </Box>

            <Box sx={{ mt: 0.5 }}>
              <Typography variant="caption" sx={{ color: '#64748B', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Key size={11} /> Public Key:
              </Typography>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{
                  display: 'block',
                  mt: 0.2,
                  p: 0.8,
                  backgroundColor: '#FFFFFF',
                  border: '1px solid #CBD5E1',
                  borderRadius: 1,
                  fontSize: '0.68rem',
                  wordBreak: 'break-all',
                  color: '#475569',
                }}
              >
                {link.to.public_key}
              </Typography>
            </Box>
          </Box>
        </Box>

        {error && (
          <Alert severity="error" sx={{ borderRadius: 2 }}>
            {error}
          </Alert>
        )}
      </Box>

      {/* Actions */}
      <Box sx={{ mt: 'auto', pt: 3, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
        <Button
          fullWidth
          variant="contained"
          color="primary"
          startIcon={<Edit2 size={16} />}
          onClick={() => onEditLink(link)}
          disabled={deleting}
        >
          Edit Link
        </Button>
        <Button
          fullWidth
          variant="outlined"
          color="error"
          startIcon={deleting ? <CircularProgress size={16} color="inherit" /> : <Trash2 size={16} />}
          onClick={handleDelete}
          disabled={deleting}
        >
          {deleting ? 'Deleting...' : 'Delete WireGuard Link'}
        </Button>
      </Box>
    </Drawer>
  );
};
