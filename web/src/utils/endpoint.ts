import { Node, Entrypoint } from "../types/api";

export interface ResolvedPeerEndpoint {
  entrypoint?: Entrypoint;
  matchedTag?: string;
}

/**
 * Resolves the entrypoint of nodeTo to be used by nodeFrom.
 * Matches internal node linking mechanism:
 * 1. Find a pair of entrypoints between nodeFrom and nodeTo that share a tag (case-insensitive).
 * 2. Fallback to the first non-none (non-empty IP) entrypoint of nodeTo.
 */
export function resolvePeerEntrypoint(
  nodeFrom?: Node | null,
  nodeTo?: Node | null,
): ResolvedPeerEndpoint {
  if (!nodeTo || !nodeTo.entrypoints || nodeTo.entrypoints.length === 0) {
    return {};
  }

  // 1. Try to find matching tags between entrypoints
  if (nodeFrom && nodeFrom.entrypoints) {
    for (const epFrom of nodeFrom.entrypoints) {
      if (!epFrom.tags) continue;
      for (const tagFrom of epFrom.tags) {
        const trimmedFrom = tagFrom.trim().toLowerCase();
        if (!trimmedFrom) continue;
        for (const epTo of nodeTo.entrypoints) {
          if (!epTo.ip || epTo.ip.trim() === "" || !epTo.tags) continue;
          for (const tagTo of epTo.tags) {
            if (trimmedFrom === tagTo.trim().toLowerCase()) {
              return { entrypoint: epTo, matchedTag: tagFrom.trim() };
            }
          }
        }
      }
    }
  }

  // 1b. Try matching node-level tags of nodeFrom
  if (nodeFrom && nodeFrom.tags) {
    for (const tagFrom of nodeFrom.tags) {
      const trimmedFrom = tagFrom.trim().toLowerCase();
      if (!trimmedFrom) continue;
      for (const epTo of nodeTo.entrypoints) {
        if (!epTo.ip || epTo.ip.trim() === "" || !epTo.tags) continue;
        for (const tagTo of epTo.tags) {
          if (trimmedFrom === tagTo.trim().toLowerCase()) {
            return { entrypoint: epTo, matchedTag: tagFrom.trim() };
          }
        }
      }
    }
  }

  // 2. Fallback to first non-none endpoint of nodeTo
  for (const epTo of nodeTo.entrypoints) {
    if (epTo.ip && epTo.ip.trim() !== "") {
      return { entrypoint: epTo };
    }
  }

  return {};
}

/**
 * Parses port number from an endpoint string (e.g. "1.2.3.4:51820" or "[2001:db8::1]:51820" or "peer.com:51820").
 */
export function extractPort(endpointStr?: string): number | undefined {
  if (!endpointStr) return undefined;
  const s = endpointStr.trim();
  const lastColon = s.lastIndexOf(":");
  if (lastColon === -1) return undefined;

  // Make sure the colon is after any closing bracket for IPv6
  const closeBracket = s.lastIndexOf("]");
  if (closeBracket !== -1 && lastColon < closeBracket) {
    return undefined;
  }

  const portPart = s.substring(lastColon + 1);
  const p = parseInt(portPart, 10);
  return isNaN(p) || p <= 0 ? undefined : p;
}

/**
 * Formats host/ip and port into standard WireGuard endpoint string.
 */
export function formatEndpoint(ipOrHost?: string, port?: number): string {
  if (!ipOrHost || !ipOrHost.trim() || !port || port <= 0) return "";
  const clean = ipOrHost.trim();
  if (clean.includes(":") && !clean.startsWith("[")) {
    return `[${clean}]:${port}`;
  }
  return `${clean}:${port}`;
}
