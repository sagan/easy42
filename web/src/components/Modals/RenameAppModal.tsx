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
import { Layers, Box as BoxIcon, Cpu, AlertCircle, CheckCircle2, Tag, Clock } from "lucide-react";
import type { App } from "../../types/api";
import { api } from "../../api/client";

interface RenameAppModalProps {
  app: App | null;
  existingApps: App[];
  open: boolean;
  onClose: () => void;
  onAppRenamed: (oldName: string, updatedApp: App) => void;
}

export const RenameAppModal: React.FC<RenameAppModalProps> = ({
  app,
  existingApps,
  open,
  onClose,
  onAppRenamed,
}) => {
  const [newName, setNewName] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (app && open) {
      setNewName(app.name);
      setError(null);
    }
  }, [app, open]);

  if (!app) return null;

  const trimmedNewName = newName.trim();
  const isSameName = trimmedNewName.toLowerCase() === app.name.toLowerCase();
  const nameExists = existingApps.some(
    (a) => a.id !== app.id && a.name.toLowerCase() === trimmedNewName.toLowerCase()
  );
  const isValidFormat = /^[a-zA-Z0-9_-]+$/.test(trimmedNewName);
  const isValidLength = trimmedNewName.length >= 1 && trimmedNewName.length <= 64;
  const canSubmit = isValidLength && isValidFormat && !isSameName && !nameExists && !saving;

  const handleNameChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, "");
    setNewName(val);
    if (error) setError(null);
  };

  const handleRename = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    if (!canSubmit) return;

    setSaving(true);
    setError(null);

    try {
      const updated = await api.renameApp(app.id, trimmedNewName);
      onAppRenamed(app.name, updated);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to rename application");
    } finally {
      setSaving(false);
    }
  };

  const isDocker = app.app_type === "docker";
  const isCron = app.app_type === "cron";

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
              backgroundColor: isDocker
                ? "rgba(2, 132, 199, 0.1)"
                : isCron
                ? "rgba(217, 119, 6, 0.1)"
                : "rgba(147, 51, 234, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: isDocker ? "#0284C7" : isCron ? "#D97706" : "#9333EA",
            }}
          >
            <Layers size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem", lineHeight: 1.2, color: "#0F172A" }}>
              Rename App
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Change workload identifier across mesh nodes
            </Typography>
          </Box>
        </DialogTitle>

        <DialogContent sx={{ pt: 2.5, display: "flex", flexDirection: "column", gap: 2 }}>
          {error && (
            <Alert severity="error" icon={<AlertCircle size={18} />} sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}

          {/* Current App Info */}
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
              {isDocker ? (
                <BoxIcon size={16} color="#0284C7" />
              ) : isCron ? (
                <Clock size={16} color="#D97706" />
              ) : (
                <Cpu size={16} color="#9333EA" />
              )}
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                Current Name:
              </Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Chip
                label={app.name}
                size="small"
                className="mono-font"
                sx={{
                  fontWeight: 700,
                  backgroundColor: "#EEF2FF",
                  color: "#4338CA",
                }}
              />
              <Chip
                label={isDocker ? "Docker" : isCron ? "Cron" : "Systemd"}
                size="small"
                sx={{
                  fontSize: "0.68rem",
                  fontWeight: 700,
                  height: 20,
                  backgroundColor: isDocker ? "#E0F2FE" : isCron ? "#FEF3C7" : "#F3E8FF",
                  color: isDocker ? "#0369A1" : isCron ? "#B45309" : "#7E22CE",
                }}
              />
            </Box>
          </Box>

          {/* New Name Input */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 0.5 }}>
            <TextField
              id="rename-app-new-name-input"
              autoFocus
              fullWidth
              size="small"
              label="New App Name"
              placeholder="e.g. vproxy or coredns"
              value={newName}
              onChange={handleNameChange}
              error={Boolean(
                trimmedNewName &&
                  (nameExists || isSameName || !isValidFormat || !isValidLength)
              )}
              helperText={
                nameExists ? (
                  <span style={{ color: "#DC2626" }}>App name &quot;{trimmedNewName}&quot; already exists!</span>
                ) : isSameName ? (
                  "Please enter a different name"
                ) : !isValidFormat && trimmedNewName ? (
                  <span style={{ color: "#DC2626" }}>Only lowercase letters, numbers, hyphens, and underscores allowed</span>
                ) : (
                  "Workload prefix 'ezapp-' will be used on host nodes"
                )
              }
              disabled={saving}
              InputProps={{
                endAdornment: canSubmit ? <CheckCircle2 size={18} color="#10B981" /> : undefined,
              }}
            />
            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", px: 0.5 }}>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{
                  color: "#4F46E5",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                }}
              >
                Project / Service: {trimmedNewName ? `ezapp-${trimmedNewName}` : "—"}
              </Typography>
              <Typography
                variant="caption"
                className="mono-font"
                sx={{
                  color: "#94A3B8",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                }}
              >
                {trimmedNewName.length}/64 chars
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
              Workload Impact:
            </Typography>
            <Typography variant="caption" sx={{ color: "#475569", lineHeight: 1.4, display: "block" }}>
              Renaming updates the app identifier across all assigned nodes. On next agent sync, nodes will transition workloads from <code>ezapp-{app.name}</code> to <code>ezapp-{trimmedNewName || "..."}</code>.
            </Typography>
          </Box>
        </DialogContent>

        <DialogActions sx={{ px: 3, pb: 2.5, pt: 1, borderTop: "1px solid #E2E8F0" }}>
          <Button onClick={onClose} disabled={saving} sx={{ color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            id="rename-app-submit-btn"
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
            {saving ? "Renaming..." : "Rename App"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
