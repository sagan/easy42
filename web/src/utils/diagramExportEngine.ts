import { Node, Link, NetworkPolicy, GraphBlock } from "../types/api";
import { getEffectiveLinkCost } from "./cost";
import { isManualLink } from "./link";
import { calculateGraphCentrality, getLinkId, CentralityMetric, WeightingMode } from "./graphCentrality";

export type NodeNameSource = "name" | "caption" | "none";
export type NodeIpSource = "main" | "external" | "none";
export type ExportFormat = "svg" | "png";
export type ExportAction = "download" | "new_tab";

export interface DiagramExportOptions {
  nodeNameSource: NodeNameSource;
  nodeIpSource: NodeIpSource;
  displayLinkCost: boolean;
  displayBlocks: boolean;
  displayGroupName: boolean;
  displayExternalNodes: boolean;
  diagramTitle?: string;
  format: ExportFormat;
  emphasizeCentrality?: boolean;
  centralityMetric?: CentralityMetric;
  centralityWeighting?: WeightingMode;
}

const BLOCK_PALETTE: Record<string, string> = {
  indigo: "#6366F1",
  cyan: "#06B6D4",
  emerald: "#10B981",
  amber: "#F59E0B",
  purple: "#A855F7",
  rose: "#F43F5E",
};

function resolveBlockColor(colorStr?: string): string {
  if (!colorStr) return "#6366F1";
  if (colorStr.startsWith("#")) return colorStr;
  const lower = colorStr.toLowerCase();
  return BLOCK_PALETTE[lower] || "#6366F1";
}

function escapeXml(unsafe: string): string {
  return unsafe
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

export function formatExportNodeName(node: Node, source: NodeNameSource): string {
  switch (source) {
    case "name":
      return node.name || "***";
    case "caption": {
      const caption = node.caption?.trim() || "";
      // a special prefix is recognized specially and treated as private (secret) value
      return caption && !caption.startsWith("!") ? caption : "***";
    }
    case "none":
      return "***";
    default:
      return node.name || "***";
  }
}

export function formatExportNodeIp(node: Node, source: NodeIpSource): string {
  switch (source) {
    case "main": {
      const v4 = node.ip?.trim();
      if (v4) return v4;
      const v6 = node.ip6?.trim();
      if (v6) return v6;
      return "***";
    }
    case "external": {
      const v4 = node.external_ip?.trim();
      if (v4) return v4;
      const v6 = node.external_ip6?.trim();
      if (v6) return v6;
      return "***";
    }
    case "none":
      return "***";
    default:
      return "***";
  }
}

export interface NodePosition {
  cardX: number;
  cardY: number;
  centerX: number;
  centerY: number;
  width: number;
  height: number;
}

/**
 * Computes deterministic node positions based on reference coordinates or geometric circle fallback.
 */
export function computeDeterministicNodePositions(
  nodes: Node[],
  cardWidth: number = 180,
  cardHeight: number = 64,
  nodeScales?: Map<string, number>,
): Map<string, NodePosition> {
  const sorted = [...nodes].sort((a, b) => a.name.localeCompare(b.name));
  const total = sorted.length;
  const radius = Math.max(240, total * 65);
  const centerX = 500;
  const centerY = 380;

  const positions = new Map<string, NodePosition>();

  sorted.forEach((node, idx) => {
    let cx: number;
    let cy: number;

    if (typeof node.x === "number" && !isNaN(node.x) && typeof node.y === "number" && !isNaN(node.y)) {
      cx = node.x + 130;
      cy = node.y + 90;
    } else {
      const angle = (idx / (total || 1)) * 2 * Math.PI - Math.PI / 2;
      cx = total === 1 ? centerX : Math.round(centerX + radius * Math.cos(angle));
      cy = total === 1 ? centerY : Math.round(centerY + radius * Math.sin(angle));
    }

    const scale = nodeScales?.get(node.name) ?? 1.0;
    const w = Math.round(cardWidth * scale);
    const h = Math.round(cardHeight * scale);

    const cardX = Math.round(cx - w / 2);
    const cardY = Math.round(cy - h / 2);

    positions.set(node.name, {
      cardX,
      cardY,
      centerX: cx,
      centerY: cy,
      width: w,
      height: h,
    });
  });

  return positions;
}

export interface ComputedBlockBounds {
  id: string;
  name: string;
  color: string;
  x: number;
  y: number;
  width: number;
  height: number;
  memberCount: number;
  isFullMesh: boolean;
}

/**
 * Standalone engine to deterministically generate a clean, modern SVG representation of the network topology.
 */
export function generateTopologySvg(
  nodes: Node[],
  links: Link[],
  blocks: GraphBlock[] | undefined,
  nodeBlockMap: Record<string, string> | undefined,
  networkPolicies: NetworkPolicy[] | undefined,
  options: DiagramExportOptions,
): string {
  const CARD_W = 180;
  const CARD_H = 64;
  const PADDING = 60;

  const effectiveNodes = options.displayExternalNodes !== false ? nodes : nodes.filter((n) => !n.is_external);
  const effectiveNodeNames = new Set(effectiveNodes.map((n) => n.name));
  const effectiveLinks = links.filter((l) => effectiveNodeNames.has(l.from.name) && effectiveNodeNames.has(l.to.name));

  // Compute graph centrality metrics for nodes and links
  const centralityResult = calculateGraphCentrality(effectiveNodes, effectiveLinks, {
    metric: options.centralityMetric || "betweenness",
    weighting: options.centralityWeighting || "weighted",
    networkPolicies,
  });

  const nodeScales = new Map<string, number>();
  if (options.emphasizeCentrality !== false) {
    effectiveNodes.forEach((n) => {
      const c = centralityResult.nodes.get(n.name);
      if (c) nodeScales.set(n.name, c.scale);
    });
  }

  const nodePositions = computeDeterministicNodePositions(effectiveNodes, CARD_W, CARD_H, nodeScales);
  const nodeMap = new Map<string, Node>();
  effectiveNodes.forEach((n) => nodeMap.set(n.name, n));

  const isLinkExternal = (l: Link) =>
    Boolean(nodeMap.get(l.from?.name)?.is_external || nodeMap.get(l.to?.name)?.is_external);

  const internalNodes = effectiveNodes.filter((n) => !n.is_external);
  const externalNodes = effectiveNodes.filter((n) => Boolean(n.is_external));

  const validLinks = effectiveLinks.filter((l) => nodePositions.has(l.from.name) && nodePositions.has(l.to.name));
  const internalLinks = validLinks.filter((l) => !isLinkExternal(l));
  const externalLinks = validLinks.filter((l) => isLinkExternal(l));

  const hasExternalElements = externalNodes.length > 0 || externalLinks.length > 0;
  const HEADER_H = hasExternalElements ? 68 : 54;

  // Determine block rectangles if displayBlocks is enabled
  const computedBlocks: ComputedBlockBounds[] = [];
  if (options.displayBlocks && blocks && blocks.length > 0) {
    blocks.forEach((block) => {
      const memberNodes = effectiveNodes.filter((n) => {
        const mappedBlockId = nodeBlockMap?.[n.name];
        if (mappedBlockId) return mappedBlockId === block.id;
        return block.nodes?.includes(n.name);
      });

      let bx = typeof block.x === "number" ? block.x : 0;
      let by = typeof block.y === "number" ? block.y : 0;
      let bw = typeof block.width === "number" && block.width > 0 ? block.width : 280;
      let bh = typeof block.height === "number" && block.height > 0 ? block.height : 200;

      if (memberNodes.length > 0) {
        let memMinX = Infinity;
        let memMinY = Infinity;
        let memMaxX = -Infinity;
        let memMaxY = -Infinity;

        memberNodes.forEach((mn) => {
          const p = nodePositions.get(mn.name);
          if (p) {
            memMinX = Math.min(memMinX, p.cardX);
            memMinY = Math.min(memMinY, p.cardY);
            memMaxX = Math.max(memMaxX, p.cardX + p.width);
            memMaxY = Math.max(memMaxY, p.cardY + p.height);
          }
        });

        if (memMinX !== Infinity) {
          const padX = 30;
          const padTop = 46;
          const padBottom = 26;

          const boundsX = memMinX - padX;
          const boundsY = memMinY - padTop;
          const boundsW = memMaxX - memMinX + padX * 2;
          const boundsH = memMaxY - memMinY + padTop + padBottom;

          if (typeof block.x === "number" && typeof block.width === "number" && block.width > 0) {
            bx = Math.min(boundsX, block.x);
            by = Math.min(boundsY, block.y);
            bw = Math.max(boundsX + boundsW, block.x + block.width) - bx;
            bh = Math.max(boundsY + boundsH, block.y + block.height) - by;
          } else {
            bx = boundsX;
            by = boundsY;
            bw = boundsW;
            bh = boundsH;
          }
        }
      }

      let isFullMesh = false;
      if (memberNodes.length >= 2) {
        const hasIntraLink = (u: string, v: string) => {
          const uClean = u.trim().toLowerCase();
          const vClean = v.trim().toLowerCase();
          return effectiveLinks.some((l) => {
            const fromName = l.from?.name?.trim().toLowerCase();
            const toName = l.to?.name?.trim().toLowerCase();
            return (fromName === uClean && toName === vClean) || (fromName === vClean && toName === uClean);
          });
        };
        isFullMesh = memberNodes.every((u, i) => memberNodes.slice(i + 1).every((v) => hasIntraLink(u.name, v.name)));
      }

      const blockName = block.name.trim();
      computedBlocks.push({
        id: block.id,
        name: blockName && !blockName.startsWith("!") ? blockName : "Group",
        color: resolveBlockColor(block.color),
        x: Math.round(bx),
        y: Math.round(by),
        width: Math.round(bw),
        height: Math.round(bh),
        memberCount: memberNodes.length,
        isFullMesh,
      });
    });
  }

  // Determine bounding box across all nodes and blocks
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  if (nodes.length === 0 && computedBlocks.length === 0) {
    minX = 0;
    minY = 0;
    maxX = 640;
    maxY = 400;
  } else {
    nodePositions.forEach((pos) => {
      if (pos.cardX < minX) minX = pos.cardX;
      if (pos.cardY < minY) minY = pos.cardY;
      if (pos.cardX + pos.width > maxX) maxX = pos.cardX + pos.width;
      if (pos.cardY + pos.height > maxY) maxY = pos.cardY + pos.height;
    });

    computedBlocks.forEach((cb) => {
      if (cb.x < minX) minX = cb.x;
      if (cb.y < minY) minY = cb.y;
      if (cb.x + cb.width > maxX) maxX = cb.x + cb.width;
      if (cb.y + cb.height > maxY) maxY = cb.y + cb.height;
    });
  }

  const viewBoxX = Math.round(minX - PADDING);
  const viewBoxY = Math.round(minY - PADDING - HEADER_H);
  const viewBoxW = Math.max(680, Math.round(maxX - minX + PADDING * 2));
  const viewBoxH = Math.max(480, Math.round(maxY - minY + PADDING * 2 + HEADER_H));

  // Deterministically sort and group links by node pairs
  const pairGroups = new Map<string, Link[]>();

  validLinks.forEach((l) => {
    const key = [l.from.name, l.to.name].sort().join("---");
    const arr = pairGroups.get(key) || [];
    arr.push(l);
    pairGroups.set(key, arr);
  });

  // Sort links in each pair deterministically
  pairGroups.forEach((groupLinks) => {
    groupLinks.sort((a, b) => {
      const keyA = `${a.from.name}:${a.from.interface}->${a.to.name}:${a.to.interface}`;
      const keyB = `${b.from.name}:${b.from.interface}->${b.to.name}:${b.to.interface}`;
      return keyA.localeCompare(keyB);
    });
  });

  // Render Blocks SVG
  let blocksSvg = "";
  if (computedBlocks.length > 0) {
    computedBlocks.forEach((cb) => {
      const displayName = options.displayGroupName !== false ? cb.name : "***";
      const headerPillW = Math.max(90, displayName.length * 8 + 36);

      let fullMeshBadgeSvg = "";
      if (cb.isFullMesh) {
        fullMeshBadgeSvg = `
      <!-- Full Mesh Badge -->
      <rect x="${cb.x + cb.width - 80}" y="${cb.y + 11}" width="66" height="20" rx="5" fill="#ECFDF5" stroke="#10B981" stroke-width="1.2" stroke-opacity="0.8" />
      <circle cx="${cb.x + cb.width - 70}" cy="${cb.y + 21}" r="3" fill="#10B981" />
      <text x="${cb.x + cb.width - 61}" y="${cb.y + 24.5}" font-size="9.5" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif" font-weight="700" fill="#047857">Full Mesh</text>`;
      }

      blocksSvg += `    <g class="block-group" id="block-${escapeXml(cb.id)}">
      <!-- Container background -->
      <rect x="${cb.x}" y="${cb.y}" width="${cb.width}" height="${cb.height}" rx="14" fill="${cb.color}" fill-opacity="0.04" stroke="${cb.color}" stroke-width="1.8" stroke-dasharray="6 4" stroke-opacity="0.75" />
      
      <!-- Group Title Pill -->
      <rect x="${cb.x + 14}" y="${cb.y + 10}" width="${headerPillW}" height="22" rx="6" fill="#FFFFFF" stroke="${cb.color}" stroke-width="1.2" stroke-opacity="0.5" filter="url(#badge-shadow)" />
      <circle cx="${cb.x + 24}" cy="${cb.y + 21}" r="3.5" fill="${cb.color}" />
      <text x="${cb.x + 33}" y="${cb.y + 24.5}" font-size="11" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif" font-weight="700" fill="${cb.color}">${escapeXml(displayName)}</text>${fullMeshBadgeSvg}
    </g>\n`;
    });
  }

  // Render Links SVG
  let linksSvg = "";
  let linkCostBadgesSvg = "";

  pairGroups.forEach((groupLinks) => {
    const totalLinks = groupLinks.length;

    groupLinks.forEach((link, idx) => {
      const posFrom = nodePositions.get(link.from.name)!;
      const posTo = nodePositions.get(link.to.name)!;

      const ax = posFrom.centerX;
      const ay = posFrom.centerY;
      const bx = posTo.centerX;
      const by = posTo.centerY;

      const nodeFrom = nodeMap.get(link.from.name);
      const nodeTo = nodeMap.get(link.to.name);
      const isExternal = Boolean(nodeFrom?.is_external || nodeTo?.is_external);
      const isManual = isManualLink(link);

      const linkId = getLinkId(link);
      const linkCentrality = centralityResult.links.get(linkId);
      const isCoreLink = Boolean(options.emphasizeCentrality !== false && linkCentrality?.isCore);
      const strokeWidth =
        options.emphasizeCentrality !== false && linkCentrality ? linkCentrality.strokeWidth : 2;
      const strokeOpacity =
        options.emphasizeCentrality !== false && linkCentrality ? linkCentrality.opacity : 0.85;

      let strokeColor = "#10B981"; // Emerald
      let strokeDash = "";
      if (isManual) {
        strokeColor = "#7C3AED"; // Consistent Violet
        strokeDash = 'stroke-dasharray="5 3"';
      } else if (isExternal) {
        strokeColor = "#8B5CF6"; // Violet
        strokeDash = 'stroke-dasharray="6 4"';
      } else if (isCoreLink) {
        strokeColor = "#059669"; // Deeper rich emerald for backbone transit links
      }

      let pathD = "";
      let labelX = 0;
      let labelY = 0;

      if (totalLinks > 1) {
        const midX = (ax + bx) / 2;
        const midY = (ay + by) / 2;
        const dx = bx - ax;
        const dy = by - ay;
        const len = Math.hypot(dx, dy) || 1;
        const nx = -dy / len;
        const ny = dx / len;
        const step = 30;
        const offset = (idx - (totalLinks - 1) / 2) * step;
        const cx = midX + nx * offset;
        const cy = midY + ny * offset;

        pathD = `M ${ax} ${ay} Q ${cx} ${cy} ${bx} ${by}`;
        labelX = Math.round(0.25 * ax + 0.5 * cx + 0.25 * bx);
        labelY = Math.round(0.25 * ay + 0.5 * cy + 0.25 * by);
      } else {
        pathD = `M ${ax} ${ay} L ${bx} ${by}`;
        labelX = Math.round((ax + bx) / 2);
        labelY = Math.round((ay + by) / 2);
      }

      linksSvg += `    <path d="${pathD}" fill="none" stroke="${strokeColor}" stroke-width="${strokeWidth.toFixed(1)}" stroke-linecap="round" stroke-opacity="${strokeOpacity.toFixed(2)}" ${strokeDash} />\n`;

      if (options.displayLinkCost) {
        const fromIsRemoteExternal = Boolean(nodeTo?.is_external);
        const toIsRemoteExternal = Boolean(nodeFrom?.is_external);
        const fromCost = getEffectiveLinkCost(link.from, fromIsRemoteExternal, networkPolicies);
        const toCost = getEffectiveLinkCost(link.to, toIsRemoteExternal, networkPolicies);

        let costLabel = "";
        let isOverridden = false;
        let badgeWidth = 36;

        if (isExternal) {
          // For external link, only the internal node side link cost matters
          const internalCost = nodeFrom?.is_external ? toCost : fromCost;
          isOverridden = nodeFrom?.is_external
            ? Boolean(link.to?.cost && link.to.cost !== 0)
            : Boolean(link.from?.cost && link.from.cost !== 0);
          costLabel = `${internalCost}`;
          badgeWidth = Math.max(34, costLabel.length * 8 + 14);
        } else if (fromCost === toCost) {
          costLabel = `${fromCost}`;
          isOverridden = Boolean((link.from?.cost && link.from.cost !== 0) || (link.to?.cost && link.to.cost !== 0));
          badgeWidth = Math.max(34, costLabel.length * 8 + 14);
        } else {
          costLabel = `${fromCost} / ${toCost}`;
          isOverridden = Boolean((link.from?.cost && link.from.cost !== 0) || (link.to?.cost && link.to.cost !== 0));
          badgeWidth = Math.max(52, costLabel.length * 7 + 16);
        }

        const badgeX = Math.round(labelX - badgeWidth / 2);
        const badgeY = Math.round(labelY - 10);
        const textY = Math.round(labelY + 3.5);
        const textColor = isOverridden ? "#D97706" : "#475569";

        linkCostBadgesSvg += `    <g class="cost-badge">
      <rect x="${badgeX}" y="${badgeY}" width="${badgeWidth}" height="20" rx="5" fill="#FFFFFF" stroke="#CBD5E1" stroke-width="1.2" filter="url(#badge-shadow)" />
      <text x="${labelX}" y="${textY}" font-size="10.5" font-family="ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace" font-weight="700" fill="${textColor}" text-anchor="middle">${escapeXml(costLabel)}</text>
    </g>\n`;
      }
    });
  });

  // Render Nodes SVG
  let nodesSvg = "";
  const sortedNodes = [...effectiveNodes].sort((a, b) => a.name.localeCompare(b.name));

  sortedNodes.forEach((node) => {
    const pos = nodePositions.get(node.name);
    if (!pos) return;

    const nameText = formatExportNodeName(node, options.nodeNameSource);
    const ipText = formatExportNodeIp(node, options.nodeIpSource);
    const isExternal = Boolean(node.is_external);
    const isRR = Boolean(node.route_reflector);

    const nodeCentrality = centralityResult.nodes.get(node.name);
    const isCoreNode = Boolean(options.emphasizeCentrality !== false && nodeCentrality?.isCore);
    const borderWidth =
      options.emphasizeCentrality !== false && nodeCentrality
        ? Math.max(isRR ? 2.5 : 1.5, nodeCentrality.borderWidth)
        : isRR
          ? 2
          : 1.5;

    let borderColor = isExternal ? "#A855F7" : isRR ? "#6366F1" : "#CBD5E1";
    if (isCoreNode && !isExternal && !isRR) {
      borderColor = "#059669";
    }
    const borderDash = isExternal ? 'stroke-dasharray="5 3"' : "";
    const dotColor = isExternal ? "#A855F7" : isRR ? "#6366F1" : isCoreNode ? "#059669" : "#10B981";

    let rightOffset = 10;
    let extBadgeSvg = "";
    if (isExternal) {
      rightOffset += 32;
      extBadgeSvg = `      <rect x="${pos.cardX + pos.width - rightOffset}" y="${pos.cardY + 10}" width="32" height="16" rx="4" fill="#FAF5FF" stroke="#DDD6FE" stroke-width="1" />
      <text x="${pos.cardX + pos.width - rightOffset + 16}" y="${pos.cardY + 21.5}" font-size="8.5" font-family="ui-monospace, monospace" font-weight="700" fill="#7C3AED" text-anchor="middle">EXT</text>\n`;
      rightOffset += 6;
    }

    let rrBadgeSvg = "";
    if (isRR) {
      rightOffset += 26;
      rrBadgeSvg = `      <rect x="${pos.cardX + pos.width - rightOffset}" y="${pos.cardY + 10}" width="26" height="16" rx="4" fill="#EEF2FF" stroke="#C7D2FE" stroke-width="1" />
      <text x="${pos.cardX + pos.width - rightOffset + 13}" y="${pos.cardY + 21.5}" font-size="8.5" font-family="ui-monospace, monospace" font-weight="800" fill="#4338CA" text-anchor="middle">RR</text>\n`;
      rightOffset += 6;
    }

    let coreBadgeSvg = "";
    if (isCoreNode) {
      rightOffset += 40;
      coreBadgeSvg = `      <rect x="${pos.cardX + pos.width - rightOffset}" y="${pos.cardY + 10}" width="40" height="16" rx="4" fill="#ECFDF5" stroke="#A7F3D0" stroke-width="1" />
      <text x="${pos.cardX + pos.width - rightOffset + 20}" y="${pos.cardY + 21.5}" font-size="8.5" font-family="ui-monospace, monospace" font-weight="800" fill="#047857" text-anchor="middle">CORE</text>\n`;
    }

    nodesSvg += `    <g class="node-card" id="node-${escapeXml(node.name)}">
      <!-- Card background -->
      <rect x="${pos.cardX}" y="${pos.cardY}" width="${pos.width}" height="${pos.height}" rx="10" fill="#FFFFFF" stroke="${borderColor}" stroke-width="${borderWidth.toFixed(1)}" ${borderDash} filter="url(#card-shadow)" />
      
      <!-- Status indicator dot -->
      <circle cx="${pos.cardX + 16}" cy="${pos.cardY + 22}" r="4.5" fill="${dotColor}" />

      <!-- Node Name -->
      <text x="${pos.cardX + 27}" y="${pos.cardY + 26}" font-size="12.5" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif" font-weight="700" fill="#0F172A">${escapeXml(nameText)}</text>
      ${extBadgeSvg}
      ${rrBadgeSvg}
      ${coreBadgeSvg}
      <!-- Divider -->
      <line x1="${pos.cardX + 12}" y1="${pos.cardY + 36}" x2="${pos.cardX + pos.width - 12}" y2="${pos.cardY + 36}" stroke="#F1F5F9" stroke-width="1" />

      <!-- Node IP -->
      <text x="${pos.cardX + 15}" y="${pos.cardY + 51}" font-size="10.5" font-family="ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace" font-weight="500" fill="#64748B">${escapeXml(ipText)}</text>
    </g>\n`;
  });

  // Header & Legend SVG
  const headerX = viewBoxX + PADDING;
  const headerY = viewBoxY + PADDING / 2 + 18;
  const legendRight = viewBoxX + viewBoxW - PADDING;
  const hasExternalNodes = effectiveNodes.some((n) => n.is_external);
  const hasRRNodes = effectiveNodes.some((n) => n.route_reflector);
  const hasCoreNodes =
    options.emphasizeCentrality !== false &&
    Array.from(centralityResult.nodes.values()).some((n) => n.isCore);
  const hasCoreLinks =
    options.emphasizeCentrality !== false &&
    Array.from(centralityResult.links.values()).some((l) => l.isCore);

  const legendItems: string[] = [];
  let curOffset = 0;

  // Internal
  legendItems.push(`<circle cx="${curOffset + 6}" cy="6" r="4" fill="#10B981" />
      <text x="${curOffset + 15}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">Internal</text>`);
  curOffset += 68;

  // Core Node (only if emphasizeCentrality and core nodes present)
  if (hasCoreNodes) {
    legendItems.push(`<rect x="${curOffset + 4}" y="0" width="28" height="12" rx="3" fill="#ECFDF5" stroke="#A7F3D0" stroke-width="1" />
      <text x="${curOffset + 18}" y="9" font-size="7" font-family="monospace" font-weight="800" fill="#047857" text-anchor="middle">CORE</text>
      <text x="${curOffset + 38}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">Core Node</text>`);
    curOffset += 96;
  }

  // External (only if present in diagram)
  if (hasExternalNodes) {
    legendItems.push(`<circle cx="${curOffset + 6}" cy="6" r="4" fill="#A855F7" />
      <text x="${curOffset + 15}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">External</text>`);
    curOffset += 70;
  }

  // Route Reflector (only if present in diagram)
  if (hasRRNodes) {
    legendItems.push(`<rect x="${curOffset + 4}" y="0" width="18" height="12" rx="3" fill="#EEF2FF" stroke="#C7D2FE" stroke-width="1" />
      <text x="${curOffset + 13}" y="9" font-size="7.5" font-family="monospace" font-weight="800" fill="#4338CA" text-anchor="middle">RR</text>
      <text x="${curOffset + 28}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">Route Reflector</text>`);
    curOffset += 115;
  }

  // WireGuard
  legendItems.push(`<line x1="${curOffset + 6}" y1="6" x2="${curOffset + 24}" y2="6" stroke="#10B981" stroke-width="2" />
      <text x="${curOffset + 31}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">WireGuard</text>`);
  curOffset += 94;

  // Backbone link
  if (hasCoreLinks) {
    legendItems.push(`<line x1="${curOffset + 6}" y1="6" x2="${curOffset + 24}" y2="6" stroke="#059669" stroke-width="4.5" stroke-linecap="round" />
      <text x="${curOffset + 31}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">Backbone</text>`);
    curOffset += 92;
  }

  // Group
  if (computedBlocks.length > 0) {
    legendItems.push(`<rect x="${curOffset + 6}" y="0" width="16" height="12" rx="3" fill="#6366F1" fill-opacity="0.1" stroke="#6366F1" stroke-width="1.2" stroke-dasharray="3 2" />
      <text x="${curOffset + 28}" y="9.5" font-size="10" font-family="sans-serif" font-weight="600" fill="#475569">Group</text>`);
    curOffset += 68;
  }

  const titleText = options.diagramTitle?.trim() || "easy42 Network Topology";

  const groupText =
    computedBlocks.length > 0 ? ` · ${computedBlocks.length} group${computedBlocks.length === 1 ? "" : "s"}` : "";
  const centralityLabel =
    options.emphasizeCentrality !== false
      ? ` · ${options.centralityMetric === "degree" ? "Degree Centrality" : "Betweenness Centrality"}`
      : "";
  const mainStatsText = `${internalNodes.length} nodes · ${internalLinks.length} links${groupText}${centralityLabel}`;

  let externalStatsSvg = "";
  if (hasExternalElements) {
    externalStatsSvg = `\n    <text x="${headerX}" y="${headerY + 31}" font-size="9.5" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif" font-weight="500" fill="#64748B">${externalNodes.length} external nodes · ${externalLinks.length} external links</text>`;
  }

  const headerSvg = `  <!-- Header & Legend -->
  <g class="diagram-header">
    <text x="${headerX}" y="${headerY}" font-size="15" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif" font-weight="700" fill="#1E293B">${escapeXml(titleText)}</text>
    <text x="${headerX}" y="${headerY + 16}" font-size="10.5" font-family="-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif" font-weight="500" fill="#64748B">${mainStatsText}</text>${externalStatsSvg}

    <!-- Legend -->
    <g transform="translate(${legendRight - curOffset}, ${headerY - 8})">
      ${legendItems.join("\n      ")}
    </g>
  </g>`;

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBoxX} ${viewBoxY} ${viewBoxW} ${viewBoxH}" width="${viewBoxW}" height="${viewBoxH}">
  <defs>
    <filter id="card-shadow" x="-8%" y="-10%" width="120%" height="130%">
      <feDropShadow dx="0" dy="3" stdDeviation="4" flood-color="#0F172A" flood-opacity="0.08" />
    </filter>
    <filter id="badge-shadow" x="-10%" y="-15%" width="125%" height="145%">
      <feDropShadow dx="0" dy="1.5" stdDeviation="2" flood-color="#0F172A" flood-opacity="0.06" />
    </filter>
    <pattern id="grid-pattern" width="24" height="24" patternUnits="userSpaceOnUse">
      <circle cx="12" cy="12" r="1" fill="#E2E8F0" />
    </pattern>
  </defs>

  <!-- Background -->
  <rect x="${viewBoxX}" y="${viewBoxY}" width="${viewBoxW}" height="${viewBoxH}" fill="#F8FAFC" />
  <rect x="${viewBoxX}" y="${viewBoxY}" width="${viewBoxW}" height="${viewBoxH}" fill="url(#grid-pattern)" />

${headerSvg}

  <!-- Block Groups -->
  <g class="block-groups">
${blocksSvg}  </g>

  <!-- Links -->
  <g class="network-links">
${linksSvg}  </g>

  <!-- Link Cost Badges -->
  <g class="cost-badges">
${linkCostBadgesSvg}  </g>

  <!-- Nodes -->
  <g class="network-nodes">
${nodesSvg}  </g>
</svg>
`;
}

/**
 * Converts an SVG string into a high-DPI PNG blob.
 */
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
          reject(new Error("Failed to get canvas 2D context"));
          return;
        }
        ctx.fillStyle = "#F8FAFC";
        ctx.fillRect(0, 0, canvas.width, canvas.height);
        ctx.scale(scale, scale);
        ctx.drawImage(img, 0, 0, width, height);

        canvas.toBlob(
          (blob) => {
            if (blob) {
              resolve(blob);
            } else {
              reject(new Error("Failed to create PNG blob from canvas"));
            }
          },
          "image/png",
          1.0,
        );
      } catch (err) {
        reject(err);
      }
    };

    img.onerror = (err) => {
      URL.revokeObjectURL(url);
      reject(new Error("Failed to render SVG image: " + err));
    };

    img.src = url;
  });
}

/**
 * Executes the export action (triggering browser download or opening in new tab).
 */
export async function exportDiagramFile(
  nodes: Node[],
  links: Link[],
  blocks: GraphBlock[] | undefined,
  nodeBlockMap: Record<string, string> | undefined,
  networkPolicies: NetworkPolicy[] | undefined,
  options: DiagramExportOptions,
  action: ExportAction = "download",
): Promise<void> {
  const dateStr = new Date().toISOString().slice(0, 10);
  const baseFilename = `easy42-topology-${dateStr}`;

  // If opening in a new tab, open the tab immediately during the user gesture to avoid popup blockers
  const newTab = action === "new_tab" ? window.open("about:blank", "_blank") : null;

  try {
    const svgString = generateTopologySvg(nodes, links, blocks, nodeBlockMap, networkPolicies, options);
    const blob =
      options.format === "svg"
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
      a.download = `${baseFilename}.${options.format}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      setTimeout(() => URL.revokeObjectURL(url), 10000);
    }
  } catch (err) {
    if (newTab) {
      newTab.close();
    }
    throw err;
  }
}
