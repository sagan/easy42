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
} from "@mui/material";
import { TracerouteResult, MTRResult } from "../../types/api";

interface TracerouteViewProps {
  traceResult?: TracerouteResult;
  mtrResult?: MTRResult;
}

export const TracerouteView: React.FC<TracerouteViewProps> = ({
  traceResult,
  mtrResult,
}) => {
  if (mtrResult && mtrResult.hops && mtrResult.hops.length > 0) {
    return (
      <TableContainer component={Paper} elevation={0} sx={{ border: "1px solid #E2E8F0", borderRadius: 2 }}>
        <Table size="small">
          <TableHead sx={{ backgroundColor: "#F1F5F9" }}>
            <TableRow>
              <TableCell sx={{ fontWeight: 600, color: "#475569", width: 60 }}>HOP</TableCell>
              <TableCell sx={{ fontWeight: 600, color: "#475569" }}>HOST / IP</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>LOSS %</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>SENT</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>LAST</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>AVG</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>BEST</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>WRST</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600, color: "#475569" }}>STDEV</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {mtrResult.hops.map((hop) => {
              const hasLoss = hop.loss_pct > 0;
              return (
                <TableRow
                  key={hop.hop}
                  sx={{
                    "&:hover": { backgroundColor: "#F8FAFC" },
                    backgroundColor: hop.loss_pct >= 50 ? "rgba(239, 68, 68, 0.05)" : "inherit",
                  }}
                >
                  <TableCell sx={{ fontWeight: 700, fontFamily: "monospace", color: "#64748B" }}>
                    {hop.hop}
                  </TableCell>
                  <TableCell sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A" }}>
                    {hop.host}
                  </TableCell>
                  <TableCell align="right">
                    <Chip
                      label={`${hop.loss_pct.toFixed(1)}%`}
                      size="small"
                      sx={{
                        backgroundColor: !hasLoss ? "#ECFDF5" : hop.loss_pct < 20 ? "#FFFBEB" : "#FEF2F2",
                        color: !hasLoss ? "#065F46" : hop.loss_pct < 20 ? "#92400E" : "#991B1B",
                        fontWeight: 600,
                        fontSize: "0.7rem",
                        height: 20,
                      }}
                    />
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                    {hop.sent}
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}>
                    {hop.last_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontWeight: 600, fontSize: "0.8rem", color: "#4F46E5" }}>
                    {hop.avg_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#10B981" }}>
                    {hop.best_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#F59E0B" }}>
                    {hop.worst_ms.toFixed(1)} ms
                  </TableCell>
                  <TableCell align="right" sx={{ fontFamily: "monospace", fontSize: "0.8rem", color: "#64748B" }}>
                    {hop.stdev_ms.toFixed(1)}
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </TableContainer>
    );
  }

  const hops = traceResult?.hops || [];

  return (
    <TableContainer component={Paper} elevation={0} sx={{ border: "1px solid #E2E8F0", borderRadius: 2 }}>
      <Table size="small">
        <TableHead sx={{ backgroundColor: "#F1F5F9" }}>
          <TableRow>
            <TableCell sx={{ fontWeight: 600, color: "#475569", width: 60 }}>HOP</TableCell>
            <TableCell sx={{ fontWeight: 600, color: "#475569" }}>HOST / IP</TableCell>
            <TableCell sx={{ fontWeight: 600, color: "#475569" }}>PROBES / LATENCY</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {hops.length === 0 ? (
            <TableRow>
              <TableCell colSpan={3} align="center" sx={{ py: 3, color: "#94A3B8" }}>
                No hops recorded
              </TableCell>
            </TableRow>
          ) : (
            hops.map((hop) => (
              <TableRow key={hop.hop} sx={{ "&:hover": { backgroundColor: "#F8FAFC" } }}>
                <TableCell sx={{ fontWeight: 700, fontFamily: "monospace", color: "#64748B" }}>
                  {hop.hop}
                </TableCell>
                <TableCell>
                  {hop.timed_out ? (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography sx={{ fontFamily: "monospace", color: "#94A3B8", fontStyle: "italic" }}>
                        * * * (Request timed out)
                      </Typography>
                    </Box>
                  ) : (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                      <Typography sx={{ fontFamily: "monospace", fontWeight: 600, color: "#0F172A" }}>
                        {hop.host}
                      </Typography>
                      {hop.ip && hop.ip !== hop.host && (
                        <Typography variant="caption" sx={{ fontFamily: "monospace", color: "#64748B" }}>
                          ({hop.ip})
                        </Typography>
                      )}
                    </Box>
                  )}
                </TableCell>
                <TableCell>
                  {hop.timed_out ? (
                    <Chip
                      label="Timeout"
                      size="small"
                      sx={{
                        backgroundColor: "#FEF2F2",
                        color: "#991B1B",
                        fontSize: "0.7rem",
                        height: 20,
                      }}
                    />
                  ) : (
                    <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
                      {hop.times_ms.map((t, tIdx) => (
                        <Chip
                          key={tIdx}
                          label={`${t.toFixed(2)} ms`}
                          size="small"
                          sx={{
                            backgroundColor:
                              t < 25 ? "#ECFDF5" : t < 70 ? "#FFFBEB" : "#F5F3FF",
                            color:
                              t < 25 ? "#065F46" : t < 70 ? "#92400E" : "#6D28D9",
                            border:
                              t < 25 ? "1px solid #A7F3D0" : t < 70 ? "1px solid #FDE68A" : "1px solid #DDD6FE",
                            fontFamily: "monospace",
                            fontWeight: 600,
                            fontSize: "0.72rem",
                            height: 22,
                          }}
                        />
                      ))}
                    </Box>
                  )}
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </TableContainer>
  );
};
