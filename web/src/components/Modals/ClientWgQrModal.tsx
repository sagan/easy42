import React, { useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  Typography,
} from "@mui/material";
import { Check, Copy, Download, QrCode, X } from "lucide-react";
import QRCode from "qrcode";
import { api } from "../../api/client";
import { downloadTextFile } from "../../utils/common";

interface ClientWgQrModalProps {
  open: boolean;
  onClose: () => void;
  nodeName: string | null;
}

/**
 * Displays a scannable WireGuard QR code for a client node's wg.conf so users
 * can import the tunnel directly in the Android/iOS WireGuard app.
 */
export const ClientWgQrModal: React.FC<ClientWgQrModalProps> = ({ open, onClose, nodeName }) => {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [configText, setConfigText] = useState<string>("");
  const [filename, setFilename] = useState<string>("");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open || !nodeName) {
      return;
    }

    let cancelled = false;
    setLoading(true);
    setError(null);
    setQrDataUrl(null);
    setConfigText("");
    setFilename(`wgc-${nodeName}.conf`);
    setCopied(false);

    (async () => {
      try {
        const res = await api.getClientWgConfig(nodeName);
        if (cancelled) return;
        const dataUrl = await QRCode.toDataURL(res.config, {
          width: 320,
          margin: 2,
          errorCorrectionLevel: "M",
          color: {
            dark: "#000000",
            light: "#FFFFFF",
          },
        });
        if (cancelled) return;
        setConfigText(res.config);
        setFilename(res.filename || `wgc-${nodeName}.conf`);
        setQrDataUrl(dataUrl);
      } catch (err: any) {
        if (!cancelled) {
          setError(err?.message || "Failed to generate WireGuard QR code");
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [open, nodeName]);

  const handleCopy = async () => {
    if (!configText) return;
    try {
      await navigator.clipboard.writeText(configText);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Clipboard API may be unavailable in non-secure contexts
    }
  };

  const handleDownload = () => {
    if (!configText) return;
    downloadTextFile(filename || `wgc-${nodeName || "client"}.conf`, configText);
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth sx={{ zIndex: 1400 }}>
      <DialogTitle
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1,
          pb: 1.5,
          borderBottom: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, minWidth: 0 }}>
          <Box
            sx={{
              width: 34,
              height: 34,
              borderRadius: 1,
              backgroundColor: "rgba(13, 148, 136, 0.12)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#0D9488",
              flexShrink: 0,
            }}
          >
            <QrCode size={18} />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="h6" sx={{ fontWeight: 700, fontSize: "1rem", lineHeight: 1.2 }}>
              WireGuard QR Code
            </Typography>
            {nodeName && (
              <Chip
                label={nodeName}
                size="small"
                sx={{
                  mt: 0.4,
                  height: 20,
                  fontSize: "0.7rem",
                  fontWeight: 600,
                  backgroundColor: "rgba(13, 148, 136, 0.1)",
                  color: "#0F766E",
                }}
              />
            )}
          </Box>
        </Box>
        <IconButton size="small" onClick={onClose} aria-label="Close">
          <X size={18} />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 2, py: 3 }}>
        {loading && (
          <Box sx={{ py: 6, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5 }}>
            <CircularProgress size={32} sx={{ color: "#0D9488" }} />
            <Typography variant="body2" color="text.secondary">
              Generating QR code...
            </Typography>
          </Box>
        )}

        {!loading && error && (
          <Alert severity="error" sx={{ width: "100%", borderRadius: 2 }}>
            {error}
          </Alert>
        )}

        {!loading && !error && qrDataUrl && (
          <>
            <Box
              sx={{
                p: 1.5,
                backgroundColor: "#FFFFFF",
                borderRadius: 2,
                border: "1px solid #E2E8F0",
                boxShadow: "0 4px 12px rgba(15, 23, 42, 0.08)",
                display: "inline-flex",
              }}
            >
              <img
                src={qrDataUrl}
                alt={`WireGuard QR code for ${nodeName || "client"}`}
                style={{ width: 260, height: 260, display: "block" }}
              />
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ textAlign: "center", maxWidth: 300 }}>
              Scan this QR code in the WireGuard app (Android / iOS) to import <strong>{filename}</strong>.
            </Typography>
          </>
        )}
      </DialogContent>

      <DialogActions sx={{ px: 2.5, py: 1.5, borderTop: "1px solid #E2E8F0", justifyContent: "space-between" }}>
        <Box sx={{ display: "flex", gap: 1 }}>
          <Button
            size="small"
            variant="outlined"
            startIcon={copied ? <Check size={14} /> : <Copy size={14} />}
            onClick={handleCopy}
            disabled={loading || !configText}
            sx={{ textTransform: "none", fontWeight: 600 }}
          >
            {copied ? "Copied" : "Copy"}
          </Button>
          <Button
            size="small"
            variant="outlined"
            startIcon={<Download size={14} />}
            onClick={handleDownload}
            disabled={loading || !configText}
            sx={{
              textTransform: "none",
              fontWeight: 600,
              color: "#0D9488",
              borderColor: "#99F6E4",
              "&:hover": {
                borderColor: "#0D9488",
                backgroundColor: "rgba(13, 148, 136, 0.06)",
              },
            }}
          >
            Download
          </Button>
        </Box>
        <Button
          size="small"
          variant="contained"
          onClick={onClose}
          sx={{
            textTransform: "none",
            fontWeight: 600,
            backgroundColor: "#0D9488",
            "&:hover": { backgroundColor: "#0F766E" },
          }}
        >
          Close
        </Button>
      </DialogActions>
    </Dialog>
  );
};
