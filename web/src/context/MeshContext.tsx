import React, { createContext, useContext, useState, useEffect, useCallback, useMemo, useRef } from "react";
import { api } from "../api/client";
import {
  Node,
  Link,
  NodeStatus,
  NetworkState,
  NetworkPolicy,
  NetworkSettings,
  FleetLiveResponse,
  GraphBlock,
  FlapAlertedMetrics,
} from "../types/api";
import { isSameLink } from "../utils/link";

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
  networkSettings: NetworkSettings | null;
  refreshNetworkSettings: () => Promise<void>;
  networkName: string;
  flapAlertedUrl: string | null;
  flapAlertedMetrics: FlapAlertedMetrics | null;
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
  locateNodeName: string | null;
  setLocateNodeName: (name: string | null) => void;
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
  openSyncModal: (node?: string | null) => void;
  closeSyncModal: () => void;
  selectedNode: Node | null;
  setSelectedNode: (node: Node | null) => void;
  selectedLink: Link | null;
  setSelectedLink: (link: Link | null) => void;
  addBlockTrigger: number;
  setAddBlockTrigger: React.Dispatch<React.SetStateAction<number>>;

  // Apply Config Modal
  applyOpen: boolean;
  setApplyOpen: (open: boolean) => void;
  applyInitialContent: string;
  setApplyInitialContent: (content: string) => void;
  openApplyModal: (snippet?: string) => void;

  // Node position update
  updateNodePosition: (name: string, x: number, y: number) => void;

  // Toast
  stateToast: {
    message: string;
    severity: "success" | "warning" | "error" | "info";
  } | null;
  setStateToast: (toast: { message: string; severity: "success" | "warning" | "error" | "info" } | null) => void;
}

export const MeshContext = createContext<MeshContextType | null>(null);

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
  const [networkSettings, setNetworkSettings] = useState<NetworkSettings | null>(null);
  const [networkName, setNetworkName] = useState<string>("Easy42");
  const [fleetLive, setFleetLive] = useState<FleetLiveResponse | null>(null);
  const [loadingData, setLoadingData] = useState(false);

  // Selected for drawers
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [selectedLink, setSelectedLink] = useState<Link | null>(null);

  // Synchronize selectedNode and selectedLink when nodes or links are updated
  useEffect(() => {
    setSelectedNode((prev) => {
      if (!prev) return null;
      const fresh = nodes.find((n) => n.name === prev.name);
      return fresh || null;
    });
  }, [nodes]);

  useEffect(() => {
    setSelectedLink((prev) => {
      if (!prev) return null;
      const fresh = links.find((l) => isSameLink(l, prev));
      return fresh || null;
    });
  }, [links]);

  // Modals
  const [addNodeOpen, setAddNodeOpen] = useState(false);
  const [nodeToEdit, setNodeToEdit] = useState<Node | null>(null);
  const [locateNodeName, setLocateNodeName] = useState<string | null>(null);
  const [renameModalOpen, setRenameModalOpen] = useState(false);
  const [nodeToRename, setNodeToRename] = useState<Node | null>(null);
  const [addLinkOpen, setAddLinkOpen] = useState(false);
  const [linkToEdit, setLinkToEdit] = useState<Link | null>(null);
  const [connectFrom, setConnectFrom] = useState<string>("");
  const [connectTo, setConnectTo] = useState<string>("");
  const [unlockOpen, setUnlockOpen] = useState(false);
  const [syncOpen, setSyncOpenState] = useState(false);
  const [syncTargetNode, setSyncTargetNode] = useState<string | null>(null);

  const setSyncOpen = useCallback((open: boolean) => {
    setSyncOpenState(open);
    if (!open) {
      setSyncTargetNode(null);
    }
  }, []);

  const openSyncModal = useCallback((node?: string | null) => {
    setSyncTargetNode(node || null);
    setSyncOpenState(true);
  }, []);

  const closeSyncModal = useCallback(() => {
    setSyncOpenState(false);
    setSyncTargetNode(null);
  }, []);

  const [updatingState, setUpdatingState] = useState(false);
  const [addBlockTrigger, setAddBlockTrigger] = useState(0);

  // Apply Config Modal State
  const [applyOpen, setApplyOpen] = useState(false);
  const [applyInitialContent, setApplyInitialContent] = useState("");

  const openApplyModal = useCallback((snippet?: string) => {
    if (snippet) {
      setApplyInitialContent(snippet);
    } else {
      setApplyInitialContent("");
    }
    setApplyOpen(true);
  }, []);

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
      if (res.network_name) {
        setNetworkName(res.network_name);
      }
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
      const [nodesData, linksData, statusesData, stateData, policiesData, liveData, blocksData, settingsData] =
        await Promise.all([
          api.getNodes(),
          api.getLinks(),
          api.getNodeStatuses().catch(() => ({})),
          api.getState().catch(() => null),
          api.getNetworkPolicies().catch(() => []),
          api.getFleetLiveStatus().catch(() => null),
          api.getBlocks().catch(() => []),
          api.getNetworkSettings().catch(() => null),
        ]);
      setNodes(nodesData || []);
      setLinks(linksData || []);
      setSelectedNode((prev) => {
        if (!prev) return null;
        return (nodesData || []).find((n) => n.name === prev.name) || null;
      });
      setSelectedLink((prev) => {
        if (!prev) return null;
        return (linksData || []).find((l) => isSameLink(l, prev)) || null;
      });
      setNodeStatuses(statusesData || {});
      setNetworkState(stateData);
      setNetworkPolicies(policiesData || []);
      if (settingsData) {
        setNetworkSettings(settingsData);
        if (settingsData.name) {
          setNetworkName(settingsData.name);
        }
      }
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

  const refreshNetworkSettings = useCallback(async () => {
    try {
      const data = await api.getNetworkSettings();
      setNetworkSettings(data);
      if (data?.name) {
        setNetworkName(data.name);
      }
    } catch {
      // Ignore background refresh errors
    }
  }, []);

  const flapAlertedUrl = useMemo(() => {
    let customUrl = (networkSettings?.flapalerted_url || "").trim();
    if (customUrl) {
      if (!customUrl.startsWith("http://") && !customUrl.startsWith("https://")) {
        customUrl = `http://${customUrl}`;
      }
      return customUrl;
    }
    const addr = (networkSettings?.flapalerted_address || "").trim();
    if (!addr) return null;
    return `http://${addr.includes(":") && !addr.startsWith("[") ? `[${addr}]` : addr}:8699`;
  }, [networkSettings]);

  const [flapAlertedMetrics, setFlapAlertedMetrics] = useState<FlapAlertedMetrics | null>(null);

  useEffect(() => {
    if (!flapAlertedUrl || !authenticated) {
      setFlapAlertedMetrics(null);
      return;
    }

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let isCancelled = false;

    const connect = () => {
      if (isCancelled) return;
      try {
        es = new EventSource("/api/flaps/metrics/stream", { withCredentials: true });

        es.onmessage = (event) => {
          if (isCancelled) return;
          try {
            if (event.data) {
              const data = JSON.parse(event.data);
              setFlapAlertedMetrics(data);
            }
          } catch {
            // Ignore invalid parse
          }
        };

        es.onerror = () => {
          if (isCancelled) return;
          setFlapAlertedMetrics(null);
          if (es) {
            es.close();
            es = null;
          }
          reconnectTimer = setTimeout(connect, 10000);
        };
      } catch {
        setFlapAlertedMetrics(null);
        if (!isCancelled) {
          reconnectTimer = setTimeout(connect, 10000);
        }
      }
    };

    connect();

    return () => {
      isCancelled = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (es) {
        es.close();
        es = null;
      }
    };
  }, [flapAlertedUrl, authenticated]);

  const refreshFleetLive = useCallback(async (flush = false) => {
    try {
      const [liveData, statusesData, stateData] = await Promise.all([
        api.getFleetLiveStatus(flush).catch(() => null),
        api.getNodeStatuses().catch(() => ({})),
        api.getState().catch(() => null),
      ]);
      if (liveData) setFleetLive(liveData);
      if (statusesData) setNodeStatuses(statusesData);
      if (stateData) setNetworkState(stateData);
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

  const reloadDebounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isReloadingRef = useRef(false);
  const pendingReloadRef = useRef(false);

  const scheduleReload = useCallback(() => {
    if (reloadDebounceTimer.current) {
      clearTimeout(reloadDebounceTimer.current);
    }
    reloadDebounceTimer.current = setTimeout(() => {
      if (isReloadingRef.current) {
        pendingReloadRef.current = true;
        return;
      }
      isReloadingRef.current = true;
      loadData().finally(() => {
        isReloadingRef.current = false;
        if (pendingReloadRef.current) {
          pendingReloadRef.current = false;
          loadData();
        }
      });
    }, 100);
  }, [loadData]);

  // Real-time WebSocket connection to keep frontend state in sync with backend and other instances
  useEffect(() => {
    if (!authenticated) {
      return;
    }

    let ws: WebSocket | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let isCancelled = false;
    let reconnectAttempts = 0;

    const connect = () => {
      if (isCancelled) return;

      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
      const wsUrl = `${protocol}//${window.location.host}/api/ws`;

      try {
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
          if (isCancelled) {
            ws?.close();
            return;
          }
          reconnectAttempts = 0;
          scheduleReload();
        };

        ws.onmessage = (event) => {
          if (isCancelled) return;
          try {
            const data = JSON.parse(event.data);
            if (data?.type === "cluster_updated") {
              scheduleReload();
            }
          } catch {
            // Ignore non-json messages
          }
        };

        ws.onclose = () => {
          if (isCancelled) return;
          const delay = Math.min(1000 * Math.pow(1.5, reconnectAttempts), 10000);
          reconnectAttempts++;
          reconnectTimer = setTimeout(connect, delay);
        };

        ws.onerror = () => {
          if (ws) {
            try {
              ws.close();
            } catch {
              // Ignore close error
            }
          }
        };
      } catch {
        if (!isCancelled) {
          const delay = Math.min(1000 * Math.pow(1.5, reconnectAttempts), 10000);
          reconnectAttempts++;
          reconnectTimer = setTimeout(connect, delay);
        }
      }
    };

    connect();

    return () => {
      isCancelled = true;
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
      }
      if (ws) {
        try {
          ws.close();
        } catch {
          // Ignore close error
        }
      }
    };
  }, [authenticated, scheduleReload]);

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

  const updateNodePosition = useCallback((name: string, x: number, y: number) => {
    setNodes((prev) =>
      prev.map((n) => (n.name === name ? { ...n, x, y } : n))
    );
  }, []);

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
    networkSettings,
    refreshNetworkSettings,
    networkName,
    flapAlertedUrl,
    flapAlertedMetrics,
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
    locateNodeName,
    setLocateNodeName,
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
    openSyncModal,
    closeSyncModal,
    selectedNode,
    setSelectedNode,
    selectedLink,
    setSelectedLink,
    addBlockTrigger,
    setAddBlockTrigger,
    applyOpen,
    setApplyOpen,
    applyInitialContent,
    setApplyInitialContent,
    openApplyModal,
    updateNodePosition,
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
