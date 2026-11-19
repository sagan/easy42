import React from "react";
import {
  Box,
  Button,
  Chip,
  FormControl,
  FormControlLabel,
  Grid,
  InputLabel,
  MenuItem,
  Radio,
  RadioGroup,
  Select,
  TextField,
  Typography,
} from "@mui/material";
import { Code2, Globe, Info, Sliders, Tag } from "lucide-react";
import type { Node } from "../../types/api";
import {
  detectTargetMode,
  getSelectorValue,
  matchesTaskSelector,
  parseSelectorJSON,
  splitSelectorList,
  type SelectorTargetMode,
} from "../../utils/taskSelector";

/**
 * Special selector value meaning "every managed node" for forms that must tell
 * "all nodes" apart from "nothing selected". Kept in sync with the server's
 * config.AllNodesSelectorKey. Regular Consumers (workload apps) treat an empty
 * selector as "all", so they never need this marker.
 */
export const ALL_NODES_SELECTOR: Record<string, string> = { all: "true" };

export function isAllNodesSelector(selector: Record<string, string> | null | undefined): boolean {
  return !!selector && Object.keys(selector).some((k) => k.trim().toLowerCase() === "all");
}

export interface TargetNodesSelectorProps {
  /** All cluster nodes; external nodes are filtered out automatically. */
  nodes: Node[];
  /** Current selector object. */
  selector: Record<string, string>;
  /** Raw JSON currently shown in the textarea (may be mid-edit / invalid). */
  selectorText: string;
  /** Parse error of `selectorText`, if any. */
  selectorError?: string | null;
  /** Called with the new selector when the user picks a preset. */
  onChange: (selector: Record<string, string>) => void;
  /** Called when the user types into the selector JSON textarea. */
  onSelectorTextChange: (text: string) => void;
  /**
   * When true, the "All Nodes" option writes the explicit ALL_NODES_SELECTOR
   * marker and an empty selector is NOT treated as "all nodes". Used by
   * features where an empty selection must be distinguishable (SSH key rules).
   */
  explicitAllNodes?: boolean;
  /** Heading of the section. */
  title?: string;
  /** Introductory help text. */
  description?: string;
  /** Description shown in the "All nodes" preset. */
  allNodesDescription?: string;
  /** Optional extra validation error to show below the picker. */
  validationError?: string | null;
}

/**
 * Target node selector form shared by the App editor and the SSH key rule
 * editor: preset modes (manual / all / tag / advanced) plus a live selector
 * JSON editor with match preview.
 */
export const TargetNodesSelector: React.FC<TargetNodesSelectorProps> = ({
  nodes,
  selector,
  selectorText,
  selectorError,
  onChange,
  onSelectorTextChange,
  explicitAllNodes = false,
  title = "Target Nodes",
  description = "Choose nodes with the shortcuts below or edit the selector JSON directly. An empty selector matches every managed node. All selector conditions must match (AND).",
  allNodesDescription,
  validationError,
}) => {
  const managedNodes = React.useMemo(() => nodes.filter((n) => !n.is_external), [nodes]);

  const availableTags = React.useMemo(() => {
    const s = new Set<string>();
    nodes.forEach((n) => {
      n.tags?.forEach((t) => {
        if (t.trim()) s.add(t.trim());
      });
    });
    return Array.from(s).sort();
  }, [nodes]);

  const targetType: SelectorTargetMode = React.useMemo(() => {
    if (explicitAllNodes && isAllNodesSelector(selector)) return "all";
    const mode = detectTargetMode(selector);
    // Empty selectors mean "nothing chosen" in explicit mode.
    return explicitAllNodes && mode === "all" ? "manual" : mode;
  }, [selector, explicitAllNodes]);

  // Match preview: in explicit mode an empty selector matches nothing.
  const matchedNodes =
    explicitAllNodes && Object.keys(selector).length === 0
      ? []
      : managedNodes.filter((n) => matchesTaskSelector(n, explicitAllNodes && isAllNodesSelector(selector) ? {} : selector));

  const selectedNodes = splitSelectorList(getSelectorValue(selector, "name"));
  const targetTag = getSelectorValue(selector, "tag");

  const handleTargetModeChange = (mode: SelectorTargetMode) => {
    if (mode === "manual") {
      onChange({ name: getSelectorValue(selector, "name") });
    } else if (mode === "all") {
      onChange(explicitAllNodes ? ALL_NODES_SELECTOR : {});
    } else if (mode === "tag") {
      onChange({ tag: getSelectorValue(selector, "tag") });
    } else {
      // Advanced mode keeps the current selector; the JSON editor is shown below.
      const parsed = parseSelectorJSON(selectorText);
      onChange(parsed.selector || {});
    }
  };

  const allNodesHelp =
    allNodesDescription ??
    (explicitAllNodes
      ? "Selects every cluster node. Any new agent node added in the future is automatically included."
      : "An empty selector applies to all cluster nodes running in agent mode. Any new agent node added in the future is automatically included.");

  return (
    <Box sx={{ p: 1, bgcolor: "#F8FAFC", borderRadius: "10px", border: "1px solid #E2E8F0" }}>
      <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
        {title}
      </Typography>
      <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1 }}>
        {description}
      </Typography>

      <FormControl component="fieldset" fullWidth sx={{ mb: 1 }}>
        <RadioGroup row value={targetType} onChange={(e) => handleTargetModeChange(e.target.value as SelectorTargetMode)}>
          <FormControlLabel
            value="manual"
            control={<Radio size="small" />}
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                <Sliders size={16} color="#6366F1" />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Manually Select Nodes
                </Typography>
              </Box>
            }
          />
          <FormControlLabel
            value="all"
            control={<Radio size="small" />}
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                <Globe size={16} color="#0EA5E9" />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  All Nodes
                </Typography>
              </Box>
            }
          />
          <FormControlLabel
            value="tag"
            control={<Radio size="small" />}
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                <Tag size={16} color="#8B5CF6" />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Nodes of Selected Tag(s)
                </Typography>
              </Box>
            }
          />
          <FormControlLabel
            value="custom"
            control={<Radio size="small" />}
            label={
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                <Code2 size={16} color="#0F766E" />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Advanced Selector
                </Typography>
              </Box>
            }
          />
        </RadioGroup>
      </FormControl>

      {/* Option 1: Manual */}
      {targetType === "manual" && (
        <Box>
          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1 }}>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              {explicitAllNodes
                ? "Select one or more nodes. Nothing is selected by default."
                : "Select zero or more nodes. Selecting none keeps this app unassigned."}
            </Typography>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Button
                size="small"
                sx={{ textTransform: "none", fontSize: "0.75rem", p: 0 }}
                onClick={() => onChange({ name: managedNodes.map((n) => n.name).join(",") })}
              >
                Select All
              </Button>
              <Button
                size="small"
                sx={{ textTransform: "none", fontSize: "0.75rem", p: 0, color: "#64748B" }}
                onClick={() => onChange({ name: "" })}
              >
                Clear (None)
              </Button>
            </Box>
          </Box>

          <FormControl fullWidth size="small">
            <InputLabel>Selected Nodes ({selectedNodes.length})</InputLabel>
            <Select
              multiple
              value={selectedNodes}
              label={`Selected Nodes (${selectedNodes.length})`}
              onChange={(e) => {
                const val = e.target.value;
                const selected = typeof val === "string" ? val.split(",") : val;
                onChange({ name: selected.join(",") });
              }}
              renderValue={(selected) => (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                  {selected.length === 0 ? (
                    <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                      {explicitAllNodes ? "None selected" : "None selected (unassigned)"}
                    </Typography>
                  ) : (
                    selected.map((val) => (
                      <Chip key={val} label={val} size="small" sx={{ height: 22, fontSize: "0.75rem" }} />
                    ))
                  )}
                </Box>
              )}
            >
              {managedNodes.map((n) => (
                <MenuItem key={n.name} value={n.name}>
                  <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%", alignItems: "center" }}>
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      {n.name}
                    </Typography>
                    {n.tags && n.tags.length > 0 && (
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        {n.tags.map((t) => (
                          <Chip
                            key={t}
                            label={`#${t}`}
                            size="small"
                            sx={{ height: 18, fontSize: "0.65rem", bgcolor: "#EDE9FE", color: "#6D28D9" }}
                          />
                        ))}
                      </Box>
                    )}
                  </Box>
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Typography
            variant="caption"
            sx={{ color: selectedNodes.length === 0 ? "#D97706" : "#64748B", mt: 0.5, display: "block" }}
          >
            {selectedNodes.length === 0
              ? explicitAllNodes
                ? "Notice: No nodes selected. Choose specific nodes, a tag, or the All Nodes option."
                : "Notice: No nodes selected. The selector will match no node until a node name is added."
              : `Assigned to ${selectedNodes.length} node(s).`}
          </Typography>
        </Box>
      )}

      {/* Option 2: All Nodes */}
      {targetType === "all" && (
        <Box sx={{ p: 1, bgcolor: "#E0F2FE", borderRadius: "8px", border: "1px solid #BAE6FD" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1 }}>
            <Globe size={18} color="#0284C7" />
            <Typography variant="body2" sx={{ fontWeight: 700, color: "#0369A1" }}>
              {explicitAllNodes
                ? `All Cluster Nodes (${managedNodes.length})`
                : `Automatic Deployment to All Agent Nodes (${managedNodes.length})`}
            </Typography>
          </Box>
          <Typography variant="caption" sx={{ color: "#0C4A6E", display: "block", mb: 1 }}>
            {allNodesHelp}
          </Typography>
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
            {managedNodes.map((n) => (
              <Chip
                key={n.name}
                label={n.name}
                size="small"
                sx={{
                  bgcolor: "#FFFFFF",
                  color: "#0369A1",
                  border: "1px solid #7DD3FC",
                  fontWeight: 600,
                  fontSize: "0.75rem",
                }}
              />
            ))}
          </Box>
        </Box>
      )}

      {/* Option 3: Nodes of Selected Tag(s) */}
      {targetType === "tag" && (
        <Box>
          <Grid container spacing={2} alignItems="center">
            <Grid item xs={12} sm={6}>
              <FormControl fullWidth size="small">
                <InputLabel>Select from Existing Tags</InputLabel>
                <Select
                  multiple
                  value={splitSelectorList(targetTag)}
                  label="Select from Existing Tags"
                  onChange={(e) => {
                    const val = e.target.value;
                    const selected = typeof val === "string" ? val.split(",") : val;
                    onChange({ tag: selected.join(",") });
                  }}
                  renderValue={(selected) => (
                    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                      {selected.length === 0 ? (
                        <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
                          None selected
                        </Typography>
                      ) : (
                        selected.map((tag) => (
                          <Chip key={tag} label={`#${tag}`} size="small" sx={{ height: 22, fontSize: "0.75rem" }} />
                        ))
                      )}
                    </Box>
                  )}
                >
                  {availableTags.map((tag) => {
                    const matchCount = managedNodes.filter((n) => n.tags?.includes(tag)).length;
                    return (
                      <MenuItem key={tag} value={tag}>
                        <Box sx={{ display: "flex", justifyContent: "space-between", width: "100%", alignItems: "center" }}>
                          <Typography variant="body2" sx={{ fontWeight: 600, color: "#7C3AED" }}>
                            #{tag}
                          </Typography>
                          <Chip label={`${matchCount} node(s)`} size="small" sx={{ height: 18, fontSize: "0.68rem" }} />
                        </Box>
                      </MenuItem>
                    );
                  })}
                </Select>
              </FormControl>
            </Grid>
            <Grid item xs={12} sm={6}>
              <TextField
                fullWidth
                size="small"
                label="Or Enter Tags (comma-separated)"
                placeholder="e.g. edge, ingress, prod"
                value={targetTag}
                onChange={(e) => onChange({ tag: e.target.value.toLowerCase() })}
                InputProps={{
                  startAdornment: <Tag size={16} style={{ marginRight: 6, color: "#8B5CF6" }} />,
                }}
              />
            </Grid>
          </Grid>

          {/* Tag match preview */}
          <Box sx={{ mt: 1, p: 1, bgcolor: "#F5F3FF", borderRadius: "8px", border: "1px solid #DDD6FE" }}>
            {(() => {
              const matchingNodes = managedNodes.filter((n) => matchesTaskSelector(n, { tag: targetTag }));
              if (!targetTag) {
                return (
                  <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                    Please select or enter at least one tag above to see matching nodes.
                  </Typography>
                );
              }
              if (matchingNodes.length === 0) {
                return (
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Info size={16} color="#8B5CF6" />
                    <Typography variant="caption" sx={{ color: "#6D28D9" }}>
                      No nodes currently have any of these tags. This selector will automatically match whenever a node
                      is tagged with <strong>#{targetTag}</strong>.
                    </Typography>
                  </Box>
                );
              }
              return (
                <Box>
                  <Typography variant="caption" sx={{ color: "#5B21B6", fontWeight: 700, display: "block", mb: 0.8 }}>
                    {matchingNodes.length} node(s) currently match tag selection:
                  </Typography>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
                    {matchingNodes.map((n) => (
                      <Chip
                        key={n.name}
                        label={n.name}
                        size="small"
                        sx={{
                          bgcolor: "#FFFFFF",
                          color: "#6D28D9",
                          border: "1px solid #C4B5FD",
                          fontWeight: 600,
                          fontSize: "0.75rem",
                        }}
                      />
                    ))}
                  </Box>
                </Box>
              );
            })()}
          </Box>
        </Box>
      )}

      {targetType === "custom" && (
        <Box sx={{ p: 1, bgcolor: "#F0FDFA", borderRadius: "8px", border: "1px solid #99F6E4" }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Code2 size={16} color="#0F766E" />
            <Typography variant="caption" sx={{ color: "#115E59" }}>
              Build an arbitrary selector below. Supported keys are <code>name</code>, <code>tag</code>, and{" "}
              <code>env:&lt;NAME&gt;</code>.
            </Typography>
          </Box>
        </Box>
      )}

      <Box sx={{ mt: 1 }}>
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 0.8 }}>
          <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700 }}>
            Selector JSON
          </Typography>
          <Typography variant="caption" sx={{ color: matchedNodes.length > 0 ? "#059669" : "#D97706" }}>
            {matchedNodes.length} node(s) match
          </Typography>
        </Box>
        <TextField
          fullWidth
          multiline
          minRows={4}
          size="small"
          value={selectorText}
          onChange={(e) => onSelectorTextChange(e.target.value)}
          error={Boolean(selectorError || validationError)}
          helperText={
            selectorError ||
            validationError ||
            (explicitAllNodes
              ? 'Use {"name":"node-a,node-b"}, {"tag":"edge,prod"}, {"all":"true"} for every node, or {"env:NODE_IP":"10.0.0.1"}.'
              : 'Use {"name":"node-a,node-b"}, {"tag":"edge,prod"}, or {"env:NODE_IP":"10.0.0.1"}. Empty {} matches all nodes.')
          }
          InputProps={{
            sx: { fontFamily: "monospace", fontSize: "0.8rem" },
          }}
        />
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6, mt: 1 }}>
          {matchedNodes.length === 0 ? (
            <Typography variant="caption" sx={{ color: "#94A3B8", fontStyle: "italic" }}>
              No managed nodes match this selector.
            </Typography>
          ) : (
            matchedNodes.map((n) => (
              <Chip
                key={n.name}
                label={n.name}
                size="small"
                sx={{ height: 20, fontSize: "0.7rem", bgcolor: "#ECFDF5", color: "#047857" }}
              />
            ))
          )}
        </Box>
      </Box>
    </Box>
  );
};
