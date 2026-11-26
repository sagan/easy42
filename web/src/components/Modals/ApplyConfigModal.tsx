import React, { useState, useEffect } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Alert,
  CircularProgress,
  Chip,
  Tabs,
  Tab,
  TextField,
  FormControlLabel,
  Checkbox,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Table,
  TableContainer,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  Paper,
  IconButton,
  useTheme,
  useMediaQuery,
} from "@mui/material";
import {
  FileCode2,
  CheckCircle2,
  AlertTriangle,
  Play,
  RotateCw,
  Save,
  Trash2,
  Copy,
  Eye,
  FolderOpen,
  Sparkles,
  X,
} from "lucide-react";
import { api } from "../../api/client";
import {
  ApplyPreviewResult,
  ApplyResult,
  CloudflareDNSConfig,
  ConfigChange,
  ConfigTemplate,
  CronTask,
  DatabaseInfo,
  GraphBlock,
  IntegrationsSettings,
  Link,
  LinkEnd,
  App,
  NetworkPolicy,
  NetworkSettings,
  Node,
  ServerTask,
  SSHKeyRule,
} from "../../types/api";

interface ApplyConfigModalProps {
  open: boolean;
  onClose: () => void;
  onApplied?: (result: ApplyResult, shouldSync: boolean) => void;
  initialContent?: string;
  targetNodeName?: string;
  nodes?: Node[];
  links?: Link[];
  apps?: App[];
}

/**
 * Extracts all valid apply config fields from a node's current state.
 * Strictly excludes sensitive/system/derivative fields like modified_at, agent_version, etc.
 */
export function generateNodeApplySnippet(node: Node): string {
  const p: Record<string, any> = {
    name: node.name,
  };
  if (node.host !== undefined && node.host !== "") p.host = node.host;
  if (node.is_external !== undefined) p.is_external = node.is_external;
  if (node.description !== undefined && node.description !== "") p.description = node.description;
  if (node.note !== undefined && node.note !== "") p.note = node.note;
  if (node.ip !== undefined && node.ip !== "") p.ip = node.ip;
  if (node.ip6 !== undefined && node.ip6 !== "") p.ip6 = node.ip6;
  if (node.external_ip !== undefined && node.external_ip !== "") p.external_ip = node.external_ip;
  if (node.external_ip6 !== undefined && node.external_ip6 !== "") p.external_ip6 = node.external_ip6;
  if (node.interface !== undefined && node.interface !== "") p.interface = node.interface;
  if (node.asn !== undefined && node.asn !== 0) p.asn = node.asn;
  if (node.entrypoints && node.entrypoints.length > 0) p.entrypoints = node.entrypoints;
  if (node.tags && node.tags.length > 0) p.tags = node.tags;
  if (node.table !== undefined) p.table = node.table;
  if (node.external_table !== undefined) p.external_table = node.external_table;
  if (node.internet_table !== undefined) p.internet_table = node.internet_table;
  if (node.metric !== undefined) p.metric = node.metric;
  if (node.static_routes && node.static_routes.length > 0) p.static_routes = node.static_routes;
  if (node.routes && node.routes.length > 0) p.routes = node.routes;
  if (node.config_hooks && node.config_hooks.length > 0) p.config_hooks = node.config_hooks;
  if (node.wg_template !== undefined && node.wg_template !== "") p.wg_template = node.wg_template;
  if (node.bird_template !== undefined && node.bird_template !== "") p.bird_template = node.bird_template;
  if (node.nft_template !== undefined && node.nft_template !== "") p.nft_template = node.nft_template;
  if (node.x !== undefined) p.x = node.x;
  if (node.y !== undefined) p.y = node.y;
  if (node.mode !== undefined && node.mode !== "") p.mode = node.mode;
  if (node.enable_fa !== undefined) p.enable_fa = node.enable_fa;
  if (node.update_hosts !== undefined) p.update_hosts = node.update_hosts;
  if (node.extra_hosts !== undefined && node.extra_hosts !== "") p.extra_hosts = node.extra_hosts;
  if (node.route_reflector !== undefined) p.route_reflector = node.route_reflector;
  if (node.enable_dynamic_bird_config !== undefined) p.enable_dynamic_bird_config = node.enable_dynamic_bird_config;

  return JSON.stringify({ nodes: [p] }, null, 2);
}

/**
 * Extracts all valid apply config fields from a link's current state.
 * Strictly excludes sensitive/system/crypto fields like public_key, private_key, resolved_endpoint, modified_at,
 * and system-managed derived fields like neighbor_address.
 */
export function generateLinkApplySnippet(link: Link): string {
  const cleanEnd = (end: LinkEnd) => {
    const ep: Record<string, any> = {
      name: end.name,
    };
    if (end.type !== undefined && end.type !== "") ep.type = end.type;
    if (end.interface !== undefined && end.interface !== "") ep.interface = end.interface;
    if (end.address !== undefined && end.address !== "") ep.address = end.address;
    if (end.address4 !== undefined && end.address4 !== "") ep.address4 = end.address4;
    if (end.listen_port !== undefined && end.listen_port !== 0) ep.listen_port = end.listen_port;
    if (end.endpoint !== undefined && end.endpoint !== "") ep.endpoint = end.endpoint;
    if (end.persistent_keepalive !== undefined) ep.persistent_keepalive = end.persistent_keepalive;
    if (end.mtu !== undefined && end.mtu !== 0) ep.mtu = end.mtu;
    if (end.use_ip !== undefined) ep.use_ip = end.use_ip;
    if (end.policy !== undefined && end.policy !== "") ep.policy = end.policy;
    if (end.cost !== undefined) ep.cost = end.cost;
    if (end.fwmark !== undefined && end.fwmark !== "") ep.fwmark = end.fwmark;
    if (end.preference !== undefined) ep.preference = end.preference;
    if (end.mark !== undefined && end.mark !== "") ep.mark = end.mark;
    if (end.routing_policy !== undefined && end.routing_policy !== "") ep.routing_policy = end.routing_policy;
    if (end.add_paths !== undefined && end.add_paths !== "") ep.add_paths = end.add_paths;
    if (end.note !== undefined && end.note !== "") ep.note = end.note;
    if (end.obfuscator !== undefined && end.obfuscator !== "") {
      ep.obfuscator = end.obfuscator;
      if (end.obfuscator_parameters) ep.obfuscator_parameters = end.obfuscator_parameters;
    }
    return ep;
  };

  const lp: Record<string, any> = {
    from: cleanEnd(link.from),
    to: cleanEnd(link.to),
  };
  if (link.type !== undefined && link.type !== "") lp.type = link.type;
  if (link.tags && link.tags.length > 0) lp.tags = link.tags;
  if (link.assign_ipv4 !== undefined) lp.assign_ipv4 = link.assign_ipv4;
  if (link.disable_bfd !== undefined) lp.disable_bfd = link.disable_bfd;
  if (link.psk !== undefined && link.psk !== "") lp.psk = link.psk;
  if (link.note !== undefined && link.note !== "") lp.note = link.note;

  return JSON.stringify({ links: [lp] }, null, 2);
}

/**
 * Extracts all valid apply config fields from an app's current state.
 * Strictly excludes system-managed timestamps like created_at, updated_at.
 */
export function generateAppApplySnippet(app: App): string {
  const p: Record<string, any> = {
    name: app.name,
  };
  if (app.description !== undefined && app.description !== "") p.description = app.description;
  if (app.group !== undefined && app.group !== "") p.group = app.group;
  if (app.note !== undefined && app.note !== "") p.note = app.note;
  if (app.url !== undefined && app.url !== "") p.url = app.url;
  if (app.version !== undefined && app.version !== "") p.version = app.version;
  if (app.app_type) p.app_type = app.app_type;
  if (app.main_content !== undefined && app.main_content !== "") p.main_content = app.main_content;
  if (app.env_content !== undefined && app.env_content !== "") p.env_content = app.env_content;
  if (app.files && Object.keys(app.files).length > 0) p.files = app.files;
  if (app.artifact) p.artifact = app.artifact;
  // Always emit the selector: an empty selector means "no node" and must not be
  // silently replaced by an implicit default.
  p.selector = app.selector && Object.keys(app.selector).length > 0 ? app.selector : {};
  if (app.published !== undefined) p.published = app.published;
  if (app.enabled !== undefined) p.enabled = app.enabled;

  return JSON.stringify({ apps: [p] }, null, 2);
}

export function generateSSHKeyRuleApplySnippet(rule: SSHKeyRule): string {
  const p: Record<string, any> = {
    id: rule.id,
    type: rule.type,
    dst_selector: rule.dst_selector || {},
    enabled: rule.enabled,
  };
  if (rule.source_keys && rule.source_keys.trim() !== "") {
    p.source_keys = rule.source_keys;
  } else if (rule.src_selector && Object.keys(rule.src_selector).length > 0) {
    p.src_selector = rule.src_selector;
  }
  return JSON.stringify({ ssh_key_rules: [p] }, null, 2);
}

export function generateNetworkSettingsApplySnippet(ns: NetworkSettings): string {
  const p: Record<string, any> = {};
  if (ns.name !== undefined && ns.name !== "") p.name = ns.name;
  if (ns.public_asn !== undefined && ns.public_asn !== 0) p.public_asn = ns.public_asn;
  if (ns.prefixes && ns.prefixes.length > 0) p.prefixes = ns.prefixes;
  if (ns.disallowed_dn42_networks && ns.disallowed_dn42_networks.length > 0) {
    p.disallowed_dn42_networks = ns.disallowed_dn42_networks;
  }
  if (ns.disallowed_dn42_cidrs && ns.disallowed_dn42_cidrs.length > 0) {
    p.disallowed_dn42_cidrs = ns.disallowed_dn42_cidrs;
  }
  if (ns.flapalerted_address !== undefined && ns.flapalerted_address !== "") p.flapalerted_address = ns.flapalerted_address;
  if (ns.flapalerted_port !== undefined && ns.flapalerted_port !== 0) p.flapalerted_port = ns.flapalerted_port;
  if (ns.flapalerted_asn !== undefined && ns.flapalerted_asn !== 0) p.flapalerted_asn = ns.flapalerted_asn;
  if (ns.flapalerted_url !== undefined && ns.flapalerted_url !== "") p.flapalerted_url = ns.flapalerted_url;
  if (ns.extra_hosts !== undefined && ns.extra_hosts !== "") p.extra_hosts = ns.extra_hosts;
  if (ns.bird_import_filter !== undefined && ns.bird_import_filter !== "") p.bird_import_filter = ns.bird_import_filter;
  if (ns.bird_export_filter !== undefined && ns.bird_export_filter !== "") p.bird_export_filter = ns.bird_export_filter;
  if (ns.bird_pre_snippet !== undefined && ns.bird_pre_snippet !== "") p.bird_pre_snippet = ns.bird_pre_snippet;
  if (ns.bird_post_snippet !== undefined && ns.bird_post_snippet !== "") p.bird_post_snippet = ns.bird_post_snippet;
  return JSON.stringify({ network_settings: p }, null, 2);
}

export function generateDNSApplySnippet(dns: CloudflareDNSConfig): string {
  const p: Record<string, any> = {};
  if (dns.zone_id !== undefined && dns.zone_id !== "") p.zone_id = dns.zone_id;
  if (dns.api_token !== undefined && dns.api_token !== "") p.api_token = dns.api_token;
  if (dns.base_domain !== undefined && dns.base_domain !== "") p.base_domain = dns.base_domain;
  if (dns.publish_ipv6_own_name !== undefined) p.publish_ipv6_own_name = dns.publish_ipv6_own_name;
  return JSON.stringify({ dns: p }, null, 2);
}

export function generateServerTaskApplySnippet(task: ServerTask): string {
  return JSON.stringify(
    {
      server_tasks: [
        {
          id: task.id,
          name: task.name,
          description: task.description,
          cron_expr: task.cron_expr,
          enabled: task.enabled,
        },
      ],
    },
    null,
    2,
  );
}

export function generateDatabaseApplySnippet(db: DatabaseInfo): string {
  const p: Record<string, any> = {
    retention_days: db.retention_days,
  };
  if (db.raw_retention_days !== undefined && db.raw_retention_days > 0) {
    p.raw_retention_days = db.raw_retention_days;
  }
  return JSON.stringify({ database: p }, null, 2);
}

export function generateIntegrationsApplySnippet(integ: IntegrationsSettings): string {
  const p: Record<string, any> = {
    prometheus_enabled: integ.prometheus_enabled,
  };
  if (integ.prometheus_auth_token) p.prometheus_auth_token = integ.prometheus_auth_token;
  if (integ.telegram_enabled !== undefined) p.telegram_enabled = integ.telegram_enabled;
  if (integ.telegram_bot_token) p.telegram_bot_token = integ.telegram_bot_token;
  if (integ.telegram_admin_id !== undefined && integ.telegram_admin_id !== "") {
    const numId = Number(integ.telegram_admin_id);
    if (!Number.isNaN(numId) && Number.isInteger(numId)) p.telegram_admin_id = numId;
  }
  if (integ.telegram_secret_token) p.telegram_secret_token = integ.telegram_secret_token;
  if (integ.server_url) p.server_url = integ.server_url;
  return JSON.stringify({ integrations: p }, null, 2);
}

type GeneratorType =
  | "node"
  | "link"
  | "app"
  | "ssh_key_rule"
  | "network_policy"
  | "block"
  | "template"
  | "cron_task"
  | "server_task"
  | "network_settings"
  | "dns"
  | "database"
  | "integrations";

const GENERATOR_OPTIONS: { value: GeneratorType; label: string; singleton?: boolean }[] = [
  { value: "node", label: "Node" },
  { value: "link", label: "Link" },
  { value: "app", label: "App" },
  { value: "ssh_key_rule", label: "SSH Key Rule" },
  { value: "network_policy", label: "Network Policy" },
  { value: "block", label: "Block" },
  { value: "template", label: "Template" },
  { value: "cron_task", label: "Cron Task" },
  { value: "server_task", label: "Server Task" },
  { value: "network_settings", label: "Network Settings", singleton: true },
  { value: "dns", label: "DNS", singleton: true },
  { value: "database", label: "Database", singleton: true },
  { value: "integrations", label: "Integrations", singleton: true },
];

const PRESET_SNIPPETS: { label: string; description: string; content: string }[] = [
  {
    label: "Update Node Static Routes",
    description: "Modify static routes announced by a node via BGP",
    content: `{
  "nodes": [
    {
      "name": "foobar",
      "static_routes": [
        "1.2.3.4",
        "192.168.100.0/24"
      ]
    }
  ]
}`,
  },
  {
    label: "Delta Add/Remove Static Routes",
    description: "Add or remove static routes incrementally without overwriting existing routes",
    content: `{
  "nodes": [
    {
      "name": "foobar",
      "static_routes_add": [
        "0.0.0.0/0",
        "::/0"
      ],
      "static_routes_del": [
        "1.2.3.4"
      ]
    }
  ]
}`,
  },
  {
    label: "Override Link Cost & Policy",
    description: "Set custom link metric and routing policy override between two nodes",
    content: `{
  "links": [
    {
      "from": {
        "name": "foobar",
        "cost": 50,
        "policy": "dn42"
      },
      "to": {
        "name": "hkg",
        "cost": 50,
        "policy": "dn42"
      }
    }
  ]
}`,
  },
  {
    label: "Add Node Config Hook",
    description: "Inject custom BIRD or WireGuard configuration snippets into a node",
    content: `{
  "nodes": [
    {
      "name": "foobar",
      "config_hooks": [
        {
          "type": "bird.pre",
          "content": "# Custom BIRD protocol prefix\\ndefine MY_CUSTOM_COMMUNITY = (4242, 100);\\n"
        }
      ]
    }
  ]
}`,
  },
  {
    label: "Update Global Network Settings",
    description: "Update permitted peering prefixes or FlapAlerted configuration",
    content: `{
  "network_settings": {
    "prefixes": [
      "172.20.0.0/14{21,29}",
      "fd00::/8{44,64}"
    ],
    "flapalerted_address": "172.20.0.53",
    "flapalerted_port": 1790
  }
}`,
  },
  {
    label: "Batch Multi-Resource Patch",
    description: "Modify multiple nodes and link attributes in one atomic commit",
    content: `{
  "nodes": [
    {
      "name": "foobar",
      "tags": ["core", "gateway"]
    }
  ],
  "links": [
    {
      "from": { "name": "foobar", "cost": 20 },
      "to": { "name": "fra01", "cost": 20 }
    }
  ]
}`,
  },
  {
    label: "Deploy / Update App Workload",
    description: "Declaratively manage native systemd, Docker, or cron app workloads",
    content: `{
  "apps": [
    {
      "name": "web-service",
      "app_type": "docker",
      "main_content": "services:\\n  app:\\n    image: nginx:alpine\\n    restart: always\\n",
      "env_content": "PORT=80\\n",
      "enabled": true
    }
  ]
}`,
  },
];

export const ApplyConfigModal: React.FC<ApplyConfigModalProps> = ({
  open,
  onClose,
  onApplied,
  initialContent,
  targetNodeName,
  nodes = [],
  links = [],
  apps = [],
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));

  const [activeTab, setActiveTab] = useState<number>(0);
  const [content, setContent] = useState<string>("");
  const [autoSync, setAutoSync] = useState<boolean>(true);

  // Available mesh entities for snippet generator
  const [availableNodes, setAvailableNodes] = useState<Node[]>(nodes);
  const [availableLinks, setAvailableLinks] = useState<Link[]>(links);
  const [availableApps, setAvailableApps] = useState<App[]>(apps);
  const [availableSSHKeyRules, setAvailableSSHKeyRules] = useState<SSHKeyRule[] | null>(null);
  const [availablePolicies, setAvailablePolicies] = useState<NetworkPolicy[] | null>(null);
  const [availableBlocks, setAvailableBlocks] = useState<GraphBlock[] | null>(null);
  const [availableTemplates, setAvailableTemplates] = useState<ConfigTemplate[] | null>(null);
  const [availableCronTasks, setAvailableCronTasks] = useState<CronTask[] | null>(null);
  const [availableServerTasks, setAvailableServerTasks] = useState<ServerTask[] | null>(null);
  const [currentNetworkSettings, setCurrentNetworkSettings] = useState<NetworkSettings | null>(null);
  const [currentDNSConfig, setCurrentDNSConfig] = useState<CloudflareDNSConfig | null>(null);
  const [currentDatabaseInfo, setCurrentDatabaseInfo] = useState<DatabaseInfo | null>(null);
  const [currentIntegrations, setCurrentIntegrations] = useState<IntegrationsSettings | null>(null);

  // Generator UI state
  const [genType, setGenType] = useState<GeneratorType>("node");
  const [selectedGenNode, setSelectedGenNode] = useState<string>("");
  const [selectedGenLink, setSelectedGenLink] = useState<string>("");
  const [selectedGenApp, setSelectedGenApp] = useState<string>("");
  const [selectedGenItem, setSelectedGenItem] = useState<string>("");

  // Validation state
  const [validating, setValidating] = useState<boolean>(false);
  const [validationResult, setValidationResult] = useState<{ valid: boolean; format?: string; error?: string } | null>(
    null,
  );

  // Preview & Diff state
  const [previewing, setPreviewing] = useState<boolean>(false);
  const [previewResult, setPreviewResult] = useState<ApplyPreviewResult | null>(null);

  // Apply state
  const [applying, setApplying] = useState<boolean>(false);
  const [applyResult, setApplyResult] = useState<ApplyResult | null>(null);
  const [applyError, setApplyError] = useState<string | null>(null);

  // Saved Snippets state
  const [savedSnippets, setSavedSnippets] = useState<ConfigChange[]>([]);
  const [loadingSnippets, setLoadingSnippets] = useState<boolean>(false);
  const [snippetName, setSnippetName] = useState<string>("");
  const [snippetDesc, setSnippetDesc] = useState<string>("");
  const [savingSnippet, setSavingSnippet] = useState<boolean>(false);
  const [applyingSnippetId, setApplyingSnippetId] = useState<string | null>(null);

  const existingSnippet = snippetName.trim()
    ? savedSnippets.find((s) => s.name.trim().toLowerCase() === snippetName.trim().toLowerCase())
    : undefined;
  const isExistingSnippet = Boolean(existingSnippet);

  // Sync available nodes, links & apps on open
  useEffect(() => {
    if (open) {
      if (nodes && nodes.length > 0) {
        setAvailableNodes(nodes);
      } else {
        api
          .getNodes()
          .then((res) => setAvailableNodes(res || []))
          .catch(() => {});
      }

      if (links && links.length > 0) {
        setAvailableLinks(links);
      } else {
        api
          .getLinks()
          .then((res) => setAvailableLinks(res || []))
          .catch(() => {});
      }

      if (apps && apps.length > 0) {
        setAvailableApps(apps);
      } else {
        api
          .getApps()
          .then((res) => setAvailableApps(res || []))
          .catch(() => {});
      }
    }
  }, [open, nodes, links, apps]);

  // Initialize content on open
  useEffect(() => {
    if (open) {
      if (initialContent) {
        setContent(initialContent);
      } else if (targetNodeName) {
        const found = (availableNodes.length > 0 ? availableNodes : nodes).find((n) => n.name === targetNodeName);
        if (found) {
          setContent(generateNodeApplySnippet(found));
          setSelectedGenNode(found.name);
        } else {
          setContent(PRESET_SNIPPETS[0].content);
        }
      } else if (!content) {
        setContent(PRESET_SNIPPETS[0].content);
      }
      setApplyError(null);
      setApplyResult(null);
      loadSavedSnippets();
    }
  }, [open, initialContent, targetNodeName, availableNodes, nodes]);

  // Load saved snippets
  const loadSavedSnippets = async () => {
    setLoadingSnippets(true);
    try {
      const list = await api.listConfigChanges();
      setSavedSnippets(list || []);
    } catch {
      setSavedSnippets([]);
    } finally {
      setLoadingSnippets(false);
    }
  };

  // Debounced schema validation
  useEffect(() => {
    const trimmed = content.trim();
    if (!trimmed) {
      setValidationResult(null);
      return;
    }

    const timer = setTimeout(async () => {
      setValidating(true);
      try {
        const res = await api.validateConfigApply(trimmed);
        setValidationResult(res);
      } catch (err: any) {
        setValidationResult({
          valid: false,
          error: err.message || "Failed to validate schema",
        });
      } finally {
        setValidating(false);
      }
    }, 400);

    return () => clearTimeout(timer);
  }, [content]);

  // Lazily fetch data for other generator object types when selected
  useEffect(() => {
    if (!open) return;
    switch (genType) {
      case "ssh_key_rule":
        if (availableSSHKeyRules === null) {
          api.getSSHKeyRules().then((res) => setAvailableSSHKeyRules(res || [])).catch(() => setAvailableSSHKeyRules([]));
        }
        break;
      case "network_policy":
        if (availablePolicies === null) {
          api.getNetworkPolicies().then((res) => setAvailablePolicies(res || [])).catch(() => setAvailablePolicies([]));
        }
        break;
      case "block":
        if (availableBlocks === null) {
          api.getBlocks().then((res) => setAvailableBlocks(res || [])).catch(() => setAvailableBlocks([]));
        }
        break;
      case "template":
        if (availableTemplates === null) {
          api.getTemplates().then((res) => setAvailableTemplates(res || [])).catch(() => setAvailableTemplates([]));
        }
        break;
      case "cron_task":
        if (availableCronTasks === null) {
          api.getCronTasks().then((res) => setAvailableCronTasks(res || [])).catch(() => setAvailableCronTasks([]));
        }
        break;
      case "server_task":
        if (availableServerTasks === null) {
          api.getServerTasks().then((res) => setAvailableServerTasks(res || [])).catch(() => setAvailableServerTasks([]));
        }
        break;
      case "network_settings":
        if (currentNetworkSettings === null) {
          api.getNetworkSettings().then((res) => setCurrentNetworkSettings(res)).catch(() => {});
        }
        break;
      case "dns":
        if (currentDNSConfig === null) {
          api.getDNSConfig().then((res) => setCurrentDNSConfig(res)).catch(() => {});
        }
        break;
      case "database":
        if (currentDatabaseInfo === null) {
          api.getDatabaseInfo().then((res) => setCurrentDatabaseInfo(res)).catch(() => {});
        }
        break;
      case "integrations":
        if (currentIntegrations === null) {
          api.getIntegrationsSettings().then((res) => setCurrentIntegrations(res)).catch(() => {});
        }
        break;
    }
  }, [
    open,
    genType,
    availableSSHKeyRules,
    availablePolicies,
    availableBlocks,
    availableTemplates,
    availableCronTasks,
    availableServerTasks,
    currentNetworkSettings,
    currentDNSConfig,
    currentDatabaseInfo,
    currentIntegrations,
  ]);

  // Generate snippet from chosen object's current state
  const handleGenerateFromState = () => {
    switch (genType) {
      case "node": {
        const target = availableNodes.find((n) => n.name === selectedGenNode);
        if (target) setContent(generateNodeApplySnippet(target));
        break;
      }
      case "link": {
        const target = availableLinks.find((l, idx) => `${l.from.name}<->${l.to.name}-${idx}` === selectedGenLink);
        if (target) setContent(generateLinkApplySnippet(target));
        break;
      }
      case "app": {
        const target = availableApps.find((a) => a.name === selectedGenApp || a.id === selectedGenApp);
        if (target) setContent(generateAppApplySnippet(target));
        break;
      }
      case "ssh_key_rule": {
        const target = (availableSSHKeyRules || []).find((r) => r.id === selectedGenItem);
        if (target) setContent(generateSSHKeyRuleApplySnippet(target));
        break;
      }
      case "network_policy": {
        const target = (availablePolicies || []).find((p) => p.id === selectedGenItem || p.name === selectedGenItem);
        if (target) setContent(JSON.stringify({ network_policies: [target] }, null, 2));
        break;
      }
      case "block": {
        const target = (availableBlocks || []).find((b) => b.id === selectedGenItem);
        if (target) setContent(JSON.stringify({ blocks: [target] }, null, 2));
        break;
      }
      case "template": {
        const target = (availableTemplates || []).find((t) => t.id === selectedGenItem || t.name === selectedGenItem);
        if (target) setContent(JSON.stringify({ templates: [target] }, null, 2));
        break;
      }
      case "cron_task": {
        const target = (availableCronTasks || []).find((t) => t.id === selectedGenItem || t.name === selectedGenItem);
        if (target) setContent(JSON.stringify({ cron_tasks: [target] }, null, 2));
        break;
      }
      case "server_task": {
        const target = (availableServerTasks || []).find((t) => t.id === selectedGenItem);
        if (target) setContent(generateServerTaskApplySnippet(target));
        break;
      }
      case "network_settings":
        if (currentNetworkSettings) setContent(generateNetworkSettingsApplySnippet(currentNetworkSettings));
        break;
      case "dns":
        if (currentDNSConfig) setContent(generateDNSApplySnippet(currentDNSConfig));
        break;
      case "database":
        if (currentDatabaseInfo) setContent(generateDatabaseApplySnippet(currentDatabaseInfo));
        break;
      case "integrations":
        if (currentIntegrations) setContent(generateIntegrationsApplySnippet(currentIntegrations));
        break;
    }
  };

  const canGenerateFromState = (() => {
    switch (genType) {
      case "node":
        return Boolean(selectedGenNode);
      case "link":
        return Boolean(selectedGenLink);
      case "app":
        return Boolean(selectedGenApp);
      case "network_settings":
        return Boolean(currentNetworkSettings);
      case "dns":
        return Boolean(currentDNSConfig);
      case "database":
        return Boolean(currentDatabaseInfo);
      case "integrations":
        return Boolean(currentIntegrations);
      default:
        return Boolean(selectedGenItem);
    }
  })();

  // Run Preview / Dry-run
  const handlePreview = async () => {
    if (!content.trim()) return;
    setPreviewing(true);
    setApplyError(null);
    try {
      const res = await api.previewConfigApply(content);
      setPreviewResult(res);
      setActiveTab(1); // Switch to Preview & Diff tab
    } catch (err: any) {
      setApplyError(err.message || "Failed to generate preview");
    } finally {
      setPreviewing(false);
    }
  };

  // Execute Apply
  const handleApply = async () => {
    if (!content.trim()) return;
    setApplying(true);
    setApplyError(null);
    try {
      const res = await api.executeConfigApply(content, false);
      setApplyResult(res);
      if (onApplied) {
        onApplied(res, autoSync);
      }
      onClose();
    } catch (err: any) {
      setApplyError(err.message || "Failed to apply config changes");
    } finally {
      setApplying(false);
    }
  };

  // Format JSON
  const handleFormat = () => {
    try {
      const parsed = JSON.parse(content);
      setContent(JSON.stringify(parsed, null, 2));
    } catch {
      // Ignore if YAML or invalid JSON
    }
  };

  // Save current snippet (overrides existing snippet of the same name if present)
  const handleSaveSnippet = async () => {
    if (!snippetName.trim() || !content.trim()) return;
    setSavingSnippet(true);
    try {
      await api.saveConfigChange({
        id: existingSnippet?.id,
        name: snippetName.trim(),
        description: snippetDesc.trim(),
        content: content,
      });
      setSnippetName("");
      setSnippetDesc("");
      await loadSavedSnippets();
    } catch (err: any) {
      setApplyError(err.message || "Failed to save snippet");
    } finally {
      setSavingSnippet(false);
    }
  };

  // Delete saved snippet
  const handleDeleteSnippet = async (s: ConfigChange) => {
    if (!confirm(`Are you sure to delete apply config snippet "${s.name}"`)) {
      return;
    }
    try {
      await api.deleteConfigChange(s.id);
      await loadSavedSnippets();
    } catch (err: any) {
      setApplyError(err.message || "Failed to delete snippet");
    }
  };

  // Load saved snippet into editor and immediately activate Preview & Diff tab
  const handleLoadSnippetAndPreview = async (snippet: ConfigChange) => {
    setContent(snippet.content);
    setSnippetName(snippet.name);
    setSnippetDesc(snippet.description || "");
    setActiveTab(1); // Switch to Preview & Diff tab
    setPreviewing(true);
    setApplyError(null);
    try {
      const res = await api.previewConfigApply(snippet.content);
      setPreviewResult(res);
    } catch (err: any) {
      setApplyError(err.message || "Failed to generate preview");
    } finally {
      setPreviewing(false);
    }
  };

  // Directly apply saved snippet
  const handleApplySnippetDirect = async (snippet: ConfigChange) => {
    setApplyingSnippetId(snippet.id);
    setApplyError(null);
    try {
      const res = await api.executeConfigApply(snippet.content, false);
      setApplyResult(res);
      if (onApplied) {
        onApplied(res, autoSync);
      }
      onClose();
    } catch (err: any) {
      setApplyError(err.message || `Failed to apply snippet "${snippet.name}"`);
    } finally {
      setApplyingSnippetId(null);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      fullScreen={isMobile}
      PaperProps={{
        sx: {
          borderRadius: { xs: 0, sm: 1 },
          height: { xs: "100%", sm: "85vh" },
          m: { xs: 0, sm: 1 },
        },
      }}
    >
      <DialogTitle
        sx={{
          p: 1,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "flex-start", sm: "center" },
            justifyContent: "space-between",
            gap: 1,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1, width: "100%" }}>
            <Box
              sx={{
                width: 38,
                height: 38,
                borderRadius: 1,
                bgcolor: "rgba(37, 99, 235, 0.1)",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: "#2563EB",
                flexShrink: 0,
                mt: 0.25,
              }}
            >
              <FileCode2 size={22} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1 }}>
                <Typography
                  variant="h6"
                  sx={{
                    fontWeight: 700,
                    color: "#0F172A",
                    fontSize: { xs: "1.05rem", sm: "1.25rem" },
                    lineHeight: 1.3,
                  }}
                >
                  Apply Config Changes
                </Typography>
                <IconButton
                  size="small"
                  onClick={onClose}
                  sx={{ display: { xs: "inline-flex", sm: "none" }, color: "text.secondary", p: 0.5 }}
                >
                  <X size={20} />
                </IconButton>
              </Box>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.25 }}>
                Declaratively patch nodes, links, and network settings with strict schema verification
              </Typography>
            </Box>
          </Box>

          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: { xs: "space-between", sm: "flex-end" },
              width: { xs: "100%", sm: "auto" },
              gap: 1,
            }}
          >
            {validating ? (
              <Chip size="small" icon={<RotateCw className="animate-spin" size={14} />} label="Validating..." />
            ) : validationResult?.valid ? (
              <Chip
                size="small"
                color="success"
                icon={<CheckCircle2 size={14} />}
                label={`Schema Valid (${validationResult.format?.toUpperCase() || "JSON"})`}
              />
            ) : validationResult?.error ? (
              <Chip size="small" color="error" icon={<AlertTriangle size={14} />} label="Schema Violation" />
            ) : null}
            <IconButton
              size="small"
              onClick={onClose}
              sx={{ display: { xs: "none", sm: "inline-flex" }, color: "text.secondary", ml: 1 }}
            >
              <X size={20} />
            </IconButton>
          </Box>
        </Box>
      </DialogTitle>

      <Box sx={{ borderBottom: 1, borderColor: "divider", px: 1, pt: 1 }}>
        <Tabs
          value={activeTab}
          onChange={(_, val) => setActiveTab(val)}
          variant="scrollable"
          scrollButtons="auto"
          allowScrollButtonsMobile
        >
          <Tab label="Editor & Schema" icon={<FileCode2 size={16} />} iconPosition="start" />
          <Tab
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <span>Preview & Diff</span>
                {previewResult && (previewResult.summary?.total_changes || 0) > 0 && (
                  <Chip
                    size="small"
                    color="primary"
                    sx={{ height: 20, fontSize: 11 }}
                    label={`${previewResult.summary?.total_changes || 0} changes`}
                  />
                )}
              </Box>
            }
            icon={<Eye size={16} />}
            iconPosition="start"
          />
          <Tab
            label={`Saved Snippets (${(savedSnippets || []).length})`}
            icon={<FolderOpen size={16} />}
            iconPosition="start"
          />
        </Tabs>
      </Box>

      <DialogContent sx={{ p: 1, display: "flex", flexDirection: "column", gap: 1, overflowY: "auto" }}>
        {applyError && (
          <Alert severity="error" onClose={() => setApplyError(null)}>
            {applyError}
          </Alert>
        )}

        {applyResult && (
          <Alert severity="success" onClose={() => setApplyResult(null)}>
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              Config change successfully applied at {new Date(applyResult.applied_at).toLocaleTimeString()}!
            </Typography>
            {(applyResult.affected_nodes || []).length > 0 && (
              <Typography variant="caption" sx={{ display: "block", mt: 0.5 }}>
                Affected Nodes: {(applyResult.affected_nodes || []).join(", ")}
              </Typography>
            )}
            {(applyResult.affected_apps || []).length > 0 && (
              <Typography variant="caption" sx={{ display: "block", mt: 0.5 }}>
                Affected Apps: {(applyResult.affected_apps || []).join(", ")}
              </Typography>
            )}
            {applyResult.backup_file && (
              <Typography variant="caption" sx={{ display: "block", color: "#64748B" }}>
                Backup created: {applyResult.backup_file}
              </Typography>
            )}
          </Alert>
        )}

        {/* TAB 0: EDITOR */}
        {activeTab === 0 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1, flex: 1 }}>
            {/* Top Toolbar: Preset Templates & Format Actions */}
            <Box
              sx={{
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                justifyContent: "space-between",
                gap: 1,
              }}
            >
              <FormControl size="small" sx={{ minWidth: { xs: "100%", sm: 260 } }}>
                <InputLabel>Preset Templates</InputLabel>
                <Select
                  label="Preset Templates"
                  defaultValue=""
                  onChange={(e) => {
                    const preset = PRESET_SNIPPETS.find((p) => p.label === e.target.value);
                    if (preset) setContent(preset.content);
                  }}
                >
                  {PRESET_SNIPPETS.map((p) => (
                    <MenuItem key={p.label} value={p.label}>
                      {p.label}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              <Box sx={{ display: "flex", gap: 1, width: { xs: "100%", sm: "auto" } }}>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={handleFormat}
                  sx={{ flex: { xs: 1, sm: "none" }, whiteSpace: "nowrap" }}
                >
                  Format JSON
                </Button>
                <Button
                  size="small"
                  variant="outlined"
                  onClick={() => {
                    navigator.clipboard.writeText(content);
                  }}
                  startIcon={<Copy size={14} />}
                  sx={{ flex: { xs: 1, sm: "none" }, whiteSpace: "nowrap" }}
                >
                  Copy
                </Button>
              </Box>
            </Box>

            {/* Quick Generator from Current State Bar */}
            <Paper
              variant="outlined"
              sx={{
                p: 1,
                backgroundColor: "#F8FAFC",
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                justifyContent: "space-between",
                gap: 1,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Sparkles size={16} color="#0284C7" />
                <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Generate from Current State:
                </Typography>
              </Box>

              <Box
                sx={{
                  display: "flex",
                  alignItems: { xs: "stretch", sm: "center" },
                  flexDirection: { xs: "column", sm: "row" },
                  gap: 1,
                  flex: 1,
                  justifyContent: "flex-end",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                  <FormControl size="small" sx={{ minWidth: 155 }}>
                    <Select
                      value={genType}
                      onChange={(e) => {
                        setGenType(e.target.value as GeneratorType);
                        setSelectedGenItem("");
                      }}
                      sx={{ height: 32, fontSize: 13, backgroundColor: "#FFFFFF", fontWeight: 600 }}
                    >
                      {GENERATOR_OPTIONS.map((opt) => (
                        <MenuItem key={opt.value} value={opt.value}>
                          {opt.label}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>

                  {genType === "node" ? (
                    <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 200 }, flex: 1 }}>
                      <Select
                        displayEmpty
                        value={selectedGenNode}
                        onChange={(e) => setSelectedGenNode(e.target.value)}
                        sx={{ height: 32, fontSize: 13, backgroundColor: "#FFFFFF" }}
                      >
                        <MenuItem value="" disabled>
                          <em>Select a node...</em>
                        </MenuItem>
                        {availableNodes.map((n) => (
                          <MenuItem key={n.name} value={n.name}>
                            {n.name} {n.ip ? `(${n.ip})` : ""}
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  ) : genType === "link" ? (
                    <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 220 }, flex: 1 }}>
                      <Select
                        displayEmpty
                        value={selectedGenLink}
                        onChange={(e) => setSelectedGenLink(e.target.value)}
                        sx={{ height: 32, fontSize: 13, backgroundColor: "#FFFFFF" }}
                      >
                        <MenuItem value="" disabled>
                          <em>Select a link...</em>
                        </MenuItem>
                        {availableLinks.map((l, idx) => {
                          const linkKey = `${l.from.name}<->${l.to.name}-${idx}`;
                          return (
                            <MenuItem key={linkKey} value={linkKey}>
                              {l.from.name} ↔ {l.to.name} ({l.from.interface || "wg42"})
                            </MenuItem>
                          );
                        })}
                      </Select>
                    </FormControl>
                  ) : genType === "app" ? (
                    <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 200 }, flex: 1 }}>
                      <Select
                        displayEmpty
                        value={selectedGenApp}
                        onChange={(e) => setSelectedGenApp(e.target.value)}
                        sx={{ height: 32, fontSize: 13, backgroundColor: "#FFFFFF" }}
                      >
                        <MenuItem value="" disabled>
                          <em>Select an app...</em>
                        </MenuItem>
                        {availableApps.map((a) => (
                          <MenuItem key={a.id || a.name} value={a.name}>
                            {a.name} ({a.app_type})
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  ) : !GENERATOR_OPTIONS.find((o) => o.value === genType)?.singleton ? (
                    <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 220 }, flex: 1 }}>
                      <Select
                        displayEmpty
                        value={selectedGenItem}
                        onChange={(e) => setSelectedGenItem(e.target.value)}
                        sx={{ height: 32, fontSize: 13, backgroundColor: "#FFFFFF" }}
                      >
                        <MenuItem value="" disabled>
                          <em>Select {GENERATOR_OPTIONS.find((o) => o.value === genType)?.label.toLowerCase()}...</em>
                        </MenuItem>
                        {genType === "ssh_key_rule" &&
                          (availableSSHKeyRules || []).map((r) => (
                            <MenuItem key={r.id} value={r.id}>
                              {r.id} ({r.type})
                            </MenuItem>
                          ))}
                        {genType === "network_policy" &&
                          (availablePolicies || []).map((p) => (
                            <MenuItem key={p.id || p.name} value={p.id || p.name}>
                              {p.name || p.id}
                            </MenuItem>
                          ))}
                        {genType === "block" &&
                          (availableBlocks || []).map((b) => (
                            <MenuItem key={b.id} value={b.id}>
                              {b.name || b.id}
                            </MenuItem>
                          ))}
                        {genType === "template" &&
                          (availableTemplates || []).map((t) => (
                            <MenuItem key={t.id || t.name} value={t.id || t.name}>
                              {t.name} ({t.type})
                            </MenuItem>
                          ))}
                        {genType === "cron_task" &&
                          (availableCronTasks || []).map((t) => (
                            <MenuItem key={t.id || t.name} value={t.id || t.name}>
                              {t.name} ({t.cron_expr})
                            </MenuItem>
                          ))}
                        {genType === "server_task" &&
                          (availableServerTasks || []).map((t) => (
                            <MenuItem key={t.id} value={t.id}>
                              {t.name} ({t.cron_expr})
                            </MenuItem>
                          ))}
                      </Select>
                    </FormControl>
                  ) : null}
                </Box>

                <Button
                  variant="contained"
                  size="small"
                  disabled={!canGenerateFromState}
                  onClick={handleGenerateFromState}
                  startIcon={<Sparkles size={14} />}
                  sx={{ textTransform: "none", height: 32, whiteSpace: "nowrap" }}
                >
                  Insert Snippet
                </Button>
              </Box>
            </Paper>

            {validationResult?.error && (
              <Alert severity="warning" sx={{ py: 0.5 }}>
                <Typography variant="caption" sx={{ fontFamily: "monospace", display: "block" }}>
                  {validationResult.error}
                </Typography>
              </Alert>
            )}

            <TextField
              multiline
              rows={isMobile ? 10 : 15}
              fullWidth
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="Paste JSON or YAML config change snippet here..."
              InputProps={{
                sx: {
                  fontFamily: "monospace",
                  fontSize: "13px",
                  lineHeight: "1.5",
                  backgroundColor: "#F8FAFC",
                  "& textarea": { resize: "vertical" },
                },
              }}
            />

            {/* Quick Save Snippet Bar */}
            <Box
              sx={{
                p: 1,
                backgroundColor: "#F1F5F9",
                borderRadius: 1,
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                gap: 1,
              }}
            >
              <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", minWidth: { xs: "auto", sm: 100 } }}>
                Save Snippet:
              </Typography>
              <TextField
                size="small"
                placeholder="Snippet Name (e.g. Frankfurt BGP Update)"
                value={snippetName}
                onChange={(e) => setSnippetName(e.target.value)}
                sx={{ flex: 1, backgroundColor: "#FFFFFF" }}
                helperText={isExistingSnippet ? "Will overwrite existing snippet with this name" : undefined}
                FormHelperTextProps={{ sx: { color: "#D97706", fontWeight: 500 } }}
              />
              <TextField
                size="small"
                placeholder="Description (optional)"
                value={snippetDesc}
                onChange={(e) => setSnippetDesc(e.target.value)}
                sx={{ flex: 1, backgroundColor: "#FFFFFF" }}
              />
              <Button
                variant={isExistingSnippet ? "contained" : "outlined"}
                color={isExistingSnippet ? "warning" : "primary"}
                size="medium"
                startIcon={<Save size={16} />}
                onClick={handleSaveSnippet}
                disabled={!snippetName.trim() || savingSnippet || !validationResult?.valid}
                sx={{ width: { xs: "100%", sm: "auto" }, whiteSpace: "nowrap" }}
              >
                {isExistingSnippet ? "Overwrite" : "Save"}
              </Button>
            </Box>
          </Box>
        )}

        {/* TAB 1: PREVIEW & DIFF */}
        {activeTab === 1 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1, flex: 1 }}>
            {!previewResult ? (
              <Box sx={{ textAlign: "center", py: 8 }}>
                <Typography variant="body2" sx={{ color: "#64748B", mb: 1 }}>
                  No preview generated yet. Click "Preview Changes" to inspect what will be modified.
                </Typography>
                <Button variant="contained" onClick={handlePreview} disabled={previewing || !validationResult?.valid}>
                  {previewing ? "Analyzing..." : "Generate Preview"}
                </Button>
              </Box>
            ) : (
              <>
                <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
                  <Chip
                    label={`${previewResult.summary?.modified_nodes || 0} Nodes Modified`}
                    color="primary"
                    variant="outlined"
                  />
                  <Chip
                    label={`${previewResult.summary?.added_nodes || 0} Nodes Added`}
                    color="success"
                    variant="outlined"
                  />
                  <Chip
                    label={`${previewResult.summary?.modified_links || 0} Links Modified`}
                    color="primary"
                    variant="outlined"
                  />
                  <Chip
                    label={`${previewResult.summary?.added_links || 0} Links Added`}
                    color="success"
                    variant="outlined"
                  />
                  {previewResult.summary?.modified_apps !== undefined && previewResult.summary.modified_apps > 0 && (
                    <Chip
                      label={`${previewResult.summary.modified_apps} Apps Modified`}
                      color="primary"
                      variant="outlined"
                    />
                  )}
                  {previewResult.summary?.added_apps !== undefined && previewResult.summary.added_apps > 0 && (
                    <Chip label={`${previewResult.summary.added_apps} Apps Added`} color="success" variant="outlined" />
                  )}
                  <Chip
                    label={`${previewResult.summary?.total_changes || 0} Total Attributes Changed`}
                    color="secondary"
                  />
                </Box>

                {(previewResult.nodes || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
                      Node Modifications
                    </Typography>
                    <TableContainer sx={{ overflowX: "auto" }}>
                      <Table size="small" sx={{ minWidth: 480 }}>
                        <TableHead>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 600 }}>Node</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Action</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Field</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Old Value</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>New Value</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {(previewResult.nodes || []).map((n) =>
                            (n.changes || []).map((c, idx) => (
                              <TableRow key={`${n.name}-${c.field}-${idx}`}>
                                <TableCell sx={{ fontFamily: "monospace" }}>{n.name}</TableCell>
                                <TableCell>
                                  <Chip
                                    size="small"
                                    label={n.action}
                                    color={n.action === "create" ? "success" : "info"}
                                  />
                                </TableCell>
                                <TableCell sx={{ fontFamily: "monospace" }}>{c.field}</TableCell>
                                <TableCell sx={{ color: "#991B1B", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.old_value) || "null"}
                                </TableCell>
                                <TableCell sx={{ color: "#166534", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.new_value)}
                                </TableCell>
                              </TableRow>
                            )),
                          )}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  </Paper>
                )}

                {(previewResult.links || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
                      Link Modifications
                    </Typography>
                    <TableContainer sx={{ overflowX: "auto" }}>
                      <Table size="small" sx={{ minWidth: 480 }}>
                        <TableHead>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 600 }}>Link</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Action</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Field</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Old Value</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>New Value</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {(previewResult.links || []).map((l) =>
                            (l.changes || []).map((c, idx) => (
                              <TableRow key={`${l.from}-${l.to}-${c.field}-${idx}`}>
                                <TableCell sx={{ fontFamily: "monospace" }}>
                                  {l.from} &lt;-&gt; {l.to}
                                </TableCell>
                                <TableCell>
                                  <Chip
                                    size="small"
                                    label={l.action}
                                    color={l.action === "create" ? "success" : "info"}
                                  />
                                </TableCell>
                                <TableCell sx={{ fontFamily: "monospace" }}>{c.field}</TableCell>
                                <TableCell sx={{ color: "#991B1B", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.old_value) || "null"}
                                </TableCell>
                                <TableCell sx={{ color: "#166534", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.new_value)}
                                </TableCell>
                              </TableRow>
                            )),
                          )}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  </Paper>
                )}

                {(previewResult.apps || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
                      App Modifications
                    </Typography>
                    <TableContainer sx={{ overflowX: "auto" }}>
                      <Table size="small" sx={{ minWidth: 480 }}>
                        <TableHead>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 600 }}>App</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Action</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Field</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Old Value</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>New Value</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {(previewResult.apps || []).map((a) =>
                            (a.changes || []).map((c, idx) => (
                              <TableRow key={`${a.name}-${c.field}-${idx}`}>
                                <TableCell sx={{ fontFamily: "monospace" }}>{a.name}</TableCell>
                                <TableCell>
                                  <Chip
                                    size="small"
                                    label={a.action}
                                    color={a.action === "create" ? "success" : "info"}
                                  />
                                </TableCell>
                                <TableCell sx={{ fontFamily: "monospace" }}>{c.field}</TableCell>
                                <TableCell sx={{ color: "#991B1B", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.old_value) || "null"}
                                </TableCell>
                                <TableCell sx={{ color: "#166534", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.new_value)}
                                </TableCell>
                              </TableRow>
                            )),
                          )}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  </Paper>
                )}

                {(previewResult.settings || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
                      Settings Modifications
                    </Typography>
                    <TableContainer sx={{ overflowX: "auto" }}>
                      <Table size="small" sx={{ minWidth: 420 }}>
                        <TableHead>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 600 }}>Category</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Field</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>Old Value</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>New Value</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {(previewResult.settings || []).map((s) =>
                            (s.changes || []).map((c, idx) => (
                              <TableRow key={`${s.category}-${c.field}-${idx}`}>
                                <TableCell sx={{ fontFamily: "monospace" }}>{s.category}</TableCell>
                                <TableCell sx={{ fontFamily: "monospace" }}>{c.field}</TableCell>
                                <TableCell sx={{ color: "#991B1B", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.old_value) || "null"}
                                </TableCell>
                                <TableCell sx={{ color: "#166534", fontFamily: "monospace", fontSize: 12 }}>
                                  {JSON.stringify(c.new_value)}
                                </TableCell>
                              </TableRow>
                            )),
                          )}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  </Paper>
                )}

                {previewResult.unified_diff && (
                  <Paper variant="outlined" sx={{ p: 1, backgroundColor: "#0F172A", color: "#F8FAFC" }}>
                    <Typography variant="caption" sx={{ color: "#94A3B8", display: "block", mb: 1 }}>
                      Unified JSON Diff
                    </Typography>
                    <Box
                      component="pre"
                      sx={{
                        m: 0,
                        fontFamily: "monospace",
                        fontSize: 12,
                        lineHeight: 1.6,
                        maxHeight: 280,
                        overflowY: "auto",
                        overflowX: "auto",
                      }}
                    >
                      {previewResult.unified_diff.split("\n").map((line, i) => {
                        let color = "#E2E8F0";
                        if (line.startsWith("+")) color = "#4ADE80";
                        else if (line.startsWith("-")) color = "#F87171";
                        return (
                          <div key={i} style={{ color }}>
                            {line}
                          </div>
                        );
                      })}
                    </Box>
                  </Paper>
                )}
              </>
            )}
          </Box>
        )}

        {/* TAB 2: SAVED SNIPPETS */}
        {activeTab === 2 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1, flex: 1 }}>
            {loadingSnippets ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
                <CircularProgress size={32} />
              </Box>
            ) : (savedSnippets || []).length === 0 ? (
              <Box sx={{ textAlign: "center", py: 8 }}>
                <Typography variant="body2" sx={{ color: "#64748B" }}>
                  No saved config snippets yet. Save frequently used patches in the Editor tab to reuse them anytime.
                </Typography>
              </Box>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                {(savedSnippets || []).map((s) => (
                  <Paper
                    key={s.id}
                    variant="outlined"
                    sx={{
                      p: 1,
                      display: "flex",
                      flexDirection: { xs: "column", sm: "row" },
                      alignItems: { xs: "flex-start", sm: "center" },
                      justifyContent: "space-between",
                      gap: 1,
                      "&:hover": { backgroundColor: "#F8FAFC" },
                    }}
                  >
                    <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                          {s.name}
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          {new Date(s.modified_at).toLocaleDateString()}
                        </Typography>
                      </Box>
                      {s.description && (
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {s.description}
                        </Typography>
                      )}
                    </Box>

                    <Box
                      sx={{
                        display: "flex",
                        alignItems: "center",
                        gap: 1,
                        width: { xs: "100%", sm: "auto" },
                        justifyContent: { xs: "flex-end", sm: "flex-start" },
                        flexWrap: "wrap",
                      }}
                    >
                      <Button
                        size="small"
                        variant="contained"
                        color="primary"
                        startIcon={
                          applyingSnippetId === s.id ? (
                            <CircularProgress size={14} color="inherit" />
                          ) : (
                            <Play size={14} />
                          )
                        }
                        disabled={applyingSnippetId !== null || applying}
                        onClick={() => handleApplySnippetDirect(s)}
                        sx={{ textTransform: "none" }}
                      >
                        {applyingSnippetId === s.id ? "Applying..." : "Apply"}
                      </Button>
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => handleLoadSnippetAndPreview(s)}
                        sx={{ textTransform: "none" }}
                      >
                        Load into Editor
                      </Button>
                      <IconButton size="small" color="error" onClick={() => handleDeleteSnippet(s)}>
                        <Trash2 size={16} />
                      </IconButton>
                    </Box>
                  </Paper>
                ))}
              </Box>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions
        sx={{
          px: 1,
          py: 1,
          borderTop: "1px solid #E2E8F0",
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          alignItems: { xs: "stretch", sm: "center" },
          justifyContent: "space-between",
          gap: 1,
        }}
      >
        <FormControlLabel
          control={<Checkbox checked={autoSync} onChange={(e) => setAutoSync(e.target.checked)} />}
          label={
            <Typography variant="body2" sx={{ fontSize: { xs: "0.825rem", sm: "0.875rem" } }}>
              Sync network immediately after applying
            </Typography>
          }
          sx={{ mr: 0 }}
        />

        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1,
            width: { xs: "100%", sm: "auto" },
            flexWrap: { xs: "wrap", sm: "nowrap" },
          }}
        >
          <Button variant="outlined" onClick={onClose} sx={{ flex: { xs: "1 1 30%", sm: "none" } }}>
            Cancel
          </Button>
          <Button
            variant="outlined"
            onClick={handlePreview}
            disabled={previewing || !validationResult?.valid}
            startIcon={previewing ? <RotateCw className="animate-spin" size={16} /> : <Eye size={16} />}
            sx={{ flex: { xs: "1 1 55%", sm: "none" }, whiteSpace: "nowrap" }}
          >
            {previewing ? "Previewing..." : "Preview Changes"}
          </Button>
          <Button
            variant="contained"
            color="primary"
            onClick={handleApply}
            disabled={applying || !validationResult?.valid}
            startIcon={applying ? <CircularProgress size={16} color="inherit" /> : <Play size={16} />}
            sx={{
              fontWeight: 700,
              flex: { xs: "1 1 100%", sm: "none" },
              whiteSpace: "nowrap",
            }}
          >
            {applying ? "Applying..." : "Apply"}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
};
