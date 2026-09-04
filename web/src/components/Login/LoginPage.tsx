import React, { useState } from "react";
import { Box, Card, CardContent, Typography, TextField, Button, CircularProgress, Alert, Chip } from "@mui/material";
import { Network, ArrowRight } from "lucide-react";
import { api } from "../../api/client";

interface LoginPageProps {
  onLoginSuccess: () => void;
}

export const LoginPage: React.FC<LoginPageProps> = ({ onLoginSuccess }) => {
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
            <Box
              sx={{
                width: 52,
                height: 52,
                borderRadius: 3,
                background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
                boxShadow: "0 4px 14px rgba(79, 70, 229, 0.3)",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                mb: 1,
              }}
            >
              <Network size={28} color="#FFFFFF" />
            </Box>

            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography variant="h5" sx={{ fontWeight: 800, letterSpacing: "-0.5px", color: "#0F172A" }}>
                easy<span style={{ color: "#0891B2" }}>42</span>
              </Typography>
              <Chip
                label="v0.1"
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

            <Typography variant="body2" sx={{ color: "#64748B" }}>
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
        </CardContent>
      </Card>
    </Box>
  );
};
