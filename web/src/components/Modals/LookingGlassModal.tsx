import React, { useState, useEffect, useMemo } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  Box,
  Typography,
  IconButton,
  Button,
  Tabs,
  Tab,
  Chip,
  Paper,
  Divider,
  CircularProgress,
  Alert,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from "@mui/material";
import {
  Compass,
  X,
  Play,
  Terminal,
  Settings2,
  CheckCircle2,
  XCircle,
  Plus,
  Edit3,
  Trash2,
  Eye,
  Server,
  Zap,
} from "lucide-react";
import { api } from "../../api/client";
import {
  Node,
  LookingGlassTask,
  LGRunRequest,
  LGRunResponse,
  LGNodeResult,
} from "../../types/api";
import { TaskParamForm } from "../LookingGlass/TaskParamForm";
import { TerminalView } from "../LookingGlass/TerminalView";
import { BirdProtocolsView } from "../LookingGlass/BirdProtocolsView";
import { BirdRouteView } from "../LookingGlass/BirdRouteView";
import { PingView } from "../LookingGlass/PingView";
import { TracerouteView } from "../LookingGlass/TracerouteView";
import { CustomTaskEditorModal } from "../LookingGlass/CustomTaskEditorModal";

interface LookingGlassModalProps {
  open: boolean;
  onClose: () => void;
  nodes: Node[];
  initialNode?: string;
  initialTask?: string;
}

export const LookingGlassModal: React.FC<LookingGlassModalProps> = ({
  open,
  onClose,
  nodes,
  initialNode,
  initialTask,
}) => {
  // Navigation tabs: 0 = Presets, 1 = Ad-Hoc, 2 = Manage Tasks
  const [activeTab, setActiveTab] = useState<number>(0);

  // Managed nodes (exclude external)
  const managedNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external);
  }, [nodes]);

  // Tag filter
  const [selectedTag, setSelectedTag] = useState<string>("All");

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

  const filteredNodes = useMemo(() => {
    if (selectedTag === "All") return managedNodes;
    return managedNodes.filter((n) => n.tags && n.tags.includes(selectedTag));
  }, [managedNodes, selectedTag]);

  // Selected node names
  const [selectedNodes, setSelectedNodes] = useState<string[]>([]);

  // Task Catalog
  const [builtinTasks, setBuiltinTasks] = useState<LookingGlassTask[]>([]);
  const [customTasks, setCustomTasks] = useState<LookingGlassTask[]>([]);
  const [, setLoadingTasks] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState<string>("ping");

  // Selected task object
  const selectedTask = useMemo(() => {
    const all = [...builtinTasks, ...customTasks];
    return all.find((t) => t.id === selectedTaskId) || all[0] || null;
  }, [builtinTasks, customTasks, selectedTaskId]);

  // Task Parameters state: map[key]value
  const [paramValues, setParamValues] = useState<Record<string, string>>({});

  // Ad-Hoc command state
  const [adHocTool, setAdHocTool] = useState<string>("ping");
  const [adHocCommand, setAdHocCommand] = useState<string>("ping -c 4 172.20.0.1");
  const [adHocParser, setAdHocParser] = useState<string>("ping");

  // Task execution state
  const [running, setRunning] = useState(false);
  const [runResponse, setRunResponse] = useState<LGRunResponse | null>(null);
  const [activeResultNode, setActiveResultNode] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<"visualizer" | "terminal">("visualizer");
  const [error, setError] = useState<string | null>(null);

  // Custom Task Editor Modal state
  const [taskEditorOpen, setTaskEditorOpen] = useState(false);
  const [taskToEdit, setTaskToEdit] = useState<LookingGlassTask | null>(null);

  // Load task templates on open
  const loadTasks = async () => {
    setLoadingTasks(true);
    try {
      const res = await api.getLookingGlassTasks();
      setBuiltinTasks(res.builtin || []);
      setCustomTasks(res.custom || []);
    } catch (err: unknown) {
      console.error("Failed to load Looking Glass tasks:", err);
    } finally {
      setLoadingTasks(false);
    }
  };

  useEffect(() => {
    if (open) {
      loadTasks();
      setError(null);
      setRunResponse(null);
      setActiveResultNode(null);

      // Set initial node
      if (initialNode && managedNodes.some((n) => n.name === initialNode)) {
        setSelectedNodes([initialNode]);
      } else if (managedNodes.length > 0 && selectedNodes.length === 0) {
        setSelectedNodes([managedNodes[0].name]);
      }

      // Set initial task
      if (initialTask) {
        setSelectedTaskId(initialTask);
      }
    }
  }, [open, initialNode, initialTask, managedNodes]);

  // Sync param defaults when selected task changes
  useEffect(() => {
    if (selectedTask && selectedTask.params) {
      const defaults: Record<string, string> = {};
      selectedTask.params.forEach((p) => {
        defaults[p.key] = p.default_value || "";
      });
      setParamValues(defaults);
    }
  }, [selectedTaskId, selectedTask]);

  // Quick Ad-Hoc presets
  const handleAdHocToolChange = (tool: string) => {
    setAdHocTool(tool);
    switch (tool) {
      case "ping":
        setAdHocCommand("ping -c 4 172.20.0.1");
        setAdHocParser("ping");
        break;
      case "traceroute":
        setAdHocCommand("traceroute -m 30 -w 2 172.20.0.1");
        setAdHocParser("traceroute");
        break;
      case "mtr":
        setAdHocCommand("mtr --report --report-cycles 5 --no-dns 172.20.0.1");
        setAdHocParser("mtr");
        break;
      case "bird_protocols":
        setAdHocCommand("birdc show protocols");
        setAdHocParser("bird_protocols");
        break;
      case "bird_route":
        setAdHocCommand("birdc show route for 172.20.0.1 all");
        setAdHocParser("bird_route");
        break;
      case "ip_route":
        setAdHocCommand("ip route get 1.1.1.1");
        setAdHocParser("raw");
        break;
      case "whois":
        setAdHocCommand("whois -h whois.dn42 AS4242420000");
        setAdHocParser("raw");
        break;
      default:
        setAdHocParser("raw");
    }
  };

  // Node selection toggles
  const handleToggleNode = (nodeName: string) => {
    if (selectedNodes.includes(nodeName)) {
      if (selectedNodes.length > 1) {
        setSelectedNodes(selectedNodes.filter((n) => n !== nodeName));
      }
    } else {
      setSelectedNodes([...selectedNodes, nodeName]);
    }
  };

  const handleSelectAllFiltered = () => {
    setSelectedNodes(filteredNodes.map((n) => n.name));
  };

  // Execute Looking Glass
  const handleRun = async () => {
    if (selectedNodes.length === 0) {
      setError("Please select at least one target node");
      return;
    }

    setRunning(true);
    setError(null);

    const req: LGRunRequest = {
      nodes: selectedNodes,
      ad_hoc: activeTab === 1,
      timeout_sec: selectedTask?.timeout_sec || 20,
    };

    if (activeTab === 1) {
      req.custom_command = adHocCommand.trim();
      req.custom_parser = adHocParser;
    } else {
      req.task_id = selectedTaskId;
      req.params = paramValues;
    }

    try {
      const res = await api.runLookingGlass(req);
      setRunResponse(res);

      // Set active result node to first executed node
      const firstNode = Object.keys(res.results || {})[0];
      if (firstNode) {
        setActiveResultNode(firstNode);
      }
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to execute Looking Glass task");
    } finally {
      setRunning(false);
    }
  };

  // Save / Delete custom task
  const handleSaveCustomTask = async (task: LookingGlassTask) => {
    await api.saveLookingGlassTask(task);
    await loadTasks();
    setSelectedTaskId(task.id);
  };

  const handleDeleteCustomTask = async (taskId: string) => {
    if (!window.confirm("Are you sure you want to delete this custom task?")) return;
    try {
      await api.deleteLookingGlassTask(taskId);
      await loadTasks();
      if (selectedTaskId === taskId) {
        setSelectedTaskId("ping");
      }
    } catch (err: unknown) {
      const e = err as Error;
      alert(`Failed to delete task: ${e.message}`);
    }
  };

  // Current node result
  const activeResult: LGNodeResult | undefined =
    runResponse && activeResultNode ? runResponse.results[activeResultNode] : undefined;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
      {/* Header */}
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          pb: 1.5,
          pt: 2,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 40,
              height: 40,
              borderRadius: 2,
              background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
              color: "#FFFFFF",
              boxShadow: "0 2px 10px rgba(79, 70, 229, 0.25)",
            }}
          >
            <Compass size={22} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.2rem", color: "#0F172A" }}>
              Looking Glass
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.78rem" }}>
              Agentless network queries, BGP routing inspection & diagnostics across nodes
            </Typography>
          </Box>
        </Box>

        <IconButton size="small" onClick={onClose}>
          <X size={20} />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3, display: "flex", flexDirection: "column", gap: 2.5 }}>
        {/* Navigation Tabs */}
        <Box sx={{ borderBottom: "1px solid #E2E8F0" }}>
          <Tabs
            value={activeTab}
            onChange={(_, val) => setActiveTab(val)}
            textColor="primary"
            indicatorColor="primary"
            sx={{ minHeight: 44 }}
          >
            <Tab
              label="Preset Tasks"
              icon={<Zap size={16} />}
              iconPosition="start"
              sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.875rem", minHeight: 44 }}
            />
            <Tab
              label="Ad-Hoc Tool"
              icon={<Terminal size={16} />}
              iconPosition="start"
              sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.875rem", minHeight: 44 }}
            />
            <Tab
              label="Manage Tasks"
              icon={<Settings2 size={16} />}
              iconPosition="start"
              sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.875rem", minHeight: 44 }}
            />
          </Tabs>
        </Box>

        {error && (
          <Alert severity="error" onClose={() => setError(null)}>
            {error}
          </Alert>
        )}

        {/* Tab 0: Preset Tasks & Tab 1: Ad-Hoc -> Configuration & Execution Controls */}
        {activeTab !== 2 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {/* 1. Target Node Selector Bar */}
            <Paper
              elevation={0}
              sx={{
                p: 2,
                borderRadius: 2,
                border: "1px solid #E2E8F0",
                backgroundColor: "#F8FAFC",
                display: "flex",
                flexDirection: "column",
                gap: 1.5,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Server size={16} color="#4F46E5" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                    Select Target Node(s)
                  </Typography>
                  <Chip
                    label={`${selectedNodes.length} Selected`}
                    size="small"
                    sx={{
                      backgroundColor: "#EEF2FF",
                      color: "#4338CA",
                      fontWeight: 600,
                      fontSize: "0.72rem",
                    }}
                  />
                </Box>

                {/* Tag Filters */}
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                  <Typography variant="caption" sx={{ color: "#64748B", mr: 0.5 }}>
                    Filter:
                  </Typography>
                  <Chip
                    label="All"
                    size="small"
                    variant={selectedTag === "All" ? "filled" : "outlined"}
                    color={selectedTag === "All" ? "primary" : "default"}
                    onClick={() => setSelectedTag("All")}
                    sx={{ fontSize: "0.72rem", height: 24, cursor: "pointer" }}
                  />
                  {uniqueTags.map((tag) => (
                    <Chip
                      key={tag}
                      label={tag}
                      size="small"
                      variant={selectedTag === tag ? "filled" : "outlined"}
                      color={selectedTag === tag ? "primary" : "default"}
                      onClick={() => setSelectedTag(tag)}
                      sx={{ fontSize: "0.72rem", height: 24, cursor: "pointer" }}
                    />
                  ))}
                  <Button
                    size="small"
                    variant="text"
                    onClick={handleSelectAllFiltered}
                    sx={{ fontSize: "0.75rem", textTransform: "none", ml: 1, py: 0 }}
                  >
                    Select All
                  </Button>
                </Box>
              </Box>

              {/* Node Chips */}
              <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 1 }}>
                {filteredNodes.map((n) => {
                  const isSelected = selectedNodes.includes(n.name);
                  return (
                    <Chip
                      key={n.name}
                      label={n.name}
                      size="small"
                      onClick={() => handleToggleNode(n.name)}
                      color={isSelected ? "primary" : "default"}
                      variant={isSelected ? "filled" : "outlined"}
                      sx={{
                        fontWeight: isSelected ? 600 : 400,
                        fontSize: "0.8rem",
                        cursor: "pointer",
                        backgroundColor: isSelected ? "#4F46E5" : "#FFFFFF",
                        "&:hover": {
                          backgroundColor: isSelected ? "#4338CA" : "#F1F5F9",
                        },
                      }}
                    />
                  );
                })}
              </Box>
            </Paper>

            {/* 2. Task / Ad-Hoc Configuration */}
            {activeTab === 0 ? (
              /* Preset Task Picker & Dynamic Parameters */
              <Paper
                elevation={0}
                sx={{
                  p: 2,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  backgroundColor: "#FFFFFF",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1.5fr 2fr" }, gap: 2, alignItems: "center" }}>
                  {/* Task Selector */}
                  <FormControl size="small" fullWidth>
                    <InputLabel id="preset-task-select">Select Task Template</InputLabel>
                    <Select
                      labelId="preset-task-select"
                      value={selectedTaskId}
                      label="Select Task Template"
                      onChange={(e) => setSelectedTaskId(e.target.value)}
                    >
                      {/* Built-ins */}
                      <MenuItem disabled sx={{ fontSize: "0.75rem", fontWeight: 700, color: "#94A3B8" }}>
                        BUILT-IN TASKS
                      </MenuItem>
                      {builtinTasks.map((t) => (
                        <MenuItem key={t.id} value={t.id}>
                          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                            <span>{t.name}</span>
                            <Chip label={t.category} size="small" sx={{ height: 20, fontSize: "0.68rem" }} />
                          </Box>
                        </MenuItem>
                      ))}

                      {/* Custom Tasks */}
                      {customTasks.length > 0 && [
                        <MenuItem key="custom_header" disabled sx={{ fontSize: "0.75rem", fontWeight: 700, color: "#94A3B8" }}>
                          CUSTOM TASKS
                        </MenuItem>,
                        ...customTasks.map((t) => (
                          <MenuItem key={t.id} value={t.id}>
                            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                              <span>{t.name}</span>
                              <Chip label={t.category || "Custom"} size="small" color="secondary" sx={{ height: 20, fontSize: "0.68rem" }} />
                            </Box>
                          </MenuItem>
                        )),
                      ]}
                    </Select>
                  </FormControl>

                  {/* Task Description */}
                  {selectedTask && (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography variant="body2" sx={{ color: "#475569", fontSize: "0.85rem" }}>
                        {selectedTask.description}
                      </Typography>
                    </Box>
                  )}
                </Box>

                {/* Parameters Form */}
                {selectedTask && selectedTask.params && selectedTask.params.length > 0 && (
                  <Box sx={{ pt: 1 }}>
                    <TaskParamForm
                      params={selectedTask.params}
                      values={paramValues}
                      onChange={(k, v) => setParamValues((prev) => ({ ...prev, [k]: v }))}
                      disabled={running}
                    />
                  </Box>
                )}
              </Paper>
            ) : (
              /* Ad-Hoc Command Picker & Direct Input */
              <Paper
                elevation={0}
                sx={{
                  p: 2,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  backgroundColor: "#FFFFFF",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
                }}
              >
                {/* Ad-Hoc Tool Presets */}
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Quick Tools:
                  </Typography>
                  {[
                    { id: "ping", label: "Ping" },
                    { id: "traceroute", label: "Traceroute" },
                    { id: "mtr", label: "MTR" },
                    { id: "bird_protocols", label: "BIRD Protocols" },
                    { id: "bird_route", label: "BIRD Route" },
                    { id: "ip_route", label: "IP Route Get" },
                    { id: "whois", label: "DN42 WHOIS" },
                  ].map((tool) => (
                    <Chip
                      key={tool.id}
                      label={tool.label}
                      size="small"
                      onClick={() => handleAdHocToolChange(tool.id)}
                      color={adHocTool === tool.id ? "primary" : "default"}
                      variant={adHocTool === tool.id ? "filled" : "outlined"}
                      sx={{ fontSize: "0.75rem", cursor: "pointer", height: 26 }}
                    />
                  ))}
                </Box>

                {/* Command Input & Visualizer Selector */}
                <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "3fr 1fr" }, gap: 2 }}>
                  <TextField
                    label="CLI Command"
                    size="small"
                    fullWidth
                    value={adHocCommand}
                    onChange={(e) => setAdHocCommand(e.target.value)}
                    placeholder="e.g. ping -c 4 172.20.0.1"
                    InputProps={{
                      sx: { fontFamily: "monospace", fontSize: "0.85rem", backgroundColor: "#F8FAFC" },
                    }}
                    helperText="Allowed prefixes: ping, traceroute, mtr, birdc, ip route, whois, dig"
                  />

                  <FormControl size="small" fullWidth>
                    <InputLabel id="adhoc-parser-label">Visualizer</InputLabel>
                    <Select
                      labelId="adhoc-parser-label"
                      value={adHocParser}
                      label="Visualizer"
                      onChange={(e) => setAdHocParser(e.target.value)}
                    >
                      <MenuItem value="raw">Raw Terminal</MenuItem>
                      <MenuItem value="ping">Ping Metrics</MenuItem>
                      <MenuItem value="traceroute">Traceroute</MenuItem>
                      <MenuItem value="mtr">MTR Report</MenuItem>
                      <MenuItem value="bird_protocols">BIRD Protocols</MenuItem>
                      <MenuItem value="bird_route">BIRD Route</MenuItem>
                    </Select>
                  </FormControl>
                </Box>
              </Paper>
            )}

            {/* Run Button Bar */}
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Targeting <strong>{selectedNodes.length}</strong> node(s) via SSH connection pool
              </Typography>

              <Button
                variant="contained"
                disabled={running || selectedNodes.length === 0}
                onClick={handleRun}
                startIcon={running ? <CircularProgress size={16} color="inherit" /> : <Play size={16} />}
                sx={{
                  px: 3,
                  py: 1,
                  fontWeight: 600,
                  textTransform: "none",
                  borderRadius: 2,
                  background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
                  boxShadow: "0 2px 8px rgba(79, 70, 229, 0.3)",
                }}
              >
                {running ? "Executing Query..." : "Run Looking Glass"}
              </Button>
            </Box>

            {/* Results Section */}
            {runResponse && (
              <Box sx={{ mt: 1, display: "flex", flexDirection: "column", gap: 1.5 }}>
                <Divider />

                {/* Node Result Tabs & View Mode Switch */}
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}>
                  {/* Node Result Selectors */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                    {Object.keys(runResponse.results).map((nodeName) => {
                      const res = runResponse.results[nodeName];
                      const isActive = activeResultNode === nodeName;
                      const isSuccess = res.exit_code === 0 && !res.error;

                      return (
                        <Chip
                          key={nodeName}
                          icon={
                            isSuccess ? (
                              <CheckCircle2 size={13} color={isActive ? "#FFFFFF" : "#10B981"} />
                            ) : (
                              <XCircle size={13} color={isActive ? "#FFFFFF" : "#EF4444"} />
                            )
                          }
                          label={`${nodeName} (${res.duration_ms}ms)`}
                          onClick={() => setActiveResultNode(nodeName)}
                          color={isActive ? "primary" : "default"}
                          variant={isActive ? "filled" : "outlined"}
                          sx={{
                            fontFamily: "monospace",
                            fontWeight: isActive ? 700 : 500,
                            cursor: "pointer",
                            fontSize: "0.78rem",
                          }}
                        />
                      );
                    })}
                  </Box>

                  {/* View Mode: Visualizer vs Raw Terminal */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Chip
                      icon={<Eye size={13} />}
                      label="Visualizer"
                      size="small"
                      color={viewMode === "visualizer" ? "primary" : "default"}
                      variant={viewMode === "visualizer" ? "filled" : "outlined"}
                      onClick={() => setViewMode("visualizer")}
                      sx={{ cursor: "pointer", fontWeight: 600, fontSize: "0.72rem" }}
                    />
                    <Chip
                      icon={<Terminal size={13} />}
                      label="Raw Terminal"
                      size="small"
                      color={viewMode === "terminal" ? "primary" : "default"}
                      variant={viewMode === "terminal" ? "filled" : "outlined"}
                      onClick={() => setViewMode("terminal")}
                      sx={{ cursor: "pointer", fontWeight: 600, fontSize: "0.72rem" }}
                    />
                  </Box>
                </Box>

                {/* Active Node Output Container */}
                {activeResult && (
                  <Box sx={{ mt: 1 }}>
                    {viewMode === "visualizer" && activeResult.parsed ? (
                      activeResult.parser === "bird_protocols" ? (
                        <BirdProtocolsView protocols={activeResult.parsed} />
                      ) : activeResult.parser === "bird_route" ? (
                        <BirdRouteView
                          routeResult={activeResult.parsed}
                          onQueryASN={(asn) => {
                            setActiveTab(1);
                            setAdHocTool("whois");
                            setAdHocCommand(`whois -h whois.dn42 ${asn}`);
                            setAdHocParser("raw");
                          }}
                        />
                      ) : activeResult.parser === "ping" ? (
                        <PingView ping={activeResult.parsed} />
                      ) : activeResult.parser === "traceroute" ? (
                        <TracerouteView traceResult={activeResult.parsed} />
                      ) : activeResult.parser === "mtr" ? (
                        <TracerouteView mtrResult={activeResult.parsed} />
                      ) : (
                        <TerminalView
                          output={activeResult.raw_output}
                          command={activeResult.command}
                          durationMs={activeResult.duration_ms}
                          exitCode={activeResult.exit_code}
                        />
                      )
                    ) : (
                      <TerminalView
                        output={activeResult.raw_output || activeResult.error || "No output returned"}
                        command={activeResult.command}
                        durationMs={activeResult.duration_ms}
                        exitCode={activeResult.exit_code}
                      />
                    )}
                  </Box>
                )}
              </Box>
            )}
          </Box>
        )}

        {/* Tab 2: Manage Tasks Catalog */}
        {activeTab === 2 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Box>
                <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#1E293B" }}>
                  Task Template Catalog
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Create and manage reusable diagnostic and routing inspection tasks
                </Typography>
              </Box>

              <Button
                variant="contained"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={() => {
                  setTaskToEdit(null);
                  setTaskEditorOpen(true);
                }}
                sx={{
                  textTransform: "none",
                  fontWeight: 600,
                  background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
                }}
              >
                New Custom Task
              </Button>
            </Box>

            {/* Custom Tasks Section */}
            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#475569" }}>
              Custom Tasks ({customTasks.length})
            </Typography>

            {customTasks.length === 0 ? (
              <Paper
                elevation={0}
                sx={{
                  p: 3,
                  textAlign: "center",
                  backgroundColor: "#F8FAFC",
                  border: "1px dashed #CBD5E1",
                  borderRadius: 2,
                }}
              >
                <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                  No custom tasks saved yet. Click "New Custom Task" above to create one.
                </Typography>
              </Paper>
            ) : (
              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                {customTasks.map((t) => (
                  <Paper
                    key={t.id}
                    elevation={0}
                    sx={{
                      p: 2,
                      borderRadius: 2,
                      border: "1px solid #E2E8F0",
                      display: "flex",
                      flexDirection: "column",
                      justifyContent: "space-between",
                      gap: 1.5,
                      "&:hover": { borderColor: "#6366F1" },
                    }}
                  >
                    <Box>
                      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                          {t.name}
                        </Typography>
                        <Chip label={t.category || "Custom"} size="small" color="secondary" sx={{ height: 20, fontSize: "0.68rem" }} />
                      </Box>
                      <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1 }}>
                        {t.description || "No description provided"}
                      </Typography>
                      <Box
                        sx={{
                          p: 1,
                          borderRadius: 1,
                          backgroundColor: "#F1F5F9",
                          fontFamily: "monospace",
                          fontSize: "0.75rem",
                          color: "#1E293B",
                        }}
                      >
                        {t.command_tmpl}
                      </Box>
                    </Box>

                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pt: 1, borderTop: "1px solid #F1F5F9" }}>
                      <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                        Visualizer: {t.parser}
                      </Typography>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                        <IconButton
                          size="small"
                          onClick={() => {
                            setTaskToEdit(t);
                            setTaskEditorOpen(true);
                          }}
                        >
                          <Edit3 size={15} color="#4F46E5" />
                        </IconButton>
                        <IconButton size="small" color="error" onClick={() => handleDeleteCustomTask(t.id)}>
                          <Trash2 size={15} />
                        </IconButton>
                      </Box>
                    </Box>
                  </Paper>
                ))}
              </Box>
            )}

            <Divider sx={{ my: 1 }} />

            {/* Built-in Tasks Section */}
            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#475569" }}>
              Standard Built-in Tasks ({builtinTasks.length})
            </Typography>

            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
              {builtinTasks.map((t) => (
                <Paper
                  key={t.id}
                  elevation={0}
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    border: "1px solid #E2E8F0",
                    backgroundColor: "#FAFBFD",
                    display: "flex",
                    flexDirection: "column",
                    justifyContent: "space-between",
                    gap: 1,
                  }}
                >
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                      <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                        {t.name}
                      </Typography>
                      <Chip label={t.category} size="small" sx={{ height: 20, fontSize: "0.68rem" }} />
                    </Box>
                    <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1 }}>
                      {t.description}
                    </Typography>
                    <Box
                      sx={{
                        p: 1,
                        borderRadius: 1,
                        backgroundColor: "#F1F5F9",
                        fontFamily: "monospace",
                        fontSize: "0.75rem",
                        color: "#475569",
                      }}
                    >
                      {t.command_tmpl}
                    </Box>
                  </Box>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pt: 1, borderTop: "1px solid #F1F5F9" }}>
                    <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                      Visualizer: {t.parser}
                    </Typography>
                    <Button
                      size="small"
                      onClick={() => {
                        setSelectedTaskId(t.id);
                        setActiveTab(0);
                      }}
                      sx={{ textTransform: "none", fontSize: "0.75rem" }}
                    >
                      Use Template
                    </Button>
                  </Box>
                </Paper>
              ))}
            </Box>
          </Box>
        )}
      </DialogContent>

      {/* Task Editor Modal */}
      <CustomTaskEditorModal
        open={taskEditorOpen}
        onClose={() => setTaskEditorOpen(false)}
        taskToEdit={taskToEdit}
        onSave={handleSaveCustomTask}
      />
    </Dialog>
  );
};
