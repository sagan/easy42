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
  Switch,
  ToggleButtonGroup,
  ToggleButton,
  Divider,
  Paper,
  CircularProgress,
  Tooltip,
  TextField,
  useTheme,
  useMediaQuery,
} from "@mui/material";
import { X, Download, ExternalLink, Image as ImageIcon, FileCode, CheckCircle2, Activity } from "lucide-react";
import { Node, Link, NetworkPolicy, GraphBlock } from "../../types/api";
import {
  NodeNameSource,
  NodeIpSource,
  ExportFormat,
  ExportAction,
  DiagramExportOptions,
  generateTopologySvg,
  exportDiagramFile,
} from "../../utils/diagramExportEngine";
import { CentralityMetric, WeightingMode } from "../../utils/graphCentrality";
import { useMesh } from "../../context/MeshContext";

export interface ExportDiagramModalProps {
  open: boolean;
  onClose: () => void;
  nodes: Node[];
  links: Link[];
  blocks?: GraphBlock[];
  nodeBlockMap?: Record<string, string>;
  networkPolicies?: NetworkPolicy[];
}

export const ExportDiagramModal: React.FC<ExportDiagramModalProps> = ({
  open,
  onClose,
  nodes,
  links,
  blocks = [],
  nodeBlockMap = {},
  networkPolicies,
}) => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down("md"));

  const [nodeNameSource, setNodeNameSource] = useState<NodeNameSource>("name");
  const [nodeIpSource, setNodeIpSource] = useState<NodeIpSource>("main");
  const [displayLinkCost, setDisplayLinkCost] = useState<boolean>(true);
  const [displayBlocks, setDisplayBlocks] = useState<boolean>(true);
  const [displayGroupName, setDisplayGroupName] = useState<boolean>(true);
  const [displayExternalNodes, setDisplayExternalNodes] = useState<boolean>(true);
  const { networkName } = useMesh();
  const defaultDiagramTitle = `${networkName || "Easy42"} Topology`;
  const [diagramTitle, setDiagramTitle] = useState<string>(defaultDiagramTitle);

  useEffect(() => {
    if (open) {
      setDiagramTitle(defaultDiagramTitle);
    }
  }, [open, defaultDiagramTitle]);

  const [format, setFormat] = useState<ExportFormat>("svg");
  const [emphasizeCentrality, setEmphasizeCentrality] = useState<boolean>(true);
  const [centralityMetric, setCentralityMetric] = useState<CentralityMetric>("betweenness");
  const [centralityWeighting, setCentralityWeighting] = useState<WeightingMode>("weighted");
  const [isExporting, setIsExporting] = useState<boolean>(false);
  const [exportSuccess, setExportSuccess] = useState<boolean>(false);

  const options: DiagramExportOptions = useMemo(
    () => ({
      nodeNameSource,
      nodeIpSource,
      displayLinkCost,
      displayBlocks,
      displayGroupName,
      displayExternalNodes,
      diagramTitle,
      format,
      emphasizeCentrality,
      centralityMetric,
      centralityWeighting,
      networkName: networkName || "Easy42",
    }),
    [
      nodeNameSource,
      nodeIpSource,
      displayLinkCost,
      displayBlocks,
      displayGroupName,
      displayExternalNodes,
      diagramTitle,
      format,
      emphasizeCentrality,
      centralityMetric,
      centralityWeighting,
      networkName,
    ],
  );

  // Real-time preview SVG
  const previewSvg = useMemo(() => {
    if (!open) return "";
    return generateTopologySvg(nodes, links, blocks, nodeBlockMap, networkPolicies, options);
  }, [open, nodes, links, blocks, nodeBlockMap, networkPolicies, options]);

  const handleExport = async (targetAction: ExportAction) => {
    try {
      setIsExporting(true);
      await exportDiagramFile(nodes, links, blocks, nodeBlockMap, networkPolicies, options, targetAction);
      setExportSuccess(true);
      setTimeout(() => setExportSuccess(false), 3000);
    } catch (err) {
      console.error("Failed to export diagram:", err);
    } finally {
      setIsExporting(false);
    }
  };

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
              Export Diagram
            </Typography>
            <Typography variant="caption" sx={{ fontSize: "0.75rem", color: "#64748B", display: "block" }}>
              Export easy42 cluster network topology to standalone SVG or PNG format
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
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.25 }}>Node Name</Typography>
            <Typography variant="caption" sx={{ fontSize: "0.71rem", color: "#64748B", display: "block", mb: 0.75 }}>
              Displayed node name label source
            </Typography>
            <FormControl component="fieldset" size="small">
              <RadioGroup value={nodeNameSource} onChange={(e) => setNodeNameSource(e.target.value as NodeNameSource)}>
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
                        Node &quot;caption&quot; field (or *** if it's empty or starts with !)
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 0.5 }}
                />
                <FormControlLabel
                  value="none"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>None</Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Display node name as &quot;***&quot;
                      </Typography>
                    </Box>
                  }
                />
              </RadioGroup>
            </FormControl>
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Node IP */}
          <Box>
            <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B", mb: 0.25 }}>Node IP</Typography>
            <Typography variant="caption" sx={{ fontSize: "0.71rem", color: "#64748B", display: "block", mb: 0.75 }}>
              Displayed node IP source (IPv4 preferred over IPv6)
            </Typography>
            <FormControl component="fieldset" size="small">
              <RadioGroup value={nodeIpSource} onChange={(e) => setNodeIpSource(e.target.value as NodeIpSource)}>
                <FormControlLabel
                  value="main"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        Main IP{" "}
                        <span style={{ color: "#4F46E5", fontSize: "0.72rem", fontWeight: 700 }}>(Default)</span>
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Main IP or Main IPv6
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 0.5 }}
                />
                <FormControlLabel
                  value="external"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>
                        External IP
                      </Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        External IP or External IPv6
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 0.5 }}
                />
                <FormControlLabel
                  value="none"
                  control={<Radio size="small" sx={{ color: "#4F46E5", "&.Mui-checked": { color: "#4F46E5" } }} />}
                  label={
                    <Box>
                      <Typography sx={{ fontSize: "0.82rem", fontWeight: 600, color: "#1E293B" }}>None</Typography>
                      <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B" }}>
                        Display node IP as &quot;***&quot;
                      </Typography>
                    </Box>
                  }
                />
              </RadioGroup>
            </FormControl>
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Link Cost */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B" }}>Link Cost</Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                1 number if symmetric, 2 if asymmetric
              </Typography>
            </Box>
            <Switch
              checked={displayLinkCost}
              onChange={(e) => setDisplayLinkCost(e.target.checked)}
              color="primary"
              size="small"
              sx={{
                "& .MuiSwitch-switchBase.Mui-checked": {
                  color: "#4F46E5",
                  "& + .MuiSwitch-track": {
                    backgroundColor: "#4F46E5",
                  },
                },
              }}
            />
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Display Blocks (Groups) */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B" }}>
                Display Groups (Blocks)
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                Show group rectangles &amp; labels (default: on)
              </Typography>
            </Box>
            <Switch
              checked={displayBlocks}
              onChange={(e) => setDisplayBlocks(e.target.checked)}
              color="primary"
              size="small"
              sx={{
                "& .MuiSwitch-switchBase.Mui-checked": {
                  color: "#4F46E5",
                  "& + .MuiSwitch-track": {
                    backgroundColor: "#4F46E5",
                  },
                },
              }}
            />
          </Box>

          {/* Option: Display Group Name (sub-option under Display Groups) */}
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              pl: 1.5,
              opacity: displayBlocks ? 1 : 0.45,
              transition: "opacity 0.2s ease",
            }}
          >
            <Box>
              <Typography
                sx={{
                  fontSize: "0.80rem",
                  fontWeight: 600,
                  color: displayBlocks ? "#334155" : "#94A3B8",
                }}
              >
                Display group name (unless it starts with !)
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.68rem", color: "#64748B", display: "block" }}>
                Replace with &quot;***&quot; if toggled off
              </Typography>
            </Box>
            <Switch
              checked={displayGroupName}
              disabled={!displayBlocks}
              onChange={(e) => setDisplayGroupName(e.target.checked)}
              color="primary"
              size="small"
              sx={{
                "& .MuiSwitch-switchBase.Mui-checked": {
                  color: "#4F46E5",
                  "& + .MuiSwitch-track": {
                    backgroundColor: "#4F46E5",
                  },
                },
              }}
            />
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Display External Nodes */}
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B" }}>
                Display external nodes
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                Show external peering nodes &amp; links (default: on)
              </Typography>
            </Box>
            <Switch
              checked={displayExternalNodes}
              onChange={(e) => setDisplayExternalNodes(e.target.checked)}
              color="primary"
              size="small"
              sx={{
                "& .MuiSwitch-switchBase.Mui-checked": {
                  color: "#4F46E5",
                  "& + .MuiSwitch-track": {
                    backgroundColor: "#4F46E5",
                  },
                },
              }}
            />
          </Box>

          <Divider sx={{ borderColor: "#F1F5F9" }} />

          {/* Option: Graph Centrality & Importance Emphasis */}
          <Box
            sx={{
              p: 1.5,
              borderRadius: 2,
              bgcolor: emphasizeCentrality ? "#F8FAFC" : "transparent",
              border: emphasizeCentrality ? "1px solid #E2E8F0" : "1px dashed #E2E8F0",
              transition: "all 0.2s ease",
            }}
          >
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                mb: emphasizeCentrality ? 1.5 : 0,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Box
                  sx={{
                    width: 26,
                    height: 26,
                    borderRadius: 1,
                    bgcolor: emphasizeCentrality ? "#ECFDF5" : "#F1F5F9",
                    color: emphasizeCentrality ? "#059669" : "#64748B",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <Activity size={14} />
                </Box>
                <Box>
                  <Typography sx={{ fontSize: "0.82rem", fontWeight: 700, color: "#1E293B" }}>
                    Emphasize Core Elements
                  </Typography>
                  <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                    Highlight core nodes &amp; backbone links
                  </Typography>
                </Box>
              </Box>
              <Switch
                checked={emphasizeCentrality}
                onChange={(e) => setEmphasizeCentrality(e.target.checked)}
                color="primary"
                size="small"
                sx={{
                  "& .MuiSwitch-switchBase.Mui-checked": {
                    color: "#059669",
                    "& + .MuiSwitch-track": {
                      backgroundColor: "#059669",
                    },
                  },
                }}
              />
            </Box>

            {emphasizeCentrality && (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25, pt: 0.5 }}>
                {/* Metric Selector */}
                <Box>
                  <Typography
                    sx={{
                      fontSize: "0.72rem",
                      fontWeight: 700,
                      color: "#475569",
                      mb: 0.5,
                      textTransform: "uppercase",
                      letterSpacing: "0.03em",
                    }}
                  >
                    Centrality Metric
                  </Typography>
                  <ToggleButtonGroup
                    value={centralityMetric}
                    exclusive
                    onChange={(_, val) => val && setCentralityMetric(val as CentralityMetric)}
                    fullWidth
                    size="small"
                    sx={{
                      "& .MuiToggleButton-root": {
                        py: 0.5,
                        px: 1,
                        fontSize: "0.75rem",
                        fontWeight: 600,
                        textTransform: "none",
                        border: "1px solid #CBD5E1",
                        "&.Mui-selected": {
                          backgroundColor: "#ECFDF5",
                          color: "#047857",
                          borderColor: "#10B981",
                          "&:hover": {
                            backgroundColor: "#D1FAE5",
                          },
                        },
                      },
                    }}
                  >
                    <ToggleButton value="betweenness">Betweenness (Transit)</ToggleButton>
                    <ToggleButton value="degree">Degree (Hub)</ToggleButton>
                  </ToggleButtonGroup>
                </Box>

                {/* Weighting Selector */}
                <Box>
                  <Typography
                    sx={{
                      fontSize: "0.72rem",
                      fontWeight: 700,
                      color: "#475569",
                      mb: 0.5,
                      textTransform: "uppercase",
                      letterSpacing: "0.03em",
                    }}
                  >
                    Shortest Path Weighting
                  </Typography>
                  <ToggleButtonGroup
                    value={centralityWeighting}
                    exclusive
                    onChange={(_, val) => val && setCentralityWeighting(val as WeightingMode)}
                    fullWidth
                    size="small"
                    sx={{
                      "& .MuiToggleButton-root": {
                        py: 0.5,
                        px: 1,
                        fontSize: "0.75rem",
                        fontWeight: 600,
                        textTransform: "none",
                        border: "1px solid #CBD5E1",
                        "&.Mui-selected": {
                          backgroundColor: "#ECFDF5",
                          color: "#047857",
                          borderColor: "#10B981",
                          "&:hover": {
                            backgroundColor: "#D1FAE5",
                          },
                        },
                      },
                    }}
                  >
                    <ToggleButton value="weighted">Cost Weighted (Routing)</ToggleButton>
                    <ToggleButton value="unweighted">Hop Count (Physical)</ToggleButton>
                  </ToggleButtonGroup>
                </Box>
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
              onChange={(_, val) => val && setFormat(val as ExportFormat)}
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

          {/* Direct Quick Action Buttons inside side-panel */}
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1, mt: 1 }}>
            <Button
              variant="contained"
              onClick={() => handleExport("download")}
              disabled={isExporting || nodes.length === 0}
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
              disabled={isExporting || nodes.length === 0}
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
          </Box>
        </Box>

        {/* Right Column: Interactive Live Preview */}
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
                Diagram Live Preview
              </Typography>
              <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.72rem" }}>
                (Deterministic Standalone Engine)
              </Typography>
            </Box>
            <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#94A3B8" }}>
              {nodes.length} nodes · {links.length} links
              {blocks.length > 0 && displayBlocks ? ` · ${blocks.length} groups` : ""}
            </Typography>
          </Box>

          <Paper
            elevation={0}
            sx={{
              flex: "1 1 auto",
              width: "100%",
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
                No nodes or links to preview
              </Typography>
            )}
          </Paper>
        </Box>
      </DialogContent>

      {/* Dialog Actions Bar - Fixed at bottom with generous spacing and complete visibility */}
      <DialogActions
        sx={{
          px: 3,
          py: 2,
          pb: 2.75,
          borderTop: "1px solid #E2E8F0",
          bgcolor: "#F8FAFC",
          flexShrink: 0,
          justifyContent: "space-between",
        }}
      >
        <Button onClick={onClose} variant="text" sx={{ color: "#64748B", textTransform: "none", fontWeight: 600 }}>
          Cancel
        </Button>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          {exportSuccess && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, color: "#10B981" }}>
              <CheckCircle2 size={16} />
              <Typography variant="caption" sx={{ fontWeight: 700, fontSize: "0.78rem" }}>
                Exported Successfully
              </Typography>
            </Box>
          )}

          <Tooltip title="Directly open diagram in browser new tab">
            <Button
              variant="outlined"
              size="small"
              onClick={() => handleExport("new_tab")}
              disabled={isExporting || nodes.length === 0}
              startIcon={<ExternalLink size={15} />}
              sx={{
                textTransform: "none",
                fontWeight: 600,
                fontSize: "0.82rem",
                borderRadius: 1.75,
                borderColor: "#CBD5E1",
                color: "#475569",
                px: 2,
                "&:hover": {
                  borderColor: "#94A3B8",
                  bgcolor: "#F1F5F9",
                },
              }}
            >
              Open in Tab
            </Button>
          </Tooltip>

          <Button
            variant="contained"
            size="small"
            onClick={() => handleExport("download")}
            disabled={isExporting || nodes.length === 0}
            startIcon={isExporting ? <CircularProgress size={15} color="inherit" /> : <Download size={15} />}
            sx={{
              textTransform: "none",
              fontWeight: 700,
              fontSize: "0.82rem",
              borderRadius: 1.75,
              px: 2.5,
              py: 0.75,
              bgcolor: "#4F46E5",
              boxShadow: "0 4px 12px rgba(79, 70, 229, 0.25)",
              "&:hover": {
                bgcolor: "#4338CA",
              },
            }}
          >
            {isExporting ? "Exporting..." : `Download ${format.toUpperCase()}`}
          </Button>
        </Box>
      </DialogActions>
    </Dialog>
  );
};
