import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useMesh } from "../../context/MeshContext";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  CircularProgress,
  Alert,
  IconButton,
  Tabs,
  Tab,
  InputAdornment,
  Chip,
  Switch,
  FormControlLabel,
  MenuItem,
  Checkbox,
  Divider,
  Tooltip,
  Paper,
} from "@mui/material";
import {
  KeyRound,
  Eye,
  EyeOff,
  LogOut,
  ShieldAlert,
  CheckCircle2,
  X,
  Globe,
  Shield,
  Plus,
  Edit2,
  Trash2,
  Sliders,
  Lock,
  Copy,
  Cloud,
  RefreshCw,
  Zap,
  FileCode,
  Search,
  Check,
  Activity,
  CalendarClock,
  Clock,
  Play,
  AlertCircle,
  Database,
  HardDrive,
  Sparkles,
  Info,
  ExternalLink,
  Cpu,
  GitCommit,
  Send,
  Bot,
  ChevronDown,
  ChevronUp,
} from "lucide-react";
import { api } from "../../api/client";
import {
  NetworkSettings,
  NetworkPolicy,
  CloudflareDNSConfig,
  DNSSyncResult,
  ConfigTemplate,
  ServerTask,
  DatabaseInfo,
  VersionInfo,
  IntegrationsSettings,
} from "../../types/api";
import { ViewNetworkPolicyModal } from "./ViewNetworkPolicyModal";
import TextFieldWithCopy from "../Common/TextFieldWithCopy";
import { Easy42Logo } from "../Logo/Easy42Logo";
import { flushCacheAndServiceWorker } from "../../utils/pwa";

export const parsePrefixList = (input: string): string[] => {
  const result: string[] = [];
  let current = "";
  let inBraces = false;

  for (let i = 0; i < input.length; i++) {
    const char = input[i];
    if (char === "{") {
      inBraces = true;
      current += char;
    } else if (char === "}") {
      inBraces = false;
      current += char;
    } else if (inBraces && (char === " " || char === "\t")) {
      continue;
    } else if ((char === "," || char === "\n" || char === "\r") && !inBraces) {
      const trimmed = current.trim();
      if (trimmed) {
        result.push(trimmed);
      }
      current = "";
    } else {
      current += char;
    }
  }
  const trimmed = current.trim();
  if (trimmed) {
    result.push(trimmed);
  }
  return result;
};

export const parsePortList = (input: string): string[] => {
  return input
    .split(/[\n,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
};

export const parseWhitelistList = (input: string): string[] => {
  return input
    .split(/[\n,\s]+/)
    .map((s) => s.trim())
    .filter(Boolean);
};

export const IMPORT_FILTER_EXAMPLES = [
  {
    label: "Adjust Local Pref",
    desc: "Set local_pref based on incoming community",
    snippet: `# Adjust bgp_local_pref according to incoming community\nif (64512, 100) ~ bgp_community then bgp_local_pref = 200;\nelse if (64512, 50) ~ bgp_community then bgp_local_pref = 50;\n`,
  },
  {
    label: "Add Large Community",
    desc: "Tag accepted routes with a large community",
    snippet: `# Add custom large community to imported routes\nbgp_large_community.add((4242421234, 1, 1));\n`,
  },
  {
    label: "Filter by Community",
    desc: "Reject routes matching a specific community",
    snippet: `# Reject blackhole or do-not-import community\nif (64512, 666) ~ bgp_community then reject;\n`,
  },
  {
    label: "Remove Community",
    desc: "Strip specific communities on import",
    snippet: `# Remove peer's internal communities\nbgp_community.delete([(64512, *)]);\n`,
  },
];

export const EXPORT_FILTER_EXAMPLES = [
  {
    label: "Add Large Community",
    desc: "Tag exported routes with an egress community",
    snippet: `# Tag outgoing routes with community\nbgp_large_community.add((4242421234, 2, 1));\n`,
  },
  {
    label: "Strip Communities",
    desc: "Remove internal communities before announcing",
    snippet: `# Strip internal communities before export\nbgp_community.delete([(64512, *)]);\nbgp_large_community.delete([(4242421234, *, *)]);\n`,
  },
  {
    label: "AS-Path Prepend",
    desc: "Prepend ASN to deprioritize path",
    snippet: `# Deprioritize this link by prepending own AS on export\nbgp_path.prepend(SELF_AS);\n`,
  },
  {
    label: "Filter by Community",
    desc: "Do not export routes with specific community",
    snippet: `# Do not export routes marked no-export\nif (64512, 666) ~ bgp_community then reject;\n`,
  },
];

export type SettingsTab =
  "network" | "security" | "policies" | "templates" | "dns" | "tasks" | "database" | "integrations" | "about";

const VALID_TABS: Record<string, SettingsTab> = {
  network: "network",
  security: "security",
  password: "security",
  sessions: "security",
  policies: "policies",
  templates: "templates",
  dns: "dns",
  tasks: "tasks",
  database: "database",
  integrations: "integrations",
  about: "about",
};

interface SettingsModalProps {
  open?: boolean;
  onClose?: () => void;
  onLogoutAll: () => void;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({ onClose, onLogoutAll }) => {
  const { tab } = useParams<{ tab?: string }>();
  const navigate = useNavigate();
  const { refreshNetworkSettings } = useMesh();

  const activeTab: SettingsTab = (tab && VALID_TABS[tab]) || "network";

  useEffect(() => {
    if (tab === "password" || tab === "sessions") {
      navigate("/settings/security", { replace: true });
    } else if (!tab || !VALID_TABS[tab]) {
      navigate("/settings/network", { replace: true });
    }
  }, [tab, navigate]);

  // Config Templates state
  const [templates, setTemplates] = useState<ConfigTemplate[]>([]);
  const [templatesLoading, setTemplatesLoading] = useState(false);
  const [templatesError, setTemplatesError] = useState<string | null>(null);
  const [templatesSuccess, setTemplatesSuccess] = useState<string | null>(null);
  const [templateTypeFilter, setTemplateTypeFilter] = useState<"all" | "wg" | "bird" | "nft">("all");
  const [templateSearch, setTemplateSearch] = useState("");

  // Template Dialog state (create / edit / view)
  const [templateDialogOpen, setTemplateDialogOpen] = useState(false);
  const [templateDialogMode, setTemplateDialogMode] = useState<"create" | "edit" | "view">("create");
  const [templateId, setTemplateId] = useState("");
  const [templateName, setTemplateName] = useState("");
  const [templateType, setTemplateType] = useState<"wg" | "bird" | "nft">("bird");
  const [templateDesc, setTemplateDesc] = useState("");
  const [templateContent, setTemplateContent] = useState("");
  const [templateDialogError, setTemplateDialogError] = useState<string | null>(null);
  const [templateDialogSaving, setTemplateDialogSaving] = useState(false);
  const [copiedTemplateContent, setCopiedTemplateContent] = useState(false);

  // Template Delete confirmation state
  const [templateToDelete, setTemplateToDelete] = useState<ConfigTemplate | null>(null);
  const [templateDeleting, setTemplateDeleting] = useState(false);

  // Cloudflare DNS state
  const [cfZoneId, setCfZoneId] = useState("");
  const [cfApiToken, setCfApiToken] = useState("");
  const [cfBaseDomain, setCfBaseDomain] = useState("");
  const [cfPublishIPv6OwnName, setCfPublishIPv6OwnName] = useState(false);
  const [showCfToken, setShowCfToken] = useState(false);
  const [dnsLoading, setDnsLoading] = useState(false);
  const [dnsSaving, setDnsSaving] = useState(false);
  const [dnsSyncing, setDnsSyncing] = useState(false);
  const [dnsForceSyncing, setDnsForceSyncing] = useState(false);
  const [dnsError, setDnsError] = useState<string | null>(null);
  const [dnsSuccess, setDnsSuccess] = useState<string | null>(null);
  const [dnsSyncResult, setDnsSyncResult] = useState<DNSSyncResult | null>(null);

  // Server Background Tasks state
  const [serverTasks, setServerTasks] = useState<ServerTask[]>([]);
  const [tasksLoading, setTasksLoading] = useState(false);
  const [tasksError, setTasksError] = useState<string | null>(null);
  const [tasksSuccess, setTasksSuccess] = useState<string | null>(null);
  const [taskCronDraft, setTaskCronDraft] = useState<Record<string, string>>({});
  const [taskEnabledDraft, setTaskEnabledDraft] = useState<Record<string, boolean>>({});
  const [savingTaskId, setSavingTaskId] = useState<string | null>(null);
  const [runningTaskId, setRunningTaskId] = useState<string | null>(null);

  // Database & Storage state
  const [dbInfo, setDbInfo] = useState<DatabaseInfo | null>(null);
  const [dbLoading, setDbLoading] = useState(false);
  const [dbSaving, setDbSaving] = useState(false);
  const [dbVacuuming, setDbVacuuming] = useState(false);
  const [dbError, setDbError] = useState<string | null>(null);
  const [dbSuccess, setDbSuccess] = useState<string | null>(null);
  const [retentionDraft, setRetentionDraft] = useState<number | string>(47);
  const [rawRetentionDraft, setRawRetentionDraft] = useState<number | string>(2);

  // Integrations & Prometheus state
  const [integrations, setIntegrations] = useState<IntegrationsSettings | null>(null);
  const [integrationsLoading, setIntegrationsLoading] = useState(false);
  const [integrationsSaving, setIntegrationsSaving] = useState(false);
  const [integrationsError, setIntegrationsError] = useState<string | null>(null);
  const [integrationsSuccess, setIntegrationsSuccess] = useState<string | null>(null);
  const [showAuthToken, setShowAuthToken] = useState(false);
  const [resetTokenConfirmOpen, setResetTokenConfirmOpen] = useState(false);
  const [resettingToken, setResettingToken] = useState(false);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  // Telegram Bot integration state
  const [tgEnabled, setTgEnabled] = useState(false);
  const [tgBotToken, setTgBotToken] = useState("");
  const [tgAdminID, setTgAdminID] = useState("");
  const [tgServerURL, setTgServerURL] = useState("");
  const [showTgToken, setShowTgToken] = useState(false);
  const [tgSaving, setTgSaving] = useState(false);
  const [tgTesting, setTgTesting] = useState(false);
  const [tgTestSuccess, setTgTestSuccess] = useState<string | null>(null);
  const [tgTestError, setTgTestError] = useState<string | null>(null);
  const [tgGuideOpen, setTgGuideOpen] = useState(true);

  // About & Version state
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [versionLoading, setVersionLoading] = useState(false);
  const [versionError, setVersionError] = useState<string | null>(null);
  const [versionCopied, setVersionCopied] = useState(false);
  const [swFlushLoading, setSwFlushLoading] = useState(false);
  const [swFlushResult, setSwFlushResult] = useState<{ text: string; severity: "success" | "error" } | null>(null);

  const handleFlushCacheAndSw = async () => {
    setSwFlushLoading(true);
    setSwFlushResult(null);
    try {
      const res = await flushCacheAndServiceWorker();
      if (res.success) {
        setSwFlushResult({
          text: `Service worker uninstalled (${res.uninstalled}) and cache storage cleared (${res.cachesDeleted} bucket(s)). Reloading page...`,
          severity: "success",
        });
        setTimeout(() => {
          window.location.reload();
        }, 1200);
      } else {
        setSwFlushResult({
          text: res.error || "Failed to flush cache & service worker",
          severity: "error",
        });
      }
    } catch (err: unknown) {
      setSwFlushResult({
        text: (err as Error).message || "Failed to flush cache & service worker",
        severity: "error",
      });
    } finally {
      setSwFlushLoading(false);
    }
  };

  // Password change state
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [passwordSuccess, setPasswordSuccess] = useState<string | null>(null);

  // Network / Peering state
  const [publicAsn, setPublicAsn] = useState<number | "">("");
  const [disallowedDn42Networks, setDisallowedDn42Networks] = useState("");
  const [networkPrefixes, setNetworkPrefixes] = useState<string[]>([]);
  const [faAddress, setFaAddress] = useState("");
  const [faPort, setFaPort] = useState<number | "">(1790);
  const [faAsn, setFaAsn] = useState<number | "">(4224420000);
  const [faUrl, setFaUrl] = useState("");
  const [networkLoading, setNetworkLoading] = useState(false);
  const [networkSaving, setNetworkSaving] = useState(false);
  const [networkError, setNetworkError] = useState<string | null>(null);
  const [networkSuccess, setNetworkSuccess] = useState<string | null>(null);

  // Network Policies state
  const [policies, setPolicies] = useState<NetworkPolicy[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [policiesSuccess, setPoliciesSuccess] = useState<string | null>(null);

  // Policy editor / viewer dialog state
  const [policyDialogOpen, setPolicyDialogOpen] = useState(false);
  const [policyDialogMode, setPolicyDialogMode] = useState<"view" | "edit" | "create">("edit");
  const [policyDialogError, setPolicyDialogError] = useState<string | null>(null);
  const [policyDialogSaving, setPolicyDialogSaving] = useState(false);

  // Standalone View Policy Modal
  const [viewPolicyModalOpen, setViewPolicyModalOpen] = useState(false);
  const [policyToView, setPolicyToView] = useState<NetworkPolicy | null>(null);

  const [policyId, setPolicyId] = useState("");
  const [policyName, setPolicyName] = useState("");
  const [policyDesc, setPolicyDesc] = useState("");
  const [policyCost, setPolicyCost] = useState<number | string>(100);
  const [policyAllowedDst, setPolicyAllowedDst] = useState("");
  const [policyAllowedSrc, setPolicyAllowedSrc] = useState("");
  const [policyDisallowedDst, setPolicyDisallowedDst] = useState("");
  const [policyDisallowedSrc, setPolicyDisallowedSrc] = useState("");
  const [policyLocalNetworks, setPolicyLocalNetworks] = useState("");
  const [policyRejectInternet, setPolicyRejectInternet] = useState(true);
  const [policyFilterForward, setPolicyFilterForward] = useState(true);
  const [policyFilterInput, setPolicyFilterInput] = useState(false);
  const [policyInputAllowIcmp, setPolicyInputAllowIcmp] = useState(true);
  const [policyInputAllowIcmp6, setPolicyInputAllowIcmp6] = useState(true);
  const [policyInputTcpPorts, setPolicyInputTcpPorts] = useState("");
  const [policyInputUdpPorts, setPolicyInputUdpPorts] = useState("");
  const [policySnatEnabled, setPolicySnatEnabled] = useState(false);
  const [policySnatCondition, setPolicySnatCondition] = useState<string>("not_dst");
  const [policySnatTarget, setPolicySnatTarget] = useState<string>("masquerade");
  const [policyForwardSnat, setPolicyForwardSnat] = useState(false);
  const [policyForwardSnatTarget, setPolicyForwardSnatTarget] = useState<string>("masquerade");
  const [policyRoa4, setPolicyRoa4] = useState("");
  const [policyRoa6, setPolicyRoa6] = useState("");
  const [policyRoaStrict, setPolicyRoaStrict] = useState(false);
  const [policyDscpIngress, setPolicyDscpIngress] = useState<number | string>("");
  const [policyDscpEgress, setPolicyDscpEgress] = useState<number | string>("");
  const [policyFwmark, setPolicyFwmark] = useState("");
  const [policyPreference, setPolicyPreference] = useState<number | string>("");
  const [policyMark, setPolicyMark] = useState("");
  const [policyRoutingPolicy, setPolicyRoutingPolicy] = useState<string>("full");
  const [policyBlockIngressNew, setPolicyBlockIngressNew] = useState<string>("");
  const [policyIngressNewWhitelist, setPolicyIngressNewWhitelist] = useState<string>("");
  const [policyBirdImportFilter, setPolicyBirdImportFilter] = useState<string>("");
  const [policyBirdExportFilter, setPolicyBirdExportFilter] = useState<string>("");

  const handleAppendImportExample = (snippet: string) => {
    setPolicyBirdImportFilter((prev) => {
      const trimmed = prev.trim();
      if (!trimmed) return snippet;
      return `${trimmed}\n\n${snippet}`;
    });
  };

  const handleAppendExportExample = (snippet: string) => {
    setPolicyBirdExportFilter((prev) => {
      const trimmed = prev.trim();
      if (!trimmed) return snippet;
      return `${trimmed}\n\n${snippet}`;
    });
  };

  // Logout all state
  const [logoutAllConfirming, setLogoutAllConfirming] = useState(false);
  const [logoutAllLoading, setLogoutAllLoading] = useState(false);
  const [logoutAllError, setLogoutAllError] = useState<string | null>(null);

  // Load network settings when network tab is opened
  React.useEffect(() => {
    if (activeTab === "network") {
      loadNetworkSettings();
    }
  }, [activeTab]);

  // Load policies when policies tab is opened
  React.useEffect(() => {
    if (activeTab === "policies") {
      loadPolicies();
    }
  }, [activeTab]);

  // Load templates when templates tab is opened
  React.useEffect(() => {
    if (activeTab === "templates") {
      loadTemplates();
    }
  }, [activeTab]);

  const loadTemplates = async () => {
    setTemplatesLoading(true);
    setTemplatesError(null);
    try {
      const data = await api.getTemplates();
      setTemplates(data || []);
    } catch (err: unknown) {
      const e = err as Error;
      setTemplatesError(e.message || "Failed to load templates.");
    } finally {
      setTemplatesLoading(false);
    }
  };

  const handleOpenCreateTemplate = (initialType?: "wg" | "bird" | "nft") => {
    setTemplateDialogMode("create");
    setTemplateId("");
    setTemplateName("");
    setTemplateType(initialType || "bird");
    setTemplateDesc("");
    setTemplateContent("");
    setTemplateDialogError(null);
    setTemplateDialogOpen(true);
  };

  const handleOpenViewTemplate = (t: ConfigTemplate) => {
    setTemplateDialogMode("view");
    setTemplateId(t.id);
    setTemplateName(t.name);
    setTemplateType((t.type as "wg" | "bird" | "nft") || "bird");
    setTemplateDesc(t.description || "");
    setTemplateContent(t.content);
    setTemplateDialogError(null);
    setTemplateDialogOpen(true);
  };

  const handleOpenEditTemplate = (t: ConfigTemplate) => {
    setTemplateDialogMode("edit");
    setTemplateId(t.id);
    setTemplateName(t.name);
    setTemplateType((t.type as "wg" | "bird" | "nft") || "bird");
    setTemplateDesc(t.description || "");
    setTemplateContent(t.content);
    setTemplateDialogError(null);
    setTemplateDialogOpen(true);
  };

  const handleOpenCloneTemplate = (t: ConfigTemplate) => {
    setTemplateDialogMode("create");
    const baseId = t.id.replace(/^default_/, "");
    setTemplateId(`${baseId}-copy`);
    setTemplateName(`${t.name} (Copy)`);
    setTemplateType((t.type as "wg" | "bird" | "nft") || "bird");
    setTemplateDesc(t.description || "");
    setTemplateContent(t.content);
    setTemplateDialogError(null);
    setTemplateDialogOpen(true);
  };

  const handleSaveTemplate = async (e: React.FormEvent) => {
    e.preventDefault();
    setTemplateDialogSaving(true);
    setTemplateDialogError(null);

    const id = templateId.trim();
    const name = templateName.trim();
    if (!id) {
      setTemplateDialogError("Template ID is required.");
      setTemplateDialogSaving(false);
      return;
    }
    if (!name) {
      setTemplateDialogError("Template Name is required.");
      setTemplateDialogSaving(false);
      return;
    }
    if (!templateContent.trim()) {
      setTemplateDialogError("Template Content cannot be empty.");
      setTemplateDialogSaving(false);
      return;
    }

    try {
      if (templateDialogMode === "create") {
        await api.createTemplate({
          id,
          name,
          type: templateType,
          description: templateDesc.trim() || undefined,
          content: templateContent,
        });
        setTemplatesSuccess(`Template "${name}" created successfully.`);
      } else {
        await api.updateTemplate(id, {
          name,
          type: templateType,
          description: templateDesc.trim() || undefined,
          content: templateContent,
        });
        setTemplatesSuccess(`Template "${name}" updated successfully.`);
      }
      setTemplateDialogOpen(false);
      await loadTemplates();
    } catch (err: unknown) {
      const e = err as Error;
      setTemplateDialogError(e.message || "Failed to save template.");
    } finally {
      setTemplateDialogSaving(false);
    }
  };

  const handleConfirmDeleteTemplate = async () => {
    if (!templateToDelete) return;
    setTemplateDeleting(true);
    try {
      await api.deleteTemplate(templateToDelete.id);
      setTemplatesSuccess(`Template "${templateToDelete.name}" deleted successfully.`);
      setTemplateToDelete(null);
      await loadTemplates();
    } catch (err: unknown) {
      const e = err as Error;
      setTemplatesError(e.message || "Failed to delete template.");
    } finally {
      setTemplateDeleting(false);
    }
  };

  const getTemplateTypeBadge = (type: string) => {
    switch (type) {
      case "wg":
        return { label: "WireGuard", color: "#6366F1", bg: "rgba(99, 102, 241, 0.1)", border: "#C7D2FE" };
      case "bird":
        return { label: "BIRD 2", color: "#0284C7", bg: "rgba(2, 132, 199, 0.1)", border: "#BAE6FD" };
      case "nft":
        return { label: "nftables", color: "#D97706", bg: "rgba(217, 119, 6, 0.1)", border: "#FDE68A" };
      default:
        return { label: type, color: "#64748B", bg: "rgba(100, 116, 139, 0.1)", border: "#E2E8F0" };
    }
  };

  const filteredTemplates = templates.filter((t) => {
    if (templateTypeFilter !== "all" && t.type !== templateTypeFilter) {
      return false;
    }
    if (templateSearch.trim()) {
      const q = templateSearch.toLowerCase().trim();
      const matchName = t.name.toLowerCase().includes(q);
      const matchId = t.id.toLowerCase().includes(q);
      const matchDesc = (t.description || "").toLowerCase().includes(q);
      if (!matchName && !matchId && !matchDesc) return false;
    }
    return true;
  });

  // Load DNS settings when DNS tab is opened
  React.useEffect(() => {
    if (activeTab === "dns") {
      loadDNSSettings();
    }
  }, [activeTab]);

  const loadDNSSettings = async () => {
    setDnsLoading(true);
    setDnsError(null);
    try {
      const cfg = await api.getDNSConfig();
      setCfZoneId(cfg.zone_id || "");
      setCfApiToken(cfg.api_token || "");
      setCfBaseDomain(cfg.base_domain || "");
      setCfPublishIPv6OwnName(Boolean(cfg.publish_ipv6_own_name));
    } catch (err: unknown) {
      const e = err as Error;
      setDnsError(e.message || "Failed to load Cloudflare DNS settings.");
    } finally {
      setDnsLoading(false);
    }
  };

  const handleSaveDNSSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setDnsSaving(true);
    setDnsError(null);
    setDnsSuccess(null);
    setDnsSyncResult(null);

    const payload: CloudflareDNSConfig = {
      zone_id: cfZoneId.trim(),
      api_token: cfApiToken.trim(),
      base_domain: cfBaseDomain.trim(),
      publish_ipv6_own_name: cfPublishIPv6OwnName,
    };

    try {
      await api.updateDNSConfig(payload);
      setDnsSuccess("Cloudflare DNS settings saved successfully.");
    } catch (err: unknown) {
      const e = err as Error;
      setDnsError(e.message || "Failed to save Cloudflare DNS settings.");
    } finally {
      setDnsSaving(false);
    }
  };

  const handleSyncDNS = async (force: boolean) => {
    if (force) {
      setDnsForceSyncing(true);
    } else {
      setDnsSyncing(true);
    }
    setDnsError(null);
    setDnsSuccess(null);
    setDnsSyncResult(null);

    try {
      const res = await api.syncDNS(force);
      setDnsSyncResult(res);
      if (res.errors && res.errors.length > 0) {
        setDnsError(`Sync completed with ${res.errors.length} issue(s): ${res.errors.join("; ")}`);
      } else {
        setDnsSuccess(
          force
            ? `Force Sync completed: ${res.created} created, ${res.updated} updated, ${res.deleted} deleted, ${res.ignored} unchanged.`
            : `Sync completed: ${res.created} created, ${res.updated} updated, ${res.deleted} deleted, ${res.ignored} unchanged.`,
        );
      }
    } catch (err: unknown) {
      const e = err as Error;
      setDnsError(e.message || "Failed to sync DNS records with Cloudflare.");
    } finally {
      setDnsSyncing(false);
      setDnsForceSyncing(false);
    }
  };

  // Load Server Tasks when Tasks tab is opened
  React.useEffect(() => {
    if (activeTab === "tasks") {
      loadServerTasks();
    }
  }, [activeTab]);

  const loadServerTasks = async () => {
    setTasksLoading(true);
    setTasksError(null);
    try {
      const data = await api.getServerTasks();
      setServerTasks(data);
      const crons: Record<string, string> = {};
      const enabledMap: Record<string, boolean> = {};
      data.forEach((t) => {
        crons[t.id] = t.cron_expr;
        enabledMap[t.id] = t.enabled;
      });
      setTaskCronDraft(crons);
      setTaskEnabledDraft(enabledMap);
    } catch (err: unknown) {
      const e = err as Error;
      setTasksError(e.message || "Failed to load background tasks.");
    } finally {
      setTasksLoading(false);
    }
  };

  // Load Database Info when Database tab is opened
  React.useEffect(() => {
    if (activeTab === "database") {
      loadDatabaseInfo();
    }
  }, [activeTab]);

  const loadDatabaseInfo = async () => {
    setDbLoading(true);
    setDbError(null);
    try {
      const data = await api.getDatabaseInfo();
      setDbInfo(data);
      setRetentionDraft(data.retention_days);
      setRawRetentionDraft(data.raw_retention_days || 2);
    } catch (err: unknown) {
      const e = err as Error;
      setDbError(e.message || "Failed to load database status.");
    } finally {
      setDbLoading(false);
    }
  };

  // Load Version Info when About tab is opened
  React.useEffect(() => {
    if (activeTab === "about") {
      loadVersionInfo();
    }
  }, [activeTab]);

  const loadVersionInfo = async () => {
    setVersionLoading(true);
    setVersionError(null);
    try {
      const data = await api.getVersion();
      setVersionInfo(data);
    } catch (err: unknown) {
      const e = err as Error;
      setVersionError(e.message || "Failed to load version details.");
    } finally {
      setVersionLoading(false);
    }
  };

  const handleSaveRetention = async () => {
    const days = parseInt(String(retentionDraft), 10);
    const rawDays = parseInt(String(rawRetentionDraft), 10);
    if (isNaN(days) || days < 1 || days > 3650) {
      setDbError("Long-term retention must be between 1 and 3650 days (up to 10 years).");
      return;
    }
    if (isNaN(rawDays) || rawDays < 1 || rawDays > 30) {
      setDbError("Raw retention must be between 1 and 30 days.");
      return;
    }
    if (rawDays > days) {
      setDbError("Raw retention cannot exceed long-term retention.");
      return;
    }
    setDbSaving(true);
    setDbError(null);
    setDbSuccess(null);
    try {
      const updated = await api.updateDatabaseSettings({ retention_days: days, raw_retention_days: rawDays });
      setDbInfo(updated);
      setRetentionDraft(updated.retention_days);
      setRawRetentionDraft(updated.raw_retention_days || rawDays);
      setDbSuccess(
        `Retention policy updated: Raw telemetry kept for ${updated.raw_retention_days || rawDays} days; older data summarized into 5-minute statistical windows retained for ${updated.retention_days} days.`,
      );
    } catch (err: unknown) {
      const e = err as Error;
      setDbError(e.message || "Failed to update retention days.");
    } finally {
      setDbSaving(false);
    }
  };

  const handleReclaimDatabase = async () => {
    setDbVacuuming(true);
    setDbError(null);
    setDbSuccess(null);
    try {
      const updated = await api.reclaimDatabase();
      setDbInfo(updated);
      setDbSuccess(`Database reclaimed successfully! Current database size is ${updated.total_size_human}.`);
    } catch (err: unknown) {
      const e = err as Error;
      setDbError(e.message || "Failed to reclaim database space.");
    } finally {
      setDbVacuuming(false);
    }
  };

  // Load Integrations Settings when Integrations tab is opened
  React.useEffect(() => {
    if (activeTab === "integrations") {
      loadIntegrations();
    }
  }, [activeTab]);

  const loadIntegrations = async () => {
    setIntegrationsLoading(true);
    setIntegrationsError(null);
    try {
      const data = await api.getIntegrationsSettings();
      setIntegrations(data);
      setTgEnabled(data.telegram_enabled ?? false);
      setTgBotToken(data.telegram_bot_token || "");
      setTgAdminID(data.telegram_admin_id ? String(data.telegram_admin_id) : "");
      setTgServerURL(data.server_url || window.location.origin);
    } catch (err: unknown) {
      const e = err as Error;
      setIntegrationsError(e.message || "Failed to load integrations settings.");
    } finally {
      setIntegrationsLoading(false);
    }
  };

  const handleSaveTelegram = async () => {
    setTgSaving(true);
    setIntegrationsError(null);
    setIntegrationsSuccess(null);
    setTgTestSuccess(null);
    setTgTestError(null);
    try {
      const adminIdNum = tgAdminID.trim() ? parseInt(tgAdminID.trim(), 10) : 0;
      const updated = await api.updateIntegrationsSettings({
        telegram_enabled: tgEnabled,
        telegram_bot_token: tgBotToken.trim(),
        telegram_admin_id: isNaN(adminIdNum) ? 0 : adminIdNum,
        server_url: tgServerURL.trim() || window.location.origin,
      });
      setIntegrations(updated);
      setTgEnabled(updated.telegram_enabled ?? false);
      setTgBotToken(updated.telegram_bot_token || "");
      setTgAdminID(updated.telegram_admin_id ? String(updated.telegram_admin_id) : "");
      setTgServerURL(updated.server_url || window.location.origin);
      setIntegrationsSuccess("Telegram Bot settings saved successfully.");
    } catch (err: unknown) {
      const e = err as Error;
      setIntegrationsError(e.message || "Failed to save Telegram Bot settings.");
    } finally {
      setTgSaving(false);
    }
  };

  const handleTestTelegram = async () => {
    setTgTesting(true);
    setTgTestSuccess(null);
    setTgTestError(null);
    try {
      const adminIdNum = tgAdminID.trim() ? parseInt(tgAdminID.trim(), 10) : 0;
      const res = await api.testTelegramIntegration({
        telegram_bot_token: tgBotToken.trim(),
        telegram_admin_id: isNaN(adminIdNum) ? 0 : adminIdNum,
        server_url: tgServerURL.trim() || window.location.origin,
      });
      setTgTestSuccess(res.message || `Test notification sent successfully to @${res.bot_username || "your bot"}`);
    } catch (err: unknown) {
      const e = err as Error;
      setTgTestError(e.message || "Failed to test Telegram Bot connection.");
    } finally {
      setTgTesting(false);
    }
  };

  const handleTogglePrometheus = async (enabled: boolean) => {
    if (!integrations) return;
    setIntegrationsSaving(true);
    setIntegrationsError(null);
    setIntegrationsSuccess(null);
    try {
      const updated = await api.updateIntegrationsSettings({ prometheus_enabled: enabled });
      setIntegrations(updated);
      setIntegrationsSuccess(enabled ? "Prometheus metrics export enabled." : "Prometheus metrics export disabled.");
    } catch (err: unknown) {
      const e = err as Error;
      setIntegrationsError(e.message || "Failed to update Prometheus export setting.");
    } finally {
      setIntegrationsSaving(false);
    }
  };

  const handleResetToken = async () => {
    setResettingToken(true);
    setIntegrationsError(null);
    setIntegrationsSuccess(null);
    try {
      const updated = await api.resetPrometheusToken();
      setIntegrations(updated);
      setResetTokenConfirmOpen(false);
      setIntegrationsSuccess("Prometheus auth token successfully reset. Remember to update your scrapers.");
    } catch (err: unknown) {
      const e = err as Error;
      setIntegrationsError(e.message || "Failed to reset Prometheus auth token.");
    } finally {
      setResettingToken(false);
    }
  };

  const handleCopyText = (text: string, fieldId: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedField(fieldId);
    setTimeout(() => {
      setCopiedField((prev) => (prev === fieldId ? null : prev));
    }, 2000);
  };

  const handleSaveTaskConfig = async (taskId: string) => {
    const cronExpr = (taskCronDraft[taskId] || "0 0 * * *").trim();
    const enabled = taskEnabledDraft[taskId] !== undefined ? taskEnabledDraft[taskId] : true;
    setSavingTaskId(taskId);
    setTasksError(null);
    setTasksSuccess(null);
    try {
      const updated = await api.updateServerTask(taskId, { cron_expr: cronExpr, enabled });
      setServerTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
      setTaskCronDraft((prev) => ({ ...prev, [taskId]: updated.cron_expr }));
      setTaskEnabledDraft((prev) => ({ ...prev, [taskId]: updated.enabled }));
      setTasksSuccess(`Successfully updated configuration for "${updated.name}".`);
    } catch (err: unknown) {
      const e = err as Error;
      setTasksError(e.message || `Failed to update task ${taskId}.`);
    } finally {
      setSavingTaskId(null);
    }
  };

  const handleRunTaskImmediately = async (taskId: string) => {
    setRunningTaskId(taskId);
    setTasksError(null);
    setTasksSuccess(null);
    try {
      const updated = await api.runServerTask(taskId);
      setServerTasks((prev) => prev.map((t) => (t.id === taskId ? updated : t)));
      if (updated.last_status === "error") {
        setTasksError(`Task "${updated.name}" encountered error: ${updated.last_message}`);
      } else {
        setTasksSuccess(`Task "${updated.name}" executed successfully: ${updated.last_message}`);
      }
    } catch (err: unknown) {
      const e = err as Error;
      setTasksError(e.message || `Failed to run task ${taskId}.`);
      await loadServerTasks();
    } finally {
      setRunningTaskId(null);
    }
  };

  const formatDateTime = (isoStr?: string) => {
    if (!isoStr) return "Never";
    return isoStr;
  };

  const formatRelativeTime = (isoStr?: string) => {
    if (!isoStr) return "";
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return "";
    const diffSec = Math.floor((Date.now() - d.getTime()) / 1000);
    if (diffSec < 5) return "just now";
    if (diffSec < 60) return `${diffSec}s ago`;
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    return `${diffDay}d ago`;
  };

  const getCronHumanSummary = (expr?: string) => {
    if (!expr) return "Daily schedule (default)";
    const trimmed = expr.trim().replace(/\s+/g, " ");
    const match = trimmed.match(/^(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*$/);
    if (match) {
      const min = parseInt(match[1], 10);
      const hour = parseInt(match[2], 10);
      if (min >= 0 && min < 60 && hour >= 0 && hour < 24) {
        const timeStr = `${hour.toString().padStart(2, "0")}:${min.toString().padStart(2, "0")}`;
        return `Once every day at ${timeStr}`;
      }
    }
    if (trimmed === "0 */12 * * *") return "Every 12 hours";
    if (trimmed === "0 */6 * * *") return "Every 6 hours";
    if (trimmed === "0 */2 * * *") return "Every 2 hours";
    if (trimmed === "0 * * * *") return "Every hour at minute 0";
    if (trimmed === "*/30 * * * *") return "Every 30 minutes";
    if (trimmed === "*/15 * * * *") return "Every 15 minutes";
    if (trimmed === "*/5 * * * *") return "Every 5 minutes";
    return "Custom schedule";
  };

  const getTaskCronPresets = (expr?: string) => {
    const trimmed = (expr || "").trim().replace(/\s+/g, " ");
    const match = trimmed.match(/^(\d{1,2})\s+(\d{1,2})\s+\*\s+\*\s+\*$/);
    let dailyLabel = "Daily (00:00)";
    let dailyValue = "0 0 * * *";

    if (match) {
      const min = parseInt(match[1], 10);
      const hour = parseInt(match[2], 10);
      if (min >= 0 && min < 60 && hour >= 0 && hour < 24) {
        const timeStr = `${hour.toString().padStart(2, "0")}:${min.toString().padStart(2, "0")}`;
        dailyLabel = `Daily (${timeStr})`;
        dailyValue = `${min} ${hour} * * *`;
      }
    }

    return [
      { label: dailyLabel, value: dailyValue },
      { label: "Every 12h", value: "0 */12 * * *" },
      { label: "Every 6h", value: "0 */6 * * *" },
      { label: "Every 1h", value: "0 * * * *" },
    ];
  };

  const loadPolicies = async () => {
    setPoliciesLoading(true);
    setPoliciesError(null);
    try {
      const list = await api.getNetworkPolicies();
      setPolicies(list);
    } catch (err: unknown) {
      const e = err as Error;
      setPoliciesError(e.message || "Failed to load network policies.");
    } finally {
      setPoliciesLoading(false);
    }
  };

  const loadNetworkSettings = async () => {
    setNetworkLoading(true);
    setNetworkError(null);
    try {
      const settings = await api.getNetworkSettings();
      setPublicAsn(settings.public_asn || "");
      setDisallowedDn42Networks((settings.disallowed_dn42_networks || settings.disallowed_dn42_cidrs || []).join("\n"));
      setNetworkPrefixes(settings.prefixes || []);
      setFaAddress(settings.flapalerted_address || "");
      setFaPort(
        settings.flapalerted_port !== undefined && settings.flapalerted_port !== null
          ? settings.flapalerted_port
          : 1790,
      );
      setFaAsn(
        settings.flapalerted_asn !== undefined && settings.flapalerted_asn !== null
          ? settings.flapalerted_asn
          : 4224420000,
      );
      setFaUrl(settings.flapalerted_url || "");
    } catch (err: unknown) {
      const e = err as Error;
      setNetworkError(e.message || "Failed to load network settings.");
    } finally {
      setNetworkLoading(false);
    }
  };

  const handleSaveNetworkSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setNetworkSaving(true);
    setNetworkError(null);
    setNetworkSuccess(null);

    const payload: NetworkSettings = {
      public_asn: publicAsn === "" ? 0 : Number(publicAsn),
      prefixes: networkPrefixes,
      disallowed_dn42_networks: parsePrefixList(disallowedDn42Networks),
      disallowed_dn42_cidrs: parsePrefixList(disallowedDn42Networks),
      flapalerted_address: faAddress.trim() || undefined,
      flapalerted_port: faPort === "" ? 1790 : Number(faPort),
      flapalerted_asn: faAsn === "" ? 4224420000 : Number(faAsn),
      flapalerted_url: faUrl.trim() || undefined,
    };

    try {
      await api.updateNetworkSettings(payload);
      setNetworkSuccess("BGP confederation and network settings updated successfully.");
      if (refreshNetworkSettings) {
        refreshNetworkSettings();
      }
    } catch (err: unknown) {
      const e = err as Error;
      setNetworkError(e.message || "Failed to update network settings.");
    } finally {
      setNetworkSaving(false);
    }
  };

  const handleOpenCreatePolicy = () => {
    setPolicyDialogMode("create");
    setPolicyId("");
    setPolicyName("");
    setPolicyDesc("");
    setPolicyCost(100);
    setPolicyAllowedDst("");
    setPolicyAllowedSrc("");
    setPolicyDisallowedDst("");
    setPolicyDisallowedSrc("");
    setPolicyLocalNetworks("");
    setPolicyRejectInternet(true);
    setPolicyFilterForward(true);
    setPolicyFilterInput(false);
    setPolicyInputAllowIcmp(true);
    setPolicyInputAllowIcmp6(true);
    setPolicyInputTcpPorts("");
    setPolicyInputUdpPorts("");
    setPolicySnatEnabled(false);
    setPolicySnatCondition("not_dst");
    setPolicySnatTarget("masquerade");
    setPolicyForwardSnat(false);
    setPolicyForwardSnatTarget("masquerade");
    setPolicyRoa4("");
    setPolicyRoa6("");
    setPolicyRoaStrict(false);
    setPolicyDscpIngress("");
    setPolicyDscpEgress("");
    setPolicyFwmark("");
    setPolicyPreference("");
    setPolicyMark("");
    setPolicyRoutingPolicy("full");
    setPolicyBlockIngressNew("");
    setPolicyIngressNewWhitelist("");
    setPolicyBirdImportFilter("");
    setPolicyBirdExportFilter("");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleOpenViewPolicy = (p: NetworkPolicy) => {
    setPolicyToView(p);
    setViewPolicyModalOpen(true);
  };

  const handleOpenEditPolicy = (p: NetworkPolicy) => {
    setPolicyDialogMode("edit");
    setPolicyId(p.id);
    setPolicyName(p.name);
    setPolicyDesc(p.description || "");
    setPolicyCost(p.cost ?? 100);
    setPolicyAllowedDst((p.allowed_dst_cidrs || []).join("\n"));
    setPolicyAllowedSrc((p.allowed_src_cidrs || []).join("\n"));
    setPolicyDisallowedDst((p.disallowed_dst_cidrs || []).join("\n"));
    setPolicyDisallowedSrc((p.disallowed_src_cidrs || []).join("\n"));
    setPolicyLocalNetworks((p.local_networks || []).join("\n"));
    setPolicyRejectInternet(Boolean(p.reject_internet));
    setPolicyFilterForward(Boolean(p.filter_forward));
    setPolicyFilterInput(Boolean(p.filter_input));
    setPolicyInputAllowIcmp(p.input_allow_icmp !== false);
    setPolicyInputAllowIcmp6(p.input_allow_icmp6 !== false);
    setPolicyInputTcpPorts((p.input_tcp_ports || []).join(", "));
    setPolicyInputUdpPorts((p.input_udp_ports || []).join(", "));
    setPolicySnatEnabled(Boolean(p.snat?.enabled));
    setPolicySnatCondition(p.snat?.condition || "not_dst");
    setPolicySnatTarget(p.snat?.target || "masquerade");
    setPolicyForwardSnat(Boolean(p.forward_snat));
    setPolicyForwardSnatTarget(p.forward_snat_target || "masquerade");
    setPolicyRoa4(p.roa4 || "");
    setPolicyRoa6(p.roa6 || "");
    setPolicyRoaStrict(Boolean(p.roa_strict));
    setPolicyDscpIngress(p.dscp_ingress ?? "");
    setPolicyDscpEgress(p.dscp_egress ?? "");
    setPolicyFwmark(p.fwmark || "");
    setPolicyPreference(p.preference !== undefined ? p.preference : "");
    setPolicyMark(p.mark || "");
    setPolicyRoutingPolicy(p.routing_policy || "full");
    setPolicyBlockIngressNew(p.block_ingress_new || "");
    setPolicyIngressNewWhitelist((p.ingress_new_whitelist || []).join("\n"));
    setPolicyBirdImportFilter(p.bird_import_filter || "");
    setPolicyBirdExportFilter(p.bird_export_filter || p.bird_export_fillter || "");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleDeletePolicy = async (p: NetworkPolicy) => {
    if (
      !window.confirm(
        `Are you sure you want to delete custom network policy "${p.name}" (${p.id})? Links assigned to this policy will revert to the default policy.`,
      )
    ) {
      return;
    }
    setPoliciesError(null);
    try {
      await api.deleteNetworkPolicy(p.id);
      setPoliciesSuccess(`Policy "${p.name}" deleted successfully.`);
      await loadPolicies();
    } catch (err: unknown) {
      const e = err as Error;
      setPoliciesError(e.message || "Failed to delete network policy.");
    }
  };

  const handleSavePolicy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (policyDialogMode === "view") {
      setPolicyDialogOpen(false);
      return;
    }
    const cleanId = policyId.trim().toLowerCase();
    if (!cleanId) {
      setPolicyDialogError("Policy ID is required.");
      return;
    }
    if (!/^[a-z0-9_-]+$/.test(cleanId)) {
      setPolicyDialogError("Policy ID must contain only lowercase letters, numbers, hyphens, and underscores.");
      return;
    }
    if (!policyName.trim()) {
      setPolicyDialogError("Policy Name is required.");
      return;
    }

    setPolicyDialogSaving(true);
    setPolicyDialogError(null);
    try {
      const policyPayload: Partial<NetworkPolicy> = {
        id: cleanId,
        name: policyName.trim(),
        description: policyDesc.trim() || undefined,
        cost: Number(policyCost) > 0 ? Number(policyCost) : 100,
        local_networks: parsePrefixList(policyLocalNetworks),
        allowed_dst_cidrs: parsePrefixList(policyAllowedDst),
        allowed_src_cidrs: parsePrefixList(policyAllowedSrc),
        disallowed_dst_cidrs: parsePrefixList(policyDisallowedDst),
        disallowed_src_cidrs: parsePrefixList(policyDisallowedSrc),
        reject_internet: policyRejectInternet,
        filter_forward: policyFilterForward,
        filter_input: policyFilterInput,
        input_allow_icmp: policyInputAllowIcmp,
        input_allow_icmp6: policyInputAllowIcmp6,
        input_tcp_ports: parsePortList(policyInputTcpPorts),
        input_udp_ports: parsePortList(policyInputUdpPorts),
        snat: policySnatEnabled
          ? {
              enabled: true,
              condition: policySnatCondition,
              target: policySnatTarget.trim() || undefined,
            }
          : undefined,
        forward_snat: policyForwardSnat,
        forward_snat_target: policyForwardSnat ? policyForwardSnatTarget.trim() || undefined : undefined,
        roa4: policyRoa4.trim() || undefined,
        roa6: policyRoa6.trim() || undefined,
        roa_strict: policyRoaStrict,
        dscp_ingress: policyDscpIngress !== "" ? Number(policyDscpIngress) : undefined,
        dscp_egress: policyDscpEgress !== "" ? Number(policyDscpEgress) : undefined,
        fwmark: policyFwmark.trim() || undefined,
        preference: policyPreference !== "" ? Number(policyPreference) : undefined,
        mark: policyMark.trim() || undefined,
        routing_policy: policyRoutingPolicy || "full",
        block_ingress_new: policyBlockIngressNew || undefined,
        ingress_new_whitelist:
          policyBlockIngressNew && policyBlockIngressNew !== "_"
            ? parseWhitelistList(policyIngressNewWhitelist)
            : undefined,
        bird_import_filter: policyBirdImportFilter.trim() || undefined,
        bird_export_filter: policyBirdExportFilter.trim() || undefined,
        bird_export_fillter: policyBirdExportFilter.trim() || undefined,
      };

      if (policyDialogMode === "create") {
        await api.createNetworkPolicy(policyPayload);
        setPoliciesSuccess(`Policy "${policyName.trim()}" created successfully.`);
      } else {
        await api.updateNetworkPolicy(cleanId, policyPayload);
        setPoliciesSuccess(`Policy "${policyName.trim()}" updated successfully.`);
      }
      setPolicyDialogOpen(false);
      await loadPolicies();
    } catch (err: unknown) {
      const e = err as Error;
      setPolicyDialogError(e.message || "Failed to save network policy.");
    } finally {
      setPolicyDialogSaving(false);
    }
  };

  const resetForm = () => {
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setPasswordError(null);
    setPasswordSuccess(null);
    setLogoutAllConfirming(false);
    setLogoutAllError(null);
    setNetworkError(null);
    setNetworkSuccess(null);
    setPoliciesError(null);
    setPoliciesSuccess(null);
  };

  const handleOpenClonePolicy = (p: NetworkPolicy) => {
    setPolicyDialogMode("create");
    setPolicyId(`${p.id}-copy`);
    setPolicyName(`${p.name} (Copy)`);
    setPolicyDesc(p.description || "");
    setPolicyCost(p.cost ?? 100);
    setPolicyAllowedDst((p.allowed_dst_cidrs || []).join("\n"));
    setPolicyAllowedSrc((p.allowed_src_cidrs || []).join("\n"));
    setPolicyDisallowedDst((p.disallowed_dst_cidrs || []).join("\n"));
    setPolicyDisallowedSrc((p.disallowed_src_cidrs || []).join("\n"));
    setPolicyLocalNetworks((p.local_networks || []).join("\n"));
    setPolicyRejectInternet(Boolean(p.reject_internet));
    setPolicyFilterForward(Boolean(p.filter_forward));
    setPolicyFilterInput(Boolean(p.filter_input));
    setPolicyInputAllowIcmp(p.input_allow_icmp !== false);
    setPolicyInputAllowIcmp6(p.input_allow_icmp6 !== false);
    setPolicyInputTcpPorts((p.input_tcp_ports || []).join(", "));
    setPolicyInputUdpPorts((p.input_udp_ports || []).join(", "));
    setPolicySnatEnabled(Boolean(p.snat?.enabled));
    setPolicySnatCondition(p.snat?.condition || "not_dst");
    setPolicySnatTarget(p.snat?.target || "masquerade");
    setPolicyForwardSnat(Boolean(p.forward_snat));
    setPolicyForwardSnatTarget(p.forward_snat_target || "masquerade");
    setPolicyRoa4(p.roa4 || "");
    setPolicyRoa6(p.roa6 || "");
    setPolicyRoaStrict(Boolean(p.roa_strict));
    setPolicyDscpIngress(p.dscp_ingress ?? "");
    setPolicyDscpEgress(p.dscp_egress ?? "");
    setPolicyFwmark(p.fwmark || "");
    setPolicyPreference(p.preference !== undefined ? p.preference : "");
    setPolicyMark(p.mark || "");
    setPolicyRoutingPolicy(p.routing_policy || "full");
    setPolicyBlockIngressNew(p.block_ingress_new || "");
    setPolicyIngressNewWhitelist((p.ingress_new_whitelist || []).join("\n"));
    setPolicyBirdImportFilter(p.bird_import_filter || "");
    setPolicyBirdExportFilter(p.bird_export_filter || p.bird_export_fillter || "");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError(null);
    setPasswordSuccess(null);

    if (!currentPassword) {
      setPasswordError("Please enter your current password.");
      return;
    }

    if (newPassword.length < 6) {
      setPasswordError("New password must be at least 6 characters long.");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordError("New passwords do not match.");
      return;
    }

    if (currentPassword === newPassword) {
      setPasswordError("New password must be different from current password.");
      return;
    }

    setPasswordLoading(true);
    try {
      const res = await api.changePassword(currentPassword, newPassword);
      setPasswordSuccess(res.message || "Password changed successfully.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err: unknown) {
      const e = err as Error;
      setPasswordError(e.message || "Failed to change password.");
    } finally {
      setPasswordLoading(false);
    }
  };

  const handleLogoutAll = async () => {
    setLogoutAllLoading(true);
    setLogoutAllError(null);

    try {
      await api.logoutAll();
      resetForm();
      if (onClose) onClose();
      onLogoutAll();
    } catch (err: unknown) {
      const e = err as Error;
      setLogoutAllError(e.message || "Failed to log out all sessions.");
      setLogoutAllLoading(false);
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
      {/* Main Settings Card */}
      <Paper
        elevation={0}
        sx={{
          borderRadius: 3,
          border: "1px solid #E2E8F0",
          bgcolor: "#FFFFFF",
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
          flex: 1,
          minHeight: 0,
        }}
      >
        <Box
          sx={{
            borderBottom: "1px solid #E2E8F0",
            px: { xs: 1, sm: 3 },
            pt: 1,
            backgroundColor: "#F8FAFC",
            flexShrink: 0,
          }}
        >
          <Tabs
            value={activeTab}
            onChange={(_, val) => {
              navigate(`/settings/${val}`);
              setPasswordError(null);
              setPasswordSuccess(null);
              setLogoutAllError(null);
            }}
            variant="scrollable"
            scrollButtons="auto"
            allowScrollButtonsMobile
            sx={{
              minHeight: 44,
              "& .MuiTab-root": {
                minHeight: 44,
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.875rem",
                gap: 1,
              },
            }}
          >
            <Tab
              value="network"
              icon={<Globe size={16} />}
              iconPosition="start"
              label="Network"
              title="Peering & BGP"
            />
            <Tab
              value="security"
              icon={<KeyRound size={16} />}
              iconPosition="start"
              label="Security"
              title="Password & Sessions"
            />
            <Tab
              value="policies"
              icon={<Shield size={16} />}
              iconPosition="start"
              label="Policies"
              title="Network Policies"
            />
            <Tab
              value="templates"
              icon={<FileCode size={16} />}
              iconPosition="start"
              label="Templates"
              title="Config Template"
            />
            <Tab value="dns" icon={<Cloud size={16} />} iconPosition="start" label="DNS" title="DNS" />
            <Tab value="tasks" icon={<CalendarClock size={16} />} iconPosition="start" label="Tasks" title="Tasks" />
            <Tab
              value="database"
              icon={<Database size={16} />}
              iconPosition="start"
              label="Database"
              title="Database"
            />
            <Tab
              value="integrations"
              icon={<Zap size={16} />}
              iconPosition="start"
              label="Integrations"
              title="Integrations & Prometheus"
            />
            <Tab value="about" icon={<Info size={16} />} iconPosition="start" label="About" title="About" />
          </Tabs>
        </Box>

        {/* Tab: Password & Sessions */}
        {activeTab === "security" && (
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}>
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 3.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Section 1: Change Master Password */}
              <Box
                component="form"
                onSubmit={handleChangePassword}
                sx={{ display: "flex", flexDirection: "column", gap: 2 }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <KeyRound size={18} color="#4F46E5" />
                  <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Change Master Password
                  </Typography>
                </Box>
                <Typography variant="body2" sx={{ color: "#64748B" }}>
                  Update your easy42 admin password. Changing the password will re-encrypt your master data encryption
                  key (DEK) and invalidate other active sessions.
                </Typography>

                {passwordSuccess && (
                  <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                    {passwordSuccess}
                  </Alert>
                )}

                {passwordError && (
                  <Alert severity="error" sx={{ borderRadius: 2 }}>
                    {passwordError}
                  </Alert>
                )}

                <TextField
                  fullWidth
                  size="small"
                  label="Current Password"
                  type={showCurrentPassword ? "text" : "password"}
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  required
                  disabled={passwordLoading}
                  slotProps={{
                    input: {
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            size="small"
                            edge="end"
                            onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                            aria-label="toggle current password visibility"
                          >
                            {showCurrentPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    },
                  }}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="New Password"
                  type={showNewPassword ? "text" : "password"}
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  required
                  disabled={passwordLoading}
                  helperText="Minimum 6 characters"
                  slotProps={{
                    input: {
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            size="small"
                            edge="end"
                            onClick={() => setShowNewPassword(!showNewPassword)}
                            aria-label="toggle new password visibility"
                          >
                            {showNewPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    },
                  }}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="Confirm New Password"
                  type={showConfirmPassword ? "text" : "password"}
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  required
                  disabled={passwordLoading}
                  error={Boolean(confirmPassword && newPassword !== confirmPassword)}
                  helperText={confirmPassword && newPassword !== confirmPassword ? "Passwords do not match" : ""}
                  slotProps={{
                    input: {
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            size="small"
                            edge="end"
                            onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                            aria-label="toggle confirm password visibility"
                          >
                            {showConfirmPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    },
                  }}
                />
                <Box sx={{ display: "flex", justifyContent: "flex-end", pt: 0.5 }}>
                  <Button
                    type="submit"
                    variant="contained"
                    disabled={passwordLoading || !currentPassword || !newPassword || !confirmPassword}
                    startIcon={
                      passwordLoading ? <CircularProgress size={16} color="inherit" /> : <KeyRound size={16} />
                    }
                    sx={{
                      background: "linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)",
                      fontWeight: 700,
                      color: "#ffffff",
                    }}
                  >
                    {passwordLoading ? "Updating Password..." : "Change Password"}
                  </Button>
                </Box>
              </Box>

              <Divider />

              {/* Section 2: Active Sessions & Invalidation */}
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <ShieldAlert size={18} color="#DC2626" />
                  <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                    Active Sessions & Invalidation
                  </Typography>
                </Box>

                {logoutAllError && (
                  <Alert severity="error" sx={{ borderRadius: 2 }}>
                    {logoutAllError}
                  </Alert>
                )}

                <Box
                  sx={{
                    p: 2.5,
                    borderRadius: 2,
                    border: "1px solid #FEE2E2",
                    backgroundColor: "#FEF2F2",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.5,
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, color: "#B91C1C" }}>
                    <ShieldAlert size={22} />
                    <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>
                      Reset Session Secret & Log Out All Sessions
                    </Typography>
                  </Box>

                  <Typography variant="body2" sx={{ color: "#7F1D1D", lineHeight: 1.6 }}>
                    Clicking <strong>Logout all</strong> will regenerate the <code>session_secret</code> in{" "}
                    <code>config.json</code> and immediately lock the in-memory vault.
                  </Typography>

                  <Typography variant="body2" sx={{ color: "#991B1B", lineHeight: 1.6 }}>
                    This invalidates every active session across all devices and browsers, including your current one.
                    You and any other users will be required to log in again.
                  </Typography>
                </Box>

                {!logoutAllConfirming ? (
                  <Box sx={{ display: "flex", justifyContent: "flex-start", pt: 0.5 }}>
                    <Button
                      variant="outlined"
                      color="error"
                      startIcon={<LogOut size={16} />}
                      onClick={() => setLogoutAllConfirming(true)}
                      sx={{
                        borderColor: "#F87171",
                        color: "#DC2626",
                        fontWeight: 700,
                        "&:hover": {
                          borderColor: "#DC2626",
                          backgroundColor: "rgba(220, 38, 38, 0.06)",
                        },
                      }}
                    >
                      Logout all
                    </Button>
                  </Box>
                ) : (
                  <Box
                    sx={{
                      p: 2,
                      borderRadius: 2,
                      border: "1px solid #CBD5E1",
                      backgroundColor: "#F8FAFC",
                      display: "flex",
                      flexDirection: "column",
                      gap: 1.5,
                    }}
                  >
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                      Are you sure you want to log out all sessions?
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B" }}>
                      All connected browsers will immediately lose authentication and return to the login screen.
                    </Typography>
                    <Box sx={{ display: "flex", gap: 1.5, pt: 0.5 }}>
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => setLogoutAllConfirming(false)}
                        disabled={logoutAllLoading}
                        sx={{ color: "#64748B", borderColor: "#CBD5E1" }}
                      >
                        Cancel
                      </Button>
                      <Button
                        size="small"
                        variant="contained"
                        color="error"
                        disabled={logoutAllLoading}
                        onClick={handleLogoutAll}
                        startIcon={
                          logoutAllLoading ? <CircularProgress size={14} color="inherit" /> : <LogOut size={14} />
                        }
                        sx={{ fontWeight: 700 }}
                      >
                        {logoutAllLoading ? "Logging out all..." : "Confirm: Log Out All"}
                      </Button>
                    </Box>
                  </Box>
                )}
              </Box>
            </DialogContent>
          </Box>
        )}

        {/* Tab 3: Peering & BGP Network Settings */}
        {activeTab === "network" && (
          <form
            onSubmit={handleSaveNetworkSettings}
            style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}
          >
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              <Typography variant="body2" sx={{ color: "#64748B" }}>
                Configure global BGP confederation parameters for external peering (such as DN42 or private networks).
                BGP confederation replaces your internal mesh ASNs with your public ASN in external BGP sessions. Subnet
                filtering and firewall policies are configured per-link under Network Policies.
              </Typography>

              {networkLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : (
                <>
                  {networkSuccess && (
                    <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                      {networkSuccess}
                    </Alert>
                  )}

                  {networkError && (
                    <Alert severity="error" sx={{ borderRadius: 2 }}>
                      {networkError}
                    </Alert>
                  )}

                  <TextFieldWithCopy
                    fullWidth
                    size="small"
                    label="Public / DN42 ASN"
                    type="number"
                    placeholder="e.g. 4242420000"
                    value={publicAsn}
                    onChange={(e) => setPublicAsn(e.target.value === "" ? "" : Number(e.target.value))}
                    helperText="Your network's public ASN (e.g. DN42 ASN). When set, BIRD confederation exposes this ASN to external peers."
                    disabled={networkSaving}
                  />

                  <TextFieldWithCopy
                    fullWidth
                    size="small"
                    multiline
                    rows={2}
                    label="Disallowed CIDRs for dn42"
                    placeholder="e.g. fd42:1234:5678::/48, 172.20.99.0/24"
                    value={disallowedDn42Networks}
                    onChange={(e) => setDisallowedDn42Networks(e.target.value)}
                    helperText="Excluded subnets for built-in dn42 policy (used as DisallowedDstCIDRs & DisallowedSrcCIDRs). Rejects BGP export, drops inbound traffic, and triggers SNAT for internal egress traffic."
                    disabled={networkSaving}
                  />

                  <Divider sx={{ my: 0.5 }} />

                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Activity size={18} color="#4F46E5" />
                      <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                        FlapAlerted Integration
                      </Typography>
                      <Chip
                        label="BGP Flap Detection"
                        size="small"
                        sx={{
                          height: 20,
                          fontSize: "0.68rem",
                          fontWeight: 600,
                          bgcolor: "#EEF2FF",
                          color: "#4F46E5",
                        }}
                      />
                    </Box>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.82rem" }}>
                      Configure{" "}
                      <a
                        href="https://github.com/Kioubit/FlapAlerted"
                        target="_blank"
                        rel="noopener noreferrer"
                        style={{ color: "#4F46E5", textDecoration: "none", fontWeight: 600 }}
                      >
                        FlapAlerted{" "}
                      </a>
                      BGP route flap detection. The FlapAlerted service is deployed separately on any host in the
                      network (e.g.{" "}
                      <code
                        style={{ background: "#F1F5F9", padding: "2px 6px", borderRadius: 4, fontFamily: "monospace" }}
                      >
                        FlapAlerted -asn {faAsn || 4224420000}
                      </code>
                      ). Nodes with <strong>enable_fa</strong> enabled will peer with this instance.
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.82rem" }}>
                      Deployed FA instance:{" "}
                      {(() => {
                        const trimmedUrl = faUrl.trim();
                        const effectiveUrl = trimmedUrl
                          ? trimmedUrl.startsWith("http://") || trimmedUrl.startsWith("https://")
                            ? trimmedUrl
                            : `http://${trimmedUrl}`
                          : faAddress.trim()
                            ? `http://${faAddress.trim().includes(":") && !faAddress.trim().startsWith("[") ? `[${faAddress.trim()}]` : faAddress.trim()}:8699`
                            : "";
                        return effectiveUrl ? (
                          <>
                            <a
                              href={effectiveUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              style={{ color: "#4F46E5", textDecoration: "none", fontWeight: 600 }}
                            >
                              {effectiveUrl}
                            </a>
                            &nbsp;(Click to Open Dashboard)
                          </>
                        ) : (
                          "-"
                        );
                      })()}
                    </Typography>

                    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "2fr 1fr 1fr" }, gap: 2 }}>
                      <TextFieldWithCopy
                        fullWidth
                        size="small"
                        label="FlapAlerted Address"
                        placeholder="e.g. fd42:a159:f9f0:8000::c8"
                        value={faAddress}
                        onChange={(e) => setFaAddress(e.target.value)}
                        helperText="Private IPv4 or IPv6 routable inside easy42"
                        disabled={networkSaving}
                      />
                      <TextFieldWithCopy
                        fullWidth
                        size="small"
                        label="FlapAlerted Port"
                        type="number"
                        placeholder="1790"
                        value={faPort}
                        onChange={(e) => setFaPort(e.target.value === "" ? "" : Number(e.target.value))}
                        helperText="Default: 1790"
                        disabled={networkSaving}
                      />
                      <TextFieldWithCopy
                        fullWidth
                        size="small"
                        label="FlapAlerted ASN"
                        type="number"
                        placeholder="4224420000"
                        value={faAsn}
                        onChange={(e) => setFaAsn(e.target.value === "" ? "" : Number(e.target.value))}
                        helperText="Default: 4224420000"
                        disabled={networkSaving}
                      />
                    </Box>

                    <TextFieldWithCopy
                      fullWidth
                      size="small"
                      label="FlapAlerted URL"
                      placeholder="e.g. http://[fd42:a159:f9f0:8000::c8]:8699 or https://fa.example.com"
                      value={faUrl}
                      onChange={(e) => setFaUrl(e.target.value)}
                      helperText="Custom FlapAlerted web & metrics URL. If not set, derived from FlapAlerted address and port 8699."
                      disabled={networkSaving}
                    />
                  </Box>
                  <Box sx={{ display: "flex", justifyContent: "flex-end", pt: 1 }}>
                    <Button
                      type="submit"
                      variant="contained"
                      disabled={networkLoading || networkSaving}
                      startIcon={networkSaving && <CircularProgress size={16} color="inherit" />}
                      sx={{ fontWeight: 600 }}
                    >
                      {networkSaving ? "Saving..." : "Save Settings"}
                    </Button>
                  </Box>
                </>
              )}
            </DialogContent>
          </form>
        )}

        {/* Tab 4: Network Policies */}
        {activeTab === "policies" && (
          <Box
            sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, height: "100%", overflow: "hidden" }}
          >
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2 }}>
                <Box>
                  <Typography variant="body2" sx={{ color: "#475569", lineHeight: 1.5 }}>
                    Define link-level firewall and routing policies. Network policies restrict ingress allowed
                    destination & source IPs (dropping unauthorized packets in nftables while preserving established
                    return flows), filter BGP route imports, and configure outbound SNAT / masquerade.
                  </Typography>
                </Box>
                <Button
                  variant="contained"
                  size="small"
                  startIcon={<Plus size={16} />}
                  onClick={handleOpenCreatePolicy}
                  sx={{
                    whiteSpace: "nowrap",
                    fontWeight: 600,
                    bgcolor: "#4F46E5",
                    "&:hover": { bgcolor: "#4338CA" },
                  }}
                >
                  Create Policy
                </Button>
              </Box>

              {policiesSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {policiesSuccess}
                </Alert>
              )}

              {policiesError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {policiesError}
                </Alert>
              )}

              {policiesLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1 }}>
                  {policies.map((p) => {
                    const isBuiltin = Boolean(p.is_internal);
                    return (
                      <Box
                        key={p.id}
                        sx={{
                          p: 2,
                          borderRadius: 2,
                          border: "1px solid",
                          borderColor: isBuiltin ? "#E0E7FF" : "#E2E8F0",
                          bgcolor: isBuiltin ? "#F8FAFC" : "#FFFFFF",
                          display: "flex",
                          flexDirection: "column",
                          gap: 1.2,
                          transition: "all 0.15s ease",
                          "&:hover": {
                            borderColor: isBuiltin ? "#C7D2FE" : "#CBD5E1",
                            boxShadow: "0 2px 4px rgba(0,0,0,0.03)",
                          },
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                              {p.name}
                            </Typography>
                            <Typography
                              variant="caption"
                              className="mono-font"
                              sx={{ color: "#64748B", bgcolor: "#F1F5F9", px: 0.8, py: 0.2, borderRadius: 1 }}
                            >
                              {p.id}
                            </Typography>
                            <Chip
                              label={isBuiltin ? "Built-in" : "Custom"}
                              size="small"
                              sx={{
                                height: 20,
                                fontSize: "0.68rem",
                                fontWeight: 700,
                                bgcolor: isBuiltin ? "rgba(79, 70, 229, 0.1)" : "rgba(16, 185, 129, 0.12)",
                                color: isBuiltin ? "#4F46E5" : "#059669",
                              }}
                            />
                            {(p.roa4 || p.roa6) && (
                              <Chip
                                label={p.roa_strict ? "ROA (Strict)" : "ROA"}
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: "rgba(14, 165, 233, 0.12)",
                                  color: "#0284C7",
                                }}
                              />
                            )}
                            {p.routing_policy && p.routing_policy !== "full" && (
                              <Chip
                                label={
                                  p.routing_policy === "stub"
                                    ? "Stub"
                                    : p.routing_policy === "receive_only"
                                      ? "Receive Only"
                                      : p.routing_policy === "advertise_only"
                                        ? "Advertise Only"
                                        : p.routing_policy
                                }
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: "rgba(245, 158, 11, 0.12)",
                                  color: "#D97706",
                                }}
                              />
                            )}
                            {p.block_ingress_new && (
                              <Chip
                                label={
                                  p.block_ingress_new === "all"
                                    ? "Block Ingress New"
                                    : p.block_ingress_new === "forward"
                                      ? "Block Ingress Forward"
                                      : p.block_ingress_new
                                }
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: "rgba(239, 68, 68, 0.12)",
                                  color: "#DC2626",
                                }}
                              />
                            )}
                            {p.block_ingress_new && p.ingress_new_whitelist && p.ingress_new_whitelist.length > 0 && (
                              <Chip
                                label={`WL: ${p.ingress_new_whitelist.length}`}
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: "rgba(16, 185, 129, 0.12)",
                                  color: "#059669",
                                }}
                              />
                            )}
                            {(p.bird_import_filter || p.bird_export_filter || p.bird_export_fillter) && (
                              <Chip
                                label="BIRD Filter"
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: "rgba(99, 102, 241, 0.12)",
                                  color: "#4F46E5",
                                }}
                              />
                            )}
                          </Box>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                            {(p.roa4 || p.roa6) && (
                              <Button
                                size="small"
                                variant="outlined"
                                onClick={async () => {
                                  try {
                                    await api.refreshPolicyROA(p.id);
                                    setPoliciesSuccess(`ROA cache for "${p.name}" refreshed.`);
                                  } catch (err: unknown) {
                                    const e = err as Error;
                                    setPoliciesError(e.message || "Failed to refresh ROA.");
                                  }
                                }}
                                sx={{
                                  py: 0.3,
                                  px: 1,
                                  minWidth: 0,
                                  fontSize: "0.75rem",
                                  color: "#0284C7",
                                  borderColor: "#BAE6FD",
                                }}
                              >
                                Sync ROA
                              </Button>
                            )}
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => handleOpenViewPolicy(p)}
                              sx={{
                                py: 0.3,
                                px: 1,
                                minWidth: 0,
                                fontSize: "0.75rem",
                                color: "#475569",
                                borderColor: "#CBD5E1",
                              }}
                            >
                              View
                            </Button>
                            <Button
                              size="small"
                              variant="outlined"
                              startIcon={<Copy size={12} />}
                              onClick={() => handleOpenClonePolicy(p)}
                              sx={{
                                py: 0.3,
                                px: 1,
                                minWidth: 0,
                                fontSize: "0.75rem",
                                color: "#4F46E5",
                                borderColor: "#C7D2FE",
                              }}
                            >
                              Clone
                            </Button>
                            {!isBuiltin && (
                              <>
                                <IconButton
                                  size="small"
                                  onClick={() => handleOpenEditPolicy(p)}
                                  sx={{ color: "#4F46E5", p: 0.5 }}
                                  title="Edit Policy"
                                >
                                  <Edit2 size={15} />
                                </IconButton>
                                <IconButton
                                  size="small"
                                  onClick={() => handleDeletePolicy(p)}
                                  sx={{ color: "#EF4444", p: 0.5 }}
                                  title="Delete Policy"
                                >
                                  <Trash2 size={15} />
                                </IconButton>
                              </>
                            )}
                          </Box>
                        </Box>

                        {p.description && (
                          <Typography variant="caption" sx={{ color: "#64748B" }}>
                            {p.description}
                          </Typography>
                        )}

                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, pt: 0.5 }}>
                          <Chip
                            size="small"
                            label={`Cost: ${p.cost ?? 100}`}
                            variant="outlined"
                            sx={{
                              fontSize: "0.7rem",
                              height: 22,
                              borderColor: "#C7D2FE",
                              bgcolor: "#EEF2FF",
                              color: "#4338CA",
                            }}
                          />
                          {p.fwmark && (
                            <Chip
                              size="small"
                              label={`FwMark: ${p.fwmark}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#DDD6FE",
                                bgcolor: "#FAF5FF",
                                color: "#6D28D9",
                              }}
                            />
                          )}
                          {p.preference !== undefined && (
                            <Chip
                              size="small"
                              label={`Preference: ${p.preference}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#BBF7D0",
                                bgcolor: "#F0FDF4",
                                color: "#15803D",
                              }}
                            />
                          )}
                          {p.mark && (
                            <Chip
                              size="small"
                              label={`Mark: ${p.mark}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#DDD6FE",
                                bgcolor: "#FAF5FF",
                                color: "#6D28D9",
                              }}
                            />
                          )}
                          <Chip
                            size="small"
                            label={`Allowed Dst: ${p.allowed_dst_cidrs && p.allowed_dst_cidrs.length > 0 ? `${p.allowed_dst_cidrs.length} prefix(es)` : "All"}`}
                            variant="outlined"
                            sx={{ fontSize: "0.7rem", height: 22 }}
                          />
                          <Chip
                            size="small"
                            label={`Allowed Src: ${p.allowed_src_cidrs && p.allowed_src_cidrs.length > 0 ? `${p.allowed_src_cidrs.length} prefix(es)` : "All"}`}
                            variant="outlined"
                            sx={{ fontSize: "0.7rem", height: 22 }}
                          />
                          {p.disallowed_dst_cidrs && p.disallowed_dst_cidrs.length > 0 && (
                            <Chip
                              size="small"
                              label={`Disallowed Dst: ${p.disallowed_dst_cidrs.length} prefix(es)`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#FECACA",
                                bgcolor: "#FEF2F2",
                                color: "#DC2626",
                              }}
                            />
                          )}
                          {p.disallowed_src_cidrs && p.disallowed_src_cidrs.length > 0 && (
                            <Chip
                              size="small"
                              label={`Disallowed Src: ${p.disallowed_src_cidrs.length} prefix(es)`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#FECACA",
                                bgcolor: "#FEF2F2",
                                color: "#DC2626",
                              }}
                            />
                          )}
                          <Chip
                            size="small"
                            label={`Forward Filter: ${p.filter_forward ? "Enabled" : "Disabled"}`}
                            variant="outlined"
                            sx={{
                              fontSize: "0.7rem",
                              height: 22,
                              borderColor: p.filter_forward ? "#C7D2FE" : undefined,
                              bgcolor: p.filter_forward ? "#EEF2FF" : undefined,
                              color: p.filter_forward ? "#4338CA" : undefined,
                            }}
                          />
                          <Chip
                            size="small"
                            label={`Input Filter: ${p.filter_input ? "Enabled" : "Disabled"}`}
                            variant="outlined"
                            sx={{
                              fontSize: "0.7rem",
                              height: 22,
                              borderColor: p.filter_input ? "#FED7AA" : undefined,
                              bgcolor: p.filter_input ? "#FFF7ED" : undefined,
                              color: p.filter_input ? "#C2410C" : undefined,
                            }}
                          />
                          <Chip
                            size="small"
                            label={`Leak Protect: ${p.reject_internet ? "Internet Rejection" : "None"}`}
                            variant="outlined"
                            sx={{ fontSize: "0.7rem", height: 22 }}
                          />
                          <Chip
                            size="small"
                            label={`SNAT: ${p.snat?.enabled ? (p.snat.condition === "not_dst" ? "SNAT (src != dst)" : "SNAT (all)") : "Disabled"}`}
                            variant="outlined"
                            sx={{
                              fontSize: "0.7rem",
                              height: 22,
                              borderColor: p.snat?.enabled ? "#A5F3FC" : undefined,
                              bgcolor: p.snat?.enabled ? "#F0FDFA" : undefined,
                              color: p.snat?.enabled ? "#0F766E" : undefined,
                            }}
                          />
                          {p.forward_snat && (
                            <Chip
                              size="small"
                              label={`Forward SNAT: ${p.forward_snat_target === "external_ip" ? "External IP" : p.forward_snat_target === "main_ip" ? "Main IP" : "Masquerade"}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#BAE6FD",
                                bgcolor: "#F0F9FF",
                                color: "#0369A1",
                              }}
                            />
                          )}
                          {p.local_networks && p.local_networks.length > 0 && (
                            <Chip
                              size="small"
                              label={`Local Nets: ${p.local_networks.length}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#E2E8F0",
                                bgcolor: "#F8FAFC",
                                color: "#475569",
                              }}
                            />
                          )}
                          {p.dscp_ingress !== undefined && p.dscp_ingress !== null && (
                            <Chip
                              size="small"
                              label={`DSCP In: ${p.dscp_ingress}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#DDD6FE",
                                bgcolor: "#F5F3FF",
                                color: "#6D28D9",
                              }}
                            />
                          )}
                          {p.dscp_egress !== undefined && p.dscp_egress !== null && (
                            <Chip
                              size="small"
                              label={`DSCP Out: ${p.dscp_egress}`}
                              variant="outlined"
                              sx={{
                                fontSize: "0.7rem",
                                height: 22,
                                borderColor: "#DDD6FE",
                                bgcolor: "#F5F3FF",
                                color: "#6D28D9",
                              }}
                            />
                          )}
                        </Box>
                      </Box>
                    );
                  })}
                </Box>
              )}
            </DialogContent>
          </Box>
        )}

        {/* Tab: Config Templates */}
        {activeTab === "templates" && (
          <Box
            sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, height: "100%", overflow: "hidden" }}
          >
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              <Box
                sx={{
                  display: "flex",
                  alignItems: "flex-start",
                  justifyContent: "space-between",
                  gap: 2,
                  flexWrap: "wrap",
                }}
              >
                <Box sx={{ flex: 1, minWidth: 240 }}>
                  <Typography variant="body2" sx={{ color: "#475569", lineHeight: 1.5 }}>
                    Customize the Go text templates used to generate WireGuard, BIRD 2, and nftables configuration
                    files. System default templates are listed below and can be viewed (read-only) or cloned to create
                    custom templates for nodes. You can use{" "}
                    <a href="https://github.com/go-sprout/sprout" target="_blank" rel="noopener noreferrer">
                      Sprout functions
                    </a>{" "}
                    in custom template.
                  </Typography>
                </Box>
                <Button
                  variant="contained"
                  size="small"
                  startIcon={<Plus size={16} />}
                  onClick={() => handleOpenCreateTemplate()}
                  sx={{
                    whiteSpace: "nowrap",
                    fontWeight: 600,
                    bgcolor: "#4F46E5",
                    "&:hover": { bgcolor: "#4338CA" },
                  }}
                >
                  Create Template
                </Button>
              </Box>

              {templatesSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {templatesSuccess}
                </Alert>
              )}

              {templatesError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {templatesError}
                </Alert>
              )}

              {/* Filter and Search Bar */}
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 1.5,
                  flexWrap: "wrap",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                  {(["all", "wg", "bird", "nft"] as const).map((filterType) => {
                    const count =
                      filterType === "all" ? templates.length : templates.filter((t) => t.type === filterType).length;
                    const label =
                      filterType === "all"
                        ? `All (${count})`
                        : filterType === "wg"
                          ? `WireGuard (${count})`
                          : filterType === "bird"
                            ? `BIRD (${count})`
                            : `nftables (${count})`;
                    const isSelected = templateTypeFilter === filterType;
                    return (
                      <Chip
                        key={filterType}
                        label={label}
                        size="small"
                        clickable
                        onClick={() => setTemplateTypeFilter(filterType)}
                        sx={{
                          fontWeight: 500,
                          bgcolor: isSelected ? "#4F46E5" : "#F1F5F9",
                          color: isSelected ? "#FFFFFF" : "#475569",
                          "&:hover": {
                            bgcolor: isSelected ? "#4338CA" : "#E2E8F0",
                          },
                        }}
                      />
                    );
                  })}
                </Box>
                <Box sx={{ width: { xs: "100%", sm: 220 } }}>
                  <TextField
                    size="small"
                    placeholder="Search templates..."
                    value={templateSearch}
                    onChange={(e) => setTemplateSearch(e.target.value)}
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <Search size={15} color="#94A3B8" />
                        </InputAdornment>
                      ),
                    }}
                    fullWidth
                  />
                </Box>
              </Box>

              {templatesLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                  {filteredTemplates.length === 0 ? (
                    <Box
                      sx={{
                        p: 4,
                        textAlign: "center",
                        borderRadius: 2,
                        border: "1px dashed #CBD5E1",
                        bgcolor: "#F8FAFC",
                      }}
                    >
                      <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                        No templates match your search or filter.
                      </Typography>
                    </Box>
                  ) : (
                    filteredTemplates.map((t) => {
                      const isBuiltin = Boolean(t.is_builtin);
                      const badge = getTemplateTypeBadge(t.type);
                      const lineCount = (t.content || "").split("\n").length;
                      return (
                        <Box
                          key={t.id}
                          sx={{
                            p: 2,
                            borderRadius: 2,
                            border: "1px solid",
                            borderColor: isBuiltin ? "#E0E7FF" : "#E2E8F0",
                            bgcolor: isBuiltin ? "#F8FAFC" : "#FFFFFF",
                            display: "flex",
                            flexDirection: "column",
                            gap: 1.2,
                            transition: "all 0.15s ease",
                            "&:hover": {
                              borderColor: isBuiltin ? "#C7D2FE" : "#CBD5E1",
                              boxShadow: "0 2px 4px rgba(0,0,0,0.03)",
                            },
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
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                                {t.name}
                              </Typography>
                              <Typography
                                variant="caption"
                                className="mono-font"
                                sx={{ color: "#64748B", bgcolor: "#F1F5F9", px: 0.8, py: 0.2, borderRadius: 1 }}
                              >
                                {t.id}
                              </Typography>
                              <Chip
                                label={badge.label}
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: badge.bg,
                                  color: badge.color,
                                  border: `1px solid ${badge.border}`,
                                }}
                              />
                              <Chip
                                label={isBuiltin ? "System Default" : "Custom"}
                                size="small"
                                sx={{
                                  height: 20,
                                  fontSize: "0.68rem",
                                  fontWeight: 700,
                                  bgcolor: isBuiltin ? "rgba(79, 70, 229, 0.1)" : "rgba(16, 185, 129, 0.12)",
                                  color: isBuiltin ? "#4F46E5" : "#059669",
                                }}
                              />
                            </Box>

                            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                              <Tooltip title="View template source (read-only)">
                                <IconButton
                                  size="small"
                                  onClick={() => handleOpenViewTemplate(t)}
                                  sx={{ color: "#64748B" }}
                                >
                                  <Eye size={16} />
                                </IconButton>
                              </Tooltip>
                              <Tooltip title="Clone as new template">
                                <IconButton
                                  size="small"
                                  onClick={() => handleOpenCloneTemplate(t)}
                                  sx={{ color: "#64748B" }}
                                >
                                  <Copy size={16} />
                                </IconButton>
                              </Tooltip>
                              {!isBuiltin && (
                                <>
                                  <Tooltip title="Edit template">
                                    <IconButton
                                      size="small"
                                      onClick={() => handleOpenEditTemplate(t)}
                                      sx={{ color: "#64748B" }}
                                    >
                                      <Edit2 size={16} />
                                    </IconButton>
                                  </Tooltip>
                                  <Tooltip title="Delete template">
                                    <IconButton
                                      size="small"
                                      onClick={() => setTemplateToDelete(t)}
                                      sx={{ color: "#EF4444" }}
                                    >
                                      <Trash2 size={16} />
                                    </IconButton>
                                  </Tooltip>
                                </>
                              )}
                            </Box>
                          </Box>

                          {t.description && (
                            <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.825rem" }}>
                              {t.description}
                            </Typography>
                          )}

                          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mt: 0.5 }}>
                            <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                              {lineCount} lines •{" "}
                              {t.content ? `${Math.round((t.content.length / 1024) * 10) / 10} KB` : "0 KB"}
                            </Typography>
                          </Box>
                        </Box>
                      );
                    })
                  )}
                </Box>
              )}
            </DialogContent>
          </Box>
        )}

        {/* Tab 5: Cloudflare DNS Integration */}
        {activeTab === "dns" && (
          <form
            onSubmit={handleSaveDNSSettings}
            style={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}
          >
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Informative Header / Description */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F0F9FF",
                  border: "1px solid #BAE6FD",
                  display: "flex",
                  flexDirection: "column",
                  gap: 0.8,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Cloud size={20} color="#0284C7" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0369A1" }}>
                    Cloudflare DNS Integration
                  </Typography>
                </Box>
                <Typography variant="body2" sx={{ color: "#0C4A6E", fontSize: "0.85rem", lineHeight: 1.5 }}>
                  Automatically publish your easy42 nodes' main IPv4 (<b>A</b>) and IPv6 (<b>AAAA</b>) addresses to
                  Cloudflare DNS. Records are automatically updated when adding or renaming nodes, or can be
                  synchronized on-demand.
                </Typography>
              </Box>

              {dnsSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {dnsSuccess}
                </Alert>
              )}

              {dnsError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {dnsError}
                </Alert>
              )}

              {dnsSyncResult && (
                <Box
                  sx={{
                    p: 1.8,
                    borderRadius: 2,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    display: "flex",
                    flexDirection: "column",
                    gap: 1,
                  }}
                >
                  <Typography
                    variant="caption"
                    sx={{ fontWeight: 700, color: "#475569", textTransform: "uppercase", letterSpacing: "0.05em" }}
                  >
                    Sync Statistics
                  </Typography>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                    <Chip
                      size="small"
                      label={`Created: ${dnsSyncResult.created}`}
                      sx={{ bgcolor: "#DCFCE7", color: "#15803D", fontWeight: 600 }}
                    />
                    <Chip
                      size="small"
                      label={`Updated: ${dnsSyncResult.updated}`}
                      sx={{ bgcolor: "#E0F2FE", color: "#0369A1", fontWeight: 600 }}
                    />
                    <Chip
                      size="small"
                      label={`Deleted: ${dnsSyncResult.deleted}`}
                      sx={{ bgcolor: "#FEE2E2", color: "#B91C1C", fontWeight: 600 }}
                    />
                    <Chip
                      size="small"
                      label={`Unchanged: ${dnsSyncResult.ignored}`}
                      sx={{ bgcolor: "#F1F5F9", color: "#475569", fontWeight: 600 }}
                    />
                  </Box>
                </Box>
              )}

              {dnsLoading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                  <CircularProgress size={24} />
                </Box>
              ) : (
                <>
                  <TextFieldWithCopy
                    fullWidth
                    size="small"
                    label="Cloudflare Zone ID"
                    placeholder="e.g. 023e105f4ecef8ad9ca31a8372d0c353"
                    value={cfZoneId}
                    onChange={(e) => setCfZoneId(e.target.value)}
                    helperText="Your domain's Zone ID found on the Cloudflare dashboard Overview tab."
                    disabled={dnsSaving || dnsSyncing || dnsForceSyncing}
                  />

                  <TextField
                    fullWidth
                    size="small"
                    label="Cloudflare API Token"
                    placeholder="e.g. secret_api_token"
                    type={showCfToken ? "text" : "password"}
                    value={cfApiToken}
                    onChange={(e) => setCfApiToken(e.target.value)}
                    helperText="Create a token with 'Zone.DNS - Edit' permission in Cloudflare User API Tokens."
                    disabled={dnsSaving || dnsSyncing || dnsForceSyncing}
                    slotProps={{
                      input: {
                        endAdornment: (
                          <InputAdornment position="end">
                            <IconButton
                              size="small"
                              edge="end"
                              onClick={() => setShowCfToken(!showCfToken)}
                              aria-label="toggle API token visibility"
                            >
                              {showCfToken ? <EyeOff size={16} /> : <Eye size={16} />}
                            </IconButton>
                          </InputAdornment>
                        ),
                      },
                    }}
                  />

                  <TextFieldWithCopy
                    fullWidth
                    size="small"
                    label="Base Domain"
                    placeholder="e.g. easy42.example.com"
                    value={cfBaseDomain}
                    onChange={(e) => setCfBaseDomain(e.target.value)}
                    helperText="Node hostnames are published as <name>.<domain> and *.<name>.<domain> under this base domain."
                    disabled={dnsSaving || dnsSyncing || dnsForceSyncing}
                  />

                  {/* Toggle: Publish main ipv6 to it's own name */}
                  <Box
                    sx={{
                      p: 2,
                      borderRadius: 2,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 0.8,
                    }}
                  >
                    <FormControlLabel
                      control={
                        <Switch
                          checked={cfPublishIPv6OwnName}
                          onChange={(e) => setCfPublishIPv6OwnName(e.target.checked)}
                          disabled={dnsSaving || dnsSyncing || dnsForceSyncing}
                          color="primary"
                        />
                      }
                      label={
                        <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                          Publish main ipv6 to it's own name
                        </Typography>
                      }
                    />
                    <Typography variant="caption" sx={{ color: "#64748B", ml: 3.5, mt: -0.5 }}>
                      {cfPublishIPv6OwnName
                        ? "When enabled, main IPv6 is published to <name>6.<domain> (separate AAAA record), while main IPv4 is published to <name>.<domain> and *.<name>.<domain> (A records)."
                        : "When disabled, main IPv4 (A) and main IPv6 (AAAA) are both published under <name>.<domain> and *.<name>.<domain>."}
                    </Typography>
                  </Box>

                  {/* Live DNS Record Preview */}
                  <Box
                    sx={{
                      p: 2,
                      borderRadius: 2,
                      border: "1px dashed #CBD5E1",
                      bgcolor: "#F8FAFC",
                      display: "flex",
                      flexDirection: "column",
                      gap: 1.2,
                    }}
                  >
                    <Typography
                      variant="caption"
                      sx={{ fontWeight: 700, color: "#475569", textTransform: "uppercase", letterSpacing: "0.05em" }}
                    >
                      DNS Records Preview (Example Node: node1)
                    </Typography>
                    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 1,
                          fontFamily: "monospace",
                          fontSize: "0.82rem",
                          color: "#1E293B",
                        }}
                      >
                        <Chip
                          label="A"
                          size="small"
                          sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#E0E7FF", color: "#4338CA" }}
                        />
                        <span>{`node1.${cfBaseDomain.trim() || "easy42.example.com"}`}</span>
                        <span style={{ color: "#94A3B8" }}>→</span>
                        <span style={{ color: "#059669" }}>192.168.100.1</span>
                      </Box>
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 1,
                          fontFamily: "monospace",
                          fontSize: "0.82rem",
                          color: "#1E293B",
                        }}
                      >
                        <Chip
                          label="A"
                          size="small"
                          sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#E0E7FF", color: "#4338CA" }}
                        />
                        <span>{`*.node1.${cfBaseDomain.trim() || "easy42.example.com"}`}</span>
                        <span style={{ color: "#94A3B8" }}>→</span>
                        <span style={{ color: "#059669" }}>192.168.100.1</span>
                      </Box>
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          gap: 1,
                          fontFamily: "monospace",
                          fontSize: "0.82rem",
                          color: "#1E293B",
                        }}
                      >
                        <Chip
                          label="AAAA"
                          size="small"
                          sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#EDE9FE", color: "#6D28D9" }}
                        />
                        <span>
                          {cfPublishIPv6OwnName
                            ? `node16.${cfBaseDomain.trim() || "easy42.example.com"}`
                            : `node1.${cfBaseDomain.trim() || "easy42.example.com"}`}
                        </span>
                        <span style={{ color: "#94A3B8" }}>→</span>
                        <span style={{ color: "#059669" }}>fd42:a159:f9f0::1</span>
                      </Box>
                      {!cfPublishIPv6OwnName && (
                        <Box
                          sx={{
                            display: "flex",
                            alignItems: "center",
                            gap: 1,
                            fontFamily: "monospace",
                            fontSize: "0.82rem",
                            color: "#1E293B",
                          }}
                        >
                          <Chip
                            label="AAAA"
                            size="small"
                            sx={{
                              height: 20,
                              fontSize: "0.7rem",
                              fontWeight: 700,
                              bgcolor: "#EDE9FE",
                              color: "#6D28D9",
                            }}
                          />
                          <span>{`*.node1.${cfBaseDomain.trim() || "easy42.example.com"}`}</span>
                          <span style={{ color: "#94A3B8" }}>→</span>
                          <span style={{ color: "#059669" }}>fd42:a159:f9f0::1</span>
                        </Box>
                      )}
                    </Box>
                  </Box>
                  <Box
                    sx={{
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                      pt: 2,
                      flexWrap: "wrap",
                      gap: 1.5,
                    }}
                  >
                    <Box sx={{ display: "flex", gap: 1 }}>
                      <Button
                        variant="outlined"
                        size="small"
                        onClick={() => handleSyncDNS(false)}
                        disabled={
                          dnsLoading ||
                          dnsSaving ||
                          dnsSyncing ||
                          dnsForceSyncing ||
                          !cfZoneId ||
                          !cfApiToken ||
                          !cfBaseDomain
                        }
                        startIcon={
                          dnsSyncing ? <CircularProgress size={14} color="inherit" /> : <RefreshCw size={14} />
                        }
                        sx={{
                          fontWeight: 600,
                          textTransform: "none",
                          borderColor: "#0284C7",
                          color: "#0284C7",
                          "&:hover": { borderColor: "#0369A1", bgcolor: "rgba(2, 132, 199, 0.04)" },
                        }}
                      >
                        {dnsSyncing ? "Syncing..." : "Sync DNS"}
                      </Button>
                      <Button
                        variant="outlined"
                        size="small"
                        color="warning"
                        onClick={() => handleSyncDNS(true)}
                        disabled={
                          dnsLoading ||
                          dnsSaving ||
                          dnsSyncing ||
                          dnsForceSyncing ||
                          !cfZoneId ||
                          !cfApiToken ||
                          !cfBaseDomain
                        }
                        startIcon={dnsForceSyncing ? <CircularProgress size={14} color="inherit" /> : <Zap size={14} />}
                        sx={{
                          fontWeight: 600,
                          textTransform: "none",
                          borderColor: "#F59E0B",
                          color: "#D97706",
                          "&:hover": { borderColor: "#D97706", bgcolor: "rgba(245, 158, 11, 0.04)" },
                        }}
                      >
                        {dnsForceSyncing ? "Force Syncing..." : "Force Sync"}
                      </Button>
                    </Box>

                    <Button
                      type="submit"
                      variant="contained"
                      disabled={dnsLoading || dnsSaving || dnsSyncing || dnsForceSyncing}
                      startIcon={dnsSaving && <CircularProgress size={16} color="inherit" />}
                      sx={{ fontWeight: 600, bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                    >
                      {dnsSaving ? "Saving..." : "Save Settings"}
                    </Button>
                  </Box>
                </>
              )}
            </DialogContent>
          </form>
        )}

        {/* Tab: Server Background Tasks */}
        {activeTab === "tasks" && (
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}>
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Header description */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <Box
                    sx={{
                      width: 40,
                      height: 40,
                      borderRadius: 2,
                      bgcolor: "#EEF2FF",
                      color: "#4F46E5",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      flexShrink: 0,
                    }}
                  >
                    <CalendarClock size={22} />
                  </Box>
                  <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      Server Background Tasks
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", lineHeight: 1.4 }}>
                      Central management for easy42 background tasks. The task list is immutable; configure schedules,
                      toggle automation, or run any task on demand.
                    </Typography>
                  </Box>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<RefreshCw size={14} />}
                  onClick={loadServerTasks}
                  disabled={tasksLoading}
                  sx={{
                    textTransform: "none",
                    fontWeight: 600,
                    flexShrink: 0,
                    borderColor: "#CBD5E1",
                    color: "#334155",
                    "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
                  }}
                >
                  {tasksLoading ? "Refreshing..." : "Refresh"}
                </Button>
              </Box>

              {tasksSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {tasksSuccess}
                </Alert>
              )}

              {tasksError && (
                <Alert severity="error" icon={<AlertCircle size={18} />} sx={{ borderRadius: 2 }}>
                  {tasksError}
                </Alert>
              )}

              {tasksLoading && serverTasks.length === 0 ? (
                <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", py: 8 }}>
                  <CircularProgress size={32} />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
                  {serverTasks.map((task) => {
                    const isRunning = runningTaskId === task.id || task.last_status === "running";
                    const isSaving = savingTaskId === task.id;
                    const draftCron = taskCronDraft[task.id] !== undefined ? taskCronDraft[task.id] : task.cron_expr;
                    const draftEnabled =
                      taskEnabledDraft[task.id] !== undefined ? taskEnabledDraft[task.id] : task.enabled;
                    const hasChanges = draftCron !== task.cron_expr || draftEnabled !== task.enabled;

                    const getTaskIcon = (id: string) => {
                      switch (id) {
                        case "update_roa":
                          return <Globe size={20} color="#2563EB" />;
                        case "sync":
                          return <Zap size={20} color="#7C3AED" />;
                        case "sync_dns":
                          return <Cloud size={20} color="#0284C7" />;
                        default:
                          return <Activity size={20} color="#4F46E5" />;
                      }
                    };

                    const getTaskBg = (id: string) => {
                      switch (id) {
                        case "update_roa":
                          return "#EFF6FF";
                        case "sync":
                          return "#F5F3FF";
                        case "sync_dns":
                          return "#F0F9FF";
                        default:
                          return "#EEF2FF";
                      }
                    };

                    return (
                      <Paper
                        key={task.id}
                        elevation={0}
                        sx={{
                          p: 2.5,
                          borderRadius: 2.5,
                          border: "1px solid #E2E8F0",
                          bgcolor: "#FFFFFF",
                          display: "flex",
                          flexDirection: "column",
                          gap: 2,
                          boxShadow: "0 1px 3px rgba(0,0,0,0.03)",
                        }}
                      >
                        {/* Task Card Header */}
                        <Box
                          sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2 }}
                        >
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                            <Box
                              sx={{
                                width: 40,
                                height: 40,
                                borderRadius: 2,
                                bgcolor: getTaskBg(task.id),
                                display: "flex",
                                alignItems: "center",
                                justifyContent: "center",
                                flexShrink: 0,
                              }}
                            >
                              {getTaskIcon(task.id)}
                            </Box>
                            <Box>
                              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                                <Typography sx={{ fontWeight: 700, fontSize: "1.05rem", color: "#0F172A" }}>
                                  {task.name}
                                </Typography>
                                <Chip
                                  size="small"
                                  label={task.id}
                                  sx={{
                                    fontFamily: "monospace",
                                    fontSize: "0.72rem",
                                    fontWeight: 600,
                                    height: 20,
                                    bgcolor: "#F1F5F9",
                                    color: "#475569",
                                  }}
                                />
                                <Chip
                                  size="small"
                                  label="Predefined"
                                  sx={{
                                    fontSize: "0.72rem",
                                    fontWeight: 600,
                                    height: 20,
                                    bgcolor: "#EDE9FE",
                                    color: "#6D28D9",
                                  }}
                                />
                              </Box>
                              <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.3 }}>
                                {task.description}
                              </Typography>
                            </Box>
                          </Box>

                          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexShrink: 0 }}>
                            {/* Status Chip */}
                            {isRunning ? (
                              <Chip
                                size="small"
                                icon={<CircularProgress size={12} color="inherit" />}
                                label="Running"
                                sx={{ bgcolor: "#DBEAFE", color: "#1D4ED8", fontWeight: 700, fontSize: "0.8rem" }}
                              />
                            ) : task.last_status === "success" ? (
                              <Chip
                                size="small"
                                icon={<CheckCircle2 size={13} color="#15803D" />}
                                label="Success"
                                sx={{ bgcolor: "#DCFCE7", color: "#15803D", fontWeight: 700, fontSize: "0.8rem" }}
                              />
                            ) : task.last_status === "error" ? (
                              <Chip
                                size="small"
                                icon={<AlertCircle size={13} color="#B91C1C" />}
                                label="Failed"
                                sx={{ bgcolor: "#FEE2E2", color: "#B91C1C", fontWeight: 700, fontSize: "0.8rem" }}
                              />
                            ) : (
                              <Chip
                                size="small"
                                icon={<Clock size={13} color="#64748B" />}
                                label="Never Run"
                                sx={{ bgcolor: "#F1F5F9", color: "#64748B", fontWeight: 600, fontSize: "0.8rem" }}
                              />
                            )}

                            {/* Run Now Button */}
                            <Button
                              variant="outlined"
                              size="small"
                              startIcon={
                                isRunning ? (
                                  <CircularProgress size={14} color="inherit" />
                                ) : (
                                  <Play size={14} fill="currentColor" />
                                )
                              }
                              onClick={() => handleRunTaskImmediately(task.id)}
                              disabled={isRunning || isSaving || runningTaskId !== null}
                              sx={{
                                textTransform: "none",
                                fontWeight: 600,
                                borderColor: "#4F46E5",
                                color: "#4F46E5",
                                "&:hover": { borderColor: "#4338CA", bgcolor: "rgba(79, 70, 229, 0.04)" },
                              }}
                            >
                              {isRunning ? "Running..." : "Run Now"}
                            </Button>
                          </Box>
                        </Box>

                        <Divider sx={{ borderColor: "#F1F5F9" }} />

                        {/* Schedule Configuration Box */}
                        <Box
                          sx={{
                            p: 2,
                            borderRadius: 2,
                            bgcolor: "#F8FAFC",
                            border: "1px solid #E2E8F0",
                            display: "flex",
                            flexDirection: "column",
                            gap: 1.8,
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
                            <FormControlLabel
                              control={
                                <Switch
                                  checked={draftEnabled}
                                  onChange={(e) =>
                                    setTaskEnabledDraft((prev) => ({ ...prev, [task.id]: e.target.checked }))
                                  }
                                  color="primary"
                                />
                              }
                              label={
                                <Typography
                                  sx={{
                                    fontWeight: 600,
                                    fontSize: "0.875rem",
                                    color: draftEnabled ? "#0F172A" : "#64748B",
                                  }}
                                >
                                  {draftEnabled ? "Automated Execution Active" : "Automated Execution Disabled"}
                                </Typography>
                              }
                            />

                            <Button
                              variant="contained"
                              size="small"
                              onClick={() => handleSaveTaskConfig(task.id)}
                              disabled={!hasChanges || isSaving}
                              startIcon={isSaving && <CircularProgress size={14} color="inherit" />}
                              sx={{
                                textTransform: "none",
                                fontWeight: 600,
                                bgcolor: "#4F46E5",
                                "&:hover": { bgcolor: "#4338CA" },
                              }}
                            >
                              {isSaving ? "Saving..." : "Save Settings"}
                            </Button>
                          </Box>

                          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
                              <TextField
                                size="small"
                                label="Cron Expression"
                                value={draftCron}
                                onChange={(e) => setTaskCronDraft((prev) => ({ ...prev, [task.id]: e.target.value }))}
                                placeholder="0 0 * * *"
                                helperText={getCronHumanSummary(draftCron)}
                                sx={{
                                  width: { xs: "100%", sm: 260 },
                                  "& .MuiInputBase-input": { fontFamily: "monospace", fontWeight: 600 },
                                }}
                              />

                              <Box
                                sx={{
                                  display: "flex",
                                  alignItems: "center",
                                  flexWrap: "wrap",
                                  gap: 0.8,
                                  mt: { xs: 0, sm: -2 },
                                }}
                              >
                                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, mr: 0.5 }}>
                                  Presets:
                                </Typography>
                                {getTaskCronPresets(draftCron).map((preset) => {
                                  const normalizedDraft = (draftCron || "").trim().replace(/\s+/g, " ");
                                  const isSelected = normalizedDraft === preset.value;
                                  return (
                                    <Chip
                                      key={preset.label}
                                      size="small"
                                      label={preset.label}
                                      clickable
                                      variant={"outlined"}
                                      onClick={() => setTaskCronDraft((prev) => ({ ...prev, [task.id]: preset.value }))}
                                      sx={{
                                        fontSize: "0.75rem",
                                        fontWeight: 500,
                                        bgcolor: isSelected ? "#4F46E5" : "transparent",
                                        color: isSelected ? "#FFFFFF" : "#475569",
                                        borderColor: "#CBD5E1",
                                      }}
                                    />
                                  );
                                })}
                              </Box>
                            </Box>
                          </Box>
                        </Box>

                        {/* Execution State & Telemetry */}
                        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                          <Box
                            sx={{
                              display: "grid",
                              gridTemplateColumns: { xs: "1fr", sm: "1fr 1.2fr 0.8fr" },
                              gap: 1.5,
                            }}
                          >
                            <Box
                              sx={{
                                p: 1.5,
                                borderRadius: 1.5,
                                bgcolor: "#F8FAFC",
                                border: "1px solid #E2E8F0",
                                display: "flex",
                                flexDirection: "column",
                                gap: 0.3,
                              }}
                            >
                              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, color: "#64748B" }}>
                                <CalendarClock size={14} />
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.04em" }}
                                >
                                  Next Run
                                </Typography>
                              </Box>
                              <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                                {draftEnabled && task.next_run
                                  ? formatDateTime(task.next_run)
                                  : draftEnabled
                                    ? "Scheduled"
                                    : "Disabled"}
                              </Typography>
                            </Box>

                            <Box
                              sx={{
                                p: 1.5,
                                borderRadius: 1.5,
                                bgcolor: "#F8FAFC",
                                border: "1px solid #E2E8F0",
                                display: "flex",
                                flexDirection: "column",
                                gap: 0.3,
                              }}
                            >
                              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, color: "#64748B" }}>
                                <Clock size={14} />
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.04em" }}
                                >
                                  Last Executed
                                </Typography>
                              </Box>
                              <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                                {task.last_run ? (
                                  <>
                                    {formatDateTime(task.last_run)}{" "}
                                    <span style={{ color: "#64748B", fontWeight: 400 }}>
                                      ({formatRelativeTime(task.last_run)})
                                    </span>
                                  </>
                                ) : (
                                  "Never executed"
                                )}
                              </Typography>
                            </Box>

                            <Box
                              sx={{
                                p: 1.5,
                                borderRadius: 1.5,
                                bgcolor: "#F8FAFC",
                                border: "1px solid #E2E8F0",
                                display: "flex",
                                flexDirection: "column",
                                gap: 0.3,
                              }}
                            >
                              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, color: "#64748B" }}>
                                <Activity size={14} />
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.04em" }}
                                >
                                  Duration
                                </Typography>
                              </Box>
                              <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                                {task.last_run ? `${task.last_duration_ms} ms` : "—"}
                              </Typography>
                            </Box>
                          </Box>

                          {/* Last Result Box */}
                          {task.last_message && (
                            <Box
                              sx={{
                                p: 1.5,
                                borderRadius: 1.5,
                                bgcolor:
                                  task.last_status === "error"
                                    ? "#FEF2F2"
                                    : task.last_status === "success"
                                      ? "#F0FDF4"
                                      : "#F8FAFC",
                                border:
                                  task.last_status === "error"
                                    ? "1px solid #FECACA"
                                    : task.last_status === "success"
                                      ? "1px solid #BBF7D0"
                                      : "1px solid #E2E8F0",
                                display: "flex",
                                alignItems: "flex-start",
                                gap: 1.2,
                              }}
                            >
                              {task.last_status === "error" ? (
                                <AlertCircle size={16} color="#DC2626" style={{ marginTop: 2, flexShrink: 0 }} />
                              ) : task.last_status === "success" ? (
                                <CheckCircle2 size={16} color="#16A34A" style={{ marginTop: 2, flexShrink: 0 }} />
                              ) : (
                                <Activity size={16} color="#475569" style={{ marginTop: 2, flexShrink: 0 }} />
                              )}
                              <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography
                                  variant="caption"
                                  sx={{
                                    fontWeight: 700,
                                    textTransform: "uppercase",
                                    letterSpacing: "0.04em",
                                    color:
                                      task.last_status === "error"
                                        ? "#991B1B"
                                        : task.last_status === "success"
                                          ? "#166534"
                                          : "#475569",
                                    display: "block",
                                    mb: 0.3,
                                  }}
                                >
                                  {task.last_status === "error" ? "Execution Error" : "Execution Result"}
                                </Typography>
                                <Typography
                                  variant="body2"
                                  sx={{
                                    fontFamily: "monospace",
                                    fontSize: "0.82rem",
                                    color:
                                      task.last_status === "error"
                                        ? "#B91C1C"
                                        : task.last_status === "success"
                                          ? "#15803D"
                                          : "#334155",
                                    wordBreak: "break-all",
                                  }}
                                >
                                  {task.last_message}
                                </Typography>
                              </Box>
                            </Box>
                          )}
                        </Box>
                      </Paper>
                    );
                  })}
                </Box>
              )}
            </DialogContent>
          </Box>
        )}

        {/* Tab: Database & Storage Maintenance */}
        {activeTab === "database" && (
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}>
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Header description */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <Box
                    sx={{
                      width: 40,
                      height: 40,
                      borderRadius: 2,
                      bgcolor: "#EEF2FF",
                      color: "#4F46E5",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      flexShrink: 0,
                    }}
                  >
                    <Database size={22} />
                  </Box>
                  <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      SQLite Database & Storage
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", lineHeight: 1.4 }}>
                      Monitor SQLite data storage metrics, configure unified retention duration for metrics and cron
                      results, and reclaim unused disk space.
                    </Typography>
                  </Box>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<RefreshCw size={14} className={dbLoading ? "spin" : ""} />}
                  onClick={loadDatabaseInfo}
                  disabled={dbLoading || dbVacuuming}
                  sx={{
                    textTransform: "none",
                    fontWeight: 600,
                    flexShrink: 0,
                    borderColor: "#CBD5E1",
                    color: "#334155",
                    "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
                  }}
                >
                  {dbLoading ? "Refreshing..." : "Refresh"}
                </Button>
              </Box>

              {dbSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {dbSuccess}
                </Alert>
              )}

              {dbError && (
                <Alert icon={<AlertCircle size={18} />} severity="error" sx={{ borderRadius: 2 }}>
                  {dbError}
                </Alert>
              )}

              {dbLoading && !dbInfo ? (
                <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", py: 8 }}>
                  <CircularProgress size={32} />
                </Box>
              ) : dbInfo ? (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
                  {/* Card 1: Storage Overview & Statistics */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
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
                        <HardDrive size={18} color="#4F46E5" />
                        <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                          Database State & Storage Metrics
                        </Typography>
                      </Box>
                      <Chip
                        size="small"
                        label="SQLite 3 (WAL Mode)"
                        sx={{ bgcolor: "#F1F5F9", color: "#475569", fontWeight: 600, fontSize: "0.75rem" }}
                      />
                    </Box>

                    {/* Stats Grid */}
                    <Box
                      sx={{
                        display: "grid",
                        gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr", md: "1fr 1fr 1fr 1fr" },
                        gap: 2,
                      }}
                    >
                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Typography
                          variant="caption"
                          sx={{
                            color: "#64748B",
                            fontWeight: 600,
                            textTransform: "uppercase",
                            letterSpacing: "0.04em",
                          }}
                        >
                          Total DB Size
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mt: 0.5 }}>
                          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            {dbInfo.total_size_human}
                          </Typography>
                        </Box>
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          {dbInfo.total_size_bytes.toLocaleString()} bytes
                        </Typography>
                        {Boolean(dbInfo.wal_size_bytes && dbInfo.wal_size_bytes > 0) && (
                          <Box sx={{ mt: 0.8 }}>
                            <Chip
                              size="small"
                              label={`WAL: ${((dbInfo.wal_size_bytes || 0) / 1024).toFixed(1)} KB`}
                              sx={{
                                height: 20,
                                fontSize: "0.7rem",
                                bgcolor: "#EFF6FF",
                                color: "#1D4ED8",
                                fontWeight: 600,
                              }}
                            />
                          </Box>
                        )}
                      </Box>

                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Typography
                          variant="caption"
                          sx={{
                            color: "#64748B",
                            fontWeight: 600,
                            textTransform: "uppercase",
                            letterSpacing: "0.04em",
                          }}
                        >
                          Retention Threshold
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mt: 0.5 }}>
                          <Typography variant="h5" sx={{ fontWeight: 700, color: "#4F46E5" }}>
                            {dbInfo.raw_retention_days || 2}d / {dbInfo.retention_days}d
                          </Typography>
                        </Box>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {dbInfo.raw_retention_days || 2}d raw, {dbInfo.retention_days}d 5m summary
                        </Typography>
                      </Box>

                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Typography
                          variant="caption"
                          sx={{
                            color: "#64748B",
                            fontWeight: 600,
                            textTransform: "uppercase",
                            letterSpacing: "0.04em",
                          }}
                        >
                          Telemetry Records
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mt: 0.5 }}>
                          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            {(
                              (dbInfo.metrics_count || 0) +
                              (dbInfo.metrics_5m_count || 0) +
                              (dbInfo.interface_metrics_count || 0)
                            ).toLocaleString()}
                          </Typography>
                        </Box>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {(dbInfo.metrics_count || 0).toLocaleString()} node raw
                          {dbInfo.interface_metrics_count
                            ? `, ${dbInfo.interface_metrics_count.toLocaleString()} iface raw`
                            : ""}{" "}
                          / {(dbInfo.metrics_5m_count || 0).toLocaleString()} 5m summary
                        </Typography>
                      </Box>

                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Typography
                          variant="caption"
                          sx={{
                            color: "#64748B",
                            fontWeight: 600,
                            textTransform: "uppercase",
                            letterSpacing: "0.04em",
                          }}
                        >
                          Cron Task Results
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "baseline", gap: 1, mt: 0.5 }}>
                          <Typography variant="h5" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            {((dbInfo.cron_results_count || 0) + (dbInfo.cron_results_5m_count || 0)).toLocaleString()}
                          </Typography>
                        </Box>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {(dbInfo.cron_results_count || 0).toLocaleString()} raw /{" "}
                          {(dbInfo.cron_results_5m_count || 0).toLocaleString()} 5m summary
                        </Typography>
                      </Box>
                    </Box>

                    {/* Detailed Properties Row */}
                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        bgcolor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
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
                          <Typography variant="caption" sx={{ fontWeight: 600, color: "#64748B" }}>
                            Database File:
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              fontFamily: "monospace",
                              bgcolor: "#FFFFFF",
                              px: 1,
                              py: 0.3,
                              borderRadius: 1,
                              border: "1px solid #CBD5E1",
                              fontSize: "0.8rem",
                              color: "#1E293B",
                            }}
                          >
                            {dbInfo.file_path}
                          </Typography>
                        </Box>
                        <Button
                          size="small"
                          startIcon={<Copy size={13} />}
                          onClick={() => {
                            if (dbInfo.file_path) {
                              navigator.clipboard.writeText(dbInfo.file_path);
                            }
                          }}
                          sx={{ textTransform: "none", fontSize: "0.75rem", color: "#64748B" }}
                        >
                          Copy Path
                        </Button>
                      </Box>

                      <Divider sx={{ borderColor: "#E2E8F0" }} />

                      <Box
                        sx={{
                          display: "grid",
                          gridTemplateColumns: { xs: "1fr", sm: "repeat(3, 1fr)" },
                          gap: 1.5,
                        }}
                      >
                        <Box>
                          <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                            Page Size
                          </Typography>
                          <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                            {dbInfo.page_size ? `${dbInfo.page_size} bytes` : "4096 bytes"}
                          </Typography>
                        </Box>
                        <Box>
                          <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                            Allocated Pages
                          </Typography>
                          <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                            {dbInfo.page_count ? dbInfo.page_count.toLocaleString() : "0"}
                          </Typography>
                        </Box>
                        <Box>
                          <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                            Freelist Pages
                          </Typography>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                            <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                              {dbInfo.freelist_count ? dbInfo.freelist_count.toLocaleString() : "0"}
                            </Typography>
                            {Boolean(dbInfo.freelist_count && dbInfo.freelist_count > 0) && (
                              <Chip
                                size="small"
                                label="Reclaimable Space"
                                sx={{
                                  height: 18,
                                  fontSize: "0.65rem",
                                  bgcolor: "#FEF3C7",
                                  color: "#B45309",
                                  fontWeight: 600,
                                }}
                              />
                            )}
                          </Box>
                        </Box>
                      </Box>
                    </Box>
                  </Paper>

                  {/* Card 2: Data Retention Settings */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Clock size={18} color="#4F46E5" />
                      <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                        Data Retention Policy
                      </Typography>
                    </Box>
                    <Typography variant="body2" sx={{ color: "#64748B", lineHeight: 1.5 }}>
                      Configure easy42's tiered data retention strategy. High-resolution raw samples (1s–10s telemetry
                      and full probe outputs) are kept for a short period to draw accurate 1h, 6h, and 24h charts. Older
                      data is automatically summarized into compact 5-minute statistical windows (averages, min/max, run
                      counts) and retained until long-term retention expires.
                    </Typography>

                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        bgcolor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
                        display: "flex",
                        flexDirection: "column",
                        gap: 2.5,
                      }}
                    >
                      {/* Setting 1: Raw Data Retention */}
                      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                          1. Raw Data Retention (High Resolution)
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Duration to retain granular raw telemetry metrics and raw probe outputs before summarizing
                          into 5-minute windows.
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 2, mt: 0.5 }}>
                          <TextField
                            label="Raw Retention"
                            type="number"
                            size="small"
                            value={rawRetentionDraft}
                            onChange={(e) => setRawRetentionDraft(e.target.value)}
                            InputProps={{
                              endAdornment: <InputAdornment position="end">days</InputAdornment>,
                              inputProps: { min: 1, max: 30 },
                            }}
                            sx={{ width: 170, bgcolor: "#FFFFFF" }}
                          />
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                            {[
                              { days: 1, label: "1 Day" },
                              { days: 2, label: "2 Days (Recommended)" },
                              { days: 3, label: "3 Days" },
                              { days: 7, label: "7 Days" },
                            ].map((preset) => {
                              const isSelected = Number(rawRetentionDraft) === preset.days;
                              return (
                                <Chip
                                  key={preset.days}
                                  size="small"
                                  label={preset.label}
                                  clickable
                                  variant={isSelected ? "filled" : "outlined"}
                                  onClick={() => setRawRetentionDraft(preset.days)}
                                  sx={{
                                    fontSize: "0.75rem",
                                    fontWeight: 500,
                                    bgcolor: isSelected ? "#4F46E5" : "transparent",
                                    color: isSelected ? "#FFFFFF" : "#475569",
                                    borderColor: "#CBD5E1",
                                  }}
                                />
                              );
                            })}
                          </Box>
                        </Box>
                      </Box>

                      <Divider sx={{ borderColor: "#E2E8F0" }} />

                      {/* Setting 2: Long-Term Summary Retention */}
                      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                          2. Long-Term Retention (5-Minute Summaries)
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          Duration to keep summarized 5-minute statistical windows for long-term historical trends.
                        </Typography>
                        <Box sx={{ display: "flex", alignItems: "center", flexWrap: "wrap", gap: 2, mt: 0.5 }}>
                          <TextField
                            label="Summary Retention"
                            type="number"
                            size="small"
                            value={retentionDraft}
                            onChange={(e) => setRetentionDraft(e.target.value)}
                            InputProps={{
                              endAdornment: <InputAdornment position="end">days</InputAdornment>,
                              inputProps: { min: 1, max: 3650 },
                            }}
                            sx={{ width: 170, bgcolor: "#FFFFFF" }}
                          />
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                            {[
                              { days: 7, label: "7 Days" },
                              { days: 14, label: "14 Days" },
                              { days: 30, label: "30 Days" },
                              { days: 47, label: "47 Days (Default)" },
                              { days: 90, label: "90 Days" },
                              { days: 180, label: "180 Days" },
                            ].map((preset) => {
                              const isSelected = Number(retentionDraft) === preset.days;
                              return (
                                <Chip
                                  key={preset.days}
                                  size="small"
                                  label={preset.label}
                                  clickable
                                  variant={isSelected ? "filled" : "outlined"}
                                  onClick={() => setRetentionDraft(preset.days)}
                                  sx={{
                                    fontSize: "0.75rem",
                                    fontWeight: 500,
                                    bgcolor: isSelected ? "#4F46E5" : "transparent",
                                    color: isSelected ? "#FFFFFF" : "#475569",
                                    borderColor: "#CBD5E1",
                                  }}
                                />
                              );
                            })}
                          </Box>
                        </Box>
                      </Box>

                      <Box sx={{ display: "flex", justifyContent: "flex-start", mt: 1 }}>
                        <Button
                          variant="contained"
                          startIcon={dbSaving ? <CircularProgress size={14} color="inherit" /> : <Check size={16} />}
                          onClick={handleSaveRetention}
                          disabled={
                            dbSaving ||
                            dbLoading ||
                            (Number(retentionDraft) === dbInfo?.retention_days &&
                              Number(rawRetentionDraft) === (dbInfo?.raw_retention_days || 2))
                          }
                          sx={{
                            textTransform: "none",
                            fontWeight: 600,
                            bgcolor: "#4F46E5",
                            "&:hover": { bgcolor: "#4338CA" },
                          }}
                        >
                          {dbSaving ? "Saving..." : "Save Retention Policy"}
                        </Button>
                      </Box>
                    </Box>
                  </Paper>

                  {/* Card 3: Database Space Reclamation */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Sparkles size={18} color="#4F46E5" />
                      <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                        Reclaim Database Space
                      </Typography>
                    </Box>
                    <Typography variant="body2" sx={{ color: "#64748B", lineHeight: 1.5 }}>
                      When older metrics and logs are cleaned, SQLite preserves the empty pages in its internal freelist
                      for future writes, so the file size on disk may not immediately shrink. Triggering a{" "}
                      <strong>VACUUM</strong> rebuilds the entire database file, defragmenting pages, releasing unused
                      freelists, and truncating the Write-Ahead Log (WAL) to return space to the operating system.
                    </Typography>

                    <Box
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        bgcolor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        flexWrap: "wrap",
                        gap: 2,
                      }}
                    >
                      <Box>
                        <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                          Run SQLite VACUUM Optimization
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                          Safe online maintenance. Current database size: {dbInfo.total_size_human}
                        </Typography>
                      </Box>

                      <Button
                        variant="contained"
                        startIcon={
                          dbVacuuming ? <CircularProgress size={14} color="inherit" /> : <Sparkles size={16} />
                        }
                        onClick={handleReclaimDatabase}
                        disabled={dbVacuuming || dbLoading}
                        sx={{
                          textTransform: "none",
                          fontWeight: 600,
                          bgcolor: "#4F46E5",
                          "&:hover": { bgcolor: "#4338CA" },
                        }}
                      >
                        {dbVacuuming ? "Reclaiming Space..." : "Reclaim Database"}
                      </Button>
                    </Box>
                  </Paper>
                </Box>
              ) : null}
            </DialogContent>
          </Box>
        )}

        {/* Tab: Integrations & Prometheus */}
        {activeTab === "integrations" && (
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}>
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Header description */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <Box
                    sx={{
                      width: 40,
                      height: 40,
                      borderRadius: 2,
                      bgcolor: "#EEF2FF",
                      color: "#4F46E5",
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      flexShrink: 0,
                    }}
                  >
                    <Zap size={22} />
                  </Box>
                  <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      Telegram Bot & External Integrations
                    </Typography>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", lineHeight: 1.4 }}>
                      Configure the easy42 Telegram bot for real-time fleet notifications and chat queries, or export
                      live metrics to Prometheus, VictoriaMetrics, and Grafana.
                    </Typography>
                  </Box>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<RefreshCw size={14} className={integrationsLoading ? "spin" : ""} />}
                  onClick={loadIntegrations}
                  disabled={integrationsLoading || integrationsSaving || tgSaving || tgTesting}
                  sx={{
                    textTransform: "none",
                    fontWeight: 600,
                    flexShrink: 0,
                    borderColor: "#CBD5E1",
                    color: "#334155",
                    "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
                  }}
                >
                  {integrationsLoading ? "Refreshing..." : "Refresh"}
                </Button>
              </Box>

              {integrationsSuccess && (
                <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                  {integrationsSuccess}
                </Alert>
              )}

              {integrationsError && (
                <Alert icon={<AlertCircle size={18} />} severity="error" sx={{ borderRadius: 2 }}>
                  {integrationsError}
                </Alert>
              )}

              {integrationsLoading && !integrations ? (
                <Box sx={{ display: "flex", justifyContent: "center", alignItems: "center", py: 8 }}>
                  <CircularProgress size={32} />
                </Box>
              ) : integrations ? (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
                  {/* SECTION 1: Telegram Bot Integration */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 3,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2.5,
                      boxShadow: "0 1px 3px rgba(0,0,0,0.02)",
                    }}
                  >
                    {/* Header with Telegram styling */}
                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        flexWrap: "wrap",
                        gap: 1.5,
                        pb: 2,
                        borderBottom: "1px solid #F1F5F9",
                      }}
                    >
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                        <Box
                          sx={{
                            width: 42,
                            height: 42,
                            borderRadius: "12px",
                            background: "linear-gradient(135deg, #0088cc 0%, #29b6f6 100%)",
                            color: "#FFFFFF",
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            boxShadow: "0 4px 10px rgba(0,136,204,0.3)",
                            flexShrink: 0,
                          }}
                        >
                          <Send size={22} style={{ transform: "translate(-1px, 1px)" }} />
                        </Box>
                        <Box>
                          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                            <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "1.05rem" }}>
                              Telegram Bot Integration
                            </Typography>
                            {integrations.telegram_enabled ? (
                              <Chip
                                size="small"
                                icon={<CheckCircle2 size={13} />}
                                label={
                                  integrations.telegram_bot_username
                                    ? `@${integrations.telegram_bot_username}`
                                    : "Active"
                                }
                                sx={{
                                  bgcolor: "#E0F2FE",
                                  color: "#0284C7",
                                  fontWeight: 700,
                                  fontSize: "0.75rem",
                                  height: 24,
                                }}
                              />
                            ) : (
                              <Chip
                                size="small"
                                label="Disabled"
                                sx={{
                                  bgcolor: "#F1F5F9",
                                  color: "#64748B",
                                  fontWeight: 600,
                                  fontSize: "0.75rem",
                                  height: 24,
                                }}
                              />
                            )}
                            {integrations.telegram_enabled && integrations.telegram_webhook_status === "registered" && (
                              <Chip
                                size="small"
                                label="Webhook Active"
                                sx={{
                                  bgcolor: "#ECFDF5",
                                  color: "#059669",
                                  fontWeight: 600,
                                  fontSize: "0.72rem",
                                  height: 22,
                                }}
                              />
                            )}
                            {integrations.telegram_webhook_error && (
                              <Tooltip title={integrations.telegram_webhook_error}>
                                <Chip
                                  size="small"
                                  icon={<AlertCircle size={12} />}
                                  label="Webhook Warning"
                                  sx={{
                                    bgcolor: "#FEF3C7",
                                    color: "#B45309",
                                    fontWeight: 600,
                                    fontSize: "0.72rem",
                                    height: 22,
                                  }}
                                />
                              </Tooltip>
                            )}
                          </Box>
                          <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.3 }}>
                            Receive automated node online/offline alerts and query cluster status directly from
                            Telegram.
                          </Typography>
                        </Box>
                      </Box>

                      <FormControlLabel
                        control={
                          <Switch
                            checked={tgEnabled}
                            onChange={(e) => setTgEnabled(e.target.checked)}
                            size="small"
                            color="primary"
                          />
                        }
                        label={
                          <Typography
                            variant="body2"
                            sx={{ fontWeight: 600, color: tgEnabled ? "#0284C7" : "#64748B" }}
                          >
                            {tgEnabled ? "Bot Active" : "Bot Inactive"}
                          </Typography>
                        }
                        sx={{ m: 0 }}
                      />
                    </Box>

                    {/* Test alert feedback if present */}
                    {tgTestSuccess && (
                      <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                        {tgTestSuccess}
                      </Alert>
                    )}
                    {tgTestError && (
                      <Alert icon={<AlertCircle size={18} />} severity="error" sx={{ borderRadius: 2 }}>
                        {tgTestError}
                      </Alert>
                    )}

                    {/* Form Fields Grid */}
                    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                      {/* Field 1: Bot API Token */}
                      <Box>
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.75 }}>
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 700,
                              color: "#334155",
                              textTransform: "uppercase",
                              letterSpacing: 0.5,
                              fontSize: "0.75rem",
                            }}
                          >
                            Telegram Bot API Token <span style={{ color: "#EF4444" }}>*</span>
                          </Typography>
                          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
                            Provided by <code>@BotFather</code>
                          </Typography>
                        </Box>
                        <TextField
                          fullWidth
                          size="small"
                          placeholder="e.g. 123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ"
                          type={showTgToken ? "text" : "password"}
                          value={tgBotToken}
                          onChange={(e) => setTgBotToken(e.target.value)}
                          InputProps={{
                            endAdornment: (
                              <InputAdornment position="end">
                                <IconButton size="small" onClick={() => setShowTgToken(!showTgToken)} edge="end">
                                  {showTgToken ? <EyeOff size={16} /> : <Eye size={16} />}
                                </IconButton>
                              </InputAdornment>
                            ),
                            sx: {
                              fontFamily: "monospace",
                              fontSize: "0.85rem",
                              bgcolor: "#F8FAFC",
                              "& fieldset": { borderColor: "#CBD5E1" },
                            },
                          }}
                        />
                        <Typography
                          variant="caption"
                          sx={{ color: "#64748B", fontSize: "0.78rem", mt: 0.5, display: "block" }}
                        >
                          The secret HTTP API token for your bot created with Telegram's @BotFather.
                        </Typography>
                      </Box>

                      {/* Field 2: Admin Telegram User ID */}
                      <Box>
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.75 }}>
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 700,
                              color: "#334155",
                              textTransform: "uppercase",
                              letterSpacing: 0.5,
                              fontSize: "0.75rem",
                            }}
                          >
                            Admin Telegram User ID (Implicit Authentication) <span style={{ color: "#EF4444" }}>*</span>
                          </Typography>
                          <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
                            Numeric user ID
                          </Typography>
                        </Box>
                        <TextField
                          fullWidth
                          size="small"
                          placeholder="e.g. 123456789"
                          value={tgAdminID}
                          onChange={(e) => setTgAdminID(e.target.value.replace(/\D/g, ""))}
                          InputProps={{
                            sx: {
                              fontFamily: "monospace",
                              fontSize: "0.85rem",
                              bgcolor: "#F8FAFC",
                              "& fieldset": { borderColor: "#CBD5E1" },
                            },
                          }}
                        />
                        <Typography
                          variant="caption"
                          sx={{ color: "#64748B", fontSize: "0.78rem", mt: 0.5, display: "block" }}
                        >
                          Used as implicit authentication source. Only this single user is authorized to interact with
                          the easy42 server via Telegram and receive node alerts.
                        </Typography>
                      </Box>

                      {/* Field 3: easy42 Server URL */}
                      <Box>
                        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.75 }}>
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 700,
                              color: "#334155",
                              textTransform: "uppercase",
                              letterSpacing: 0.5,
                              fontSize: "0.75rem",
                            }}
                          >
                            easy42 Server URL (Webhook & Notification Links) <span style={{ color: "#EF4444" }}>*</span>
                          </Typography>
                          {tgServerURL !== window.location.origin && (
                            <Button
                              size="small"
                              onClick={() => setTgServerURL(window.location.origin)}
                              sx={{
                                textTransform: "none",
                                p: 0,
                                minWidth: "auto",
                                fontSize: "0.75rem",
                                color: "#0284C7",
                              }}
                            >
                              Reset to current origin
                            </Button>
                          )}
                        </Box>
                        <TextField
                          fullWidth
                          size="small"
                          placeholder="e.g. https://easy42.example.com"
                          value={tgServerURL}
                          onChange={(e) => setTgServerURL(e.target.value)}
                          InputProps={{
                            sx: {
                              fontFamily: "monospace",
                              fontSize: "0.85rem",
                              bgcolor: "#F8FAFC",
                              "& fieldset": { borderColor: "#CBD5E1" },
                            },
                          }}
                        />
                        <Typography
                          variant="caption"
                          sx={{ color: "#64748B", fontSize: "0.78rem", mt: 0.5, display: "block" }}
                        >
                          Derived from current browser origin (<code>{window.location.origin}</code>) and persisted on
                          Save. Used by Telegram to deliver webhook updates to <code>/api/telegram/webhook</code> and as
                          the base URL for alert links.
                        </Typography>
                      </Box>
                    </Box>

                    {/* Action Buttons: Save & Test */}
                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        flexWrap: "wrap",
                        gap: 1.5,
                        pt: 1.5,
                        borderTop: "1px solid #F1F5F9",
                      }}
                    >
                      <Button
                        variant="outlined"
                        onClick={() => setTgGuideOpen(!tgGuideOpen)}
                        startIcon={tgGuideOpen ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                        sx={{
                          textTransform: "none",
                          fontSize: "0.82rem",
                          fontWeight: 600,
                          borderColor: "#CBD5E1",
                          color: "#475569",
                          "&:hover": { borderColor: "#94A3B8", bgcolor: "#F8FAFC" },
                        }}
                      >
                        {tgGuideOpen ? "Hide Setup Guide" : "Show Setup Guide"}
                      </Button>

                      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                        <Button
                          variant="outlined"
                          onClick={handleTestTelegram}
                          disabled={tgTesting || tgSaving || !tgBotToken.trim() || !tgAdminID.trim()}
                          startIcon={tgTesting ? <CircularProgress size={16} /> : <Send size={15} />}
                          sx={{
                            textTransform: "none",
                            fontWeight: 600,
                            borderColor: "#0284C7",
                            color: "#0284C7",
                            "&:hover": { borderColor: "#0369A1", bgcolor: "#F0F9FF" },
                          }}
                        >
                          {tgTesting ? "Testing..." : "Test Connection"}
                        </Button>

                        <Button
                          variant="contained"
                          onClick={handleSaveTelegram}
                          disabled={tgSaving || tgTesting}
                          startIcon={tgSaving ? <CircularProgress size={16} color="inherit" /> : <Check size={16} />}
                          sx={{
                            textTransform: "none",
                            fontWeight: 700,
                            bgcolor: "#0284C7",
                            boxShadow: "0 2px 8px rgba(2,132,199,0.3)",
                            "&:hover": { bgcolor: "#0369A1", boxShadow: "0 4px 12px rgba(2,132,199,0.4)" },
                          }}
                        >
                          {tgSaving ? "Saving..." : "Save Settings"}
                        </Button>
                      </Box>
                    </Box>

                    {/* Concise Setup Guide & Commands Collapsible */}
                    {tgGuideOpen && (
                      <Box
                        sx={{
                          bgcolor: "#F8FAFC",
                          p: 2.5,
                          borderRadius: 2,
                          border: "1px solid #E2E8F0",
                          display: "flex",
                          flexDirection: "column",
                          gap: 2,
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Bot size={18} color="#0284C7" />
                          <Typography
                            variant="subtitle2"
                            sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.9rem" }}
                          >
                            Concise Setup Guide: How to create and configure your Telegram bot
                          </Typography>
                        </Box>

                        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 1.5 }}>
                          {/* Step 1 */}
                          <Box sx={{ p: 1.5, bgcolor: "#FFFFFF", borderRadius: 1.5, border: "1px solid #E2E8F0" }}>
                            <Typography
                              variant="caption"
                              sx={{ fontWeight: 700, color: "#0284C7", display: "block", mb: 0.5 }}
                            >
                              1. CREATE BOT WITH @BotFather
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: "0.82rem", color: "#475569", lineHeight: 1.4 }}>
                              Open Telegram and search for <b>@BotFather</b>. Send <code>/newbot</code>, follow
                              instructions to set a display name and username (e.g. <code>my_easy42_bot</code>), then
                              copy the API token.
                            </Typography>
                          </Box>

                          {/* Step 2 */}
                          <Box sx={{ p: 1.5, bgcolor: "#FFFFFF", borderRadius: 1.5, border: "1px solid #E2E8F0" }}>
                            <Typography
                              variant="caption"
                              sx={{ fontWeight: 700, color: "#0284C7", display: "block", mb: 0.5 }}
                            >
                              2. FETCH YOUR TELEGRAM USER ID
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: "0.82rem", color: "#475569", lineHeight: 1.4 }}>
                              Message <b>@userinfobot</b> on Telegram. It will instantly reply with your numeric{" "}
                              <code>Id</code> (e.g. <code>123456789</code>). Copy this into the Admin User ID field.
                            </Typography>
                          </Box>

                          {/* Step 3 */}
                          <Box sx={{ p: 1.5, bgcolor: "#FFFFFF", borderRadius: 1.5, border: "1px solid #E2E8F0" }}>
                            <Typography
                              variant="caption"
                              sx={{ fontWeight: 700, color: "#0284C7", display: "block", mb: 0.5 }}
                            >
                              3. START CHAT WITH YOUR BOT
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: "0.82rem", color: "#475569", lineHeight: 1.4 }}>
                              Search for your newly created bot username in Telegram and press <b>Start</b>. Telegram
                              requires you to initiate the conversation before bots can send messages.
                            </Typography>
                          </Box>

                          {/* Step 4 */}
                          <Box sx={{ p: 1.5, bgcolor: "#FFFFFF", borderRadius: 1.5, border: "1px solid #E2E8F0" }}>
                            <Typography
                              variant="caption"
                              sx={{ fontWeight: 700, color: "#0284C7", display: "block", mb: 0.5 }}
                            >
                              4. SAVE & TEST CONNECTION
                            </Typography>
                            <Typography variant="body2" sx={{ fontSize: "0.82rem", color: "#475569", lineHeight: 1.4 }}>
                              Paste the token and user ID above, click <b>Save Settings</b>, then click{" "}
                              <b>Test Connection</b>. You will receive an immediate test notification in your Telegram
                              app!
                            </Typography>
                          </Box>
                        </Box>

                        {/* Supported Bot Commands */}
                        <Box sx={{ mt: 0.5 }}>
                          <Typography
                            variant="caption"
                            sx={{
                              fontWeight: 700,
                              color: "#334155",
                              textTransform: "uppercase",
                              fontSize: "0.75rem",
                              display: "block",
                              mb: 1,
                            }}
                          >
                            Supported Bot Commands:
                          </Typography>
                          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                            <Chip
                              size="small"
                              label="/status"
                              sx={{ fontFamily: "monospace", fontWeight: 700, bgcolor: "#E0F2FE", color: "#0369A1" }}
                            />
                            <Typography variant="caption" sx={{ color: "#64748B", alignSelf: "center", mr: 1.5 }}>
                              Cluster overview, node online/offline counts, bandwidth
                            </Typography>

                            <Chip
                              size="small"
                              label="/nodes"
                              sx={{ fontFamily: "monospace", fontWeight: 700, bgcolor: "#E0F2FE", color: "#0369A1" }}
                            />
                            <Typography variant="caption" sx={{ color: "#64748B", alignSelf: "center", mr: 1.5 }}>
                              Detailed node health, CPU, memory, and uptime
                            </Typography>

                            <Chip
                              size="small"
                              label="/help"
                              sx={{ fontFamily: "monospace", fontWeight: 700, bgcolor: "#F1F5F9", color: "#475569" }}
                            />
                            <Typography variant="caption" sx={{ color: "#64748B", alignSelf: "center" }}>
                              Command usage instructions
                            </Typography>
                          </Box>
                        </Box>
                      </Box>
                    )}
                  </Paper>

                  {/* Section Divider */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 2, my: 1 }}>
                    <Divider sx={{ flex: 1 }} />
                    <Chip
                      size="small"
                      label="Prometheus & Metrics Exporter"
                      sx={{ bgcolor: "#F1F5F9", color: "#475569", fontWeight: 700, fontSize: "0.75rem" }}
                    />
                    <Divider sx={{ flex: 1 }} />
                  </Box>

                  {/* Card 1: Prometheus Metrics Endpoint */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
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
                        <Activity size={18} color="#4F46E5" />
                        <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                          Prometheus Metrics Endpoint
                        </Typography>
                      </Box>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                        <Chip
                          size="small"
                          label="GET"
                          sx={{ bgcolor: "#E0F2FE", color: "#0369A1", fontWeight: 700, fontSize: "0.75rem" }}
                        />
                        <Chip
                          size="small"
                          label="Text 0.0.4"
                          sx={{ bgcolor: "#F1F5F9", color: "#475569", fontWeight: 600, fontSize: "0.75rem" }}
                        />
                        <FormControlLabel
                          control={
                            <Switch
                              checked={integrations.prometheus_enabled}
                              onChange={(e) => handleTogglePrometheus(e.target.checked)}
                              disabled={integrationsSaving}
                              size="small"
                              color="primary"
                            />
                          }
                          label={
                            <Typography
                              variant="body2"
                              sx={{ fontWeight: 600, color: integrations.prometheus_enabled ? "#16A34A" : "#64748B" }}
                            >
                              {integrations.prometheus_enabled ? "Enabled" : "Disabled"}
                            </Typography>
                          }
                          sx={{ m: 0 }}
                        />
                      </Box>
                    </Box>

                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                      easy42 serves live fleet metrics over HTTP. Metrics are rendered from an in-memory state cache for
                      near-instant response times and zero disk overhead.
                    </Typography>

                    <Box sx={{ display: "flex", gap: 1 }}>
                      <TextField
                        fullWidth
                        size="small"
                        value={`${window.location.origin}${integrations.endpoint || "/metrics"}`}
                        InputProps={{
                          readOnly: true,
                          sx: {
                            fontFamily: "monospace",
                            fontSize: "0.85rem",
                            bgcolor: "#F8FAFC",
                            "& fieldset": { borderColor: "#CBD5E1" },
                          },
                        }}
                      />
                      <Button
                        variant="outlined"
                        onClick={() =>
                          handleCopyText(`${window.location.origin}${integrations.endpoint || "/metrics"}`, "endpoint")
                        }
                        startIcon={copiedField === "endpoint" ? <Check size={16} /> : <Copy size={16} />}
                        sx={{
                          textTransform: "none",
                          fontWeight: 600,
                          flexShrink: 0,
                          borderColor: copiedField === "endpoint" ? "#10B981" : "#CBD5E1",
                          color: copiedField === "endpoint" ? "#10B981" : "#334155",
                        }}
                      >
                        {copiedField === "endpoint" ? "Copied" : "Copy URL"}
                      </Button>
                    </Box>
                  </Paper>

                  {/* Card 2: Bearer Authentication & Token Management */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
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
                        <Lock size={18} color="#4F46E5" />
                        <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                          Scraper Authorization Token
                        </Typography>
                      </Box>
                      <Button
                        size="small"
                        variant="outlined"
                        color="warning"
                        startIcon={<RefreshCw size={14} />}
                        onClick={() => setResetTokenConfirmOpen(true)}
                        disabled={resettingToken || integrationsSaving}
                        sx={{ textTransform: "none", fontWeight: 600 }}
                      >
                        Reset Token
                      </Button>
                    </Box>

                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                      The <code>/metrics</code> endpoint requires authorization. External monitoring systems must supply
                      this token either via the HTTP <code>Authorization: Bearer &lt;TOKEN&gt;</code> header or as a{" "}
                      <code>?token=&lt;TOKEN&gt;</code> query parameter.
                    </Typography>

                    <Box sx={{ display: "flex", gap: 1 }}>
                      <TextField
                        fullWidth
                        size="small"
                        type={showAuthToken ? "text" : "password"}
                        value={integrations.prometheus_auth_token || ""}
                        InputProps={{
                          readOnly: true,
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton size="small" onClick={() => setShowAuthToken(!showAuthToken)} edge="end">
                                {showAuthToken ? <EyeOff size={16} /> : <Eye size={16} />}
                              </IconButton>
                            </InputAdornment>
                          ),
                          sx: {
                            fontFamily: "monospace",
                            fontSize: "0.85rem",
                            bgcolor: "#F8FAFC",
                            "& fieldset": { borderColor: "#CBD5E1" },
                          },
                        }}
                      />
                      <Button
                        variant="outlined"
                        onClick={() => handleCopyText(integrations.prometheus_auth_token || "", "token")}
                        startIcon={copiedField === "token" ? <Check size={16} /> : <Copy size={16} />}
                        sx={{
                          textTransform: "none",
                          fontWeight: 600,
                          flexShrink: 0,
                          borderColor: copiedField === "token" ? "#10B981" : "#CBD5E1",
                          color: copiedField === "token" ? "#10B981" : "#334155",
                        }}
                      >
                        {copiedField === "token" ? "Copied" : "Copy Token"}
                      </Button>
                    </Box>
                  </Paper>

                  {/* Card 3: Quick Start & Scraper Setup */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <FileCode size={18} color="#4F46E5" />
                      <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                        Scraper Configuration Example
                      </Typography>
                    </Box>

                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                      Add this job to your <code>prometheus.yml</code> or VictoriaMetrics scrape configuration:
                    </Typography>

                    <Box
                      sx={{
                        position: "relative",
                        bgcolor: "#0F172A",
                        borderRadius: 2,
                        p: 2,
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        color: "#E2E8F0",
                        overflowX: "auto",
                      }}
                    >
                      <Button
                        size="small"
                        onClick={() =>
                          handleCopyText(
                            `scrape_configs:\n  - job_name: 'easy42'\n    scrape_interval: 15s\n    metrics_path: '/metrics'\n    bearer_token: '${integrations.prometheus_auth_token || "YOUR_TOKEN"}'\n    static_configs:\n      - targets: ['${window.location.host}']`,
                            "yaml",
                          )
                        }
                        startIcon={copiedField === "yaml" ? <Check size={14} /> : <Copy size={14} />}
                        sx={{
                          position: "absolute",
                          top: 8,
                          right: 8,
                          textTransform: "none",
                          fontSize: "0.75rem",
                          bgcolor: "rgba(255,255,255,0.1)",
                          color: copiedField === "yaml" ? "#34D399" : "#CBD5E1",
                          "&:hover": { bgcolor: "rgba(255,255,255,0.2)" },
                        }}
                      >
                        {copiedField === "yaml" ? "Copied" : "Copy YAML"}
                      </Button>
                      <pre style={{ margin: 0, whiteSpace: "pre" }}>
                        {`scrape_configs:
  - job_name: 'easy42'
    scrape_interval: 15s
    metrics_path: '/metrics'
    bearer_token: '${integrations.prometheus_auth_token || "YOUR_TOKEN"}'
    static_configs:
      - targets: ['${window.location.host}']`}
                      </pre>
                    </Box>

                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.5 }}>
                      Or test with <code>curl</code> from the command line:
                    </Typography>

                    <Box
                      sx={{
                        position: "relative",
                        bgcolor: "#0F172A",
                        borderRadius: 2,
                        p: 2,
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        color: "#E2E8F0",
                        overflowX: "auto",
                      }}
                    >
                      <Button
                        size="small"
                        onClick={() =>
                          handleCopyText(
                            `curl -H "Authorization: Bearer ${integrations.prometheus_auth_token || "YOUR_TOKEN"}" "${window.location.origin}${integrations.endpoint || "/metrics"}"`,
                            "curl",
                          )
                        }
                        startIcon={copiedField === "curl" ? <Check size={14} /> : <Copy size={14} />}
                        sx={{
                          position: "absolute",
                          top: 8,
                          right: 8,
                          textTransform: "none",
                          fontSize: "0.75rem",
                          bgcolor: "rgba(255,255,255,0.1)",
                          color: copiedField === "curl" ? "#34D399" : "#CBD5E1",
                          "&:hover": { bgcolor: "rgba(255,255,255,0.2)" },
                        }}
                      >
                        {copiedField === "curl" ? "Copied" : "Copy cURL"}
                      </Button>
                      <pre style={{ margin: 0, whiteSpace: "pre" }}>
                        {`curl -H "Authorization: Bearer ${integrations.prometheus_auth_token || "YOUR_TOKEN"}" "${window.location.origin}${integrations.endpoint || "/metrics"}"`}
                      </pre>
                    </Box>
                  </Paper>

                  {/* Card 4: Exposed Metrics & Dashboard Ecosystem Compatibility */}
                  <Paper
                    elevation={0}
                    sx={{
                      p: 2.5,
                      borderRadius: 2.5,
                      border: "1px solid #E2E8F0",
                      bgcolor: "#FFFFFF",
                      display: "flex",
                      flexDirection: "column",
                      gap: 2,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Globe size={18} color="#4F46E5" />
                      <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A" }}>
                        Exposed Metrics & Dashboard Compatibility
                      </Typography>
                    </Box>

                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem" }}>
                      Metric names, labels, and formats follow standard Prometheus conventions, ensuring seamless
                      out-of-the-box compatibility with existing Grafana community dashboards.
                    </Typography>

                    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "1fr 1fr" }, gap: 2 }}>
                      {/* Subcard A: Linux System */}
                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                          <Cpu size={16} color="#0284C7" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            Linux System (node_exporter compatible)
                          </Typography>
                        </Box>
                        <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.8rem", mb: 1 }}>
                          Compatible with standard Node Exporter Grafana dashboards (e.g. Dashboard 1860).
                        </Typography>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {[
                            "node_load1",
                            "node_load5",
                            "node_load15",
                            "node_cpu_percent",
                            "node_memory_MemTotal_bytes",
                            "node_memory_used_bytes",
                            "node_filesystem_*",
                            "node_network_*",
                          ].map((m) => (
                            <Chip
                              key={m}
                              size="small"
                              label={m}
                              sx={{
                                fontFamily: "monospace",
                                fontSize: "0.72rem",
                                bgcolor: "#E0F2FE",
                                color: "#0369A1",
                              }}
                            />
                          ))}
                        </Box>
                      </Box>

                      {/* Subcard B: WireGuard */}
                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                          <Shield size={16} color="#16A34A" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            WireGuard (wireguard_exporter compatible)
                          </Typography>
                        </Box>
                        <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.8rem", mb: 1 }}>
                          Compatible with wireguard_exporter dashboards, enriched with <code>peer_node</code> labels.
                        </Typography>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {[
                            "wireguard_latest_handshake_seconds",
                            "wireguard_latest_handshake_delay_seconds",
                            "wireguard_received_bytes_total",
                            "wireguard_sent_bytes_total",
                            "wireguard_interface_up",
                          ].map((m) => (
                            <Chip
                              key={m}
                              size="small"
                              label={m}
                              sx={{
                                fontFamily: "monospace",
                                fontSize: "0.72rem",
                                bgcolor: "#DCFCE7",
                                color: "#15803D",
                              }}
                            />
                          ))}
                        </Box>
                      </Box>

                      {/* Subcard C: BIRD & BGP */}
                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                          <Zap size={16} color="#D97706" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            BIRD / BGP (bird_exporter compatible)
                          </Typography>
                        </Box>
                        <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.8rem", mb: 1 }}>
                          Tracks protocol states and BGP session health, enriched with <code>peer_node</code> labels.
                        </Typography>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {["bird_protocol_up", "bird_protocol_state", "bird_bgp_session_established"].map((m) => (
                            <Chip
                              key={m}
                              size="small"
                              label={m}
                              sx={{
                                fontFamily: "monospace",
                                fontSize: "0.72rem",
                                bgcolor: "#FEF3C7",
                                color: "#B45309",
                              }}
                            />
                          ))}
                        </Box>
                      </Box>

                      {/* Subcard D: easy42 Fleet */}
                      <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
                          <Activity size={16} color="#7C3AED" />
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            easy42 Fleet Cluster State
                          </Typography>
                        </Box>
                        <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.8rem", mb: 1 }}>
                          Cluster-wide node topology, agent connection status, and link health monitoring.
                        </Typography>
                        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                          {[
                            "easy42_node_up",
                            "easy42_node_info",
                            "easy42_link_up",
                            "easy42_total_nodes",
                            "easy42_connected_nodes",
                            "easy42_scrape_duration_seconds",
                          ].map((m) => (
                            <Chip
                              key={m}
                              size="small"
                              label={m}
                              sx={{
                                fontFamily: "monospace",
                                fontSize: "0.72rem",
                                bgcolor: "#F3E8FF",
                                color: "#6D28D9",
                              }}
                            />
                          ))}
                        </Box>
                      </Box>
                    </Box>
                  </Paper>
                </Box>
              ) : null}
            </DialogContent>
          </Box>
        )}

        {/* Reset Token Confirmation Dialog */}
        <Dialog
          open={resetTokenConfirmOpen}
          onClose={() => !resettingToken && setResetTokenConfirmOpen(false)}
          maxWidth="xs"
          fullWidth
          PaperProps={{ sx: { borderRadius: 3, p: 1 } }}
        >
          <DialogTitle sx={{ fontWeight: 700, display: "flex", alignItems: "center", gap: 1, color: "#0F172A" }}>
            <AlertCircle size={22} color="#D97706" />
            Reset Prometheus Auth Token?
          </DialogTitle>
          <DialogContent>
            <Typography variant="body2" sx={{ color: "#475569", lineHeight: 1.5 }}>
              Are you sure you want to regenerate the Prometheus auth token?
            </Typography>
            <Alert severity="warning" sx={{ mt: 2, borderRadius: 2 }}>
              <Typography variant="caption" sx={{ fontWeight: 600, display: "block" }}>
                Important notice:
              </Typography>
              Existing scrapers or external monitoring systems using the current token will immediately receive HTTP 401
              Unauthorized errors until you update them with the newly generated token.
            </Alert>
          </DialogContent>
          <DialogActions sx={{ px: 3, pb: 2, pt: 1 }}>
            <Button
              onClick={() => setResetTokenConfirmOpen(false)}
              disabled={resettingToken}
              sx={{ textTransform: "none", color: "#64748B", fontWeight: 600 }}
            >
              Cancel
            </Button>
            <Button
              variant="contained"
              color="warning"
              onClick={handleResetToken}
              disabled={resettingToken}
              startIcon={<RefreshCw size={14} className={resettingToken ? "spin" : ""} />}
              sx={{ textTransform: "none", fontWeight: 700, borderRadius: 2 }}
            >
              {resettingToken ? "Resetting..." : "Yes, Reset Token"}
            </Button>
          </DialogActions>
        </Dialog>

        {activeTab === "about" && (
          <Box sx={{ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, overflow: "hidden" }}>
            <DialogContent
              sx={{
                display: "flex",
                flexDirection: "column",
                gap: 2.5,
                py: 3,
                px: 3,
                flex: 1,
                minHeight: 0,
                overflowY: "auto",
              }}
            >
              {/* Header Description & Logo Card */}
              <Box
                sx={{
                  p: 2.5,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 2,
                  flexWrap: { xs: "wrap", sm: "nowrap" },
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
                  <Easy42Logo
                    size={48}
                    sx={{
                      borderRadius: "12px",
                      boxShadow: "0 4px 14px rgba(79, 70, 229, 0.25)",
                      flexShrink: 0,
                    }}
                  />
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", letterSpacing: "-0.3px" }}>
                        easy<span style={{ color: "#0891B2" }}>42</span>
                      </Typography>
                      <Chip
                        label={versionInfo?.version || "v0.1"}
                        size="small"
                        sx={{
                          height: 22,
                          fontSize: "0.75rem",
                          fontWeight: 700,
                          bgcolor: "rgba(79, 70, 229, 0.08)",
                          color: "#4F46E5",
                          border: "1px solid rgba(79, 70, 229, 0.2)",
                        }}
                      />
                    </Box>
                    <Typography variant="body2" sx={{ color: "#64748B", fontSize: "0.85rem", mt: 0.2 }}>
                      WireGuard Mesh Overlay Network Manager for DN42 and Private Networks
                    </Typography>
                  </Box>
                </Box>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<RefreshCw size={14} className={versionLoading ? "spin" : ""} />}
                  onClick={loadVersionInfo}
                  disabled={versionLoading}
                  sx={{
                    textTransform: "none",
                    fontWeight: 600,
                    flexShrink: 0,
                    borderColor: "#CBD5E1",
                    color: "#475569",
                    "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
                  }}
                >
                  Refresh
                </Button>
              </Box>

              {versionError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {versionError}
                </Alert>
              )}

              {/* Section 1: easy42 Go Controller Server */}
              <Paper
                elevation={0}
                sx={{
                  p: 2.5,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Cpu size={18} color="#4F46E5" />
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                    easy42 Go Server
                  </Typography>
                </Box>
                <Typography variant="caption" sx={{ color: "#64748B", mt: -1 }}>
                  Backend controller daemon responsible for mesh state synchronization, WireGuard endpoints, and BGP
                  templates.
                </Typography>

                <Box
                  sx={{
                    display: "grid",
                    gridTemplateColumns: { xs: "1fr", sm: "repeat(2, 1fr)", md: "repeat(3, 1fr)" },
                    gap: 1.5,
                  }}
                >
                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Typography
                      variant="caption"
                      sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                    >
                      Program Version
                    </Typography>
                    <Typography variant="body1" sx={{ fontWeight: 700, color: "#0F172A", mt: 0.3 }}>
                      {versionInfo?.version || "dev"}
                    </Typography>
                  </Box>

                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                      <GitCommit size={12} color="#64748B" />
                      <Typography
                        variant="caption"
                        sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                      >
                        Git Commit
                      </Typography>
                    </Box>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mt: 0.3 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: "monospace",
                          fontWeight: 600,
                          color: "#334155",
                          bgcolor: "#E2E8F0",
                          px: 0.8,
                          py: 0.2,
                          borderRadius: 1,
                          fontSize: "0.8rem",
                        }}
                      >
                        {versionInfo?.commit && versionInfo.commit !== "none"
                          ? versionInfo.commit.substring(0, 10)
                          : "none"}
                      </Typography>
                      {versionInfo?.commit && versionInfo.commit !== "none" && (
                        <Tooltip title="Copy full commit SHA">
                          <IconButton
                            size="small"
                            onClick={() => {
                              navigator.clipboard.writeText(versionInfo.commit || "");
                            }}
                            sx={{ p: 0.5, color: "#64748B" }}
                          >
                            <Copy size={14} />
                          </IconButton>
                        </Tooltip>
                      )}
                    </Box>
                  </Box>

                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Typography
                      variant="caption"
                      sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                    >
                      Build Date
                    </Typography>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", mt: 0.3 }}>
                      {versionInfo?.date || "unknown"}
                    </Typography>
                  </Box>

                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Typography
                      variant="caption"
                      sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                    >
                      Go Runtime
                    </Typography>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", mt: 0.3 }}>
                      {versionInfo?.go_version || "unknown"}
                    </Typography>
                  </Box>

                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Typography
                      variant="caption"
                      sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                    >
                      Platform / Arch
                    </Typography>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", mt: 0.3 }}>
                      {versionInfo?.platform || "unknown"}
                    </Typography>
                  </Box>

                  <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                    <Typography
                      variant="caption"
                      sx={{ color: "#64748B", fontWeight: 600, textTransform: "uppercase" }}
                    >
                      Built By
                    </Typography>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", mt: 0.3 }}>
                      {versionInfo?.built_by || "unknown"}
                    </Typography>
                  </Box>
                </Box>
              </Paper>

              {/* Section 2: easy42 Bundled Agent */}
              <Paper
                elevation={0}
                sx={{
                  p: 2.5,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Zap size={18} color="#0891B2" />
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      Bundled easy42-agent
                    </Typography>
                  </Box>
                  <Chip
                    label={`v${versionInfo?.agent_version || "0.1.0"}`}
                    size="small"
                    sx={{
                      height: 22,
                      fontSize: "0.75rem",
                      fontWeight: 700,
                      bgcolor: "rgba(8, 145, 178, 0.08)",
                      color: "#0891B2",
                      border: "1px solid rgba(8, 145, 178, 0.2)",
                    }}
                  />
                </Box>
                <Typography variant="caption" sx={{ color: "#64748B", mt: -1 }}>
                  High-performance Rust-based node daemon compiled statically with musl libc for zero runtime
                  dependencies.
                </Typography>

                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                  <Typography variant="caption" sx={{ fontWeight: 600, color: "#475569" }}>
                    Download Precompiled Binaries:
                  </Typography>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                    {[
                      { arch: "x86_64", label: "x86_64 (AMD64 Linux)" },
                      { arch: "aarch64", label: "aarch64 (ARM64 Linux)" },
                      { arch: "armv7", label: "armv7 (ARMv7 Linux)" },
                      { arch: "mipsel", label: "mipsel (MIPS LE / OpenWrt)" },
                    ].map((item) => (
                      <Button
                        key={item.arch}
                        component="a"
                        href={`/api/agent/bin/${item.arch}`}
                        download={`easy42-agent-${item.arch}`}
                        size="small"
                        variant="outlined"
                        sx={{
                          textTransform: "none",
                          fontSize: "0.8rem",
                          fontWeight: 600,
                          borderColor: "#E2E8F0",
                          color: "#334155",
                          bgcolor: "#F8FAFC",
                          "&:hover": { bgcolor: "#EEF2FF", borderColor: "#4F46E5", color: "#4F46E5" },
                        }}
                      >
                        {item.label}
                      </Button>
                    ))}
                  </Box>

                  {/* One-line Install Script helper */}
                  <Box sx={{ mt: 1, p: 1.5, borderRadius: 1.5, bgcolor: "#F1F5F9", border: "1px solid #E2E8F0" }}>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block", mb: 0.5 }}>
                      Automated Node Installation Script:
                    </Typography>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
                      <Typography
                        variant="caption"
                        sx={{
                          fontFamily: "monospace",
                          color: "#1E293B",
                          wordBreak: "break-all",
                          fontWeight: 500,
                        }}
                      >
                        curl -sSL {window.location.origin}/api/agent/install.sh | sh
                      </Typography>
                      <Tooltip title="Copy command">
                        <IconButton
                          size="small"
                          onClick={() => {
                            navigator.clipboard.writeText(
                              `curl -sSL ${window.location.origin}/api/agent/install.sh | sh`,
                            );
                          }}
                          sx={{ p: 0.5, color: "#64748B" }}
                        >
                          <Copy size={14} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </Box>
                </Box>
              </Paper>

              {/* Section 3: Project & GitHub Repository */}
              <Paper
                elevation={0}
                sx={{
                  p: 2.5,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
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
                    <Globe size={18} color="#4F46E5" />
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      Project & Source Code
                    </Typography>
                  </Box>
                  <Button
                    component="a"
                    href={versionInfo?.github_url || "https://github.com/sagan/easy42"}
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="contained"
                    size="small"
                    endIcon={<ExternalLink size={14} />}
                    sx={{
                      textTransform: "none",
                      fontWeight: 700,
                      bgcolor: "#4F46E5",
                      "&:hover": { bgcolor: "#4338CA" },
                    }}
                  >
                    GitHub: sagan/easy42
                  </Button>
                </Box>

                <Box
                  sx={{
                    p: 1.5,
                    borderRadius: 1.5,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: 1,
                  }}
                >
                  <Box>
                    <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                      Official Repository URL
                    </Typography>
                    <Typography
                      component="a"
                      href={versionInfo?.github_url || "https://github.com/sagan/easy42"}
                      target="_blank"
                      rel="noopener noreferrer"
                      variant="body2"
                      sx={{
                        color: "#4F46E5",
                        fontWeight: 600,
                        textDecoration: "none",
                        display: "block",
                        "&:hover": { textDecoration: "underline" },
                      }}
                    >
                      {versionInfo?.github_url || "https://github.com/sagan/easy42"}
                    </Typography>
                  </Box>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={versionCopied ? <Check size={14} color="#10B981" /> : <Copy size={14} />}
                    onClick={() => {
                      const text = JSON.stringify(versionInfo, null, 2);
                      navigator.clipboard.writeText(text);
                      setVersionCopied(true);
                      setTimeout(() => setVersionCopied(false), 2000);
                    }}
                    sx={{
                      textTransform: "none",
                      fontWeight: 600,
                      borderColor: "#CBD5E1",
                      color: versionCopied ? "#10B981" : "#475569",
                      "&:hover": { borderColor: "#94A3B8", bgcolor: "#F1F5F9" },
                    }}
                  >
                    {versionCopied ? "Copied Info!" : "Copy Diagnostics"}
                  </Button>
                </Box>
              </Paper>

              {/* Section 4: Progressive Web App & Service Worker Cache */}
              <Paper
                elevation={0}
                sx={{
                  p: 2.5,
                  borderRadius: 2,
                  border: "1px solid #E2E8F0",
                  display: "flex",
                  flexDirection: "column",
                  gap: 2,
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
                    <HardDrive size={18} color="#0891B2" />
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A", fontSize: "0.95rem" }}>
                      PWA & Service Worker Cache
                    </Typography>
                  </Box>
                  <Button
                    size="small"
                    variant="outlined"
                    color="error"
                    disabled={swFlushLoading}
                    startIcon={swFlushLoading ? <CircularProgress size={14} color="inherit" /> : <Trash2 size={14} />}
                    onClick={handleFlushCacheAndSw}
                    sx={{
                      textTransform: "none",
                      fontWeight: 700,
                    }}
                  >
                    {swFlushLoading ? "Flushing..." : "Flush cache & service worker"}
                  </Button>
                </Box>

                <Typography variant="caption" sx={{ color: "#64748B", mt: -1 }}>
                  Clears local browser CacheStorage assets and unregisters active service workers to ensure fresh
                  bundles are pulled from the server.
                </Typography>

                {swFlushResult && (
                  <Alert severity={swFlushResult.severity} sx={{ borderRadius: 1.5, py: 0.5, fontSize: "0.8rem" }}>
                    {swFlushResult.text}
                  </Alert>
                )}
              </Paper>
            </DialogContent>
          </Box>
        )}

        {/* Sub-dialog: Policy Editor & Viewer */}
        <Dialog open={policyDialogOpen} onClose={() => setPolicyDialogOpen(false)} maxWidth="md" fullWidth>
          <form onSubmit={handleSavePolicy}>
            <DialogTitle
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                pb: 1,
                borderBottom: "1px solid #E2E8F0",
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Shield size={20} color="#4F46E5" />
                <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem", color: "#0F172A" }}>
                  {policyDialogMode === "view"
                    ? "View Network Policy"
                    : policyDialogMode === "edit"
                      ? "Edit Network Policy"
                      : "Create Network Policy"}
                </Typography>
              </Box>
              <IconButton size="small" onClick={() => setPolicyDialogOpen(false)} sx={{ color: "#94A3B8" }}>
                <X size={18} />
              </IconButton>
            </DialogTitle>

            <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, py: 2.5, px: 3 }}>
              {policyDialogMode === "view" && (
                <Alert icon={<Lock size={16} />} severity="info" sx={{ py: 0.5, borderRadius: 2 }}>
                  Built-in policies are managed by easy42 and are read-only.
                </Alert>
              )}

              {policyDialogError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {policyDialogError}
                </Alert>
              )}

              <Box sx={{ display: "flex", gap: 2 }}>
                <TextField
                  fullWidth
                  size="small"
                  label="Policy ID"
                  placeholder="e.g. partner-lan"
                  value={policyId}
                  onChange={(e) => setPolicyId(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ""))}
                  disabled={policyDialogMode !== "create" || policyDialogSaving}
                  helperText="Unique identifier (letters, numbers, hyphens)"
                  required
                />
                <TextField
                  fullWidth
                  size="small"
                  label="Policy Name"
                  placeholder="e.g. Partner LAN Policy"
                  value={policyName}
                  onChange={(e) => setPolicyName(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  required
                />
              </Box>

              <TextField
                fullWidth
                size="small"
                label="Description (optional)"
                placeholder="e.g. Limits access to specific subnets and enables anti-spoofing"
                value={policyDesc}
                onChange={(e) => setPolicyDesc(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
              />

              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="Internal BGP Cost"
                  placeholder="100"
                  value={policyCost}
                  onChange={(e) => setPolicyCost(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="Cost deducted from local preference on each internal hop (default 100)."
                />
                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Routing Policy"
                  value={policyRoutingPolicy}
                  onChange={(e) => setPolicyRoutingPolicy(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="BGP route advertising & receiving behavior."
                >
                  <MenuItem value="full">Full (Default) — Advertise & receive all valid routes</MenuItem>
                  <MenuItem value="stub">Stub — Receive all, only send local routes (no transit)</MenuItem>
                  <MenuItem value="receive_only">Receive Only — Import routes only, advertise none</MenuItem>
                  <MenuItem value="advertise_only">Advertise Only — Export local routes only, import none</MenuItem>
                </TextField>
              </Box>

              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 2 }}>
                <TextField
                  fullWidth
                  size="small"
                  label="WireGuard FwMark (optional)"
                  placeholder="e.g. 51820 or 0xca64"
                  value={policyFwmark}
                  onChange={(e) => setPolicyFwmark(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="FwMark set in local WireGuard [Interface]."
                />
                <TextField
                  fullWidth
                  size="small"
                  type="number"
                  label="BGP Preference (optional)"
                  placeholder="e.g. 100"
                  value={policyPreference}
                  onChange={(e) => setPolicyPreference(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="BGP protocol preference in bird.conf."
                />
                <TextField
                  fullWidth
                  size="small"
                  label="Netfilter Mark (optional)"
                  placeholder="e.g. 42 or 0x1234"
                  value={policyMark}
                  onChange={(e) => setPolicyMark(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="Rewrite mark of received packets via nftables."
                />
              </Box>

              <TextField
                fullWidth
                size="small"
                label="Ingress Allowed Destination Subnets (CIDRs)"
                placeholder={"e.g. 172.20.0.0/14{21,29}\nfd00::/8{44,64}"}
                multiline
                rows={3}
                value={policyAllowedDst}
                onChange={(e) => setPolicyAllowedDst(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
                helperText="Restricts traffic dst IP via nftables (return traffic allowed) & filters BGP export routes. Leave empty for unrestricted."
              />

              <TextField
                fullWidth
                size="small"
                label="Ingress Allowed Source Subnets (CIDRs)"
                placeholder={"e.g. 172.20.10.0/24\nfd00:1::/48"}
                multiline
                rows={3}
                value={policyAllowedSrc}
                onChange={(e) => setPolicyAllowedSrc(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
                helperText="Anti-spoofing & BGP route import filter: drops inbound traffic with source IP not matching, and BGP import only permits routes matching these subnets. Leave empty for unrestricted."
              />

              <TextField
                fullWidth
                size="small"
                label="Ingress Disallowed Destination Subnets (CIDRs)"
                placeholder={"e.g. fd42:1234:5678::/48\n172.20.99.0/24"}
                multiline
                rows={3}
                value={policyDisallowedDst}
                onChange={(e) => setPolicyDisallowedDst(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
                helperText="Reverse of Allowed Dst: drops inbound traffic to these subnets, blocks BGP export, and triggers SNAT (under not_dst) for egress traffic originating from these subnets (e.g. private LAN subnets carved out of DN42 ULA fd00::/8)."
              />

              <TextField
                fullWidth
                size="small"
                label="Ingress Disallowed Source Subnets (CIDRs)"
                placeholder={"e.g. 172.20.99.0/24\nfd42:1234:5678::/48"}
                multiline
                rows={3}
                value={policyDisallowedSrc}
                onChange={(e) => setPolicyDisallowedSrc(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
                helperText="Reverse of Allowed Src: inbound traffic on this link matching these source subnets will be dropped, and blocks matching BGP import routes. Leave empty for unrestricted."
              />

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Typography
                  variant="subtitle2"
                  sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
                >
                  <Sliders size={16} /> Route Leak Protection
                </Typography>
                <FormControlLabel
                  control={
                    <Switch
                      checked={policyRejectInternet}
                      onChange={(e) => setPolicyRejectInternet(e.target.checked)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        Reject Internet Routes
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        Prevents importing and exporting default / Internet routes (0.0.0.0/0, 128.0.0.0/1, ::/0).
                      </Typography>
                    </Box>
                  }
                />
              </Box>

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Typography
                  variant="subtitle2"
                  sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
                >
                  <Shield size={16} /> Route Origination Authorization (ROA)
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Validates route origin ASNs in BGP import filters using BIRD ROA tables. Accepts a URL or local server
                  file path.
                </Typography>

                <TextField
                  fullWidth
                  size="small"
                  label="IPv4 ROA File / URL"
                  placeholder="https://dn42.burble.com/roa/dn42_roa_bird2_4.conf"
                  value={policyRoa4}
                  onChange={(e) => setPolicyRoa4(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="URL or local path to IPv4 BIRD ROA definitions."
                />

                <TextField
                  fullWidth
                  size="small"
                  label="IPv6 ROA File / URL"
                  placeholder="https://dn42.burble.com/roa/dn42_roa_bird2_6.conf"
                  value={policyRoa6}
                  onChange={(e) => setPolicyRoa6(e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="URL or local path to IPv6 BIRD ROA definitions."
                />

                <FormControlLabel
                  control={
                    <Switch
                      checked={policyRoaStrict}
                      onChange={(e) => setPolicyRoaStrict(e.target.checked)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        Strict ROA Validation
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        When enabled, routes with ROA_UNKNOWN status are rejected. (Default: disabled, permits VALID and
                        UNKNOWN, rejects INVALID).
                      </Typography>
                    </Box>
                  }
                />
              </Box>

              {/* Traffic Filtering & Host Firewall */}
              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Typography
                  variant="subtitle2"
                  sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
                >
                  <Shield size={16} /> Traffic Filtering & Host Firewall
                </Typography>

                <FormControlLabel
                  control={
                    <Switch
                      checked={policyFilterForward}
                      onChange={(e) => setPolicyFilterForward(e.target.checked)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        Filter Transit / Forwarded Traffic
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        Drops forwarded packets whose source or destination IP does not match the configured allowed
                        CIDRs.
                      </Typography>
                    </Box>
                  }
                />

                <Divider sx={{ my: 0.5 }} />

                <FormControlLabel
                  control={
                    <Switch
                      checked={policyFilterInput}
                      onChange={(e) => setPolicyFilterInput(e.target.checked)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                        Filter Input Traffic (Host Firewall)
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B" }}>
                        Restricts incoming traffic targeting the host itself from this link. BGP (TCP port 179) and
                        established return traffic are always permitted.
                      </Typography>
                    </Box>
                  }
                />

                {policyFilterInput && (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, pl: 4, pt: 0.5 }}>
                    <Box sx={{ display: "flex", gap: 3 }}>
                      <FormControlLabel
                        control={
                          <Checkbox
                            size="small"
                            checked={policyInputAllowIcmp}
                            onChange={(e) => setPolicyInputAllowIcmp(e.target.checked)}
                            disabled={policyDialogMode === "view" || policyDialogSaving}
                          />
                        }
                        label={
                          <Typography variant="body2" sx={{ fontSize: "0.85rem", color: "#334155" }}>
                            Allow ICMP (IPv4 Ping)
                          </Typography>
                        }
                      />
                      <FormControlLabel
                        control={
                          <Checkbox
                            size="small"
                            checked={policyInputAllowIcmp6}
                            onChange={(e) => setPolicyInputAllowIcmp6(e.target.checked)}
                            disabled={policyDialogMode === "view" || policyDialogSaving}
                          />
                        }
                        label={
                          <Typography variant="body2" sx={{ fontSize: "0.85rem", color: "#334155" }}>
                            Allow ICMPv6 (IPv6 Ping & NDP)
                          </Typography>
                        }
                      />
                    </Box>

                    <TextField
                      fullWidth
                      size="small"
                      label="Allowed TCP Ports"
                      placeholder="e.g. 80, 443, 8000-8080 (or all)"
                      value={policyInputTcpPorts}
                      onChange={(e) => setPolicyInputTcpPorts(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                      helperText="Comma or newline separated list of ports (1-65535) or ranges (e.g. 8000-8080). BGP 179 is always permitted."
                    />

                    <TextField
                      fullWidth
                      size="small"
                      label="Allowed UDP Ports"
                      placeholder="e.g. 53, 5000-5010 (or all)"
                      value={policyInputUdpPorts}
                      onChange={(e) => setPolicyInputUdpPorts(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                      helperText="Comma or newline separated list of ports (1-65535) or ranges. Defaults to 53 for DN42 DNS."
                    />

                    <TextField
                      fullWidth
                      size="small"
                      multiline
                      rows={2}
                      label="Protected Local Networks"
                      placeholder="e.g. 172.20.229.0/27, fd00:dead:beef::/48"
                      value={policyLocalNetworks}
                      onChange={(e) => setPolicyLocalNetworks(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                      helperText="Subnets treated as local. Forwarded traffic entering from this link destined to these subnets will be subjected to the input filter port rules."
                    />
                  </Box>
                )}

                <Divider sx={{ my: 0.5 }} />

                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Block Ingress New Connections"
                  value={policyBlockIngressNew || "_"}
                  onChange={(e) => setPolicyBlockIngressNew(e.target.value === "_" ? "" : e.target.value)}
                  disabled={policyDialogMode === "view" || policyDialogSaving}
                  helperText="Stateful ingress firewall: drop incoming packets from link peer if conntrack state is new or invalid."
                >
                  <MenuItem value="_">Disabled (Default) — Do not block any</MenuItem>
                  <MenuItem value="all">Block Ingress New — Drop incoming new or invalid packets from peer</MenuItem>
                  <MenuItem value="forward">
                    Block Ingress New for Forwarding — Drop incoming new or invalid packets if destination is not local
                    (fib daddr type != local)
                  </MenuItem>
                </TextField>

                {Boolean(policyBlockIngressNew && policyBlockIngressNew !== "_") && (
                  <TextField
                    label="Ingress New Whitelist (Allowed Destinations)"
                    multiline
                    rows={3}
                    fullWidth
                    size="small"
                    placeholder={"192.168.1.1:443\n192.168.1.1:443/tcp\n10.0.0.1:80/udp"}
                    value={policyIngressNewWhitelist}
                    onChange={(e) => setPolicyIngressNewWhitelist(e.target.value)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                    helperText="Exceptions to block ingress new: list of allowed dst addresses and ports (e.g. 192.168.1.1:443 or 192.168.1.1:443/tcp). Omitting protocol implies TCP+UDP."
                  />
                )}
              </Box>

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                    Outbound SNAT / Masquerade
                  </Typography>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={policySnatEnabled}
                        onChange={(e) => setPolicySnatEnabled(e.target.checked)}
                        disabled={policyDialogMode === "view" || policyDialogSaving}
                      />
                    }
                    label={policySnatEnabled ? "Enabled" : "Disabled"}
                    sx={{ m: 0 }}
                  />
                </Box>

                {policySnatEnabled && (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, pt: 1 }}>
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="SNAT Condition"
                      value={policySnatCondition}
                      onChange={(e) => setPolicySnatCondition(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    >
                      <MenuItem value="not_dst">Traffic whose source IP is NOT in allowed destination CIDRs</MenuItem>
                      <MenuItem value="all">All egress traffic on this link</MenuItem>
                    </TextField>

                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="SNAT Target IP"
                      value={policySnatTarget}
                      onChange={(e) => setPolicySnatTarget(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    >
                      <MenuItem value="masquerade">Interface IP (Masquerade)</MenuItem>
                      <MenuItem value="external_ip">External / Peering IP</MenuItem>
                      <MenuItem value="main_ip">Main IP</MenuItem>
                    </TextField>
                  </Box>
                )}
              </Box>

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                      Forward SNAT / Masquerade
                    </Typography>
                    <Typography variant="caption" sx={{ color: "#64748B" }}>
                      SNAT / masquerade when forwarding received traffic from link peer to any other interface.
                    </Typography>
                  </Box>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={policyForwardSnat}
                        onChange={(e) => setPolicyForwardSnat(e.target.checked)}
                        disabled={policyDialogMode === "view" || policyDialogSaving}
                      />
                    }
                    label={policyForwardSnat ? "Enabled" : "Disabled"}
                    sx={{ m: 0 }}
                  />
                </Box>

                {policyForwardSnat && (
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, pt: 1 }}>
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="SNAT Target IP"
                      value={policyForwardSnatTarget}
                      onChange={(e) => setPolicyForwardSnatTarget(e.target.value)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    >
                      <MenuItem value="masquerade">Interface IP (Masquerade)</MenuItem>
                      <MenuItem value="external_ip">External / Peering IP</MenuItem>
                      <MenuItem value="main_ip">Main IP</MenuItem>
                    </TextField>
                  </Box>
                )}
              </Box>

              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Typography
                  variant="subtitle2"
                  sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
                >
                  <Shield size={16} /> DSCP Marking
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Set / rewrite the Differentiated Services Code Point (DSCP) on packets traversing this link. Valid
                  values: 0–63. Leave empty to disable.
                </Typography>

                <Box sx={{ display: "flex", gap: 2 }}>
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label="Ingress DSCP"
                    placeholder="e.g. 46 (EF)"
                    value={policyDscpIngress}
                    onChange={(e) =>
                      setPolicyDscpIngress(
                        e.target.value === "" ? "" : Math.max(0, Math.min(63, Number(e.target.value))),
                      )
                    }
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                    helperText="Rewrite DSCP on packets received from the link peer."
                    inputProps={{ min: 0, max: 63 }}
                  />
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label="Egress DSCP"
                    placeholder="e.g. 46 (EF)"
                    value={policyDscpEgress}
                    onChange={(e) =>
                      setPolicyDscpEgress(
                        e.target.value === "" ? "" : Math.max(0, Math.min(63, Number(e.target.value))),
                      )
                    }
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                    helperText="Rewrite DSCP on packets sent to the link peer."
                    inputProps={{ min: 0, max: 63 }}
                  />
                </Box>
              </Box>

              {/* Custom BIRD Route Filters */}
              <Box
                sx={{
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                  p: 2,
                  borderRadius: 2,
                  bgcolor: "#F8FAFC",
                  border: "1px solid #E2E8F0",
                }}
              >
                <Box>
                  <Typography
                    variant="subtitle2"
                    sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
                  >
                    <FileCode size={16} /> Custom BIRD Route Filters
                  </Typography>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Append custom BIRD routing daemon configuration snippets to the peer's IPv4 &amp; IPv6 import and
                    export filter blocks (e.g. matching/adding/stripping BGP communities, modifying bgp_local_pref, or
                    AS-path prepending).
                  </Typography>
                </Box>

                {/* Import Filter */}
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      flexWrap: "wrap",
                      gap: 1,
                    }}
                  >
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155" }}>
                      BIRD Import Filter Snippet
                    </Typography>
                    {policyDialogMode !== "view" && (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, mr: 0.5 }}>
                          Add example:
                        </Typography>
                        {IMPORT_FILTER_EXAMPLES.map((ex) => (
                          <Tooltip key={ex.label} title={ex.desc}>
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => handleAppendImportExample(ex.snippet)}
                              disabled={policyDialogSaving}
                              sx={{
                                textTransform: "none",
                                fontSize: "0.75rem",
                                py: 0.2,
                                px: 1,
                                height: 24,
                                borderColor: "#CBD5E1",
                                color: "#475569",
                                bgcolor: "#FFFFFF",
                                "&:hover": { bgcolor: "#F1F5F9", borderColor: "#94A3B8" },
                              }}
                            >
                              + {ex.label}
                            </Button>
                          </Tooltip>
                        ))}
                      </Box>
                    )}
                  </Box>
                  <TextField
                    fullWidth
                    size="small"
                    multiline
                    rows={4}
                    placeholder={`# Snippet appended to peer's IPv4 & IPv6 import filter block end\n# e.g.\nif (64512, 100) ~ bgp_community then bgp_local_pref = 200;\nbgp_large_community.add((4242421234, 1, 1));`}
                    value={policyBirdImportFilter}
                    onChange={(e) => setPolicyBirdImportFilter(e.target.value)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                    InputProps={{
                      sx: {
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        bgcolor: "#FFFFFF",
                      },
                    }}
                    helperText="Executed before accepting valid routes on import. Can adjust bgp_local_pref, add/remove BGP communities, or reject matching routes."
                  />
                </Box>

                <Divider sx={{ my: 0.5 }} />

                {/* Export Filter */}
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      flexWrap: "wrap",
                      gap: 1,
                    }}
                  >
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155" }}>
                      BIRD Export Filter Snippet
                    </Typography>
                    {policyDialogMode !== "view" && (
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, mr: 0.5 }}>
                          Add example:
                        </Typography>
                        {EXPORT_FILTER_EXAMPLES.map((ex) => (
                          <Tooltip key={ex.label} title={ex.desc}>
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => handleAppendExportExample(ex.snippet)}
                              disabled={policyDialogSaving}
                              sx={{
                                textTransform: "none",
                                fontSize: "0.75rem",
                                py: 0.2,
                                px: 1,
                                height: 24,
                                borderColor: "#CBD5E1",
                                color: "#475569",
                                bgcolor: "#FFFFFF",
                                "&:hover": { bgcolor: "#F1F5F9", borderColor: "#94A3B8" },
                              }}
                            >
                              + {ex.label}
                            </Button>
                          </Tooltip>
                        ))}
                      </Box>
                    )}
                  </Box>
                  <TextField
                    fullWidth
                    size="small"
                    multiline
                    rows={4}
                    placeholder={`# Snippet appended to peer's IPv4 & IPv6 export filter block end\n# e.g.\nbgp_community.delete([(64512, *)]);\nbgp_large_community.add((4242421234, 2, 1));`}
                    value={policyBirdExportFilter}
                    onChange={(e) => setPolicyBirdExportFilter(e.target.value)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                    InputProps={{
                      sx: {
                        fontFamily: "monospace",
                        fontSize: "0.82rem",
                        bgcolor: "#FFFFFF",
                      },
                    }}
                    helperText="Executed before exporting routes to the peer. Can strip private communities, add export tags, or prepend AS path."
                  />
                </Box>
              </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
              <Button onClick={() => setPolicyDialogOpen(false)} sx={{ color: "#64748B" }}>
                {policyDialogMode === "view" ? "Close" : "Cancel"}
              </Button>
              {policyDialogMode !== "view" && (
                <Button
                  type="submit"
                  variant="contained"
                  disabled={policyDialogSaving}
                  startIcon={policyDialogSaving && <CircularProgress size={16} color="inherit" />}
                  sx={{ fontWeight: 600, bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                >
                  {policyDialogSaving ? "Saving..." : policyDialogMode === "create" ? "Create Policy" : "Save Changes"}
                </Button>
              )}
            </DialogActions>
          </form>
        </Dialog>

        {/* Standalone View Network Policy Modal */}
        <ViewNetworkPolicyModal
          open={viewPolicyModalOpen}
          onClose={() => setViewPolicyModalOpen(false)}
          policy={policyToView}
          onEdit={(p) => handleOpenEditPolicy(p)}
        />

        {/* Template Create / Edit / View Dialog */}
        <Dialog open={templateDialogOpen} onClose={() => setTemplateDialogOpen(false)} maxWidth="md" fullWidth>
          <form onSubmit={handleSaveTemplate}>
            <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pb: 1.5 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <FileCode size={20} color="#4F46E5" />
                <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem" }}>
                  {templateDialogMode === "view"
                    ? `View Template: ${templateName}`
                    : templateDialogMode === "create"
                      ? "Create Config Template"
                      : `Edit Template: ${templateName}`}
                </Typography>
              </Box>
              <IconButton size="small" onClick={() => setTemplateDialogOpen(false)}>
                <X size={18} />
              </IconButton>
            </DialogTitle>

            <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, py: 2 }}>
              {templateDialogError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {templateDialogError}
                </Alert>
              )}

              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                <TextField
                  select
                  label="Template Type"
                  size="small"
                  value={templateType}
                  onChange={(e) => setTemplateType(e.target.value as "wg" | "bird" | "nft")}
                  disabled={templateDialogMode === "view" || templateDialogSaving}
                  helperText={
                    templateType === "wg"
                      ? "WireGuard: /etc/wireguard/<iface>.conf"
                      : templateType === "bird"
                        ? "BIRD 2: /etc/bird/bird.conf"
                        : "nftables: /etc/nftables.conf"
                  }
                >
                  <MenuItem value="wg">WireGuard (wg)</MenuItem>
                  <MenuItem value="bird">BIRD 2 (bird)</MenuItem>
                  <MenuItem value="nft">nftables (nft)</MenuItem>
                </TextField>

                <TextField
                  label="Template ID"
                  size="small"
                  value={templateId}
                  onChange={(e) => setTemplateId(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ""))}
                  disabled={templateDialogMode !== "create" || templateDialogSaving}
                  placeholder="e.g. my-bird-template"
                  helperText={
                    templateDialogMode === "create"
                      ? "Unique ID (lowercase letters, numbers, hyphens)"
                      : "Immutable template ID"
                  }
                />
              </Box>

              <TextField
                label="Template Name"
                size="small"
                value={templateName}
                onChange={(e) => setTemplateName(e.target.value)}
                disabled={templateDialogMode === "view" || templateDialogSaving}
                placeholder="e.g. Custom BIRD with Internal OSPF"
                fullWidth
              />

              <TextField
                label="Description (optional)"
                size="small"
                value={templateDesc}
                onChange={(e) => setTemplateDesc(e.target.value)}
                disabled={templateDialogMode === "view" || templateDialogSaving}
                placeholder="Explain the customizations or intended usage for this template"
                fullWidth
              />

              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                    TEMPLATE CONTENT (GO TEXT TEMPLATE)
                  </Typography>
                  <Button
                    size="small"
                    variant="text"
                    startIcon={copiedTemplateContent ? <Check size={14} color="#10B981" /> : <Copy size={14} />}
                    onClick={() => {
                      navigator.clipboard.writeText(templateContent);
                      setCopiedTemplateContent(true);
                      setTimeout(() => setCopiedTemplateContent(false), 2000);
                    }}
                    sx={{ fontSize: "0.75rem", py: 0.2 }}
                  >
                    {copiedTemplateContent ? "Copied!" : "Copy Template"}
                  </Button>
                </Box>
                <TextField
                  multiline
                  minRows={14}
                  maxRows={26}
                  value={templateContent}
                  onChange={(e) => setTemplateContent(e.target.value)}
                  disabled={templateDialogSaving}
                  InputProps={{
                    readOnly: templateDialogMode === "view",
                  }}
                  fullWidth
                  sx={{
                    "& .MuiInputBase-root": {
                      backgroundColor: "#0F172A",
                      borderRadius: "6px",
                      p: 1.5,
                      border: "1px solid #334155",
                      "&:hover": {
                        borderColor: templateDialogMode === "view" ? "#334155" : "#64748B",
                      },
                      "&.Mui-focused": {
                        borderColor: "#6366F1",
                      },
                      "&.Mui-disabled": {
                        backgroundColor: "#0F172A",
                      },
                    },
                    "& .MuiOutlinedInput-notchedOutline": {
                      border: "none",
                    },
                    "& .MuiInputBase-input": {
                      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
                      fontSize: "0.82rem",
                      lineHeight: 1.5,
                      color: "#F8FAFC !important",
                      WebkitTextFillColor: "#F8FAFC !important",
                      p: 0,
                    },
                    "& .MuiInputBase-input.Mui-disabled": {
                      color: "#94A3B8 !important",
                      WebkitTextFillColor: "#94A3B8 !important",
                    },
                  }}
                  inputProps={{
                    style: {
                      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
                      fontSize: "0.82rem",
                      lineHeight: 1.5,
                      color: "#F8FAFC",
                      WebkitTextFillColor: "#F8FAFC",
                    },
                  }}
                />
                <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                  Supports standard Go text/template syntax,{" "}
                  <a href="https://github.com/go-sprout/sprout" target="_blank" rel="noreferrer">
                    sprout functions
                  </a>
                  , easy42 context objects, and some custom functions: <b>eval</b>: execute a JavaScript snippet (using{" "}
                  <a href="https://github.com/dop251/goja" target="_blank" rel="noreferrer">
                    goja
                  </a>
                  ) and return result.
                  <br />
                </Typography>
              </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
              {templateDialogMode === "view" ? (
                <>
                  <Button
                    startIcon={<Copy size={16} />}
                    variant="outlined"
                    onClick={() => {
                      const t: ConfigTemplate = {
                        id: templateId,
                        name: templateName,
                        type: templateType,
                        description: templateDesc,
                        content: templateContent,
                      };
                      handleOpenCloneTemplate(t);
                    }}
                    sx={{ mr: "auto" }}
                  >
                    Clone As Custom Template
                  </Button>
                  <Button onClick={() => setTemplateDialogOpen(false)} sx={{ color: "#64748B" }}>
                    Close
                  </Button>
                </>
              ) : (
                <>
                  <Button
                    onClick={() => setTemplateDialogOpen(false)}
                    disabled={templateDialogSaving}
                    sx={{ color: "#64748B" }}
                  >
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    variant="contained"
                    disabled={templateDialogSaving}
                    startIcon={templateDialogSaving && <CircularProgress size={16} color="inherit" />}
                    sx={{ fontWeight: 600, bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
                  >
                    {templateDialogSaving
                      ? "Saving..."
                      : templateDialogMode === "create"
                        ? "Create Template"
                        : "Save Changes"}
                  </Button>
                </>
              )}
            </DialogActions>
          </form>
        </Dialog>

        {/* Delete Template Confirmation Dialog */}
        <Dialog open={Boolean(templateToDelete)} onClose={() => setTemplateToDelete(null)} maxWidth="xs" fullWidth>
          <DialogTitle sx={{ pb: 1 }}>Delete Config Template?</DialogTitle>
          <DialogContent>
            <Typography variant="body2" sx={{ color: "#475569" }}>
              Are you sure you want to delete template <strong>{templateToDelete?.name}</strong> (
              <code>{templateToDelete?.id}</code>)? This action cannot be undone.
            </Typography>
          </DialogContent>
          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={() => setTemplateToDelete(null)} disabled={templateDeleting} sx={{ color: "#64748B" }}>
              Cancel
            </Button>
            <Button
              variant="contained"
              color="error"
              onClick={handleConfirmDeleteTemplate}
              disabled={templateDeleting}
              startIcon={templateDeleting && <CircularProgress size={16} color="inherit" />}
            >
              {templateDeleting ? "Deleting..." : "Delete"}
            </Button>
          </DialogActions>
        </Dialog>
      </Paper>
    </Box>
  );
};
