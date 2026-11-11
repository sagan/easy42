/**
 * Compares two semver-like strings (e.g. "0.1.10", "v0.1.11", "0.1.11-beta", etc.).
 * Returns < 0 if v1 < v2, > 0 if v1 > v2, and 0 if equal.
 */
export function compareVersions(v1: string, v2: string): number {
  const clean1 = (v1 || "").trim().replace(/^[vV]/, "");
  const clean2 = (v2 || "").trim().replace(/^[vV]/, "");

  if (!clean1 && !clean2) return 0;
  if (!clean1) return -1;
  if (!clean2) return 1;

  const parts1 = clean1.split(/[-+]/)[0].split(".").map((n) => parseInt(n, 10) || 0);
  const parts2 = clean2.split(/[-+]/)[0].split(".").map((n) => parseInt(n, 10) || 0);

  const maxLength = Math.max(parts1.length, parts2.length);
  for (let i = 0; i < maxLength; i++) {
    const num1 = parts1[i] ?? 0;
    const num2 = parts2[i] ?? 0;
    if (num1 < num2) return -1;
    if (num1 > num2) return 1;
  }
  return 0;
}

/**
 * Checks whether an agent's version is older than the target (latest bundled) agent version.
 */
export function isAgentVersionOutdated(
  nodeVersion?: string | null,
  targetVersion?: string | null
): boolean {
  if (!nodeVersion || !targetVersion) return false;
  const cleanNode = nodeVersion.trim();
  const cleanTarget = targetVersion.trim();
  if (
    !cleanNode ||
    !cleanTarget ||
    cleanNode === "—" ||
    cleanNode.toLowerCase().includes("not reported") ||
    cleanNode.toLowerCase().includes("unknown")
  ) {
    return false;
  }
  return compareVersions(cleanNode, cleanTarget) < 0;
}
