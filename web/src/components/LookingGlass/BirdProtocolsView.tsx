import React, { useState, useMemo } from "react";
import {
  Box,
  Typography,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  TextField,
  InputAdornment,
} from "@mui/material";
import { Search, CheckCircle2, AlertTriangle, XCircle, Activity, ShieldCheck } from "lucide-react";
import { BirdProtocolEntry } from "../../types/api";

interface BirdProtocolsViewProps {
  protocols: BirdProtocolEntry[];
}

export const BirdProtocolsView: React.FC<BirdProtocolsViewProps> = ({ protocols }) => {
  const [filter, setFilter] = useState("");

  const filtered = useMemo(() => {
    if (!filter.trim()) return protocols;
    const q = filter.toLowerCase();
    return protocols.filter(
      (p) =>
        p.name.toLowerCase().includes(q) ||
        p.proto.toLowerCase().includes(q) ||
        p.table.toLowerCase().includes(q) ||
        p.state.toLowerCase().includes(q) ||
        p.info.toLowerCase().includes(q)
    );
  }, [protocols, filter]);

  const stats = useMemo(() => {
    const total = protocols.length;
    const up = protocols.filter((p) => p.connected).length;
    const bgpTotal = protocols.filter((p) => p.proto.toUpperCase() === "BGP").length;
    const bgpUp = protocols.filter(
      (p) => p.proto.toUpperCase() === "BGP" && p.connected
    ).length;
    return { total, up, bgpTotal, bgpUp };
  }, [protocols]);

  const getProtoColor = (proto: string) => {
    switch (proto.toUpperCase()) {
      case "BGP":
        return { bg: "#EEF2FF", text: "#4F46E5", border: "#C7D2FE" };
      case "KERNEL":
        return { bg: "#EFF6FF", text: "#2563EB", border: "#BFDBFE" };
      case "DIRECT":
        return { bg: "#F0FDFA", text: "#0D9488", border: "#99F6E4" };
      case "DEVICE":
        return { bg: "#F8FAFC", text: "#475569", border: "#E2E8F0" };
      case "OSPF":
        return { bg: "#FFFBEB", text: "#D97706", border: "#FDE68A" };
      case "STATIC":
        return { bg: "#FAF5FF", text: "#9333EA", border: "#E9D5FF" };
      default:
        return { bg: "#F1F5F9", text: "#334155", border: "#CBD5E1" };
    }
  };

  const getStateBadge = (p: BirdProtocolEntry) => {
    if (p.connected) {
      return (
        <Chip
          icon={<CheckCircle2 size={12} color="#10B981" />}
          label={p.state.toUpperCase()}
          size="small"
          sx={{
            backgroundColor: "#ECFDF5",
            color: "#065F46",
            border: "1px solid #A7F3D0",
            fontWeight: 600,
            fontSize: "0.72rem",
          }}
        />
      );
    }
    if (p.state.toLowerCase() === "start" || p.state.toLowerCase() === "connect") {
      return (
        <Chip
          icon={<AlertTriangle size={12} color="#F59E0B" />}
          label={p.state.toUpperCase()}
          size="small"
          sx={{
            backgroundColor: "#FFFBEB",
            color: "#92400E",
            border: "1px solid #FDE68A",
            fontWeight: 600,
            fontSize: "0.72rem",
          }}
        />
      );
    }
    return (
      <Chip
        icon={<XCircle size={12} color="#EF4444" />}
        label={p.state.toUpperCase()}
        size="small"
        sx={{
          backgroundColor: "#FEF2F2",
          color: "#991B1B",
          border: "1px solid #FECACA",
          fontWeight: 600,
          fontSize: "0.72rem",
        }}
      />
    );
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      {/* Quick Stats Bar */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          flexWrap: "wrap",
          gap: 1.5,
          p: 1.5,
          borderRadius: 2,
          backgroundColor: "#F8FAFC",
          border: "1px solid #E2E8F0",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 2 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Activity size={16} color="#4F46E5" />
            <Typography variant="body2" sx={{ fontWeight: 600, color: "#1E293B" }}>
              Total Protocols: {stats.total}
            </Typography>
          </Box>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <ShieldCheck size={16} color="#10B981" />
            <Typography variant="body2" sx={{ color: "#065F46", fontWeight: 600 }}>
              Up: {stats.up} / {stats.total}
            </Typography>
          </Box>
          {stats.bgpTotal > 0 && (
            <Chip
              label={`BGP Sessions: ${stats.bgpUp}/${stats.bgpTotal} Established`}
              size="small"
              sx={{
                backgroundColor: stats.bgpUp === stats.bgpTotal ? "#ECFDF5" : "#FFFBEB",
                color: stats.bgpUp === stats.bgpTotal ? "#065F46" : "#92400E",
                fontWeight: 600,
                fontSize: "0.72rem",
              }}
            />
          )}
        </Box>

        <TextField
          size="small"
          placeholder="Filter protocols..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search size={14} color="#94A3B8" />
              </InputAdornment>
            ),
          }}
          sx={{
            width: { xs: "100%", sm: 220 },
            "& .MuiOutlinedInput-root": {
              backgroundColor: "#FFFFFF",
              fontSize: "0.825rem",
            },
          }}
        />
      </Box>

      {/* Protocols Table */}
      <TableContainer component={Paper} elevation={0} sx={{ border: "1px solid #E2E8F0", borderRadius: 2 }}>
        <Table size="small">
          <TableHead sx={{ backgroundColor: "#F1F5F9" }}>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>NAME</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>TYPE</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>TABLE</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>STATE</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>SINCE</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569", fontSize: "0.75rem" }}>INFO / STATUS</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} align="center" sx={{ py: 3, color: "#94A3B8" }}>
                  No protocols match the filter
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((p, idx) => {
                const colors = getProtoColor(p.proto);
                return (
                  <TableRow
                    key={`${p.name}-${idx}`}
                    sx={{
                      "&:hover": { backgroundColor: "#F8FAFC" },
                      backgroundColor: idx % 2 === 0 ? "#FFFFFF" : "#FAFBFD",
                    }}
                  >
                    <TableCell sx={{ fontWeight: 600, color: "#0F172A", fontFamily: "monospace" }}>
                      {p.name}
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={p.proto}
                        size="small"
                        sx={{
                          backgroundColor: colors.bg,
                          color: colors.text,
                          border: `1px solid ${colors.border}`,
                          fontWeight: 600,
                          fontSize: "0.7rem",
                          height: 22,
                        }}
                      />
                    </TableCell>
                    <TableCell sx={{ color: "#475569", fontFamily: "monospace", fontSize: "0.8rem" }}>
                      {p.table}
                    </TableCell>
                    <TableCell>{getStateBadge(p)}</TableCell>
                    <TableCell sx={{ color: "#64748B", fontSize: "0.8rem", whiteSpace: "nowrap" }}>
                      {p.since || "-"}
                    </TableCell>
                    <TableCell sx={{ color: "#1E293B", fontSize: "0.8rem", maxWidth: 300 }}>
                      <Typography variant="body2" sx={{ fontSize: "0.8rem", wordBreak: "break-word" }}>
                        {p.info || "-"}
                      </Typography>
                    </TableCell>
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};
