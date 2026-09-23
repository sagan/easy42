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
import { Link as LinkIcon, ArrowRightLeft, Edit2, Globe, Copy, Check, FileText, Network } from "lucide-react";
import { api } from "../../api/client";
import { Node, Link, NetworkPolicy } from "../../types/api";
import { MarkdownView } from "../Common/MarkdownView";
import { derivePortFromIP } from "../../utils/port";
import { resolvePeerEntrypoint, extractPort, formatEndpoint } from "../../utils/endpoint";

export function getInterfaceNameWithSuffix(peerName: string, suffix: string, isExternal?: boolean): string {
  const cleanName = (peerName || "").trim();
  const prefix = isExternal ? "wg42-" : "wg42";
  let maxPeerLen = isExternal ? 10 : 11;
  if (suffix) {
    maxPeerLen -= suffix.length;
    if (maxPeerLen < 0) maxPeerLen = 0;
  }
  const truncated = cleanName.slice(0, maxPeerLen);
  return `${prefix}${truncated}${suffix}`;
}

interface AddLinkModalProps {
  open: boolean;
  nodes: Node[];
  links?: Link[];
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
  links = [],
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
  const [fromCost, setFromCost] = useState<number | string>("");
  const [toCost, setToCost] = useState<number | string>("");
  const [fromFwmark, setFromFwmark] = useState<string>("");
  const [toFwmark, setToFwmark] = useState<string>("");
  const [fromPreference, setFromPreference] = useState<number | string>("");
  const [toPreference, setToPreference] = useState<number | string>("");
  const [fromMark, setFromMark] = useState<string>("");
  const [toMark, setToMark] = useState<string>("");
  const [fromRoutingPolicy, setFromRoutingPolicy] = useState<string>("");
  const [toRoutingPolicy, setToRoutingPolicy] = useState<string>("");
  const [fromNote, setFromNote] = useState<string>("");
  const [toNote, setToNote] = useState<string>("");
  const [fromNoteTab, setFromNoteTab] = useState<"write" | "preview">("write");
  const [toNoteTab, setToNoteTab] = useState<"write" | "preview">("write");
  const [assignIPv4, setAssignIPv4] = useState<boolean>(false);

  // Manual link fields
  const [linkType, setLinkType] = useState<"wireguard" | "manual">("wireguard");
  const [fromInterface, setFromInterface] = useState<string>("");
  const [toInterface, setToInterface] = useState<string>("");
  const [fromAddress, setFromAddress] = useState<string>("");
  const [toAddress, setToAddress] = useState<string>("");
  const [fromNeighborAddress, setFromNeighborAddress] = useState<string>("");
  const [toNeighborAddress, setToNeighborAddress] = useState<string>("");

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

  // Derive interface suffix and port offset for new link
  const existingLinksBetween = (links || []).filter(
    (l) =>
      (l.from.name === fromNodeName && l.to.name === toNodeName) ||
      (l.from.name === toNodeName && l.to.name === fromNodeName),
  );
  const linkSuffix = linkToEdit ? "" : existingLinksBetween.length > 0 ? `${existingLinksBetween.length}` : "";
  const linkPortOffset = linkToEdit ? 0 : existingLinksBetween.length;

  const managedListenPort = (managedNode === fromNode ? fromPort : toPort) || (51820 + linkPortOffset);
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
      const isMan = linkToEdit.type === "manual" || linkToEdit.from?.type === "manual" || linkToEdit.to?.type === "manual";
      setLinkType(isMan ? "manual" : "wireguard");
      setFromInterface(linkToEdit.from?.interface || "");
      setToInterface(linkToEdit.to?.interface || "");
      setFromAddress(linkToEdit.from?.address || "");
      setToAddress(linkToEdit.to?.address || "");
      setFromNeighborAddress(linkToEdit.from?.neighbor_address || "");
      setToNeighborAddress(linkToEdit.to?.neighbor_address || "");

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
      setFromCost(linkToEdit.from.cost !== undefined && linkToEdit.from.cost !== 0 ? linkToEdit.from.cost : "");
      setToCost(linkToEdit.to.cost !== undefined && linkToEdit.to.cost !== 0 ? linkToEdit.to.cost : "");
      setFromFwmark(linkToEdit.from.fwmark || "");
      setToFwmark(linkToEdit.to.fwmark || "");
      setFromPreference(linkToEdit.from.preference !== undefined ? linkToEdit.from.preference : "");
      setToPreference(linkToEdit.to.preference !== undefined ? linkToEdit.to.preference : "");
      setFromMark(linkToEdit.from.mark || "");
      setToMark(linkToEdit.to.mark || "");
      setFromRoutingPolicy(linkToEdit.from.routing_policy || "");
      setToRoutingPolicy(linkToEdit.to.routing_policy || "");
      setFromNote(linkToEdit.from.note || "");
      setToNote(linkToEdit.to.note || "");
      setAssignIPv4(Boolean(linkToEdit.assign_ipv4));
      setError(null);
    } else {
      setLinkType("wireguard");
      setAssignIPv4(false);
      setFromInterface("");
      setToInterface("");
      setFromAddress("");
      setToAddress("");
      setFromNeighborAddress("");
      setToNeighborAddress("");

      setFromNodeName(initialFrom || "");
      setToNodeName(initialTo || "");
      setFromPort(0);
      setToPort(0);
      setFromMtu(1420);
      setToMtu(1420);
      const isExt = Boolean(nodes.find((n) => n.name === initialFrom)?.is_external || nodes.find((n) => n.name === initialTo)?.is_external);
      setFromPolicy(isExt ? "dn42" : "default");
      setToPolicy(isExt ? "dn42" : "default");
      setFromCost("");
      setToCost("");
      setFromFwmark("");
      setToFwmark("");
      setFromPreference("");
      setToPreference("");
      setFromMark("");
      setToMark("");
      setFromRoutingPolicy("");
      setToRoutingPolicy("");
      setFromNote("");
      setToNote("");
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
        setFromPort(51820 + linkPortOffset);
      }
      if (managedNode === toNode && toPort === 0) {
        setToPort(51820 + linkPortOffset);
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
      setFromPort(derivePortFromIP(toNode.ip) + linkPortOffset);
    }
    if (fromNode && fromNode.ip) {
      setToPort(derivePortFromIP(fromNode.ip) + linkPortOffset);
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

    const parsedFromCost = fromCost === "" ? 0 : Number(fromCost);
    const parsedToCost = toCost === "" ? 0 : Number(toCost);
    const parsedFromFwmark = fromFwmark.trim() || undefined;
    const parsedToFwmark = toFwmark.trim() || undefined;
    const parsedFromPreference = fromPreference === "" ? undefined : Number(fromPreference);
    const parsedToPreference = toPreference === "" ? undefined : Number(toPreference);
    const parsedFromMark = fromMark.trim() || undefined;
    const parsedToMark = toMark.trim() || undefined;

    try {
      if (linkType === "manual") {
        if (!fromInterface.trim() || !toInterface.trim()) {
          setError("Both endpoints must specify an interface name for a manual link");
          setSubmitting(false);
          return;
        }
        if (!fromAddress.trim() || !toAddress.trim()) {
          setError("Both endpoints must specify a local IP address for a manual link");
          setSubmitting(false);
          return;
        }

        const reqFrom = {
          name: fromNodeName,
          type: "manual",
          interface: fromInterface.trim(),
          address: fromAddress.trim(),
          neighbor_address: fromNeighborAddress.trim() || undefined,
          policy: fromPolicy,
          routing_policy: fromRoutingPolicy || (linkToEdit ? "inherit" : undefined),
          cost: parsedFromCost,
          fwmark: parsedFromFwmark,
          preference: parsedFromPreference,
          mark: parsedFromMark,
          note: fromNote.trim() || undefined,
        };

        const reqTo = {
          name: toNodeName,
          type: "manual",
          interface: toInterface.trim(),
          address: toAddress.trim(),
          neighbor_address: toNeighborAddress.trim() || undefined,
          policy: toPolicy,
          routing_policy: toRoutingPolicy || (linkToEdit ? "inherit" : undefined),
          cost: parsedToCost,
          fwmark: parsedToFwmark,
          preference: parsedToPreference,
          mark: parsedToMark,
          note: toNote.trim() || undefined,
        };

        if (linkToEdit) {
          const updated = await api.updateLink({
            type: "manual",
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
            from_policy: reqFrom.policy,
            to_policy: reqTo.policy,
            from_routing_policy: reqFrom.routing_policy,
            to_routing_policy: reqTo.routing_policy,
            from_cost: reqFrom.cost,
            to_cost: reqTo.cost,
            from_fwmark: reqFrom.fwmark,
            to_fwmark: reqTo.fwmark,
            from_preference: reqFrom.preference,
            to_preference: reqTo.preference,
            from_mark: reqFrom.mark,
            to_mark: reqTo.mark,
            from_note: reqFrom.note || "",
            to_note: reqTo.note || "",
          });
          onLinkUpdated?.(updated);
        } else {
          const link = await api.addLink({
            type: "manual",
            from_node: fromNodeName,
            to_node: toNodeName,
            from: reqFrom,
            to: reqTo,
            from_policy: reqFrom.policy,
            to_policy: reqTo.policy,
            from_routing_policy: reqFrom.routing_policy,
            to_routing_policy: reqTo.routing_policy,
            from_cost: reqFrom.cost,
            to_cost: reqTo.cost,
            from_fwmark: reqFrom.fwmark,
            to_fwmark: reqTo.fwmark,
            from_preference: reqFrom.preference,
            to_preference: reqTo.preference,
            from_mark: reqFrom.mark,
            to_mark: reqTo.mark,
            from_note: reqFrom.note,
            to_note: reqTo.note,
          });
          onLinkAdded?.(link);
        }
        onClose();
        return;
      } else if (isExternalLink && managedNode && externalNode) {
        const managedMtuVal = (managedNode === fromNode ? fromMtu : toMtu) || 1420;
        const extEndpoint = fullRemoteEndpoint || undefined;
        const parsedRemotePort = typeof remotePort === "number" ? remotePort : (Number(remotePort) || 0);
        const managedPolicy = managedNode === fromNode ? fromPolicy : toPolicy;
        const managedFwmark = managedNode === fromNode ? parsedFromFwmark : parsedToFwmark;
        const managedPreference = managedNode === fromNode ? parsedFromPreference : parsedToPreference;
        const managedMark = managedNode === fromNode ? parsedFromMark : parsedToMark;
        const managedRoutingPolicy = managedNode === fromNode ? fromRoutingPolicy : toRoutingPolicy;
        const externalFwmark = externalNode === fromNode ? parsedFromFwmark : parsedToFwmark;
        const externalPreference = externalNode === fromNode ? parsedFromPreference : parsedToPreference;
        const externalMark = externalNode === fromNode ? parsedFromMark : parsedToMark;
        const externalRoutingPolicy = externalNode === fromNode ? fromRoutingPolicy : toRoutingPolicy;

        const managedEnd = {
          name: managedNode.name,
          listen_port: managedListenPort,
          address: localAddress.trim() || "fe80::1/64",
          endpoint: extEndpoint,
          mtu: managedMtuVal,
          use_ip: managedNode === fromNode ? fromUseIp : toUseIp,
          policy: managedPolicy,
          routing_policy: managedRoutingPolicy || undefined,
          cost: managedNode === fromNode ? parsedFromCost : parsedToCost,
          fwmark: managedFwmark,
          preference: managedPreference,
          mark: managedMark,
          note: (managedNode === fromNode ? fromNote : toNote).trim() || undefined,
        };

        const externalEnd = {
          name: externalNode.name,
          listen_port: parsedRemotePort,
          address: remoteAddress.trim() || "fe80::2/64",
          endpoint: managedPeerEndpoint || undefined,
          public_key: remotePublicKey.trim() || undefined,
          mtu: 1420,
          policy: "none",
          routing_policy: externalRoutingPolicy || undefined,
          cost: externalNode === fromNode ? parsedFromCost : parsedToCost,
          fwmark: externalFwmark,
          preference: externalPreference,
          mark: externalMark,
          note: (externalNode === fromNode ? fromNote : toNote).trim() || undefined,
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
            from_routing_policy: reqFrom.routing_policy || (linkToEdit ? "inherit" : undefined),
            to_routing_policy: reqTo.routing_policy || (linkToEdit ? "inherit" : undefined),
            from_cost: reqFrom.cost,
            to_cost: reqTo.cost,
            from_fwmark: reqFrom.fwmark,
            to_fwmark: reqTo.fwmark,
            from_preference: reqFrom.preference,
            to_preference: reqTo.preference,
            from_mark: reqFrom.mark,
            to_mark: reqTo.mark,
            from_note: reqFrom.note || "",
            to_note: reqTo.note || "",
            assign_ipv4: assignIPv4,
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
            from_routing_policy: reqFrom.routing_policy,
            to_routing_policy: reqTo.routing_policy,
            from_cost: reqFrom.cost,
            to_cost: reqTo.cost,
            from_fwmark: reqFrom.fwmark,
            to_fwmark: reqTo.fwmark,
            from_preference: reqFrom.preference,
            to_preference: reqTo.preference,
            from_mark: reqFrom.mark,
            to_mark: reqTo.mark,
            from_note: reqFrom.note,
            to_note: reqTo.note,
            assign_ipv4: assignIPv4,
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
            from_routing_policy: fromRoutingPolicy ? fromRoutingPolicy : (linkToEdit ? "inherit" : undefined),
            to_routing_policy: toRoutingPolicy ? toRoutingPolicy : (linkToEdit ? "inherit" : undefined),
            from_cost: parsedFromCost,
            to_cost: parsedToCost,
            from_fwmark: parsedFromFwmark,
            to_fwmark: parsedToFwmark,
            from_preference: parsedFromPreference,
            to_preference: parsedToPreference,
            from_mark: parsedFromMark,
            to_mark: parsedToMark,
            from_note: fromNote.trim(),
            to_note: toNote.trim(),
            from: {
              ...linkToEdit.from,
              listen_port: fromPort || undefined,
              mtu: fromMtu || undefined,
              use_ip: fromUseIp,
              policy: fromPolicy,
              routing_policy: fromRoutingPolicy ? fromRoutingPolicy : "inherit",
              cost: parsedFromCost,
              fwmark: parsedFromFwmark,
              preference: parsedFromPreference,
              mark: parsedFromMark,
              endpoint: undefined,
              resolved_endpoint: undefined,
              note: fromNote.trim() || undefined,
            },
            to: {
              ...linkToEdit.to,
              listen_port: toPort || undefined,
              mtu: toMtu || undefined,
              use_ip: toUseIp,
              policy: toPolicy,
              routing_policy: toRoutingPolicy ? toRoutingPolicy : "inherit",
              cost: parsedToCost,
              fwmark: parsedToFwmark,
              preference: parsedToPreference,
              mark: parsedToMark,
              endpoint: undefined,
              resolved_endpoint: undefined,
              note: toNote.trim() || undefined,
            },
            assign_ipv4: assignIPv4,
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
            from_routing_policy: fromRoutingPolicy || undefined,
            to_routing_policy: toRoutingPolicy || undefined,
            from_cost: parsedFromCost,
            to_cost: parsedToCost,
            from_fwmark: parsedFromFwmark,
            to_fwmark: parsedToFwmark,
            from_preference: parsedFromPreference,
            to_preference: parsedToPreference,
            from_mark: parsedFromMark,
            to_mark: parsedToMark,
            from_note: fromNote.trim() || undefined,
            to_note: toNote.trim() || undefined,
            assign_ipv4: assignIPv4,
            from: {
              use_ip: fromUseIp,
              policy: fromPolicy,
              routing_policy: fromRoutingPolicy || undefined,
              cost: parsedFromCost,
              fwmark: parsedFromFwmark,
              preference: parsedFromPreference,
              mark: parsedFromMark,
              note: fromNote.trim() || undefined,
            },
            to: {
              use_ip: toUseIp,
              policy: toPolicy,
              routing_policy: toRoutingPolicy || undefined,
              cost: parsedToCost,
              fwmark: parsedToFwmark,
              preference: parsedToPreference,
              mark: parsedToMark,
              note: toNote.trim() || undefined,
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
            backgroundColor: isExternalLink
              ? "rgba(139, 92, 246, 0.1)"
              : linkType === "manual"
              ? "rgba(124, 58, 237, 0.1)"
              : "rgba(8, 145, 178, 0.1)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: isExternalLink ? "#8B5CF6" : linkType === "manual" ? "#7C3AED" : "#0891B2",
          }}
        >
          {isExternalLink ? <Globe size={18} /> : linkType === "manual" ? <Network size={18} /> : linkToEdit ? <Edit2 size={18} /> : <LinkIcon size={18} />}
        </Box>
        <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A" }}>
          {linkToEdit
            ? (linkType === "manual" ? `Edit Manual Link: ${linkToEdit.from.name} ↔ ${linkToEdit.to.name}` : `Edit WireGuard Link: ${linkToEdit.from.name} ↔ ${linkToEdit.to.name}`)
            : isExternalLink
              ? "Create External Peering Link"
              : linkType === "manual"
                ? "Create Manual (BGP Only) Link"
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
              {linkType === "manual"
                ? "Editing manual link interfaces and peering settings."
                : "Endpoints are fixed for this WireGuard link. You can configure listen ports and MTUs below."}
            </Typography>
          )}

          {/* Link Type Selector (when not external link) */}
          {!isExternalLink && (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, letterSpacing: "0.5px" }}>
                LINK TYPE
              </Typography>
              <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
                <Box
                  id="link-type-wireguard"
                  onClick={() => !linkToEdit && setLinkType("wireguard")}
                  sx={{
                    p: 1.5,
                    borderRadius: 2,
                    border: "2px solid",
                    borderColor: linkType === "wireguard" ? "#0891B2" : "#E2E8F0",
                    backgroundColor: linkType === "wireguard" ? "rgba(8, 145, 178, 0.05)" : "#F8FAFC",
                    cursor: linkToEdit ? "default" : "pointer",
                    transition: "all 0.15s ease",
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                    <Box
                      sx={{
                        width: 10,
                        height: 10,
                        borderRadius: "50%",
                        backgroundColor: linkType === "wireguard" ? "#0891B2" : "#CBD5E1",
                      }}
                    />
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: linkType === "wireguard" ? "#0E7490" : "#475569" }}>
                      WireGuard (Managed)
                    </Typography>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", lineHeight: 1.3 }}>
                    Automatically manages WireGuard interface, crypto keys, endpoints & BGP peering.
                  </Typography>
                </Box>

                <Box
                  id="link-type-manual"
                  onClick={() => !linkToEdit && setLinkType("manual")}
                  sx={{
                    p: 1.5,
                    borderRadius: 2,
                    border: "2px solid",
                    borderColor: linkType === "manual" ? "#7C3AED" : "#E2E8F0",
                    backgroundColor: linkType === "manual" ? "rgba(124, 58, 237, 0.05)" : "#F8FAFC",
                    cursor: linkToEdit ? "default" : "pointer",
                    transition: "all 0.15s ease",
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                    <Box
                      sx={{
                        width: 10,
                        height: 10,
                        borderRadius: "50%",
                        backgroundColor: linkType === "manual" ? "#7C3AED" : "#CBD5E1",
                      }}
                    />
                    <Typography variant="subtitle2" sx={{ fontWeight: 700, color: linkType === "manual" ? "#6D28D9" : "#475569" }}>
                      Manual Link (BGP Only)
                    </Typography>
                  </Box>
                  <Typography variant="caption" sx={{ color: "#64748B", display: "block", lineHeight: 1.3 }}>
                    Data plane managed manually (ethernet/tunnel). easy42 manages BGP peering & nftables.
                  </Typography>
                </Box>
              </Box>
            </Box>
          )}

          <Divider sx={{ borderColor: "#E2E8F0" }} />

          {/* WireGuard Options: Assign IPv4 Link-Local */}
          {linkType === "wireguard" && (
            <Box
              sx={{
                p: 1.75,
                borderRadius: 2,
                backgroundColor: assignIPv4 ? "rgba(8, 145, 178, 0.05)" : "#F8FAFC",
                border: "1px solid",
                borderColor: assignIPv4 ? "rgba(8, 145, 178, 0.35)" : "#E2E8F0",
                transition: "all 0.2s ease",
              }}
            >
              <FormControlLabel
                control={
                  <Switch
                    id="link-assign-ipv4-switch"
                    size="small"
                    checked={assignIPv4}
                    onChange={(e) => setAssignIPv4(e.target.checked)}
                    color="primary"
                  />
                }
                label={
                  <Box>
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography variant="body2" sx={{ fontSize: "0.85rem", fontWeight: 700, color: "#1E293B" }}>
                        Assign IPv4
                      </Typography>
                      <Chip
                        label={assignIPv4 ? "169.254.X.X/32 Enabled" : "Optional"}
                        size="small"
                        sx={{
                          height: 18,
                          fontSize: "0.62rem",
                          fontWeight: 800,
                          bgcolor: assignIPv4 ? "rgba(8, 145, 178, 0.15)" : "#E2E8F0",
                          color: assignIPv4 ? "#0891B2" : "#64748B",
                          borderRadius: "4px",
                        }}
                      />
                    </Box>
                    <Typography variant="caption" sx={{ color: "#64748B", display: "block", fontSize: "0.72rem", lineHeight: 1.4, mt: 0.3 }}>
                      Assigns a pair of IPv4 link-local addresses (169.254.X.X/32 derived automatically from the peer's main IP and link index) to the WireGuard interface of link ends. Intended for nftables IPv4 masquerading; BIRD BGP peering still uses IPv6 link-local addresses exclusively.
                    </Typography>
                  </Box>
                }
                sx={{ alignItems: "flex-start", ml: 0, m: 0 }}
              />
            </Box>
          )}

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
                  <TextField
                    label="Interface Name"
                    size="small"
                    value={
                      linkToEdit
                        ? (managedNode === fromNode ? linkToEdit.from.interface : linkToEdit.to.interface)
                        : getInterfaceNameWithSuffix(externalNode.name, linkSuffix, true)
                    }
                    disabled
                  />
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

                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
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
                    helperText="Firewall & BGP routing policy applied on this peering link"
                  >
                    {networkPolicies.map((p) => (
                      <MenuItem key={p.id} value={p.id}>
                        {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                      </MenuItem>
                    ))}
                  </TextField>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Routing Policy (Optional)"
                    value={managedNode === fromNode ? fromRoutingPolicy : toRoutingPolicy}
                    onChange={(e) => {
                      if (managedNode === fromNode) setFromRoutingPolicy(e.target.value);
                      else setToRoutingPolicy(e.target.value);
                    }}
                    helperText="Overrides policy routing policy"
                  >
                    <MenuItem value="">Policy default</MenuItem>
                    <MenuItem value="full">Full (Default) — All valid routes</MenuItem>
                    <MenuItem value="stub">Stub — Receive all, only send local</MenuItem>
                    <MenuItem value="receive_only">Receive Only — Import only</MenuItem>
                    <MenuItem value="advertise_only">Advertise Only — Export only</MenuItem>
                  </TextField>
                  <TextField
                    fullWidth
                    size="small"
                    label="Link Cost (Optional)"
                    type="number"
                    value={managedNode === fromNode ? fromCost : toCost}
                    onChange={(e) => {
                      const val = e.target.value === "" ? "" : Number(e.target.value);
                      if (managedNode === fromNode) setFromCost(val);
                      else setToCost(val);
                    }}
                    placeholder="Policy default"
                    helperText="Overrides policy cost if set (non-zero)"
                  />
                </Box>
                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
                  <TextField
                    fullWidth
                    size="small"
                    label="WireGuard FwMark (Optional)"
                    value={managedNode === fromNode ? fromFwmark : toFwmark}
                    onChange={(e) => {
                      const val = e.target.value;
                      if (managedNode === fromNode) setFromFwmark(val);
                      else setToFwmark(val);
                    }}
                    placeholder="Policy default"
                    helperText="Overrides policy WireGuard FwMark"
                  />
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label="BGP Preference (Optional)"
                    value={managedNode === fromNode ? fromPreference : toPreference}
                    onChange={(e) => {
                      const val = e.target.value === "" ? "" : Number(e.target.value);
                      if (managedNode === fromNode) setFromPreference(val);
                      else setToPreference(val);
                    }}
                    placeholder="Policy default"
                    helperText="Overrides policy BGP preference"
                  />
                  <TextField
                    fullWidth
                    size="small"
                    label="Netfilter Mark (Optional)"
                    value={managedNode === fromNode ? fromMark : toMark}
                    onChange={(e) => {
                      const val = e.target.value;
                      if (managedNode === fromNode) setFromMark(val);
                      else setToMark(val);
                    }}
                    placeholder="Policy default"
                    helperText="Overrides policy netfilter mark"
                  />
                </Box>

                {/* Managed Node Endpoint Note */}
                <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                    <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                      <FileText size={13} color="#4F46E5" /> {managedNode?.name} ENDPOINT NOTE (MARKDOWN)
                    </Typography>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      <Button
                        size="small"
                        variant={(managedNode === fromNode ? fromNoteTab : toNoteTab) === "write" ? "contained" : "text"}
                        onClick={() => (managedNode === fromNode ? setFromNoteTab("write") : setToNoteTab("write"))}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Write
                      </Button>
                      <Button
                        size="small"
                        variant={(managedNode === fromNode ? fromNoteTab : toNoteTab) === "preview" ? "contained" : "text"}
                        onClick={() => (managedNode === fromNode ? setFromNoteTab("preview") : setToNoteTab("preview"))}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Preview
                      </Button>
                    </Box>
                  </Box>
                  {(managedNode === fromNode ? fromNoteTab : toNoteTab) === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={5}
                      size="small"
                      placeholder="Markdown note for this endpoint..."
                      value={managedNode === fromNode ? fromNote : toNote}
                      onChange={(e) => (managedNode === fromNode ? setFromNote(e.target.value) : setToNote(e.target.value))}
                      disabled={submitting}
                      sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                    />
                  ) : (
                    <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#FFFFFF", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                      <MarkdownView content={managedNode === fromNode ? fromNote : toNote} emptyText="No note written yet" />
                    </Box>
                  )}
                </Box>

                {/* External Peer Endpoint Note */}
                <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                    <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                      <FileText size={13} color="#4F46E5" /> {externalNode?.name} ENDPOINT NOTE (MARKDOWN)
                    </Typography>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      <Button
                        size="small"
                        variant={(externalNode === fromNode ? fromNoteTab : toNoteTab) === "write" ? "contained" : "text"}
                        onClick={() => (externalNode === fromNode ? setFromNoteTab("write") : setToNoteTab("write"))}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Write
                      </Button>
                      <Button
                        size="small"
                        variant={(externalNode === fromNode ? fromNoteTab : toNoteTab) === "preview" ? "contained" : "text"}
                        onClick={() => (externalNode === fromNode ? setFromNoteTab("preview") : setToNoteTab("preview"))}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Preview
                      </Button>
                    </Box>
                  </Box>
                  {(externalNode === fromNode ? fromNoteTab : toNoteTab) === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={5}
                      size="small"
                      placeholder="Markdown note for external peer endpoint..."
                      value={externalNode === fromNode ? fromNote : toNote}
                      onChange={(e) => (externalNode === fromNode ? setFromNote(e.target.value) : setToNote(e.target.value))}
                      disabled={submitting}
                      sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                    />
                  ) : (
                    <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#FFFFFF", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                      <MarkdownView content={externalNode === fromNode ? fromNote : toNote} emptyText="No note written yet" />
                    </Box>
                  )}
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
          ) : fromNode && toNode && linkType === "manual" ? (
            /* Manual Link Form */
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Alert
                icon={<Network size={18} />}
                severity="info"
                sx={{
                  borderRadius: 2,
                  backgroundColor: "#FAF5FF",
                  borderColor: "rgba(124, 58, 237, 0.3)",
                  color: "#5B21B6",
                  "& .MuiAlert-icon": { color: "#7C3AED" },
                }}
              >
                <strong>Manual Link Mode:</strong> easy42 will <em>not</em> create or manage WireGuard interfaces for this link.
                You create and manage the data plane link manually (via physical ethernet, VLAN, or custom tunnel).
                easy42 will configure BIRD BGP peering and automatically add the specified interfaces to the <code>easy42_ifname</code> nftables set.
              </Alert>

              <Typography variant="caption" sx={{ color: "#475569", fontWeight: 700, letterSpacing: "0.5px" }}>
                MANUAL LINK INTERFACE SPECIFICATIONS
              </Typography>

              {/* End 1: fromNode */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: "#FAF5FF",
                  border: "1px solid #DDD6FE",
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#6D28D9", mb: 1.2 }}>
                  End 1: {fromNode.name}
                </Typography>

                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5, mb: 1.5 }}>
                  <TextField
                    id="manual-from-interface"
                    label="Interface Name"
                    size="small"
                    placeholder="e.g. eth1, tun0"
                    value={fromInterface}
                    onChange={(e) => setFromInterface(e.target.value)}
                    required
                    helperText="Name of existing link interface"
                  />
                  <TextField
                    id="manual-from-address"
                    label="Local IP Address"
                    size="small"
                    placeholder="e.g. 10.0.0.1/30 or fe80::1/64"
                    value={fromAddress}
                    onChange={(e) => setFromAddress(e.target.value)}
                    required
                    helperText="Local IP on this interface"
                  />
                  <TextField
                    id="manual-from-neighbor-address"
                    label="Neighbor IP Address"
                    size="small"
                    placeholder="e.g. 10.0.0.2 or fe80::2"
                    value={fromNeighborAddress}
                    onChange={(e) => setFromNeighborAddress(e.target.value)}
                    helperText="Remote BGP peer IP (optional)"
                  />
                </Box>

                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Network Policy"
                    value={fromPolicy}
                    onChange={(e) => setFromPolicy(e.target.value)}
                    helperText="Firewall & BGP routing policy applied"
                  >
                    {networkPolicies.map((p) => (
                      <MenuItem key={p.id} value={p.id}>
                        {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                      </MenuItem>
                    ))}
                  </TextField>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Routing Policy (Optional)"
                    value={fromRoutingPolicy}
                    onChange={(e) => setFromRoutingPolicy(e.target.value)}
                    helperText="Overrides policy routing policy"
                  >
                    <MenuItem value="">Policy default</MenuItem>
                    <MenuItem value="full">Full (Default) — All valid routes</MenuItem>
                    <MenuItem value="stub">Stub — Receive all, only send local</MenuItem>
                    <MenuItem value="receive_only">Receive Only — Import only</MenuItem>
                    <MenuItem value="advertise_only">Advertise Only — Export only</MenuItem>
                  </TextField>
                  <TextField
                    fullWidth
                    size="small"
                    label="Link Cost (Optional)"
                    type="number"
                    value={fromCost}
                    onChange={(e) => setFromCost(e.target.value === "" ? "" : Number(e.target.value))}
                    placeholder="Policy default"
                    helperText="Overrides policy cost if set (non-zero)"
                  />
                </Box>

                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label="BGP Preference (Optional)"
                    value={fromPreference}
                    onChange={(e) => setFromPreference(e.target.value === "" ? "" : Number(e.target.value))}
                    placeholder="Policy default"
                    helperText="Overrides policy BGP preference"
                  />
                  <TextField
                    fullWidth
                    size="small"
                    label="Netfilter Mark (Optional)"
                    value={fromMark}
                    onChange={(e) => setFromMark(e.target.value)}
                    placeholder="Policy default"
                    helperText="Overrides policy netfilter mark"
                  />
                </Box>

                {/* End 1 Note */}
                <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #DDD6FE" }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                    <Typography variant="caption" sx={{ color: "#6D28D9", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                      <FileText size={13} color="#7C3AED" /> {fromNode.name} ENDPOINT NOTE (MARKDOWN)
                    </Typography>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      <Button
                        size="small"
                        variant={fromNoteTab === "write" ? "contained" : "text"}
                        onClick={() => setFromNoteTab("write")}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Write
                      </Button>
                      <Button
                        size="small"
                        variant={fromNoteTab === "preview" ? "contained" : "text"}
                        onClick={() => setFromNoteTab("preview")}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Preview
                      </Button>
                    </Box>
                  </Box>
                  {fromNoteTab === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={5}
                      size="small"
                      placeholder="Markdown note for this endpoint..."
                      value={fromNote}
                      onChange={(e) => setFromNote(e.target.value)}
                      disabled={submitting}
                      sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                    />
                  ) : (
                    <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#F8FAFC", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                      <MarkdownView content={fromNote} emptyText="No note written yet" />
                    </Box>
                  )}
                </Box>
              </Box>

              {/* End 2: toNode */}
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  backgroundColor: "#FAF5FF",
                  border: "1px solid #DDD6FE",
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#6D28D9", mb: 1.2 }}>
                  End 2: {toNode.name}
                </Typography>

                <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5, mb: 1.5 }}>
                  <TextField
                    id="manual-to-interface"
                    label="Interface Name"
                    size="small"
                    placeholder="e.g. eth2, tun1"
                    value={toInterface}
                    onChange={(e) => setToInterface(e.target.value)}
                    required
                    helperText="Name of existing link interface"
                  />
                  <TextField
                    id="manual-to-address"
                    label="Local IP Address"
                    size="small"
                    placeholder="e.g. 10.0.0.2/30 or fe80::2/64"
                    value={toAddress}
                    onChange={(e) => setToAddress(e.target.value)}
                    required
                    helperText="Local IP on this interface"
                  />
                  <TextField
                    id="manual-to-neighbor-address"
                    label="Neighbor IP Address"
                    size="small"
                    placeholder="e.g. 10.0.0.1 or fe80::1"
                    value={toNeighborAddress}
                    onChange={(e) => setToNeighborAddress(e.target.value)}
                    helperText="Remote BGP peer IP (optional)"
                  />
                </Box>

                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Network Policy"
                    value={toPolicy}
                    onChange={(e) => setToPolicy(e.target.value)}
                    helperText="Firewall & BGP routing policy applied"
                  >
                    {networkPolicies.map((p) => (
                      <MenuItem key={p.id} value={p.id}>
                        {p.name} ({p.id}){p.is_internal ? " — Built-in" : ""}
                      </MenuItem>
                    ))}
                  </TextField>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="Routing Policy (Optional)"
                    value={toRoutingPolicy}
                    onChange={(e) => setToRoutingPolicy(e.target.value)}
                    helperText="Overrides policy routing policy"
                  >
                    <MenuItem value="">Policy default</MenuItem>
                    <MenuItem value="full">Full (Default) — All valid routes</MenuItem>
                    <MenuItem value="stub">Stub — Receive all, only send local</MenuItem>
                    <MenuItem value="receive_only">Receive Only — Import only</MenuItem>
                    <MenuItem value="advertise_only">Advertise Only — Export only</MenuItem>
                  </TextField>
                  <TextField
                    fullWidth
                    size="small"
                    label="Link Cost (Optional)"
                    type="number"
                    value={toCost}
                    onChange={(e) => setToCost(e.target.value === "" ? "" : Number(e.target.value))}
                    placeholder="Policy default"
                    helperText="Overrides policy cost if set (non-zero)"
                  />
                </Box>

                <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
                  <TextField
                    fullWidth
                    size="small"
                    type="number"
                    label="BGP Preference (Optional)"
                    value={toPreference}
                    onChange={(e) => setToPreference(e.target.value === "" ? "" : Number(e.target.value))}
                    placeholder="Policy default"
                    helperText="Overrides policy BGP preference"
                  />
                  <TextField
                    fullWidth
                    size="small"
                    label="Netfilter Mark (Optional)"
                    value={toMark}
                    onChange={(e) => setToMark(e.target.value)}
                    placeholder="Policy default"
                    helperText="Overrides policy netfilter mark"
                  />
                </Box>

                {/* End 2 Note */}
                <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #DDD6FE" }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                    <Typography variant="caption" sx={{ color: "#6D28D9", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                      <FileText size={13} color="#7C3AED" /> {toNode.name} ENDPOINT NOTE (MARKDOWN)
                    </Typography>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      <Button
                        size="small"
                        variant={toNoteTab === "write" ? "contained" : "text"}
                        onClick={() => setToNoteTab("write")}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Write
                      </Button>
                      <Button
                        size="small"
                        variant={toNoteTab === "preview" ? "contained" : "text"}
                        onClick={() => setToNoteTab("preview")}
                        sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                      >
                        Preview
                      </Button>
                    </Box>
                  </Box>
                  {toNoteTab === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={5}
                      size="small"
                      placeholder="Markdown note for this endpoint..."
                      value={toNote}
                      onChange={(e) => setToNote(e.target.value)}
                      disabled={submitting}
                      sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                    />
                  ) : (
                    <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#F8FAFC", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                      <MarkdownView content={toNote} emptyText="No note written yet" />
                    </Box>
                  )}
                </Box>
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
                    <TextField
                      label="Interface"
                      size="small"
                      value={
                        linkToEdit
                          ? linkToEdit.from.interface
                          : (toNode ? getInterfaceNameWithSuffix(toNode.name, linkSuffix, toNode.is_external) : "")
                      }
                      disabled
                    />
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
                            : (() => {
                                const resolved = resolvePeerEntrypoint(fromNode, toNode);
                                const port = toPort || (fromNode?.ip ? derivePortFromIP(fromNode.ip) : 20000);
                                if (resolved.entrypoint?.ip) {
                                  return `${resolved.entrypoint.ip}:${port}`;
                                }
                                if (linkToEdit?.from.endpoint) {
                                  const parts = linkToEdit.from.endpoint.split(":");
                                  if (parts.length === 2 && port) {
                                    return `${parts[0]}:${port}`;
                                  }
                                  return linkToEdit.from.endpoint;
                                }
                                return "Dynamic / Automatic";
                              })()}
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

                  <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
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
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="Routing Policy (Optional)"
                      value={fromRoutingPolicy}
                      onChange={(e) => setFromRoutingPolicy(e.target.value)}
                      helperText="Overrides policy routing policy"
                    >
                      <MenuItem value="">Policy default</MenuItem>
                      <MenuItem value="full">Full (Default) — All valid routes</MenuItem>
                      <MenuItem value="stub">Stub — Receive all, only send local</MenuItem>
                      <MenuItem value="receive_only">Receive Only — Import only</MenuItem>
                      <MenuItem value="advertise_only">Advertise Only — Export only</MenuItem>
                    </TextField>
                    <TextField
                      fullWidth
                      size="small"
                      label="Link Cost (Optional)"
                      type="number"
                      value={fromCost}
                      onChange={(e) => setFromCost(e.target.value === "" ? "" : Number(e.target.value))}
                      placeholder="Policy default"
                      helperText="Overrides policy cost if set (non-zero)"
                    />
                  </Box>
                  <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
                    <TextField
                      fullWidth
                      size="small"
                      label="WireGuard FwMark (Optional)"
                      value={fromFwmark}
                      onChange={(e) => setFromFwmark(e.target.value)}
                      placeholder="Policy default"
                      helperText="Overrides policy WireGuard FwMark"
                    />
                    <TextField
                      fullWidth
                      size="small"
                      type="number"
                      label="BGP Preference (Optional)"
                      value={fromPreference}
                      onChange={(e) => setFromPreference(e.target.value === "" ? "" : Number(e.target.value))}
                      placeholder="Policy default"
                      helperText="Overrides policy BGP preference"
                    />
                    <TextField
                      fullWidth
                      size="small"
                      label="Netfilter Mark (Optional)"
                      value={fromMark}
                      onChange={(e) => setFromMark(e.target.value)}
                      placeholder="Policy default"
                      helperText="Overrides policy netfilter mark"
                    />
                  </Box>

                  {/* Node 1 Endpoint Note */}
                  <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #C7D2FE" }}>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                      <Typography variant="caption" sx={{ color: "#4338CA", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                        <FileText size={13} color="#4F46E5" /> {fromNode.name} ENDPOINT NOTE (MARKDOWN)
                      </Typography>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <Button
                          size="small"
                          variant={fromNoteTab === "write" ? "contained" : "text"}
                          onClick={() => setFromNoteTab("write")}
                          sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                        >
                          Write
                        </Button>
                        <Button
                          size="small"
                          variant={fromNoteTab === "preview" ? "contained" : "text"}
                          onClick={() => setFromNoteTab("preview")}
                          sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                        >
                          Preview
                        </Button>
                      </Box>
                    </Box>
                    {fromNoteTab === "write" ? (
                      <TextField
                        fullWidth
                        multiline
                        minRows={2}
                        maxRows={5}
                        size="small"
                        placeholder="Arbitrary markdown note for this endpoint..."
                        value={fromNote}
                        onChange={(e) => setFromNote(e.target.value)}
                        disabled={submitting}
                        sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                      />
                    ) : (
                      <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#F8FAFC", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                        <MarkdownView content={fromNote} emptyText="No note written yet" />
                      </Box>
                    )}
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
                    <TextField
                      label="Interface"
                      size="small"
                      value={
                        linkToEdit
                          ? linkToEdit.to.interface
                          : (fromNode ? getInterfaceNameWithSuffix(fromNode.name, linkSuffix, fromNode.is_external) : "")
                      }
                      disabled
                    />
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
                            : (() => {
                                const resolved = resolvePeerEntrypoint(toNode, fromNode);
                                const port = fromPort || (toNode?.ip ? derivePortFromIP(toNode.ip) : 20000);
                                if (resolved.entrypoint?.ip) {
                                  return `${resolved.entrypoint.ip}:${port}`;
                                }
                                if (linkToEdit?.to.endpoint) {
                                  const parts = linkToEdit.to.endpoint.split(":");
                                  if (parts.length === 2 && port) {
                                    return `${parts[0]}:${port}`;
                                  }
                                  return linkToEdit.to.endpoint;
                                }
                                return "Dynamic / Automatic";
                              })()}
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

                  <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
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
                    <TextField
                      select
                      fullWidth
                      size="small"
                      label="Routing Policy (Optional)"
                      value={toRoutingPolicy}
                      onChange={(e) => setToRoutingPolicy(e.target.value)}
                      helperText="Overrides policy routing policy"
                    >
                      <MenuItem value="">Policy default</MenuItem>
                      <MenuItem value="full">Full (Default) — All valid routes</MenuItem>
                      <MenuItem value="stub">Stub — Receive all, only send local</MenuItem>
                      <MenuItem value="receive_only">Receive Only — Import only</MenuItem>
                      <MenuItem value="advertise_only">Advertise Only — Export only</MenuItem>
                    </TextField>
                    <TextField
                      fullWidth
                      size="small"
                      label="Link Cost (Optional)"
                      type="number"
                      value={toCost}
                      onChange={(e) => setToCost(e.target.value === "" ? "" : Number(e.target.value))}
                      placeholder="Policy default"
                      helperText="Overrides policy cost if set (non-zero)"
                    />
                  </Box>
                  <Box sx={{ mt: 1.5, display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 1.5 }}>
                    <TextField
                      fullWidth
                      size="small"
                      label="WireGuard FwMark (Optional)"
                      value={toFwmark}
                      onChange={(e) => setToFwmark(e.target.value)}
                      placeholder="Policy default"
                      helperText="Overrides policy WireGuard FwMark"
                    />
                    <TextField
                      fullWidth
                      size="small"
                      type="number"
                      label="BGP Preference (Optional)"
                      value={toPreference}
                      onChange={(e) => setToPreference(e.target.value === "" ? "" : Number(e.target.value))}
                      placeholder="Policy default"
                      helperText="Overrides policy BGP preference"
                    />
                    <TextField
                      fullWidth
                      size="small"
                      label="Netfilter Mark (Optional)"
                      value={toMark}
                      onChange={(e) => setToMark(e.target.value)}
                      placeholder="Policy default"
                      helperText="Overrides policy netfilter mark"
                    />
                  </Box>

                  {/* Node 2 Endpoint Note */}
                  <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #A5F3FC" }}>
                    <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
                      <Typography variant="caption" sx={{ color: "#0E7490", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}>
                        <FileText size={13} color="#0891B2" /> {toNode.name} ENDPOINT NOTE (MARKDOWN)
                      </Typography>
                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <Button
                          size="small"
                          variant={toNoteTab === "write" ? "contained" : "text"}
                          onClick={() => setToNoteTab("write")}
                          sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                        >
                          Write
                        </Button>
                        <Button
                          size="small"
                          variant={toNoteTab === "preview" ? "contained" : "text"}
                          onClick={() => setToNoteTab("preview")}
                          sx={{ minWidth: "auto", px: 1, py: 0.2, fontSize: "0.7rem", textTransform: "none" }}
                        >
                          Preview
                        </Button>
                      </Box>
                    </Box>
                    {toNoteTab === "write" ? (
                      <TextField
                        fullWidth
                        multiline
                        minRows={2}
                        maxRows={5}
                        size="small"
                        placeholder="Arbitrary markdown note for this endpoint..."
                        value={toNote}
                        onChange={(e) => setToNote(e.target.value)}
                        disabled={submitting}
                        sx={{ "& .MuiInputBase-root": { fontSize: "0.8rem", fontFamily: "'JetBrains Mono', 'Fira Code', monospace" } }}
                      />
                    ) : (
                      <Box sx={{ p: 1, minHeight: 60, maxHeight: 150, overflowY: "auto", backgroundColor: "#F8FAFC", borderRadius: 1, border: "1px solid #E2E8F0" }}>
                        <MarkdownView content={toNote} emptyText="No note written yet" />
                      </Box>
                    )}
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
