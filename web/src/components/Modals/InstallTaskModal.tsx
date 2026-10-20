import React, { useState, useEffect, useCallback } from "react";
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
  IconButton,
  TextField,
  InputAdornment,
  FormControl,
  Select,
  MenuItem,
  Paper,
  Tooltip,
} from "@mui/material";
import {
  DownloadCloud,
  Globe,
  Search,
  X,
  RotateCw,
  CheckCircle2,
  Trash2,
  FileCode,
  Sparkles,
} from "lucide-react";
import { api } from "../../api/client";
import { OnlineTask, TaskMeta } from "../../types/api";

interface InstallTaskModalProps {
  open: boolean;
  onClose: () => void;
  installedTasks: TaskMeta[];
  onTaskInstalled: () => void;
}

export const InstallTaskModal: React.FC<InstallTaskModalProps> = ({
  open,
  onClose,
  installedTasks,
  onTaskInstalled,
}) => {
  const [sources, setSources] = useState<string[]>([]);
  const [defaultSource, setDefaultSource] = useState<string>("");
  const [selectedSource, setSelectedSource] = useState<string>("");
  const [tasks, setTasks] = useState<OnlineTask[]>([]);
  const [loadingSources, setLoadingSources] = useState<boolean>(false);
  const [loadingManifest, setLoadingManifest] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [searchFilter, setSearchFilter] = useState<string>("");

  // Track install / delete actions per task ID
  const [actionInProgress, setActionInProgress] = useState<Record<string, "install" | "delete">>({});
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  // Delete confirmation state
  const [confirmDeleteTask, setConfirmDeleteTask] = useState<string | null>(null);

  const installedTaskMap = React.useMemo(() => {
    const map = new Map<string, TaskMeta>();
    installedTasks.forEach((t) => map.set(t.id, t));
    return map;
  }, [installedTasks]);

  const loadSources = useCallback(async () => {
    setLoadingSources(true);
    setError(null);
    try {
      const res = await api.getOnlineSources();
      setSources(res.sources || []);
      setDefaultSource(res.default_source || "");
      if (!selectedSource && res.sources && res.sources.length > 0) {
        setSelectedSource(res.sources[0]);
      }
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to load online sources");
    } finally {
      setLoadingSources(false);
    }
  }, [selectedSource]);

  const loadManifest = useCallback(async (sourceURL: string) => {
    if (!sourceURL) return;
    setLoadingManifest(true);
    setError(null);
    setTasks([]);
    try {
      const manifest = await api.getOnlineManifest(sourceURL);
      setTasks(manifest.tasks || []);
    } catch (err: unknown) {
      const e = err as Error;
      setError(
        e.message ||
          "Failed to load repository manifest. Ensure manifest.json exists in the repository root.",
      );
    } finally {
      setLoadingManifest(false);
    }
  }, []);

  useEffect(() => {
    if (open) {
      loadSources();
      setSuccessMessage(null);
      setError(null);
    }
  }, [open, loadSources]);

  useEffect(() => {
    if (open && selectedSource) {
      loadManifest(selectedSource);
    }
  }, [open, selectedSource, loadManifest]);

  const handleInstall = async (task: OnlineTask) => {
    setActionInProgress((prev) => ({ ...prev, [task.id]: "install" }));
    setError(null);
    setSuccessMessage(null);
    try {
      await api.installOnlineTask(selectedSource, task.id, task.files);
      setSuccessMessage(`Successfully installed task "${task.title}"!`);
      onTaskInstalled();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || `Failed to install task ${task.id}`);
    } finally {
      setActionInProgress((prev) => {
        const next = { ...prev };
        delete next[task.id];
        return next;
      });
    }
  };

  const handleDelete = async (taskId: string) => {
    setActionInProgress((prev) => ({ ...prev, [taskId]: "delete" }));
    setError(null);
    setSuccessMessage(null);
    setConfirmDeleteTask(null);
    try {
      await api.deleteCustomTask(taskId);
      setSuccessMessage(`Custom task "${taskId}" was deleted.`);
      onTaskInstalled();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || `Failed to delete task ${taskId}`);
    } finally {
      setActionInProgress((prev) => {
        const next = { ...prev };
        delete next[taskId];
        return next;
      });
    }
  };

  const filteredTasks = tasks.filter((t) => {
    const q = searchFilter.trim().toLowerCase();
    if (!q) return true;
    return (
      (t.title || "").toLowerCase().includes(q) ||
      (t.id || "").toLowerCase().includes(q) ||
      (t.description || "").toLowerCase().includes(q) ||
      (t.category || "").toLowerCase().includes(q)
    );
  });

  return (
    <>
      <Dialog
        open={open}
        onClose={onClose}
        maxWidth="md"
        fullWidth
        PaperProps={{
          sx: {
            borderRadius: 3,
            bgcolor: "background.paper",
            maxHeight: "85vh",
            display: "flex",
            flexDirection: "column",
          },
        }}
      >
        <DialogTitle
          sx={{
            p: 2.5,
            pb: 1.5,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: 1,
            borderColor: "divider",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Box
              sx={{
                width: 38,
                height: 38,
                borderRadius: 2,
                bgcolor: "primary.main",
                color: "#ffffff",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Globe size={20} />
            </Box>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography variant="h6" sx={{ fontWeight: 800 }}>
                  Online Tasks Repository
                </Typography>
                <Chip
                  size="small"
                  label="Community & Official"
                  color="primary"
                  variant="outlined"
                  sx={{ fontSize: "0.68rem", height: 20, fontWeight: 700 }}
                />
              </Box>
              <Typography variant="caption" color="text.secondary">
                Browse and install ready-to-run automation tasks directly into your Easy42 instance
              </Typography>
            </Box>
          </Box>
          <IconButton size="small" onClick={onClose} sx={{ color: "text.secondary" }}>
            <X size={18} />
          </IconButton>
        </DialogTitle>

        <DialogContent sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 2 }}>
          {/* Source Selection & Refresh Toolbar */}
          <Paper
            elevation={0}
            sx={{
              p: 2,
              borderRadius: 2,
              border: "1px solid",
              borderColor: "divider",
              bgcolor: "background.default",
              display: "flex",
              flexDirection: { xs: "column", sm: "row" },
              alignItems: { xs: "stretch", sm: "center" },
              gap: 1.5,
            }}
          >
            <Box sx={{ flex: 1 }}>
              <Typography
                variant="caption"
                sx={{ fontWeight: 700, textTransform: "uppercase", color: "text.secondary", display: "block", mb: 0.5 }}
              >
                Repository Source URL
              </Typography>
              <FormControl size="small" fullWidth>
                <Select
                  value={selectedSource || (sources.length > 0 ? sources[0] : "")}
                  onChange={(e) => setSelectedSource(e.target.value)}
                  disabled={loadingSources || loadingManifest}
                  sx={{
                    bgcolor: "background.paper",
                    borderRadius: 1.5,
                    fontSize: "0.82rem",
                    fontFamily: "monospace",
                  }}
                >
                  {sources.map((s) => (
                    <MenuItem key={s} value={s} sx={{ fontSize: "0.82rem", fontFamily: "monospace" }}>
                      {s === defaultSource ? `${s} (Official Default)` : s}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            </Box>

            <Box sx={{ display: "flex", alignItems: "flex-end", gap: 1 }}>
              <Button
                variant="outlined"
                size="small"
                onClick={() => loadManifest(selectedSource)}
                disabled={loadingManifest || !selectedSource}
                startIcon={<RotateCw size={14} className={loadingManifest ? "animate-spin" : undefined} />}
                sx={{
                  height: 38,
                  textTransform: "none",
                  fontWeight: 600,
                  bgcolor: "background.paper",
                }}
              >
                {loadingManifest ? "Fetching..." : "Fetch Manifest"}
              </Button>
            </Box>
          </Paper>

          {/* Sources customization tip */}
          <Typography variant="caption" sx={{ color: "text.secondary", px: 0.5 }}>
            Tip: You can add custom online repository sources by creating <code>&lt;data-dir&gt;/tasks/sources.txt</code> and adding URLs line-by-line.
          </Typography>

          {/* Alerts */}
          {error && (
            <Alert severity="error" onClose={() => setError(null)} sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}
          {successMessage && (
            <Alert severity="success" onClose={() => setSuccessMessage(null)} sx={{ borderRadius: 2 }}>
              {successMessage}
            </Alert>
          )}

          {/* Filter Bar */}
          <TextField
            size="small"
            fullWidth
            placeholder="Search online tasks by name, category, or description..."
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start" sx={{ color: "text.secondary", mr: 0.5 }}>
                  <Search size={15} />
                </InputAdornment>
              ),
              endAdornment: searchFilter ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={() => setSearchFilter("")} sx={{ p: 0.25 }}>
                    <X size={13} />
                  </IconButton>
                </InputAdornment>
              ) : null,
              sx: {
                borderRadius: 1.5,
                bgcolor: "background.paper",
                fontSize: "0.85rem",
              },
            }}
          />

          {/* Tasks List Content */}
          <Box sx={{ flex: 1, minHeight: 250, overflowY: "auto" }}>
            {loadingManifest ? (
              <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", py: 8 }}>
                <CircularProgress size={32} sx={{ mb: 1.5 }} />
                <Typography variant="body2" color="text.secondary">
                  Loading repository manifest...
                </Typography>
              </Box>
            ) : tasks.length === 0 ? (
              <Box
                sx={{
                  textAlign: "center",
                  py: 6,
                  px: 2,
                  bgcolor: "background.default",
                  borderRadius: 2,
                  border: "1px dashed",
                  borderColor: "divider",
                }}
              >
                <Sparkles size={32} color="#94a3b8" style={{ marginBottom: 8 }} />
                <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 0.5 }}>
                  No Tasks Found in Repository
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ display: "block", maxWidth: 450, mx: "auto" }}>
                  The source repository may be empty or its <code>manifest.json</code> is still being set up.
                </Typography>
              </Box>
            ) : filteredTasks.length === 0 ? (
              <Box sx={{ textAlign: "center", py: 6, color: "text.secondary" }}>
                <Typography variant="body2">No tasks match "{searchFilter}"</Typography>
              </Box>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                {filteredTasks.map((task) => {
                  const isInstalled = installedTaskMap.has(task.id);
                  const isActioning = Boolean(actionInProgress[task.id]);
                  const actionType = actionInProgress[task.id];

                  return (
                    <Paper
                      key={task.id}
                      elevation={0}
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        border: "1px solid",
                        borderColor: isInstalled ? "success.light" : "divider",
                        bgcolor: isInstalled ? "rgba(34, 197, 94, 0.02)" : "background.paper",
                        display: "flex",
                        flexDirection: { xs: "column", sm: "row" },
                        alignItems: { xs: "flex-start", sm: "center" },
                        justifyContent: "space-between",
                        gap: 2,
                        transition: "all 0.15s ease",
                        "&:hover": {
                          borderColor: isInstalled ? "success.main" : "primary.light",
                          boxShadow: "0 2px 8px rgba(0,0,0,0.04)",
                        },
                      }}
                    >
                      <Box sx={{ flex: 1 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5, flexWrap: "wrap" }}>
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, fontSize: "0.95rem" }}>
                            {task.title}
                          </Typography>
                          <Chip
                            size="small"
                            label={task.category || "General"}
                            sx={{ fontSize: "0.65rem", height: 18 }}
                          />
                          {task.version && (
                            <Chip
                              size="small"
                              label={`v${task.version}`}
                              variant="outlined"
                              sx={{ fontSize: "0.65rem", height: 18 }}
                            />
                          )}
                          {isInstalled && (
                            <Chip
                              size="small"
                              icon={<CheckCircle2 size={12} />}
                              label="Installed"
                              color="success"
                              sx={{ fontWeight: 700, fontSize: "0.68rem", height: 20 }}
                            />
                          )}
                        </Box>

                        <Typography variant="body2" color="text.secondary" sx={{ mb: 1, fontSize: "0.82rem" }}>
                          {task.description}
                        </Typography>

                        {/* Files list chips */}
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, flexWrap: "wrap" }}>
                          <Typography variant="caption" sx={{ color: "text.disabled", fontWeight: 600 }}>
                            Files:
                          </Typography>
                          {(task.files && task.files.length > 0 ? task.files : ["run.sh"]).map((file) => (
                            <Chip
                              key={file}
                              icon={<FileCode size={11} />}
                              size="small"
                              label={file}
                              variant="outlined"
                              sx={{ height: 18, fontSize: "0.65rem", fontFamily: "monospace" }}
                            />
                          ))}
                        </Box>
                      </Box>

                      {/* Action buttons */}
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1, alignSelf: { xs: "flex-end", sm: "center" } }}>
                        {isInstalled && (
                          <Tooltip title="Delete this installed task from server filesystem">
                            <span>
                              <IconButton
                                size="small"
                                color="error"
                                disabled={isActioning}
                                onClick={() => setConfirmDeleteTask(task.id)}
                                sx={{ border: "1px solid", borderColor: "error.light" }}
                              >
                                {isActioning && actionType === "delete" ? (
                                  <CircularProgress size={16} color="inherit" />
                                ) : (
                                  <Trash2 size={15} />
                                )}
                              </IconButton>
                            </span>
                          </Tooltip>
                        )}

                        <Button
                          size="small"
                          variant={isInstalled ? "outlined" : "contained"}
                          color={isInstalled ? "primary" : "primary"}
                          disabled={isActioning}
                          startIcon={
                            isActioning && actionType === "install" ? (
                              <CircularProgress size={14} color="inherit" />
                            ) : (
                              <DownloadCloud size={14} />
                            )
                          }
                          onClick={() => handleInstall(task)}
                          sx={{
                            textTransform: "none",
                            fontWeight: 700,
                            whiteSpace: "nowrap",
                          }}
                        >
                          {isActioning && actionType === "install"
                            ? "Installing..."
                            : isInstalled
                            ? "Update / Reinstall"
                            : "Install Task"}
                        </Button>
                      </Box>
                    </Paper>
                  );
                })}
              </Box>
            )}
          </Box>
        </DialogContent>

        <DialogActions sx={{ p: 2, px: 2.5, borderTop: 1, borderColor: "divider", justifyContent: "space-between" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography variant="caption" color="text.secondary">
              Default repository:
            </Typography>
            <Typography variant="caption" sx={{ fontFamily: "monospace", color: "text.primary" }}>
              sagan/easy42-tasks
            </Typography>
          </Box>
          <Button onClick={onClose} variant="outlined" size="small" sx={{ textTransform: "none", fontWeight: 600 }}>
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={Boolean(confirmDeleteTask)}
        onClose={() => setConfirmDeleteTask(null)}
        maxWidth="xs"
        fullWidth
        PaperProps={{ sx: { borderRadius: 2.5 } }}
      >
        <DialogTitle sx={{ fontWeight: 700 }}>Delete Custom Task?</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary">
            Are you sure you want to delete custom task <strong>"{confirmDeleteTask}"</strong>?
            This will permanently remove the task directory from your server's filesystem.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ p: 2 }}>
          <Button onClick={() => setConfirmDeleteTask(null)} size="small" sx={{ textTransform: "none" }}>
            Cancel
          </Button>
          <Button
            onClick={() => confirmDeleteTask && handleDelete(confirmDeleteTask)}
            color="error"
            variant="contained"
            size="small"
            sx={{ textTransform: "none", fontWeight: 700 }}
          >
            Delete Task
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};
