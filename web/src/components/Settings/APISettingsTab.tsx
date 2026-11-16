import React, { useState, useEffect } from "react";
import {
  Box,
  Typography,
  Button,
  IconButton,
  Paper,
  TextField,
  InputAdornment,
  Chip,
  Alert,
  CircularProgress,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Tooltip,
  Autocomplete,
  Accordion,
  AccordionSummary,
  AccordionDetails,
  Switch,
  FormControlLabel,
} from "@mui/material";
import {
  Code,
  Plus,
  Trash2,
  Copy,
  Check,
  Play,
  Lock,
  Globe,
  RefreshCw,
  Sparkles,
  Eye,
  EyeOff,
  ExternalLink,
  ChevronDown,
  FileText,
  Search,
  Shuffle,
  Terminal,
} from "lucide-react";
import { useMesh } from "../../context/MeshContext";
import { api } from "../../api/client";
import type { CustomAPI, CustomAPITestResponse } from "../../types/api";
import { generatePassword } from "../../utils/common";

const PREDEFINED_CONTENT_TYPES = [
  "application/json",
  "text/plain",
  "text/html",
  "application/xml",
  "application/x-yaml",
  "text/csv",
  "application/octet-stream",
];

interface HeaderRow {
  key: string;
  value: string;
}

const TEMPLATE_PRESETS = [
  {
    name: "Cluster Overview (JSON)",
    contentType: "application/json",
    template: `{
  "network": "{{ .config.network_settings.name }}",
  "asn": {{ .config.network_settings.public_asn }},
  "total_nodes": {{ len .config.nodes }},
  "nodes": [
    {{ range $i, $n := .config.nodes }}{{ if $i }},{{ end }}{
      "name": "{{ $n.name }}",
      "host": "{{ $n.host }}",
      "asn": {{ $n.asn }}
    }{{ end }}
  ]
}`,
  },
  {
    name: "Prometheus HTTP Service Discovery (JSON)",
    contentType: "application/json",
    template: `[
  {
    "targets": [
      {{ range $i, $n := .config.nodes }}{{ if $i }},{{ end }}"{{ $n.host }}:9100"{{ end }}
    ],
    "labels": {
      "cluster": "{{ .config.network_settings.name }}",
      "job": "node_exporter"
    }
  }
]`,
  },
  {
    name: "Plain Text Node List",
    contentType: "text/plain",
    template: `# Easy42 Fleet Nodes: {{ .config.network_settings.name }}
{{ range .config.nodes }}{{ .name }} {{ .host }} AS{{ .asn }}
{{ end }}`,
  },
  {
    name: "Echo Request & State (JSON)",
    contentType: "application/json",
    template: `{
  "request": {
    "method": "{{ .request.method }}",
    "url": "{{ .request.url }}",
    "path": "{{ .request.path }}",
    "client_ip": "{{ .request.client_ip }}",
    "query": {{ .request.query | toJson }}
  },
  "cluster": {
    "nodes_count": {{ len .config.nodes }},
    "links_count": {{ len .config.links }}
  }
}`,
  },
];

export const APISettingsTab: React.FC = () => {
  const { setStateToast } = useMesh();
  const [apis, setApis] = useState<CustomAPI[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchTerm, setSearchTerm] = useState("");

  // Create / Edit modal state
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingApi, setEditingApi] = useState<CustomAPI | null>(null);
  const [formId, setFormId] = useState("");
  const [formName, setFormName] = useState("");
  const [formDesc, setFormDesc] = useState("");
  const [formContentType, setFormContentType] = useState("application/json");
  const [formToken, setFormToken] = useState("");
  const [formHeaders, setFormHeaders] = useState<HeaderRow[]>([]);
  const [formTemplate, setFormTemplate] = useState("");
  const [formDisabled, setFormDisabled] = useState(false);
  const [showToken, setShowToken] = useState(false);
  const [saving, setSaving] = useState(false);

  // Test / Preview state inside modal
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<CustomAPITestResponse | null>(null);
  const [testError, setTestError] = useState<string | null>(null);
  const [testMethod, setTestMethod] = useState("GET");
  const [testQueryParam, setTestQueryParam] = useState("");
  const [testBody, setTestBody] = useState("");

  // Copy feedback tracking
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const fetchAPIs = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api.getCustomAPIs();
      setApis(data || []);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to load custom APIs";
      setError(msg);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAPIs();
  }, []);

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    setTimeout(() => {
      setCopiedKey((prev) => (prev === key ? null : prev));
    }, 2000);
  };

  const handleOpenCreate = () => {
    setEditingApi(null);
    setFormId("");
    setFormName("");
    setFormDesc("");
    setFormContentType("application/json");
    setFormToken("");
    setFormHeaders([]);
    setFormTemplate(TEMPLATE_PRESETS[0].template);
    setFormDisabled(false);
    setShowToken(false);
    setTestResult(null);
    setTestError(null);
    setDialogOpen(true);
  };

  const handleOpenEdit = (target: CustomAPI) => {
    setEditingApi(target);
    setFormId(target.id);
    setFormName(target.name || "");
    setFormDesc(target.description || "");
    setFormContentType(target.content_type || "application/json");
    setFormToken(target.token || "");
    const headersList: HeaderRow[] = [];
    if (target.extra_headers) {
      for (const [k, v] of Object.entries(target.extra_headers)) {
        headersList.push({ key: k, value: v });
      }
    }
    setFormHeaders(headersList);
    setFormTemplate(target.template || "");
    setFormDisabled(Boolean(target.disabled));
    setShowToken(false);
    setTestResult(null);
    setTestError(null);
    setDialogOpen(true);
  };

  const handleGenerateToken = () => {
    const token = "e42_api_" + generatePassword(32);
    setFormToken(token);
    setShowToken(true);
  };

  const handleAddHeader = () => {
    setFormHeaders((prev) => [...prev, { key: "", value: "" }]);
  };

  const handleHeaderChange = (index: number, field: "key" | "value", val: string) => {
    setFormHeaders((prev) => {
      const updated = [...prev];
      updated[index] = { ...updated[index], [field]: val };
      return updated;
    });
  };

  const handleRemoveHeader = (index: number) => {
    setFormHeaders((prev) => prev.filter((_, i) => i !== index));
  };

  const handleApplyPreset = (preset: (typeof TEMPLATE_PRESETS)[0]) => {
    setFormContentType(preset.contentType);
    setFormTemplate(preset.template);
  };

  const handleTestAPI = async () => {
    try {
      setTesting(true);
      setTestError(null);
      setTestResult(null);

      const extraHeadersMap: Record<string, string> = {};
      formHeaders.forEach((h) => {
        if (h.key.trim()) {
          extraHeadersMap[h.key.trim()] = h.value;
        }
      });

      const queryMap: Record<string, string> = {};
      if (testQueryParam.trim()) {
        const parts = testQueryParam.split("&");
        for (const part of parts) {
          const [k, v] = part.split("=");
          if (k) queryMap[k.trim()] = v ? decodeURIComponent(v.trim()) : "";
        }
      }

      const res = await api.testCustomAPI({
        api: {
          id: formId.trim() || "test-preview",
          name: formName.trim(),
          description: formDesc.trim(),
          content_type: formContentType.trim() || "application/json",
          token: formToken.trim(),
          extra_headers: extraHeadersMap,
          template: formTemplate,
          disabled: formDisabled,
        },
        request: {
          method: testMethod,
          path: `/api/custom/${formId.trim() || "test-preview"}`,
          query: queryMap,
          body: testBody,
        },
      });

      setTestResult(res);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Test execution failed";
      setTestError(msg);
    } finally {
      setTesting(false);
    }
  };

  const handleSaveAPI = async () => {
    const cleanId = formId.trim();
    if (!cleanId) {
      setError("API ID is required");
      return;
    }
    if (!formTemplate.trim()) {
      setError("API Template is required");
      return;
    }

    const extraHeadersMap: Record<string, string> = {};
    formHeaders.forEach((h) => {
      if (h.key.trim()) {
        extraHeadersMap[h.key.trim()] = h.value;
      }
    });

    const payload: Partial<CustomAPI> = {
      id: cleanId,
      name: formName.trim(),
      description: formDesc.trim(),
      content_type: formContentType.trim() || "application/json",
      token: formToken.trim(),
      extra_headers: extraHeadersMap,
      template: formTemplate,
      disabled: formDisabled,
    };

    try {
      setSaving(true);
      setError(null);
      if (editingApi) {
        await api.updateCustomAPI(editingApi.id, payload);
        setStateToast({
          message: `Custom API "${cleanId}" updated successfully.`,
          severity: "success",
        });
      } else {
        await api.createCustomAPI(payload);
        setStateToast({
          message: `Custom API "${cleanId}" created successfully.`,
          severity: "success",
        });
      }
      setDialogOpen(false);
      await fetchAPIs();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to save API";
      setError(msg);
      setStateToast({
        message: msg,
        severity: "error",
      });
    } finally {
      setSaving(false);
    }
  };

  const handleToggleDisabled = async (target: CustomAPI) => {
    try {
      setLoading(true);
      setError(null);
      const updated = !target.disabled;
      await api.updateCustomAPI(target.id, {
        ...target,
        disabled: updated,
      });
      setStateToast({
        message: `Custom API "${target.id}" is now ${updated ? "disabled" : "enabled"}.`,
        severity: "success",
      });
      await fetchAPIs();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to toggle API status";
      setError(msg);
      setStateToast({
        message: msg,
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const handleDeleteAPI = async (id: string) => {
    if (!window.confirm(`Are you sure you want to delete custom API "${id}"?`)) {
      return;
    }
    try {
      setLoading(true);
      setError(null);
      await api.deleteCustomAPI(id);
      setStateToast({
        message: `Custom API "${id}" deleted successfully.`,
        severity: "success",
      });
      await fetchAPIs();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to delete API";
      setError(msg);
      setStateToast({
        message: msg,
        severity: "error",
      });
    } finally {
      setLoading(false);
    }
  };

  const filteredApis = apis.filter((item) => {
    if (!searchTerm.trim()) return true;
    const term = searchTerm.toLowerCase();
    return (
      item.id.toLowerCase().includes(term) ||
      (item.name && item.name.toLowerCase().includes(term)) ||
      (item.description && item.description.toLowerCase().includes(term)) ||
      item.content_type.toLowerCase().includes(term)
    );
  });

  const getCurlSnippet = (item: CustomAPI) => {
    const origin = window.location.origin;
    const url = `${origin}/api/custom/${item.id}`;
    if (item.token) {
      return `curl -H "Authorization: Bearer ${item.token}" "${url}"`;
    }
    return `curl "${url}"`;
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      {/* Header Banner */}
      <Paper
        elevation={0}
        sx={{
          p: 3,
          borderRadius: 3,
          background: "linear-gradient(135deg, #EEF2FF 0%, #F5F3FF 50%, #F8FAFC 100%)",
          border: "1px solid #E0E7FF",
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 2,
          flexWrap: "wrap",
        }}
      >
        <Box sx={{ maxWidth: 720 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
            <Box
              sx={{
                bgcolor: "#4F46E5",
                color: "#FFFFFF",
                p: 1,
                borderRadius: 2,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
              }}
            >
              <Code size={20} />
            </Box>
            <Typography variant="h6" sx={{ fontWeight: 700, color: "#1E1B4B" }}>
              Programmable Custom APIs
            </Typography>
          </Box>
          <Typography variant="body2" sx={{ color: "#4B5563", lineHeight: 1.6 }}>
            Expose Easy42 cluster configuration and live runtime state to external software, Prometheus, status pages,
            or automation pipelines in any arbitrary format. Responses are rendered on-demand using Go text templates
            with full Sprout helper functions and request context.
          </Typography>
        </Box>

        <Button
          variant="contained"
          startIcon={<Plus size={16} />}
          onClick={handleOpenCreate}
          sx={{
            bgcolor: "#4F46E5",
            "&:hover": { bgcolor: "#4338CA" },
            fontWeight: 600,
            textTransform: "none",
            borderRadius: 2,
            px: 2.5,
            py: 1,
            boxShadow: "0 2px 8px rgba(79, 70, 229, 0.25)",
          }}
        >
          New API
        </Button>
      </Paper>

      {/* Feedback Alerts */}
      {error && (
        <Alert severity="error" onClose={() => setError(null)} sx={{ borderRadius: 2 }}>
          {error}
        </Alert>
      )}

      {/* Search and Action Bar */}
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2, flexWrap: "wrap" }}>
        <TextField
          size="small"
          placeholder="Search APIs by ID, name, content-type..."
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          sx={{ minWidth: 300, bgcolor: "#FFFFFF" }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search size={16} color="#94A3B8" />
              </InputAdornment>
            ),
          }}
        />

        <Button
          variant="outlined"
          size="small"
          startIcon={<RefreshCw size={14} />}
          onClick={fetchAPIs}
          disabled={loading}
          sx={{ textTransform: "none", color: "#475569", borderColor: "#CBD5E1" }}
        >
          Refresh
        </Button>
      </Box>

      {/* APIs List */}
      {loading && apis.length === 0 ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
          <CircularProgress size={36} sx={{ color: "#4F46E5" }} />
        </Box>
      ) : filteredApis.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            py: 8,
            px: 3,
            textAlign: "center",
            borderRadius: 3,
            border: "1px dashed #CBD5E1",
            bgcolor: "#F8FAFC",
          }}
        >
          <Code size={40} color="#94A3B8" style={{ marginBottom: 12 }} />
          <Typography variant="subtitle1" sx={{ fontWeight: 600, color: "#334155" }}>
            {searchTerm ? "No matching APIs found" : "No programmable APIs configured yet"}
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 460, mx: "auto", mt: 0.5, mb: 2.5 }}>
            Create custom programmable endpoints to integrate Easy42 with Prometheus, custom webhooks, bash scripts, or
            third-party monitoring systems.
          </Typography>
          <Button
            variant="contained"
            size="small"
            startIcon={<Plus size={16} />}
            onClick={handleOpenCreate}
            sx={{ bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" }, textTransform: "none" }}
          >
            Create Your First API
          </Button>
        </Paper>
      ) : (
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          {filteredApis.map((item) => {
            const origin = window.location.origin;
            const fullUrl = `${origin}/api/custom/${item.id}`;
            const curlCmd = getCurlSnippet(item);
            const extraHeaderCount = item.extra_headers ? Object.keys(item.extra_headers).length : 0;

            return (
              <Paper
                key={item.id}
                elevation={0}
                sx={{
                  p: 2.5,
                  borderRadius: 2.5,
                  border: item.disabled ? "1px dashed #CBD5E1" : "1px solid #E2E8F0",
                  bgcolor: item.disabled ? "#F8FAFC" : "#FFFFFF",
                  opacity: item.disabled ? 0.9 : 1,
                  transition: "all 0.15s ease",
                  "&:hover": {
                    borderColor: item.disabled ? "#94A3B8" : "#CBD5E1",
                    boxShadow: "0 4px 12px rgba(0, 0, 0, 0.04)",
                  },
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
                  <Box sx={{ flex: 1, minWidth: 260 }}>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 0.75 }}>
                      <Typography
                        variant="subtitle1"
                        sx={{ fontWeight: 700, color: item.disabled ? "#64748B" : "#0F172A" }}
                      >
                        {item.name || item.id}
                      </Typography>
                      {item.disabled ? (
                        <Tooltip title="This API is currently disabled. Incoming requests will receive 403 Forbidden.">
                          <Chip
                            label="Disabled"
                            size="small"
                            sx={{
                              fontSize: "0.75rem",
                              fontWeight: 600,
                              bgcolor: "#FEE2E2",
                              color: "#DC2626",
                              border: "1px solid #FECACA",
                            }}
                          />
                        </Tooltip>
                      ) : (
                        <Chip
                          label="Enabled"
                          size="small"
                          sx={{
                            fontSize: "0.75rem",
                            fontWeight: 600,
                            bgcolor: "#ECFDF5",
                            color: "#047857",
                            border: "1px solid #A7F3D0",
                          }}
                        />
                      )}
                      <Chip
                        label={`/api/custom/${item.id}`}
                        size="small"
                        sx={{
                          fontFamily: "monospace",
                          fontWeight: 600,
                          fontSize: "0.75rem",
                          bgcolor: "#EEF2FF",
                          color: "#4338CA",
                          border: "1px solid #C7D2FE",
                        }}
                      />
                      <Chip
                        label={item.content_type || "application/json"}
                        size="small"
                        sx={{
                          fontSize: "0.75rem",
                          fontWeight: 500,
                          bgcolor: "#F1F5F9",
                          color: "#334155",
                        }}
                      />
                      {item.token ? (
                        <Tooltip title="Authorization Bearer token is required">
                          <Chip
                            icon={<Lock size={12} color="#D97706" />}
                            label="Protected"
                            size="small"
                            sx={{
                              fontSize: "0.75rem",
                              fontWeight: 600,
                              bgcolor: "#FEF3C7",
                              color: "#B45309",
                              border: "1px solid #FDE68A",
                            }}
                          />
                        </Tooltip>
                      ) : (
                        <Tooltip title="Public endpoint (no auth required)">
                          <Chip
                            icon={<Globe size={12} color="#059669" />}
                            label="Public"
                            size="small"
                            sx={{
                              fontSize: "0.75rem",
                              fontWeight: 600,
                              bgcolor: "#ECFDF5",
                              color: "#047857",
                              border: "1px solid #A7F3D0",
                            }}
                          />
                        </Tooltip>
                      )}
                      {extraHeaderCount > 0 && (
                        <Chip
                          label={`${extraHeaderCount} extra header${extraHeaderCount > 1 ? "s" : ""}`}
                          size="small"
                          sx={{ fontSize: "0.75rem", bgcolor: "#F8FAFC", color: "#64748B" }}
                        />
                      )}
                    </Box>

                    {item.description && (
                      <Typography variant="body2" sx={{ color: "#64748B", mb: 1.5 }}>
                        {item.description}
                      </Typography>
                    )}

                    {/* URL and cURL Quick Copy */}
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mt: 1 }}>
                      <Box
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          bgcolor: "#F8FAFC",
                          border: "1px solid #E2E8F0",
                          borderRadius: 1.5,
                          px: 1.25,
                          py: 0.5,
                          fontSize: "0.8rem",
                          fontFamily: "monospace",
                          color: "#334155",
                          maxWidth: "100%",
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                        }}
                      >
                        <span style={{ whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>
                          {fullUrl}
                        </span>
                        <Tooltip title="Copy full URL">
                          <IconButton
                            size="small"
                            onClick={() => handleCopy(`url-${item.id}`, fullUrl)}
                            sx={{ ml: 0.5, p: 0.25 }}
                          >
                            {copiedKey === `url-${item.id}` ? (
                              <Check size={14} color="#16A34A" />
                            ) : (
                              <Copy size={14} color="#64748B" />
                            )}
                          </IconButton>
                        </Tooltip>
                      </Box>

                      <Tooltip title="Copy cURL command">
                        <Button
                          variant="outlined"
                          size="small"
                          startIcon={
                            copiedKey === `curl-${item.id}` ? (
                              <Check size={13} color="#16A34A" />
                            ) : (
                              <Terminal size={13} />
                            )
                          }
                          onClick={() => handleCopy(`curl-${item.id}`, curlCmd)}
                          sx={{
                            textTransform: "none",
                            fontSize: "0.75rem",
                            py: 0.4,
                            px: 1.2,
                            color: "#475569",
                            borderColor: "#CBD5E1",
                          }}
                        >
                          {copiedKey === `curl-${item.id}` ? "Copied cURL!" : "Copy cURL"}
                        </Button>
                      </Tooltip>

                      <Tooltip
                        title={
                          item.disabled ? "API is disabled (returns 403 Forbidden)" : "Open in browser / call endpoint"
                        }
                      >
                        <span>
                          <IconButton
                            size="small"
                            component="a"
                            href={item.token ? `${fullUrl}?token=${encodeURIComponent(item.token)}` : fullUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            disabled={item.disabled}
                            sx={{ color: item.disabled ? "#CBD5E1" : "#475569" }}
                          >
                            <ExternalLink size={15} />
                          </IconButton>
                        </span>
                      </Tooltip>
                    </Box>
                  </Box>

                  {/* Actions */}
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Tooltip title={item.disabled ? "Click to enable API" : "Click to disable API"}>
                      <Box sx={{ display: "flex", alignItems: "center", mr: 0.5 }}>
                        <Switch
                          size="small"
                          checked={!item.disabled}
                          onChange={() => handleToggleDisabled(item)}
                          disabled={loading}
                          color="primary"
                        />
                      </Box>
                    </Tooltip>
                    <Button
                      variant="outlined"
                      size="small"
                      startIcon={<Play size={14} />}
                      onClick={() => handleOpenEdit(item)}
                      sx={{
                        textTransform: "none",
                        color: "#4F46E5",
                        borderColor: "#C7D2FE",
                        "&:hover": { borderColor: "#818CF8", bgcolor: "#EEF2FF" },
                      }}
                    >
                      Test & Edit
                    </Button>
                    <IconButton
                      size="small"
                      onClick={() => handleDeleteAPI(item.id)}
                      sx={{ color: "#94A3B8", "&:hover": { color: "#EF4444", bgcolor: "#FEE2E2" } }}
                    >
                      <Trash2 size={16} />
                    </IconButton>
                  </Box>
                </Box>
              </Paper>
            );
          })}
        </Box>
      )}

      {/* Create / Edit Dialog */}
      <Dialog
        open={dialogOpen}
        onClose={() => !saving && setDialogOpen(false)}
        maxWidth="md"
        fullWidth
        PaperProps={{
          sx: { borderRadius: 3, maxHeight: "90vh" },
        }}
      >
        <DialogTitle sx={{ fontWeight: 700, pb: 1, borderBottom: "1px solid #E2E8F0" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Box
              sx={{
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                p: 0.75,
                borderRadius: 1.5,
                display: "flex",
                alignItems: "center",
              }}
            >
              <Code size={18} />
            </Box>
            <Typography variant="h6" sx={{ fontWeight: 700 }}>
              {editingApi ? `Edit API: ${editingApi.id}` : "New Programmable API"}
            </Typography>
          </Box>
        </DialogTitle>

        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2.5, pb: 2 }}>
          {/* Top row: ID and Content Type */}
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
            <Box sx={{ flex: 1, minWidth: 240 }}>
              <TextField
                label="API ID *"
                placeholder="e.g. cluster-info, prometheus-sd"
                value={formId}
                onChange={(e) => setFormId(e.target.value)}
                disabled={saving}
                fullWidth
                size="small"
                helperText={`URL Path: /api/custom/${formId.trim() || "<id>"}`}
                InputProps={{
                  sx: { fontFamily: "monospace" },
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title="Generate random id">
                        <IconButton size="small" disabled={saving} onClick={() => setFormId(generatePassword(32))}>
                          <Shuffle size={14} />
                        </IconButton>
                      </Tooltip>
                    </InputAdornment>
                  ),
                }}
              />
            </Box>

            <Box sx={{ flex: 1, minWidth: 240 }}>
              <Autocomplete
                freeSolo
                options={PREDEFINED_CONTENT_TYPES}
                value={formContentType}
                onChange={(_, newValue) => setFormContentType(newValue || "application/json")}
                onInputChange={(_, newInputValue) => setFormContentType(newInputValue)}
                disabled={saving}
                renderInput={(params) => (
                  <TextField
                    {...params}
                    label="Content Type *"
                    placeholder="e.g. application/json, text/plain"
                    size="small"
                    helperText="MUI Autocomplete freeSolo (predefined or custom)"
                  />
                )}
              />
            </Box>
          </Box>

          {/* Optional Name & Description */}
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
            <TextField
              label="Display Name (optional)"
              placeholder="e.g. Fleet Overview for External Dashboard"
              value={formName}
              onChange={(e) => setFormName(e.target.value)}
              disabled={saving}
              size="small"
              sx={{ flex: 1, minWidth: 240 }}
            />
            <TextField
              label="Description (optional)"
              placeholder="Brief explanation of consumer software or service"
              value={formDesc}
              onChange={(e) => setFormDesc(e.target.value)}
              disabled={saving}
              size="small"
              sx={{ flex: 1, minWidth: 240 }}
            />
          </Box>

          {/* Status & Disabled Switch */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              p: 1.75,
              borderRadius: 2,
              border: `1px solid ${formDisabled ? "#FECACA" : "#E2E8F0"}`,
              bgcolor: formDisabled ? "#FEF2F2" : "#F8FAFC",
              transition: "all 0.15s ease",
            }}
          >
            <Box sx={{ pr: 2 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.25 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: formDisabled ? "#991B1B" : "#1E293B" }}>
                  API Status: {formDisabled ? "Disabled" : "Enabled"}
                </Typography>
                <Chip
                  label={formDisabled ? "Disabled" : "Enabled"}
                  size="small"
                  sx={{
                    fontSize: "0.7rem",
                    fontWeight: 600,
                    bgcolor: formDisabled ? "#FEE2E2" : "#ECFDF5",
                    color: formDisabled ? "#DC2626" : "#047857",
                    border: `1px solid ${formDisabled ? "#FECACA" : "#A7F3D0"}`,
                    height: 20,
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ color: formDisabled ? "#B91C1C" : "#047857", display: "block" }}>
                {formDisabled
                  ? "This API is disabled. Requests to /api/custom/<id> will return 403 Forbidden."
                  : "This API is enabled. Requests to /api/custom/<id> will execute the template."}
              </Typography>
            </Box>
            <FormControlLabel
              control={
                <Switch
                  checked={!formDisabled}
                  onChange={(e) => setFormDisabled(!e.target.checked)}
                  color={formDisabled ? "error" : "success"}
                  disabled={saving}
                />
              }
              label={
                <Typography
                  variant="body2"
                  sx={{ fontWeight: 600, fontSize: "0.85rem", color: formDisabled ? "#DC2626" : "#047857" }}
                >
                  Enabled
                </Typography>
              }
              labelPlacement="start"
              sx={{ m: 0, flexShrink: 0 }}
            />
          </Box>

          {/* Token (Optional Authorization Bearer token) */}
          <Box>
            <TextField
              label="Bearer Token (optional)"
              placeholder="Leave empty for public API, or enter / generate Bearer token"
              value={formToken}
              type={showToken ? "text" : "password"}
              onChange={(e) => setFormToken(e.target.value)}
              disabled={saving}
              fullWidth
              size="small"
              helperText={
                formToken.trim()
                  ? "Requests to this endpoint must include 'Authorization: Bearer <token>' or '?token=<token>'"
                  : "Public access: anyone can query this endpoint without authentication"
              }
              InputProps={{
                sx: { fontFamily: formToken ? "monospace" : "inherit" },
                endAdornment: (
                  <InputAdornment position="end">
                    <Tooltip title={showToken ? "Hide token" : "Show token"}>
                      <IconButton size="small" onClick={() => setShowToken(!showToken)}>
                        {showToken ? <EyeOff size={16} /> : <Eye size={16} />}
                      </IconButton>
                    </Tooltip>
                    {formToken && (
                      <Tooltip title="Copy token">
                        <IconButton size="small" onClick={() => handleCopy("form-token", formToken)}>
                          {copiedKey === "form-token" ? <Check size={16} color="#16A34A" /> : <Copy size={16} />}
                        </IconButton>
                      </Tooltip>
                    )}
                    <Tooltip title="Generate random secure token">
                      <Button
                        size="small"
                        startIcon={<Sparkles size={14} />}
                        onClick={handleGenerateToken}
                        sx={{ textTransform: "none", ml: 0.5, fontWeight: 600, fontSize: "0.75rem" }}
                      >
                        Generate
                      </Button>
                    </Tooltip>
                  </InputAdornment>
                ),
              }}
            />
          </Box>

          {/* Extra Headers Section */}
          <Box sx={{ border: "1px solid #E2E8F0", borderRadius: 2, p: 2, bgcolor: "#F8FAFC" }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Extra Response Headers (optional)
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Add custom HTTP response headers such as CORS origins or cache-control directives.
                </Typography>
              </Box>
              <Button
                size="small"
                variant="outlined"
                startIcon={<Plus size={14} />}
                onClick={handleAddHeader}
                sx={{ textTransform: "none", fontSize: "0.75rem", color: "#4F46E5", borderColor: "#C7D2FE" }}
              >
                Add Header
              </Button>
            </Box>

            {formHeaders.length === 0 ? (
              <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic", fontSize: "0.85rem" }}>
                No extra headers configured. (Standard Content-Type header will be applied automatically).
              </Typography>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                {formHeaders.map((hdr, idx) => (
                  <Box key={idx} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <TextField
                      size="small"
                      placeholder="Header Name (e.g. Access-Control-Allow-Origin)"
                      value={hdr.key}
                      onChange={(e) => handleHeaderChange(idx, "key", e.target.value)}
                      sx={{ flex: 1, bgcolor: "#FFFFFF" }}
                    />
                    <TextField
                      size="small"
                      placeholder="Header Value (e.g. *)"
                      value={hdr.value}
                      onChange={(e) => handleHeaderChange(idx, "value", e.target.value)}
                      sx={{ flex: 1, bgcolor: "#FFFFFF" }}
                    />
                    <IconButton size="small" onClick={() => handleRemoveHeader(idx)} sx={{ color: "#94A3B8" }}>
                      <Trash2 size={16} />
                    </IconButton>
                  </Box>
                ))}
              </Box>
            )}
          </Box>

          {/* Template Presets & Code Editor */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            <Box
              sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 1 }}
            >
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                  API Template (Go text/template) *
                </Typography>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Context contains <code>.config</code>, <code>.state</code>, and <code>.request</code> object.
                </Typography>
              </Box>

              {/* Template Presets */}
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                  Load Starter:
                </Typography>
                {TEMPLATE_PRESETS.map((preset, idx) => (
                  <Button
                    key={idx}
                    size="small"
                    variant="text"
                    onClick={() => handleApplyPreset(preset)}
                    sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.25, px: 0.75, color: "#4F46E5" }}
                  >
                    {preset.name.split(" ")[0]}
                  </Button>
                ))}
              </Box>
            </Box>

            <TextField
              multiline
              minRows={11}
              maxRows={24}
              value={formTemplate}
              onChange={(e) => setFormTemplate(e.target.value)}
              disabled={saving}
              fullWidth
              placeholder="Enter Go text/template code here..."
              sx={{
                "& .MuiInputBase-root": {
                  backgroundColor: "#0F172A !important",
                  borderRadius: "8px",
                  p: 1.5,
                  border: "1px solid #334155",
                  "&:hover": {
                    borderColor: "#64748B",
                  },
                  "&.Mui-focused": {
                    borderColor: "#6366F1",
                  },
                },
                "& .MuiOutlinedInput-notchedOutline": {
                  border: "none",
                },
                "& .MuiInputBase-input, & textarea": {
                  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace !important",
                  fontSize: "0.85rem !important",
                  lineHeight: 1.6,
                  color: "#F8FAFC !important",
                  WebkitTextFillColor: "#F8FAFC !important",
                  caretColor: "#38BDF8 !important",
                  p: 0,
                },
                "& .MuiInputBase-input::placeholder, & textarea::placeholder": {
                  color: "#64748B !important",
                  WebkitTextFillColor: "#64748B !important",
                  opacity: 1,
                },
              }}
              inputProps={{
                style: {
                  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
                  fontSize: "0.85rem",
                  lineHeight: 1.6,
                  color: "#F8FAFC",
                  WebkitTextFillColor: "#F8FAFC",
                },
              }}
            />
          </Box>

          {/* Context Reference Guide Accordion */}
          <Accordion
            elevation={0}
            sx={{
              border: "1px solid #E2E8F0",
              borderRadius: "8px !important",
              "&:before": { display: "none" },
              bgcolor: "#F8FAFC",
            }}
          >
            <AccordionSummary expandIcon={<ChevronDown size={16} />}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <FileText size={16} color="#4F46E5" />
                <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#334155" }}>
                  Template Context Reference & Functions Guide
                </Typography>
              </Box>
            </AccordionSummary>
            <AccordionDetails sx={{ pt: 0 }}>
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1, fontSize: "0.82rem", color: "#475569" }}>
                <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  Available in Context:
                </Typography>
                <ul style={{ margin: 0, paddingLeft: 20 }}>
                  <li>
                    <code>.config</code> or <code>.Config</code>: Easy42 cluster configuration (e.g.{" "}
                    <code>.config.nodes</code>, <code>.config.links</code>, <code>.config.network_settings</code>).
                  </li>
                  <li>
                    <code>.state</code> or <code>.State</code>: Runtime cluster state from database (e.g.{" "}
                    <code>.state.nodes</code>, telemetry).
                  </li>
                  <li>
                    <code>.request</code>: Recursive map formatted like JavaScript <code>Request</code> object:
                    <ul style={{ marginTop: 4 }}>
                      <li>
                        <code>.request.url</code>: full requested URL string
                      </li>
                      <li>
                        <code>.request.method</code>: HTTP method (e.g. <code>"GET"</code>, <code>"POST"</code>)
                      </li>
                      <li>
                        <code>.request.path</code>: request URI path (e.g. <code>/api/custom/...</code>)
                      </li>
                      <li>
                        <code>.request.query</code>: query parameters map (e.g. <code>.request.query.filter</code>)
                      </li>
                      <li>
                        <code>.request.headers</code>: lowercased headers map (e.g.{" "}
                        <code>.request.headers.authorization</code>)
                      </li>
                      <li>
                        <code>.request.body</code>: parsed body (parsed as recursive JSON map/array if JSON, or string)
                      </li>
                      <li>
                        <code>.request.client_ip</code>: client IP address
                      </li>
                    </ul>
                  </li>
                  <li>
                    <a href="https://github.com/go-sprout/sprout" target="_blank" rel="noopener noreferrer">
                      <strong>Sprout Functions</strong>
                    </a>
                    : <code>toJson</code>, <code>toPrettyJson</code>, <code>toYaml</code>, <code>len</code>,{" "}
                    <code>index</code>, <code>default</code>, <code>split</code>, <code>join</code>, <code>upper</code>,{" "}
                    <code>lower</code>, <code>trim</code>, <code>eval</code> (JavaScript runtime).
                  </li>
                </ul>
              </Box>
            </AccordionDetails>
          </Accordion>

          {/* Test / Preview Runner */}
          <Box sx={{ border: "1px solid #E2E8F0", borderRadius: 2, p: 2, bgcolor: "#FFFFFF" }}>
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Play size={16} color="#16A34A" />
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                  Live Test & Preview
                </Typography>
              </Box>
              <Button
                variant="contained"
                size="small"
                startIcon={testing ? <CircularProgress size={14} color="inherit" /> : <Play size={14} />}
                onClick={handleTestAPI}
                disabled={testing || !formTemplate.trim()}
                sx={{
                  bgcolor: "#16A34A",
                  "&:hover": { bgcolor: "#15803D" },
                  textTransform: "none",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                }}
              >
                {testing ? "Testing..." : "Run Test"}
              </Button>
            </Box>

            {/* Test input controls */}
            <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mb: 1.5 }}>
              <TextField
                select
                size="small"
                label="Method"
                value={testMethod}
                onChange={(e) => setTestMethod(e.target.value)}
                SelectProps={{ native: true }}
                sx={{ width: 110 }}
              >
                <option value="GET">GET</option>
                <option value="POST">POST</option>
                <option value="PUT">PUT</option>
                <option value="DELETE">DELETE</option>
              </TextField>

              <TextField
                size="small"
                label="Simulate Query Params"
                placeholder="e.g. filter=active&tag=edge"
                value={testQueryParam}
                onChange={(e) => setTestQueryParam(e.target.value)}
                sx={{ flex: 1, minWidth: 200 }}
              />

              {testMethod !== "GET" && (
                <TextField
                  size="small"
                  label="Simulate JSON Request Body"
                  placeholder='{"key": "value"}'
                  value={testBody}
                  onChange={(e) => setTestBody(e.target.value)}
                  sx={{ flex: 1, minWidth: 200 }}
                />
              )}
            </Box>

            {/* Test output display */}
            {testError && (
              <Alert severity="error" sx={{ borderRadius: 1.5, fontSize: "0.85rem" }}>
                {testError}
              </Alert>
            )}

            {testResult && (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography variant="caption" sx={{ fontWeight: 600, color: "#16A34A" }}>
                    Status: 200 OK | Content-Type: {testResult.content_type}
                  </Typography>
                  <Button
                    size="small"
                    startIcon={<Copy size={13} />}
                    onClick={() => handleCopy("test-output", testResult.output)}
                    sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.2 }}
                  >
                    {copiedKey === "test-output" ? "Copied!" : "Copy Output"}
                  </Button>
                </Box>
                <Box
                  sx={{
                    p: 1.5,
                    borderRadius: 1.5,
                    bgcolor: "#0F172A",
                    color: "#38BDF8",
                    fontFamily: "monospace",
                    fontSize: "0.8rem",
                    maxHeight: 200,
                    overflowY: "auto",
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-all",
                  }}
                >
                  {testResult.output || "(Empty output rendered)"}
                </Box>
              </Box>
            )}
          </Box>
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0" }}>
          <Button
            onClick={() => setDialogOpen(false)}
            disabled={saving}
            sx={{ textTransform: "none", color: "#64748B" }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleSaveAPI}
            disabled={saving || !formId.trim() || !formTemplate.trim()}
            startIcon={saving && <CircularProgress size={14} color="inherit" />}
            sx={{
              bgcolor: "#4F46E5",
              "&:hover": { bgcolor: "#4338CA" },
              fontWeight: 600,
              textTransform: "none",
              px: 3,
            }}
          >
            {saving ? "Saving..." : editingApi ? "Update API" : "Create API"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
