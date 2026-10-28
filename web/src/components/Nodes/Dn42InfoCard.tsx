import React, { useState } from "react";
import { Box, Typography, IconButton, Tooltip, Chip, Paper } from "@mui/material";
import { Globe, Network, Compass, ExternalLink, Copy, Check } from "lucide-react";
import { getDn42ResourceLinks, Dn42ResourceLink } from "../../utils/dn42";

export interface Dn42InfoCardProps {
  asn: number | string;
  variant?: "drawer" | "page";
}

export const Dn42InfoCard: React.FC<Dn42InfoCardProps> = ({ asn, variant = "page" }) => {
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const links = getDn42ResourceLinks(asn);

  const handleCopy = (e: React.MouseEvent, key: string, text: string) => {
    e.stopPropagation();
    e.preventDefault();
    if (navigator?.clipboard?.writeText) {
      navigator.clipboard.writeText(text);
      setCopiedKey(key);
      setTimeout(() => setCopiedKey(null), 2000);
    }
  };

  const getLinkIcon = (id: string) => {
    switch (id) {
      case "official-registry":
        return <Globe size={15} color="#0284C7" />;
      case "iedon-map":
        return <Network size={15} color="#059669" />;
      case "nedifinita-explorer":
      default:
        return <Compass size={15} color="#7C3AED" />;
    }
  };

  const getIconBg = (id: string) => {
    switch (id) {
      case "official-registry":
        return "rgba(2, 132, 199, 0.1)";
      case "iedon-map":
        return "rgba(5, 150, 105, 0.1)";
      case "nedifinita-explorer":
      default:
        return "rgba(124, 58, 237, 0.1)";
    }
  };

  const isPage = variant === "page";
  const commonSx = {
    p: isPage ? 3 : 1.5,
    borderRadius: isPage ? 3 : 2,
    bgcolor: isPage ? "#FFFFFF" : "#F8FAFC",
    border: "1px solid #E2E8F0",
    position: "relative" as const,
    transition: "border-color 0.2s ease",
    "&:hover": {
      borderColor: "#CBD5E1",
    },
  };

  const cardContent = (
    <>
      {/* Header */}
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", mb: isPage ? 2 : 1.2 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              width: isPage ? 28 : 22,
              height: isPage ? 28 : 22,
              borderRadius: 1.5,
              background: "linear-gradient(135deg, rgba(79, 70, 229, 0.12) 0%, rgba(14, 165, 233, 0.12) 100%)",
              border: "1px solid rgba(79, 70, 229, 0.2)",
            }}
          >
            <Globe size={isPage ? 16 : 13} color="#4F46E5" />
          </Box>
          <Typography
            variant={isPage ? "subtitle2" : "caption"}
            sx={{
              fontWeight: 700,
              color: "#0F172A",
              display: "flex",
              alignItems: "center",
              gap: 0.8,
              letterSpacing: isPage ? "normal" : "0.5px",
            }}
          >
            DN42
          </Typography>
          <Chip
            label={`AS${asn}`}
            size="small"
            sx={{
              height: isPage ? 22 : 18,
              fontSize: isPage ? "0.72rem" : "0.65rem",
              fontWeight: 700,
              bgcolor: "rgba(79, 70, 229, 0.08)",
              color: "#4F46E5",
              border: "1px solid rgba(79, 70, 229, 0.2)",
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
            }}
          />
        </Box>

        <Chip
          label="External DN42 AS"
          size="small"
          sx={{
            height: isPage ? 20 : 18,
            fontSize: "0.65rem",
            fontWeight: 600,
            bgcolor: "#FEF3C7",
            color: "#92400E",
            border: "1px solid #FDE68A",
          }}
        />
      </Box>

      {/* Description / Subtitle */}
      <Typography
        variant="caption"
        sx={{
          color: "#64748B",
          display: "block",
          mb: 1.5,
          fontSize: isPage ? "0.78rem" : "0.7rem",
        }}
      >
        Quick query and lookup tools for Autonomous System <strong>AS{asn}</strong> in the DN42 network:
      </Typography>

      {/* Links List */}
      <Box sx={{ display: "flex", flexDirection: "column", gap: isPage ? 1.2 : 1 }}>
        {links.map((link: Dn42ResourceLink) => {
          const isCopied = copiedKey === link.id;

          return (
            <Box
              key={link.id}
              component="a"
              href={link.url}
              target="_blank"
              rel="noopener noreferrer"
              sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                p: isPage ? 1.5 : 1.2,
                borderRadius: 2,
                bgcolor: isPage ? "#F8FAFC" : "#FFFFFF",
                border: "1px solid #E2E8F0",
                textDecoration: "none",
                color: "inherit",
                transition: "all 0.18s cubic-bezier(0.4, 0, 0.2, 1)",
                cursor: "pointer",
                "&:hover": {
                  bgcolor: "#F1F5F9",
                  borderColor: "#CBD5E1",
                  transform: "translateY(-1px)",
                  boxShadow: "0 2px 6px rgba(15, 23, 42, 0.04)",
                  "& .ext-icon": {
                    transform: "translate(2px, -2px)",
                    color: "#4F46E5",
                  },
                  "& .link-title": {
                    color: "#4F46E5",
                  },
                },
              }}
            >
              <Box sx={{ display: "flex", alignItems: "center", gap: 1.2, minWidth: 0, pr: 1 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    width: isPage ? 32 : 28,
                    height: isPage ? 32 : 28,
                    borderRadius: 1.5,
                    bgcolor: getIconBg(link.id),
                    flexShrink: 0,
                  }}
                >
                  {getLinkIcon(link.id)}
                </Box>

                <Box sx={{ minWidth: 0 }}>
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.8, flexWrap: "wrap" }}>
                    <Typography
                      className="link-title"
                      variant="body2"
                      sx={{
                        fontWeight: 600,
                        fontSize: isPage ? "0.84rem" : "0.78rem",
                        color: "#1E293B",
                        transition: "color 0.15s ease",
                      }}
                    >
                      {link.name}
                    </Typography>
                    {link.provider && (
                      <Typography
                        variant="caption"
                        sx={{
                          fontSize: isPage ? "0.72rem" : "0.68rem",
                          color: "#64748B",
                          fontWeight: 500,
                        }}
                      >
                        ({link.provider})
                      </Typography>
                    )}
                  </Box>

                  <Typography
                    variant="caption"
                    className="mono-font"
                    sx={{
                      display: "block",
                      color: "#64748B",
                      fontSize: isPage ? "0.73rem" : "0.68rem",
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                      whiteSpace: "nowrap",
                      mt: 0.2,
                    }}
                  >
                    {link.url}
                  </Typography>
                </Box>
              </Box>

              <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, flexShrink: 0 }}>
                <Tooltip title={isCopied ? "Copied!" : "Copy URL"} arrow>
                  <IconButton
                    size="small"
                    onClick={(e) => handleCopy(e, link.id, link.url)}
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

                <Tooltip title="Open in new tab" arrow>
                  <Box
                    className="ext-icon"
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      color: "#94A3B8",
                      transition: "transform 0.18s ease, color 0.18s ease",
                      p: 0.5,
                    }}
                  >
                    <ExternalLink size={15} />
                  </Box>
                </Tooltip>
              </Box>
            </Box>
          );
        })}
      </Box>
    </>
  );

  return isPage ? (
    <Paper elevation={0} sx={commonSx}>
      {cardContent}
    </Paper>
  ) : (
    <Box sx={commonSx}>{cardContent}</Box>
  );
};
