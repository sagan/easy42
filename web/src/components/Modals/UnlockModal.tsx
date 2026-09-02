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
} from '@mui/material';
import { KeyRound } from 'lucide-react';
import { api } from '../../api/client';

interface UnlockModalProps {
  open: boolean;
  onClose: () => void;
  onUnlocked: () => void;
}

export const UnlockModal: React.FC<UnlockModalProps> = ({ open, onClose, onUnlocked }) => {
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleUnlock = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;

    setLoading(true);
    setError(null);

    try {
      await api.unlock(password);
      setPassword('');
      onUnlocked();
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || 'Incorrect password');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5, pb: 1, borderBottom: '1px solid #E2E8F0' }}>
        <Box
          sx={{
            width: 34,
            height: 34,
            borderRadius: 2,
            backgroundColor: 'rgba(217, 119, 6, 0.1)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#D97706',
          }}
        >
          <KeyRound size={18} />
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: '#0F172A' }}>
          Unlock easy42 Vault
        </Typography>
      </DialogTitle>

      <form onSubmit={handleUnlock}>
        <DialogContent sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 2.5 }}>
          <Typography variant="body2" sx={{ color: '#64748B' }}>
            The master key vault is locked in memory. Please enter your administrator password to unlock sensitive WireGuard keys.
          </Typography>

          <TextField
            fullWidth
            type="password"
            label="Master Password"
            size="small"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoFocus
            required
            disabled={loading}
          />

          {error && (
            <Alert severity="error" sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: '1px solid #E2E8F0', backgroundColor: '#F8FAFC' }}>
          <Button onClick={onClose} disabled={loading} sx={{ color: '#64748B' }}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="contained"
            color="warning"
            disabled={loading || !password}
            startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
            sx={{ fontWeight: 700, color: '#FFFFFF' }}
          >
            {loading ? 'Unlocking...' : 'Unlock Vault'}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
