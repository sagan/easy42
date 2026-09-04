import React, { useState, useEffect, useMemo } from "react";
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
  Checkbox,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Tooltip,
  FormControl,
  Select,
  MenuItem,
} from "@mui/material";
import { Wrench, CheckCircle2, XCircle, Play, RotateCw, X, Terminal, Server, HelpCircle, Tag } from "lucide-react";
import { api } from "../../api/client";
import { Node, TaskMeta, TaskStatusResult, TaskRunResult } from "../../types/api";

interface DeviceHelperModalProps {
  open: boolean;
  onClose: () => void;
  nodes: Node[];
  initialNode?: string;
}

export const DeviceHelperModal: React.FC<DeviceHelperModalProps> = ({ open, onClose, nodes, initialNode }) => {
  const [tasks, setTasks] = useState<TaskMeta[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState<string>("");

  // Tag filter state
  const [selectedTag, setSelectedTag] = useState<string>("All");

  // Unique tags across all nodes
  const uniqueTags = useMemo(() => {
    const set = new Set<string>();
    nodes.forEach((n) => {
      n.tags?.forEach((t) => {
        const trimmed = t.trim();
        if (trimmed) set.add(trimmed);
      });
    });
    return Array.from(set).sort();
  }, [nodes]);

  // Nodes filtered by tag
  const filteredNodes = useMemo(() => {
    if (selectedTag === "All") return nodes;
    return nodes.filter((n) => n.tags && n.tags.includes(selectedTag));
  }, [nodes, selectedTag]);

  // Selected node names for check / run
  const [selectedNodes, setSelectedNodes] = useState<string[]>([]);

  // Task statuses: map[nodeName]TaskStatusResult
  const [statuses, setStatuses] = useState<Record<string, TaskStatusResult>>({});
  const [checkingNodes, setCheckingNodes] = useState<Record<string, boolean>>({});

  // Execution results: map[nodeName]TaskRunResult
  const [runResults, setRunResults] = useState<Record<string, TaskRunResult>>({});
  const [runningNodes, setRunningNodes] = useState<Record<string, boolean>>({});

  // Active terminal log tab/node
  const [activeLogNode, setActiveLogNode] = useState<string | null>(null);

  // Global error
  const [error, setError] = useState<string | null>(null);

  // Load tasks on open
  useEffect(() => {
    if (open) {
      setError(null);
      setStatuses({});
      setRunResults({});
      setCheckingNodes({});
      setRunningNodes({});
      setActiveLogNode(null);
      setSelectedTag("All");

      // Default selected nodes: initialNode if provided, otherwise all nodes
      if (initialNode && nodes.some((n) => n.name === initialNode)) {
        setSelectedNodes([initialNode]);
      } else {
        setSelectedNodes(nodes.map((n) => n.name));
      }

      loadTasks();
    }
  }, [open, initialNode, nodes]);

  const loadTasks = async () => {
    setLoadingTasks(true);
    try {
      const data = await api.getTasks();
      setTasks(data);
      if (data.length > 0 && !selectedTaskId) {
        setSelectedTaskId(data[0].id);
      }
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to fetch helper tasks");
    } finally {
      setLoadingTasks(false);
    }
  };

  const selectedTask = useMemo(() => {
    return tasks.find((t) => t.id === selectedTaskId) || tasks[0];
  }, [tasks, selectedTaskId]);

  // When selected task changes, optionally clear old statuses or keep
  useEffect(() => {
    if (selectedTaskId) {
      setStatuses({});
      setRunResults({});
      setActiveLogNode(null);
    }
  }, [selectedTaskId]);

  const handleSelectAll = (e: React.ChangeEvent<HTMLInputElement>) => {
    const filteredNames = new Set(filteredNodes.map((n) => n.name));
    if (e.target.checked) {
      setSelectedNodes((prev) => Array.from(new Set([...prev, ...filteredNames])));
    } else {
      setSelectedNodes((prev) => prev.filter((name) => !filteredNames.has(name)));
    }
  };

  const selectedFilteredCount = useMemo(() => {
    const selectedSet = new Set(selectedNodes);
    return filteredNodes.filter((n) => selectedSet.has(n.name)).length;
  }, [filteredNodes, selectedNodes]);

  const isAllFilteredSelected = filteredNodes.length > 0 && selectedFilteredCount === filteredNodes.length;
  const isSomeFilteredSelected = selectedFilteredCount > 0 && selectedFilteredCount < filteredNodes.length;

  const toggleSelectNode = (name: string) => {
    setSelectedNodes((prev) => (prev.includes(name) ? prev.filter((n) => n !== name) : [...prev, name]));
  };

  // Run status check on selected nodes (or specific node)
  const handleCheckStatus = async (targetNodes?: string[]) => {
    if (!selectedTask) return;
    const target = targetNodes || selectedNodes;
    if (target.length === 0) return;

    // Mark checking state
    const newChecking: Record<string, boolean> = { ...checkingNodes };
    target.forEach((n) => {
      newChecking[n] = true;
    });
    setCheckingNodes(newChecking);
    setError(null);

    try {
      const res = await api.checkTaskStatus(selectedTask.id, target);
      setStatuses((prev) => ({ ...prev, ...res }));
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Status check failed");
    } finally {
      setCheckingNodes((prev) => {
        const next = { ...prev };
        target.forEach((n) => delete next[n]);
        return next;
      });
    }
  };

  // Run task on selected nodes (or specific node)
  const handleRunTask = async (targetNodes?: string[]) => {
    if (!selectedTask) return;
    const target = targetNodes || selectedNodes;
    if (target.length === 0) return;

    // Mark running state
    const newRunning: Record<string, boolean> = { ...runningNodes };
    target.forEach((n) => {
      newRunning[n] = true;
    });
    setRunningNodes(newRunning);
    setError(null);

    // Default active log to first node
    if (target.length > 0) {
      setActiveLogNode(target[0]);
    }

    try {
      const res = await api.runTask(selectedTask.id, target);
      setRunResults((prev) => ({ ...prev, ...res }));

      // Automatically re-check status for updated nodes
      handleCheckStatus(target);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Task execution failed");
    } finally {
      setRunningNodes((prev) => {
        const next = { ...prev };
        target.forEach((n) => delete next[n]);
        return next;
      });
    }
  };

  const isAnyChecking = Object.values(checkingNodes).some(Boolean);
  const isAnyRunning = Object.values(runningNodes).some(Boolean);

  const renderStatusBadge = (nodeName: string) => {
    if (runningNodes[nodeName]) {
      return (
        <Chip
          size="small"
          icon={<CircularProgress size={12} color="inherit" />}
          label="Running..."
          color="primary"
          variant="outlined"
        />
      );
    }

    if (checkingNodes[nodeName]) {
      return (
        <Chip
          size="small"
          icon={<CircularProgress size={12} color="inherit" />}
          label="Checking..."
          variant="outlined"
        />
      );
    }

    const st = statuses[nodeName];
    if (!st) {
      return (
        <Chip
          size="small"
          label="Not Checked"
          variant="outlined"
          sx={{ color: "text.disabled", borderColor: "divider" }}
        />
      );
    }

    switch (st.status) {
      case "done":
        return (
          <Chip
            size="small"
            icon={<CheckCircle2 size={13} />}
            label="Already Configured"
            color="success"
            sx={{ fontWeight: 600 }}
          />
        );
      case "ready":
        return (
          <Chip
            size="small"
            icon={<RotateCw size={13} />}
            label="Needs Execution"
            color="info"
            sx={{ fontWeight: 600 }}
          />
        );
      case "incompatible":
        return (
          <Chip
            size="small"
            icon={<HelpCircle size={13} />}
            label="Not Applicable"
            sx={{ bgcolor: "action.disabledBackground", color: "text.secondary" }}
          />
        );
      case "error":
      default:
        return (
          <Chip size="small" icon={<XCircle size={13} />} label="Check Failed" color="error" sx={{ fontWeight: 600 }} />
        );
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: {
          height: "85vh",
          display: "flex",
          flexDirection: "column",
          bgcolor: "background.paper",
          backgroundImage: "none",
        },
      }}
    >
      {/* Title */}
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          borderBottom: 1,
          borderColor: "divider",
          py: 1.5,
          px: 3,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              p: 0.8,
              borderRadius: 1.5,
              bgcolor: "primary.main",
              color: "primary.contrastText",
              display: "flex",
            }}
          >
            <Wrench size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2 }}>
              Device Config Helper
            </Typography>
            <Typography variant="caption" color="text.secondary">
              One-time idempotent setup and configuration tasks via SSH/SFTP
            </Typography>
          </Box>
        </Box>
        <IconButton onClick={onClose} size="small">
          <X size={20} />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 0, display: "flex", flex: 1, overflow: "hidden" }}>
        {/* Left Sidebar: Task List */}
        <Box
          sx={{
            width: 300,
            borderRight: 1,
            borderColor: "divider",
            display: "flex",
            flexDirection: "column",
            bgcolor: "background.default",
          }}
        >
          <Box sx={{ p: 2, borderBottom: 1, borderColor: "divider" }}>
            <Typography
              variant="subtitle2"
              sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.5, color: "text.secondary" }}
            >
              Available Tasks
            </Typography>
          </Box>

          <Box sx={{ flex: 1, overflowY: "auto", p: 1.5 }}>
            {loadingTasks ? (
              <Box sx={{ display: "flex", justifyContent: "center", p: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : tasks.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
                No tasks available
              </Typography>
            ) : (
              tasks.map((task) => {
                const isSelected = selectedTask?.id === task.id;
                return (
                  <Box
                    key={task.id}
                    onClick={() => setSelectedTaskId(task.id)}
                    sx={{
                      p: 1.5,
                      mb: 1,
                      borderRadius: 1.5,
                      cursor: "pointer",
                      border: 1,
                      borderColor: isSelected ? "primary.main" : "divider",
                      bgcolor: isSelected ? "action.selected" : "background.paper",
                      transition: "all 0.15s ease",
                      "&:hover": {
                        borderColor: "primary.light",
                        bgcolor: "action.hover",
                      },
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                      <Typography variant="subtitle2" sx={{ fontWeight: isSelected ? 700 : 600 }}>
                        {task.title}
                      </Typography>
                      <Chip size="small" label={task.category} sx={{ fontSize: "0.65rem", height: 18 }} />
                    </Box>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{
                        display: "-webkit-box",
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: "vertical",
                        overflow: "hidden",
                        lineHeight: 1.3,
                      }}
                    >
                      {task.description}
                    </Typography>
                  </Box>
                );
              })
            )}
          </Box>
        </Box>

        {/* Right Main Area: Task Details, Node Selection & Status Matrix */}
        <Box sx={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          {selectedTask ? (
            <Box sx={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden", p: 2.5 }}>
              {/* Task Header */}
              <Box
                sx={{ mb: 2, p: 2, borderRadius: 2, bgcolor: "background.default", border: 1, borderColor: "divider" }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                  <Typography variant="h6" sx={{ fontWeight: 700 }}>
                    {selectedTask.title}
                  </Typography>
                  <Chip size="small" label={selectedTask.category} color="primary" variant="outlined" />
                </Box>
                <Typography variant="body2" color="text.secondary">
                  {selectedTask.description}
                </Typography>
              </Box>

              {error && (
                <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
                  {error}
                </Alert>
              )}

              {/* Action Toolbar */}
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                    Target Devices ({selectedNodes.length} / {nodes.length} selected)
                  </Typography>

                  {/* Tag Filter Selector */}
                  {uniqueTags.length > 0 && (
                    <FormControl size="small">
                      <Select
                        value={selectedTag}
                        onChange={(e) => setSelectedTag(e.target.value)}
                        displayEmpty
                        startAdornment={
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              mr: 0.75,
                              color: selectedTag === "All" ? "text.secondary" : "primary.main",
                            }}
                          >
                            <Tag size={13} />
                          </Box>
                        }
                        sx={{
                          height: 28,
                          fontSize: "0.75rem",
                          fontWeight: 600,
                          borderRadius: 1.5,
                          backgroundColor: selectedTag === "All" ? "background.paper" : "rgba(79, 70, 229, 0.08)",
                          color: selectedTag === "All" ? "text.primary" : "primary.main",
                          "& .MuiSelect-select": { py: 0.3, pr: 3, pl: 0.5 },
                          "& .MuiOutlinedInput-notchedOutline": {
                            borderColor: selectedTag === "All" ? "divider" : "primary.light",
                          },
                          "&:hover .MuiOutlinedInput-notchedOutline": {
                            borderColor: "primary.main",
                          },
                        }}
                      >
                        <MenuItem value="All" sx={{ fontSize: "0.8rem", fontWeight: 600 }}>
                          Tag: All ({nodes.length})
                        </MenuItem>
                        {uniqueTags.map((tag) => {
                          const count = nodes.filter((n) => n.tags && n.tags.includes(tag)).length;
                          return (
                            <MenuItem key={tag} value={tag} sx={{ fontSize: "0.8rem", fontWeight: 600 }}>
                              #{tag} ({count})
                            </MenuItem>
                          );
                        })}
                      </Select>
                    </FormControl>
                  )}
                </Box>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={isAnyChecking ? <CircularProgress size={14} /> : <RotateCw size={14} />}
                    disabled={isAnyChecking || isAnyRunning || selectedNodes.length === 0}
                    onClick={() => handleCheckStatus()}
                  >
                    Check Status
                  </Button>
                  <Button
                    size="small"
                    variant="contained"
                    color="primary"
                    startIcon={isAnyRunning ? <CircularProgress size={14} color="inherit" /> : <Play size={14} />}
                    disabled={isAnyChecking || isAnyRunning || selectedNodes.length === 0}
                    onClick={() => handleRunTask()}
                  >
                    Execute Task ({selectedNodes.length})
                  </Button>
                </Box>
              </Box>

              {/* Node Table */}
              <TableContainer component={Paper} variant="outlined" sx={{ flex: 1, overflowY: "auto", mb: 2 }}>
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell padding="checkbox">
                        <Checkbox
                          size="small"
                          checked={isAllFilteredSelected}
                          indeterminate={isSomeFilteredSelected}
                          onChange={handleSelectAll}
                        />
                      </TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Device</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Host / IP</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                      <TableCell sx={{ fontWeight: 700 }}>Details</TableCell>
                      <TableCell align="right" sx={{ fontWeight: 700 }}>
                        Action
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {filteredNodes.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} sx={{ textAlign: "center", py: 4, color: "text.secondary" }}>
                          No devices match tag #{selectedTag}
                        </TableCell>
                      </TableRow>
                    )}
                    {filteredNodes.map((node) => {
                      const isSelected = selectedNodes.includes(node.name);
                      const st = statuses[node.name];
                      const isChecking = checkingNodes[node.name];
                      const isRunning = runningNodes[node.name];

                      return (
                        <TableRow
                          key={node.name}
                          hover
                          selected={isSelected}
                          sx={{ "&:last-child td, &:last-child th": { border: 0 } }}
                        >
                          <TableCell padding="checkbox">
                            <Checkbox size="small" checked={isSelected} onChange={() => toggleSelectNode(node.name)} />
                          </TableCell>
                          <TableCell>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                              <Server size={14} />
                              <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                {node.name}
                              </Typography>
                              {node.tags &&
                                node.tags.map((t) => (
                                  <Chip key={t} size="small" label={t} sx={{ height: 16, fontSize: "0.65rem" }} />
                                ))}
                            </Box>
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption" sx={{ fontFamily: "monospace" }}>
                              {node.host || node.ip}
                            </Typography>
                          </TableCell>
                          <TableCell>{renderStatusBadge(node.name)}</TableCell>
                          <TableCell>
                            <Typography
                              variant="caption"
                              color={st?.status === "error" ? "error.main" : "text.secondary"}
                              sx={{
                                display: "block",
                                maxWidth: 280,
                                whiteSpace: "nowrap",
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                              }}
                              title={st?.message}
                            >
                              {st?.message || "—"}
                            </Typography>
                          </TableCell>
                          <TableCell align="right">
                            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.5 }}>
                              <Tooltip title="Check Status on this node">
                                <span>
                                  <IconButton
                                    size="small"
                                    disabled={isChecking || isRunning}
                                    onClick={() => handleCheckStatus([node.name])}
                                  >
                                    <RotateCw size={14} />
                                  </IconButton>
                                </span>
                              </Tooltip>
                              <Tooltip title="Execute Task on this node">
                                <span>
                                  <IconButton
                                    size="small"
                                    color="primary"
                                    disabled={isChecking || isRunning}
                                    onClick={() => handleRunTask([node.name])}
                                  >
                                    <Play size={14} />
                                  </IconButton>
                                </span>
                              </Tooltip>
                            </Box>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </TableContainer>

              {/* Terminal Logs Output (Collapsible if execution results exist) */}
              {Object.keys(runResults).length > 0 && (
                <Box
                  sx={{
                    borderRadius: 2,
                    border: 1,
                    borderColor: "divider",
                    bgcolor: "#0f172a",
                    color: "#f8fafc",
                    p: 1.5,
                    maxHeight: 220,
                    display: "flex",
                    flexDirection: "column",
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Terminal size={16} />
                      <Typography variant="subtitle2" sx={{ fontWeight: 700, fontFamily: "monospace" }}>
                        Execution Output
                      </Typography>
                    </Box>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      {Object.keys(runResults).map((n) => {
                        const r = runResults[n];
                        return (
                          <Chip
                            key={n}
                            size="small"
                            label={n}
                            icon={
                              r.success ? (
                                <CheckCircle2 size={12} color="#4ade80" />
                              ) : (
                                <XCircle size={12} color="#f87171" />
                              )
                            }
                            onClick={() => setActiveLogNode(n)}
                            sx={{
                              bgcolor: activeLogNode === n ? "#334155" : "#1e293b",
                              color: "#f8fafc",
                              cursor: "pointer",
                              border: activeLogNode === n ? "1px solid #38bdf8" : "none",
                              fontSize: "0.75rem",
                              height: 22,
                            }}
                          />
                        );
                      })}
                    </Box>
                  </Box>

                  <Box
                    sx={{
                      flex: 1,
                      overflowY: "auto",
                      bgcolor: "#020617",
                      p: 1.5,
                      borderRadius: 1,
                      fontFamily: "monospace",
                      fontSize: "0.8rem",
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-all",
                    }}
                  >
                    {activeLogNode && runResults[activeLogNode] ? (
                      <>
                        <Box
                          sx={{
                            color: runResults[activeLogNode].success ? "#4ade80" : "#f87171",
                            mb: 0.5,
                            fontWeight: 600,
                          }}
                        >
                          [{activeLogNode}] {runResults[activeLogNode].success ? "✓ Succeeded" : "✗ Failed"} in{" "}
                          {runResults[activeLogNode].duration_ms}ms
                        </Box>
                        {runResults[activeLogNode].output || "(no output)"}
                      </>
                    ) : (
                      <Typography variant="caption" sx={{ color: "#64748b" }}>
                        Select a node above to view stdout/stderr output
                      </Typography>
                    )}
                  </Box>
                </Box>
              )}
            </Box>
          ) : (
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "center", flex: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Select a task on the left
              </Typography>
            </Box>
          )}
        </Box>
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 1.5, borderTop: 1, borderColor: "divider" }}>
        <Button onClick={onClose} variant="outlined">
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};
