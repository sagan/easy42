import React, { useState, useEffect, useCallback, useMemo } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Typography,
  Paper,
  Button,
  IconButton,
  Chip,
  Tooltip,
  CircularProgress,
  TextField,
  InputAdornment,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Stack,
  Fade,
} from "@mui/material";
import {
  Bell,
  CheckCheck,
  Trash2,
  RefreshCw,
  Server,
  ExternalLink,
  AlertTriangle,
  Wifi,
  WifiOff,
  Clock,
  Search,
  Check,
  CheckCircle2,
} from "lucide-react";
import { api } from "../api/client";
import { EventItem, EventType } from "../types/api";
import { useMesh } from "../context/MeshContext";

export const EventsPage: React.FC = () => {
  const navigate = useNavigate();
  const { unreadEventsCount, refreshUnreadEventsCount } = useMesh();

  const [events, setEvents] = useState<EventItem[]>([]);
  const [totalCount, setTotalCount] = useState<number>(0);
  const [loading, setLoading] = useState<boolean>(true);
  const [refreshing, setRefreshing] = useState<boolean>(false);

  // Filters
  const [searchQuery, setSearchQuery] = useState<string>("");
  const [selectedType, setSelectedType] = useState<string>("all");
  const [unreadOnly, setUnreadOnly] = useState<boolean>(false);

  // Clear confirmation dialog
  const [clearDialogOpen, setClearDialogOpen] = useState<boolean>(false);
  const [clearing, setClearing] = useState<boolean>(false);

  const fetchEvents = useCallback(async () => {
    try {
      const res = await api.getEvents({
        limit: 200,
        offset: 0,
        unread_only: unreadOnly,
        type: selectedType !== "all" ? selectedType : undefined,
      });
      setEvents(res.events || []);
      setTotalCount(res.total || 0);
      refreshUnreadEventsCount();
    } catch (err) {
      console.error("Failed to load events:", err);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [unreadOnly, selectedType, refreshUnreadEventsCount]);

  useEffect(() => {
    setLoading(true);
    fetchEvents();
  }, [fetchEvents]);

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchEvents();
  };

  const handleMarkAsRead = async (id: number) => {
    try {
      await api.markEventRead(id);
      setEvents((prev) => prev.map((e) => (e.id === id ? { ...e, is_read: true } : e)));
      refreshUnreadEventsCount();
    } catch (err) {
      console.error("Failed to mark event as read:", err);
    }
  };

  const handleMarkAllRead = async () => {
    try {
      await api.markAllEventsRead();
      setEvents((prev) => prev.map((e) => ({ ...e, is_read: true })));
      refreshUnreadEventsCount();
    } catch (err) {
      console.error("Failed to mark all as read:", err);
    }
  };

  const handleDeleteEvent = async (id: number) => {
    try {
      await api.deleteEvent(id);
      setEvents((prev) => prev.filter((e) => e.id !== id));
      setTotalCount((prev) => Math.max(0, prev - 1));
      refreshUnreadEventsCount();
    } catch (err) {
      console.error("Failed to delete event:", err);
    }
  };

  const handleClearAll = async () => {
    setClearing(true);
    try {
      await api.clearEvents();
      setEvents([]);
      setTotalCount(0);
      setClearDialogOpen(false);
      refreshUnreadEventsCount();
    } catch (err) {
      console.error("Failed to clear events:", err);
    } finally {
      setClearing(false);
    }
  };

  // Filtered in memory for instant full-text search
  const filteredEvents = useMemo(() => {
    let result = events;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase().trim();
      result = result.filter(
        (e) =>
          e.node_name.toLowerCase().includes(q) ||
          e.title.toLowerCase().includes(q) ||
          e.message.toLowerCase().includes(q) ||
          (e.task_id && e.task_id.toLowerCase().includes(q)) ||
          (e.metadata?.target && String(e.metadata.target).toLowerCase().includes(q)) ||
          (e.metadata?.task_name && String(e.metadata.task_name).toLowerCase().includes(q)),
      );
    }
    return result;
  }, [events, searchQuery]);

  // Statistics
  const stats = useMemo(() => {
    let offline = 0;
    let online = 0;
    let cronFailure = 0;
    events.forEach((e) => {
      if (e.type === "node_offline") offline++;
      else if (e.type === "node_online") online++;
      else if (e.type === "cron_task_failure") cronFailure++;
    });
    return { offline, online, cronFailure };
  }, [events]);

  const formatRelativeTime = (isoString: string) => {
    try {
      const date = new Date(isoString);
      const now = new Date();
      const diffSec = Math.floor((now.getTime() - date.getTime()) / 1000);

      if (diffSec < 60) return "just now";
      if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
      if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
      if (diffSec < 604800) return `${Math.floor(diffSec / 86400)}d ago`;
      return date.toLocaleDateString();
    } catch {
      return isoString;
    }
  };

  const formatExactDate = (isoString: string) => {
    try {
      const date = new Date(isoString);
      return date.toUTCString();
    } catch {
      return isoString;
    }
  };

  const renderTypeBadge = (type: EventType) => {
    switch (type) {
      case "node_offline":
        return (
          <Chip
            size="small"
            icon={<WifiOff size={13} color="#E11D48" />}
            label="Node Offline"
            sx={{
              bgcolor: "#FFE4E6",
              color: "#BE123C",
              fontWeight: 700,
              fontSize: "0.73rem",
              borderRadius: "6px",
              height: 24,
            }}
          />
        );
      case "node_online":
        return (
          <Chip
            size="small"
            icon={<Wifi size={13} color="#059669" />}
            label="Node Online"
            sx={{
              bgcolor: "#D1FAE5",
              color: "#047857",
              fontWeight: 700,
              fontSize: "0.73rem",
              borderRadius: "6px",
              height: 24,
            }}
          />
        );
      case "cron_task_failure":
        return (
          <Chip
            size="small"
            icon={<AlertTriangle size={13} color="#D97706" />}
            label="Cron Task Failure"
            sx={{
              bgcolor: "#FEF3C7",
              color: "#B45309",
              fontWeight: 700,
              fontSize: "0.73rem",
              borderRadius: "6px",
              height: 24,
            }}
          />
        );
      default:
        return (
          <Chip
            size="small"
            label={type}
            sx={{
              bgcolor: "#F1F5F9",
              color: "#475569",
              fontWeight: 600,
              fontSize: "0.73rem",
              borderRadius: "6px",
              height: 24,
            }}
          />
        );
    }
  };

  return (
    <Box
      sx={{
        width: "100%",
        boxSizing: "border-box",
        p: { xs: 1.5, sm: 2.5, md: 3 },
        display: "flex",
        flexDirection: "column",
        gap: 2.5,
      }}
    >
      {/* Top Header Bar */}
      <Box
        sx={{
          display: "flex",
          flexDirection: { xs: "column", sm: "row" },
          alignItems: { xs: "flex-start", sm: "center" },
          justifyContent: "space-between",
          gap: 2,
        }}
      >
        <Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1.5 }}>
            <Box
              sx={{
                width: 40,
                height: 40,
                borderRadius: "10px",
                bgcolor: "#EEF2FF",
                color: "#4F46E5",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                boxShadow: "0 2px 6px rgba(79, 70, 229, 0.15)",
              }}
            >
              <Bell size={22} />
            </Box>
            <Box>
              <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                <Typography variant="h5" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.2 }}>
                  Events & Notifications
                </Typography>
                {unreadEventsCount > 0 && (
                  <Chip
                    label={`${unreadEventsCount} new`}
                    size="small"
                    sx={{
                      bgcolor: "#EF4444",
                      color: "#FFFFFF",
                      fontWeight: 700,
                      fontSize: "0.72rem",
                      height: 22,
                    }}
                  />
                )}
              </Box>
              <Typography variant="body2" sx={{ color: "#64748B", mt: 0.3 }}>
                Fleet connectivity transitions, Looking Glass cron task alerts, and system notifications ({totalCount}{" "}
                total).
              </Typography>
            </Box>
          </Box>
        </Box>

        {/* Global Actions */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.2, alignSelf: { xs: "stretch", sm: "auto" } }}>
          <Button
            size="small"
            variant="outlined"
            onClick={handleMarkAllRead}
            disabled={unreadEventsCount === 0}
            startIcon={<CheckCheck size={16} />}
            sx={{
              borderColor: "#CBD5E1",
              color: "#334155",
              fontWeight: 600,
              fontSize: "0.82rem",
              textTransform: "none",
              borderRadius: "8px",
              py: 0.8,
              px: 1.8,
              "&:hover": {
                borderColor: "#4F46E5",
                bgcolor: "rgba(79, 70, 229, 0.05)",
                color: "#4F46E5",
              },
            }}
          >
            Mark all read
          </Button>

          <Button
            size="small"
            variant="outlined"
            color="error"
            onClick={() => setClearDialogOpen(true)}
            disabled={events.length === 0}
            startIcon={<Trash2 size={16} />}
            sx={{
              borderColor: "#FECDD3",
              color: "#E11D48",
              fontWeight: 600,
              fontSize: "0.82rem",
              textTransform: "none",
              borderRadius: "8px",
              py: 0.8,
              px: 1.8,
              "&:hover": {
                bgcolor: "#FFF1F2",
                borderColor: "#E11D48",
              },
            }}
          >
            Clear all
          </Button>

          <Tooltip title="Refresh Events">
            <IconButton
              size="small"
              onClick={handleRefresh}
              disabled={refreshing}
              sx={{
                bgcolor: "#FFFFFF",
                border: "1px solid #E2E8F0",
                color: "#64748B",
                p: 0.9,
                borderRadius: "8px",
                "&:hover": { bgcolor: "#F8FAFC", color: "#0F172A" },
              }}
            >
              <RefreshCw size={17} className={refreshing ? "spin" : ""} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Metrics Counters Cards */}
      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: { xs: "1fr 1fr", sm: "repeat(4, 1fr)" },
          gap: 2,
        }}
      >
        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: "12px",
            border: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            display: "flex",
            alignItems: "center",
            gap: 1.5,
          }}
        >
          <Box
            sx={{
              width: 42,
              height: 42,
              borderRadius: "10px",
              bgcolor: unreadEventsCount > 0 ? "rgba(239, 68, 68, 0.1)" : "#F1F5F9",
              color: unreadEventsCount > 0 ? "#EF4444" : "#64748B",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Bell size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.1 }}>
              {unreadEventsCount}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              Unread Events
            </Typography>
          </Box>
        </Paper>

        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: "12px",
            border: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            display: "flex",
            alignItems: "center",
            gap: 1.5,
          }}
        >
          <Box
            sx={{
              width: 42,
              height: 42,
              borderRadius: "10px",
              bgcolor: "rgba(225, 29, 72, 0.1)",
              color: "#E11D48",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <WifiOff size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.1 }}>
              {stats.offline}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              Node Offline
            </Typography>
          </Box>
        </Paper>

        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: "12px",
            border: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            display: "flex",
            alignItems: "center",
            gap: 1.5,
          }}
        >
          <Box
            sx={{
              width: 42,
              height: 42,
              borderRadius: "10px",
              bgcolor: "rgba(16, 185, 129, 0.1)",
              color: "#10B981",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Wifi size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.1 }}>
              {stats.online}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              Node Online
            </Typography>
          </Box>
        </Paper>

        <Paper
          elevation={0}
          sx={{
            p: 2,
            borderRadius: "12px",
            border: "1px solid #E2E8F0",
            bgcolor: "#FFFFFF",
            display: "flex",
            alignItems: "center",
            gap: 1.5,
          }}
        >
          <Box
            sx={{
              width: 42,
              height: 42,
              borderRadius: "10px",
              bgcolor: "rgba(245, 158, 11, 0.1)",
              color: "#F59E0B",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <AlertTriangle size={20} />
          </Box>
          <Box>
            <Typography variant="h6" sx={{ fontWeight: 800, color: "#0F172A", lineHeight: 1.1 }}>
              {stats.cronFailure}
            </Typography>
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
              Cron Failures
            </Typography>
          </Box>
        </Paper>
      </Box>

      {/* Filter and Search Bar */}
      <Paper
        elevation={0}
        sx={{
          p: 1.8,
          borderRadius: "12px",
          border: "1px solid #E2E8F0",
          bgcolor: "#FFFFFF",
          display: "flex",
          flexDirection: { xs: "column", md: "row" },
          alignItems: { xs: "stretch", md: "center" },
          justifyContent: "space-between",
          gap: 1.8,
        }}
      >
        {/* Search */}
        <TextField
          size="small"
          placeholder="Filter events by node, task, target, error..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search size={16} color="#94A3B8" />
              </InputAdornment>
            ),
          }}
          sx={{
            flex: 1,
            "& .MuiOutlinedInput-root": {
              borderRadius: "8px",
              fontSize: "0.85rem",
              bgcolor: "#F8FAFC",
              "& fieldset": { borderColor: "#E2E8F0" },
              "&:hover fieldset": { borderColor: "#CBD5E1" },
              "&.Mui-focused fieldset": { borderColor: "#4F46E5" },
            },
          }}
        />

        {/* Filter Badges */}
        <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
          <Chip
            clickable
            label="All Types"
            onClick={() => setSelectedType("all")}
            variant={selectedType === "all" ? "filled" : "outlined"}
            sx={{
              fontWeight: 600,
              fontSize: "0.78rem",
              borderRadius: "8px",
              bgcolor: selectedType === "all" ? "#0F172A" : "transparent",
              color: selectedType === "all" ? "#FFFFFF" : "#475569",
              borderColor: selectedType === "all" ? "#0F172A" : "#CBD5E1",
            }}
          />

          <Chip
            clickable
            icon={<WifiOff size={13} />}
            label="Offline"
            onClick={() => setSelectedType(selectedType === "node_offline" ? "all" : "node_offline")}
            variant={selectedType === "node_offline" ? "filled" : "outlined"}
            sx={{
              fontWeight: 600,
              fontSize: "0.78rem",
              borderRadius: "8px",
              bgcolor: selectedType === "node_offline" ? "#E11D48" : "transparent",
              color: selectedType === "node_offline" ? "#FFFFFF" : "#E11D48",
              borderColor: selectedType === "node_offline" ? "#E11D48" : "#FECDD3",
              "& .MuiChip-icon": { color: selectedType === "node_offline" ? "#FFFFFF" : "#E11D48" },
            }}
          />

          <Chip
            clickable
            icon={<Wifi size={13} />}
            label="Online"
            onClick={() => setSelectedType(selectedType === "node_online" ? "all" : "node_online")}
            variant={selectedType === "node_online" ? "filled" : "outlined"}
            sx={{
              fontWeight: 600,
              fontSize: "0.78rem",
              borderRadius: "8px",
              bgcolor: selectedType === "node_online" ? "#059669" : "transparent",
              color: selectedType === "node_online" ? "#FFFFFF" : "#059669",
              borderColor: selectedType === "node_online" ? "#059669" : "#A7F3D0",
              "& .MuiChip-icon": { color: selectedType === "node_online" ? "#FFFFFF" : "#059669" },
            }}
          />

          <Chip
            clickable
            icon={<AlertTriangle size={13} />}
            label="Cron Failures"
            onClick={() => setSelectedType(selectedType === "cron_task_failure" ? "all" : "cron_task_failure")}
            variant={selectedType === "cron_task_failure" ? "filled" : "outlined"}
            sx={{
              fontWeight: 600,
              fontSize: "0.78rem",
              borderRadius: "8px",
              bgcolor: selectedType === "cron_task_failure" ? "#D97706" : "transparent",
              color: selectedType === "cron_task_failure" ? "#FFFFFF" : "#D97706",
              borderColor: selectedType === "cron_task_failure" ? "#D97706" : "#FDE68A",
              "& .MuiChip-icon": { color: selectedType === "cron_task_failure" ? "#FFFFFF" : "#D97706" },
            }}
          />
          <Chip
            clickable
            label="Unread Only"
            onClick={() => setUnreadOnly((prev) => !prev)}
            variant={unreadOnly ? "filled" : "outlined"}
            sx={{
              fontWeight: 600,
              fontSize: "0.78rem",
              borderRadius: "8px",
              bgcolor: unreadOnly ? "#4F46E5" : "transparent",
              color: unreadOnly ? "#FFFFFF" : "#4F46E5",
              borderColor: unreadOnly ? "#4F46E5" : "#C7D2FE",
            }}
          />
        </Box>
      </Paper>

      {/* Events List */}
      {loading ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 8 }}>
          <CircularProgress size={36} sx={{ color: "#4F46E5" }} />
        </Box>
      ) : filteredEvents.length === 0 ? (
        <Paper
          elevation={0}
          sx={{
            py: 8,
            px: 3,
            borderRadius: "14px",
            border: "1px dashed #CBD5E1",
            bgcolor: "#FFFFFF",
            textAlign: "center",
          }}
        >
          <Box
            sx={{
              width: 56,
              height: 56,
              borderRadius: "50%",
              bgcolor: "#F1F5F9",
              color: "#94A3B8",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              margin: "0 auto 16px",
            }}
          >
            <CheckCircle2 size={30} />
          </Box>
          <Typography variant="h6" sx={{ fontWeight: 700, color: "#1E293B", mb: 0.5 }}>
            No Events Found
          </Typography>
          <Typography variant="body2" sx={{ color: "#64748B", maxWidth: 450, mx: "auto" }}>
            {searchQuery || selectedType !== "all" || unreadOnly
              ? "No events match the active search criteria or filters."
              : "All clear! Node status transitions and Looking Glass cron task alerts will appear here as they occur."}
          </Typography>
        </Paper>
      ) : (
        <Stack spacing={1.5}>
          {filteredEvents.map((event) => {
            const isUnread = !event.is_read;
            const meta = event.metadata || {};

            return (
              <Fade in key={event.id}>
                <Paper
                  elevation={0}
                  sx={{
                    p: { xs: 1.8, sm: 2.2 },
                    borderRadius: "12px",
                    border: "1px solid",
                    borderColor: isUnread ? "rgba(79, 70, 229, 0.35)" : "#E2E8F0",
                    bgcolor: isUnread ? "#FAF5FF" : "#FFFFFF",
                    boxShadow: isUnread ? "0 4px 14px rgba(79, 70, 229, 0.08)" : "0 1px 3px rgba(0, 0, 0, 0.02)",
                    transition: "all 0.15s ease",
                    "&:hover": {
                      borderColor: isUnread ? "#4F46E5" : "#CBD5E1",
                      boxShadow: "0 4px 12px rgba(0, 0, 0, 0.05)",
                    },
                    display: "flex",
                    flexDirection: "column",
                    gap: 1.2,
                  }}
                >
                  {/* Event Header Line */}
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      gap: 1.5,
                      flexWrap: "wrap",
                    }}
                  >
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1.2 }}>
                      {/* Unread indicator dot */}
                      {isUnread && (
                        <Box
                          sx={{
                            width: 8,
                            height: 8,
                            borderRadius: "50%",
                            bgcolor: "#4F46E5",
                            boxShadow: "0 0 6px rgba(79, 70, 229, 0.6)",
                            flexShrink: 0,
                          }}
                        />
                      )}

                      {/* Type Badge */}
                      {renderTypeBadge(event.type)}

                      {/* Affected Node Link */}
                      <Tooltip title={`Open details for node ${event.node_name}`}>
                        <Chip
                          icon={<Server size={13} color="#4F46E5" />}
                          label={event.node_name}
                          onClick={() => navigate(`/nodes/${encodeURIComponent(event.node_name)}`)}
                          size="small"
                          sx={{
                            cursor: "pointer",
                            bgcolor: "#EEF2FF",
                            color: "#4F46E5",
                            fontWeight: 700,
                            fontSize: "0.76rem",
                            borderRadius: "6px",
                            height: 24,
                            "&:hover": {
                              bgcolor: "#E0E7FF",
                              color: "#3730A3",
                            },
                          }}
                        />
                      </Tooltip>
                    </Box>

                    {/* Timestamp & Per-event Actions */}
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Tooltip title={formatExactDate(event.created_at)}>
                        <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, color: "#64748B" }}>
                          <Clock size={13} />
                          <Typography variant="caption" sx={{ fontWeight: 600, fontSize: "0.75rem" }}>
                            {formatRelativeTime(event.created_at)}
                          </Typography>
                        </Box>
                      </Tooltip>

                      {/* Mark Read Button */}
                      {isUnread && (
                        <Tooltip title="Mark as read">
                          <IconButton
                            size="small"
                            onClick={() => handleMarkAsRead(event.id)}
                            sx={{ color: "#4F46E5", p: 0.6, "&:hover": { bgcolor: "rgba(79, 70, 229, 0.08)" } }}
                          >
                            <Check size={16} />
                          </IconButton>
                        </Tooltip>
                      )}

                      {/* Delete Event Button */}
                      <Tooltip title="Delete event">
                        <IconButton
                          size="small"
                          onClick={() => handleDeleteEvent(event.id)}
                          sx={{ color: "#94A3B8", p: 0.6, "&:hover": { color: "#E11D48", bgcolor: "#FFF1F2" } }}
                        >
                          <Trash2 size={15} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  </Box>

                  {/* Event Title and Message */}
                  <Box>
                    <Typography
                      variant="subtitle2"
                      sx={{
                        fontWeight: 700,
                        color: "#0F172A",
                        fontSize: "0.92rem",
                        lineHeight: 1.3,
                      }}
                    >
                      {event.title}
                    </Typography>
                    <Typography
                      variant="body2"
                      sx={{
                        color: "#475569",
                        fontSize: "0.85rem",
                        mt: 0.3,
                        lineHeight: 1.45,
                      }}
                    >
                      {event.message}
                    </Typography>
                  </Box>

                  {/* Metadata Chips (if Cron Task Failure or specific details) */}
                  {event.type === "cron_task_failure" && (
                    <Box
                      sx={{
                        p: 1.2,
                        borderRadius: "8px",
                        bgcolor: isUnread ? "rgba(254, 243, 199, 0.4)" : "#F8FAFC",
                        border: "1px solid",
                        borderColor: isUnread ? "rgba(245, 158, 11, 0.25)" : "#E2E8F0",
                        display: "flex",
                        alignItems: "center",
                        gap: 1.5,
                        flexWrap: "wrap",
                        fontSize: "0.78rem",
                      }}
                    >
                      {meta.task_name && (
                        <Box sx={{ display: "flex", gap: 0.5, color: "#334155" }}>
                          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                            Task:
                          </Typography>
                          <Typography variant="caption" sx={{ fontWeight: 700 }}>
                            {String(meta.task_name)}
                          </Typography>
                        </Box>
                      )}

                      {meta.target && (
                        <Box sx={{ display: "flex", gap: 0.5, color: "#334155" }}>
                          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                            Target:
                          </Typography>
                          <Typography variant="caption" sx={{ fontWeight: 700, fontFamily: "monospace" }}>
                            {String(meta.target)}
                          </Typography>
                        </Box>
                      )}

                      {meta.task_type && (
                        <Box sx={{ display: "flex", gap: 0.5, color: "#334155" }}>
                          <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                            Type:
                          </Typography>
                          <Typography variant="caption" sx={{ fontWeight: 700, textTransform: "uppercase" }}>
                            {String(meta.task_type)}
                          </Typography>
                        </Box>
                      )}

                      <Box sx={{ display: "flex", gap: 0.5 }}>
                        <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 600 }}>
                          Packet Loss:
                        </Typography>
                        <Typography variant="caption" sx={{ fontWeight: 800, color: "#DC2626" }}>
                          {meta.packet_loss_pct !== undefined ? `${meta.packet_loss_pct}%` : "100%"}
                        </Typography>
                      </Box>

                      {meta.error && (
                        <Box sx={{ display: "flex", gap: 0.5, width: "100%" }}>
                          <Typography variant="caption" sx={{ color: "#DC2626", fontWeight: 600 }}>
                            Error:
                          </Typography>
                          <Typography variant="caption" sx={{ color: "#B91C1C", fontFamily: "monospace" }}>
                            {String(meta.error)}
                          </Typography>
                        </Box>
                      )}
                    </Box>
                  )}

                  {/* Navigation Footer for Node */}
                  <Box sx={{ display: "flex", justifyContent: "flex-end", pt: 0.2 }}>
                    <Button
                      size="small"
                      onClick={() => navigate(`/nodes/${encodeURIComponent(event.node_name)}`)}
                      endIcon={<ExternalLink size={13} />}
                      sx={{
                        textTransform: "none",
                        fontSize: "0.76rem",
                        fontWeight: 600,
                        color: "#4F46E5",
                        p: 0,
                        minWidth: 0,
                        "&:hover": { bgcolor: "transparent", textDecoration: "underline" },
                      }}
                    >
                      View node details
                    </Button>
                  </Box>
                </Paper>
              </Fade>
            );
          })}
        </Stack>
      )}

      {/* Clear All Confirmation Dialog */}
      <Dialog
        open={clearDialogOpen}
        onClose={() => setClearDialogOpen(false)}
        PaperProps={{ sx: { borderRadius: "14px", p: 1 } }}
      >
        <DialogTitle sx={{ fontWeight: 800, color: "#0F172A" }}>Clear all events?</DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ color: "#64748B" }}>
            This will permanently delete all recorded events from the database. This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button
            onClick={() => setClearDialogOpen(false)}
            sx={{ color: "#64748B", textTransform: "none", fontWeight: 600 }}
          >
            Cancel
          </Button>
          <Button
            variant="contained"
            color="error"
            onClick={handleClearAll}
            disabled={clearing}
            sx={{ textTransform: "none", fontWeight: 700, borderRadius: "8px" }}
          >
            {clearing ? "Clearing..." : "Yes, clear all"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
