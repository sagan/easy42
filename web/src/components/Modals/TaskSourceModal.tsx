import React, { useState, useEffect } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  CircularProgress,
  Alert,
  Chip,
  IconButton,
  Tooltip,
  useTheme,
  useMediaQuery,
} from "@mui/material";
import {
  Code2,
  FileCode,
  FileText,
  Copy,
  Check,
  X,
  Layers,
  Sparkles,
} from "lucide-react";
import { api } from "../../api/client";
import { TaskMeta, TaskSourceFile } from "../../types/api";

interface TaskSourceModalProps {
  open: boolean;
  onClose: () => void;
  task: TaskMeta | null;
}

export const TaskSourceModal: React.FC<TaskSourceModalProps> = ({ open, onClose, task }) => {
  const [sources, setSources] = useState<TaskSourceFile[]>([]);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedFileName, setSelectedFileName] = useState<string>("");
  const [copied, setCopied] = useState<boolean>(false);

  useEffect(() => {
    if (!open || !task) {
      return;
    }

    let isMounted = true;
    setLoading(true);
    setError(null);
    setCopied(false);

    api
      .getTaskSources(task.id)
      .then((data) => {
        if (!isMounted) return;
        setSources(data);
        if (data.length > 0) {
          // Default to run.sh if present, otherwise first file
          const defaultFile = data.find((f) => f.name === "run.sh") || data[0];
          setSelectedFileName(defaultFile.name);
        } else {
          setSelectedFileName("");
        }
      })
      .catch((err: unknown) => {
        if (!isMounted) return;
        const e = err as Error;
        setError(e.message || "Failed to load task source scripts");
      })
      .finally(() => {
        if (isMounted) setLoading(false);
      });

    return () => {
      isMounted = false;
    };
  }, [open, task]);

  const activeFile = sources.find((f) => f.name === selectedFileName) || sources[0] || null;

  const handleCopy = () => {
    if (!activeFile) return;
    navigator.clipboard.writeText(activeFile.content);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const lines = activeFile ? activeFile.content.split("\n") : [];
  const lineCount = lines.length;
  const charCount = activeFile ? activeFile.content.length : 0;

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    return `${(bytes / 1024).toFixed(1)} KB`;
  };

  // Separate task files from shared files
  const taskFiles = sources.filter((f) => !f.shared && !f.name.endsWith(".json"));
  const sharedFiles = sources.filter((f) => f.shared);
  const configFiles = sources.filter((f) => f.name.endsWith(".json"));

  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("sm"));

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      fullScreen={isMobile}
      PaperProps={{
        sx: {
          borderRadius: { xs: 0, sm: 3 },
          height: { xs: "100%", sm: "85vh" },
          maxHeight: { xs: "100%", sm: 850 },
        },
      }}
    >
      {/* Dialog Header */}
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          py: 1.5,
          px: { xs: 2, sm: 2.5 },
          borderBottom: "1px solid #E2E8F0",
          bgcolor: "background.paper",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, minWidth: 0 }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: 2,
              bgcolor: "primary.50",
              color: "primary.main",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
            }}
          >
            <Code2 size={20} />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
              <Typography
                variant="subtitle1"
                sx={{
                  fontWeight: 700,
                  lineHeight: 1.2,
                  whiteSpace: "nowrap",
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                }}
              >
                {task?.title || "Task Source"}
              </Typography>
              {task?.category && (
                <Chip
                  size="small"
                  label={task.category}
                  color="primary"
                  variant="outlined"
                  sx={{ height: 20, fontSize: "0.7rem" }}
                />
              )}
            </Box>
            <Typography variant="caption" sx={{ color: "text.secondary", fontFamily: "monospace", display: "block" }}>
              Task ID: {task?.id}
            </Typography>
          </Box>
        </Box>
        <IconButton size="small" onClick={onClose} sx={{ color: "text.secondary", ml: 1 }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      {/* Dialog Body */}
      <DialogContent sx={{ p: 0, display: "flex", flexDirection: { xs: "column", md: "row" }, overflow: "hidden", flex: 1 }}>
        {/* Left Sidebar: File List (Desktop) */}
        <Box
          sx={{
            display: { xs: "none", md: "flex" },
            width: 240,
            borderRight: "1px solid #E2E8F0",
            bgcolor: "#F8FAFC",
            flexDirection: "column",
            flexShrink: 0,
            overflowY: "auto",
          }}
        >
          <Box sx={{ p: 1.5, pb: 1 }}>
            <Typography
              variant="caption"
              sx={{ fontWeight: 700, textTransform: "uppercase", letterSpacing: 0.5, color: "#64748B", px: 1, display: "block", mb: 0.5 }}
            >
              Source Files
            </Typography>
          </Box>

          {loading ? (
            <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
              <CircularProgress size={20} />
            </Box>
          ) : sources.length === 0 ? (
            <Typography variant="caption" color="text.secondary" sx={{ p: 2, textAlign: "center" }}>
              No source files found
            </Typography>
          ) : (
            <Box sx={{ px: 1, pb: 2, display: "flex", flexDirection: "column", gap: 0.5 }}>
              {/* Task Scripts */}
              {taskFiles.length > 0 && (
                <>
                  <Typography variant="caption" sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#94A3B8", px: 1, pt: 0.5 }}>
                    Task Scripts
                  </Typography>
                  {taskFiles.map((f) => {
                    const isSelected = activeFile?.name === f.name;
                    return (
                      <Box
                        key={f.name}
                        onClick={() => setSelectedFileName(f.name)}
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          px: 1.2,
                          py: 0.8,
                          borderRadius: 1.5,
                          cursor: "pointer",
                          bgcolor: isSelected ? "rgba(79, 70, 229, 0.1)" : "transparent",
                          color: isSelected ? "primary.main" : "#334155",
                          border: "1px solid",
                          borderColor: isSelected ? "primary.light" : "transparent",
                          transition: "all 0.15s ease",
                          "&:hover": {
                            bgcolor: isSelected ? "rgba(79, 70, 229, 0.14)" : "rgba(0, 0, 0, 0.04)",
                          },
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
                          <FileCode size={15} color={isSelected ? "#4F46E5" : "#64748B"} />
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: isSelected ? 700 : 500,
                              fontSize: "0.82rem",
                              fontFamily: "monospace",
                              whiteSpace: "nowrap",
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                            }}
                          >
                            {f.name}
                          </Typography>
                        </Box>
                      </Box>
                    );
                  })}
                </>
              )}

              {/* Shared Scripts */}
              {sharedFiles.length > 0 && (
                <>
                  <Typography variant="caption" sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#94A3B8", px: 1, pt: 1.5 }}>
                    Shared Libraries
                  </Typography>
                  {sharedFiles.map((f) => {
                    const isSelected = activeFile?.name === f.name;
                    return (
                      <Box
                        key={f.name}
                        onClick={() => setSelectedFileName(f.name)}
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          px: 1.2,
                          py: 0.8,
                          borderRadius: 1.5,
                          cursor: "pointer",
                          bgcolor: isSelected ? "rgba(79, 70, 229, 0.1)" : "transparent",
                          color: isSelected ? "primary.main" : "#334155",
                          border: "1px solid",
                          borderColor: isSelected ? "primary.light" : "transparent",
                          transition: "all 0.15s ease",
                          "&:hover": {
                            bgcolor: isSelected ? "rgba(79, 70, 229, 0.14)" : "rgba(0, 0, 0, 0.04)",
                          },
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
                          <Layers size={15} color={isSelected ? "#4F46E5" : "#0284C7"} />
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: isSelected ? 700 : 500,
                              fontSize: "0.82rem",
                              fontFamily: "monospace",
                              whiteSpace: "nowrap",
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                            }}
                          >
                            {f.name}
                          </Typography>
                        </Box>
                        <Chip
                          size="small"
                          label="shared"
                          sx={{
                            height: 16,
                            fontSize: "0.62rem",
                            fontWeight: 600,
                            bgcolor: isSelected ? "primary.main" : "#E2E8F0",
                            color: isSelected ? "#fff" : "#475569",
                          }}
                        />
                      </Box>
                    );
                  })}
                </>
              )}

              {/* Configuration Files */}
              {configFiles.length > 0 && (
                <>
                  <Typography variant="caption" sx={{ fontSize: "0.68rem", fontWeight: 700, color: "#94A3B8", px: 1, pt: 1.5 }}>
                    Configuration
                  </Typography>
                  {configFiles.map((f) => {
                    const isSelected = activeFile?.name === f.name;
                    return (
                      <Box
                        key={f.name}
                        onClick={() => setSelectedFileName(f.name)}
                        sx={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          px: 1.2,
                          py: 0.8,
                          borderRadius: 1.5,
                          cursor: "pointer",
                          bgcolor: isSelected ? "rgba(79, 70, 229, 0.1)" : "transparent",
                          color: isSelected ? "primary.main" : "#334155",
                          border: "1px solid",
                          borderColor: isSelected ? "primary.light" : "transparent",
                          transition: "all 0.15s ease",
                          "&:hover": {
                            bgcolor: isSelected ? "rgba(79, 70, 229, 0.14)" : "rgba(0, 0, 0, 0.04)",
                          },
                        }}
                      >
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
                          <FileText size={15} color={isSelected ? "#4F46E5" : "#64748B"} />
                          <Typography
                            variant="body2"
                            sx={{
                              fontWeight: isSelected ? 700 : 500,
                              fontSize: "0.82rem",
                              fontFamily: "monospace",
                              whiteSpace: "nowrap",
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                            }}
                          >
                            {f.name}
                          </Typography>
                        </Box>
                      </Box>
                    );
                  })}
                </>
              )}
            </Box>
          )}
        </Box>

        {/* Right Main Area: Code Viewer */}
        <Box sx={{ flex: 1, display: "flex", flexDirection: "column", bgcolor: "#0B0F19", minWidth: 0, overflow: "hidden" }}>
          {/* Mobile Horizontal File Switcher */}
          {sources.length > 0 && (
            <Box
              sx={{
                display: { xs: "flex", md: "none" },
                alignItems: "center",
                overflowX: "auto",
                py: 1,
                px: 1.5,
                gap: 1,
                bgcolor: "#F8FAFC",
                borderBottom: "1px solid #E2E8F0",
                flexShrink: 0,
                "&::-webkit-scrollbar": { height: 4 },
                "&::-webkit-scrollbar-thumb": { backgroundColor: "#CBD5E1", borderRadius: 2 },
              }}
            >
              {sources.map((f) => {
                const isSelected = activeFile?.name === f.name;
                return (
                  <Box
                    key={f.name}
                    onClick={() => setSelectedFileName(f.name)}
                    sx={{
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 0.75,
                      px: 1.25,
                      py: 0.6,
                      borderRadius: 1.5,
                      cursor: "pointer",
                      whiteSpace: "nowrap",
                      flexShrink: 0,
                      bgcolor: isSelected ? "rgba(79, 70, 229, 0.1)" : "#FFFFFF",
                      color: isSelected ? "primary.main" : "#334155",
                      border: "1px solid",
                      borderColor: isSelected ? "primary.light" : "#E2E8F0",
                      transition: "all 0.15s ease",
                      "&:active": {
                        transform: "scale(0.97)",
                      },
                    }}
                  >
                    {f.shared ? (
                      <Layers size={14} color={isSelected ? "#4F46E5" : "#0284C7"} />
                    ) : f.name.endsWith(".json") ? (
                      <FileText size={14} color={isSelected ? "#4F46E5" : "#64748B"} />
                    ) : (
                      <FileCode size={14} color={isSelected ? "#4F46E5" : "#64748B"} />
                    )}
                    <Typography
                      variant="body2"
                      sx={{
                        fontWeight: isSelected ? 700 : 500,
                        fontSize: "0.78rem",
                        fontFamily: "monospace",
                      }}
                    >
                      {f.name}
                    </Typography>
                    {f.shared && (
                      <Chip
                        size="small"
                        label="shared"
                        sx={{
                          height: 16,
                          fontSize: "0.6rem",
                          fontWeight: 600,
                          bgcolor: isSelected ? "primary.main" : "#E2E8F0",
                          color: isSelected ? "#fff" : "#475569",
                        }}
                      />
                    )}
                  </Box>
                );
              })}
            </Box>
          )}

          {error && (
            <Alert severity="error" sx={{ m: 2 }} onClose={() => setError(null)}>
              {error}
            </Alert>
          )}

          {loading ? (
            <Box sx={{ flex: 1, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 2 }}>
              <CircularProgress size={32} />
              <Typography variant="body2" sx={{ color: "#94A3B8" }}>
                Loading source code...
              </Typography>
            </Box>
          ) : activeFile ? (
            <>
              {/* Code Viewer Toolbar */}
              <Box
                sx={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  px: { xs: 1.5, sm: 2 },
                  py: 1,
                  bgcolor: "#111827",
                  borderBottom: "1px solid #1F2937",
                  gap: 1,
                }}
              >
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0, flexWrap: "wrap", flex: 1 }}>
                  <Typography
                    variant="body2"
                    sx={{
                      color: "#F3F4F6",
                      fontFamily: "monospace",
                      fontWeight: 600,
                      fontSize: { xs: "0.78rem", sm: "0.85rem" },
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {activeFile.path}
                  </Typography>
                  {activeFile.shared && (
                    <Chip
                      size="small"
                      icon={<Sparkles size={11} />}
                      label="Shared"
                      sx={{
                        height: 18,
                        fontSize: "0.65rem",
                        bgcolor: "rgba(14, 165, 233, 0.15)",
                        color: "#38BDF8",
                        border: "1px solid rgba(14, 165, 233, 0.3)",
                      }}
                    />
                  )}
                  <Typography variant="caption" sx={{ color: "#9CA3AF", fontSize: "0.72rem", whiteSpace: "nowrap" }}>
                    {lineCount} lines • {formatFileSize(charCount)}
                  </Typography>
                </Box>

                <Tooltip title={copied ? "Copied to clipboard!" : "Copy code"}>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={copied ? <Check size={14} color="#4ADE80" /> : <Copy size={14} />}
                    onClick={handleCopy}
                    sx={{
                      height: 26,
                      fontSize: "0.75rem",
                      textTransform: "none",
                      color: copied ? "#4ADE80" : "#D1D5DB",
                      borderColor: copied ? "#4ADE80" : "#374151",
                      whiteSpace: "nowrap",
                      flexShrink: 0,
                      "&:hover": {
                        borderColor: copied ? "#4ADE80" : "#9CA3AF",
                        bgcolor: "rgba(255, 255, 255, 0.05)",
                      },
                    }}
                  >
                    {copied ? "Copied" : "Copy"}
                  </Button>
                </Tooltip>
              </Box>

              {/* Code Area with Line Numbers */}
              <Box
                sx={{
                  flex: 1,
                  overflow: "auto",
                  display: "flex",
                  p: { xs: 1.5, sm: 2 },
                  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace",
                  fontSize: { xs: "0.75rem", sm: "0.82rem" },
                  lineHeight: "1.55",
                }}
              >
                {/* Line Numbers Gutter */}
                <Box
                  sx={{
                    userSelect: "none",
                    pr: { xs: 1.25, sm: 2 },
                    mr: { xs: 1.25, sm: 2 },
                    borderRight: "1px solid #1E293B",
                    color: "#475569",
                    textAlign: "right",
                    minWidth: { xs: "2rem", sm: "2.5rem" },
                    flexShrink: 0,
                  }}
                >
                  {lines.map((_, idx) => (
                    <div key={idx}>{idx + 1}</div>
                  ))}
                </Box>

                {/* Actual Code */}
                <Box
                  component="pre"
                  sx={{
                    m: 0,
                    p: 0,
                    color: "#E2E8F0",
                    whiteSpace: "pre",
                    overflowX: "visible",
                    flex: 1,
                  }}
                >
                  {activeFile.content}
                </Box>
              </Box>
            </>
          ) : (
            <Box sx={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center" }}>
              <Typography variant="body2" sx={{ color: "#64748B" }}>
                Select a file from the sidebar
              </Typography>
            </Box>
          )}
        </Box>
      </DialogContent>

      {/* Dialog Footer */}
      <DialogActions sx={{ px: { xs: 2, sm: 2.5 }, py: 1.25, borderTop: "1px solid #E2E8F0", bgcolor: "background.paper" }}>
        <Button onClick={onClose} variant="outlined" sx={{ color: "text.secondary", borderColor: "divider" }}>
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};
