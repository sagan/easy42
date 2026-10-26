import React, { useState } from "react";
import { Box, Card, CardContent, Typography, TextField, Button, CircularProgress, Alert, Chip } from "@mui/material";
import { ArrowRight, RefreshCw } from "lucide-react";
import { api } from "../../api/client";
import { Easy42Logo } from "../Logo/Easy42Logo";
import { flushCacheAndServiceWorker } from "../../utils/pwa";
import { useMesh } from "../../context/MeshContext";

interface LoginPageProps {
  onLoginSuccess: () => void;
}

export const LoginPage: React.FC<LoginPageProps> = ({ onLoginSuccess }) => {
  const { networkName } = useMesh();
  const displayName = networkName || "Easy42";
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [appVersion, setAppVersion] = useState<string>("v0.1");
  const [flushLoading, setFlushLoading] = useState(false);
  const [flushMsg, setFlushMsg] = useState<{ text: string; severity: "success" | "error" } | null>(null);

  React.useEffect(() => {
    api
      .getVersion()
      .then((info) => {
        if (info.version) {
          const display = info.version.startsWith("v") ? info.version : `v${info.version}`;
          setAppVersion(display);
        }
      })
      .catch(() => {
        // Fallback to default
      });
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;

    setLoading(true);
    setError(null);

    try {
      await api.login(password);
      onLoginSuccess();
    } catch (err: unknown) {
      const e = err as Error;
      setError(e.message || "Invalid password");
    } finally {
      setLoading(false);
    }
  };

  const handleFlushCache = async () => {
    setFlushLoading(true);
    setFlushMsg(null);
    try {
      const res = await flushCacheAndServiceWorker();
      if (res.success) {
        setFlushMsg({
          text: `Cache flushed & service worker uninstalled (${res.uninstalled} worker(s), ${res.cachesDeleted} cache(s)). Reloading...`,
          severity: "success",
        });
        setTimeout(() => {
          window.location.reload();
        }, 1000);
      } else {
        setFlushMsg({ text: res.error || "Failed to flush cache", severity: "error" });
      }
    } catch (err: unknown) {
      setFlushMsg({ text: (err as Error).message || "Failed to flush cache", severity: "error" });
    } finally {
      setFlushLoading(false);
    }
  };

  return (
    <Box
      sx={{
        minHeight: "100vh",
        width: "100vw",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        backgroundColor: "#F8FAFC",
        backgroundImage: `
          radial-gradient(circle at 15% 15%, rgba(79, 70, 229, 0.08), transparent 35%),
          radial-gradient(circle at 85% 85%, rgba(8, 145, 178, 0.08), transparent 35%)
        `,
        p: 2,
      }}
    >
      <Card
        elevation={0}
        sx={{
          maxWidth: 400,
          width: "100%",
          backgroundColor: "#FFFFFF",
          border: "1px solid #E2E8F0",
          borderRadius: 4,
          boxShadow: "0 20px 25px -5px rgba(0, 0, 0, 0.08), 0 8px 10px -6px rgba(0, 0, 0, 0.04)",
          overflow: "hidden",
        }}
      >
        <Box
          sx={{
            height: 4,
            background: "linear-gradient(90deg, #4F46E5 0%, #0891B2 100%)",
          }}
        />

        <CardContent sx={{ p: 4, display: "flex", flexDirection: "column", gap: 3 }}>
          {/* Header */}
          <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center", gap: 1 }}>
            <Easy42Logo
              size={52}
              sx={{
                mb: 1,
                borderRadius: "14px",
                boxShadow: "0 4px 14px rgba(79, 70, 229, 0.3)",
              }}
            />

            <Box sx={{ display: "flex", alignItems: "center", gap: 1, width: "100%", justifyContent: "space-between" }}>
              <Typography variant="h5" sx={{ fontWeight: 800, letterSpacing: "-0.5px", color: "#0F172A" }}>
                {displayName}
              </Typography>
              <Chip
                label={appVersion}
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  backgroundColor: "rgba(79, 70, 229, 0.08)",
                  color: "#4F46E5",
                }}
              />
            </Box>

            <Typography variant="body2" sx={{ color: "#64748B", width: "100%", textAlign: "left" }}>
              WireGuard Mesh Overlay Network Manager
            </Typography>
          </Box>

          {/* Login Form */}
          <form onSubmit={handleSubmit}>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 2.5 }}>
              <TextField
                fullWidth
                type="password"
                label="Admin Password"
                placeholder="Enter password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoFocus
                required
                disabled={loading}
              />

              {error && (
                <Alert severity="error" sx={{ borderRadius: 2 }}>
                  {error}
                </Alert>
              )}

              <Button
                fullWidth
                type="submit"
                variant="contained"
                size="large"
                disabled={loading || !password}
                endIcon={loading ? <CircularProgress size={18} color="inherit" /> : <ArrowRight size={18} />}
                sx={{
                  py: 1.2,
                  fontWeight: 700,
                  color: "#FFFFFF",
                  background: "linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)",
                }}
              >
                {loading ? "Authenticating..." : "Enter Dashboard"}
              </Button>
            </Box>
          </form>

          <Box sx={{ pt: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 1.5 }}>
            {flushMsg && (
              <Alert severity={flushMsg.severity} sx={{ width: "100%", borderRadius: 2, fontSize: "0.75rem", py: 0.5 }}>
                {flushMsg.text}
              </Alert>
            )}

            <Button
              size="small"
              variant="text"
              onClick={handleFlushCache}
              disabled={flushLoading}
              startIcon={flushLoading ? <CircularProgress size={12} color="inherit" /> : <RefreshCw size={13} />}
              sx={{
                color: "#64748B",
                fontSize: "0.75rem",
                textTransform: "none",
                fontWeight: 600,
                "&:hover": { color: "#4F46E5", bgcolor: "rgba(79, 70, 229, 0.04)" },
              }}
            >
              {flushLoading ? "Flushing cache..." : "Flush cache & service worker"}
            </Button>

            <Typography
              component="a"
              href="https://github.com/sagan/easy42"
              target="_blank"
              rel="noopener noreferrer"
              variant="caption"
              sx={{
                color: "#94A3B8",
                textDecoration: "none",
                display: "inline-flex",
                alignItems: "center",
                gap: 0.5,
                transition: "color 0.2s",
                "&:hover": { color: "#4F46E5" },
              }}
            >
              GitHub: sagan/easy42
            </Typography>
          </Box>
        </CardContent>
      </Card>
    </Box>
  );
};
