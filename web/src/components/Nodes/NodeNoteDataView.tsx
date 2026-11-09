import React, { useMemo, useState } from "react";
import {
  Box,
  Chip,
  IconButton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from "@mui/material";
import { Braces, Check, Copy } from "lucide-react";
import { buildNoteDataEnvVars, parseNoteData } from "../../utils/taskSelector";

export interface NodeNoteDataViewProps {
  note?: string | null;
}

/**
 * Displays the structured data object parsed from the first ``` or ```json
 * fenced block in a node note, along with the NODE_DATA_* environment
 * variables recognized from it. Renders nothing when the note contains no
 * valid structured data object.
 */
export const NodeNoteDataView: React.FC<NodeNoteDataViewProps> = ({ note }) => {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);

  const noteData = useMemo(() => parseNoteData(note), [note]);

  const envEntries = useMemo(() => {
    if (!noteData) return [];
    return Object.entries(buildNoteDataEnvVars(noteData)).sort(([a], [b]) => a.localeCompare(b));
  }, [noteData]);

  const prettyJson = useMemo(() => (noteData ? JSON.stringify(noteData, null, 2) : ""), [noteData]);

  const handleCopy = (key: string, text: string) => {
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(text);
      setCopiedKey(key);
      setTimeout(() => setCopiedKey(null), 2000);
    }
  };

  if (!noteData) return null;

  return (
    <Box
      sx={{
        mt: 2,
        p: 2,
        borderRadius: 2,
        bgcolor: "#FFFFFF",
        border: "1px solid #E2E8F0",
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Braces size={16} color="#4F46E5" />
        <Typography variant="subtitle2" sx={{ fontWeight: 700, color: "#0F172A" }}>
          Structured Node Data
        </Typography>
        <Chip
          label="NODE_DATA_*"
          size="small"
          className="mono-font"
          sx={{
            height: 20,
            fontSize: "0.68rem",
            fontWeight: 700,
            bgcolor: "rgba(79, 70, 229, 0.08)",
            color: "#4F46E5",
            border: "1px solid rgba(79, 70, 229, 0.2)",
          }}
        />
        <Chip
          label={`${envEntries.length} variable${envEntries.length === 1 ? "" : "s"}`}
          size="small"
          sx={{
            height: 20,
            fontSize: "0.68rem",
            fontWeight: 600,
            bgcolor: "#F1F5F9",
            color: "#475569",
            border: "1px solid #E2E8F0",
          }}
        />
      </Box>

      <Typography
        variant="caption"
        sx={{
          display: "block",
          color: "#64748B",
          mt: 0.75,
          fontSize: "0.75rem",
        }}
      >
        Parsed from the first fenced block in this note; exposed to wg/bird/nft templates as{" "}
        <code>{"{{ .data }}"}</code> object variable; exposed to Device Helper tasks & Apps as{" "}
        <Box component="span" className="mono-font" sx={{ color: "#334155", fontWeight: 600 }}>
          NODE_DATA_*
        </Box>{" "}
        environment variables.
      </Typography>

      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 2, mt: 1.5 }}>
        {/* Parsed structured data object */}
        <Box sx={{ flex: "1 1 300px", minWidth: 0 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              mb: 0.75,
            }}
          >
            <Typography variant="caption" sx={{ color: "#64748B", fontWeight: 700 }}>
              PARSED DATA
            </Typography>
            <Tooltip title={copiedKey === "__json__" ? "Copied!" : "Copy parsed JSON"} arrow>
              <IconButton
                size="small"
                onClick={() => handleCopy("__json__", prettyJson)}
                sx={{
                  p: 0.5,
                  color: copiedKey === "__json__" ? "#059669" : "#94A3B8",
                  "&:hover": {
                    color: "#475569",
                    bgcolor: "rgba(0, 0, 0, 0.04)",
                  },
                }}
              >
                {copiedKey === "__json__" ? <Check size={14} color="#059669" /> : <Copy size={14} />}
              </IconButton>
            </Tooltip>
          </Box>
          <Box
            component="pre"
            className="mono-font"
            sx={{
              m: 0,
              p: 1.5,
              borderRadius: 1.5,
              bgcolor: "#0F172A",
              color: "#F8FAFC",
              fontSize: "0.76rem",
              lineHeight: 1.6,
              overflow: "auto",
              maxHeight: 320,
              whiteSpace: "pre",
            }}
          >
            {prettyJson}
          </Box>
        </Box>

        {/* Recognized NODE_DATA_* environment variables */}
        <Box sx={{ flex: "1 1 300px", minWidth: 0 }}>
          <Typography
            variant="caption"
            sx={{
              display: "block",
              color: "#64748B",
              fontWeight: 700,
              mb: 0.75,
            }}
          >
            NODE_DATA_* VARIABLES
          </Typography>
          {envEntries.length > 0 ? (
            <TableContainer
              sx={{
                border: "1px solid #E2E8F0",
                borderRadius: 1.5,
                maxHeight: 320,
                overflow: "auto",
              }}
            >
              <Table size="small" stickyHeader>
                <TableHead>
                  <TableRow>
                    <TableCell
                      sx={{
                        fontWeight: 700,
                        color: "#334155",
                        bgcolor: "#F1F5F9",
                        fontSize: "0.72rem",
                      }}
                    >
                      Variable
                    </TableCell>
                    <TableCell
                      sx={{
                        fontWeight: 700,
                        color: "#334155",
                        bgcolor: "#F1F5F9",
                        fontSize: "0.72rem",
                      }}
                    >
                      Value
                    </TableCell>
                    <TableCell sx={{ width: 40, bgcolor: "#F1F5F9" }} />
                  </TableRow>
                </TableHead>
                <TableBody>
                  {envEntries.map(([key, value]) => {
                    const isCopied = copiedKey === key;
                    const assignment = `${key}=${value}`;
                    return (
                      <TableRow
                        key={key}
                        sx={{
                          "&:last-child td, &:last-child th": {
                            borderBottom: 0,
                          },
                        }}
                      >
                        <TableCell
                          className="mono-font"
                          sx={{
                            color: "#4F46E5",
                            fontWeight: 600,
                            fontSize: "0.74rem",
                            wordBreak: "break-all",
                          }}
                        >
                          {key}
                        </TableCell>
                        <TableCell
                          className="mono-font"
                          sx={{
                            color: "#0F172A",
                            fontSize: "0.74rem",
                            wordBreak: "break-word",
                          }}
                        >
                          {value === "" ? '""' : value}
                        </TableCell>
                        <TableCell sx={{ p: 0.5 }}>
                          <Tooltip title={isCopied ? "Copied!" : `Copy ${assignment}`} arrow>
                            <IconButton
                              size="small"
                              onClick={() => handleCopy(key, assignment)}
                              sx={{
                                p: 0.5,
                                color: isCopied ? "#059669" : "#94A3B8",
                                "&:hover": {
                                  color: "#475569",
                                  bgcolor: "rgba(0, 0, 0, 0.04)",
                                },
                              }}
                            >
                              {isCopied ? <Check size={14} color="#059669" /> : <Copy size={14} />}
                            </IconButton>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </TableContainer>
          ) : (
            <Box
              sx={{
                p: 1.5,
                borderRadius: 1.5,
                bgcolor: "#F8FAFC",
                border: "1px dashed #CBD5E1",
                color: "#64748B",
                fontSize: "0.76rem",
                fontStyle: "italic",
              }}
            >
              No NODE_DATA_* variables recognized from this data — only string, number, and boolean fields (up to one
              nested object level) are exposed.
            </Box>
          )}
        </Box>
      </Box>
    </Box>
  );
};
