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
  IconButton,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Chip,
  Paper,
  Divider,
  Alert,
} from "@mui/material";
import { X, Plus, Trash2, Settings2 } from "lucide-react";
import { LookingGlassTask, TaskParam, ParameterType } from "../../types/api";

interface CustomTaskEditorModalProps {
  open: boolean;
  onClose: () => void;
  taskToEdit?: LookingGlassTask | null;
  onSave: (task: LookingGlassTask) => Promise<void>;
}

export const CustomTaskEditorModal: React.FC<CustomTaskEditorModalProps> = ({
  open,
  onClose,
  taskToEdit,
  onSave,
}) => {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [category, setCategory] = useState("Custom");
  const [commandTmpl, setCommandTmpl] = useState("");
  const [parser, setParser] = useState("raw");
  const [timeoutSec, setTimeoutSec] = useState(15);
  const [params, setParams] = useState<TaskParam[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (taskToEdit) {
      setName(taskToEdit.name || "");
      setDescription(taskToEdit.description || "");
      setCategory(taskToEdit.category || "Custom");
      setCommandTmpl(taskToEdit.command_tmpl || "");
      setParser(taskToEdit.parser || "raw");
      setTimeoutSec(taskToEdit.timeout_sec || 15);
      setParams(taskToEdit.params || []);
    } else {
      setName("");
      setDescription("");
      setCategory("Custom");
      setCommandTmpl("ping -c {{count}} {{target}}");
      setParser("ping");
      setTimeoutSec(15);
      setParams([
        {
          key: "target",
          label: "Target Host or IP",
          type: "ip_or_cidr",
          required: true,
          default_value: "172.20.0.1",
        },
        {
          key: "count",
          label: "Count",
          type: "number",
          required: false,
          default_value: "4",
        },
      ]);
    }
    setError(null);
  }, [taskToEdit, open]);

  const handleAddParam = () => {
    const key = `param_${params.length + 1}`;
    setParams([
      ...params,
      {
        key,
        label: `Param ${params.length + 1}`,
        type: "string",
        required: true,
        default_value: "",
      },
    ]);
  };

  const handleUpdateParam = (index: number, updated: Partial<TaskParam>) => {
    const next = [...params];
    next[index] = { ...next[index], ...updated };
    setParams(next);
  };

  const handleRemoveParam = (index: number) => {
    setParams(params.filter((_, i) => i !== index));
  };

  const handleInsertPlaceholder = (key: string) => {
    setCommandTmpl((prev) => `${prev} {{${key}}}`);
  };

  const handleSubmit = async () => {
    if (!name.trim()) {
      setError("Task name is required");
      return;
    }
    if (!commandTmpl.trim()) {
      setError("Command template is required");
      return;
    }

    setSaving(true);
    setError(null);
    try {
      const payload: LookingGlassTask = {
        id: taskToEdit?.id || "",
        name: name.trim(),
        description: description.trim(),
        category: category.trim() || "Custom",
        command_tmpl: commandTmpl.trim(),
        parser,
        timeout_sec: timeoutSec,
        params,
        is_builtin: false,
      };
      await onSave(payload);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to save task");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", pb: 1 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 36,
              height: 36,
              borderRadius: 1.5,
              background: "linear-gradient(135deg, #6366F1 0%, #06B6D4 100%)",
              color: "#FFFFFF",
            }}
          >
            <Settings2 size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem" }}>
              {taskToEdit ? "Edit Looking Glass Task" : "Create Looking Glass Task"}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Define parameterized command templates for repeated diagnostic tasks
            </Typography>
          </Box>
        </Box>
        <IconButton size="small" onClick={onClose}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <Divider />

      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 2.5 }}>
        {error && <Alert severity="error">{error}</Alert>}

        {/* Basic Info */}
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "2fr 1fr" }, gap: 2 }}>
          <TextField
            label="Task Name"
            size="small"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Ping Core Router"
          />
          <TextField
            label="Category"
            size="small"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            placeholder="e.g. Connectivity, BGP, Custom"
          />
        </Box>

        <TextField
          label="Description"
          size="small"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="Short description of what this diagnostic task tests"
          multiline
          rows={2}
        />

        {/* Command Template */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
              Command Template
            </Typography>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
              <Typography variant="caption" sx={{ color: "#64748B", mr: 0.5 }}>
                Insert placeholder:
              </Typography>
              {params.map((p) => (
                <Chip
                  key={p.key}
                  label={`{{${p.key}}}`}
                  size="small"
                  onClick={() => handleInsertPlaceholder(p.key)}
                  sx={{
                    fontFamily: "monospace",
                    fontSize: "0.72rem",
                    cursor: "pointer",
                    backgroundColor: "#EEF2FF",
                    color: "#4338CA",
                    "&:hover": { backgroundColor: "#E0E7FF" },
                  }}
                />
              ))}
            </Box>
          </Box>

          <TextField
            fullWidth
            size="small"
            required
            value={commandTmpl}
            onChange={(e) => setCommandTmpl(e.target.value)}
            placeholder="e.g. ping -c {{count}} {{target}}"
            InputProps={{
              sx: { fontFamily: "monospace", fontSize: "0.85rem", backgroundColor: "#F8FAFC" },
            }}
            helperText="Allowed command prefixes: ping, traceroute, mtr, birdc, ip route, whois, dig"
          />
        </Box>

        {/* Parser & Timeout */}
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
          <FormControl size="small" fullWidth>
            <InputLabel id="parser-select-label">Output Visualizer</InputLabel>
            <Select
              labelId="parser-select-label"
              value={parser}
              label="Output Visualizer"
              onChange={(e) => setParser(e.target.value)}
            >
              <MenuItem value="raw">Raw Terminal Only</MenuItem>
              <MenuItem value="bird_protocols">BIRD Protocols Table</MenuItem>
              <MenuItem value="bird_route">BIRD Route Visualizer</MenuItem>
              <MenuItem value="ping">ICMP Ping Metrics</MenuItem>
              <MenuItem value="traceroute">Traceroute Hop Table</MenuItem>
              <MenuItem value="mtr">MTR Report Table</MenuItem>
            </Select>
          </FormControl>

          <TextField
            label="Timeout (seconds)"
            size="small"
            type="number"
            value={timeoutSec}
            onChange={(e) => setTimeoutSec(Math.max(1, Math.min(60, parseInt(e.target.value) || 15)))}
            helperText="Maximum allowed execution time (1 - 60s)"
          />
        </Box>

        <Divider />

        {/* Parameters Section */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Box>
              <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                Task Parameters ({params.length})
              </Typography>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Variables that the user can configure prior to executing this task
              </Typography>
            </Box>
            <Button
              size="small"
              startIcon={<Plus size={14} />}
              onClick={handleAddParam}
              variant="outlined"
              sx={{ textTransform: "none", fontSize: "0.8rem" }}
            >
              Add Parameter
            </Button>
          </Box>

          {params.length === 0 ? (
            <Paper
              elevation={0}
              sx={{
                p: 2,
                textAlign: "center",
                backgroundColor: "#F8FAFC",
                border: "1px dashed #CBD5E1",
                borderRadius: 2,
              }}
            >
              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                No parameters defined. This task will execute statically without user prompts.
              </Typography>
            </Paper>
          ) : (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
              {params.map((p, idx) => (
                <Paper
                  key={idx}
                  elevation={0}
                  sx={{
                    p: 1.5,
                    border: "1px solid #E2E8F0",
                    borderRadius: 2,
                    backgroundColor: "#FAFBFD",
                    display: "grid",
                    gridTemplateColumns: { xs: "1fr", sm: "1fr 1.5fr 1fr 1fr auto" },
                    gap: 1.5,
                    alignItems: "center",
                  }}
                >
                  <TextField
                    label="Key"
                    size="small"
                    value={p.key}
                    onChange={(e) => handleUpdateParam(idx, { key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, "") })}
                    InputProps={{ sx: { fontFamily: "monospace", fontSize: "0.8rem" } }}
                  />

                  <TextField
                    label="Label"
                    size="small"
                    value={p.label}
                    onChange={(e) => handleUpdateParam(idx, { label: e.target.value })}
                  />

                  <FormControl size="small" fullWidth>
                    <InputLabel id={`type-${idx}`}>Type</InputLabel>
                    <Select
                      labelId={`type-${idx}`}
                      value={p.type}
                      label="Type"
                      onChange={(e) => handleUpdateParam(idx, { type: e.target.value as ParameterType })}
                    >
                      <MenuItem value="string">String</MenuItem>
                      <MenuItem value="ip_or_cidr">IP or CIDR</MenuItem>
                      <MenuItem value="number">Number</MenuItem>
                      <MenuItem value="select">Select</MenuItem>
                      <MenuItem value="boolean">Boolean</MenuItem>
                    </Select>
                  </FormControl>

                  <TextField
                    label="Default"
                    size="small"
                    value={p.default_value || ""}
                    onChange={(e) => handleUpdateParam(idx, { default_value: e.target.value })}
                  />

                  <IconButton size="small" color="error" onClick={() => handleRemoveParam(idx)}>
                    <Trash2 size={16} />
                  </IconButton>
                </Paper>
              ))}
            </Box>
          )}
        </Box>
      </DialogContent>

      <Divider />

      <DialogActions sx={{ px: 3, py: 2 }}>
        <Button onClick={onClose} disabled={saving} sx={{ textTransform: "none" }}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={saving}
          sx={{
            textTransform: "none",
            background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
          }}
        >
          {saving ? "Saving..." : taskToEdit ? "Update Task" : "Create Task"}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
