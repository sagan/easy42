import { PingTableTask, Node } from "../types/api";

export type PingTableNodeNameSource = "name" | "caption";
export type PingTableNodeFilter = "all" | "external" | "tag";
export type PingTableExportFormat = "svg" | "png";
export type PingTableExportAction = "download" | "new_tab";

export interface PingTableExportOptions {
  diagramTitle?: string;
  nodeNameSource?: PingTableNodeNameSource;
  nodeFilter?: PingTableNodeFilter;
  filterTag?: string;
  networkName?: string;
  nodes?: Node[];
}

function escapeXml(unsafe: string): string {
  return unsafe
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

export function generatePingTableSvg(
  task: PingTableTask,
  networkNameOrOptions?: string | PingTableExportOptions,
): string {
  const options: PingTableExportOptions =
    typeof networkNameOrOptions === "string"
      ? { networkName: networkNameOrOptions }
      : networkNameOrOptions || {};

  const rawNodes = task.nodes || [];
  const nodeMap = new Map<string, Node>((options.nodes || []).map((n) => [n.name, n]));

  let selectedNodes = rawNodes;
  if (options.nodeFilter === "external") {
    selectedNodes = selectedNodes.filter((name) => {
      const n = nodeMap.get(name);
      return Boolean(n?.external_ip?.trim() || n?.external_ip6?.trim());
    });
  } else if (options.nodeFilter === "tag" && options.filterTag) {
    selectedNodes = selectedNodes.filter((name) => {
      const n = nodeMap.get(name);
      return Boolean(n?.tags && n.tags.includes(options.filterTag!));
    });
  }

  const netTitle = options.networkName?.trim() || "Easy42";
  const typeTitle = task.latency_type === "link" ? "Direct Link Latency" : "End-to-End Latency";
  const defaultTitle = `${netTitle} Ping Table — ${typeTitle}`;
  const diagramTitle = options.diagramTitle?.trim() || defaultTitle;
  const dateStr = task.created_at ? new Date(task.created_at * 1000).toLocaleString() : "";
  const statusStr = task.status ? task.status.toUpperCase() : "COMPLETED";

  const getNodeLabel = (name: string): string => {
    if (options.nodeNameSource === "caption") {
      const n = nodeMap.get(name);
      const cap = n?.caption?.trim() || "";
      return cap && !cap.startsWith("!") ? cap : "***";
    }
    return name;
  };

  if (selectedNodes.length === 0) {
    const width = 640;
    const height = 240;
    return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <style>
    .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 18px; font-weight: 700; fill: #0F172A; }
    .msg { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 14px; fill: #64748B; text-anchor: middle; dominant-baseline: middle; }
  </style>
  <rect width="${width}" height="${height}" fill="#FFFFFF" rx="8" />
  <rect x="1" y="1" width="${width - 2}" height="${height - 2}" fill="none" stroke="#E2E8F0" stroke-width="2" rx="8" />
  <text x="28" y="48" class="title">${escapeXml(diagramTitle)}</text>
  <text x="${width / 2}" y="${height / 2 + 10}" class="msg">No nodes match the selected filter</text>
</svg>`;
  }

  const n = selectedNodes.length;

  let maxLabelLen = 0;
  for (const name of selectedNodes) {
    const lbl = getNodeLabel(name);
    if (lbl.length > maxLabelLen) {
      maxLabelLen = lbl.length;
    }
  }

  const colWidth = Math.max(110, Math.min(220, maxLabelLen * 8.5 + 24));
  const rowHeight = 38;
  const headerColWidth = Math.max(130, Math.min(240, maxLabelLen * 8.5 + 28));
  const topHeaderHeight = 80;
  const margin = 28;
  const legendHeight = 60;

  const width = margin * 2 + headerColWidth + n * colWidth;
  const height = margin * 2 + topHeaderHeight + (n + 1) * rowHeight + legendHeight;

  let svg = `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
  <style>
    .title { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 20px; font-weight: 700; fill: #0F172A; }
    .subtitle { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 12px; fill: #64748B; }
    .col-header { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; font-weight: 600; fill: #1E293B; text-anchor: middle; dominant-baseline: middle; }
    .row-header { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; font-weight: 600; fill: #1E293B; dominant-baseline: middle; }
    .cell-val { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, monospace; font-size: 12px; font-weight: 600; text-anchor: middle; dominant-baseline: middle; }
    .legend-text { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 11px; fill: #475569; dominant-baseline: middle; }
  </style>

  <!-- Background -->
  <rect width="${width}" height="${height}" fill="#FFFFFF" rx="8" />
  <rect x="1" y="1" width="${width - 2}" height="${height - 2}" fill="none" stroke="#E2E8F0" stroke-width="2" rx="8" />

  <!-- Header Section -->
  <text x="${margin}" y="${margin + 20}" class="title">${escapeXml(diagramTitle)}</text>
  <text x="${margin}" y="${margin + 42}" class="subtitle">Measured: ${escapeXml(dateStr)} | Included Nodes: ${n} | Status: ${escapeXml(statusStr)}</text>
`;

  const gridStartX = margin;
  const gridStartY = margin + topHeaderHeight;

  // Header row background
  svg += `  <rect x="${gridStartX}" y="${gridStartY}" width="${headerColWidth + n * colWidth}" height="${rowHeight}" fill="#F8FAFC" stroke="#E2E8F0" rx="4" />\n`;

  // Corner header
  svg += `  <text x="${gridStartX + 12}" y="${gridStartY + rowHeight / 2}" class="row-header" fill="#64748B">From \\ To</text>\n`;

  // Column headers
  for (let j = 0; j < n; j++) {
    const colX = gridStartX + headerColWidth + j * colWidth;
    const label = getNodeLabel(selectedNodes[j]);
    svg += `  <text x="${colX + colWidth / 2}" y="${gridStartY + rowHeight / 2}" class="col-header">${escapeXml(label)}</text>\n`;
  }

  // Rows
  for (let i = 0; i < n; i++) {
    const rowY = gridStartY + (i + 1) * rowHeight;
    const u = selectedNodes[i];
    const uLabel = getNodeLabel(u);

    // Row header cell
    svg += `  <rect x="${gridStartX}" y="${rowY}" width="${headerColWidth}" height="${rowHeight}" fill="#F8FAFC" stroke="#E2E8F0" />\n`;
    svg += `  <text x="${gridStartX + 12}" y="${rowY + rowHeight / 2}" class="row-header">${escapeXml(uLabel)}</text>\n`;

    // Data cells
    for (let j = 0; j < n; j++) {
      const v = selectedNodes[j];
      const cellX = gridStartX + headerColWidth + j * colWidth;

      let cellBg = "#FFFFFF";
      let cellBorder = "#E2E8F0";
      let cellText = "-";
      let textColor = "#64748B";

      if (u === v) {
        cellBg = "#F1F5F9";
        cellText = "-";
        textColor = "#94A3B8";
      } else {
        const item = task.results?.[u]?.[v];
        if (item) {
          if (item.status === "success" && item.latency_ms != null) {
            const lat = item.latency_ms;
            cellText = `${lat.toFixed(1)} ms`;
            if (lat < 30) {
              cellBg = "#ECFDF5";
              cellBorder = "#A7F3D0";
              textColor = "#065F46";
            } else if (lat < 70) {
              cellBg = "#F0F9FF";
              cellBorder = "#BAE6FD";
              textColor = "#0369A1";
            } else if (lat < 150) {
              cellBg = "#FFFBEB";
              cellBorder = "#FDE68A";
              textColor = "#B45309";
            } else {
              cellBg = "#FEF2F2";
              cellBorder = "#FECACA";
              textColor = "#B91C1C";
            }
          } else if (item.status === "no_link") {
            cellBg = "#F8FAFC";
            cellText = "N/A";
            textColor = "#94A3B8";
          } else if (item.status === "no_agent") {
            cellBg = "#FFFBEB";
            cellText = "No agent";
            textColor = "#D97706";
          } else if (item.status === "unreachable" || item.status === "failed") {
            cellBg = "#FEF2F2";
            cellText = "Timeout";
            textColor = "#DC2626";
          } else if (item.status === "pending") {
            cellBg = "#F8FAFC";
            cellText = "...";
            textColor = "#CBD5E1";
          }
        }
      }

      svg += `  <rect x="${cellX}" y="${rowY}" width="${colWidth}" height="${rowHeight}" fill="${cellBg}" stroke="${cellBorder}" />\n`;
      svg += `  <text x="${cellX + colWidth / 2}" y="${rowY + rowHeight / 2}" class="cell-val" fill="${textColor}">${escapeXml(cellText)}</text>\n`;
    }
  }

  // Legend at bottom
  const legendY = gridStartY + (n + 1) * rowHeight + 20;
  const legendItems = [
    { label: "< 30 ms", bg: "#ECFDF5", border: "#A7F3D0", text: "#065F46" },
    { label: "30 - 70 ms", bg: "#F0F9FF", border: "#BAE6FD", text: "#0369A1" },
    { label: "70 - 150 ms", bg: "#FFFBEB", border: "#FDE68A", text: "#B45309" },
    { label: "> 150 ms", bg: "#FEF2F2", border: "#FECACA", text: "#B91C1C" },
    { label: "No Link / N/A", bg: "#F8FAFC", border: "#E2E8F0", text: "#94A3B8" },
    { label: "Timeout / Error", bg: "#FEF2F2", border: "#FECACA", text: "#DC2626" },
  ];

  let curX = margin;
  svg += `  <!-- Legend -->\n`;
  for (const item of legendItems) {
    svg += `  <rect x="${curX}" y="${legendY}" width="18" height="14" rx="3" fill="${item.bg}" stroke="${item.border}" />\n`;
    svg += `  <text x="${curX + 24}" y="${legendY + 7}" class="legend-text">${escapeXml(item.label)}</text>\n`;
    curX += 130;
  }

  svg += `</svg>\n`;
  return svg;
}

export async function svgToPngBlob(svgString: string, scale: number = 2): Promise<Blob> {
  return new Promise((resolve, reject) => {
    const parser = new DOMParser();
    const doc = parser.parseFromString(svgString, "image/svg+xml");
    const svgEl = doc.documentElement;
    const width = parseFloat(svgEl.getAttribute("width") || "800");
    const height = parseFloat(svgEl.getAttribute("height") || "600");

    const svgBlob = new Blob([svgString], { type: "image/svg+xml;charset=utf-8" });
    const url = URL.createObjectURL(svgBlob);
    const img = new Image();

    img.onload = () => {
      URL.revokeObjectURL(url);
      try {
        const canvas = document.createElement("canvas");
        canvas.width = Math.round(width * scale);
        canvas.height = Math.round(height * scale);
        const ctx = canvas.getContext("2d");
        if (!ctx) {
          reject(new Error("Failed to get canvas 2d context"));
          return;
        }
        ctx.fillStyle = "#FFFFFF";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.scale(scale, scale);
        ctx.drawImage(img, 0, 0, width, height);

        canvas.toBlob(
          (blob) => {
            if (blob) {
              resolve(blob);
            } else {
              reject(new Error("Failed to generate PNG blob"));
            }
          },
          "image/png",
          1.0,
        );
      } catch (err) {
        reject(err);
      }
    };

    img.onerror = (e) => {
      URL.revokeObjectURL(url);
      reject(new Error("Failed to render SVG image: " + e));
    };

    img.src = url;
  });
}

export async function exportPingTableFile(
  task: PingTableTask,
  format: "svg" | "png",
  networkNameOrOptions?: string | PingTableExportOptions,
  action: PingTableExportAction = "download",
): Promise<void> {
  const options: PingTableExportOptions =
    typeof networkNameOrOptions === "string"
      ? { networkName: networkNameOrOptions }
      : networkNameOrOptions || {};

  const svgString = generatePingTableSvg(task, options);
  const dateStr = new Date(task.created_at ? task.created_at * 1000 : Date.now()).toISOString().slice(0, 10);
  const trimmedNet = options.networkName?.trim() || "easy42";
  const safePrefix = trimmedNet.toLowerCase() === "easy42"
    ? "easy42"
    : trimmedNet.replace(/\s+/g, "-").replace(/[/\\?%*:|"<>]/g, "");
  const baseFilename = `${safePrefix}-ping-table-${task.latency_type}-${dateStr}`;

  // If opening in a new tab, open immediately to avoid popup blockers
  const newTab = action === "new_tab" ? window.open("about:blank", "_blank") : null;

  try {
    const blob =
      format === "svg"
        ? new Blob([svgString], { type: "image/svg+xml;charset=utf-8" })
        : await svgToPngBlob(svgString, 2);

    const url = URL.createObjectURL(blob);

    if (action === "new_tab") {
      if (newTab) {
        newTab.location.href = url;
      } else {
        window.open(url, "_blank");
      }
      setTimeout(() => URL.revokeObjectURL(url), 60000);
    } else {
      const a = document.createElement("a");
      a.href = url;
      a.download = `${baseFilename}.${format}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      setTimeout(() => URL.revokeObjectURL(url), 10000);
    }
  } catch (err) {
    if (newTab) newTab.close();
    throw err;
  }
}
