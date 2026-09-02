import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  CircularProgress,
  Alert,
  MenuItem,
  Divider,
} from '@mui/material';
import { Link as LinkIcon, ArrowRightLeft, Edit2 } from 'lucide-react';
import { api } from '../../api/client';
import { Node, Link } from '../../types/api';

interface AddLinkModalProps {
  open: boolean;
  nodes: Node[];
  initialFrom?: string;
  initialTo?: string;
  linkToEdit?: Link | null;
  onClose: () => void;
  onLinkAdded?: (link: Link) => void;
  onLinkUpdated?: (link: Link) => void;
  onNeedUnlock?: () => void;
}

export const AddLinkModal: React.FC<AddLinkModalProps> = ({
  open,
  nodes,
  initialFrom = '',
  initialTo = '',
  linkToEdit,
  onClose,
  onLinkAdded,
  onLinkUpdated,
  onNeedUnlock,
}) => {
  const [fromNodeName, setFromNodeName] = useState(initialFrom);
  const [toNodeName, setToNodeName] = useState(initialTo);
  const [fromPort, setFromPort] = useState<number>(0);
  const [toPort, setToPort] = useState<number>(0);
  const [fromMtu, setFromMtu] = useState<number>(1420);
  const [toMtu, setToMtu] = useState<number>(1420);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    if (linkToEdit) {
      setFromNodeName(linkToEdit.from.name);
      setToNodeName(linkToEdit.to.name);
      setFromPort(linkToEdit.from.listen_port);
      setToPort(linkToEdit.to.listen_port);
      setFromMtu(linkToEdit.from.mtu || 1420);
      setToMtu(linkToEdit.to.mtu || 1420);
      setError(null);
    } else {
      setFromNodeName(initialFrom || '');
      setToNodeName(initialTo || '');
      setFromPort(0);
      setToPort(0);
      setFromMtu(1420);
      setToMtu(1420);
      setError(null);
    }
  }, [open, linkToEdit, initialFrom, initialTo]);

  const fromNode = nodes.find((n) => n.name === fromNodeName);
  const toNode = nodes.find((n) => n.name === toNodeName);

  // Auto calculate default ports and MTUs only when creating new link
  useEffect(() => {
    if (linkToEdit) return;

    if (toNode) {
      setFromPort(Number(toNode.asn % 100000));
    }
    if (fromNode) {
      setToPort(Number(fromNode.asn % 100000));
    }

    const getUsedEpMTU = (targetNode?: Node, sourceNode?: Node) => {
      let foundMTU = 1500;
      if (targetNode?.entrypoints) {
        for (const ep of targetNode.entrypoints) {
          if (ep.ip && ep.mtu && ep.mtu > 0) {
            foundMTU = ep.mtu;
            break;
          }
        }
      }
      if (foundMTU === 1500 && sourceNode?.entrypoints) {
        for (const ep of sourceNode.entrypoints) {
          if (ep.ip && ep.mtu && ep.mtu > 0) {
            foundMTU = ep.mtu;
            break;
          }
        }
      }
      return foundMTU - 80;
    };

    if (fromNode || toNode) {
      setFromMtu(getUsedEpMTU(toNode, fromNode));
      setToMtu(getUsedEpMTU(fromNode, toNode));
    }
  }, [fromNode, toNode, linkToEdit]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!fromNodeName || !toNodeName || fromNodeName === toNodeName) {
      setError('Please select two distinct nodes');
      return;
    }

    setSubmitting(true);
    setError(null);

    try {
      if (linkToEdit) {
        const updated = await api.updateLink({
          from_node: fromNodeName,
          to_node: toNodeName,
          from_port: fromPort || undefined,
          to_port: toPort || undefined,
          from_mtu: fromMtu || undefined,
          to_mtu: toMtu || undefined,
        });
        onLinkUpdated?.(updated);
      } else {
        const link = await api.addLink({
          from_node: fromNodeName,
          to_node: toNodeName,
          from_port: fromPort || undefined,
          to_port: toPort || undefined,
          from_mtu: fromMtu || undefined,
          to_mtu: toMtu || undefined,
        });
        onLinkAdded?.(link);
      }
      onClose();
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423 && onNeedUnlock) {
        onClose();
        onNeedUnlock();
        return;
      }
      setError(e.message || (linkToEdit ? 'Failed to update link' : 'Failed to create link'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5, pb: 1, borderBottom: '1px solid #E2E8F0' }}>
        <Box
          sx={{
            width: 34,
            height: 34,
            borderRadius: 2,
            backgroundColor: 'rgba(8, 145, 178, 0.1)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#0891B2',
          }}
        >
          {linkToEdit ? <Edit2 size={18} /> : <LinkIcon size={18} />}
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: '#0F172A' }}>
          {linkToEdit ? `Edit WireGuard Link: ${linkToEdit.from.name} ↔ ${linkToEdit.to.name}` : 'Create WireGuard Link'}
        </Typography>
      </DialogTitle>

      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2.5, pt: 2.5 }}>
          {/* Node Selection */}
          <Box sx={{ display: 'grid', gridTemplateColumns: '1fr auto 1fr', gap: 1.5, alignItems: 'center' }}>
            <TextField
              select
              label="Source Node (From)"
              size="small"
              value={fromNodeName}
              onChange={(e) => setFromNodeName(e.target.value)}
              required
              disabled={submitting || Boolean(linkToEdit)}
            >
              {nodes.map((n) => (
                <MenuItem key={n.name} value={n.name} disabled={n.name === toNodeName}>
                  {n.name} ({n.ip})
                </MenuItem>
              ))}
            </TextField>

            <Box
              sx={{
                width: 32,
                height: 32,
                borderRadius: '50%',
                backgroundColor: '#F1F5F9',
                border: '1px solid #E2E8F0',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: '#64748B',
              }}
            >
              <ArrowRightLeft size={16} />
            </Box>

            <TextField
              select
              label="Target Node (To)"
              size="small"
              value={toNodeName}
              onChange={(e) => setToNodeName(e.target.value)}
              required
              disabled={submitting || Boolean(linkToEdit)}
            >
              {nodes.map((n) => (
                <MenuItem key={n.name} value={n.name} disabled={n.name === fromNodeName}>
                  {n.name} ({n.ip})
                </MenuItem>
              ))}
            </TextField>
          </Box>

          {linkToEdit && (
            <Typography variant="caption" sx={{ color: '#64748B', fontStyle: 'italic', mt: -1.5 }}>
              Endpoints are fixed for this WireGuard link. You can configure listen ports and MTUs below.
            </Typography>
          )}

          <Divider sx={{ borderColor: '#E2E8F0' }} />

          {/* Generated Link End 1 */}
          {fromNode && toNode && (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, letterSpacing: '0.5px' }}>
                WIREGUARD INTERFACE SPECIFICATIONS
              </Typography>

              {/* From Node End */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: '#EEF2FF',
                  border: '1px solid #C7D2FE',
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: '#3730A3', mb: 1.2 }}>
                  End 1: {fromNode.name}
                </Typography>
                <Box sx={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr 1fr', gap: 1.5 }}>
                  <TextField
                    label="Interface"
                    size="small"
                    value={`wg42${toNode.name}`}
                    disabled
                  />
                  <TextField
                    label="Listen Port"
                    type="number"
                    size="small"
                    value={fromPort}
                    onChange={(e) => setFromPort(Number(e.target.value))}
                    helperText={`Derived from AS${toNode.asn}`}
                  />
                  <TextField
                    label="MTU"
                    type="number"
                    size="small"
                    value={fromMtu}
                    onChange={(e) => setFromMtu(Number(e.target.value))}
                    helperText="Default: 1420 (-80 overhead)"
                  />
                </Box>
              </Box>

              {/* To Node End */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: '#ECFEFF',
                  border: '1px solid #A5F3FC',
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: '#0E7490', mb: 1.2 }}>
                  End 2: {toNode.name}
                </Typography>
                <Box sx={{ display: 'grid', gridTemplateColumns: '1.2fr 1fr 1fr', gap: 1.5 }}>
                  <TextField
                    label="Interface"
                    size="small"
                    value={`wg42${fromNode.name}`}
                    disabled
                  />
                  <TextField
                    label="Listen Port"
                    type="number"
                    size="small"
                    value={toPort}
                    onChange={(e) => setToPort(Number(e.target.value))}
                    helperText={`Derived from AS${fromNode.asn}`}
                  />
                  <TextField
                    label="MTU"
                    type="number"
                    size="small"
                    value={toMtu}
                    onChange={(e) => setToMtu(Number(e.target.value))}
                    helperText="Default: 1420 (-80 overhead)"
                  />
                </Box>
              </Box>
            </Box>
          )}

          {error && (
            <Alert severity="error" sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: '1px solid #E2E8F0', backgroundColor: '#F8FAFC' }}>
          <Button onClick={onClose} disabled={submitting} sx={{ color: '#64748B' }}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="contained"
            color="primary"
            disabled={submitting || !fromNodeName || !toNodeName || fromNodeName === toNodeName}
            startIcon={submitting ? <CircularProgress size={16} color="inherit" /> : null}
          >
            {linkToEdit
              ? (submitting ? 'Saving...' : 'Save Changes')
              : (submitting ? 'Creating Link...' : 'Create Link')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
