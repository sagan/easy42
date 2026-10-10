import React, { createContext, useContext, useState, useEffect, useCallback, useMemo } from "react";
import { api } from "../api/client";
import {
  Node,
  Link,
  NodeStatus,
  NetworkState,
  NetworkPolicy,
  FleetLiveResponse,
  GraphBlock,
} from "../types/api";

interface MeshContextType {
  // Auth state
  checkingAuth: boolean;
  authenticated: boolean;
  isUnlocked: boolean;
  setAuthenticated: (v: boolean) => void;
  setIsUnlocked: (v: boolean) => void;
  checkAuth: () => Promise<void>;

  // Data state
  nodes: Node[];
  links: Link[];
  blocks: GraphBlock[];
  setBlocks: (blocks: GraphBlock[]) => void;
  nodeStatuses: Record<string, NodeStatus>;
  networkState: NetworkState | null;
  networkPolicies: NetworkPolicy[];
  fleetLive: FleetLiveResponse | null;
  loadingData: boolean;
  loadData: () => Promise<void>;
  refreshFleetLive: (flush?: boolean) => Promise<void>;

  // Filter & tags
  selectedTag: string;
  setSelectedTag: (tag: string) => void;
  uniqueTags: string[];
  displayedNodes: Node[];
  displayedLinks: Link[];
  displayedInternalNodes: Node[];
  missingMeshLinksCount: number;
  unreachableNodes: Node[];

  // Action states
  updatingState: boolean;
  handleUpdateState: (nodeName?: string) => Promise<void>;
  handleCreateFullMesh: () => Promise<void>;

  // Modals & Drawers
  addNodeOpen: boolean;
  setAddNodeOpen: (open: boolean) => void;
  nodeToEdit: Node | null;
  setNodeToEdit: (node: Node | null) => void;
  renameModalOpen: boolean;
  setRenameModalOpen: (open: boolean) => void;
  nodeToRename: Node | null;
  setNodeToRename: (node: Node | null) => void;
  addLinkOpen: boolean;
  setAddLinkOpen: (open: boolean) => void;
  linkToEdit: Link | null;
  setLinkToEdit: (link: Link | null) => void;
  connectFrom: string;
  setConnectFrom: (node: string) => void;
  connectTo: string;
  setConnectTo: (node: string) => void;
  unlockOpen: boolean;
  setUnlockOpen: (open: boolean) => void;
  syncOpen: boolean;
  setSyncOpen: (open: boolean) => void;
  syncTargetNode: string | null;
  setSyncTargetNode: (node: string | null) => void;
  selectedNode: Node | null;
  setSelectedNode: (node: Node | null) => void;
  selectedLink: Link | null;
  setSelectedLink: (link: Link | null) => void;
  addBlockTrigger: number;
  setAddBlockTrigger: React.Dispatch<React.SetStateAction<number>>;

  // Toast
  stateToast: {
    message: string;
    severity: "success" | "warning" | "error" | "info";
  } | null;
  setStateToast: (toast: { message: string; severity: "success" | "warning" | "error" | "info" } | null) => void;
}

const MeshContext = createContext<MeshContextType | null>(null);

export const MeshProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);
  const [isUnlocked, setIsUnlocked] = useState(false);

  // Mesh State
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [blocks, setBlocks] = useState<GraphBlock[]>([]);
  const [nodeStatuses, setNodeStatuses] = useState<Record<string, NodeStatus>>({});
  const [networkState, setNetworkState] = useState<NetworkState | null>(null);
  const [networkPolicies, setNetworkPolicies] = useState<NetworkPolicy[]>([]);
  const [fleetLive, setFleetLive] = useState<FleetLiveResponse | null>(null);
  const [loadingData, setLoadingData] = useState(false);

  // Selected for drawers
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [selectedLink, setSelectedLink] = useState<Link | null>(null);

  // Modals
  const [addNodeOpen, setAddNodeOpen] = useState(false);
  const [nodeToEdit, setNodeToEdit] = useState<Node | null>(null);
  const [renameModalOpen, setRenameModalOpen] = useState(false);
  const [nodeToRename, setNodeToRename] = useState<Node | null>(null);
  const [addLinkOpen, setAddLinkOpen] = useState(false);
  const [linkToEdit, setLinkToEdit] = useState<Link | null>(null);
  const [connectFrom, setConnectFrom] = useState<string>("");
  const [connectTo, setConnectTo] = useState<string>("");
  const [unlockOpen, setUnlockOpen] = useState(false);
  const [syncOpen, setSyncOpen] = useState(false);
  const [syncTargetNode, setSyncTargetNode] = useState<string | null>(null);
  const [updatingState, setUpdatingState] = useState(false);
  const [addBlockTrigger, setAddBlockTrigger] = useState(0);

  const [stateToast, setStateToast] = useState<{
    message: string;
    severity: "success" | "warning" | "error" | "info";
  } | null>(null);

  // Unreachable nodes
  const unreachableNodes = useMemo(() => {
    return nodes.filter((n) => !n.is_external && nodeStatuses[n.name] && !nodeStatuses[n.name].connected);
  }, [nodes, nodeStatuses]);

  // Tag filter
  const [selectedTag, setSelectedTag] = useState<string>("All");

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

  useEffect(() => {
    if (selectedTag !== "All" && !uniqueTags.includes(selectedTag)) {
      setSelectedTag("All");
    }
  }, [selectedTag, uniqueTags]);

  const displayedNodes = useMemo(() => {
    if (selectedTag === "All") return nodes;
    return nodes.filter((n) => n.tags && n.tags.includes(selectedTag));
  }, [nodes, selectedTag]);

  const displayedLinks = useMemo(() => {
    const displayedNames = new Set(displayedNodes.map((n) => n.name));
    return links.filter((l) => displayedNames.has(l.from.name) && displayedNames.has(l.to.name));
  }, [links, displayedNodes]);

  const displayedInternalNodes = useMemo(() => {
    return displayedNodes.filter((n) => !n.is_external);
  }, [displayedNodes]);

  const missingMeshLinksCount = useMemo(() => {
    if (displayedInternalNodes.length < 2) return 0;
    let count = 0;
    for (let i = 0; i < displayedInternalNodes.length; i++) {
      for (let j = i + 1; j < displayedInternalNodes.length; j++) {
        const n1 = displayedInternalNodes[i].name;
        const n2 = displayedInternalNodes[j].name;
        const exists = links.some(
          (l) => (l.from.name === n1 && l.to.name === n2) || (l.from.name === n2 && l.to.name === n1),
        );
        if (!exists) count++;
      }
    }
    return count;
  }, [displayedInternalNodes, links]);

  const checkAuth = useCallback(async () => {
    try {
      const res = await api.getAuthStatus();
      setAuthenticated(res.authenticated);
      setIsUnlocked(res.unlocked);
    } catch {
      setAuthenticated(false);
      setIsUnlocked(false);
    } finally {
      setCheckingAuth(false);
    }
  }, []);

  const loadData = useCallback(async () => {
    setLoadingData(true);
    try {
      const [nodesData, linksData, statusesData, stateData, policiesData, liveData, blocksData] = await Promise.all([
        api.getNodes(),
        api.getLinks(),
        api.getNodeStatuses().catch(() => ({})),
        api.getState().catch(() => null),
        api.getNetworkPolicies().catch(() => []),
        api.getFleetLiveStatus().catch(() => null),
        api.getBlocks().catch(() => []),
      ]);
      setNodes(nodesData || []);
      setLinks(linksData || []);
      setNodeStatuses(statusesData || {});
      setNetworkState(stateData);
      setNetworkPolicies(policiesData || []);
      if (liveData) setFleetLive(liveData);
      if (blocksData && blocksData.length > 0) {
        setBlocks(blocksData);
      } else {
        try {
          const cached = localStorage.getItem("easy42_graph_blocks_v1");
          if (cached) {
            setBlocks(JSON.parse(cached));
          }
        } catch {
          // ignore
        }
      }
    } catch {
      // Handled
    } finally {
      setLoadingData(false);
    }
  }, []);

  const refreshFleetLive = useCallback(async (flush = false) => {
    try {
      const [liveData, statusesData] = await Promise.all([
        api.getFleetLiveStatus(flush).catch(() => null),
        api.getNodeStatuses().catch(() => ({})),
      ]);
      if (liveData) setFleetLive(liveData);
      if (statusesData) setNodeStatuses(statusesData);
    } catch {
      // Ignore background refresh errors
    }
  }, []);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  useEffect(() => {
    if (authenticated) {
      loadData();
    }
  }, [authenticated, loadData]);

  // Periodic poll for live monitoring metrics
  useEffect(() => {
    if (!authenticated) return;
    const interval = setInterval(() => {
      refreshFleetLive();
    }, 6000);
    return () => clearInterval(interval);
  }, [authenticated, refreshFleetLive]);

  const handleUpdateState = useCallback(
    async (nodeName?: string) => {
      setUpdatingState(true);
      try {
        const res = await api.updateState(nodeName);
        if (res.state) {
          setNetworkState(res.state);
        }
        await refreshFleetLive();
        setStateToast({
          message: nodeName
            ? `Live state for ${nodeName} updated.`
            : "Network state successfully fetched from all devices.",
          severity: "success",
        });
      } catch (err: unknown) {
        const e = err as Error;
        setStateToast({
          message: `Failed to update state: ${e.message}`,
          severity: "error",
        });
      } finally {
        setUpdatingState(false);
      }
    },
    [refreshFleetLive],
  );

  const handleCreateFullMesh = useCallback(async () => {
    if (displayedInternalNodes.length < 2) {
      setStateToast({
        message: "At least 2 displayed internal nodes are required to create a full mesh.",
        severity: "warning",
      });
      return;
    }
    if (missingMeshLinksCount === 0) {
      setStateToast({
        message: "All displayed internal nodes are already fully connected in a mesh.",
        severity: "info",
      });
      return;
    }
    if (!isUnlocked) {
      setUnlockOpen(true);
      return;
    }

    const confirmMsg = `Create full mesh network between ${displayedInternalNodes.length} displayed internal nodes? This will automatically add ${missingMeshLinksCount} missing link(s).`;
    if (!window.confirm(confirmMsg)) return;

    try {
      const nodeNames = displayedInternalNodes.map((n) => n.name);
      const added = await api.createFullMesh(nodeNames);
      await loadData();
      setStateToast({
        message: `Full mesh established: added ${added.length} new link(s).`,
        severity: "success",
      });
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423) {
        setUnlockOpen(true);
      } else {
        setStateToast({
          message: `Failed to create full mesh: ${e.message}`,
          severity: "error",
        });
      }
    }
  }, [displayedInternalNodes, missingMeshLinksCount, isUnlocked, loadData]);

  const value: MeshContextType = {
    checkingAuth,
    authenticated,
    isUnlocked,
    setAuthenticated,
    setIsUnlocked,
    checkAuth,
    nodes,
    links,
    blocks,
    setBlocks,
    nodeStatuses,
    networkState,
    networkPolicies,
    fleetLive,
    loadingData,
    loadData,
    refreshFleetLive,
    selectedTag,
    setSelectedTag,
    uniqueTags,
    displayedNodes,
    displayedLinks,
    displayedInternalNodes,
    missingMeshLinksCount,
    unreachableNodes,
    updatingState,
    handleUpdateState,
    handleCreateFullMesh,
    addNodeOpen,
    setAddNodeOpen,
    nodeToEdit,
    setNodeToEdit,
    renameModalOpen,
    setRenameModalOpen,
    nodeToRename,
    setNodeToRename,
    addLinkOpen,
    setAddLinkOpen,
    linkToEdit,
    setLinkToEdit,
    connectFrom,
    setConnectFrom,
    connectTo,
    setConnectTo,
    unlockOpen,
    setUnlockOpen,
    syncOpen,
    setSyncOpen,
    syncTargetNode,
    setSyncTargetNode,
    selectedNode,
    setSelectedNode,
    selectedLink,
    setSelectedLink,
    addBlockTrigger,
    setAddBlockTrigger,
    stateToast,
    setStateToast,
  };

  return <MeshContext.Provider value={value}>{children}</MeshContext.Provider>;
};

export const useMesh = () => {
  const context = useContext(MeshContext);
  if (!context) {
    throw new Error("useMesh must be used within a MeshProvider");
  }
  return context;
};
