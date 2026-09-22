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
import { Box, Typography, Button, Chip, Tooltip } from "@mui/material";
import { Network, Layers, EyeOff, X, Eye, Maximize2 } from "lucide-react";
import { NodeCard } from "./NodeCard";
import { CustomEdge } from "./CustomEdge";
import { BlockNode, BLOCK_PALETTE } from "./BlockNode";
import { BlockVirtualEdge } from "./BlockVirtualEdge";
import { Node, Link, NodeStatus, NetworkState, GraphBlock } from "../../types/api";
import { api } from "../../api/client";

const STORAGE_KEY_VIEWPORT = "easy42_graph_viewport";
const STORAGE_KEY_ZOOM = "easy42_graph_zoom";
const STORAGE_KEY_BLOCKS = "easy42_graph_blocks_v1";
const STORAGE_KEY_NODE_BLOCKS = "easy42_node_blocks_v1";

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

/**
 * Helper to determine whether a link is manual (BGP-only / non-WireGuard data plane).
 */
export function isManualLink(
  link?: {
    type?: string;
    from?: { type?: string; interface?: string };
    to?: { type?: string; interface?: string };
    tags?: string[];
  } | null,
): boolean {
  if (!link) return false;
  // 1. Explicit link type
  if (link.type && link.type.toLowerCase().trim() === "manual") return true;
  // 2. Explicit endpoint type
  if (link.from?.type && link.from.type.toLowerCase().trim() === "manual") return true;
  if (link.to?.type && link.to.type.toLowerCase().trim() === "manual") return true;
  // 3. Tags containing "manual"
  if (link.tags && link.tags.some((t) => t.toLowerCase().trim() === "manual")) return true;
  // 4. In easy42, all WireGuard interfaces start with "wg". If an interface doesn't start with "wg", it's a manual interface.
  if (link.from?.interface && !link.from.interface.toLowerCase().startsWith("wg")) return true;
  if (link.to?.interface && !link.to.interface.toLowerCase().startsWith("wg")) return true;

  return false;
}

/**
 * Helper to determine whether a link is a WireGuard link (managed data plane).
 * Manual links and non-WireGuard links are excluded.
 */
export function isWireGuardLink(
  link?: {
    type?: string;
    from?: { type?: string; interface?: string };
    to?: { type?: string; interface?: string };
    tags?: string[];
  } | null,
): boolean {
  if (!link) return false;
  if (isManualLink(link)) return false;
  const lType = link.type?.toLowerCase().trim();
  if (lType && lType !== "wireguard") return false;
  const fromType = link.from?.type?.toLowerCase().trim();
  if (fromType && fromType !== "wireguard") return false;
  const toType = link.to?.type?.toLowerCase().trim();
  if (toType && toType !== "wireguard") return false;
  return true;
}

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
    // Manual links do not count towards WireGuard full-mesh
    if (isManualLink(l)) return;

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
 * Determines the working state of a link ("working", "not_working", or "unknown").
 * For WireGuard links, uses the interface's recorded working_state in networkState.
 * For manual links (BGP-only data plane), easy42 does not manage WireGuard handshakes;
 * if both participating nodes are online and reachable, the manual link is considered working.
 */
export function getLinkWorkingState(
  link: Link,
  networkState?: NetworkState | null,
  nodeStatuses?: Record<string, NodeStatus>,
): "working" | "not_working" | "unknown" {
  const isManual = isManualLink(link);
  const fromIface = networkState?.nodes?.[link.from.name]?.interfaces?.[link.from.interface];
  const toIface = networkState?.nodes?.[link.to.name]?.interfaces?.[link.to.interface];

  if (fromIface?.working_state === "working" || toIface?.working_state === "working") {
    return "working";
  }
  if (fromIface?.working_state === "not_working" || toIface?.working_state === "not_working") {
    return "not_working";
  }
  if (isManual) {
    const fromOnline = nodeStatuses?.[link.from.name] ? nodeStatuses[link.from.name].connected : true;
    const toOnline = nodeStatuses?.[link.to.name] ? nodeStatuses[link.to.name].connected : true;
    return fromOnline && toOnline ? "working" : "not_working";
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
  selectedTag?: string;
  onSelectNode: (node: Node) => void;
  onSelectLink: (link: Link) => void;
  onConnectNodes: (sourceName: string, targetName: string) => void;
  onNodePositionChange?: (name: string, x: number, y: number) => void;
  onRefreshNode?: (nodeName: string) => void;
  refreshingNodeName?: string | null;
  addBlockTrigger?: number;
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
  onAddBlock: () => void;
  focusedNodeName: string | null;
  onClearFocus: () => void;
  blocksCount: number;
  hiddenLinksCount: number;
}

const FloatingToolbar: React.FC<FloatingToolbarProps> = ({
  onAddBlock,
  focusedNodeName,
  onClearFocus,
  blocksCount,
  hiddenLinksCount,
}) => {
  const { fitView } = useReactFlow();

  return (
    <Panel position="top-right" style={{ margin: 16 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          backgroundColor: "rgba(255, 255, 255, 0.95)",
          backdropFilter: "blur(8px)",
          border: "1px solid #E2E8F0",
          borderRadius: 2.5,
          p: 0.75,
          boxShadow: "0 4px 12px rgba(0, 0, 0, 0.05)",
        }}
      >
        {/* Add Block Button */}
        <Button
          variant="contained"
          size="small"
          startIcon={<Layers size={15} />}
          onClick={onAddBlock}
          sx={{
            backgroundColor: "#4F46E5",
            color: "#FFFFFF",
            fontWeight: 600,
            fontSize: "0.8rem",
            textTransform: "none",
            borderRadius: 1.75,
            px: 1.5,
            py: 0.6,
            boxShadow: "0 2px 4px rgba(79, 70, 229, 0.2)",
            "&:hover": {
              backgroundColor: "#4338CA",
            },
          }}
        >
          Add Block
        </Button>

        {/* Fit View Button */}
        <Tooltip title="Center and fit all nodes in view">
          <Button
            variant="outlined"
            size="small"
            startIcon={<Maximize2 size={15} />}
            onClick={() => fitView({ padding: 0.2, duration: 400 })}
            sx={{
              borderColor: "#CBD5E1",
              color: "#334155",
              fontWeight: 600,
              fontSize: "0.8rem",
              textTransform: "none",
              borderRadius: 1.75,
              px: 1.25,
              py: 0.6,
              "&:hover": {
                borderColor: "#94A3B8",
                backgroundColor: "#F8FAFC",
              },
            }}
          >
            Fit View
          </Button>
        </Tooltip>

        {/* Focused Node Banner */}
        {focusedNodeName && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              backgroundColor: "#EEF2FF",
              border: "1px solid #C7D2FE",
              borderRadius: 1.75,
              px: 1.25,
              py: 0.4,
            }}
          >
            <Eye size={14} color="#4F46E5" />
            <Typography variant="caption" sx={{ fontWeight: 700, color: "#3730A3" }}>
              Focus: {focusedNodeName} (all links shown)
            </Typography>
            <Tooltip title="Clear node focus (or press Escape)">
              <Box
                onClick={onClearFocus}
                sx={{
                  cursor: "pointer",
                  display: "flex",
                  alignItems: "center",
                  color: "#6366F1",
                  "&:hover": { color: "#312E81" },
                }}
              >
                <X size={14} />
              </Box>
            </Tooltip>
          </Box>
        )}

        {/* Stats Indicator */}
        {blocksCount > 0 && (
          <Tooltip title={`${blocksCount} block(s) configured. Intra-block and inter-block links collapsed by default; click any node to view its links.`}>
            <Chip
              icon={<EyeOff size={13} color="#64748B" style={{ marginLeft: 6 }} />}
              label={`${hiddenLinksCount} links collapsed`}
              size="small"
              sx={{
                height: 26,
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
      </Box>
    </Panel>
  );
};

export const TopologyGraph: React.FC<TopologyGraphProps> = ({
  nodes,
  links,
  nodeStatuses,
  networkState,
  selectedTag,
  onSelectNode,
  onSelectLink,
  onConnectNodes,
  onNodePositionChange,
  onRefreshNode,
  refreshingNodeName,
  addBlockTrigger,
}) => {
  // Block state
  const [blocks, setBlocks] = useState<GraphBlock[]>(getStoredBlocks);
  const [nodeBlockMap, setNodeBlockMap] = useState<Record<string, string>>(getStoredNodeBlocks);
  const [focusedNodeName, setFocusedNodeName] = useState<string | null>(null);

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

  // Escape key to reset focus
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setFocusedNodeName(null);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

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

  // Listen for trigger from parent (e.g. Navbar menu)
  const prevTriggerRef = useRef(addBlockTrigger);
  useEffect(() => {
    if (addBlockTrigger && addBlockTrigger !== prevTriggerRef.current) {
      handleAddBlock();
    }
    prevTriggerRef.current = addBlockTrigger;
  }, [addBlockTrigger, handleAddBlock]);

  const handleRenameBlock = useCallback((id: string, name: string) => {
    setBlocks((prev) => prev.map((b) => (b.id === id ? { ...b, name } : b)));
  }, []);

  const handleDeleteBlock = useCallback((id: string) => {
    setBlocks((prev) => prev.filter((b) => b.id !== id));
    setNodeBlockMap((prev) => {
      const next = { ...prev };
      for (const [nodeName, bId] of Object.entries(next)) {
        if (bId === id) {
          delete next[nodeName];
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
      const pos = posMap.get(node.name);
      if (pos) {
        const cx = pos.x + 130;
        const cy = pos.y + 90;
        const target = blocks.find(
          (b) => cx >= b.x && cx <= b.x + b.width && cy >= b.y && cy <= b.y + b.height,
        );
        if (target) {
          effMap[node.name] = target.id;
        }
      }
    });

    return { nodePosMap: posMap, effectiveNodeBlockMap: effMap };
  }, [nodes, blocks, nodeBlockMap]);

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
      const isOnline = nodeStatuses[node.name] ? nodeStatuses[node.name].connected : true;
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
    // A block is full-mesh IF AND ONLY IF:
    // - It has at least 2 member nodes.
    // - Every pair of distinct member nodes (u, v) in the block has a direct WireGuard link (non-manual).
    const blockFullMeshStatusMap = new Map<string, boolean>();
    blocks.forEach((block) => {
      const memberNames = nodes
        .filter((n) => effectiveNodeBlockMap[n.name] === block.id)
        .map((n) => n.name);

      if (memberNames.length < 2) {
        blockFullMeshStatusMap.set(block.id, false);
        return;
      }

      // Check if every pair (u, v) of nodes inside this block has a direct WireGuard link
      const hasIntraWireGuardLink = (u: string, v: string) => {
        return links.some(
          (l) =>
            isWireGuardLink(l) &&
            ((l.from.name === u && l.to.name === v) || (l.from.name === v && l.to.name === u)),
        );
      };

      const isFullMesh = memberNames.every((u, i) =>
        memberNames.slice(i + 1).every((v) => hasIntraWireGuardLink(u, v)),
      );

      blockFullMeshStatusMap.set(block.id, isFullMesh);
    });

    // 2. Block nodes (zIndex: 0 with pointer-events handling so they render beneath node cards)
    const blockFlowNodes: FlowNode[] = blocks.map((block) => {
      const memberNodes = nodes.filter((n) => effectiveNodeBlockMap[n.name] === block.id);
      const memberNamesSet = new Set(memberNodes.map((n) => n.name));
      const intraLinks = links.filter(
        (l) => memberNamesSet.has(l.from.name) && memberNamesSet.has(l.to.name),
      );
      const brokenLinks = intraLinks.filter((l) => {
        return getLinkWorkingState(l, networkState, nodeStatuses) === "not_working";
      });

      const isFullMesh = blockFullMeshStatusMap.get(block.id) ?? false;
      const fullMeshCount = isFullMesh ? memberNodes.length : 0;
      const healthyCount = memberNodes.filter((n) => nodeHealthMap[n.name]?.isHealthy).length;

      return {
        id: block.id,
        type: "blockGroup",
        position: { x: block.x, y: block.y },
        style: { width: block.width, height: block.height, zIndex: 0 },
        initialWidth: block.width,
        initialHeight: block.height,
        width: block.width,
        height: block.height,
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
          onRenameBlock: handleRenameBlock,
          onDeleteBlock: handleDeleteBlock,
          onChangeColor: handleChangeBlockColor,
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

      return {
        id: node.name,
        type: "customNode",
        position: pos,
        style: { zIndex: 10, width: 260 },
        initialWidth: 260,
        initialHeight: 180,
        width: 260,
        height: 180,
        data: {
          node,
          status: nodeStatuses[node.name],
          blockName: assignedBlock?.name,
          blockColor: assignedBlock?.color,
          inBlock,
          isBlockFullMesh,
          isHealthy: health?.isHealthy ?? false,
          healthDetails: health,
          isFocused: focusedNodeName === node.name,
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
    focusedNodeName,
    onSelectNode,
    onRefreshNode,
    refreshingNodeName,
    handleRenameBlock,
    handleDeleteBlock,
    handleChangeBlockColor,
  ]);

  // Convert links to React Flow edges with filtering rules:
  // 1. By default, don't display links between two nodes of different blocks.
  //    Display ONLY ONE virtual line between two blocks instead, no matter how many node links exist.
  // 2. Intra-block links (two nodes of the same block) are hidden by default.
  // 3. When user clicks a node to focus it, ALWAYS display the node's all links.
  // 4. The global scope node (doesn't belong to any block) STILL ALWAYS displays all links.
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

      blockVirtualEdges.push({
        id: `block-edge-${pairKey}`,
        source: b1.id,
        target: b2.id,
        sourceHandle: "block-source",
        targetHandle: "block-target",
        type: "blockVirtualEdge",
        zIndex: 1,
        data: {
          sourceBlock: b1,
          targetBlock: b2,
          links: pairLinks,
          workingCount,
          downCount,
          unknownCount,
        } as unknown as Record<string, unknown>,
      });
    });

    // 2. Filter node-to-node links according to requirements
    const visibleLinks = links.filter((link) => {
      // Focus rule: when user clicks a node to focus it, always display the node's all links
      if (focusedNodeName) {
        if (link.from.name === focusedNodeName || link.to.name === focusedNodeName) {
          return true;
        }
      }

      const fromBlock = effectiveNodeBlockMap[link.from.name];
      const toBlock = effectiveNodeBlockMap[link.to.name];

      // Global scope node (doesn't belong to any block) still always displays all links
      const isGlobalInvolved = !fromBlock || !toBlock;
      if (isGlobalInvolved) {
        return true;
      }

      // Intra-block links (same block): hidden by default unless focused
      if (fromBlock === toBlock) {
        return false;
      }

      // Inter-block links (different blocks): don't display by default (virtual line displayed instead)
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
      const isExternal = Boolean(fromNode?.is_external || toNode?.is_external);

      return {
        id: edgeId,
        source: link.from.name,
        target: link.to.name,
        type: "customEdge",
        data: {
          link,
          workingState,
          latestHandshake,
          transferRxBytes: rxBytes,
          transferTxBytes: txBytes,
          isExternal,
          linkIndexInPair,
          totalLinksInPair,
          onSelect: onSelectLink,
        } as unknown as Record<string, unknown>,
      };
    });

    return [...blockVirtualEdges, ...visibleNodeEdges];
  }, [nodes, links, blocks, networkState, onSelectLink, effectiveNodeBlockMap, focusedNodeName, nodeStatuses]);

  const [flowNodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [flowEdges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync state when props change while preserving current node positions and dimensions
  React.useEffect(() => {
    setNodes((currentNodes) => {
      const currentMap = new Map(currentNodes.map((n) => [n.id, n]));
      return initialNodes.map((n) => {
        const existing = currentMap.get(n.id);
        const nodeData = (n.data as { node?: Node })?.node;
        let pos = n.position;
        if (typeof nodeData?.x === "number" && typeof nodeData?.y === "number") {
          pos = { x: nodeData.x, y: nodeData.y };
        } else if (existing?.position) {
          pos = existing.position;
        }

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
      if (node.type === "blockGroup") {
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
              delete next[fn.id];
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
            nodeCenterX >= b.x &&
            nodeCenterX <= b.x + b.width &&
            nodeCenterY >= b.y &&
            nodeCenterY <= b.y + b.height,
        );

        setNodeBlockMap((prev) => {
          const currentBlockId = prev[n.id];
          if (targetBlock) {
            if (currentBlockId === targetBlock.id) return prev;
            return { ...prev, [n.id]: targetBlock.id };
          } else {
            if (!currentBlockId) return prev;
            const next = { ...prev };
            delete next[n.id];
            return next;
          }
        });
      }
    },
    [blocks, flowNodes, onNodePositionChange],
  );

  const initialViewport = useMemo(() => getStoredViewport(), []);

  const handleMoveEnd: OnMoveEnd = useCallback((_event, viewport) => {
    try {
      localStorage.setItem(STORAGE_KEY_VIEWPORT, JSON.stringify(viewport));
      localStorage.setItem(STORAGE_KEY_ZOOM, JSON.stringify(viewport.zoom));
    } catch (e) {
      console.error("Failed to save viewport to localStorage", e);
    }
  }, []);

  const onConnect = useCallback(
    (connection: Connection) => {
      if (connection.source && connection.target && connection.source !== connection.target) {
        onConnectNodes(connection.source, connection.target);
      }
      setEdges((eds) => addEdge(connection, eds));
    },
    [onConnectNodes, setEdges],
  );

  // Hidden links count (both intra-block links and collapsed inter-block links)
  const hiddenLinksCount = useMemo(() => {
    return links.filter((link) => {
      // If node is focused, all its connected links are visible
      if (focusedNodeName) {
        if (link.from.name === focusedNodeName || link.to.name === focusedNodeName) {
          return false;
        }
      }
      const fromBlock = effectiveNodeBlockMap[link.from.name];
      const toBlock = effectiveNodeBlockMap[link.to.name];

      // Global scope node links are always displayed
      if (!fromBlock || !toBlock) {
        return false;
      }

      // Both belong to blocks (same block or different blocks): collapsed by default
      return true;
    }).length;
  }, [links, effectiveNodeBlockMap, focusedNodeName]);

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
        nodes={flowNodes}
        edges={flowEdges}
        onNodesChange={handleNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeDragStart={handleNodeDragStart}
        onNodeDrag={handleNodeDrag}
        onNodeDragStop={handleNodeDragStop}
        onPaneClick={() => setFocusedNodeName(null)}
        onNodeClick={(_event, flowNode) => {
          if (flowNode.type === "customNode") {
            setFocusedNodeName((prev) => (prev === flowNode.id ? null : flowNode.id));
            const n = (flowNode.data as any)?.node;
            if (n) {
              onSelectNode(n);
            }
          }
        }}
        onEdgeClick={(_event, edge) => {
          const edgeData = edge.data as unknown as { link?: Link };
          if (edgeData?.link) {
            onSelectLink(edgeData.link);
          }
        }}
        onConnect={onConnect}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        defaultViewport={initialViewport}
        fitView={!initialViewport}
        onMoveEnd={handleMoveEnd}
        attributionPosition="bottom-left"
      >
        {/* Floating Top-Right Block & Focus Toolbar */}
        <FloatingToolbar
          onAddBlock={handleAddBlock}
          focusedNodeName={focusedNodeName}
          onClearFocus={() => setFocusedNodeName(null)}
          blocksCount={blocks.length}
          hiddenLinksCount={hiddenLinksCount}
        />

        <Background variant={BackgroundVariant.Dots} gap={20} size={1.2} color="#CBD5E1" />
        <Controls />
        <MiniMap
          nodeStrokeColor="#4F46E5"
          nodeColor="#E2E8F0"
          nodeBorderRadius={4}
          maskColor="rgba(248, 250, 252, 0.7)"
        />
      </ReactFlow>
    </Box>
  );
};
