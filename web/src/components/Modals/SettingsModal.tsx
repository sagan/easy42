import React, { useState } from "react";
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
  IconButton,
  Tabs,
  Tab,
  InputAdornment,
  Chip,
  Switch,
  FormControlLabel,
  MenuItem,
} from "@mui/material";
import {
  Settings as SettingsIcon,
  KeyRound,
  Eye,
  EyeOff,
  LogOut,
  ShieldAlert,
  CheckCircle2,
  X,
  Globe,
  Shield,
  Plus,
  Edit2,
  Trash2,
  Sliders,
  Lock,
  Copy,
} from "lucide-react";
import { api } from "../../api/client";
import { NetworkSettings, NetworkPolicy } from "../../types/api";

export const parsePrefixList = (input: string): string[] => {
  const result: string[] = [];
  let current = "";
  let inBraces = false;

  for (let i = 0; i < input.length; i++) {
    const char = input[i];
    if (char === "{") {
      inBraces = true;
      current += char;
    } else if (char === "}") {
      inBraces = false;
      current += char;
    } else if (inBraces && (char === " " || char === "\t")) {
      continue;
    } else if ((char === "," || char === "\n" || char === "\r") && !inBraces) {
      const trimmed = current.trim();
      if (trimmed) {
        result.push(trimmed);
      }
      current = "";
    } else {
      current += char;
    }
  }
  const trimmed = current.trim();
  if (trimmed) {
    result.push(trimmed);
  }
  return result;
};

interface SettingsModalProps {
  open: boolean;
  onClose: () => void;
  onLogoutAll: () => void;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({ open, onClose, onLogoutAll }) => {
  const [activeTab, setActiveTab] = useState<"password" | "sessions" | "network" | "policies">("password");

  // Password change state
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showCurrentPassword, setShowCurrentPassword] = useState(false);
  const [showNewPassword, setShowNewPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);
  const [passwordSuccess, setPasswordSuccess] = useState<string | null>(null);

  // Network / Peering state
  const [publicAsn, setPublicAsn] = useState<number | "">("");
  const [networkLoading, setNetworkLoading] = useState(false);
  const [networkSaving, setNetworkSaving] = useState(false);
  const [networkError, setNetworkError] = useState<string | null>(null);
  const [networkSuccess, setNetworkSuccess] = useState<string | null>(null);

  // Network Policies state
  const [policies, setPolicies] = useState<NetworkPolicy[]>([]);
  const [policiesLoading, setPoliciesLoading] = useState(false);
  const [policiesError, setPoliciesError] = useState<string | null>(null);
  const [policiesSuccess, setPoliciesSuccess] = useState<string | null>(null);

  // Policy editor / viewer dialog state
  const [policyDialogOpen, setPolicyDialogOpen] = useState(false);
  const [policyDialogMode, setPolicyDialogMode] = useState<"view" | "edit" | "create">("view");
  const [policyDialogError, setPolicyDialogError] = useState<string | null>(null);
  const [policyDialogSaving, setPolicyDialogSaving] = useState(false);

  const [policyId, setPolicyId] = useState("");
  const [policyName, setPolicyName] = useState("");
  const [policyDesc, setPolicyDesc] = useState("");
  const [policyAllowedDst, setPolicyAllowedDst] = useState("");
  const [policyAllowedSrc, setPolicyAllowedSrc] = useState("");
  const [policyAllowedImport, setPolicyAllowedImport] = useState("");
  const [policyRejectInternet, setPolicyRejectInternet] = useState(true);
  const [policyFilterForward, setPolicyFilterForward] = useState(true);
  const [policyFilterInput, setPolicyFilterInput] = useState(false);
  const [policySnatEnabled, setPolicySnatEnabled] = useState(false);
  const [policySnatCondition, setPolicySnatCondition] = useState<string>("not_dst");
  const [policySnatTarget, setPolicySnatTarget] = useState<string>("masquerade");

  // Logout all state
  const [logoutAllConfirming, setLogoutAllConfirming] = useState(false);
  const [logoutAllLoading, setLogoutAllLoading] = useState(false);
  const [logoutAllError, setLogoutAllError] = useState<string | null>(null);

  // Load network settings when network tab is opened
  React.useEffect(() => {
    if (open && activeTab === "network") {
      loadNetworkSettings();
    }
  }, [open, activeTab]);

  // Load policies when policies tab is opened
  React.useEffect(() => {
    if (open && activeTab === "policies") {
      loadPolicies();
    }
  }, [open, activeTab]);

  const loadPolicies = async () => {
    setPoliciesLoading(true);
    setPoliciesError(null);
    try {
      const list = await api.getNetworkPolicies();
      setPolicies(list);
    } catch (err: unknown) {
      const e = err as Error;
      setPoliciesError(e.message || "Failed to load network policies.");
    } finally {
      setPoliciesLoading(false);
    }
  };

  const loadNetworkSettings = async () => {
    setNetworkLoading(true);
    setNetworkError(null);
    try {
      const settings = await api.getNetworkSettings();
      setPublicAsn(settings.public_asn || "");
    } catch (err: unknown) {
      const e = err as Error;
      setNetworkError(e.message || "Failed to load network settings.");
    } finally {
      setNetworkLoading(false);
    }
  };

  const handleSaveNetworkSettings = async (e: React.FormEvent) => {
    e.preventDefault();
    setNetworkSaving(true);
    setNetworkError(null);
    setNetworkSuccess(null);

    const payload: NetworkSettings = {
      public_asn: publicAsn === "" ? 0 : Number(publicAsn),
    };

    try {
      await api.updateNetworkSettings(payload);
      setNetworkSuccess("BGP confederation settings updated successfully.");
    } catch (err: unknown) {
      const e = err as Error;
      setNetworkError(e.message || "Failed to update network settings.");
    } finally {
      setNetworkSaving(false);
    }
  };

  const handleOpenCreatePolicy = () => {
    setPolicyDialogMode("create");
    setPolicyId("");
    setPolicyName("");
    setPolicyDesc("");
    setPolicyAllowedDst("");
    setPolicyAllowedSrc("");
    setPolicyAllowedImport("");
    setPolicyRejectInternet(true);
    setPolicyFilterForward(true);
    setPolicyFilterInput(false);
    setPolicySnatEnabled(false);
    setPolicySnatCondition("not_dst");
    setPolicySnatTarget("masquerade");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleOpenViewPolicy = (p: NetworkPolicy) => {
    setPolicyDialogMode("view");
    setPolicyId(p.id);
    setPolicyName(p.name);
    setPolicyDesc(p.description || "");
    setPolicyAllowedDst((p.allowed_dst_cidrs || []).join("\n"));
    setPolicyAllowedSrc((p.allowed_src_cidrs || []).join("\n"));
    setPolicyAllowedImport((p.allowed_import_cidrs || []).join("\n"));
    setPolicyRejectInternet(Boolean(p.reject_internet));
    setPolicyFilterForward(Boolean(p.filter_forward));
    setPolicyFilterInput(Boolean(p.filter_input));
    setPolicySnatEnabled(Boolean(p.snat?.enabled));
    setPolicySnatCondition(p.snat?.condition || "not_dst");
    setPolicySnatTarget(p.snat?.target || "masquerade");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleOpenEditPolicy = (p: NetworkPolicy) => {
    setPolicyDialogMode("edit");
    setPolicyId(p.id);
    setPolicyName(p.name);
    setPolicyDesc(p.description || "");
    setPolicyAllowedDst((p.allowed_dst_cidrs || []).join("\n"));
    setPolicyAllowedSrc((p.allowed_src_cidrs || []).join("\n"));
    setPolicyAllowedImport((p.allowed_import_cidrs || []).join("\n"));
    setPolicyRejectInternet(Boolean(p.reject_internet));
    setPolicyFilterForward(Boolean(p.filter_forward));
    setPolicyFilterInput(Boolean(p.filter_input));
    setPolicySnatEnabled(Boolean(p.snat?.enabled));
    setPolicySnatCondition(p.snat?.condition || "not_dst");
    setPolicySnatTarget(p.snat?.target || "masquerade");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleDeletePolicy = async (p: NetworkPolicy) => {
    if (
      !window.confirm(
        `Are you sure you want to delete custom network policy "${p.name}" (${p.id})? Links assigned to this policy will revert to the default policy.`,
      )
    ) {
      return;
    }
    setPoliciesError(null);
    try {
      await api.deleteNetworkPolicy(p.id);
      setPoliciesSuccess(`Policy "${p.name}" deleted successfully.`);
      await loadPolicies();
    } catch (err: unknown) {
      const e = err as Error;
      setPoliciesError(e.message || "Failed to delete network policy.");
    }
  };

  const handleSavePolicy = async (e: React.FormEvent) => {
    e.preventDefault();
    if (policyDialogMode === "view") {
      setPolicyDialogOpen(false);
      return;
    }
    const cleanId = policyId.trim().toLowerCase();
    if (!cleanId) {
      setPolicyDialogError("Policy ID is required.");
      return;
    }
    if (!/^[a-z0-9_-]+$/.test(cleanId)) {
      setPolicyDialogError("Policy ID must contain only lowercase letters, numbers, hyphens, and underscores.");
      return;
    }
    if (!policyName.trim()) {
      setPolicyDialogError("Policy Name is required.");
      return;
    }

    setPolicyDialogSaving(true);
    setPolicyDialogError(null);
    try {
      const policyPayload: Partial<NetworkPolicy> = {
        id: cleanId,
        name: policyName.trim(),
        description: policyDesc.trim() || undefined,
        allowed_dst_cidrs: parsePrefixList(policyAllowedDst),
        allowed_src_cidrs: parsePrefixList(policyAllowedSrc),
        allowed_import_cidrs: parsePrefixList(policyAllowedImport),
        reject_internet: policyRejectInternet,
        filter_forward: policyFilterForward,
        filter_input: policyFilterInput,
        snat: policySnatEnabled
          ? {
              enabled: true,
              condition: policySnatCondition,
              target: policySnatTarget.trim() || undefined,
            }
          : undefined,
      };

      if (policyDialogMode === "create") {
        await api.createNetworkPolicy(policyPayload);
        setPoliciesSuccess(`Policy "${policyName.trim()}" created successfully.`);
      } else {
        await api.updateNetworkPolicy(cleanId, policyPayload);
        setPoliciesSuccess(`Policy "${policyName.trim()}" updated successfully.`);
      }
      setPolicyDialogOpen(false);
      await loadPolicies();
    } catch (err: unknown) {
      const e = err as Error;
      setPolicyDialogError(e.message || "Failed to save network policy.");
    } finally {
      setPolicyDialogSaving(false);
    }
  };

  const resetForm = () => {
    setCurrentPassword("");
    setNewPassword("");
    setConfirmPassword("");
    setPasswordError(null);
    setPasswordSuccess(null);
    setLogoutAllConfirming(false);
    setLogoutAllError(null);
    setNetworkError(null);
    setNetworkSuccess(null);
    setPoliciesError(null);
    setPoliciesSuccess(null);
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };
  const handleOpenClonePolicy = (p: NetworkPolicy) => {
    setPolicyDialogMode("create");
    setPolicyId(`${p.id}-copy`);
    setPolicyName(`${p.name} (Copy)`);
    setPolicyDesc(p.description || "");
    setPolicyAllowedDst((p.allowed_dst_cidrs || []).join("\n"));
    setPolicyAllowedSrc((p.allowed_src_cidrs || []).join("\n"));
    setPolicyAllowedImport((p.allowed_import_cidrs || []).join("\n"));
    setPolicyRejectInternet(Boolean(p.reject_internet));
    setPolicyFilterForward(Boolean(p.filter_forward));
    setPolicyFilterInput(Boolean(p.filter_input));
    setPolicySnatEnabled(Boolean(p.snat?.enabled));
    setPolicySnatCondition(p.snat?.condition || "not_dst");
    setPolicySnatTarget(p.snat?.target || "masquerade");
    setPolicyDialogError(null);
    setPolicyDialogOpen(true);
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    setPasswordError(null);
    setPasswordSuccess(null);

    if (!currentPassword) {
      setPasswordError("Please enter your current password.");
      return;
    }

    if (newPassword.length < 6) {
      setPasswordError("New password must be at least 6 characters long.");
      return;
    }

    if (newPassword !== confirmPassword) {
      setPasswordError("New passwords do not match.");
      return;
    }

    if (currentPassword === newPassword) {
      setPasswordError("New password must be different from current password.");
      return;
    }

    setPasswordLoading(true);
    try {
      const res = await api.changePassword(currentPassword, newPassword);
      setPasswordSuccess(res.message || "Password changed successfully.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err: unknown) {
      const e = err as Error;
      setPasswordError(e.message || "Failed to change password.");
    } finally {
      setPasswordLoading(false);
    }
  };

  const handleLogoutAll = async () => {
    setLogoutAllLoading(true);
    setLogoutAllError(null);

    try {
      await api.logoutAll();
      resetForm();
      onClose();
      onLogoutAll();
    } catch (err: unknown) {
      const e = err as Error;
      setLogoutAllError(e.message || "Failed to log out all sessions.");
      setLogoutAllLoading(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth={activeTab === "policies" ? "md" : "sm"} fullWidth>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          pb: 1,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              width: 36,
              height: 36,
              borderRadius: 2,
              backgroundColor: "rgba(79, 70, 229, 0.1)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#4F46E5",
            }}
          >
            <SettingsIcon size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
              Settings
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B" }}>
              easy42 configuration & security
            </Typography>
          </Box>
        </Box>

        <IconButton size="small" onClick={handleClose} sx={{ color: "#94A3B8", "&:hover": { color: "#0F172A" } }}>
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <Box sx={{ borderBottom: "1px solid #E2E8F0", px: 3, pt: 1, backgroundColor: "#F8FAFC" }}>
        <Tabs
          value={activeTab}
          onChange={(_, val) => {
            setActiveTab(val);
            setPasswordError(null);
            setPasswordSuccess(null);
            setLogoutAllError(null);
          }}
          sx={{
            minHeight: 44,
            "& .MuiTab-root": {
              minHeight: 44,
              textTransform: "none",
              fontWeight: 600,
              fontSize: "0.875rem",
              gap: 1,
            },
          }}
        >
          <Tab value="password" icon={<KeyRound size={16} />} iconPosition="start" label="Change Password" />
          <Tab value="sessions" icon={<ShieldAlert size={16} />} iconPosition="start" label="Sessions" />
          <Tab value="network" icon={<Globe size={16} />} iconPosition="start" label="Peering & BGP" />
          <Tab value="policies" icon={<Shield size={16} />} iconPosition="start" label="Network Policies" />
        </Tabs>
      </Box>

      {/* Tab 1: Change Password Form */}
      {activeTab === "password" && (
        <form onSubmit={handleChangePassword}>
          <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 3, px: 3 }}>
            <Typography variant="body2" sx={{ color: "#64748B" }}>
              Update your easy42 admin password. Changing the password will re-encrypt your master data encryption key
              (DEK) and invalidate other active sessions.
            </Typography>

            {passwordSuccess && (
              <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                {passwordSuccess}
              </Alert>
            )}

            {passwordError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {passwordError}
              </Alert>
            )}

            <TextField
              fullWidth
              size="small"
              label="Current Password"
              type={showCurrentPassword ? "text" : "password"}
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              required
              disabled={passwordLoading}
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        edge="end"
                        onClick={() => setShowCurrentPassword(!showCurrentPassword)}
                        aria-label="toggle current password visibility"
                      >
                        {showCurrentPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            />

            <TextField
              fullWidth
              size="small"
              label="New Password"
              type={showNewPassword ? "text" : "password"}
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              required
              disabled={passwordLoading}
              helperText="Minimum 6 characters"
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        edge="end"
                        onClick={() => setShowNewPassword(!showNewPassword)}
                        aria-label="toggle new password visibility"
                      >
                        {showNewPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            />

            <TextField
              fullWidth
              size="small"
              label="Confirm New Password"
              type={showConfirmPassword ? "text" : "password"}
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              required
              disabled={passwordLoading}
              error={Boolean(confirmPassword && newPassword !== confirmPassword)}
              helperText={confirmPassword && newPassword !== confirmPassword ? "Passwords do not match" : ""}
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        size="small"
                        edge="end"
                        onClick={() => setShowConfirmPassword(!showConfirmPassword)}
                        aria-label="toggle confirm password visibility"
                      >
                        {showConfirmPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            />
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={handleClose} disabled={passwordLoading} sx={{ color: "#64748B" }}>
              Cancel
            </Button>
            <Button
              type="submit"
              variant="contained"
              disabled={passwordLoading || !currentPassword || !newPassword || !confirmPassword}
              startIcon={passwordLoading ? <CircularProgress size={16} color="inherit" /> : <KeyRound size={16} />}
              sx={{
                background: "linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)",
                fontWeight: 700,
                color: "#FFFFFF",
              }}
            >
              {passwordLoading ? "Updating Password..." : "Change Password"}
            </Button>
          </DialogActions>
        </form>
      )}

      {/* Tab 2: Sessions / Logout All */}
      {activeTab === "sessions" && (
        <Box>
          <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 3, px: 3 }}>
            {logoutAllError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {logoutAllError}
              </Alert>
            )}

            <Box
              sx={{
                p: 2.5,
                borderRadius: 2,
                border: "1px solid #FEE2E2",
                backgroundColor: "#FEF2F2",
                display: "flex",
                flexDirection: "column",
                gap: 1.5,
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, color: "#B91C1C" }}>
                <ShieldAlert size={22} />
                <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>
                  Reset Session Secret & Log Out All Sessions
                </Typography>
              </Box>

              <Typography variant="body2" sx={{ color: "#7F1D1D", lineHeight: 1.6 }}>
                Clicking <strong>Logout all</strong> will regenerate the <code>session_secret</code> in{" "}
                <code>config.json</code> and immediately lock the in-memory vault.
              </Typography>

              <Typography variant="body2" sx={{ color: "#991B1B", lineHeight: 1.6 }}>
                This invalidates every active session across all devices and browsers, including your current one. You
                and any other users will be required to log in again.
              </Typography>
            </Box>

            {!logoutAllConfirming ? (
              <Box sx={{ display: "flex", justifyContent: "flex-start", pt: 1 }}>
                <Button
                  variant="outlined"
                  color="error"
                  startIcon={<LogOut size={16} />}
                  onClick={() => setLogoutAllConfirming(true)}
                  sx={{
                    borderColor: "#F87171",
                    color: "#DC2626",
                    fontWeight: 700,
                    "&:hover": {
                      borderColor: "#DC2626",
                      backgroundColor: "rgba(220, 38, 38, 0.06)",
                    },
                  }}
                >
                  Logout all
                </Button>
              </Box>
            ) : (
              <Box
                sx={{
                  p: 2,
                  borderRadius: 2,
                  border: "1px solid #CBD5E1",
                  backgroundColor: "#F8FAFC",
                  display: "flex",
                  flexDirection: "column",
                  gap: 1.5,
                }}
              >
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                  Are you sure you want to log out all sessions?
                </Typography>
                <Typography variant="body2" sx={{ color: "#64748B" }}>
                  All connected browsers will immediately lose authentication and return to the login screen.
                </Typography>
                <Box sx={{ display: "flex", gap: 1.5, pt: 0.5 }}>
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => setLogoutAllConfirming(false)}
                    disabled={logoutAllLoading}
                    sx={{ color: "#64748B", borderColor: "#CBD5E1" }}
                  >
                    Cancel
                  </Button>
                  <Button
                    size="small"
                    variant="contained"
                    color="error"
                    disabled={logoutAllLoading}
                    onClick={handleLogoutAll}
                    startIcon={logoutAllLoading ? <CircularProgress size={14} color="inherit" /> : <LogOut size={14} />}
                    sx={{ fontWeight: 700 }}
                  >
                    {logoutAllLoading ? "Logging out all..." : "Confirm: Log Out All"}
                  </Button>
                </Box>
              </Box>
            )}
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={handleClose} sx={{ color: "#64748B" }}>
              Close
            </Button>
          </DialogActions>
        </Box>
      )}

      {/* Tab 3: Peering & BGP Network Settings */}
      {activeTab === "network" && (
        <form onSubmit={handleSaveNetworkSettings}>
          <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 3, px: 3 }}>
            <Typography variant="body2" sx={{ color: "#64748B" }}>
              Configure global BGP confederation parameters for external peering (such as DN42 or private networks). BGP
              confederation replaces your internal mesh ASNs with your public ASN in external BGP sessions. Subnet filtering and firewall policies are configured per-link under Network Policies.
            </Typography>

            {networkLoading ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : (
              <>
                {networkSuccess && (
                  <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                    {networkSuccess}
                  </Alert>
                )}

                {networkError && (
                  <Alert severity="error" sx={{ borderRadius: 2 }}>
                    {networkError}
                  </Alert>
                )}

                <TextField
                  fullWidth
                  size="small"
                  label="Public / DN42 ASN"
                  type="number"
                  placeholder="e.g. 4242420000"
                  value={publicAsn}
                  onChange={(e) => setPublicAsn(e.target.value === "" ? "" : Number(e.target.value))}
                  helperText="Your network's public ASN (e.g. DN42 ASN). When set, BIRD confederation exposes this ASN to external peers."
                  disabled={networkSaving}
                />
              </>
            )}
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={handleClose} sx={{ color: "#64748B" }}>
              Close
            </Button>
            <Button
              type="submit"
              variant="contained"
              disabled={networkLoading || networkSaving}
              startIcon={networkSaving && <CircularProgress size={16} color="inherit" />}
              sx={{ fontWeight: 600 }}
            >
              {networkSaving ? "Saving..." : "Save Settings"}
            </Button>
          </DialogActions>
        </form>
      )}

      {/* Tab 4: Network Policies */}
      {activeTab === "policies" && (
        <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
          <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, py: 3, px: 3 }}>
            <Box sx={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 2 }}>
              <Box>
                <Typography variant="body2" sx={{ color: "#475569", lineHeight: 1.5 }}>
                  Define link-level firewall and routing policies. Network policies restrict allowed destination & source IPs (dropping unauthorized packets in nftables while preserving established return flows), filter BGP route imports, and configure outbound SNAT / masquerade.
                </Typography>
              </Box>
              <Button
                variant="contained"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={handleOpenCreatePolicy}
                sx={{
                  whiteSpace: "nowrap",
                  fontWeight: 600,
                  bgcolor: "#4F46E5",
                  "&:hover": { bgcolor: "#4338CA" },
                }}
              >
                Create Policy
              </Button>
            </Box>

            {policiesSuccess && (
              <Alert icon={<CheckCircle2 size={18} />} severity="success" sx={{ borderRadius: 2 }}>
                {policiesSuccess}
              </Alert>
            )}

            {policiesError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {policiesError}
              </Alert>
            )}

            {policiesLoading ? (
              <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
                <CircularProgress size={24} />
              </Box>
            ) : (
              <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1 }}>
                {policies.map((p) => {
                  const isBuiltin = Boolean(p.is_internal);
                  return (
                    <Box
                      key={p.id}
                      sx={{
                        p: 2,
                        borderRadius: 2,
                        border: "1px solid",
                        borderColor: isBuiltin ? "#E0E7FF" : "#E2E8F0",
                        bgcolor: isBuiltin ? "#F8FAFC" : "#FFFFFF",
                        display: "flex",
                        flexDirection: "column",
                        gap: 1.2,
                        transition: "all 0.15s ease",
                        "&:hover": {
                          borderColor: isBuiltin ? "#C7D2FE" : "#CBD5E1",
                          boxShadow: "0 2px 4px rgba(0,0,0,0.03)",
                        },
                      }}
                    >
                      <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                          <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                            {p.name}
                          </Typography>
                          <Typography
                            variant="caption"
                            className="mono-font"
                            sx={{ color: "#64748B", bgcolor: "#F1F5F9", px: 0.8, py: 0.2, borderRadius: 1 }}
                          >
                            {p.id}
                          </Typography>
                          <Chip
                            label={isBuiltin ? "Built-in" : "Custom"}
                            size="small"
                            sx={{
                              height: 20,
                              fontSize: "0.68rem",
                              fontWeight: 700,
                              bgcolor: isBuiltin ? "rgba(79, 70, 229, 0.1)" : "rgba(16, 185, 129, 0.12)",
                              color: isBuiltin ? "#4F46E5" : "#059669",
                            }}
                          />
                        </Box>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                          <Button
                            size="small"
                            variant="outlined"
                            onClick={() => handleOpenViewPolicy(p)}
                            sx={{ py: 0.3, px: 1, minWidth: 0, fontSize: "0.75rem", color: "#475569", borderColor: "#CBD5E1" }}
                          >
                            View
                          </Button>
                          <Button
                            size="small"
                            variant="outlined"
                            startIcon={<Copy size={12} />}
                            onClick={() => handleOpenClonePolicy(p)}
                            sx={{ py: 0.3, px: 1, minWidth: 0, fontSize: "0.75rem", color: "#4F46E5", borderColor: "#C7D2FE" }}
                          >
                            Clone
                          </Button>
                          {!isBuiltin && (
                            <>
                              <IconButton
                                size="small"
                                onClick={() => handleOpenEditPolicy(p)}
                                sx={{ color: "#4F46E5", p: 0.5 }}
                                title="Edit Policy"
                              >
                                <Edit2 size={15} />
                              </IconButton>
                              <IconButton
                                size="small"
                                onClick={() => handleDeletePolicy(p)}
                                sx={{ color: "#EF4444", p: 0.5 }}
                                title="Delete Policy"
                              >
                                <Trash2 size={15} />
                              </IconButton>
                            </>
                          )}
                        </Box>
                      </Box>

                      {p.description && (
                        <Typography variant="caption" sx={{ color: "#64748B" }}>
                          {p.description}
                        </Typography>
                      )}

                      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, pt: 0.5 }}>
                        <Chip
                          size="small"
                          label={`Allowed Dst: ${p.allowed_dst_cidrs && p.allowed_dst_cidrs.length > 0 ? `${p.allowed_dst_cidrs.length} prefix(es)` : "All"}`}
                          variant="outlined"
                          sx={{ fontSize: "0.7rem", height: 22 }}
                        />
                        <Chip
                          size="small"
                          label={`Allowed Src: ${p.allowed_src_cidrs && p.allowed_src_cidrs.length > 0 ? `${p.allowed_src_cidrs.length} prefix(es)` : "All"}`}
                          variant="outlined"
                          sx={{ fontSize: "0.7rem", height: 22 }}
                        />
                        <Chip
                          size="small"
                          label={`BGP Import: ${p.allowed_import_cidrs && p.allowed_import_cidrs.length > 0 ? `${p.allowed_import_cidrs.length} prefix(es)` : "All"}`}
                          variant="outlined"
                          sx={{ fontSize: "0.7rem", height: 22 }}
                        />
                        <Chip
                          size="small"
                          label={`Leak Protect: ${p.reject_internet ? "Internet Rejection" : "None"}`}
                          variant="outlined"
                          sx={{ fontSize: "0.7rem", height: 22 }}
                        />
                        <Chip
                          size="small"
                          label={`SNAT: ${p.snat?.enabled ? (p.snat.condition === "not_dst" ? "SNAT (src != dst)" : "SNAT (all)") : "Disabled"}`}
                          variant="outlined"
                          sx={{
                            fontSize: "0.7rem",
                            height: 22,
                            borderColor: p.snat?.enabled ? "#A5F3FC" : undefined,
                            bgcolor: p.snat?.enabled ? "#F0FDFA" : undefined,
                            color: p.snat?.enabled ? "#0F766E" : undefined,
                          }}
                        />
                      </Box>
                    </Box>
                  );
                })}
              </Box>
            )}
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={handleClose} sx={{ color: "#64748B" }}>
              Close
            </Button>
          </DialogActions>
        </Box>
      )}

      {/* Sub-dialog: Policy Editor & Viewer */}
      <Dialog
        open={policyDialogOpen}
        onClose={() => setPolicyDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <form onSubmit={handleSavePolicy}>
          <DialogTitle
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              pb: 1,
              borderBottom: "1px solid #E2E8F0",
            }}
          >
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Shield size={20} color="#4F46E5" />
              <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1.1rem", color: "#0F172A" }}>
                {policyDialogMode === "view"
                  ? "View Network Policy"
                  : policyDialogMode === "edit"
                  ? "Edit Network Policy"
                  : "Create Network Policy"}
              </Typography>
            </Box>
            <IconButton size="small" onClick={() => setPolicyDialogOpen(false)} sx={{ color: "#94A3B8" }}>
              <X size={18} />
            </IconButton>
          </DialogTitle>

          <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, py: 2.5, px: 3 }}>
            {policyDialogMode === "view" && (
              <Alert icon={<Lock size={16} />} severity="info" sx={{ py: 0.5, borderRadius: 2 }}>
                Built-in policies are managed by easy42 and are read-only.
              </Alert>
            )}

            {policyDialogError && (
              <Alert severity="error" sx={{ borderRadius: 2 }}>
                {policyDialogError}
              </Alert>
            )}

            <Box sx={{ display: "flex", gap: 2 }}>
              <TextField
                fullWidth
                size="small"
                label="Policy ID"
                placeholder="e.g. partner-lan"
                value={policyId}
                onChange={(e) => setPolicyId(e.target.value.toLowerCase().replace(/[^a-z0-9_-]/g, ""))}
                disabled={policyDialogMode !== "create" || policyDialogSaving}
                helperText="Unique identifier (letters, numbers, hyphens)"
                required
              />
              <TextField
                fullWidth
                size="small"
                label="Policy Name"
                placeholder="e.g. Partner LAN Policy"
                value={policyName}
                onChange={(e) => setPolicyName(e.target.value)}
                disabled={policyDialogMode === "view" || policyDialogSaving}
                required
              />
            </Box>

            <TextField
              fullWidth
              size="small"
              label="Description (optional)"
              placeholder="e.g. Limits access to specific subnets and enables anti-spoofing"
              value={policyDesc}
              onChange={(e) => setPolicyDesc(e.target.value)}
              disabled={policyDialogMode === "view" || policyDialogSaving}
            />

            <TextField
              fullWidth
              size="small"
              label="Allowed Destination Subnets (CIDRs)"
              placeholder={"e.g. 172.20.0.0/14{21,29}\nfd00::/8{44,64}"}
              multiline
              rows={3}
              value={policyAllowedDst}
              onChange={(e) => setPolicyAllowedDst(e.target.value)}
              disabled={policyDialogMode === "view" || policyDialogSaving}
              helperText="Restricts traffic dst IP via nftables (return traffic allowed) & filters BGP export routes. Leave empty for unrestricted."
            />

            <TextField
              fullWidth
              size="small"
              label="Allowed Source Subnets (CIDRs)"
              placeholder={"e.g. 172.20.10.0/24\nfd00:1::/48"}
              multiline
              rows={3}
              value={policyAllowedSrc}
              onChange={(e) => setPolicyAllowedSrc(e.target.value)}
              disabled={policyDialogMode === "view" || policyDialogSaving}
              helperText="Anti-spoofing: inbound traffic on this link with source IP not matching will be dropped. Leave empty for unrestricted."
            />

            <TextField
              fullWidth
              size="small"
              label="Allowed BGP Import Subnets (CIDRs)"
              placeholder={"e.g. 172.20.0.0/16+\nfd00::/48+"}
              multiline
              rows={3}
              value={policyAllowedImport}
              onChange={(e) => setPolicyAllowedImport(e.target.value)}
              disabled={policyDialogMode === "view" || policyDialogSaving}
              helperText="BGP route import filter: only permits routes matching these subnets. Leave empty for unrestricted."
            />

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1, p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B", display: "flex", alignItems: "center", gap: 0.8 }}>
                <Sliders size={16} /> Route Leak Protection
              </Typography>
              <FormControlLabel
                control={
                  <Switch
                    checked={policyRejectInternet}
                    onChange={(e) => setPolicyRejectInternet(e.target.checked)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                  />
                }
                label={
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                      Reject Internet Routes
                    </Typography>
                    <Typography variant="caption" sx={{ color: "#64748B" }}>
                      Prevents importing and exporting default / Internet routes (0.0.0.0/0, 128.0.0.0/1, ::/0).
                    </Typography>
                  </Box>
                }
              />
            </Box>

            <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, p: 2, borderRadius: 2, bgcolor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
              <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#1E293B" }}>
                  Outbound SNAT / Masquerade
                </Typography>
                <FormControlLabel
                  control={
                    <Switch
                      checked={policySnatEnabled}
                      onChange={(e) => setPolicySnatEnabled(e.target.checked)}
                      disabled={policyDialogMode === "view" || policyDialogSaving}
                    />
                  }
                  label={policySnatEnabled ? "Enabled" : "Disabled"}
                  sx={{ m: 0 }}
                />
              </Box>

              {policySnatEnabled && (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, pt: 1 }}>
                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="SNAT Condition"
                    value={policySnatCondition}
                    onChange={(e) => setPolicySnatCondition(e.target.value)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                  >
                    <MenuItem value="not_dst">Traffic whose source IP is NOT in allowed destination CIDRs</MenuItem>
                    <MenuItem value="all">All egress traffic on this link</MenuItem>
                  </TextField>

                  <TextField
                    select
                    fullWidth
                    size="small"
                    label="SNAT Target IP"
                    value={policySnatTarget}
                    onChange={(e) => setPolicySnatTarget(e.target.value)}
                    disabled={policyDialogMode === "view" || policyDialogSaving}
                  >
                    <MenuItem value="masquerade">Interface IP (Masquerade)</MenuItem>
                    <MenuItem value="external_ip">External / Peering IP</MenuItem>
                  </TextField>
                </Box>
              )}
            </Box>
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={() => setPolicyDialogOpen(false)} sx={{ color: "#64748B" }}>
              {policyDialogMode === "view" ? "Close" : "Cancel"}
            </Button>
            {policyDialogMode !== "view" && (
              <Button
                type="submit"
                variant="contained"
                disabled={policyDialogSaving}
                startIcon={policyDialogSaving && <CircularProgress size={16} color="inherit" />}
                sx={{ fontWeight: 600, bgcolor: "#4F46E5", "&:hover": { bgcolor: "#4338CA" } }}
              >
                {policyDialogSaving ? "Saving..." : policyDialogMode === "create" ? "Create Policy" : "Save Changes"}
              </Button>
            )}
          </DialogActions>
        </form>
      </Dialog>
    </Dialog>
  );
};
