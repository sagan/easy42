import React, { useState, useEffect, useMemo, useCallback, useRef } from "react";
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
  Menu,
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
  TableSortLabel,
  Radio,
  RadioGroup,
  Checkbox,
  InputAdornment,
  ToggleButton,
  ToggleButtonGroup,
  Autocomplete,
} from "@mui/material";
import {
  Layers,
  Bookmark,
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
  Clock,
  LayoutGrid,
  List,
  ExternalLink,
  Package,
  HardDrive,
  ShieldCheck,
} from "lucide-react";
import { api } from "../api/client";
import { useMesh } from "../context/MeshContext";
import { Link as RouterLink, useSearchParams, useNavigate, useParams } from "react-router-dom";
import type {
  App,
  AppVariable,
  TranspilePreviewResponse,
  ArtifactFile,
  ArtifactSpec,
  AppArtifactStatus,
} from "../types/api";
import { dockerRunToCompose, cmdlineToSystemd, ensureSystemdEnvironmentFile } from "../utils/appGenerators";
import { MarkdownView } from "../components/Common/MarkdownView";
import { AppDetailsDrawer } from "../components/Apps/AppDetailsDrawer";
import {
  matchesTaskSelector,
  splitSelectorList,
  getSelectorValue,
  detectTargetMode,
  parseSelectorJSON,
  type SelectorTargetMode,
} from "../utils/taskSelector";
import { resolveHomepageUrl, isHttpUrl } from "../utils/appUrl";
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
User=root
WorkingDirectory=/var/log
EnvironmentFile=/etc/systemd/system/ezapp-sample.env
ExecStart=/usr/bin/python3 -m http.server \${PORT:-8080}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`;

export function parseCronMainContent(mainContent: string): { cronExpr: string; cronCmd: string } {
  const trimmed = (mainContent || "").trim();
  if (!trimmed) {
    return { cronExpr: "0 0 * * *", cronCmd: "" };
  }
  if (trimmed.startsWith("@")) {
    const parts = trimmed.split(/\s+/);
    return {
      cronExpr: parts[0] || "@daily",
      cronCmd: parts.slice(1).join(" "),
    };
  }
  const parts = trimmed.split(/\s+/);
  if (parts.length >= 6) {
    return {
      cronExpr: parts.slice(0, 5).join(" "),
      cronCmd: parts.slice(5).join(" "),
    };
  }
  return {
    cronExpr: parts.slice(0, Math.min(parts.length, 5)).join(" "),
    cronCmd: parts.slice(Math.min(parts.length, 5)).join(" "),
  };
}

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
    key: "NODE_DATA_",
    label: "${NODE_DATA_*}",
    tooltip: "The node data variables extracted from node Note markdown ``` block",
  },
  {
    key: "EZAPP_DIR",
    label: "${EZAPP_DIR}",
    tooltip: "App managed directory (/etc/ezapp/<name> for native, /etc/ezappc/ezapp-<name> for docker)",
  },
  {
    key: "ARCH",
    label: "${ARCH}",
    tooltip: "Target node Go architecture (amd64, arm64, 386)",
  },
  {
    key: "ARCH_GNU",
    label: "${ARCH_GNU}",
    tooltip: "Target node GNU/C/Rust architecture (x86_64, aarch64, i386)",
  },
];

type AppTargetMode = SelectorTargetMode;

type AppSortField = "name" | "type" | "published" | "enabled";
const APP_SORT_FIELDS: AppSortField[] = ["name", "type", "published", "enabled"];
const APP_TYPE_FILTERS = ["all", "docker", "systemd", "cron"] as const;

export const AppsPage: React.FC = () => {
  const { nodes, setStateToast } = useMesh();

  // Top-level Navigation: separated URLs /apps (Workload Apps) and /apps/variables (Global Variables)
  const { tab: tabParam } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const activeTab: "apps" | "variables" = tabParam === "variables" ? "variables" : "apps";

  const handleTabChange = (_: React.SyntheticEvent, newValue: string) => {
    if (newValue === activeTab) return;
    navigate(newValue === "variables" ? "/apps/variables" : "/apps");
  };

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

  // Apps Filters & Search (synced to URL query parameters via replaceState)
  const [searchParams, setSearchParams] = useSearchParams();
  const urlSearch = searchParams.get("search") || "";
  const rawUrlType = searchParams.get("type") || "all";
  const typeFilter: string = (APP_TYPE_FILTERS as readonly string[]).includes(rawUrlType) ? rawUrlType : "all";
  const rawUrlNode = searchParams.get("node") || "all";
  const groupFilter = searchParams.get("group") || "all";
  const view: "cards" | "table" = searchParams.get("view") === "table" ? "table" : "cards";
  const rawSort = searchParams.get("sort");
  const sortField = (APP_SORT_FIELDS as string[]).includes(rawSort || "")
    ? (rawSort as AppSortField)
    : ("name" as AppSortField);
  const sortOrder = (searchParams.get("order") === "desc" ? "desc" : "asc") as "asc" | "desc";

  // Local search terms (per-tab, synced to that tab's `?search` via replaceState)
  // `?search` means app search on /apps and variable search on /apps/variables.
  const [search, setSearch] = useState(() => (activeTab === "apps" ? urlSearch : ""));
  const [varSearch, setVarSearch] = useState(() => (activeTab === "variables" ? urlSearch : ""));

  // Normalize unknown sub-paths and legacy ?tab= params to canonical separated URLs
  useEffect(() => {
    if (tabParam && tabParam !== "variables") {
      navigate("/apps", { replace: true });
      return;
    }
    const queryTab = searchParams.get("tab");
    if (!queryTab) return;
    if (queryTab === "variables" && activeTab !== "variables") {
      setVarSearch(urlSearch);
      navigate(`/apps/variables${urlSearch ? `?search=${encodeURIComponent(urlSearch)}` : ""}`, { replace: true });
    } else if (queryTab === "apps" && activeTab !== "apps") {
      setSearch(urlSearch);
      navigate(`/apps${urlSearch ? `?search=${encodeURIComponent(urlSearch)}` : ""}`, { replace: true });
    } else if (queryTab !== "variables" && queryTab !== "apps") {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("tab");
          return next;
        },
        { replace: true },
      );
    } else {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.delete("tab");
          return next;
        },
        { replace: true },
      );
    }
  }, [tabParam, searchParams, activeTab, navigate, setSearchParams, urlSearch]);

  // Synchronize local search term if URL search changes externally (back/forward),
  // skipping the tab-switch itself so each tab keeps its own search value
  const prevTabRef = useRef(activeTab);
  useEffect(() => {
    if (prevTabRef.current !== activeTab) {
      prevTabRef.current = activeTab;
      return;
    }
    if (activeTab === "apps") {
      setSearch(urlSearch);
    } else {
      setVarSearch(urlSearch);
    }
  }, [urlSearch, activeTab]);

  // Debounce app search updates to URL (replaceState, like NodesPage) — apps tab only
  useEffect(() => {
    if (activeTab !== "apps") return;
    const timer = setTimeout(() => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          const trimmed = search.trim();
          if (trimmed) {
            if (next.get("search") !== trimmed) next.set("search", trimmed);
          } else {
            next.delete("search");
          }
          return next;
        },
        { replace: true },
      );
    }, 250);
    return () => clearTimeout(timer);
  }, [search, activeTab, setSearchParams]);

  // Debounce variable search updates to URL (replaceState) — variables tab only
  useEffect(() => {
    if (activeTab !== "variables") return;
    const timer = setTimeout(() => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          const trimmed = varSearch.trim();
          if (trimmed) {
            if (next.get("search") !== trimmed) next.set("search", trimmed);
          } else {
            next.delete("search");
          }
          return next;
        },
        { replace: true },
      );
    }, 250);
    return () => clearTimeout(timer);
  }, [varSearch, activeTab, setSearchParams]);

  // Helper to update URL query parameters (replaceState)
  const updateQueryParam = useCallback(
    (updates: { type?: string; node?: string; view?: "cards" | "table"; group?: string }) => {
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (updates.type !== undefined) {
            if (updates.type && updates.type !== "all") {
              next.set("type", updates.type);
            } else {
              next.delete("type");
            }
          }
          if (updates.node !== undefined) {
            if (updates.node && updates.node !== "all") {
              next.set("node", updates.node);
            } else {
              next.delete("node");
            }
          }
          if (updates.group !== undefined) {
            if (updates.group && updates.group !== "all") {
              next.set("group", updates.group);
            } else {
              next.delete("group");
            }
          }
          if (updates.view !== undefined) {
            if (updates.view && updates.view !== "cards") {
              next.set("view", updates.view);
            } else {
              next.delete("view");
            }
          }
          return next;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  const handleSelectType = (type: string) => updateQueryParam({ type });
  const handleSelectNode = (node: string) => updateQueryParam({ node });
  const handleSelectGroup = (group: string) => updateQueryParam({ group: group === groupFilter ? "all" : group });
  const handleToggleView = (v: "cards" | "table") => updateQueryParam({ view: v });

  const handleRequestSort = useCallback(
    (field: AppSortField) => {
      const isAsc = sortField === field && sortOrder === "asc";
      const newOrder = isAsc ? "desc" : "asc";
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          next.set("sort", field);
          next.set("order", newOrder);
          return next;
        },
        { replace: true },
      );
    },
    [sortField, sortOrder, setSearchParams],
  );

  const handleClearFilters = () => {
    setSearch("");
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.delete("search");
        next.delete("type");
        next.delete("node");
        next.delete("group");
        return next;
      },
      { replace: true },
    );
  };

  // App Modal State
  const [modalOpen, setModalOpen] = useState(false);
  const [editingApp, setEditingApp] = useState<App | null>(null);
  const [modalTab, setModalTab] = useState<
    "general" | "definition" | "env" | "files" | "note" | "preview" | "transpile" | "artifact"
  >("general");

  // App Details Drawer State
  const [detailsApp, setDetailsApp] = useState<App | null>(null);

  // Note editor sub-state (write / preview)
  const [noteEditMode, setNoteEditMode] = useState<"write" | "preview">("write");

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
  const [formGroup, setFormGroup] = useState("");
  const [formNote, setFormNote] = useState("");
  const [formUrl, setFormUrl] = useState("");
  const [formType, setFormType] = useState<"docker" | "systemd" | "cron">("docker");
  const [formMain, setFormMain] = useState(DOCKER_SAMPLE);
  const [formCronExpr, setFormCronExpr] = useState("0 0 * * *");
  const [formCronCmd, setFormCronCmd] = useState("");
  const [formEnv, setFormEnv] = useState("PORT=80\nDOMAIN=\${DOMAIN:-localhost}\n");
  const [formTargetType, setFormTargetType] = useState<AppTargetMode>("manual");
  const [formSelector, setFormSelector] = useState<Record<string, string>>({ name: "" });
  const [formSelectorText, setFormSelectorText] = useState<string>('{\n  "name": ""\n}');
  const [formSelectorError, setFormSelectorError] = useState<string | null>(null);
  const [formPublished, setFormPublished] = useState(true);
  const [formEnabled, setFormEnabled] = useState(true);
  const [togglingPublishedAppIds, setTogglingPublishedAppIds] = useState<Record<string, boolean>>({});
  const [formFiles, setFormFiles] = useState<{ name: string; content: string }[]>([]);

  // Artifact Form Fields (Native Systemd apps)
  const [formVersion, setFormVersion] = useState("");
  const [formArtifactEnabled, setFormArtifactEnabled] = useState(false);
  const [formArtifactSourceMode, setFormArtifactSourceMode] = useState<"url" | "github">("url");
  const [formArtifactUrl, setFormArtifactUrl] = useState("");
  const [formArtifactGithubRepo, setFormArtifactGithubRepo] = useState("");
  const [formArtifactGithubVersion, setFormArtifactGithubVersion] = useState("");
  const [formArtifactGithubPattern, setFormArtifactGithubPattern] = useState("");
  const [formArtifactFiles, setFormArtifactFiles] = useState<ArtifactFile[]>([
    {
      name: "",
      extract_path: "",
      target_path: "${PATH}/",
      pull_policy: "if_changed",
      file_mode: 0o755,
    },
  ]);
  const [formArtifactChecksums, setFormArtifactChecksums] = useState<Record<string, string>>({});
  const [formArtifactArchMapping, setFormArtifactArchMapping] = useState<Record<string, string>>({});

  // Artifact Cache & Validation States
  const [artifactStatusMap, setArtifactStatusMap] = useState<Record<string, AppArtifactStatus>>({});
  const [loadingArtifactStatus, setLoadingArtifactStatus] = useState<Record<string, boolean>>({});
  const [validatingArtifactId, setValidatingArtifactId] = useState<string | null>(null);
  const [clearingArtifactId, setClearingArtifactId] = useState<string | null>(null);
  const [artifactCacheModalApp, setArtifactCacheModalApp] = useState<App | null>(null);
  const [artifactToolbarMenuAnchor, setArtifactToolbarMenuAnchor] = useState<null | HTMLElement>(null);
  const [copiedSha, setCopiedSha] = useState<string | null>(null);

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
  const [systemdUser, setSystemdUser] = useState("root");
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

  // Validated node filter for display + filtering (unknown names fall back to "all")
  const isValidNodeFilter = rawUrlNode === "all" || managedNodes.some((n) => n.name === rawUrlNode);
  const nodeFilter = isValidNodeFilter ? rawUrlNode : "all";

  // Target-node resolution shared by card + table views
  const getTargetNodeInfo = (app: App) => {
    const selector = app.selector || {};
    const mode = detectTargetMode(selector);
    const manualNodes = splitSelectorList(getSelectorValue(selector, "name"));
    const matchingNodes = managedNodes.filter((n) => matchesTaskSelector(n, selector));
    const nodeNames = mode === "manual" ? manualNodes : matchingNodes.map((n) => n.name);
    return { selector, mode, manualNodes, matchingNodes, nodeNames };
  };

  const targetChipColors = (mode: SelectorTargetMode) =>
    mode === "tag"
      ? { bgcolor: "#EDE9FE", color: "#5B21B6", hoverBg: "#DDD6FE", hoverColor: "#4C1D95" }
      : mode === "custom"
        ? { bgcolor: "#CCFBF1", color: "#0F766E", hoverBg: "#99F6E4", hoverColor: "#115E59" }
        : mode === "manual"
          ? { bgcolor: "#F1F5F9", color: "#334155", hoverBg: "#E2E8F0", hoverColor: "#1E293B" }
          : { bgcolor: "#E0F2FE", color: "#0369A1", hoverBg: "#BAE6FD", hoverColor: "#0284C7" };

  const availableTags = useMemo(() => {
    const s = new Set<string>();
    nodes.forEach((n) => {
      n.tags?.forEach((t) => {
        if (t.trim()) s.add(t.trim());
      });
    });
    return Array.from(s).sort();
  }, [nodes]);

  const formSelectedNodes = splitSelectorList(getSelectorValue(formSelector, "name"));
  const formTargetTag = getSelectorValue(formSelector, "tag");
  const selectorMatchedNodes = managedNodes.filter((n) => matchesTaskSelector(n, formSelector));

  const applyFormSelector = (selector: Record<string, string>) => {
    setFormSelector(selector);
    setFormSelectorText(JSON.stringify(selector, null, 2));
    setFormSelectorError(null);
    setFormTargetType(detectTargetMode(selector));
  };

  const handleSelectorTextChange = (text: string) => {
    setFormSelectorText(text);
    const parsed = parseSelectorJSON(text);
    if (parsed.error || !parsed.selector) {
      setFormSelectorError(parsed.error);
      return;
    }
    setFormSelectorError(null);
    setFormSelector(parsed.selector);
    setFormTargetType(detectTargetMode(parsed.selector));
  };

  const handleTargetModeChange = (mode: AppTargetMode) => {
    if (mode === "manual") {
      applyFormSelector({ name: getSelectorValue(formSelector, "name") });
    } else if (mode === "all") {
      applyFormSelector({});
    } else if (mode === "tag") {
      applyFormSelector({ tag: getSelectorValue(formSelector, "tag") });
    } else {
      setFormTargetType("custom");
    }
  };

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
    if (previewNodeName || managedNodes.length === 0) return;
    const match = managedNodes.find((n) => matchesTaskSelector(n, formSelector));
    setPreviewNodeName((match || managedNodes[0]).name);
  }, [formSelector, managedNodes, previewNodeName]);

  const openCreateAppModal = () => {
    setEditingApp(null);
    setFormName("");
    setFormDesc("");
    setFormGroup("");
    setFormNote("");
    setNoteEditMode("write");
    setFormUrl("");
    setFormVersion("");
    setFormArtifactEnabled(false);
    setFormArtifactSourceMode("url");
    setFormArtifactUrl("");
    setFormArtifactGithubRepo("");
    setFormArtifactGithubVersion("");
    setFormArtifactGithubPattern("");
    setFormArtifactFiles([
      {
        name: "",
        extract_path: "",
        target_path: "${PATH}/",
        pull_policy: "if_changed",
        file_mode: 0o755,
      },
    ]);
    setFormArtifactChecksums({});
    setFormArtifactArchMapping({});
    setFormType("docker");
    setFormMain(DOCKER_SAMPLE);
    setFormCronExpr("0 0 * * *");
    setFormCronCmd("");
    setFormEnv("DIR=${EZAPP_DIR}\nIP=${NODE_IP}\n");
    setFormTargetType("manual");
    setFormSelector({ name: "" });
    setFormSelectorText('{\n  "name": ""\n}');
    setFormSelectorError(null);
    setFormPublished(true);
    setFormEnabled(true);
    setFormFiles([]);
    setPreviewResult(null);
    setShowGenerator(false);
    setDockerRunCmd("");
    setDockerRunError(null);
    setSystemdCmd("");
    setSystemdDesc("");
    setSystemdUser("root");
    setSystemdWorkingDir("/var/log");
    setSystemdError(null);
    setModalTab("general");
    setModalOpen(true);
  };

  const openEditAppModal = (app: App) => {
    setEditingApp(app);
    setFormName(app.name);
    setFormDesc(app.description || "");
    setFormGroup(app.group || "");
    setFormVersion(app.version || "");
    if (app.artifact) {
      setFormArtifactEnabled(true);
      if (app.artifact.resolver) {
        setFormArtifactSourceMode("github");
        setFormArtifactGithubRepo(app.artifact.resolver.repo || "");
        setFormArtifactGithubVersion(app.artifact.resolver.version || "");
        setFormArtifactGithubPattern(app.artifact.resolver.asset_pattern || "");
        setFormArtifactUrl(app.artifact.url || "");
      } else {
        setFormArtifactSourceMode("url");
        setFormArtifactUrl(app.artifact.url || "");
        setFormArtifactGithubRepo("");
        setFormArtifactGithubVersion("");
        setFormArtifactGithubPattern("");
      }
      setFormArtifactFiles(
        app.artifact.files && app.artifact.files.length > 0
          ? app.artifact.files.map((f) => ({ ...f }))
          : [
              {
                name: app.name,
                extract_path: "",
                target_path: "${PATH}/" + app.name,
                pull_policy: "if_changed",
                file_mode: 0o755,
              },
            ],
      );
      setFormArtifactChecksums(app.artifact.checksums || {});
      setFormArtifactArchMapping(app.artifact.arch_mapping || {});
    } else {
      setFormArtifactEnabled(false);
      setFormArtifactSourceMode("url");
      setFormArtifactUrl("");
      setFormArtifactGithubRepo("");
      setFormArtifactGithubVersion("");
      setFormArtifactGithubPattern("");
      setFormArtifactFiles([
        {
          name: app.name,
          extract_path: "",
          target_path: "${PATH}/" + app.name,
          pull_policy: "if_changed",
          file_mode: 0o755,
        },
      ]);
      setFormArtifactChecksums({});
      setFormArtifactArchMapping({});
    }
    setFormNote(app.note || "");
    setNoteEditMode("write");
    setFormUrl(app.url || "");
    setFormType(app.app_type);
    setFormMain(app.main_content);
    if (app.app_type === "cron") {
      const parsed = parseCronMainContent(app.main_content);
      setFormCronExpr(parsed.cronExpr);
      setFormCronCmd(parsed.cronCmd);
    } else {
      setFormCronExpr("0 0 * * *");
      setFormCronCmd("");
    }
    setFormEnv(app.env_content || "");
    const selector = app.selector || {};
    setFormSelector(selector);
    setFormSelectorText(JSON.stringify(selector, null, 2));
    setFormSelectorError(null);
    setFormTargetType(detectTargetMode(selector));
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
    setSystemdUser("root");
    setSystemdWorkingDir("/var/log");
    setSystemdError(null);
    setModalTab("general");
    setModalOpen(true);
  };

  const handleTypeChange = (newType: "docker" | "systemd" | "cron") => {
    setFormType(newType);
    if (
      newType === "cron" &&
      (modalTab === "env" || modalTab === "files" || modalTab === "transpile" || modalTab === "artifact")
    ) {
      setModalTab("definition");
    } else if (newType !== "systemd" && (modalTab === "artifact" || modalTab === "transpile")) {
      setModalTab("definition");
    }
    if (!editingApp) {
      if (newType === "docker") {
        setFormMain(DOCKER_SAMPLE);
      } else if (newType === "systemd") {
        setFormMain(SYSTEMD_SAMPLE);
      } else {
        const cronStr = `${formCronExpr.trim()} ${formCronCmd.trim()}`.trim() || "0 0 * * *";
        setFormMain(cronStr);
      }
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

      const parsedSelector = parseSelectorJSON(formSelectorText);
      if (parsedSelector.error || !parsedSelector.selector) {
        alert(parsedSelector.error || "Invalid selector JSON");
        setSavingApp(false);
        return;
      }

      if (formType === "cron") {
        if (!formCronExpr.trim()) {
          alert("Cron expression cannot be empty");
          setSavingApp(false);
          return;
        }
        if (!formCronCmd.trim()) {
          alert("Command line cannot be empty");
          setSavingApp(false);
          return;
        }
      }

      const resolvedMainContent =
        formType === "systemd"
          ? ensureSystemdEnvironmentFile(formName.trim(), formMain)
          : formType === "cron"
            ? `${formCronExpr.trim()} ${formCronCmd.trim()}`
            : formMain;

      let artifactPayload: ArtifactSpec | undefined = undefined;
      if (formType === "systemd" && formArtifactEnabled) {
        const validFiles = formArtifactFiles.filter((f) => f.name.trim() && f.target_path.trim());
        if (validFiles.length === 0) {
          alert("Artifact is enabled: please specify at least one file to extract with name and target path");
          setSavingApp(false);
          return;
        }
        if (formArtifactSourceMode === "url" && !formArtifactUrl.trim()) {
          alert("Artifact is enabled: URL template cannot be empty");
          setSavingApp(false);
          return;
        }
        if (formArtifactSourceMode === "github" && !formArtifactGithubRepo.trim()) {
          alert("Artifact is enabled: GitHub repo (owner/repo) cannot be empty");
          setSavingApp(false);
          return;
        }

        artifactPayload = {
          files: validFiles.map((f) => ({
            name: f.name.trim(),
            extract_path: f.extract_path?.trim() || undefined,
            target_path: f.target_path.trim(),
            pull_policy: f.pull_policy || "if_changed",
            file_mode: Number(f.file_mode) || 0o755,
          })),
        };

        if (formArtifactSourceMode === "url") {
          artifactPayload.url = formArtifactUrl.trim();
        } else {
          artifactPayload.resolver = {
            type: "github_release",
            repo: formArtifactGithubRepo.trim(),
            version: formArtifactGithubVersion.trim() || undefined,
            asset_pattern: formArtifactGithubPattern.trim() || undefined,
          };
        }

        const validChecksums: Record<string, string> = {};
        for (const [k, v] of Object.entries(formArtifactChecksums)) {
          if (k.trim() && v.trim()) validChecksums[k.trim()] = v.trim();
        }
        if (Object.keys(validChecksums).length > 0) {
          artifactPayload.checksums = validChecksums;
        }

        const validArchMapping: Record<string, string> = {};
        for (const [k, v] of Object.entries(formArtifactArchMapping)) {
          if (k.trim() && v.trim()) validArchMapping[k.trim()] = v.trim();
        }
        if (Object.keys(validArchMapping).length > 0) {
          artifactPayload.arch_mapping = validArchMapping;
        }
      }

      const payload: Partial<App> = {
        name: formName.trim(),
        description: formDesc.trim(),
        group: formGroup.trim() || undefined,
        version: formVersion.trim() || undefined,
        note: formNote,
        url: formUrl.trim(),
        app_type: formType,
        main_content: resolvedMainContent,
        env_content: formType === "cron" ? "" : formEnv,
        files: formType === "cron" ? {} : filesMap,
        artifact: formType === "systemd" ? artifactPayload : undefined,
        selector: parsedSelector.selector,
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
      let previewContent = "";
      if (formType === "cron") {
        const fullCron = `${formCronExpr.trim()} ${formCronCmd.trim()}`.trim();
        previewContent = `--- [cron schedule & command] ---\n${fullCron}\n\n--- [installed crontab entry] ---\n${formEnabled ? fullCron : `#${fullCron}`}`;
      } else {
        previewContent = `--- [compose / service manifest] ---\n${formMain}\n\n--- [.env] ---\n${formEnv}`;
      }
      const res = await api.previewInterpolate({
        content: previewContent,
        node_name: nodeName,
        app_name: formName.trim() || "sample",
        app_type: formType,
      });
      setInterpolatedPreview(res.rendered);
    } catch (e: any) {
      setInterpolatedPreview(`Error previewing interpolation: ${e.message}`);
    } finally {
      setLoadingInterpolate(false);
    }
  };

  const fetchAppArtifactStatus = useCallback(async (appId: string) => {
    try {
      setLoadingArtifactStatus((prev) => ({ ...prev, [appId]: true }));
      const status = await api.getAppArtifactStatus(appId);
      setArtifactStatusMap((prev) => ({ ...prev, [appId]: status }));
      return status;
    } catch (err) {
      console.error("Failed to load artifact status for app", appId, err);
    } finally {
      setLoadingArtifactStatus((prev) => ({ ...prev, [appId]: false }));
    }
  }, []);

  const handleValidateAppArtifact = async (app: App) => {
    try {
      setValidatingArtifactId(app.id);
      const res = await api.validateAppArtifact(app.id);
      await fetchAppArtifactStatus(app.id);
      setStateToast({
        message: res.message || `Artifact for "${app.name}" validated.`,
        severity: res.updated ? "success" : "info",
      });
    } catch (err: any) {
      setStateToast({
        message: err.message || "Failed to validate artifact with origin",
        severity: "error",
      });
    } finally {
      setValidatingArtifactId(null);
    }
  };

  const handleClearAppArtifactCache = async (app: App) => {
    try {
      setClearingArtifactId(app.id);
      const res = await api.clearAppArtifactCache(app.id);
      await fetchAppArtifactStatus(app.id);
      setStateToast({
        message: res.message || `Server cache cleared for "${app.name}". Metadata preserved for node sync.`,
        severity: "success",
      });
    } catch (err: any) {
      setStateToast({
        message: err.message || "Failed to clear server artifact cache",
        severity: "error",
      });
    } finally {
      setClearingArtifactId(null);
    }
  };

  const handleValidateAllArtifacts = async () => {
    setArtifactToolbarMenuAnchor(null);
    try {
      setStateToast({
        message: "Checking origin sources for all app artifacts...",
        severity: "info",
      });
      const results = await api.validateAllAppArtifacts();
      const updatedCount = results.filter((r) => r.updated).length;
      setStateToast({
        message: `Validated ${results.length} artifact(s). ${updatedCount} updated from origin.`,
        severity: "success",
      });
      for (const a of apps) {
        if (a.artifact) {
          fetchAppArtifactStatus(a.id);
        }
      }
    } catch (err: any) {
      setStateToast({
        message: err.message || "Failed to validate all artifacts",
        severity: "error",
      });
    }
  };

  const handleClearAllArtifactCache = async () => {
    setArtifactToolbarMenuAnchor(null);
    try {
      const res = await api.clearAllAppArtifactCache();
      setStateToast({
        message: res.message || "All cached artifact files cleared from server. Metadata preserved for node sync.",
        severity: "success",
      });
      for (const a of apps) {
        if (a.artifact) {
          fetchAppArtifactStatus(a.id);
        }
      }
    } catch (err: any) {
      setStateToast({
        message: err.message || "Failed to clear all artifact caches",
        severity: "error",
      });
    }
  };

  const handleCopySha = (sha: string) => {
    navigator.clipboard.writeText(sha);
    setCopiedSha(sha);
    setTimeout(() => setCopiedSha(null), 2000);
  };

  useEffect(() => {
    if (modalOpen && modalTab === "artifact" && editingApp && editingApp.artifact) {
      fetchAppArtifactStatus(editingApp.id);
    }
  }, [modalOpen, modalTab, editingApp, fetchAppArtifactStatus]);

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
      if (search.trim()) {
        const q = search.trim().toLowerCase();
        const matchName = a.name.toLowerCase().includes(q);
        const matchDesc = a.description?.toLowerCase().includes(q);
        const matchNote = a.note?.toLowerCase().includes(q);
        const matchGroup = a.group?.toLowerCase().includes(q);
        if (!matchName && !matchDesc && !matchNote && !matchGroup) return false;
      }
      if (typeFilter !== "all" && a.app_type !== typeFilter) {
        return false;
      }
      if (nodeFilter !== "all") {
        const nodeObj = nodes.find((n) => n.name === nodeFilter);
        if (!nodeObj || !matchesTaskSelector(nodeObj, a.selector)) return false;
      }
      if (groupFilter !== "all") {
        if (groupFilter === "ungrouped") {
          if (a.group && a.group.trim()) return false;
        } else {
          if ((a.group || "").trim().toLowerCase() !== groupFilter.trim().toLowerCase()) return false;
        }
      }
      return true;
    });
  }, [apps, search, typeFilter, nodeFilter, groupFilter, nodes]);

  const appGroups = useMemo(() => {
    const set = new Set<string>();
    apps.forEach((a) => {
      if (a.group && a.group.trim()) {
        set.add(a.group.trim());
      }
    });
    return Array.from(set).sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
  }, [apps]);

  const ungroupedAppsCount = useMemo(() => {
    return apps.filter((a) => !a.group || !a.group.trim()).length;
  }, [apps]);

  // Sorted apps for the list (table) view; card view keeps API order
  const sortedApps = useMemo(() => {
    if (view !== "table") return filteredApps;
    const list = [...filteredApps];
    list.sort((a, b) => {
      let comparison = 0;
      switch (sortField) {
        case "name":
          comparison = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
          break;
        case "type":
          comparison = a.app_type.localeCompare(b.app_type);
          if (comparison === 0) comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
          break;
        case "published":
          comparison = Number(Boolean(a.published)) - Number(Boolean(b.published));
          if (comparison === 0) comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
          break;
        case "enabled":
          comparison = Number(a.enabled) - Number(b.enabled);
          if (comparison === 0) comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
          break;
        default:
          comparison = a.name.localeCompare(b.name, undefined, { numeric: true });
      }
      return sortOrder === "asc" ? comparison : -comparison;
    });
    return list;
  }, [filteredApps, view, sortField, sortOrder]);

  const isFiltering = Boolean(search.trim() || typeFilter !== "all" || nodeFilter !== "all" || groupFilter !== "all");

  const filteredVars = useMemo(() => {
    return variables.filter((v) => {
      if (!varSearch.trim()) return true;
      const q = varSearch.trim().toLowerCase();
      return v.key.toLowerCase().includes(q) || v.description?.toLowerCase().includes(q);
    });
  }, [variables, varSearch]);

  const dockerCount = apps.filter((a) => a.app_type === "docker").length;
  const systemdCount = apps.filter((a) => a.app_type === "systemd").length;
  const cronCount = apps.filter((a) => a.app_type === "cron").length;
  const enabledCount = apps.filter((a) => a.enabled).length;

  const renderArtifactCacheCard = (app: App) => {
    const status = artifactStatusMap[app.id];
    const isLoading = loadingArtifactStatus[app.id];
    const isValidating = validatingArtifactId === app.id;
    const isClearing = clearingArtifactId === app.id;

    return (
      <Paper
        elevation={0}
        sx={{
          p: 2.5,
          borderRadius: "14px",
          bgcolor: "#F8FAFC",
          border: "1px solid #E2E8F0",
          display: "flex",
          flexDirection: "column",
          gap: 2,
        }}
      >
        {/* Header with Title and Status Badges */}
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 1.5 }}>
          <Box>
            <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}>
              <HardDrive size={18} color="#4F46E5" />
              Server Artifact Cache & Storage
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.5 }}>
              Cached under <code>~/.config/easy42/cache/artifacts</code>. Metadata is stored in a separate persistent cache meta file so disk files can be purged while node sync continues seamlessly.
            </Typography>
          </Box>

          {/* Badges & Refresh */}
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {isLoading ? (
              <CircularProgress size={16} />
            ) : status?.files_on_disk ? (
              <Chip
                size="small"
                icon={<HardDrive size={12} />}
                label={`Cached on Server (${status.disk_size_human})`}
                sx={{
                  height: 24,
                  fontSize: "0.72rem",
                  fontWeight: 700,
                  bgcolor: "#DCFCE7",
                  color: "#15803D",
                  "& .MuiChip-icon": { color: "#15803D" },
                }}
              />
            ) : status?.meta_exists ? (
              <Chip
                size="small"
                icon={<ShieldCheck size={12} />}
                label="Metadata Preserved (Disk Cleared)"
                sx={{
                  height: 24,
                  fontSize: "0.72rem",
                  fontWeight: 700,
                  bgcolor: "#FEF3C7",
                  color: "#B45309",
                  "& .MuiChip-icon": { color: "#B45309" },
                }}
              />
            ) : (
              <Chip
                size="small"
                label="Not Cached on Server"
                sx={{
                  height: 24,
                  fontSize: "0.72rem",
                  fontWeight: 600,
                  bgcolor: "#F1F5F9",
                  color: "#64748B",
                }}
              />
            )}
            <Tooltip title="Refresh Cache Status">
              <IconButton size="small" onClick={() => fetchAppArtifactStatus(app.id)} disabled={isLoading} sx={{ p: 0.5 }}>
                <RefreshCw size={13} className={isLoading ? "spin" : ""} />
              </IconButton>
            </Tooltip>
          </Box>
        </Box>

        {/* Metadata Details Grid */}
        <Box
          sx={{
            bgcolor: "#FFFFFF",
            borderRadius: "10px",
            p: 1.5,
            border: "1px solid #EDF2F7",
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "repeat(4, 1fr)" },
            gap: 1.5,
          }}
        >
          <Box>
            <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase", fontSize: "0.65rem" }}>
              App / Version
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 700, color: "#1E293B", mt: 0.2 }}>
              {app.name} ({app.version || "latest"})
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase", fontSize: "0.65rem" }}>
              Server Disk Usage
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 700, color: status?.files_on_disk ? "#15803D" : "#64748B", mt: 0.2 }}>
              {status?.disk_size_human || "0 B"}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase", fontSize: "0.65rem" }}>
              Last Origin Check
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600, color: "#475569", mt: 0.2 }}>
              {status?.last_checked ? new Date(status.last_checked).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : "Never"}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase", fontSize: "0.65rem" }}>
              Last Validated
            </Typography>
            <Typography variant="body2" sx={{ fontWeight: 600, color: "#475569", mt: 0.2 }}>
              {status?.last_validated ? new Date(status.last_validated).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : "Never"}
            </Typography>
          </Box>
        </Box>

        {/* Extracted Binaries and SHA-256 Hashes Breakdown */}
        {status?.versions && Object.keys(status.versions).length > 0 ? (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
              Cached Targets & Extracted Binaries:
            </Typography>
            {Object.entries(status.versions).map(([verKey, verStatus]) => (
              <Box key={verKey} sx={{ bgcolor: "#FFFFFF", borderRadius: "8px", p: 1.5, border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1, flexWrap: "wrap" }}>
                  <Chip size="small" label={`Version: ${verKey}`} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700 }} />
                  {verStatus.resolved_version && verStatus.resolved_version !== verKey && (
                    <Chip size="small" label={`Resolved Tag: ${verStatus.resolved_version}`} sx={{ height: 20, fontSize: "0.7rem", fontWeight: 600, bgcolor: "#EEF2FF", color: "#4F46E5" }} />
                  )}
                </Box>
                {Object.entries(verStatus.architectures || {}).map(([archKey, archStatus]) => (
                  <Box key={archKey} sx={{ mt: 1, pt: 1, borderTop: "1px dashed #F1F5F9" }}>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                      <Typography variant="caption" sx={{ fontWeight: 700, color: "#334155" }}>
                        Arch: <span style={{ color: "#2563EB" }}>{archKey}</span> ({archStatus.files_on_disk ? "files present on disk" : "metadata preserved, disk freed"})
                      </Typography>
                      {archStatus.archive_size > 0 && (
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Archive: {(archStatus.archive_size / (1024 * 1024)).toFixed(1)} MB
                        </Typography>
                      )}
                    </Box>
                    {archStatus.extracted_files?.map((ef) => (
                      <Box
                        key={ef.name}
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          bgcolor: "#F8FAFC",
                          p: 0.8,
                          borderRadius: "6px",
                          mb: 0.5,
                          fontFamily: "monospace",
                          fontSize: "0.74rem",
                        }}
                      >
                        <Typography sx={{ fontWeight: 700, fontSize: "0.74rem", color: "#0F172A" }}>
                          {ef.name} ({(ef.size / (1024 * 1024)).toFixed(2)} MB)
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                          <Tooltip title={`Click to copy full SHA-256: ${ef.sha256}`} arrow>
                            <Chip
                              size="small"
                              label={`SHA256: ${ef.sha256.slice(0, 14)}...`}
                              onClick={() => handleCopySha(ef.sha256)}
                              icon={copiedSha === ef.sha256 ? <Check size={11} /> : <Copy size={11} />}
                              sx={{ height: 20, fontSize: "0.68rem", cursor: "pointer", bgcolor: copiedSha === ef.sha256 ? "#DCFCE7" : undefined }}
                            />
                          </Tooltip>
                        </Box>
                      </Box>
                    ))}
                  </Box>
                ))}
              </Box>
            ))}
          </Box>
        ) : (
          <Box sx={{ bgcolor: "#FFFFFF", p: 2, borderRadius: "8px", border: "1px dashed #CBD5E1", textAlign: "center" }}>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              No artifact has been downloaded or cached on server yet. Click "Check & Validate Origin" to download and cache now.
            </Typography>
          </Box>
        )}

        {/* Action Buttons Row */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, pt: 1, borderTop: "1px solid #E2E8F0", flexWrap: "wrap" }}>
          <Button
            size="small"
            variant="contained"
            startIcon={isValidating ? <CircularProgress size={13} color="inherit" /> : <RefreshCw size={14} />}
            disabled={isValidating || isClearing}
            onClick={() => handleValidateAppArtifact(app)}
            sx={{
              bgcolor: "#4F46E5",
              textTransform: "none",
              fontWeight: 700,
              fontSize: "0.78rem",
              "&:hover": { bgcolor: "#4338CA" },
            }}
          >
            {isValidating ? "Validating with Origin..." : "Check & Validate Origin"}
          </Button>

          <Button
            size="small"
            variant="outlined"
            color="error"
            startIcon={isClearing ? <CircularProgress size={13} color="inherit" /> : <Trash2 size={14} />}
            disabled={isClearing || isValidating || !status?.files_on_disk}
            onClick={() => handleClearAppArtifactCache(app)}
            sx={{
              textTransform: "none",
              fontWeight: 700,
              fontSize: "0.78rem",
              borderColor: "#FECACA",
              color: "#DC2626",
              "&:hover": { bgcolor: "#FEF2F2", borderColor: "#F87171" },
            }}
          >
            {isClearing ? "Clearing Files..." : "Clear Cache (Free Disk Space)"}
          </Button>

          <Typography variant="caption" sx={{ color: "#64748B", ml: "auto" }}>
            Clearing cache frees server disk while preserving metadata for node sync.
          </Typography>
        </Box>
      </Paper>
    );
  };

  return (
    <Box sx={{ width: "100%", px: { xs: 1.5, sm: 2.5, md: 3 }, py: 2.5, boxSizing: "border-box" }}>
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

          {activeTab === "apps" && (
            <>
              <Button
                variant="outlined"
                startIcon={<HardDrive size={15} />}
                onClick={(e) => setArtifactToolbarMenuAnchor(e.currentTarget)}
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
                Artifact Cache
              </Button>
              <Menu
                anchorEl={artifactToolbarMenuAnchor}
                open={Boolean(artifactToolbarMenuAnchor)}
                onClose={() => setArtifactToolbarMenuAnchor(null)}
                PaperProps={{
                  sx: {
                    borderRadius: "10px",
                    boxShadow: "0 10px 25px rgba(0,0,0,0.1)",
                    border: "1px solid #E2E8F0",
                    minWidth: 260,
                  },
                }}
              >
                <MenuItem onClick={handleValidateAllArtifacts} sx={{ fontSize: "0.82rem", fontWeight: 600, py: 1, gap: 1.2 }}>
                  <RefreshCw size={15} color="#4F46E5" />
                  Check All Artifacts for Origin Updates
                </MenuItem>
                <MenuItem onClick={handleClearAllArtifactCache} sx={{ fontSize: "0.82rem", fontWeight: 600, py: 1, gap: 1.2, color: "#DC2626" }}>
                  <Trash2 size={15} color="#DC2626" />
                  Clear All Server Caches (Free Disk Space)
                </MenuItem>
              </Menu>
            </>
          )}

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
              New App
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
          variant="scrollable"
          onChange={handleTabChange}
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
                    Apps (Active/Total)
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {enabledCount} / {apps.length}
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
                    bgcolor: "#FEF3C7",
                    color: "#D97706",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Clock size={22} />
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Cron Jobs (Crontab)
                  </Typography>
                  <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                    {cronCount}
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
              flexDirection: "column",
              gap: 1.5,
              bgcolor: "#FFFFFF",
            }}
          >
            <Box
              sx={{
                display: "flex",
                gap: 2,
                flexWrap: "wrap",
                alignItems: "center",
                width: "100%",
              }}
            >
              <TextField
                size="small"
                type="search"
                placeholder="Search apps by name, description, note, or referenced variables..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                InputProps={{
                  startAdornment: <Search size={16} style={{ marginRight: 8, color: "#94A3B8" }} />,
                }}
                sx={{ flex: 1, minWidth: 280 }}
              />

              <FormControl size="small" sx={{ minWidth: 170 }}>
                <InputLabel>Runtime Type</InputLabel>
                <Select value={typeFilter} label="Runtime Type" onChange={(e) => handleSelectType(e.target.value)}>
                  <MenuItem value="all">All Types</MenuItem>
                  <MenuItem value="docker">Docker Compose ({dockerCount})</MenuItem>
                  <MenuItem value="systemd">Native Systemd ({systemdCount})</MenuItem>
                  <MenuItem value="cron">Cron Jobs ({cronCount})</MenuItem>
                </Select>
              </FormControl>

              <FormControl size="small" sx={{ minWidth: 180 }}>
                <InputLabel>Deployed Node</InputLabel>
                <Select value={nodeFilter} label="Deployed Node" onChange={(e) => handleSelectNode(e.target.value)}>
                  <MenuItem value="all">All Nodes</MenuItem>
                  {managedNodes.map((n) => (
                    <MenuItem key={n.name} value={n.name}>
                      {n.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              <Box sx={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 1 }}>
                <Button
                  size="small"
                  variant="text"
                  disabled={!isFiltering}
                  onClick={handleClearFilters}
                  startIcon={<X size={14} />}
                  sx={{ color: "#64748B", fontSize: "0.75rem", textTransform: "none", py: 0.4 }}
                >
                  Clear ({filteredApps.length}/{apps.length})
                </Button>
                <Tooltip title={view === "cards" ? "Card view" : "List (table) view"}>
                  <ToggleButtonGroup
                    value={view}
                    exclusive
                    onChange={(_, val) => val && handleToggleView(val)}
                    size="small"
                    sx={{ height: 36 }}
                  >
                    <ToggleButton value="cards" aria-label="card view" sx={{ px: 1.5 }}>
                      <LayoutGrid size={16} />
                    </ToggleButton>
                    <ToggleButton value="table" aria-label="list view" sx={{ px: 1.5 }}>
                      <List size={16} />
                    </ToggleButton>
                  </ToggleButtonGroup>
                </Tooltip>
              </Box>
            </Box>

            {/* Clickable Groups Filter Chips */}
            {appGroups.length > 0 && (
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  flexWrap: "wrap",
                  pt: 1.5,
                  mt: 0.5,
                  borderTop: "1px solid #F1F5F9",
                  width: "100%",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mr: 0.5 }}>
                  <Bookmark size={14} color="#64748B" />
                  <Typography
                    variant="caption"
                    sx={{
                      color: "#64748B",
                      fontWeight: 700,
                      textTransform: "uppercase",
                      letterSpacing: "0.5px",
                      fontSize: "0.7rem",
                    }}
                  >
                    Groups:
                  </Typography>
                </Box>

                <Chip
                  size="small"
                  label={`All (${apps.length})`}
                  onClick={() => handleSelectGroup("all")}
                  color={groupFilter === "all" ? "primary" : "default"}
                  variant={groupFilter === "all" ? "filled" : "outlined"}
                  clickable
                  sx={{
                    fontWeight: 600,
                    fontSize: "0.75rem",
                    height: 26,
                    cursor: "pointer",
                    transition: "all 0.15s ease",
                    ...(groupFilter === "all"
                      ? {
                          bgcolor: "#4F46E5",
                          color: "#FFFFFF",
                          "&:hover": { bgcolor: "#4338CA" },
                        }
                      : {
                          bgcolor: "#F8FAFC",
                          color: "#475569",
                          borderColor: "#E2E8F0",
                          "&:hover": { bgcolor: "#F1F5F9", borderColor: "#CBD5E1" },
                        }),
                  }}
                />

                {appGroups.map((grp) => {
                  const count = apps.filter((a) => (a.group || "").trim() === grp).length;
                  const isSelected = groupFilter.toLowerCase() === grp.toLowerCase();
                  return (
                    <Chip
                      key={grp}
                      size="small"
                      label={`${grp} (${count})`}
                      onClick={() => handleSelectGroup(grp)}
                      color={isSelected ? "primary" : "default"}
                      variant={isSelected ? "filled" : "outlined"}
                      clickable
                      sx={{
                        fontWeight: 600,
                        fontSize: "0.75rem",
                        height: 26,
                        cursor: "pointer",
                        transition: "all 0.15s ease",
                        ...(isSelected
                          ? {
                              bgcolor: "#4F46E5",
                              color: "#FFFFFF",
                              "&:hover": { bgcolor: "#4338CA" },
                            }
                          : {
                              bgcolor: "#F8FAFC",
                              color: "#475569",
                              borderColor: "#E2E8F0",
                              "&:hover": { bgcolor: "#F1F5F9", borderColor: "#CBD5E1" },
                            }),
                      }}
                    />
                  );
                })}

                {ungroupedAppsCount > 0 && (
                  <Chip
                    size="small"
                    label={`Ungrouped (${ungroupedAppsCount})`}
                    onClick={() => handleSelectGroup("ungrouped")}
                    color={groupFilter === "ungrouped" ? "primary" : "default"}
                    variant={groupFilter === "ungrouped" ? "filled" : "outlined"}
                    clickable
                    sx={{
                      fontWeight: 600,
                      fontSize: "0.75rem",
                      height: 26,
                      cursor: "pointer",
                      transition: "all 0.15s ease",
                      fontStyle: "italic",
                      ...(groupFilter === "ungrouped"
                        ? {
                            bgcolor: "#4F46E5",
                            color: "#FFFFFF",
                            "&:hover": { bgcolor: "#4338CA" },
                          }
                        : {
                            bgcolor: "#F8FAFC",
                            color: "#64748B",
                            borderColor: "#E2E8F0",
                            "&:hover": { bgcolor: "#F1F5F9", borderColor: "#CBD5E1" },
                          }),
                    }}
                  />
                )}
              </Box>
            )}
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
          ) : view === "table" ? (
            /* List (table) view — mirrors NodesPage table view */
            <TableContainer
              component={Paper}
              elevation={0}
              sx={{
                borderRadius: "14px",
                border: "1px solid #E2E8F0",
                bgcolor: "#FFFFFF",
                overflowX: "auto",
              }}
            >
              <Table size="small" sx={{ minWidth: 1100 }}>
                <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                  <TableRow>
                    <TableCell
                      sortDirection={sortField === "name" ? sortOrder : false}
                      sx={{ fontWeight: 800, color: "#475569", minWidth: 200 }}
                    >
                      <TableSortLabel
                        active={sortField === "name"}
                        direction={sortField === "name" ? sortOrder : "asc"}
                        onClick={() => handleRequestSort("name")}
                      >
                        App
                      </TableSortLabel>
                    </TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569", minWidth: 220 }}>Description</TableCell>
                    <TableCell
                      sortDirection={sortField === "type" ? sortOrder : false}
                      sx={{ fontWeight: 800, color: "#475569", minWidth: 130 }}
                    >
                      <TableSortLabel
                        active={sortField === "type"}
                        direction={sortField === "type" ? sortOrder : "asc"}
                        onClick={() => handleRequestSort("type")}
                      >
                        Runtime
                      </TableSortLabel>
                    </TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569", minWidth: 220 }}>Homepage URL</TableCell>
                    <TableCell sx={{ fontWeight: 800, color: "#475569", minWidth: 240 }}>Target Nodes</TableCell>
                    <TableCell
                      sortDirection={sortField === "published" ? sortOrder : false}
                      sx={{
                        fontWeight: 800,
                        color: "#475569",
                        minWidth: 110,
                        textAlign: "center",
                      }}
                    >
                      <TableSortLabel
                        active={sortField === "published"}
                        direction={sortField === "published" ? sortOrder : "asc"}
                        onClick={() => handleRequestSort("published")}
                      >
                        Published
                      </TableSortLabel>
                    </TableCell>
                    <TableCell
                      sortDirection={sortField === "enabled" ? sortOrder : false}
                      sx={{
                        fontWeight: 800,
                        color: "#475569",
                        minWidth: 110,
                        textAlign: "center",
                      }}
                    >
                      <TableSortLabel
                        active={sortField === "enabled"}
                        direction={sortField === "enabled" ? sortOrder : "asc"}
                        onClick={() => handleRequestSort("enabled")}
                      >
                        Enabled
                      </TableSortLabel>
                    </TableCell>
                    <TableCell align="right" sx={{ fontWeight: 800, color: "#475569", minWidth: 140 }}>
                      Actions
                    </TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {sortedApps.map((app) => {
                    const target = getTargetNodeInfo(app);
                    const chipColors = targetChipColors(target.mode);
                    const resolved = resolveHomepageUrl(app, managedNodes, variables);
                    const isClickableHomepage = !!resolved && !resolved.containsTemplate && isHttpUrl(resolved.url);
                    const displayHomepage = resolved && !resolved.containsTemplate ? resolved.url : app.url;
                    const homepageTooltip = isClickableHomepage
                      ? `Homepage on ${resolved!.node.name}: ${resolved!.url}`
                      : resolved
                        ? `Homepage template ${resolved.containsTemplate ? "(unresolved variables)" : ""}: ${app.url} (deployed to ${target.nodeNames.length} node(s))`
                        : `Homepage template: ${app.url} (deployed to ${target.nodeNames.length} node(s))`;
                    return (
                      <TableRow
                        key={app.id}
                        hover
                        sx={{
                          "&:hover": { bgcolor: "#F8FAFC" },
                          "& td": { borderColor: "#F1F5F9" },
                          verticalAlign: "top",
                        }}
                      >
                        {/* App */}
                        <TableCell>
                          <Box
                            sx={{
                              display: "flex",
                              alignItems: "center",
                              gap: 1.2,
                            }}
                          >
                            <Box
                              sx={{
                                width: 34,
                                height: 34,
                                borderRadius: "8px",
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                                flexShrink: 0,
                                bgcolor:
                                  app.app_type === "docker"
                                    ? "#E0F2FE"
                                    : app.app_type === "cron"
                                      ? "#FEF3C7"
                                      : "#F3E8FF",
                                color:
                                  app.app_type === "docker"
                                    ? "#0284C7"
                                    : app.app_type === "cron"
                                      ? "#D97706"
                                      : "#9333EA",
                              }}
                            >
                              {app.app_type === "docker" ? (
                                <BoxIcon size={17} />
                              ) : app.app_type === "cron" ? (
                                <Clock size={17} />
                              ) : (
                                <Cpu size={17} />
                              )}
                            </Box>
                            <Box sx={{ minWidth: 0 }}>
                              <Box
                                sx={{
                                  display: "flex",
                                  alignItems: "center",
                                  gap: 0.5,
                                }}
                              >
                                <Typography
                                  variant="body2"
                                  onClick={() => setDetailsApp(app)}
                                  sx={{
                                    fontWeight: 700,
                                    color: "#4F46E5",
                                    lineHeight: 1.2,
                                    cursor: "pointer",
                                    "&:hover": { textDecoration: "underline" },
                                  }}
                                >
                                  {app.name}
                                </Typography>
                                <Tooltip title="App Details">
                                  <IconButton
                                    size="small"
                                    onClick={() => setDetailsApp(app)}
                                    sx={{
                                      p: 0.3,
                                      color: "#94A3B8",
                                      "&:hover": {
                                        color: "#4F46E5",
                                        bgcolor: "#EEF2FF",
                                      },
                                    }}
                                  >
                                    <Info size={13} />
                                  </IconButton>
                                </Tooltip>
                                <Tooltip title="Rename App">
                                  <IconButton
                                    size="small"
                                    onClick={() => handleOpenRenameApp(app)}
                                    sx={{
                                      p: 0.3,
                                      color: "#94A3B8",
                                      "&:hover": {
                                        color: "#4F46E5",
                                        bgcolor: "#EEF2FF",
                                      },
                                    }}
                                  >
                                    <Tag size={13} />
                                  </IconButton>
                                </Tooltip>
                              </Box>
                              <Typography
                                variant="caption"
                                sx={{
                                  color: "#94A3B8",
                                  fontWeight: 600,
                                  display: "block",
                                  mt: 0.2,
                                }}
                              >
                                {app.published ? (app.enabled ? "Active" : "Stopped") : "Draft"}
                              </Typography>
                            </Box>
                          </Box>
                        </TableCell>

                        {/* Description */}
                        <TableCell>
                          <Typography
                            variant="body2"
                            sx={{
                              color: "#64748B",
                              fontSize: "0.85rem",
                              lineHeight: 1.4,
                              display: "-webkit-box",
                              WebkitLineClamp: 3,
                              WebkitBoxOrient: "vertical",
                              overflow: "hidden",
                            }}
                          >
                            {app.description || "No description provided."}
                          </Typography>
                        </TableCell>

                        {/* Runtime */}
                        <TableCell>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, flexWrap: "wrap" }}>
                            <Chip
                              size="small"
                              label={
                                app.app_type === "docker" ? "Docker" : app.app_type === "cron" ? "Cron" : "Systemd"
                              }
                              sx={{
                                height: 22,
                                fontSize: "0.7rem",
                                fontWeight: 700,
                                bgcolor:
                                  app.app_type === "docker"
                                    ? "#E0F2FE"
                                    : app.app_type === "cron"
                                      ? "#FEF3C7"
                                      : "#F3E8FF",
                                color:
                                  app.app_type === "docker"
                                    ? "#0369A1"
                                    : app.app_type === "cron"
                                      ? "#B45309"
                                      : "#7E22CE",
                              }}
                            />
                            {app.artifact && (
                              <Tooltip
                                title="Click to view server artifact cache, check origin updates, or free disk space"
                                arrow
                              >
                                <Chip
                                  size="small"
                                  icon={<Package size={11} />}
                                  label={app.version || "Artifact"}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    setArtifactCacheModalApp(app);
                                    fetchAppArtifactStatus(app.id);
                                  }}
                                  sx={{
                                    height: 20,
                                    fontSize: "0.68rem",
                                    fontWeight: 700,
                                    bgcolor: "#ECFDF5",
                                    color: "#059669",
                                    cursor: "pointer",
                                    "& .MuiChip-icon": { color: "#059669" },
                                    "&:hover": { bgcolor: "#D1FAE5" },
                                  }}
                                />
                              </Tooltip>
                            )}
                            {app.group && (
                              <Tooltip title={`Filter by group: ${app.group}`} arrow>
                                <Chip
                                  size="small"
                                  label={app.group}
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleSelectGroup(app.group!);
                                  }}
                                  sx={{
                                    height: 20,
                                    fontSize: "0.68rem",
                                    fontWeight: 600,
                                    bgcolor: "#F1F5F9",
                                    color: "#475569",
                                    border: "1px solid #E2E8F0",
                                    cursor: "pointer",
                                    "&:hover": { bgcolor: "#E2E8F0" },
                                  }}
                                />
                              </Tooltip>
                            )}
                          </Box>
                        </TableCell>

                        {/* Homepage URL */}
                        <TableCell>
                          {app.url ? (
                            isClickableHomepage ? (
                              <Tooltip title={homepageTooltip} arrow placement="top">
                                <Box
                                  component="a"
                                  href={displayHomepage}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  sx={{
                                    display: "inline-flex",
                                    alignItems: "center",
                                    gap: 0.5,
                                    maxWidth: "100%",
                                    color: "#2563EB",
                                    textDecoration: "none",
                                    fontSize: "0.78rem",
                                    fontWeight: 600,
                                    bgcolor: "#EFF6FF",
                                    px: 1,
                                    py: 0.4,
                                    borderRadius: "6px",
                                    border: "1px solid #BFDBFE",
                                    transition: "all 0.15s ease",
                                    "&:hover": {
                                      bgcolor: "#DBEAFE",
                                      textDecoration: "underline",
                                    },
                                  }}
                                >
                                  <Globe size={13} style={{ flexShrink: 0 }} />
                                  <span style={{ wordBreak: "break-all" }}>{displayHomepage}</span>
                                  <ExternalLink size={12} style={{ flexShrink: 0 }} />
                                </Box>
                              </Tooltip>
                            ) : (
                              <Tooltip title={homepageTooltip} arrow placement="top">
                                <Box
                                  sx={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: 0.6,
                                    maxWidth: "100%",
                                  }}
                                >
                                  <Globe size={13} style={{ flexShrink: 0, color: "#2563EB" }} />
                                  <Typography
                                    variant="caption"
                                    sx={{
                                      fontWeight: 600,
                                      color: "#2563EB",
                                      wordBreak: "break-all",
                                    }}
                                  >
                                    {displayHomepage}
                                  </Typography>
                                </Box>
                              </Tooltip>
                            )
                          ) : (
                            <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                              —
                            </Typography>
                          )}
                        </TableCell>

                        {/* Target Nodes */}
                        <TableCell>
                          <Box
                            sx={{
                              display: "flex",
                              flexDirection: "column",
                              gap: 0.6,
                            }}
                          >
                            <Box>
                              <Chip
                                size="small"
                                label={
                                  target.mode === "manual"
                                    ? `Manual (${target.manualNodes.length})`
                                    : target.mode === "tag"
                                      ? `Tag: #${getSelectorValue(app.selector || {}, "tag")}`
                                      : target.mode === "custom"
                                        ? "Selector"
                                        : "All Nodes"
                                }
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: chipColors.bgcolor,
                                  color: chipColors.color,
                                }}
                              />
                            </Box>
                            <Box
                              sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 0.5,
                              }}
                            >
                              {target.nodeNames.length > 0 ? (
                                <>
                                  {target.nodeNames.slice(0, 3).map((nodeName) => (
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
                                        bgcolor: chipColors.bgcolor,
                                        color: chipColors.color,
                                        textDecoration: "none",
                                        cursor: "pointer",
                                        transition: "all 0.15s ease",
                                        "&:hover": {
                                          bgcolor: chipColors.hoverBg,
                                          color: chipColors.hoverColor,
                                        },
                                      }}
                                    />
                                  ))}
                                  {target.nodeNames.length > 3 && (
                                    <Tooltip title={target.nodeNames.join(", ")} arrow placement="top">
                                      <Chip
                                        size="small"
                                        label={`+${target.nodeNames.length - 3} more`}
                                        sx={{
                                          height: 20,
                                          fontSize: "0.7rem",
                                          fontWeight: 700,
                                          bgcolor: "#F1F5F9",
                                          color: "#475569",
                                        }}
                                      />
                                    </Tooltip>
                                  )}
                                </>
                              ) : (
                                <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                  {target.mode === "all"
                                    ? "No agent nodes available"
                                    : target.mode === "manual"
                                      ? "None assigned (unassigned)"
                                      : "No nodes currently match this selector"}
                                </Typography>
                              )}
                            </Box>
                          </Box>
                        </TableCell>

                        {/* Published */}
                        <TableCell align="center">
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
                                {app.published ? "Yes" : "Draft"}
                              </Typography>
                            }
                            sx={{ m: 0 }}
                          />
                        </TableCell>

                        {/* Enabled */}
                        <TableCell align="center">
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
                                {app.enabled ? "On" : "Off"}
                              </Typography>
                            }
                            sx={{ m: 0 }}
                          />
                        </TableCell>

                        {/* Actions */}
                        <TableCell align="right">
                          <Box
                            sx={{
                              display: "flex",
                              justifyContent: "flex-end",
                              gap: 0.5,
                            }}
                          >
                            <Tooltip title="App Details">
                              <IconButton
                                size="small"
                                onClick={() => setDetailsApp(app)}
                                sx={{
                                  color: "#64748B",
                                  "&:hover": {
                                    color: "#4F46E5",
                                    bgcolor: "#EEF2FF",
                                  },
                                }}
                              >
                                <Info size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Rename App">
                              <IconButton
                                size="small"
                                onClick={() => handleOpenRenameApp(app)}
                                sx={{
                                  color: "#64748B",
                                  "&:hover": {
                                    color: "#4F46E5",
                                    bgcolor: "#EEF2FF",
                                  },
                                }}
                              >
                                <Tag size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Duplicate App">
                              <IconButton
                                size="small"
                                disabled={duplicatingAppId === app.id}
                                onClick={() => handleDuplicateApp(app)}
                                sx={{
                                  color: "#64748B",
                                  "&:hover": {
                                    color: "#4F46E5",
                                    bgcolor: "#EEF2FF",
                                  },
                                }}
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
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          ) : (
            /* Full-Width Grid Cards */
            <Grid container spacing={2.5}>
              {filteredApps.map((app) => (
                <Grid item xs={12} sm={6} md={4} lg={3} xl={2.4} key={app.id}>
                  <Card
                    elevation={0}
                    onClick={() => setDetailsApp(app)}
                    sx={{
                      borderRadius: "14px",
                      border: "1px solid #E2E8F0",
                      height: "100%",
                      display: "flex",
                      flexDirection: "column",
                      cursor: "pointer",
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
                      <Box
                        sx={{
                          display: "flex",
                          justifyContent: "space-between",
                          alignItems: "flex-start",
                          flexWrap: "wrap",
                          gap: 1,
                          mb: 1.5,
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1.2, flex: "1 1 auto", minWidth: 0 }}>
                          <Box
                            sx={{
                              width: 36,
                              height: 36,
                              flexShrink: 0,
                              borderRadius: "8px",
                              display: "flex",
                              alignItems: "center",
                              justifyContent: "center",
                              bgcolor:
                                app.app_type === "docker" ? "#E0F2FE" : app.app_type === "cron" ? "#FEF3C7" : "#F3E8FF",
                              color:
                                app.app_type === "docker" ? "#0284C7" : app.app_type === "cron" ? "#D97706" : "#9333EA",
                            }}
                          >
                            {app.app_type === "docker" ? (
                              <BoxIcon size={18} />
                            ) : app.app_type === "cron" ? (
                              <Clock size={18} />
                            ) : (
                              <Cpu size={18} />
                            )}
                          </Box>
                          <Box sx={{ minWidth: 0, flex: 1 }}>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, minWidth: 0 }}>
                              <Typography
                                variant="subtitle1"
                                title={app.name}
                                sx={{
                                  fontWeight: 700,
                                  color: "#0F172A",
                                  lineHeight: 1.2,
                                  minWidth: 0,
                                  overflow: "hidden",
                                  textOverflow: "ellipsis",
                                  whiteSpace: "nowrap",
                                }}
                              >
                                {app.name}
                              </Typography>
                              <Tooltip title="Rename App">
                                <IconButton
                                  size="small"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    handleOpenRenameApp(app);
                                  }}
                                  sx={{
                                    p: 0.3,
                                    flexShrink: 0,
                                    color: "#94A3B8",
                                    "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" },
                                  }}
                                >
                                  <Tag size={13} />
                                </IconButton>
                              </Tooltip>
                            </Box>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, flexWrap: "wrap", mt: 0.3 }}>
                              <Chip
                                size="small"
                                label={
                                  app.app_type === "docker" ? "Docker" : app.app_type === "cron" ? "Cron" : "Systemd"
                                }
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor:
                                    app.app_type === "docker"
                                      ? "#E0F2FE"
                                      : app.app_type === "cron"
                                        ? "#FEF3C7"
                                        : "#F3E8FF",
                                  color:
                                    app.app_type === "docker"
                                      ? "#0369A1"
                                      : app.app_type === "cron"
                                        ? "#B45309"
                                        : "#7E22CE",
                                }}
                              />
                              {app.artifact && (
                                <Tooltip
                                  title="Click to view server artifact cache, check origin updates, or free disk space"
                                  arrow
                                >
                                  <Chip
                                    size="small"
                                    icon={<Package size={11} />}
                                    label={app.version || "Artifact"}
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      setArtifactCacheModalApp(app);
                                      fetchAppArtifactStatus(app.id);
                                    }}
                                    sx={{
                                      height: 20,
                                      fontSize: "0.68rem",
                                      fontWeight: 700,
                                      bgcolor: "#ECFDF5",
                                      color: "#059669",
                                      cursor: "pointer",
                                      "& .MuiChip-icon": { color: "#059669" },
                                      "&:hover": { bgcolor: "#D1FAE5" },
                                    }}
                                  />
                                </Tooltip>
                              )}
                              {app.group && (
                                <Tooltip title={`Filter by group: ${app.group}`} arrow>
                                  <Chip
                                    size="small"
                                    label={app.group}
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      handleSelectGroup(app.group!);
                                    }}
                                    sx={{
                                      height: 20,
                                      fontSize: "0.68rem",
                                      fontWeight: 600,
                                      bgcolor: "#F1F5F9",
                                      color: "#475569",
                                      border: "1px solid #E2E8F0",
                                      cursor: "pointer",
                                      "&:hover": { bgcolor: "#E2E8F0" },
                                    }}
                                  />
                                </Tooltip>
                              )}
                            </Box>
                          </Box>
                        </Box>

                        <Box
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "flex-end",
                            flexWrap: "wrap",
                            gap: 1,
                            flexShrink: 0,
                            ml: "auto",
                          }}
                          onClick={(e) => e.stopPropagation()}
                        >
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
                        <Box sx={{ mb: 1.5 }}>
                          {(() => {
                            const resolved = resolveHomepageUrl(app, managedNodes, variables);
                            const isClickable = !!resolved && !resolved.containsTemplate && isHttpUrl(resolved.url);
                            const displayUrl = resolved && !resolved.containsTemplate ? resolved.url : app.url;
                            const tooltip = isClickable
                              ? `Homepage on ${resolved!.node.name}: ${resolved!.url}`
                              : resolved
                                ? `Homepage template ${resolved.containsTemplate ? "(unresolved variables)" : ""}: ${app.url} (deployed to ${getTargetNodeInfo(app).nodeNames.length} node(s))`
                                : `Homepage template: ${app.url} (deployed to ${getTargetNodeInfo(app).nodeNames.length} node(s))`;
                            if (isClickable) {
                              return (
                                <Tooltip title={tooltip} arrow placement="top">
                                  <Box
                                    component="a"
                                    href={displayUrl}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    onClick={(e) => e.stopPropagation()}
                                    sx={{
                                      display: "inline-flex",
                                      alignItems: "center",
                                      gap: 0.5,
                                      maxWidth: "100%",
                                      color: "#2563EB",
                                      textDecoration: "none",
                                      fontSize: "0.75rem",
                                      fontWeight: 600,
                                      bgcolor: "#EFF6FF",
                                      px: 1,
                                      py: 0.4,
                                      borderRadius: "6px",
                                      border: "1px solid #BFDBFE",
                                      transition: "all 0.15s ease",
                                      "&:hover": {
                                        bgcolor: "#DBEAFE",
                                        textDecoration: "underline",
                                      },
                                    }}
                                  >
                                    <Globe size={13} style={{ flexShrink: 0 }} />
                                    <span
                                      style={{
                                        overflow: "hidden",
                                        textOverflow: "ellipsis",
                                        whiteSpace: "nowrap",
                                      }}
                                    >
                                      {displayUrl}
                                    </span>
                                    <ExternalLink size={12} style={{ flexShrink: 0 }} />
                                  </Box>
                                </Tooltip>
                              );
                            }
                            return (
                              <Tooltip title={tooltip} arrow placement="top">
                                <Box
                                  sx={{
                                    display: "flex",
                                    alignItems: "center",
                                    gap: 0.6,
                                    color: "#2563EB",
                                    maxWidth: "100%",
                                  }}
                                >
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
                                    {displayUrl}
                                  </Typography>
                                </Box>
                              </Tooltip>
                            );
                          })()}
                        </Box>
                      )}

                      <Divider sx={{ my: 1.2, borderColor: "#F1F5F9" }} />

                      {/* Target Nodes */}
                      <Box sx={{ mb: 1.5 }}>
                        {(() => {
                          const selector = app.selector || {};
                          const mode = detectTargetMode(selector);
                          const manualNodes = splitSelectorList(getSelectorValue(selector, "name"));
                          const tagValue = getSelectorValue(selector, "tag");
                          const matchingNodes = managedNodes.filter((n) => matchesTaskSelector(n, selector));

                          const chipColors =
                            mode === "tag"
                              ? { bgcolor: "#EDE9FE", color: "#5B21B6", hoverBg: "#DDD6FE", hoverColor: "#4C1D95" }
                              : mode === "custom"
                                ? { bgcolor: "#CCFBF1", color: "#0F766E", hoverBg: "#99F6E4", hoverColor: "#115E59" }
                                : mode === "manual"
                                  ? { bgcolor: "#F1F5F9", color: "#334155", hoverBg: "#E2E8F0", hoverColor: "#1E293B" }
                                  : { bgcolor: "#E0F2FE", color: "#0369A1", hoverBg: "#BAE6FD", hoverColor: "#0284C7" };

                          const headerChip =
                            mode === "manual" ? (
                              <Chip
                                size="small"
                                icon={<Sliders size={12} />}
                                label={`Manual (${manualNodes.length})`}
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: chipColors.bgcolor,
                                  color: chipColors.color,
                                }}
                              />
                            ) : mode === "tag" ? (
                              <Chip
                                size="small"
                                icon={<Tag size={12} />}
                                label={`Tag: #${tagValue}`}
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: chipColors.bgcolor,
                                  color: chipColors.color,
                                }}
                              />
                            ) : mode === "custom" ? (
                              <Chip
                                size="small"
                                icon={<Code2 size={12} />}
                                label="Selector"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: chipColors.bgcolor,
                                  color: chipColors.color,
                                }}
                              />
                            ) : (
                              <Chip
                                size="small"
                                icon={<Globe size={12} />}
                                label="All Nodes"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: chipColors.bgcolor,
                                  color: chipColors.color,
                                }}
                              />
                            );

                          const nodeNames = mode === "manual" ? manualNodes : matchingNodes.map((n) => n.name);

                          return (
                            <>
                              <Box
                                sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.6 }}
                              >
                                <Typography
                                  variant="caption"
                                  sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase" }}
                                >
                                  Target Nodes
                                </Typography>
                                {headerChip}
                              </Box>

                              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
                                {nodeNames.length > 0 ? (
                                  nodeNames.map((nodeName) => (
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
                                        bgcolor: chipColors.bgcolor,
                                        color: chipColors.color,
                                        textDecoration: "none",
                                        cursor: "pointer",
                                        transition: "all 0.15s ease",
                                        "&:hover": {
                                          bgcolor: chipColors.hoverBg,
                                          color: chipColors.hoverColor,
                                        },
                                      }}
                                    />
                                  ))
                                ) : (
                                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                    {mode === "all"
                                      ? "No agent nodes available"
                                      : mode === "manual"
                                        ? "None assigned (unassigned)"
                                        : "No nodes currently match this selector"}
                                  </Typography>
                                )}
                              </Box>
                            </>
                          );
                        })()}
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
                          <Tooltip title="App Details">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                setDetailsApp(app);
                              }}
                              sx={{ color: "#64748B", "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" } }}
                            >
                              <Info size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Rename App">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleOpenRenameApp(app);
                              }}
                              sx={{ color: "#64748B", "&:hover": { color: "#4F46E5", bgcolor: "#EEF2FF" } }}
                            >
                              <Tag size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Duplicate App">
                            <IconButton
                              size="small"
                              disabled={duplicatingAppId === app.id}
                              onClick={(e) => {
                                e.stopPropagation();
                                handleDuplicateApp(app);
                              }}
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
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                openEditAppModal(app);
                              }}
                              sx={{ color: "#64748B" }}
                            >
                              <Edit size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Delete App">
                            <IconButton
                              size="small"
                              onClick={(e) => {
                                e.stopPropagation();
                                setDeleteApp(app);
                              }}
                              sx={{ color: "#EF4444" }}
                            >
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
              overflowX: "auto",
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
                overflowX: "auto",
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
                            <Tooltip title={isCopied ? "Copied!" : "Copy value"}>
                              <IconButton
                                size="small"
                                onClick={() => copyText(v.key, v.key + "-key")}
                                sx={{ color: isCopied ? "#10B981" : "#94A3B8" }}
                              >
                                {isCopied ? <Check size={14} /> : <Copy size={14} />}
                              </IconButton>
                            </Tooltip>
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
                                onClick={() => copyText(v.value, v.key + "-value")}
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
            variant="scrollable"
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
              label={
                formType === "docker"
                  ? "Compose Manifest"
                  : formType === "cron"
                    ? "Cron Schedule & Command"
                    : "Systemd Unit"
              }
              icon={<FileCode size={16} />}
              iconPosition="start"
            />
            {formType !== "cron" && (
              <Tab value="env" label="Environment (.env)" icon={<FileText size={16} />} iconPosition="start" />
            )}
            {formType !== "cron" && (
              <Tab
                value="files"
                label={`Config Files (${formFiles.length})`}
                icon={<FilePlus size={16} />}
                iconPosition="start"
              />
            )}
            <Tab value="note" label="Note" icon={<FileText size={16} />} iconPosition="start" />
            <Tab value="preview" label="Variables Preview" icon={<Code2 size={16} />} iconPosition="start" />
            {formType === "systemd" && (
              <Tab
                value="artifact"
                label={formArtifactEnabled ? "Artifact Spec (Active)" : "Artifact Spec"}
                icon={<Package size={16} />}
                iconPosition="start"
              />
            )}
            {formType === "systemd" && (
              <Tab value="transpile" label="Transpile Preview" icon={<Eye size={16} />} iconPosition="start" />
            )}
          </Tabs>
        </Box>

        <DialogContent sx={{ p: 3 }}>
          {/* Quick Variables Insert Chips Bar */}
          {(modalTab === "definition" || modalTab === "env" || modalTab === "files") && (
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
                    if (modalTab === "definition") {
                      if (formType === "cron") {
                        setFormCronCmd((prev) => {
                          const updated = prev ? (prev.endsWith(" ") ? prev + tag : prev + " " + tag) : tag;
                          setFormMain(`${formCronExpr.trim()} ${updated}`.trim());
                          return updated;
                        });
                      } else {
                        setFormMain((prev) => prev + tag);
                      }
                    } else if (modalTab === "env") {
                      setFormEnv((prev) => prev + tag);
                    } else {
                      copyText(`\${${v.key}}`, "env:" + v.key);
                      setStateToast({
                        message: `Copied`,
                        severity: "info",
                      });
                    }
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
                      const tag = nv.label;
                      if (modalTab === "definition") {
                        if (formType === "cron") {
                          setFormCronCmd((prev) => {
                            const updated = prev ? (prev.endsWith(" ") ? prev + tag : prev + " " + tag) : tag;
                            setFormMain(`${formCronExpr.trim()} ${updated}`.trim());
                            return updated;
                          });
                        } else {
                          setFormMain((prev) => prev + tag);
                        }
                      } else if (modalTab === "env") {
                        setFormEnv((prev) => prev + tag);
                      } else {
                        copyText(`\${${nv.key}}`, "env:" + nv.key);
                        setStateToast({
                          message: `Copied`,
                          severity: "info",
                        });
                      }
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
                      onChange={(e) => handleTypeChange(e.target.value as "docker" | "systemd" | "cron")}
                    >
                      <MenuItem value="docker">Docker Compose (Container)</MenuItem>
                      <MenuItem value="systemd">Systemd / Native Service (Daemon)</MenuItem>
                      <MenuItem value="cron">Cron Job (Crontab Scheduled Task)</MenuItem>
                    </Select>
                  </FormControl>
                </Grid>
              </Grid>

              <Autocomplete
                freeSolo
                options={appGroups}
                value={formGroup}
                onInputChange={(_, newValue) => setFormGroup(newValue)}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    fullWidth
                    label="Group / Category (Optional)"
                    placeholder="e.g. Monitoring, Proxy, Database, Core..."
                    helperText="Category used to organize and filter apps in the apps list"
                  />
                )}
              />

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
                  Choose nodes with the shortcuts below or edit the selector JSON directly. An empty selector matches
                  every managed node. All selector conditions must match (AND).
                </Typography>

                <FormControl component="fieldset" fullWidth sx={{ mb: 2 }}>
                  <RadioGroup
                    row
                    value={formTargetType}
                    onChange={(e) => handleTargetModeChange(e.target.value as AppTargetMode)}
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
                            Nodes of Selected Tag(s)
                          </Typography>
                        </Box>
                      }
                    />
                    <FormControlLabel
                      value="custom"
                      control={<Radio size="small" />}
                      label={
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                          <Code2 size={16} color="#0F766E" />
                          <Typography variant="body2" sx={{ fontWeight: 600 }}>
                            Advanced Selector
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
                          onClick={() => applyFormSelector({ name: managedNodes.map((n) => n.name).join(",") })}
                        >
                          Select All
                        </Button>
                        <Button
                          size="small"
                          sx={{ textTransform: "none", fontSize: "0.75rem", p: 0, color: "#64748B" }}
                          onClick={() => applyFormSelector({ name: "" })}
                        >
                          Clear (None)
                        </Button>
                      </Box>
                    </Box>

                    <FormControl fullWidth size="small">
                      <InputLabel>Selected Nodes ({formSelectedNodes.length})</InputLabel>
                      <Select
                        multiple
                        value={formSelectedNodes}
                        label={`Selected Nodes (${formSelectedNodes.length})`}
                        onChange={(e) => {
                          const val = e.target.value;
                          const selected = typeof val === "string" ? val.split(",") : val;
                          applyFormSelector({ name: selected.join(",") });
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
                      sx={{ color: formSelectedNodes.length === 0 ? "#D97706" : "#64748B", mt: 0.5, display: "block" }}
                    >
                      {formSelectedNodes.length === 0
                        ? "Notice: No nodes selected. The selector will match no node until a node name is added."
                        : `Assigned to ${formSelectedNodes.length} node(s).`}
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
                      An empty selector deploys this app to all cluster nodes running in agent mode. Any new agent node
                      added in the future will automatically receive this app.
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

                {/* Option 3: Nodes of Selected Tag(s) */}
                {formTargetType === "tag" && (
                  <Box>
                    <Grid container spacing={2} alignItems="center">
                      <Grid item xs={12} sm={6}>
                        <FormControl fullWidth size="small">
                          <InputLabel>Select from Existing Tags</InputLabel>
                          <Select
                            multiple
                            value={splitSelectorList(formTargetTag)}
                            label="Select from Existing Tags"
                            onChange={(e) => {
                              const val = e.target.value;
                              const selected = typeof val === "string" ? val.split(",") : val;
                              applyFormSelector({ tag: selected.join(",") });
                            }}
                            renderValue={(selected) => (
                              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                                {selected.length === 0 ? (
                                  <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                                    None selected
                                  </Typography>
                                ) : (
                                  selected.map((tag) => (
                                    <Chip
                                      key={tag}
                                      label={`#${tag}`}
                                      size="small"
                                      sx={{ height: 22, fontSize: "0.75rem" }}
                                    />
                                  ))
                                )}
                              </Box>
                            )}
                          >
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
                          label="Or Enter Tags (comma-separated)"
                          placeholder="e.g. edge, ingress, prod"
                          value={formTargetTag}
                          onChange={(e) => applyFormSelector({ tag: e.target.value.toLowerCase() })}
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
                          matchesTaskSelector(n, { tag: formTargetTag }),
                        );
                        if (!formTargetTag) {
                          return (
                            <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                              Please select or enter at least one tag above to see matching nodes.
                            </Typography>
                          );
                        }
                        if (matchingNodes.length === 0) {
                          return (
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                              <Info size={16} color="#8B5CF6" />
                              <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                                No nodes currently have any of these tags. The app will automatically deploy whenever a
                                node is tagged with <strong>#{formTargetTag}</strong>.
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
                              {matchingNodes.length} node(s) currently match tag selection:
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

                {formTargetType === "custom" && (
                  <Box sx={{ p: 2, bgcolor: "#F0FDFA", borderRadius: "8px", border: "1px solid #99F6E4" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Code2 size={16} color="#0F766E" />
                      <Typography variant="caption" sx={{ color: "#115E59" }}>
                        Build an arbitrary selector below. Supported keys are <code>name</code>, <code>tag</code>, and{" "}
                        <code>env:&lt;NAME&gt;</code>.
                      </Typography>
                    </Box>
                  </Box>
                )}

                <Box sx={{ mt: 2 }}>
                  <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.8 }}>
                    <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700 }}>
                      Selector JSON
                    </Typography>
                    <Typography
                      variant="caption"
                      sx={{ color: selectorMatchedNodes.length > 0 ? "#059669" : "#D97706" }}
                    >
                      {selectorMatchedNodes.length} node(s) match
                    </Typography>
                  </Box>
                  <TextField
                    fullWidth
                    multiline
                    minRows={4}
                    size="small"
                    value={formSelectorText}
                    onChange={(e) => handleSelectorTextChange(e.target.value)}
                    error={Boolean(formSelectorError)}
                    helperText={
                      formSelectorError ||
                      'Use {"name":"node-a,node-b"}, {"tag":"edge,prod"}, or {"env:NODE_IP":"10.0.0.1"}. Empty {} matches all nodes.'
                    }
                    InputProps={{
                      sx: { fontFamily: "monospace", fontSize: "0.8rem" },
                    }}
                  />
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6, mt: 1 }}>
                    {selectorMatchedNodes.length === 0 ? (
                      <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                        No managed nodes match this selector.
                      </Typography>
                    ) : (
                      selectorMatchedNodes.map((n) => (
                        <Chip
                          key={n.name}
                          label={n.name}
                          size="small"
                          sx={{ height: 20, fontSize: "0.7rem", bgcolor: "#ECFDF5", color: "#047857" }}
                        />
                      ))
                    )}
                  </Box>
                </Box>
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

          {/* TAB 2: Definition Editor (Cron) */}
          {modalTab === "definition" && formType === "cron" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  flexWrap: "wrap",
                  gap: 1,
                }}
              >
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Cron Job Schedule & Command
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Installed into standard system crontab inside the Easy42 managed block
                  </Typography>
                </Box>
                <Button
                  size="small"
                  onClick={() => {
                    setFormCronExpr("0 0 * * *");
                    setFormCronCmd("/root/backup.sh");
                    setFormMain("0 0 * * * /root/backup.sh");
                  }}
                  sx={{ textTransform: "none", fontSize: "0.75rem", color: "#64748B" }}
                >
                  Reset Template
                </Button>
              </Box>

              {/* Schedule Presets */}
              <Box sx={{ p: 2, bgcolor: "#FEF3C7", borderRadius: "10px", border: "1px solid #FDE68A" }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                  <Clock size={16} color="#B45309" />
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#92400E" }}>
                    Schedule Presets:
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 0.8, flexWrap: "wrap" }}>
                  {[
                    { label: "Every minute (* * * * *)", expr: "* * * * *" },
                    { label: "Every 5 min (*/5 * * * *)", expr: "*/5 * * * *" },
                    { label: "Hourly (0 * * * *)", expr: "0 * * * *" },
                    { label: "Daily midnight (0 0 * * *)", expr: "0 0 * * *" },
                    { label: "Daily 3 AM (0 3 * * *)", expr: "0 3 * * *" },
                    { label: "Weekly Sunday (0 0 * * 0)", expr: "0 0 * * 0" },
                    { label: "Monthly 1st (0 0 1 * *)", expr: "0 0 1 * *" },
                  ].map((p) => (
                    <Chip
                      key={p.expr}
                      size="small"
                      label={p.label}
                      onClick={() => {
                        setFormCronExpr(p.expr);
                        setFormMain(`${p.expr} ${formCronCmd.trim()}`.trim());
                      }}
                      sx={{
                        cursor: "pointer",
                        fontSize: "0.72rem",
                        fontWeight: formCronExpr === p.expr ? 700 : 500,
                        bgcolor: formCronExpr === p.expr ? "#F59E0B" : "#FFFFFF",
                        color: formCronExpr === p.expr ? "#FFFFFF" : "#78350F",
                        border: "1px solid #FCD34D",
                        "&:hover": { bgcolor: "#FDE68A" },
                      }}
                    />
                  ))}
                </Box>
              </Box>

              {/* Separated Fields */}
              <Grid container spacing={2}>
                <Grid item xs={12} md={5}>
                  <TextField
                    fullWidth
                    label="Cron Expression (Schedule)"
                    placeholder="0 0 * * *"
                    value={formCronExpr}
                    onChange={(e) => {
                      setFormCronExpr(e.target.value);
                      setFormMain(`${e.target.value.trim()} ${formCronCmd.trim()}`.trim());
                    }}
                    helperText="5 standard fields: minute hour day-of-month month day-of-week"
                    InputProps={{
                      style: { fontFamily: "monospace", fontSize: "0.88rem", fontWeight: 700 },
                    }}
                  />
                </Grid>
                <Grid item xs={12} md={7}>
                  <TextField
                    fullWidth
                    label="Command Line (Executable)"
                    placeholder="/root/backup.sh"
                    value={formCronCmd}
                    onChange={(e) => {
                      setFormCronCmd(e.target.value);
                      setFormMain(`${formCronExpr.trim()} ${e.target.value.trim()}`.trim());
                    }}
                    helperText="Command or script to execute on the node at scheduled times. Supports variables like ${PORT}, ${NODE_IP}, ${NODE_NAME}"
                    InputProps={{
                      style: { fontFamily: "monospace", fontSize: "0.85rem" },
                    }}
                  />
                </Grid>
              </Grid>

              {/* Crontab Block Live Preview */}
              <Paper
                elevation={0}
                sx={{
                  p: 2,
                  borderRadius: "10px",
                  bgcolor: "#0F172A",
                  color: "#F8FAFC",
                  border: "1px solid #1E293B",
                }}
              >
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 700, textTransform: "uppercase" }}>
                    Crontab Block Preview (Installed on Target Node)
                  </Typography>
                  <Chip
                    size="small"
                    label={formEnabled ? "Active" : "Disabled (Commented with #)"}
                    sx={{
                      height: 20,
                      fontSize: "0.68rem",
                      fontWeight: 700,
                      bgcolor: formEnabled ? "rgba(16, 185, 129, 0.2)" : "rgba(148, 163, 184, 0.2)",
                      color: formEnabled ? "#34D399" : "#94A3B8",
                    }}
                  />
                </Box>
                <Box
                  component="pre"
                  sx={{
                    m: 0,
                    p: 1.5,
                    bgcolor: "#020617",
                    borderRadius: "6px",
                    fontFamily: "monospace",
                    fontSize: "0.85rem",
                    lineHeight: 1.6,
                    color: "#E2E8F0",
                    overflowX: "auto",
                  }}
                >
                  <code>
                    {`### EASY42_START
${formEnabled ? `${formCronExpr.trim() || "0 0 * * *"} ${formCronCmd.trim() || "/root/backup.sh"}` : `#${formCronExpr.trim() || "0 0 * * *"} ${formCronCmd.trim() || "/root/backup.sh"}`}
### EASY42_END`}
                  </code>
                </Box>
              </Paper>
            </Box>
          )}

          {/* TAB 2: Definition Editor (Docker / Systemd) */}
          {modalTab === "definition" && formType !== "cron" && (
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
                              user: systemdUser || "root",
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
                    Injected into app directory (./filename) for Docker or /etc/ezapp/&lt;name&gt;/ for native daemons.
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

          {/* TAB 5: Note (Markdown) */}
          {modalTab === "note" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Box
                sx={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  flexWrap: "wrap",
                  gap: 1,
                }}
              >
                <Box>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    App Note (Markdown)
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Markdown documentation for this app — usage notes, runbooks, provisioning steps, and more.
                  </Typography>
                </Box>
                <Box sx={{ display: "flex", gap: 1 }}>
                  <Button
                    size="small"
                    variant={noteEditMode === "write" ? "contained" : "outlined"}
                    onClick={() => setNoteEditMode("write")}
                    sx={{ textTransform: "none", fontSize: "0.78rem" }}
                  >
                    Write
                  </Button>
                  <Button
                    size="small"
                    variant={noteEditMode === "preview" ? "contained" : "outlined"}
                    onClick={() => setNoteEditMode("preview")}
                    sx={{ textTransform: "none", fontSize: "0.78rem" }}
                  >
                    Preview
                  </Button>
                </Box>
              </Box>

              {noteEditMode === "write" ? (
                <TextField
                  fullWidth
                  multiline
                  minRows={12}
                  maxRows={22}
                  placeholder={
                    "# App documentation\n\nWrite deployment notes, access instructions, or operational runbooks in Markdown.\n\nUse a ```json block to store structured data (optional)."
                  }
                  value={formNote}
                  onChange={(e) => setFormNote(e.target.value)}
                  inputProps={{
                    style: {
                      fontFamily: "monospace",
                      fontSize: "0.85rem",
                      lineHeight: 1.5,
                    },
                  }}
                  sx={{
                    bgcolor: "#F8FAFC",
                    "& .MuiOutlinedInput-root": { bgcolor: "#F8FAFC" },
                  }}
                />
              ) : (
                <Box
                  sx={{
                    p: 2,
                    bgcolor: "#F8FAFC",
                    borderRadius: "10px",
                    border: "1px solid #E2E8F0",
                    minHeight: 240,
                    maxHeight: 480,
                    overflowY: "auto",
                  }}
                >
                  <MarkdownView content={formNote} emptyText="No note content written yet." />
                </Box>
              )}
            </Box>
          )}

          {/* TAB 6: Variables Interpolation Live Preview */}
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

          {/* TAB: Artifact Spec (Native Systemd only) */}
          {modalTab === "artifact" && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
              <Box
                sx={{
                  p: 2,
                  bgcolor: formArtifactEnabled ? "#F0FDF4" : "#F8FAFC",
                  borderRadius: "12px",
                  border: `1px solid ${formArtifactEnabled ? "#BBF7D0" : "#E2E8F0"}`,
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                }}
              >
                <Box sx={{ pr: 2 }}>
                  <Typography
                    variant="subtitle2"
                    sx={{ fontWeight: 800, color: "#0F172A", display: "flex", alignItems: "center", gap: 1 }}
                  >
                    <Package size={18} color={formArtifactEnabled ? "#16A34A" : "#4F46E5"} />
                    Declarative Artifact Management
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.5 }}>
                    Enable this to automatically download, verify, decompress, and deploy app binaries or archives to
                    nodes. The native service will only start after all required artifacts are verified on the node.
                  </Typography>
                </Box>
                <FormControlLabel
                  control={
                    <Switch
                      checked={formArtifactEnabled}
                      onChange={(e) => setFormArtifactEnabled(e.target.checked)}
                      color="success"
                    />
                  }
                  label={
                    <Typography
                      sx={{ fontWeight: 700, fontSize: "0.85rem", color: formArtifactEnabled ? "#15803D" : "#64748B" }}
                    >
                      {formArtifactEnabled ? "Enabled" : "Disabled"}
                    </Typography>
                  }
                />
              </Box>

              {formArtifactEnabled && (
                <>
                  {/* Version & Source Mode Selector */}
                  <Grid container spacing={2}>
                    <Grid item xs={12} sm={4}>
                      <TextField
                        label="Version"
                        size="small"
                        fullWidth
                        placeholder="e.g. v1.68.1 or latest"
                        value={formVersion}
                        onChange={(e) => setFormVersion(e.target.value)}
                        helperText="Available via ${VERSION} and ${RAW_VERSION}"
                      />
                    </Grid>
                    <Grid item xs={12} sm={8}>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, height: "100%", pt: 0.5 }}>
                        <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                          Source:
                        </Typography>
                        <ToggleButtonGroup
                          size="small"
                          exclusive
                          value={formArtifactSourceMode}
                          onChange={(_, val) => val && setFormArtifactSourceMode(val)}
                        >
                          <ToggleButton
                            value="url"
                            sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.78rem", px: 2 }}
                          >
                            URL Template
                          </ToggleButton>
                          <ToggleButton
                            value="github"
                            sx={{ textTransform: "none", fontWeight: 600, fontSize: "0.78rem", px: 2 }}
                          >
                            GitHub Releases Resolver
                          </ToggleButton>
                        </ToggleButtonGroup>
                      </Box>
                    </Grid>
                  </Grid>

                  {/* Mode A: URL Template */}
                  {formArtifactSourceMode === "url" && (
                    <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: "10px", border: "1px solid #E2E8F0" }}>
                      <TextField
                        label="Download URL Template"
                        size="small"
                        fullWidth
                        placeholder="https://downloads.rclone.org/${VERSION}/rclone-${VERSION}-linux-${ARCH}.zip"
                        value={formArtifactUrl}
                        onChange={(e) => setFormArtifactUrl(e.target.value)}
                        helperText="Supports tar.gz, zip, tar.bz2, gz, or raw binary. Easy42 Go server caches and serves files to node agents."
                      />
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap", mt: 1.5 }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                          Insert Variables:
                        </Typography>
                        {[
                          { label: "${VERSION}", desc: "Version tag (e.g. v1.68.1)" },
                          { label: "${RAW_VERSION}", desc: "Version without 'v' prefix (e.g. 1.68.1)" },
                          { label: "${ARCH}", desc: "Go format: amd64 / arm64 / 386 / armv7 / mipsel (MIPS endianness resolved)" },
                          { label: "${ARCH_GNU}", desc: "GNU/C/Rust format: x86_64 / aarch64 / i386 / armv7l / mipsel" },
                          { label: "${ARCH_RUST}", desc: "Rust target format: x86_64 / aarch64 / armv7 / mipsel (little-endian MIPS)" },
                          { label: "${ARCH_DEB}", desc: "Debian format: amd64 / arm64 / armhf" },
                          { label: "${OS}", desc: "OS name (linux)" },
                          { label: "${APP_NAME}", desc: "App name" },
                        ].map((item) => (
                          <Tooltip key={item.label} title={item.desc} arrow>
                            <Chip
                              label={item.label}
                              size="small"
                              onClick={() => setFormArtifactUrl((prev) => prev + item.label)}
                              sx={{
                                height: 22,
                                fontSize: "0.7rem",
                                fontFamily: "monospace",
                                cursor: "pointer",
                                bgcolor: "#EEF2FF",
                                color: "#4F46E5",
                                "&:hover": { bgcolor: "#E0E7FF" },
                              }}
                            />
                          </Tooltip>
                        ))}
                      </Box>
                    </Box>
                  )}

                  {/* Mode B: GitHub Releases Resolver */}
                  {formArtifactSourceMode === "github" && (
                    <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: "10px", border: "1px solid #E2E8F0" }}>
                      <Grid container spacing={2}>
                        <Grid item xs={12} sm={5}>
                          <TextField
                            label="GitHub Repository"
                            size="small"
                            fullWidth
                            placeholder="e.g. rclone/rclone"
                            value={formArtifactGithubRepo}
                            onChange={(e) => setFormArtifactGithubRepo(e.target.value)}
                            helperText="GitHub owner/repo"
                          />
                        </Grid>
                        <Grid item xs={12} sm={3}>
                          <TextField
                            label="Release Tag / Version"
                            size="small"
                            fullWidth
                            placeholder="${VERSION}"
                            value={formArtifactGithubVersion}
                            onChange={(e) => setFormArtifactGithubVersion(e.target.value)}
                            helperText="Default: ${VERSION} or 'latest'"
                          />
                        </Grid>
                        <Grid item xs={12} sm={4}>
                          <TextField
                            label="Asset Pattern (Optional)"
                            size="small"
                            fullWidth
                            placeholder="rclone-*-linux-${ARCH_GNU}.zip"
                            value={formArtifactGithubPattern}
                            onChange={(e) => setFormArtifactGithubPattern(e.target.value)}
                            helperText="Glob / regex pattern to pick asset"
                          />
                        </Grid>
                      </Grid>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap", mt: 1.5 }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                          Insert Variables into Pattern:
                        </Typography>
                        {[
                          { label: "${VERSION}", desc: "Version tag (e.g. v1.68.1)" },
                          { label: "${RAW_VERSION}", desc: "Version without 'v' prefix" },
                          { label: "${ARCH}", desc: "Go format: amd64 / arm64 / mipsel" },
                          { label: "${ARCH_GNU}", desc: "GNU/C/Rust format: x86_64 / aarch64 / mipsel" },
                          { label: "${ARCH_RUST}", desc: "Rust target format: x86_64 / aarch64 / mipsel" },
                          { label: "${ARCH_DEB}", desc: "Debian format: amd64 / arm64" },
                          { label: "${OS}", desc: "OS name (linux)" },
                        ].map((item) => (
                          <Tooltip key={item.label} title={item.desc} arrow>
                            <Chip
                              label={item.label}
                              size="small"
                              onClick={() => setFormArtifactGithubPattern((prev) => prev + item.label)}
                              sx={{
                                height: 22,
                                fontSize: "0.7rem",
                                fontFamily: "monospace",
                                cursor: "pointer",
                                bgcolor: "#EEF2FF",
                                color: "#4F46E5",
                                "&:hover": { bgcolor: "#E0E7FF" },
                              }}
                            />
                          </Tooltip>
                        ))}
                      </Box>
                    </Box>
                  )}

                  {/* Extracted & Deployed Files */}
                  <Box sx={{ p: 2, bgcolor: "#FFFFFF", borderRadius: "12px", border: "1px solid #E2E8F0" }}>
                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.5 }}>
                      <Box>
                        <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#0F172A" }}>
                          Extracted & Deployed Files ({formArtifactFiles.length})
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Specify the file(s) to unpack from the archive and deploy on the node filesystem.
                        </Typography>
                      </Box>
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<Plus size={14} />}
                        onClick={() =>
                          setFormArtifactFiles((prev) => [
                            ...prev,
                            {
                              name: "",
                              extract_path: "",
                              target_path: "${PATH}/",
                              pull_policy: "if_changed",
                              file_mode: 0o755,
                            },
                          ])
                        }
                        sx={{ textTransform: "none", fontSize: "0.78rem" }}
                      >
                        Add File
                      </Button>
                    </Box>

                    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                      {formArtifactFiles.map((file, idx) => (
                        <Box
                          key={idx}
                          sx={{
                            p: 1.5,
                            bgcolor: "#F8FAFC",
                            borderRadius: "8px",
                            border: "1px solid #E2E8F0",
                            display: "flex",
                            flexDirection: "column",
                            gap: 1.5,
                          }}
                        >
                          <Grid container spacing={1.5} alignItems="center">
                            <Grid item xs={12} sm={3}>
                              <TextField
                                label="File Name"
                                size="small"
                                fullWidth
                                placeholder="rclone"
                                value={file.name}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setFormArtifactFiles((prev) =>
                                    prev.map((f, i) => (i === idx ? { ...f, name: val } : f)),
                                  );
                                }}
                              />
                            </Grid>
                            <Grid item xs={12} sm={3}>
                              <TextField
                                label="Extract Path in Archive"
                                size="small"
                                fullWidth
                                placeholder="rclone-*/rclone"
                                value={file.extract_path || ""}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setFormArtifactFiles((prev) =>
                                    prev.map((f, i) => (i === idx ? { ...f, extract_path: val } : f)),
                                  );
                                }}
                                helperText="Omit if raw binary"
                              />
                            </Grid>
                            <Grid item xs={12} sm={3}>
                              <TextField
                                label="Target Path on Node"
                                size="small"
                                fullWidth
                                placeholder="${PATH}/rclone"
                                value={file.target_path}
                                onChange={(e) => {
                                  const val = e.target.value;
                                  setFormArtifactFiles((prev) =>
                                    prev.map((f, i) => (i === idx ? { ...f, target_path: val } : f)),
                                  );
                                }}
                              />
                            </Grid>
                            <Grid item xs={12} sm={2}>
                              <FormControl size="small" fullWidth>
                                <InputLabel>Pull Policy</InputLabel>
                                <Select
                                  label="Pull Policy"
                                  value={file.pull_policy || "if_changed"}
                                  onChange={(e) => {
                                    const val = e.target.value;
                                    setFormArtifactFiles((prev) =>
                                      prev.map((f, i) => (i === idx ? { ...f, pull_policy: val } : f)),
                                    );
                                  }}
                                >
                                  <MenuItem value="if_changed">if_changed (default)</MenuItem>
                                  <MenuItem value="if_not_present">if_not_present</MenuItem>
                                  <MenuItem value="never">never</MenuItem>
                                </Select>
                              </FormControl>
                            </Grid>
                            <Grid item xs={12} sm={1} sx={{ textAlign: "right" }}>
                              <IconButton
                                size="small"
                                color="error"
                                disabled={formArtifactFiles.length === 1}
                                onClick={() => setFormArtifactFiles((prev) => prev.filter((_, i) => i !== idx))}
                              >
                                <Trash2 size={16} />
                              </IconButton>
                            </Grid>
                          </Grid>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <Typography variant="caption" sx={{ color: "#64748B" }}>
                              Target path helpers:
                            </Typography>
                            <Chip
                              size="small"
                              label="${PATH}"
                              onClick={() => {
                                setFormArtifactFiles((prev) =>
                                  prev.map((f, i) =>
                                    i === idx ? { ...f, target_path: "${PATH}/" + (f.name || "") } : f,
                                  ),
                                );
                              }}
                              sx={{ height: 20, fontSize: "0.68rem", cursor: "pointer" }}
                            />
                            <Chip
                              size="small"
                              label="${EZAPP_DIR}"
                              onClick={() => {
                                setFormArtifactFiles((prev) =>
                                  prev.map((f, i) =>
                                    i === idx ? { ...f, target_path: "${EZAPP_DIR}/bin/" + (f.name || "") } : f,
                                  ),
                                );
                              }}
                              sx={{ height: 20, fontSize: "0.68rem", cursor: "pointer" }}
                            />
                          </Box>
                        </Box>
                      ))}
                    </Box>
                  </Box>

                  {/* Server Artifact Cache & Origin Validation Section */}
                  {editingApp && renderArtifactCacheCard(editingApp)}
                </>
              )}
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

      {/* Standalone Artifact Cache Management Dialog */}
      <Dialog
        open={Boolean(artifactCacheModalApp)}
        onClose={() => setArtifactCacheModalApp(null)}
        maxWidth="md"
        fullWidth
        PaperProps={{
          sx: {
            borderRadius: "16px",
            boxShadow: "0 20px 40px rgba(0,0,0,0.15)",
            border: "1px solid #E2E8F0",
          },
        }}
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            pb: 1.5,
            borderBottom: "1px solid #F1F5F9",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box
              sx={{
                width: 32,
                height: 32,
                borderRadius: "8px",
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Package size={18} />
            </Box>
            <Box>
              <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                Artifact Cache: {artifactCacheModalApp?.name}
              </Typography>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Inspect server cached files, trigger validation against origin, or purge disk files
              </Typography>
            </Box>
          </Box>
          <IconButton size="small" onClick={() => setArtifactCacheModalApp(null)}>
            <X size={16} />
          </IconButton>
        </DialogTitle>
        <DialogContent sx={{ p: 2.5 }}>
          {artifactCacheModalApp && renderArtifactCacheCard(artifactCacheModalApp)}
        </DialogContent>
        <DialogActions sx={{ px: 2.5, py: 1.5, borderTop: "1px solid #F1F5F9" }}>
          <Button
            onClick={() => setArtifactCacheModalApp(null)}
            sx={{ textTransform: "none", fontWeight: 600, color: "#64748B" }}
          >
            Close
          </Button>
        </DialogActions>
      </Dialog>

      {/* App Details Sidebar Drawer */}
      <AppDetailsDrawer
        app={detailsApp}
        open={!!detailsApp}
        onClose={() => setDetailsApp(null)}
        targetNodeNames={detailsApp ? getTargetNodeInfo(detailsApp).nodeNames : undefined}
        targetModeLabel={
          detailsApp
            ? (() => {
                const mode = getTargetNodeInfo(detailsApp).mode;
                return mode === "manual"
                  ? `Manual`
                  : mode === "tag"
                    ? `Tag: #${getSelectorValue(detailsApp.selector || {}, "tag")}`
                    : mode === "custom"
                      ? "Selector"
                      : "All Nodes";
              })()
            : undefined
        }
        homepageUrl={
          detailsApp ? resolveHomepageUrl(detailsApp, managedNodes, variables)?.url || detailsApp.url : undefined
        }
        homepageNode={detailsApp ? resolveHomepageUrl(detailsApp, managedNodes, variables)?.node.name : undefined}
        homepageClickable={
          detailsApp
            ? (() => {
                const r = resolveHomepageUrl(detailsApp, managedNodes, variables);
                return !!r && !r.containsTemplate && isHttpUrl(r.url);
              })()
            : false
        }
        onEdit={(app) => {
          setDetailsApp(null);
          openEditAppModal(app);
        }}
      />
    </Box>
  );
};
