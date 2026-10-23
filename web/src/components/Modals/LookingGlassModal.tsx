import React, { useState, useEffect, useMemo, useRef } from "react";
import { useParams, useNavigate, useLocation, useSearchParams } from "react-router-dom";
import {
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
  Clock,
} from "lucide-react";
import { api } from "../../api/client";
import { Node, LookingGlassTask, LGRunRequest, LGRunResponse, LGNodeResult } from "../../types/api";
import { TaskParamForm } from "../LookingGlass/TaskParamForm";
import { TerminalView } from "../LookingGlass/TerminalView";
import { BirdProtocolsView } from "../LookingGlass/BirdProtocolsView";
import { BirdRouteView } from "../LookingGlass/BirdRouteView";
import { PingView } from "../LookingGlass/PingView";
import { TracerouteView } from "../LookingGlass/TracerouteView";
import { TcpPingView } from "../LookingGlass/TcpPingView";
import { CustomTaskEditorModal } from "../LookingGlass/CustomTaskEditorModal";
import { CronTasksTab } from "../LookingGlass/CronTasksTab";

export type LookingGlassTab = "presets" | "adhoc" | "manage" | "cron";

export const VALID_LG_TABS: Record<string, LookingGlassTab> = {
  presets: "presets",
  preset: "presets",
  adhoc: "adhoc",
  "ad-hoc": "adhoc",
  manage: "manage",
  "manage-tasks": "manage",
  tasks: "manage",
  cron: "cron",
  "cron-tasks": "cron",
};

export const TAB_TO_INDEX: Record<LookingGlassTab, number> = {
  presets: 0,
  adhoc: 1,
  manage: 2,
  cron: 3,
};

export const INDEX_TO_TAB: Record<number, LookingGlassTab> = {
  0: "presets",
  1: "adhoc",
  2: "manage",
  3: "cron",
};

interface LookingGlassModalProps {
  open?: boolean;
  onClose?: () => void;
  nodes: Node[];
  initialNode?: string;
  initialTask?: string;
}

export const LookingGlassModal: React.FC<LookingGlassModalProps> = ({ nodes, initialNode, initialTask }) => {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams] = useSearchParams();

  // Route-based active tab (0 = Presets, 1 = Ad-Hoc, 2 = Manage Tasks, 3 = Cron Tasks)
  const currentTab: LookingGlassTab = (tab && VALID_LG_TABS[tab]) || "presets";
  const activeTab: number = TAB_TO_INDEX[currentTab];

  // Canonicalize or redirect /looking-glass or aliases to /looking-glass/<tab>, preserving query parameters
  useEffect(() => {
    if (!tab || !VALID_LG_TABS[tab]) {
      navigate(`/looking-glass/presets${location.search}`, { replace: true });
    } else if (tab !== VALID_LG_TABS[tab]) {
      navigate(`/looking-glass/${VALID_LG_TABS[tab]}${location.search}`, { replace: true });
    }
  }, [tab, location.search, navigate]);

  const handleTabChange = (val: number) => {
    const nextTab = INDEX_TO_TAB[val] || "presets";
    const nextParams = new URLSearchParams();
    if ((nextTab === "presets" || nextTab === "adhoc") && selectedNodes.length > 0) {
      nextParams.set("node", [...selectedNodes].sort().join(","));
    }
    const searchStr = nextParams.toString();
    navigate(`/looking-glass/${nextTab}${searchStr ? `?${searchStr}` : ""}`);
  };

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

  // Selected node names (restored from URL params or default)
  const [selectedNodes, setSelectedNodes] = useState<string[]>([]);
  const hasInitializedNodes = useRef(false);
  const lastParamNodesRef = useRef<string | null>(null);

  useEffect(() => {
    if (managedNodes.length === 0) return;

    const rawParams = [
      ...searchParams.getAll("node"),
      ...searchParams.getAll("nodes"),
      ...(initialNode ? [initialNode] : []),
    ];
    let queryNodes: string[] = [];
    for (const p of rawParams) {
      if (p) {
        queryNodes.push(
          ...p
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
        );
      }
    }

    const valid = Array.from(new Set(queryNodes.filter((q) => managedNodes.some((m) => m.name === q)))).sort();

    const currentParamKey = valid.length > 0 ? `node:${valid.join(",")}` : null;

    if (!hasInitializedNodes.current) {
      hasInitializedNodes.current = true;
      lastParamNodesRef.current = currentParamKey;
      setSelectedNodes(valid);
    } else if (currentParamKey !== lastParamNodesRef.current) {
      lastParamNodesRef.current = currentParamKey;
      setSelectedNodes(valid);
    }
  }, [managedNodes, searchParams, initialNode]);

  // Synchronize selectedNodes to the current URL "node" query variable on presets & adhoc tabs
  useEffect(() => {
    if (!hasInitializedNodes.current) return;
    if (currentTab !== "presets" && currentTab !== "adhoc") return;

    const sortedNodes = [...selectedNodes].sort();
    const nodeParamValue = sortedNodes.join(",");

    const currentNodeParam = searchParams.get("node") || "";
    const hasLegacyNodes = searchParams.has("nodes");

    // Skip if URL is already in sync and there is no legacy 'nodes' parameter to clean up
    if (currentNodeParam === nodeParamValue && !hasLegacyNodes) {
      return;
    }

    const nextParams = new URLSearchParams(searchParams);
    if (sortedNodes.length > 0) {
      nextParams.set("node", nodeParamValue);
    } else {
      nextParams.delete("node");
    }
    nextParams.delete("nodes");

    lastParamNodesRef.current = nodeParamValue ? `node:${nodeParamValue}` : null;
    navigate(
      {
        pathname: location.pathname,
        search: nextParams.toString() ? `?${nextParams.toString()}` : "",
      },
      { replace: true },
    );
  }, [selectedNodes, currentTab, searchParams, location.pathname, navigate]);

  // Task Catalog
  const [builtinTasks, setBuiltinTasks] = useState<LookingGlassTask[]>([]);
  const [customTasks, setCustomTasks] = useState<LookingGlassTask[]>([]);
  const [, setLoadingTasks] = useState(false);
  const [selectedTaskId, setSelectedTaskId] = useState<string>(() => searchParams.get("task") || initialTask || "ping");

  // Selected task object
  const selectedTask = useMemo(() => {
    const all = [...builtinTasks, ...customTasks];
    return all.find((t) => t.id === selectedTaskId) || all[0] || null;
  }, [builtinTasks, customTasks, selectedTaskId]);

  // Task Parameters state: map[key]value
  const [paramValues, setParamValues] = useState<Record<string, string>>({});

  // Ad-Hoc command state (initialized from URL if present)
  const [adHocTool, setAdHocTool] = useState<string>(() => searchParams.get("tool") || "ping");
  const [adHocCommand, setAdHocCommand] = useState<string>(
    () => searchParams.get("cmd") || searchParams.get("command") || "ping -c 4 172.20.0.53",
  );
  const [adHocParser, setAdHocParser] = useState<string>(
    () => searchParams.get("parser") || searchParams.get("visualizer") || "ping",
  );

  useEffect(() => {
    if (currentTab === "adhoc") {
      const cmd = searchParams.get("cmd") || searchParams.get("command");
      const parser = searchParams.get("parser") || searchParams.get("visualizer");
      const tool = searchParams.get("tool");
      if (cmd !== null) setAdHocCommand(cmd);
      if (parser !== null) setAdHocParser(parser);
      if (tool !== null) setAdHocTool(tool);
    }
  }, [currentTab, searchParams]);

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
    loadTasks();
  }, []);

  // Sync param defaults when selected task changes or on initial load with URL parameters
  useEffect(() => {
    if (selectedTask && selectedTask.params) {
      const urlTask = searchParams.get("task") || initialTask;
      const isFromUrl = !urlTask || urlTask === selectedTask.id;
      const defaults: Record<string, string> = {};
      selectedTask.params.forEach((p) => {
        if (isFromUrl && searchParams.has(p.key)) {
          defaults[p.key] = searchParams.get(p.key)!;
        } else {
          defaults[p.key] = p.default_value || "";
        }
      });
      setParamValues(defaults);
    }
  }, [selectedTaskId, selectedTask]);

  // Quick Ad-Hoc presets
  const handleAdHocToolChange = (tool: string) => {
    setAdHocTool(tool);
    switch (tool) {
      case "ping":
        setAdHocCommand("ping -c 4 172.20.0.53");
        setAdHocParser("ping");
        break;
      case "tcp_ping":
        setAdHocCommand("tcp_ping 1.1.1.1:80 4");
        setAdHocParser("tcp_ping");
        break;
      case "traceroute":
        setAdHocCommand("traceroute -m 30 -w 2 172.20.0.53");
        setAdHocParser("traceroute");
        break;
      case "mtr":
        setAdHocCommand("mtr --report --report-cycles 5 --no-dns 172.20.0.53");
        setAdHocParser("mtr");
        break;
      case "bird_protocols":
        setAdHocCommand("birdc show protocols");
        setAdHocParser("bird_protocols");
        break;
      case "bird_route":
        setAdHocCommand("birdc show route for 172.20.0.53 all");
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
    setSelectedNodes((prev) => {
      const next = prev.includes(nodeName) ? prev.filter((n) => n !== nodeName) : [...prev, nodeName];
      return [...next].sort();
    });
  };

  const handleSelectAllFiltered = () => {
    setSelectedNodes(filteredNodes.map((n) => n.name).sort());
  };

  // Execute Looking Glass
  const handleRun = async () => {
    if (selectedNodes.length === 0) {
      setError("Please select at least one target node");
      return;
    }

    // Sync current tab form values to URL query variables ONLY on Run Looking Glass
    const newParams = new URLSearchParams();
    if (selectedNodes.length > 0) {
      newParams.set("node", [...selectedNodes].sort().join(","));
    }

    if (activeTab === 0) {
      newParams.set("task", selectedTaskId);
      if (selectedTask?.params) {
        selectedTask.params.forEach((p) => {
          const val = paramValues[p.key] !== undefined ? paramValues[p.key] : p.default_value || "";
          if (val !== "") {
            newParams.set(p.key, val);
          }
        });
      }
      lastParamNodesRef.current = newParams.has("node") ? `node:${newParams.get("node")}` : null;
      navigate({ pathname: "/looking-glass/presets", search: newParams.toString() }, { replace: true });
    } else if (activeTab === 1) {
      if (adHocTool) {
        newParams.set("tool", adHocTool);
      }
      if (adHocCommand.trim()) {
        newParams.set("cmd", adHocCommand.trim());
      }
      if (adHocParser) {
        newParams.set("parser", adHocParser);
      }
      lastParamNodesRef.current = newParams.has("node") ? `node:${newParams.get("node")}` : null;
      navigate({ pathname: "/looking-glass/adhoc", search: newParams.toString() }, { replace: true });
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
          borderRadius: 3,
          border: "1px solid #E2E8F0",
          bgcolor: "#FFFFFF",
          p: { xs: 2, sm: 3 },
          display: "flex",
          flexDirection: "column",
          gap: 2.5,
        }}
      >
        {/* Navigation Tabs */}
        <Box sx={{ borderBottom: "1px solid #E2E8F0" }}>
          <Tabs
            value={activeTab}
            onChange={(_, val) => handleTabChange(val)}
            variant="scrollable"
            scrollButtons="auto"
            allowScrollButtonsMobile
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
            <Tab
              label="Cron Tasks"
              icon={<Clock size={16} />}
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
        {activeTab < 2 && (
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
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  flexWrap: "wrap",
                  gap: 1,
                }}
              >
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
                  <span>
                    <Button
                      size="small"
                      variant="text"
                      onClick={handleSelectAllFiltered}
                      sx={{ fontSize: "0.75rem", textTransform: "none", ml: 1, p: 0 }}
                    >
                      Select All
                    </Button>
                    <Button
                      size="small"
                      variant="text"
                      onClick={() => setSelectedNodes([])}
                      sx={{ fontSize: "0.75rem", textTransform: "none", ml: 1, p: 0 }}
                    >
                      Select None
                    </Button>
                  </span>
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
                      variant={"filled"}
                      sx={{
                        fontWeight: 400,
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
                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: { xs: "1fr", sm: "1.5fr 2fr" },
                    gap: 2,
                    alignItems: "center",
                  }}
                >
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
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "space-between",
                              width: "100%",
                            }}
                          >
                            <span>{t.name}</span>
                            <Chip label={t.category} size="small" sx={{ height: 20, fontSize: "0.68rem" }} />
                          </Box>
                        </MenuItem>
                      ))}

                      {/* Custom Tasks */}
                      {customTasks.length > 0 && [
                        <MenuItem
                          key="custom_header"
                          disabled
                          sx={{ fontSize: "0.75rem", fontWeight: 700, color: "#94A3B8" }}
                        >
                          CUSTOM TASKS
                        </MenuItem>,
                        ...customTasks.map((t) => (
                          <MenuItem key={t.id} value={t.id}>
                            <Box
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "space-between",
                                width: "100%",
                              }}
                            >
                              <span>{t.name}</span>
                              <Chip
                                label={t.category || "Custom"}
                                size="small"
                                color="secondary"
                                sx={{ height: 20, fontSize: "0.68rem" }}
                              />
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
                    { id: "tcp_ping", label: "TCP Ping" },
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
                    placeholder="e.g. ping -c 4 172.20.0.53"
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
                      <MenuItem value="tcp_ping">TCP Ping Metrics</MenuItem>
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
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    flexWrap: "wrap",
                    gap: 1,
                  }}
                >
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
                            setAdHocTool("whois");
                            setAdHocCommand(`whois -h whois.dn42 ${asn}`);
                            setAdHocParser("raw");
                            const nodeSearch =
                              selectedNodes.length > 0
                                ? `?node=${encodeURIComponent([...selectedNodes].sort().join(","))}`
                                : "";
                            navigate(`/looking-glass/adhoc${nodeSearch}`);
                          }}
                        />
                      ) : activeResult.parser === "ping" ? (
                        <PingView ping={activeResult.parsed} />
                      ) : activeResult.parser === "tcp_ping" ? (
                        <TcpPingView tcpPing={activeResult.parsed} />
                      ) : activeResult.parser === "traceroute" ? (
                        <TracerouteView traceResult={activeResult.parsed} nodes={nodes} />
                      ) : activeResult.parser === "mtr" ? (
                        <TracerouteView mtrResult={activeResult.parsed} nodes={nodes} />
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
                        <Chip
                          label={t.category || "Custom"}
                          size="small"
                          color="secondary"
                          sx={{ height: 20, fontSize: "0.68rem" }}
                        />
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

                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        pt: 1,
                        borderTop: "1px solid #F1F5F9",
                      }}
                    >
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
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      pt: 1,
                      borderTop: "1px solid #F1F5F9",
                    }}
                  >
                    <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                      Visualizer: {t.parser}
                    </Typography>
                    <Button
                      size="small"
                      onClick={() => {
                        setSelectedTaskId(t.id);
                        navigate("/looking-glass/presets");
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

        {/* Tab 3: Cron Tasks */}
        {activeTab === 3 && (
          <CronTasksTab
            nodes={managedNodes}
            onExecuteNow={(task) => {
              setSelectedTaskId(task.task_type === "tcp_ping" ? "tcp_ping" : "ping");
              setSelectedNodes([...task.nodes].sort());
              setParamValues(task.params || {});
              navigate("/looking-glass/presets");
            }}
          />
        )}
      </Paper>

      {/* Task Editor Modal */}
      <CustomTaskEditorModal
        open={taskEditorOpen}
        onClose={() => setTaskEditorOpen(false)}
        taskToEdit={taskToEdit}
        onSave={handleSaveCustomTask}
      />
    </Box>
  );
};
