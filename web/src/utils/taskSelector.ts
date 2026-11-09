import { Node } from "../types/api";

export type SelectorTargetMode = "manual" | "all" | "tag" | "custom";

export function getSelectorValue(selector: Record<string, string>, key: string): string {
  const target = key.trim().toLowerCase();
  const entry = Object.entries(selector).find(([k]) => k.trim().toLowerCase() === target);
  return entry ? entry[1] : "";
}

export function detectTargetMode(selector: Record<string, string>): SelectorTargetMode {
  const keys = Object.keys(selector).map((k) => k.trim().toLowerCase());
  if (keys.length === 0) return "all";
  if (keys.length === 1 && keys[0] === "name") return "manual";
  if (keys.length === 1 && keys[0] === "tag") return "tag";
  return "custom";
}

export function parseSelectorJSON(text: string): { selector: Record<string, string> | null; error: string | null } {
  const trimmed = text.trim();
  if (!trimmed) return { selector: {}, error: null };

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (e: any) {
    return { selector: null, error: `Invalid selector JSON: ${e.message || e}` };
  }

  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    return { selector: null, error: "Selector must be a JSON object" };
  }

  const selector: Record<string, string> = {};
  for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
    if (typeof value !== "string") {
      return { selector: null, error: `Selector value for "${key}" must be a string` };
    }
    selector[key] = value;
  }
  return { selector, error: null };
}

/**
 * Split a comma-separated selector value into trimmed, non-empty items.
 */
export function splitSelectorList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item !== "");
}

function cleanCIDR(ip?: string): string {
  if (!ip) return "";
  const trimmed = ip.trim();
  const slash = trimmed.indexOf("/");
  return slash === -1 ? trimmed : trimmed.slice(0, slash).trim();
}

function sanitizeEnvKey(key: string): string {
  return key.toUpperCase().replace(/[^A-Z0-9_]/g, "_");
}

function formatEnvValue(value: unknown): string | null {
  if (typeof value === "string") return value;
  if (typeof value === "boolean") return value ? "true" : "false";
  if (typeof value === "number") {
    if (Number.isFinite(value) && Number.isInteger(value)) return String(value);
    return String(value);
  }
  return null;
}

/**
 * Extract structured JSON data from the first ``` or ```json fenced block in a
 * node note. Mirrors the backend config.ExtractNoteData implementation.
 *
 * Unlike extractNoteData, returns null when the note contains no valid
 * structured data object (no ``` / ```json block, an empty or unparseable
 * block, or a block that does not contain a JSON object), so callers can
 * distinguish a valid empty object ({}) from the absence of structured data.
 */
export function parseNoteData(note?: string | null): Record<string, unknown> | null {
  const trimmed = (note || "").trim();
  if (!trimmed) return null;

  const regex = /```+([a-zA-Z0-9_-]*)?[ \t]*(?:\r?\n)?([\s\S]*?)```+/gi;
  let match: RegExpExecArray | null;
  while ((match = regex.exec(note || "")) !== null) {
    const lang = (match[1] || "").trim().toLowerCase();
    if (lang !== "" && lang !== "json") continue;

    const body = (match[2] || "").trim();
    if (!body) return null;
    try {
      const parsed = JSON.parse(body);
      if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>;
      }
    } catch {
      // Best effort: the first ``` or ```json block could not be parsed.
    }
    return null;
  }
  return null;
}

/**
 * Extract structured JSON data from the first ``` or ```json fenced block in a
 * node note. Mirrors the backend config.ExtractNoteData implementation.
 * Returns an empty object when no valid structured data object is present.
 */
export function extractNoteData(note?: string | null): Record<string, unknown> {
  return parseNoteData(note) ?? {};
}

/**
 * Build the recognized NODE_DATA_* environment variables for already-parsed
 * structured note data. Only string, number, and boolean fields are exposed,
 * up to one nested object level; nested fields also get a NODE_DATA_<child>
 * alias when that name is not already taken by a top-level field.
 */
export function buildNoteDataEnvVars(noteData: Record<string, unknown>): Record<string, string> {
  const res: Record<string, string> = {};

  // Layer 0 primitive fields
  Object.entries(noteData).forEach(([key, value]) => {
    const s0 = sanitizeEnvKey(key);
    if (!s0) return;
    const formatted = formatEnvValue(value);
    if (formatted !== null) res[`NODE_DATA_${s0}`] = formatted;
  });

  // Layer 1 nested object fields
  Object.entries(noteData).forEach(([key, value]) => {
    const s0 = sanitizeEnvKey(key);
    if (!s0) return;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      Object.entries(value as Record<string, unknown>).forEach(([childKey, childValue]) => {
        const s1 = sanitizeEnvKey(childKey);
        if (!s1) return;
        const formatted = formatEnvValue(childValue);
        if (formatted !== null) {
          res[`NODE_DATA_${s0}_${s1}`] = formatted;
          const aliasKey = `NODE_DATA_${s1}`;
          if (!(aliasKey in res)) res[aliasKey] = formatted;
        }
      });
    }
  });

  return res;
}

/**
 * Build the recognized NODE_DATA_* environment variables from a node note.
 * Returns an empty object when the note contains no valid structured data.
 */
export function getNoteDataEnvVars(note?: string | null): Record<string, string> {
  const noteData = parseNoteData(note);
  if (!noteData) return {};
  return buildNoteDataEnvVars(noteData);
}

/**
 * Build the node context variables used by Device Helper tasks. Mirrors the
 * backend apps.BuildNodeContextVars implementation so that selector matching is
 * consistent between the UI and the server.
 */
export function buildNodeContextVars(node: Node): Record<string, string> {
  const res: Record<string, string> = {};
  const name = (node.name || "").trim();
  const iface = (node.interface || "").trim();
  const nodeIP = cleanCIDR(node.ip);
  const nodeIP6 = cleanCIDR(node.ip6);
  const nodeExtIP = cleanCIDR(node.external_ip);
  const nodeExtIP6 = cleanCIDR(node.external_ip6);
  const customScript = (node.custom_network_script || "").trim();

  res.NODE_NAME = name;
  res.NODE_INTERFACE = iface;
  res.NODE_IP = nodeIP;
  res.NODE_IP6 = nodeIP6;
  res.NODE_IPV4 = nodeIP;
  res.NODE_IPV6 = nodeIP6;

  if (nodeExtIP !== "" && nodeExtIP !== nodeIP) res.NODE_EXTERNAL_IP = nodeExtIP;
  if (nodeExtIP6 !== "" && nodeExtIP6 !== nodeIP6) res.NODE_EXTERNAL_IP6 = nodeExtIP6;

  res.HAS_CUSTOM_SCRIPT = customScript !== "" ? "1" : "0";

  const cleanTags: string[] = [];
  const seenTags = new Set<string>();
  (node.tags || []).forEach((tag) => {
    const trimmed = (tag || "").trim();
    if (trimmed && !seenTags.has(trimmed)) {
      seenTags.add(trimmed);
      cleanTags.push(trimmed);
    }
  });
  cleanTags.sort();
  res.NODE_TAGS = cleanTags.join(",");
  cleanTags.forEach((tag) => {
    const tagKey = sanitizeEnvKey(tag);
    if (tagKey) res[`NODE_TAG_${tagKey}`] = "1";
  });

  Object.assign(res, getNoteDataEnvVars(node.note));

  return res;
}

/**
 * Returns true when the node satisfies every condition in the task selector.
 * An empty or missing selector matches every node.
 *
 * Supported keys:
 * - name: comma-separated node name whitelist.
 * - tag: comma-separated tag whitelist; the node must have at least one tag.
 * - env:<NAME>: required node metadata value (e.g. NODE_IP, NODE_IP6,
 *   NODE_DATA_*).
 */
export function matchesTaskSelector(node: Node, selector?: Record<string, string> | null): boolean {
  if (!selector) return true;
  const entries = Object.entries(selector);
  if (entries.length === 0) return true;

  const contextVars = buildNodeContextVars(node);
  const nodeName = (node.name || "").trim();
  const nodeTags = (node.tags || []).map((tag) => (tag || "").trim()).filter((tag) => tag !== "");

  for (const [rawKey, rawValue] of entries) {
    const key = (rawKey || "").trim();
    if (!key) return false;
    const lowerKey = key.toLowerCase();

    if (lowerKey === "name") {
      const items = splitSelectorList(rawValue);
      if (items.length === 0 || !items.includes(nodeName)) return false;
    } else if (lowerKey === "tag") {
      const items = splitSelectorList(rawValue);
      if (items.length === 0 || !items.some((item) => nodeTags.includes(item))) return false;
    } else if (lowerKey.startsWith("env:")) {
      const envKey = key.slice("env:".length).trim().toUpperCase();
      if (!envKey) return false;
      const actual = contextVars[envKey];
      if (actual === undefined || actual !== rawValue) return false;
    } else {
      // Unknown selector keys are rejected (fail closed), matching the backend.
      return false;
    }
  }

  return true;
}
