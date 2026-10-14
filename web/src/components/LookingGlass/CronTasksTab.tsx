import React, { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Typography,
  Button,
  Paper,
  Chip,
  Switch,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  MenuItem,
  Select,
  FormControl,
  InputLabel,
  Alert,
  CircularProgress,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  TableContainer,
} from "@mui/material";
import { Clock, Plus, Trash2, Edit2, Play, CheckCircle2, Activity, RefreshCw } from "lucide-react";
import { api } from "../../api/client";
import type { CronTask, Node } from "../../types/api";

interface CronTasksTabProps {
  nodes: Node[];
  onExecuteNow?: (task: CronTask) => void;
}

export const describeCronExpr = (expr: string): string => {
  const clean = expr.trim();
  if (clean === "* * * * *" || clean === "*/1 * * * *") return "Every minute";
  if (clean === "*/2 * * * *") return "Every 2 minutes";
  if (clean === "*/5 * * * *") return "Every 5 minutes";
  if (clean === "*/10 * * * *") return "Every 10 minutes";
  if (clean === "*/15 * * * *") return "Every 15 minutes";
  if (clean === "*/30 * * * *") return "Every 30 minutes";
  if (clean === "0 * * * *" || clean === "0 */1 * * *") return "Every hour at minute 0";
  if (clean === "0 */2 * * *") return "Every 2 hours";
  if (clean === "0 0 * * *") return "Daily at midnight";
  if (clean === "0 12 * * *") return "Daily at 12:00 PM";
  return `Schedule: ${clean}`;
};

export const CronTasksTab: React.FC<CronTasksTabProps> = ({ nodes, onExecuteNow }) => {
  const navigate = useNavigate();
  const [tasks, setTasks] = useState<CronTask[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [error, setError] = useState<string | null>(null);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  // Modal state
  const [modalOpen, setModalOpen] = useState<boolean>(false);
  const [editingTask, setEditingTask] = useState<CronTask | null>(null);

  // Form fields
  const [formName, setFormName] = useState("");
  const [formDesc, setFormDesc] = useState("");
  const [formType, setFormType] = useState<"ping" | "tcp_ping">("ping");
  const [formTarget, setFormTarget] = useState("");
  const [formCount, setFormCount] = useState("4");
  const [formTimeout, setFormTimeout] = useState("15");
  const [formCronExpr, setFormCronExpr] = useState("*/5 * * * *");
  const [formNodes, setFormNodes] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const fetchTasks = async () => {
    setLoading(true);
    try {
      const data = await api.getCronTasks();
      setTasks(data || []);
      setError(null);
    } catch (err: any) {
      setError(err?.message || "Failed to load cron tasks");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchTasks();
  }, []);

  // Managed nodes only (exclude external nodes)
  const availableNodes = React.useMemo(() => {
    return (nodes || []).filter((n) => !n.is_external);
  }, [nodes]);

  const openCreateModal = () => {
    setEditingTask(null);
    setFormName("");
    setFormDesc("");
    setFormType("ping");
    setFormTarget("1.1.1.1");
    setFormCount("4");
    setFormTimeout("15");
    setFormCronExpr("*/5 * * * *");
    // Default to first agent node or first available node
    const defaultNode =
      availableNodes.find((n) => n.mode === "agent" || Boolean(n.agent_token))?.name || availableNodes[0]?.name;
    setFormNodes(defaultNode ? [defaultNode] : []);
    setFormError(null);
    setModalOpen(true);
  };

  const openEditModal = (task: CronTask) => {
    setEditingTask(task);
    setFormName(task.name);
    setFormDesc(task.description || "");
    setFormType(task.task_type);
    setFormTarget(task.params?.target || "");
    setFormCount(task.params?.count || "4");
    setFormTimeout(task.timeout_sec ? String(task.timeout_sec) : "15");
    setFormCronExpr(task.cron_expr);
    setFormNodes(task.nodes || []);
    setFormError(null);
    setModalOpen(true);
  };

  const handleSave = async () => {
    if (!formName.trim()) {
      setFormError("Task name is required");
      return;
    }
    if (!formTarget.trim()) {
      setFormError("Target host or address is required");
      return;
    }
    if (formNodes.length === 0) {
      setFormError("Select at least one target node");
      return;
    }
    if (!formCronExpr.trim()) {
      setFormError("Cron time expression is required");
      return;
    }

    setSaving(true);
    setFormError(null);
    try {
      const payload: Partial<CronTask> = {
        name: formName.trim(),
        description: formDesc.trim(),
        task_type: formType,
        cron_expr: formCronExpr.trim(),
        nodes: formNodes,
        params: {
          target: formTarget.trim(),
          count: formCount.trim() || "4",
        },
        timeout_sec: parseInt(formTimeout, 10) || 15,
        enabled: editingTask ? editingTask.enabled : true,
      };

      if (editingTask?.id) {
        await api.updateCronTask(editingTask.id, payload);
        setSuccessMsg(`Cron task "${formName}" updated successfully`);
      } else {
        await api.createCronTask(payload);
        setSuccessMsg(`Cron task "${formName}" created successfully`);
      }
      setModalOpen(false);
      await fetchTasks();
    } catch (err: any) {
      setFormError(err?.message || "Failed to save cron task");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (task: CronTask) => {
    if (!window.confirm(`Are you sure you want to delete cron task "${task.name}"?`)) {
      return;
    }
    try {
      await api.deleteCronTask(task.id);
      setSuccessMsg(`Cron task "${task.name}" deleted`);
      await fetchTasks();
    } catch (err: any) {
      setError(err?.message || "Failed to delete cron task");
    }
  };

  const handleToggle = async (task: CronTask) => {
    try {
      await api.toggleCronTask(task.id);
      await fetchTasks();
    } catch (err: any) {
      setError(err?.message || "Failed to toggle cron task");
    }
  };

  const toggleNodeSelection = (nodeName: string) => {
    if (formNodes.includes(nodeName)) {
      setFormNodes(formNodes.filter((n) => n !== nodeName));
    } else {
      setFormNodes([...formNodes, nodeName]);
    }
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Banner / Header */}
      <Paper
        elevation={0}
        sx={{
          p: 2.5,
          borderRadius: 3,
          bgcolor: "#FFFFFF",
          border: "1px solid #E2E8F0",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          flexWrap: "wrap",
          gap: 2,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Box
            sx={{
              p: 1.5,
              borderRadius: 2.5,
              bgcolor: "#EEF2FF",
              color: "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Clock size={24} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "1.1rem" }}>
              Scheduled Cron Tasks
            </Typography>
            <Typography variant="body2" sx={{ color: "#64748B", mt: 0.25 }}>
              Periodically probe network paths via node agents. Metrics & history are persisted to SQLite.
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", gap: 1.5 }}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<RefreshCw size={14} className={loading ? "spin" : ""} />}
            onClick={fetchTasks}
            disabled={loading}
            sx={{ borderColor: "#CBD5E1", color: "#475569" }}
          >
            Refresh
          </Button>
          <Button
            variant="contained"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={openCreateModal}
            sx={{
              bgcolor: "#4F46E5",
              fontWeight: 600,
              textTransform: "none",
              px: 2,
              "&:hover": { bgcolor: "#4338CA" },
            }}
          >
            New Cron Task
          </Button>
        </Box>
      </Paper>

      {/* Info Notice */}
      <Alert
        severity="info"
        icon={<Activity size={18} />}
        sx={{
          borderRadius: 2.5,
          bgcolor: "#F0FDF4",
          color: "#166534",
          border: "1px solid #BBF7D0",
          "& .MuiAlert-icon": { color: "#16A34A" },
        }}
      >
        <strong>Agent-Driven Execution:</strong> Only <strong>Ping (ICMP)</strong> and <strong>TCP Ping</strong> tasks
        are supported as cron tasks. Tasks run stateless in node agent memory and stream latency & loss statistics back
        to easy42.
      </Alert>

      {error && (
        <Alert severity="error" onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {successMsg && (
        <Alert severity="success" onClose={() => setSuccessMsg(null)}>
          {successMsg}
        </Alert>
      )}

      {/* Tasks Table */}
      {loading && tasks.length === 0 ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
          <CircularProgress size={36} />
        </Box>
      ) : tasks.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            p: 6,
            textAlign: "center",
            borderRadius: 3,
            bgcolor: "#FFFFFF",
            border: "1px dashed #CBD5E1",
          }}
        >
          <Clock size={40} color="#94A3B8" style={{ marginBottom: 12 }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#334155" }}>
            No Cron Tasks Configured
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", mt: 0.5, maxWidth: 450, mx: "auto", mb: 2 }}>
            Create automated recurring ping or TCP ping checks to continuously monitor connectivity and latency across
            your nodes.
          </Typography>
          <Button
            variant="contained"
            startIcon={<Plus size={16} />}
            onClick={openCreateModal}
            sx={{ bgcolor: "#4F46E5", textTransform: "none", fontWeight: 600 }}
          >
            Create Your First Cron Task
          </Button>
        </Paper>
      ) : (
        <TableContainer
          component={Paper}
          elevation={0}
          sx={{ borderRadius: 3, border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}
        >
          <Table>
            <TableHead sx={{ bgcolor: "#F8FAFC" }}>
              <TableRow>
                <TableCell sx={{ fontWeight: 700, color: "#475569", width: 80 }}>Active</TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Task Name</TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Type</TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Target</TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Schedule</TableCell>
                <TableCell sx={{ fontWeight: 700, color: "#475569" }}>Nodes</TableCell>
                <TableCell align="right" sx={{ fontWeight: 700, color: "#475569" }}>
                  Actions
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {tasks.map((task) => (
                <TableRow key={task.id} hover sx={{ "&:last-child td, &:last-child th": { border: 0 } }}>
                  <TableCell>
                    <Switch size="small" checked={task.enabled} onChange={() => handleToggle(task)} color="primary" />
                  </TableCell>
                  <TableCell>
                    <Box>
                      <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                        {task.name}
                      </Typography>
                      {task.description && (
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {task.description}
                        </Typography>
                      )}
                    </Box>
                  </TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      label={task.task_type === "tcp_ping" ? "TCP Ping" : "ICMP Ping"}
                      sx={{
                        fontWeight: 600,
                        fontSize: "0.75rem",
                        bgcolor: task.task_type === "tcp_ping" ? "#F5F3FF" : "#EFF6FF",
                        color: task.task_type === "tcp_ping" ? "#7C3AED" : "#2563EB",
                        border: `1px solid ${task.task_type === "tcp_ping" ? "#DDD6FE" : "#BFDBFE"}`,
                      }}
                    />
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2" sx={{ fontFamily: "monospace", fontWeight: 600, color: "#334155" }}>
                      {task.params?.target || "—"}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Box>
                      <Chip
                        size="small"
                        label={task.cron_expr}
                        sx={{
                          fontFamily: "monospace",
                          fontWeight: 600,
                          fontSize: "0.75rem",
                          bgcolor: "#F1F5F9",
                          color: "#334155",
                          mb: 0.5,
                        }}
                      />
                      <Typography variant="caption" display="block" sx={{ color: "#64748B" }}>
                        {describeCronExpr(task.cron_expr)}
                      </Typography>
                    </Box>
                  </TableCell>
                  <TableCell>
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, maxWidth: 220 }}>
                      {task.nodes && task.nodes.length > 0 ? (
                        task.nodes.map((nodeName) => (
                          <Chip
                            size="small"
                            title={`View task "${task.name}" on ${nodeName}`}
                            label={nodeName}
                            clickable
                            onClick={() =>
                              navigate(
                                `/nodes/${encodeURIComponent(nodeName)}?tab=cron_tasks&task=${encodeURIComponent(task.id)}`,
                              )
                            }
                            variant="outlined"
                            sx={{
                              fontSize: "0.7rem",
                              height: 22,
                              borderColor: "#CBD5E1",
                              color: "#4F46E5",
                              bgcolor: "#F8FAFC",
                              cursor: "pointer",
                              fontWeight: 500,
                              "&:hover": {
                                borderColor: "#818CF8",
                                bgcolor: "#EEF2FF",
                                color: "#3730A3",
                              },
                            }}
                          />
                        ))
                      ) : (
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          None
                        </Typography>
                      )}
                    </Box>
                  </TableCell>
                  <TableCell align="right">
                    <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.5 }}>
                      {onExecuteNow && (
                        <IconButton
                          size="small"
                          title="Run now in Looking Glass"
                          onClick={() => onExecuteNow(task)}
                          sx={{ color: "#4F46E5", "&:hover": { bgcolor: "#EEF2FF" } }}
                        >
                          <Play size={16} />
                        </IconButton>
                      )}
                      <IconButton
                        size="small"
                        title="Edit task"
                        onClick={() => openEditModal(task)}
                        sx={{ color: "#64748B", "&:hover": { bgcolor: "#F1F5F9" } }}
                      >
                        <Edit2 size={16} />
                      </IconButton>
                      <IconButton
                        size="small"
                        title="Delete task"
                        onClick={() => handleDelete(task)}
                        sx={{ color: "#DC2626", "&:hover": { bgcolor: "#FEE2E2" } }}
                      >
                        <Trash2 size={16} />
                      </IconButton>
                    </Box>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {/* Create / Edit Dialog */}
      <Dialog
        open={modalOpen}
        onClose={() => !saving && setModalOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { borderRadius: 3, p: 1 } }}
      >
        <DialogTitle sx={{ fontWeight: 700, color: "#0F172A", display: "flex", alignItems: "center", gap: 1.5 }}>
          <Clock size={20} color="#4F46E5" />
          {editingTask ? "Edit Cron Task" : "Create New Cron Task"}
        </DialogTitle>

        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: 1 }}>
          {formError && (
            <Alert severity="error" onClose={() => setFormError(null)}>
              {formError}
            </Alert>
          )}

          {/* Task Name & Description */}
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
            <TextField
              label="Task Name"
              required
              fullWidth
              size="small"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              placeholder="e.g. Cloudflare DNS Ping"
            />
            <TextField
              label="Description (Optional)"
              fullWidth
              size="small"
              value={formDesc}
              onChange={(e) => setFormDesc(e.target.value)}
              placeholder="e.g. Measure path latency to 1.1.1.1"
            />
          </Box>

          {/* Task Type & Target */}
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 2fr", gap: 2 }}>
            <FormControl size="small" fullWidth>
              <InputLabel id="task-type-label">Task Type</InputLabel>
              <Select
                labelId="task-type-label"
                value={formType}
                label="Task Type"
                onChange={(e) => {
                  const val = e.target.value as "ping" | "tcp_ping";
                  setFormType(val);
                  if (val === "tcp_ping" && !formTarget.includes(":")) {
                    setFormTarget(formTarget ? `${formTarget}:443` : "1.1.1.1:443");
                  } else if (val === "ping" && formTarget.includes(":")) {
                    setFormTarget(formTarget.split(":")[0]);
                  }
                }}
              >
                <MenuItem value="ping">Ping (ICMP)</MenuItem>
                <MenuItem value="tcp_ping">TCP Ping (Port Probe)</MenuItem>
              </Select>
            </FormControl>

            <TextField
              label={formType === "tcp_ping" ? "Target (<host>:<port>)" : "Target Host or IP"}
              required
              fullWidth
              size="small"
              value={formTarget}
              onChange={(e) => setFormTarget(e.target.value)}
              placeholder={
                formType === "tcp_ping" ? "e.g. 1.1.1.1:443 or example.com:80" : "e.g. 1.1.1.1 or example.com"
              }
              helperText={
                formType === "tcp_ping"
                  ? "Requires easy42-agent on nodes. Measures TCP handshake RTT."
                  : "Standard ICMP echo request probe."
              }
            />
          </Box>

          {/* Packet count & timeout */}
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
            <TextField
              label="Probe / Packet Count"
              type="number"
              size="small"
              fullWidth
              value={formCount}
              onChange={(e) => setFormCount(e.target.value)}
              inputProps={{ min: 1, max: 20 }}
              helperText="Number of packets to send (1-20, default 4)"
            />
            <TextField
              label="Timeout (seconds)"
              type="number"
              size="small"
              fullWidth
              value={formTimeout}
              onChange={(e) => setFormTimeout(e.target.value)}
              inputProps={{ min: 1, max: 60 }}
              helperText="Maximum execution timeout in seconds"
            />
          </Box>

          {/* Cron Expression & Presets */}
          <Paper elevation={0} sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B", mb: 1 }}>
              Schedule Frequency (Cron Expression)
            </Typography>

            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mb: 1.5 }}>
              {[
                { label: "Every 1 min", expr: "*/1 * * * *" },
                { label: "Every 5 mins", expr: "*/5 * * * *" },
                { label: "Every 15 mins", expr: "*/15 * * * *" },
                { label: "Every 30 mins", expr: "*/30 * * * *" },
                { label: "Hourly (0 */1 * * *)", expr: "0 */1 * * *" },
                { label: "Daily (0 0 * * *)", expr: "0 0 * * *" },
              ].map((preset) => (
                <Chip
                  key={preset.expr}
                  label={preset.label}
                  clickable
                  onClick={() => setFormCronExpr(preset.expr)}
                  color={formCronExpr === preset.expr ? "primary" : "default"}
                  variant={formCronExpr === preset.expr ? "filled" : "outlined"}
                  sx={{ fontWeight: 500, fontSize: "0.75rem" }}
                />
              ))}
            </Box>

            <TextField
              label="Cron Expression (5 or 6 fields)"
              required
              fullWidth
              size="small"
              value={formCronExpr}
              onChange={(e) => setFormCronExpr(e.target.value)}
              placeholder="e.g. 0 */1 * * * or */5 * * * *"
              sx={{ fontFamily: "monospace" }}
              helperText={`Human-readable: ${describeCronExpr(formCronExpr)}`}
            />
          </Paper>

          {/* Node Selection */}
          <Box>
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                Target Nodes ({formNodes.length} selected)
              </Typography>
              <Box sx={{ display: "flex", gap: 1 }}>
                <Button
                  size="small"
                  onClick={() => setFormNodes(availableNodes.map((n) => n.name))}
                  sx={{ textTransform: "none", fontSize: "0.75rem" }}
                >
                  Select All
                </Button>
                <Button
                  size="small"
                  onClick={() => setFormNodes([])}
                  sx={{ textTransform: "none", fontSize: "0.75rem" }}
                >
                  Clear
                </Button>
              </Box>
            </Box>

            <Box
              sx={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))",
                gap: 1.5,
                maxHeight: 180,
                overflowY: "auto",
                p: 1.5,
                bgcolor: "#F8FAFC",
                borderRadius: 2,
                border: "1px solid #E2E8F0",
              }}
            >
              {availableNodes.map((n) => {
                const isSelected = formNodes.includes(n.name);
                const isAgent = n.mode === "agent" || Boolean(n.agent_token);
                return (
                  <Paper
                    key={n.name}
                    elevation={0}
                    onClick={() => toggleNodeSelection(n.name)}
                    sx={{
                      p: 1.25,
                      borderRadius: 2,
                      border: "1px solid",
                      borderColor: isSelected ? "#4F46E5" : "#E2E8F0",
                      bgcolor: isSelected ? "#EEF2FF" : "#FFFFFF",
                      cursor: "pointer",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      transition: "all 0.15s ease",
                      "&:hover": { borderColor: "#818CF8" },
                    }}
                  >
                    <Box>
                      <Typography
                        variant="body2"
                        sx={{ fontWeight: isSelected ? 700 : 500, color: isSelected ? "#312E81" : "#1E293B" }}
                      >
                        {n.name}
                      </Typography>
                      <Typography variant="caption" sx={{ color: isAgent ? "#16A34A" : "#D97706" }}>
                        {isAgent ? "Agent Node" : "No Agent"}
                      </Typography>
                    </Box>
                    {isSelected && <CheckCircle2 size={16} color="#4F46E5" />}
                  </Paper>
                );
              })}
            </Box>
            <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.75 }}>
              * Cron tasks are executed solely by node agents. If a node does not have an active agent, the task will
              automatically execute once the agent comes online.
            </Typography>
          </Box>
        </DialogContent>

        <DialogActions sx={{ p: 2, pt: 1 }}>
          <Button onClick={() => setModalOpen(false)} disabled={saving} sx={{ color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={saving}
            startIcon={saving ? <CircularProgress size={16} color="inherit" /> : undefined}
            sx={{
              bgcolor: "#4F46E5",
              fontWeight: 600,
              textTransform: "none",
              px: 3,
              "&:hover": { bgcolor: "#4338CA" },
            }}
          >
            {saving ? "Saving..." : editingTask ? "Update Task" : "Create Task"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
