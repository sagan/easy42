import React, { useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import {
  AppBar,
  Toolbar,
  Typography,
  Box,
  Paper,
  IconButton,
  Button,
  Chip,
  Tooltip,
  Drawer,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Divider,
  Snackbar,
  Alert,
  BottomNavigation,
  BottomNavigationAction,
  useTheme,
  useMediaQuery,
} from "@mui/material";
import {
  Server,
  Network,
  Compass,
  Wrench,
  Settings,
  Lock,
  Unlock,
  RefreshCw,
  LogOut,
  Menu,
  X,
  Share2,
  Activity,
} from "lucide-react";
import { useMesh } from "../../context/MeshContext";
import { api } from "../../api/client";
import { AddNodeModal } from "../Modals/AddNodeModal";
import { AddLinkModal } from "../Modals/AddLinkModal";
import { RenameNodeModal } from "../Modals/RenameNodeModal";
import { UnlockModal } from "../Modals/UnlockModal";
import { SyncProgressModal } from "../Modals/SyncProgressModal";

export const AppLayout: React.FC = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const location = useLocation();
  const navigate = useNavigate();

  const {
    nodes,
    fleetLive,
    isUnlocked,
    setIsUnlocked,
    setAuthenticated,
    loadData,
    refreshFleetLive,
    updatingState,
    addNodeOpen,
    setAddNodeOpen,
    nodeToEdit,
    renameModalOpen,
    setRenameModalOpen,
    nodeToRename,
    addLinkOpen,
    setAddLinkOpen,
    linkToEdit,
    connectFrom,
    connectTo,
    unlockOpen,
    setUnlockOpen,
    syncOpen,
    setSyncOpen,
    syncTargetNode,
    stateToast,
    setStateToast,
  } = useMesh();

  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);

  // Active page path
  const currentPath = location.pathname.startsWith("/topology")
    ? "/topology"
    : location.pathname.startsWith("/looking-glass")
    ? "/looking-glass"
    : location.pathname.startsWith("/helper")
    ? "/helper"
    : location.pathname.startsWith("/settings")
    ? "/settings"
    : "/nodes";

  const navItems = [
    { label: "Nodes", path: "/nodes", icon: <Server size={18} /> },
    { label: "Topology", path: "/topology", icon: <Network size={18} /> },
    { label: isMobile ? "LG" : "Looking Glass", path: "/looking-glass", icon: <Compass size={18} /> },
    { label: isMobile ? "Helper" : "Device Helper", path: "/helper", icon: <Wrench size={18} /> },
    { label: "Settings", path: "/settings", icon: <Settings size={18} /> },
  ];

  const handleUnlockToggle = async () => {
    if (isUnlocked) {
      await api.lock();
      setIsUnlocked(false);
      setStateToast({ message: "Key vault locked.", severity: "info" });
    } else {
      setUnlockOpen(true);
    }
  };

  const handleLogout = async () => {
    try {
      await api.logout();
      setAuthenticated(false);
      setIsUnlocked(false);
    } catch {
      // ignore
    }
  };

  const formatNetRate = (bytesPerSec?: number) => {
    if (!bytesPerSec) return "0 B/s";
    if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`;
    if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(0)} KB/s`;
    return `${bytesPerSec.toFixed(0)} B/s`;
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100vh", bgcolor: "#F8FAFC", overflow: "hidden" }}>
      {/* Top Application Bar */}
      <AppBar
        position="static"
        elevation={0}
        sx={{
          bgcolor: "#FFFFFF",
          borderBottom: "1px solid #E2E8F0",
          color: "#0F172A",
          zIndex: 1100,
        }}
      >
        <Toolbar sx={{ px: { xs: 1.5, sm: 2.5 }, minHeight: { xs: 56, sm: 62 }, justifyContent: "space-between" }}>
          {/* Brand Logo & Title */}
          <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 1, sm: 2 } }}>
            {isMobile && (
              <IconButton size="small" onClick={() => setMobileDrawerOpen(true)} sx={{ color: "#334155" }}>
                <Menu size={22} />
              </IconButton>
            )}

            <Box
              onClick={() => navigate("/nodes")}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1.2,
                cursor: "pointer",
                userSelect: "none",
              }}
            >
              <Box
                sx={{
                  width: 32,
                  height: 32,
                  borderRadius: 2,
                  bgcolor: "#4F46E5",
                  color: "#FFFFFF",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  boxShadow: "0 4px 10px rgba(79, 70, 229, 0.25)",
                }}
              >
                <Share2 size={18} />
              </Box>
              <Box>
                <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.1 }}>
                  easy42
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.65rem", fontWeight: 600 }}>
                  FLEET & MESH
                </Typography>
              </Box>
            </Box>

            {/* Desktop Navigation Tabs */}
            {!isMobile && (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, ml: 3 }}>
                {navItems.map((item) => {
                  const isActive = currentPath === item.path;
                  return (
                    <Button
                      key={item.path}
                      onClick={() => navigate(item.path)}
                      startIcon={item.icon}
                      sx={{
                        px: 1.8,
                        py: 0.8,
                        borderRadius: 2,
                        fontSize: "0.85rem",
                        fontWeight: isActive ? 700 : 500,
                        color: isActive ? "#4F46E5" : "#64748B",
                        bgcolor: isActive ? "#EEF2FF" : "transparent",
                        "&:hover": {
                          bgcolor: isActive ? "#E0E7FF" : "#F1F5F9",
                          color: isActive ? "#4338CA" : "#0F172A",
                        },
                      }}
                    >
                      {item.label}
                    </Button>
                  );
                })}
              </Box>
            )}
          </Box>

          {/* Right Action Tools */}
          <Box sx={{ display: "flex", alignItems: "center", gap: { xs: 0.8, sm: 1.5 } }}>
            {/* Live Fleet Pill */}
            {fleetLive && (
              <Tooltip
                title={`Traffic: ↓ ${formatNetRate(fleetLive.summary.total_rx_rate)} | ↑ ${formatNetRate(
                  fleetLive.summary.total_tx_rate,
                )}`}
              >
                <Chip
                  icon={<Activity size={13} color="#059669" />}
                  label={`${fleetLive.summary.online_nodes}/${fleetLive.summary.total_nodes} Online`}
                  size="small"
                  onClick={() => navigate("/nodes")}
                  sx={{
                    bgcolor: "#ECFDF5",
                    color: "#059669",
                    fontWeight: 700,
                    fontSize: "0.75rem",
                    cursor: "pointer",
                    height: 26,
                    display: { xs: "none", sm: "inline-flex" },
                  }}
                />
              </Tooltip>
            )}

            {/* Sync Button */}
            <Button
              size="small"
              variant="contained"
              onClick={() => setSyncOpen(true)}
              startIcon={<RefreshCw size={14} className={updatingState ? "spin" : ""} />}
              sx={{
                bgcolor: "#4F46E5",
                fontSize: "0.8rem",
                px: { xs: 1.2, sm: 1.8 },
                height: 32,
              }}
            >
              Sync
            </Button>

            {/* Lock / Unlock Toggle */}
            <Tooltip title={isUnlocked ? "Vault Unlocked (Click to Lock)" : "Vault Locked (Click to Unlock)"}>
              <IconButton
                size="small"
                onClick={handleUnlockToggle}
                sx={{
                  color: isUnlocked ? "#059669" : "#E11D48",
                  bgcolor: isUnlocked ? "#ECFDF5" : "#FEF2F2",
                  p: 0.8,
                  borderRadius: 2,
                }}
              >
                {isUnlocked ? <Unlock size={17} /> : <Lock size={17} />}
              </IconButton>
            </Tooltip>

            {/* Logout button on desktop */}
            {!isMobile && (
              <Tooltip title="Log out">
                <IconButton size="small" onClick={handleLogout} sx={{ color: "#94A3B8", p: 0.8, borderRadius: 2 }}>
                  <LogOut size={17} />
                </IconButton>
              </Tooltip>
            )}
          </Box>
        </Toolbar>
      </AppBar>

      {/* Main Page Outlet */}
      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          position: "relative",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
          pb: isMobile ? "calc(58px + env(safe-area-inset-bottom, 0px))" : 0,
        }}
      >
        <Outlet />
      </Box>

      {/* Mobile Bottom Navigation Bar (< md screens) */}
      {isMobile && (
        <Paper
          elevation={4}
          sx={{
            position: "fixed",
            bottom: 0,
            left: 0,
            right: 0,
            zIndex: 1000,
            borderTop: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            pb: "env(safe-area-inset-bottom, 0px)",
          }}
        >
          <BottomNavigation
            value={currentPath}
            onChange={(_, newPath) => navigate(newPath)}
            showLabels
            sx={{
              height: 58,
              bgcolor: "transparent",
              "& .MuiBottomNavigationAction-root": {
                minWidth: "auto",
                px: 0.5,
                color: "#64748B",
                "&.Mui-selected": {
                  color: "#4F46E5",
                  fontWeight: 700,
                },
              },
            }}
          >
            {navItems.map((item) => (
              <BottomNavigationAction
                key={item.path}
                label={item.label}
                value={item.path}
                icon={item.icon}
                sx={{ fontSize: "0.75rem" }}
              />
            ))}
          </BottomNavigation>
        </Paper>
      )}

      {/* Mobile Side Drawer */}
      <Drawer anchor="left" open={mobileDrawerOpen} onClose={() => setMobileDrawerOpen(false)}>
        <Box sx={{ width: 260, p: 2, display: "flex", flexDirection: "column", height: "100%" }}>
          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Box
                sx={{
                  width: 30,
                  height: 30,
                  borderRadius: 1.5,
                  bgcolor: "#4F46E5",
                  color: "#FFFFFF",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                <Share2 size={16} />
              </Box>
              <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A" }}>
                easy42
              </Typography>
            </Box>
            <IconButton size="small" onClick={() => setMobileDrawerOpen(false)}>
              <X size={20} />
            </IconButton>
          </Box>

          <Divider sx={{ mb: 2 }} />

          <List sx={{ p: 0 }}>
            {navItems.map((item) => {
              const isActive = currentPath === item.path;
              return (
                <ListItem key={item.path} disablePadding sx={{ mb: 0.5 }}>
                  <ListItemButton
                    onClick={() => {
                      navigate(item.path);
                      setMobileDrawerOpen(false);
                    }}
                    sx={{
                      borderRadius: 2,
                      bgcolor: isActive ? "#EEF2FF" : "transparent",
                      color: isActive ? "#4F46E5" : "#334155",
                      fontWeight: isActive ? 700 : 500,
                    }}
                  >
                    <ListItemIcon sx={{ minWidth: 36, color: isActive ? "#4F46E5" : "#64748B" }}>
                      {item.icon}
                    </ListItemIcon>
                    <ListItemText primary={item.label} primaryTypographyProps={{ fontSize: "0.9rem", fontWeight: isActive ? 700 : 500 }} />
                  </ListItemButton>
                </ListItem>
              );
            })}
          </List>

          <Box sx={{ mt: "auto", pt: 2, borderTop: "1px solid #E2E8F0" }}>
            <Button
              fullWidth
              variant="outlined"
              color="error"
              startIcon={<LogOut size={16} />}
              onClick={handleLogout}
              sx={{ borderRadius: 2 }}
            >
              Sign Out
            </Button>
          </Box>
        </Box>
      </Drawer>

      {/* Global Modals */}
      <AddNodeModal
        open={addNodeOpen}
        onClose={() => setAddNodeOpen(false)}
        nodeToEdit={nodeToEdit}
        onNodeAdded={() => loadData()}
        onNodeUpdated={() => loadData()}
      />

      <RenameNodeModal
        open={renameModalOpen}
        onClose={() => setRenameModalOpen(false)}
        node={nodeToRename}
        existingNodes={nodes}
        onNodeRenamed={() => loadData()}
      />

      <AddLinkModal
        open={addLinkOpen}
        onClose={() => setAddLinkOpen(false)}
        linkToEdit={linkToEdit}
        initialFrom={connectFrom}
        initialTo={connectTo}
        nodes={nodes}
        onLinkAdded={() => loadData()}
        onLinkUpdated={() => loadData()}
      />

      <UnlockModal
        open={unlockOpen}
        onClose={() => setUnlockOpen(false)}
        onUnlocked={() => {
          setIsUnlocked(true);
          setUnlockOpen(false);
          loadData();
        }}
      />

      <SyncProgressModal
        open={syncOpen}
        onClose={() => setSyncOpen(false)}
        onSyncComplete={() => {
          loadData();
          refreshFleetLive();
        }}
        targetNode={syncTargetNode}
      />

      {/* Notification Toast */}
      <Snackbar
        open={Boolean(stateToast)}
        autoHideDuration={4000}
        onClose={() => setStateToast(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "center" }}
      >
        <Alert
          onClose={() => setStateToast(null)}
          severity={stateToast?.severity || "info"}
          sx={{ width: "100%", boxShadow: "0 10px 15px -3px rgba(0, 0, 0, 0.1)" }}
        >
          {stateToast?.message}
        </Alert>
      </Snackbar>
    </Box>
  );
};
