import React, { useState, useEffect } from "react";
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
  Tooltip,
} from "@mui/material";
import {
  RefreshCw,
  CheckCircle2,
  XCircle,
  ChevronDown,
  FileCode,
  Check,
  Trash2,
  Zap,
  Radio,
  CheckCheck,
  Network,
} from "lucide-react";
import { api } from "../../api/client";
import { SyncAction, SyncResult } from "../../types/api";

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
  const [updatingState, setUpdatingState] = useState(false);
  const [results, setResults] = useState<SyncResult[] | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [stateMessage, setStateMessage] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setResults(null);
      setSyncError(null);
      setStateMessage(null);
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
      setPreviewError(e.message || "Failed to generate sync preview");
    } finally {
      setLoadingPreview(false);
    }
  };

  const handleUpdateLiveState = async () => {
    setUpdatingState(true);
    setStateMessage(null);
    try {
      const res = await api.updateState();
      if (res.warnings && res.warnings.length > 0) {
        setStateMessage(`State updated with warnings: ${res.warnings.join(", ")}`);
      } else {
        setStateMessage("Device states successfully fetched and reconciled.");
      }
      await loadPreview();
    } catch (err: unknown) {
      const e = err as Error;
      setSyncError(e.message || "Failed to update device state");
    } finally {
      setUpdatingState(false);
    }
  };

  const handleExecuteSync = async (force: boolean = false) => {
    setSyncing(true);
    setSyncError(null);

    try {
      const res = await api.executeSync(force);
      setResults(res || []);
      onSyncComplete();
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423 && onNeedUnlock) {
        onClose();
        onNeedUnlock();
        return;
      }
      setSyncError(e.message || "Sync execution failed");
    } finally {
      setSyncing(false);
    }
  };

  const safeActions = actions || [];
  const safeResults = results;

  const pendingActions = safeActions.filter((a) => a.needs_apply !== false);
  const syncedActions = safeActions.filter((a) => a.needs_apply === false);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          pb: 1,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              width: 34,
              height: 34,
              borderRadius: 2,
              backgroundColor: "rgba(79, 70, 229, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#4F46E5",
            }}
          >
            <RefreshCw size={18} />
          </Box>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
            Synchronize Mesh Topology
          </Typography>
        </Box>

        {!safeResults && !loadingPreview && (
          <Tooltip title="Connect to all devices via SSH to probe live state and update state.json">
            <Button
              size="small"
              variant="outlined"
              onClick={handleUpdateLiveState}
              disabled={updatingState || syncing}
              startIcon={updatingState ? <CircularProgress size={14} color="inherit" /> : <Radio size={14} />}
              sx={{
                borderColor: "#E2E8F0",
                color: "#475569",
                fontSize: "0.75rem",
                textTransform: "none",
                fontWeight: 600,
                "&:hover": {
                  borderColor: "#0284C7",
                  backgroundColor: "rgba(2, 132, 199, 0.05)",
                  color: "#0284C7",
                },
              }}
            >
              {updatingState ? "Probing Devices..." : "Update State from Devices"}
            </Button>
          </Tooltip>
        )}
      </DialogTitle>

      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2.5 }}>
        {stateMessage && (
          <Alert severity="info" onClose={() => setStateMessage(null)} sx={{ borderRadius: 2 }}>
            {stateMessage}
          </Alert>
        )}

        {loadingPreview ? (
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", py: 6, gap: 2 }}>
            <CircularProgress size={24} color="primary" />
            <Typography variant="body2" sx={{ color: "#64748B" }}>
              Computing WireGuard peer configurations, state diffs, and deleted interfaces...
            </Typography>
          </Box>
        ) : previewError ? (
          <Alert severity="error" sx={{ borderRadius: 2 }}>
            {previewError}
          </Alert>
        ) : safeResults ? (
          /* Execution Results */
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
            <Alert severity={safeResults.every((r) => r.success) ? "success" : "warning"} sx={{ borderRadius: 2 }}>
              {safeResults.every((r) => r.success)
                ? "All node configurations synchronized successfully!"
                : "Some actions encountered errors. Review details below."}
            </Alert>

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1, maxHeight: 360, overflowY: "auto" }}>
              {safeResults.map((res, i) => (
                <Box
                  key={i}
                  sx={{
                    p: 1.5,
                    borderRadius: 2,
                    backgroundColor: "#FFFFFF",
                    border: "1px solid",
                    borderColor: res.success ? "#A7F3D0" : "#FECDD3",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                    {res.success ? <CheckCircle2 size={18} color="#059669" /> : <XCircle size={18} color="#E11D48" />}
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        {res.node_name} — {res.action}
                      </Typography>
                      {res.output && (
                        <Typography
                          variant="caption"
                          className="mono-font"
                          sx={{ color: "#059669", display: "block", mt: 0.2 }}
                        >
                          {res.output}
                        </Typography>
                      )}
                      {res.error && (
                        <Typography variant="caption" sx={{ color: "#E11D48", display: "block" }}>
                          {res.error}
                        </Typography>
                      )}
                    </Box>
                  </Box>
                  <Chip
                    label={`${res.duration_ms.toFixed(0)} ms`}
                    size="small"
                    sx={{ fontSize: "0.7rem", backgroundColor: "#F1F5F9" }}
                  />
                </Box>
              ))}
            </Box>
          </Box>
        ) : (
          /* Action Plan Preview */
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {/* Status Summary */}
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                backgroundColor: "#F8FAFC",
                p: 1.5,
                borderRadius: 2,
                border: "1px solid #E2E8F0",
              }}
            >
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Configuration Reconciliation Plan
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Diff calculated against recorded state and live device interfaces
                </Typography>
              </Box>
              <Box sx={{ display: "flex", gap: 1 }}>
                <Chip
                  label={`${pendingActions.length} Pending`}
                  size="small"
                  color={pendingActions.length > 0 ? "primary" : "default"}
                  sx={{ fontWeight: 700 }}
                />
                <Chip
                  label={`${syncedActions.length} In Sync`}
                  size="small"
                  sx={{ fontWeight: 600, bgcolor: "#ECFDF5", color: "#059669" }}
                />
              </Box>
            </Box>

            {pendingActions.length === 0 && safeActions.length > 0 && (
              <Alert severity="success" sx={{ borderRadius: 2 }}>
                All WireGuard interfaces and BIRD routing configurations are currently in sync with recorded device
                states. You can use <strong>Force Apply All</strong> to re-push configs from scratch, or{" "}
                <strong>Update State</strong> to refresh from remote hosts.
              </Alert>
            )}

            {safeActions.length === 0 && (
              <Alert severity="info" sx={{ borderRadius: 2 }}>
                No mesh nodes or links configured yet.
              </Alert>
            )}

            {/* Pending Actions Section */}
            {pendingActions.length > 0 && (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Typography
                  variant="caption"
                  sx={{ fontWeight: 700, color: "#475569", textTransform: "uppercase", letterSpacing: "0.5px" }}
                >
                  Required Actions to Apply ({pendingActions.length})
                </Typography>
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1, maxHeight: 260, overflowY: "auto" }}>
                  {pendingActions.map((act, i) => (
                    <Accordion
                      key={i}
                      elevation={0}
                      sx={{
                        backgroundColor: "#FFFFFF",
                        border: "1px solid #CBD5E1",
                        borderRadius: "8px !important",
                        "&:before": { display: "none" },
                      }}
                    >
                      <AccordionSummary expandIcon={<ChevronDown size={18} color="#64748B" />}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flex: 1 }}>
                          {act.type === "delete_config" ||
                          act.type === "down_interface" ||
                          act.diff_status === "delete" ? (
                            <Trash2 size={16} color="#DC2626" />
                          ) : act.type === "sync_bird" ? (
                            <Network size={16} color="#0284C7" />
                          ) : (
                            <FileCode size={16} color="#4F46E5" />
                          )}
                          <Box sx={{ flex: 1 }}>
                            <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                              {act.description}
                            </Typography>
                            <Typography
                              variant="caption"
                              className="mono-font"
                              sx={{ color: "#64748B", fontSize: "0.7rem" }}
                            >
                              {act.host}:{act.target_file} {act.command ? `• (${act.command})` : ""}
                            </Typography>
                          </Box>
                          {act.diff_status === "delete" || act.type === "delete_config" ? (
                            <Chip
                              label="DELETE"
                              size="small"
                              sx={{
                                fontSize: "0.65rem",
                                fontWeight: 700,
                                height: 20,
                                bgcolor: "#FEE2E2",
                                color: "#DC2626",
                                mr: 1,
                              }}
                            />
                          ) : act.diff_status === "update" ? (
                            <Chip
                              label="UPDATE"
                              size="small"
                              sx={{
                                fontSize: "0.65rem",
                                fontWeight: 700,
                                height: 20,
                                bgcolor: "#FEF3C7",
                                color: "#D97706",
                                mr: 1,
                              }}
                            />
                          ) : (
                            <Chip
                              label="CREATE"
                              size="small"
                              sx={{
                                fontSize: "0.65rem",
                                fontWeight: 700,
                                height: 20,
                                bgcolor: "#E0E7FF",
                                color: "#4338CA",
                                mr: 1,
                              }}
                            />
                          )}
                        </Box>
                      </AccordionSummary>
                      <AccordionDetails sx={{ pt: 0 }}>
                        {act.command && act.file_content && (
                          <Box
                            sx={{
                              mb: 1,
                              p: 0.8,
                              borderRadius: 1,
                              backgroundColor: "#F1F5F9",
                              border: "1px solid #E2E8F0",
                              fontSize: "0.72rem",
                              color: "#334155",
                            }}
                          >
                            <strong>Apply Command:</strong> <code>{act.command}</code>
                          </Box>
                        )}
                        <Box
                          component="pre"
                          className="mono-font"
                          sx={{
                            p: 1.5,
                            m: 0,
                            borderRadius: 1.5,
                            backgroundColor: "#F8FAFC",
                            border: "1px solid #E2E8F0",
                            fontSize: "0.75rem",
                            color: "#1E293B",
                            overflowX: "auto",
                            whiteSpace: "pre-wrap",
                          }}
                        >
                          {act.file_content || act.command}
                        </Box>
                      </AccordionDetails>
                    </Accordion>
                  ))}
                </Box>
              </Box>
            )}

            {/* Already Synchronized Section */}
            {syncedActions.length > 0 && (
              <Accordion
                elevation={0}
                defaultExpanded={pendingActions.length === 0}
                sx={{
                  backgroundColor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                  borderRadius: "8px !important",
                  "&:before": { display: "none" },
                }}
              >
                <AccordionSummary expandIcon={<ChevronDown size={18} color="#64748B" />}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flex: 1 }}>
                    <CheckCheck size={18} color="#059669" />
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155" }}>
                      Already Synchronized on Nodes ({syncedActions.length})
                    </Typography>
                    <Chip
                      label="UP TO DATE"
                      size="small"
                      sx={{
                        fontSize: "0.65rem",
                        fontWeight: 700,
                        height: 20,
                        bgcolor: "#ECFDF5",
                        color: "#059669",
                        ml: "auto",
                        mr: 1,
                      }}
                    />
                  </Box>
                </AccordionSummary>
                <AccordionDetails sx={{ pt: 0 }}>
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1, maxHeight: 180, overflowY: "auto" }}>
                    {syncedActions.map((act, i) => (
                      <Box
                        key={i}
                        sx={{
                          p: 1,
                          px: 1.5,
                          borderRadius: 1.5,
                          backgroundColor: "#FFFFFF",
                          border: "1px solid #E2E8F0",
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                        }}
                      >
                        <Box>
                          <Typography variant="body2" sx={{ fontSize: "0.8rem", color: "#334155" }}>
                            {act.description}
                          </Typography>
                          <Typography
                            variant="caption"
                            className="mono-font"
                            sx={{ color: "#94A3B8", fontSize: "0.68rem" }}
                          >
                            {act.host}:{act.target_file}
                          </Typography>
                        </Box>
                        <Chip
                          label="SYNCED"
                          size="small"
                          sx={{
                            fontSize: "0.65rem",
                            fontWeight: 600,
                            height: 18,
                            bgcolor: "#F1F5F9",
                            color: "#64748B",
                          }}
                        />
                      </Box>
                    ))}
                  </Box>
                </AccordionDetails>
              </Accordion>
            )}

            {syncError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {syncError}
              </Alert>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions
        sx={{
          px: 3,
          py: 2,
          borderTop: "1px solid #E2E8F0",
          backgroundColor: "#F8FAFC",
          justifyContent: "space-between",
        }}
      >
        <Button onClick={onClose} disabled={syncing || updatingState} sx={{ color: "#64748B" }}>
          {safeResults ? "Close" : "Cancel"}
        </Button>

        {!safeResults && (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            {/* Force Apply All Button */}
            {safeActions.length > 0 && (
              <Tooltip title="Force push all configurations to remote nodes, re-writing files and restarting interfaces even if recorded as synced.">
                <Button
                  variant="outlined"
                  color="warning"
                  onClick={() => handleExecuteSync(true)}
                  disabled={syncing || updatingState}
                  startIcon={syncing ? <CircularProgress size={14} color="inherit" /> : <Zap size={14} />}
                  sx={{
                    borderColor: "#F59E0B",
                    color: "#D97706",
                    fontWeight: 600,
                    fontSize: "0.82rem",
                    textTransform: "none",
                    "&:hover": {
                      borderColor: "#D97706",
                      backgroundColor: "rgba(245, 158, 11, 0.08)",
                    },
                  }}
                >
                  Force Apply All ({safeActions.length})
                </Button>
              </Tooltip>
            )}

            {/* Default Apply Button (Only applies diffed / pending actions) */}
            <Button
              variant="contained"
              color="primary"
              onClick={() => handleExecuteSync(false)}
              disabled={syncing || updatingState || pendingActions.length === 0}
              startIcon={syncing ? <CircularProgress size={16} color="inherit" /> : <Check size={16} />}
              sx={{
                background: pendingActions.length > 0 ? "linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)" : undefined,
                fontWeight: 700,
                fontSize: "0.82rem",
                textTransform: "none",
                px: 2.5,
              }}
            >
              {syncing
                ? "Applying to Nodes..."
                : pendingActions.length > 0
                  ? `Apply Changes (${pendingActions.length})`
                  : "No Pending Changes"}
            </Button>
          </Box>
        )}
      </DialogActions>
    </Dialog>
  );
};
