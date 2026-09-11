import React, { useState } from "react";
import {
  AppBar,
  Toolbar,
  Typography,
  Button,
  Box,
  Chip,
  IconButton,
  Tooltip,
  CircularProgress,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Divider,
  Select,
  FormControl,
} from "@mui/material";
import {
  Plus,
  Link as LinkIcon,
  RefreshCw,
  Lock,
  Unlock,
  LogOut,
  Network,
  Server,
  Layers,
  Settings,
  Radio,
  ChevronDown,
  Share2,
  Tag,
  Wrench,
  AlertTriangle,
  Compass,
} from "lucide-react";
import { Node, NodeStatus } from "../types/api";

interface NavbarProps {
  nodeCount: number;
  totalNodeCount: number;
  linkCount: number;
  totalLinkCount: number;
  isUnlocked: boolean;
  uniqueTags: string[];
  selectedTag: string;
  onSelectTag: (tag: string) => void;
  onAddNode: () => void;
  onAddLink: () => void;
  onCreateFullMesh: () => void;
  missingMeshLinksCount: number;
  displayedNodeCount: number;
  onSync: () => void;
  onUpdateState?: () => void;
  updatingState?: boolean;
  onOpenHelper?: () => void;
  onOpenLookingGlass?: () => void;
  onUnlockToggle: () => void;
  onOpenSettings: () => void;
  onLogout: () => void;
  unreachableNodes?: Node[];
  nodeStatuses?: Record<string, NodeStatus>;
  nodes?: Node[];
  onUpdateNodeState?: (nodeName: string) => void;
}

export const Navbar: React.FC<NavbarProps> = ({
  nodeCount,
  totalNodeCount,
  linkCount,
  totalLinkCount,
  isUnlocked,
  uniqueTags,
  selectedTag,
  onSelectTag,
  onAddNode,
  onAddLink,
  onCreateFullMesh,
  missingMeshLinksCount,
  displayedNodeCount,
  onSync,
  onUpdateState,
  updatingState,
  onOpenHelper,
  onOpenLookingGlass,
  onUnlockToggle,
  onOpenSettings,
  onLogout,
  unreachableNodes,
  nodeStatuses,
  nodes,
  onUpdateNodeState,
}) => {
  const [addMenuAnchor, setAddMenuAnchor] = useState<null | HTMLElement>(null);
  const [unreachableAnchor, setUnreachableAnchor] = useState<null | HTMLElement>(null);
  const [updateMenuAnchor, setUpdateMenuAnchor] = useState<null | HTMLElement>(null);

  return (
    <AppBar
      position="static"
      elevation={0}
      sx={{
        backgroundColor: "#FFFFFF",
        borderBottom: "1px solid #E2E8F0",
        zIndex: 10,
      }}
    >
      <Toolbar sx={{ justifyContent: "space-between", minHeight: 64 }}>
        {/* Brand */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: 38,
              height: 38,
              borderRadius: 2,
              background: "linear-gradient(135deg, #4F46E5 0%, #0891B2 100%)",
              boxShadow: "0 2px 8px rgba(79, 70, 229, 0.3)",
            }}
          >
            <Network size={22} color="#FFFFFF" />
          </Box>
          <Box>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
              <Typography
                variant="h6"
                sx={{ fontWeight: 800, letterSpacing: "-0.5px", lineHeight: 1, color: "#0F172A" }}
              >
                easy<span style={{ color: "#0891B2" }}>42</span>
              </Typography>
              <Chip
                label="Mesh Manager"
                size="small"
                sx={{
                  height: 20,
                  fontSize: "0.65rem",
                  fontWeight: 700,
                  backgroundColor: "rgba(79, 70, 229, 0.08)",
                  color: "#4F46E5",
                  border: "1px solid rgba(79, 70, 229, 0.2)",
                }}
              />
            </Box>
            <Typography variant="caption" sx={{ color: "#64748B", fontSize: "0.75rem" }}>
              Linux & WireGuard Overlay Network
            </Typography>
          </Box>
        </Box>

        {/* Mesh Stats & Tag Filter */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          <Chip
            icon={<Server size={14} color="#4F46E5" />}
            label={selectedTag === "All" ? `${totalNodeCount} Nodes` : `${nodeCount}/${totalNodeCount} Nodes`}
            size="small"
            sx={{
              backgroundColor: "#F1F5F9",
              border: "1px solid #E2E8F0",
              color: "#334155",
              fontWeight: 600,
              px: 0.5,
            }}
          />
          <Chip
            icon={<Layers size={14} color="#0891B2" />}
            label={selectedTag === "All" ? `${totalLinkCount} Links` : `${linkCount}/${totalLinkCount} Links`}
            size="small"
            sx={{
              backgroundColor: "#F1F5F9",
              border: "1px solid #E2E8F0",
              color: "#334155",
              fontWeight: 600,
              px: 0.5,
            }}
          />

          {unreachableNodes && unreachableNodes.length > 0 && (
            <Tooltip title={`${unreachableNodes.length} device(s) currently unreachable via SSH. Click to view & retry.`}>
              <Chip
                icon={<AlertTriangle size={13} color="#DC2626" />}
                label={`${unreachableNodes.length} Unreachable`}
                size="small"
                onClick={(e) => setUnreachableAnchor(e.currentTarget)}
                sx={{
                  backgroundColor: "#FEF2F2",
                  border: "1px solid #FECDD3",
                  color: "#DC2626",
                  fontWeight: 700,
                  cursor: "pointer",
                  px: 0.5,
                  "&:hover": { backgroundColor: "#FEE2E2" },
                }}
              />
            </Tooltip>
          )}

          {/* Tag Selector */}
          <FormControl size="small">
            <Select
              value={selectedTag}
              onChange={(e) => onSelectTag(e.target.value)}
              displayEmpty
              startAdornment={
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    mr: 0.75,
                    color: selectedTag === "All" ? "#64748B" : "#0891B2",
                  }}
                >
                  <Tag size={13} />
                </Box>
              }
              sx={{
                height: 28,
                fontSize: "0.75rem",
                fontWeight: 600,
                borderRadius: 1.5,
                backgroundColor: selectedTag === "All" ? "#F8FAFC" : "rgba(8, 145, 178, 0.08)",
                color: selectedTag === "All" ? "#334155" : "#0891B2",
                "& .MuiSelect-select": { py: 0.3, pr: 3, pl: 0.5 },
                "& .MuiOutlinedInput-notchedOutline": {
                  borderColor: selectedTag === "All" ? "#CBD5E1" : "rgba(8, 145, 178, 0.3)",
                },
                "&:hover .MuiOutlinedInput-notchedOutline": {
                  borderColor: "#0891B2",
                },
              }}
            >
              <MenuItem value="All" sx={{ fontSize: "0.8rem", fontWeight: 600 }}>
                Tag: All ({totalNodeCount})
              </MenuItem>
              {uniqueTags.map((tag) => (
                <MenuItem key={tag} value={tag} sx={{ fontSize: "0.8rem", fontWeight: 600, color: "#0891B2" }}>
                  #{tag}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
        </Box>

        {/* Action Controls */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
          {/* Multiple Options Add Dropdown Menu */}
          <Button
            variant="outlined"
            size="small"
            startIcon={<Plus size={16} />}
            endIcon={<ChevronDown size={14} />}
            onClick={(e) => setAddMenuAnchor(e.currentTarget)}
            sx={{
              borderColor: "#CBD5E1",
              color: "#1E293B",
              "&:hover": {
                borderColor: "#4F46E5",
                backgroundColor: "rgba(79, 70, 229, 0.05)",
              },
            }}
          >
            Add
          </Button>
          <Menu
            anchorEl={addMenuAnchor}
            open={Boolean(addMenuAnchor)}
            onClose={() => setAddMenuAnchor(null)}
            anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
            transformOrigin={{ vertical: "top", horizontal: "left" }}
            PaperProps={{
              sx: {
                minWidth: 270,
                p: 0.5,
                borderRadius: 2,
                boxShadow: "0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1)",
                border: "1px solid #E2E8F0",
              },
            }}
          >
            <MenuItem
              onClick={() => {
                setAddMenuAnchor(null);
                onAddNode();
              }}
              sx={{ borderRadius: 1.5, py: 1 }}
            >
              <ListItemIcon sx={{ minWidth: 32, color: "#4F46E5" }}>
                <Server size={18} />
              </ListItemIcon>
              <ListItemText
                primary={
                  <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                    Add Node
                  </Typography>
                }
                secondary={
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Configure new network node
                  </Typography>
                }
              />
            </MenuItem>

            <Divider sx={{ my: 0.5, borderColor: "#F1F5F9" }} />

            <MenuItem
              onClick={() => {
                setAddMenuAnchor(null);
                onAddLink();
              }}
              sx={{ borderRadius: 1.5, py: 1 }}
            >
              <ListItemIcon sx={{ minWidth: 32, color: "#0891B2" }}>
                <LinkIcon size={18} />
              </ListItemIcon>
              <ListItemText
                primary={
                  <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                    Add Link
                  </Typography>
                }
                secondary={
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    Custom point-to-point link
                  </Typography>
                }
              />
            </MenuItem>

            <Divider sx={{ my: 0.5, borderColor: "#F1F5F9" }} />

            <MenuItem
              onClick={() => {
                setAddMenuAnchor(null);
                onCreateFullMesh();
              }}
              disabled={displayedNodeCount < 2}
              sx={{ borderRadius: 1.5, py: 1 }}
            >
              <ListItemIcon sx={{ minWidth: 32, color: "#4F46E5" }}>
                <Share2 size={18} />
              </ListItemIcon>
              <ListItemText
                primary={
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                    <Typography variant="body2" sx={{ fontWeight: 600, color: "#0F172A" }}>
                      Create full mesh
                    </Typography>
                    {missingMeshLinksCount > 0 && (
                      <Chip
                        label={`+${missingMeshLinksCount}`}
                        size="small"
                        sx={{
                          height: 18,
                          fontSize: "0.65rem",
                          fontWeight: 700,
                          backgroundColor: "rgba(79, 70, 229, 0.1)",
                          color: "#4F46E5",
                        }}
                      />
                    )}
                  </Box>
                }
                secondary={
                  <Typography variant="caption" sx={{ color: "#64748B" }}>
                    {displayedNodeCount < 2
                      ? "Need at least 2 displayed internal nodes"
                      : missingMeshLinksCount === 0
                        ? "All displayed internal nodes already meshed"
                        : `Add ${missingMeshLinksCount} missing link(s) between ${displayedNodeCount} internal nodes`}
                  </Typography>
                }
              />
            </MenuItem>
          </Menu>

          {/* Update State */}
          <Box sx={{ display: "inline-flex", alignItems: "center" }}>
            <Tooltip title="Connect to all devices via SSH to fetch their live state and reconcile state.json">
              <Button
                variant="outlined"
                size="small"
                startIcon={updatingState ? <CircularProgress size={14} color="inherit" /> : <Radio size={16} />}
                onClick={onUpdateState}
                disabled={updatingState}
                sx={{
                  borderTopRightRadius: nodes && nodes.length > 0 ? 0 : undefined,
                  borderBottomRightRadius: nodes && nodes.length > 0 ? 0 : undefined,
                  borderColor: "#CBD5E1",
                  color: "#334155",
                  fontWeight: 600,
                  fontSize: "0.8rem",
                  textTransform: "none",
                  "&:hover": {
                    borderColor: "#0284C7",
                    backgroundColor: "rgba(2, 132, 199, 0.05)",
                    color: "#0284C7",
                  },
                }}
              >
                {updatingState ? "Updating State..." : "Update State"}
              </Button>
            </Tooltip>
            {nodes && nodes.length > 0 && onUpdateNodeState && (
              <Tooltip title="Update specific node & its links">
                <Button
                  size="small"
                  variant="outlined"
                  onClick={(e) => setUpdateMenuAnchor(e.currentTarget)}
                  disabled={updatingState}
                  sx={{
                    minWidth: "28px",
                    px: 0.5,
                    borderTopLeftRadius: 0,
                    borderBottomLeftRadius: 0,
                    borderLeft: 0,
                    borderColor: "#CBD5E1",
                    color: "#475569",
                    "&:hover": {
                      borderColor: "#0284C7",
                      backgroundColor: "rgba(2, 132, 199, 0.05)",
                      color: "#0284C7",
                    },
                  }}
                >
                  <ChevronDown size={14} />
                </Button>
              </Tooltip>
            )}
          </Box>

          {/* Update Specific Node Menu */}
          <Menu
            anchorEl={updateMenuAnchor}
            open={Boolean(updateMenuAnchor)}
            onClose={() => setUpdateMenuAnchor(null)}
            PaperProps={{
              sx: {
                width: 250,
                maxHeight: 360,
                borderRadius: 2,
                boxShadow: "0 10px 15px -3px rgba(0, 0, 0, 0.1)",
                border: "1px solid #E2E8F0",
              },
            }}
          >
            <MenuItem
              onClick={() => {
                setUpdateMenuAnchor(null);
                if (onUpdateState) onUpdateState();
              }}
            >
              <ListItemIcon>
                <Radio size={16} color="#0284C7" />
              </ListItemIcon>
              <ListItemText primary="Update All Nodes" secondary="Probes all cluster devices" />
            </MenuItem>
            <Divider sx={{ my: 0.5 }} />
            <Box sx={{ px: 2, py: 0.5 }}>
              <Typography variant="caption" sx={{ fontWeight: 700, color: "#64748B", letterSpacing: "0.5px" }}>
                REFRESH SPECIFIC NODE
              </Typography>
            </Box>
            {nodes
              ?.filter((n) => !n.is_external)
              .map((n) => {
                const st = nodeStatuses?.[n.name];
                const isOnline = st ? st.connected : true;
                return (
                  <MenuItem
                    key={n.name}
                    onClick={() => {
                      setUpdateMenuAnchor(null);
                      if (onUpdateNodeState) onUpdateNodeState(n.name);
                    }}
                  >
                    <ListItemIcon>
                      {isOnline ? <Server size={15} color="#4F46E5" /> : <AlertTriangle size={15} color="#DC2626" />}
                    </ListItemIcon>
                    <ListItemText
                      primary={n.name}
                      secondary={isOnline ? n.host || "SSH Host" : "Unreachable"}
                      secondaryTypographyProps={{
                        sx: { color: isOnline ? "#64748B" : "#DC2626", fontSize: "0.7rem" },
                      }}
                    />
                  </MenuItem>
                );
              })}
          </Menu>

          {/* Unreachable Nodes Menu */}
          <Menu
            anchorEl={unreachableAnchor}
            open={Boolean(unreachableAnchor)}
            onClose={() => setUnreachableAnchor(null)}
            PaperProps={{
              sx: {
                width: 320,
                maxHeight: 400,
                borderRadius: 2,
                boxShadow: "0 10px 15px -3px rgba(220, 38, 38, 0.15)",
                border: "1px solid #FECDD3",
              },
            }}
          >
            <Box sx={{ p: 1.5, borderBottom: "1px solid #FECDD3", backgroundColor: "#FEF2F2" }}>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <AlertTriangle size={16} color="#DC2626" />
                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#991B1B" }}>
                  Unreachable Devices ({unreachableNodes?.length || 0})
                </Typography>
              </Box>
              <Typography variant="caption" sx={{ color: "#B91C1C", display: "block", mt: 0.5 }}>
                These nodes failed SSH connection or timed out.
              </Typography>
            </Box>
            {unreachableNodes?.map((n) => {
              const st = nodeStatuses?.[n.name];
              return (
                <MenuItem
                  key={n.name}
                  sx={{
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "flex-start",
                    py: 1,
                    borderBottom: "1px solid #F1F5F9",
                  }}
                  onClick={() => {
                    setUnreachableAnchor(null);
                    if (onUpdateNodeState) onUpdateNodeState(n.name);
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", width: "100%" }}>
                    <Typography variant="body2" sx={{ fontWeight: 700, color: "#0F172A" }}>
                      {n.name}
                    </Typography>
                    <Button
                      size="small"
                      variant="text"
                      startIcon={<RefreshCw size={12} />}
                      sx={{ fontSize: "0.7rem", py: 0, textTransform: "none" }}
                    >
                      Retry
                    </Button>
                  </Box>
                  <Typography variant="caption" className="mono-font" sx={{ color: "#64748B", fontSize: "0.7rem" }}>
                    Host: {n.host}
                  </Typography>
                  {st?.error && (
                    <Typography
                      variant="caption"
                      className="mono-font"
                      sx={{
                        color: "#DC2626",
                        fontSize: "0.68rem",
                        display: "block",
                        mt: 0.25,
                        backgroundColor: "#FFF5F5",
                        p: 0.5,
                        borderRadius: 1,
                        width: "100%",
                        wordBreak: "break-all",
                      }}
                    >
                      {st.error}
                    </Typography>
                  )}
                </MenuItem>
              );
            })}
          </Menu>

          {onOpenHelper && (
            <Tooltip title="Device Config Helper Tasks">
              <Button
                variant="outlined"
                size="small"
                startIcon={<Wrench size={16} />}
                onClick={onOpenHelper}
                sx={{
                  borderColor: "#CBD5E1",
                  color: "#334155",
                  fontWeight: 600,
                  fontSize: "0.8rem",
                  textTransform: "none",
                  "&:hover": {
                    borderColor: "#4F46E5",
                    backgroundColor: "rgba(79, 70, 229, 0.05)",
                    color: "#4F46E5",
                  },
                }}
              >
                Device Helper
              </Button>
            </Tooltip>
          )}

          {onOpenLookingGlass && (
            <Tooltip title="Looking Glass (BGP Routing, Ping, Traceroute)">
              <Button
                variant="outlined"
                size="small"
                startIcon={<Compass size={16} />}
                onClick={onOpenLookingGlass}
                sx={{
                  borderColor: "#CBD5E1",
                  color: "#334155",
                  fontWeight: 600,
                  fontSize: "0.8rem",
                  textTransform: "none",
                  "&:hover": {
                    borderColor: "#4F46E5",
                    backgroundColor: "rgba(79, 70, 229, 0.05)",
                    color: "#4F46E5",
                  },
                }}
              >
                Looking Glass
              </Button>
            </Tooltip>
          )}

          <Button
            variant="contained"
            color="primary"
            size="small"
            startIcon={<RefreshCw size={16} />}
            onClick={onSync}
            sx={{
              background: "linear-gradient(135deg, #4F46E5 0%, #3730A3 100%)",
              fontWeight: 700,
              color: "#FFFFFF",
            }}
          >
            Sync Mesh
          </Button>

          {/* Settings */}
          <Tooltip title="Settings">
            <IconButton
              size="small"
              onClick={onOpenSettings}
              sx={{
                border: "1px solid #E2E8F0",
                color: "#64748B",
                "&:hover": {
                  color: "#4F46E5",
                  borderColor: "#CBD5E1",
                  backgroundColor: "rgba(79, 70, 229, 0.06)",
                },
              }}
            >
              <Settings size={18} />
            </IconButton>
          </Tooltip>

          {/* Vault Lock/Unlock */}
          <Tooltip title={isUnlocked ? "Vault is Unlocked (Click to Lock)" : "Vault is Locked (Click to Unlock)"}>
            <IconButton
              size="small"
              onClick={onUnlockToggle}
              sx={{
                border: "1px solid",
                borderColor: isUnlocked ? "rgba(5, 150, 105, 0.3)" : "rgba(217, 119, 6, 0.3)",
                backgroundColor: isUnlocked ? "rgba(5, 150, 105, 0.08)" : "rgba(217, 119, 6, 0.08)",
                color: isUnlocked ? "#059669" : "#D97706",
              }}
            >
              {isUnlocked ? <Unlock size={18} /> : <Lock size={18} />}
            </IconButton>
          </Tooltip>

          {/* Logout */}
          <Tooltip title="Logout">
            <IconButton
              size="small"
              onClick={onLogout}
              sx={{
                border: "1px solid #E2E8F0",
                color: "#64748B",
                "&:hover": { color: "#E11D48", backgroundColor: "rgba(225, 29, 72, 0.06)" },
              }}
            >
              <LogOut size={18} />
            </IconButton>
          </Tooltip>
        </Box>
      </Toolbar>
    </AppBar>
  );
};
