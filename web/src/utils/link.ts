import { Link } from "../types/api";

/**
 * Checks whether two Link objects represent the same network link,
 * matching either from->to or to->from and taking interface names into account.
 */
export function isSameLink(a?: Link | null, b?: Link | null): boolean {
  if (!a || !b) return false;
  if (a === b) return true;

  // Case 1: Same direction
  if (a.from?.name === b.from?.name && a.to?.name === b.to?.name) {
    if (a.from?.interface && b.from?.interface && a.from.interface !== b.from.interface) {
      return false;
    }
    if (a.to?.interface && b.to?.interface && a.to.interface !== b.to.interface) {
      return false;
    }
    return true;
  }

  // Case 2: Inverted direction
  if (a.from?.name === b.to?.name && a.to?.name === b.from?.name) {
    if (a.from?.interface && b.to?.interface && a.from.interface !== b.to.interface) {
      return false;
    }
    if (a.to?.interface && b.from?.interface && a.to.interface !== b.from.interface) {
      return false;
    }
    return true;
  }

  return false;
}

/**
 * Helper to determine whether a link is manual (BGP-only / non-WireGuard data plane).
 */
export function isManualLink(
  link?: {
    type?: string;
    from?: { type?: string; interface?: string };
    to?: { type?: string; interface?: string };
    tags?: string[];
  } | null,
): boolean {
  if (!link) return false;
  // 1. Explicit link type
  const lType = link.type?.toLowerCase().trim();
  if (lType === "manual") return true;
  if (lType === "wireguard") return false;

  // 2. Explicit endpoint type
  const fromType = link.from?.type?.toLowerCase().trim();
  const toType = link.to?.type?.toLowerCase().trim();
  if (fromType === "manual" || toType === "manual") return true;
  if (fromType === "wireguard" && toType === "wireguard") return false;

  // 3. Tags containing "manual"
  if (link.tags && link.tags.some((t) => t.toLowerCase().trim() === "manual")) return true;

  // 4. In easy42, all managed WireGuard interfaces start with "wg".
  // If interface is specified and explicitly doesn't start with "wg", treat as manual only when type is unspecified.
  const fromIface = link.from?.interface?.toLowerCase().trim();
  const toIface = link.to?.interface?.toLowerCase().trim();
  if (fromIface && !fromIface.startsWith("wg")) return true;
  if (toIface && !toIface.startsWith("wg")) return true;

  return false;
}

/**
 * Helper to determine whether a link is a WireGuard link (managed data plane).
 * Manual links and non-WireGuard links are excluded.
 */
export function isWireGuardLink(
  link?: {
    type?: string;
    from?: { type?: string; interface?: string };
    to?: { type?: string; interface?: string };
    tags?: string[];
  } | null,
): boolean {
  if (!link) return false;
  if (isManualLink(link)) return false;
  const lType = link.type?.toLowerCase().trim();
  if (lType && lType !== "wireguard") return false;
  const fromType = link.from?.type?.toLowerCase().trim();
  if (fromType && fromType !== "wireguard") return false;
  const toType = link.to?.type?.toLowerCase().trim();
  if (toType && toType !== "wireguard") return false;
  return true;
}

