import React, { useState } from "react";
import {
  Box,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  MenuItem,
  TextField,
  Tooltip,
  Typography,
} from "@mui/material";
import { Edit2, Plus, Trash2, X } from "lucide-react";
import type { DNSCustomRecordRule, DNSCustomRecordType, Node } from "../../types/api";
import { ALL_NODES_SELECTOR, isAllNodesSelector, NodeSelector, resolveSelector } from "../Common/NodeSelector";

interface DNSCustomRecordsSectionProps {
  rules: DNSCustomRecordRule[];
  onChange: (rules: DNSCustomRecordRule[]) => void;
  nodes: Node[];
  disabled?: boolean;
}

const RECORD_TYPES: DNSCustomRecordType[] = ["A", "AAAA", "CNAME", "TXT", "SRV"];

const TYPE_COLORS: Record<DNSCustomRecordType, { bg: string; color: string }> = {
  A: { bg: "#E0E7FF", color: "#4338CA" },
  AAAA: { bg: "#EDE9FE", color: "#6D28D9" },
  CNAME: { bg: "#CCFBF1", color: "#0F766E" },
  TXT: { bg: "#FEF3C7", color: "#B45309" },
  SRV: { bg: "#FCE7F3", color: "#BE185D" },
};

function summarizeSelector(selector: Record<string, string>, nodes: Node[]): string {
  if (!selector || Object.keys(selector).length === 0) return "No nodes";
  if (isAllNodesSelector(selector)) return `All nodes (${nodes.length})`;
  const resolved = resolveSelector(nodes, selector, true);
  if (resolved.error) return "Invalid selector";
  if (resolved.matched.length === 0) return "0 nodes matched";
  if (resolved.matched.length <= 2) return resolved.matched.map((n) => n.name).join(", ");
  return `${resolved.matched.length} nodes`;
}

export const DNSCustomRecordsSection: React.FC<DNSCustomRecordsSectionProps> = ({
  rules,
  onChange,
  nodes,
  disabled = false,
}) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);

  const [formSelector, setFormSelector] = useState<Record<string, string>>(ALL_NODES_SELECTOR);
  const [formSelectorError, setFormSelectorError] = useState<string | null>(null);
  const [formName, setFormName] = useState("");
  const [formType, setFormType] = useState<DNSCustomRecordType>("A");
  const [formValue, setFormValue] = useState("");
  const [formTTL, setFormTTL] = useState("Auto");
  const [formError, setFormError] = useState<string | null>(null);

  const openAddDialog = () => {
    setEditingIndex(null);
    setFormSelector(ALL_NODES_SELECTOR);
    setFormSelectorError(null);
    setFormName("");
    setFormType("A");
    setFormValue("");
    setFormTTL("Auto");
    setFormError(null);
    setDialogOpen(true);
  };

  const openEditDialog = (index: number) => {
    const rule = rules[index];
    setEditingIndex(index);
    setFormSelector(rule.target_nodes && Object.keys(rule.target_nodes).length > 0 ? rule.target_nodes : {});
    setFormSelectorError(null);
    setFormName(rule.name || "");
    setFormType((rule.type as DNSCustomRecordType) || "A");
    setFormValue(rule.value || "");
    setFormTTL(rule.ttl && rule.ttl.trim() !== "" ? rule.ttl : "Auto");
    setFormError(null);
    setDialogOpen(true);
  };

  const handleDelete = (index: number) => {
    onChange(rules.filter((_, i) => i !== index));
  };

  const handleSaveRule = () => {
    const name = formName.trim();
    const value = formValue.trim();
    const ttl = formTTL.trim();

    if (formSelectorError) {
      setFormError(formSelectorError);
      return;
    }
    if (!name) {
      setFormError("Record Name is required.");
      return;
    }
    if (!value) {
      setFormError("Record Value is required.");
      return;
    }
    if (ttl && ttl.toLowerCase() !== "auto" && !ttl.includes("${")) {
      const parsed = Number(ttl);
      if (!Number.isInteger(parsed) || parsed < 1) {
        setFormError('TTL must be "Auto", a positive integer (seconds), or a ${VAR} expression.');
        return;
      }
    }

    const newRule: DNSCustomRecordRule = {
      target_nodes: formSelector,
      name,
      type: formType,
      value,
      ttl: ttl || "Auto",
    };

    if (editingIndex !== null) {
      const next = [...rules];
      next[editingIndex] = newRule;
      onChange(next);
    } else {
      onChange([...rules, newRule]);
    }
    setDialogOpen(false);
  };

  const getValueHelperText = (type: DNSCustomRecordType): string => {
    switch (type) {
      case "SRV":
        return 'Cloudflare API SRV format: JSON e.g. {"priority": 10, "weight": 5, "port": 8080, "target": "${NODE_NAME}.${BASE}"} or "<priority> <weight> <port> <target>". Supports ${NODE_*} variables.';
      case "CNAME":
        return "Target hostname (e.g. ${NODE_NAME}.${BASE}). Supports ${NODE_*} variables.";
      case "TXT":
        return "TXT record string value. Supports ${NODE_*} variables.";
      case "AAAA":
        return "IPv6 address (e.g. ${NODE_IP6} or ${NODE_EXTERNAL_IP6}). Supports ${NODE_*} variables.";
      case "A":
      default:
        return "IPv4 address (e.g. ${NODE_IP} or ${NODE_EXTERNAL_IP}). Supports ${NODE_*} variables.";
    }
  };

  const getValuePlaceholder = (type: DNSCustomRecordType): string => {
    switch (type) {
      case "SRV":
        return '{"priority": 10, "weight": 5, "port": 8080, "target": "${NODE_NAME}.${BASE}"}';
      case "CNAME":
        return "${NODE_NAME}.${BASE}";
      case "TXT":
        return "node=${NODE_NAME}";
      case "AAAA":
        return "${NODE_IP6}";
      case "A":
      default:
        return "${NODE_EXTERNAL_IP}";
    }
  };

  return (
    <Box
      sx={{
        p: 1.5,
        borderRadius: 1,
        border: "1px solid #E2E8F0",
        bgcolor: "#FFFFFF",
        display: "flex",
        flexDirection: "column",
        gap: 1.2,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 1 }}>
        <Box>
          <Typography variant="body2" sx={{ fontWeight: 700, color: "#1E293B" }}>
            Custom Record Rules
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.2 }}>
            Generate extra DNS records per selected node. Supports <code>{"${NODE_IP}"}</code> style variables in Name,
            Value, and TTL, plus <code>{"${BASE}"}</code> and <code>{"${ROOT}"}</code> in Name. Custom records have
            higher priority and can override built-in <code>&lt;name&gt;.&lt;base&gt;</code> records.
          </Typography>
        </Box>
        <Button
          size="small"
          variant="outlined"
          startIcon={<Plus size={14} />}
          onClick={openAddDialog}
          disabled={disabled}
          sx={{
            flexShrink: 0,
            textTransform: "none",
            fontWeight: 600,
            borderColor: "#CBD5E1",
            color: "#334155",
            "&:hover": { borderColor: "#94A3B8", bgcolor: "#F8FAFC" },
          }}
        >
          Add Rule
        </Button>
      </Box>

      {rules.length === 0 ? (
        <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic", py: 0.5 }}>
          No custom record rules configured. Click "Add Rule" to create one.
        </Typography>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
          {rules.map((rule, idx) => {
            const typeStyle = TYPE_COLORS[rule.type] || TYPE_COLORS.A;
            const ttlDisplay = rule.ttl && rule.ttl.trim() !== "" ? rule.ttl : "Auto";
            return (
              <Box
                key={idx}
                sx={{
                  p: 1,
                  borderRadius: 1,
                  border: "1px solid #E2E8F0",
                  bgcolor: "#F8FAFC",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  gap: 1,
                }}
              >
                <Box sx={{ display: "flex", flexDirection: "column", gap: 0.4, minWidth: 0, flex: 1 }}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                    <Chip
                      label={rule.type}
                      size="small"
                      sx={{
                        height: 20,
                        fontSize: "0.7rem",
                        fontWeight: 700,
                        bgcolor: typeStyle.bg,
                        color: typeStyle.color,
                      }}
                    />
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: "monospace",
                        fontSize: "0.8rem",
                        fontWeight: 600,
                        color: "#1E293B",
                        wordBreak: "break-all",
                      }}
                    >
                      {rule.name}
                    </Typography>
                    <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                      →
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: "monospace",
                        fontSize: "0.8rem",
                        color: "#059669",
                        wordBreak: "break-all",
                      }}
                    >
                      {rule.value}
                    </Typography>
                  </Box>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                    <Chip
                      label={`Target: ${summarizeSelector(rule.target_nodes, nodes)}`}
                      size="small"
                      variant="outlined"
                      sx={{ height: 18, fontSize: "0.68rem", color: "#475569", borderColor: "#CBD5E1" }}
                    />
                    <Chip
                      label={`TTL: ${ttlDisplay}`}
                      size="small"
                      variant="outlined"
                      sx={{ height: 18, fontSize: "0.68rem", color: "#475569", borderColor: "#CBD5E1" }}
                    />
                  </Box>
                </Box>

                <Box sx={{ display: "flex", alignItems: "center", gap: 0.3, flexShrink: 0 }}>
                  <Tooltip title="Edit rule">
                    <span>
                      <IconButton size="small" onClick={() => openEditDialog(idx)} disabled={disabled}>
                        <Edit2 size={14} />
                      </IconButton>
                    </span>
                  </Tooltip>
                  <Tooltip title="Delete rule">
                    <span>
                      <IconButton
                        size="small"
                        color="error"
                        onClick={() => handleDelete(idx)}
                        disabled={disabled}
                      >
                        <Trash2 size={14} />
                      </IconButton>
                    </span>
                  </Tooltip>
                </Box>
              </Box>
            );
          })}
        </Box>
      )}

      {/* Add / Edit Custom DNS Rule Dialog */}
      <Dialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { borderRadius: 2 } }}
      >
        <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pb: 1 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#1E293B" }}>
            {editingIndex !== null ? "Edit Custom DNS Record Rule" : "Add Custom DNS Record Rule"}
          </Typography>
          <IconButton size="small" onClick={() => setDialogOpen(false)}>
            <X size={18} />
          </IconButton>
        </DialogTitle>
        <DialogContent dividers sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {formError && (
            <Typography variant="body2" color="error" sx={{ fontWeight: 500 }}>
              {formError}
            </Typography>
          )}

          <NodeSelector
            label="Target Nodes"
            description="Generates one DNS record per matching node."
            nodes={nodes}
            value={formSelector}
            onChange={setFormSelector}
            onErrorChange={setFormSelectorError}
            compact
          />

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "140px 1fr 140px" }, gap: 1.5 }}>
            <TextField
              select
              size="small"
              label="Type"
              value={formType}
              onChange={(e) => setFormType(e.target.value as DNSCustomRecordType)}
            >
              {RECORD_TYPES.map((t) => (
                <MenuItem key={t} value={t}>
                  {t}
                </MenuItem>
              ))}
            </TextField>

            <TextField
              size="small"
              label="Name"
              placeholder="${NODE_NAME}.${BASE}"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              helperText="Supports ${BASE}, ${ROOT}, and ${NODE_*}. Appends .<root-domain> automatically if omitted."
            />

            <TextField
              size="small"
              label="TTL"
              placeholder="Auto"
              value={formTTL}
              onChange={(e) => setFormTTL(e.target.value)}
              helperText='Defaults to "Auto"'
            />
          </Box>

          <TextField
            fullWidth
            size="small"
            multiline={formType === "SRV" || formType === "TXT"}
            minRows={formType === "SRV" ? 2 : 1}
            label="Value"
            placeholder={getValuePlaceholder(formType)}
            value={formValue}
            onChange={(e) => setFormValue(e.target.value)}
            helperText={getValueHelperText(formType)}
            InputProps={{ sx: { fontFamily: "monospace", fontSize: "0.85rem" } }}
          />
        </DialogContent>
        <DialogActions sx={{ px: 2.5, py: 1.5 }}>
          <Button onClick={() => setDialogOpen(false)} sx={{ textTransform: "none", color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSaveRule}
            sx={{ textTransform: "none", fontWeight: 600, bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
          >
            {editingIndex !== null ? "Update Rule" : "Add Rule"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
