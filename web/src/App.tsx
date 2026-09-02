import React, { useState, useEffect, useCallback } from 'react';
import { ThemeProvider, CssBaseline, Box, CircularProgress, Typography } from '@mui/material';
import { theme } from './theme';
import { api } from './api/client';
import { Node, Link, NodeStatus } from './types/api';
import { Navbar } from './components/Navbar';
import { TopologyGraph } from './components/Topology/TopologyGraph';
import { NodeDetailDrawer } from './components/Topology/NodeDetailDrawer';
import { LinkDetailDrawer } from './components/Topology/LinkDetailDrawer';
import { AddNodeModal } from './components/Modals/AddNodeModal';
import { AddLinkModal } from './components/Modals/AddLinkModal';
import { UnlockModal } from './components/Modals/UnlockModal';
import { SyncProgressModal } from './components/Modals/SyncProgressModal';
import { SettingsModal } from './components/Modals/SettingsModal';
import { LoginPage } from './components/Login/LoginPage';

export const App: React.FC = () => {
  const [checkingAuth, setCheckingAuth] = useState(true);
  const [authenticated, setAuthenticated] = useState(false);
  const [isUnlocked, setIsUnlocked] = useState(false);

  // Mesh State
  const [nodes, setNodes] = useState<Node[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [nodeStatuses, setNodeStatuses] = useState<Record<string, NodeStatus>>({});
  const [, setLoadingData] = useState(false);

  // Selected for drawers
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [selectedLink, setSelectedLink] = useState<Link | null>(null);

  // Modals
  const [addNodeOpen, setAddNodeOpen] = useState(false);
  const [nodeToEdit, setNodeToEdit] = useState<Node | null>(null);
  const [addLinkOpen, setAddLinkOpen] = useState(false);
  const [linkToEdit, setLinkToEdit] = useState<Link | null>(null);
  const [connectFrom, setConnectFrom] = useState<string>('');
  const [connectTo, setConnectTo] = useState<string>('');
  const [unlockOpen, setUnlockOpen] = useState(false);
  const [syncOpen, setSyncOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

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
      const [nodesData, linksData, statusesData] = await Promise.all([
        api.getNodes(),
        api.getLinks(),
        api.getNodeStatuses().catch(() => ({})),
      ]);
      setNodes(nodesData || []);
      setLinks(linksData || []);
      setNodeStatuses(statusesData || {});
    } catch {
      // Handled
    } finally {
      setLoadingData(false);
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

  const handleLogout = async () => {
    try {
      await api.logout();
      setAuthenticated(false);
      setIsUnlocked(false);
      setNodes([]);
      setLinks([]);
    } catch {
      // ignore
    }
  };

  const handleUnlockToggle = async () => {
    if (isUnlocked) {
      await api.lock();
      setIsUnlocked(false);
    } else {
      setUnlockOpen(true);
    }
  };

  const handleConnectNodes = useCallback((source: string, target: string) => {
    setConnectFrom(source);
    setConnectTo(target);
    setLinkToEdit(null);
    setAddLinkOpen(true);
  }, []);

  const handleSelectNode = useCallback((node: Node) => {
    setSelectedNode(node);
  }, []);

  const handleSelectLink = useCallback((link: Link) => {
    setSelectedLink(link);
  }, []);

  const handleNodePositionChange = useCallback(async (name: string, x: number, y: number) => {
    setNodes((prev) =>
      prev.map((n) => (n.name === name ? { ...n, x, y } : n))
    );
    setSelectedNode((prev) => (prev && prev.name === name ? { ...prev, x, y } : prev));
    try {
      await api.updateNodePosition(name, x, y);
    } catch (err) {
      console.error('Failed to persist node position:', err);
    }
  }, []);

  const handleEditNode = (node: Node) => {
    setNodeToEdit(node);
    setAddNodeOpen(true);
  };

  const handleEditLink = (link: Link) => {
    setLinkToEdit(link);
    setAddLinkOpen(true);
  };

  const handleNodeAdded = (newNode: Node) => {
    setNodes((prev) => [...prev, newNode]);
  };

  const handleNodeUpdated = (updatedNode: Node) => {
    const oldName = nodeToEdit?.name || updatedNode.name;
    setNodes((prev) => prev.map((n) => (n.name === oldName ? updatedNode : n)));
    if (selectedNode?.name === oldName) {
      setSelectedNode(updatedNode);
    }
    loadData();
  };

  const handleNodeDeleted = (name: string) => {
    setNodes((prev) => prev.filter((n) => n.name !== name));
    setLinks((prev) => prev.filter((l) => l.from.name !== name && l.to.name !== name));
    if (selectedNode?.name === name) {
      setSelectedNode(null);
    }
  };

  const handleLinkAdded = (newLink: Link) => {
    setLinks((prev) => [...prev, newLink]);
  };

  const handleLinkUpdated = (updatedLink: Link) => {
    setLinks((prev) =>
      prev.map((l) => {
        const matches =
          (l.from.name === updatedLink.from.name && l.to.name === updatedLink.to.name) ||
          (l.from.name === updatedLink.to.name && l.to.name === updatedLink.from.name);
        return matches ? updatedLink : l;
      })
    );
    if (
      selectedLink &&
      ((selectedLink.from.name === updatedLink.from.name && selectedLink.to.name === updatedLink.to.name) ||
        (selectedLink.from.name === updatedLink.to.name && selectedLink.to.name === updatedLink.from.name))
    ) {
      setSelectedLink(updatedLink);
    }
  };

  const handleLinkDeleted = (from: string, to: string) => {
    setLinks((prev) =>
      prev.filter(
        (l) =>
          !(
            (l.from.name === from && l.to.name === to) ||
            (l.from.name === to && l.to.name === from)
          )
      )
    );
    setSelectedLink(null);
  };

  const handleStatusRefreshed = (status: NodeStatus) => {
    setNodeStatuses((prev) => ({ ...prev, [status.name]: status }));
  };

  if (checkingAuth) {
    return (
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <Box
          sx={{
            minHeight: '100vh',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            backgroundColor: '#F8FAFC',
          }}
        >
          <CircularProgress size={32} color="primary" />
          <Typography variant="body1" sx={{ color: '#64748B' }}>
            Initializing easy42...
          </Typography>
        </Box>
      </ThemeProvider>
    );
  }

  if (!authenticated) {
    return (
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <LoginPage
          onLoginSuccess={() => {
            setAuthenticated(true);
            setIsUnlocked(true);
            loadData();
          }}
        />
      </ThemeProvider>
    );
  }

  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Box sx={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', backgroundColor: '#F8FAFC' }}>
        {/* Navigation Bar */}
        <Navbar
          nodeCount={nodes.length}
          linkCount={links.length}
          isUnlocked={isUnlocked}
          onAddNode={() => {
            setNodeToEdit(null);
            setAddNodeOpen(true);
          }}
          onAddLink={() => {
            setConnectFrom('');
            setConnectTo('');
            setLinkToEdit(null);
            setAddLinkOpen(true);
          }}
          onSync={() => setSyncOpen(true)}
          onUnlockToggle={handleUnlockToggle}
          onOpenSettings={() => setSettingsOpen(true)}
          onLogout={handleLogout}
        />

        {/* Visual Topology Editor */}
        <Box sx={{ flex: 1, position: 'relative' }}>
          <TopologyGraph
            nodes={nodes}
            links={links}
            nodeStatuses={nodeStatuses}
            onSelectNode={handleSelectNode}
            onSelectLink={handleSelectLink}
            onConnectNodes={handleConnectNodes}
            onNodePositionChange={handleNodePositionChange}
          />
        </Box>

        {/* Drawers */}
        <NodeDetailDrawer
          node={selectedNode}
          status={selectedNode ? nodeStatuses[selectedNode.name] : undefined}
          open={Boolean(selectedNode)}
          onClose={() => setSelectedNode(null)}
          onEditNode={handleEditNode}
          onNodeDeleted={handleNodeDeleted}
          onStatusRefreshed={handleStatusRefreshed}
        />

        <LinkDetailDrawer
          link={selectedLink}
          open={Boolean(selectedLink)}
          onClose={() => setSelectedLink(null)}
          onEditLink={handleEditLink}
          onLinkDeleted={handleLinkDeleted}
        />

        {/* Modals */}
        <AddNodeModal
          open={addNodeOpen}
          nodeToEdit={nodeToEdit}
          onClose={() => {
            setAddNodeOpen(false);
            setNodeToEdit(null);
          }}
          onNodeAdded={handleNodeAdded}
          onNodeUpdated={handleNodeUpdated}
        />

        <AddLinkModal
          open={addLinkOpen}
          nodes={nodes}
          initialFrom={connectFrom}
          initialTo={connectTo}
          linkToEdit={linkToEdit}
          onClose={() => {
            setAddLinkOpen(false);
            setLinkToEdit(null);
          }}
          onLinkAdded={handleLinkAdded}
          onLinkUpdated={handleLinkUpdated}
          onNeedUnlock={() => setUnlockOpen(true)}
        />

        <UnlockModal
          open={unlockOpen}
          onClose={() => setUnlockOpen(false)}
          onUnlocked={() => {
            setIsUnlocked(true);
            loadData();
          }}
        />

        <SyncProgressModal
          open={syncOpen}
          onClose={() => setSyncOpen(false)}
          onSyncComplete={() => loadData()}
          onNeedUnlock={() => setUnlockOpen(true)}
        />

        <SettingsModal
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          onLogoutAll={() => {
            setAuthenticated(false);
            setIsUnlocked(false);
            setNodes([]);
            setLinks([]);
            setSelectedNode(null);
            setSelectedLink(null);
          }}
        />
      </Box>
    </ThemeProvider>
  );
};
