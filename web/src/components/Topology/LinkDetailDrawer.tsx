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
} from '@mui/material';
import { X, Trash2, Link as LinkIcon, Key, ArrowRightLeft, Edit2 } from 'lucide-react';
import { api } from '../../api/client';
import { Link } from '../../types/api';

interface LinkDetailDrawerProps {
  link: Link | null;
  open: boolean;
  onClose: () => void;
  onEditLink: (link: Link) => void;
  onLinkDeleted: (from: string, to: string) => void;
}

export const LinkDetailDrawer: React.FC<LinkDetailDrawerProps> = ({
  link,
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
