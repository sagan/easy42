/**
 * DN42 Network Utilities
 *
 * Standard DN42 ASN Range: 4242420000 – 4242429999
 */

export const DN42_ASN_MIN = 4242420000;
export const DN42_ASN_MAX = 4242429999;

/**
 * Checks if a given ASN falls within the standard DN42 ASN allocation range.
 */
export function isDn42Asn(asn?: number | string | null): boolean {
  if (asn === undefined || asn === null) return false;
  const num = typeof asn === "number" ? asn : parseInt(String(asn).trim(), 10);
  return !isNaN(num) && num >= DN42_ASN_MIN && num <= DN42_ASN_MAX;
}

/**
 * Checks if a node is an external node and belongs to the DN42 network.
 */
export function isDn42ExternalNode(node?: { is_external?: boolean; asn?: number | string | null } | null): boolean {
  if (!node || !node.is_external) return false;
  return isDn42Asn(node.asn);
}

export interface Dn42ResourceLink {
  id: string;
  name: string;
  provider?: string;
  url: string;
  description: string;
}

/**
 * Builds the list of DN42 lookup and query links for a specified DN42 ASN.
 */
export function getDn42ResourceLinks(asn: number | string): Dn42ResourceLink[] {
  const cleanAsn = String(asn).trim();
  return [
    {
      id: "official-registry",
      name: "DN42 official registry explorer",
      url: `https://explorer.dn42.dev/#/${cleanAsn}`,
      description: "Official registry database, aut-num, route objects & WHOIS metadata",
    },
    {
      id: "iedon-map",
      name: "DN42 Realtime Network Map",
      provider: "provided by iedon",
      url: `https://map.iedon.net/#${cleanAsn}`,
      description: "Interactive real-time global topology and BGP routing visualization",
    },
    {
      id: "nedifinita-explorer",
      name: "DN42 Explorer",
      provider: "provided by nedifinita",
      url: `https://explorer.nedifinita.com/#/as/${cleanAsn}`,
      description: "Route collector, looking glass & historical network statistics",
    },
  ];
}
