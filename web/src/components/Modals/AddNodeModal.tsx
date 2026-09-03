import React, { useState } from 'react';
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
  IconButton,
  Tooltip,
  Chip,
} from '@mui/material';
import { Search, Plus, Trash2, Server, Globe, Shield, Edit2, Tag } from 'lucide-react';
import { api } from '../../api/client';
import { Node, Entrypoint } from '../../types/api';

interface AddNodeModalProps {
  open: boolean;
  nodeToEdit?: Node | null;
  onClose: () => void;
  onNodeAdded?: (node: Node) => void;
  onNodeUpdated?: (node: Node) => void;
}

interface EditableEntrypoint {
  id: string;
  ip: string;
  portStr: string;
  tagStr: string;
  mtuStr: string;
  isNone: boolean;
}

export const AddNodeModal: React.FC<AddNodeModalProps> = ({
  open,
  nodeToEdit,
  onClose,
  onNodeAdded,
  onNodeUpdated,
}) => {
  const [sshHost, setSshHost] = useState('');
  const [probing, setProb] = useState(false);
  const [probeError, setProbeError] = useState<string | null>(null);

  // Form state
  const [name, setName] = useState('');
  const [ip, setIp] = useState('');
  const [iface, setIface] = useState('lo');
  const [asn, setAsn] = useState<number>(4299420001);
  const [nodeTags, setNodeTags] = useState('');
  const [entrypoints, setEntrypoints] = useState<EditableEntrypoint[]>([
    { id: 'nat-fallback', ip: '', portStr: '', tagStr: 'nat', mtuStr: '', isNone: true },
  ]);
  const [discoveredIps, setDiscoveredIps] = useState<{ ip: string; iface: string }[]>([]);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  React.useEffect(() => {
    if (!open) return;
    if (nodeToEdit) {
      setName(nodeToEdit.name);
      setSshHost(nodeToEdit.host);
      setIp(nodeToEdit.ip);
      setIface(nodeToEdit.interface);
      setAsn(nodeToEdit.asn);
      setNodeTags(nodeToEdit.tags?.join(', ') || '');
      setDiscoveredIps([]);
      setProbeError(null);
      setSaveError(null);

      if (nodeToEdit.entrypoints && nodeToEdit.entrypoints.length > 0) {
        let hasNone = false;
        const mapped: EditableEntrypoint[] = nodeToEdit.entrypoints.map((ep, idx) => {
          const isNone = !ep.ip || ep.ip.trim() === '';
          if (isNone) hasNone = true;
          let portStr = '';
          if (ep.ports && ep.ports.length > 0) {
            const p = ep.ports[0];
            if (p.range) portStr = p.range;
            else if (p.external_port && p.port && p.external_port !== p.port) {
              portStr = `${p.port}:${p.external_port}`;
            } else if (p.port) {
              portStr = String(p.port);
            }
          }
          return {
            id: `ep-${idx}-${Date.now()}`,
            ip: ep.ip || '',
            portStr,
            tagStr: ep.tags?.join(', ') || (isNone ? 'nat' : 'direct'),
            mtuStr: ep.mtu ? String(ep.mtu) : (isNone ? '' : '1500'),
            isNone,
          };
        });
        if (!hasNone) {
          mapped.push({ id: 'nat-fallback', ip: '', portStr: '', tagStr: 'nat', mtuStr: '', isNone: true });
        }
        setEntrypoints(mapped);
      } else {
        setEntrypoints([
          { id: 'nat-fallback', ip: '', portStr: '', tagStr: 'nat', mtuStr: '', isNone: true },
        ]);
      }
    } else {
      setName('');
      setSshHost('');
      setIp('');
      setIface('lo');
      setAsn(4299420001);
      setNodeTags('');
      setEntrypoints([
        { id: 'nat-fallback', ip: '', portStr: '', tagStr: 'nat', mtuStr: '', isNone: true },
      ]);
      setDiscoveredIps([]);
      setProbeError(null);
      setSaveError(null);
    }
  }, [open, nodeToEdit]);

  const handleProbe = async () => {
    if (!sshHost.trim()) return;
    setProb(true);
    setProbeError(null);

    try {
      const res = await api.probeNode(sshHost.trim());
      setName(res.suggested_name);
      setIp(res.suggested_ip || '');
      setIface(res.suggested_interface || 'lo');
      setAsn(res.suggested_asn);

      if (res.detected_entrypoints && res.detected_entrypoints.length > 0) {
        const mapped: EditableEntrypoint[] = res.detected_entrypoints.map((ep, idx) => {
          const isNone = !ep.ip || ep.ip.trim() === '';
          let portStr = '';
          if (ep.ports && ep.ports.length > 0) {
            const p = ep.ports[0];
            if (p.range) portStr = p.range;
            else if (p.external_port && p.port && p.external_port !== p.port) {
              portStr = `${p.port}:${p.external_port}`;
            } else if (p.port) {
              portStr = String(p.port);
            }
          }
          return {
            id: `ep-${idx}-${Date.now()}`,
            ip: ep.ip || '',
            portStr,
            tagStr: ep.tags?.join(', ') || (isNone ? 'nat' : 'direct'),
            mtuStr: ep.mtu ? String(ep.mtu) : (isNone ? '' : '1500'),
            isNone,
          };
        });
        setEntrypoints(mapped);
      }

      // Collect all discovered interface IPs
      const ips: { ip: string; iface: string }[] = [];
      res.interfaces.forEach((inf) => {
        inf.addresses.forEach((addr) => {
          if (addr.includes('.')) {
            ips.push({ ip: addr.split('/')[0], iface: inf.name });
          }
        });
      });
      setDiscoveredIps(ips);
    } catch (err: unknown) {
      const e = err as Error;
      setProbeError(e.message || 'SSH probe failed');
    } finally {
      setProb(false);
    }
  };

  const handleAddEntrypoint = () => {
    const newEp: EditableEntrypoint = {
      id: `ep-${Date.now()}`,
      ip: '',
      portStr: '',
      tagStr: 'direct',
      mtuStr: '1500',
      isNone: false,
    };
    // Insert before the fixed NAT endpoint at the end
    const lastIsNone = entrypoints.length > 0 && entrypoints[entrypoints.length - 1].isNone;
    if (lastIsNone) {
      setEntrypoints([...entrypoints.slice(0, -1), newEp, entrypoints[entrypoints.length - 1]]);
    } else {
      setEntrypoints([...entrypoints, newEp]);
    }
  };

  const handleRemoveEntrypoint = (id: string) => {
    setEntrypoints(entrypoints.filter((e) => e.id !== id));
  };

  const handleUpdateEntrypoint = (id: string, field: keyof EditableEntrypoint, value: string) => {
    setEntrypoints((prev) =>
      prev.map((e) => (e.id === id ? { ...e, [field]: value } : e))
    );
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !sshHost.trim() || !ip.trim()) {
      setSaveError('Name, SSH Host, and Main IP are required');
      return;
    }

    setSaving(true);
    setSaveError(null);

    // Convert editable entrypoints to API format
    const finalEntrypoints: Entrypoint[] = entrypoints.map((ep) => {
      const tags = ep.tagStr
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);

      if (ep.isNone) {
        return {
          ip: '',
          tags: tags.length > 0 ? tags : ['nat'],
        };
      }

      const entry: Entrypoint = {
        ip: ep.ip.trim(),
        tags: tags.length > 0 ? tags : ['direct'],
      };

      if (ep.mtuStr && !isNaN(parseInt(ep.mtuStr, 10)) && parseInt(ep.mtuStr, 10) > 0) {
        entry.mtu = parseInt(ep.mtuStr, 10);
      }

      if (ep.portStr.trim()) {
        const pStr = ep.portStr.trim();
        if (pStr.includes('-')) {
          entry.ports = [{ range: pStr }];
        } else if (pStr.includes(':')) {
          const parts = pStr.split(':');
          const p = parseInt(parts[0], 10);
          const ext = parseInt(parts[1], 10);
          if (!isNaN(p)) {
            entry.ports = [{ port: p, external_port: isNaN(ext) ? p : ext }];
          }
        } else {
          const num = parseInt(pStr, 10);
          if (!isNaN(num)) {
            entry.ports = [{ port: num, external_port: num }];
          }
        }
      }

      return entry;
    });

    const parsedTags = nodeTags
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean);

    const newNode: Node = {
      name: name.trim(),
      host: sshHost.trim(),
      ip: ip.trim(),
      interface: iface.trim(),
      asn: Number(asn),
      entrypoints: finalEntrypoints,
      tags: parsedTags.length > 0 ? parsedTags : undefined,
      x: nodeToEdit?.x,
      y: nodeToEdit?.y,
    };

    try {
      if (nodeToEdit) {
        const saved = await api.updateNode(nodeToEdit.name, newNode);
        onNodeUpdated?.(saved);
      } else {
        const saved = await api.addNode(newNode);
        onNodeAdded?.(saved);
      }
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setSaveError(e.message || (nodeToEdit ? 'Failed to update node' : 'Failed to add node'));
    } finally {
      setSaving(false);
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
            backgroundColor: 'rgba(79, 70, 229, 0.1)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#4F46E5',
          }}
        >
          {nodeToEdit ? <Edit2 size={18} /> : <Server size={18} />}
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: '#0F172A' }}>
          {nodeToEdit ? `Edit Node: ${nodeToEdit.name}` : 'Add New Mesh Node'}
        </Typography>
      </DialogTitle>

      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2.5, pt: 2.5 }}>
          {/* Step 1: SSH Host & Probe */}
          <Box>
            <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, mb: 0.8, display: 'block', letterSpacing: '0.5px' }}>
              STEP 1: SSH CONNECTION
            </Typography>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <TextField
                fullWidth
                size="small"
                placeholder="e.g. router-gw1 or 192.168.1.1 or user@host"
                value={sshHost}
                onChange={(e) => setSshHost(e.target.value)}
                disabled={probing || saving}
              />
              <Button
                variant="contained"
                color="primary"
                size="small"
                onClick={handleProbe}
                disabled={probing || !sshHost.trim() || saving}
                startIcon={probing ? <CircularProgress size={16} color="inherit" /> : <Search size={16} />}
                sx={{ whiteSpace: 'nowrap', px: 2.5 }}
              >
                {probing ? 'Probing...' : 'Probe via SSH'}
              </Button>
            </Box>
            {probeError && (
              <Alert severity="error" sx={{ mt: 1.5, borderRadius: 2 }}>
                {probeError}
              </Alert>
            )}
          </Box>

          {/* Step 2: Node Properties */}
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, display: 'block', letterSpacing: '0.5px' }}>
              STEP 2: NODE SPECIFICATION
            </Typography>

            <Box sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 2 }}>
              <TextField
                label="Node Name (Max 11 chars)"
                size="small"
                value={name}
                onChange={(e) => setName(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '').slice(0, 11))}
                helperText="Unique hostname in mesh"
                required
                disabled={saving}
              />

              <TextField
                label="AS Number (ASN)"
                type="number"
                size="small"
                value={asn}
                onChange={(e) => setAsn(Number(e.target.value))}
                helperText="e.g. 4299420001 or DN42 ASN"
                required
                disabled={saving}
              />
            </Box>

            <Box sx={{ display: 'grid', gridTemplateColumns: '1.2fr 0.8fr', gap: 2 }}>
              {discoveredIps.length > 0 ? (
                <TextField
                  select
                  label="Main IPv4 Address"
                  size="small"
                  value={ip}
                  onChange={(e) => {
                    setIp(e.target.value);
                    const match = discoveredIps.find((item) => item.ip === e.target.value);
                    if (match) setIface(match.iface);
                  }}
                  required
                  disabled={saving}
                >
                  {discoveredIps.map((item) => (
                    <MenuItem key={item.ip} value={item.ip}>
                      {item.ip} ({item.iface})
                    </MenuItem>
                  ))}
                </TextField>
              ) : (
                <TextField
                  label="Main IPv4 Address"
                  size="small"
                  value={ip}
                  onChange={(e) => setIp(e.target.value)}
                  placeholder="e.g. 192.168.100.1"
                  required
                  disabled={saving}
                />
              )}

              <TextField
                label="Interface Name"
                size="small"
                value={iface}
                onChange={(e) => setIface(e.target.value)}
                placeholder="e.g. lo, dn42, eth0"
                required
                disabled={saving}
              />
            </Box>

            <Box>
              <TextField
                fullWidth
                label="Node Tags (comma-separated)"
                size="small"
                value={nodeTags}
                onChange={(e) => setNodeTags(e.target.value)}
                placeholder="e.g. core, eu, gateway"
                helperText="Categorize this node (e.g. core, eu, gateway) for grouping and filtering"
                disabled={saving}
              />
              {nodeTags.split(',').map((t) => t.trim()).filter(Boolean).length > 0 && (
                <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5, mt: 0.75, alignItems: 'center' }}>
                  <Tag size={13} color="#64748B" />
                  {nodeTags
                    .split(',')
                    .map((t) => t.trim())
                    .filter(Boolean)
                    .map((tag) => (
                      <Chip
                        key={tag}
                        label={`#${tag}`}
                        size="small"
                        sx={{
                          height: 20,
                          fontSize: '0.65rem',
                          fontWeight: 600,
                          backgroundColor: 'rgba(8, 145, 178, 0.08)',
                          color: '#0891B2',
                          border: '1px solid rgba(8, 145, 178, 0.25)',
                        }}
                      />
                    ))}
                </Box>
              )}
            </Box>
          </Box>

          {/* Step 3: Entrypoints */}
          <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1.2 }}>
              <Typography variant="caption" sx={{ color: '#475569', fontWeight: 700, letterSpacing: '0.5px' }}>
                STEP 3: EXTERNAL ENTRYPOINTS
              </Typography>
              <Button
                size="small"
                variant="outlined"
                startIcon={<Plus size={14} />}
                onClick={handleAddEntrypoint}
                sx={{ fontSize: '0.75rem', py: 0.3 }}
              >
                Add Endpoint
              </Button>
            </Box>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
              {entrypoints.map((ep) => {
                if (ep.isNone) {
                  return (
                    <Box
                      key={ep.id}
                      sx={{
                        display: 'flex',
                        flexDirection: { xs: 'column', sm: 'row' },
                        alignItems: { xs: 'flex-start', sm: 'center' },
                        justifyContent: 'space-between',
                        gap: 1.5,
                        p: 1.5,
                        borderRadius: 2,
                        backgroundColor: '#FFFBEB',
                        border: '1px solid #FDE68A',
                      }}
                    >
                      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, flex: 1, minWidth: 0 }}>
                        <Shield size={16} color="#D97706" style={{ marginTop: 2, flexShrink: 0 }} />
                        <Box>
                          <Typography variant="body2" sx={{ color: '#B45309', fontWeight: 700, fontSize: '0.8rem' }}>
                            Strictly NAT / Firewall Fallback (Outbound only)
                          </Typography>
                          <Typography variant="caption" sx={{ color: '#D97706', fontSize: '0.7rem' }}>
                            For nodes behind NAT that cannot accept inbound WireGuard connections.
                          </Typography>
                        </Box>
                      </Box>

                      <Box sx={{ width: { xs: '100%', sm: 180 }, flexShrink: 0 }}>
                        <TextField
                          label="Tags"
                          size="small"
                          placeholder="nat"
                          value={ep.tagStr}
                          onChange={(e) => handleUpdateEntrypoint(ep.id, 'tagStr', e.target.value)}
                          helperText="Default: nat"
                          fullWidth
                          sx={{
                            backgroundColor: '#FFFFFF',
                            borderRadius: 1,
                            '& .MuiInputBase-input': {
                              fontSize: '0.8rem',
                              py: 0.8,
                            },
                            '& .MuiInputLabel-root': {
                              fontSize: '0.8rem',
                            },
                            '& .MuiFormHelperText-root': {
                              fontSize: '0.65rem',
                              color: '#B45309',
                            },
                          }}
                        />
                      </Box>
                    </Box>
                  );
                }

                return (
                  <Box
                    key={ep.id}
                    sx={{
                      p: 1.5,
                      borderRadius: 2,
                      backgroundColor: '#F8FAFC',
                      border: '1px solid #E2E8F0',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 1.2,
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8 }}>
                        <Globe size={15} color="#0891B2" />
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, fontSize: '0.8rem', color: '#0F172A' }}>
                          Direct Entrypoint
                        </Typography>
                      </Box>
                      <Tooltip title="Remove entrypoint">
                        <IconButton size="small" onClick={() => handleRemoveEntrypoint(ep.id)} sx={{ color: '#94A3B8', '&:hover': { color: '#E11D48' } }}>
                          <Trash2 size={15} />
                        </IconButton>
                      </Tooltip>
                    </Box>

                    <Box sx={{ display: 'grid', gridTemplateColumns: '1.2fr 0.9fr 1fr 0.7fr', gap: 1.2 }}>
                      <TextField
                        label="Public IP or Domain"
                        size="small"
                        placeholder="e.g. 1.2.3.4 or gw.domain.com"
                        value={ep.ip}
                        onChange={(e) => handleUpdateEntrypoint(ep.id, 'ip', e.target.value)}
                        required
                      />

                      <TextField
                        label="Port(s) (Optional)"
                        size="small"
                        placeholder="e.g. 51820, 2000-2999"
                        value={ep.portStr}
                        onChange={(e) => handleUpdateEntrypoint(ep.id, 'portStr', e.target.value)}
                        helperText="Blank for auto port"
                      />

                      <TextField
                        label="Tags"
                        size="small"
                        placeholder="e.g. direct, wan1"
                        value={ep.tagStr}
                        onChange={(e) => handleUpdateEntrypoint(ep.id, 'tagStr', e.target.value)}
                        helperText="Comma separated"
                      />

                      <TextField
                        label="MTU"
                        type="number"
                        size="small"
                        placeholder="1500"
                        value={ep.mtuStr}
                        onChange={(e) => handleUpdateEntrypoint(ep.id, 'mtuStr', e.target.value)}
                        helperText="Default: 1500"
                      />
                    </Box>
                  </Box>
                );
              })}
            </Box>
          </Box>

          {saveError && (
            <Alert severity="error" sx={{ borderRadius: 2 }}>
              {saveError}
            </Alert>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: '1px solid #E2E8F0', backgroundColor: '#F8FAFC' }}>
          <Button onClick={onClose} disabled={saving} sx={{ color: '#64748B' }}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="contained"
            color="primary"
            disabled={saving || !name || !sshHost || !ip}
            startIcon={saving ? <CircularProgress size={16} color="inherit" /> : null}
          >
            {nodeToEdit
              ? (saving ? 'Saving...' : 'Save Changes')
              : (saving ? 'Adding Node...' : 'Add Node')}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
