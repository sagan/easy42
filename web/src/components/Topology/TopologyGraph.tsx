import React, { useMemo, useCallback } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  MiniMap,
  useNodesState,
  useEdgesState,
  addEdge,
  Connection,
  Edge,
  Node as FlowNode,
  BackgroundVariant,
  OnNodeDrag,
} from '@xyflow/react';
import { Box, Typography } from '@mui/material';
import { Network } from 'lucide-react';
import { NodeCard } from './NodeCard';
import { CustomEdge } from './CustomEdge';
import { Node, Link, NodeStatus } from '../../types/api';

interface TopologyGraphProps {
  nodes: Node[];
  links: Link[];
  nodeStatuses: Record<string, NodeStatus>;
  onSelectNode: (node: Node) => void;
  onSelectLink: (link: Link) => void;
  onConnectNodes: (sourceName: string, targetName: string) => void;
  onNodePositionChange?: (name: string, x: number, y: number) => void;
}

const nodeTypes = {
  customNode: NodeCard,
};

const edgeTypes = {
  customEdge: CustomEdge,
};

export const TopologyGraph: React.FC<TopologyGraphProps> = ({
  nodes,
  links,
  nodeStatuses,
  onSelectNode,
  onSelectLink,
  onConnectNodes,
  onNodePositionChange,
}) => {
  // Convert easy42 nodes to React Flow nodes with circular/grid layout or saved coordinates
  const initialNodes: FlowNode[] = useMemo(() => {
    const total = nodes.length;
    const radius = Math.max(220, total * 60);
    const centerX = 500;
    const centerY = 350;

    return nodes.map((node, i) => {
      const angle = (i / (total || 1)) * 2 * Math.PI - Math.PI / 2;
      const defaultX = total === 1 ? centerX : centerX + radius * Math.cos(angle);
      const defaultY = total === 1 ? centerY : centerY + radius * Math.sin(angle);

      const x = typeof node.x === 'number' ? node.x : defaultX;
      const y = typeof node.y === 'number' ? node.y : defaultY;

      return {
        id: node.name,
        type: 'customNode',
        position: { x, y },
        data: {
          node,
          status: nodeStatuses[node.name],
          onSelect: onSelectNode,
        } as unknown as Record<string, unknown>,
      };
    });
  }, [nodes, nodeStatuses, onSelectNode]);

  // Convert easy42 links to React Flow edges
  const initialEdges: Edge[] = useMemo(() => {
    return links.map((link) => {
      const edgeId = `link-${link.from.name}-${link.to.name}`;
      return {
        id: edgeId,
        source: link.from.name,
        target: link.to.name,
        type: 'customEdge',
        data: {
          link,
          onSelect: onSelectLink,
        } as unknown as Record<string, unknown>,
      };
    });
  }, [links, onSelectLink]);

  const [flowNodes, setNodes, onNodesChange] = useNodesState(initialNodes);
  const [flowEdges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

  // Sync state when props change while preserving current node positions
  React.useEffect(() => {
    setNodes((currentNodes) => {
      const currentPosMap = new Map(currentNodes.map((n) => [n.id, n.position]));
      return initialNodes.map((n) => {
        const nodeData = (n.data as { node?: Node })?.node;
        if (typeof nodeData?.x === 'number' && typeof nodeData?.y === 'number') {
          return { ...n, position: { x: nodeData.x, y: nodeData.y } };
        }
        const existingPos = currentPosMap.get(n.id);
        if (existingPos) {
          return { ...n, position: existingPos };
        }
        return n;
      });
    });
  }, [initialNodes, setNodes]);

  React.useEffect(() => {
    setEdges(initialEdges);
  }, [initialEdges, setEdges]);

  const handleNodeDragStop: OnNodeDrag = useCallback(
    (_event, node, draggedNodes) => {
      const list = draggedNodes && draggedNodes.length > 0 ? draggedNodes : [node];
      for (const n of list) {
        const x = Math.round(n.position.x);
        const y = Math.round(n.position.y);
        onNodePositionChange?.(n.id, x, y);
      }
    },
    [onNodePositionChange]
  );

  const onConnect = useCallback(
    (connection: Connection) => {
      if (connection.source && connection.target && connection.source !== connection.target) {
        onConnectNodes(connection.source, connection.target);
      }
      setEdges((eds) => addEdge(connection, eds));
    },
    [onConnectNodes, setEdges]
  );

  return (
    <Box sx={{ width: '100%', height: 'calc(100vh - 64px)', position: 'relative', backgroundColor: '#F8FAFC' }}>
      {nodes.length === 0 ? (
        <Box
          sx={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 2,
            zIndex: 5,
            pointerEvents: 'none',
          }}
        >
          <Box
            sx={{
              width: 64,
              height: 64,
              borderRadius: 4,
              backgroundColor: 'rgba(79, 70, 229, 0.08)',
              border: '1px dashed rgba(79, 70, 229, 0.3)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Network size={32} color="#4F46E5" />
          </Box>
          <Typography variant="h6" sx={{ fontWeight: 700, color: '#0F172A' }}>
            No Nodes in Mesh
          </Typography>
          <Typography variant="body2" sx={{ color: '#64748B', maxWidth: 360, textAlign: 'center' }}>
            Click &quot;Add Node&quot; above to discover and connect your Linux servers via WireGuard.
          </Typography>
        </Box>
      ) : null}

      <ReactFlow
        nodes={flowNodes}
        edges={flowEdges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeDragStop={handleNodeDragStop}
        onConnect={onConnect}
        nodeTypes={nodeTypes}
        edgeTypes={edgeTypes}
        fitView
        attributionPosition="bottom-left"
      >
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
