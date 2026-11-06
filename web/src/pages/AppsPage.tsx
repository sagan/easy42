import React, { useState, useEffect, useMemo } from "react";
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
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Switch,
  FormControlLabel,
  Tabs,
  Tab,
  CircularProgress,
  Alert,
  Tooltip,
  Card,
  CardContent,
  Grid,
  Divider,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  TableContainer,
  Radio,
  RadioGroup,
  Checkbox,
  InputAdornment,
} from "@mui/material";
import {
  Layers,
  Plus,
  Trash2,
  Edit,
  Square,
  RefreshCw,
  Box as BoxIcon,
  Cpu,
  FileCode,
  FileText,
  FileEdit,
  Sliders,
  CheckCircle2,
  Search,
  Eye,
  EyeOff,
  FilePlus,
  X,
  Database,
  Lock,
  Copy,
  Check,
  Code2,
  Info,
  Globe,
  Tag,
  Wand2,
  Terminal,
} from "lucide-react";
import { api } from "../api/client";
import { useMesh } from "../context/MeshContext";
import { Link as RouterLink } from "react-router-dom";
import type { App, AppVariable, TranspilePreviewResponse } from "../types/api";
import { dockerRunToCompose, cmdlineToSystemd, ensureSystemdEnvironmentFile } from "../utils/appGenerators";
import { RenameAppModal } from "../components/Modals/RenameAppModal";

const DOCKER_SAMPLE = `version: '3.8'
services:
  web:
    image: nginx:alpine
    restart: always
    ports:
      - "\${PORT:-80}:80"
    environment:
      - DOMAIN=\${DOMAIN}
      - NODE_NAME=\${NODE_NAME}
      - NODE_IP=\${NODE_IP}
    volumes:
      - ./config.json:/etc/nginx/config.json:ro
`;

const SYSTEMD_SAMPLE = `[Unit]
Description=Sample Native Service on \${NODE_NAME} (\${NODE_IP})
After=network.target

[Service]
Type=simple
User=nobody
WorkingDirectory=/var/log
EnvironmentFile=/etc/systemd/system/ezapp-sample.env
ExecStart=/usr/bin/python3 -m http.server \${PORT:-8080}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`;

const INTERNAL_NODE_VARS = [
  { key: "NODE_NAME", label: "${NODE_NAME}", tooltip: "Node name (node.name)" },
  { key: "NODE_INTERFACE", label: "${NODE_INTERFACE}", tooltip: "Node interface (node.interface)" },
  { key: "NODE_IP", label: "${NODE_IP}", tooltip: "Node IPv4 stripped CIDR (node.ip)" },
  { key: "NODE_IP6", label: "${NODE_IP6}", tooltip: "Node IPv6 stripped CIDR (node.ip6)" },
  {
    key: "NODE_EXTERNAL_IP",
    label: "${NODE_EXTERNAL_IP}",
    tooltip: "Node external IPv4 (node.external_ip, when different from NODE_IP)",
  },
  {
    key: "NODE_EXTERNAL_IP6",
    label: "${NODE_EXTERNAL_IP6}",
    tooltip: "Node external IPv6 (node.external_ip6, when different from NODE_IP6)",
  },
  {
    key: "HAS_CUSTOM_SCRIPT",
    label: "${HAS_CUSTOM_SCRIPT}",
    tooltip: "'1' if custom network script configured, otherwise '0'",
  },
  {
    key: "NODE_TAGS",
    label: "${NODE_TAGS}",
    tooltip: "Comma-separated, alphabetical list of node tags",
  },
  {
    key: "NODE_TAG_*",
    label: "${NODE_TAG_*}",
    tooltip: "Tag flag env set to '1' for each tag (e.g. ${NODE_TAG_VPS})",
  },
  {
    key: "NODE_DATA_*",
    label: "${NODE_DATA_*}",
    tooltip: "Structured JSON fields from node Note (up to 1 layer hierarchy)",
  },
];

export const AppsPage: React.FC = () => {
  const { nodes, setStateToast } = useMesh();

  // Top-level Navigation: "apps" or "variables"
  const [activeTab, setActiveTab] = useState<"apps" | "variables">("apps");

  // Apps State
  const [apps, setApps] = useState<App[]>([]);
  const [loadingApps, setLoadingApps] = useState(true);
  const [appsError, setAppsError] = useState<string | null>(null);
  const [togglingAppIds, setTogglingAppIds] = useState<Record<string, boolean>>({});

  // Variables State
  const [variables, setVariables] = useState<AppVariable[]>([]);
  const [loadingVars, setLoadingVars] = useState(true);
  const [varsError, setVarsError] = useState<string | null>(null);
  const [revealedSecrets, setRevealedSecrets] = useState<Record<string, boolean>>({});
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  // Apps Filters & Search
  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<string>("all");
  const [nodeFilter, setNodeFilter] = useState<string>("all");

  // Variables Search
  const [varSearch, setVarSearch] = useState("");

  // App Modal State
  const [modalOpen, setModalOpen] = useState(false);
  const [editingApp, setEditingApp] = useState<App | null>(null);
  const [modalTab, setModalTab] = useState<"general" | "definition" | "env" | "files" | "preview" | "transpile">(
    "general",
  );

  // Rename App Modal State
  const [renameModalOpen, setRenameModalOpen] = useState(false);
  const [appToRename, setAppToRename] = useState<App | null>(null);

  const handleOpenRenameApp = (app: App) => {
    setAppToRename(app);
    setRenameModalOpen(true);
  };

  const [duplicatingAppId, setDuplicatingAppId] = useState<string | null>(null);

  // App Form Fields
  const [formName, setFormName] = useState("");
  const [formDesc, setFormDesc] = useState("");
  const [formUrl, setFormUrl] = useState("");
  const [formType, setFormType] = useState<"docker" | "systemd">("docker");
  const [formMain, setFormMain] = useState(DOCKER_SAMPLE);
  const [formEnv, setFormEnv] = useState("PORT=80\nDOMAIN=\${DOMAIN:-localhost}\n");
  const [formTargetType, setFormTargetType] = useState<"manual" | "all" | "tag">("manual");
  const [formTargetTag, setFormTargetTag] = useState<string>("");
  const [formNodes, setFormNodes] = useState<string[]>([]);
  const [formPublished, setFormPublished] = useState(true);
  const [formEnabled, setFormEnabled] = useState(true);
  const [togglingPublishedAppIds, setTogglingPublishedAppIds] = useState<Record<string, boolean>>({});
  const [formFiles, setFormFiles] = useState<{ name: string; content: string }[]>([]);

  // Transpile preview state
  const [targetInit, setTargetInit] = useState<"procd" | "openrc">("procd");
  const [previewing, setPreviewing] = useState(false);
  const [previewResult, setPreviewResult] = useState<TranspilePreviewResponse | null>(null);

  // Interpolation preview state in modal
  const [previewNodeName, setPreviewNodeName] = useState<string>("");
  const [interpolatedPreview, setInterpolatedPreview] = useState<string>("");
  const [loadingInterpolate, setLoadingInterpolate] = useState(false);

  // Generator states inside app modal
  const [showGenerator, setShowGenerator] = useState(false);
  const [dockerRunCmd, setDockerRunCmd] = useState("");
  const [dockerRunError, setDockerRunError] = useState<string | null>(null);

  const [systemdCmd, setSystemdCmd] = useState("");
  const [systemdDesc, setSystemdDesc] = useState("");
  const [systemdUser, setSystemdUser] = useState("nobody");
  const [systemdWorkingDir, setSystemdWorkingDir] = useState("/var/log");
  const [systemdError, setSystemdError] = useState<string | null>(null);

  // Variable Modal State
  const [varModalOpen, setVarModalOpen] = useState(false);
  const [varKey, setVarKey] = useState("");
  const [varVal, setVarVal] = useState("");
  const [varDesc, setVarDesc] = useState("");
  const [varIsSecret, setVarIsSecret] = useState(false);
  const [savingVar, setSavingVar] = useState(false);
  const [deleteVarKey, setDeleteVarKey] = useState<string | null>(null);

  // Saving / Delete states
  const [savingApp, setSavingApp] = useState(false);
  const [deleteApp, setDeleteApp] = useState<App | null>(null);

  const managedNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external);
  }, [nodes]);

  const availableTags = useMemo(() => {
    const s = new Set<string>();
    nodes.forEach((n) => {
      n.tags?.forEach((t) => {
        if (t.trim()) s.add(t.trim());
      });
    });
    return Array.from(s).sort();
  }, [nodes]);

  const loadApps = async () => {
    try {
      setLoadingApps(true);
      setAppsError(null);
      const data = await api.getApps();
      setApps(data || []);
    } catch (e: any) {
      setAppsError(e.message || "Failed to load applications");
    } finally {
      setLoadingApps(false);
    }
  };

  const loadVariables = async () => {
    try {
      setLoadingVars(true);
      setVarsError(null);
      const data = await api.getAppVariables();
      setVariables(data || []);
    } catch (e: any) {
      setVarsError(e.message || "Failed to load variables");
    } finally {
      setLoadingVars(false);
    }
  };

  useEffect(() => {
    loadApps();
    loadVariables();
  }, []);

  // Set default node for preview
  useEffect(() => {
    if (formTargetType === "manual" && formNodes.length > 0 && !previewNodeName) {
      setPreviewNodeName(formNodes[0]);
    } else if (formTargetType === "tag" && formTargetTag && !previewNodeName) {
      const match = managedNodes.find((n) => n.tags?.some((t) => t.toLowerCase() === formTargetTag.toLowerCase()));
      if (match) setPreviewNodeName(match.name);
      else if (managedNodes.length > 0) setPreviewNodeName(managedNodes[0].name);
    } else if (managedNodes.length > 0 && !previewNodeName) {
      setPreviewNodeName(managedNodes[0].name);
    }
  }, [formTargetType, formTargetTag, formNodes, managedNodes, previewNodeName]);

  const openCreateAppModal = () => {
    setEditingApp(null);
    setFormName("");
    setFormDesc("");
    setFormUrl("");
    setFormType("docker");
    setFormMain(DOCKER_SAMPLE);
    setFormEnv("PORT=8080\nIP=${NODE_IP}\n");
    setFormTargetType("manual");
    setFormTargetTag("");
    setFormNodes([]);
    setFormPublished(true);
    setFormEnabled(true);
    setFormFiles([]);
    setPreviewResult(null);
    setShowGenerator(false);
    setDockerRunCmd("");
    setDockerRunError(null);
    setSystemdCmd("");
    setSystemdDesc("");
    setSystemdUser("nobody");
    setSystemdWorkingDir("/var/log");
    setSystemdError(null);
    setModalTab("general");
    setModalOpen(true);
  };

  const openEditAppModal = (app: App) => {
    setEditingApp(app);
    setFormName(app.name);
    setFormDesc(app.description || "");
    setFormUrl(app.url || "");
    setFormType(app.app_type);
    setFormMain(app.main_content);
    setFormEnv(app.env_content || "");
    setFormTargetType(app.target_type || "manual");
    setFormTargetTag(app.target_tag || "");
    setFormNodes(app.nodes || []);
    setFormPublished(app.published ?? false);
    setFormEnabled(app.enabled);
    const filesList = Object.entries(app.files || {}).map(([name, content]) => ({ name, content }));
    setFormFiles(filesList);
    setPreviewResult(null);
    setShowGenerator(false);
    setDockerRunCmd("");
    setDockerRunError(null);
    setSystemdCmd("");
    setSystemdDesc("");
    setSystemdUser("nobody");
    setSystemdWorkingDir("/var/log");
    setSystemdError(null);
    setModalTab("general");
    setModalOpen(true);
  };

  const handleTypeChange = (newType: "docker" | "systemd") => {
    setFormType(newType);
    if (!editingApp) {
      setFormMain(newType === "docker" ? DOCKER_SAMPLE : SYSTEMD_SAMPLE);
    }
  };

  const handleSaveApp = async () => {
    if (!formName.trim()) {
      alert("Application name cannot be empty");
      return;
    }
    if (!/^[a-zA-Z0-9_-]{1,64}$/.test(formName.trim())) {
      alert("App name must only contain letters, numbers, dashes and underscores (max 64 characters)");
      return;
    }

    try {
      setSavingApp(true);
      const filesMap: Record<string, string> = {};
      for (const f of formFiles) {
        if (f.name.trim()) {
          filesMap[f.name.trim()] = f.content;
        }
      }

      if (formTargetType === "tag" && !formTargetTag.trim()) {
        alert("Please select or enter a target tag");
        setSavingApp(false);
        return;
      }

      const resolvedMainContent =
        formType === "systemd" ? ensureSystemdEnvironmentFile(formName.trim(), formMain) : formMain;

      const payload: Partial<App> = {
        name: formName.trim(),
        description: formDesc.trim(),
        url: formUrl.trim(),
        app_type: formType,
        main_content: resolvedMainContent,
        env_content: formEnv,
        files: filesMap,
        target_type: formTargetType,
        target_tag: formTargetType === "tag" ? formTargetTag.trim() : "",
        nodes: formTargetType === "manual" ? formNodes : [],
        published: formPublished,
        enabled: formEnabled,
      };

      if (editingApp) {
        const updated = await api.updateApp(editingApp.id, payload);
        setApps((prev) => prev.map((a) => (a.id === editingApp.id ? updated : a)));
        setStateToast({
          message: `App "${updated.name}" updated successfully.`,
          severity: "success",
        });
      } else {
        const created = await api.createApp(payload);
        setApps((prev) => [...prev, created]);
        setStateToast({
          message: `App "${created.name}" created successfully.`,
          severity: "success",
        });
      }

      setModalOpen(false);
    } catch (e: any) {
      alert(e.message || "Failed to save application");
    } finally {
      setSavingApp(false);
    }
  };

  const handleDeleteApp = async (id: string) => {
    const targetApp = apps.find((a) => a.id === id);
    const appName = targetApp ? targetApp.name : id;
    try {
      await api.deleteApp(id);
      setDeleteApp(null);
      setApps((prev) => prev.filter((a) => a.id !== id));
      setStateToast({
        message: `App "${appName}" deleted successfully.`,
        severity: "success",
      });
    } catch (e: any) {
      setStateToast({
        message: e.message || "Failed to delete app",
        severity: "error",
      });
    }
  };

  const handleTogglePublished = async (appOrId: App | string) => {
    const targetApp = typeof appOrId === "string" ? apps.find((a) => a.id === appOrId) : appOrId;
    if (!targetApp) return;

    const nextState = !targetApp.published;
    const actionLabel = nextState ? "published" : "set to draft";

    // Optimistically toggle state for this app only - no layout shift or full list re-render
    setApps((prev) => prev.map((a) => (a.id === targetApp.id ? { ...a, published: nextState } : a)));
    setTogglingPublishedAppIds((prev) => ({ ...prev, [targetApp.id]: true }));

    try {
      const updated = await api.toggleAppPublished(targetApp.id);
      setApps((prev) => prev.map((a) => (a.id === targetApp.id ? updated : a)));
      setStateToast({
        message: `App "${targetApp.name}" ${actionLabel} successfully.`,
        severity: "success",
      });
    } catch (e: any) {
      // Revert optimistic change on failure
      setApps((prev) => prev.map((a) => (a.id === targetApp.id ? { ...a, published: targetApp.published } : a)));
      setStateToast({
        message: e.message || `Failed to update published status for app "${targetApp.name}"`,
        severity: "error",
      });
    } finally {
      setTogglingPublishedAppIds((prev) => {
        const next = { ...prev };
        delete next[targetApp.id];
        return next;
      });
    }
  };

  const handleToggleApp = async (appOrId: App | string) => {
    const targetApp = typeof appOrId === "string" ? apps.find((a) => a.id === appOrId) : appOrId;
    if (!targetApp) return;

    const nextState = !targetApp.enabled;
    const actionLabel = nextState ? "enabled" : "disabled";

    // Optimistically toggle state for this app only - no layout shift or full list re-render
    setApps((prev) => prev.map((a) => (a.id === targetApp.id ? { ...a, enabled: nextState } : a)));
    setTogglingAppIds((prev) => ({ ...prev, [targetApp.id]: true }));

    try {
      const updated = await api.toggleApp(targetApp.id);
      setApps((prev) => prev.map((a) => (a.id === targetApp.id ? updated : a)));
      setStateToast({
        message: `App "${targetApp.name}" ${actionLabel} successfully.`,
        severity: "success",
      });
    } catch (e: any) {
      // Revert optimistic change on failure
      setApps((prev) => prev.map((a) => (a.id === targetApp.id ? { ...a, enabled: targetApp.enabled } : a)));
      setStateToast({
        message: e.message || `Failed to ${nextState ? "enable" : "disable"} app "${targetApp.name}"`,
        severity: "error",
      });
    } finally {
      setTogglingAppIds((prev) => {
        const next = { ...prev };
        delete next[targetApp.id];
        return next;
      });
    }
  };

  const handleDuplicateApp = async (app: App) => {
    try {
      setDuplicatingAppId(app.id);
      const duplicated = await api.duplicateApp(app.id);
      setApps((prev) => [...prev, duplicated]);
      setStateToast({
        message: `App "${app.name}" duplicated as "${duplicated.name}" (${duplicated.id}).`,
        severity: "success",
      });
    } catch (e: any) {
      setStateToast({
        message: e.message || `Failed to duplicate app "${app.name}"`,
        severity: "error",
      });
    } finally {
      setDuplicatingAppId(null);
    }
  };

  const handleTranspilePreview = async () => {
    try {
      setPreviewing(true);
      const res = await api.previewTranspile({
        app_name: formName.trim() || "sample",
        target_init: targetInit,
        main_content: formMain,
        env_content: formEnv,
      });
      setPreviewResult(res);
    } catch (e: any) {
      setPreviewResult({ script: "", error: e.message || "Preview failed" });
    } finally {
      setPreviewing(false);
    }
  };

  const handleInterpolatePreview = async (nodeName: string) => {
    try {
      setLoadingInterpolate(true);
      const res = await api.previewInterpolate({
        content: `--- [compose / service manifest] ---\n${formMain}\n\n--- [.env] ---\n${formEnv}`,
        node_name: nodeName,
        app_name: formName.trim() || "sample",
      });
      setInterpolatedPreview(res.rendered);
    } catch (e: any) {
      setInterpolatedPreview(`Error previewing interpolation: ${e.message}`);
    } finally {
      setLoadingInterpolate(false);
    }
  };

  // Variable Actions
  const openCreateVarModal = () => {
    setVarKey("");
    setVarVal("");
    setVarDesc("");
    setVarIsSecret(false);
    setVarModalOpen(true);
  };

  const openEditVarModal = (v: AppVariable) => {
    setVarKey(v.key);
    setVarVal(v.value);
    setVarDesc(v.description || "");
    setVarIsSecret(!!v.is_secret);
    setVarModalOpen(true);
  };

  const handleSaveVariable = async () => {
    const cleanKey = varKey.trim().toUpperCase();
    if (!cleanKey) {
      alert("Variable key cannot be empty");
      return;
    }
    if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(cleanKey)) {
      alert("Variable key must be a valid identifier (e.g. DOMAIN, API_PORT, DB_PASS)");
      return;
    }

    try {
      setSavingVar(true);
      await api.saveAppVariable({
        key: cleanKey,
        value: varVal,
        description: varDesc.trim(),
        is_secret: varIsSecret,
      });
      setVarModalOpen(false);
      await loadVariables();
    } catch (e: any) {
      alert(e.message || "Failed to save variable");
    } finally {
      setSavingVar(false);
    }
  };

  const handleDeleteVariable = async (key: string) => {
    try {
      await api.deleteAppVariable(key);
      setDeleteVarKey(null);
      await loadVariables();
    } catch (e: any) {
      alert(e.message || "Failed to delete variable");
    }
  };

  const copyText = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(id);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const filteredApps = useMemo(() => {
    return apps.filter((a) => {
      if (search) {
        const q = search.toLowerCase();
        const matchName = a.name.toLowerCase().includes(q);
        const matchDesc = a.description?.toLowerCase().includes(q);
        if (!matchName && !matchDesc) return false;
      }
      if (typeFilter !== "all" && a.app_type !== typeFilter) {
        return false;
      }
      if (nodeFilter !== "all") {
        const matchesNode = (() => {
          const targetType = a.target_type || "manual";
          if (targetType === "all") return true;
          if (targetType === "tag") {
            const nodeObj = nodes.find((n) => n.name === nodeFilter);
            return nodeObj?.tags?.some((t) => t.toLowerCase() === (a.target_tag || "").toLowerCase()) ?? false;
          }
          return a.nodes?.includes(nodeFilter) ?? false;
        })();
        if (!matchesNode) return false;
      }
      return true;
    });
  }, [apps, search, typeFilter, nodeFilter]);

  const filteredVars = useMemo(() => {
    return variables.filter((v) => {
      if (!varSearch) return true;
      const q = varSearch.toLowerCase();
      return v.key.toLowerCase().includes(q) || v.description?.toLowerCase().includes(q);
    });
  }, [variables, varSearch]);

  const dockerCount = apps.filter((a) => a.app_type === "docker").length;
  const systemdCount = apps.filter((a) => a.app_type === "systemd").length;
  const enabledCount = apps.filter((a) => a.enabled).length;

  return (
    <Box sx={{ width: "100%", px: { xs: 2, sm: 3, md: 4 }, py: 3, boxSizing: "border-box" }}>
      {/* Top Header & Breadcrumbs Bar */}
      <Box
        sx={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          mb: 3,
          flexWrap: "wrap",
          gap: 2,
        }}
      >
        <Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Box
              sx={{
                width: 42,
                height: 42,
                borderRadius: "10px",
                bgcolor: "rgba(79, 70, 229, 0.1)",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Layers size={24} />
            </Box>
            <Box>
              <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", letterSpacing: "-0.02em" }}>
                Workloads & Variables
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", fontWeight: 500 }}>
                Manage Docker Compose, Native Systemd services, and cluster-wide global variables
              </Typography>
            </Box>
          </Box>
        </Box>

        {/* Global Action Buttons */}
        <Box sx={{ display: "flex", gap: 1.5, alignItems: "center" }}>
          <Button
            variant="outlined"
            startIcon={<RefreshCw size={15} />}
            onClick={() => {
              loadApps();
              loadVariables();
            }}
            disabled={loadingApps || loadingVars}
            sx={{
              borderColor: "#CBD5E1",
              color: "#475569",
              textTransform: "none",
              fontWeight: 600,
              fontSize: "0.85rem",
              bgcolor: "#FFFFFF",
              "&:hover": { borderColor: "#94A3B8", bgcolor: "#F8FAFC" },
            }}
          >
            Refresh
          </Button>

          {activeTab === "apps" ? (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              onClick={openCreateAppModal}
              sx={{
                bgcolor: "#4F46E5",
                color: "#FFFFFF",
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.85rem",
                boxShadow: "0 4px 12px rgba(79, 70, 229, 0.25)",
                "&:hover": { bgcolor: "#4338CA" },
              }}
            >
              Create App
            </Button>
          ) : (
            <Button
              variant="contained"
              startIcon={<Plus size={16} />}
              onClick={openCreateVarModal}
              sx={{
                bgcolor: "#4F46E5",
                color: "#FFFFFF",
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.85rem",
                boxShadow: "0 4px 12px rgba(79, 70, 229, 0.25)",
                "&:hover": { bgcolor: "#4338CA" },
              }}
            >
              Add Variable
            </Button>
          )}
        </Box>
      </Box>

      {/* Main Navigation Segmented Tabs */}
      <Paper
        elevation={0}
        sx={{
          borderRadius: "12px",
          border: "1px solid #E2E8F0",
          bgcolor: "#FFFFFF",
          mb: 3,
          px: 2,
        }}
      >
        <Tabs
          value={activeTab}
          onChange={(_, val) => setActiveTab(val)}
          sx={{
            "& .MuiTab-root": {
              textTransform: "none",
              fontWeight: 700,
              fontSize: "0.9rem",
              minHeight: 52,
              px: 3,
            },
            "& .Mui-selected": { color: "#4F46E5" },
            "& .MuiTabs-indicator": { bgcolor: "#4F46E5", height: 3 },
          }}
        >
          <Tab
            value="apps"
            icon={<Layers size={18} />}
            iconPosition="start"
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <span>Workload Apps</span>
                <Chip size="small" label={apps.length} sx={{ height: 20, fontSize: "0.72rem", fontWeight: 700 }} />
              </Box>
            }
          />
          <Tab
            value="variables"
            icon={<Database size={18} />}
            iconPosition="start"
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <span>Global Variables</span>
                <Chip
                  size="small"
                  label={variables.length}
                  sx={{
                    height: 20,
                    fontSize: "0.72rem",
                    fontWeight: 700,
                    bgcolor: "#EEF2FF",
                    color: "#4F46E5",
                  }}
                />
              </Box>
            }
          />
        </Tabs>
      </Paper>

      {/* ========================================================================================= */}
      {/* TAB 1: WORKLOAD APPLICATIONS VIEW                                                        */}
      {/* ========================================================================================= */}
      {activeTab === "apps" && (
        <Box>
          {/* Stats Summary Cards (Full-Width Grid) */}
          <Grid container spacing={2.5} sx={{ mb: 3 }}>
            <Grid item xs={12} sm={6} md={3}>
              <Paper
                elevation={0}
                sx={{
                  p: 2.2,
                  borderRadius: "12px",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  bgcolor: "#FFFFFF",
                }}
              >
                <Box
                  sx={{
                    width: 44,
                    height: 44,
                    borderRadius: "10px",
                    bgcolor: "#EEF2FF",
                    color: "#4F46E5",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Layers size={22} />
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Total Apps
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {apps.length}
                  </Typography>
                </Box>
              </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
              <Paper
                elevation={0}
                sx={{
                  p: 2.2,
                  borderRadius: "12px",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  bgcolor: "#FFFFFF",
                }}
              >
                <Box
                  sx={{
                    width: 44,
                    height: 44,
                    borderRadius: "10px",
                    bgcolor: "#ECFDF5",
                    color: "#10B981",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <CheckCircle2 size={22} />
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Active / Running
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {enabledCount}
                  </Typography>
                </Box>
              </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
              <Paper
                elevation={0}
                sx={{
                  p: 2.2,
                  borderRadius: "12px",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  bgcolor: "#FFFFFF",
                }}
              >
                <Box
                  sx={{
                    width: 44,
                    height: 44,
                    borderRadius: "10px",
                    bgcolor: "#E0F2FE",
                    color: "#0284C7",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <BoxIcon size={22} />
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Docker Compose
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {dockerCount}
                  </Typography>
                </Box>
              </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
              <Paper
                elevation={0}
                sx={{
                  p: 2.2,
                  borderRadius: "12px",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  bgcolor: "#FFFFFF",
                }}
              >
                <Box
                  sx={{
                    width: 44,
                    height: 44,
                    borderRadius: "10px",
                    bgcolor: "#F3E8FF",
                    color: "#9333EA",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Cpu size={22} />
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Native (Systemd / OpenRC)
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {systemdCount}
                  </Typography>
                </Box>
              </Paper>
            </Grid>
          </Grid>

          {/* Full-Width Search & Filter Bar */}
          <Paper
            elevation={0}
            sx={{
              p: 2,
              mb: 3,
              borderRadius: "12px",
              border: "1px solid #E2E8F0",
              display: "flex",
              gap: 2,
              flexWrap: "wrap",
              alignItems: "center",
              bgcolor: "#FFFFFF",
            }}
          >
            <TextField
              size="small"
              placeholder="Search apps by name, description, or referenced variables..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              InputProps={{
                startAdornment: <Search size={16} style={{ marginRight: 8, color: "#94A3B8" }} />,
              }}
              sx={{ flex: 1, minWidth: 280 }}
            />

            <FormControl size="small" sx={{ minWidth: 170 }}>
              <InputLabel>Runtime Type</InputLabel>
              <Select value={typeFilter} label="Runtime Type" onChange={(e) => setTypeFilter(e.target.value)}>
                <MenuItem value="all">All Types</MenuItem>
                <MenuItem value="docker">Docker Compose</MenuItem>
                <MenuItem value="systemd">Native Systemd</MenuItem>
              </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: 180 }}>
              <InputLabel>Deployed Node</InputLabel>
              <Select value={nodeFilter} label="Deployed Node" onChange={(e) => setNodeFilter(e.target.value)}>
                <MenuItem value="all">All Nodes</MenuItem>
                {managedNodes.map((n) => (
                  <MenuItem key={n.name} value={n.name}>
                    {n.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
          </Paper>

          {/* Content */}
          {loadingApps ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={32} sx={{ color: "#4F46E5" }} />
            </Box>
          ) : appsError ? (
            <Alert severity="error" sx={{ mb: 3 }}>
              {appsError}
            </Alert>
          ) : filteredApps.length === 0 ? (
            <Paper
              elevation={0}
              sx={{
                py: 8,
                px: 3,
                textAlign: "center",
                borderRadius: "16px",
                border: "1px dashed #CBD5E1",
                bgcolor: "#F8FAFC",
              }}
            >
              <Box
                sx={{
                  width: 56,
                  height: 56,
                  borderRadius: "50%",
                  bgcolor: "#EEF2FF",
                  color: "#4F46E5",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  mx: "auto",
                  mb: 2,
                }}
              >
                <Layers size={28} />
              </Box>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
                No Applications Found
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 460, mx: "auto", mb: 3 }}>
                {apps.length === 0
                  ? "Define your first workload application to deploy containers or native daemons across cluster nodes."
                  : "No applications match your active search filters."}
              </Typography>
              {apps.length === 0 && (
                <Button
                  variant="contained"
                  startIcon={<Plus size={16} />}
                  onClick={openCreateAppModal}
                  sx={{ bgcolor: "#4F46E5", fontWeight: 600, textTransform: "none", "&:hover": { bgcolor: "#4338CA" } }}
                >
                  Create First App
                </Button>
              )}
            </Paper>
          ) : (
            /* Full-Width Grid Cards */
            <Grid container spacing={2.5}>
              {filteredApps.map((app) => (
                <Grid item xs={12} sm={6} md={4} lg={3} xl={2.4} key={app.id}>
                  <Card
                    elevation={0}
                    sx={{
                      borderRadius: "14px",
                      border: "1px solid #E2E8F0",
                      height: "100%",
                      display: "flex",
                      flexDirection: "column",
                      transition: "all 0.2s ease-in-out",
                      "&:hover": {
                        borderColor: "#CBD5E1",
                        boxShadow: "0 8px 24px rgba(15, 23, 42, 0.06)",
                        transform: "translateY(-2px)",
                      },
                    }}
                  >
                    <CardContent sx={{ p: 2.5, flex: 1, display: "flex", flexDirection: "column" }}>
                      {/* Header */}
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", mb: 1.5 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
                          <Box
                            sx={{
                              width: 36,
                              height: 36,
                              borderRadius: "8px",
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "center",
                              bgcolor: app.app_type === "docker" ? "#E0F2FE" : "#F3E8FF",
                              color: app.app_type === "docker" ? "#0284C7" : "#9333EA",
                            }}
                          >
                            {app.app_type === "docker" ? <BoxIcon size={18} /> : <Cpu size={18} />}
                          </Box>
                          <Box>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                              <Typography
                                variant="subtitle1"
                                sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}
                              >
                                {app.name}
                              </Typography>
                              <Tooltip title="Rename App">
                                <IconButton
                                  size="small"
                                  onClick={() => handleOpenRenameApp(app)}
                                  sx={{ p: 0.3, color: "#94A3B8", "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" } }}
                                >
                                  <Tag size={13} />
                                </IconButton>
                              </Tooltip>
                            </Box>
                            <Chip
                              size="small"
                              label={app.app_type === "docker" ? "Docker" : "Systemd"}
                              sx={{
                                height: 20,
                                fontSize: "0.68rem",
                                fontWeight: 700,
                                bgcolor: app.app_type === "docker" ? "#E0F2FE" : "#F3E8FF",
                                color: app.app_type === "docker" ? "#0369A1" : "#7E22CE",
                                mt: 0.3,
                              }}
                            />
                          </Box>
                        </Box>

                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <FormControlLabel
                            title={
                              app.published
                                ? "Published (app deployed to nodes on sync)"
                                : "Draft (app will not be deployed to nodes)"
                            }
                            control={
                              <Checkbox
                                size="small"
                                checked={Boolean(app.published)}
                                disabled={Boolean(togglingPublishedAppIds[app.id])}
                                onChange={() => handleTogglePublished(app)}
                                sx={{
                                  p: 0.4,
                                  color: "#94A3B8",
                                  "&.Mui-checked": { color: "#059669" },
                                }}
                              />
                            }
                            label={
                              <Typography
                                variant="caption"
                                sx={{
                                  fontWeight: 600,
                                  fontSize: "0.75rem",
                                  color: app.published ? "#059669" : "#64748B",
                                  userSelect: "none",
                                }}
                              >
                                Published
                              </Typography>
                            }
                            sx={{ m: 0 }}
                          />

                          <FormControlLabel
                            title={
                              app.enabled ? "Enabled (service runs on node)" : "Disabled (service stopped on node)"
                            }
                            control={
                              <Checkbox
                                size="small"
                                checked={app.enabled}
                                disabled={Boolean(togglingAppIds[app.id])}
                                onChange={() => handleToggleApp(app)}
                                sx={{
                                  p: 0.4,
                                  color: "#94A3B8",
                                  "&.Mui-checked": { color: "#4F46E5" },
                                }}
                              />
                            }
                            label={
                              <Typography
                                variant="caption"
                                sx={{
                                  fontWeight: 600,
                                  fontSize: "0.75rem",
                                  color: app.enabled ? "#4F46E5" : "#64748B",
                                  userSelect: "none",
                                }}
                              >
                                Enabled
                              </Typography>
                            }
                            sx={{ m: 0 }}
                          />
                        </Box>
                      </Box>

                      {/* Description */}
                      <Typography
                        variant="body2"
                        sx={{
                          color: "#64748B",
                          fontSize: "0.85rem",
                          mb: 2,
                          minHeight: 36,
                          lineHeight: 1.4,
                          display: "-webkit-box",
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: "vertical",
                          overflow: "hidden",
                        }}
                      >
                        {app.description || "No description provided."}
                      </Typography>

                      {app.url && (
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, mb: 1.5, color: "#2563EB" }}>
                          <Globe size={13} style={{ flexShrink: 0 }} />
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 600,
                              color: "#2563EB",
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                              maxWidth: "100%",
                            }}
                          >
                            {app.url}
                          </Typography>
                        </Box>
                      )}

                      <Divider sx={{ my: 1.2, borderColor: "#F1F5F9" }} />

                      {/* Target Nodes */}
                      <Box sx={{ mb: 1.5 }}>
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.6 }}>
                          <Typography
                            variant="caption"
                            sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase" }}
                          >
                            Target Nodes
                          </Typography>
                          {app.target_type === "all" ? (
                            <Chip
                              size="small"
                              icon={<Globe size={12} />}
                              label="All Nodes"
                              sx={{
                                height: 20,
                                fontSize: "0.68rem",
                                fontWeight: 700,
                                bgcolor: "#E0F2FE",
                                color: "#0369A1",
                              }}
                            />
                          ) : app.target_type === "tag" ? (
                            <Chip
                              size="small"
                              icon={<Tag size={12} />}
                              label={`Tag: #${app.target_tag}`}
                              sx={{
                                height: 20,
                                fontSize: "0.68rem",
                                fontWeight: 700,
                                bgcolor: "#F5F3FF",
                                color: "#6D28D9",
                              }}
                            />
                          ) : (
                            <Chip
                              size="small"
                              icon={<Sliders size={12} />}
                              label={`Manual (${app.nodes?.length || 0})`}
                              sx={{
                                height: 20,
                                fontSize: "0.68rem",
                                fontWeight: 700,
                                bgcolor: "#F1F5F9",
                                color: "#475569",
                              }}
                            />
                          )}
                        </Box>

                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
                          {(() => {
                            if (app.target_type === "all") {
                              if (managedNodes.length === 0) {
                                return (
                                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                    No agent nodes available
                                  </Typography>
                                );
                              }
                              return managedNodes.map((n) => (
                                <Chip
                                  key={n.name}
                                  component={RouterLink}
                                  to={`/nodes/${encodeURIComponent(n.name)}?tab=apps`}
                                  clickable
                                  onClick={(e) => e.stopPropagation()}
                                  size="small"
                                  label={n.name}
                                  sx={{
                                    height: 20,
                                    fontSize: "0.7rem",
                                    fontWeight: 600,
                                    bgcolor: "#E0F2FE",
                                    color: "#0369A1",
                                    textDecoration: "none",
                                    cursor: "pointer",
                                    transition: "all 0.15s ease",
                                    "&:hover": {
                                      bgcolor: "#BAE6FD",
                                      color: "#0284C7",
                                    },
                                  }}
                                />
                              ));
                            }
                            if (app.target_type === "tag") {
                              const matching = managedNodes.filter((n) =>
                                n.tags?.some((t) => t.toLowerCase() === (app.target_tag || "").toLowerCase()),
                              );
                              if (matching.length === 0) {
                                return (
                                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                    No nodes currently matching tag #{app.target_tag}
                                  </Typography>
                                );
                              }
                              return matching.map((n) => (
                                <Chip
                                  key={n.name}
                                  component={RouterLink}
                                  to={`/nodes/${encodeURIComponent(n.name)}?tab=apps`}
                                  clickable
                                  onClick={(e) => e.stopPropagation()}
                                  size="small"
                                  label={n.name}
                                  sx={{
                                    height: 20,
                                    fontSize: "0.7rem",
                                    fontWeight: 600,
                                    bgcolor: "#EDE9FE",
                                    color: "#5B21B6",
                                    textDecoration: "none",
                                    cursor: "pointer",
                                    transition: "all 0.15s ease",
                                    "&:hover": {
                                      bgcolor: "#DDD6FE",
                                      color: "#4C1D95",
                                    },
                                  }}
                                />
                              ));
                            }
                            if (app.nodes && app.nodes.length > 0) {
                              return app.nodes.map((nodeName) => (
                                <Chip
                                  key={nodeName}
                                  component={RouterLink}
                                  to={`/nodes/${encodeURIComponent(nodeName)}?tab=apps`}
                                  clickable
                                  onClick={(e) => e.stopPropagation()}
                                  size="small"
                                  label={nodeName}
                                  sx={{
                                    height: 20,
                                    fontSize: "0.7rem",
                                    fontWeight: 600,
                                    bgcolor: "#F1F5F9",
                                    color: "#334155",
                                    textDecoration: "none",
                                    cursor: "pointer",
                                    transition: "all 0.15s ease",
                                    "&:hover": {
                                      bgcolor: "#E2E8F0",
                                      color: "#1E293B",
                                    },
                                  }}
                                />
                              ));
                            }
                            return (
                              <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                None assigned (unassigned)
                              </Typography>
                            );
                          })()}
                        </Box>
                      </Box>

                      <Box sx={{ flexGrow: 1 }} />

                      {/* Footer Actions */}
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", pt: 1 }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                          {!app.published ? (
                            <Tooltip title="Draft app will not be deployed to nodes">
                              <Chip
                                size="small"
                                icon={<FileEdit size={12} />}
                                label="Draft"
                                sx={{
                                  height: 22,
                                  fontSize: "0.72rem",
                                  fontWeight: 700,
                                  bgcolor: "#FEF3C7",
                                  color: "#D97706",
                                  "& .MuiChip-icon": { color: "inherit" },
                                }}
                              />
                            </Tooltip>
                          ) : (
                            <Chip
                              size="small"
                              icon={
                                togglingAppIds[app.id] ? (
                                  <CircularProgress size={10} sx={{ color: "inherit" }} />
                                ) : app.enabled ? (
                                  <CheckCircle2 size={12} />
                                ) : (
                                  <Square size={12} />
                                )
                              }
                              label={app.enabled ? "Active" : "Stopped"}
                              sx={{
                                height: 22,
                                fontSize: "0.72rem",
                                fontWeight: 700,
                                bgcolor: app.enabled ? "#ECFDF5" : "#F1F5F9",
                                color: app.enabled ? "#059669" : "#64748B",
                                "& .MuiChip-icon": { color: "inherit" },
                              }}
                            />
                          )}
                        </Box>

                        <Box sx={{ display: "flex", gap: 0.5 }}>
                          <Tooltip title="Rename App">
                            <IconButton
                              size="small"
                              onClick={() => handleOpenRenameApp(app)}
                              sx={{ color: "#64748B", "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" } }}
                            >
                              <Tag size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Duplicate App">
                            <IconButton
                              size="small"
                              disabled={duplicatingAppId === app.id}
                              onClick={() => handleDuplicateApp(app)}
                              sx={{ color: "#64748B", "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" } }}
                            >
                              {duplicatingAppId === app.id ? (
                                <CircularProgress size={16} sx={{ color: "inherit" }} />
                              ) : (
                                <Copy size={16} />
                              )}
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Edit App">
                            <IconButton size="small" onClick={() => openEditAppModal(app)} sx={{ color: "#64748B" }}>
                              <Edit size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Delete App">
                            <IconButton size="small" onClick={() => setDeleteApp(app)} sx={{ color: "#EF4444" }}>
                              <Trash2 size={16} />
                            </IconButton>
                          </Tooltip>
                        </Box>
                      </Box>
                    </CardContent>
                  </Card>
                </Grid>
              ))}
            </Grid>
          )}
        </Box>
      )}

      {/* ========================================================================================= */}
      {/* TAB 2: GLOBAL VARIABLES KV STORE VIEW                                                     */}
      {/* ========================================================================================= */}
      {activeTab === "variables" && (
        <Box>
          {/* Global Variables Guide Banner */}
          <Paper
            elevation={0}
            sx={{
              p: 2.5,
              mb: 3,
              borderRadius: "14px",
              bgcolor: "#EEF2FF",
              border: "1px solid #C7D2FE",
              display: "flex",
              alignItems: "flex-start",
              gap: 2,
            }}
          >
            <Box sx={{ color: "#4F46E5", mt: 0.2 }}>
              <Info size={22} />
            </Box>
            <Box>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#312E81", mb: 0.5 }}>
                Global Variables & Dynamic Templates
              </Typography>
              <Typography variant="body2" sx={{ color: "#4338CA", fontSize: "0.85rem", lineHeight: 1.5 }}>
                Variables are cluster-wide key-value entries automatically substituted in your <code>.env</code> files
                and auxiliary config files. Systemd units and Docker Compose manifests are excluded to prevent conflicts
                with their native variable interpolation.
              </Typography>
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1.5, mt: 1.2 }}>
                <Chip
                  size="small"
                  label="${VAR_NAME} or {{VAR_NAME}}"
                  sx={{ bgcolor: "#FFFFFF", color: "#3730A3", fontWeight: 700, fontSize: "0.75rem" }}
                />
                <Chip
                  size="small"
                  label="${VAR_NAME:-default_fallback}"
                  sx={{ bgcolor: "#FFFFFF", color: "#3730A3", fontWeight: 700, fontSize: "0.75rem" }}
                />
                <Chip
                  size="small"
                  label="Nested & Bash: ${VAR:-${FALLBACK}}, ${VAR^^}, ${VAR:0:4}"
                  sx={{ bgcolor: "#FFFFFF", color: "#3730A3", fontWeight: 700, fontSize: "0.75rem" }}
                />
                <Chip
                  size="small"
                  label="Node Context: ${NODE_NAME}, ${NODE_INTERFACE}, ${NODE_IP}, ${NODE_IP6}, ${NODE_EXTERNAL_IP}, ${NODE_EXTERNAL_IP6}, ${HAS_CUSTOM_SCRIPT}, ${NODE_TAGS}, ${NODE_TAG_*}, ${NODE_DATA_*}"
                  sx={{ bgcolor: "#FFFFFF", color: "#3730A3", fontWeight: 700, fontSize: "0.75rem" }}
                />
              </Box>
            </Box>
          </Paper>

          {/* Variables Filter Bar */}
          <Paper
            elevation={0}
            sx={{
              p: 2,
              mb: 3,
              borderRadius: "12px",
              border: "1px solid #E2E8F0",
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              bgcolor: "#FFFFFF",
              flexWrap: "wrap",
              gap: 2,
            }}
          >
            <TextField
              size="small"
              placeholder="Search variables by key or description..."
              value={varSearch}
              onChange={(e) => setVarSearch(e.target.value)}
              InputProps={{
                startAdornment: <Search size={16} style={{ marginRight: 8, color: "#94A3B8" }} />,
              }}
              sx={{ width: { xs: "100%", sm: 360 } }}
            />

            <Typography variant="body2" sx={{ color: "#64748B", fontWeight: 600 }}>
              Showing {filteredVars.length} of {variables.length} variable(s)
            </Typography>
          </Paper>

          {loadingVars ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
              <CircularProgress size={32} sx={{ color: "#4F46E5" }} />
            </Box>
          ) : varsError ? (
            <Alert severity="error" sx={{ mb: 3 }}>
              {varsError}
            </Alert>
          ) : filteredVars.length === 0 ? (
            <Paper
              elevation={0}
              sx={{
                py: 8,
                px: 3,
                textAlign: "center",
                borderRadius: "16px",
                border: "1px dashed #CBD5E1",
                bgcolor: "#F8FAFC",
              }}
            >
              <Box
                sx={{
                  width: 56,
                  height: 56,
                  borderRadius: "50%",
                  bgcolor: "#EEF2FF",
                  color: "#4F46E5",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  mx: "auto",
                  mb: 2,
                }}
              >
                <Database size={28} />
              </Box>
              <Typography variant="h6" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
                No Variables Defined
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 440, mx: "auto", mb: 3 }}>
                Store cluster-wide values like domain names, shared ports, database URLs, or secret tokens.
              </Typography>
              <Button
                variant="contained"
                startIcon={<Plus size={16} />}
                onClick={openCreateVarModal}
                sx={{ bgcolor: "#4F46E5", fontWeight: 600, textTransform: "none", "&:hover": { bgcolor: "#4338CA" } }}
              >
                Add First Variable
              </Button>
            </Paper>
          ) : (
            /* Full-Width Variables Table */
            <TableContainer
              component={Paper}
              elevation={0}
              sx={{
                borderRadius: "14px",
                border: "1px solid #E2E8F0",
                bgcolor: "#FFFFFF",
                overflow: "hidden",
              }}
            >
              <Table sx={{ minWidth: 700 }}>
                <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                  <TableRow>
                    <TableCell sx={{ fontWeight: 800, color: "#475569", width: 260 }}>KEY</TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569" }}>VALUE</TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569" }}>DESCRIPTION</TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569", width: 150 }}>UPDATED</TableCell>
                    <TableCell align="right" sx={{ fontWeight: 800, color: "#475569", width: 110 }}>
                      ACTIONS
                    </TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {filteredVars.map((v) => {
                    const isSecret = !!v.is_secret;
                    const isRevealed = !!revealedSecrets[v.key];
                    const isCopied = copiedKey === v.key;

                    return (
                      <TableRow
                        key={v.key}
                        sx={{
                          "&:hover": { bgcolor: "#F8FAFC" },
                          "& td": { borderColor: "#F1F5F9" },
                        }}
                      >
                        {/* Key */}
                        <TableCell>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <Box
                              component="span"
                              sx={{
                                fontFamily: "monospace",
                                fontWeight: 700,
                                fontSize: "0.85rem",
                                color: "#0F172A",
                                bgcolor: "#F1F5F9",
                                px: 1,
                                py: 0.3,
                                borderRadius: "6px",
                                border: "1px solid #E2E8F0",
                              }}
                            >
                              {v.key}
                            </Box>
                            {isSecret && (
                              <Tooltip title="Secret Variable">
                                <Lock size={14} style={{ color: "#F59E0B" }} />
                              </Tooltip>
                            )}
                          </Box>
                        </TableCell>

                        {/* Value */}
                        <TableCell>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <Typography
                              component="span"
                              sx={{
                                fontFamily: "monospace",
                                fontSize: "0.85rem",
                                color: isSecret && !isRevealed ? "#94A3B8" : "#334155",
                                wordBreak: "break-all",
                              }}
                            >
                              {isSecret && !isRevealed ? "••••••••••••••••" : v.value || "(empty)"}
                            </Typography>
                            {isSecret && (
                              <IconButton
                                size="small"
                                onClick={() => setRevealedSecrets((prev) => ({ ...prev, [v.key]: !prev[v.key] }))}
                                sx={{ color: "#94A3B8" }}
                              >
                                {isRevealed ? <EyeOff size={15} /> : <Eye size={15} />}
                              </IconButton>
                            )}
                            <Tooltip title={isCopied ? "Copied!" : "Copy value"}>
                              <IconButton
                                size="small"
                                onClick={() => copyText(v.value, v.key)}
                                sx={{ color: isCopied ? "#10B981" : "#94A3B8" }}
                              >
                                {isCopied ? <Check size={14} /> : <Copy size={14} />}
                              </IconButton>
                            </Tooltip>
                          </Box>
                        </TableCell>

                        {/* Description */}
                        <TableCell>
                          <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                            {v.description || "—"}
                          </Typography>
                        </TableCell>

                        {/* Updated */}
                        <TableCell>
                          <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 600 }}>
                            {v.updated_at ? new Date(v.updated_at).toLocaleDateString() : "Just now"}
                          </Typography>
                        </TableCell>

                        {/* Actions */}
                        <TableCell align="right">
                          <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.5 }}>
                            <Tooltip title="Edit Variable">
                              <IconButton size="small" onClick={() => openEditVarModal(v)} sx={{ color: "#64748B" }}>
                                <Edit size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete Variable">
                              <IconButton size="small" onClick={() => setDeleteVarKey(v.key)} sx={{ color: "#EF4444" }}>
                                <Trash2 size={16} />
                              </IconButton>
                            </Tooltip>
                          </Box>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Box>
      )}

      {/* ========================================================================================= */}
      {/* VARIABLE CREATE / EDIT MODAL                                                              */}
      {/* ========================================================================================= */}
      <Dialog open={varModalOpen} onClose={() => setVarModalOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1.5, pb: 1 }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: "8px",
              bgcolor: "#EEF2FF",
              color: "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Database size={20} />
          </Box>
          <Typography variant="h6" sx={{ fontWeight: 800 }}>
            {variables.some((v) => v.key === varKey) ? `Edit Variable: ${varKey}` : "Add Global Variable"}
          </Typography>
        </DialogTitle>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: 2 }}>
          <TextField
            fullWidth
            label="Variable Key"
            placeholder="e.g. CLUSTER_DNS or DB_PASS"
            value={varKey}
            onChange={(e) => setVarKey(e.target.value.toUpperCase().replace(/[^A-Z0-9_]/g, ""))}
            helperText="Uppercase identifier matching ^[A-Z_][A-Z0-9_]*$"
          />
          <TextField
            fullWidth
            multiline
            rows={3}
            label="Variable Value"
            placeholder="String value or token..."
            value={varVal}
            onChange={(e) => setVarVal(e.target.value)}
            inputProps={{ style: { fontFamily: "monospace", fontSize: "0.85rem" } }}
          />
          <TextField
            fullWidth
            label="Description (Optional)"
            placeholder="What this variable is used for..."
            value={varDesc}
            onChange={(e) => setVarDesc(e.target.value)}
          />
          <FormControlLabel
            control={
              <Switch checked={varIsSecret} onChange={(e) => setVarIsSecret(e.target.checked)} color="primary" />
            }
            label={
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Mask value in UI (Secret)
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Masks the value with dots in the table view to prevent accidental shoulder surfing.
                </Typography>
              </Box>
            }
          />
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2.5 }}>
          <Button onClick={() => setVarModalOpen(false)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSaveVariable}
            disabled={savingVar}
            sx={{ bgcolor: "#4F46E5", fontWeight: 600, textTransform: "none", "&:hover": { bgcolor: "#4338CA" } }}
          >
            {savingVar ? <CircularProgress size={16} /> : "Save Variable"}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Variable Confirmation Dialog */}
      <Dialog open={!!deleteVarKey} onClose={() => setDeleteVarKey(null)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ fontWeight: 700 }}>Delete Variable</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: "#475569" }}>
            Are you sure you want to delete variable <strong>{deleteVarKey}</strong>? Applications referencing this
            variable will fall back to default values or empty strings upon next sync.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDeleteVarKey(null)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={() => deleteVarKey && handleDeleteVariable(deleteVarKey)}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete App Confirmation Dialog */}
      <Dialog open={!!deleteApp} onClose={() => setDeleteApp(null)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ fontWeight: 700 }}>Confirm Delete {deleteApp?.name}</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: "#475569" }}>
            Are you sure you want to delete this workload app {deleteApp?.name}? Easy42 agents will immediately stop the
            service, remove its configuration, and clean up orphaned files on all nodes.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDeleteApp(null)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={() => deleteApp && handleDeleteApp(deleteApp.id)}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Delete App
          </Button>
        </DialogActions>
      </Dialog>

      {/* Rename App Modal */}
      <RenameAppModal
        open={renameModalOpen}
        app={appToRename}
        existingApps={apps}
        onClose={() => {
          setRenameModalOpen(false);
          setAppToRename(null);
        }}
        onAppRenamed={async (_oldName, updatedApp) => {
          setApps((prev) => prev.map((a) => (a.id === updatedApp.id ? updatedApp : a)));
          setStateToast({
            message: `App renamed to "${updatedApp.name}" successfully.`,
            severity: "success",
          });
        }}
      />

      {/* ========================================================================================= */}
      {/* APP CREATE / EDIT FULL-WIDTH MODAL                                                        */}
      {/* ========================================================================================= */}
      <Dialog
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        maxWidth="lg"
        fullWidth
        PaperProps={{
          sx: { borderRadius: "16px", minHeight: 680 },
        }}
      >
        <DialogTitle sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", pb: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Box
              sx={{
                width: 36,
                height: 36,
                borderRadius: "8px",
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Layers size={20} />
            </Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A" }}>
              {editingApp ? `Edit App: ${editingApp.name}` : "Create Workload App"}
            </Typography>
          </Box>
          <IconButton size="small" onClick={() => setModalOpen(false)}>
            <X size={18} />
          </IconButton>
        </DialogTitle>

        {/* Modal Navigation Tabs */}
        <Box sx={{ px: 3, borderBottom: "1px solid #E2E8F0" }}>
          <Tabs
            value={modalTab}
            onChange={(_, val) => setModalTab(val)}
            sx={{
              "& .MuiTab-root": { textTransform: "none", fontWeight: 700, fontSize: "0.85rem", minHeight: 46 },
              "& .Mui-selected": { color: "#4F46E5" },
              "& .MuiTabs-indicator": { bgcolor: "#4F46E5" },
            }}
          >
            <Tab value="general" label="General & Nodes" icon={<Sliders size={16} />} iconPosition="start" />
            <Tab
              value="definition"
              label={formType === "docker" ? "Compose Manifest" : "Systemd Unit"}
              icon={<FileCode size={16} />}
              iconPosition="start"
            />
            <Tab value="env" label="Environment (.env)" icon={<FileText size={16} />} iconPosition="start" />
            <Tab
              value="files"
              label={`Config Files (${formFiles.length})`}
              icon={<FilePlus size={16} />}
              iconPosition="start"
            />
            <Tab value="preview" label="Variables Preview" icon={<Code2 size={16} />} iconPosition="start" />
            {formType === "systemd" && (
              <Tab value="transpile" label="Transpile Preview" icon={<Eye size={16} />} iconPosition="start" />
            )}
          </Tabs>
        </Box>

        <DialogContent sx={{ p: 3 }}>
          {/* Quick Variables Insert Chips Bar */}
          {modalTab !== "general" && modalTab !== "preview" && modalTab !== "transpile" && (
            <Box
              sx={{
                mb: 2,
                p: 1.5,
                bgcolor: "#F8FAFC",
                borderRadius: "10px",
                border: "1px solid #E2E8F0",
                display: "flex",
                alignItems: "center",
                flexWrap: "wrap",
                gap: 1,
              }}
            >
              <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B", mr: 0.5 }}>
                Insert Variable:
              </Typography>
              {variables.map((v) => (
                <Chip
                  key={v.key}
                  size="small"
                  label={`\${${v.key}}`}
                  onClick={() => {
                    const tag = `\${${v.key}}`;
                    if (modalTab === "definition") setFormMain((prev) => prev + tag);
                    else if (modalTab === "env") setFormEnv((prev) => prev + tag);
                  }}
                  sx={{
                    fontFamily: "monospace",
                    fontSize: "0.72rem",
                    fontWeight: 700,
                    cursor: "pointer",
                    bgcolor: "#FFFFFF",
                    border: "1px solid #CBD5E1",
                    "&:hover": { bgcolor: "#EEF2FF", borderColor: "#818CF8" },
                  }}
                />
              ))}
              <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
              <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B", mr: 0.5 }}>
                Node Context:
              </Typography>
              {INTERNAL_NODE_VARS.map((nv) => (
                <Tooltip key={nv.key} title={nv.tooltip} arrow>
                  <Chip
                    size="small"
                    label={nv.label}
                    onClick={() => {
                      const tag = nv.key === "NODE_DATA_*" ? "${NODE_DATA_}" : nv.label;
                      if (modalTab === "definition") setFormMain((prev) => prev + tag);
                      else if (modalTab === "env") setFormEnv((prev) => prev + tag);
                    }}
                    sx={{
                      fontFamily: "monospace",
                      fontSize: "0.72rem",
                      fontWeight: 700,
                      cursor: "pointer",
                      bgcolor: "#F8FAFC",
                      color: "#334155",
                      border: "1px solid #CBD5E1",
                      "&:hover": { bgcolor: "#EFF6FF", borderColor: "#3B82F6", color: "#1D4ED8" },
                    }}
                  />
                </Tooltip>
              ))}
            </Box>
          )}

          {/* TAB 1: General & Nodes */}
          {modalTab === "general" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
              <Grid container spacing={2}>
                <Grid item xs={12} sm={6}>
                  <TextField
                    fullWidth
                    label="App Name"
                    placeholder="e.g. vproxy or coredns"
                    value={formName}
                    onChange={(e) => setFormName(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ""))}
                    helperText={
                      editingApp && formName && formName !== editingApp.name
                        ? `Renaming from '${editingApp.name}' to '${formName}' (prefix 'ezapp-' used on nodes)`
                        : "Fixed prefix 'ezapp-' will be used on nodes (e.g. ezapp-vproxy)"
                    }
                  />
                </Grid>
                <Grid item xs={12} sm={6}>
                  <FormControl fullWidth>
                    <InputLabel>Runtime Type</InputLabel>
                    <Select
                      value={formType}
                      label="Runtime Type"
                      onChange={(e) => handleTypeChange(e.target.value as "docker" | "systemd")}
                    >
                      <MenuItem value="docker">Docker Compose (Container)</MenuItem>
                      <MenuItem value="systemd">Systemd / Native Service (Daemon)</MenuItem>
                    </Select>
                  </FormControl>
                </Grid>
              </Grid>

              <TextField
                fullWidth
                label="Description"
                placeholder="Brief description of what this workload does..."
                value={formDesc}
                onChange={(e) => setFormDesc(e.target.value)}
              />

              <TextField
                fullWidth
                label="Homepage URL (Optional)"
                placeholder="e.g. http://${NODE_IP}:8080 or https://${DOMAIN}"
                helperText="Optional homepage/dashboard URL displayed in Node Details. Supports variables like ${NODE_IP}, ${NODE_NAME}, and custom variables."
                value={formUrl}
                onChange={(e) => setFormUrl(e.target.value)}
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <Globe size={18} color="#64748B" />
                    </InputAdornment>
                  ),
                }}
              />

              <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: "10px", border: "1px solid #E2E8F0" }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
                  Target Nodes
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1.5 }}>
                  Choose which cluster nodes will deploy and run this workload.
                </Typography>

                <FormControl component="fieldset" fullWidth sx={{ mb: 2 }}>
                  <RadioGroup
                    row
                    value={formTargetType}
                    onChange={(e) => setFormTargetType(e.target.value as "manual" | "all" | "tag")}
                  >
                    <FormControlLabel
                      value="manual"
                      control={<Radio size="small" />}
                      label={
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                          <Sliders size={16} color="#6366F1" />
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            Manually Select Nodes
                          </Typography>
                        </Box>
                      }
                    />
                    <FormControlLabel
                      value="all"
                      control={<Radio size="small" />}
                      label={
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                          <Globe size={16} color="#0EA5E9" />
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            All Nodes
                          </Typography>
                        </Box>
                      }
                    />
                    <FormControlLabel
                      value="tag"
                      control={<Radio size="small" />}
                      label={
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                          <Tag size={16} color="#8B5CF6" />
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            Nodes of a Selected Tag
                          </Typography>
                        </Box>
                      }
                    />
                  </RadioGroup>
                </FormControl>

                {/* Option 1: Manual */}
                {formTargetType === "manual" && (
                  <Box>
                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        Select zero or more nodes. Selecting none keeps this app unassigned.
                      </Typography>
                      <Box sx={{ display: "flex", gap: 1 }}>
                        <Button
                          size="small"
                          sx={{ textTransform: "none", fontSize: "0.75rem", p: 0 }}
                          onClick={() => setFormNodes(managedNodes.map((n) => n.name))}
                        >
                          Select All
                        </Button>
                        <Button
                          size="small"
                          sx={{ textTransform: "none", fontSize: "0.75rem", p: 0, color: "#64748B" }}
                          onClick={() => setFormNodes([])}
                        >
                          Clear (None)
                        </Button>
                      </Box>
                    </Box>

                    <FormControl fullWidth size="small">
                      <InputLabel>Selected Nodes ({formNodes.length})</InputLabel>
                      <Select
                        multiple
                        value={formNodes}
                        label={`Selected Nodes (${formNodes.length})`}
                        onChange={(e) => {
                          const val = e.target.value;
                          setFormNodes(typeof val === "string" ? val.split(",") : val);
                        }}
                        renderValue={(selected) => (
                          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                            {selected.length === 0 ? (
                              <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                None selected (unassigned)
                              </Typography>
                            ) : (
                              selected.map((val) => (
                                <Chip key={val} label={val} size="small" sx={{ height: 22, fontSize: "0.75rem" }} />
                              ))
                            )}
                          </Box>
                        )}
                      >
                        {managedNodes.map((n) => (
                          <MenuItem key={n.name} value={n.name}>
                            <Box
                              sx={{
                                display: "flex",
                                justifyContent: "space-between",
                                width: "100%",
                                alignItems: "center",
                              }}
                            >
                              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                {n.name}
                              </Typography>
                              {n.tags && n.tags.length > 0 && (
                                <Box sx={{ display: "flex", gap: 0.5 }}>
                                  {n.tags.map((t) => (
                                    <Chip
                                      key={t}
                                      label={`#${t}`}
                                      size="small"
                                      sx={{ height: 18, fontSize: "0.65rem", bgcolor: "#EDE9FE", color: "#6D28D9" }}
                                    />
                                  ))}
                                </Box>
                              )}
                            </Box>
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                    <Typography
                      variant="caption"
                      sx={{ color: formNodes.length === 0 ? "#D97706" : "#64748B", mt: 0.5, display: "block" }}
                    >
                      {formNodes.length === 0
                        ? "Notice: No nodes selected. The app will be created but will not deploy anywhere until nodes are assigned."
                        : `Assigned to ${formNodes.length} node(s).`}
                    </Typography>
                  </Box>
                )}

                {/* Option 2: All Nodes */}
                {formTargetType === "all" && (
                  <Box sx={{ p: 2, bgcolor: "#E0F2FE", borderRadius: "8px", border: "1px solid #BAE6FD" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                      <Globe size={18} color="#0284C7" />
                      <Typography variant="body2" sx={{ fontWeight: 700, color: "#0369A1" }}>
                        Automatic Deployment to All Agent Nodes ({managedNodes.length})
                      </Typography>
                    </Box>
                    <Typography variant="caption" sx={{ color: "#0C4A6E", display: "block", mb: 1.5 }}>
                      This app will be deployed to all cluster nodes running in agent mode. Any new agent node added in
                      the future will automatically receive this app.
                    </Typography>
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
                      {managedNodes.map((n) => (
                        <Chip
                          key={n.name}
                          label={n.name}
                          size="small"
                          sx={{
                            bgcolor: "#FFFFFF",
                            color: "#0369A1",
                            border: "1px solid #7DD3FC",
                            fontWeight: 600,
                            fontSize: "0.75rem",
                          }}
                        />
                      ))}
                    </Box>
                  </Box>
                )}

                {/* Option 3: Nodes of a Tag */}
                {formTargetType === "tag" && (
                  <Box>
                    <Grid container spacing={2} alignItems="center">
                      <Grid item xs={12} sm={6}>
                        <FormControl fullWidth size="small">
                          <InputLabel>Select from Existing Tags</InputLabel>
                          <Select
                            value={availableTags.includes(formTargetTag) ? formTargetTag : ""}
                            label="Select from Existing Tags"
                            onChange={(e) => setFormTargetTag(e.target.value)}
                          >
                            <MenuItem value="">
                              <em>-- Select a tag --</em>
                            </MenuItem>
                            {availableTags.map((tag) => {
                              const matchCount = managedNodes.filter((n) => n.tags?.includes(tag)).length;
                              return (
                                <MenuItem key={tag} value={tag}>
                                  <Box
                                    sx={{
                                      display: "flex",
                                      justifyContent: "space-between",
                                      width: "100%",
                                      alignItems: "center",
                                    }}
                                  >
                                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#7C3AED" }}>
                                      #{tag}
                                    </Typography>
                                    <Chip
                                      label={`${matchCount} node(s)`}
                                      size="small"
                                      sx={{ height: 18, fontSize: "0.68rem" }}
                                    />
                                  </Box>
                                </MenuItem>
                              );
                            })}
                          </Select>
                        </FormControl>
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <TextField
                          fullWidth
                          size="small"
                          label="Or Enter Custom Tag"
                          placeholder="e.g. edge, ingress, prod"
                          value={formTargetTag}
                          onChange={(e) => setFormTargetTag(e.target.value.toLowerCase().trim())}
                          InputProps={{
                            startAdornment: <Tag size={16} style={{ marginRight: 6, color: "#8B5CF6" }} />,
                          }}
                        />
                      </Grid>
                    </Grid>

                    {/* Tag match preview */}
                    <Box sx={{ mt: 1.5, p: 1.5, bgcolor: "#F5F3FF", borderRadius: "8px", border: "1px solid #DDD6FE" }}>
                      {(() => {
                        const matchingNodes = managedNodes.filter((n) =>
                          n.tags?.some((t) => t.toLowerCase() === formTargetTag.toLowerCase()),
                        );
                        if (!formTargetTag) {
                          return (
                            <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                              Please select or enter a tag above to see matching nodes.
                            </Typography>
                          );
                        }
                        if (matchingNodes.length === 0) {
                          return (
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                              <Info size={16} color="#8B5CF6" />
                              <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                                No nodes currently have the tag <strong>#{formTargetTag}</strong>. The app will
                                automatically deploy whenever a node is tagged with <strong>#{formTargetTag}</strong>.
                              </Typography>
                            </Box>
                          );
                        }
                        return (
                          <Box>
                            <Typography
                              variant="caption"
                              sx={{ color: "#5B21B6", fontWeight: 700, display: "block", mb: 0.8 }}
                            >
                              {matchingNodes.length} node(s) currently match tag #{formTargetTag}:
                            </Typography>
                            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
                              {matchingNodes.map((n) => (
                                <Chip
                                  key={n.name}
                                  label={n.name}
                                  size="small"
                                  sx={{
                                    bgcolor: "#FFFFFF",
                                    color: "#6D28D9",
                                    border: "1px solid #C4B5FD",
                                    fontWeight: 600,
                                    fontSize: "0.75rem",
                                  }}
                                />
                              ))}
                            </Box>
                          </Box>
                        );
                      })()}
                    </Box>
                  </Box>
                )}
              </Box>

              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1 }}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formPublished}
                      onChange={(e) => setFormPublished(e.target.checked)}
                      color="success"
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                        Published
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        When published, this app is deployed to target nodes during topology synchronization. If
                        disabled, it stays as a draft and is not deployed.
                      </Typography>
                    </Box>
                  }
                />

                <FormControlLabel
                  control={
                    <Switch checked={formEnabled} onChange={(e) => setFormEnabled(e.target.checked)} color="primary" />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                        Enabled
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        When enabled, agents will actively run the service upon synchronization.
                      </Typography>
                    </Box>
                  }
                />
              </Box>
            </Box>
          )}

          {/* TAB 2: Definition Editor */}
          {modalTab === "definition" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  flexWrap: "wrap",
                  gap: 1,
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                  {formType === "docker" ? "compose.yaml Manifest" : "Systemd .service Unit File"}
                </Typography>
                <Box sx={{ display: "flex", gap: 1 }}>
                  {formType === "docker" ? (
                    <Button
                      size="small"
                      variant={showGenerator ? "contained" : "outlined"}
                      startIcon={<Wand2 size={14} />}
                      onClick={() => setShowGenerator(!showGenerator)}
                      sx={{
                        textTransform: "none",
                        fontSize: "0.75rem",
                        bgcolor: showGenerator ? "#4F46E5" : undefined,
                        color: showGenerator ? "#FFFFFF" : "#4F46E5",
                        borderColor: "#818CF8",
                      }}
                    >
                      Convert from 'docker run'
                    </Button>
                  ) : (
                    <Button
                      size="small"
                      variant={showGenerator ? "contained" : "outlined"}
                      startIcon={<Wand2 size={14} />}
                      onClick={() => setShowGenerator(!showGenerator)}
                      sx={{
                        textTransform: "none",
                        fontSize: "0.75rem",
                        bgcolor: showGenerator ? "#4F46E5" : undefined,
                        color: showGenerator ? "#FFFFFF" : "#4F46E5",
                        borderColor: "#818CF8",
                      }}
                    >
                      Generate from Command Line
                    </Button>
                  )}
                  <Button
                    size="small"
                    onClick={() => setFormMain(formType === "docker" ? DOCKER_SAMPLE : SYSTEMD_SAMPLE)}
                    sx={{ textTransform: "none", fontSize: "0.75rem", color: "#64748B" }}
                  >
                    Reset Template
                  </Button>
                </Box>
              </Box>

              {/* Generator Panel */}
              {showGenerator && (
                <Paper
                  elevation={0}
                  sx={{
                    p: 2,
                    borderRadius: "10px",
                    border: "1px solid #C7D2FE",
                    bgcolor: "#EEF2FF",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.5,
                  }}
                >
                  {formType === "docker" ? (
                    <>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Terminal size={18} color="#4338CA" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#312E81" }}>
                            Convert 'docker run' to Compose YAML (via composerize)
                          </Typography>
                        </Box>
                        <IconButton size="small" onClick={() => setShowGenerator(false)}>
                          <X size={16} />
                        </IconButton>
                      </Box>
                      <Typography variant="caption" sx={{ color: "#4338CA" }}>
                        Paste any standard <code>docker run</code> command. It will be converted into a full{" "}
                        <code>compose.yaml</code> service definition.
                      </Typography>

                      <TextField
                        fullWidth
                        multiline
                        minRows={2}
                        maxRows={4}
                        size="small"
                        placeholder="docker run -d --name web -p 8080:80 -v /data:/app --restart always nginx:alpine"
                        value={dockerRunCmd}
                        onChange={(e) => {
                          setDockerRunCmd(e.target.value);
                          if (dockerRunError) setDockerRunError(null);
                        }}
                        InputProps={{
                          style: { fontFamily: "monospace", fontSize: "0.8rem", backgroundColor: "#FFFFFF" },
                        }}
                      />

                      {dockerRunError && (
                        <Alert severity="error" sx={{ py: 0.5, fontSize: "0.8rem" }}>
                          {dockerRunError}
                        </Alert>
                      )}

                      <Box
                        sx={{
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                          flexWrap: "wrap",
                          gap: 1,
                        }}
                      >
                        <Box sx={{ display: "flex", gap: 0.8, alignItems: "center" }}>
                          <Typography variant="caption" sx={{ color: "#6366F1", fontWeight: 600 }}>
                            Examples:
                          </Typography>
                          <Chip
                            size="small"
                            label="Nginx"
                            onClick={() =>
                              setDockerRunCmd(
                                "docker run -d --name web -p 80:80 -v /html:/usr/share/nginx/html nginx:alpine",
                              )
                            }
                            sx={{
                              cursor: "pointer",
                              height: 22,
                              fontSize: "0.72rem",
                              bgcolor: "#FFFFFF",
                              border: "1px solid #C7D2FE",
                            }}
                          />
                          <Chip
                            size="small"
                            label="Redis"
                            onClick={() =>
                              setDockerRunCmd(
                                "docker run -d --name redis -p 6379:6379 -v redis-data:/data redis:alpine --requirepass ${REDIS_PASS}",
                              )
                            }
                            sx={{
                              cursor: "pointer",
                              height: 22,
                              fontSize: "0.72rem",
                              bgcolor: "#FFFFFF",
                              border: "1px solid #C7D2FE",
                            }}
                          />
                        </Box>

                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<Wand2 size={14} />}
                          onClick={() => {
                            const res = dockerRunToCompose(dockerRunCmd);
                            if (res.error) {
                              setDockerRunError(res.error);
                            } else {
                              setFormMain(res.composeYaml);
                              setShowGenerator(false);
                            }
                          }}
                          sx={{ textTransform: "none", bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                        >
                          Generate Compose YAML
                        </Button>
                      </Box>
                    </>
                  ) : (
                    <>
                      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Terminal size={18} color="#4338CA" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#312E81" }}>
                            Generate Systemd Unit from Single Command Line
                          </Typography>
                        </Box>
                        <IconButton size="small" onClick={() => setShowGenerator(false)}>
                          <X size={16} />
                        </IconButton>
                      </Box>
                      <Typography variant="caption" sx={{ color: "#4338CA" }}>
                        Enter a command line (e.g. <code>rclone serve webdav /root --addr 0.0.0.0:8080</code>). If the
                        program is not an absolute path, it will automatically wrap it with{" "}
                        <code>/bin/sh -c "exec ..."</code> for systemd compatibility.
                      </Typography>

                      <TextField
                        fullWidth
                        size="small"
                        placeholder="rclone serve webdav /root --addr 0.0.0.0:8080"
                        value={systemdCmd}
                        onChange={(e) => {
                          setSystemdCmd(e.target.value);
                          if (systemdError) setSystemdError(null);
                        }}
                        InputProps={{
                          style: { fontFamily: "monospace", fontSize: "0.82rem", backgroundColor: "#FFFFFF" },
                        }}
                      />

                      {/* Live preview badge of path type */}
                      {systemdCmd.trim() &&
                        (() => {
                          try {
                            const res = cmdlineToSystemd(systemdCmd);
                            if (res.isAbsolute) {
                              return (
                                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                                  <Chip
                                    size="small"
                                    label="Absolute Path"
                                    color="success"
                                    sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700 }}
                                  />
                                  <Typography variant="caption" sx={{ color: "#065F46", fontFamily: "monospace" }}>
                                    ExecStart={res.execStart}
                                  </Typography>
                                </Box>
                              );
                            } else {
                              return (
                                <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                                  <Chip
                                    size="small"
                                    label="Relative Executable"
                                    color="primary"
                                    sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700 }}
                                  />
                                  <Typography variant="caption" sx={{ color: "#1E40AF", fontFamily: "monospace" }}>
                                    ExecStart={res.execStart}
                                  </Typography>
                                </Box>
                              );
                            }
                          } catch {
                            return null;
                          }
                        })()}

                      <Grid container spacing={1.5}>
                        <Grid item xs={12} sm={4}>
                          <TextField
                            fullWidth
                            size="small"
                            label="Service Description"
                            placeholder={formName || "My Service"}
                            value={systemdDesc}
                            onChange={(e) => setSystemdDesc(e.target.value)}
                            sx={{ "& input": { backgroundColor: "#FFFFFF" } }}
                          />
                        </Grid>
                        <Grid item xs={12} sm={4}>
                          <TextField
                            fullWidth
                            size="small"
                            label="User"
                            value={systemdUser}
                            onChange={(e) => setSystemdUser(e.target.value)}
                            sx={{ "& input": { backgroundColor: "#FFFFFF" } }}
                          />
                        </Grid>
                        <Grid item xs={12} sm={4}>
                          <TextField
                            fullWidth
                            size="small"
                            label="Working Directory"
                            value={systemdWorkingDir}
                            onChange={(e) => setSystemdWorkingDir(e.target.value)}
                            sx={{ "& input": { backgroundColor: "#FFFFFF" } }}
                          />
                        </Grid>
                      </Grid>

                      {systemdError && (
                        <Alert severity="error" sx={{ py: 0.5, fontSize: "0.8rem" }}>
                          {systemdError}
                        </Alert>
                      )}

                      <Box
                        sx={{
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "center",
                          flexWrap: "wrap",
                          gap: 1,
                        }}
                      >
                        <Box sx={{ display: "flex", gap: 0.8, alignItems: "center" }}>
                          <Typography variant="caption" sx={{ color: "#6366F1", fontWeight: 600 }}>
                            Examples:
                          </Typography>
                          <Chip
                            size="small"
                            label="rclone webdav"
                            onClick={() => {
                              setSystemdCmd("rclone serve webdav /root --addr 0.0.0.0:8080");
                              setSystemdDesc("Rclone WebDAV Server");
                            }}
                            sx={{
                              cursor: "pointer",
                              height: 22,
                              fontSize: "0.72rem",
                              bgcolor: "#FFFFFF",
                              border: "1px solid #C7D2FE",
                            }}
                          />
                          <Chip
                            size="small"
                            label="python3 http"
                            onClick={() => {
                              setSystemdCmd("/usr/bin/python3 -m http.server ${PORT:-8080}");
                              setSystemdDesc("Python Simple HTTP Server");
                            }}
                            sx={{
                              cursor: "pointer",
                              height: 22,
                              fontSize: "0.72rem",
                              bgcolor: "#FFFFFF",
                              border: "1px solid #C7D2FE",
                            }}
                          />
                          <Chip
                            size="small"
                            label="coredns"
                            onClick={() => {
                              setSystemdCmd("coredns -conf /etc/coredns/Corefile");
                              setSystemdDesc("CoreDNS DNS Server");
                            }}
                            sx={{
                              cursor: "pointer",
                              height: 22,
                              fontSize: "0.72rem",
                              bgcolor: "#FFFFFF",
                              border: "1px solid #C7D2FE",
                            }}
                          />
                        </Box>

                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<Wand2 size={14} />}
                          onClick={() => {
                            const appName = formName.trim() || "sample";
                            const res = cmdlineToSystemd(systemdCmd, {
                              description: systemdDesc || formName || undefined,
                              user: systemdUser || "nobody",
                              workingDir: systemdWorkingDir || "/var/log",
                              appName: appName,
                              envFile: `/etc/systemd/system/ezapp-${appName}.env`,
                            });
                            if (res.error) {
                              setSystemdError(res.error);
                            } else {
                              setFormMain(res.unitContent);
                              setShowGenerator(false);
                            }
                          }}
                          sx={{ textTransform: "none", bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                        >
                          Generate Unit File
                        </Button>
                      </Box>
                    </>
                  )}
                </Paper>
              )}

              <TextField
                fullWidth
                multiline
                minRows={14}
                maxRows={22}
                value={formMain}
                onChange={(e) => setFormMain(e.target.value)}
                inputProps={{
                  style: {
                    fontFamily: "monospace",
                    fontSize: "0.85rem",
                    lineHeight: 1.5,
                  },
                }}
                sx={{
                  "& .MuiOutlinedInput-root": {
                    bgcolor: "#0F172A",
                    color: "#F8FAFC",
                  },
                }}
              />
            </Box>
          )}

          {/* TAB 3: Environment .env Editor */}
          {modalTab === "env" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                Environment Configuration (.env)
              </Typography>
              <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.82rem" }}>
                Standard KEY=VALUE pairs. Variables like <code>{"${DOMAIN}"}</code> will be substituted automatically.
                Check{" "}
                <a
                  target="_blank"
                  rel="noopener noreferrer"
                  href="https://github.com/fluxcd/pkg/tree/main/envsubst"
                  style={{ textDecoration: "none" }}
                >
                  envsubst
                </a>{" "}
                for more.
              </Typography>

              <TextField
                fullWidth
                multiline
                minRows={12}
                maxRows={20}
                value={formEnv}
                placeholder="KEY=VALUE&#10;PORT=8080&#10;DOMAIN=${DOMAIN}"
                onChange={(e) => setFormEnv(e.target.value)}
                inputProps={{
                  style: {
                    fontFamily: "monospace",
                    fontSize: "0.85rem",
                    lineHeight: 1.5,
                  },
                }}
                sx={{
                  "& .MuiOutlinedInput-root": {
                    bgcolor: "#1E293B",
                    color: "#F1F5F9",
                  },
                }}
              />
            </Box>
          )}

          {/* TAB 4: Config File Map */}
          {modalTab === "files" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Auxiliary Config Files Map
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Injected into app directory (./filename) for Docker or /etc/ezapp-data/&lt;name&gt;/ for native
                    daemons.
                  </Typography>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<Plus size={14} />}
                  onClick={() => setFormFiles([...formFiles, { name: "config.json", content: "" }])}
                  sx={{ textTransform: "none", fontSize: "0.75rem" }}
                >
                  Add File
                </Button>
              </Box>

              {formFiles.length === 0 ? (
                <Paper
                  elevation={0}
                  sx={{
                    p: 4,
                    textAlign: "center",
                    bgcolor: "#F8FAFC",
                    border: "1px dashed #CBD5E1",
                    borderRadius: "10px",
                  }}
                >
                  <Typography variant="body2" sx={{ color: "#64748B" }}>
                    No auxiliary config files defined. If your app needs a JSON, YAML, or TOML file, click "Add File"
                    above.
                  </Typography>
                </Paper>
              ) : (
                formFiles.map((file, idx) => (
                  <Paper key={idx} elevation={0} sx={{ p: 2, border: "1px solid #E2E8F0", borderRadius: "10px" }}>
                    <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", mb: 1.5 }}>
                      <TextField
                        size="small"
                        label="Relative Filename / Path"
                        placeholder="e.g. config.json or settings.yaml"
                        value={file.name}
                        onChange={(e) => {
                          const updated = [...formFiles];
                          updated[idx].name = e.target.value;
                          setFormFiles(updated);
                        }}
                        sx={{ flex: 1 }}
                      />
                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => {
                          const updated = formFiles.filter((_, i) => i !== idx);
                          setFormFiles(updated);
                        }}
                      >
                        <Trash2 size={16} />
                      </IconButton>
                    </Box>
                    <TextField
                      fullWidth
                      multiline
                      minRows={4}
                      maxRows={8}
                      placeholder="File contents..."
                      value={file.content}
                      onChange={(e) => {
                        const updated = [...formFiles];
                        updated[idx].content = e.target.value;
                        setFormFiles(updated);
                      }}
                      inputProps={{
                        style: {
                          fontFamily: "monospace",
                          fontSize: "0.8rem",
                        },
                      }}
                    />
                  </Paper>
                ))
              )}
            </Box>
          )}

          {/* TAB 5: Variables Interpolation Live Preview */}
          {modalTab === "preview" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  flexWrap: "wrap",
                  gap: 2,
                }}
              >
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Live Variable Interpolation Preview
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Select a target node to preview how manifests and configs will be rendered with actual variable
                    values.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 1.5, alignItems: "center" }}>
                  <FormControl size="small" sx={{ minWidth: 170 }}>
                    <InputLabel>Simulated Node</InputLabel>
                    <Select
                      value={previewNodeName}
                      label="Simulated Node"
                      onChange={(e) => {
                        setPreviewNodeName(e.target.value);
                        handleInterpolatePreview(e.target.value);
                      }}
                    >
                      {managedNodes.map((n) => (
                        <MenuItem key={n.name} value={n.name}>
                          {n.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Button
                    size="small"
                    variant="contained"
                    onClick={() => handleInterpolatePreview(previewNodeName)}
                    disabled={loadingInterpolate}
                    sx={{ textTransform: "none", bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                  >
                    {loadingInterpolate ? "Rendering..." : "Render Preview"}
                  </Button>
                </Box>
              </Box>

              <TextField
                fullWidth
                multiline
                minRows={14}
                maxRows={20}
                value={interpolatedPreview || "Click 'Render Preview' above to simulate variable substitution."}
                inputProps={{
                  readOnly: true,
                  style: {
                    fontFamily: "monospace",
                    fontSize: "0.82rem",
                    backgroundColor: "#0F172A",
                    color: "#F8FAFC",
                    lineHeight: 1.5,
                  },
                }}
              />
            </Box>
          )}

          {/* TAB 6: Transpile Preview (Native Systemd only) */}
          {modalTab === "transpile" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Init System Transpilation Preview
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    See how your Systemd service unit will be converted dynamically for non-systemd nodes.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
                  <FormControl size="small" sx={{ minWidth: 150 }}>
                    <Select value={targetInit} onChange={(e) => setTargetInit(e.target.value as "procd" | "openrc")}>
                      <MenuItem value="procd">OpenWrt (Procd)</MenuItem>
                      <MenuItem value="openrc">Alpine (OpenRC)</MenuItem>
                    </Select>
                  </FormControl>
                  <Button
                    size="small"
                    variant="contained"
                    onClick={handleTranspilePreview}
                    disabled={previewing}
                    sx={{ textTransform: "none", bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                  >
                    {previewing ? "Converting..." : "Preview Conversion"}
                  </Button>
                </Box>
              </Box>

              {previewResult?.error && (
                <Alert severity="error" sx={{ mb: 1 }}>
                  {previewResult.error}
                </Alert>
              )}

              {previewResult?.script ? (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#334155" }}>
                    Generated Init Script (/etc/init.d/ezapp-{formName.trim() || "sample"}):
                  </Typography>
                  <TextField
                    fullWidth
                    multiline
                    minRows={10}
                    maxRows={14}
                    value={previewResult.script}
                    inputProps={{
                      readOnly: true,
                      style: {
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        backgroundColor: "#0F172A",
                        color: "#F8FAFC",
                      },
                    }}
                  />
                  {previewResult.conf_d && (
                    <>
                      <Typography variant="caption" sx={{ fontWeight: 700, color: "#334155" }}>
                        Generated Conf.d (/etc/conf.d/ezapp-{formName.trim() || "sample"}):
                      </Typography>
                      <TextField
                        fullWidth
                        multiline
                        minRows={4}
                        maxRows={6}
                        value={previewResult.conf_d}
                        inputProps={{
                          readOnly: true,
                          style: {
                            fontFamily: "monospace",
                            fontSize: "0.82rem",
                            backgroundColor: "#1E293B",
                            color: "#F1F5F9",
                          },
                        }}
                      />
                    </>
                  )}
                </Box>
              ) : (
                <Paper
                  elevation={0}
                  sx={{
                    p: 4,
                    textAlign: "center",
                    bgcolor: "#F8FAFC",
                    border: "1px dashed #CBD5E1",
                    borderRadius: "10px",
                  }}
                >
                  <Typography variant="body2" sx={{ color: "#64748B" }}>
                    Click "Preview Conversion" above to inspect the transpiled OpenRC or Procd init script.
                  </Typography>
                </Paper>
              )}
            </Box>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, pb: 2.5, pt: 1.5, borderTop: "1px solid #E2E8F0" }}>
          {editingApp && (
            <Button
              startIcon={
                duplicatingAppId === editingApp.id ? (
                  <CircularProgress size={16} sx={{ color: "inherit" }} />
                ) : (
                  <Copy size={16} />
                )
              }
              onClick={async () => {
                await handleDuplicateApp(editingApp);
                setModalOpen(false);
              }}
              disabled={duplicatingAppId === editingApp.id}
              sx={{ textTransform: "none", color: "#4F46E5", mr: "auto", fontWeight: 600 }}
            >
              Duplicate App
            </Button>
          )}
          <Button onClick={() => setModalOpen(false)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSaveApp}
            disabled={savingApp}
            sx={{
              bgcolor: "#4F46E5",
              fontWeight: 600,
              textTransform: "none",
              px: 3,
              "&:hover": { bgcolor: "#4338CA" },
            }}
          >
            {savingApp ? (
              <CircularProgress size={18} sx={{ color: "#FFFFFF" }} />
            ) : editingApp ? (
              "Save Changes"
            ) : (
              "Deploy App"
            )}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
