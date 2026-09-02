import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  CircularProgress,
  Alert,
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import { RefreshCw, CheckCircle2, XCircle, ChevronDown, FileCode, Check, Trash2 } from 'lucide-react';
import { api } from '../../api/client';
import { SyncAction, SyncResult } from '../../types/api';

interface SyncProgressModalProps {
  open: boolean;
  onClose: () => void;
  onSyncComplete: () => void;
  onNeedUnlock?: () => void;
}

export const SyncProgressModal: React.FC<SyncProgressModalProps> = ({
  open,
  onClose,
  onSyncComplete,
  onNeedUnlock,
}) => {
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [actions, setActions] = useState<SyncAction[]>([]);
  const [previewError, setPreviewError] = useState<string | null>(null);

  const [syncing, setSyncing] = useState(false);
  const [results, setResults] = useState<SyncResult[] | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setResults(null);
      setSyncError(null);
      loadPreview();
    }
  }, [open]);

  const loadPreview = async () => {
    setLoadingPreview(true);
    setPreviewError(null);

    try {
      const data = await api.getSyncPreview();
      setActions(data || []);
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423 && onNeedUnlock) {
        onClose();
        onNeedUnlock();
        return;
      }
      setPreviewError(e.message || 'Failed to generate sync preview');
    } finally {
      setLoadingPreview(false);
    }
  };

  const handleExecuteSync = async () => {
    setSyncing(true);
    setSyncError(null);

    try {
      const res = await api.executeSync();
      setResults(res || []);
      onSyncComplete();
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423 && onNeedUnlock) {
        onClose();
        onNeedUnlock();
        return;
      }
      setSyncError(e.message || 'Sync execution failed');
    } finally {
      setSyncing(false);
    }
  };

  const safeActions = actions || [];
  const safeResults = results;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
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
          <RefreshCw size={18} />
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: '#0F172A' }}>
          Synchronize Mesh Topology
        </Typography>
      </DialogTitle>

      <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 2.5 }}>
        {loadingPreview ? (
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', py: 6, gap: 2 }}>
            <CircularProgress size={24} color="primary" />
            <Typography variant="body2" sx={{ color: '#64748B' }}>
              Computing WireGuard peer configurations and diffs...
            </Typography>
          </Box>
        ) : previewError ? (
          <Alert severity="error" sx={{ borderRadius: 2 }}>
            {previewError}
          </Alert>
        ) : safeResults ? (
          /* Execution Results */
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            <Alert
              severity={safeResults.every((r) => r.success) ? 'success' : 'warning'}
              sx={{ borderRadius: 2 }}
            >
              {safeResults.every((r) => r.success)
                ? 'All node configurations synchronized successfully!'
                : 'Some actions encountered errors. Review details below.'}
            </Alert>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, maxHeight: 360, overflowY: 'auto' }}>
              {safeResults.map((res, i) => (
                <Box
                  key={i}
                  sx={{
                    p: 1.5,
                    borderRadius: 2,
                    backgroundColor: '#FFFFFF',
                    border: '1px solid',
                    borderColor: res.success ? '#A7F3D0' : '#FECDD3',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    {res.success ? (
                      <CheckCircle2 size={18} color="#059669" />
                    ) : (
                      <XCircle size={18} color="#E11D48" />
                    )}
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: '#0F172A' }}>
                        {res.node_name} — {res.action}
                      </Typography>
                      {res.error && (
                        <Typography variant="caption" sx={{ color: '#E11D48', display: 'block' }}>
                          {res.error}
                        </Typography>
                      )}
                    </Box>
                  </Box>
                  <Chip
                    label={`${res.duration_ms.toFixed(0)} ms`}
                    size="small"
                    sx={{ fontSize: '0.7rem', backgroundColor: '#F1F5F9' }}
                  />
                </Box>
              ))}
            </Box>
          </Box>
        ) : (
          /* Action Plan Preview */
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <Typography variant="body2" sx={{ color: '#64748B' }}>
                {safeActions.length === 0
                  ? 'No WireGuard links to synchronize.'
                  : `${safeActions.length} configuration updates planned across nodes:`}
              </Typography>
              <Chip
                label={`${safeActions.length} Actions`}
                size="small"
                color="primary"
                sx={{ fontWeight: 700 }}
              />
            </Box>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, maxHeight: 380, overflowY: 'auto' }}>
              {safeActions.map((act, i) => (
                <Accordion
                  key={i}
                  elevation={0}
                  sx={{
                    backgroundColor: '#FFFFFF',
                    border: '1px solid #E2E8F0',
                    borderRadius: '8px !important',
                    '&:before': { display: 'none' },
                  }}
                >
                  <AccordionSummary expandIcon={<ChevronDown size={18} color="#64748B" />}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flex: 1 }}>
                      {act.type === 'delete_config' || act.type === 'down_interface' ? (
                        <Trash2 size={16} color="#DC2626" />
                      ) : (
                        <FileCode size={16} color="#4F46E5" />
                      )}
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="body2" sx={{ fontWeight: 600, color: '#0F172A' }}>
                          {act.description}
                        </Typography>
                        <Typography variant="caption" className="mono-font" sx={{ color: '#64748B', fontSize: '0.7rem' }}>
                          {act.host}:{act.target_file}
                        </Typography>
                      </Box>
                      {(act.type === 'delete_config' || act.type === 'down_interface') && (
                        <Chip
                          label="DELETE"
                          size="small"
                          sx={{
                            fontSize: '0.65rem',
                            fontWeight: 700,
                            height: 20,
                            bgcolor: '#FEE2E2',
                            color: '#DC2626',
                            mr: 1,
                          }}
                        />
                      )}
                    </Box>
                  </AccordionSummary>
                  <AccordionDetails sx={{ pt: 0 }}>
                    <Box
                      component="pre"
                      className="mono-font"
                      sx={{
                        p: 1.5,
                        m: 0,
                        borderRadius: 1.5,
                        backgroundColor: '#F8FAFC',
                        border: '1px solid #E2E8F0',
                        fontSize: '0.75rem',
                        color: '#1E293B',
                        overflowX: 'auto',
                        whiteSpace: 'pre-wrap',
                      }}
                    >
                      {act.file_content}
                    </Box>
                  </AccordionDetails>
                </Accordion>
              ))}
            </Box>

            {syncError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {syncError}
              </Alert>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 2, borderTop: '1px solid #E2E8F0', backgroundColor: '#F8FAFC' }}>
        <Button onClick={onClose} disabled={syncing} sx={{ color: '#64748B' }}>
          {safeResults ? 'Close' : 'Cancel'}
        </Button>
        {!safeResults && (
          <Button
            variant="contained"
            color="primary"
            onClick={handleExecuteSync}
            disabled={syncing || safeActions.length === 0}
            startIcon={syncing ? <CircularProgress size={16} color="inherit" /> : <Check size={16} />}
          >
            {syncing ? 'Pushing to Nodes...' : 'Apply Mesh Configs'}
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};
