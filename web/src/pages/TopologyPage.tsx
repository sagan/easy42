import React, { useState, useCallback } from "react";
import { Box } from "@mui/material";
import { useMesh } from "../context/MeshContext";
import { TopologyGraph } from "../components/Topology/TopologyGraph";
import { NodeDetailDrawer } from "../components/Topology/NodeDetailDrawer";
import { LinkDetailDrawer } from "../components/Topology/LinkDetailDrawer";
import { Node, Link } from "../types/api";
import { api } from "../api/client";
import { useNavigate } from "react-router-dom";
import { isSameLink } from "../utils/link";

export const TopologyPage: React.FC = () => {
  const {
    nodes,
    links,
    displayedNodes,
    displayedLinks,
    displayedInternalNodes,
    missingMeshLinksCount,
    nodeStatuses,
    networkState,
    networkPolicies,
    networkSettings,
    selectedTag,
    setSelectedTag,
    uniqueTags,
    handleCreateFullMesh,
    handleUpdateState,
    loadData,
    isUnlocked,
    setUnlockOpen,
    setAddNodeOpen,
    setNodeToEdit,
    setRenameModalOpen,
    setNodeToRename,
    setAddLinkOpen,
    setLinkToEdit,
    setConnectFrom,
    setConnectTo,
    selectedNode,
    setSelectedNode,
    selectedLink,
    setSelectedLink,
    openSyncModal,
    addBlockTrigger,
    setAddBlockTrigger,
    updateNodePosition,
    setStateToast,
  } = useMesh();

  const navigate = useNavigate();
  const [refreshingNodeName, setRefreshingNodeName] = useState<string | null>(null);

  const handleSelectNode = useCallback(
    (node: Node) => {
      setSelectedNode(node);
    },
    [setSelectedNode],
  );

  const handleSelectLink = useCallback(
    (link: Link) => {
      setSelectedLink(link);
    },
    [setSelectedLink],
  );

  const handleConnectNodes = useCallback(
    (source: string, target: string) => {
      setConnectFrom(source);
      setConnectTo(target);
      setLinkToEdit(null);
      setAddLinkOpen(true);
    },
    [setConnectFrom, setConnectTo, setLinkToEdit, setAddLinkOpen],
  );

  const handleNodePositionChange = useCallback(
    async (name: string, x: number, y: number) => {
      updateNodePosition(name, x, y);
      try {
        await api.updateNodePosition(name, x, y);
      } catch (err) {
        console.error("Failed to persist node position:", err);
      }
    },
    [updateNodePosition],
  );

  const handleEditNode = (node: Node) => {
    const latest = nodes.find((n) => n.name === node.name) || node;
    setNodeToEdit(latest);
    setAddNodeOpen(true);
  };

  const handleOpenRenameNode = (node: Node) => {
    const latest = nodes.find((n) => n.name === node.name) || node;
    setNodeToRename(latest);
    setRenameModalOpen(true);
  };

  const handleEditLink = (link: Link) => {
    const latest = links.find((l) => isSameLink(l, link)) || link;
    setLinkToEdit(latest);
    setAddLinkOpen(true);
  };

  const handleRefreshNode = async (nodeName: string) => {
    setRefreshingNodeName(nodeName);
    try {
      await handleUpdateState(nodeName);
    } finally {
      setRefreshingNodeName(null);
    }
  };

  const handleAddNode = useCallback(() => {
    if (!isUnlocked) {
      setUnlockOpen(true);
    } else {
      setNodeToEdit(null);
      setAddNodeOpen(true);
    }
  }, [isUnlocked, setUnlockOpen, setNodeToEdit, setAddNodeOpen]);

  const handleAddLink = useCallback(() => {
    if (!isUnlocked) {
      setUnlockOpen(true);
    } else {
      setLinkToEdit(null);
      setConnectFrom("");
      setConnectTo("");
      setAddLinkOpen(true);
    }
  }, [isUnlocked, setUnlockOpen, setLinkToEdit, setConnectFrom, setConnectTo, setAddLinkOpen]);

  const handleAddBlock = useCallback(() => {
    if (!isUnlocked) {
      setUnlockOpen(true);
    } else {
      setAddBlockTrigger((c) => c + 1);
    }
  }, [isUnlocked, setUnlockOpen, setAddBlockTrigger]);

  const handleCreateBlockFullMesh = useCallback(
    async (blockName: string, memberNames: string[]) => {
      if (memberNames.length < 2) {
        setStateToast({
          message: "At least 2 nodes in the block are required to create a full mesh.",
          severity: "warning",
        });
        return;
      }
      if (!isUnlocked) {
        setUnlockOpen(true);
        return;
      }

      const confirmMsg = `Create full mesh network between ${memberNames.length} nodes in block "${blockName}"? This will automatically add missing link(s).`;
      if (!window.confirm(confirmMsg)) return;

      try {
        const added = await api.createFullMesh(memberNames);
        await loadData();
        setStateToast({
          message: `Block "${blockName}" full mesh established: added ${added.length} new link(s).`,
          severity: "success",
        });
      } catch (err: unknown) {
        const e = err as Error & { status?: number };
        if (e.status === 423) {
          setUnlockOpen(true);
        } else {
          setStateToast({
            message: `Failed to create block full mesh: ${e.message}`,
            severity: "error",
          });
        }
      }
    },
    [isUnlocked, setUnlockOpen, setStateToast, loadData],
  );

  return (
    <Box
      sx={{
        width: "100%",
        flex: 1,
        minHeight: 0,
        position: "relative",
        overflow: "hidden",
        display: "flex",
        flexDirection: "column",
      }}
    >
      {/* Main Canvas with single consolidated floating toolbar */}
      <Box sx={{ flex: 1, position: "relative", width: "100%", height: "100%" }}>
        <TopologyGraph
          nodes={displayedNodes}
          links={displayedLinks}
          nodeStatuses={nodeStatuses}
          networkState={networkState}
          networkPolicies={networkPolicies}
          selectedTag={selectedTag}
          onSelectTag={setSelectedTag}
          uniqueTags={uniqueTags}
          onSelectNode={handleSelectNode}
          onSelectLink={handleSelectLink}
          onConnectNodes={handleConnectNodes}
          onNodePositionChange={handleNodePositionChange}
          onRefreshNode={handleRefreshNode}
          refreshingNodeName={refreshingNodeName}
          addBlockTrigger={addBlockTrigger}
          onAddNode={handleAddNode}
          onAddLink={handleAddLink}
          onAddBlock={handleAddBlock}
          onCreateFullMesh={handleCreateFullMesh}
          missingMeshLinksCount={missingMeshLinksCount}
          displayedInternalNodesCount={displayedInternalNodes.length}
          onCreateBlockFullMesh={handleCreateBlockFullMesh}
        />
      </Box>

      {/* Drawers */}
      <NodeDetailDrawer
        node={selectedNode}
        status={selectedNode ? nodeStatuses[selectedNode.name] : undefined}
        open={Boolean(selectedNode)}
        onClose={() => setSelectedNode(null)}
        onEditNode={handleEditNode}
        onRenameNode={handleOpenRenameNode}
        onNodeDeleted={() => {
          setSelectedNode(null);
          loadData();
        }}
        onStatusRefreshed={() => loadData()}
        onOpenDetails={(name) => navigate(`/nodes/${encodeURIComponent(name)}`)}
        onOpenHelper={(name) => navigate(`/helper?node=${encodeURIComponent(name)}`)}
        onOpenLookingGlass={(name) => navigate(`/looking-glass/presets?node=${encodeURIComponent(name)}`)}
        onUpdateNodeState={(name) => handleUpdateState(name)}
        onSyncNode={(name) => {
          openSyncModal(name);
        }}
        onNodeUpdated={(updated) => {
          setSelectedNode(updated);
          loadData();
        }}
      />

      <LinkDetailDrawer
        link={selectedLink}
        open={Boolean(selectedLink)}
        onClose={() => setSelectedLink(null)}
        onEditLink={handleEditLink}
        onLinkDeleted={() => {
          setSelectedLink(null);
          loadData();
        }}
        onRefreshLink={async () => {
          await loadData();
        }}
        onLinkUpdated={(updated) => {
          setSelectedLink(updated);
          loadData();
        }}
        nodes={nodes}
        networkPolicies={networkPolicies}
        networkSettings={networkSettings}
        networkState={networkState}
        nodeStatuses={nodeStatuses}
      />
    </Box>
  );
};
