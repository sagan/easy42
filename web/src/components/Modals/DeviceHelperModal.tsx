import React, { useState, useEffect, useMemo, useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import {
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
  TextField,
  InputAdornment,
} from "@mui/material";
import { CheckCircle2, XCircle, Play, RotateCw, Terminal, Server, HelpCircle, Tag, CheckCheck, Search, X, Code2, Plus } from "lucide-react";
import { api } from "../../api/client";
import { Node, TaskMeta, TaskStatusResult, TaskRunResult } from "../../types/api";
import { TaskSourceModal } from "./TaskSourceModal";
import { CustomTaskHelp } from "../DeviceHelper/CustomTaskHelp";

export const CUSTOM_TASK_HELP_ID = "+task";

interface DeviceHelperModalProps {
  open?: boolean;
  onClose?: () => void;
  nodes: Node[];
  initialNode?: string;
  initialTaskId?: string;
}

export const DeviceHelperModal: React.FC<DeviceHelperModalProps> = ({ nodes, initialNode, initialTaskId }) => {
  const [searchParams, setSearchParams] = useSearchParams();
  const [tasks, setTasks] = useState<TaskMeta[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState<string>(() => {
    return searchParams.get("task") || searchParams.get("taskId") || initialTaskId || "";
  });

  // Tag filter state
  const [selectedTag, setSelectedTag] = useState<string>("All");

  // Exclude external unmanaged nodes
  const managedNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external);
  }, [nodes]);

  // Unique tags across all managed nodes
  const uniqueTags = useMemo(() => {
    const set = new Set<string>();
    managedNodes.forEach((n) => {
      n.tags?.forEach((t) => {
        const trimmed = t.trim();
        if (trimmed) set.add(trimmed);
      });
    });
    return Array.from(set).sort();
  }, [managedNodes]);

  // Nodes filtered by tag
  const filteredNodes = useMemo(() => {
    if (selectedTag === "All") return managedNodes;
    return managedNodes.filter((n) => n.tags && n.tags.includes(selectedTag));
  }, [managedNodes, selectedTag]);

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

  // Track if status has been checked for current task
  const [hasCheckedStatus, setHasCheckedStatus] = useState<boolean>(false);

  // Load tasks on mount
  useEffect(() => {
    setError(null);
    setStatuses({});
    setRunResults({});
    setCheckingNodes({});
    setRunningNodes({});
    setActiveLogNode(null);
    setSelectedTag("All");
    setHasCheckedStatus(false);

    // Default selected nodes: initialNode if provided and managed, otherwise all managed nodes
    if (initialNode && managedNodes.some((n) => n.name === initialNode)) {
      setSelectedNodes([initialNode]);
    } else {
      setSelectedNodes(managedNodes.map((n) => n.name));
    }

    loadTasks();
  }, [initialNode, managedNodes]);

  const handleSelectTask = useCallback(
    (taskId: string) => {
      setSelectedTaskId(taskId);
      setSearchParams(
        (prev) => {
          if (taskId) {
            if (prev.has("taskId") && !prev.has("task")) {
              if (prev.get("taskId") === taskId) return prev;
              const next = new URLSearchParams(prev);
              next.set("taskId", taskId);
              return next;
            }
            if (prev.get("task") === taskId && !prev.has("taskId")) {
              return prev;
            }
            const next = new URLSearchParams(prev);
            next.set("task", taskId);
            next.delete("taskId");
            return next;
          } else {
            if (!prev.has("task") && !prev.has("taskId")) return prev;
            const next = new URLSearchParams(prev);
            next.delete("task");
            next.delete("taskId");
            return next;
          }
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  // Sync selected task if URL query params change (e.g. browser back/forward or external navigation)
  useEffect(() => {
    const paramTask = searchParams.get("task") || searchParams.get("taskId") || "";
    if (paramTask) {
      if (paramTask === CUSTOM_TASK_HELP_ID || paramTask === "+task") {
        if (selectedTaskId !== CUSTOM_TASK_HELP_ID) {
          setSelectedTaskId(CUSTOM_TASK_HELP_ID);
        }
      } else if (paramTask !== selectedTaskId && (tasks.length === 0 || tasks.some((t) => t.id === paramTask))) {
        setSelectedTaskId(paramTask);
      }
    } else if (tasks.length > 0 && selectedTaskId !== tasks[0].id && selectedTaskId !== CUSTOM_TASK_HELP_ID) {
      setSelectedTaskId(tasks[0].id);
    }
  }, [searchParams, tasks, selectedTaskId]);

  const loadTasks = async () => {
    setLoadingTasks(true);
    try {
      const data = await api.getTasks();
      setTasks(data);
      if (data.length > 0) {
        const urlTask = searchParams.get("task") || searchParams.get("taskId") || initialTaskId || "";
        if (urlTask === CUSTOM_TASK_HELP_ID || urlTask === "+task") {
          setSelectedTaskId(CUSTOM_TASK_HELP_ID);
          if (!searchParams.get("task") && !searchParams.get("taskId")) {
            handleSelectTask(CUSTOM_TASK_HELP_ID);
          }
        } else {
          const matched = data.find((t) => t.id === urlTask);
          if (matched) {
            setSelectedTaskId(matched.id);
            if (!searchParams.get("task") && !searchParams.get("taskId")) {
              handleSelectTask(matched.id);
            }
          } else {
            const firstTaskId = data[0].id;
            setSelectedTaskId(firstTaskId);
            handleSelectTask(firstTaskId);
          }
        }
      }
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to fetch helper tasks");
    } finally {
      setLoadingTasks(false);
    }
  };

  const isCustomTaskHelp = selectedTaskId === CUSTOM_TASK_HELP_ID;

  const selectedTask = useMemo(() => {
    if (isCustomTaskHelp) return null;
    return tasks.find((t) => t.id === selectedTaskId) || tasks[0] || null;
  }, [tasks, selectedTaskId, isCustomTaskHelp]);

  // Task search filter state and View Source modal state
  const [taskFilter, setTaskFilter] = useState<string>("");
  const [sourceModalOpen, setSourceModalOpen] = useState<boolean>(false);

  // Filter displayed tasks by name or description (or ID).
  // The current selected task should also always be displayed in the list even if the filter doesn't match it.
  const filteredTasks = useMemo(() => {
    const q = taskFilter.trim().toLowerCase();
    if (!q) return tasks;
    return tasks.filter((task) => {
      if (selectedTask && task.id === selectedTask.id) return true;
      const title = (task.title || "").toLowerCase();
      const desc = (task.description || "").toLowerCase();
      const id = (task.id || "").toLowerCase();
      return title.includes(q) || desc.includes(q) || id.includes(q);
    });
  }, [tasks, taskFilter, selectedTask]);

  const builtInTasks = useMemo(() => {
    return filteredTasks.filter((t) => !t.custom);
  }, [filteredTasks]);

  const customTasks = useMemo(() => {
    return filteredTasks.filter((t) => t.custom);
  }, [filteredTasks]);

  const matchesHelpFilter = useMemo(() => {
    const q = taskFilter.trim().toLowerCase();
    if (!q) return true;
    if (isCustomTaskHelp) return true;
    return (
      "+task".includes(q) ||
      "+ task".includes(q) ||
      "custom".includes(q) ||
      "add".includes(q) ||
      "help".includes(q) ||
      "guide".includes(q) ||
      "prompt".includes(q)
    );
  }, [taskFilter, isCustomTaskHelp]);

  // When selected task changes, optionally clear old statuses or keep
  useEffect(() => {
    if (selectedTaskId) {
      setStatuses({});
      setRunResults({});
      setActiveLogNode(null);
      setHasCheckedStatus(false);
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
      if (res && Object.keys(res).length > 0) {
        setHasCheckedStatus(true);
      }
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

  const canAutoSelect =
    hasCheckedStatus &&
    Object.keys(statuses).length > 0 &&
    !isAnyChecking &&
    !isAnyRunning;

  const needsExecutionCount = useMemo(() => {
    const targetPool = selectedTag === "All" ? managedNodes : filteredNodes;
    return targetPool.filter((n) => statuses[n.name]?.status === "ready").length;
  }, [selectedTag, managedNodes, filteredNodes, statuses]);

  // Auto-select nodes whose status is "ready" ("Needs Execution") and unselect other nodes
  const handleAutoSelect = () => {
    const targetPool = selectedTag === "All" ? managedNodes : filteredNodes;
    const readyNodeNames = targetPool
      .filter((n) => statuses[n.name]?.status === "ready")
      .map((n) => n.name);
    setSelectedNodes(readyNodeNames);
  };

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
    <Box
      sx={{
        p: { xs: 1.5, sm: 2.5, md: 3 },
        height: "100%",
        minHeight: 0,
        overflowY: "auto",
        display: "flex",
        flexDirection: "column",
      }}
    >
      {/* Main Content Area */}
      <Paper
        elevation={0}
        sx={{
          flex: 1,
          minHeight: { xs: 550, md: 600 },
          borderRadius: 3,
          border: "1px solid #E2E8F0",
          bgcolor: "#FFFFFF",
          overflow: "hidden",
          display: "flex",
          flexDirection: { xs: "column", md: "row" },
        }}
      >
        {/* Left Sidebar: Task List */}
        <Box
          sx={{
            width: { xs: "100%", md: 300 },
            maxHeight: { xs: 260, md: "none" },
            borderRight: { xs: 0, md: 1 },
            borderBottom: { xs: 1, md: 0 },
            borderColor: "divider",
            display: "flex",
            flexDirection: "column",
            bgcolor: "background.default",
            flexShrink: 0,
          }}
        >
          <Box sx={{ p: 1.5, pb: 1, borderBottom: 1, borderColor: "divider" }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
              <Typography
                variant="subtitle2"
                sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.5, color: "text.secondary", fontSize: "0.75rem" }}
              >
                Device Helper Tasks
              </Typography>
              {taskFilter.trim() && (
                <Chip
                  size="small"
                  label={`${filteredTasks.length}/${tasks.length}`}
                  sx={{ height: 18, fontSize: "0.65rem", fontWeight: 600 }}
                />
              )}
            </Box>
            <TextField
              size="small"
              fullWidth
              placeholder="Filter tasks..."
              value={taskFilter}
              onChange={(e) => setTaskFilter(e.target.value)}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start" sx={{ color: "text.secondary", mr: 0.5 }}>
                    <Search size={14} />
                  </InputAdornment>
                ),
                endAdornment: taskFilter ? (
                  <InputAdornment position="end">
                    <IconButton size="small" onClick={() => setTaskFilter("")} sx={{ p: 0.25, color: "text.secondary" }}>
                      <X size={12} />
                    </IconButton>
                  </InputAdornment>
                ) : null,
                sx: {
                  height: 32,
                  fontSize: "0.8rem",
                  bgcolor: "background.paper",
                  borderRadius: 1.5,
                  "& .MuiOutlinedInput-input": {
                    py: 0.5,
                    px: 0.5,
                  },
                },
              }}
            />
          </Box>

          <Box sx={{ flex: 1, overflowY: "auto", p: 1.5 }}>
            {loadingTasks ? (
              <Box sx={{ display: "flex", justifyContent: "center", p: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : tasks.length === 0 && !matchesHelpFilter ? (
              <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
                No tasks available
              </Typography>
            ) : filteredTasks.length === 0 && !matchesHelpFilter ? (
              <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
                No tasks match "{taskFilter}"
              </Typography>
            ) : (
              <>
                {/* Built-in Tasks Section */}
                <Box sx={{ mb: 2 }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", px: 0.5, mb: 1 }}>
                    <Typography
                      variant="caption"
                      sx={{
                        fontWeight: 700,
                        textTransform: "uppercase",
                        letterSpacing: 0.5,
                        color: "text.secondary",
                        fontSize: "0.68rem",
                      }}
                    >
                      Built-in Tasks ({builtInTasks.length})
                    </Typography>
                  </Box>
                  {builtInTasks.length === 0 ? (
                    <Typography variant="caption" sx={{ color: "text.disabled", px: 1, display: "block" }}>
                      No matching built-in tasks
                    </Typography>
                  ) : (
                    builtInTasks.map((task) => {
                      const isSelected = !isCustomTaskHelp && selectedTask?.id === task.id;
                      return (
                        <Box
                          key={task.id}
                          onClick={() => handleSelectTask(task.id)}
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

                {/* Custom Tasks Section */}
                <Box sx={{ mt: 1 }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", px: 0.5, mb: 1 }}>
                    <Typography
                      variant="caption"
                      sx={{
                        fontWeight: 700,
                        textTransform: "uppercase",
                        letterSpacing: 0.5,
                        color: "text.secondary",
                        fontSize: "0.68rem",
                      }}
                    >
                      Custom Tasks ({customTasks.length})
                    </Typography>
                    <Chip
                      size="small"
                      label="Tasks Dir"
                      sx={{
                        height: 16,
                        fontSize: "0.6rem",
                        fontWeight: 700,
                        bgcolor: "rgba(124, 58, 237, 0.08)",
                        color: "#7c3aed",
                      }}
                    />
                  </Box>

                  {customTasks.map((task) => {
                    const isSelected = !isCustomTaskHelp && selectedTask?.id === task.id;
                    return (
                      <Box
                        key={task.id}
                        onClick={() => handleSelectTask(task.id)}
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
                          <Typography variant="subtitle2" sx={{ fontWeight: isSelected ? 700 : 600, color: "#1e1b4b" }}>
                            {task.title}
                          </Typography>
                          <Chip
                            size="small"
                            label={task.category || "Custom"}
                            sx={{ fontSize: "0.65rem", height: 18, bgcolor: "rgba(124, 58, 237, 0.1)", color: "#7c3aed" }}
                          />
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
                  })}

                  {/* Placeholder "+ task" Help Item */}
                  {matchesHelpFilter && (
                    <Box
                      onClick={() => handleSelectTask(CUSTOM_TASK_HELP_ID)}
                      sx={{
                        p: 1.5,
                        mb: 1,
                        borderRadius: 1.5,
                        cursor: "pointer",
                        border: "1.5px dashed",
                        borderColor: isCustomTaskHelp ? "primary.main" : "#cbd5e1",
                        bgcolor: isCustomTaskHelp ? "action.selected" : "background.paper",
                        transition: "all 0.15s ease",
                        "&:hover": {
                          borderColor: "primary.main",
                          bgcolor: "rgba(79, 70, 229, 0.04)",
                        },
                      }}
                    >
                      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
                          <Plus size={15} color="#4f46e5" />
                          <Typography
                            variant="subtitle2"
                            sx={{ fontWeight: isCustomTaskHelp ? 800 : 700, color: "primary.main" }}
                          >
                            + Task
                          </Typography>
                        </Box>
                        <Chip
                          size="small"
                          label="Help & Guide"
                          color="primary"
                          variant="outlined"
                          sx={{ fontSize: "0.62rem", height: 18, fontWeight: 700 }}
                        />
                      </Box>
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{
                          display: "block",
                          lineHeight: 1.3,
                        }}
                      >
                        How to write &amp; add custom tasks &amp; AI prompt generator
                      </Typography>
                    </Box>
                  )}
                </Box>
              </>
            )}
          </Box>
        </Box>

        {/* Right Main Area: Task Details, Node Selection & Status Matrix */}
        <Box sx={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          {isCustomTaskHelp ? (
            <CustomTaskHelp
              onRefreshTasks={loadTasks}
              isRefreshing={loadingTasks}
            />
          ) : selectedTask ? (
            <Box sx={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden", p: 2.5 }}>
              {/* Task Header */}
              <Box
                sx={{ mb: 2, p: 2, borderRadius: 2, bgcolor: "background.default", border: 1, borderColor: "divider" }}
              >
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Typography variant="h6" sx={{ fontWeight: 700 }}>
                      {selectedTask.title}
                    </Typography>
                    <Chip size="small" label={selectedTask.category} color="primary" variant="outlined" />
                    {selectedTask.custom && (
                      <Chip
                        size="small"
                        label="Custom Task"
                        sx={{
                          fontWeight: 700,
                          fontSize: "0.68rem",
                          bgcolor: "rgba(124, 58, 237, 0.1)",
                          color: "#7c3aed",
                          border: "1px solid rgba(124, 58, 237, 0.25)",
                        }}
                      />
                    )}
                  </Box>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<Code2 size={14} />}
                    onClick={() => setSourceModalOpen(true)}
                    sx={{
                      textTransform: "none",
                      fontWeight: 600,
                      fontSize: "0.78rem",
                      bgcolor: "background.paper",
                    }}
                  >
                    View Source
                  </Button>
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
                    Target Devices ({selectedNodes.length} / {managedNodes.length} selected)
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
                          Tag: All ({managedNodes.length})
                        </MenuItem>
                        {uniqueTags.map((tag) => {
                          const count = managedNodes.filter((n) => n.tags && n.tags.includes(tag)).length;
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
                  <Tooltip
                    title={
                      !hasCheckedStatus
                        ? "Click 'Check Status' first to fetch task status"
                        : `Auto-select ${needsExecutionCount} device(s) needing execution`
                    }
                  >
                    <span>
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<CheckCheck size={14} />}
                        disabled={!canAutoSelect}
                        onClick={handleAutoSelect}
                      >
                        Auto Select
                      </Button>
                    </span>
                  </Tooltip>
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
      </Paper>

      {/* Task Source Modal */}
      <TaskSourceModal
        open={sourceModalOpen}
        onClose={() => setSourceModalOpen(false)}
        task={selectedTask || null}
      />
    </Box>
  );
};
