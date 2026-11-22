import React, { useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  FormGroup,
  IconButton,
  Tooltip,
  Typography,
} from "@mui/material";
import { Edit2, Plus, Trash2, X } from "lucide-react";
import type { DNSSSHFPAlgorithm, DNSSSHFPRule, Node } from "../../types/api";
import { ALL_NODES_SELECTOR, isAllNodesSelector, NodeSelector, resolveSelector } from "../Common/NodeSelector";

interface DNSSSHFPRulesSectionProps {
  rules: DNSSSHFPRule[];
  onChange: (rules: DNSSSHFPRule[]) => void;
  nodes: Node[];
  disabled?: boolean;
}

const SSHFP_ALGORITHMS: { value: DNSSSHFPAlgorithm; label: string; short: string }[] = [
  { value: 3, label: "3 — ECDSA (ecdsa-sha2-*)", short: "3: ECDSA" },
  { value: 4, label: "4 — Ed25519 (ssh-ed25519)", short: "4: Ed25519" },
  { value: 6, label: "6 — Ed448 (ssh-ed448)", short: "6: Ed448" },
];

function summarizeSelector(selector: Record<string, string>, nodes: Node[]): string {
  if (!selector || Object.keys(selector).length === 0) return "No nodes";
  if (isAllNodesSelector(selector)) return `All nodes (${nodes.length})`;
  const resolved = resolveSelector(nodes, selector, true);
  if (resolved.error) return "Invalid selector";
  if (resolved.matched.length === 0) return "0 nodes matched";
  if (resolved.matched.length <= 2) return resolved.matched.map((n) => n.name).join(", ");
  return `${resolved.matched.length} nodes`;
}

function formatAlgorithmShort(alg: DNSSSHFPAlgorithm): string {
  const found = SSHFP_ALGORITHMS.find((a) => a.value === alg);
  return found ? found.short : String(alg);
}

export const DNSSSHFPRulesSection: React.FC<DNSSSHFPRulesSectionProps> = ({
  rules,
  onChange,
  nodes,
  disabled = false,
}) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);

  const [formSelector, setFormSelector] = useState<Record<string, string>>(ALL_NODES_SELECTOR);
  const [formSelectorError, setFormSelectorError] = useState<string | null>(null);
  const [formAlgorithms, setFormAlgorithms] = useState<DNSSSHFPAlgorithm[]>([3, 4, 6]);
  const [formError, setFormError] = useState<string | null>(null);

  const openAddDialog = () => {
    setEditingIndex(null);
    setFormSelector(ALL_NODES_SELECTOR);
    setFormSelectorError(null);
    setFormAlgorithms([3, 4, 6]);
    setFormError(null);
    setDialogOpen(true);
  };

  const openEditDialog = (index: number) => {
    const rule = rules[index];
    setEditingIndex(index);
    setFormSelector(rule.target_nodes && Object.keys(rule.target_nodes).length > 0 ? rule.target_nodes : {});
    setFormSelectorError(null);
    setFormAlgorithms(Array.isArray(rule.algorithms) && rule.algorithms.length > 0 ? [...rule.algorithms] : [3, 4]);
    setFormError(null);
    setDialogOpen(true);
  };

  const handleDelete = (index: number) => {
    onChange(rules.filter((_, i) => i !== index));
  };

  const toggleAlgorithm = (alg: DNSSSHFPAlgorithm) => {
    setFormAlgorithms((prev) => {
      if (prev.includes(alg)) {
        return prev.filter((a) => a !== alg);
      }
      return [...prev, alg].sort((a, b) => a - b);
    });
  };

  const handleSaveRule = () => {
    if (formSelectorError) {
      setFormError(formSelectorError);
      return;
    }
    if (formAlgorithms.length === 0) {
      setFormError("Select at least one SSHFP algorithm.");
      return;
    }

    const newRule: DNSSSHFPRule = {
      target_nodes: formSelector,
      algorithms: [...formAlgorithms].sort((a, b) => a - b),
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
            SSHFP Records Rules
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 0.2 }}>
            Automatically publish <code>SSHFP</code> records at <code>&lt;node&gt;.&lt;base&gt;</code> using agent-collected
            SSH host keys (Apps → SSH tab). Hash type is fixed to <b>2 (SHA-256)</b>; legacy RSA (1) and DSA (2)
            algorithms are not supported.
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
          No SSHFP record rules configured. Click "Add Rule" to publish SSH host key fingerprints.
        </Typography>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
          {rules.map((rule, idx) => (
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
              <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5, minWidth: 0, flex: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                  <Chip
                    label="SSHFP"
                    size="small"
                    sx={{
                      height: 20,
                      fontSize: "0.7rem",
                      fontWeight: 700,
                      bgcolor: "#E0F2FE",
                      color: "#0369A1",
                    }}
                  />
                  {rule.algorithms.map((alg) => (
                    <Chip
                      key={alg}
                      label={formatAlgorithmShort(alg)}
                      size="small"
                      sx={{
                        height: 20,
                        fontSize: "0.7rem",
                        fontWeight: 600,
                        bgcolor: "#F1F5F9",
                        color: "#334155",
                      }}
                    />
                  ))}
                  <Chip
                    label="Hash: 2 (SHA-256)"
                    size="small"
                    variant="outlined"
                    sx={{ height: 20, fontSize: "0.68rem", color: "#475569", borderColor: "#CBD5E1" }}
                  />
                </Box>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                  <Chip
                    label={`Target: ${summarizeSelector(rule.target_nodes, nodes)}`}
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
                    <IconButton size="small" color="error" onClick={() => handleDelete(idx)} disabled={disabled}>
                      <Trash2 size={14} />
                    </IconButton>
                  </span>
                </Tooltip>
              </Box>
            </Box>
          ))}
        </Box>
      )}

      {/* Add / Edit SSHFP Rule Dialog */}
      <Dialog
        open={dialogOpen}
        onClose={() => setDialogOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{ sx: { borderRadius: 2 } }}
      >
        <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pb: 1 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#1E293B" }}>
            {editingIndex !== null ? "Edit SSHFP Record Rule" : "Add SSHFP Record Rule"}
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
            description="Generates SSHFP records at <node>.<base> from each selected node's agent-collected SSH host keys."
            nodes={nodes}
            value={formSelector}
            onChange={setFormSelector}
            onErrorChange={setFormSelectorError}
            compact
          />

          <Box
            sx={{
              p: 1.5,
              borderRadius: 1,
              border: "1px solid #E2E8F0",
              bgcolor: "#F8FAFC",
              display: "flex",
              flexDirection: "column",
              gap: 0.5,
            }}
          >
            <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
              Algorithms
            </Typography>
            <FormGroup row sx={{ gap: 1 }}>
              {SSHFP_ALGORITHMS.map((alg) => (
                <FormControlLabel
                  key={alg.value}
                  control={
                    <Checkbox
                      size="small"
                      checked={formAlgorithms.includes(alg.value)}
                      onChange={() => toggleAlgorithm(alg.value)}
                    />
                  }
                  label={<Typography variant="body2">{alg.label}</Typography>}
                />
              ))}
            </FormGroup>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Hash type is fixed to <b>2 (SHA-256)</b>. Values are automatically computed from the node host keys listed
              in the Apps → SSH tab; legacy 1 (RSA) and 2 (DSA) keys are excluded.
            </Typography>
          </Box>
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
