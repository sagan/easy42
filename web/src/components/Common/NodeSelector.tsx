import React from "react";
import {
  Box,
  Button,
  Chip,
  Tab,
  Tabs,
  TextField,
  Typography,
} from "@mui/material";
import {
  AlertTriangle,
  CheckCircle2,
  Globe,
  Search,
  Tag,
  X,
} from "lucide-react";
import type { Node } from "../../types/api";
import {
  ALL_NODES_SELECTOR,
  isAllNodesSelector,
  matchesTaskSelector,
  parseSelectorJSON,
  type SelectorTargetMode,
} from "../../utils/taskSelector";

export { ALL_NODES_SELECTOR, isAllNodesSelector };

export interface NodeSelectorProps {
  /** Label of the selector, e.g. "Target Nodes" or "Source nodes". */
  label?: string;
  /** One-line description shown under the label. */
  description?: string;
  /** All cluster nodes; external nodes are never listed. */
  nodes: Node[];
  /** Current selector shown in the form. */
  value: Record<string, string>;
  onChange: (selector: Record<string, string>) => void;
  /** Optional extra validation message. */
  error?: string | null;
  /** Called with the current validation error (null when the selector is valid). */
  onErrorChange?: (error: string | null) => void;
  /** Optional extra controls rendered on the right of the tab row. */
  action?: React.ReactNode;
  /** Dense layout for dialogs that show several selectors. */
  compact?: boolean;
}

function toggle<T>(list: T[], item: T): T[] {
  return list.includes(item) ? list.filter((x) => x !== item) : [...list, item];
}

function keyMatches(key: string, kind: "name" | "tag"): boolean {
  return key.trim().toLowerCase() === kind;
}

function splitList(value: string, separator = ","): string[] {
  return value
    .split(separator)
    .map((item) => item.trim())
    .filter(Boolean);
}

/** Pretty-prints a selector, keeping "name"/"tag" first for readability. */
export function formatSelectorJSON(selector: Record<string, string>): string {
  const entries = Object.entries(selector || {});
  if (entries.length === 0) return "{}";
  const ordered: Record<string, string> = {};
  for (const [key, value] of entries) {
    if (keyMatches(key, "name") || keyMatches(key, "tag")) ordered[key] = value;
  }
  for (const [key, value] of entries) {
    if (!(key in ordered)) ordered[key] = value;
  }
  return JSON.stringify(ordered, null, 2);
}

/**
 * Resolves the nodes matched by a selector and validates its shape.
 * An empty selector matches no node; {"all": "1"} matches every node.
 * With `strict`, `env:` values must be a single exact value (like the server).
 */
export function resolveSelector(
  nodes: Node[],
  selector: Record<string, string> | null | undefined,
  strict = false,
): { matched: Node[]; error: string | null } {
  const entries = Object.entries(selector || {});
  if (entries.length === 0) return { matched: [], error: null };
  if (isAllNodesSelector(selector))
    return { matched: nodes.slice(), error: null };
  const fail = (error: string) => ({ matched: [] as Node[], error });
  for (const [rawKey, rawValue] of entries) {
    const key = (rawKey || "").trim();
    const lower = key.toLowerCase();
    const values = splitList(rawValue);
    if (!key) return fail("Selector keys cannot be empty.");
    if (lower === "all") {
      return fail(
        'Invalid "all" selector: it must be {"all": "1"} and cannot be combined with other conditions.',
      );
    }
    if (lower === "name" || lower === "tag") {
      if (values.length === 0) {
        return fail(
          `The "${key}" condition matches no node because it has no value. Remove it or enter one or more values.`,
        );
      }
    } else if (lower.startsWith("env:")) {
      if (!key.slice("env:".length).trim()) {
        return fail(
          `Invalid selector key "${rawKey}": environment variable name cannot be empty.`,
        );
      }
      if (strict && values.length > 1) {
        return fail(`"${key}" must be one exact value (found "${rawValue}").`);
      }
    } else {
      return fail(
        `Unsupported selector key "${rawKey}". Use "name", "tag", "env:<NAME>", or {"all": "1"}.`,
      );
    }
  }
  return {
    matched: nodes.filter((n) => matchesTaskSelector(n, selector)),
    error: null,
  };
}

const mono = {
  fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
} as const;

const matchChipSx = {
  height: 22,
  fontSize: "0.7rem",
  bgcolor: "#ECFDF5",
  color: "#047857",
  maxWidth: 260,
} as const;

function MatchList({ nodes }: { nodes: Node[] }) {
  if (nodes.length === 0) {
    return (
      <Typography
        variant="caption"
        sx={{ color: "#94A3B8", fontStyle: "italic" }}
      >
        No managed nodes match this selector.
      </Typography>
    );
  }
  return (
    <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.6 }}>
      {nodes.map((n) => (
        <Chip key={n.name} label={n.name} size="small" sx={matchChipSx} />
      ))}
    </Box>
  );
}

/** Inline reference of the supported selector syntax. */
const SelectorSyntaxHelp: React.FC = () => (
  <Box
    sx={{
      mt: 1,
      p: 1,
      bgcolor: "#F0FDFA",
      borderRadius: "8px",
      border: "1px solid #99F6E4",
    }}
  >
    <Typography
      variant="caption"
      component="div"
      sx={{ color: "#134E4A", lineHeight: 1.7, "& code": mono }}
    >
      A JSON object of conditions; every condition must match (AND), and values
      are compared exactly.
      <br />
      <code>name</code> — comma-separated node names; the node name must be one
      of them.
      <br />
      <code>tag</code> — comma-separated tags; the node must have{" "}
      <em>at least one</em> of them.
      <br />
      <code>env:&lt;NAME&gt;</code> — one exact node value: <code>NODE_IP</code>
      , <code>NODE_IP6</code>, <code>NODE_EXTERNAL_IP</code>,{" "}
      <code>NODE_INTERFACE</code>, <code>NODE_TAGS</code>,{" "}
      <code>NODE_TAG_&lt;TAG&gt;</code> (e.g. <code>env:NODE_TAG_EDGE</code> ={" "}
      <code>"1"</code>), or <code>NODE_DATA_*</code> from the node note.
      <br />
      <code>all</code> — <code>{'{"all": "1"}'}</code> targets every node (this
      is what the “All nodes” tab stores).
      <br />
      <code>{"{}"}</code> matches no node. Unknown keys match nothing.
    </Typography>
    <Box
      component="pre"
      sx={{
        mt: 0.8,
        mb: 0,
        p: 0.8,
        bgcolor: "#FFFFFF",
        border: "1px solid #CCFBF1",
        borderRadius: "6px",
        ...mono,
        fontSize: "0.72rem",
        color: "#134E4A",
        overflowX: "auto",
      }}
    >{`{
  "tag": "edge,prod",
  "env:NODE_DATA_ROLE": "gateway"
}`}</Box>
  </Box>
);

/**
 * Node selector editor shared by every feature that targets nodes with a
 * map[string]string selector (workload apps & scripts, cron tasks, SSH key
 * rules, ...).
 *
 * Tabs: Specific nodes / By tag / All nodes / Custom JSON. The selected tab is
 * purely local UI state, so switching tabs always works and every tab reflects
 * the same selector. Multiple nodes or tags are combined with OR.
 */
export const NodeSelector: React.FC<NodeSelectorProps> = ({
  label = "Target Nodes",
  description,
  nodes,
  value,
  onChange,
  error,
  onErrorChange,
  action,
  compact = false,
}) => {
  const [mode, setMode] = React.useState<SelectorTargetMode>("manual");
  // Raw text of the Custom JSON editor while the user types.
  const [draft, setDraft] = React.useState<string | null>(null);
  // The selector this component last wrote; changes coming from anywhere else
  // (loading a saved item, resetting the form) re-select the matching tab.
  const ownValue = React.useRef<Record<string, string> | null>(null);
  const lastSyncedValue = React.useRef<Record<string, string> | null>(null);
  const [nodeQuery, setNodeQuery] = React.useState("");
  const [tagQuery, setTagQuery] = React.useState("");
  const [syntaxHelpOpen, setSyntaxHelpOpen] = React.useState(false);

  const agentNodes = React.useMemo(
    () => nodes.filter((n) => !n.is_external),
    [nodes],
  );
  const allNames = React.useMemo(
    () => agentNodes.map((n) => n.name).sort(),
    [agentNodes],
  );
  const allTags = React.useMemo(() => {
    const set = new Set<string>();
    agentNodes.forEach((n) =>
      n.tags?.forEach((t) => t.trim() && set.add(t.trim())),
    );
    return Array.from(set).sort();
  }, [agentNodes]);

  const entries = React.useMemo(() => Object.entries(value || {}), [value]);
  const nameValue = entries.find(([key]) => keyMatches(key, "name"))?.[1] ?? "";
  const tagValue = entries.find(([key]) => keyMatches(key, "tag"))?.[1] ?? "";
  const selectedNames = React.useMemo(() => splitList(nameValue), [nameValue]);
  const selectedTags = React.useMemo(() => splitList(tagValue), [tagValue]);

  // The tab matching a selector: the one that produced it.
  const modeOf = (selector: Record<string, string>): SelectorTargetMode => {
    const keys = Object.keys(selector || {}).map((k) => k.trim().toLowerCase());
    if (keys.length === 0) return "manual";
    if (isAllNodesSelector(selector)) return "all";
    if (keys.length === 1 && keys[0] === "name") return "manual";
    if (keys.length === 1 && keys[0] === "tag") return "tag";
    return "custom";
  };

  const write = (selector: Record<string, string>) => {
    ownValue.current = selector;
    lastSyncedValue.current = selector;
    onChange(selector);
  };

  // Follow selectors set by the parent (e.g. the edit dialog populating the form).
  React.useEffect(() => {
    if (ownValue.current === value || lastSyncedValue.current === value) return;
    lastSyncedValue.current = value;
    setMode(modeOf(value));
    setDraft(null);
  }, [value]);

  /** The selector in its canonical form for a given tab. */
  const canonical = React.useMemo((): Record<string, string> => {
    switch (mode) {
      case "all":
        return ALL_NODES_SELECTOR;
      case "manual":
        return selectedNames.length ? { name: selectedNames.join(",") } : {};
      case "tag":
        return selectedTags.length ? { tag: selectedTags.join(",") } : {};
      case "custom":
        return value;
    }
  }, [mode, selectedNames, selectedTags, value]);

  // The JSON editor mirrors the selector; any user draft is used while editing.
  const customText = draft ?? formatSelectorJSON(canonical);
  const customParsed = mode === "custom" ? parseSelectorJSON(customText) : null;
  const customError = React.useMemo(() => {
    if (mode !== "custom" || !customParsed) return null;
    if (customParsed.error || !customParsed.selector) return customParsed.error;
    return resolveSelector(agentNodes, customParsed.selector, true).error;
  }, [mode, customParsed, agentNodes]);

  const resolution = React.useMemo(
    () => resolveSelector(agentNodes, canonical, mode === "custom"),
    [agentNodes, canonical, mode],
  );
  const currentError = mode === "custom" ? customError : resolution.error;

  // Report validation errors to the parent form.
  const lastReported = React.useRef<string | null | undefined>(undefined);
  React.useEffect(() => {
    if (lastReported.current === currentError) return;
    lastReported.current = currentError;
    onErrorChange?.(currentError);
  }, [currentError, onErrorChange]);

  /** Writes the selector for the active tab (always a full selector object). */
  const commit = (next: { name?: string[]; tag?: string[]; all?: boolean }) => {
    setDraft(null);
    if (next.all) {
      write(ALL_NODES_SELECTOR);
      return;
    }
    if (mode === "manual") {
      const names = next.name ?? selectedNames;
      write(names.length ? { name: names.join(",") } : {});
    } else if (mode === "tag") {
      const tags = next.tag ?? selectedTags;
      write(tags.length ? { tag: tags.join(",") } : {});
    }
    // The Custom JSON tab only changes through its text editor.
  };

  const onCustomChange = (text: string) => {
    setDraft(text);
    const parsed = parseSelectorJSON(text);
    if (parsed.error || !parsed.selector) return;
    if (resolveSelector(agentNodes, parsed.selector, true).error) return;
    write(parsed.selector);
  };

  const visibleNodes = nodeQuery.trim()
    ? agentNodes.filter((n) => {
        const q = nodeQuery.trim().toLowerCase();
        return (
          n.name.toLowerCase().includes(q) ||
          (n.description || "").toLowerCase().includes(q) ||
          (n.host || "").toLowerCase().includes(q) ||
          (n.ip || "").toLowerCase().includes(q) ||
          (n.ip6 || "").toLowerCase().includes(q) ||
          (n.tags || []).some((t) => t.toLowerCase().includes(q))
        );
      })
    : agentNodes;

  const visibleTags = tagQuery.trim()
    ? allTags.filter((t) =>
        t.toLowerCase().includes(tagQuery.trim().toLowerCase()),
      )
    : allTags;

  const tagNodeCount = (tag: string) =>
    agentNodes.filter((n) => (n.tags || []).includes(tag)).length;
  const unavailableNames = selectedNames.filter(
    (name) => !allNames.includes(name),
  );
  const allSelected =
    agentNodes.length > 0 && selectedNames.length === agentNodes.length;
  const matchedIsAll =
    agentNodes.length > 0 &&
    resolution.matched.length === agentNodes.length &&
    !resolution.error;

  const smallButtonSx = {
    textTransform: "none",
    fontSize: "0.72rem",
    fontWeight: 600,
    py: 0.2,
    px: 1,
    color: "#475569",
    borderColor: "#E2E8F0",
    bgcolor: "#FFFFFF",
  } as const;

  return (
    <Box
      sx={{
        border: "1px solid #E2E8F0",
        borderRadius: "12px",
        p: compact ? 1.2 : 1.5,
        bgcolor: "#FCFCFD",
      }}
    >
      <Typography
        variant="subtitle2"
        sx={{ fontWeight: 800, color: "#1E293B", mb: 0.3 }}
      >
        {label}
      </Typography>
      {description && (
        <Typography
          variant="caption"
          sx={{ color: "#64748B", display: "block", mb: 1 }}
        >
          {description}
        </Typography>
      )}

      <Box
        sx={{
          display: "flex",
          alignItems: "flex-end",
          gap: 1,
          borderBottom: "1px solid #E2E8F0",
          mb: 1.2,
        }}
      >
        <Tabs
          value={mode}
          onChange={(_, v: SelectorTargetMode) => {
            setMode(v);
            if (v === "all") write(ALL_NODES_SELECTOR);
          }}
          variant="scrollable"
          scrollButtons={false}
          sx={{
            minHeight: 34,
            flex: "1 1 auto",
            minWidth: 0,
            "& .MuiTab-root": {
              textTransform: "none",
              fontWeight: 700,
              fontSize: "0.78rem",
              minHeight: 34,
              px: 1.5,
            },
            "& .Mui-selected": { color: "#4F46E5" },
            "& .MuiTabs-indicator": { bgcolor: "#4F46E5" },
          }}
        >
          <Tab value="manual" label="Specific nodes" />
          <Tab value="tag" label="By tag" />
          <Tab value="all" label="All nodes" />
          <Tab value="custom" label="Custom JSON" />
        </Tabs>
        {action && <Box sx={{ pb: 0.6 }}>{action}</Box>}
      </Box>

      {mode === "manual" && (
        <Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mb: 0.8,
              flexWrap: "wrap",
            }}
          >
            <TextField
              size="small"
              sx={{ flex: 1, minWidth: 180 }}
              placeholder="Filter nodes by name, IP, tag…"
              value={nodeQuery}
              onChange={(e) => setNodeQuery(e.target.value)}
              InputProps={{
                startAdornment: (
                  <Search
                    size={15}
                    color="#94A3B8"
                    style={{ marginRight: 6 }}
                  />
                ),
                endAdornment: nodeQuery ? (
                  <X
                    size={14}
                    color="#94A3B8"
                    style={{ cursor: "pointer" }}
                    onClick={() => setNodeQuery("")}
                  />
                ) : undefined,
                sx: { height: 32, fontSize: "0.8rem", bgcolor: "#FFFFFF" },
              }}
            />
            <Button
              size="small"
              variant="outlined"
              sx={smallButtonSx}
              disabled={allSelected}
              onClick={() => commit({ name: allNames })}
            >
              Select all
            </Button>
            <Button
              size="small"
              variant="outlined"
              sx={smallButtonSx}
              disabled={selectedNames.length === 0}
              onClick={() => commit({ name: [] })}
            >
              Clear
            </Button>
          </Box>

          <Box
            sx={{
              display: "grid",
              gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))",
              gap: 0.6,
              maxHeight: compact ? 150 : 190,
              overflowY: "auto",
              p: 0.8,
              bgcolor: "#FFFFFF",
              borderRadius: 1,
              border: "1px solid #E2E8F0",
            }}
          >
            {visibleNodes.length === 0 ? (
              <Typography
                variant="body2"
                sx={{
                  gridColumn: "1 / -1",
                  py: 2,
                  textAlign: "center",
                  color: "#94A3B8",
                }}
              >
                {agentNodes.length === 0
                  ? "No managed nodes available"
                  : `No nodes matching "${nodeQuery}"`}
              </Typography>
            ) : (
              visibleNodes.map((n) => {
                const isSelected = selectedNames.includes(n.name);
                return (
                  <Box
                    key={n.name}
                    role="checkbox"
                    aria-checked={isSelected}
                    onClick={() =>
                      commit({ name: toggle(selectedNames, n.name) })
                    }
                    sx={{
                      p: 0.6,
                      px: 0.9,
                      borderRadius: 1,
                      border: "1px solid",
                      borderColor: isSelected ? "#4F46E5" : "#E2E8F0",
                      bgcolor: isSelected ? "#EEF2FF" : "#F8FAFC",
                      cursor: "pointer",
                      display: "flex",
                      alignItems: "center",
                      gap: 0.6,
                      transition:
                        "border-color 0.15s ease, background-color 0.15s ease",
                      "&:hover": { borderColor: "#818CF8" },
                    }}
                  >
                    {isSelected ? (
                      <CheckCircle2
                        size={15}
                        color="#4F46E5"
                        style={{ flexShrink: 0 }}
                      />
                    ) : (
                      <Box
                        sx={{
                          width: 15,
                          height: 15,
                          borderRadius: "50%",
                          border: "1.5px solid #CBD5E1",
                          flexShrink: 0,
                        }}
                      />
                    )}
                    <Box sx={{ minWidth: 0 }}>
                      <Typography
                        variant="body2"
                        noWrap
                        sx={{
                          fontWeight: isSelected ? 700 : 500,
                          color: isSelected ? "#312E81" : "#1E293B",
                          lineHeight: 1.3,
                        }}
                      >
                        {n.name}
                      </Typography>
                      <Typography
                        variant="caption"
                        noWrap
                        sx={{
                          display: "block",
                          color: "#64748B",
                          fontSize: "0.66rem",
                        }}
                      >
                        {(n.tags || []).length
                          ? (n.tags || []).map((t) => `#${t}`).join(" ")
                          : n.mode === "agent"
                            ? "agent"
                            : "no agent"}
                      </Typography>
                    </Box>
                  </Box>
                );
              })
            )}
          </Box>

          {unavailableNames.length > 0 && (
            <Box
              sx={{
                display: "flex",
                gap: 0.5,
                alignItems: "center",
                mt: 0.8,
                color: "#B45309",
              }}
            >
              <AlertTriangle size={14} />
              <Typography variant="caption">
                Not a managed node (never matched):{" "}
                <strong>{unavailableNames.join(", ")}</strong>
              </Typography>
            </Box>
          )}
        </Box>
      )}

      {mode === "tag" && (
        <Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mb: 0.8,
              flexWrap: "wrap",
            }}
          >
            <TextField
              size="small"
              sx={{ flex: 1, minWidth: 180 }}
              placeholder="Filter tags…"
              value={tagQuery}
              onChange={(e) => setTagQuery(e.target.value)}
              InputProps={{
                startAdornment: (
                  <Search
                    size={15}
                    color="#94A3B8"
                    style={{ marginRight: 6 }}
                  />
                ),
                sx: { height: 32, fontSize: "0.8rem", bgcolor: "#FFFFFF" },
              }}
            />
            <Button
              size="small"
              variant="outlined"
              sx={smallButtonSx}
              disabled={visibleTags.length === 0}
              onClick={() => commit({ tag: visibleTags })}
            >
              Select filtered
            </Button>
            <Button
              size="small"
              variant="outlined"
              sx={smallButtonSx}
              disabled={selectedTags.length === 0}
              onClick={() => commit({ tag: [] })}
            >
              Clear
            </Button>
          </Box>

          <Box
            sx={{
              display: "flex",
              flexWrap: "wrap",
              gap: 0.5,
              minHeight: 32,
              alignItems: "center",
            }}
          >
            {visibleTags.length === 0 ? (
              <Typography
                variant="caption"
                sx={{ color: "#94A3B8", fontStyle: "italic" }}
              >
                {allTags.length === 0
                  ? "No node tags defined. Enter a tag below to match nodes tagged later."
                  : `No tags matching "${tagQuery}"`}
              </Typography>
            ) : (
              visibleTags.map((tag) => {
                const selected = selectedTags.includes(tag);
                return (
                  <Chip
                    key={tag}
                    label={`#${tag} · ${tagNodeCount(tag)}`}
                    size="small"
                    onClick={() => commit({ tag: toggle(selectedTags, tag) })}
                    sx={{
                      height: 24,
                      fontSize: "0.75rem",
                      fontWeight: selected ? 700 : 500,
                      bgcolor: selected ? "#EDE9FE" : "#FFFFFF",
                      color: selected ? "#5B21B6" : "#475569",
                      border: "1px solid",
                      borderColor: selected ? "#C4B5FD" : "#E2E8F0",
                    }}
                  />
                );
              })
            )}
          </Box>
          <TextField
            size="small"
            fullWidth
            placeholder="Or enter tags separated by commas (e.g. core, edge)"
            value={tagValue}
            onChange={(e) => commit({ tag: splitList(e.target.value) })}
            sx={{ mt: 0.8 }}
            InputProps={{
              startAdornment: (
                <Tag size={15} color="#8B5CF6" style={{ marginRight: 6 }} />
              ),
              sx: { bgcolor: "#FFFFFF" },
            }}
          />
        </Box>
      )}

      {mode === "all" && (
        <Box
          sx={{
            p: 1,
            bgcolor: "#E0F2FE",
            borderRadius: "8px",
            border: "1px solid #BAE6FD",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Globe size={16} color="#0284C7" />
            <Typography
              variant="body2"
              sx={{ fontWeight: 700, color: "#0369A1" }}
            >
              All nodes ({agentNodes.length} managed node
              {agentNodes.length === 1 ? "" : "s"})
            </Typography>
          </Box>
          <Typography
            variant="caption"
            sx={{ color: "#0C4A6E", display: "block", mt: 0.3 }}
          >
            Applies to every managed node, including nodes added later.
          </Typography>
        </Box>
      )}

      {mode === "custom" && (
        <Box>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mb: 0.6,
              flexWrap: "wrap",
            }}
          >
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Editing the selector directly. Changes apply to the other tabs
              once the JSON is valid.
            </Typography>
            <Typography
              component="button"
              type="button"
              onClick={() => setSyntaxHelpOpen((v) => !v)}
              sx={{
                all: "unset",
                ml: "auto",
                cursor: "pointer",
                fontSize: "0.72rem",
                fontWeight: 700,
                color: "#0F766E",
                "&:hover": { textDecoration: "underline" },
              }}
            >
              {syntaxHelpOpen ? "Hide syntax help" : "Selector syntax help"}
            </Typography>
          </Box>
          <TextField
            fullWidth
            multiline
            minRows={compact ? 3 : 4}
            size="small"
            value={customText}
            onChange={(e) => onCustomChange(e.target.value)}
            placeholder='{"tag": "core", "env:NODE_IP": "192.168.100.1"}'
            error={Boolean(customError || error)}
            helperText={
              customError ||
              error ||
              'JSON object, e.g. {"name":"node-a,node-b"}, {"tag":"edge,prod"}, {"env:NODE_IP":"10.0.0.1"} or {"all":"1"}. {} matches no node.'
            }
            inputProps={{
              style: { fontFamily: mono.fontFamily, fontSize: "0.8rem" },
            }}
            FormHelperTextProps={{
              sx: { fontFamily: customError ? mono.fontFamily : undefined },
            }}
            sx={{ "& textarea": { bgcolor: "#FFFFFF" } }}
          />
          {syntaxHelpOpen && <SelectorSyntaxHelp />}
        </Box>
      )}

      {mode !== "custom" && (currentError || error) && (
        <Typography
          variant="caption"
          sx={{ color: "#DC2626", display: "block", mt: 0.8 }}
        >
          {currentError || error}
        </Typography>
      )}

      <Box sx={{ mt: 1 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.6, mb: 0.5 }}>
          <Typography
            variant="caption"
            sx={{ color: "#475569", fontWeight: 700 }}
          >
            Matched nodes ({resolution.matched.length}
            {!matchedIsAll ? ` of ${agentNodes.length}` : ""})
          </Typography>
          {matchedIsAll && (
            <Chip
              label="All nodes"
              size="small"
              sx={{
                height: 18,
                fontSize: "0.65rem",
                bgcolor: "#E0F2FE",
                color: "#0369A1",
              }}
            />
          )}
        </Box>
        <MatchList nodes={resolution.matched} />
      </Box>
    </Box>
  );
};

export default NodeSelector;
