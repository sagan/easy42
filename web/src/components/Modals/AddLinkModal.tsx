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
  FormControlLabel,
  Switch,
  Chip,
  Tooltip,
  IconButton,
} from "@mui/material";
import { Link as LinkIcon, ArrowRightLeft, Edit2, Globe, Copy, Check } from "lucide-react";
import { api } from "../../api/client";
import { Node, Link, NetworkPolicy } from "../../types/api";
import { derivePortFromIP } from "../../utils/port";
import { resolvePeerEntrypoint, extractPort, formatEndpoint } from "../../utils/endpoint";

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
  const [fromUseIp, setFromUseIp] = useState<boolean>(false);
  const [toUseIp, setToUseIp] = useState<boolean>(false);
  const [networkPolicies, setNetworkPolicies] = useState<NetworkPolicy[]>([]);
  const [fromPolicy, setFromPolicy] = useState<string>("default");
  const [toPolicy, setToPolicy] = useState<string>("default");

  // External peering custom fields
  const [localAddress, setLocalAddress] = useState("");
  const [remoteAddress, setRemoteAddress] = useState("");
  const [remotePort, setRemotePort] = useState<number | "">("");
  const [remotePublicKey, setRemotePublicKey] = useState("");
  const [copiedEndpoint, setCopiedEndpoint] = useState(false);

  const copyEndpointToClipboard = (text: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedEndpoint(true);
    setTimeout(() => setCopiedEndpoint(false), 2000);
  };

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fromNode = nodes.find((n) => n.name === fromNodeName);
  const toNode = nodes.find((n) => n.name === toNodeName);
  const isExternalLink = Boolean(fromNode?.is_external || toNode?.is_external);
  const managedNode = fromNode?.is_external ? toNode : fromNode;
  const externalNode = fromNode?.is_external ? fromNode : toNode;

  // Resolve external peer's entrypoint using matching tags with managed node
  const { entrypoint: resolvedExtEP, matchedTag } = resolvePeerEntrypoint(managedNode, externalNode);
  const resolvedExtIp = resolvedExtEP?.ip || "";

  const fullRemoteEndpoint = formatEndpoint(
    resolvedExtIp,
    typeof remotePort === "number" ? remotePort : Number(remotePort),
  );

  const managedListenPort = (managedNode === fromNode ? fromPort : toPort) || 51820;
  const { entrypoint: managedEP } = resolvePeerEntrypoint(externalNode, managedNode);
  const managedPeerEndpoint = formatEndpoint(
    managedEP?.ip || managedNode?.host,
    managedListenPort,
  );

  useEffect(() => {
    if (!open) return;
    api.getNetworkPolicies().then(setNetworkPolicies).catch(console.error);
  }, [open]);

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
      const isExt = Boolean(fNode?.is_external || tNode?.is_external);

      setFromPolicy(linkToEdit.from.policy || (isExt ? "dn42" : "default"));
      setToPolicy(linkToEdit.to.policy || (isExt ? "dn42" : "default"));

      if (fNode?.is_external) {
        setLocalAddress(linkToEdit.to.address || "fe80::1/64");
        setRemoteAddress(linkToEdit.from.address || "fe80::2/64");
        const ep = linkToEdit.to.endpoint || linkToEdit.from.endpoint || "";
        setRemotePort(extractPort(ep) ?? "");
        setRemotePublicKey(linkToEdit.from.public_key || "");
      } else if (tNode?.is_external) {
        setLocalAddress(linkToEdit.from.address || "fe80::1/64");
        setRemoteAddress(linkToEdit.to.address || "fe80::2/64");
        const ep = linkToEdit.from.endpoint || linkToEdit.to.endpoint || "";
        setRemotePort(extractPort(ep) ?? "");
        setRemotePublicKey(linkToEdit.to.public_key || "");
      } else {
        setLocalAddress("fe80::1/64");
        setRemoteAddress("fe80::2/64");
        setRemotePort("");
        setRemotePublicKey("");
      }
      setFromUseIp(Boolean(linkToEdit.from.use_ip));
      setToUseIp(Boolean(linkToEdit.to.use_ip));
      setError(null);
    } else {
      setFromNodeName(initialFrom || "");
      setToNodeName(initialTo || "");
      setFromPort(0);
      setToPort(0);
      setFromMtu(1420);
      setToMtu(1420);
      const isExt = Boolean(nodes.find((n) => n.name === initialFrom)?.is_external || nodes.find((n) => n.name === initialTo)?.is_external);
      setFromPolicy(isExt ? "dn42" : "default");
      setToPolicy(isExt ? "dn42" : "default");
      setLocalAddress("fe80::1/64");
      setRemoteAddress("fe80::2/64");
      setRemotePort("");
      setRemotePublicKey("");
      setFromUseIp(false);
      setToUseIp(false);
      setError(null);
    }
  }, [open, linkToEdit, initialFrom, initialTo, nodes]);

  // Update default policies when external status changes for new link
  useEffect(() => {
    if (linkToEdit) return;
    if (isExternalLink) {
      setFromPolicy("dn42");
      setToPolicy("dn42");
    } else {
      setFromPolicy("default");
      setToPolicy("default");
    }
  }, [isExternalLink, linkToEdit]);

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

      // Auto prefill remotePort from external node's resolved entrypoint if available
      const { entrypoint: extEP } = resolvePeerEntrypoint(managedNode, externalNode);
      if (extEP?.ports && extEP.ports.length > 0) {
        const p = extEP.ports[0].external_port || extEP.ports[0].port;
        if (p) {
          setRemotePort(p);
        }
      }

      if (extEP?.mtu && extEP.mtu > 0) {
        if (managedNode === fromNode) setFromMtu(extEP.mtu - 80);
        else setToMtu(extEP.mtu - 80);
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
      const { entrypoint: matchedEP } = resolvePeerEntrypoint(sourceNode, targetNode);
      if (matchedEP?.mtu && matchedEP.mtu > 0) {
        return matchedEP.mtu - 80;
      }
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
  }, [fromNode, toNode, linkToEdit, isExternalLink, managedNode, externalNode, fromPort, toPort]);

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
        const managedMtuVal = (managedNode === fromNode ? fromMtu : toMtu) || 1420;
        const extEndpoint = fullRemoteEndpoint || undefined;
        const parsedRemotePort = typeof remotePort === "number" ? remotePort : (Number(remotePort) || 0);
        const managedPolicy = managedNode === fromNode ? fromPolicy : toPolicy;

        const managedEnd = {
          name: managedNode.name,
          listen_port: managedListenPort,
          address: localAddress.trim() || "fe80::1/64",
          endpoint: extEndpoint,
          mtu: managedMtuVal,
          use_ip: managedNode === fromNode ? fromUseIp : toUseIp,
          policy: managedPolicy,
        };

        const externalEnd = {
          name: externalNode.name,
          listen_port: parsedRemotePort,
          address: remoteAddress.trim() || "fe80::2/64",
          endpoint: managedPeerEndpoint || undefined,
          public_key: remotePublicKey.trim() || undefined,
          mtu: 1420,
          policy: "none",
        };

        const reqFrom = fromNode === managedNode ? managedEnd : externalEnd;
        const reqTo = toNode === managedNode ? managedEnd : externalEnd;

        if (linkToEdit) {
          const updated = await api.updateLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
            from_use_ip: fromUseIp,
            to_use_ip: toUseIp,
            from_policy: reqFrom.policy,
            to_policy: reqTo.policy,
          });
          onLinkUpdated?.(updated);
        } else {
          const link = await api.addLink({
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
            from_use_ip: fromUseIp,
            to_use_ip: toUseIp,
            from_policy: reqFrom.policy,
            to_policy: reqTo.policy,
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
            from_use_ip: fromUseIp,
            to_use_ip: toUseIp,
            from_policy: fromPolicy,
            to_policy: toPolicy,
            from: {
              ...linkToEdit.from,
              listen_port: fromPort || undefined,
              mtu: fromMtu || undefined,
              use_ip: fromUseIp,
              policy: fromPolicy,
            },
            to: {
              ...linkToEdit.to,
              listen_port: toPort || undefined,
              mtu: toMtu || undefined,
              use_ip: toUseIp,
              policy: toPolicy,
            },
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
            from_use_ip: fromUseIp,
            to_use_ip: toUseIp,
            from_policy: fromPolicy,
            to_policy: toPolicy,
            from: {
              use_ip: fromUseIp,
              policy: fromPolicy,
            },
            to: {
              use_ip: toUseIp,
              policy: toPolicy,
            },
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
                  <TextField label="Interface Name" size="small" value={`wg42-${externalNode.name}`} disabled />
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
                <FormControlLabel
                  control={
                    <Switch
                      size="small"
                      checked={managedNode === fromNode ? fromUseIp : toUseIp}
                      onChange={(e) => {
                        if (managedNode === fromNode) setFromUseIp(e.target.checked);
                        else setToUseIp(e.target.checked);
                      }}
                      color="primary"
                    />
                  }
                  label={
                    <Box>
                      <Typography variant="body2" sx={{ fontSize: "0.8rem", fontWeight: 600, color: "#1E293B" }}>
                        Use IP (Resolve external peer domain to IP)
                      </Typography>
                      <Typography variant="caption" sx={{ color: "#64748B", display: "block", fontSize: "0.72rem" }}>
                        Resolves remote endpoint hostname to IP in easy42 server and uses IP in {managedNode.name}'s WireGuard config.
                      </Typography>
                    </Box>
                  }
                  sx={{ alignItems: "flex-start", ml: 0, mt: 1.5 }}
                />

                <Box sx={{ mt: 1.5 }}>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Network Policy"
                    value={managedNode === fromNode ? fromPolicy : toPolicy}
                    onChange={(e) => {
                      if (managedNode === fromNode) setFromPolicy(e.target.value);
                      else setToPolicy(e.target.value);
                    }}
                    helperText="Firewall & BGP routing policy applied on this peering link (defaults to dn42)"
                  >
                    {networkPolicies.map((p) => (
                      <MenuItem key={p.id} value={p.id}>
                        {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                      </MenuItem>
                    ))}
                  </TextField>
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
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.2 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#6D28D9" }}>
                    Remote Peer: {externalNode.name} (AS{externalNode.asn})
                  </Typography>
                  {matchedTag && (
                    <Chip
                      label={`Tag: #${matchedTag}`}
                      size="small"
                      sx={{
                        height: 20,
                        fontSize: "0.65rem",
                        fontWeight: 700,
                        backgroundColor: "rgba(109, 40, 217, 0.1)",
                        color: "#6D28D9",
                        border: "1px solid rgba(109, 40, 217, 0.3)",
                      }}
                    />
                  )}
                </Box>

                <Box sx={{ display: "grid", gridTemplateColumns: "1.2fr 0.8fr", gap: 1.5, mb: 1.5 }}>
                  <TextField
                    label="Resolved Entrypoint (IP / Host)"
                    size="small"
                    value={resolvedExtIp || (externalNode ? "No Entrypoint Configured" : "")}
                    disabled
                    helperText={
                      matchedTag
                        ? `Matched tag: #${matchedTag} with ${managedNode?.name}`
                        : resolvedExtIp
                          ? "Resolved from external peer's entrypoints"
                          : "Configure an entrypoint on the external peer"
                    }
                    InputProps={{
                      sx: {
                        backgroundColor: "#FFFFFF",
                        fontWeight: 600,
                        color: resolvedExtIp ? "#6D28D9" : "#94A3B8",
                      },
                    }}
                  />
                  <TextField
                    label="Remote Port"
                    type="number"
                    size="small"
                    value={remotePort}
                    onChange={(e) => setRemotePort(e.target.value === "" ? "" : Number(e.target.value))}
                    placeholder="e.g. 51820"
                    helperText="WireGuard listen port (or leave empty)"
                  />
                </Box>

                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5, mb: 1.5 }}>
                  <TextField
                    label="Remote WG Address"
                    size="small"
                    value={remoteAddress}
                    onChange={(e) => setRemoteAddress(e.target.value)}
                    placeholder="e.g. fe80::2/64 or 172.20.x.y/32"
                    helperText="Peer's WireGuard IP address"
                    required
                  />
                  <TextField
                    size="small"
                    label="Remote WireGuard Public Key"
                    value={remotePublicKey}
                    onChange={(e) => setRemotePublicKey(e.target.value)}
                    placeholder="Base64 44-character key"
                    helperText="WireGuard public key of peer"
                    required
                  />
                </Box>

                {managedPeerEndpoint && (
                  <Box
                    sx={{
                      mt: 1.5,
                      p: 1.5,
                      borderRadius: 1.5,
                      bgcolor: "#FFFFFF",
                      border: "1px solid #DDD6FE",
                      display: "flex",
                      justifyContent: "space-between",
                      alignItems: "center",
                    }}
                  >
                    <Box>
                      <Typography variant="caption" sx={{ color: "#6D28D9", fontWeight: 700, display: "block" }}>
                        Peer Endpoint (Provide to External Peer):
                      </Typography>
                      <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                        {managedPeerEndpoint}
                      </Typography>
                    </Box>
                    <Tooltip title={copiedEndpoint ? "Copied!" : "Copy Endpoint"}>
                      <IconButton
                        size="small"
                        onClick={() => copyEndpointToClipboard(managedPeerEndpoint)}
                        sx={{ p: 0.5, color: copiedEndpoint ? "#10B981" : "#6D28D9" }}
                      >
                        {copiedEndpoint ? <Check size={15} /> : <Copy size={15} />}
                      </IconButton>
                    </Tooltip>
                  </Box>
                )}
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

                  <Box
                    sx={{
                      mt: 1.5,
                      p: 1.5,
                      borderRadius: 1.5,
                      backgroundColor: "rgba(255, 255, 255, 0.7)",
                      border: "1px solid #C7D2FE",
                      display: "flex",
                      flexDirection: "column",
                      gap: 1,
                    }}
                  >
                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                        Peer Endpoint:
                      </Typography>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                        <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                          {fromUseIp
                            ? linkToEdit?.from.resolved_endpoint || "(Resolves domain to IP on server)"
                            : linkToEdit?.from.endpoint ||
                              (resolvePeerEntrypoint(fromNode, toNode).entrypoint?.ip
                                ? `${resolvePeerEntrypoint(fromNode, toNode).entrypoint!.ip}:${toPort || (fromNode?.ip ? derivePortFromIP(fromNode.ip) : 20000)}`
                                : "Dynamic / Automatic")}
                        </Typography>
                        {fromUseIp && (
                          <Chip
                            label="IP"
                            size="small"
                            sx={{
                              height: 16,
                              fontSize: "0.6rem",
                              fontWeight: 800,
                              bgcolor: "rgba(16, 185, 129, 0.15)",
                              color: "#059669",
                              borderRadius: "4px",
                            }}
                          />
                        )}
                      </Box>
                    </Box>

                    <FormControlLabel
                      control={
                        <Switch
                          size="small"
                          checked={fromUseIp}
                          onChange={(e) => setFromUseIp(e.target.checked)}
                          color="primary"
                        />
                      }
                      label={
                        <Box>
                          <Typography variant="body2" sx={{ fontSize: "0.8rem", fontWeight: 600, color: "#1E293B" }}>
                            Use IP (Resolve peer endpoint domain to IP)
                          </Typography>
                          <Typography variant="caption" sx={{ color: "#64748B", display: "block", fontSize: "0.72rem" }}>
                            Resolves peer's endpoint hostname to IP in easy42 server and uses IP in {fromNode.name}'s WireGuard config.
                          </Typography>
                        </Box>
                      }
                      sx={{ alignItems: "flex-start", ml: 0, mt: 0.5 }}
                    />
                  </Box>

                  <Box sx={{ mt: 1.5 }}>
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="Network Policy"
                      value={fromPolicy}
                      onChange={(e) => setFromPolicy(e.target.value)}
                      helperText="Firewall & BGP routing policy applied on this endpoint"
                    >
                      {networkPolicies.map((p) => (
                        <MenuItem key={p.id} value={p.id}>
                          {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                        </MenuItem>
                      ))}
                    </TextField>
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

                  <Box
                    sx={{
                      mt: 1.5,
                      p: 1.5,
                      borderRadius: 1.5,
                      backgroundColor: "rgba(255, 255, 255, 0.7)",
                      border: "1px solid #A5F3FC",
                      display: "flex",
                      flexDirection: "column",
                      gap: 1,
                    }}
                  >
                    <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                        Peer Endpoint:
                      </Typography>
                      <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                        <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                          {toUseIp
                            ? linkToEdit?.to.resolved_endpoint || "(Resolves domain to IP on server)"
                            : linkToEdit?.to.endpoint ||
                              (resolvePeerEntrypoint(toNode, fromNode).entrypoint?.ip
                                ? `${resolvePeerEntrypoint(toNode, fromNode).entrypoint!.ip}:${fromPort || (toNode?.ip ? derivePortFromIP(toNode.ip) : 20000)}`
                                : "Dynamic / Automatic")}
                        </Typography>
                        {toUseIp && (
                          <Chip
                            label="IP"
                            size="small"
                            sx={{
                              height: 16,
                              fontSize: "0.6rem",
                              fontWeight: 800,
                              bgcolor: "rgba(16, 185, 129, 0.15)",
                              color: "#059669",
                              borderRadius: "4px",
                            }}
                          />
                        )}
                      </Box>
                    </Box>

                    <FormControlLabel
                      control={
                        <Switch
                          size="small"
                          checked={toUseIp}
                          onChange={(e) => setToUseIp(e.target.checked)}
                          color="primary"
                        />
                      }
                      label={
                        <Box>
                          <Typography variant="body2" sx={{ fontSize: "0.8rem", fontWeight: 600, color: "#1E293B" }}>
                            Use IP (Resolve peer endpoint domain to IP)
                          </Typography>
                          <Typography variant="caption" sx={{ color: "#64748B", display: "block", fontSize: "0.72rem" }}>
                            Resolves peer's endpoint hostname to IP in easy42 server and uses IP in {toNode.name}'s WireGuard config.
                          </Typography>
                        </Box>
                      }
                      sx={{ alignItems: "flex-start", ml: 0, mt: 0.5 }}
                    />
                  </Box>

                  <Box sx={{ mt: 1.5 }}>
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="Network Policy"
                      value={toPolicy}
                      onChange={(e) => setToPolicy(e.target.value)}
                      helperText="Firewall & BGP routing policy applied on this endpoint"
                    >
                      {networkPolicies.map((p) => (
                        <MenuItem key={p.id} value={p.id}>
                          {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                        </MenuItem>
                      ))}
                    </TextField>
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
