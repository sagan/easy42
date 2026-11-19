import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
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
  InputAdornment,
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
  TextField,
  ToggleButton,
  ToggleButtonGroup,
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
  Search,
  ShieldCheck,
  Trash2,
  X,
} from "lucide-react";
import { api } from "../../api/client";
import { useMesh } from "../../context/MeshContext";
import type { Node, SSHKeyNodeState, SSHKeyRule, SSHKeyRuleType, SSHKeysResponse } from "../../types/api";
import { matchesTaskSelector } from "../../utils/taskSelector";
import { isAllNodesSelector, NodeSelector } from "../Common/NodeSelector";
import { KeyFingerprints, sshKeyFingerprints } from "../../utils/sshFingerprint";

const RULE_TYPE_OPTIONS: { value: SSHKeyRuleType; label: string; description: string }[] = [
  {
    value: "all",
    label: "All",
    description: "Both files: SSH server host keys into known_hosts, root login keys into authorized_keys",
  },
  {
    value: "known_hosts",
    label: "known_hosts",
    description:
      "Add the source nodes' SSH server host keys to the destinations, so the destinations can verify those nodes when connecting to them",
  },
  {
    value: "authorized_keys",
    label: "authorized_keys",
    description: "Allow the source nodes' root login keys to log in to the destinations as root",
  },
];

const RULE_TYPE_COLORS: Record<SSHKeyRuleType, { bg: string; fg: string }> = {
  all: { bg: "#EEF2FF", fg: "#4F46E5" },
  known_hosts: { bg: "#ECFDF5", fg: "#047857" },
  authorized_keys: { bg: "#FFF7ED", fg: "#C2410C" },
};

/** Short, non-reversible fingerprint of a key blob (first 16 base64 chars of its SHA-256 is overkill here). */
export function shortKeyFingerprint(algorithm: string, key: string): string {
  const tail = key.length > 12 ? key.slice(-12) : key;
  return `${algorithm} …${tail}`;
}

/** SSH server host keys (installed into known_hosts); anything else is a root login identity. */
export function isHostKey(key: { kind?: string }): boolean {
  return (key.kind || "").trim().toLowerCase() === "host";
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
  const [srcSelector, setSrcSelector] = useState<Record<string, string>>({});
  const [dstSelector, setDstSelector] = useState<Record<string, string>>({});
  const [selectorError, setSelectorError] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) return;
    setError(null);
    setSelectorError(null);
    setType((rule?.type as SSHKeyRuleType) || "all");
    setEnabled(rule ? rule.enabled : true);
    setSrcSelector(rule?.src_selector || {});
    setDstSelector(rule?.dst_selector || {});
  }, [open, rule]);

  // Both sides must be chosen explicitly: an empty selector matches no node, so
  // "every node" requires the explicit {"all": "1"} selector from the All nodes tab.
  const handleSave = async () => {
    if (Object.keys(srcSelector).length === 0) {
      setError('Select at least one source node, a tag, or "All nodes".');
      return;
    }
    if (Object.keys(dstSelector).length === 0) {
      setError('Choose at least one destination node (or select "All nodes"). Keys are never distributed implicitly.');
      return;
    }
    setError(null);
    await onSave({ type, src_selector: srcSelector, dst_selector: dstSelector, enabled });
  };

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

        <NodeSelector
          compact
          label="Source nodes"
          description="The nodes whose keys are distributed: their SSH server host keys (known_hosts) and/or root login keys (authorized_keys)."
          nodes={nodes}
          value={srcSelector}
          onChange={setSrcSelector}
          onErrorChange={setSelectorError}
        />
        <NodeSelector
          compact
          label="Destination nodes"
          description="The nodes whose known_hosts / authorized_keys files are updated."
          nodes={nodes}
          value={dstSelector}
          onChange={setDstSelector}
          onErrorChange={setSelectorError}
        />

        <FormControlLabel
          control={<Switch checked={enabled} onChange={(e) => setEnabled(e.target.checked)} color="primary" />}
          label={
            <Typography variant="body2" sx={{ fontWeight: 600 }}>
              Rule enabled
            </Typography>
          }
        />

        {selectorError && <Alert severity="warning">{selectorError}</Alert>}
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
  // Fingerprints are computed on reveal (async, via WebCrypto) and cached per
  // key id, so re-opening a revealed key does not recompute or flicker.
  const [keyFingerprints, setKeyFingerprints] = useState<Record<string, KeyFingerprints | "loading">>({});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<SSHKeyRule | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleteRule, setDeleteRule] = useState<SSHKeyRule | null>(null);

  // Collected-keys filters. The table can be very long on big clusters, so it is
  // filtered by key role and a free-text search over node, key and comment.
  const [keyRoleFilter, setKeyRoleFilter] = useState<"all" | "user" | "host">("all");
  const [keySearch, setKeySearch] = useState("");

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
    return nodes.filter((n) => !n.is_external && matchesTaskSelector(n, selector)).map((n) => n.name);
  };

  // Computes and caches the MD5/SHA1/SHA256 fingerprints of a key the first time
  // it is revealed. Hashing is async (WebCrypto); MD5 is done in TS because
  // WebCrypto has no MD5.
  const pendingFingerprints = useRef<Set<string>>(new Set());
  const ensureFingerprints = useCallback(async (id: string, algorithm: string, blob: string) => {
    // A ref (not state) tracks in-flight work so a double click cannot start the
    // same hash twice, independently of React batching/StrictMode replay.
    if (pendingFingerprints.current.has(id)) return;
    pendingFingerprints.current.add(id);
    try {
      const fp = await sshKeyFingerprints(`${algorithm} ${blob}`);
      setKeyFingerprints((prev) => ({ ...prev, [id]: fp }));
    } catch {
      setKeyFingerprints((prev) => {
        const next = { ...prev };
        delete next[id];
        return next;
      });
    }
  }, []);

  const toggleReveal = (id: string, algorithm: string, blob: string) => {
    const nextRevealed = !revealedKeys[id];
    setRevealedKeys((prev) => ({ ...prev, [id]: nextRevealed }));
    if (nextRevealed) void ensureFingerprints(id, algorithm, blob);
  };

  const renderFingerprintLines = (entry: KeyFingerprints | "loading" | undefined) => {
    if (!entry || entry === "loading") {
      return (
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, mt: 0.4 }}>
          <CircularProgress size={10} />
          <Typography component="span" sx={{ fontFamily: "monospace", fontSize: "0.7rem", color: "#94A3B8" }}>
            computing fingerprints…
          </Typography>
        </Box>
      );
    }
    // `paste` is what OpenSSH accepts at the
    // "Are you sure you want to continue connecting (yes/no/[fingerprint])?" prompt.
    // Only the exact `SHA256:<base64>` form is accepted there; the sha1/md5
    // variants copy their own (displayed) value, which is useful when comparing
    // against other tools but is not a valid answer to that prompt.
    const lines: { text: string; paste: string; promptable: boolean }[] = [];
    if (entry.sha256) {
      const paste = `SHA256:${entry.sha256.base64}`;
      lines.push({ text: `sha256:${entry.sha256.base64} (${entry.sha256.hex})`, paste, promptable: true });
    }
    if (entry.sha1) {
      lines.push({
        text: `sha1:${entry.sha1.base64} (${entry.sha1.hex})`,
        paste: `SHA1:${entry.sha1.base64}`,
        promptable: false,
      });
    }
    if (entry.md5) {
      // OpenSSH's MD5 form is colon-separated, uppercase: aa:bb:cc:...
      const colonHex = (entry.md5.hex.match(/.{2}/g) || []).join(":").toUpperCase();
      lines.push({ text: `md5:${colonHex}`, paste: `MD5:${colonHex}`, promptable: false });
    }
    return (
      <Box sx={{ mt: 0.4, display: "flex", flexDirection: "column", gap: 0.1 }}>
        {lines.map((line) => {
          const copyId = `fp-${line.paste}`;
          const done = copied === copyId;
          return (
            <Box key={line.text} sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Typography
                component="span"
                sx={{ fontFamily: "monospace", fontSize: "0.7rem", color: "#64748B", wordBreak: "break-all", flex: 1 }}
              >
                {line.text}
              </Typography>
              <Tooltip
                title={
                  done
                    ? "Copied!"
                    : line.promptable
                      ? "Copy — pasteable at the ssh host key confirmation prompt"
                      : "Copy fingerprint"
                }
              >
                <IconButton
                  size="small"
                  onClick={(e) => {
                    e.stopPropagation();
                    copy(line.paste, copyId);
                  }}
                  sx={{ color: done ? "#10B981" : "#CBD5E1", p: 0.2 }}
                >
                  {done ? <Check size={12} /> : <Copy size={12} />}
                </IconButton>
              </Tooltip>
            </Box>
          );
        })}
      </Box>
    );
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
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.6 }}>
            <Chip
              size="small"
              label={key.algorithm}
              sx={{ height: 20, fontSize: "0.7rem", fontWeight: 700, bgcolor: "#F1F5F9", color: "#334155" }}
            />
            <Chip
              size="small"
              label={isHostKey(key) ? "host" : "user"}
              title={
                isHostKey(key)
                  ? "SSH server host key (/etc/ssh/ssh_host_*.pub) — installed into known_hosts"
                  : "Root login identity (~/.ssh/id_*.pub) — installed into authorized_keys"
              }
              sx={{
                height: 20,
                fontSize: "0.66rem",
                fontWeight: 700,
                bgcolor: isHostKey(key) ? "#ECFDF5" : "#FFF7ED",
                color: isHostKey(key) ? "#047857" : "#C2410C",
              }}
            />
          </Box>
        </TableCell>
        <TableCell>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <Typography
              component="span"
              sx={{ fontFamily: "monospace", fontSize: "0.78rem", color: "#334155", wordBreak: "break-all" }}
            >
              {revealed ? `${key.algorithm} ${key.key}` : short}
            </Typography>
            <Tooltip title={revealed ? "Hide full key" : "Show full key and fingerprints"}>
              <IconButton
                size="small"
                onClick={() => toggleReveal(id, key.algorithm, key.key)}
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
          {revealed && renderFingerprintLines(keyFingerprints[id])}
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
  const keyRows = useMemo(
    () => (data?.nodes || []).flatMap((n) => (Array.isArray(n.keys) ? n.keys : []).map((_, i) => ({ node: n, i }))),
    [data],
  );
  const unsupportedNodes = (data?.nodes || []).filter((n) => !n.supported);
  const rules = data?.rules || [];

  // Role counts are computed from the unfiltered set so the toggle always shows
  // how many of each kind exist.
  const roleCounts = useMemo(() => {
    let user = 0;
    let host = 0;
    keyRows.forEach(({ node, i }) => {
      if (isHostKey(node.keys[i])) host += 1;
      else user += 1;
    });
    return { all: user + host, user, host };
  }, [keyRows]);

  const filteredKeyRows = useMemo(() => {
    const query = keySearch.trim().toLowerCase();
    return keyRows.filter(({ node, i }) => {
      const key = node.keys[i];
      if (keyRoleFilter === "user" && isHostKey(key)) return false;
      if (keyRoleFilter === "host" && !isHostKey(key)) return false;
      if (!query) return true;
      return (
        node.name.toLowerCase().includes(query) ||
        key.key.toLowerCase().includes(query) ||
        (key.comment || "").toLowerCase().includes(query) ||
        (key.path || "").toLowerCase().includes(query)
      );
    });
  }, [keyRows, keyRoleFilter, keySearch]);

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
            Each agent collects two kinds of public keys: root's login identities (<code>~/.ssh/id_*.pub</code>) and the
            node&apos;s SSH <strong>server</strong> host keys (<code>/etc/ssh/ssh_host_*_key.pub</code> on OpenSSH,{" "}
            <code>/etc/dropbear/dropbear_*_host_key</code> on Dropbear/OpenWrt). Rules install the source nodes&apos;
            host keys into <code>/root/.ssh/known_hosts</code> (so destinations can verify them when connecting) and
            their login keys into <code>/root/.ssh/authorized_keys</code> (so they can log in as root). Changes apply on
            the next sync. Only the Easy42-managed block of these files is touched; other lines are preserved.
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
            <Typography
              variant="caption"
              sx={{ color: "#64748B", fontWeight: 600, bgcolor: stat.bg, px: 0.8, py: 0.2, borderRadius: "6px" }}
            >
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

      {/* Rules (first: they are what operators configure; the key list can be long) */}
      <Paper elevation={0} sx={{ borderRadius: "14px", border: "1px solid #E2E8F0", bgcolor: "#FFFFFF", mb: 3 }}>
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
            sx={{
              bgcolor: "#4F46E5",
              fontWeight: 700,
              textTransform: "none",
              borderRadius: "10px",
              "&:hover": { bgcolor: "#4338CA" },
            }}
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
              sx={{
                textTransform: "none",
                fontWeight: 700,
                borderColor: "#4F46E5",
                color: "#4F46E5",
                borderRadius: "10px",
              }}
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
                    <TableRow
                      key={rule.id}
                      sx={{ "&:hover": { bgcolor: "#F8FAFC" }, "& td": { borderColor: "#F1F5F9" } }}
                    >
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

      {/* Collected keys (filterable: the table can be long on large clusters) */}
      <Paper elevation={0} sx={{ borderRadius: "14px", border: "1px solid #E2E8F0", bgcolor: "#FFFFFF" }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, p: 2, pb: 1, flexWrap: "wrap" }}>
          <Fingerprint size={18} style={{ color: "#4F46E5" }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 800, color: "#1E293B" }}>
            Collected Public Keys
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
            {filteredKeyRows.length === keyRows.length
              ? `${keyRows.length} key(s)`
              : `${filteredKeyRows.length} of ${keyRows.length} key(s)`}
          </Typography>
          <Box sx={{ flex: 1 }} />
          <Tooltip title="Ask all connected supporting agents to re-read their ~/.ssh/*.pub, /etc/ssh/ssh_host_*_key.pub and /etc/dropbear/dropbear_*_host_key files">
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

        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, px: 2, pb: 1.5, flexWrap: "wrap" }}>
          <ToggleButtonGroup
            size="small"
            exclusive
            value={keyRoleFilter}
            onChange={(_, value) => {
              if (value !== null) setKeyRoleFilter(value);
            }}
            sx={{
              "& .MuiToggleButton-root": {
                textTransform: "none",
                fontWeight: 700,
                fontSize: "0.75rem",
                px: 1.4,
                py: 0.4,
                borderColor: "#E2E8F0",
                color: "#475569",
                "&.Mui-selected": { bgcolor: "#EEF2FF", color: "#4F46E5", borderColor: "#C7D2FE" },
              },
            }}
          >
            <ToggleButton value="all">All ({roleCounts.all})</ToggleButton>
            <ToggleButton value="user">User ({roleCounts.user})</ToggleButton>
            <ToggleButton value="host">Host ({roleCounts.host})</ToggleButton>
          </ToggleButtonGroup>

          <TextField
            size="small"
            placeholder="Filter by node, key or comment…"
            value={keySearch}
            onChange={(e) => setKeySearch(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={15} />
                </InputAdornment>
              ),
              endAdornment: keySearch ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={() => setKeySearch("")} sx={{ color: "#94A3B8" }}>
                    <X size={14} />
                  </IconButton>
                </InputAdornment>
              ) : undefined,
            }}
            sx={{ ml: "auto", width: { xs: "100%", sm: 300 } }}
          />
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
              Keys appear here once agent-mode nodes with agent v0.1.19+ report their <code>~/.ssh/id_*.pub</code>,{" "}
              <code>/etc/ssh/ssh_host_*_key.pub</code> and <code>/etc/dropbear/dropbear_*_host_key</code> files.
            </Typography>
          </Box>
        ) : filteredKeyRows.length === 0 ? (
          <Box sx={{ py: 5, px: 3, textAlign: "center" }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
              No keys match the current filter
            </Typography>
            <Button
              size="small"
              onClick={() => {
                setKeySearch("");
                setKeyRoleFilter("all");
              }}
              sx={{ textTransform: "none", fontWeight: 700, color: "#4F46E5", mt: 1 }}
            >
              Clear filters
            </Button>
          </Box>
        ) : (
          <TableContainer sx={{ overflowX: "auto" }}>
            <Table sx={{ minWidth: 760 }}>
              <TableHead sx={{ bgcolor: "#F8FAFC" }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>NODE</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>KEY</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569", minWidth: 200 }}>PUBLIC KEY</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>COMMENT</TableCell>
                  <TableCell sx={{ fontWeight: 800, color: "#475569" }}>SOURCE FILE</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>{filteredKeyRows.map((r) => renderKeyRow(r.node, r.i))}</TableBody>
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
          <Button
            variant="contained"
            color="error"
            onClick={handleDeleteRule}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            Delete Rule
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
