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
