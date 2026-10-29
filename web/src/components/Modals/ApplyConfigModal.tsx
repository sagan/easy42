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
  ToggleButton,
  ToggleButtonGroup,
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
import { ApplyPreviewResult, ApplyResult, ConfigChange, Node, Link, LinkEnd } from "../../types/api";

interface ApplyConfigModalProps {
  open: boolean;
  onClose: () => void;
  onApplied?: (result: ApplyResult, shouldSync: boolean) => void;
  initialContent?: string;
  targetNodeName?: string;
  nodes?: Node[];
  links?: Link[];
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
  if (node.route_reflector !== undefined) p.route_reflector = node.route_reflector;

  return JSON.stringify({ nodes: [p] }, null, 2);
}

/**
 * Extracts all valid apply config fields from a link's current state.
 * Strictly excludes sensitive/system/crypto fields like public_key, private_key, resolved_endpoint, modified_at.
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
    if (end.neighbor_address !== undefined && end.neighbor_address !== "") ep.neighbor_address = end.neighbor_address;
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
    if (end.note !== undefined && end.note !== "") ep.note = end.note;
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

  return JSON.stringify({ links: [lp] }, null, 2);
}

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
];

export const ApplyConfigModal: React.FC<ApplyConfigModalProps> = ({
  open,
  onClose,
  onApplied,
  initialContent,
  targetNodeName,
  nodes = [],
  links = [],
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));

  const [activeTab, setActiveTab] = useState<number>(0);
  const [content, setContent] = useState<string>("");
  const [autoSync, setAutoSync] = useState<boolean>(true);

  // Available mesh entities for snippet generator
  const [availableNodes, setAvailableNodes] = useState<Node[]>(nodes);
  const [availableLinks, setAvailableLinks] = useState<Link[]>(links);

  // Generator UI state
  const [genType, setGenType] = useState<"node" | "link">("node");
  const [selectedGenNode, setSelectedGenNode] = useState<string>("");
  const [selectedGenLink, setSelectedGenLink] = useState<string>("");

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
    ? savedSnippets.find(
        (s) => s.name.trim().toLowerCase() === snippetName.trim().toLowerCase()
      )
    : undefined;
  const isExistingSnippet = Boolean(existingSnippet);

  // Sync available nodes & links on open
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
    }
  }, [open, nodes, links]);

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

  // Generate snippet from chosen node or link current state
  const handleGenerateFromState = () => {
    if (genType === "node") {
      const target = availableNodes.find((n) => n.name === selectedGenNode);
      if (target) {
        setContent(generateNodeApplySnippet(target));
      }
    } else {
      const target = availableLinks.find((l, idx) => `${l.from.name}<->${l.to.name}-${idx}` === selectedGenLink);
      if (target) {
        setContent(generateLinkApplySnippet(target));
      }
    }
  };

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
  const handleDeleteSnippet = async (id: string) => {
    try {
      await api.deleteConfigChange(id);
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
          borderRadius: { xs: 0, sm: 2 },
          height: { xs: "100%", sm: "85vh" },
          m: { xs: 0, sm: 2 },
        },
      }}
    >
      <DialogTitle
        sx={{
          p: { xs: 2, sm: 2.5 },
          pb: 1.5,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "flex-start", sm: "center" },
            justifyContent: "space-between",
            gap: 1.5,
          }}
        >
          <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.5, width: "100%" }}>
            <Box
              sx={{
                width: 38,
                height: 38,
                borderRadius: 2,
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

      <Box sx={{ borderBottom: 1, borderColor: "divider", px: { xs: 1.5, sm: 3 }, pt: 1 }}>
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

      <DialogContent
        sx={{ p: { xs: 1.5, sm: 3 }, display: "flex", flexDirection: "column", gap: 2, overflowY: "auto" }}
      >
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
            {applyResult.backup_file && (
              <Typography variant="caption" sx={{ display: "block", color: "#64748B" }}>
                Backup created: {applyResult.backup_file}
              </Typography>
            )}
          </Alert>
        )}

        {/* TAB 0: EDITOR */}
        {activeTab === 0 && (
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2, flex: 1 }}>
            {/* Top Toolbar: Preset Templates & Format Actions */}
            <Box
              sx={{
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                justifyContent: "space-between",
                gap: 1.5,
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
                p: 1.5,
                backgroundColor: "#F8FAFC",
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                justifyContent: "space-between",
                gap: 1.5,
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
                  gap: 1.5,
                  flex: 1,
                  justifyContent: "flex-end",
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
                  <ToggleButtonGroup
                    size="small"
                    value={genType}
                    exclusive
                    onChange={(_, val) => {
                      if (val) setGenType(val);
                    }}
                    sx={{ height: 32 }}
                  >
                    <ToggleButton value="node" sx={{ textTransform: "none", px: 1.5, py: 0.5, fontSize: 13 }}>
                      Node
                    </ToggleButton>
                    <ToggleButton value="link" sx={{ textTransform: "none", px: 1.5, py: 0.5, fontSize: 13 }}>
                      Link
                    </ToggleButton>
                  </ToggleButtonGroup>

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
                  ) : (
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
                  )}
                </Box>

                <Button
                  variant="contained"
                  size="small"
                  disabled={genType === "node" ? !selectedGenNode : !selectedGenLink}
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
                p: { xs: 1.5, sm: 2 },
                backgroundColor: "#F1F5F9",
                borderRadius: 1.5,
                display: "flex",
                flexDirection: { xs: "column", sm: "row" },
                alignItems: { xs: "stretch", sm: "center" },
                gap: 1.5,
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
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2, flex: 1 }}>
            {!previewResult ? (
              <Box sx={{ textAlign: "center", py: 8 }}>
                <Typography variant="body2" sx={{ color: "#64748B", mb: 2 }}>
                  No preview generated yet. Click "Preview Changes" to inspect what will be modified.
                </Typography>
                <Button variant="contained" onClick={handlePreview} disabled={previewing || !validationResult?.valid}>
                  {previewing ? "Analyzing..." : "Generate Preview"}
                </Button>
              </Box>
            ) : (
              <>
                <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
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
                  <Chip
                    label={`${previewResult.summary?.total_changes || 0} Total Attributes Changed`}
                    color="secondary"
                  />
                </Box>

                {(previewResult.nodes || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1.5, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
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
                    <Typography variant="subtitle2" sx={{ p: 1.5, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
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

                {(previewResult.settings || []).length > 0 && (
                  <Paper variant="outlined" sx={{ overflow: "hidden" }}>
                    <Typography variant="subtitle2" sx={{ p: 1.5, backgroundColor: "#F8FAFC", fontWeight: 600 }}>
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
                  <Paper variant="outlined" sx={{ p: 2, backgroundColor: "#0F172A", color: "#F8FAFC" }}>
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
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2, flex: 1 }}>
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
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
                {(savedSnippets || []).map((s) => (
                  <Paper
                    key={s.id}
                    variant="outlined"
                    sx={{
                      p: 2,
                      display: "flex",
                      flexDirection: { xs: "column", sm: "row" },
                      alignItems: { xs: "flex-start", sm: "center" },
                      justifyContent: "space-between",
                      gap: 1.5,
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
                      <IconButton size="small" color="error" onClick={() => handleDeleteSnippet(s.id)}>
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
          px: { xs: 2, sm: 3 },
          py: 2,
          borderTop: "1px solid #E2E8F0",
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          alignItems: { xs: "stretch", sm: "center" },
          justifyContent: "space-between",
          gap: 1.5,
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
            {applying ? "Applying..." : "Apply Changes"}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
};
