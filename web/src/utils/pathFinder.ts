import { Link, NetworkPolicy } from "../types/api";
import { getEffectiveLinkCost } from "./cost";
import { getLinkId } from "./graphCentrality";

export interface DirectedLink {
  /** Full directed link id: `link-<from>-<fromIf>-<to>-<toIf>` (same as React Flow edge id). */
  id: string;
  /** Undirected pair key shared by both directions of the same link. */
  pairKey: string;
  link: Link;
  from: string;
  to: string;
  /** Directional effective cost (what BIRD uses as the link cost in this direction). */
  cost: number;
  workingState?: "working" | "not_working" | "unknown";
}

export interface PathHop {
  node: string;
  /** Directed link id used to reach `node` from the previous hop. Absent on the first hop. */
  viaEdgeId?: string;
  /** Cost of `viaEdgeId` in the forward direction. Absent on the first hop. */
  viaCost?: number;
}

export interface DirectedPath {
  from: string;
  to: string;
  hops: PathHop[];
  totalCost: number;
  /** Directed link ids along the path, in traversal order. */
  edgeIds: string[];
}

export interface BestPathPair {
  from: string;
  to: string;
  forward: DirectedPath | null;
  reverse: DirectedPath | null;
  /** True when the two directions disagree (asymmetric routing / cost). */
  isAsymmetric: boolean;
  /**
   * Node-name pair keys (`a---b`, interface independent) touched by either direction. This is
   * the key space used by the canvas edge filter, which is node based.
   */
  nodePairKeys: Set<string>;
  /**
   * Per rendered link id, whether the link carries the forward and/or reverse direction.
   * Keyed by the exact link id so parallel interfaces are attributed individually.
   */
  linkDirections: Map<string, { forward: boolean; reverse: boolean }>;
}

/** Undirected key for the node pair (ignores interfaces). */
export function getNodePairKey(a: string, b: string): string {
  return a < b ? `${a}---${b}` : `${b}---${a}`;
}

/** Undirected pair key for a specific link (interface aware), mirrors graphCentrality's helper. */
export function getLinkPairKey(link: Link): string {
  const p1 = `${link.from.name}:${link.from.interface}`;
  const p2 = `${link.to.name}:${link.to.interface}`;
  return p1 < p2 ? `${p1}---${p2}` : `${p2}---${p1}`;
}

/**
 * Builds the directed adjacency used for path computation.
 *
 * Only enabled links are considered: disabled links are never used by BIRD, so they
 * must not be used by the preview either. Every link yields two directed edges, each
 * carrying the cost of the *sending* endpoint (link.from.cost for from→to and
 * link.to.cost for to→from), which matches how BIRD derives per-neighbor preference.
 */
export function buildDirectedLinkGraph(
  links: Link[],
  nodes: { name: string; is_external?: boolean }[],
  networkPolicies?: NetworkPolicy[],
  workingStates?: Map<string, "working" | "not_working" | "unknown">,
): Map<string, DirectedLink[]> {
  const isExternal = new Map(
    nodes.map((n) => [n.name, Boolean(n.is_external)]),
  );
  const adjacency = new Map<string, DirectedLink[]>();

  const push = (edge: DirectedLink) => {
    const bucket = adjacency.get(edge.from);
    if (bucket) {
      bucket.push(edge);
    } else {
      adjacency.set(edge.from, [edge]);
    }
  };

  links.forEach((link) => {
    if (link.disabled) return;

    const forwardCost = getEffectiveLinkCost(
      link.from,
      isExternal.get(link.to.name),
      networkPolicies,
    );
    const reverseCost = getEffectiveLinkCost(
      link.to,
      isExternal.get(link.from.name),
      networkPolicies,
    );
    const pairKey = getLinkPairKey(link);
    const state = workingStates?.get(getLinkId(link));

    push({
      id: getLinkId(link),
      pairKey,
      link,
      from: link.from.name,
      to: link.to.name,
      cost: forwardCost,
      workingState: state,
    });

    // Virtual links are point-to-point and directional; a normal WG link is bidirectional.
    push({
      id: getLinkId(link),
      pairKey,
      link,
      from: link.to.name,
      to: link.from.name,
      cost: reverseCost,
      workingState: state,
    });
  });

  return adjacency;
}

interface QueueEntry {
  cost: number;
  node: string;
}

/**
 * Minimal binary min-heap keyed by cost. Ties are broken by node name so the result is
 * deterministic when several equal-cost paths exist.
 */
class CostHeap {
  private items: QueueEntry[] = [];

  get size(): number {
    return this.items.length;
  }

  private less(a: QueueEntry, b: QueueEntry): boolean {
    return a.cost !== b.cost ? a.cost < b.cost : a.node < b.node;
  }

  push(entry: QueueEntry) {
    this.items.push(entry);
    let i = this.items.length - 1;
    while (i > 0) {
      const parent = (i - 1) >> 1;
      if (!this.less(this.items[i], this.items[parent])) break;
      [this.items[i], this.items[parent]] = [this.items[parent], this.items[i]];
      i = parent;
    }
  }

  pop(): QueueEntry | undefined {
    if (this.items.length === 0) return undefined;
    const top = this.items[0];
    const last = this.items.pop()!;
    if (this.items.length > 0) {
      this.items[0] = last;
      let i = 0;
      const len = this.items.length;
      for (;;) {
        const left = i * 2 + 1;
        const right = left + 1;
        let smallest = i;
        if (left < len && this.less(this.items[left], this.items[smallest]))
          smallest = left;
        if (right < len && this.less(this.items[right], this.items[smallest]))
          smallest = right;
        if (smallest === i) break;
        [this.items[i], this.items[smallest]] = [
          this.items[smallest],
          this.items[i],
        ];
        i = smallest;
      }
    }
    return top;
  }
}

/**
 * Dijkstra over the directed link graph, minimising the summed directional link cost.
 *
 * When several paths reach the same destination with an equal cost we prefer the one
 * with fewer hops (BIRD does not do this itself, but it makes the highlighted path stable
 * and readable when all links share the same default cost).
 */
export function findBestPath(
  adjacency: Map<string, DirectedLink[]>,
  from: string,
  to: string,
): DirectedPath | null {
  if (from === to) {
    return { from, to, hops: [{ node: from }], totalCost: 0, edgeIds: [] };
  }

  const distances = new Map<string, number>();
  const hops = new Map<string, number>();
  const previous = new Map<string, { node: string; edge: DirectedLink }>();
  const settled = new Set<string>();

  const heap = new CostHeap();
  distances.set(from, 0);
  hops.set(from, 0);
  heap.push({ cost: 0, node: from });

  while (heap.size > 0) {
    const current = heap.pop()!;
    const currentDistance = distances.get(current.node);
    if (currentDistance === undefined || settled.has(current.node)) continue;
    settled.add(current.node);

    if (current.node === to) break;

    const outbound = adjacency.get(current.node) || [];
    outbound.forEach((edge) => {
      if (settled.has(edge.to)) return;

      const nextCost = currentDistance + edge.cost;
      const nextHops = (hops.get(current.node) ?? 0) + 1;
      const knownCost = distances.get(edge.to);
      const knownHops = hops.get(edge.to) ?? Number.POSITIVE_INFINITY;

      if (
        knownCost === undefined ||
        nextCost < knownCost ||
        (nextCost === knownCost && nextHops < knownHops)
      ) {
        distances.set(edge.to, nextCost);
        hops.set(edge.to, nextHops);
        previous.set(edge.to, { node: current.node, edge });
        heap.push({ cost: nextCost, node: edge.to });
      }
    });
  }

  const totalCost = distances.get(to);
  if (totalCost === undefined) return null;

  // Walk the predecessor chain from the destination back to the source, collecting the
  // node order and the cost of each hop; then reverse into source → destination order.
  const nodeChain: string[] = [];
  const hopCosts: number[] = [];
  let cursor = to;
  for (;;) {
    nodeChain.push(cursor);
    if (cursor === from) break;
    const step = previous.get(cursor);
    if (!step) return null;
    hopCosts.push(step.edge.cost);
    cursor = step.node;
  }
  nodeChain.reverse();
  hopCosts.reverse();

  const pathHops: PathHop[] = nodeChain.map((node, idx) => {
    if (idx === 0) return { node };
    const edge = previous.get(nodeChain[idx])!;
    return { node, viaEdgeId: edge.edge.id, viaCost: hopCosts[idx - 1] };
  });
  const edgeIds = pathHops.slice(1).map((hop) => hop.viaEdgeId!);

  return { from, to, hops: pathHops, totalCost, edgeIds };
}

/**
 * Two directed paths use the same route when they traverse the same physical links in
 * opposite order. A directed link id encodes the link's configured end order (from→to), so
 * comparing raw id sequences is wrong: when a link is configured B→A but traversed A→B, the
 * id is identical in both directions and a symmetric route looks asymmetric. Compare the
 * undirected pair keys instead.
 */
function isMirrorRoute(a: DirectedLink[], b: DirectedLink[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i].pairKey !== b[b.length - 1 - i].pairKey) return false;
  }
  return true;
}

/** Computes the best path in both directions between `a` and `b`. */
export function findBestPathPair(
  adjacency: Map<string, DirectedLink[]>,
  a: string,
  b: string,
): BestPathPair {
  const forward = findBestPath(adjacency, a, b);
  const reverse = findBestPath(adjacency, b, a);

  const forwardSteps = forward ? getPathSteps(adjacency, forward) : [];
  const reverseSteps = reverse ? getPathSteps(adjacency, reverse) : [];

  // Asymmetric means one direction is unreachable, the two directions do not share the same
  // physical links, or an identical route accumulates a different cost per direction.
  const isAsymmetric =
    Boolean(forward) !== Boolean(reverse) ||
    (forward !== null &&
      reverse !== null &&
      (!isMirrorRoute(forwardSteps, reverseSteps) ||
        forward.totalCost !== reverse.totalCost));

  // Node-pair keys drive the canvas filter (always show links on the path), while the
  // per-link direction map drives styling so parallel interfaces are attributed correctly.
  const nodePairKeys = new Set<string>();
  [...forwardSteps, ...reverseSteps].forEach((edge) =>
    nodePairKeys.add(getNodePairKey(edge.from, edge.to)),
  );

  const linkDirections = new Map<string, { forward: boolean; reverse: boolean }>();
  const mark = (edges: DirectedLink[], key: "forward" | "reverse") => {
    edges.forEach((edge) => {
      // A physical link can be traversed either way; both directions map to the same link id.
      const entry = linkDirections.get(edge.id) ?? { forward: false, reverse: false };
      entry[key] = true;
      linkDirections.set(edge.id, entry);
    });
  };
  mark(forwardSteps, "forward");
  mark(reverseSteps, "reverse");

  return { from: a, to: b, forward, reverse, isAsymmetric, nodePairKeys, linkDirections };
}

/**
 * Returns the in-edge for each hop (index 0 has none), i.e. the directed edge traversed to
 * arrive at `path.hops[i].node` from `path.hops[i-1].node`.
 */
export function getPathSteps(
  adjacency: Map<string, DirectedLink[]>,
  path: DirectedPath,
): DirectedLink[] {
  const steps: DirectedLink[] = [];
  for (let i = 1; i < path.hops.length; i++) {
    const edgeId = path.hops[i].viaEdgeId;
    const from = path.hops[i - 1].node;
    const edge = edgeId
      ? (adjacency.get(from) || []).find((e) => e.id === edgeId)
      : undefined;
    if (edge) steps.push(edge);
  }
  return steps;
}
