import React from "react";
import {
  Drawer,
  Box,
  Typography,
  IconButton,
  Chip,
  Divider,
  Tooltip,
  Button,
} from "@mui/material";
import {
  X,
  Layers,
  FileText,
  Globe,
  ExternalLink,
  Copy,
  Check,
  Clock,
  Box as BoxIcon,
  Cpu,
  FileCode,
  Calendar,
  Tag,
  Package,
} from "lucide-react";
import { MarkdownView } from "../Common/MarkdownView";
import type { App, AppStatusRecord } from "../../types/api";

interface AppDetailsDrawerProps {
  app: App | null;
  open: boolean;
  onClose: () => void;
  targetNodeNames?: string[];
  targetModeLabel?: string;
  status?: AppStatusRecord;
  homepageUrl?: string;
  homepageNode?: string;
  homepageClickable?: boolean;
  onEdit?: (app: App) => void;
}

const formatDate = (iso?: string): string => {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString();
};

const getTypeLabel = (t: string) =>
  t === "docker" ? "Docker" : t === "cron" ? "Cron" : "Systemd";

export const AppDetailsDrawer: React.FC<AppDetailsDrawerProps> = ({
  app,
  open,
  onClose,
  targetNodeNames,
  targetModeLabel,
  status,
  homepageUrl,
  homepageNode,
  homepageClickable,
  onEdit,
}) => {
  const [copied, setCopied] = React.useState<string | null>(null);

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  if (!app) return null;

  const displayUrl = homepageUrl !== undefined ? homepageUrl : app.url;
  const isClickable = homepageClickable !== undefined ? homepageClickable : !!displayUrl && /^https?:\/\//i.test(displayUrl);
  const fileEntries = Object.entries(app.files || {});
  const selectorEntries = Object.entries(app.selector || {});

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100%", sm: 480 },
          backgroundColor: "#FFFFFF",
          borderLeft: "1px solid #E2E8F0",
          p: 3,
        },
      }}
    >
      {/* Header */}
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, minWidth: 0 }}>
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              backgroundColor: "rgba(79, 70, 229, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#4F46E5",
              flexShrink: 0,
            }}
          >
            {app.app_type === "docker" ? (
              <BoxIcon size={20} />
            ) : app.app_type === "cron" ? (
              <Clock size={20} />
            ) : (
              <Cpu size={20} />
            )}
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
              <Typography variant="h6" sx={{ fontWeight: 800, lineHeight: 1.2, color: "#0F172A" }}>
                App Details
              </Typography>
            </Box>
            <Typography variant="caption" className="mono-font" sx={{ color: "#64748B" }}>
              {app.name}
            </Typography>
          </Box>
        </Box>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          {onEdit && (
            <Button
              size="small"
              onClick={() => onEdit(app)}
              sx={{ textTransform: "none", fontSize: "0.78rem", fontWeight: 600 }}
            >
              Edit
            </Button>
          )}
          <IconButton size="small" onClick={onClose} sx={{ color: "#94A3B8" }}>
            <X size={20} />
          </IconButton>
        </Box>
      </Box>

      <Divider sx={{ borderColor: "#E2E8F0", mb: 2.5 }} />

      <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
        {/* Identity & Status */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Chip
              size="small"
              label={getTypeLabel(app.app_type)}
              sx={{
                height: 22,
                fontSize: "0.72rem",
                fontWeight: 700,
                bgcolor:
                  app.app_type === "docker" ? "#E0F2FE" : app.app_type === "cron" ? "#FEF3C7" : "#F3E8FF",
                color:
                  app.app_type === "docker" ? "#0369A1" : app.app_type === "cron" ? "#B45309" : "#7E22CE",
              }}
            />
            <Chip
              size="small"
              label={app.published ? "Published" : "Draft"}
              sx={{
                height: 22,
                fontSize: "0.72rem",
                fontWeight: 700,
                bgcolor: app.published ? "#ECFDF5" : "#FEF3C7",
                color: app.published ? "#059669" : "#D97706",
              }}
            />
            <Chip
              size="small"
              label={app.enabled ? "Enabled" : "Disabled"}
              sx={{
                height: 22,
                fontSize: "0.72rem",
                fontWeight: 700,
                bgcolor: app.enabled ? "#EEF2FF" : "#F1F5F9",
                color: app.enabled ? "#4F46E5" : "#64748B",
              }}
            />
            {app.group && (
              <Chip
                size="small"
                label={`Group: ${app.group}`}
                sx={{
                  height: 22,
                  fontSize: "0.72rem",
                  fontWeight: 700,
                  bgcolor: "#F1F5F9",
                  color: "#475569",
                  border: "1px solid #E2E8F0",
                }}
              />
            )}
            {status && (
              <Chip
                size="small"
                label={status.state ? status.state.toUpperCase() : "UNKNOWN"}
                sx={{
                  height: 22,
                  fontSize: "0.72rem",
                  fontWeight: 800,
                  bgcolor:
                    status.state?.toLowerCase() === "running"
                      ? "#ECFDF5"
                      : status.state?.toLowerCase() === "failed"
                        ? "#FEF2F2"
                        : "#F1F5F9",
                  color:
                    status.state?.toLowerCase() === "running"
                      ? "#059669"
                      : status.state?.toLowerCase() === "failed"
                        ? "#DC2626"
                        : "#64748B",
                }}
              />
            )}
          </Box>

          {app.description ? (
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "block", mb: 0.5 }}>
                DESCRIPTION
              </Typography>
              <Typography variant="body2" sx={{ color: "#334155", fontSize: "0.85rem", lineHeight: 1.5 }}>
                {app.description}
              </Typography>
            </Box>
          ) : (
            <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
              No description provided.
            </Typography>
          )}

          {/* Homepage URL */}
          {displayUrl && (
            <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1, mb: 0.5 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.5 }}>
                  <Globe size={12} /> HOMEPAGE URL
                </Typography>
                <Tooltip title={copied === "url" ? "Copied!" : "Copy URL"}>
                  <IconButton size="small" onClick={() => handleCopy("url", displayUrl)} sx={{ p: 0.3 }}>
                    {copied === "url" ? <Check size={13} color="#059669" /> : <Copy size={13} />}
                  </IconButton>
                </Tooltip>
              </Box>
              {homepageNode && (
                <Typography variant="caption" sx={{ color: "#94A3B8", display: "block", mb: 0.5 }}>
                  Resolved on node: {homepageNode}
                </Typography>
              )}
              {isClickable ? (
                <Box
                  component="a"
                  href={displayUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  sx={{
                    display: "inline-flex",
                    alignItems: "center",
                    gap: 0.5,
                    color: "#2563EB",
                    textDecoration: "none",
                    fontSize: "0.8rem",
                    fontWeight: 600,
                    wordBreak: "break-all",
                    "&:hover": { textDecoration: "underline" },
                  }}
                >
                  {displayUrl}
                  <ExternalLink size={13} />
                </Box>
              ) : (
                <Typography variant="body2" className="mono-font" sx={{ color: "#334155", fontSize: "0.8rem", wordBreak: "break-all" }}>
                  {displayUrl}
                </Typography>
              )}
            </Box>
          )}

          {/* Target Nodes */}
          <Box sx={{ p: 1.5, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.5, mb: 0.8 }}>
              <Tag size={12} /> TARGET NODES
              {targetModeLabel && (
                <Chip size="small" label={targetModeLabel} sx={{ height: 18, fontSize: "0.65rem", fontWeight: 700, ml: 1 }} />
              )}
            </Typography>
            {targetNodeNames && targetNodeNames.length > 0 ? (
              <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                {targetNodeNames.map((n) => (
                  <Chip key={n} label={n} size="small" sx={{ height: 22, fontSize: "0.72rem", fontWeight: 600, bgcolor: "#EEF2FF", color: "#4338CA" }} />
                ))}
              </Box>
            ) : (
              <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                No nodes currently match.
              </Typography>
            )}
            {selectorEntries.length > 0 && (
              <Box sx={{ mt: 1 }}>
                <Typography variant="caption" sx={{ color: "#94A3B8", fontWeight: 600 }}>Selector:</Typography>
                <Box
                  component="pre"
                  sx={{
                    m: 0,
                    mt: 0.5,
                    p: 1,
                    bgcolor: "#FFFFFF",
                    borderRadius: 1,
                    border: "1px solid #E2E8F0",
                    fontFamily: "monospace",
                    fontSize: "0.72rem",
                    color: "#334155",
                    overflowX: "auto",
                  }}
                >
                  {JSON.stringify(Object.fromEntries(selectorEntries), null, 2)}
                </Box>
              </Box>
            )}
            {status?.status_text && (
              <Typography variant="caption" sx={{ color: "#64748B", display: "block", mt: 1 }}>
                Status: {status.status_text}
              </Typography>
            )}
            {status?.error_message && (
              <Typography variant="caption" sx={{ color: "#DC2626", display: "block", mt: 0.5 }}>
                Error: {status.error_message}
              </Typography>
            )}
          </Box>

          {/* Created / Updated */}
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
              <Calendar size={13} color="#64748B" />
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Created: <strong style={{ color: "#334155" }}>{formatDate(app.created_at)}</strong>
              </Typography>
            </Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
              <Calendar size={13} color="#64748B" />
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Updated: <strong style={{ color: "#334155" }}>{formatDate(app.updated_at)}</strong>
              </Typography>
            </Box>
          </Box>
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            ID: <span className="mono-font">{app.id}</span>
          </Typography>
        </Box>

        {/* Note (Markdown) */}
        <Box sx={{ p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
          <Typography
            variant="caption"
            sx={{
              color: "#475569",
              fontWeight: 700,
              display: "flex",
              alignItems: "center",
              gap: 0.8,
              letterSpacing: "0.5px",
              mb: 1.2,
            }}
          >
            <FileText size={14} color="#4F46E5" /> APP NOTE (MARKDOWN)
          </Typography>
          <Box
            sx={{
              p: 1.5,
              borderRadius: 1.5,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
              maxHeight: 400,
              overflowY: "auto",
            }}
          >
            <MarkdownView content={app.note} emptyText="No note recorded for this app." />
          </Box>
        </Box>

        {/* Definition */}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, letterSpacing: "0.5px", display: "flex", alignItems: "center", gap: 0.8 }}>
            <FileCode size={14} color="#4F46E5" /> DEFINITION
          </Typography>

          <Box>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block", mb: 0.5 }}>
              {app.app_type === "docker" ? "compose.yaml" : app.app_type === "cron" ? "Cron schedule & command" : "Systemd unit"}
            </Typography>
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 1.5,
                bgcolor: "#0F172A",
                color: "#F8FAFC",
                borderRadius: 1.5,
                fontFamily: "monospace",
                fontSize: "0.75rem",
                lineHeight: 1.5,
                maxHeight: 260,
                overflowY: "auto",
                overflowX: "auto",
                whiteSpace: "pre",
              }}
            >
              {app.main_content || "(empty)"}
            </Box>
          </Box>

          {app.env_content && (
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block", mb: 0.5 }}>
                Environment (.env)
              </Typography>
              <Box
                component="pre"
                sx={{
                  m: 0,
                  p: 1.5,
                  bgcolor: "#1E293B",
                  color: "#F1F5F9",
                  borderRadius: 1.5,
                  fontFamily: "monospace",
                  fontSize: "0.75rem",
                  lineHeight: 1.5,
                  maxHeight: 180,
                  overflowY: "auto",
                  overflowX: "auto",
                  whiteSpace: "pre",
                }}
              >
                {app.env_content}
              </Box>
            </Box>
          )}

          {/* Artifact Management (Native Systemd apps) */}
          {app.artifact && (
            <Box sx={{ p: 2, bgcolor: "#F8FAFC", borderRadius: 2, border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.8 }}>
                  <Package size={14} color="#4F46E5" /> ARTIFACT MANAGEMENT
                </Typography>
                {app.version && (
                  <Chip size="small" label={`v: ${app.version}`} sx={{ height: 20, fontSize: "0.68rem", fontWeight: 700, bgcolor: "#EEF2FF", color: "#4F46E5" }} />
                )}
              </Box>

              {app.artifact.url && (
                <Box sx={{ mb: 1.5 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block" }}>
                    URL Template:
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ color: "#334155", fontSize: "0.75rem", wordBreak: "break-all" }}>
                    {app.artifact.url}
                  </Typography>
                </Box>
              )}

              {app.artifact.resolver && (
                <Box sx={{ mb: 1.5, display: "flex", flexDirection: "column", gap: 0.5 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    Resolver ({app.artifact.resolver.type}):
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ color: "#334155", fontSize: "0.75rem" }}>
                    Repo: <strong>{app.artifact.resolver.repo}</strong> {app.artifact.resolver.version && `(tag: ${app.artifact.resolver.version})`}
                  </Typography>
                  {app.artifact.resolver.asset_pattern && (
                    <Typography variant="caption" className="mono-font" sx={{ color: "#64748B" }}>
                      Pattern: {app.artifact.resolver.asset_pattern}
                    </Typography>
                  )}
                </Box>
              )}

              {app.artifact.files && app.artifact.files.length > 0 && (
                <Box sx={{ mt: 1 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block", mb: 0.5 }}>
                    Extracted Files ({app.artifact.files.length}):
                  </Typography>
                  <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
                    {app.artifact.files.map((f, i) => (
                      <Box key={i} sx={{ p: 1, bgcolor: "#FFFFFF", borderRadius: 1.5, border: "1px solid #E2E8F0" }}>
                        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                          <Typography variant="caption" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            {f.name}
                          </Typography>
                          <Chip size="small" label={f.pull_policy || "if_changed"} sx={{ height: 18, fontSize: "0.62rem" }} />
                        </Box>
                        <Typography variant="caption" className="mono-font" sx={{ color: "#475569", display: "block", mt: 0.3 }}>
                          Target: {f.target_path}
                        </Typography>
                        {f.extract_path && (
                          <Typography variant="caption" className="mono-font" sx={{ color: "#94A3B8", display: "block" }}>
                            Extract: {f.extract_path}
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Box>
                </Box>
              )}
            </Box>
          )}

          {fileEntries.length > 0 && (
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600, display: "block", mb: 0.8 }}>
                CONFIG FILES ({fileEntries.length})
              </Typography>
              {fileEntries.map(([name, content]) => (
                <Box key={name} sx={{ mb: 1 }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#334155", fontWeight: 600 }}>
                      {name}
                    </Typography>
                    <Tooltip title={copied === name ? "Copied!" : "Copy"}>
                      <IconButton size="small" onClick={() => handleCopy(name, content)} sx={{ p: 0.3 }}>
                        {copied === name ? <Check size={13} color="#059669" /> : <Copy size={13} />}
                      </IconButton>
                    </Tooltip>
                  </Box>
                  <Box
                    component="pre"
                    sx={{
                      m: 0,
                      p: 1.2,
                      bgcolor: "#F8FAFC",
                      color: "#1E293B",
                      borderRadius: 1.5,
                      border: "1px solid #E2E8F0",
                      fontFamily: "monospace",
                      fontSize: "0.72rem",
                      lineHeight: 1.5,
                      maxHeight: 160,
                      overflowY: "auto",
                      overflowX: "auto",
                      whiteSpace: "pre",
                    }}
                  >
                    {content || "(empty)"}
                  </Box>
                </Box>
              ))}
            </Box>
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, pb: 1 }}>
          <Layers size={13} color="#94A3B8" />
          <Typography variant="caption" sx={{ color: "#94A3B8" }}>
            Service name on nodes: ezapp-{app.name}
          </Typography>
        </Box>
      </Box>
    </Drawer>
  );
};
