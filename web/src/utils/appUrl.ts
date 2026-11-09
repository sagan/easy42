import type { App, AppVariable, Node } from "../types/api";
import { buildNodeContextVars, matchesTaskSelector } from "./taskSelector";

/**
 * Frontend mirror of the backend apps.InterpolateVariables behaviour
 * (internal/apps/interpolate.go) so homepage URLs can be resolved
 * without a network round-trip.
 */

export function isHttpUrl(url?: string): boolean {
  if (!url) return false;
  return /^https?:\/\//i.test(url.trim());
}

export function getMatchingNodes(app: App, nodes: Node[]): Node[] {
  return nodes.filter((n) => matchesTaskSelector(n, app.selector));
}

export interface ResolvedHomepage {
  /** Interpolated URL (or template when vars remain unresolved). */
  url: string;
  /** The single deployed node. */
  node: Node;
  /** True when interpolation changed the string. */
  isResolved: boolean;
  /** True when `${...}` / `{{...}}` placeholders remain after interpolation. */
  containsTemplate: boolean;
}

/**
 * Resolve `app.url` against the single node the app is deployed to.
 * Returns null when the app has no URL or is not deployed to exactly one node.
 */
export function resolveHomepageUrl(app: App, nodes: Node[], variables: AppVariable[]): ResolvedHomepage | null {
  const raw = (app.url || "").trim();
  if (!raw) return null;
  const matching = getMatchingNodes(app, nodes);
  if (matching.length !== 1) return null;
  const node = matching[0];
  const contextVars = buildNodeContextVars(node);
  contextVars["APP_NAME"] = app.name;
  const globalVars: Record<string, string> = {};
  for (const v of variables || []) {
    const k = (v.key || "").trim();
    if (k) globalVars[k] = v.value ?? "";
  }
  const resolved = interpolateVariables(raw, globalVars, contextVars);
  return {
    url: resolved,
    node,
    isResolved: resolved !== raw,
    containsTemplate: /\$\{|\{\{/.test(resolved),
  };
}

const MUSTACHE_RE = /\{\{\s*\.?([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}/g;

type LookupResult = { found: boolean; value: string };

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function globMatches(value: string, pattern: string): boolean {
  const re = new RegExp(`^${escapeRegExp(pattern).replace(/\\\*/g, ".*").replace(/\\\?/g, ".")}$`);
  return re.test(value);
}

function removePrefix(value: string, pattern: string, longest: boolean): string {
  if (pattern === "") return value;
  if (!pattern.includes("*") && !pattern.includes("?")) {
    return value.startsWith(pattern) ? value.slice(pattern.length) : value;
  }
  const lengths: number[] = [];
  for (let len = 0; len <= value.length; len++) lengths.push(len);
  if (longest) lengths.reverse();
  for (const len of lengths) {
    if (globMatches(value.slice(0, len), pattern)) return value.slice(len);
  }
  return value;
}

function removeSuffix(value: string, pattern: string, longest: boolean): string {
  if (pattern === "") return value;
  if (!pattern.includes("*") && !pattern.includes("?")) {
    return value.endsWith(pattern) ? value.slice(0, value.length - pattern.length) : value;
  }
  const lengths: number[] = [];
  for (let len = 0; len <= value.length; len++) lengths.push(len);
  if (longest) lengths.reverse();
  for (const len of lengths) {
    if (globMatches(value.slice(value.length - len), pattern)) {
      return value.slice(0, value.length - len);
    }
  }
  return value;
}

function findMatchingBrace(s: string, start: number): number {
  // start points at '$' of '${'
  let depth = 1;
  let p = start + 2;
  while (p < s.length) {
    if (s.startsWith("${", p)) {
      depth++;
      p += 2;
      continue;
    }
    if (s[p] === "}") {
      depth--;
      if (depth === 0) return p;
    }
    p++;
  }
  return -1;
}

/**
 * Expand `${...}` / `{{...}}` / `$VAR` placeholders using contextVars first,
 * then globalVars (matching the backend lookup order). Unknown variables
 * without a default/operator are preserved literally (e.g. `${UNKNOWN}`),
 * matching the backend's envsubst escaping.
 */
export function interpolateVariables(
  content: string,
  globalVars: Record<string, string> = {},
  contextVars: Record<string, string> = {},
): string {
  if (!content) return "";

  const lookup = (key: string): LookupResult => {
    if (contextVars && Object.prototype.hasOwnProperty.call(contextVars, key)) {
      return { found: true, value: contextVars[key] };
    }
    if (globalVars && Object.prototype.hasOwnProperty.call(globalVars, key)) {
      return { found: true, value: globalVars[key] };
    }
    return { found: false, value: "" };
  };

  const expand = (s: string, depth: number): string => {
    if (depth > 32) return s;
    let out = "";
    let i = 0;
    while (i < s.length) {
      // Escaped dollar: $${FOO} -> literal ${FOO}
      if (s.startsWith("$${", i)) {
        out += "${";
        i += 3;
        continue;
      }
      if (s.startsWith("${", i)) {
        const end = findMatchingBrace(s, i);
        if (end === -1) {
          out += s[i];
          i++;
          continue;
        }
        const inner = s.slice(i + 2, end);
        out += evalBraced(inner, depth);
        i = end + 1;
        continue;
      }
      if (s[i] === "$" && i + 1 < s.length && /[A-Za-z_]/.test(s[i + 1])) {
        let j = i + 1;
        while (j < s.length && /[A-Za-z0-9_]/.test(s[j])) j++;
        const name = s.slice(i + 1, j);
        const res = lookup(name);
        out += res.found ? res.value : s.slice(i, j);
        i = j;
        continue;
      }
      out += s[i];
      i++;
    }
    return out;
  };

  const evalBraced = (inner: string, depth: number): string => {
    const literal = `\${${inner}}`;
    const m = inner.match(/^([A-Za-z_][A-Za-z0-9_]*)([\s\S]*)$/);
    if (!m) return literal;
    const name = m[1];
    const rest = m[2];
    const res = lookup(name);
    const expandArg = (arg: string) => expand(arg, depth + 1);

    if (rest === "") {
      return res.found ? res.value : literal;
    }

    // Default / alternate / error operators (order matters: :- before -)
    if (rest.startsWith(":-")) {
      return !res.found || res.value === "" ? expandArg(rest.slice(2)) : res.value;
    }
    if (rest.startsWith("-")) {
      return !res.found ? expandArg(rest.slice(1)) : res.value;
    }
    if (rest.startsWith(":=")) {
      return !res.found || res.value === "" ? expandArg(rest.slice(2)) : res.value;
    }
    if (rest.startsWith("=")) {
      return !res.found ? expandArg(rest.slice(1)) : res.value;
    }
    if (rest.startsWith(":+")) {
      return res.found && res.value !== "" ? expandArg(rest.slice(2)) : "";
    }
    if (rest.startsWith("+")) {
      return res.found ? expandArg(rest.slice(1)) : "";
    }
    if (rest.startsWith(":?")) {
      if (!res.found || res.value === "") {
        const arg = rest.slice(2);
        return arg ? expandArg(arg) : literal;
      }
      return res.value;
    }
    if (rest.startsWith("?")) {
      if (!res.found) {
        const arg = rest.slice(1);
        return arg ? expandArg(arg) : literal;
      }
      return res.value;
    }

    // Case conversion
    if (rest.startsWith("^^")) {
      return res.found ? res.value.toUpperCase() : literal;
    }
    if (rest.startsWith("^")) {
      return res.found ? res.value.charAt(0).toUpperCase() + res.value.slice(1) : literal;
    }
    if (rest.startsWith(",,")) {
      return res.found ? res.value.toLowerCase() : literal;
    }
    if (rest.startsWith(",")) {
      return res.found ? res.value.charAt(0).toLowerCase() + res.value.slice(1) : literal;
    }

    // Substring ${VAR:offset} / ${VAR:offset:length} (colon not part of :- etc.)
    if (rest.startsWith(":") && (rest.length === 1 || /\d/.test(rest[1]) || rest[1] === " " || rest[1] === "-")) {
      const body = rest.slice(1).trim();
      if (/^-?\d+(:-?\d+)?$/.test(body)) {
        if (!res.found) return literal;
        const parts = body.split(":").map((p) => parseInt(p, 10));
        const offset = parts[0];
        const len = res.value.length;
        const start = offset >= 0 ? offset : Math.max(0, len + offset);
        if (parts.length === 1) return res.value.slice(start);
        const length = parts[1];
        if (length >= 0) return res.value.slice(start, start + length);
        return res.value.slice(start, Math.max(start, len + length));
      }
      return literal;
    }

    // Pattern substitution ${VAR/pat/rep} / ${VAR//pat/rep}
    if (rest.startsWith("/")) {
      if (!res.found) return literal;
      const isGlobal = rest.startsWith("//");
      const patternPart = isGlobal ? rest.slice(2) : rest.slice(1);
      const slash = patternPart.indexOf("/");
      const pat = slash === -1 ? patternPart : patternPart.slice(0, slash);
      const rep = slash === -1 ? "" : expandArg(patternPart.slice(slash + 1));
      if (pat === "") return res.value;
      if (isGlobal) return res.value.split(pat).join(rep);
      const idx = res.value.indexOf(pat);
      if (idx === -1) return res.value;
      return res.value.slice(0, idx) + rep + res.value.slice(idx + pat.length);
    }

    // Suffix / prefix removal
    if (rest.startsWith("%%")) {
      return res.found ? removeSuffix(res.value, rest.slice(2), true) : literal;
    }
    if (rest.startsWith("%")) {
      return res.found ? removeSuffix(res.value, rest.slice(1), false) : literal;
    }
    if (rest.startsWith("##")) {
      return res.found ? removePrefix(res.value, rest.slice(2), true) : literal;
    }
    if (rest.startsWith("#")) {
      return res.found ? removePrefix(res.value, rest.slice(1), false) : literal;
    }

    // @ transforms (subset: U/L/u/l/Q)
    if (rest.startsWith("@")) {
      if (!res.found) return literal;
      const op = rest.slice(1);
      if (op === "U") return res.value.toUpperCase();
      if (op === "L") return res.value.toLowerCase();
      if (op === "u") return res.value.charAt(0).toUpperCase() + res.value.slice(1);
      if (op === "l") return res.value.charAt(0).toLowerCase() + res.value.slice(1);
      if (op === "Q") return `'${res.value.replace(/'/g, "'\\''")}'`;
      return res.value;
    }

    return literal;
  };

  // 1. {{VAR}} / {{.VAR}} mustache (matches backend step 1)
  const afterMustache = content.replace(MUSTACHE_RE, (match, key: string) => {
    const res = lookup(key);
    return res.found ? res.value : match;
  });

  // 2. ${VAR} + bash expansions
  return expand(afterMustache, 0);
}
