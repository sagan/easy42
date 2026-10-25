import React, { useState } from "react";
import {
  Drawer,
  Box,
  Typography,
  IconButton,
  Button,
  Divider,
  CircularProgress,
  Alert,
  Tooltip,
  Chip,
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  InputAdornment,
} from "@mui/material";
import {
  X,
  Trash2,
  Link as LinkIcon,
  Key,
  KeyRound,
  ArrowRightLeft,
  Edit2,
  Activity,
  Copy,
  Check,
  Shield,
  RefreshCw,
  Gauge,
  Zap,
  Route,
  RotateCcw,
  FileText,
  Network,
  Share2,
  Lock,
  Eye,
  EyeOff,
} from "lucide-react";
import { api } from "../../api/client";
import { Link, NetworkState, Node, NetworkPolicy, NodeStatus, NetworkSettings } from "../../types/api";
import { MarkdownView } from "../Common/MarkdownView";
import { getEffectiveLinkCost } from "../../utils/cost";
import { getLinkWorkingState } from "./TopologyGraph";
import { LinkKeyActionModal, LinkKeyActionMode } from "../Modals/LinkKeyActionModal";
import { MeshContext } from "../../context/MeshContext";

interface LinkDetailDrawerProps {
  link: Link | null;
  networkState?: NetworkState | null;
  nodeStatuses?: Record<string, NodeStatus>;
  nodes?: Node[];
  networkPolicies?: NetworkPolicy[];
  networkSettings?: NetworkSettings | null;
  open: boolean;
  onClose: () => void;
  onEditLink: (link: Link) => void;
  onLinkDeleted: (from: string, to: string, iface?: string) => void;
  onRefreshLink?: (link: Link) => Promise<void>;
  onLinkUpdated?: (link: Link) => void;
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function formatHandshakeAgo(dateStr?: string): string {
  if (!dateStr) return "";
  const date = new Date(dateStr);
  const diffSec = Math.floor((Date.now() - date.getTime()) / 1000);
  if (diffSec < 0) return "now";
  if (diffSec < 60) return `${diffSec}s`;
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m`;
  const diffHours = Math.floor(diffMin / 60);
  return `${diffHours}h`;
}

export const LinkDetailDrawer: React.FC<LinkDetailDrawerProps> = ({
  link,
  networkState,
  nodeStatuses,
  nodes,
  networkPolicies,
  networkSettings,
  open,
  onClose,
  onEditLink,
  onLinkDeleted,
  onRefreshLink,
  onLinkUpdated,
}) => {
  const mesh = React.useContext(MeshContext);
  const effectiveNodes = nodes || mesh?.nodes || [];
  const effectivePolicies = networkPolicies || mesh?.networkPolicies;
  const effectiveNetworkSettings = networkSettings !== undefined ? networkSettings : mesh?.networkSettings;
  const [deleting, setDeleting] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [restartingFrom, setRestartingFrom] = useState(false);
  const [restartingTo, setRestartingTo] = useState(false);
  const [actionSuccess, setActionSuccess] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [copiedEndpoint, setCopiedEndpoint] = useState<string | null>(null);
  const [copiedField, setCopiedField] = useState<string | null>(null);

  const [keyModalOpen, setKeyModalOpen] = useState(false);
  const [keyModalMode, setKeyModalMode] = useState<LinkKeyActionMode>("reveal_psk");
  const [keyModalNode, setKeyModalNode] = useState<string | undefined>(undefined);

  const [copiedPeeringInfo, setCopiedPeeringInfo] = useState(false);
  const [peeringAuthModalOpen, setPeeringAuthModalOpen] = useState(false);
  const [peeringPassword, setPeeringPassword] = useState("");
  const [showPeeringPassword, setShowPeeringPassword] = useState(false);
  const [peeringAuthLoading, setPeeringAuthLoading] = useState(false);
  const [peeringAuthError, setPeeringAuthError] = useState<string | null>(null);
  const [peeringPreviewText, setPeeringPreviewText] = useState<string | null>(null);

  const handleOpenKeyModal = (mode: LinkKeyActionMode, node?: string) => {
    setKeyModalMode(mode);
    setKeyModalNode(node);
    setKeyModalOpen(true);
  };

  const [isEditingFromNote, setIsEditingFromNote] = useState(false);
  const [fromNoteDraft, setFromNoteDraft] = useState("");
  const [savingFromNote, setSavingFromNote] = useState(false);
  const [fromNoteTab, setFromNoteTab] = useState<"write" | "preview">("write");

  const [isEditingToNote, setIsEditingToNote] = useState(false);
  const [toNoteDraft, setToNoteDraft] = useState("");
  const [savingToNote, setSavingToNote] = useState(false);
  const [toNoteTab, setToNoteTab] = useState<"write" | "preview">("write");

  React.useEffect(() => {
    if (link) {
      setFromNoteDraft(link.from.note || "");
      setToNoteDraft(link.to.note || "");
      setIsEditingFromNote(false);
      setIsEditingToNote(false);
      setFromNoteTab("write");
      setToNoteTab("write");
    }
  }, [link?.from?.name, link?.to?.name, link?.from?.note, link?.to?.note]);

  if (!link) return null;

  const fromNode = effectiveNodes.find((n) => n.name === link.from.name);
  const toNode = effectiveNodes.find((n) => n.name === link.to.name);
  const isFromExternal = fromNode?.is_external;
  const isToExternal = toNode?.is_external;
  const isManual = link.type === "manual" || link.from.type === "manual" || link.to.type === "manual";

  const isExternalLink = Boolean(isFromExternal || isToExternal);
  const globalAsn =
    effectiveNetworkSettings?.public_asn && effectiveNetworkSettings.public_asn > 0
      ? effectiveNetworkSettings.public_asn
      : undefined;

  const fromIsGlobalAsn = Boolean(isExternalLink && !isFromExternal && globalAsn);
  const fromDisplayedAsn = fromIsGlobalAsn ? globalAsn : fromNode?.asn;

  const toIsGlobalAsn = Boolean(isExternalLink && !isToExternal && globalAsn);
  const toDisplayedAsn = toIsGlobalAsn ? globalAsn : toNode?.asn;

  const hasPsk = Boolean(link.psk || link.from.psk || link.to.psk);

  const getPeeringInfoData = (pskValue?: string) => {
    if (!link) return "";
    const isFromExt = Boolean(isFromExternal);

    const ourLinkEnd = isFromExt ? link.to : link.from;
    const theirLinkEnd = isFromExt ? link.from : link.to;

    const ourNode = effectiveNodes.find((n) => n.name === ourLinkEnd.name);
    const theirNode = effectiveNodes.find((n) => n.name === theirLinkEnd.name);

    // Our side ASN: global settings public_asn if configured, else ourNode.asn
    const ourAsn =
      (effectiveNetworkSettings?.public_asn && effectiveNetworkSettings.public_asn > 0
        ? effectiveNetworkSettings.public_asn
        : ourNode?.asn) ?? "";
    const theirAsn = theirNode?.asn ?? "";

    // Endpoints
    let ourEndpoint =
      (isFromExt ? link.from.resolved_endpoint || link.from.endpoint : link.to.resolved_endpoint || link.to.endpoint) ||
      "";
    if (!ourEndpoint || ourEndpoint === "Dynamic / Automatic") {
      const ep = ourNode?.entrypoints?.find((e) => e.ip);
      const host = ep?.ip || ourNode?.host || "";
      if (host && ourLinkEnd.listen_port) {
        ourEndpoint = `${host}:${ourLinkEnd.listen_port}`;
      } else {
        ourEndpoint = ourEndpoint === "Dynamic / Automatic" ? "" : ourEndpoint;
      }
    }

    let theirEndpoint =
      (isFromExt ? link.to.resolved_endpoint || link.to.endpoint : link.from.resolved_endpoint || link.from.endpoint) ||
      "";
    if ((!theirEndpoint || theirEndpoint === "Dynamic / Automatic") && theirNode?.host && theirLinkEnd.listen_port) {
      theirEndpoint = `${theirNode.host}:${theirLinkEnd.listen_port}`;
    } else if (theirEndpoint === "Dynamic / Automatic") {
      theirEndpoint = "";
    }

    const ourPublicKey = ourLinkEnd.public_key || "";
    const theirPublicKey = theirLinkEnd.public_key || "";

    const ourAddress = getLinkLocalIpv6CopyText(ourLinkEnd.address);
    const theirAddress = getLinkLocalIpv6CopyText(theirLinkEnd.address || ourLinkEnd.neighbor_address);

    const ourLines = [
      "Our side:",
      `ASN: ${ourAsn}`,
      `WireGuard Endpoint: ${ourEndpoint}`,
      `WireGuard Public Key: ${ourPublicKey}`,
    ];
    if (pskValue) {
      ourLines.push(`WireGuard PSK: ${pskValue}`);
    }
    ourLines.push(`BGP Peering Address (ipv6 link local): ${ourAddress}`);
    ourLines.push("Configurations: ROA, MP-BGP, Extended Next-hop");

    const theirLines = [
      "Your side:",
      `ASN: ${theirAsn}`,
      `WireGuard Endpoint: ${theirEndpoint}`,
      `WireGuard Public Key: ${theirPublicKey}`,
    ];
    if (pskValue) {
      theirLines.push(`WireGuard PSK: ${pskValue}`);
    }
    theirLines.push(`BGP Peering Address (ipv6 link local): ${theirAddress}`);

    return `${ourLines.join("\n")}\n\n${theirLines.join("\n")}\n`;
  };

  const handleCopyPeeringInfo = async () => {
    if (!isExternalLink) return;

    if (hasPsk) {
      setPeeringPassword("");
      setShowPeeringPassword(false);
      setPeeringAuthLoading(false);
      setPeeringAuthError(null);
      setPeeringPreviewText(null);
      setPeeringAuthModalOpen(true);
    } else {
      const text = getPeeringInfoData();
      try {
        await navigator.clipboard.writeText(text);
        setCopiedPeeringInfo(true);
        setActionSuccess("Peering info copied to clipboard");
        setTimeout(() => {
          setCopiedPeeringInfo(false);
          setActionSuccess(null);
        }, 3000);
      } catch {
        setError("Failed to copy peering info to clipboard");
      }
    }
  };

  const handleVerifyAndCopyPeeringInfo = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!link) return;
    if (!peeringPassword) {
      setPeeringAuthError("Please enter your administrator password");
      return;
    }
    setPeeringAuthLoading(true);
    setPeeringAuthError(null);

    try {
      const res = await api.revealLinkPsk({
        from: link.from.name,
        to: link.to.name,
        interface: link.from.interface,
        password: peeringPassword,
      });
      const decryptedPsk = res.psk || "";
      const text = getPeeringInfoData(decryptedPsk);
      setPeeringPreviewText(text);
      await navigator.clipboard.writeText(text);
      setCopiedPeeringInfo(true);
      setActionSuccess("Peering info with PSK copied to clipboard");
      setTimeout(() => {
        setCopiedPeeringInfo(false);
        setActionSuccess(null);
      }, 4000);
    } catch (err: unknown) {
      const e = err as Error;
      setPeeringAuthError(e.message || "Failed to verify password and decrypt PSK");
    } finally {
      setPeeringAuthLoading(false);
    }
  };

  const handleSaveFromNote = async () => {
    if (!link) return;
    setSavingFromNote(true);
    setError(null);
    try {
      const updated = await api.updateLink({
        from_node: link.from.name,
        to_node: link.to.name,
        from_note: fromNoteDraft.trim(),
        from: {
          ...link.from,
          note: fromNoteDraft.trim() || undefined,
        },
      });
      onLinkUpdated?.(updated);
      setIsEditingFromNote(false);
      setActionSuccess("Endpoint note updated");
      setTimeout(() => setActionSuccess(null), 3000);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to save endpoint note");
    } finally {
      setSavingFromNote(false);
    }
  };

  const handleSaveToNote = async () => {
    if (!link) return;
    setSavingToNote(true);
    setError(null);
    try {
      const updated = await api.updateLink({
        from_node: link.from.name,
        to_node: link.to.name,
        to_note: toNoteDraft.trim(),
        to: {
          ...link.to,
          note: toNoteDraft.trim() || undefined,
        },
      });
      onLinkUpdated?.(updated);
      setIsEditingToNote(false);
      setActionSuccess("Endpoint note updated");
      setTimeout(() => setActionSuccess(null), 3000);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to save endpoint note");
    } finally {
      setSavingToNote(false);
    }
  };

  const handleRestartEnd = async (end: "from" | "to") => {
    const endData = end === "from" ? link.from : link.to;
    if (
      !window.confirm(
        `Are you sure you want to restart WireGuard interface "${endData.interface}" on node "${endData.name}"? This will run "wg-quick down ${endData.interface}; wg-quick up ${endData.interface}".`,
      )
    ) {
      return;
    }
    if (end === "from") setRestartingFrom(true);
    else setRestartingTo(true);
    setError(null);
    setActionSuccess(null);
    try {
      const res = await api.restartNodeInterface(endData.name, endData.interface);
      setActionSuccess(res.message || `Restarted interface ${endData.interface} on ${endData.name}`);
      if (onRefreshLink) {
        await onRefreshLink(link);
      }
      setTimeout(() => setActionSuccess(null), 4000);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || `Failed to restart interface ${endData.interface}`);
    } finally {
      if (end === "from") setRestartingFrom(false);
      else setRestartingTo(false);
    }
  };

  const copyToClipboard = (text: string, id: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedKey(id);
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const copyEndpointToClipboard = (text: string, id: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedEndpoint(id);
    setTimeout(() => setCopiedEndpoint(null), 2000);
  };

  const copyFieldToClipboard = (text: string, id: string) => {
    if (!text) return;
    navigator.clipboard.writeText(text);
    setCopiedField(id);
    setTimeout(() => setCopiedField(null), 2000);
  };

  const getLinkLocalIpv6CopyText = (addr?: string) => {
    if (!addr) return "";
    return addr.trim().replace(/\/64$/, "");
  };

  const handleDelete = async () => {
    if (
      !window.confirm(
        `Are you sure you want to delete the ${isManual ? "manual" : "WireGuard"} link between "${link.from.name}" and "${link.to.name}"?`,
      )
    ) {
      return;
    }
    setDeleting(true);
    try {
      await api.deleteLink(link.from.name, link.to.name, link.from.interface);
      onLinkDeleted(link.from.name, link.to.name, link.from.interface);
      onClose();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to delete link");
    } finally {
      setDeleting(false);
    }
  };

  const handleRefreshLink = async () => {
    if (!onRefreshLink) return;
    setRefreshing(true);
    setError(null);
    try {
      await onRefreshLink(link);
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Failed to refresh link state");
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{
        sx: {
          width: { xs: "100%", sm: 440 },
          backgroundColor: "#FFFFFF",
          borderLeft: "1px solid #E2E8F0",
          p: 3,
        },
      }}
    >
      {/* Header */}
      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              backgroundColor: isManual ? "rgba(124, 58, 237, 0.1)" : "rgba(8, 145, 178, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: isManual ? "#7C3AED" : "#0891B2",
            }}
          >
            {isManual ? <Network size={20} /> : <LinkIcon size={20} />}
          </Box>
          <Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, color: "#0F172A" }}>
                {isManual ? "Manual Link" : "WireGuard Link"}
              </Typography>
              {isManual && (
                <Chip
                  label="MANUAL"
                  size="small"
                  sx={{
                    height: 18,
                    fontSize: "0.6rem",
                    fontWeight: 800,
                    bgcolor: "rgba(124, 58, 237, 0.12)",
                    color: "#7C3AED",
                    border: "1px solid rgba(124, 58, 237, 0.25)",
                    borderRadius: "4px",
                  }}
                />
              )}
              {link.assign_ipv4 && (
                <Chip
                  label="IPv4 169.254.X.X"
                  size="small"
                  sx={{
                    height: 18,
                    fontSize: "0.6rem",
                    fontWeight: 800,
                    bgcolor: "rgba(16, 185, 129, 0.12)",
                    color: "#059669",
                    border: "1px solid rgba(16, 185, 129, 0.25)",
                    borderRadius: "4px",
                  }}
                />
              )}
            </Box>
            <Typography variant="caption" className="mono-font" sx={{ color: "#64748B" }}>
              {link.from.name} ↔ {link.to.name}
            </Typography>
          </Box>
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
          {isExternalLink && (
            <Tooltip title={copiedPeeringInfo ? "Peering Info Copied!" : "Copy Peering Info"}>
              <IconButton
                id="btn-header-copy-peering-info"
                size="small"
                onClick={handleCopyPeeringInfo}
                sx={{
                  color: copiedPeeringInfo ? "#059669" : "#4F46E5",
                  backgroundColor: copiedPeeringInfo ? "rgba(16, 185, 129, 0.1)" : "rgba(99, 102, 241, 0.08)",
                  "&:hover": {
                    backgroundColor: copiedPeeringInfo ? "rgba(16, 185, 129, 0.15)" : "rgba(99, 102, 241, 0.15)",
                  },
                }}
              >
                {copiedPeeringInfo ? <Check size={18} /> : <Share2 size={18} />}
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="Edit Link">
            <IconButton
              size="small"
              onClick={() => onEditLink(link)}
              sx={{
                color: isManual ? "#7C3AED" : "#0891B2",
                "&:hover": { backgroundColor: isManual ? "rgba(124, 58, 237, 0.08)" : "rgba(8, 145, 178, 0.08)" },
              }}
            >
              <Edit2 size={18} />
            </IconButton>
          </Tooltip>
          <IconButton size="small" onClick={onClose} sx={{ color: "#94A3B8" }}>
            <X size={20} />
          </IconButton>
        </Box>
      </Box>

      <Divider sx={{ borderColor: "#E2E8F0", mb: 2.5 }} />

      {/* Live Connection Health Card */}
      {(() => {
        if (isManual) {
          return (
            <Box
              sx={{
                p: 2,
                mb: 2.5,
                borderRadius: 2,
                backgroundColor: "#FAF5FF",
                border: "1px solid #DDD6FE",
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
                <Typography
                  variant="subtitle2"
                  sx={{ fontWeight: 700, color: "#6D28D9", display: "flex", alignItems: "center", gap: 1 }}
                >
                  <Network size={16} color="#7C3AED" />
                  Manual Data Plane Link
                </Typography>
                <Chip
                  label="BGP ONLY"
                  size="small"
                  sx={{
                    fontWeight: 800,
                    fontSize: "0.62rem",
                    bgcolor: "#7C3AED",
                    color: "#FFFFFF",
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ color: "#6B21A8", display: "block", lineHeight: 1.4 }}>
                Data plane interfaces ({link.from.interface} / {link.to.interface}) are manually created and managed by
                you. easy42 automatically manages BIRD BGP routing sessions and adds interfaces to the{" "}
                <code>easy42_ifname</code> nftables set.
              </Typography>
            </Box>
          );
        }
        const fromIface = networkState?.nodes?.[link.from.name]?.interfaces?.[link.from.interface];
        const toIface = networkState?.nodes?.[link.to.name]?.interfaces?.[link.to.interface];

        const workingState = getLinkWorkingState(link, networkState, nodeStatuses);

        // Resolve real-time live peer stats if available from nodeStatuses
        const fromWg = nodeStatuses?.[link.from.name]?.wg_interfaces?.find((w) => w.name === link.from.interface);
        const fromPeer =
          fromWg?.peers?.find((p) => (link.to.public_key ? p.public_key === link.to.public_key : true)) ||
          fromWg?.peers?.[0];

        const toWg = nodeStatuses?.[link.to.name]?.wg_interfaces?.find((w) => w.name === link.to.interface);
        const toPeer =
          toWg?.peers?.find((p) => (link.from.public_key ? p.public_key === link.from.public_key : true)) ||
          toWg?.peers?.[0];

        const hsFromStr = fromPeer?.latest_handshake || fromIface?.latest_handshake;
        const hsToStr = toPeer?.latest_handshake || toIface?.latest_handshake;

        const hsFrom = hsFromStr ? new Date(hsFromStr) : null;
        const hsTo = hsToStr ? new Date(hsToStr) : null;
        let newestHandshake: Date | null = null;
        if (hsFrom && hsTo) {
          newestHandshake = hsFrom >= hsTo ? hsFrom : hsTo;
        } else {
          newestHandshake = hsFrom || hsTo;
        }

        const rxFrom = fromPeer?.transfer_rx_bytes ?? fromIface?.transfer_rx_bytes ?? 0;
        const rxTo = toPeer?.transfer_rx_bytes ?? toIface?.transfer_rx_bytes ?? 0;
        const txFrom = fromPeer?.transfer_tx_bytes ?? fromIface?.transfer_tx_bytes ?? 0;
        const txTo = toPeer?.transfer_tx_bytes ?? toIface?.transfer_tx_bytes ?? 0;

        const totalRx = rxFrom + rxTo;
        const totalTx = txFrom + txTo;

        return (
          <Box
            sx={{
              p: 2,
              mb: 2.5,
              borderRadius: 2,
              backgroundColor:
                workingState === "working" ? "#ECFDF5" : workingState === "not_working" ? "#FEF2F2" : "#F8FAFC",
              border: "1px solid",
              borderColor:
                workingState === "working" ? "#A7F3D0" : workingState === "not_working" ? "#FECDD3" : "#E2E8F0",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1.5 }}>
              <Typography
                variant="subtitle2"
                sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 1 }}
              >
                <Activity
                  size={16}
                  color={
                    workingState === "working" ? "#059669" : workingState === "not_working" ? "#DC2626" : "#64748B"
                  }
                />
                Tunnel Health
              </Typography>
              <Chip
                label={
                  workingState === "working" ? "WORKING" : workingState === "not_working" ? "NOT WORKING" : "UNKNOWN"
                }
                size="small"
                sx={{
                  fontWeight: 700,
                  fontSize: "0.65rem",
                  bgcolor:
                    workingState === "working" ? "#10B981" : workingState === "not_working" ? "#EF4444" : "#94A3B8",
                  color: "#FFFFFF",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", flexDirection: "column", gap: 0.8 }}>
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Latest Handshake:
                </Typography>
                <Typography variant="caption" sx={{ fontWeight: 600, color: "#1E293B" }}>
                  {newestHandshake
                    ? `${newestHandshake.toISOString()} (${formatHandshakeAgo(newestHandshake.toISOString())} ago)`
                    : "None observed"}
                </Typography>
              </Box>
              {(totalRx > 0 || totalTx > 0) && (
                <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Observed Transfer:
                  </Typography>
                  <Typography variant="caption" sx={{ fontWeight: 600, color: "#1E293B" }}>
                    ↓ {formatBytes(totalRx)} / ↑ {formatBytes(totalTx)}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        );
      })()}

      {/* WireGuard PSK Card */}
      {!isManual && (
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: "#F8FAFC",
            border: "1px solid #E2E8F0",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.8 }}>
            <Typography
              variant="subtitle2"
              sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 1 }}
            >
              <KeyRound size={16} color="#4F46E5" />
              Pre-shared Key (PSK)
            </Typography>
            <Chip
              label={Boolean(link.psk || link.from.psk || link.to.psk) ? "CONFIGURED" : "NOT SET"}
              size="small"
              sx={{
                fontWeight: 700,
                fontSize: "0.65rem",
                bgcolor: Boolean(link.psk || link.from.psk || link.to.psk) ? "#4F46E5" : "#94A3B8",
                color: "#FFFFFF",
              }}
            />
          </Box>
          <Typography variant="caption" sx={{ color: "#64748B", display: "block", mb: 1.5 }}>
            Symmetric 256-bit PSK provides post-quantum security and peer authentication.
          </Typography>
          <Box sx={{ display: "flex", gap: 1 }}>
            <Button
              id="btn-reveal-psk"
              size="small"
              disabled={!Boolean(link.psk || link.from.psk || link.to.psk)}
              variant="outlined"
              startIcon={<Shield size={13} />}
              onClick={() => handleOpenKeyModal("reveal_psk")}
              sx={{
                fontSize: "0.75rem",
                py: 0.4,
                px: 1.2,
                textTransform: "none",
                fontWeight: 600,
                borderRadius: 1.5,
                borderColor: "#CBD5E1",
                color: "#334155",
                "&:hover": { borderColor: "#4F46E5", color: "#4F46E5", backgroundColor: "rgba(99, 102, 241, 0.04)" },
              }}
            >
              Reveal PSK
            </Button>
            <Button
              id="btn-reset-psk"
              size="small"
              variant="outlined"
              startIcon={<RefreshCw size={13} />}
              onClick={() => handleOpenKeyModal("reset_psk")}
              sx={{
                fontSize: "0.75rem",
                py: 0.4,
                px: 1.2,
                textTransform: "none",
                fontWeight: 600,
                borderRadius: 1.5,
                borderColor: "#CBD5E1",
                color: "#D97706",
                "&:hover": { borderColor: "#D97706", backgroundColor: "rgba(217, 119, 6, 0.05)" },
              }}
            >
              Reset PSK
            </Button>
          </Box>
        </Box>
      )}

      {/* Specifications */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
        {/* End 1 */}
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: "#EEF2FF",
            border: "1px solid #C7D2FE",
          }}
        >
          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.5 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#3730A3" }}>
              Node 1: {link.from.name}
            </Typography>
            {!isManual && (
              <Tooltip
                title={
                  isFromExternal
                    ? "Cannot restart interface on external node"
                    : `Restart ${link.from.interface} on ${link.from.name}`
                }
              >
                <span>
                  <Button
                    id={`restart-link-end-from-${link.from.name}-${link.from.interface}`}
                    size="small"
                    variant="outlined"
                    startIcon={
                      restartingFrom ? <CircularProgress size={12} color="inherit" /> : <RotateCcw size={12} />
                    }
                    onClick={() => handleRestartEnd("from")}
                    disabled={restartingFrom || restartingTo || deleting || Boolean(isFromExternal)}
                    sx={{
                      fontSize: "0.72rem",
                      py: 0.2,
                      px: 1,
                      textTransform: "none",
                      fontWeight: 600,
                      borderRadius: 1.5,
                      borderColor: "#A5B4FC",
                      color: "#4338CA",
                      "&:hover": {
                        borderColor: "#6366F1",
                        backgroundColor: "rgba(99, 102, 241, 0.08)",
                      },
                    }}
                  >
                    {restartingFrom ? "Restarting..." : "Restart"}
                  </Button>
                </span>
              </Tooltip>
            )}
          </Box>

          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            <Box sx={{ display: "flex", justifyContent: "space-between" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Interface:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                {link.from.interface}
              </Typography>
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                ASN:
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                  {fromDisplayedAsn !== undefined && fromDisplayedAsn !== null ? `AS${fromDisplayedAsn}` : "—"}
                </Typography>
                {fromIsGlobalAsn && (
                  <Chip
                    label="Global ASN"
                    size="small"
                    sx={{
                      height: 18,
                      fontSize: "0.62rem",
                      fontWeight: 700,
                      bgcolor: "rgba(79, 70, 229, 0.12)",
                      color: "#4F46E5",
                      border: "1px solid rgba(79, 70, 229, 0.25)",
                      borderRadius: "4px",
                    }}
                  />
                )}
                {fromDisplayedAsn !== undefined && fromDisplayedAsn !== null && (
                  <Tooltip title={copiedField === "from_asn" ? "Copied!" : "Copy ASN"}>
                    <IconButton
                      size="small"
                      onClick={() => copyFieldToClipboard(String(fromDisplayedAsn), "from_asn")}
                      sx={{ p: 0.3, color: copiedField === "from_asn" ? "#10B981" : "#64748B" }}
                    >
                      {copiedField === "from_asn" ? <Check size={13} /> : <Copy size={13} />}
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                {isManual ? "Local Address:" : "Link-Local IPv6:"}
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                <Typography variant="caption" className="mono-font" sx={{ color: "#0891B2", fontWeight: 600 }}>
                  {link.from.address}
                </Typography>
                {link.from.address && (
                  <Tooltip title={copiedField === "from_ipv6" ? "Copied!" : "Copy Link-Local IPv6"}>
                    <IconButton
                      size="small"
                      onClick={() => copyFieldToClipboard(getLinkLocalIpv6CopyText(link.from.address), "from_ipv6")}
                      sx={{ p: 0.3, color: copiedField === "from_ipv6" ? "#10B981" : "#64748B" }}
                    >
                      {copiedField === "from_ipv6" ? <Check size={13} /> : <Copy size={13} />}
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
            </Box>

            {link.assign_ipv4 && (
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Link-Local IPv4:
                </Typography>
                <Typography variant="caption" className="mono-font" sx={{ color: "#059669", fontWeight: 600 }}>
                  {link.from.address4 || "169.254.X.X/32 (Auto)"}
                </Typography>
              </Box>
            )}

            {isManual && (
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Neighbor Address:
                </Typography>
                <Typography variant="caption" className="mono-font" sx={{ color: "#7C3AED", fontWeight: 600 }}>
                  {link.from.neighbor_address || link.to.address || "(Auto from peer)"}
                </Typography>
              </Box>
            )}

            {!isManual && (
              <>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Listen Port:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                      {link.from.listen_port}
                    </Typography>
                    {Boolean(link.from.listen_port) && (
                      <Tooltip title={copiedField === "from_port" ? "Copied!" : "Copy Listen Port"}>
                        <IconButton
                          size="small"
                          onClick={() => copyFieldToClipboard(String(link.from.listen_port), "from_port")}
                          sx={{ p: 0.3, color: copiedField === "from_port" ? "#10B981" : "#64748B" }}
                        >
                          {copiedField === "from_port" ? <Check size={13} /> : <Copy size={13} />}
                        </IconButton>
                      </Tooltip>
                    )}
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    MTU:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                      {link.from.mtu || 1420}
                    </Typography>
                    <Tooltip title={copiedField === "from_mtu" ? "Copied!" : "Copy MTU"}>
                      <IconButton
                        size="small"
                        onClick={() => copyFieldToClipboard(String(link.from.mtu || 1420), "from_mtu")}
                        sx={{ p: 0.3, color: copiedField === "from_mtu" ? "#10B981" : "#64748B" }}
                      >
                        {copiedField === "from_mtu" ? <Check size={13} /> : <Copy size={13} />}
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Endpoint:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                      {link.to.resolved_endpoint || link.to.endpoint || "Dynamic / Automatic"}
                    </Typography>
                    {link.to.use_ip && (
                      <Chip
                        label="IP Resolved"
                        size="small"
                        sx={{
                          height: 18,
                          fontSize: "0.62rem",
                          fontWeight: 700,
                          bgcolor: "rgba(16, 185, 129, 0.15)",
                          color: "#059669",
                          borderRadius: 1,
                        }}
                      />
                    )}
                    {(link.to.resolved_endpoint || link.to.endpoint) &&
                      (link.to.resolved_endpoint || link.to.endpoint) !== "Dynamic / Automatic" && (
                        <Tooltip title={copiedEndpoint === "from" ? "Copied!" : "Copy Endpoint"}>
                          <IconButton
                            size="small"
                            onClick={() =>
                              copyEndpointToClipboard(link.to.resolved_endpoint || link.to.endpoint || "", "from")
                            }
                            sx={{ p: 0.3, color: copiedEndpoint === "from" ? "#10B981" : "#64748B" }}
                          >
                            {copiedEndpoint === "from" ? <Check size={13} /> : <Copy size={13} />}
                          </IconButton>
                        </Tooltip>
                      )}
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Use IP (DNS Resolve):
                  </Typography>
                  <Chip
                    label={link.from.use_ip ? "Enabled" : "Disabled"}
                    size="small"
                    sx={{
                      height: 20,
                      fontSize: "0.65rem",
                      fontWeight: 700,
                      bgcolor: link.from.use_ip ? "rgba(16, 185, 129, 0.12)" : "rgba(148, 163, 184, 0.15)",
                      color: link.from.use_ip ? "#059669" : "#64748B",
                      border: "1px solid",
                      borderColor: link.from.use_ip ? "rgba(16, 185, 129, 0.3)" : "rgba(148, 163, 184, 0.2)",
                    }}
                  />
                </Box>
              </>
            )}

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> Network Policy:
              </Typography>
              <Chip
                label={link.from.policy || "default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor:
                    link.from.policy === "none"
                      ? "rgba(148, 163, 184, 0.15)"
                      : link.from.policy === "dn42"
                        ? "rgba(109, 40, 217, 0.12)"
                        : "rgba(79, 70, 229, 0.12)",
                  color: link.from.policy === "none" ? "#64748B" : link.from.policy === "dn42" ? "#6D28D9" : "#4F46E5",
                  border: "1px solid",
                  borderColor:
                    link.from.policy === "none"
                      ? "rgba(148, 163, 184, 0.2)"
                      : link.from.policy === "dn42"
                        ? "rgba(109, 40, 217, 0.25)"
                        : "rgba(79, 70, 229, 0.25)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Route size={12} /> Routing Policy:
              </Typography>
              <Chip
                label={
                  link.from.routing_policy
                    ? link.from.routing_policy === "stub"
                      ? "Stub (Override)"
                      : link.from.routing_policy === "receive_only"
                        ? "Receive Only (Override)"
                        : link.from.routing_policy === "advertise_only"
                          ? "Advertise Only (Override)"
                          : `${link.from.routing_policy} (Override)`
                    : "Inherited"
                }
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.from.routing_policy ? "rgba(245, 158, 11, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.from.routing_policy ? "#D97706" : "#64748B",
                  border: "1px solid",
                  borderColor: link.from.routing_policy ? "rgba(245, 158, 11, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Gauge size={12} /> Link Cost:
              </Typography>
              <Chip
                label={
                  link.from.cost && link.from.cost !== 0
                    ? `${link.from.cost} (Override)`
                    : `Policy default (${getEffectiveLinkCost(link.from, Boolean(isToExternal), effectivePolicies)})`
                }
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor:
                    link.from.cost && link.from.cost !== 0 ? "rgba(245, 158, 11, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.from.cost && link.from.cost !== 0 ? "#D97706" : "#64748B",
                  border: "1px solid",
                  borderColor:
                    link.from.cost && link.from.cost !== 0 ? "rgba(245, 158, 11, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> FwMark:
              </Typography>
              <Chip
                label={link.from.fwmark ? `${link.from.fwmark} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.from.fwmark ? "rgba(109, 40, 217, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.from.fwmark ? "#6D28D9" : "#64748B",
                  border: "1px solid",
                  borderColor: link.from.fwmark ? "rgba(109, 40, 217, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Zap size={12} /> BGP Preference:
              </Typography>
              <Chip
                label={link.from.preference !== undefined ? `${link.from.preference} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor:
                    link.from.preference !== undefined ? "rgba(16, 185, 129, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.from.preference !== undefined ? "#059669" : "#64748B",
                  border: "1px solid",
                  borderColor:
                    link.from.preference !== undefined ? "rgba(16, 185, 129, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> Netfilter Mark:
              </Typography>
              <Chip
                label={link.from.mark ? `${link.from.mark} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.from.mark ? "rgba(109, 40, 217, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.from.mark ? "#6D28D9" : "#64748B",
                  border: "1px solid",
                  borderColor: link.from.mark ? "rgba(109, 40, 217, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            {!isManual && (
              <Box sx={{ mt: 0.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}
                  >
                    <Key size={11} /> Public Key:
                  </Typography>
                  {link.from.public_key && (
                    <Tooltip title={copiedKey === "from" ? "Copied!" : "Copy Public Key"}>
                      <IconButton
                        size="small"
                        onClick={() => copyToClipboard(link.from.public_key, "from")}
                        sx={{ p: 0.3, color: copiedKey === "from" ? "#10B981" : "#64748B" }}
                      >
                        {copiedKey === "from" ? <Check size={12} /> : <Copy size={12} />}
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
                <Typography
                  variant="caption"
                  className="mono-font"
                  sx={{
                    display: "block",
                    mt: 0.2,
                    p: 0.8,
                    backgroundColor: "#FFFFFF",
                    border: "1px solid #CBD5E1",
                    borderRadius: 1,
                    fontSize: "0.68rem",
                    wordBreak: "break-all",
                    color: "#475569",
                  }}
                >
                  {link.from.public_key || "(None)"}
                </Typography>
              </Box>
            )}

            {!isManual && !isFromExternal && (
              <Box sx={{ mt: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}
                  >
                    <Shield size={11} /> Private Key:
                  </Typography>
                  <Box sx={{ display: "flex", gap: 0.8 }}>
                    <Button
                      id={`reveal-privkey-${link.from.name}`}
                      size="small"
                      variant="text"
                      onClick={() => handleOpenKeyModal("reveal_private_key", link.from.name)}
                      sx={{
                        fontSize: "0.68rem",
                        p: 0.2,
                        minWidth: "auto",
                        textTransform: "none",
                        color: "#4F46E5",
                        fontWeight: 600,
                      }}
                    >
                      Reveal
                    </Button>
                    <Typography variant="caption" sx={{ color: "#CBD5E1", alignSelf: "center" }}>
                      •
                    </Typography>
                    <Button
                      id={`reset-privkey-${link.from.name}`}
                      size="small"
                      variant="text"
                      onClick={() => handleOpenKeyModal("reset_private_key", link.from.name)}
                      sx={{
                        fontSize: "0.68rem",
                        p: 0.2,
                        minWidth: "auto",
                        textTransform: "none",
                        color: "#D97706",
                        fontWeight: 600,
                      }}
                    >
                      Reset
                    </Button>
                  </Box>
                </Box>
                <Typography
                  variant="caption"
                  className="mono-font"
                  sx={{
                    display: "block",
                    mt: 0.2,
                    p: 0.8,
                    backgroundColor: "#FFFFFF",
                    border: "1px solid #CBD5E1",
                    borderRadius: 1,
                    fontSize: "0.68rem",
                    wordBreak: "break-all",
                    color: "#94A3B8",
                  }}
                >
                  •••••••••••••••••••••••••••••••••••••••••• (Vault Encrypted)
                </Typography>
              </Box>
            )}

            {/* Node 1 LinkEnd Note */}
            <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #C7D2FE" }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
                <Typography
                  variant="caption"
                  sx={{ color: "#4338CA", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}
                >
                  <FileText size={13} color="#4F46E5" /> {link.from.name} NOTE (MARKDOWN)
                </Typography>
                {!isEditingFromNote ? (
                  <Button
                    id={`edit-from-note-${link.from.name}`}
                    size="small"
                    variant="outlined"
                    startIcon={<Edit2 size={11} />}
                    onClick={() => {
                      setFromNoteDraft(link.from.note || "");
                      setIsEditingFromNote(true);
                      setFromNoteTab("write");
                    }}
                    sx={{
                      fontSize: "0.68rem",
                      py: 0.1,
                      px: 0.8,
                      textTransform: "none",
                      fontWeight: 600,
                      borderRadius: 1,
                      borderColor: "#C7D2FE",
                      color: "#4338CA",
                      "&:hover": { borderColor: "#4F46E5", backgroundColor: "rgba(79, 70, 229, 0.05)" },
                    }}
                  >
                    {link.from.note ? "Edit Note" : "Add Note"}
                  </Button>
                ) : (
                  <Box sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                    <Button
                      size="small"
                      variant={fromNoteTab === "write" ? "contained" : "text"}
                      onClick={() => setFromNoteTab("write")}
                      sx={{ minWidth: "auto", px: 0.8, py: 0.1, fontSize: "0.68rem", textTransform: "none" }}
                    >
                      Write
                    </Button>
                    <Button
                      size="small"
                      variant={fromNoteTab === "preview" ? "contained" : "text"}
                      onClick={() => setFromNoteTab("preview")}
                      sx={{ minWidth: "auto", px: 0.8, py: 0.1, fontSize: "0.68rem", textTransform: "none" }}
                    >
                      Preview
                    </Button>
                  </Box>
                )}
              </Box>

              {!isEditingFromNote ? (
                <Box
                  sx={{
                    p: 1,
                    borderRadius: 1,
                    backgroundColor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    maxHeight: 180,
                    overflowY: "auto",
                  }}
                >
                  <MarkdownView
                    content={link.from.note}
                    emptyText="No note recorded for this endpoint. Click 'Add Note' to record markdown notes."
                  />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  {fromNoteTab === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={8}
                      size="small"
                      placeholder="Input arbitrary markdown format text..."
                      value={fromNoteDraft}
                      onChange={(e) => setFromNoteDraft(e.target.value)}
                      disabled={savingFromNote}
                      sx={{
                        backgroundColor: "#FFFFFF",
                        borderRadius: 1,
                        "& .MuiInputBase-root": {
                          fontSize: "0.8rem",
                          fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
                        },
                      }}
                    />
                  ) : (
                    <Box
                      sx={{
                        p: 1,
                        borderRadius: 1,
                        backgroundColor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
                        minHeight: 60,
                        maxHeight: 180,
                        overflowY: "auto",
                      }}
                    >
                      <MarkdownView content={fromNoteDraft} emptyText="No markdown note content" />
                    </Box>
                  )}
                  <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.8, mt: 0.2 }}>
                    <Button
                      size="small"
                      onClick={() => setIsEditingFromNote(false)}
                      disabled={savingFromNote}
                      sx={{ fontSize: "0.72rem", textTransform: "none", color: "#64748B" }}
                    >
                      Cancel
                    </Button>
                    <Button
                      id={`save-from-note-${link.from.name}`}
                      size="small"
                      variant="contained"
                      color="primary"
                      onClick={handleSaveFromNote}
                      disabled={savingFromNote}
                      startIcon={savingFromNote ? <CircularProgress size={11} color="inherit" /> : <Check size={11} />}
                      sx={{ fontSize: "0.72rem", textTransform: "none", fontWeight: 600, px: 1.2 }}
                    >
                      {savingFromNote ? "Saving..." : "Save Note"}
                    </Button>
                  </Box>
                </Box>
              )}
            </Box>
          </Box>
        </Box>

        {/* Center Divider Icon */}
        <Box sx={{ display: "flex", justifyContent: "center", my: -1 }}>
          <Box
            sx={{
              p: 0.7,
              borderRadius: "50%",
              backgroundColor: "#FFFFFF",
              border: "1px solid #CBD5E1",
              color: "#64748B",
            }}
          >
            <ArrowRightLeft size={16} />
          </Box>
        </Box>

        {/* End 2 */}
        <Box
          sx={{
            p: 2,
            borderRadius: 2,
            backgroundColor: "#ECFEFF",
            border: "1px solid #A5F3FC",
          }}
        >
          <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: 1.5 }}>
            <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0E7490" }}>
              Node 2: {link.to.name}
            </Typography>
            {!isManual && (
              <Tooltip
                title={
                  isToExternal
                    ? "Cannot restart interface on external node"
                    : `Restart ${link.to.interface} on ${link.to.name}`
                }
              >
                <span>
                  <Button
                    id={`restart-link-end-to-${link.to.name}-${link.to.interface}`}
                    size="small"
                    variant="outlined"
                    startIcon={restartingTo ? <CircularProgress size={12} color="inherit" /> : <RotateCcw size={12} />}
                    onClick={() => handleRestartEnd("to")}
                    disabled={restartingFrom || restartingTo || deleting || Boolean(isToExternal)}
                    sx={{
                      fontSize: "0.72rem",
                      py: 0.2,
                      px: 1,
                      textTransform: "none",
                      fontWeight: 600,
                      borderRadius: 1.5,
                      borderColor: "#A5F3FC",
                      color: "#0891B2",
                      "&:hover": {
                        borderColor: "#06B6D4",
                        backgroundColor: "rgba(6, 182, 212, 0.08)",
                      },
                    }}
                  >
                    {restartingTo ? "Restarting..." : "Restart"}
                  </Button>
                </span>
              </Tooltip>
            )}
          </Box>

          <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
            <Box sx={{ display: "flex", justifyContent: "space-between" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                Interface:
              </Typography>
              <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                {link.to.interface}
              </Typography>
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                ASN:
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                  {toDisplayedAsn !== undefined && toDisplayedAsn !== null ? `AS${toDisplayedAsn}` : "—"}
                </Typography>
                {toIsGlobalAsn && (
                  <Chip
                    label="Global ASN"
                    size="small"
                    sx={{
                      height: 18,
                      fontSize: "0.62rem",
                      fontWeight: 700,
                      bgcolor: "rgba(79, 70, 229, 0.12)",
                      color: "#4F46E5",
                      border: "1px solid rgba(79, 70, 229, 0.25)",
                      borderRadius: "4px",
                    }}
                  />
                )}
                {toDisplayedAsn !== undefined && toDisplayedAsn !== null && (
                  <Tooltip title={copiedField === "to_asn" ? "Copied!" : "Copy ASN"}>
                    <IconButton
                      size="small"
                      onClick={() => copyFieldToClipboard(String(toDisplayedAsn), "to_asn")}
                      sx={{ p: 0.3, color: copiedField === "to_asn" ? "#10B981" : "#64748B" }}
                    >
                      {copiedField === "to_asn" ? <Check size={13} /> : <Copy size={13} />}
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B" }}>
                {isManual ? "Local Address:" : "Link-Local IPv6:"}
              </Typography>
              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                <Typography variant="caption" className="mono-font" sx={{ color: "#0891B2", fontWeight: 600 }}>
                  {link.to.address}
                </Typography>
                {link.to.address && (
                  <Tooltip title={copiedField === "to_ipv6" ? "Copied!" : "Copy Link-Local IPv6"}>
                    <IconButton
                      size="small"
                      onClick={() => copyFieldToClipboard(getLinkLocalIpv6CopyText(link.to.address), "to_ipv6")}
                      sx={{ p: 0.3, color: copiedField === "to_ipv6" ? "#10B981" : "#64748B" }}
                    >
                      {copiedField === "to_ipv6" ? <Check size={13} /> : <Copy size={13} />}
                    </IconButton>
                  </Tooltip>
                )}
              </Box>
            </Box>

            {link.assign_ipv4 && (
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Link-Local IPv4:
                </Typography>
                <Typography variant="caption" className="mono-font" sx={{ color: "#059669", fontWeight: 600 }}>
                  {link.to.address4 || "169.254.X.X/32 (Auto)"}
                </Typography>
              </Box>
            )}

            {isManual && (
              <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                <Typography variant="caption" sx={{ color: "#64748B" }}>
                  Neighbor Address:
                </Typography>
                <Typography variant="caption" className="mono-font" sx={{ color: "#7C3AED", fontWeight: 600 }}>
                  {link.to.neighbor_address || link.from.address || "(Auto from peer)"}
                </Typography>
              </Box>
            )}

            {!isManual && (
              <>
                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Listen Port:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                      {link.to.listen_port}
                    </Typography>
                    {Boolean(link.to.listen_port) && (
                      <Tooltip title={copiedField === "to_port" ? "Copied!" : "Copy Listen Port"}>
                        <IconButton
                          size="small"
                          onClick={() => copyFieldToClipboard(String(link.to.listen_port), "to_port")}
                          sx={{ p: 0.3, color: copiedField === "to_port" ? "#10B981" : "#64748B" }}
                        >
                          {copiedField === "to_port" ? <Check size={13} /> : <Copy size={13} />}
                        </IconButton>
                      </Tooltip>
                    )}
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    MTU:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 600 }}>
                      {link.to.mtu || 1420}
                    </Typography>
                    <Tooltip title={copiedField === "to_mtu" ? "Copied!" : "Copy MTU"}>
                      <IconButton
                        size="small"
                        onClick={() => copyFieldToClipboard(String(link.to.mtu || 1420), "to_mtu")}
                        sx={{ p: 0.3, color: copiedField === "to_mtu" ? "#10B981" : "#64748B" }}
                      >
                        {copiedField === "to_mtu" ? <Check size={13} /> : <Copy size={13} />}
                      </IconButton>
                    </Tooltip>
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Endpoint:
                  </Typography>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8 }}>
                    <Typography variant="caption" className="mono-font" sx={{ color: "#0F172A", fontWeight: 700 }}>
                      {link.from.resolved_endpoint || link.from.endpoint || "Dynamic / Automatic"}
                    </Typography>
                    {link.from.use_ip && (
                      <Chip
                        label="IP Resolved"
                        size="small"
                        sx={{
                          height: 18,
                          fontSize: "0.62rem",
                          fontWeight: 700,
                          bgcolor: "rgba(16, 185, 129, 0.15)",
                          color: "#059669",
                          borderRadius: 1,
                        }}
                      />
                    )}
                    {(link.from.resolved_endpoint || link.from.endpoint) &&
                      (link.from.resolved_endpoint || link.from.endpoint) !== "Dynamic / Automatic" && (
                        <Tooltip title={copiedEndpoint === "to" ? "Copied!" : "Copy Endpoint"}>
                          <IconButton
                            size="small"
                            onClick={() =>
                              copyEndpointToClipboard(link.from.resolved_endpoint || link.from.endpoint || "", "to")
                            }
                            sx={{ p: 0.3, color: copiedEndpoint === "to" ? "#10B981" : "#64748B" }}
                          >
                            {copiedEndpoint === "to" ? <Check size={13} /> : <Copy size={13} />}
                          </IconButton>
                        </Tooltip>
                      )}
                  </Box>
                </Box>

                <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Use IP (DNS Resolve):
                  </Typography>
                  <Chip
                    label={link.to.use_ip ? "Enabled" : "Disabled"}
                    size="small"
                    sx={{
                      height: 20,
                      fontSize: "0.65rem",
                      fontWeight: 700,
                      bgcolor: link.to.use_ip ? "rgba(16, 185, 129, 0.12)" : "rgba(148, 163, 184, 0.15)",
                      color: link.to.use_ip ? "#059669" : "#64748B",
                      border: "1px solid",
                      borderColor: link.to.use_ip ? "rgba(16, 185, 129, 0.3)" : "rgba(148, 163, 184, 0.2)",
                    }}
                  />
                </Box>
              </>
            )}

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> Network Policy:
              </Typography>
              <Chip
                label={link.to.policy || "default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor:
                    link.to.policy === "none"
                      ? "rgba(148, 163, 184, 0.15)"
                      : link.to.policy === "dn42"
                        ? "rgba(109, 40, 217, 0.12)"
                        : "rgba(79, 70, 229, 0.12)",
                  color: link.to.policy === "none" ? "#64748B" : link.to.policy === "dn42" ? "#6D28D9" : "#4F46E5",
                  border: "1px solid",
                  borderColor:
                    link.to.policy === "none"
                      ? "rgba(148, 163, 184, 0.2)"
                      : link.to.policy === "dn42"
                        ? "rgba(109, 40, 217, 0.25)"
                        : "rgba(79, 70, 229, 0.25)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Route size={12} /> Routing Policy:
              </Typography>
              <Chip
                label={
                  link.to.routing_policy
                    ? link.to.routing_policy === "stub"
                      ? "Stub (Override)"
                      : link.to.routing_policy === "receive_only"
                        ? "Receive Only (Override)"
                        : link.to.routing_policy === "advertise_only"
                          ? "Advertise Only (Override)"
                          : `${link.to.routing_policy} (Override)`
                    : "Inherited"
                }
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.to.routing_policy ? "rgba(245, 158, 11, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.to.routing_policy ? "#D97706" : "#64748B",
                  border: "1px solid",
                  borderColor: link.to.routing_policy ? "rgba(245, 158, 11, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Gauge size={12} /> Link Cost:
              </Typography>
              <Chip
                label={
                  link.to.cost && link.to.cost !== 0
                    ? `${link.to.cost} (Override)`
                    : `Policy default (${getEffectiveLinkCost(link.to, Boolean(isFromExternal), effectivePolicies)})`
                }
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor:
                    link.to.cost && link.to.cost !== 0 ? "rgba(245, 158, 11, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.to.cost && link.to.cost !== 0 ? "#D97706" : "#64748B",
                  border: "1px solid",
                  borderColor:
                    link.to.cost && link.to.cost !== 0 ? "rgba(245, 158, 11, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> FwMark:
              </Typography>
              <Chip
                label={link.to.fwmark ? `${link.to.fwmark} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.to.fwmark ? "rgba(109, 40, 217, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.to.fwmark ? "#6D28D9" : "#64748B",
                  border: "1px solid",
                  borderColor: link.to.fwmark ? "rgba(109, 40, 217, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Zap size={12} /> BGP Preference:
              </Typography>
              <Chip
                label={link.to.preference !== undefined ? `${link.to.preference} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.to.preference !== undefined ? "rgba(16, 185, 129, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.to.preference !== undefined ? "#059669" : "#64748B",
                  border: "1px solid",
                  borderColor:
                    link.to.preference !== undefined ? "rgba(16, 185, 129, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <Typography variant="caption" sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}>
                <Shield size={12} /> Netfilter Mark:
              </Typography>
              <Chip
                label={link.to.mark ? `${link.to.mark} (Override)` : "Policy default"}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  bgcolor: link.to.mark ? "rgba(109, 40, 217, 0.12)" : "rgba(148, 163, 184, 0.15)",
                  color: link.to.mark ? "#6D28D9" : "#64748B",
                  border: "1px solid",
                  borderColor: link.to.mark ? "rgba(109, 40, 217, 0.3)" : "rgba(148, 163, 184, 0.2)",
                }}
              />
            </Box>

            {!isManual && (
              <Box sx={{ mt: 0.5 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}
                  >
                    <Key size={11} /> Public Key:
                  </Typography>
                  {link.to.public_key && (
                    <Tooltip title={copiedKey === "to" ? "Copied!" : "Copy Public Key"}>
                      <IconButton
                        size="small"
                        onClick={() => copyToClipboard(link.to.public_key, "to")}
                        sx={{ p: 0.3, color: copiedKey === "to" ? "#10B981" : "#64748B" }}
                      >
                        {copiedKey === "to" ? <Check size={12} /> : <Copy size={12} />}
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
                <Typography
                  variant="caption"
                  className="mono-font"
                  sx={{
                    display: "block",
                    mt: 0.2,
                    p: 0.8,
                    backgroundColor: "#FFFFFF",
                    border: "1px solid #CBD5E1",
                    borderRadius: 1,
                    fontSize: "0.68rem",
                    wordBreak: "break-all",
                    color: "#475569",
                  }}
                >
                  {link.to.public_key || "(None)"}
                </Typography>
              </Box>
            )}

            {!isManual && !isToExternal && (
              <Box sx={{ mt: 1 }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <Typography
                    variant="caption"
                    sx={{ color: "#64748B", display: "flex", alignItems: "center", gap: 0.5 }}
                  >
                    <Shield size={11} /> Private Key:
                  </Typography>
                  <Box sx={{ display: "flex", gap: 0.8 }}>
                    <Button
                      id={`reveal-privkey-${link.to.name}`}
                      size="small"
                      variant="text"
                      onClick={() => handleOpenKeyModal("reveal_private_key", link.to.name)}
                      sx={{
                        fontSize: "0.68rem",
                        p: 0.2,
                        minWidth: "auto",
                        textTransform: "none",
                        color: "#0891B2",
                        fontWeight: 600,
                      }}
                    >
                      Reveal
                    </Button>
                    <Typography variant="caption" sx={{ color: "#CBD5E1", alignSelf: "center" }}>
                      •
                    </Typography>
                    <Button
                      id={`reset-privkey-${link.to.name}`}
                      size="small"
                      variant="text"
                      onClick={() => handleOpenKeyModal("reset_private_key", link.to.name)}
                      sx={{
                        fontSize: "0.68rem",
                        p: 0.2,
                        minWidth: "auto",
                        textTransform: "none",
                        color: "#D97706",
                        fontWeight: 600,
                      }}
                    >
                      Reset
                    </Button>
                  </Box>
                </Box>
                <Typography
                  variant="caption"
                  className="mono-font"
                  sx={{
                    display: "block",
                    mt: 0.2,
                    p: 0.8,
                    backgroundColor: "#FFFFFF",
                    border: "1px solid #CBD5E1",
                    borderRadius: 1,
                    fontSize: "0.68rem",
                    wordBreak: "break-all",
                    color: "#94A3B8",
                  }}
                >
                  •••••••••••••••••••••••••••••••••••••••••• (Vault Encrypted)
                </Typography>
              </Box>
            )}

            {/* Node 2 LinkEnd Note */}
            <Box sx={{ mt: 1.5, p: 1.5, borderRadius: 1.5, backgroundColor: "#FFFFFF", border: "1px solid #A5F3FC" }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1 }}>
                <Typography
                  variant="caption"
                  sx={{ color: "#0E7490", fontWeight: 700, display: "flex", alignItems: "center", gap: 0.6 }}
                >
                  <FileText size={13} color="#0891B2" /> {link.to.name} NOTE (MARKDOWN)
                </Typography>
                {!isEditingToNote ? (
                  <Button
                    id={`edit-to-note-${link.to.name}`}
                    size="small"
                    variant="outlined"
                    startIcon={<Edit2 size={11} />}
                    onClick={() => {
                      setToNoteDraft(link.to.note || "");
                      setIsEditingToNote(true);
                      setToNoteTab("write");
                    }}
                    sx={{
                      fontSize: "0.68rem",
                      py: 0.1,
                      px: 0.8,
                      textTransform: "none",
                      fontWeight: 600,
                      borderRadius: 1,
                      borderColor: "#A5F3FC",
                      color: "#0891B2",
                      "&:hover": { borderColor: "#06B6D4", backgroundColor: "rgba(6, 182, 212, 0.05)" },
                    }}
                  >
                    {link.to.note ? "Edit Note" : "Add Note"}
                  </Button>
                ) : (
                  <Box sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                    <Button
                      size="small"
                      variant={toNoteTab === "write" ? "contained" : "text"}
                      onClick={() => setToNoteTab("write")}
                      sx={{ minWidth: "auto", px: 0.8, py: 0.1, fontSize: "0.68rem", textTransform: "none" }}
                    >
                      Write
                    </Button>
                    <Button
                      size="small"
                      variant={toNoteTab === "preview" ? "contained" : "text"}
                      onClick={() => setToNoteTab("preview")}
                      sx={{ minWidth: "auto", px: 0.8, py: 0.1, fontSize: "0.68rem", textTransform: "none" }}
                    >
                      Preview
                    </Button>
                  </Box>
                )}
              </Box>

              {!isEditingToNote ? (
                <Box
                  sx={{
                    p: 1,
                    borderRadius: 1,
                    backgroundColor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    maxHeight: 180,
                    overflowY: "auto",
                  }}
                >
                  <MarkdownView
                    content={link.to.note}
                    emptyText="No note recorded for this endpoint. Click 'Add Note' to record markdown notes."
                  />
                </Box>
              ) : (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
                  {toNoteTab === "write" ? (
                    <TextField
                      fullWidth
                      multiline
                      minRows={2}
                      maxRows={8}
                      size="small"
                      placeholder="Input arbitrary markdown format text..."
                      value={toNoteDraft}
                      onChange={(e) => setToNoteDraft(e.target.value)}
                      disabled={savingToNote}
                      sx={{
                        backgroundColor: "#FFFFFF",
                        borderRadius: 1,
                        "& .MuiInputBase-root": {
                          fontSize: "0.8rem",
                          fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
                        },
                      }}
                    />
                  ) : (
                    <Box
                      sx={{
                        p: 1,
                        borderRadius: 1,
                        backgroundColor: "#F8FAFC",
                        border: "1px solid #E2E8F0",
                        minHeight: 60,
                        maxHeight: 180,
                        overflowY: "auto",
                      }}
                    >
                      <MarkdownView content={toNoteDraft} emptyText="No markdown note content" />
                    </Box>
                  )}
                  <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 0.8, mt: 0.2 }}>
                    <Button
                      size="small"
                      onClick={() => setIsEditingToNote(false)}
                      disabled={savingToNote}
                      sx={{ fontSize: "0.72rem", textTransform: "none", color: "#64748B" }}
                    >
                      Cancel
                    </Button>
                    <Button
                      id={`save-to-note-${link.to.name}`}
                      size="small"
                      variant="contained"
                      color="primary"
                      onClick={handleSaveToNote}
                      disabled={savingToNote}
                      startIcon={savingToNote ? <CircularProgress size={11} color="inherit" /> : <Check size={11} />}
                      sx={{ fontSize: "0.72rem", textTransform: "none", fontWeight: 600, px: 1.2 }}
                    >
                      {savingToNote ? "Saving..." : "Save Note"}
                    </Button>
                  </Box>
                </Box>
              )}
            </Box>
          </Box>
        </Box>

        {actionSuccess && (
          <Alert severity="success" sx={{ borderRadius: 2 }}>
            {actionSuccess}
          </Alert>
        )}

        {error && (
          <Alert severity="error" sx={{ borderRadius: 2 }}>
            {error}
          </Alert>
        )}
      </Box>

      {/* Actions */}
      <Box sx={{ mt: "auto", pt: 3, display: "flex", flexDirection: "column", gap: 1.5 }}>
        {isExternalLink && (
          <Button
            id="btn-copy-peering-info"
            fullWidth
            variant="outlined"
            startIcon={copiedPeeringInfo ? <Check size={16} /> : <Share2 size={16} />}
            onClick={handleCopyPeeringInfo}
            disabled={deleting}
            sx={{
              borderColor: copiedPeeringInfo ? "#10B981" : "#818CF8",
              color: copiedPeeringInfo ? "#059669" : "#4F46E5",
              backgroundColor: copiedPeeringInfo ? "rgba(16, 185, 129, 0.08)" : "rgba(99, 102, 241, 0.05)",
              fontWeight: 700,
              textTransform: "none",
              py: 0.9,
              borderRadius: 1.5,
              "&:hover": {
                borderColor: copiedPeeringInfo ? "#059669" : "#4F46E5",
                backgroundColor: copiedPeeringInfo ? "rgba(16, 185, 129, 0.12)" : "rgba(99, 102, 241, 0.1)",
              },
            }}
          >
            {copiedPeeringInfo ? "Peering Info Copied!" : "Copy Peering Info"}
          </Button>
        )}
        {onRefreshLink && (
          <Button
            fullWidth
            variant="outlined"
            startIcon={refreshing ? <CircularProgress size={16} color="inherit" /> : <RefreshCw size={16} />}
            onClick={handleRefreshLink}
            disabled={refreshing || deleting}
            sx={{
              borderColor: "#CBD5E1",
              color: "#334155",
              fontWeight: 600,
              textTransform: "none",
              "&:hover": {
                borderColor: "#0284C7",
                backgroundColor: "rgba(2, 132, 199, 0.05)",
                color: "#0284C7",
              },
            }}
          >
            {refreshing ? "Refreshing Link State..." : "Refresh Link State"}
          </Button>
        )}
        <Button
          fullWidth
          variant="contained"
          color="primary"
          startIcon={<Edit2 size={16} />}
          onClick={() => onEditLink(link)}
          disabled={refreshing || deleting}
        >
          Edit Link
        </Button>
        <Button
          fullWidth
          variant="outlined"
          color="error"
          startIcon={deleting ? <CircularProgress size={16} color="inherit" /> : <Trash2 size={16} />}
          onClick={handleDelete}
          disabled={deleting}
        >
          {deleting ? "Deleting..." : "Delete WireGuard Link"}
        </Button>
      </Box>

      <LinkKeyActionModal
        open={keyModalOpen}
        onClose={() => setKeyModalOpen(false)}
        mode={keyModalMode}
        link={link}
        nodeName={keyModalNode}
        onSuccess={() => {
          if (onRefreshLink && link) {
            onRefreshLink(link);
          }
        }}
      />

      {/* Peering Info PSK Verification Dialog */}
      <Dialog
        open={peeringAuthModalOpen}
        onClose={() => setPeeringAuthModalOpen(false)}
        maxWidth="sm"
        fullWidth
        sx={{ zIndex: 1400 }}
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1.5,
            pb: 1.5,
            borderBottom: "1px solid #E2E8F0",
          }}
        >
          <Box
            sx={{
              width: 38,
              height: 38,
              borderRadius: 2,
              backgroundColor: "rgba(99, 102, 241, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#4F46E5",
            }}
          >
            <Lock size={20} />
          </Box>
          <Box>
            <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
              Copy Peering Info
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              Administrator password verification required to decrypt Pre-shared Key (PSK).
            </Typography>
          </Box>
        </DialogTitle>

        {peeringPreviewText ? (
          <Box sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 2 }}>
            <Alert severity="success" sx={{ borderRadius: 2 }}>
              Peering info copied to clipboard successfully!
            </Alert>
            <Box
              sx={{
                p: 2,
                borderRadius: 2,
                backgroundColor: "#F8FAFC",
                border: "1px solid #E2E8F0",
                maxHeight: 280,
                overflowY: "auto",
              }}
            >
              <Typography
                variant="body2"
                className="mono-font"
                sx={{
                  whiteSpace: "pre-wrap",
                  fontSize: "0.78rem",
                  color: "#0F172A",
                  lineHeight: 1.6,
                }}
              >
                {peeringPreviewText}
              </Typography>
            </Box>
            <DialogActions sx={{ px: 0, pb: 0 }}>
              <Button
                variant="outlined"
                startIcon={<Copy size={15} />}
                onClick={() => {
                  navigator.clipboard.writeText(peeringPreviewText);
                  setActionSuccess("Peering info copied to clipboard");
                }}
                sx={{ textTransform: "none", fontWeight: 600 }}
              >
                Copy Again
              </Button>
              <Button
                variant="contained"
                onClick={() => setPeeringAuthModalOpen(false)}
                sx={{ textTransform: "none", fontWeight: 600 }}
              >
                Done
              </Button>
            </DialogActions>
          </Box>
        ) : (
          <form onSubmit={handleVerifyAndCopyPeeringInfo}>
            <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 2.5 }}>
              {peeringAuthError && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {peeringAuthError}
                </Alert>
              )}
              <Typography variant="body2" sx={{ color: "#475569" }}>
                This link has a WireGuard Pre-shared Key (PSK) configured. Please authenticate with your easy42
                administrator password to decrypt the PSK and copy the peering info:
              </Typography>
              <TextField
                id="peering-info-admin-password-input"
                fullWidth
                type={showPeeringPassword ? "text" : "password"}
                label="Administrator Password"
                size="small"
                value={peeringPassword}
                onChange={(e) => setPeeringPassword(e.target.value)}
                autoFocus
                required
                disabled={peeringAuthLoading}
                InputProps={{
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton size="small" onClick={() => setShowPeeringPassword(!showPeeringPassword)} edge="end">
                        {showPeeringPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                      </IconButton>
                    </InputAdornment>
                  ),
                }}
              />
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2.5 }}>
              <Button
                onClick={() => setPeeringAuthModalOpen(false)}
                disabled={peeringAuthLoading}
                sx={{ textTransform: "none", color: "#64748B" }}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                disabled={peeringAuthLoading}
                startIcon={peeringAuthLoading ? <CircularProgress size={14} color="inherit" /> : <Copy size={14} />}
                sx={{ textTransform: "none", fontWeight: 600 }}
              >
                {peeringAuthLoading ? "Verifying..." : "Verify & Copy"}
              </Button>
            </DialogActions>
          </form>
        )}
      </Dialog>
    </Drawer>
  );
};
