import React from 'react';
import {
  AppBar,
  Toolbar,
  Typography,
  Button,
  Box,
  Chip,
  IconButton,
  Tooltip,
} from '@mui/material';
import {
  Plus,
  Link as LinkIcon,
  RefreshCw,
  Lock,
  Unlock,
  LogOut,
  Network,
  Server,
  Layers,
  Settings,
} from 'lucide-react';

interface NavbarProps {
  nodeCount: number;
  linkCount: number;
  isUnlocked: boolean;
  onAddNode: () => void;
  onAddLink: () => void;
  onSync: () => void;
  onUnlockToggle: () => void;
  onOpenSettings: () => void;
  onLogout: () => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  nodeCount,
  linkCount,
  isUnlocked,
  onAddNode,
  onAddLink,
  onSync,
  onUnlockToggle,
  onOpenSettings,
  onLogout,
}) => {
  return (
    <AppBar
      position="static"
      elevation={0}
      sx={{
        backgroundColor: '#FFFFFF',
        borderBottom: '1px solid #E2E8F0',
        zIndex: 10,
      }}
    >
      <Toolbar sx={{ justifyContent: 'space-between', minHeight: 64 }}>
        {/* Brand */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 38,
              height: 38,
              borderRadius: 2,
              background: 'linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)',
              boxShadow: '0 2px 8px rgba(79, 70, 229, 0.3)',
            }}
          >
            <Network size={22} color="#FFFFFF" />
          </Box>
          <Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Typography variant="h6" sx={{ fontWeight: 800, letterSpacing: '-0.5px', lineHeight: 1, color: '#0F172A' }}>
                easy<span style={{ color: '#0891B2' }}>42</span>
              </Typography>
              <Chip
                label="Mesh Manager"
                size="small"
                sx={{
                  height: 20,
                  fontSize: '0.65rem',
                  fontWeight: 700,
                  backgroundColor: 'rgba(79, 70, 229, 0.08)',
                  color: '#4F46E5',
                  border: '1px solid rgba(79, 70, 229, 0.2)',
                }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: '#64748B', fontSize: '0.75rem' }}>
              Linux & WireGuard Overlay Network
            </Typography>
          </Box>
        </Box>

        {/* Mesh Stats */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Chip
            icon={<Server size={14} color="#4F46E5" />}
            label={`${nodeCount} Nodes`}
            size="small"
            sx={{
              backgroundColor: '#F1F5F9',
              border: '1px solid #E2E8F0',
              color: '#334155',
              fontWeight: 600,
              px: 0.5,
            }}
          />
          <Chip
            icon={<Layers size={14} color="#0891B2" />}
            label={`${linkCount} Links`}
            size="small"
            sx={{
              backgroundColor: '#F1F5F9',
              border: '1px solid #E2E8F0',
              color: '#334155',
              fontWeight: 600,
              px: 0.5,
            }}
          />
        </Box>

        {/* Action Controls */}
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={onAddNode}
            sx={{
              borderColor: '#CBD5E1',
              color: '#1E293B',
              '&:hover': {
                borderColor: '#4F46E5',
                backgroundColor: 'rgba(79, 70, 229, 0.05)',
              },
            }}
          >
            Add Node
          </Button>

          <Button
            variant="outlined"
            size="small"
            startIcon={<LinkIcon size={16} />}
            onClick={onAddLink}
            sx={{
              borderColor: '#CBD5E1',
              color: '#1E293B',
              '&:hover': {
                borderColor: '#0891B2',
                backgroundColor: 'rgba(8, 145, 178, 0.05)',
              },
            }}
          >
            Add Link
          </Button>

          <Button
            variant="contained"
            color="primary"
            size="small"
            startIcon={<RefreshCw size={16} />}
            onClick={onSync}
            sx={{
              background: 'linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)',
              fontWeight: 700,
              color: '#FFFFFF',
            }}
          >
            Sync Mesh
          </Button>

          {/* Settings */}
          <Tooltip title="Settings">
            <IconButton
              size="small"
              onClick={onOpenSettings}
              sx={{
                border: '1px solid #E2E8F0',
                color: '#64748B',
                '&:hover': {
                  color: '#4F46E5',
                  borderColor: '#CBD5E1',
                  backgroundColor: 'rgba(79, 70, 229, 0.06)',
                },
              }}
            >
              <Settings size={18} />
            </IconButton>
          </Tooltip>

          {/* Vault Lock/Unlock */}
          <Tooltip title={isUnlocked ? "Vault is Unlocked (Click to Lock)" : "Vault is Locked (Click to Unlock)"}>
            <IconButton
              size="small"
              onClick={onUnlockToggle}
              sx={{
                border: '1px solid',
                borderColor: isUnlocked ? 'rgba(5, 150, 105, 0.3)' : 'rgba(217, 119, 6, 0.3)',
                backgroundColor: isUnlocked ? 'rgba(5, 150, 105, 0.08)' : 'rgba(217, 119, 6, 0.08)',
                color: isUnlocked ? '#059669' : '#D97706',
              }}
            >
              {isUnlocked ? <Unlock size={18} /> : <Lock size={18} />}
            </IconButton>
          </Tooltip>

          {/* Logout */}
          <Tooltip title="Logout">
            <IconButton
              size="small"
              onClick={onLogout}
              sx={{
                border: '1px solid #E2E8F0',
                color: '#64748B',
                '&:hover': { color: '#E11D48', backgroundColor: 'rgba(225, 29, 72, 0.06)' },
              }}
            >
              <LogOut size={18} />
            </IconButton>
          </Tooltip>
        </Box>
      </Toolbar>
    </AppBar>
  );
};
