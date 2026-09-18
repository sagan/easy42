import React, { useMemo } from "react";
import { Box, SxProps, Theme } from "@mui/material";
import { marked } from "marked";
import DOMPurify from "dompurify";

interface MarkdownViewProps {
  content?: string | null;
  emptyText?: string;
  sx?: SxProps<Theme>;
}

// Custom renderer to ensure safe external links with target="_blank"
const renderer = new marked.Renderer();
renderer.link = ({ href, title, text }) => {
  const cleanHref = href || "#";
  const titleAttr = title ? ` title="${title}"` : "";
  return `<a href="${cleanHref}"${titleAttr} target="_blank" rel="noopener noreferrer">${text}</a>`;
};

export const MarkdownView: React.FC<MarkdownViewProps> = ({
  content,
  emptyText = "No note recorded",
  sx,
}) => {
  const renderedHtml = useMemo(() => {
    if (!content || !content.trim()) return "";
    try {
      const rawHtml = marked.parse(content, {
        renderer,
        breaks: true,
        gfm: true,
      }) as string;
      return DOMPurify.sanitize(rawHtml, {
        ADD_ATTR: ["target", "rel"],
      });
    } catch (err) {
      console.error("Failed to render markdown:", err);
      return DOMPurify.sanitize(content);
    }
  }, [content]);

  if (!content || !content.trim()) {
    return (
      <Box
        sx={{
          color: "#94A3B8",
          fontSize: "0.8rem",
          fontStyle: "italic",
          py: 0.5,
          ...sx,
        }}
      >
        {emptyText}
      </Box>
    );
  }

  return (
    <Box
      sx={{
        color: "#1E293B",
        fontSize: "0.825rem",
        lineHeight: 1.6,
        wordBreak: "break-word",
        "& > *:first-of-type": { mt: 0 },
        "& > *:last-of-type": { mb: 0 },
        "& h1, & h2, & h3, & h4, & h5, & h6": {
          color: "#0F172A",
          fontWeight: 700,
          mt: 1.5,
          mb: 0.75,
          lineHeight: 1.3,
        },
        "& h1": { fontSize: "1.15rem", borderBottom: "1px solid #E2E8F0", pb: 0.5 },
        "& h2": { fontSize: "1.025rem", borderBottom: "1px solid #F1F5F9", pb: 0.25 },
        "& h3": { fontSize: "0.925rem" },
        "& h4": { fontSize: "0.85rem" },
        "& p": {
          mb: 1,
          "&:last-child": { mb: 0 },
        },
        "& a": {
          color: "#4F46E5",
          textDecoration: "underline",
          textUnderlineOffset: "2px",
          fontWeight: 500,
          "&:hover": { color: "#3730A3" },
        },
        "& ul, & ol": {
          pl: 2.5,
          mb: 1,
          "& li": {
            mb: 0.25,
          },
        },
        "& blockquote": {
          borderLeft: "3px solid #6366F1",
          backgroundColor: "#F8FAFC",
          color: "#475569",
          m: "8px 0",
          p: "6px 12px",
          borderRadius: "0 6px 6px 0",
          fontStyle: "italic",
        },
        "& code": {
          fontFamily: "'JetBrains Mono', 'Fira Code', ui-monospace, monospace",
          fontSize: "0.76rem",
          backgroundColor: "rgba(15, 23, 42, 0.06)",
          color: "#0F172A",
          px: 0.6,
          py: 0.2,
          borderRadius: 1,
          border: "1px solid rgba(15, 23, 42, 0.08)",
        },
        "& pre": {
          backgroundColor: "#0F172A",
          color: "#F8FAFC",
          p: 1.5,
          borderRadius: 1.5,
          overflowX: "auto",
          my: 1,
          "& code": {
            backgroundColor: "transparent",
            color: "inherit",
            border: "none",
            p: 0,
            fontSize: "0.76rem",
          },
        },
        "& table": {
          width: "100%",
          borderCollapse: "collapse",
          my: 1,
          fontSize: "0.775rem",
        },
        "& th, & td": {
          border: "1px solid #CBD5E1",
          p: "5px 8px",
          textAlign: "left",
        },
        "& th": {
          backgroundColor: "#F1F5F9",
          fontWeight: 600,
          color: "#334155",
        },
        "& tr:nth-of-type(even)": {
          backgroundColor: "#F8FAFC",
        },
        "& hr": {
          border: "none",
          borderTop: "1px solid #E2E8F0",
          my: 1.5,
        },
        ...sx,
      }}
      dangerouslySetInnerHTML={{ __html: renderedHtml }}
    />
  );
};
