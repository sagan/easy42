import { Node, Link, NetworkPolicy } from "../types/api";
import { getEffectiveLinkCost } from "./cost";

export type CentralityMetric = "betweenness" | "degree";
export type WeightingMode = "weighted" | "unweighted";

export interface GraphCentralityOptions {
  metric?: CentralityMetric;
  weighting?: WeightingMode;
  powerCurve?: number; // gamma exponent for non-linear scale interpolation, default 0.6
  networkPolicies?: NetworkPolicy[];
}

export interface NodeCentralityInfo {
  nodeName: string;
  rawBetweenness: number;
  rawDegree: number;
  normalizedScore: number; // [0, 1] with power curve applied
  isCore: boolean;
  scale: number; // dimension scale multiplier: 1.0 to 1.22
  borderWidth: number; // border thickness: 1.5 to 3.5
  rank: number; // 1-based rank among all nodes
}

export interface LinkCentralityInfo {
  linkId: string;
  canonicalKey: string;
  rawBetweenness: number;
  normalizedScore: number; // [0, 1] with power curve applied
  strokeWidth: number; // edge thickness: 1.8 to 5.5
  opacity: number; // 0.75 to 1.0
  isCore: boolean;
  rank: number; // 1-based rank among all links
}

export interface GraphCentralityResult {
  nodes: Map<string, NodeCentralityInfo>;
  links: Map<string, LinkCentralityInfo>;
  maxNodeBetweenness: number;
  maxLinkBetweenness: number;
  maxDegree: number;
}

/**
 * Returns a unique string identifier for a given Link.
 */
export function getLinkId(link: Link): string {
  return `link-${link.from.name}-${link.from.interface}-${link.to.name}-${link.to.interface}`;
}

/**
 * Returns an undirected pair key for the link.
 */
export function getUndirectedLinkKey(link: Link): string {
  const p1 = `${link.from.name}:${link.from.interface}`;
  const p2 = `${link.to.name}:${link.to.interface}`;
  return p1 < p2 ? `${p1}---${p2}` : `${p2}---${p1}`;
}

/**
 * Binary min-heap for efficient Dijkstra implementation.
 */
class MinHeap<T> {
  private data: { key: number; value: T }[] = [];

  push(key: number, value: T) {
    this.data.push({ key, value });
    this.up(this.data.length - 1);
  }

  pop(): { key: number; value: T } | undefined {
    if (this.data.length === 0) return undefined;
    const top = this.data[0];
    const bottom = this.data.pop()!;
    if (this.data.length > 0) {
      this.data[0] = bottom;
      this.down(0);
    }
    return top;
  }

  get size(): number {
    return this.data.length;
  }

  private up(i: number) {
    while (i > 0) {
      const p = (i - 1) >> 1;
      if (this.data[i].key < this.data[p].key) {
        const tmp = this.data[i];
        this.data[i] = this.data[p];
        this.data[p] = tmp;
        i = p;
      } else {
        break;
      }
    }
  }

  private down(i: number) {
    const len = this.data.length;
    while ((i << 1) + 1 < len) {
      let left = (i << 1) + 1;
      let right = left + 1;
      let smallest = i;

      if (this.data[left].key < this.data[smallest].key) {
        smallest = left;
      }
      if (right < len && this.data[right].key < this.data[smallest].key) {
        smallest = right;
      }
      if (smallest !== i) {
        const tmp = this.data[i];
        this.data[i] = this.data[smallest];
        this.data[smallest] = tmp;
        i = smallest;
      } else {
        break;
      }
    }
  }
}

interface InternalEdge {
  link: Link;
  linkId: string;
  canonicalKey: string;
  from: string;
  to: string;
  weight: number;
}

/**
 * Calculates graph centrality metrics (Betweenness Centrality & Degree Centrality)
 * for nodes and links using Brandes' algorithm.
 */
export function calculateGraphCentrality(
  nodes: Node[],
  links: Link[],
  options: GraphCentralityOptions = {},
): GraphCentralityResult {
  const metric = options.metric || "betweenness";
  const weighting = options.weighting || "weighted";
  const powerCurve = typeof options.powerCurve === "number" ? options.powerCurve : 0.6;
  const networkPolicies = options.networkPolicies;

  const nodeMap = new Map<string, Node>();
  nodes.forEach((n) => nodeMap.set(n.name, n));

  // Build adjacency list for valid links
  const adj = new Map<string, InternalEdge[]>();
  nodes.forEach((n) => adj.set(n.name, []));

  const allInternalEdges: InternalEdge[] = [];
  const linkIdSet = new Set<string>();

  links.forEach((l) => {
    if (!l.from?.name || !l.to?.name) return;
    if (!nodeMap.has(l.from.name) || !nodeMap.has(l.to.name)) return;

    const fromNode = nodeMap.get(l.from.name);
    const toNode = nodeMap.get(l.to.name);
    const fromIsExternal = Boolean(fromNode?.is_external);
    const toIsExternal = Boolean(toNode?.is_external);

    let weightFromTo = 1;
    let weightToFrom = 1;

    if (weighting === "weighted") {
      const fromCost = getEffectiveLinkCost(l.from, toIsExternal, networkPolicies);
      const toCost = getEffectiveLinkCost(l.to, fromIsExternal, networkPolicies);
      weightFromTo = Math.max(1, fromCost);
      weightToFrom = Math.max(1, toCost);
    }

    const linkId = getLinkId(l);
    const canonicalKey = getUndirectedLinkKey(l);
    linkIdSet.add(linkId);

    // Forward direction
    const eFwd: InternalEdge = {
      link: l,
      linkId,
      canonicalKey,
      from: l.from.name,
      to: l.to.name,
      weight: weightFromTo,
    };
    adj.get(l.from.name)?.push(eFwd);

    // Reverse direction (for undirected mesh traversal)
    const eRev: InternalEdge = {
      link: l,
      linkId,
      canonicalKey,
      from: l.to.name,
      to: l.from.name,
      weight: weightToFrom,
    };
    adj.get(l.to.name)?.push(eRev);

    allInternalEdges.push(eFwd);
  });

  const nodeNames = nodes.map((n) => n.name);
  const V = nodeNames.length;

  // Track raw centrality values
  const rawNodeBetweenness = new Map<string, number>();
  const rawLinkBetweenness = new Map<string, number>();
  const rawDegrees = new Map<string, number>();

  nodeNames.forEach((n) => {
    rawNodeBetweenness.set(n, 0);
    rawDegrees.set(n, adj.get(n)?.length || 0);
  });

  links.forEach((l) => {
    rawLinkBetweenness.set(getLinkId(l), 0);
  });

  // Execute Brandes' Algorithm if metric is betweenness or as default
  if (V > 1) {
    nodeNames.forEach((s) => {
      // Single Source Shortest Path
      const S: string[] = []; // Stack of visited nodes in non-decreasing order of distance
      const pred = new Map<string, { u: string; edge: InternalEdge }[]>();
      const sigma = new Map<string, number>();
      const dist = new Map<string, number>();

      nodeNames.forEach((w) => {
        pred.set(w, []);
        sigma.set(w, 0);
        dist.set(w, Infinity);
      });

      sigma.set(s, 1);
      dist.set(s, 0);

      if (weighting === "unweighted") {
        // Standard BFS for unweighted graphs
        const Q: string[] = [s];
        while (Q.length > 0) {
          const v = Q.shift()!;
          S.push(v);

          const edges = adj.get(v) || [];
          for (let i = 0; i < edges.length; i++) {
            const edge = edges[i];
            const w = edge.to;

            // Path discovery
            if (dist.get(w)! === Infinity) {
              dist.set(w, dist.get(v)! + 1);
              Q.push(w);
            }

            // Path counting
            if (dist.get(w)! === dist.get(v)! + 1) {
              sigma.set(w, sigma.get(w)! + sigma.get(v)!);
              pred.get(w)!.push({ u: v, edge });
            }
          }
        }
      } else {
        // Dijkstra with MinHeap for weighted graphs
        const heap = new MinHeap<string>();
        heap.push(0, s);

        while (heap.size > 0) {
          const item = heap.pop()!;
          const v = item.value;
          const d = item.key;

          if (d > dist.get(v)!) continue;
          S.push(v);

          const edges = adj.get(v) || [];
          for (let i = 0; i < edges.length; i++) {
            const edge = edges[i];
            const w = edge.to;
            const newDist = d + edge.weight;

            if (newDist < dist.get(w)!) {
              dist.set(w, newDist);
              sigma.set(w, sigma.get(v)!);
              pred.set(w, [{ u: v, edge }]);
              heap.push(newDist, w);
            } else if (newDist === dist.get(w)!) {
              sigma.set(w, sigma.get(w)! + sigma.get(v)!);
              pred.get(w)!.push({ u: v, edge });
            }
          }
        }
      }

      // Dependency accumulation
      const deltaNode = new Map<string, number>();
      nodeNames.forEach((w) => deltaNode.set(w, 0));

      while (S.length > 0) {
        const w = S.pop()!;
        const sigmaW = sigma.get(w)!;
        const preds = pred.get(w) || [];

        for (let i = 0; i < preds.length; i++) {
          const { u, edge } = preds[i];
          const sigmaU = sigma.get(u)!;
          if (sigmaW > 0) {
            const c = (sigmaU / sigmaW) * (1 + deltaNode.get(w)!);
            deltaNode.set(u, deltaNode.get(u)! + c);

            // Accumulate edge betweenness
            const curEdgeB = rawLinkBetweenness.get(edge.linkId) || 0;
            rawLinkBetweenness.set(edge.linkId, curEdgeB + c);
          }
        }

        if (w !== s) {
          const curB = rawNodeBetweenness.get(w) || 0;
          rawNodeBetweenness.set(w, curB + deltaNode.get(w)!);
        }
      }
    });

    // In undirected traversal, pairs (s, t) are counted in both directions, so divide by 2
    nodeNames.forEach((n) => {
      rawNodeBetweenness.set(n, (rawNodeBetweenness.get(n) || 0) / 2);
    });
    links.forEach((l) => {
      const lid = getLinkId(l);
      rawLinkBetweenness.set(lid, (rawLinkBetweenness.get(lid) || 0) / 2);
    });
  }

  // Find max values for normalization
  let maxNodeBetweenness = 0;
  rawNodeBetweenness.forEach((v) => {
    if (v > maxNodeBetweenness) maxNodeBetweenness = v;
  });

  let maxLinkBetweenness = 0;
  rawLinkBetweenness.forEach((v) => {
    if (v > maxLinkBetweenness) maxLinkBetweenness = v;
  });

  let maxDegree = 0;
  rawDegrees.forEach((v) => {
    if (v > maxDegree) maxDegree = v;
  });

  // Calculate normalized scores and visual profiles
  const nodeResult = new Map<string, NodeCentralityInfo>();

  // Determine node rank based on metric
  const sortedNodes = [...nodeNames].sort((a, b) => {
    const valA = metric === "degree" ? rawDegrees.get(a) || 0 : rawNodeBetweenness.get(a) || 0;
    const valB = metric === "degree" ? rawDegrees.get(b) || 0 : rawNodeBetweenness.get(b) || 0;
    return valB - valA;
  });

  sortedNodes.forEach((nodeName, index) => {
    const bVal = rawNodeBetweenness.get(nodeName) || 0;
    const dVal = rawDegrees.get(nodeName) || 0;

    let baseNorm = 0;
    if (metric === "degree") {
      baseNorm = maxDegree > 0 ? dVal / maxDegree : 0;
    } else {
      baseNorm = maxNodeBetweenness > 0 ? bVal / maxNodeBetweenness : 0;
    }

    // Apply power curve for non-linear scale interpolation
    const normalizedScore = Math.min(1, Math.max(0, Math.pow(baseNorm, powerCurve)));
    const isCore = normalizedScore >= 0.6 || (sortedNodes.length >= 4 && index < Math.max(1, Math.ceil(sortedNodes.length * 0.2)));

    // Scale dimensions: 1.0 (base) to 1.22
    const scale = 1.0 + normalizedScore * 0.22;
    // Border width: 1.5px (base) to 3.5px
    const borderWidth = 1.5 + normalizedScore * 2.0;

    nodeResult.set(nodeName, {
      nodeName,
      rawBetweenness: bVal,
      rawDegree: dVal,
      normalizedScore,
      isCore,
      scale,
      borderWidth,
      rank: index + 1,
    });
  });

  // Determine link rank and properties
  const linkResult = new Map<string, LinkCentralityInfo>();

  const sortedLinks = [...links].sort((a, b) => {
    const bA = rawLinkBetweenness.get(getLinkId(a)) || 0;
    const bB = rawLinkBetweenness.get(getLinkId(b)) || 0;
    return bB - bA;
  });

  sortedLinks.forEach((link, index) => {
    const lid = getLinkId(link);
    const bVal = rawLinkBetweenness.get(lid) || 0;
    const baseNorm = maxLinkBetweenness > 0 ? bVal / maxLinkBetweenness : 0;
    const normalizedScore = Math.min(1, Math.max(0, Math.pow(baseNorm, powerCurve)));
    const isCore = normalizedScore >= 0.5 || (sortedLinks.length >= 4 && index < Math.max(1, Math.ceil(sortedLinks.length * 0.2)));

    // Edge thickness: 1.8px (base) to 5.5px
    const strokeWidth = 1.8 + normalizedScore * 3.7;
    // Opacity: 0.75 (base) to 1.0
    const opacity = 0.75 + normalizedScore * 0.25;

    linkResult.set(lid, {
      linkId: lid,
      canonicalKey: getUndirectedLinkKey(link),
      rawBetweenness: bVal,
      normalizedScore,
      strokeWidth,
      opacity,
      isCore,
      rank: index + 1,
    });
  });

  return {
    nodes: nodeResult,
    links: linkResult,
    maxNodeBetweenness,
    maxLinkBetweenness,
    maxDegree,
  };
}
