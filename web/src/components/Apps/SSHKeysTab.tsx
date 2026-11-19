import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  Paper,
  Radio,
  RadioGroup,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  Check,
  Copy,
  Edit,
  Eye,
  EyeOff,
  Fingerprint,
  Info,
  KeyRound,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import { api } from "../../api/client";
import { useMesh } from "../../context/MeshContext";
import type { Node, SSHKeyNodeState, SSHKeyRule, SSHKeyRuleType, SSHKeysResponse } from "../../types/api";
import {
  detectTargetMode,
  getSelectorValue,
  matchesTaskSelector,
  parseSelectorJSON,
  splitSelectorList,
} from "../../utils/taskSelector";
import type { SelectorTargetMode } from "../../utils/taskSelector";

const RULE_TYPE_OPTIONS: { value: SSHKeyRuleType; label: string; description: string }[] = [
  { value: "all", label: "All", description: "Both known_hosts and authorized_keys" },
  { value: "known_hosts", label: "known_hosts", description: "Trust destination host keys (no host key prompt)" },
  { value: "authorized_keys", label: "authorized_keys", description: "Allow source keys to log in as root" },
];

// Explicit "All nodes" marker shared with the server (config.AllNodesSelectorKey).
// An empty selector means "nothing chosen" and is never treated as "all".
const ALL_NODES_SELECTOR: Record<string, string> = { all: "true" };

export function isAllNodesSelector(selector: Record<string, string> | null | undefined): boolean {
  return !!selector && Object.keys(selector).some((k) => k.trim().toLowerCase() === "all");
}

/** Determines which editor mode should be shown for a stored selector. */
function selectorMode(selector: Record<string, string> | null | undefined): SelectorTargetMode {
  if (isAllNodesSelector(selector)) return "all";
  const mode = detectTargetMode(selector || {});
  // An empty selector is treated as "no selection" (specific nodes).
  return mode === "all" ? "manual" : mode;
}

const RULE_TYPE_COLORS: Record<SSHKeyRuleType, { bg: string; fg: string }> = {
  all: { bg: "#EEF2FF", fg: "#4F46E5" },
  known_hosts: { bg: "#ECFDF5", fg: "#047857" },
  authorized_keys: { bg: "#FFF7ED", fg: "#C2410C" },
};

// "custom" (raw JSON selector) is a fourth, separate editor mode next to the
// three presets exposed by detectTargetMode.
type SelectorMode = SelectorTargetMode;

/** Short, non-reversible fingerprint of a key blob (first 16 base64 chars of its SHA-256 is overkill here). */
export function shortKeyFingerprint(algorithm: string, key: string): string {
  const tail = key.length > 12 ? key.slice(-12) : key;
  return `${algorithm} …${tail}`;
}

function useCopy() {
  const [copied, setCopied] = useState<string | null>(null);
  const copy = useCallback(async (text: string, id: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(id);
      setTimeout(() => setCopied((cur) => (cur === id ? null : cur)), 1500);
    } catch {
      // Clipboard unavailable (insecure context); silently ignore.
    }
  }, []);
  return { copied, copy };
}

/** Rule editor dialog: type + source/destination node selectors. */
const SSHKeyRuleDialog: React.FC<{
  open: boolean;
  rule: SSHKeyRule | null;
  nodes: Node[];
  saving: boolean;
  onClose: () => void;
  onSave: (rule: Partial<SSHKeyRule>) => Promise<void>;
}> = ({ open, rule, nodes, saving, onClose, onSave }) => {
  const [type, setType] = useState<SSHKeyRuleType>("all");
  const [enabled, setEnabled] = useState(true);
  const [srcMode, setSrcMode] = useState<SelectorMode>("manual");
  const [srcNames, setSrcNames] = useState<string[]>([]);
  const [srcTag, setSrcTag] = useState("");
  const [dstMode, setDstMode] = useState<SelectorMode>("manual");
  const [dstNames, setDstNames] = useState<string[]>([]);
  const [dstTag, setDstTag] = useState("");
  const [customSrc, setCustomSrc] = useState("");
  const [customDst, setCustomDst] = useState("");
  const [error, setError] = useState<string | null>(null);

  const agentNodes = useMemo(() => nodes.filter((n) => !n.is_external), [nodes]);
  const nodeNames = useMemo(() => agentNodes.map((n) => n.name).sort(), [agentNodes]);
  const allTags = useMemo(() => {
    const tags = new Set<string>();
    agentNodes.forEach((n) => (n.tags || []).forEach((t) => t.trim() && tags.add(t.trim())));
    return Array.from(tags).sort();
  }, [agentNodes]);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setType((rule?.type as SSHKeyRuleType) || "all");
    setEnabled(rule ? rule.enabled : true);
    const src = rule?.src_selector || {};
    const dst = rule?.dst_selector || {};
    setSrcMode(selectorMode(src));
    setSrcNames(splitSelectorList(getSelectorValue(src, "name")));
    setSrcTag(getSelectorValue(src, "tag"));
    setDstMode(selectorMode(dst));
    setDstNames(splitSelectorList(getSelectorValue(dst, "name")));
    setDstTag(getSelectorValue(dst, "tag"));
    setCustomSrc(JSON.stringify(src, null, 2));
    setCustomDst(JSON.stringify(dst, null, 2));
  }, [open, rule]);

  // Builds the selector for a mode. Returns null on invalid input or when the
  // mode has no actual selection (empty is NOT silently treated as "all": the
  // "All nodes" mode writes the explicit ALL_NODES_SELECTOR marker instead).
  const buildSelector = (mode: SelectorTargetMode, names: string[], tag: string, custom: string): Record<string, string> | null => {
    if (mode === "all") return ALL_NODES_SELECTOR;
    if (mode === "tag") return tag.trim() ? { tag: tag.trim() } : null;
    if (mode === "manual") return names.length ? { name: names.join(",") } : null;
    const parsed = parseSelectorJSON(custom);
    if (parsed.error || !parsed.selector) return null;
    return Object.keys(parsed.selector).length ? parsed.selector : ALL_NODES_SELECTOR;
  };

  const handleSave = async () => {
    const src = buildSelector(srcMode, srcNames, srcTag, customSrc);
    const dst = buildSelector(dstMode, dstNames, dstTag, customDst);
    if (src === null) {
      if (srcMode === "tag") {
        setError("Enter a tag name for the source nodes, or pick another target mode.");
      } else if (srcMode === "manual") {
        setError("Select at least one source node, or choose \"All nodes\".");
      } else {
        setError("Source selector JSON is invalid.");
      }
      return;
    }
    if (dst === null) {
      if (dstMode === "tag") {
        setError("Enter a tag name for the destination nodes, or pick another target mode.");
      } else if (dstMode === "manual") {
        setError("Choose at least one destination node (or select \"All nodes\"). Keys are never distributed implicitly.");
      } else {
        setError("Destination selector JSON is invalid.");
      }
      return;
    }
    setError(null);
    await onSave({ type, src_selector: src, dst_selector: dst, enabled });
  };

  const renderSelectorEditor = (
    title: string,
    hint: string,
    mode: SelectorMode,
    setMode: (m: SelectorMode) => void,
    names: string[],
    setNames: (n: string[]) => void,
    tag: string,
    setTag: (t: string) => void,
    custom: string,
    setCustom: (t: string) => void,
  ) => (
    <Box sx={{ border: "1px solid #E2E8F0", borderRadius: "12px", p: 1.5, bgcolor: "#FCFCFD" }}>
      <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#1E293B", mb: 0.3 }}>
        {title}
      </Typography>
      <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1 }}>
        {hint}
      </Typography>
      <Tabs
        value={mode}
        onChange={(_, v) => setMode(v)}
        sx={{
          minHeight: 34,
          mb: 1.2,
          "& .MuiTab-root": { textTransform: "none", fontWeight: 700, fontSize: "0.78rem", minHeight: 34, px: 1.5 },
          "& .Mui-selected": { color: "#4F46E5" },
          "& .MuiTabs-indicator": { bgcolor: "#4F46E5" },
        }}
      >
        <Tab value="manual" label="Specific nodes" />
        <Tab value="tag" label="By tag" />
        <Tab value="all" label="All nodes" />
        <Tab value="custom" label="Custom JSON" />
      </Tabs>

      {mode === "manual" && (
        <Autocomplete
          multiple
          size="small"
          options={nodeNames}
          value={names}
          onChange={(_, v) => setNames(v)}
          renderInput={(params) => <TextField {...params} placeholder="Select nodes" />}
          renderTags={(value, getTagProps) =>
            value.map((option, index) => {
              const { key, ...tagProps } = getTagProps({ index });
              return <Chip key={key} size="small" label={option} {...tagProps} />;
            })
          }
        />
      )}
      {mode === "tag" && (
        <Autocomplete
          freeSolo
          size="small"
          options={allTags}
          value={tag}
          onInputChange={(_, v) => setTag(v)}
          renderInput={(params) => <TextField {...params} placeholder="Tag name, e.g. core" />}
        />
      )}
      {mode === "all" && (
        <Typography variant="caption" sx={{ color: "#475569" }}>
          Matches every managed cluster node.
        </Typography>
      )}
      {mode === "custom" && (
        <TextField
          fullWidth
          multiline
          minRows={3}
          size="small"
          value={custom}
          onChange={(e) => setCustom(e.target.value)}
          placeholder='{"tag": "core", "env:NODE_IP": "192.168.100.1"}'
          inputProps={{ style: { fontFamily: "monospace", fontSize: "0.8rem" } }}
        />
      )}
    </Box>
  );

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth PaperProps={{ sx: { borderRadius: "16px" } }}>
      <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1, pb: 1 }}>
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
          <KeyRound size={20} />
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 800 }}>
          {rule ? "Edit SSH Key Rule" : "Add SSH Key Rule"}
        </Typography>
        <Box sx={{ flex: 1 }} />
        <IconButton size="small" onClick={onClose}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 1.5, pt: 1 }}>
        <Box>
          <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#1E293B", mb: 0.5 }}>
            Target files
          </Typography>
          <RadioGroup row value={type} onChange={(e) => setType(e.target.value as SSHKeyRuleType)}>
            {RULE_TYPE_OPTIONS.map((opt) => (
              <FormControlLabel
                key={opt.value}
                value={opt.value}
                control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                label={
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 700, lineHeight: 1.2 }}>
                      {opt.label}
                    </Typography>
                    <Typography variant="caption" sx={{ color: "#64748B" }}>
                      {opt.description}
                    </Typography>
                  </Box>
                }
                sx={{ mr: 3, alignItems: "flex-start", "& .MuiRadio-root": { pt: 0.4 } }}
              />
            ))}
          </RadioGroup>
        </Box>

        <Divider />

        {renderSelectorEditor(
          "Source nodes",
          "Public keys of the matching nodes are installed on the destinations.",
          srcMode,
          setSrcMode,
          srcNames,
          setSrcNames,
          srcTag,
          setSrcTag,
          customSrc,
          setCustomSrc,
        )}
        {renderSelectorEditor(
          "Destination nodes",
          "The known_hosts / authorized_keys files of the matching nodes are updated.",
          dstMode,
          setDstMode,
          dstNames,
          setDstNames,
          dstTag,
          setDstTag,
          customDst,
          setCustomDst,
        )}

        <FormControlLabel
          control={<Switch checked={enabled} onChange={(e) => setEnabled(e.target.checked)} color="primary" />}
          label={
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              Rule enabled
            </Typography>
          }
        />

        {error && <Alert severity="error">{error}</Alert>}
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose} sx={{ textTransform: "none", color: "#64748B" }}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSave}
          disabled={saving}
          sx={{ bgcolor: "#4F46E5", fontWeight: 600, textTransform: "none", "&:hover": { bgcolor: "#4338CA" } }}
        >
          {saving ? <CircularProgress size={16} /> : rule ? "Save Changes" : "Create Rule"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export const SSHKeysTab: React.FC = () => {
  const { nodes, setStateToast } = useMesh();
  const { copied, copy } = useCopy();

  const [data, setData] = useState<SSHKeysResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [busyRuleId, setBusyRuleId] = useState<string | null>(null);
  const [revealedKeys, setRevealedKeys] = useState<Record<string, boolean>>({});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<SSHKeyRule | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleteRule, setDeleteRule] = useState<SSHKeyRule | null>(null);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      const res = await api.getSSHKeys();
      setData(res);
      setError(null);
    } catch (e: any) {
      setError(e.message || "Failed to load SSH keys");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const nodeMap = useMemo(() => {
    const map = new Map<string, Node>();
    nodes.forEach((n) => map.set(n.name, n));
    return map;
  }, [nodes]);

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      const res = await api.refreshSSHKeys();
      await load(true);
      setStateToast({
        message: `SSH public keys refreshed on ${res.refreshed?.length ?? 0} node(s).`,
        severity: res.refreshed?.length ? "success" : "info",
      });
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to refresh SSH keys", severity: "error" });
    } finally {
      setRefreshing(false);
    }
  };

  const handleSaveRule = async (payload: Partial<SSHKeyRule>) => {
    setSaving(true);
    try {
      if (editingRule) {
        await api.updateSSHKeyRule(editingRule.id, payload);
      } else {
        await api.createSSHKeyRule(payload);
      }
      setDialogOpen(false);
      setEditingRule(null);
      await load(true);
      setStateToast({ message: editingRule ? "SSH key rule updated." : "SSH key rule created.", severity: "success" });
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to save SSH key rule", severity: "error" });
    } finally {
      setSaving(false);
    }
  };

  const handleToggleRule = async (rule: SSHKeyRule) => {
    setBusyRuleId(rule.id);
    try {
      await api.toggleSSHKeyRule(rule.id);
      await load(true);
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to toggle SSH key rule", severity: "error" });
    } finally {
      setBusyRuleId(null);
    }
  };

  const handleDeleteRule = async () => {
    if (!deleteRule) return;
    const rule = deleteRule;
    setDeleteRule(null);
    setBusyRuleId(rule.id);
    try {
      await api.deleteSSHKeyRule(rule.id);
      await load(true);
      setStateToast({ message: "SSH key rule deleted.", severity: "success" });
    } catch (e: any) {
      setStateToast({ message: e.message || "Failed to delete SSH key rule", severity: "error" });
    } finally {
      setBusyRuleId(null);
    }
  };

  const describeSelector = (selector: Record<string, string>): string => {
    if (isAllNodesSelector(selector)) return "All nodes";
    const entries = Object.entries(selector || {});
    if (entries.length === 0) return "—";
    return entries.map(([k, v]) => `${k}: ${v}`).join(" · ");
  };

  const matchedNames = (selector: Record<string, string>): string[] => {
    if (Object.keys(selector || {}).length === 0) return [];
    return nodes
      .filter((n) => !n.is_external && matchesTaskSelector(n, isAllNodesSelector(selector) ? {} : selector))
      .map((n) => n.name);
  };

  const renderKeyRow = (node: SSHKeyNodeState, keyIndex: number) => {
    const key = node.keys[keyIndex];
    const id = `${node.name}-${keyIndex}`;
    const revealed = !!revealedKeys[id];
    const short = shortKeyFingerprint(key.algorithm, key.key);
    return (
      <TableRow key={id} sx={{ "&:hover": { bgcolor: "#F8FAFC" }, "& td": { borderColor: "#F1F5F9", py: 0.9 } }}>
        <TableCell sx={{ fontWeight: 700, color: "#0F172A", whiteSpace: "nowrap" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
            {nodeMap.get(node.name)?.mode === "agent" && node.connected ? (
              <Box sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: "#10B981" }} />
            ) : (
              <Box sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: "#CBD5E1" }} />
            )}
            {node.name}
          </Box>
        </TableCell>
        <TableCell>
          <Chip
            size="small"
            label={key.algorithm}
            sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#F1F5F9", color: "#334155" }}
          />
        </TableCell>
        <TableCell>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Typography component="span" sx={{ fontFamily: "monospace", fontSize: "0.78rem", color: "#334155", wordBreak: "break-all" }}>
              {revealed ? `${key.algorithm} ${key.key}` : short}
            </Typography>
            <Tooltip title={revealed ? "Hide full key" : "Show full key"}>
              <IconButton
                size="small"
                onClick={() => setRevealedKeys((prev) => ({ ...prev, [id]: !prev[id] }))}
                sx={{ color: "#94A3B8" }}
              >
                {revealed ? <EyeOff size={14} /> : <Eye size={14} />}
              </IconButton>
            </Tooltip>
            <Tooltip title={copied === id ? "Copied!" : "Copy full public key"}>
              <IconButton
                size="small"
                onClick={() => copy(`${key.algorithm} ${key.key}${key.comment ? ` ${key.comment}` : ""}`, id)}
                sx={{ color: copied === id ? "#10B981" : "#94A3B8" }}
              >
                {copied === id ? <Check size={14} /> : <Copy size={14} />}
              </IconButton>
            </Tooltip>
          </Box>
        </TableCell>
        <TableCell>
          <Typography variant="caption" sx={{ color: "#64748B" }}>
            {key.comment || "—"}
          </Typography>
        </TableCell>
        <TableCell>
          <Typography variant="caption" sx={{ color: "#94A3B8", fontFamily: "monospace" }}>
            {key.path || "—"}
          </Typography>
        </TableCell>
      </TableRow>
    );
  };

  const summary = data?.summary;
  // The API always returns arrays, but tolerate null/missing fields from older
  // servers so the tab never crashes on unexpected payloads.
  const keyRows = (data?.nodes || []).flatMap((n) =>
    (Array.isArray(n.keys) ? n.keys : []).map((_, i) => ({ node: n, i })),
  );
  const unsupportedNodes = (data?.nodes || []).filter((n) => !n.supported);
  const rules = data?.rules || [];

  return (
    <Box>
      {/* Guide banner */}
      <Paper
        elevation={0}
        sx={{
          p: 1.2,
          mb: 3,
          borderRadius: "14px",
          bgcolor: "#EEF2FF",
          border: "1px solid #C7D2FE",
          display: "flex",
          alignItems: "flex-start",
          gap: 1,
        }}
      >
        <Box sx={{ color: "#4F46E5", mt: 0.2 }}>
          <Info size={22} />
        </Box>
        <Box>
          <Typography variant="subtitle2" sx={{ fontWeight: 800, color: "#312E81", mb: 0.5 }}>
            Cluster-wide SSH key management (agent mode)
          </Typography>
          <Typography variant="body2" sx={{ color: "#4338CA", fontSize: "0.85rem", lineHeight: 1.5 }}>
            Each agent collects root's OpenSSH default identity public keys (<code>~/.ssh/id_*.pub</code>). Add rules to
            install the keys of the source nodes into the <code>/root/.ssh/known_hosts</code> and/or{" "}
            <code>/root/.ssh/authorized_keys</code> files of the destination nodes, so root can SSH between cluster nodes
            without prompts. Changes apply on the next sync. Only the Easy42-managed block of these files is touched;
            other lines are preserved.
          </Typography>
        </Box>
      </Paper>

      {/* Stats */}
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 2, mb: 3 }}>
        {[
          { label: "Nodes with keys", value: summary?.nodes_with_keys ?? 0, color: "#4F46E5", bg: "#EEF2FF" },
          { label: "Collected keys", value: summary?.total_keys ?? 0, color: "#0F766E", bg: "#CCFBF1" },
          { label: "Rules", value: summary?.rule_count ?? 0, color: "#B45309", bg: "#FEF3C7" },
          { label: "Enabled rules", value: summary?.enabled_rules ?? 0, color: "#047857", bg: "#D1FAE5" },
        ].map((stat) => (
          <Paper
            key={stat.label}
            elevation={0}
            sx={{ p: 2, borderRadius: "12px", border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}
          >
            <Typography variant="h5" sx={{ fontWeight: 800, color: stat.color, lineHeight: 1.1 }}>
              {stat.value}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, bgcolor: stat.bg, px: 0.8, py: 0.2, borderRadius: "6px" }}>
              {stat.label}
            </Typography>
          </Paper>
        ))}
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          {error}
        </Alert>
      )}

      {/* Collected keys */}
      <Paper elevation={0} sx={{ borderRadius: "14px", border: "1px solid #E2E8F0", bgcolor: "#FFFFFF", mb: 3 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 2, pb: 1 }}>
          <Fingerprint size={18} style={{ color: "#4F46E5" }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#1E293B" }}>
            Collected Public Keys
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
            {keyRows.length} key(s)
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Tooltip title="Ask all connected supporting agents to re-read their ~/.ssh/*.pub files">
            <Button
              size="small"
              startIcon={refreshing ? <CircularProgress size={14} /> : <RefreshCw size={15} />}
              onClick={handleRefresh}
              disabled={refreshing}
              sx={{ textTransform: "none", fontWeight: 700, color: "#4F46E5" }}
            >
              Refresh
            </Button>
          </Tooltip>
        </Box>

        {loading ? (
          <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
            <CircularProgress size={28} sx={{ color: "#4F46E5" }} />
          </Box>
        ) : keyRows.length === 0 ? (
          <Box sx={{ py: 6, px: 3, textAlign: "center" }}>
            <Box
              sx={{
                width: 52,
                height: 52,
                borderRadius: "50%",
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                mx: "auto",
                mb: 1,
              }}
            >
              <KeyRound size={26} />
            </Box>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
              No public keys collected
            </Typography>
            <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 520, mx: "auto" }}>
              Keys appear here once agent-mode nodes with agent v0.1.18+ report their <code>~/.ssh/id_*.pub</code> files.
            </Typography>
          </Box>
        ) : (
          <TableContainer sx={{ overflowX: "auto" }}>
            <Table sx={{ minWidth: 760 }}>
              <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>NODE</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>TYPE</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>PUBLIC KEY</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>COMMENT</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>SOURCE FILE</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>{keyRows.map((r) => renderKeyRow(r.node, r.i))}</TableBody>
            </Table>
          </TableContainer>
        )}

        {unsupportedNodes.length > 0 && (
          <Box sx={{ p: 2, borderTop: "1px solid #F1F5F9" }}>
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B" }}>
              Nodes not collecting keys:
            </Typography>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.8, mt: 0.8 }}>
              {unsupportedNodes.map((n) => (
                <Tooltip key={n.name} title={n.reason || "Not supported"}>
                  <Chip
                    size="small"
                    icon={<ShieldCheck size={12} />}
                    label={n.name}
                    sx={{ bgcolor: "#F8FAFC", color: "#64748B", fontWeight: 600, fontSize: "0.72rem" }}
                  />
                </Tooltip>
              ))}
            </Box>
          </Box>
        )}
      </Paper>

      {/* Rules */}
      <Paper elevation={0} sx={{ borderRadius: "14px", border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 2, pb: 1 }}>
          <ShieldCheck size={18} style={{ color: "#4F46E5" }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#1E293B" }}>
            Distribution Rules
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
            {rules.length} rule(s)
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Button
            size="small"
            variant="contained"
            startIcon={<Plus size={16} />}
            onClick={() => {
              setEditingRule(null);
              setDialogOpen(true);
            }}
            sx={{ bgcolor: "#4F46E5", fontWeight: 700, textTransform: "none", borderRadius: "10px", "&:hover": { bgcolor: "#4338CA" } }}
          >
            Add Rule
          </Button>
        </Box>

        {rules.length === 0 && !loading ? (
          <Box sx={{ py: 6, px: 3, textAlign: "center" }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
              No rules configured
            </Typography>
            <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 520, mx: "auto", mb: 2 }}>
              Public keys are never distributed automatically. Add a rule to explicitly choose which nodes' keys are
              trusted by which other nodes.
            </Typography>
            <Button
              variant="outlined"
              startIcon={<Plus size={16} />}
              onClick={() => {
                setEditingRule(null);
                setDialogOpen(true);
              }}
              sx={{ textTransform: "none", fontWeight: 700, borderColor: "#4F46E5", color: "#4F46E5", borderRadius: "10px" }}
            >
              Add First Rule
            </Button>
          </Box>
        ) : rules.length > 0 ? (
          <TableContainer sx={{ overflowX: "auto" }}>
            <Table sx={{ minWidth: 860 }}>
              <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 800, color: "#475569", width: 150 }}>TYPE</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>SOURCE NODES</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>DESTINATION NODES</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569", width: 120 }}>ENABLED</TableCell>
                  <TableCell align="right" sx={{ fontWeight: 800, color: "#475569", width: 120 }}>
                    ACTIONS
                  </TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {rules.map((rule) => {
                  const colors = RULE_TYPE_COLORS[rule.type] || RULE_TYPE_COLORS.all;
                  const srcs = matchedNames(rule.src_selector);
                  const dsts = matchedNames(rule.dst_selector);
                  const busy = busyRuleId === rule.id;
                  return (
                    <TableRow key={rule.id} sx={{ "&:hover": { bgcolor: "#F8FAFC" }, "& td": { borderColor: "#F1F5F9" } }}>
                      <TableCell>
                        <Chip
                          size="small"
                          label={rule.type}
                          sx={{ fontWeight: 700, fontSize: "0.72rem", bgcolor: colors.bg, color: colors.fg }}
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", fontSize: "0.82rem" }}>
                          {describeSelector(rule.src_selector)}
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          {srcs.length ? `${srcs.length} node(s): ${srcs.join(", ")}` : "matches no node"}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155", fontSize: "0.82rem" }}>
                          {describeSelector(rule.dst_selector)}
                        </Typography>
                        <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                          {dsts.length ? `${dsts.length} node(s): ${dsts.join(", ")}` : "matches no node"}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Switch
                          size="small"
                          checked={rule.enabled}
                          disabled={busy}
                          onChange={() => handleToggleRule(rule)}
                        />
                      </TableCell>
                      <TableCell align="right">
                        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.5 }}>
                          <Tooltip title="Edit rule">
                            <IconButton
                              size="small"
                              onClick={() => {
                                setEditingRule(rule);
                                setDialogOpen(true);
                              }}
                              sx={{ color: "#64748B" }}
                            >
                              <Edit size={16} />
                            </IconButton>
                          </Tooltip>
                          <Tooltip title="Delete rule">
                            <IconButton size="small" onClick={() => setDeleteRule(rule)} sx={{ color: "#EF4444" }}>
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
        ) : null}
      </Paper>

      <SSHKeyRuleDialog
        open={dialogOpen}
        rule={editingRule}
        nodes={nodes}
        saving={saving}
        onClose={() => {
          setDialogOpen(false);
          setEditingRule(null);
        }}
        onSave={handleSaveRule}
      />

      <Dialog open={!!deleteRule} onClose={() => setDeleteRule(null)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ fontWeight: 700 }}>Delete SSH Key Rule</DialogTitle>
        <DialogContent>
          <Typography variant="body2" sx={{ color: "#475569" }}>
            Delete this <strong>{deleteRule?.type}</strong> rule from{" "}
            <strong>{deleteRule ? describeSelector(deleteRule.src_selector) : ""}</strong> to{" "}
            <strong>{deleteRule ? describeSelector(deleteRule.dst_selector) : ""}</strong>? Keys that are no longer
            granted by any rule are removed from the destination key files on the next sync.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDeleteRule(null)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button variant="contained" color="error" onClick={handleDeleteRule} sx={{ textTransform: "none", fontWeight: 600 }}>
            Delete Rule
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
