import { Node, GraphBlock } from "../types/api";

export const NODE_CARD_WIDTH = 260;
export const NODE_CARD_HEIGHT = 180;
export const NODE_SPACING_X = 50;
export const NODE_SPACING_Y = 50;
export const STEP_X = NODE_CARD_WIDTH + NODE_SPACING_X; // 310
export const STEP_Y = NODE_CARD_HEIGHT + NODE_SPACING_Y; // 230

const NODE_MARGIN = 30;
const BLOCK_MARGIN = 40;

export interface Position {
  x: number;
  y: number;
}

/**
 * Checks if a candidate rectangle intersects any existing node or block,
 * or if the candidate center lies within any block.
 */
export function isPositionOccupied(
  x: number,
  y: number,
  placedNodes: { x: number; y: number }[],
  blocks: GraphBlock[] = [],
): boolean {
  const candRight = x + NODE_CARD_WIDTH;
  const candBottom = y + NODE_CARD_HEIGHT;
  const centerX = x + NODE_CARD_WIDTH / 2;
  const centerY = y + NODE_CARD_HEIGHT / 2;

  // 1. Check collision with existing nodes
  for (const node of placedNodes) {
    const nodeLeft = node.x - NODE_MARGIN;
    const nodeTop = node.y - NODE_MARGIN;
    const nodeRight = node.x + NODE_CARD_WIDTH + NODE_MARGIN;
    const nodeBottom = node.y + NODE_CARD_HEIGHT + NODE_MARGIN;

    const overlap =
      candRight > nodeLeft &&
      x < nodeRight &&
      candBottom > nodeTop &&
      y < nodeBottom;

    if (overlap) {
      return true;
    }
  }

  // 2. Check collision with blocks (and ensure global scope: candidate must not be inside any block)
  for (const block of blocks) {
    const blockLeft = block.x - BLOCK_MARGIN;
    const blockTop = block.y - BLOCK_MARGIN;
    const blockRight = block.x + block.width + BLOCK_MARGIN;
    const blockBottom = block.y + block.height + BLOCK_MARGIN;

    const overlap =
      candRight > blockLeft &&
      x < blockRight &&
      candBottom > blockTop &&
      y < blockBottom;

    if (overlap) {
      return true;
    }

    // Also strictly verify center is not inside the block
    if (
      centerX >= block.x &&
      centerX <= block.x + block.width &&
      centerY >= block.y &&
      centerY <= block.y + block.height
    ) {
      return true;
    }
  }

  return false;
}

/**
 * Finds a suitable initial position (x, y) for a newly created node.
 * The node will:
 * 1. Be in global scope (NOT inside any block).
 * 2. Not overlap with any existing node or block.
 * 3. Be placed near the existing graph elements instead of far away.
 */
export function findInitialNodePosition(
  existingNodes: Node[] = [],
  blocks: GraphBlock[] = [],
): Position {
  // Extract all existing nodes that have valid coordinates
  const placedNodes: { x: number; y: number }[] = [];
  for (const n of existingNodes) {
    if (typeof n.x === "number" && !isNaN(n.x) && typeof n.y === "number" && !isNaN(n.y)) {
      placedNodes.push({ x: n.x, y: n.y });
    }
  }

  // Case 1: Empty graph (no nodes and no blocks)
  if (placedNodes.length === 0 && (!blocks || blocks.length === 0)) {
    return { x: 250, y: 180 };
  }

  // Case 2: Only blocks exist, or only nodes, or both
  // Calculate bounding box of existing elements
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;

  for (const n of placedNodes) {
    if (n.x < minX) minX = n.x;
    if (n.y < minY) minY = n.y;
    if (n.x + NODE_CARD_WIDTH > maxX) maxX = n.x + NODE_CARD_WIDTH;
    if (n.y + NODE_CARD_HEIGHT > maxY) maxY = n.y + NODE_CARD_HEIGHT;
  }

  for (const b of blocks || []) {
    if (b.x < minX) minX = b.x;
    if (b.y < minY) minY = b.y;
    if (b.x + b.width > maxX) maxX = b.x + b.width;
    if (b.y + b.height > maxY) maxY = b.y + b.height;
  }

  if (!isFinite(minX)) {
    return { x: 250, y: 180 };
  }

  const graphCenterX = (minX + maxX) / 2;
  const graphCenterY = (minY + maxY) / 2;

  // Generate candidate positions
  const candidates: Position[] = [];
  const candidateKeys = new Set<string>();

  const addCandidate = (x: number, y: number) => {
    // Snap to rounded pixels
    const rx = Math.round(x);
    const ry = Math.round(y);
    const key = `${rx},${ry}`;
    if (!candidateKeys.has(key)) {
      candidateKeys.add(key);
      candidates.push({ x: rx, y: ry });
    }
  };

  // Source 1: Offset from existing nodes (right, bottom, diagonal, left, top)
  for (const n of placedNodes) {
    addCandidate(n.x + STEP_X, n.y);
    addCandidate(n.x, n.y + STEP_Y);
    addCandidate(n.x + STEP_X, n.y + STEP_Y);
    addCandidate(n.x - STEP_X, n.y);
    addCandidate(n.x, n.y - STEP_Y);
  }

  // Source 2: Offset from blocks (just outside perimeter)
  for (const b of blocks || []) {
    // Right of block
    const rightX = b.x + b.width + NODE_SPACING_X;
    for (let y = b.y; y <= b.y + b.height; y += STEP_Y) {
      addCandidate(rightX, y);
    }
    // Bottom of block
    const bottomY = b.y + b.height + NODE_SPACING_Y;
    for (let x = b.x; x <= b.x + b.width; x += STEP_X) {
      addCandidate(x, bottomY);
    }
    // Top of block
    const topY = b.y - NODE_CARD_HEIGHT - NODE_SPACING_Y;
    for (let x = b.x; x <= b.x + b.width; x += STEP_X) {
      addCandidate(x, topY);
    }
    // Left of block
    const leftX = b.x - NODE_CARD_WIDTH - NODE_SPACING_X;
    for (let y = b.y; y <= b.y + b.height; y += STEP_Y) {
      addCandidate(leftX, y);
    }
  }

  // Source 3: Grid search covering bounding box plus 1-2 rings
  const startGridX = minX;
  const startGridY = minY;
  const colCount = Math.max(1, Math.ceil((maxX - minX) / STEP_X)) + 2;
  const rowCount = Math.max(1, Math.ceil((maxY - minY) / STEP_Y)) + 2;

  for (let c = 0; c <= colCount; c++) {
    for (let r = 0; r <= rowCount; r++) {
      addCandidate(startGridX + c * STEP_X, startGridY + r * STEP_Y);
    }
  }

  // Filter candidates that are not occupied
  const validCandidates: Position[] = [];
  for (const cand of candidates) {
    if (!isPositionOccupied(cand.x, cand.y, placedNodes, blocks)) {
      validCandidates.push(cand);
    }
  }

  // If no valid candidate found in the initial set (e.g. densely packed blocks), expand grid outwards
  if (validCandidates.length === 0) {
    for (let ring = 1; ring <= 15; ring++) {
      const extraX = ring * STEP_X;
      const extraY = ring * STEP_Y;

      // Try right side first
      const testX = maxX + extraX;
      for (let y = minY; y <= maxY + extraY; y += STEP_Y) {
        if (!isPositionOccupied(testX, y, placedNodes, blocks)) {
          return { x: testX, y };
        }
      }

      // Try bottom side
      const testY = maxY + extraY;
      for (let x = minX; x <= maxX + extraX; x += STEP_X) {
        if (!isPositionOccupied(x, testY, placedNodes, blocks)) {
          return { x, y: testY };
        }
      }
    }

    // Ultimate fallback: place to the right of maxX
    return { x: Math.round(maxX + NODE_SPACING_X), y: Math.round(minY) };
  }

  // Rank valid candidates by:
  // 1. Distance to nearest placed node / block (want it close to graph)
  // 2. Distance to graph center (keeps graph compact)
  // 3. Directional preference (prefer right and bottom over negative coordinates)
  let bestCandidate = validCandidates[0];
  let bestScore = Infinity;

  for (const cand of validCandidates) {
    const candCenterX = cand.x + NODE_CARD_WIDTH / 2;
    const candCenterY = cand.y + NODE_CARD_HEIGHT / 2;

    // Minimum distance to any placed node center
    let minDistToNode = Infinity;
    if (placedNodes.length > 0) {
      for (const n of placedNodes) {
        const d = Math.hypot(candCenterX - (n.x + NODE_CARD_WIDTH / 2), candCenterY - (n.y + NODE_CARD_HEIGHT / 2));
        if (d < minDistToNode) minDistToNode = d;
      }
    } else {
      // If no nodes yet, distance to nearest block
      for (const b of blocks || []) {
        const d = Math.hypot(candCenterX - (b.x + b.width / 2), candCenterY - (b.y + b.height / 2));
        if (d < minDistToNode) minDistToNode = d;
      }
    }

    const distToCenter = Math.hypot(candCenterX - graphCenterX, candCenterY - graphCenterY);

    // Directional preferences
    let penalty = 0;
    if (cand.x < minX - 10) penalty += 180;
    if (cand.y < minY - 10) penalty += 180;
    if (cand.x >= graphCenterX) penalty -= 40; // Slight preference for right
    if (cand.y >= graphCenterY) penalty -= 20; // Slight preference for bottom

    const score = minDistToNode * 0.6 + distToCenter * 0.4 + penalty;

    if (score < bestScore) {
      bestScore = score;
      bestCandidate = cand;
    }
  }

  return bestCandidate;
}
