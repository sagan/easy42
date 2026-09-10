import React, { useState, useEffect } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  Alert,
  CircularProgress,
  Chip,
} from "@mui/material";
import { Tag, Server, Globe, AlertCircle, CheckCircle2 } from "lucide-react";
import { Node } from "../../types/api";
import { api } from "../../api/client";

interface RenameNodeModalProps {
  node: Node | null;
  existingNodes: Node[];
  open: boolean;
  onClose: () => void;
  onNodeRenamed: (oldName: string, updatedNode: Node) => void;
}

export const RenameNodeModal: React.FC<RenameNodeModalProps> = ({
  node,
  existingNodes,
  open,
  onClose,
  onNodeRenamed,
}) => {
  const [newName, setNewName] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isExternal = Boolean(node?.is_external);
  const maxLength = isExternal ? 10 : 11;

  useEffect(() => {
    if (node && open) {
      setNewName(node.name);
      setError(null);
    }
  }, [node, open]);

  if (!node) return null;

  const trimmedNewName = newName.trim();
  const isSameName = trimmedNewName.toLowerCase() === node.name.toLowerCase();
  const nameExists = existingNodes.some(
    (n) => n.name.toLowerCase() === trimmedNewName.toLowerCase() && n.name.toLowerCase() !== node.name.toLowerCase(),
  );
  const isValidLength = trimmedNewName.length > 0 && trimmedNewName.length <= maxLength;
  const canSubmit = isValidLength && !isSameName && !nameExists && !saving;

  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    // Sanitize: allow lowercase alphanumeric, hyphen, underscore
    const val = e.target.value
      .toLowerCase()
      .replace(/[^a-z0-9-_]/g, "")
      .slice(0, maxLength);
    setNewName(val);
    if (error) setError(null);
  };

  const handleRename = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    if (!canSubmit) return;

    setSaving(true);
    setError(null);

    try {
      const updated = await api.renameNode(node.name, trimmedNewName);
      onNodeRenamed(node.name, updated);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to rename node");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog
      open={open}
      onClose={saving ? undefined : onClose}
      maxWidth="xs"
      fullWidth
      PaperProps={{
        sx: {
          borderRadius: 3,
          boxShadow: "0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)",
        },
      }}
    >
      <form onSubmit={handleRename}>
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1.5,
            pb: 1.5,
            borderBottom: "1px solid #E2E8F0",
          }}
        >
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              backgroundColor: isExternal ? "rgba(139, 92, 246, 0.12)" : "rgba(79, 70, 229, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: isExternal ? "#7C3AED" : "#4F46E5",
            }}
          >
            <Tag size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem", lineHeight: 1.2, color: "#0F172A" }}>
              Rename Node
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Change node identifier across mesh configuration
            </Typography>
          </Box>
        </DialogTitle>

        <DialogContent sx={{ pt: 2.5, display: "flex", flexDirection: "column", gap: 2 }}>
          {error && (
            <Alert severity="error" icon={<AlertCircle size={18} />} sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}

          {/* Current Node Info */}
          <Box
            sx={{
              p: 1.5,
              borderRadius: 2,
              backgroundColor: "#F8FAFC",
              border: "1px solid #E2E8F0",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              {isExternal ? <Globe size={16} color="#7C3AED" /> : <Server size={16} color="#4F46E5" />}
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                Current Name:
              </Typography>
            </Box>
            <Chip
              label={node.name}
              size="small"
              className="mono-font"
              sx={{
                fontWeight: 700,
                backgroundColor: isExternal ? "#EDE9FE" : "#EEF2FF",
                color: isExternal ? "#6D28D9" : "#4338CA",
              }}
            />
          </Box>

          {/* New Name Input */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
            <TextField
              id="rename-node-new-name-input"
              autoFocus
              fullWidth
              size="small"
              label={`New Node Name (Max ${maxLength} chars)`}
              value={newName}
              onChange={handleNameChange}
              error={Boolean(trimmedNewName && (nameExists || isSameName || !isValidLength))}
              helperText={
                nameExists ? (
                  <span style={{ color: "#DC2626" }}>Node name &quot;{trimmedNewName}&quot; already exists!</span>
                ) : isSameName ? (
                  "Please enter a different name"
                ) : (
                  `Unique hostname in mesh (1-${maxLength} alphanumeric/hyphen chars)`
                )
              }
              disabled={saving}
              InputProps={{
                endAdornment: canSubmit ? <CheckCircle2 size={18} color="#10B981" /> : undefined,
              }}
            />
            <Box sx={{ display: "flex", justifyContent: "flex-end", px: 0.5 }}>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{
                  color: trimmedNewName.length > maxLength ? "#DC2626" : "#94A3B8",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                }}
              >
                {trimmedNewName.length}/{maxLength}
              </Typography>
            </Box>
          </Box>

          {/* Explanation / Impact details */}
          <Box
            sx={{
              p: 1.5,
              borderRadius: 2,
              backgroundColor: "rgba(79, 70, 229, 0.04)",
              border: "1px dashed rgba(79, 70, 229, 0.25)",
            }}
          >
            <Typography variant="caption" sx={{ color: "#4338CA", fontWeight: 600, display: "block", mb: 0.5 }}>
              Configuration Impact:
            </Typography>
            <Typography variant="caption" sx={{ color: "#475569", lineHeight: 1.4, display: "block" }}>
              Renaming replaces this node name in <code>config.json</code>, all connected links, and WireGuard
              interfaces (<code>{isExternal ? `wg42-${trimmedNewName || "..."}` : `wg42${trimmedNewName || "..."}`}</code>).
            </Typography>
          </Box>
        </DialogContent>

        <DialogActions sx={{ px: 3, pb: 2.5, pt: 1, borderTop: "1px solid #E2E8F0" }}>
          <Button onClick={onClose} disabled={saving} sx={{ color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            id="rename-node-submit-btn"
            type="submit"
            variant="contained"
            disabled={!canSubmit}
            startIcon={saving ? <CircularProgress size={16} color="inherit" /> : <Tag size={16} />}
            sx={{
              backgroundColor: "#4F46E5",
              "&:hover": { backgroundColor: "#4338CA" },
              fontWeight: 600,
              minWidth: 120,
            }}
          >
            {saving ? "Renaming..." : "Rename Node"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
