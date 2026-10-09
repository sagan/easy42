import React, { useState, useCallback } from "react";
import { Box, Button, Select, MenuItem, FormControl, Tooltip } from "@mui/material";
import { Plus, Link as LinkIcon, Layers, Share2 } from "lucide-react";
import { useMesh } from "../context/MeshContext";
import { TopologyGraph } from "../components/Topology/TopologyGraph";
import { NodeDetailDrawer } from "../components/Topology/NodeDetailDrawer";
import { LinkDetailDrawer } from "../components/Topology/LinkDetailDrawer";
import { Node, Link } from "../types/api";
import { api } from "../api/client";
import { useNavigate } from "react-router-dom";

export const TopologyPage: React.FC = () => {
  const {
    displayedNodes,
    displayedLinks,
    displayedInternalNodes,
    missingMeshLinksCount,
    nodeStatuses,
    networkState,
    networkPolicies,
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
    setSyncTargetNode,
    setSyncOpen,
    addBlockTrigger,
    setAddBlockTrigger,
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
      try {
        await api.updateNodePosition(name, x, y);
      } catch (err) {
        console.error("Failed to persist node position:", err);
      }
    },
    [],
  );

  const handleEditNode = (node: Node) => {
    setNodeToEdit(node);
    setAddNodeOpen(true);
  };

  const handleOpenRenameNode = (node: Node) => {
    setNodeToRename(node);
    setRenameModalOpen(true);
  };

  const handleEditLink = (link: Link) => {
    setLinkToEdit(link);
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

  return (
    <Box sx={{ width: "100%", height: "100%", position: "relative", overflow: "hidden", display: "flex", flexDirection: "column" }}>
      {/* Floating Topology Toolbar */}
      <Box
        sx={{
          position: "absolute",
          top: { xs: 8, sm: 14 },
          left: { xs: 8, sm: 16 },
          right: { xs: 8, sm: "auto" },
          zIndex: 10,
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: 1,
          bgcolor: "rgba(255, 255, 255, 0.92)",
          backdropFilter: "blur(8px)",
          p: 1,
          borderRadius: 2.5,
          border: "1px solid #E2E8F0",
          boxShadow: "0 4px 12px rgba(0, 0, 0, 0.05)",
        }}
      >
        <Button
          size="small"
          variant="contained"
          startIcon={<Plus size={15} />}
          onClick={() => {
            if (!isUnlocked) {
              setUnlockOpen(true);
            } else {
              setNodeToEdit(null);
              setAddNodeOpen(true);
            }
          }}
          sx={{ borderRadius: 2, fontSize: "0.8rem", px: 1.5 }}
        >
          Node
        </Button>

        <Button
          size="small"
          variant="outlined"
          startIcon={<LinkIcon size={15} />}
          onClick={() => {
            if (!isUnlocked) {
              setUnlockOpen(true);
            } else {
              setLinkToEdit(null);
              setConnectFrom("");
              setConnectTo("");
              setAddLinkOpen(true);
            }
          }}
          sx={{ borderRadius: 2, fontSize: "0.8rem", px: 1.5 }}
        >
          Link
        </Button>

        <Button
          size="small"
          variant="outlined"
          startIcon={<Layers size={15} />}
          onClick={() => {
            if (!isUnlocked) {
              setUnlockOpen(true);
            } else {
              setAddBlockTrigger((c) => c + 1);
            }
          }}
          sx={{ borderRadius: 2, fontSize: "0.8rem", px: 1.5, display: { xs: "none", sm: "inline-flex" } }}
        >
          Block
        </Button>

        {displayedInternalNodes.length >= 2 && (
          <Tooltip title={missingMeshLinksCount > 0 ? `Add ${missingMeshLinksCount} missing links to create a full mesh` : "Mesh complete"}>
            <Button
              size="small"
              variant="outlined"
              color={missingMeshLinksCount > 0 ? "secondary" : "inherit"}
              startIcon={<Share2 size={15} />}
              onClick={handleCreateFullMesh}
              sx={{ borderRadius: 2, fontSize: "0.8rem", px: 1.5, display: { xs: "none", md: "inline-flex" } }}
            >
              Full Mesh {missingMeshLinksCount > 0 && `(${missingMeshLinksCount})`}
            </Button>
          </Tooltip>
        )}

        {/* Tag filter dropdown */}
        <FormControl size="small" sx={{ minWidth: 100 }}>
          <Select
            value={selectedTag}
            onChange={(e) => setSelectedTag(e.target.value)}
            sx={{
              height: 32,
              fontSize: "0.8rem",
              borderRadius: 2,
              bgcolor: "#FFFFFF",
            }}
          >
            <MenuItem value="All">All Tags</MenuItem>
            {uniqueTags.map((t) => (
              <MenuItem key={t} value={t}>
                {t}
              </MenuItem>
            ))}
          </Select>
        </FormControl>
      </Box>

      {/* Main Canvas */}
      <Box sx={{ flex: 1, position: "relative" }}>
        <TopologyGraph
          nodes={displayedNodes}
          links={displayedLinks}
          nodeStatuses={nodeStatuses}
          networkState={networkState}
          networkPolicies={networkPolicies}
          selectedTag={selectedTag}
          onSelectNode={handleSelectNode}
          onSelectLink={handleSelectLink}
          onConnectNodes={handleConnectNodes}
          onNodePositionChange={handleNodePositionChange}
          onRefreshNode={handleRefreshNode}
          refreshingNodeName={refreshingNodeName}
          addBlockTrigger={addBlockTrigger}
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
        onOpenHelper={(name) => navigate(`/helper?node=${encodeURIComponent(name)}`)}
        onOpenLookingGlass={(name) => navigate(`/looking-glass?node=${encodeURIComponent(name)}`)}
        onUpdateNodeState={(name) => handleUpdateState(name)}
        onSyncNode={(name) => {
          setSyncTargetNode(name);
          setSyncOpen(true);
        }}
        onNodeUpdated={() => loadData()}
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
        networkState={networkState}
      />
    </Box>
  );
};
