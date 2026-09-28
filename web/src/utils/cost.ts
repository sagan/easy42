import { LinkEnd, NetworkPolicy } from "../types/api";

/**
 * Returns the effective cost for a LinkEnd based on:
 * 1. LinkEnd's cost override if defined and non-zero
 * 2. Associated NetworkPolicy's cost (if defined and positive)
 * 3. Default link cost (100)
 */
export function getEffectiveLinkCost(
  end: LinkEnd | undefined | null,
  isRemoteExternal: boolean = false,
  policies?: NetworkPolicy[] | Record<string, NetworkPolicy>,
): number {
  if (!end) return 100;

  // 1. Link Cost field override (if defined and non-zero)
  if (end.cost !== undefined && end.cost !== null && (end.cost as unknown) !== "") {
    const num = Number(end.cost);
    if (!isNaN(num) && num !== 0) {
      return num;
    }
  }

  // 2. Determine effective policy ID
  const policyId = end.policy?.trim() || (isRemoteExternal ? "dn42" : "default");

  // 3. Look up policy in provided policies
  let policy: NetworkPolicy | undefined;
  if (Array.isArray(policies)) {
    policy = policies.find((p) => p.id === policyId);
  } else if (policies) {
    policy = policies[policyId];
  }

  // 4. Policy Cost field (if defined and positive)
  if (policy && policy.cost !== undefined && policy.cost !== null && (policy.cost as unknown) !== "") {
    const num = Number(policy.cost);
    if (!isNaN(num) && num > 0) {
      return num;
    }
  }

  // 5. Default link cost fallback
  return 100;
}
