import React, { useMemo, useCallback, useState, useEffect, useRef } from "react";
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  Panel,
  useNodesState,
  useEdgesState,
  useReactFlow,
  addEdge,
  Connection,
  Edge,
  Node as FlowNode,
  BackgroundVariant,
  OnNodeDrag,
  Viewport,
  OnMoveEnd,
  NodeChange,
} from "@xyflow/react";
import {
  Box,
  Typography,
  Button,
  Chip,
  Tooltip,
  Divider,
  FormControl,
  Select,
  MenuItem,
  Menu,
  Autocomplete,
  TextField,
} from "@mui/material";
import {
  Network,
  Layers,
  EyeOff,
  X,
  Maximize2,
  Minimize2,
  Plus,
  Link as LinkIcon,
  Share2,
  ChevronDown,
  Search,
  Focus,
  UnfoldVertical,
  FoldVertical,
  Check,
  Download,
  Activity,
} from "lucide-react";
import { NodeCard } from "./NodeCard";
import { CustomEdge } from "./CustomEdge";
import { BlockNode, BLOCK_PALETTE } from "./BlockNode";
import { BlockVirtualEdge } from "./BlockVirtualEdge";
import { ExportDiagramModal } from "./ExportDiagramModal";
import { Node, Link, NodeStatus, NetworkState, GraphBlock, NetworkPolicy } from "../../types/api";
import { api } from "../../api/client";
import { getEffectiveLinkCost } from "../../utils/cost";
import { calculateGraphCentrality } from "../../utils/graphCentrality";

const STORAGE_KEY_VIEWPORT = "easy42_graph_viewport";
const STORAGE_KEY_ZOOM = "easy42_graph_zoom";
const STORAGE_KEY_BLOCKS = "easy42_graph_blocks_v1";
const STORAGE_KEY_NODE_BLOCKS = "easy42_node_blocks_v1";
const STORAGE_KEY_EXPANDED_PAIRS = "easy42_expanded_virtual_edge_pairs_v1";
const STORAGE_KEY_CENTRALITY_VIEW = "easy42_topology_centrality_view";

const getStoredExpandedPairs = (): Set<string> => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_EXPANDED_PAIRS);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return new Set(parsed);
      }
    }
  } catch (e) {
    console.error("Failed to load expanded virtual edge pairs from localStorage", e);
  }
  return new Set();
};

const getStoredViewport = (): Viewport | undefined => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_VIEWPORT);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (
        parsed &&
        typeof parsed.zoom === "number" &&
        !isNaN(parsed.zoom) &&
        typeof parsed.x === "number" &&
        !isNaN(parsed.x) &&
        typeof parsed.y === "number" &&
        !isNaN(parsed.y)
      ) {
        return {
          x: parsed.x,
          y: parsed.y,
          zoom: parsed.zoom,
        };
      }
    }
  } catch (e) {
    console.error("Failed to load graph viewport from localStorage", e);
  }
  return undefined;
};

const getStoredBlocks = (): GraphBlock[] => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_BLOCKS);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {
        return parsed;
      }
    }
  } catch (e) {
    console.error("Failed to load blocks from localStorage", e);
  }
  return [];
};

const getStoredNodeBlocks = (): Record<string, string> => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY_NODE_BLOCKS);
    if (raw) {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === "object") {
        return parsed;
      }
    }
  } catch (e) {
    console.error("Failed to load node blocks from localStorage", e);
  }
  return {};
};

import { isManualLink, isWireGuardLink } from "../../utils/link";
export { isManualLink, isWireGuardLink };

export interface MaximumCliqueResult {
  cliqueNodes: Set<string>;
  maxSize: number;
}

/**
 * Computes the maximum full-mesh clique in an intra-block undirected graph.
 * Only WireGuard links are counted; manual type links (BGP-only data plane)
 * are excluded because easy42 does not manage WireGuard mesh data planes for them.
 * If the maximum clique has size < 2, returns an empty set and maxSize = 0.
 */
export function getMaximumCliqueNodes(
  nodeNames: string[],
  links: { from: { name: string; type?: string }; to: { name: string; type?: string }; type?: string }[],
): MaximumCliqueResult {
  if (nodeNames.length < 2) return { cliqueNodes: new Set(), maxSize: 0 };

  const nodeSet = new Set(nodeNames);
  const adj = new Map<string, Set<string>>();
  nodeNames.forEach((n) => adj.set(n, new Set()));

  links.forEach((l) => {
    if (nodeSet.has(l.from.name) && nodeSet.has(l.to.name) && l.from.name !== l.to.name) {
      adj.get(l.from.name)!.add(l.to.name);
      adj.get(l.to.name)!.add(l.from.name);
    }
  });

  let maxSize = 0;
  const maxCliques: Set<string>[] = [];

  function bronKerbosch(R: Set<string>, P: Set<string>, X: Set<string>) {
    if (P.size === 0 && X.size === 0) {
      if (R.size >= 2) {
        if (R.size > maxSize) {
          maxSize = R.size;
          maxCliques.length = 0;
          maxCliques.push(new Set(R));
        } else if (R.size === maxSize) {
          maxCliques.push(new Set(R));
        }
      }
      return;
    }

    const pivot = Array.from(P)[0] || Array.from(X)[0];
    const pivotNeighbors = adj.get(pivot) || new Set();
    const candidates = Array.from(P).filter((v) => !pivotNeighbors.has(v));

    for (const v of candidates) {
      const vNeighbors = adj.get(v) || new Set();
      const nextR = new Set(R).add(v);
      const nextP = new Set(Array.from(P).filter((p) => vNeighbors.has(p)));
      const nextX = new Set(Array.from(X).filter((x) => vNeighbors.has(x)));
      bronKerbosch(nextR, nextP, nextX);
      P.delete(v);
      X.add(v);
    }
  }

  bronKerbosch(new Set(), new Set(nodeNames), new Set());

  if (maxSize < 2 || maxCliques.length === 0) {
    return { cliqueNodes: new Set(), maxSize: 0 };
  }

  // If there is a unique maximum clique (including when the entire block is full-mesh),
  // return its members. If there are multiple tied cliques with no single dominant core,
  // do not arbitrarily pick one so nodes are not falsely labeled as the unique mesh core.
  const cliqueNodes = maxCliques.length === 1 ? maxCliques[0] : new Set<string>();

  return { cliqueNodes, maxSize };
}

/**
 * Evaluates whether a WireGuard peer has an active/recent handshake (within 3 minutes).
 * Allows future timestamps within a 5-minute clock skew window to handle client/server NTP drift.
 */
export function isRecentHandshake(dateStr?: string): boolean {
  if (!dateStr) return false;
  const d = new Date(dateStr).getTime();
  if (isNaN(d) || d <= 0) return false;
  if (new Date(dateStr).getFullYear() < 2020) return false;
  const ageMs = Date.now() - d;
  return ageMs >= -5 * 60 * 1000 && ageMs <= 3 * 60 * 1000;
}

/**
 * Checks whether live WireGuard telemetry for a node endpoint indicates
 * the link is actively working or confirmed down.
 */
export function checkLiveEndpointWorkingState(
  nodeStatus?: NodeStatus,
  ifaceName?: string,
  peerPubKey?: string,
): "working" | "not_working" | "unknown" {
  if (!nodeStatus || !nodeStatus.connected || !nodeStatus.wg_interfaces || !ifaceName) {
    return "unknown";
  }

  const wgIface = nodeStatus.wg_interfaces.find((w) => w.name === ifaceName);
  if (!wgIface) {
    return "not_working";
  }

  const peer = wgIface.peers?.find((p) => (peerPubKey ? p.public_key === peerPubKey : true)) || wgIface.peers?.[0];
  if (!peer) {
    return "not_working";
  }

  if (isRecentHandshake(peer.latest_handshake)) {
    return "working";
  }

  if (peer.persistent_keepalive > 0) {
    return "not_working";
  }

  return "unknown";
}

/**
 * Determines the working state of a link ("working", "not_working", or "unknown").
 * Prioritizes real-time live telemetry from connected agents, falling back to
 * recorded working_state in networkState.
 * For manual links (BGP-only data plane), easy42 does not manage WireGuard handshakes;
 * if both participating nodes are online and reachable, the manual link is considered working.
 */
export function getLinkWorkingState(
  link: Link,
  networkState?: NetworkState | null,
  nodeStatuses?: Record<string, NodeStatus>,
): "working" | "not_working" | "unknown" {
  const isManual = isManualLink(link);

  if (isManual) {
    const fromOnline = Boolean(nodeStatuses?.[link.from.name]?.connected);
    const toOnline = Boolean(nodeStatuses?.[link.to.name]?.connected);
    return fromOnline && toOnline ? "working" : "not_working";
  }

  // 1. Check real-time live telemetry from connected agents first
  const fromLive = checkLiveEndpointWorkingState(
    nodeStatuses?.[link.from.name],
    link.from.interface,
    link.to.public_key,
  );
  const toLive = checkLiveEndpointWorkingState(nodeStatuses?.[link.to.name], link.to.interface, link.from.public_key);

  // If either endpoint has a verified recent handshake, the WireGuard link is working!
  if (fromLive === "working" || toLive === "working") {
    return "working";
  }

  // If both endpoints are monitored live and both confirm not_working
  if (fromLive === "not_working" && toLive === "not_working") {
    return "not_working";
  }

  // If one endpoint is monitored live and confirms not_working, while other is not connected
  if (
    (fromLive === "not_working" && !nodeStatuses?.[link.to.name]?.connected) ||
    (toLive === "not_working" && !nodeStatuses?.[link.from.name]?.connected)
  ) {
    return "not_working";
  }

  // 2. Fall back to persisted networkState
  const fromIface = networkState?.nodes?.[link.from.name]?.interfaces?.[link.from.interface];
  const toIface = networkState?.nodes?.[link.to.name]?.interfaces?.[link.to.interface];

  if (fromIface?.working_state === "working" || toIface?.working_state === "working") {
    return "working";
  }
  if (fromIface?.working_state === "not_working" || toIface?.working_state === "not_working") {
    return "not_working";
  }
  if (fromIface?.working_state === "unknown" || toIface?.working_state === "unknown") {
    return "unknown";
  }
  return "unknown";
}

interface TopologyGraphProps {
  nodes: Node[];
  links: Link[];
  nodeStatuses: Record<string, NodeStatus>;
  networkState?: NetworkState | null;
  networkPolicies?: NetworkPolicy[];
  selectedTag?: string;
  onSelectTag?: (tag: string) => void;
  uniqueTags?: string[];
  onSelectNode: (node: Node) => void;
  onSelectLink: (link: Link) => void;
  onConnectNodes: (sourceName: string, targetName: string) => void;
  onNodePositionChange?: (name: string, x: number, y: number) => void;
  onRefreshNode?: (nodeName: string) => void;
  refreshingNodeName?: string | null;
  addBlockTrigger?: number;
  onAddNode?: () => void;
  onAddLink?: () => void;
  onAddBlock?: () => void;
  onCreateFullMesh?: () => void;
  missingMeshLinksCount?: number;
  displayedInternalNodesCount?: number;
  onCreateBlockFullMesh?: (blockName: string, memberNames: string[]) => void;
}

const nodeTypes = {
  customNode: NodeCard,
  blockGroup: BlockNode,
};

const edgeTypes = {
  customEdge: CustomEdge,
  blockVirtualEdge: BlockVirtualEdge,
};

interface FloatingToolbarProps {
  nodes?: Node[];
  onSelectNode?: (node: Node) => void;
  onFocusNode?: (nodeName: string) => void;
  onAddNode?: () => void;
  onAddLink?: () => void;
  onAddBlock: () => void;
  onCreateFullMesh?: () => void;
  missingMeshLinksCount?: number;
  displayedInternalNodesCount?: number;
  selectedTag?: string;
  onSelectTag?: (tag: string) => void;
  uniqueTags?: string[];
  pinnedNodeNames: Set<string>;
  focusedBlockName: string | null;
  hopDepth: 1 | 2 | 3;
  onSetHopDepth: (depth: 1 | 2 | 3) => void;
  onClearFocus: () => void;
  blocksCount: number;
  hiddenLinksCount: number;
  lodMode: "macro" | "meso" | "micro";
  onExpandAllBundles?: () => void;
  onCollapseAllBundles?: () => void;
  onExpandAllGroups?: () => void;
  onCollapseAllGroups?: () => void;
  onOpenExportDiagram?: () => void;
  centralityViewEnabled?: boolean;
  onToggleCentralityView?: () => void;
}

const FloatingToolbar: React.FC<FloatingToolbarProps> = ({
  nodes = [],
  onSelectNode,
  onFocusNode,
  onAddNode,
  onAddLink,
  onAddBlock,
  onCreateFullMesh,
  missingMeshLinksCount = 0,
  displayedInternalNodesCount = 0,
  selectedTag,
  onSelectTag,
  uniqueTags = [],
  pinnedNodeNames,
  focusedBlockName,
  hopDepth,
  onSetHopDepth,
  onClearFocus,
  blocksCount,
  hiddenLinksCount,
  lodMode,
  onExpandAllBundles,
  onCollapseAllBundles,
  onExpandAllGroups,
  onCollapseAllGroups,
  onOpenExportDiagram,
  centralityViewEnabled = false,
  onToggleCentralityView,
}) => {
  const { fitView, zoomTo } = useReactFlow();
  const [addMenuAnchor, setAddMenuAnchor] = useState<null | HTMLElement>(null);
  const [fitMenuAnchor, setFitMenuAnchor] = useState<null | HTMLElement>(null);
  const [lodMenuAnchor, setLodMenuAnchor] = useState<null | HTMLElement>(null);
  const [locateInputValue, setLocateInputValue] = useState("");

  const MAX_RECENT_COUNT = 5;

  const { sortedNodes, recentNodeNames } = useMemo(() => {
    if (!nodes || nodes.length === 0) {
      return { sortedNodes: [], recentNodeNames: new Set<string>() };
    }

    const getModifiedTime = (n: Node): number => {
      if (!n.modified_at) return 0;
      const t = new Date(n.modified_at).getTime();
      return isNaN(t) || t <= 0 ? 0 : t;
    };

    const nodesWithTime = nodes
      .filter((n) => getModifiedTime(n) > 0)
      .sort((a, b) => getModifiedTime(b) - getModifiedTime(a));

    let recent: Node[] = [];
    if (nodesWithTime.length > 0) {
      recent = nodesWithTime.slice(0, MAX_RECENT_COUNT);
    } else if (nodes.length > MAX_RECENT_COUNT) {
      recent = nodes.slice(-MAX_RECENT_COUNT).reverse();
    }

    const recentSet = new Set(recent.map((n) => n.name));
    const remaining = nodes.filter((n) => !recentSet.has(n.name)).sort((a, b) => a.name.localeCompare(b.name));

    return {
      sortedNodes: [...recent, ...remaining],
      recentNodeNames: recentSet,
    };
  }, [nodes]);

  const handleLocateNode = useCallback(
    (targetNode: Node) => {
      setFitMenuAnchor(null);
      setLocateInputValue("");

      fitView({
        nodes: [{ id: targetNode.name }],
        duration: 500,
        maxZoom: 1.2,
        padding: 0.4,
      });

      if (onFocusNode) {
        onFocusNode(targetNode.name);
      }
      if (onSelectNode) {
        onSelectNode(targetNode);
      }
    },
    [fitView, onFocusNode, onSelectNode],
  );

  return (
    <Panel position="top-right" style={{ margin: 14 }}>
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: 0.75,
          backgroundColor: "rgba(255, 255, 255, 0.94)",
          backdropFilter: "blur(8px)",
          border: "1px solid #E2E8F0",
          borderRadius: 2.5,
          p: "6px 10px",
          boxShadow: "0 4px 16px -2px rgba(15, 23, 42, 0.08), 0 2px 6px -1px rgba(15, 23, 42, 0.04)",
          maxWidth: "calc(100vw - 32px)",
        }}
      >
        {/* Pinned Nodes with N-Hop Depth Slider */}
        {pinnedNodeNames.size > 0 && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              backgroundColor: "#EEF2FF",
              border: "1px solid #C7D2FE",
              borderRadius: 2,
              px: 1.25,
              py: 0.35,
            }}
          >
            <Focus size={13} color="#4F46E5" />
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#3730A3" }}>
              Pinned: {Array.from(pinnedNodeNames).join(", ")}
            </Typography>

            {/* N-Hop Depth Slider / Button Group */}
            <Box sx={{ display: "flex", alignItems: "center", gap: 0.25 }}>
              <Typography variant="caption" sx={{ fontSize: "0.65rem", fontWeight: 700, color: "#6366F1", mr: 0.25 }}>
                Depth:
              </Typography>
              {([1, 2, 3] as const).map((depth) => {
                const label = depth === 1 ? "1 Hop" : depth === 2 ? "2 Hops" : "Full Path";
                const isSelected = hopDepth === depth;
                return (
                  <Button
                    key={depth}
                    size="small"
                    variant={isSelected ? "contained" : "text"}
                    onClick={() => onSetHopDepth(depth)}
                    sx={{
                      height: 20,
                      minWidth: "auto",
                      px: 0.75,
                      py: 0,
                      fontSize: "0.65rem",
                      fontWeight: 700,
                      textTransform: "none",
                      borderRadius: 1,
                      backgroundColor: isSelected ? "#4F46E5" : "transparent",
                      color: isSelected ? "#FFFFFF" : "#6366F1",
                      boxShadow: "none",
                      "&:hover": {
                        backgroundColor: isSelected ? "#4338CA" : "rgba(79, 70, 229, 0.08)",
                        boxShadow: "none",
                      },
                    }}
                  >
                    {label}
                  </Button>
                );
              })}
            </Box>

            <Tooltip title="Clear pin & focus (Esc)">
              <Box
                onClick={onClearFocus}
                sx={{
                  cursor: "pointer",
                  display: "flex",
                  alignItems: "center",
                  color: "#6366F1",
                  ml: 0.5,
                  "&:hover": { color: "#312E81" },
                }}
              >
                <X size={13} />
              </Box>
            </Tooltip>
          </Box>
        )}

        {/* Stats Indicator */}
        {hiddenLinksCount > 0 && (
          <Tooltip
            title={`${hiddenLinksCount} cross-group link(s) bundled into virtual edges. Click any virtual edge badge or node to inspect.`}
          >
            <Chip
              icon={<EyeOff size={13} color="#64748B" style={{ marginLeft: 6 }} />}
              label={`${hiddenLinksCount} links bundled`}
              size="small"
              sx={{
                height: 28,
                fontSize: "0.72rem",
                fontWeight: 600,
                backgroundColor: "#F1F5F9",
                color: "#475569",
                borderRadius: 1.5,
                border: "1px solid #E2E8F0",
              }}
            />
          </Tooltip>
        )}

        {/* Single "+" Dropdown Menu Button */}
        <Button
          size="small"
          variant="contained"
          startIcon={<Plus size={15} />}
          endIcon={<ChevronDown size={13} />}
          onClick={(e) => setAddMenuAnchor(e.currentTarget)}
          sx={{
            backgroundColor: "#4F46E5",
            color: "#FFFFFF",
            fontWeight: 700,
            fontSize: "0.8rem",
            textTransform: "none",
            borderRadius: 1.75,
            px: 1.25,
            py: 0.5,
            boxShadow: "0 2px 4px rgba(79, 70, 229, 0.2)",
            "&:hover": {
              backgroundColor: "#4338CA",
            },
          }}
        >
          Add
        </Button>

        <Menu
          anchorEl={addMenuAnchor}
          open={Boolean(addMenuAnchor)}
          onClose={() => setAddMenuAnchor(null)}
          PaperProps={{
            sx: {
              mt: 0.75,
              minWidth: 160,
              borderRadius: 2,
              boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.05)",
              border: "1px solid #E2E8F0",
              p: 0.5,
            },
          }}
        >
          {onAddNode && (
            <MenuItem
              onClick={() => {
                setAddMenuAnchor(null);
                onAddNode();
              }}
              sx={{
                borderRadius: 1.25,
                py: 0.75,
                px: 1.25,
                display: "flex",
                alignItems: "center",
                gap: 1.25,
                fontSize: "0.82rem",
                fontWeight: 600,
                color: "#1E293B",
                "&:hover": { bgcolor: "#F1F5F9" },
              }}
            >
              <Plus size={15} color="#4F46E5" />
              Node
            </MenuItem>
          )}

          {onAddLink && (
            <MenuItem
              onClick={() => {
                setAddMenuAnchor(null);
                onAddLink();
              }}
              sx={{
                borderRadius: 1.25,
                py: 0.75,
                px: 1.25,
                display: "flex",
                alignItems: "center",
                gap: 1.25,
                fontSize: "0.82rem",
                fontWeight: 600,
                color: "#1E293B",
                "&:hover": { bgcolor: "#F1F5F9" },
              }}
            >
              <LinkIcon size={15} color="#4F46E5" />
              Link
            </MenuItem>
          )}

          <MenuItem
            onClick={() => {
              setAddMenuAnchor(null);
              onAddBlock();
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1.25,
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              fontSize: "0.82rem",
              fontWeight: 600,
              color: "#1E293B",
              "&:hover": { bgcolor: "#F1F5F9" },
            }}
          >
            <Layers size={15} color="#4F46E5" />
            Block
          </MenuItem>

          {onCreateFullMesh && (
            <>
              <Divider sx={{ my: 0.5, borderColor: "#E2E8F0" }} />
              <MenuItem
                disabled={displayedInternalNodesCount < 2}
                onClick={() => {
                  setAddMenuAnchor(null);
                  onCreateFullMesh();
                }}
                sx={{
                  borderRadius: 1.25,
                  py: 0.75,
                  px: 1.25,
                  display: "flex",
                  alignItems: "center",
                  gap: 1.25,
                  fontSize: "0.82rem",
                  fontWeight: 600,
                  color: missingMeshLinksCount > 0 ? "#4F46E5" : "#64748B",
                  "&:hover": { bgcolor: "#F1F5F9" },
                }}
              >
                <Share2 size={15} color={missingMeshLinksCount > 0 ? "#4F46E5" : "#64748B"} />
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                  <span>Full Mesh</span>
                  {missingMeshLinksCount > 0 && (
                    <Chip
                      label={`+${missingMeshLinksCount}`}
                      size="small"
                      sx={{
                        height: 18,
                        fontSize: "0.65rem",
                        fontWeight: 700,
                        backgroundColor: "rgba(79, 70, 229, 0.1)",
                        color: "#4F46E5",
                        borderRadius: 1,
                      }}
                    />
                  )}
                </Box>
              </MenuItem>
            </>
          )}
        </Menu>

        {/* Fit Dropdown Menu Button */}
        <Button
          variant="outlined"
          size="small"
          startIcon={<Maximize2 size={15} />}
          endIcon={<ChevronDown size={13} />}
          onClick={(e) => setFitMenuAnchor(e.currentTarget)}
          sx={{
            borderColor: "#CBD5E1",
            color: "#334155",
            fontWeight: 600,
            fontSize: "0.8rem",
            textTransform: "none",
            borderRadius: 1.75,
            px: 1.25,
            py: 0.5,
            "&:hover": {
              borderColor: "#94A3B8",
              backgroundColor: "#F8FAFC",
            },
          }}
        >
          Fit
        </Button>

        <Menu
          anchorEl={fitMenuAnchor}
          open={Boolean(fitMenuAnchor)}
          onClose={() => {
            setFitMenuAnchor(null);
            setLocateInputValue("");
          }}
          transitionDuration={{ enter: 0, exit: 100 }}
          disableAutoFocusItem
          PaperProps={{
            sx: {
              mt: 0.75,
              minWidth: 260,
              maxWidth: 320,
              borderRadius: 2,
              boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.05)",
              border: "1px solid #E2E8F0",
              p: 0.5,
            },
          }}
        >
          <MenuItem
            onClick={() => {
              setFitMenuAnchor(null);
              fitView({ padding: 0.2, duration: 400 });
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1.25,
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              fontSize: "0.82rem",
              fontWeight: 600,
              color: "#1E293B",
              "&:hover": { bgcolor: "#F1F5F9" },
            }}
          >
            <Maximize2 size={15} color="#4F46E5" />
            Fit View
          </MenuItem>

          <Divider sx={{ my: 0.5, borderColor: "#E2E8F0" }} />

          <MenuItem
            component="div"
            disableRipple
            sx={{
              display: "block",
              cursor: "default",
              px: 1.25,
              py: 0.75,
              "&:hover": { backgroundColor: "transparent" },
            }}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key !== "Escape") {
                e.stopPropagation();
              }
            }}
          >
            <Typography
              variant="caption"
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 0.5,
                fontWeight: 600,
                color: "#64748B",
                fontSize: "0.72rem",
                mb: 0.75,
              }}
            >
              <Search size={12} color="#64748B" />
              Locate Node
            </Typography>

            <Autocomplete
              size="small"
              autoHighlight
              openOnFocus
              options={sortedNodes}
              getOptionLabel={(option) => option.name}
              isOptionEqualToValue={(option, value) => option.name === value?.name}
              inputValue={locateInputValue}
              onInputChange={(_event, value) => setLocateInputValue(value)}
              onChange={(_event, option) => {
                if (option) {
                  handleLocateNode(option);
                }
              }}
              filterOptions={(options, state) => {
                const q = state.inputValue.trim().toLowerCase();
                if (!q) return options;
                const prefixMatches: Node[] = [];
                const otherMatches: Node[] = [];
                for (const n of options) {
                  if (n.name.toLowerCase().startsWith(q)) {
                    prefixMatches.push(n);
                  } else if (n.name.toLowerCase().includes(q) || (n.ip && n.ip.toLowerCase().includes(q))) {
                    otherMatches.push(n);
                  }
                }
                return [...prefixMatches, ...otherMatches];
              }}
              slotProps={{
                popper: {
                  placement: "bottom-start",
                  modifiers: [
                    {
                      name: "flip",
                      enabled: false,
                    },
                    {
                      name: "preventOverflow",
                      options: {
                        altAxis: true,
                      },
                    },
                  ],
                  sx: {
                    zIndex: 1400,
                  },
                },
              }}
              renderInput={(params) => (
                <TextField
                  {...params}
                  placeholder="Enter prefix to find..."
                  autoFocus
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      const q = locateInputValue.trim().toLowerCase();
                      if (q) {
                        const match =
                          sortedNodes.find((n) => n.name.toLowerCase().startsWith(q)) ||
                          sortedNodes.find((n) => n.name.toLowerCase().includes(q));
                        if (match) {
                          e.preventDefault();
                          handleLocateNode(match);
                        }
                      }
                    }
                  }}
                  sx={{
                    "& .MuiOutlinedInput-root": {
                      fontSize: "0.82rem",
                      borderRadius: 1.5,
                      backgroundColor: "#F8FAFC",
                      "& fieldset": {
                        borderColor: "#CBD5E1",
                      },
                      "&:hover fieldset": {
                        borderColor: "#94A3B8",
                      },
                      "&.Mui-focused fieldset": {
                        borderColor: "#4F46E5",
                      },
                      "& input": {
                        py: 0.5,
                        px: 0.75,
                      },
                    },
                  }}
                />
              )}
              renderOption={(props, option) => {
                const { key, ...otherProps } = props as any;
                const isRecent = recentNodeNames.has(option.name);
                return (
                  <Box
                    component="li"
                    key={key || option.name}
                    {...otherProps}
                    sx={{
                      fontSize: "0.82rem",
                      py: 0.6,
                      px: 1,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      gap: 1,
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontSize: "0.82rem",
                          fontWeight: 600,
                          color: "#1E293B",
                        }}
                        noWrap
                      >
                        {option.name}
                      </Typography>
                      {option.ip && (
                        <Typography
                          variant="caption"
                          sx={{
                            fontSize: "0.72rem",
                            color: "#64748B",
                          }}
                          noWrap
                        >
                          {option.ip}
                        </Typography>
                      )}
                    </Box>
                    {isRecent && (
                      <Chip
                        label="Recent"
                        size="small"
                        sx={{
                          height: 18,
                          fontSize: "0.62rem",
                          fontWeight: 700,
                          backgroundColor: "rgba(79, 70, 229, 0.1)",
                          color: "#4F46E5",
                          borderRadius: 1,
                          flexShrink: 0,
                        }}
                      />
                    )}
                  </Box>
                );
              }}
            />
          </MenuItem>
        </Menu>

        {/* Tag filter dropdown */}
        {onSelectTag && (
          <FormControl size="small" sx={{ minWidth: 100 }}>
            <Select
              value={selectedTag || "All"}
              onChange={(e) => onSelectTag(e.target.value)}
              sx={{
                height: 31,
                fontSize: "0.8rem",
                fontWeight: 500,
                borderRadius: 1.75,
                bgcolor: "#FFFFFF",
                "& .MuiSelect-select": {
                  py: 0.5,
                  px: 1.25,
                },
              }}
            >
              <MenuItem value="All" sx={{ fontSize: "0.8rem" }}>
                All Tags
              </MenuItem>
              {uniqueTags.map((t) => (
                <MenuItem key={t} value={t} sx={{ fontSize: "0.8rem" }}>
                  {t}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        )}

        {/* Group-Level Focus Banner */}
        {focusedBlockName && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 0.75,
              backgroundColor: "#FAF5FF",
              border: "1px solid #E9D5FF",
              borderRadius: 2,
              px: 1.25,
              py: 0.35,
            }}
          >
            <Layers size={13} color="#9333EA" />
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#6B21A8" }}>
              Group: {focusedBlockName}
            </Typography>
            <Tooltip title="Clear group focus (Esc)">
              <Box
                onClick={onClearFocus}
                sx={{
                  cursor: "pointer",
                  display: "flex",
                  alignItems: "center",
                  color: "#A855F7",
                  "&:hover": { color: "#581C87" },
                }}
              >
                <X size={13} />
              </Box>
            </Tooltip>
          </Box>
        )}

        {/* Core Elements / Centrality Toggle */}
        <Tooltip
          title={
            centralityViewEnabled
              ? "Disable Centrality View"
              : "Enable Centrality View (highlight core transit nodes & backbone links)"
          }
        >
          <Button
            size="small"
            variant={centralityViewEnabled ? "contained" : "outlined"}
            onClick={onToggleCentralityView}
            startIcon={<Activity size={13} />}
            sx={{
              height: 28,
              fontSize: "0.72rem",
              fontWeight: 700,
              textTransform: "none",
              borderRadius: 1.75,
              px: 1,
              py: 0.25,
              backgroundColor: centralityViewEnabled ? "#059669" : "#F8FAFC",
              color: centralityViewEnabled ? "#FFFFFF" : "#334155",
              borderColor: centralityViewEnabled ? "#059669" : "#CBD5E1",
              boxShadow: centralityViewEnabled ? "0 2px 4px rgba(5, 150, 105, 0.2)" : "none",
              "&:hover": {
                backgroundColor: centralityViewEnabled ? "#047857" : "#F1F5F9",
                borderColor: centralityViewEnabled ? "#047857" : "#94A3B8",
              },
            }}
          >
            Core View
          </Button>
        </Tooltip>

        {/* More options dropdown */}
        <Button
          size="small"
          variant="outlined"
          onClick={(e) => setLodMenuAnchor(e.currentTarget)}
          endIcon={<ChevronDown size={12} />}
          sx={{
            height: 28,
            fontSize: "0.72rem",
            fontWeight: 700,
            textTransform: "none",
            borderRadius: 1.75,
            px: 1,
            py: 0.25,
            backgroundColor: "#F8FAFC",
            color: "#475569",
            border: "1px solid",
            borderColor: "#CBD5E1",
            "&:hover": {
              backgroundColor: "#F1F5F9",
              borderColor: "#94A3B8",
            },
          }}
        >
          More
        </Button>

        <Menu
          anchorEl={lodMenuAnchor}
          open={Boolean(lodMenuAnchor)}
          onClose={() => setLodMenuAnchor(null)}
          PaperProps={{
            sx: {
              mt: 0.75,
              minWidth: 230,
              borderRadius: 2,
              boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.05)",
              border: "1px solid #E2E8F0",
              p: 0.75,
            },
          }}
        >
          <Box sx={{ px: 1, py: 0.5 }}>
            <Typography
              variant="caption"
              sx={{
                fontSize: "0.65rem",
                fontWeight: 700,
                color: "#94A3B8",
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Zoom Level of Detail
            </Typography>
          </Box>

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              zoomTo(0.42, { duration: 400 });
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              fontSize: "0.82rem",
              fontWeight: lodMode === "macro" ? 700 : 500,
              color: lodMode === "macro" ? "#7C3AED" : "#1E293B",
              backgroundColor: lodMode === "macro" ? "rgba(124, 58, 237, 0.08)" : "transparent",
              "&:hover": { bgcolor: "#F5F3FF" },
            }}
          >
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: lodMode === "macro" ? 700 : 600 }}>
                Macro (&lt;0.5x)
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                Group blocks &amp; bundled links
              </Typography>
            </Box>
            {lodMode === "macro" && <Check size={15} color="#7C3AED" />}
          </MenuItem>

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              zoomTo(0.85, { duration: 400 });
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              fontSize: "0.82rem",
              fontWeight: lodMode === "meso" ? 700 : 500,
              color: lodMode === "meso" ? "#4F46E5" : "#1E293B",
              backgroundColor: lodMode === "meso" ? "rgba(79, 70, 229, 0.08)" : "transparent",
              "&:hover": { bgcolor: "#EEF2FF" },
            }}
          >
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: lodMode === "meso" ? 700 : 600 }}>
                Meso (0.5x–1.0x)
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                Balanced view with subtle mesh
              </Typography>
            </Box>
            {lodMode === "meso" && <Check size={15} color="#4F46E5" />}
          </MenuItem>

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              zoomTo(1.25, { duration: 400 });
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              fontSize: "0.82rem",
              fontWeight: lodMode === "micro" ? 700 : 500,
              color: lodMode === "micro" ? "#2563EB" : "#1E293B",
              backgroundColor: lodMode === "micro" ? "rgba(37, 99, 235, 0.08)" : "transparent",
              "&:hover": { bgcolor: "#EFF6FF" },
            }}
          >
            <Box>
              <Typography sx={{ fontSize: "0.82rem", fontWeight: lodMode === "micro" ? 700 : 600 }}>
                Micro (&gt;1.0x)
              </Typography>
              <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                Individual node telemetry &amp; clarity
              </Typography>
            </Box>
            {lodMode === "micro" && <Check size={15} color="#2563EB" />}
          </MenuItem>

          <Divider sx={{ my: 0.75, borderColor: "#E2E8F0" }} />

          <Box sx={{ px: 1, py: 0.5 }}>
            <Typography
              variant="caption"
              sx={{
                fontSize: "0.65rem",
                fontWeight: 700,
                color: "#94A3B8",
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Inter-Group Links
            </Typography>
          </Box>

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              onExpandAllBundles?.();
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              fontSize: "0.82rem",
              fontWeight: 600,
              color: "#1E293B",
              "&:hover": { bgcolor: "#F1F5F9" },
            }}
          >
            <UnfoldVertical size={15} color="#4F46E5" />
            Expand All Lines
          </MenuItem>

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              onCollapseAllBundles?.();
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              fontSize: "0.82rem",
              fontWeight: 600,
              color: "#1E293B",
              "&:hover": { bgcolor: "#F1F5F9" },
            }}
          >
            <FoldVertical size={15} color="#4F46E5" />
            Collapse All to Bundles
          </MenuItem>

          {blocksCount > 0 && (
            <>
              <Divider sx={{ my: 0.75, borderColor: "#E2E8F0" }} />

              <Box sx={{ px: 1, py: 0.5 }}>
                <Typography
                  variant="caption"
                  sx={{
                    fontSize: "0.65rem",
                    fontWeight: 700,
                    color: "#94A3B8",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  Group Containers
                </Typography>
              </Box>

              <MenuItem
                onClick={() => {
                  setLodMenuAnchor(null);
                  onExpandAllGroups?.();
                }}
                sx={{
                  borderRadius: 1.25,
                  py: 0.75,
                  px: 1,
                  display: "flex",
                  alignItems: "center",
                  gap: 1.25,
                  fontSize: "0.82rem",
                  fontWeight: 600,
                  color: "#1E293B",
                  "&:hover": { bgcolor: "#F1F5F9" },
                }}
              >
                <Maximize2 size={15} color="#4F46E5" />
                Expand All Groups
              </MenuItem>

              <MenuItem
                onClick={() => {
                  setLodMenuAnchor(null);
                  onCollapseAllGroups?.();
                }}
                sx={{
                  borderRadius: 1.25,
                  py: 0.75,
                  px: 1,
                  display: "flex",
                  alignItems: "center",
                  gap: 1.25,
                  fontSize: "0.82rem",
                  fontWeight: 600,
                  color: "#1E293B",
                  "&:hover": { bgcolor: "#F1F5F9" },
                }}
              >
                <Minimize2 size={15} color="#4F46E5" />
                Collapse All to Super-Nodes
              </MenuItem>
            </>
          )}

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              onToggleCentralityView?.();
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              fontSize: "0.82rem",
              fontWeight: centralityViewEnabled ? 700 : 500,
              color: centralityViewEnabled ? "#047857" : "#1E293B",
              backgroundColor: centralityViewEnabled ? "rgba(16, 185, 129, 0.08)" : "transparent",
              "&:hover": { bgcolor: "#ECFDF5" },
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1.25 }}>
              <Activity size={15} color={centralityViewEnabled ? "#047857" : "#64748B"} />
              <Box>
                <Typography sx={{ fontSize: "0.82rem", fontWeight: centralityViewEnabled ? 700 : 600 }}>
                  Centrality View
                </Typography>
                <Typography variant="caption" sx={{ fontSize: "0.7rem", color: "#64748B", display: "block" }}>
                  Highlight core transit nodes &amp; links
                </Typography>
              </Box>
            </Box>
            {centralityViewEnabled && <Check size={15} color="#047857" />}
          </MenuItem>

          <Divider sx={{ my: 0.75, borderColor: "#E2E8F0" }} />

          <MenuItem
            onClick={() => {
              setLodMenuAnchor(null);
              onOpenExportDiagram?.();
            }}
            sx={{
              borderRadius: 1.25,
              py: 0.75,
              px: 1,
              display: "flex",
              alignItems: "center",
              gap: 1.25,
              fontSize: "0.82rem",
              fontWeight: 600,
              color: "#1E293B",
              "&:hover": { bgcolor: "#F1F5F9" },
            }}
          >
            <Download size={15} color="#4F46E5" />
            Export Diagram
          </MenuItem>
        </Menu>
      </Box>
    </Panel>
  );
};

export const TopologyGraph: React.FC<TopologyGraphProps> = ({
  nodes,
  links,
  nodeStatuses,
  networkState,
  networkPolicies,
  selectedTag,
  onSelectTag,
  uniqueTags,
  onSelectNode,
  onSelectLink,
  onConnectNodes,
  onNodePositionChange,
  onRefreshNode,
  refreshingNodeName,
  addBlockTrigger,
  onAddNode,
  onAddLink,
  onAddBlock,
  onCreateFullMesh,
  missingMeshLinksCount,
  displayedInternalNodesCount,
  onCreateBlockFullMesh,
}) => {
  // Block state & Advanced Interaction States
  const [blocks, setBlocks] = useState<GraphBlock[]>(getStoredBlocks);
  const [nodeBlockMap, setNodeBlockMap] = useState<Record<string, string>>(getStoredNodeBlocks);
  const [pinnedNodeNames, setPinnedNodeNames] = useState<Set<string>>(new Set());
  const [hoveredNodeName, setHoveredNodeName] = useState<string | null>(null);
  const [focusedBlockId, setFocusedBlockId] = useState<string | null>(null);
  const [hopDepth, setHopDepth] = useState<1 | 2 | 3>(1);
  const [draggingNodeId, setDraggingNodeId] = useState<string | null>(null);
  const [currentZoom, setCurrentZoom] = useState<number>(() => getStoredViewport()?.zoom ?? 1);
  const [collapsedBlockIds, setCollapsedBlockIds] = useState<Set<string>>(() => {
    try {
      const raw = localStorage.getItem("easy42_collapsed_blocks");
      return raw ? new Set(JSON.parse(raw)) : new Set();
    } catch {
      return new Set();
    }
  });
  const [expandedVirtualEdgePairs, setExpandedVirtualEdgePairs] = useState<Set<string>>(getStoredExpandedPairs);
  const [exportDiagramOpen, setExportDiagramOpen] = useState(false);
  const [centralityViewEnabled, setCentralityViewEnabled] = useState<boolean>(() => {
    try {
      return localStorage.getItem(STORAGE_KEY_CENTRALITY_VIEW) === "true";
    } catch {
      return false;
    }
  });

  // Automatically synchronize centrality view state to localStorage
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY_CENTRALITY_VIEW, String(centralityViewEnabled));
    } catch (e) {
      console.error("Failed to persist centralityViewEnabled to localStorage", e);
    }
  }, [centralityViewEnabled]);

  // Compute graph theory centrality metrics when centrality view is enabled
  const centralityResult = useMemo(() => {
    if (!centralityViewEnabled) return null;
    return calculateGraphCentrality(nodes, links, {
      metric: "betweenness",
      weighting: "weighted",
      networkPolicies,
    });
  }, [nodes, links, networkPolicies, centralityViewEnabled]);

  // Automatically synchronize expanded virtual edge pairs to browser localStorage
  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY_EXPANDED_PAIRS, JSON.stringify(Array.from(expandedVirtualEdgePairs)));
    } catch (e) {
      console.error("Failed to persist expanded virtual edge pairs to localStorage", e);
    }
  }, [expandedVirtualEdgePairs]);

  // Semantic Zoom LOD mode
  const lodMode: "macro" | "meso" | "micro" = currentZoom < 0.5 ? "macro" : currentZoom > 1.0 ? "micro" : "meso";

  // Drag tracking ref for moving member nodes alongside a block
  const blockDragState = useRef<{
    blockId: string;
    startBlockPos: { x: number; y: number };
    memberStartPositions: Map<string, { x: number; y: number }>;
  } | null>(null);

  // Ref to track whether initial fetch from server has completed
  const serverSyncInitialized = useRef(false);

  // 1. Fetch blocks from easy42 server on page load (holds cache immediately, syncs from server)
  useEffect(() => {
    let isMounted = true;
    api
      .getBlocks()
      .then((serverBlocks) => {
        if (!isMounted) return;
        if (serverBlocks && serverBlocks.length > 0) {
          setBlocks(serverBlocks);
          const newMap: Record<string, string> = {};
          serverBlocks.forEach((b) => {
            if (Array.isArray(b.nodes)) {
              b.nodes.forEach((nodeName) => {
                newMap[nodeName] = b.id;
              });
            }
          });
          setNodeBlockMap(newMap);
          try {
            localStorage.setItem(STORAGE_KEY_BLOCKS, JSON.stringify(serverBlocks));
            localStorage.setItem(STORAGE_KEY_NODE_BLOCKS, JSON.stringify(newMap));
          } catch (err) {
            console.warn("Failed to update localStorage blocks cache", err);
          }
        } else {
          // If server currently has 0 blocks, check if there are cached local blocks to migrate
          const localBlocks = getStoredBlocks();
          if (localBlocks.length > 0) {
            const localMap = getStoredNodeBlocks();
            const payload: GraphBlock[] = localBlocks.map((b) => ({
              ...b,
              nodes: Object.entries(localMap)
                .filter(([_, bId]) => bId === b.id)
                .map(([nodeName]) => nodeName),
            }));
            api.updateBlocks(payload).catch((err) => {
              console.warn("Failed to initialize server blocks from local cache", err);
            });
          }
        }
        serverSyncInitialized.current = true;
      })
      .catch((err) => {
        console.warn("Could not load blocks from server, using local cache", err);
        serverSyncInitialized.current = true;
      });

    return () => {
      isMounted = false;
    };
  }, []);

  // 2. Persist changes to server in background with debounce, and update local cache
  useEffect(() => {
    // Always update local cache immediately
    try {
      localStorage.setItem(STORAGE_KEY_BLOCKS, JSON.stringify(blocks));
      localStorage.setItem(STORAGE_KEY_NODE_BLOCKS, JSON.stringify(nodeBlockMap));
    } catch (e) {
      console.error("Failed to save blocks cache", e);
    }

    if (!serverSyncInitialized.current) {
      return;
    }

    const timer = setTimeout(() => {
      const payload: GraphBlock[] = blocks.map((b) => ({
        ...b,
        nodes: Object.entries(nodeBlockMap)
          .filter(([_, bId]) => bId === b.id)
          .map(([nodeName]) => nodeName),
      }));

      api.updateBlocks(payload).catch((err) => {
        console.error("Background persistence of blocks to server failed", err);
      });
    }, 400);

    return () => clearTimeout(timer);
  }, [blocks, nodeBlockMap]);

  // Focus clearing helper
  const handleClearFocus = useCallback(() => {
    setPinnedNodeNames(new Set());
    setFocusedBlockId(null);
    setHoveredNodeName(null);
  }, []);

  // Escape key to reset focus
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        handleClearFocus();
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [handleClearFocus]);

  // Block management actions
  const handleAddBlock = useCallback(() => {
    setBlocks((prev) => {
      const nextIndex = prev.length + 1;
      const paletteItem = BLOCK_PALETTE[(nextIndex - 1) % BLOCK_PALETTE.length];
      const offset = (prev.length % 5) * 40;
      const newBlock: GraphBlock = {
        id: `block-${Date.now()}`,
        name: `Block ${nextIndex}`,
        color: paletteItem.color,
        x: 180 + offset,
        y: 120 + offset,
        width: 440,
        height: 320,
      };
      return [...prev, newBlock];
    });
  }, []);

  // Toggle collapsing of a block into a compact super-node
  const handleToggleCollapseBlock = useCallback((blockId: string) => {
    setCollapsedBlockIds((prev) => {
      const next = new Set(prev);
      if (next.has(blockId)) {
        next.delete(blockId);
      } else {
        next.add(blockId);
      }
      try {
        localStorage.setItem("easy42_collapsed_blocks", JSON.stringify(Array.from(next)));
      } catch (e) {
        console.warn("Failed to persist collapsed blocks", e);
      }
      return next;
    });
  }, []);

  // Toggle expanding a virtual edge bundle into individual lines
  const handleToggleExpandBundle = useCallback((pairKey: string) => {
    setExpandedVirtualEdgePairs((prev) => {
      const next = new Set(prev);
      if (next.has(pairKey)) {
        next.delete(pairKey);
      } else {
        next.add(pairKey);
      }
      return next;
    });
  }, []);

  // Group-level focus
  const handleFocusBlock = useCallback((blockId: string) => {
    setFocusedBlockId((prev) => (prev === blockId ? null : blockId));
  }, []);

  // Listen for trigger from parent (e.g. Navbar menu)
  const prevTriggerRef = useRef(addBlockTrigger);
  useEffect(() => {
    if (addBlockTrigger && addBlockTrigger !== prevTriggerRef.current) {
      handleAddBlock();
    }
    prevTriggerRef.current = addBlockTrigger;
  }, [addBlockTrigger, handleAddBlock]);

  const handleToolbarAddBlock = useCallback(() => {
    if (onAddBlock) {
      onAddBlock();
    } else {
      handleAddBlock();
    }
  }, [onAddBlock, handleAddBlock]);

  const handleRenameBlock = useCallback((id: string, name: string) => {
    setBlocks((prev) => prev.map((b) => (b.id === id ? { ...b, name } : b)));
  }, []);

  const handleDeleteBlock = useCallback((id: string) => {
    setBlocks((prev) => prev.filter((b) => b.id !== id));
    setNodeBlockMap((prev) => {
      const next = { ...prev };
      for (const [nodeName, bId] of Object.entries(next)) {
        if (bId === id) {
          next[nodeName] = "";
        }
      }
      return next;
    });
  }, []);

  const handleChangeBlockColor = useCallback((id: string, color: string) => {
    setBlocks((prev) => prev.map((b) => (b.id === id ? { ...b, color } : b)));
  }, []);

  // Compute consistent node positions and block containment map
  const { nodePosMap, effectiveNodeBlockMap } = useMemo(() => {
    const total = nodes.length;
    const radius = Math.max(220, total * 60);
    const centerX = 500;
    const centerY = 350;

    const posMap = new Map<string, { x: number; y: number }>();
    nodes.forEach((node, i) => {
      const angle = (i / (total || 1)) * 2 * Math.PI - Math.PI / 2;
      const defaultX = total === 1 ? centerX : centerX + radius * Math.cos(angle);
      const defaultY = total === 1 ? centerY : centerY + radius * Math.sin(angle);
      const x = typeof node.x === "number" ? node.x : defaultX;
      const y = typeof node.y === "number" ? node.y : defaultY;
      posMap.set(node.name, { x, y });
    });

    const effMap: Record<string, string> = {};
    nodes.forEach((node) => {
      const explicitId = nodeBlockMap[node.name];
      if (explicitId && blocks.some((b) => b.id === explicitId)) {
        effMap[node.name] = explicitId;
        return;
      }
      if (explicitId === "") {
        return;
      }
      const pos = posMap.get(node.name);
      if (pos) {
        const cx = pos.x + 130;
        const cy = pos.y + 90;
        const target = blocks.find((b) => cx >= b.x && cx <= b.x + b.width && cy >= b.y && cy <= b.y + b.height);
        if (target) {
          effMap[node.name] = target.id;
        }
      }
    });

    return { nodePosMap: posMap, effectiveNodeBlockMap: effMap };
  }, [nodes, blocks, nodeBlockMap]);

  // Expand all cross-group bundles into individual line pairs
  const handleExpandAllBundles = useCallback(() => {
    const keys = new Set<string>();
    links.forEach((l) => {
      const bFrom = effectiveNodeBlockMap[l.from.name];
      const bTo = effectiveNodeBlockMap[l.to.name];
      if (bFrom && bTo && bFrom !== bTo) {
        const [b1, b2] = [bFrom, bTo].sort();
        keys.add(`${b1}---${b2}`);
      }
    });
    setExpandedVirtualEdgePairs(keys);
  }, [links, effectiveNodeBlockMap]);

  // Collapse all cross-group lines back into bundled virtual edges
  const handleCollapseAllBundles = useCallback(() => {
    setExpandedVirtualEdgePairs(new Set());
  }, []);

  // Expand all groups to show internal nodes
  const handleExpandAllGroups = useCallback(() => {
    setCollapsedBlockIds(new Set());
    try {
      localStorage.setItem("easy42_collapsed_blocks", JSON.stringify([]));
    } catch (e) {
      console.warn("Failed to persist collapsed blocks", e);
    }
  }, []);

  // Collapse all groups into compact super-nodes
  const handleCollapseAllGroups = useCallback(() => {
    const allBlockIds = new Set(blocks.map((b) => b.id));
    setCollapsedBlockIds(allBlockIds);
    try {
      localStorage.setItem("easy42_collapsed_blocks", JSON.stringify(Array.from(allBlockIds)));
    } catch (e) {
      console.warn("Failed to persist collapsed blocks", e);
    }
  }, [blocks]);

  // Local Auto-Layout ("Tidy Group") action: arranges member nodes in a clean, balanced grid
  const handleTidyBlock = useCallback(
    (blockId: string) => {
      const targetBlock = blocks.find((b) => b.id === blockId);
      if (!targetBlock) return;
      const memberNames = nodes.filter((n) => effectiveNodeBlockMap[n.name] === blockId).map((n) => n.name);
      if (memberNames.length === 0) return;

      const count = memberNames.length;
      const cols = Math.max(1, Math.ceil(Math.sqrt(count)));
      const rows = Math.ceil(count / cols);

      const cardW = 260;
      const cardH = 180;
      const padX = 40;
      const padTop = 65;
      const padBottom = 40;
      const gapX = 40;
      const gapY = 30;

      const neededW = padX * 2 + cols * cardW + (cols - 1) * gapX;
      const neededH = padTop + padBottom + rows * cardH + (rows - 1) * gapY;

      const newW = Math.max(targetBlock.width, neededW);
      const newH = Math.max(targetBlock.height, neededH);

      if (newW !== targetBlock.width || newH !== targetBlock.height) {
        setBlocks((prev) => prev.map((b) => (b.id === blockId ? { ...b, width: newW, height: newH } : b)));
      }

      const posMap = new Map<string, { x: number; y: number }>();
      memberNames.forEach((name, idx) => {
        const col = idx % cols;
        const row = Math.floor(idx / cols);
        const nx = Math.round(targetBlock.x + padX + col * (cardW + gapX));
        const ny = Math.round(targetBlock.y + padTop + row * (cardH + gapY));
        posMap.set(name, { x: nx, y: ny });
        onNodePositionChange?.(name, nx, ny);
      });

      // Update flow node positions
      // setNodes will be invoked by React Flow nodes state
    },
    [blocks, nodes, effectiveNodeBlockMap, onNodePositionChange],
  );

  const handleCreateBlockFullMesh = useCallback(
    (blockId: string) => {
      const targetBlock = blocks.find((b) => b.id === blockId);
      if (!targetBlock) return;
      const memberNames = nodes.filter((n) => effectiveNodeBlockMap[n.name] === blockId).map((n) => n.name);

      if (onCreateBlockFullMesh) {
        onCreateBlockFullMesh(targetBlock.name, memberNames);
      }
    },
    [blocks, nodes, effectiveNodeBlockMap, onCreateBlockFullMesh],
  );

  // Compute N-hop focus sets using graph traversal (BFS)
  const { activeFocusNodeSet, activeFocusEdgeSet } = useMemo(() => {
    const nodeSet = new Set<string>();
    const edgeIdSet = new Set<string>();

    if (pinnedNodeNames.size > 0) {
      const visitedNodes = new Set<string>(pinnedNodeNames);
      let currentLevel = Array.from(pinnedNodeNames);
      pinnedNodeNames.forEach((n) => nodeSet.add(n));

      const maxHops = hopDepth === 3 ? 999 : hopDepth;

      for (let hop = 0; hop < maxHops; hop++) {
        if (currentLevel.length === 0) break;
        const nextLevel: string[] = [];

        currentLevel.forEach((nodeName) => {
          links.forEach((l) => {
            const edgeId = `link-${l.from.name}-${l.from.interface}-${l.to.name}-${l.to.interface}`;
            if (l.from.name === nodeName) {
              edgeIdSet.add(edgeId);
              if (!visitedNodes.has(l.to.name)) {
                visitedNodes.add(l.to.name);
                nodeSet.add(l.to.name);
                nextLevel.push(l.to.name);
              }
            } else if (l.to.name === nodeName) {
              edgeIdSet.add(edgeId);
              if (!visitedNodes.has(l.from.name)) {
                visitedNodes.add(l.from.name);
                nodeSet.add(l.from.name);
                nextLevel.push(l.from.name);
              }
            }
          });
        });

        currentLevel = nextLevel;
      }

      return { activeFocusNodeSet: nodeSet, activeFocusEdgeSet: edgeIdSet };
    }

    if (hoveredNodeName) {
      nodeSet.add(hoveredNodeName);
      links.forEach((l) => {
        const edgeId = `link-${l.from.name}-${l.from.interface}-${l.to.name}-${l.to.interface}`;
        if (l.from.name === hoveredNodeName) {
          nodeSet.add(l.to.name);
          edgeIdSet.add(edgeId);
        } else if (l.to.name === hoveredNodeName) {
          nodeSet.add(l.from.name);
          edgeIdSet.add(edgeId);
        }
      });
      return { activeFocusNodeSet: nodeSet, activeFocusEdgeSet: edgeIdSet };
    }

    if (focusedBlockId) {
      nodes.forEach((n) => {
        if (effectiveNodeBlockMap[n.name] === focusedBlockId) {
          nodeSet.add(n.name);
        }
      });
      links.forEach((l) => {
        const edgeId = `link-${l.from.name}-${l.from.interface}-${l.to.name}-${l.to.interface}`;
        if (nodeSet.has(l.from.name) && nodeSet.has(l.to.name)) {
          edgeIdSet.add(edgeId);
        }
      });
      return { activeFocusNodeSet: nodeSet, activeFocusEdgeSet: edgeIdSet };
    }

    return { activeFocusNodeSet: null, activeFocusEdgeSet: null };
  }, [pinnedNodeNames, hopDepth, hoveredNodeName, focusedBlockId, links, nodes, effectiveNodeBlockMap]);

  // Compute node link statistics for stub indicators
  const nodeLinksCountMap = useMemo(() => {
    const countMap = new Map<string, { total: number; intra: number; inter: number }>();
    nodes.forEach((n) => {
      const nBlock = effectiveNodeBlockMap[n.name];
      let total = 0;
      let intra = 0;
      let inter = 0;
      links.forEach((l) => {
        if (l.from.name === n.name || l.to.name === n.name) {
          total++;
          const otherName = l.from.name === n.name ? l.to.name : l.from.name;
          const otherBlock = effectiveNodeBlockMap[otherName];
          if (nBlock && otherBlock && nBlock === otherBlock) {
            intra++;
          } else {
            inter++;
          }
        }
      });
      countMap.set(n.name, { total, intra, inter });
    });
    return countMap;
  }, [nodes, links, effectiveNodeBlockMap]);

  // Initial nodes: combine background Block nodes and NodeCard custom nodes
  const initialNodes: FlowNode[] = useMemo(() => {
    // 0. Compute health for all nodes
    const nodeHealthMap: Record<
      string,
      {
        isHealthy: boolean;
        upLinks: number;
        downLinks: number;
        totalLinks: number;
        reason?: string;
      }
    > = {};

    nodes.forEach((node) => {
      const isOnline = Boolean(node.is_external || nodeStatuses[node.name]?.connected);
      const nodeLinks = links.filter((l) => l.from.name === node.name || l.to.name === node.name);

      let upLinks = 0;
      let downLinks = 0;
      let unknownLinks = 0;

      nodeLinks.forEach((l) => {
        const state = getLinkWorkingState(l, networkState, nodeStatuses);
        if (state === "working") {
          upLinks++;
        } else if (state === "not_working") {
          downLinks++;
        } else {
          unknownLinks++;
        }
      });

      let isHealthy = isOnline && nodeLinks.length > 0 && downLinks === 0 && upLinks === nodeLinks.length;
      let reason = "All links active & normal";
      if (!isOnline) {
        isHealthy = false;
        reason = "Node is offline";
      } else if (downLinks > 0) {
        isHealthy = false;
        reason = `${downLinks} of ${nodeLinks.length} link(s) down`;
      } else if (nodeLinks.length === 0) {
        isHealthy = false;
        reason = "No links configured";
      } else if (unknownLinks > 0) {
        isHealthy = false;
        reason = `${unknownLinks} link(s) pending/unknown`;
      }

      nodeHealthMap[node.name] = {
        isHealthy,
        upLinks,
        downLinks,
        totalLinks: nodeLinks.length,
        reason,
      };
    });

    // 1. Compute full-mesh status for each block
    const blockFullMeshStatusMap = new Map<string, boolean>();
    blocks.forEach((block) => {
      const memberNames = nodes.filter((n) => effectiveNodeBlockMap[n.name] === block.id).map((n) => n.name);

      if (memberNames.length < 2) {
        blockFullMeshStatusMap.set(block.id, false);
        return;
      }

      const hasIntraLink = (u: string, v: string) => {
        const uClean = u.trim().toLowerCase();
        const vClean = v.trim().toLowerCase();
        return links.some((l) => {
          const fromName = l.from?.name?.trim().toLowerCase();
          const toName = l.to?.name?.trim().toLowerCase();
          return (fromName === uClean && toName === vClean) || (fromName === vClean && toName === uClean);
        });
      };

      const isFullMesh = memberNames.every((u, i) => memberNames.slice(i + 1).every((v) => hasIntraLink(u, v)));
      blockFullMeshStatusMap.set(block.id, isFullMesh);
    });

    // 2. Block nodes (zIndex: 0 with pointer-events handling so they render beneath node cards)
    const blockFlowNodes: FlowNode[] = blocks.map((block) => {
      const memberNodes = nodes.filter((n) => effectiveNodeBlockMap[n.name] === block.id);
      const memberNamesSet = new Set(memberNodes.map((n) => n.name));
      const intraLinks = links.filter((l) => memberNamesSet.has(l.from.name) && memberNamesSet.has(l.to.name));
      const brokenLinks = intraLinks.filter((l) => {
        return getLinkWorkingState(l, networkState, nodeStatuses) === "not_working";
      });

      const isFullMesh = blockFullMeshStatusMap.get(block.id) ?? false;
      const fullMeshCount = isFullMesh ? memberNodes.length : 0;
      const healthyCount = memberNodes.filter((n) => nodeHealthMap[n.name]?.isHealthy).length;
      const isCollapsed = collapsedBlockIds.has(block.id);
      const isGroupFocused = focusedBlockId === block.id;

      return {
        id: block.id,
        type: "blockGroup",
        position: { x: block.x, y: block.y },
        style: isCollapsed
          ? { width: 310, height: 76, zIndex: 0 }
          : { width: block.width, height: block.height, zIndex: 0 },
        initialWidth: isCollapsed ? 310 : block.width,
        initialHeight: isCollapsed ? 76 : block.height,
        width: isCollapsed ? 310 : block.width,
        height: isCollapsed ? 76 : block.height,
        selectable: true,
        draggable: true,
        dragHandle: ".block-header",
        data: {
          block,
          memberCount: memberNodes.length,
          hiddenLinkCount: intraLinks.length,
          brokenLinkCount: brokenLinks.length,
          isFullMesh,
          fullMeshCount,
          healthyCount,
          isCollapsed,
          isGroupFocused,
          onRenameBlock: handleRenameBlock,
          onDeleteBlock: handleDeleteBlock,
          onChangeColor: handleChangeBlockColor,
          onCreateBlockFullMesh: handleCreateBlockFullMesh,
          onToggleCollapse: handleToggleCollapseBlock,
          onTidyBlock: handleTidyBlock,
          onFocusBlock: handleFocusBlock,
        } as unknown as Record<string, unknown>,
      };
    });

    // 3. Custom Node cards
    const customFlowNodes: FlowNode[] = nodes.map((node) => {
      const pos = nodePosMap.get(node.name) || { x: 500, y: 350 };
      const assignedBlockId = effectiveNodeBlockMap[node.name];
      const assignedBlock = blocks.find((b) => b.id === assignedBlockId);
      const inBlock = Boolean(assignedBlock);
      const isBlockFullMesh = inBlock && Boolean(blockFullMeshStatusMap.get(assignedBlockId!));
      const health = nodeHealthMap[node.name];
      const isBlockCollapsed = Boolean(assignedBlockId && collapsedBlockIds.has(assignedBlockId));
      const linkCounts = nodeLinksCountMap.get(node.name) || { total: 0, intra: 0, inter: 0 };
      const isHighlighted = activeFocusNodeSet ? activeFocusNodeSet.has(node.name) : false;
      const isDimmed = activeFocusNodeSet ? !activeFocusNodeSet.has(node.name) : false;

      return {
        id: node.name,
        type: "customNode",
        position: pos,
        hidden: isBlockCollapsed,
        style: {
          zIndex: isHighlighted ? 30 : 10,
          width: lodMode === "macro" ? 155 : 260,
        },
        initialWidth: lodMode === "macro" ? 155 : 260,
        initialHeight: lodMode === "macro" ? 44 : 180,
        width: lodMode === "macro" ? 155 : 260,
        height: lodMode === "macro" ? 44 : 180,
        data: {
          node,
          status: nodeStatuses[node.name],
          blockName: assignedBlock?.name,
          blockColor: assignedBlock?.color,
          inBlock,
          isBlockFullMesh,
          isHealthy: health?.isHealthy ?? false,
          healthDetails: health,
          isFocused: pinnedNodeNames.has(node.name),
          isHighlighted,
          isDimmed,
          lodMode,
          totalLinksCount: linkCounts.total,
          intraBlockLinksCount: linkCounts.intra,
          interBlockLinksCount: linkCounts.inter,
          centralityInfo: centralityResult?.nodes.get(node.name),
          isCentralityActive: centralityViewEnabled,
          onSelect: onSelectNode,
          onRefreshNode,
          refreshingNodeName,
        } as unknown as Record<string, unknown>,
      };
    });

    return [...blockFlowNodes, ...customFlowNodes];
  }, [
    blocks,
    nodes,
    links,
    effectiveNodeBlockMap,
    nodePosMap,
    nodeStatuses,
    networkState,
    collapsedBlockIds,
    focusedBlockId,
    pinnedNodeNames,
    activeFocusNodeSet,
    lodMode,
    nodeLinksCountMap,
    centralityResult,
    centralityViewEnabled,
    onSelectNode,
    onRefreshNode,
    refreshingNodeName,
    handleRenameBlock,
    handleDeleteBlock,
    handleChangeBlockColor,
    handleCreateBlockFullMesh,
    handleToggleCollapseBlock,
    handleTidyBlock,
    handleFocusBlock,
  ]);

  // Convert links to React Flow edges with filtering rules
  const initialEdges: Edge[] = useMemo(() => {
    // 1. Group links between distinct block pairs to render one virtual line per pair
    const blockPairLinksMap = new Map<string, { block1Id: string; block2Id: string; links: Link[] }>();

    links.forEach((l) => {
      const bFrom = effectiveNodeBlockMap[l.from.name];
      const bTo = effectiveNodeBlockMap[l.to.name];
      if (bFrom && bTo && bFrom !== bTo) {
        const [b1, b2] = [bFrom, bTo].sort();
        const pairKey = `${b1}---${b2}`;
        if (!blockPairLinksMap.has(pairKey)) {
          blockPairLinksMap.set(pairKey, { block1Id: b1, block2Id: b2, links: [] });
        }
        blockPairLinksMap.get(pairKey)!.links.push(l);
      }
    });

    // Create exactly one virtual edge per pair of blocks with cross-block links
    const blockVirtualEdges: Edge[] = [];
    blockPairLinksMap.forEach(({ block1Id, block2Id, links: pairLinks }, pairKey) => {
      const b1 = blocks.find((b) => b.id === block1Id);
      const b2 = blocks.find((b) => b.id === block2Id);
      if (!b1 || !b2) return;

      let workingCount = 0;
      let downCount = 0;
      let unknownCount = 0;

      pairLinks.forEach((l) => {
        const state = getLinkWorkingState(l, networkState, nodeStatuses);
        if (state === "working") {
          workingCount++;
        } else if (state === "not_working") {
          downCount++;
        } else {
          unknownCount++;
        }
      });

      const isExpanded = expandedVirtualEdgePairs.has(pairKey);

      blockVirtualEdges.push({
        id: `block-edge-${pairKey}`,
        source: b1.id,
        target: b2.id,
        sourceHandle: "block-source",
        targetHandle: "block-target",
        type: "blockVirtualEdge",
        zIndex: 1,
        data: {
          pairKey,
          sourceBlock: b1,
          targetBlock: b2,
          links: pairLinks,
          workingCount,
          downCount,
          unknownCount,
          isExpanded,
          onToggleExpandBundle: handleToggleExpandBundle,
          onSelectLink,
        } as unknown as Record<string, unknown>,
      });
    });

    // 2. Filter node-to-node links
    const visibleLinks = links.filter((link) => {
      const fromBlock = effectiveNodeBlockMap[link.from.name];
      const toBlock = effectiveNodeBlockMap[link.to.name];

      // If either block is collapsed, intra-links within it are hidden
      if (fromBlock && collapsedBlockIds.has(fromBlock) && fromBlock === toBlock) {
        return false;
      }

      // Ghost link when dragging: always show
      if (draggingNodeId && (link.from.name === draggingNodeId || link.to.name === draggingNodeId)) {
        return true;
      }

      const edgeId = `link-${link.from.name}-${link.from.interface}-${link.to.name}-${link.to.interface}`;

      // If in active focus, show it!
      if (activeFocusEdgeSet && activeFocusEdgeSet.has(edgeId)) {
        return true;
      }

      // Global scope node (doesn't belong to any block) still always displays all links
      const isGlobalInvolved = !fromBlock || !toBlock;
      if (isGlobalInvolved) {
        return true;
      }

      // Intra-block links (same block): RENDERED BY DEFAULT as subtle background texture!
      if (fromBlock === toBlock) {
        return true;
      }

      // Inter-block links (different blocks):
      const [b1, b2] = [fromBlock, toBlock].sort();
      const pairKey = `${b1}---${b2}`;
      if (expandedVirtualEdgePairs.has(pairKey)) {
        return true;
      }

      // Otherwise, virtual edge is displayed instead
      return false;
    });

    const pairGroups: Record<string, Link[]> = {};
    visibleLinks.forEach((l) => {
      const key = [l.from.name, l.to.name].sort().join("---");
      if (!pairGroups[key]) pairGroups[key] = [];
      pairGroups[key].push(l);
    });

    const visibleNodeEdges: Edge[] = visibleLinks.map((link) => {
      const pairKey = [link.from.name, link.to.name].sort().join("---");
      const group = pairGroups[pairKey] || [link];
      const linkIndexInPair = group.indexOf(link);
      const totalLinksInPair = group.length;

      const edgeId = `link-${link.from.name}-${link.from.interface}-${link.to.name}-${link.to.interface}`;

      const fromIface = networkState?.nodes?.[link.from.name]?.interfaces?.[link.from.interface];
      const toIface = networkState?.nodes?.[link.to.name]?.interfaces?.[link.to.interface];

      const workingState = getLinkWorkingState(link, networkState, nodeStatuses);
      let latestHandshake: string | undefined = undefined;
      let rxBytes = 0;
      let txBytes = 0;

      const hsFrom = fromIface?.latest_handshake ? new Date(fromIface.latest_handshake).getTime() : 0;
      const hsTo = toIface?.latest_handshake ? new Date(toIface.latest_handshake).getTime() : 0;
      if (hsFrom > 0 || hsTo > 0) {
        latestHandshake = hsFrom >= hsTo ? fromIface?.latest_handshake : toIface?.latest_handshake;
      }

      rxBytes = (fromIface?.transfer_rx_bytes || 0) + (toIface?.transfer_rx_bytes || 0);
      txBytes = (fromIface?.transfer_tx_bytes || 0) + (toIface?.transfer_tx_bytes || 0);

      const fromNode = nodes.find((n) => n.name === link.from.name);
      const toNode = nodes.find((n) => n.name === link.to.name);
      const fromIsExternal = Boolean(fromNode?.is_external);
      const toIsExternal = Boolean(toNode?.is_external);
      const isExternal = Boolean(fromIsExternal || toIsExternal);
      const fromIsRemoteExternal = toIsExternal;
      const toIsRemoteExternal = fromIsExternal;
      const fromCost = getEffectiveLinkCost(link.from, fromIsRemoteExternal, networkPolicies);
      const toCost = getEffectiveLinkCost(link.to, toIsRemoteExternal, networkPolicies);

      const fromBlock = effectiveNodeBlockMap[link.from.name];
      const toBlock = effectiveNodeBlockMap[link.to.name];
      const isIntraBlock = Boolean(fromBlock && toBlock && fromBlock === toBlock);
      const isInterBlock = Boolean(fromBlock && toBlock && fromBlock !== toBlock);
      const pairBlockKey = isInterBlock ? [fromBlock, toBlock].sort().join("---") : "";
      const isExpandedInterBlock = Boolean(isInterBlock && expandedVirtualEdgePairs.has(pairBlockKey));
      const isGhostDrag = Boolean(
        draggingNodeId && (link.from.name === draggingNodeId || link.to.name === draggingNodeId),
      );
      const isHighlighted = Boolean(activeFocusEdgeSet?.has(edgeId) || isGhostDrag);
      const isDimmed = Boolean(activeFocusEdgeSet && !activeFocusEdgeSet.has(edgeId) && !isGhostDrag);
      const isSubtle = Boolean((isIntraBlock || isExpandedInterBlock) && !isHighlighted && !isGhostDrag);
      const linkCentrality = centralityResult?.links.get(edgeId);
      const isCoreLink = Boolean(centralityViewEnabled && linkCentrality?.isCore);

      return {
        id: edgeId,
        source: link.from.name,
        target: link.to.name,
        type: "customEdge",
        zIndex: isHighlighted || isGhostDrag ? 100 : isCoreLink ? 20 : isSubtle ? 2 : 5,
        data: {
          link,
          workingState,
          latestHandshake,
          transferRxBytes: rxBytes,
          transferTxBytes: txBytes,
          isExternal,
          fromIsExternal,
          toIsExternal,
          linkIndexInPair,
          totalLinksInPair,
          fromCost,
          toCost,
          isIntraBlock,
          isInterBlock,
          isExpandedInterBlock,
          isSubtle,
          isGhostDrag,
          isHighlighted,
          isDimmed,
          zoomLevel: currentZoom,
          centralityInfo: linkCentrality,
          isCentralityActive: centralityViewEnabled,
          onSelect: onSelectLink,
        } as unknown as Record<string, unknown>,
      };
    });

    return [...blockVirtualEdges, ...visibleNodeEdges];
  }, [
    nodes,
    links,
    blocks,
    networkState,
    networkPolicies,
    onSelectLink,
    effectiveNodeBlockMap,
    collapsedBlockIds,
    expandedVirtualEdgePairs,
    activeFocusEdgeSet,
    draggingNodeId,
    currentZoom,
    centralityResult,
    centralityViewEnabled,
    handleToggleExpandBundle,
    nodeStatuses,
  ]);

  const [flowNodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [flowEdges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync state when props change while preserving current node positions and dimensions
  React.useEffect(() => {
    setNodes((currentNodes) => {
      const currentMap = new Map(currentNodes.map((n) => [n.id, n]));
      return initialNodes.map((n) => {
        const existing = currentMap.get(n.id);
        const pos = existing?.position ?? n.position;

        return {
          ...n,
          position: pos,
          measured: (existing as any)?.measured || (n as any).measured,
          width: (existing as any)?.width ?? n.width,
          height: (existing as any)?.height ?? n.height,
        };
      });
    });
  }, [initialNodes, setNodes]);

  React.useEffect(() => {
    setEdges(initialEdges);
  }, [initialEdges, setEdges]);

  // Handle dimensions change when resizing blocks
  const handleNodesChange = useCallback(
    (changes: NodeChange[]) => {
      onNodesChange(changes);

      for (const c of changes) {
        if (c.type === "dimensions" && c.dimensions) {
          const dim = c.dimensions;
          setBlocks((prev) =>
            prev.map((b) =>
              b.id === c.id
                ? {
                    ...b,
                    width: Math.max(260, Math.round(dim.width)),
                    height: Math.max(180, Math.round(dim.height)),
                  }
                : b,
            ),
          );
        }
      }
    },
    [onNodesChange],
  );

  // Drag start: record initial positions of block and its member nodes
  const handleNodeDragStart: OnNodeDrag = useCallback(
    (_event, node) => {
      if (node.type === "customNode") {
        setDraggingNodeId(node.id);
      } else if (node.type === "blockGroup") {
        const memberPositions = new Map<string, { x: number; y: number }>();
        for (const fn of flowNodes) {
          if (nodeBlockMap[fn.id] === node.id) {
            memberPositions.set(fn.id, { x: fn.position.x, y: fn.position.y });
          }
        }
        blockDragState.current = {
          blockId: node.id,
          startBlockPos: { x: node.position.x, y: node.position.y },
          memberStartPositions: memberPositions,
        };
      }
    },
    [flowNodes, nodeBlockMap],
  );

  // Live drag: move member nodes together with their parent block
  const handleNodeDrag: OnNodeDrag = useCallback(
    (_event, node) => {
      if (node.type === "blockGroup" && blockDragState.current && blockDragState.current.blockId === node.id) {
        const dx = node.position.x - blockDragState.current.startBlockPos.x;
        const dy = node.position.y - blockDragState.current.startBlockPos.y;
        const startMembers = blockDragState.current.memberStartPositions;

        if (startMembers.size > 0) {
          setNodes((currentNodes) =>
            currentNodes.map((n) => {
              const startPos = startMembers.get(n.id);
              if (startPos) {
                return {
                  ...n,
                  position: {
                    x: Math.round(startPos.x + dx),
                    y: Math.round(startPos.y + dy),
                  },
                };
              }
              return n;
            }),
          );
        }
      }
    },
    [setNodes],
  );

  // Drag stop: assign dropped nodes to blocks, or save block positions and member coordinates
  const handleNodeDragStop: OnNodeDrag = useCallback(
    (_event, node, draggedNodes) => {
      setDraggingNodeId(null);

      if (node.type === "blockGroup") {
        const finalX = Math.round(node.position.x);
        const finalY = Math.round(node.position.y);

        const updatedBlocks = blocks.map((b) => (b.id === node.id ? { ...b, x: finalX, y: finalY } : b));
        setBlocks(updatedBlocks);

        if (blockDragState.current && blockDragState.current.blockId === node.id) {
          const dx = finalX - blockDragState.current.startBlockPos.x;
          const dy = finalY - blockDragState.current.startBlockPos.y;
          const startMembers = blockDragState.current.memberStartPositions;

          startMembers.forEach((startPos, memberId) => {
            const newX = Math.round(startPos.x + dx);
            const newY = Math.round(startPos.y + dy);
            onNodePositionChange?.(memberId, newX, newY);
          });
        }
        blockDragState.current = null;

        // Synchronize nodeBlockMap for all custom nodes with the updated blocks
        setNodeBlockMap((prev) => {
          const next = { ...prev };
          let changed = false;
          flowNodes.forEach((fn) => {
            if (fn.type !== "customNode") return;
            const cx = fn.position.x + 130;
            const cy = fn.position.y + 90;
            const target = updatedBlocks.find(
              (b) => cx >= b.x && cx <= b.x + b.width && cy >= b.y && cy <= b.y + b.height,
            );
            const current = next[fn.id];
            if (target && current !== target.id) {
              next[fn.id] = target.id;
              changed = true;
            } else if (!target && current === node.id) {
              next[fn.id] = "";
              changed = true;
            }
          });
          return changed ? next : prev;
        });
        return;
      }

      // Regular device node dropped
      const list = draggedNodes && draggedNodes.length > 0 ? draggedNodes : [node];
      for (const n of list) {
        const x = Math.round(n.position.x);
        const y = Math.round(n.position.y);
        onNodePositionChange?.(n.id, x, y);

        // Check if node center falls within any block bounding box
        const nodeCenterX = x + 130;
        const nodeCenterY = y + 90;
        const targetBlock = blocks.find(
          (b) =>
            nodeCenterX >= b.x && nodeCenterX <= b.x + b.width && nodeCenterY >= b.y && nodeCenterY <= b.y + b.height,
        );

        setNodeBlockMap((prev) => {
          const currentBlockId = prev[n.id];
          if (targetBlock) {
            if (currentBlockId === targetBlock.id) return prev;
            return { ...prev, [n.id]: targetBlock.id };
          } else {
            if (currentBlockId === "") return prev;
            return { ...prev, [n.id]: "" };
          }
        });
      }
    },
    [blocks, flowNodes, onNodePositionChange],
  );

  const initialViewport = useMemo(() => getStoredViewport(), []);

  const handleMoveEnd: OnMoveEnd = useCallback((_event, viewport) => {
    setCurrentZoom(viewport.zoom);
    try {
      localStorage.setItem(STORAGE_KEY_VIEWPORT, JSON.stringify(viewport));
      localStorage.setItem(STORAGE_KEY_ZOOM, JSON.stringify(viewport.zoom));
    } catch (e) {
      console.error("Failed to save viewport to localStorage", e);
    }
  }, []);

  const isValidConnection = useCallback(
    (connection: Connection | Edge) => {
      if (!connection.source || !connection.target || connection.source === connection.target) {
        return false;
      }
      const srcNode = nodes.find((n) => n.name === connection.source);
      const tgtNode = nodes.find((n) => n.name === connection.target);
      if (srcNode?.is_external && tgtNode?.is_external) {
        return false;
      }
      return true;
    },
    [nodes],
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      if (connection.source && connection.target && connection.source !== connection.target) {
        const srcNode = nodes.find((n) => n.name === connection.source);
        const tgtNode = nodes.find((n) => n.name === connection.target);
        if (srcNode?.is_external && tgtNode?.is_external) {
          return;
        }
        onConnectNodes(connection.source, connection.target);
      }
      setEdges((eds) => addEdge(connection, eds));
    },
    [nodes, onConnectNodes, setEdges],
  );

  // Hidden/bundled links count
  const hiddenLinksCount = useMemo(() => {
    return links.filter((link) => {
      const fromBlock = effectiveNodeBlockMap[link.from.name];
      const toBlock = effectiveNodeBlockMap[link.to.name];

      // If either node is not in a block, it's global and always shown
      if (!fromBlock || !toBlock) return false;

      // If in collapsed block, intra-links are hidden
      if (fromBlock === toBlock) {
        return collapsedBlockIds.has(fromBlock);
      }

      // Inter-block link: bundled into virtual edge unless expanded
      const [b1, b2] = [fromBlock, toBlock].sort();
      const pairKey = `${b1}---${b2}`;
      return !expandedVirtualEdgePairs.has(pairKey);
    }).length;
  }, [links, effectiveNodeBlockMap, collapsedBlockIds, expandedVirtualEdgePairs]);

  const focusedBlock = blocks.find((b) => b.id === focusedBlockId);
  const focusedBlockName = focusedBlock?.name ?? null;

  const handleNodeMouseEnter = useCallback((_event: React.MouseEvent, flowNode: FlowNode) => {
    if (flowNode.type === "customNode") {
      setHoveredNodeName(flowNode.id);
    }
  }, []);

  const handleNodeMouseLeave = useCallback((_event: React.MouseEvent, flowNode: FlowNode) => {
    if (flowNode.type === "customNode") {
      setHoveredNodeName((prev) => (prev === flowNode.id ? null : prev));
    }
  }, []);

  const handleNodeClick = useCallback(
    (event: React.MouseEvent, flowNode: FlowNode) => {
      if (flowNode.type === "customNode") {
        if (event.shiftKey) {
          setPinnedNodeNames((prev) => {
            const next = new Set(prev);
            if (next.has(flowNode.id)) {
              next.delete(flowNode.id);
            } else {
              next.add(flowNode.id);
            }
            return next;
          });
        } else {
          setPinnedNodeNames((prev) => {
            if (prev.size === 1 && prev.has(flowNode.id)) {
              return new Set();
            }
            return new Set([flowNode.id]);
          });
        }

        const n = (flowNode.data as any)?.node;
        if (n) {
          onSelectNode(n);
        }
      }
    },
    [onSelectNode],
  );

  const handlePaneClick = useCallback(() => {
    handleClearFocus();
  }, [handleClearFocus]);

  return (
    <Box sx={{ width: "100%", height: "calc(100vh - 64px)", position: "relative", backgroundColor: "#F8FAFC" }}>
      {nodes.length === 0 ? (
        <Box
          sx={{
            position: "absolute",
            inset: 0,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 2,
            zIndex: 5,
            pointerEvents: "none",
          }}
        >
          <Box
            sx={{
              width: 64,
              height: 64,
              borderRadius: 4,
              backgroundColor: "rgba(79, 70, 229, 0.08)",
              border: "1px dashed rgba(79, 70, 229, 0.3)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Network size={32} color="#4F46E5" />
          </Box>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
            {selectedTag && selectedTag !== "All" ? `No Nodes with tag "${selectedTag}"` : "No Nodes in Mesh"}
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 360, textAlign: "center" }}>
            {selectedTag && selectedTag !== "All"
              ? "Try selecting a different tag filter from the toolbar above."
              : 'Click "Add Node" above to discover and connect your Linux servers via WireGuard.'}
          </Typography>
        </Box>
      ) : null}

      <ReactFlow
        minZoom={0.1} // Allows zooming out significantly further than the default 0.5
        maxZoom={3.0} // Allows zooming in closer than the default 2.0
        nodes={flowNodes}
        edges={flowEdges}
        onNodesChange={handleNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeDragStart={handleNodeDragStart}
        onNodeDrag={handleNodeDrag}
        onNodeDragStop={handleNodeDragStop}
        onNodeMouseEnter={handleNodeMouseEnter}
        onNodeMouseLeave={handleNodeMouseLeave}
        onNodeClick={handleNodeClick}
        onPaneClick={handlePaneClick}
        onEdgeClick={(_event, edge) => {
          const edgeData = edge.data as unknown as { link?: Link };
          if (edgeData?.link) {
            onSelectLink(edgeData.link);
          }
        }}
        onConnect={onConnect}
        isValidConnection={isValidConnection}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        defaultViewport={initialViewport}
        fitView={!initialViewport}
        onMoveEnd={handleMoveEnd}
        attributionPosition="bottom-left"
      >
        {/* Consolidated Floating Toolbar */}
        <FloatingToolbar
          nodes={nodes}
          onSelectNode={onSelectNode}
          onFocusNode={(nodeName) => setPinnedNodeNames(new Set([nodeName]))}
          onAddNode={onAddNode}
          onAddLink={onAddLink}
          onAddBlock={handleToolbarAddBlock}
          onCreateFullMesh={onCreateFullMesh}
          missingMeshLinksCount={missingMeshLinksCount}
          displayedInternalNodesCount={displayedInternalNodesCount}
          selectedTag={selectedTag}
          onSelectTag={onSelectTag}
          uniqueTags={uniqueTags}
          pinnedNodeNames={pinnedNodeNames}
          focusedBlockName={focusedBlockName}
          hopDepth={hopDepth}
          onSetHopDepth={setHopDepth}
          onClearFocus={handleClearFocus}
          blocksCount={blocks.length}
          hiddenLinksCount={hiddenLinksCount}
          lodMode={lodMode}
          onExpandAllBundles={handleExpandAllBundles}
          onCollapseAllBundles={handleCollapseAllBundles}
          onExpandAllGroups={handleExpandAllGroups}
          onCollapseAllGroups={handleCollapseAllGroups}
          onOpenExportDiagram={() => setExportDiagramOpen(true)}
          centralityViewEnabled={centralityViewEnabled}
          onToggleCentralityView={() => setCentralityViewEnabled((v) => !v)}
        />

        <Background variant={BackgroundVariant.Dots} gap={20} size={1.2} color="#CBD5E1" />
        <Controls />
        <MiniMap
          nodeStrokeColor="#4F46E5"
          nodeColor="#E2E8F0"
          nodeBorderRadius={4}
          maskColor="rgba(248, 250, 252, 0.7)"
          pannable
          zoomable
        />
      </ReactFlow>

      {/* Standalone Export Diagram Modal */}
      <ExportDiagramModal
        open={exportDiagramOpen}
        onClose={() => setExportDiagramOpen(false)}
        nodes={nodes}
        links={links}
        blocks={blocks}
        nodeBlockMap={effectiveNodeBlockMap}
        networkPolicies={networkPolicies}
      />
    </Box>
  );
};
