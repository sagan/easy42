import React, { useState, useMemo, useEffect } from "react";
import DOMPurify from "dompurify";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Box,
  Typography,
  Button,
  IconButton,
  FormControl,
  FormControlLabel,
  RadioGroup,
  Radio,
  Select,
  MenuItem,
  ToggleButtonGroup,
  ToggleButton,
  Divider,
  Paper,
  CircularProgress,
  TextField,
  useTheme,
  useMediaQuery,
} from "@mui/material";
import { X, Download, ExternalLink, Image as ImageIcon, FileCode, CheckCircle2, Tag } from "lucide-react";
import { Node, PingTableTask } from "../../types/api";
import {
  PingTableNodeNameSource,
  PingTableNodeFilter,
  PingTableExportFormat,
  PingTableExportAction,
  PingTableExportOptions,
  generatePingTableSvg,
  exportPingTableFile,
} from "../../utils/pingTableExport";
import { useMesh } from "../../context/MeshContext";

export interface ExportPingTableModalProps {
  open: boolean;
  onClose: () => void;
  task: PingTableTask | null;
  nodes: Node[];
}

export const ExportPingTableModal: React.FC<ExportPingTableModalProps> = ({
  open,
  onClose,
  task,
  nodes,
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));
  const { networkName } = useMesh();

  // State
  const typeTitle = task?.latency_type === "link" ? "Direct Link Latency" : "End-to-End Latency";
  const defaultDiagramTitle = `${networkName || "Easy42"} Ping Table — ${typeTitle}`;
  const [diagramTitle, setDiagramTitle] = useState<string>(defaultDiagramTitle);

  useEffect(() => {
    if (open) {
      setDiagramTitle(defaultDiagramTitle);
    }
  }, [open, defaultDiagramTitle]);

  const [nodeNameSource, setNodeNameSource] = useState<PingTableNodeNameSource>("name");
  const [nodeFilter, setNodeFilter] = useState<PingTableNodeFilter>("all");
  const [format, setFormat] = useState<PingTableExportFormat>("svg");
  const [isExporting, setIsExporting] = useState<boolean>(false);
  const [exportSuccess, setExportSuccess] = useState<boolean>(false);

  // Collect unique tags from cluster nodes
  const uniqueTags = useMemo(() => {
    const set = new Set<string>();
    nodes.forEach((n) => {
      n.tags?.forEach((t) => {
        const trimmed = t.trim();
        if (trimmed) set.add(trimmed);
      });
    });
    return Array.from(set).sort();
  }, [nodes]);

  const [filterTag, setFilterTag] = useState<string>("");

  useEffect(() => {
    if (uniqueTags.length > 0 && !filterTag) {
      setFilterTag(uniqueTags[0]);
    }
  }, [uniqueTags, filterTag]);

  // Node Map for lookups
  const nodeMap = useMemo(() => {
    return new Map<string, Node>(nodes.map((n) => [n.name, n]));
  }, [nodes]);

  // External IP count for option badge
  const externalNodesCount = useMemo(() => {
    const taskNodes = task?.nodes || [];
    return taskNodes.filter((name) => {
      const n = nodeMap.get(name);
      return Boolean(n?.external_ip?.trim() || n?.external_ip6?.trim());
    }).length;
  }, [task?.nodes, nodeMap]);

  // Options object
  const options: PingTableExportOptions = useMemo(
    () => ({
      diagramTitle,
      nodeNameSource,
      nodeFilter,
      filterTag,
      networkName,
      nodes,
    }),
    [diagramTitle, nodeNameSource, nodeFilter, filterTag, networkName, nodes],
  );

  // Live Preview SVG
  const previewSvg = useMemo(() => {
    if (!open || !task) return "";
    return generatePingTableSvg(task, options);
  }, [open, task, options]);

  // Handle Export Action
  const handleExport = async (action: PingTableExportAction) => {
    if (!task) return;
    try {
      setIsExporting(true);
      await exportPingTableFile(task, format, options, action);
      setExportSuccess(true);
      setTimeout(() => setExportSuccess(false), 3000);
    } catch (err) {
      console.error("Failed to export ping table diagram:", err);
    } finally {
      setIsExporting(false);
    }
  };

  const totalTaskNodes = task?.nodes?.length || 0;

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      fullScreen={isMobile}
      PaperProps={{
        sx: {
          borderRadius: { xs: 0, md: 3 },
          boxShadow: "0 25px 50px -12px rgba(0, 0, 0, 0.25)",
          overflow: "hidden",
          maxHeight: { xs: "100%", md: "calc(100vh - 64px)" },
          height: { xs: "100%", md: 680 },
          display: "flex",
          flexDirection: "column",
          my: { xs: 0, sm: 3 },
        },
      }}
    >
      {/* Dialog Header */}
      <DialogTitle
        sx={{
          px: 3,
          py: 2,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          borderBottom: "1px solid #E2E8F0",
          bgcolor: "#F8FAFC",
          flexShrink: 0,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              bgcolor: "#EEF2FF",
              color: "#4F46E5",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Download size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontSize: "1.05rem", fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
              Export Ping Table Diagram
            </Typography>
            <Typography variant="caption" sx={{ fontSize: "0.75rem", color: "#64748B", display: "block" }}>
              Export cluster latency matrix to standalone SVG or PNG format
            </Typography>
          </Box>
        </Box>
        <IconButton size="small" onClick={onClose} sx={{ color: "#64748B" }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      {/* Dialog Content */}
      <DialogContent
        sx={{
          p: 0,
          display: "flex",
          flexDirection: { xs: "column", md: "row" },
          flex: "1 1 auto",
          minHeight: 0,
          overflowY: { xs: "auto", md: "hidden" },
          overflowX: "hidden",
        }}
      >
        {/* Left Column: Configuration Options */}
        <Box
          sx={{
            width: { xs: "100%", md: 360 },
            borderRight: { xs: "none", md: "1px solid #E2E8F0" },
            borderBottom: { xs: "1px solid #E2E8F0", md: "none" },
            p: { xs: 2.25, sm: 3 },
            display: "flex",
            flexDirection: "column",
            gap: 2.25,
            overflowY: { xs: "visible", md: "auto" },
            flexShrink: 0,
            bgcolor: "#FFFFFF",
          }}
        >
          {/* Option: Diagram Title */}
          <Box>
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.25 }}>
              Diagram Title
            </Typography>
            <Typography variant="caption" sx={{ fontSize: "0.71rem", color: "#64748B", display: "block", mb: 0.75 }}>
              Custom title displayed on the exported diagram
            </Typography>
            <TextField
              size="small"
              fullWidth
              value={diagramTitle}
              onChange={(e) => setDiagramTitle(e.target.value)}
              placeholder={defaultDiagramTitle}
              slotProps={{
                input: {
                  sx: {
                    fontSize: "0.82rem",
                    bgcolor: "#F8FAFC",
                    borderRadius: 1.5,
                    "& .MuiOutlinedInput-input": { py: 0.8, px: 1.2 },
                  },
                },
              }}
            />
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Node Name */}
          <Box>
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.25 }}>
              Node Name
            </Typography>
            <Typography variant="caption" sx={{ fontSize: "0.71rem", color: "#64748B", display: "block", mb: 0.75 }}>
              Displayed node name label source
            </Typography>
            <FormControl component="fieldset" size="small">
              <RadioGroup
                value={nodeNameSource}
                onChange={(e) => setNodeNameSource(e.target.value as PingTableNodeNameSource)}
              >
                <FormControlLabel
                  value="name"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        Node name{" "}
                        <span style={{ color: "#4F46E5", fontSize: "0.72rem", fontWeight: 700 }}>(Default)</span>
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Easy42 node name
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 0.5 }}
                />
                <FormControlLabel
                  value="caption"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        Node caption
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Node &quot;caption&quot; field (or *** if empty or starts with !)
                      </Typography>
                    </Box>
                  }
                />
              </RadioGroup>
            </FormControl>
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Included Nodes */}
          <Box>
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.25 }}>
              Included Nodes
            </Typography>
            <Typography variant="caption" sx={{ fontSize: "0.71rem", color: "#64748B", display: "block", mb: 0.75 }}>
              Decide which nodes are included in the exported chart
            </Typography>
            <FormControl component="fieldset" size="small" fullWidth>
              <RadioGroup
                value={nodeFilter}
                onChange={(e) => setNodeFilter(e.target.value as PingTableNodeFilter)}
              >
                <FormControlLabel
                  value="all"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                      <Box>
                        <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                          All nodes{" "}
                          <span style={{ color: "#4F46E5", fontSize: "0.72rem", fontWeight: 700 }}>(Default)</span>
                        </Typography>
                        <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                          All {totalTaskNodes} cluster nodes in measurement
                        </Typography>
                      </Box>
                    </Box>
                  }
                  sx={{ mb: 0.75 }}
                />

                <FormControlLabel
                  value="external"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        Nodes with external IP
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Nodes with configured external IPv4 or IPv6 ({externalNodesCount} nodes)
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 0.75 }}
                />

                <FormControlLabel
                  value="tag"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        Nodes of certain tag
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Filter by specific cluster tag
                      </Typography>
                    </Box>
                  }
                />
              </RadioGroup>
            </FormControl>

            {/* Tag Select Dropdown (shown when tag filter is selected) */}
            {nodeFilter === "tag" && (
              <Box sx={{ mt: 1.25, pl: 3.5 }}>
                {uniqueTags.length > 0 ? (
                  <FormControl fullWidth size="small">
                    <Select
                      value={filterTag || uniqueTags[0] || ""}
                      onChange={(e) => setFilterTag(e.target.value)}
                      sx={{
                        fontSize: "0.82rem",
                        bgcolor: "#F8FAFC",
                        borderRadius: 1.5,
                        "& .MuiSelect-select": { py: 0.8, px: 1.2 },
                      }}
                    >
                      {uniqueTags.map((t) => {
                        const count = (task?.nodes || []).filter((name) => {
                          const n = nodeMap.get(name);
                          return Boolean(n?.tags && n.tags.includes(t));
                        }).length;
                        return (
                          <MenuItem key={t} value={t} sx={{ fontSize: "0.82rem" }}>
                            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                              <Tag size={14} color="#6366F1" />
                              <span>{t}</span>
                              <Typography variant="caption" sx={{ color: "#64748B", ml: 0.5 }}>
                                ({count} {count === 1 ? "node" : "nodes"})
                              </Typography>
                            </Box>
                          </MenuItem>
                        );
                      })}
                    </Select>
                  </FormControl>
                ) : (
                  <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                    No tags configured on cluster nodes
                  </Typography>
                )}
              </Box>
            )}
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Export Format */}
          <Box>
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.75 }}>
              Export Format
            </Typography>
            <ToggleButtonGroup
              value={format}
              exclusive
              onChange={(_, val) => val && setFormat(val as PingTableExportFormat)}
              fullWidth
              size="small"
              sx={{
                "& .MuiToggleButton-root": {
                  py: 0.75,
                  fontSize: "0.8rem",
                  fontWeight: 600,
                  textTransform: "none",
                  border: "1px solid #CBD5E1",
                  "&.Mui-selected": {
                    backgroundColor: "#EEF2FF",
                    color: "#4F46E5",
                    borderColor: "#6366F1",
                    "&:hover": {
                      backgroundColor: "#E0E7FF",
                    },
                  },
                },
              }}
            >
              <ToggleButton value="svg" aria-label="SVG">
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <FileCode size={15} />
                  <span>SVG Vector</span>
                </Box>
              </ToggleButton>
              <ToggleButton value="png" aria-label="PNG">
                <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <ImageIcon size={15} />
                  <span>PNG Image</span>
                </Box>
              </ToggleButton>
            </ToggleButtonGroup>
          </Box>

          {/* Action Buttons */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1, mt: 1 }}>
            <Button
              variant="contained"
              onClick={() => handleExport("download")}
              disabled={isExporting || !task}
              startIcon={isExporting ? <CircularProgress size={16} color="inherit" /> : <Download size={16} />}
              sx={{
                textTransform: "none",
                fontWeight: 700,
                fontSize: "0.82rem",
                borderRadius: 1.75,
                py: 0.9,
                bgcolor: "#4F46E5",
                boxShadow: "0 4px 12px rgba(79, 70, 229, 0.25)",
                "&:hover": {
                  bgcolor: "#4338CA",
                },
              }}
            >
              {isExporting ? "Exporting..." : `Download ${format.toUpperCase()} File`}
            </Button>

            <Button
              variant="outlined"
              onClick={() => handleExport("new_tab")}
              disabled={isExporting || !task}
              startIcon={<ExternalLink size={15} />}
              sx={{
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.82rem",
                borderRadius: 1.75,
                py: 0.75,
                borderColor: "#CBD5E1",
                color: "#475569",
                "&:hover": {
                  borderColor: "#94A3B8",
                  bgcolor: "#F8FAFC",
                },
              }}
            >
              Open in New Tab
            </Button>

            {exportSuccess && (
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, justifyContent: "center", mt: 0.5 }}>
                <CheckCircle2 size={15} color="#10B981" />
                <Typography sx={{ fontSize: "0.75rem", color: "#10B981", fontWeight: 600 }}>
                  Exported successfully!
                </Typography>
              </Box>
            )}
          </Box>
        </Box>

        {/* Right Column: Live Preview */}
        <Box
          sx={{
            flex: "1 1 auto",
            bgcolor: "#F8FAFC",
            p: { xs: 2, sm: 3 },
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
            minHeight: { xs: 340, md: 0 },
            height: { xs: "auto", md: "100%" },
            overflow: "hidden",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexShrink: 0 }}>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#334155" }}>
                Ping Table Live Preview
              </Typography>
              <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.72rem" }}>
                (Deterministic Standalone Engine)
              </Typography>
            </Box>
            <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#94A3B8" }}>
              {task?.nodes?.length || 0} nodes measured
            </Typography>
          </Box>

          <Paper
            elevation={0}
            sx={{
              flex: "1 1 auto",
              height: { xs: 320, md: "100%" },
              minHeight: { xs: 280, md: 0 },
              border: "1px solid #E2E8F0",
              borderRadius: 2,
              bgcolor: "#FFFFFF",
              overflow: "auto",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              position: "relative",
              p: 2,
            }}
          >
            {previewSvg ? (
              <Box
                component="div"
                dangerouslySetInnerHTML={{
                  __html: DOMPurify.sanitize(previewSvg, {
                    USE_PROFILES: { svg: true, svgFilters: true },
                  }),
                }}
                sx={{
                  maxWidth: "100%",
                  maxHeight: "100%",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  "& svg": {
                    maxWidth: "100%",
                    height: "auto",
                    borderRadius: 1.5,
                    boxShadow: "0 4px 14px rgba(0, 0, 0, 0.06)",
                  },
                }}
              />
            ) : (
              <Typography variant="caption" sx={{ color: "#94A3B8" }}>
                No ping table measurement data to preview
              </Typography>
            )}
          </Paper>
        </Box>
      </DialogContent>

      {/* Dialog Actions Bar */}
      <DialogActions
        sx={{
          px: 3,
          py: 1.5,
          borderTop: "1px solid #E2E8F0",
          bgcolor: "#F8FAFC",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
          Format: <strong>{format.toUpperCase()}</strong> | Type:{" "}
          <strong>{task?.latency_type === "link" ? "Direct Link" : "End-to-End"}</strong>
        </Typography>
        <Button onClick={onClose} size="small" sx={{ textTransform: "none", color: "#64748B" }}>
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};
