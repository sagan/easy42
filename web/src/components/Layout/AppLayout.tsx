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
  Activity,
  ExternalLink,
  FileCode2,
} from "lucide-react";
import { Easy42Logo } from "../Logo/Easy42Logo";
import { useMesh } from "../../context/MeshContext";
import { api } from "../../api/client";
import { AddNodeModal } from "../Modals/AddNodeModal";
import { AddLinkModal } from "../Modals/AddLinkModal";
import { RenameNodeModal } from "../Modals/RenameNodeModal";
import { UnlockModal } from "../Modals/UnlockModal";
import { SyncProgressModal } from "../Modals/SyncProgressModal";
import { ApplyConfigModal } from "../Modals/ApplyConfigModal";

export const AppLayout: React.FC = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const location = useLocation();
  const navigate = useNavigate();

  const {
    nodes,
    links,
    fleetLive,
    isUnlocked,
    setIsUnlocked,
    setAuthenticated,
    loadData,
    refreshFleetLive,
    flapAlertedUrl,
    flapAlertedMetrics,
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
    applyOpen,
    setApplyOpen,
    applyInitialContent,
    openApplyModal,
    stateToast,
    setStateToast,
    selectedNode,
    setSelectedNode,
    setSelectedLink,
  } = useMesh();

  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);

  // Active page path
  const currentPath = location.pathname === "/" ? "/nodes" : "/" + location.pathname.slice(1).split("/").slice(0, 1);

  const navItems = [
    { label: "Nodes", title: "Nodes", path: "/nodes", icon: <Server size={18} /> },
    { label: "Topology", title: "Topology", path: "/topology", icon: <Network size={18} /> },
    { label: "LG", title: "Looking Glass", path: "/looking-glass", icon: <Compass size={18} /> },
    { label: "Helper", title: "Device Helper", path: "/helper", icon: <Wrench size={18} /> },
    { label: "Settings", title: "Settings", path: "/settings", icon: <Settings size={18} /> },
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
          <Box sx={{ display: "flex", alignItems: "center" }}>
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
              <Easy42Logo
                size={32}
                sx={{
                  borderRadius: "8px",
                  boxShadow: "0 4px 10px rgba(79, 70, 229, 0.25)",
                }}
              />
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
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, ml: 2 }}>
                {navItems.map((item) => {
                  const isActive = currentPath === item.path || currentPath.startsWith(item.path + "/");
                  return (
                    <Button
                      key={item.path}
                      onClick={() => navigate(item.path)}
                      startIcon={item.icon}
                      title={item.title}
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

                {flapAlertedUrl && (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                    <Tooltip
                      arrow
                      componentsProps={{
                        tooltip: {
                          sx: {
                            bgcolor: "#0F172A",
                            color: "#F8FAFC",
                            boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.3), 0 8px 10px -6px rgba(0, 0, 0, 0.3)",
                            p: 1.5,
                            borderRadius: 2,
                            border: "1px solid rgba(255, 255, 255, 0.1)",
                            maxWidth: 280,
                          },
                        },
                        arrow: {
                          sx: { color: "#0F172A" },
                        },
                      }}
                      title={
                        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              gap: 1,
                              pb: 0.8,
                              borderBottom: "1px solid rgba(255, 255, 255, 0.1)",
                            }}
                          >
                            <Activity size={15} color="#38BDF8" />
                            <Typography
                              variant="subtitle2"
                              sx={{ fontWeight: 700, fontSize: "0.82rem", color: "#F8FAFC" }}
                            >
                              FlapAlerted Metrics
                            </Typography>
                          </Box>
                          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, fontSize: "0.78rem" }}>
                            <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2 }}>
                              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                                Avg Route Changes (90m):
                              </Typography>
                              <Typography variant="caption" sx={{ fontWeight: 700, color: "#38BDF8" }}>
                                {flapAlertedMetrics?.AverageRouteChanges90}
                              </Typography>
                            </Box>
                            <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2 }}>
                              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                                Active Flaps:
                              </Typography>
                              <Typography
                                variant="caption"
                                sx={{
                                  fontWeight: 700,
                                  color: (flapAlertedMetrics?.ActiveFlapCount ?? 0) > 0 ? "#F87171" : "#4ADE80",
                                }}
                              >
                                {flapAlertedMetrics?.ActiveFlapCount ?? 0}
                              </Typography>
                            </Box>
                            <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2 }}>
                              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                                Active Flap Path Changes:
                              </Typography>
                              <Typography variant="caption" sx={{ fontWeight: 600, color: "#F1F5F9" }}>
                                {flapAlertedMetrics?.ActiveFlapTotalPathChangeCount ?? 0}
                              </Typography>
                            </Box>
                            <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2 }}>
                              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                                Total Path Changes:
                              </Typography>
                              <Typography variant="caption" sx={{ fontWeight: 600, color: "#F1F5F9" }}>
                                {flapAlertedMetrics?.TotalPathChangeCount ?? 0}
                              </Typography>
                            </Box>
                            <Box sx={{ display: "flex", justifyContent: "space-between", gap: 2 }}>
                              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                                Sessions:
                              </Typography>
                              <Typography variant="caption" sx={{ fontWeight: 600, color: "#F1F5F9" }}>
                                {flapAlertedMetrics?.Sessions ?? 0}
                              </Typography>
                            </Box>
                          </Box>
                        </Box>
                      }
                    >
                      <Box
                        sx={{
                          display: "inline-flex",
                          alignItems: "center",
                          gap: 0.5,
                          px: 1.2,
                          py: 0.7,
                          borderRadius: 2,
                          bgcolor: "rgba(8, 145, 178, 0.08)",
                          border: "1px solid rgba(8, 145, 178, 0.25)",
                          textDecoration: "none",
                          cursor: "pointer",
                          transition: "all 0.15s ease",
                          "&:hover": {
                            bgcolor: "rgba(8, 145, 178, 0.16)",
                            borderColor: "#0891B2",
                          },
                        }}
                      >
                        <Box
                          sx={{
                            width: 6,
                            height: 6,
                            borderRadius: "50%",
                            bgcolor: (flapAlertedMetrics?.ActiveFlapCount ?? 0) > 0 ? "#EF4444" : "#10B981",
                            boxShadow:
                              (flapAlertedMetrics?.ActiveFlapCount ?? 0) > 0
                                ? "0 0 6px rgba(239, 68, 68, 0.6)"
                                : "0 0 6px rgba(16, 185, 129, 0.6)",
                          }}
                        />
                        <Typography
                          component="span"
                          sx={{
                            fontSize: "0.85rem",
                            fontWeight: 700,
                            fontVariantNumeric: "tabular-nums",
                            color: "#0891B2",
                            lineHeight: 1,
                          }}
                        >
                          <a
                            target="_blank"
                            rel="noopener noreferrer"
                            href={flapAlertedUrl}
                            style={{ textDecoration: "none" }}
                          >
                            FA {(parseFloat(flapAlertedMetrics?.AverageRouteChanges90 as string) || 0).toFixed(1)}
                          </a>
                        </Typography>
                      </Box>
                    </Tooltip>
                  </Box>
                )}
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

            {/* Apply Config Button */}
            <Tooltip title="Apply Config Changes (Declarative patch)">
              <Button
                size="small"
                variant="outlined"
                onClick={() => openApplyModal()}
                startIcon={<FileCode2 size={14} />}
                sx={{
                  borderColor: "#CBD5E1",
                  color: "#334155",
                  fontWeight: 600,
                  fontSize: "0.8rem",
                  px: { xs: 1.2, sm: 1.8 },
                  height: 32,
                  textTransform: "none",
                  "&:hover": {
                    borderColor: "#4F46E5",
                    bgcolor: "rgba(79, 70, 229, 0.05)",
                    color: "#4F46E5",
                  },
                }}
              >
                Apply
              </Button>
            </Tooltip>

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
          overflowY: "auto",
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
              <Easy42Logo size={30} sx={{ borderRadius: "7px" }} />
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
              const isActive = currentPath === item.path || currentPath.startsWith(item.path + "/");
              return (
                <ListItem key={item.path} disablePadding sx={{ mb: 0.5 }} title={item.title}>
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
                    <ListItemText
                      primary={item.label}
                      primaryTypographyProps={{ fontSize: "0.9rem", fontWeight: isActive ? 700 : 500 }}
                    />
                  </ListItemButton>
                </ListItem>
              );
            })}

            {flapAlertedUrl && (
              <ListItem disablePadding sx={{ mb: 0.5 }}>
                <ListItemButton
                  component="a"
                  href={flapAlertedUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={() => setMobileDrawerOpen(false)}
                  sx={{
                    borderRadius: 2,
                    bgcolor: "rgba(8, 145, 178, 0.08)",
                    color: "#0891B2",
                    fontWeight: 700,
                    "&:hover": { bgcolor: "rgba(8, 145, 178, 0.16)" },
                  }}
                >
                  <ListItemIcon sx={{ minWidth: 36, color: "#0891B2" }}>
                    <ExternalLink size={18} />
                  </ListItemIcon>
                  <ListItemText
                    primary="FA Dashboard"
                    secondary={
                      flapAlertedMetrics?.AverageRouteChanges90 !== undefined
                        ? `Avg Route Changes: ${flapAlertedMetrics.AverageRouteChanges90}`
                        : undefined
                    }
                    primaryTypographyProps={{ fontSize: "0.9rem", fontWeight: 700 }}
                    secondaryTypographyProps={{ fontSize: "0.75rem", color: "#0891B2" }}
                  />
                  {flapAlertedMetrics?.AverageRouteChanges90 !== undefined && (
                    <Chip
                      size="small"
                      label={flapAlertedMetrics.AverageRouteChanges90}
                      sx={{
                        height: 20,
                        fontSize: "0.72rem",
                        fontWeight: 700,
                        bgcolor: "rgba(8, 145, 178, 0.15)",
                        color: "#0891B2",
                      }}
                    />
                  )}
                </ListItemButton>
              </ListItem>
            )}
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
        onNodeUpdated={(updated) => {
          setSelectedNode(updated);
          loadData();
        }}
      />

      <RenameNodeModal
        open={renameModalOpen}
        onClose={() => setRenameModalOpen(false)}
        node={nodeToRename}
        existingNodes={nodes}
        onNodeRenamed={(oldName, updated) => {
          if (selectedNode?.name === oldName) {
            setSelectedNode(updated);
          }
          loadData();
        }}
      />

      <AddLinkModal
        open={addLinkOpen}
        onClose={() => setAddLinkOpen(false)}
        linkToEdit={linkToEdit}
        initialFrom={connectFrom}
        initialTo={connectTo}
        nodes={nodes}
        onLinkAdded={() => loadData()}
        onLinkUpdated={(updated) => {
          setSelectedLink(updated);
          loadData();
        }}
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
        onNeedUnlock={() => setUnlockOpen(true)}
      />

      <ApplyConfigModal
        open={applyOpen}
        onClose={() => setApplyOpen(false)}
        initialContent={applyInitialContent}
        nodes={nodes}
        links={links}
        onApplied={(result, shouldSync) => {
          loadData();
          refreshFleetLive();

          if (shouldSync) {
            setStateToast({
              message: `Config applied (${result.summary?.total_changes || 0} changes). Syncing network in background...`,
              severity: "info",
            });

            const targetNodes =
              result.affected_nodes && result.affected_nodes.length > 0 ? result.affected_nodes.join(",") : undefined;

            api
              .executeSync(false, targetNodes)
              .then((syncRes) => {
                loadData();
                refreshFleetLive();
                const failed = (syncRes || []).filter((r) => !r.success);
                if (failed.length > 0) {
                  const errorMsgs = failed.map((f) => `${f.node_name}: ${f.error || "failed"}`).join("; ");
                  setStateToast({
                    message: `Sync finished with ${failed.length} failure(s): ${errorMsgs}`,
                    severity: "warning",
                  });
                } else if ((syncRes || []).length > 0) {
                  const nodeNames = Array.from(new Set(syncRes.map((r) => r.node_name))).join(", ");
                  setStateToast({
                    message: `Network synced successfully (${syncRes.length} action(s) on ${nodeNames}).`,
                    severity: "success",
                  });
                } else {
                  setStateToast({
                    message: "Config applied. Network is already up to date.",
                    severity: "success",
                  });
                }
              })
              .catch((err: any) => {
                if (err?.status === 423 || err?.message?.includes("locked")) {
                  setStateToast({
                    message: "Key vault is locked. Please unlock vault to sync nodes.",
                    severity: "warning",
                  });
                  setUnlockOpen(true);
                } else {
                  setStateToast({
                    message: `Background sync failed: ${err.message || "Unknown error"}`,
                    severity: "error",
                  });
                }
              });
          } else {
            setStateToast({
              message: `Config change applied successfully (${result.summary?.total_changes || 0} changes)`,
              severity: "success",
            });
          }
        }}
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
