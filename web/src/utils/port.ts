/**
 * Derives default WireGuard listen port based on peer IP:
 * 20000 + hash(peer_ip) % 10000 (range 20000 - 29999).
 * Uses 32-bit FNV-1a hash to distribute ports evenly and match the backend implementation.
 */
export function derivePortFromIP(ip?: string): number {
  if (!ip) return 20000;
  const clean = ip.trim();
  if (!clean) return 20000;

  let hash = 2166136261;
  for (let i = 0; i < clean.length; i++) {
    hash ^= clean.charCodeAt(i);
    hash = Math.imul(hash, 16777619) >>> 0;
  }
  return 20000 + (hash % 10000);
}
