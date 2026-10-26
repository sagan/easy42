import React from "react";
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  Chip,
  IconButton,
  Alert,
  Divider,
} from "@mui/material";
import {
  Shield,
  X,
  Lock,
  Route,
  Sliders,
  Filter,
  CheckCircle2,
  XCircle,
  FileCode,
} from "lucide-react";
import { NetworkPolicy } from "../../types/api";

interface ViewNetworkPolicyModalProps {
  open: boolean;
  onClose: () => void;
  policy: NetworkPolicy | null;
  onEdit?: (policy: NetworkPolicy) => void;
}

const routingPolicyLabels: Record<string, string> = {
  full: "Full (Default) — Advertise & receive all valid routes",
  stub: "Stub — Receive all, only send local routes (no transit)",
  receive_only: "Receive Only — Import routes only, advertise none",
  advertise_only: "Advertise Only — Export local routes only, import none",
};

export const ViewNetworkPolicyModal: React.FC<ViewNetworkPolicyModalProps> = ({
  open,
  onClose,
  policy,
  onEdit,
}) => {
  if (!policy) return null;

  const isBuiltin = policy.is_internal || policy.id === "default" || policy.id === "external";
  const exportFilter = policy.bird_export_filter || policy.bird_export_fillter;

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          pb: 1.5,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: 2,
              bgcolor: "#EEF2FF",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#4F46E5",
            }}
          >
            <Shield size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem", color: "#0F172A", lineHeight: 1.2 }}>
              View Network Policy
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 500 }}>
              {policy.name} ({policy.id})
            </Typography>
          </Box>
        </Box>
        <IconButton size="small" onClick={onClose} sx={{ color: "#94A3B8" }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 2.5, px: 3 }}>
        {isBuiltin && (
          <Alert icon={<Lock size={16} />} severity="info" sx={{ py: 0.8, borderRadius: 2 }}>
            Built-in policies are managed by easy42 and are read-only.
          </Alert>
        )}

        {/* Section 1: Overview */}
        <Box
          sx={{
            display: "grid",
            gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" },
            gap: 2,
            p: 2,
            borderRadius: 2,
            bgcolor: "#F8FAFC",
            border: "1px solid #E2E8F0",
          }}
        >
          <Box>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              POLICY ID
            </Typography>
            <Typography variant="body1" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
              {policy.id}
            </Typography>
          </Box>
          <Box>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              POLICY NAME
            </Typography>
            <Typography variant="body1" sx={{ fontWeight: 700, color: "#0F172A" }}>
              {policy.name}
            </Typography>
          </Box>
          {policy.description && (
            <Box sx={{ gridColumn: { xs: "1", sm: "1 / -1" } }}>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                DESCRIPTION
              </Typography>
              <Typography variant="body2" sx={{ color: "#334155" }}>
                {policy.description}
              </Typography>
            </Box>
          )}
        </Box>

        {/* Section 2: Routing & BGP */}
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
            p: 2,
            borderRadius: 2,
            bgcolor: "#FFFFFF",
            border: "1px solid #E2E8F0",
          }}
        >
          <Typography
            variant="subtitle2"
            sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
          >
            <Route size={16} color="#4F46E5" /> Routing & BGP Configuration
          </Typography>

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 2 }}>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                INTERNAL BGP COST
              </Typography>
              <Typography variant="body2" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {policy.cost ?? 100}
              </Typography>
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                BGP PREFERENCE
              </Typography>
              <Typography variant="body2" className="mono-font" sx={{ fontWeight: 700, color: "#0F172A" }}>
                {policy.preference !== undefined ? policy.preference : "Default"}
              </Typography>
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                ROUTING BEHAVIOR
              </Typography>
              <Typography variant="body2" sx={{ fontWeight: 600, color: "#334155" }}>
                {routingPolicyLabels[policy.routing_policy || "full"] || policy.routing_policy || "Full"}
              </Typography>
            </Box>
          </Box>

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2, pt: 1 }}>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                WIREGUARD FWMARK
              </Typography>
              <Typography variant="body2" className="mono-font" sx={{ color: policy.fwmark ? "#0F172A" : "#94A3B8" }}>
                {policy.fwmark || "None"}
              </Typography>
            </Box>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                NETFILTER MARK
              </Typography>
              <Typography variant="body2" className="mono-font" sx={{ color: policy.mark ? "#0F172A" : "#94A3B8" }}>
                {policy.mark || "None"}
              </Typography>
            </Box>
          </Box>

          <Divider sx={{ my: 0.5 }} />

          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            {policy.reject_internet ? (
              <CheckCircle2 size={16} color="#16A34A" />
            ) : (
              <XCircle size={16} color="#94A3B8" />
            )}
            <Typography variant="body2" sx={{ fontWeight: 600, color: policy.reject_internet ? "#15803D" : "#64748B" }}>
              Route Leak Protection (Reject Internet Routes): {policy.reject_internet ? "Enabled" : "Disabled"}
            </Typography>
          </Box>
        </Box>

        {/* Section 3: CIDR Subnet Filters */}
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 1.5,
            p: 2,
            borderRadius: 2,
            bgcolor: "#F8FAFC",
            border: "1px solid #E2E8F0",
          }}
        >
          <Typography
            variant="subtitle2"
            sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
          >
            <Filter size={16} color="#4F46E5" /> Ingress Subnet & Anti-Spoofing Filters
          </Typography>

          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                ALLOWED DESTINATION CIDRs
              </Typography>
              {policy.allowed_dst_cidrs && policy.allowed_dst_cidrs.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                  {policy.allowed_dst_cidrs.map((c, i) => (
                    <Chip key={i} label={c} size="small" className="mono-font" sx={{ fontSize: "0.72rem" }} />
                  ))}
                </Box>
              ) : (
                <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic", mt: 0.5 }}>
                  Unrestricted (any destination)
                </Typography>
              )}
            </Box>

            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                ALLOWED SOURCE CIDRs (ANTI-SPOOFING)
              </Typography>
              {policy.allowed_src_cidrs && policy.allowed_src_cidrs.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                  {policy.allowed_src_cidrs.map((c, i) => (
                    <Chip key={i} label={c} size="small" className="mono-font" sx={{ fontSize: "0.72rem" }} />
                  ))}
                </Box>
              ) : (
                <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic", mt: 0.5 }}>
                  Unrestricted (any source)
                </Typography>
              )}
            </Box>

            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                DISALLOWED DESTINATION CIDRs
              </Typography>
              {policy.disallowed_dst_cidrs && policy.disallowed_dst_cidrs.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                  {policy.disallowed_dst_cidrs.map((c, i) => (
                    <Chip key={i} label={c} size="small" className="mono-font" sx={{ fontSize: "0.72rem", color: "#DC2626", bgcolor: "#FEF2F2" }} />
                  ))}
                </Box>
              ) : (
                <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic", mt: 0.5 }}>
                  None
                </Typography>
              )}
            </Box>

            <Box>
              <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                DISALLOWED SOURCE CIDRs
              </Typography>
              {policy.disallowed_src_cidrs && policy.disallowed_src_cidrs.length > 0 ? (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                  {policy.disallowed_src_cidrs.map((c, i) => (
                    <Chip key={i} label={c} size="small" className="mono-font" sx={{ fontSize: "0.72rem", color: "#DC2626", bgcolor: "#FEF2F2" }} />
                  ))}
                </Box>
              ) : (
                <Typography variant="body2" sx={{ color: "#94A3B8", fontStyle: "italic", mt: 0.5 }}>
                  None
                </Typography>
              )}
            </Box>
          </Box>
        </Box>

        {/* Section 4: Traffic Filtering & Host Firewall */}
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
            p: 2,
            borderRadius: 2,
            bgcolor: "#FFFFFF",
            border: "1px solid #E2E8F0",
          }}
        >
          <Typography
            variant="subtitle2"
            sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
          >
            <Shield size={16} color="#4F46E5" /> Traffic Filtering & Host Firewall
          </Typography>

          {/* Master Filtering Switches */}
          <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
            <Box
              sx={{
                p: 1.5,
                borderRadius: 1.5,
                bgcolor: policy.filter_forward ? "#F0FDF4" : "#F8FAFC",
                border: "1px solid",
                borderColor: policy.filter_forward ? "#BBF7D0" : "#E2E8F0",
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                <Typography variant="body2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                  Filter Forward Traffic
                </Typography>
                <Chip
                  label={policy.filter_forward ? "Enabled" : "Disabled"}
                  size="small"
                  sx={{
                    height: 20,
                    fontSize: "0.68rem",
                    fontWeight: 700,
                    bgcolor: policy.filter_forward ? "#DCFCE7" : "#F1F5F9",
                    color: policy.filter_forward ? "#15803D" : "#64748B",
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                {policy.filter_forward
                  ? "Drops forwarded packets whose source or destination IP does not match allowed CIDRs."
                  : "Forwarded traffic traversing this link is not restricted by nftables."}
              </Typography>
            </Box>

            <Box
              sx={{
                p: 1.5,
                borderRadius: 1.5,
                bgcolor: policy.filter_input ? "#F0FDF4" : "#F8FAFC",
                border: "1px solid",
                borderColor: policy.filter_input ? "#BBF7D0" : "#E2E8F0",
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                <Typography variant="body2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                  Filter Input Traffic (Host Firewall)
                </Typography>
                <Chip
                  label={policy.filter_input ? "Enabled" : "Disabled (Unrestricted)"}
                  size="small"
                  sx={{
                    height: 20,
                    fontSize: "0.68rem",
                    fontWeight: 700,
                    bgcolor: policy.filter_input ? "#DCFCE7" : "#F1F5F9",
                    color: policy.filter_input ? "#15803D" : "#64748B",
                  }}
                />
              </Box>
              <Typography variant="caption" sx={{ color: "#64748B", display: "block" }}>
                {policy.filter_input
                  ? "Restricts incoming traffic targeting the host itself according to the port and ICMP rules below."
                  : "Host firewall is disabled. All incoming traffic (ICMP ping, ICMPv6, and all ports) is permitted."}
              </Typography>
            </Box>
          </Box>

          {/* Sub-rules: When filter_input is Enabled vs Disabled */}
          {policy.filter_input ? (
            <Box sx={{ p: 2, borderRadius: 1.5, bgcolor: "#F8FAFC", border: "1px dashed #CBD5E1" }}>
              <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569", display: "block", mb: 1.5 }}>
                ACTIVE HOST FIREWALL RULES (FILTER INPUT ENABLED)
              </Typography>
              <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    ICMP (IPv4 Ping)
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 600, color: policy.input_allow_icmp ? "#16A34A" : "#DC2626" }}>
                    {policy.input_allow_icmp ? "Allowed" : "Blocked"}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    ICMPv6 (IPv6 Ping & NDP)
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 600, color: policy.input_allow_icmp6 ? "#16A34A" : "#DC2626" }}>
                    {policy.input_allow_icmp6 ? "Allowed" : "Blocked"}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    ALLOWED TCP PORTS
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ color: "#0F172A" }}>
                    {policy.input_tcp_ports?.length ? policy.input_tcp_ports.join(", ") : "BGP (179) only"}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    ALLOWED UDP PORTS
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ color: policy.input_udp_ports?.length ? policy.input_udp_ports.join(", ") : "None" }}>
                    {policy.input_udp_ports?.length ? policy.input_udp_ports.join(", ") : "None"}
                  </Typography>
                </Box>
              </Box>

              {policy.local_networks && policy.local_networks.length > 0 && (
                <Box sx={{ mt: 1.5 }}>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    PROTECTED LOCAL NETWORKS
                  </Typography>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                    {policy.local_networks.map((net, i) => (
                      <Chip key={i} label={net} size="small" className="mono-font" sx={{ fontSize: "0.72rem" }} />
                    ))}
                  </Box>
                </Box>
              )}
            </Box>
          ) : (
            <Alert severity="success" icon={<CheckCircle2 size={16} />} sx={{ py: 0.5, borderRadius: 2 }}>
              <Typography variant="body2" sx={{ fontSize: "0.85rem" }}>
                Host firewall is disabled on this link — ICMP (IPv4 Ping), ICMPv6, and all TCP/UDP ports are <strong>unrestricted and allowed</strong>.
              </Typography>
            </Alert>
          )}

          {policy.block_ingress_new && (
            <Box sx={{ p: 1.5, borderRadius: 1.5, bgcolor: "#FEF2F2", border: "1px solid #FECDD3" }}>
              <Typography variant="caption" sx={{ color: "#991B1B", fontWeight: 700 }}>
                BLOCK INGRESS NEW CONNECTIONS
              </Typography>
              <Typography variant="body2" sx={{ color: "#DC2626", fontWeight: 600 }}>
                Action: {policy.block_ingress_new}
              </Typography>
              {policy.ingress_new_whitelist && policy.ingress_new_whitelist.length > 0 && (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mt: 0.5 }}>
                  {policy.ingress_new_whitelist.map((w, i) => (
                    <Chip key={i} label={w} size="small" className="mono-font" sx={{ fontSize: "0.72rem" }} />
                  ))}
                </Box>
              )}
            </Box>
          )}
        </Box>

        {/* Section 5: ROA, NAT & DSCP */}
        {(policy.roa4 || policy.roa6 || policy.snat?.enabled || policy.forward_snat || policy.dscp_ingress !== undefined || policy.dscp_egress !== undefined) && (
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              gap: 1.5,
              p: 2,
              borderRadius: 2,
              bgcolor: "#F8FAFC",
              border: "1px solid #E2E8F0",
            }}
          >
            <Typography
              variant="subtitle2"
              sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
            >
              <Sliders size={16} color="#4F46E5" /> ROA, NAT & DSCP
            </Typography>

            <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
              {policy.roa4 && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    IPv4 ROA SOURCE
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ fontSize: "0.8rem", wordBreak: "break-all" }}>
                    {policy.roa4}
                  </Typography>
                </Box>
              )}
              {policy.roa6 && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    IPv6 ROA SOURCE
                  </Typography>
                  <Typography variant="body2" className="mono-font" sx={{ fontSize: "0.8rem", wordBreak: "break-all" }}>
                    {policy.roa6}
                  </Typography>
                </Box>
              )}
              {(policy.roa4 || policy.roa6) && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    ROA VALIDATION MODE
                  </Typography>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    {policy.roa_strict ? "Strict (Rejects UNKNOWN)" : "Standard (Permits VALID & UNKNOWN, rejects INVALID)"}
                  </Typography>
                </Box>
              )}
              {policy.snat?.enabled && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    OUTBOUND SNAT
                  </Typography>
                  <Typography variant="body2">
                    Enabled (Condition: {policy.snat.condition || "not_dst"}, Target: {policy.snat.target || "masquerade"})
                  </Typography>
                </Box>
              )}
              {policy.forward_snat && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    FORWARD SNAT
                  </Typography>
                  <Typography variant="body2">
                    Enabled (Target: {policy.forward_snat_target || "masquerade"})
                  </Typography>
                </Box>
              )}
              {(policy.dscp_ingress !== undefined || policy.dscp_egress !== undefined) && (
                <Box>
                  <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                    DSCP MARKING
                  </Typography>
                  <Typography variant="body2">
                    Ingress: {policy.dscp_ingress ?? "None"} • Egress: {policy.dscp_egress ?? "None"}
                  </Typography>
                </Box>
              )}
            </Box>
          </Box>
        )}

        {/* Section 6: Custom BIRD Filters & Snippets */}
        {(policy.bird_import_filter || exportFilter || policy.bird_pre_snippet || policy.bird_post_snippet) && (
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              gap: 1.5,
              p: 2,
              borderRadius: 2,
              bgcolor: "#FFFFFF",
              border: "1px solid #E2E8F0",
            }}
          >
            <Typography
              variant="subtitle2"
              sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}
            >
              <FileCode size={16} color="#4F46E5" /> Custom BIRD Route Filters &amp; Snippets
            </Typography>

            {policy.bird_pre_snippet && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                  BIRD PRE SNIPPET (EARLY POSITION)
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    p: 1.5,
                    m: 0,
                    mt: 0.5,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    borderRadius: 1.5,
                    fontFamily: "monospace",
                    fontSize: "0.8rem",
                    overflowX: "auto",
                  }}
                >
                  {policy.bird_pre_snippet}
                </Box>
              </Box>
            )}

            {policy.bird_import_filter && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                  IMPORT FILTER
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    p: 1.5,
                    m: 0,
                    mt: 0.5,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    borderRadius: 1.5,
                    fontFamily: "monospace",
                    fontSize: "0.8rem",
                    overflowX: "auto",
                  }}
                >
                  {policy.bird_import_filter}
                </Box>
              </Box>
            )}

            {exportFilter && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                  EXPORT FILTER
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    p: 1.5,
                    m: 0,
                    mt: 0.5,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    borderRadius: 1.5,
                    fontFamily: "monospace",
                    fontSize: "0.8rem",
                    overflowX: "auto",
                  }}
                >
                  {exportFilter}
                </Box>
              </Box>
            )}

            {policy.bird_post_snippet && (
              <Box>
                <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                  BIRD POST SNIPPET (END POSITION)
                </Typography>
                <Box
                  component="pre"
                  sx={{
                    p: 1.5,
                    m: 0,
                    mt: 0.5,
                    bgcolor: "#F8FAFC",
                    border: "1px solid #E2E8F0",
                    borderRadius: 1.5,
                    fontFamily: "monospace",
                    fontSize: "0.8rem",
                    overflowX: "auto",
                  }}
                >
                  {policy.bird_post_snippet}
                </Box>
              </Box>
            )}
          </Box>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
        <Button onClick={onClose} variant="outlined" sx={{ color: "#64748B", borderColor: "#CBD5E1" }}>
          Close
        </Button>
        {onEdit && !isBuiltin && (
          <Button
            variant="contained"
            onClick={() => {
              onClose();
              onEdit(policy);
            }}
            sx={{ bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
          >
            Edit Policy
          </Button>
        )}
      </DialogActions>
    </Dialog>
  );
};
