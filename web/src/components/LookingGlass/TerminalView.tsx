import React, { useState } from "react";
import { Box, IconButton, Tooltip, Typography } from "@mui/material";
import { Copy, Check, Download } from "lucide-react";

interface TerminalViewProps {
  output: string;
  command?: string;
  durationMs?: number;
  exitCode?: number;
}

// Simple ANSI color code to CSS parser
function renderAnsi(text: string): React.ReactNode[] {
  // Regex to match ANSI escape sequences: \x1b[...m or \033[...m
  const ansiRegex = /\x1b\[([0-9;]*)m/g;
  const parts: React.ReactNode[] = [];
  let lastIndex = 0;
  let currentColor = "#E2E8F0"; // default light slate
  let isBold = false;

  const colorMap: Record<string, string> = {
    "30": "#64748B", // black/dim
    "31": "#F87171", // red
    "32": "#4ADE80", // green
    "33": "#FBBF24", // yellow
    "34": "#60A5FA", // blue
    "35": "#C084FC", // magenta
    "36": "#38BDF8", // cyan
    "37": "#F1F5F9", // white
    "90": "#94A3B8", // bright black
    "91": "#FCA5A5", // bright red
    "92": "#86EFAC", // bright green
    "93": "#FDE047", // bright yellow
    "94": "#93C5FD", // bright blue
    "95": "#E9D5FF", // bright magenta
    "96": "#7DD3FC", // bright cyan
    "97": "#FFFFFF", // bright white
  };

  let match: RegExpExecArray | null;
  let keyIndex = 0;

  while ((match = ansiRegex.exec(text)) !== null) {
    const chunk = text.substring(lastIndex, match.index);
    if (chunk) {
      parts.push(
        <span
          key={keyIndex++}
          style={{
            color: currentColor,
            fontWeight: isBold ? 600 : 400,
          }}
        >
          {chunk}
        </span>
      );
    }

    const code = match[1];
    if (!code || code === "0") {
      currentColor = "#E2E8F0";
      isBold = false;
    } else {
      const codes = code.split(";");
      for (const c of codes) {
        if (c === "1") isBold = true;
        else if (colorMap[c]) currentColor = colorMap[c];
      }
    }
    lastIndex = ansiRegex.lastIndex;
  }

  const remaining = text.substring(lastIndex);
  if (remaining) {
    parts.push(
      <span
        key={keyIndex++}
        style={{
          color: currentColor,
          fontWeight: isBold ? 600 : 400,
        }}
      >
        {remaining}
      </span>
    );
  }

  return parts;
}

export const TerminalView: React.FC<TerminalViewProps> = ({
  output,
  command,
  durationMs,
  exitCode,
}) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(output);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownload = () => {
    const blob = new Blob([output], { type: "text/plain;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `looking-glass-output-${Date.now()}.txt`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  return (
    <Box
      sx={{
        position: "relative",
        backgroundColor: "#0F172A",
        borderRadius: 2,
        overflow: "hidden",
        border: "1px solid #334155",
        boxShadow: "0 4px 20px rgba(0,0,0,0.3)",
      }}
    >
      {/* Terminal Titlebar */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          px: 2,
          py: 1,
          backgroundColor: "#1E293B",
          borderBottom: "1px solid #334155",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", backgroundColor: "#EF4444" }} />
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", backgroundColor: "#F59E0B" }} />
          <Box sx={{ width: 10, height: 10, borderRadius: "50%", backgroundColor: "#10B981" }} />
          {command && (
            <Typography
              variant="caption"
              sx={{
                ml: 1.5,
                fontFamily: "monospace",
                color: "#94A3B8",
                maxWidth: 450,
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
            >
              $ {command}
            </Typography>
          )}
        </Box>

        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          {durationMs !== undefined && (
            <Typography variant="caption" sx={{ color: "#64748B", fontFamily: "monospace" }}>
              {durationMs}ms
            </Typography>
          )}
          {exitCode !== undefined && (
            <Typography
              variant="caption"
              sx={{
                fontFamily: "monospace",
                px: 0.8,
                py: 0.2,
                borderRadius: 1,
                backgroundColor: exitCode === 0 ? "rgba(16, 185, 129, 0.15)" : "rgba(239, 68, 68, 0.15)",
                color: exitCode === 0 ? "#34D399" : "#F87171",
                fontSize: "0.7rem",
                fontWeight: 600,
              }}
            >
              exit: {exitCode}
            </Typography>
          )}
          <Tooltip title={copied ? "Copied!" : "Copy raw output"}>
            <IconButton size="small" onClick={handleCopy} sx={{ color: "#94A3B8", "&:hover": { color: "#F8FAFC" } }}>
              {copied ? <Check size={14} color="#34D399" /> : <Copy size={14} />}
            </IconButton>
          </Tooltip>
          <Tooltip title="Download as text file">
            <IconButton size="small" onClick={handleDownload} sx={{ color: "#94A3B8", "&:hover": { color: "#F8FAFC" } }}>
              <Download size={14} />
            </IconButton>
          </Tooltip>
        </Box>
      </Box>

      {/* Terminal Screen */}
      <Box
        sx={{
          p: 2,
          maxHeight: 480,
          overflowY: "auto",
          fontFamily: "'JetBrains Mono', 'Fira Code', 'Courier New', monospace",
          fontSize: "0.825rem",
          lineHeight: 1.6,
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
          color: "#E2E8F0",
        }}
      >
        {output ? renderAnsi(output) : <Typography sx={{ color: "#64748B", fontStyle: "italic" }}>No output returned</Typography>}
      </Box>
    </Box>
  );
};
