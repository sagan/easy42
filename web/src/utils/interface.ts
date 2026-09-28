/**
 * Returns the standard wg42<peer_name><suffix> interface name for internal peers,
 * or wg42-<peer_name><suffix> for external peers, truncating and sanitizing to adhere to Linux's 15-char limit.
 *
 * If the truncated result ends with an invalid character such as "-", it is automatically converted
 * to a valid name in a deterministic way (by trimming trailing hyphens/underscores).
 */
export function getInterfaceNameWithSuffix(
  peerName: string,
  suffix: string = "",
  isExternal: boolean = false,
): string {
  const cleanName = (peerName || "").trim();
  const prefix = isExternal ? "wg42-" : "wg42";

  let maxPeerLen = 15 - prefix.length - suffix.length;
  if (maxPeerLen < 0) {
    maxPeerLen = 0;
  }

  let truncatedPeer = cleanName;
  if (truncatedPeer.length > maxPeerLen) {
    truncatedPeer = truncatedPeer.slice(0, maxPeerLen);
  }

  // Truncated interface name must be valid (cannot end with "-" or "_")
  if (!suffix) {
    truncatedPeer = truncatedPeer.replace(/[-_]+$/, "");
  }

  if (!truncatedPeer && !suffix) {
    truncatedPeer = isExternal ? "peer" : "node";
  }

  let res = `${prefix}${truncatedPeer}${suffix}`;
  if (res.length > 15) {
    res = res.slice(0, 15);
  }

  while (res.length > prefix.length && (res.endsWith("-") || res.endsWith("_"))) {
    res = res.replace(/[-_]+$/, "");
  }

  return res;
}

/**
 * Returns the standard wg42<peer_name> interface name for internal peers,
 * or wg42-<peer_name> for external peers.
 */
export function getInterfaceName(peerName: string, isExternal: boolean = false): string {
  return getInterfaceNameWithSuffix(peerName, "", isExternal);
}
