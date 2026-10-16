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
  RadioGroup,
  FormControlLabel,
  Radio,
  IconButton,
  InputAdornment,
} from "@mui/material";
import {
  Shield,
  Copy,
  Check,
  Eye,
  EyeOff,
  RefreshCw,
  Sparkles,
  Lock,
} from "lucide-react";
import { api } from "../../api/client";
import { Link } from "../../types/api";

export type LinkKeyActionMode =
  | "reveal_psk"
  | "reset_psk"
  | "reveal_private_key"
  | "reset_private_key";

interface LinkKeyActionModalProps {
  open: boolean;
  onClose: () => void;
  mode: LinkKeyActionMode;
  link: Link | null;
  nodeName?: string;
  onSuccess?: () => void;
}

export const LinkKeyActionModal: React.FC<LinkKeyActionModalProps> = ({
  open,
  onClose,
  mode,
  link,
  nodeName,
  onSuccess,
}) => {
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [resetMethod, setResetMethod] = useState<"random" | "custom">("random");
  const [customKey, setCustomKey] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revealedValue, setRevealedValue] = useState<string | null>(null);
  const [revealedPublicKey, setRevealedPublicKey] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [copiedPub, setCopiedPub] = useState(false);
  const [isDone, setIsDone] = useState(false);

  useEffect(() => {
    if (open) {
      setPassword("");
      setShowPassword(false);
      setResetMethod("random");
      setCustomKey("");
      setLoading(false);
      setError(null);
      setRevealedValue(null);
      setRevealedPublicKey(null);
      setCopied(false);
      setCopiedPub(false);
      setIsDone(false);
    }
  }, [open, mode, link?.from.name, link?.to.name, nodeName]);

  if (!link) return null;

  const targetNode = nodeName || link.from.name;
  const isPsk = mode === "reveal_psk" || mode === "reset_psk";
  const isReveal = mode === "reveal_psk" || mode === "reveal_private_key";

  const getTitle = () => {
    switch (mode) {
      case "reveal_psk":
        return "Reveal WireGuard PSK (PresharedKey)";
      case "reset_psk":
        return "Reset WireGuard PSK (PresharedKey)";
      case "reveal_private_key":
        return `Reveal WireGuard Private Key (${targetNode})`;
      case "reset_private_key":
        return `Reset WireGuard Private Key (${targetNode})`;
    }
  };

  const getSubtitle = () => {
    switch (mode) {
      case "reveal_psk":
        return `Link between ${link.from.name} and ${link.to.name}. Verification required.`;
      case "reset_psk":
        return `Update or regenerate the symmetric PSK for link ${link.from.name} ↔ ${link.to.name}.`;
      case "reveal_private_key":
        return `Node ${targetNode} local interface private key. Password verification required.`;
      case "reset_private_key":
        return `Update or regenerate the local private key for node ${targetNode}. The matching public key will be derived automatically.`;
    }
  };

  const copyToClipboard = async (text: string, isPub = false) => {
    try {
      await navigator.clipboard.writeText(text);
      if (isPub) {
        setCopiedPub(true);
        setTimeout(() => setCopiedPub(false), 2000);
      } else {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }
    } catch {
      // fallback
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);

    try {
      if (mode === "reveal_psk") {
        if (!password) {
          setError("Please enter your administrator password");
          setLoading(false);
          return;
        }
        const res = await api.revealLinkPsk({
          from: link.from.name,
          to: link.to.name,
          interface: link.from.interface,
          password,
        });
        setRevealedValue(res.psk || "(None configured)");
        setIsDone(true);
      } else if (mode === "reset_psk") {
        let pskToSend = "";
        if (resetMethod === "custom") {
          const trimmed = customKey.trim();
          if (!trimmed) {
            setError("Please enter a custom 32-byte base64 PSK");
            setLoading(false);
            return;
          }
          pskToSend = trimmed;
        }
        const res = await api.resetLinkPsk({
          from: link.from.name,
          to: link.to.name,
          interface: link.from.interface,
          psk: pskToSend,
        });
        setRevealedValue(res.psk);
        setIsDone(true);
        onSuccess?.();
      } else if (mode === "reveal_private_key") {
        if (!password) {
          setError("Please enter your administrator password");
          setLoading(false);
          return;
        }
        const res = await api.revealLinkPrivateKey({
          from: link.from.name,
          to: link.to.name,
          node: targetNode,
          interface: link.from.interface,
          password,
        });
        setRevealedValue(res.private_key);
        setRevealedPublicKey(res.public_key);
        setIsDone(true);
      } else if (mode === "reset_private_key") {
        let privKeyToSend = "";
        if (resetMethod === "custom") {
          const trimmed = customKey.trim();
          if (!trimmed) {
            setError("Please enter a custom 32-byte base64 WireGuard private key");
            setLoading(false);
            return;
          }
          privKeyToSend = trimmed;
        }
        const res = await api.resetLinkPrivateKey({
          from: link.from.name,
          to: link.to.name,
          node: targetNode,
          interface: link.from.interface,
          private_key: privKeyToSend,
        });
        setRevealedValue(res.private_key);
        setRevealedPublicKey(res.public_key);
        setIsDone(true);
        onSuccess?.();
      }
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Operation failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth sx={{ zIndex: 1400 }}>
      <DialogTitle sx={{ display: "flex", alignItems: "center", gap: 1.5, pb: 1.5, borderBottom: "1px solid #E2E8F0" }}>
        <Box
          sx={{
            width: 38,
            height: 38,
            borderRadius: 2,
            backgroundColor: isReveal ? "rgba(99, 102, 241, 0.1)" : "rgba(217, 119, 6, 0.1)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            color: isReveal ? "#4F46E5" : "#D97706",
          }}
        >
          {isReveal ? <Shield size={20} /> : <RefreshCw size={20} />}
        </Box>
        <Box>
          <Typography variant="subtitle1" sx={{ fontWeight: 700, color: "#0F172A", lineHeight: 1.2 }}>
            {getTitle()}
          </Typography>
          <Typography variant="caption" sx={{ color: "#64748B" }}>
            {getSubtitle()}
          </Typography>
        </Box>
      </DialogTitle>

      <form onSubmit={handleSubmit}>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2.5, pt: 2.5 }}>
          {error && (
            <Alert severity="error" sx={{ borderRadius: 2 }}>
              {error}
            </Alert>
          )}

          {/* Results State */}
          {isDone && revealedValue !== null ? (
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <Alert severity="success" sx={{ borderRadius: 2 }}>
                {isReveal
                  ? `${isPsk ? "Pre-shared Key (PSK)" : "WireGuard Private Key"} revealed successfully.`
                  : `${isPsk ? "Pre-shared Key (PSK)" : "WireGuard Private Key"} has been reset successfully. Save and sync the network to apply.`}
              </Alert>

              <Box sx={{ p: 2, borderRadius: 2, backgroundColor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                  <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                    {isPsk ? "PLAIN TEXT PSK" : `PRIVATE KEY (${targetNode})`}
                  </Typography>
                  <Button
                    size="small"
                    startIcon={copied ? <Check size={14} color="#10B981" /> : <Copy size={14} />}
                    onClick={() => copyToClipboard(revealedValue)}
                    sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.2 }}
                  >
                    {copied ? "Copied" : "Copy"}
                  </Button>
                </Box>
                <Typography
                  variant="body2"
                  className="mono-font"
                  sx={{
                    p: 1.2,
                    backgroundColor: "#FFFFFF",
                    borderRadius: 1.5,
                    border: "1px solid #CBD5E1",
                    wordBreak: "break-all",
                    color: "#0F172A",
                    fontWeight: 600,
                  }}
                >
                  {revealedValue}
                </Typography>
              </Box>

              {revealedPublicKey && (
                <Box sx={{ p: 2, borderRadius: 2, backgroundColor: "#F8FAFC", border: "1px solid #E2E8F0" }}>
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 0.5 }}>
                    <Typography variant="caption" sx={{ fontWeight: 700, color: "#475569" }}>
                      CORRESPONDING PUBLIC KEY
                    </Typography>
                    <Button
                      size="small"
                      startIcon={copiedPub ? <Check size={14} color="#10B981" /> : <Copy size={14} />}
                      onClick={() => copyToClipboard(revealedPublicKey, true)}
                      sx={{ textTransform: "none", fontSize: "0.75rem", py: 0.2 }}
                    >
                      {copiedPub ? "Copied" : "Copy"}
                    </Button>
                  </Box>
                  <Typography
                    variant="body2"
                    className="mono-font"
                    sx={{
                      p: 1.2,
                      backgroundColor: "#FFFFFF",
                      borderRadius: 1.5,
                      border: "1px solid #CBD5E1",
                      wordBreak: "break-all",
                      color: "#0F172A",
                      fontWeight: 600,
                    }}
                  >
                    {revealedPublicKey}
                  </Typography>
                </Box>
              )}
            </Box>
          ) : (
            <>
              {/* Reveal Flow: Password Required */}
              {isReveal && (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <Typography variant="body2" sx={{ color: "#475569" }}>
                    To decrypt and view this sensitive WireGuard secret, please authenticate with your easy42 administrator password:
                  </Typography>

                  <TextField
                    id="link-key-admin-password-input"
                    fullWidth
                    type={showPassword ? "text" : "password"}
                    label="Administrator Password"
                    size="small"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoFocus
                    required
                    disabled={loading}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            size="small"
                            onClick={() => setShowPassword(!showPassword)}
                            edge="end"
                          >
                            {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    }}
                  />
                </Box>
              )}

              {/* Reset Flow: Method Selection */}
              {!isReveal && (
                <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
                  <Typography variant="body2" sx={{ color: "#475569" }}>
                    Choose how you would like to reset the {isPsk ? "WireGuard PSK" : "WireGuard Private Key"}:
                  </Typography>

                  <RadioGroup
                    value={resetMethod}
                    onChange={(e) => setResetMethod(e.target.value as "random" | "custom")}
                    sx={{ gap: 1 }}
                  >
                    <Box
                      onClick={() => setResetMethod("random")}
                      sx={{
                        p: 1.5,
                        borderRadius: 2,
                        border: "1px solid",
                        borderColor: resetMethod === "random" ? "#4F46E5" : "#E2E8F0",
                        backgroundColor: resetMethod === "random" ? "rgba(99, 102, 241, 0.04)" : "#FFFFFF",
                        cursor: "pointer",
                        display: "flex",
                        alignItems: "center",
                      }}
                    >
                      <FormControlLabel
                        value="random"
                        control={<Radio size="small" />}
                        label={
                          <Box>
                            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#0F172A", display: "flex", alignItems: "center", gap: 0.8 }}>
                              <Sparkles size={16} color="#4F46E5" />
                              Generate Cryptographically Secure Random Key
                            </Typography>
                            <Typography variant="caption" sx={{ color: "#64748B" }}>
                              easy42 server generates a standard 32-byte WireGuard key automatically.
                            </Typography>
                          </Box>
                        }
                        sx={{ m: 0, width: "100%" }}
                      />
                    </Box>

                    <Box
                      onClick={() => setResetMethod("custom")}
                      sx={{
                        p: 1.5,
                        borderRadius: 2,
                        border: "1px solid",
                        borderColor: resetMethod === "custom" ? "#4F46E5" : "#E2E8F0",
                        backgroundColor: resetMethod === "custom" ? "rgba(99, 102, 241, 0.04)" : "#FFFFFF",
                        cursor: "pointer",
                        display: "flex",
                        alignItems: "center",
                      }}
                    >
                      <FormControlLabel
                        value="custom"
                        control={<Radio size="small" />}
                        label={
                          <Box>
                            <Typography variant="subtitle2" sx={{ fontWeight: 600, color: "#0F172A", display: "flex", alignItems: "center", gap: 0.8 }}>
                              <Lock size={16} color="#D97706" />
                              Provide Custom Value
                            </Typography>
                            <Typography variant="caption" sx={{ color: "#64748B" }}>
                              Enter your own 32-byte base64-encoded WireGuard secret.
                            </Typography>
                          </Box>
                        }
                        sx={{ m: 0, width: "100%" }}
                      />
                    </Box>
                  </RadioGroup>

                  {resetMethod === "custom" && (
                    <TextField
                      id="link-key-custom-input"
                      fullWidth
                      label={`Custom 32-byte Base64 ${isPsk ? "PSK" : "Private Key"}`}
                      placeholder="e.g. 44 characters base64 ending in ="
                      size="small"
                      value={customKey}
                      onChange={(e) => setCustomKey(e.target.value)}
                      required
                      disabled={loading}
                      helperText="Must be a valid 32-byte WireGuard key encoded in standard Base64."
                    />
                  )}
                </Box>
              )}
            </>
          )}
        </DialogContent>

        <DialogActions sx={{ px: 3, py: 2, borderTop: "1px solid #E2E8F0", backgroundColor: "#F8FAFC" }}>
          {isDone ? (
            <Button
              onClick={onClose}
              variant="contained"
              sx={{ fontWeight: 600, backgroundColor: "#4F46E5" }}
            >
              Close
            </Button>
          ) : (
            <>
              <Button onClick={onClose} disabled={loading} sx={{ color: "#64748B" }}>
                Cancel
              </Button>
              <Button
                type="submit"
                variant="contained"
                disabled={loading || (isReveal && !password)}
                startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
                sx={{
                  fontWeight: 700,
                  backgroundColor: isReveal ? "#4F46E5" : "#D97706",
                  "&:hover": {
                    backgroundColor: isReveal ? "#4338CA" : "#B45309",
                  },
                }}
              >
                {loading
                  ? isReveal
                    ? "Verifying..."
                    : "Resetting..."
                  : isReveal
                  ? "Reveal Secret"
                  : "Reset Secret"}
              </Button>
            </>
          )}
        </DialogActions>
      </form>
    </Dialog>
  );
};
