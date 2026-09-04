import React, { useState, useEffect } from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  CircularProgress,
  Alert,
  MenuItem,
  Divider,
} from "@mui/material";
import { Link as LinkIcon, ArrowRightLeft, Edit2, Globe } from "lucide-react";
import { api } from "../../api/client";
import { Node, Link } from "../../types/api";
import { derivePortFromIP } from "../../utils/port";

interface AddLinkModalProps {
  open: boolean;
  nodes: Node[];
  initialFrom?: string;
  initialTo?: string;
  linkToEdit?: Link | null;
  onClose: () => void;
  onLinkAdded?: (link: Link) => void;
  onLinkUpdated?: (link: Link) => void;
  onNeedUnlock?: () => void;
}

export const AddLinkModal: React.FC<AddLinkModalProps> = ({
  open,
  nodes,
  initialFrom = "",
  initialTo = "",
  linkToEdit,
  onClose,
  onLinkAdded,
  onLinkUpdated,
  onNeedUnlock,
}) => {
  const [fromNodeName, setFromNodeName] = useState(initialFrom);
  const [toNodeName, setToNodeName] = useState(initialTo);
  const [fromPort, setFromPort] = useState<number>(0);
  const [toPort, setToPort] = useState<number>(0);
  const [fromMtu, setFromMtu] = useState<number>(1420);
  const [toMtu, setToMtu] = useState<number>(1420);

  // External peering custom fields
  const [localAddress, setLocalAddress] = useState("");
  const [remoteAddress, setRemoteAddress] = useState("");
  const [remoteEndpoint, setRemoteEndpoint] = useState("");
  const [remotePublicKey, setRemotePublicKey] = useState("");

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fromNode = nodes.find((n) => n.name === fromNodeName);
  const toNode = nodes.find((n) => n.name === toNodeName);
  const isExternalLink = Boolean(fromNode?.is_external || toNode?.is_external);
  const managedNode = fromNode?.is_external ? toNode : fromNode;
  const externalNode = fromNode?.is_external ? fromNode : toNode;

  useEffect(() => {
    if (!open) return;
    if (linkToEdit) {
      setFromNodeName(linkToEdit.from.name);
      setToNodeName(linkToEdit.to.name);
      setFromPort(linkToEdit.from.listen_port);
      setToPort(linkToEdit.to.listen_port);
      setFromMtu(linkToEdit.from.mtu || 1420);
      setToMtu(linkToEdit.to.mtu || 1420);

      const fNode = nodes.find((n) => n.name === linkToEdit.from.name);
      const tNode = nodes.find((n) => n.name === linkToEdit.to.name);

      if (fNode?.is_external) {
        setLocalAddress(linkToEdit.to.address || "fe80::1/64");
        setRemoteAddress(linkToEdit.from.address || "fe80::2/64");
        setRemoteEndpoint(linkToEdit.to.endpoint || linkToEdit.from.endpoint || "");
        setRemotePublicKey(linkToEdit.from.public_key || "");
      } else if (tNode?.is_external) {
        setLocalAddress(linkToEdit.from.address || "fe80::1/64");
        setRemoteAddress(linkToEdit.to.address || "fe80::2/64");
        setRemoteEndpoint(linkToEdit.from.endpoint || linkToEdit.to.endpoint || "");
        setRemotePublicKey(linkToEdit.to.public_key || "");
      } else {
        setLocalAddress("fe80::1/64");
        setRemoteAddress("fe80::2/64");
        setRemoteEndpoint("");
        setRemotePublicKey("");
      }
      setError(null);
    } else {
      setFromNodeName(initialFrom || "");
      setToNodeName(initialTo || "");
      setFromPort(0);
      setToPort(0);
      setFromMtu(1420);
      setToMtu(1420);
      setLocalAddress("fe80::1/64");
      setRemoteAddress("fe80::2/64");
      setRemoteEndpoint("");
      setRemotePublicKey("");
      setError(null);
    }
  }, [open, linkToEdit, initialFrom, initialTo, nodes]);

  // Auto calculate default ports and MTUs only when creating new link
  useEffect(() => {
    if (linkToEdit) return;

    if (isExternalLink) {
      if (managedNode === fromNode && fromPort === 0) {
        setFromPort(51820);
      }
      if (managedNode === toNode && toPort === 0) {
        setToPort(51820);
      }
      return;
    }

    if (toNode && toNode.ip) {
      setFromPort(derivePortFromIP(toNode.ip));
    }
    if (fromNode && fromNode.ip) {
      setToPort(derivePortFromIP(fromNode.ip));
    }

    const getUsedEpMTU = (targetNode?: Node, sourceNode?: Node) => {
      let foundMTU = 1500;
      if (targetNode?.entrypoints) {
        for (const ep of targetNode.entrypoints) {
          if (ep.ip && ep.mtu && ep.mtu > 0) {
            foundMTU = ep.mtu;
            break;
          }
        }
      }
      if (foundMTU === 1500 && sourceNode?.entrypoints) {
        for (const ep of sourceNode.entrypoints) {
          if (ep.ip && ep.mtu && ep.mtu > 0) {
            foundMTU = ep.mtu;
            break;
          }
        }
      }
      return foundMTU - 80;
    };

    if (fromNode || toNode) {
      setFromMtu(getUsedEpMTU(toNode, fromNode));
      setToMtu(getUsedEpMTU(fromNode, toNode));
    }
  }, [fromNode, toNode, linkToEdit, isExternalLink, managedNode, fromPort, toPort]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!fromNodeName || !toNodeName || fromNodeName === toNodeName) {
      setError("Please select two distinct nodes");
      return;
    }

    if (fromNode?.is_external && toNode?.is_external) {
      setError("Cannot link two external nodes together");
      return;
    }

    setSubmitting(true);
    setError(null);

    try {
      if (isExternalLink && managedNode && externalNode) {
        const managedListenPort = (managedNode === fromNode ? fromPort : toPort) || 51820;
        const managedMtuVal = (managedNode === fromNode ? fromMtu : toMtu) || 1420;

        const managedEnd = {
          name: managedNode.name,
          listen_port: managedListenPort,
          address: localAddress.trim() || "fe80::1/64",
          endpoint: remoteEndpoint.trim() || undefined,
          mtu: managedMtuVal,
        };

        const externalEnd = {
          name: externalNode.name,
          address: remoteAddress.trim() || "fe80::2/64",
          endpoint: remoteEndpoint.trim() || undefined,
          public_key: remotePublicKey.trim() || undefined,
          mtu: 1420,
        };

        const reqFrom = fromNode === managedNode ? managedEnd : externalEnd;
        const reqTo = toNode === managedNode ? managedEnd : externalEnd;

        if (linkToEdit) {
          const updated = await api.updateLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
          });
          onLinkUpdated?.(updated);
        } else {
          const link = await api.addLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
          });
          onLinkAdded?.(link);
        }
      } else {
        if (linkToEdit) {
          const updated = await api.updateLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from_port: fromPort || undefined,
            to_port: toPort || undefined,
            from_mtu: fromMtu || undefined,
            to_mtu: toMtu || undefined,
          });
          onLinkUpdated?.(updated);
        } else {
          const link = await api.addLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from_port: fromPort || undefined,
            to_port: toPort || undefined,
            from_mtu: fromMtu || undefined,
            to_mtu: toMtu || undefined,
          });
          onLinkAdded?.(link);
        }
      }
      onClose();
    } catch (err: unknown) {
      const e = err as Error & { status?: number };
      if (e.status === 423 && onNeedUnlock) {
        onClose();
        onNeedUnlock();
        return;
      }
      setError(e.message || (linkToEdit ? "Failed to update link" : "Failed to create link"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1.5, pb: 1, borderBottom: "1px solid #E2E8F0" }}>
        <Box
          sx={{
            width: 34,
            height: 34,
            borderRadius: 2,
            backgroundColor: isExternalLink ? "rgba(139, 92, 246, 0.1)" : "rgba(8, 145, 178, 0.1)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: isExternalLink ? "#8B5CF6" : "#0891B2",
          }}
        >
          {isExternalLink ? <Globe size={18} /> : linkToEdit ? <Edit2 size={18} /> : <LinkIcon size={18} />}
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
          {linkToEdit
            ? `Edit WireGuard Link: ${linkToEdit.from.name} ↔ ${linkToEdit.to.name}`
            : isExternalLink
              ? "Create External Peering Link"
              : "Create WireGuard Link"}
        </Typography>
      </DialogTitle>

      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: 2.5 }}>
          {/* Node Selection */}
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr auto 1fr", gap: 1.5, alignItems: "center" }}>
            <TextField
              select
              label="Source Node (From)"
              size="small"
              value={fromNodeName}
              onChange={(e) => setFromNodeName(e.target.value)}
              required
              disabled={submitting || Boolean(linkToEdit)}
            >
              {nodes.map((n) => (
                <MenuItem key={n.name} value={n.name} disabled={n.name === toNodeName}>
                  {n.name} {n.is_external ? "(External Peer)" : n.ip ? `(${n.ip})` : ""}
                </MenuItem>
              ))}
            </TextField>

            <Box
              sx={{
                width: 32,
                height: 32,
                borderRadius: "50%",
                backgroundColor: "#F1F5F9",
                border: "1px solid #E2E8F0",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: "#64748B",
              }}
            >
              <ArrowRightLeft size={16} />
            </Box>

            <TextField
              select
              label="Target Node (To)"
              size="small"
              value={toNodeName}
              onChange={(e) => setToNodeName(e.target.value)}
              required
              disabled={submitting || Boolean(linkToEdit)}
            >
              {nodes.map((n) => (
                <MenuItem key={n.name} value={n.name} disabled={n.name === fromNodeName}>
                  {n.name} {n.is_external ? "(External Peer)" : n.ip ? `(${n.ip})` : ""}
                </MenuItem>
              ))}
            </TextField>
          </Box>

          {linkToEdit && (
            <Typography variant="caption" sx={{ color: "#64748B", fontStyle: "italic", mt: -1.5 }}>
              Endpoints are fixed for this WireGuard link. You can configure listen ports and MTUs below.
            </Typography>
          )}

          <Divider sx={{ borderColor: "#E2E8F0" }} />

          {/* External Peering Configuration Form */}
          {isExternalLink && managedNode && externalNode ? (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Alert
                icon={<Globe size={18} />}
                severity="info"
                sx={{
                  borderRadius: 2,
                  backgroundColor: "#FAF5FF",
                  borderColor: "rgba(139, 92, 246, 0.3)",
                  color: "#5B21B6",
                  "& .MuiAlert-icon": { color: "#8B5CF6" },
                }}
              >
                External Peering Link: configuring WireGuard on managed node <strong>{managedNode.name}</strong> to peer
                with <strong>{externalNode.name}</strong> (AS{externalNode.asn}).
              </Alert>

              {/* Local Managed Node Configuration */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: "#EEF2FF",
                  border: "1px solid #C7D2FE",
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#3730A3", mb: 1.2 }}>
                  Local Interface: {managedNode.name}
                </Typography>
                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mb: 1.5 }}>
                  <TextField label="Interface Name" size="small" value={`wg42${externalNode.name}`} disabled />
                  <TextField
                    label="Local Listen Port"
                    type="number"
                    size="small"
                    value={managedNode === fromNode ? fromPort : toPort}
                    onChange={(e) => {
                      const val = Number(e.target.value);
                      if (managedNode === fromNode) setFromPort(val);
                      else setToPort(val);
                    }}
                    helperText="WireGuard listen port on your node"
                    required
                  />
                </Box>
                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
                  <TextField
                    label="Local WG Address"
                    size="small"
                    value={localAddress}
                    onChange={(e) => setLocalAddress(e.target.value)}
                    placeholder="e.g. fe80::1/64 or 172.20.x.x/32"
                    helperText="IP assigned to local interface"
                    required
                  />
                  <TextField
                    label="Interface MTU"
                    type="number"
                    size="small"
                    value={managedNode === fromNode ? fromMtu : toMtu}
                    onChange={(e) => {
                      const val = Number(e.target.value);
                      if (managedNode === fromNode) setFromMtu(val);
                      else setToMtu(val);
                    }}
                    helperText="Default: 1420"
                  />
                </Box>
              </Box>

              {/* External Peer Configuration */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: "#FAF5FF",
                  border: "1px solid #DDD6FE",
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#6D28D9", mb: 1.2 }}>
                  Remote Peer: {externalNode.name} (AS{externalNode.asn})
                </Typography>
                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mb: 1.5 }}>
                  <TextField
                    label="Remote Endpoint (host:port)"
                    size="small"
                    value={remoteEndpoint}
                    onChange={(e) => setRemoteEndpoint(e.target.value)}
                    placeholder="e.g. peer.example.com:51820"
                    helperText="Leave empty if peer connects to you dynamically"
                  />
                  <TextField
                    label="Remote WG Address"
                    size="small"
                    value={remoteAddress}
                    onChange={(e) => setRemoteAddress(e.target.value)}
                    placeholder="e.g. fe80::2/64 or 172.20.x.y/32"
                    helperText="Peer's WireGuard IP address"
                    required
                  />
                </Box>
                <TextField
                  fullWidth
                  size="small"
                  label="Remote WireGuard Public Key"
                  value={remotePublicKey}
                  onChange={(e) => setRemotePublicKey(e.target.value)}
                  placeholder="Base64 44-character public key from peer"
                  helperText="WireGuard public key of the external node"
                  required
                />
              </Box>
            </Box>
          ) : (
            /* Generated Standard Link Ends */
            fromNode &&
            toNode && (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, letterSpacing: "0.5px" }}>
                  WIREGUARD INTERFACE SPECIFICATIONS
                </Typography>

                {/* From Node End */}
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    backgroundColor: "#EEF2FF",
                    border: "1px solid #C7D2FE",
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#3730A3", mb: 1.2 }}>
                    End 1: {fromNode.name}
                  </Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: "1.2fr 1fr 1fr", gap: 1.5 }}>
                    <TextField label="Interface" size="small" value={`wg42${toNode.name}`} disabled />
                    <TextField
                      label="Listen Port"
                      type="number"
                      size="small"
                      value={fromPort}
                      onChange={(e) => setFromPort(Number(e.target.value))}
                      helperText={toNode.ip ? `Derived from ${toNode.ip}` : "Default port: 20000"}
                    />
                    <TextField
                      label="MTU"
                      type="number"
                      size="small"
                      value={fromMtu}
                      onChange={(e) => setFromMtu(Number(e.target.value))}
                      helperText="Default: 1420 (-80 overhead)"
                    />
                  </Box>
                </Box>

                {/* To Node End */}
                <Box
                  sx={{
                    p: 2,
                    borderRadius: 2,
                    backgroundColor: "#ECFEFF",
                    border: "1px solid #A5F3FC",
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0E7490", mb: 1.2 }}>
                    End 2: {toNode.name}
                  </Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: "1.2fr 1fr 1fr", gap: 1.5 }}>
                    <TextField label="Interface" size="small" value={`wg42${fromNode.name}`} disabled />
                    <TextField
                      label="Listen Port"
                      type="number"
                      size="small"
                      value={toPort}
                      onChange={(e) => setToPort(Number(e.target.value))}
                      helperText={fromNode.ip ? `Derived from ${fromNode.ip}` : "Default port: 20000"}
                    />
                    <TextField
                      label="MTU"
                      type="number"
                      size="small"
                      value={toMtu}
                      onChange={(e) => setToMtu(Number(e.target.value))}
                      helperText="Default: 1420 (-80 overhead)"
                    />
                  </Box>
                </Box>
              </Box>
            )
          )}

          {error && (
            <Alert severity="error" sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
          <Button onClick={onClose} disabled={submitting} sx={{ color: "#64748B" }}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="contained"
            color="primary"
            disabled={submitting || !fromNodeName || !toNodeName || fromNodeName === toNodeName}
            startIcon={submitting ? <CircularProgress size={16} color="inherit" /> : null}
          >
            {linkToEdit ? (submitting ? "Saving..." : "Save Changes") : submitting ? "Creating Link..." : "Create Link"}
          </Button>
        </DialogActions>
      </form>
    </Dialog>
  );
};
