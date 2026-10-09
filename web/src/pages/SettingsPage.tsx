import React from "react";
import { Box } from "@mui/material";
import { useMesh } from "../context/MeshContext";
import { SettingsModal } from "../components/Modals/SettingsModal";

export const SettingsPage: React.FC = () => {
  const { setAuthenticated } = useMesh();

  return (
    <Box sx={{ width: "100%", height: "100%", minHeight: 0, overflow: "hidden", display: "flex", flexDirection: "column" }}>
      <SettingsModal
        onLogoutAll={() => setAuthenticated(false)}
      />
    </Box>
  );
};
