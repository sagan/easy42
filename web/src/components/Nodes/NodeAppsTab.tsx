import React, { useState, useEffect } from "react";
import {
  Box,
  Typography,
  Paper,
  Button,
  Chip,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  CircularProgress,
  Alert,
  Grid,
  Select,
  MenuItem,
  FormControl,
} from "@mui/material";
import {
  Layers,
  RefreshCw,
  Play,
  Square,
  RotateCw,
  Terminal,
  Copy,
  Check,
  CheckCircle2,
  XCircle,
  Box as BoxIcon,
  Cpu,
} from "lucide-react";
import { Link as RouterLink } from "react-router-dom";
import { api } from "../../api/client";
import type { App, AppStatusRecord } from "../../types/api";

interface NodeAppsTabProps {
  nodeName: string;
}

export const NodeAppsTab: React.FC<NodeAppsTabProps> = ({ nodeName }) => {
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [apps, setApps] = useState<App[]>([]);
  const [statuses, setStatuses] = useState<AppStatusRecord[]>([]);
  const [appsDisabled, setAppsDisabled] = useState(false);

  // Action states
  const [actionLoading, setActionLoading] = useState<Record<string, string>>({}); // appName -> action
  const [actionError, setActionError] = useState<string | null>(null);

  // Logs modal
  const [logsModalOpen, setLogsModalOpen] = useState(false);
  const [activeLogApp, setActiveLogApp] = useState<string>("");
  const [activeLogType, setActiveLogType] = useState<string>("docker");
  const [logLines, setLogLines] = useState<number>(100);
  const [logContent, setLogContent] = useState<string>("");
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [copied, setCopied] = useState(false);

  const loadNodeApps = async (isManualRefresh = false) => {
    try {
      if (isManualRefresh) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);
      const res = await api.getNodeApps(nodeName);
      setApps(res.apps || []);
      setStatuses(res.statuses || []);
      setAppsDisabled(!!res.disable_apps);
    } catch (e: any) {
      setError(e.message || "Failed to load node workloads");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    loadNodeApps();
    const interval = setInterval(() => {
      loadNodeApps(true);
    }, 15000);
    return () => clearInterval(interval);
  }, [nodeName]);

  const handleAction = async (appName: string, action: string, appType: string) => {
    try {
      setActionLoading((prev) => ({ ...prev, [appName]: action }));
      setActionError(null);
      await api.triggerNodeAppAction(nodeName, appName, action, appType);
      await loadNodeApps(true);
    } catch (e: any) {
      setActionError(`Failed to ${action} app '${appName}': ${e.message}`);
    } finally {
      setActionLoading((prev) => {
        const next = { ...prev };
        delete next[appName];
        return next;
      });
    }
  };

  const openLogs = async (appName: string, appType: string) => {
    setActiveLogApp(appName);
    setActiveLogType(appType);
    setLogsModalOpen(true);
    await fetchLogs(appName, appType, logLines);
  };

  const fetchLogs = async (appName: string, appType: string, lines: number) => {
    try {
      setLoadingLogs(true);
      const res = await api.triggerNodeAppAction(nodeName, appName, "logs", appType, lines);
      const out = res.stdout || res.stderr || "(No logs emitted)";
      setLogContent(out);
    } catch (e: any) {
      setLogContent(`Error fetching logs: ${e.message}`);
    } finally {
      setLoadingLogs(false);
    }
  };

  const copyToClipboard = () => {
    navigator.clipboard.writeText(logContent);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const getStatusForApp = (appName: string): AppStatusRecord | undefined => {
    return statuses.find((s) => s.name === appName);
  };

  if (loading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
        <CircularProgress size={32} sx={{ color: "#4F46E5" }} />
      </Box>
    );
  }

  return (
    <Box sx={{ py: 2 }}>
      {/* Tab Header Bar */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 2.5 }}>
        <Box>
          <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#0F172A" }}>
            Node Workloads & Services
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
            Live status of Easy42 managed apps running on {nodeName}
          </Typography>
        </Box>
        <Box sx={{ display: "flex", gap: 1.5 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={<RefreshCw size={14} className={refreshing ? "animate-spin" : ""} />}
            onClick={() => loadNodeApps(true)}
            disabled={refreshing}
            sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.8rem", color: "#475569", borderColor: "#CBD5E1" }}
          >
            Refresh
          </Button>
          <Button
            size="small"
            variant="contained"
            component={RouterLink}
            to="/apps"
            startIcon={<Layers size={14} />}
            sx={{
              bgcolor: "#4F46E5",
              textTransform: "none",
              fontWeight: 600,
              fontSize: "0.8rem",
              "&:hover": { bgcolor: "#4338CA" },
            }}
          >
            Manage Apps
          </Button>
        </Box>
      </Box>

      {appsDisabled && (
        <Alert severity="warning" sx={{ mb: 2.5, fontWeight: 500 }}>
          App management is disabled on this node (<code>disable_apps: true</code> in <code>/etc/easy42/agent.json</code>). Easy42 will not deploy or sync apps to this node, and app actions cannot be triggered.
        </Alert>
      )}

      {error && (
        <Alert severity="error" sx={{ mb: 2.5 }}>
          {error}
        </Alert>
      )}
      {actionError && (
        <Alert severity="error" sx={{ mb: 2.5 }}>
          {actionError}
        </Alert>
      )}

      {apps.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            py: 6,
            px: 3,
            textAlign: "center",
            borderRadius: "14px",
            border: "1px dashed #CBD5E1",
            bgcolor: "#F8FAFC",
          }}
        >
          <Box
            sx={{
              width: 48,
              height: 48,
              borderRadius: "50%",
              bgcolor: "#EEF2FF",
              color: "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              mx: "auto",
              mb: 1.5,
            }}
          >
            <Layers size={24} />
          </Box>
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
            No Workloads Assigned to this Node
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 440, mx: "auto", mb: 2.5, fontSize: "0.85rem" }}>
            Assign this node to one or more Docker Compose or Native Systemd apps in the top-level Apps page.
          </Typography>
          <Button
            variant="outlined"
            size="small"
            component={RouterLink}
            to="/apps"
            sx={{ textTransform: "none", fontWeight: 600, color: "#4F46E5", borderColor: "#4F46E5" }}
          >
            Go to Apps Catalog
          </Button>
        </Paper>
      ) : (
        <Grid container spacing={2}>
          {apps.map((app) => {
            const st = getStatusForApp(app.name);
            const isRunning = st?.state?.toLowerCase() === "running";
            const isFailed = st?.state?.toLowerCase() === "failed";
            const currentAction = actionLoading[app.name];

            return (
              <Grid item xs={12} key={app.id}>
                <Paper
                  elevation={0}
                  sx={{
                    p: 2.5,
                    borderRadius: "12px",
                    border: "1px solid #E2E8F0",
                    display: "flex",
                    flexDirection: { xs: "column", md: "row" },
                    alignItems: { xs: "stretch", md: "center" },
                    justifyContent: "space-between",
                    gap: 2,
                    bgcolor: "#FFFFFF",
                  }}
                >
                  {/* Left: App Info & Status */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                    <Box
                      sx={{
                        width: 44,
                        height: 44,
                        borderRadius: "10px",
                        bgcolor: app.app_type === "docker" ? "#E0F2FE" : "#F3E8FF",
                        color: app.app_type === "docker" ? "#0284C7" : "#9333EA",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        flexShrink: 0,
                      }}
                    >
                      {app.app_type === "docker" ? <BoxIcon size={22} /> : <Cpu size={22} />}
                    </Box>

                    <Box>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                          {app.name}
                        </Typography>
                        <Chip
                          size="small"
                          label={app.app_type === "docker" ? "Docker" : "Systemd"}
                          sx={{
                            height: 20,
                            fontSize: "0.68rem",
                            fontWeight: 700,
                            bgcolor: app.app_type === "docker" ? "#E0F2FE" : "#F3E8FF",
                            color: app.app_type === "docker" ? "#0369A1" : "#7E22CE",
                          }}
                        />
                      </Box>

                      <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.82rem", mt: 0.3 }}>
                        {app.description || `Installed as ezapp-${app.name}`}
                      </Typography>

                      {st?.status_text && (
                        <Typography variant="caption" sx={{ color: "#94A3B8", display: "block", mt: 0.2 }}>
                          Status: {st.status_text}
                        </Typography>
                      )}
                    </Box>
                  </Box>

                  {/* Middle: Live State Chip */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Chip
                      size="small"
                      icon={
                        isRunning ? (
                          <CheckCircle2 size={13} />
                        ) : isFailed ? (
                          <XCircle size={13} />
                        ) : (
                          <Square size={13} />
                        )
                      }
                      label={st ? st.state.toUpperCase() : "SYNCING"}
                      sx={{
                        fontWeight: 800,
                        fontSize: "0.72rem",
                        height: 24,
                        bgcolor: isRunning ? "#ECFDF5" : isFailed ? "#FEF2F2" : "#F1F5F9",
                        color: isRunning ? "#059669" : isFailed ? "#DC2626" : "#64748B",
                        "& .MuiChip-icon": { color: "inherit" },
                      }}
                    />
                  </Box>

                  {/* Right: Actions */}
                  <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={<Terminal size={14} />}
                      onClick={() => openLogs(app.name, app.app_type)}
                      sx={{ textTransform: "none", fontSize: "0.78rem", fontWeight: 600, color: "#334155" }}
                    >
                      Logs
                    </Button>

                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={
                        currentAction === "restart" ? (
                          <CircularProgress size={14} />
                        ) : (
                          <RotateCw size={14} />
                        )
                      }
                      disabled={appsDisabled || !!currentAction}
                      onClick={() => handleAction(app.name, "restart", app.app_type)}
                      sx={{ textTransform: "none", fontSize: "0.78rem", fontWeight: 600, color: "#4F46E5" }}
                    >
                      Restart
                    </Button>

                    {isRunning ? (
                      <Button
                        size="small"
                        variant="outlined"
                        color="error"
                        startIcon={
                          currentAction === "stop" ? (
                            <CircularProgress size={14} color="inherit" />
                          ) : (
                            <Square size={14} />
                          )
                        }
                        disabled={appsDisabled || !!currentAction}
                        onClick={() => handleAction(app.name, "stop", app.app_type)}
                        sx={{ textTransform: "none", fontSize: "0.78rem", fontWeight: 600 }}
                      >
                        Stop
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        variant="contained"
                        color="success"
                        startIcon={
                          currentAction === "start" ? (
                            <CircularProgress size={14} color="inherit" />
                          ) : (
                            <Play size={14} />
                          )
                        }
                        disabled={appsDisabled || !!currentAction}
                        onClick={() => handleAction(app.name, "start", app.app_type)}
                        sx={{ textTransform: "none", fontSize: "0.78rem", fontWeight: 600 }}
                      >
                        Start
                      </Button>
                    )}
                  </Box>
                </Paper>
              </Grid>
            );
          })}
        </Grid>
      )}

      {/* Logs Inspection Modal */}
      <Dialog
        open={logsModalOpen}
        onClose={() => setLogsModalOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{
          sx: { borderRadius: "16px", bgcolor: "#0F172A", color: "#F8FAFC" },
        }}
      >
        <DialogTitle sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", pb: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Terminal size={20} style={{ color: "#818CF8" }} />
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#F8FAFC" }}>
              Workload Logs: {activeLogApp}
            </Typography>
          </Box>
          <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <FormControl size="small" sx={{ minWidth: 110 }}>
              <Select
                value={logLines}
                onChange={(e) => {
                  const val = Number(e.target.value);
                  setLogLines(val);
                  fetchLogs(activeLogApp, activeLogType, val);
                }}
                sx={{
                  color: "#F8FAFC",
                  bgcolor: "#1E293B",
                  "& .MuiOutlinedInput-notchedOutline": { borderColor: "#334155" },
                  height: 32,
                  fontSize: "0.75rem",
                }}
              >
                <MenuItem value={50}>50 lines</MenuItem>
                <MenuItem value={100}>100 lines</MenuItem>
                <MenuItem value={250}>250 lines</MenuItem>
                <MenuItem value={500}>500 lines</MenuItem>
              </Select>
            </FormControl>
            <IconButton
              size="small"
              onClick={() => fetchLogs(activeLogApp, activeLogType, logLines)}
              disabled={loadingLogs}
              sx={{ color: "#94A3B8" }}
            >
              <RefreshCw size={16} className={loadingLogs ? "animate-spin" : ""} />
            </IconButton>
            <Button
              size="small"
              variant="outlined"
              startIcon={copied ? <Check size={14} /> : <Copy size={14} />}
              onClick={copyToClipboard}
              sx={{
                textTransform: "none",
                fontSize: "0.75rem",
                color: "#E2E8F0",
                borderColor: "#334155",
                height: 32,
              }}
            >
              {copied ? "Copied" : "Copy"}
            </Button>
          </Box>
        </DialogTitle>
        <DialogContent sx={{ p: 2 }}>
          {loadingLogs ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={28} sx={{ color: "#818CF8" }} />
            </Box>
          ) : (
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 2,
                borderRadius: "8px",
                bgcolor: "#020617",
                color: "#E2E8F0",
                fontFamily: "monospace",
                fontSize: "0.82rem",
                lineHeight: 1.5,
                maxHeight: 460,
                overflowY: "auto",
                whiteSpace: "pre-wrap",
                wordBreak: "break-all",
                border: "1px solid #1E293B",
              }}
            >
              {logContent}
            </Box>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setLogsModalOpen(false)} sx={{ color: "#94A3B8", textTransform: "none" }}>
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
