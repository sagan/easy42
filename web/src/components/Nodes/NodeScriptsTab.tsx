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
  Tooltip,
} from "@mui/material";
import {
  Terminal,
  RefreshCw,
  Play,
  Square,
  Copy,
  Check,
  CheckCircle2,
  XCircle,
  Clock,
  Code2,
  ExternalLink,
} from "lucide-react";
import { Link as RouterLink } from "react-router-dom";
import { api } from "../../api/client";
import type { App, AppStatusRecord } from "../../types/api";

interface NodeScriptsTabProps {
  nodeName: string;
}

export const NodeScriptsTab: React.FC<NodeScriptsTabProps> = ({ nodeName }) => {
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
  const [logLines, setLogLines] = useState<number>(100);
  const [logContent, setLogContent] = useState<string>("");
  const [logStartedAt, setLogStartedAt] = useState<number | null>(null);
  const [logEndedAt, setLogEndedAt] = useState<number | null>(null);
  const [loadingLogs, setLoadingLogs] = useState(false);
  const [copied, setCopied] = useState(false);

  // Script preview dialog
  const [previewApp, setPreviewApp] = useState<App | null>(null);

  const loadNodeScripts = async (isManualRefresh = false) => {
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
      setAppsDisabled(Boolean(res.disable_apps));
    } catch (e: any) {
      setError(e.message || "Failed to load node scripts");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    loadNodeScripts();
    const interval = setInterval(() => {
      loadNodeScripts(true);
    }, 10000);
    return () => clearInterval(interval);
  }, [nodeName]);

  const handleAction = async (appName: string, action: string) => {
    try {
      setActionLoading((prev) => ({ ...prev, [appName]: action }));
      setActionError(null);
      const res = await api.triggerNodeAppAction(nodeName, appName, action, "script");
      if (!res.success) {
        setActionError(
          res.stderr?.trim() ||
            res.stdout?.trim() ||
            `Failed to ${action} script '${appName}' (exit code ${res.exit_code})`,
        );
      }
      await loadNodeScripts(true);
    } catch (e: any) {
      setActionError(`Failed to ${action} script '${appName}': ${e.message}`);
    } finally {
      setActionLoading((prev) => {
        const next = { ...prev };
        delete next[appName];
        return next;
      });
    }
  };

  const openLogs = async (appName: string) => {
    setActiveLogApp(appName);
    setLogsModalOpen(true);
    await fetchLogs(appName, logLines);
  };

  const fetchLogs = async (appName: string, lines: number) => {
    try {
      setLoadingLogs(true);
      const res = await api.triggerNodeAppAction(nodeName, appName, "logs", "script", lines);
      const out = res.stdout || res.stderr || "(No logs emitted)";
      setLogContent(out);
      setLogStartedAt(res.started_at || null);
      setLogEndedAt(res.ended_at || null);
    } catch (e: any) {
      setLogContent(`Error fetching logs: ${e.message}`);
      setLogStartedAt(null);
      setLogEndedAt(null);
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
    return statuses.find((s) => s.name === appName && (s.app_type === "script" || !s.app_type));
  };

  const scriptApps = apps.filter((a) => a.app_type === "script");

  if (loading && !refreshing) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
        <CircularProgress size={36} sx={{ color: "#059669" }} />
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
      {/* Header bar */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", flexWrap: "wrap", gap: 1 }}>
        <Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", fontSize: "1.1rem" }}>
              Scripts on {nodeName}
            </Typography>
            <Chip
              size="small"
              label={`${scriptApps.length} Deployed`}
              sx={{ fontWeight: 700, bgcolor: "#ECFDF5", color: "#059669", fontSize: "0.75rem" }}
            />
          </Box>
          <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
            Executable on-demand scripts deployed to{" "}
            <code style={{ backgroundColor: "#F1F5F9", padding: "2px 6px", borderRadius: "4px" }}>
              /etc/ezscript/&lt;name&gt;
            </code>
          </Typography>
        </Box>

        <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
          <Button
            size="small"
            variant="outlined"
            onClick={() => loadNodeScripts(true)}
            disabled={refreshing}
            startIcon={refreshing ? <CircularProgress size={14} color="inherit" /> : <RefreshCw size={14} />}
            sx={{ textTransform: "none", fontWeight: 600, color: "#64748B", borderColor: "#CBD5E1" }}
          >
            Refresh
          </Button>

          <Button
            size="small"
            variant="contained"
            component={RouterLink}
            to="/apps/scripts"
            startIcon={<Terminal size={14} />}
            sx={{
              textTransform: "none",
              fontWeight: 700,
              bgcolor: "#059669",
              "&:hover": { bgcolor: "#047857" },
            }}
          >
            All Scripts Catalog
          </Button>
        </Box>
      </Box>

      {/* Error banner */}
      {error && (
        <Alert severity="error" onClose={() => setError(null)} sx={{ borderRadius: "10px" }}>
          {error}
        </Alert>
      )}

      {/* Disabled apps warning */}
      {appsDisabled && (
        <Alert severity="warning" sx={{ borderRadius: "10px" }}>
          App & script management is disabled on this node (<code>disable_apps: true</code> in{" "}
          <code>/etc/easy42/agent.json</code>).
        </Alert>
      )}

      {/* Action error banner */}
      {actionError && (
        <Alert severity="error" onClose={() => setActionError(null)} sx={{ borderRadius: "10px" }}>
          {actionError}
        </Alert>
      )}

      {/* Main content list */}
      {scriptApps.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            p: 6,
            borderRadius: "14px",
            border: "1px dashed #CBD5E1",
            textAlign: "center",
            bgcolor: "#F8FAFC",
          }}
        >
          <Box
            sx={{
              width: 52,
              height: 52,
              borderRadius: "12px",
              bgcolor: "#ECFDF5",
              color: "#059669",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              mx: "auto",
              mb: 1,
            }}
          >
            <Terminal size={26} />
          </Box>
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
            No Scripts Assigned to this Node
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 440, mx: "auto", mb: 1, fontSize: "0.85rem" }}>
            Scripts target nodes via node name or selector tags. Create or configure a script on the Apps page.
          </Typography>
          <Button
            variant="outlined"
            size="small"
            component={RouterLink}
            to="/apps/scripts"
            sx={{ textTransform: "none", fontWeight: 600, color: "#059669", borderColor: "#059669" }}
          >
            Go to Scripts Catalog
          </Button>
        </Paper>
      ) : (
        <Grid container spacing={2}>
          {scriptApps.map((app) => {
            const st = getStatusForApp(app.name);
            const isRunning = st?.state?.toLowerCase() === "running";
            const isFailed = st?.state?.toLowerCase() === "failed";
            const currentAction = actionLoading[app.name];

            return (
              <Grid item xs={12} key={app.id}>
                <Paper
                  elevation={0}
                  sx={{
                    p: 1,
                    borderRadius: "12px",
                    border: isRunning ? "1px solid #A7F3D0" : "1px solid #E2E8F0",
                    display: { xs: "flex", md: "grid" },
                    flexDirection: { xs: "column" },
                    gridTemplateColumns: { md: "minmax(0, 1fr) 200px auto" },
                    alignItems: "center",
                    gap: 1,
                    bgcolor: isRunning ? "#F0FDF4" : "#FFFFFF",
                    transition: "all 0.15s ease",
                  }}
                >
                  {/* Left: Info */}
                  <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1 }}>
                    <Box
                      sx={{
                        width: 44,
                        height: 44,
                        borderRadius: "10px",
                        bgcolor: isRunning ? "#D1FAE5" : "#ECFDF5",
                        color: "#059669",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "center",
                        flexShrink: 0,
                      }}
                    >
                      <Terminal size={22} />
                    </Box>
                    <Box sx={{ minWidth: 0 }}>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 0.5 }}>
                        <Typography
                          variant="subtitle1"
                          sx={{
                            fontWeight: 800,
                            color: "#0F172A",
                            fontSize: "0.95rem",
                            cursor: "pointer",
                            "&:hover": { color: "#059669" },
                          }}
                          onClick={() => setPreviewApp(app)}
                        >
                          {app.name}
                        </Typography>
                        <Chip
                          size="small"
                          label={`/etc/ezscript/${app.name}`}
                          sx={{
                            height: 20,
                            fontSize: "0.72rem",
                            fontFamily: "monospace",
                            bgcolor: "#F1F5F9",
                            color: "#475569",
                          }}
                        />
                        {app.env_content && app.env_content.trim() !== "" && (
                          <Chip
                            size="small"
                            label="Rendered .env"
                            sx={{
                              height: 20,
                              fontSize: "0.7rem",
                              bgcolor: "#EFF6FF",
                              color: "#2563EB",
                              fontWeight: 600,
                            }}
                          />
                        )}
                        {app.group && (
                          <Chip
                            size="small"
                            label={app.group}
                            sx={{ height: 20, fontSize: "0.7rem", bgcolor: "#F8FAFC", color: "#64748B" }}
                          />
                        )}
                      </Box>
                      {app.description && (
                        <Typography
                          variant="body2"
                          sx={{
                            color: "#64748B",
                            fontSize: "0.82rem",
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {app.description}
                        </Typography>
                      )}
                    </Box>
                  </Box>

                  {/* Middle: Live State on Node */}
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      {isRunning ? (
                        <Chip
                          size="small"
                          icon={<CheckCircle2 size={13} style={{ color: "#059669" }} />}
                          label={st?.status_text || "Running"}
                          sx={{
                            fontWeight: 700,
                            bgcolor: "#D1FAE5",
                            color: "#065F46",
                            fontSize: "0.75rem",
                          }}
                        />
                      ) : isFailed ? (
                        <Chip
                          size="small"
                          icon={<XCircle size={13} style={{ color: "#DC2626" }} />}
                          label={st?.status_text || "Failed"}
                          sx={{
                            fontWeight: 700,
                            bgcolor: "#FEE2E2",
                            color: "#991B1B",
                            fontSize: "0.75rem",
                          }}
                        />
                      ) : (
                        <Chip
                          size="small"
                          icon={<Clock size={13} style={{ color: "#64748B" }} />}
                          label={st?.status_text || "Ready"}
                          sx={{
                            fontWeight: 600,
                            bgcolor: "#F1F5F9",
                            color: "#475569",
                            fontSize: "0.75rem",
                          }}
                        />
                      )}
                    </Box>
                    {st?.last_updated ? (
                      <Typography
                        variant="caption"
                        sx={{ color: "#94A3B8", fontSize: "0.7rem", display: "block", mt: 0.5 }}
                      >
                        Reported {new Date(st.last_updated * 1000).toLocaleTimeString()}
                      </Typography>
                    ) : null}
                  </Box>

                  {/* Right: Actions */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, justifyContent: "flex-end" }}>
                    {/* Start Button */}
                    <Button
                      size="small"
                      variant="contained"
                      onClick={() => handleAction(app.name, "start")}
                      disabled={Boolean(currentAction) || isRunning || appsDisabled}
                      startIcon={
                        currentAction === "start" ? (
                          <CircularProgress size={13} color="inherit" />
                        ) : (
                          <Play size={13} fill="currentColor" />
                        )
                      }
                      sx={{
                        textTransform: "none",
                        fontWeight: 700,
                        fontSize: "0.8rem",
                        bgcolor: "#059669",
                        "&:hover": { bgcolor: "#047857" },
                      }}
                    >
                      Start
                    </Button>

                    {/* Stop Button */}
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => handleAction(app.name, "stop")}
                      disabled={Boolean(currentAction) || !isRunning || appsDisabled}
                      startIcon={
                        currentAction === "stop" ? (
                          <CircularProgress size={13} color="inherit" />
                        ) : (
                          <Square size={13} fill="currentColor" />
                        )
                      }
                      sx={{
                        textTransform: "none",
                        fontWeight: 600,
                        fontSize: "0.8rem",
                        color: "#DC2626",
                        borderColor: "#FECACA",
                        "&:hover": { bgcolor: "#FEF2F2", borderColor: "#FCA5A5" },
                      }}
                    >
                      Stop
                    </Button>

                    {/* Logs Button */}
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => openLogs(app.name)}
                      startIcon={<Terminal size={13} />}
                      sx={{
                        textTransform: "none",
                        fontWeight: 600,
                        fontSize: "0.8rem",
                        color: "#475569",
                        borderColor: "#CBD5E1",
                        "&:hover": { bgcolor: "#F8FAFC", borderColor: "#94A3B8" },
                      }}
                    >
                      Logs
                    </Button>

                    {/* View Code button */}
                    <Tooltip title="View Script Snippet">
                      <IconButton
                        size="small"
                        onClick={() => setPreviewApp(app)}
                        sx={{ color: "#64748B", "&:hover": { color: "#0F172A", bgcolor: "#F1F5F9" } }}
                      >
                        <Code2 size={16} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Paper>
              </Grid>
            );
          })}
        </Grid>
      )}

      {/* Script Execution Logs Modal */}
      <Dialog
        open={logsModalOpen}
        onClose={() => setLogsModalOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { borderRadius: "14px", bgcolor: "#0F172A", color: "#F8FAFC" } }}
      >
        <DialogTitle
          sx={{
            pb: 1,
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            flexWrap: "wrap",
            gap: 1,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Terminal size={20} color="#10B981" />
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#F8FAFC", fontSize: "1.05rem" }}>
              Script Logs: {activeLogApp}
            </Typography>
            <Chip
              size="small"
              label={`Node: ${nodeName}`}
              sx={{ bgcolor: "#1E293B", color: "#94A3B8", fontWeight: 600, fontSize: "0.72rem" }}
            />
            {logStartedAt ? (
              <Chip
                size="small"
                icon={<Clock size={12} color="#94A3B8" />}
                label={`Started: ${new Date(logStartedAt * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`}
                sx={{ bgcolor: "#1E293B", color: "#E2E8F0", fontSize: "0.72rem" }}
              />
            ) : null}
            {logEndedAt ? (
              <Chip
                size="small"
                label={`Ended: ${new Date(logEndedAt * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`}
                sx={{ bgcolor: "#1E293B", color: "#E2E8F0", fontSize: "0.72rem" }}
              />
            ) : null}
            {logStartedAt && logEndedAt && logEndedAt >= logStartedAt ? (
              <Chip
                size="small"
                label={`Duration: ${logEndedAt - logStartedAt}s`}
                sx={{ bgcolor: "#1E293B", color: "#94A3B8", fontSize: "0.72rem" }}
              />
            ) : null}
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <FormControl size="small" sx={{ minWidth: 100 }}>
              <Select
                value={logLines}
                onChange={(e) => {
                  const val = Number(e.target.value);
                  setLogLines(val);
                  fetchLogs(activeLogApp, val);
                }}
                sx={{
                  color: "#94A3B8",
                  fontSize: "0.75rem",
                  height: 30,
                  "& .MuiOutlinedInput-notchedOutline": { borderColor: "#334155" },
                  "&:hover .MuiOutlinedInput-notchedOutline": { borderColor: "#475569" },
                }}
              >
                <MenuItem value={50}>Last 50</MenuItem>
                <MenuItem value={100}>Last 100</MenuItem>
                <MenuItem value={250}>Last 250</MenuItem>
                <MenuItem value={500}>Last 500</MenuItem>
              </Select>
            </FormControl>
            <Button
              size="small"
              variant="outlined"
              onClick={() => fetchLogs(activeLogApp, logLines)}
              disabled={loadingLogs}
              startIcon={loadingLogs ? <CircularProgress size={12} color="inherit" /> : <RefreshCw size={12} />}
              sx={{
                color: "#94A3B8",
                borderColor: "#334155",
                fontSize: "0.75rem",
                textTransform: "none",
                "&:hover": { borderColor: "#475569", bgcolor: "#1E293B" },
              }}
            >
              Refresh
            </Button>
            <Button
              size="small"
              variant="outlined"
              onClick={copyToClipboard}
              startIcon={copied ? <Check size={12} color="#10B981" /> : <Copy size={12} />}
              sx={{
                color: copied ? "#10B981" : "#94A3B8",
                borderColor: copied ? "#10B981" : "#334155",
                fontSize: "0.75rem",
                textTransform: "none",
                "&:hover": { borderColor: "#475569", bgcolor: "#1E293B" },
              }}
            >
              {copied ? "Copied" : "Copy"}
            </Button>
          </Box>
        </DialogTitle>

        <DialogContent sx={{ p: 1, pt: 1 }}>
          {loadingLogs ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={28} sx={{ color: "#10B981" }} />
            </Box>
          ) : (
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 1,
                borderRadius: "8px",
                bgcolor: "#020617",
                color: "#E2E8F0",
                fontSize: "0.82rem",
                fontFamily: "monospace",
                lineHeight: 1.5,
                overflowX: "auto",
                maxHeight: "60vh",
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
          <Button
            size="small"
            onClick={() => setLogsModalOpen(false)}
            sx={{ textTransform: "none", color: "#94A3B8", fontWeight: 600 }}
          >
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* Script Snippet Preview Modal */}
      {previewApp && (
        <Dialog
          open={Boolean(previewApp)}
          onClose={() => setPreviewApp(null)}
          maxWidth="md"
          fullWidth
          PaperProps={{ sx: { borderRadius: "14px" } }}
        >
          <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1, pb: 1 }}>
            <Code2 size={20} color="#059669" />
            <Box>
              <Typography variant="h6" sx={{ fontWeight: 800, fontSize: "1.05rem" }}>
                {previewApp.name}
              </Typography>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Deployed to /etc/ezscript/{previewApp.name} on {nodeName}
              </Typography>
            </Box>
          </DialogTitle>
          <DialogContent>
            {previewApp.description && (
              <Typography variant="body2" sx={{ color: "#475569", mb: 1 }}>
                {previewApp.description}
              </Typography>
            )}
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B", textTransform: "uppercase" }}>
              Script Main Content:
            </Typography>
            <Box
              component="pre"
              sx={{
                mt: 1,
                mb: 1,
                p: 1,
                borderRadius: "8px",
                bgcolor: "#0F172A",
                color: "#F8FAFC",
                fontSize: "0.82rem",
                fontFamily: "monospace",
                maxHeight: 300,
                overflow: "auto",
                whiteSpace: "pre-wrap",
              }}
            >
              {previewApp.main_content}
            </Box>

            {previewApp.env_content && previewApp.env_content.trim() !== "" && (
              <>
                <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B", textTransform: "uppercase" }}>
                  Environment Variables (Injected at Deploy):
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    mt: 1,
                    p: 1,
                    borderRadius: "8px",
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    color: "#0F172A",
                    fontSize: "0.82rem",
                    fontFamily: "monospace",
                    maxHeight: 180,
                    overflow: "auto",
                    whiteSpace: "pre-wrap",
                  }}
                >
                  {previewApp.env_content}
                </Box>
              </>
            )}
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2 }}>
            <Button
              size="small"
              variant="outlined"
              component={RouterLink}
              to="/apps/scripts"
              endIcon={<ExternalLink size={14} />}
              sx={{ textTransform: "none", fontWeight: 600, color: "#059669", borderColor: "#059669" }}
            >
              Edit in Scripts Catalog
            </Button>
            <Button
              size="small"
              onClick={() => setPreviewApp(null)}
              sx={{ textTransform: "none", color: "#64748B", fontWeight: 600 }}
            >
              Close
            </Button>
          </DialogActions>
        </Dialog>
      )}
    </Box>
  );
};
