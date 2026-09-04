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
} from "lucide-react";
import { api } from "../../api/client";
import { NetworkSettings } from "../../types/api";

interface SettingsModalProps {
  open: boolean;
  onClose: () => void;
  onLogoutAll: () => void;
}

export const SettingsModal: React.FC<SettingsModalProps> = ({ open, onClose, onLogoutAll }) => {
  const [activeTab, setActiveTab] = useState<"password" | "sessions" | "network">("password");

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
  const [confedMembers, setConfedMembers] = useState("");
  const [exportPrefixes, setExportPrefixes] = useState("");
  const [importPrefixes, setImportPrefixes] = useState("");
  const [networkLoading, setNetworkLoading] = useState(false);
  const [networkSaving, setNetworkSaving] = useState(false);
  const [networkError, setNetworkError] = useState<string | null>(null);
  const [networkSuccess, setNetworkSuccess] = useState<string | null>(null);

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

  const loadNetworkSettings = async () => {
    setNetworkLoading(true);
    setNetworkError(null);
    try {
      const settings = await api.getNetworkSettings();
      setPublicAsn(settings.public_asn || "");
      setConfedMembers(settings.confed_members || "");
      setExportPrefixes(settings.export_prefixes?.join(", ") || "");
      setImportPrefixes(settings.import_prefixes?.join(", ") || "");
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

    const exportList = exportPrefixes
      .split(/[\n,]+/)
      .map((s) => s.trim())
      .filter(Boolean);

    const importList = importPrefixes
      .split(/[\n,]+/)
      .map((s) => s.trim())
      .filter(Boolean);

    const payload: NetworkSettings = {
      public_asn: publicAsn === "" ? 0 : Number(publicAsn),
      confed_members: confedMembers.trim(),
      export_prefixes: exportList,
      import_prefixes: importList,
    };

    try {
      await api.updateNetworkSettings(payload);
      setNetworkSuccess("Network and BGP confederation settings updated successfully.");
    } catch (err: unknown) {
      const e = err as Error;
      setNetworkError(e.message || "Failed to update network settings.");
    } finally {
      setNetworkSaving(false);
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
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };
  const handleReset = () => {
    setPublicAsn(4242420001);
    const prefixes = `172.20.0.0/14{21,29}
172.20.0.0/24{28,32}
172.21.0.0/24{28,32}
172.22.0.0/24{28,32}
172.23.0.0/24{28,32}
172.31.0.0/16+
10.100.0.0/14+
10.0.0.0/8{15,24}
10.127.0.0/16+
fd00::/8{44,64}`;
    setConfedMembers("4224420000..4224429999");
    setExportPrefixes(prefixes);
    setImportPrefixes(prefixes);
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
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
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
              Configure global network parameters for external peering (such as DN42 or private networks). BGP
              confederation replaces your internal mesh ASNs with your public ASN in external BGP sessions.
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

                <TextField
                  fullWidth
                  size="small"
                  label="Confederation Member ASNs"
                  placeholder="e.g. 4224420000..4224429999"
                  value={confedMembers}
                  onChange={(e) => setConfedMembers(e.target.value)}
                  helperText="BIRD confederation member specification for internal mesh ASNs (e.g. range 4224420000..4224429999)."
                  disabled={networkSaving}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="Export Prefixes (comma or newline separated)"
                  placeholder="e.g. 172.20.0.0/16+, fd00::/8+"
                  multiline
                  rows={2}
                  value={exportPrefixes}
                  onChange={(e) => setExportPrefixes(e.target.value)}
                  helperText="Subnets exported to external peers. Format with BIRD prefix syntax (e.g. 172.20.0.0/16+, fd00::/8+)."
                  disabled={networkSaving}
                />

                <TextField
                  fullWidth
                  size="small"
                  label="Import Filter Prefixes (comma or newline separated)"
                  placeholder="e.g. 172.20.0.0/14{21,29}, 172.31.0.0/16{21,29}, fd00::/8{44,64}"
                  multiline
                  rows={2}
                  value={importPrefixes}
                  onChange={(e) => setImportPrefixes(e.target.value)}
                  helperText="Valid subnet ranges accepted from external peers. Defaults to DN42 standard ranges if left empty."
                  disabled={networkSaving}
                />
              </>
            )}
          </DialogContent>

          <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
            <Button onClick={handleClose} sx={{ color: "#64748B" }}>
              Close
            </Button>
            <Button onClick={handleReset} color="secondary" variant="outlined">
              Reset to Default (DN42)
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
    </Dialog>
  );
};
