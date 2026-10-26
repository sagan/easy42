import React, { useEffect } from "react";
import { BrowserRouter, Routes, Route, Navigate, useLocation } from "react-router-dom";
import { ThemeProvider, CssBaseline, Box, CircularProgress, Typography } from "@mui/material";
import { theme } from "./theme";
import { MeshProvider, useMesh } from "./context/MeshContext";
import { AppLayout } from "./components/Layout/AppLayout";
import { NodesPage } from "./pages/NodesPage";
import { NodeDetailPage } from "./pages/NodeDetailPage";
import { TopologyPage } from "./pages/TopologyPage";
import { LookingGlassPage } from "./pages/LookingGlassPage";
import { DeviceHelperPage } from "./pages/DeviceHelperPage";
import { SettingsPage } from "./pages/SettingsPage";
import { LoginPage } from "./components/Login/LoginPage";

const AppContent: React.FC = () => {
  const { checkingAuth, authenticated, setAuthenticated, loadData, networkName } = useMesh();
  const location = useLocation();

  const netName = networkName || "Easy42";

  useEffect(() => {
    if (!authenticated) {
      document.title = `Login - ${netName}`;
      return;
    }

    const path = location.pathname;
    if (path === "/" || path === "/nodes") {
      document.title = netName;
    } else if (path.startsWith("/nodes/")) {
      const nodeName = decodeURIComponent(path.split("/")[2] || "");
      document.title = nodeName ? `${nodeName} - ${netName}` : netName;
    } else if (path.startsWith("/topology")) {
      document.title = `Topology - ${netName}`;
    } else if (path.startsWith("/looking-glass")) {
      document.title = `Looking Glass - ${netName}`;
    } else if (path.startsWith("/helper")) {
      document.title = `Device Helper - ${netName}`;
    } else if (path.startsWith("/settings")) {
      document.title = `Settings - ${netName}`;
    } else {
      document.title = netName;
    }
  }, [location.pathname, authenticated, netName]);

  if (checkingAuth) {
    return (
      <Box
        sx={{
          display: "flex",
          height: "100vh",
          width: "100vw",
          alignItems: "center",
          justifyContent: "center",
          flexDirection: "column",
          gap: 2,
          bgcolor: "#F8FAFC",
        }}
      >
        <CircularProgress size={40} sx={{ color: "#4F46E5" }} />
        <Typography variant="body2" sx={{ color: "#64748B", fontWeight: 600 }}>
          Connecting to {netName}...
        </Typography>
      </Box>
    );
  }

  if (!authenticated) {
    return (
      <LoginPage
        onLoginSuccess={() => {
          setAuthenticated(true);
          loadData();
        }}
      />
    );
  }

  return (
    <Routes>
      <Route path="/" element={<AppLayout />}>
        <Route index element={<Navigate to="/nodes" replace />} />
        <Route path="nodes" element={<NodesPage />} />
        <Route path="nodes/:nodeName" element={<NodeDetailPage />} />
        <Route path="topology" element={<TopologyPage />} />
        <Route path="looking-glass" element={<LookingGlassPage />} />
        <Route path="looking-glass/:tab" element={<LookingGlassPage />} />
        <Route path="helper" element={<DeviceHelperPage />} />
        <Route path="settings" element={<Navigate to="/settings/network" replace />} />
        <Route path="settings/:tab" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/nodes" replace />} />
      </Route>
    </Routes>
  );
};

export const App: React.FC = () => {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <MeshProvider>
        <BrowserRouter>
          <AppContent />
        </BrowserRouter>
      </MeshProvider>
    </ThemeProvider>
  );
};

