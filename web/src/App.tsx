import React from "react";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ThemeProvider, CssBaseline, Box, CircularProgress, Typography } from "@mui/material";
import { theme } from "./theme";
import { MeshProvider, useMesh } from "./context/MeshContext";
import { AppLayout } from "./components/Layout/AppLayout";
import { NodesPage } from "./pages/NodesPage";
import { TopologyPage } from "./pages/TopologyPage";
import { LookingGlassPage } from "./pages/LookingGlassPage";
import { DeviceHelperPage } from "./pages/DeviceHelperPage";
import { SettingsPage } from "./pages/SettingsPage";
import { LoginPage } from "./components/Login/LoginPage";

const AppContent: React.FC = () => {
  const { checkingAuth, authenticated, setAuthenticated, loadData } = useMesh();

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
          Connecting to easy42...
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
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<Navigate to="/nodes" replace />} />
          <Route path="nodes" element={<NodesPage />} />
          <Route path="topology" element={<TopologyPage />} />
          <Route path="looking-glass" element={<LookingGlassPage />} />
          <Route path="helper" element={<DeviceHelperPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/nodes" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
};

export const App: React.FC = () => {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <MeshProvider>
        <AppContent />
      </MeshProvider>
    </ThemeProvider>
  );
};
