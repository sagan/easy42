/**
 * SSH public key fingerprint helpers.
 *
 * OpenSSH shows fingerprints as the digest of the *key blob* (the base64 body of
 * a public key line, i.e. the wire-format key without algorithm prefix or
 * comment), rendered as `hash:base64` or `hash:hex`. We reproduce the three
 * formats users encounter in the wild:
 *
 * - `MD5:<hex>`     — legacy format still printed by older tooling; OpenSSH
 *                     renders it colon-separated and uppercase.
 * - `SHA1:<base64>` — used by some services and older `known_hosts` workflows.
 * - `SHA256:<base64>` — the OpenSSH default since 6.8.
 *
 * Base64 and hex carry the same digest; the UI shows both so a user can compare
 * against either representation without converting by hand.
 *
 * The OpenSSH base64 form is **unpadded** (`sshkey_fingerprint()` prints 43
 * characters for a 32-byte digest). This matters: `ssh(1)` accepts the pasted
 * fingerprint as a synonym for "yes" by exact, case-sensitive comparison, so a
 * padded string is rejected at the "Are you sure you want to continue
 * connecting?" prompt even though it decodes to the same bytes.
 */

const BASE64_ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/**
 * Encode bytes as base64.
 *
 * `pad` controls the trailing `=` characters. OpenSSH fingerprints are emitted
 * **unpadded**: `sshkey_fingerprint()` produces `SHA256:` plus 43 characters for
 * a 32-byte digest, and `ssh(1)` compares the value typed at the host key prompt
 * with a case-sensitive `strcmp`. A padded 44-character string is therefore
 * rejected even though it decodes to the same digest.
 */
function toBase64(bytes: Uint8Array, pad = true): string {
  let out = "";
  let i = 0;
  for (; i + 2 < bytes.length; i += 3) {
    const triple = (bytes[i] << 16) | (bytes[i + 1] << 8) | bytes[i + 2];
    out += BASE64_ALPHABET[(triple >> 18) & 0x3f];
    out += BASE64_ALPHABET[(triple >> 12) & 0x3f];
    out += BASE64_ALPHABET[(triple >> 6) & 0x3f];
    out += BASE64_ALPHABET[triple & 0x3f];
  }
  if (i < bytes.length) {
    const remaining = bytes.length - i;
    const triple = (bytes[i] << 16) | ((remaining > 1 ? bytes[i + 1] : 0) << 8);
    out += BASE64_ALPHABET[(triple >> 18) & 0x3f];
    out += BASE64_ALPHABET[(triple >> 12) & 0x3f];
    out += remaining > 1 ? BASE64_ALPHABET[(triple >> 6) & 0x3f] : pad ? "=" : "";
    if (pad) out += "=";
  }
  return out;
}

/** OpenSSH-style unpadded base64, as printed by `ssh-keygen -l`. */
function toOpenSSHBase64(bytes: Uint8Array): string {
  return toBase64(bytes, false);
}

function toHex(bytes: Uint8Array): string {
  let out = "";
  for (const b of bytes) out += b.toString(16).padStart(2, "0");
  return out;
}

/** Decode base64 (padding optional) into bytes; returns null on bad input. */
export function base64ToBytes(input: string): Uint8Array | null {
  const cleaned = (input || "").trim().replace(/\s+/g, "");
  if (!cleaned) return new Uint8Array();
  const padded = cleaned.padEnd(cleaned.length + ((4 - (cleaned.length % 4)) % 4), "=");
  const lookup = new Map<string, number>();
  for (let i = 0; i < BASE64_ALPHABET.length; i++) lookup.set(BASE64_ALPHABET[i], i);

  const bytes: number[] = [];
  for (let i = 0; i < padded.length; i += 4) {
    const chunk = padded.slice(i, i + 4);
    if (!/^[A-Za-z0-9+/]{2}[A-Za-z0-9+/=]{2}$/.test(chunk)) return null;
    const v0 = lookup.get(chunk[0]);
    const v1 = lookup.get(chunk[1]);
    if (v0 === undefined || v1 === undefined) return null;
    const pad2 = chunk[2] === "=";
    const pad3 = chunk[3] === "=";
    const v2 = pad2 ? 0 : lookup.get(chunk[2]);
    const v3 = pad3 ? 0 : lookup.get(chunk[3]);
    if (v2 === undefined || v3 === undefined) return null;
    bytes.push((v0 << 2) | (v1 >> 4));
    if (!pad2) bytes.push(((v1 & 0x0f) << 4) | (v2 >> 2));
    if (!pad3) bytes.push(((v2 & 0x03) << 6) | v3);
  }
  return new Uint8Array(bytes);
}

// --- MD5 -------------------------------------------------------------------
// WebCrypto deliberately does not provide MD5, but the legacy SSH fingerprint
// format is MD5-based, so it is implemented here. This is used only to display a
// well-known fingerprint format; it is never a security primitive.

const MD5_S = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 4,
  11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11, 16, 23, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21,
];

const MD5_K = Array.from({ length: 64 }, (_, i) => Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32) >>> 0);

/** Compute the MD5 digest of `input`. */
export function md5(input: Uint8Array): Uint8Array {
  const originalBits = input.length * 8;
  // Message + 0x80 + zero padding + 64-bit little-endian length.
  const paddedLen = (((input.length + 8) >> 6) + 1) << 6;
  const msg = new Uint8Array(paddedLen);
  msg.set(input);
  msg[input.length] = 0x80;
  // Length is written as a 64-bit little-endian value; JS bitwise ops are 32-bit,
  // so split it into the low and high words.
  const lo = originalBits >>> 0;
  const hi = Math.floor(originalBits / 0x100000000) >>> 0;
  const lenOffset = paddedLen - 8;
  msg[lenOffset] = lo & 0xff;
  msg[lenOffset + 1] = (lo >>> 8) & 0xff;
  msg[lenOffset + 2] = (lo >>> 16) & 0xff;
  msg[lenOffset + 3] = (lo >>> 24) & 0xff;
  msg[lenOffset + 4] = hi & 0xff;
  msg[lenOffset + 5] = (hi >>> 8) & 0xff;
  msg[lenOffset + 6] = (hi >>> 16) & 0xff;
  msg[lenOffset + 7] = (hi >>> 24) & 0xff;

  let a0 = 0x67452301;
  let b0 = 0xefcdab89;
  let c0 = 0x98badcfe;
  let d0 = 0x10325476;

  const m = new Uint32Array(16);
  for (let offset = 0; offset < msg.length; offset += 64) {
    for (let j = 0; j < 16; j++) {
      const p = offset + j * 4;
      m[j] = msg[p] | (msg[p + 1] << 8) | (msg[p + 2] << 16) | (msg[p + 3] << 24);
    }
    let a = a0;
    let b = b0;
    let c = c0;
    let d = d0;
    for (let i = 0; i < 64; i++) {
      let f: number;
      let g: number;
      if (i < 16) {
        f = (b & c) | (~b & d);
        g = i;
      } else if (i < 32) {
        f = (d & b) | (~d & c);
        g = (5 * i + 1) % 16;
      } else if (i < 48) {
        f = b ^ c ^ d;
        g = (3 * i + 5) % 16;
      } else {
        f = c ^ (b | ~d);
        g = (7 * i) % 16;
      }
      const sum = (a + f + MD5_K[i] + m[g]) >>> 0;
      const rotated = (sum << MD5_S[i]) | (sum >>> (32 - MD5_S[i]));
      a = d;
      d = c;
      c = b;
      b = (b + rotated) >>> 0;
    }
    a0 = (a0 + a) >>> 0;
    b0 = (b0 + b) >>> 0;
    c0 = (c0 + c) >>> 0;
    d0 = (d0 + d) >>> 0;
  }

  const out = new Uint8Array(16);
  [a0, b0, c0, d0].forEach((word, i) => {
    out[i * 4] = word & 0xff;
    out[i * 4 + 1] = (word >>> 8) & 0xff;
    out[i * 4 + 2] = (word >>> 16) & 0xff;
    out[i * 4 + 3] = (word >>> 24) & 0xff;
  });
  return out;
}

// --- SHA-1 / SHA-256 (via WebCrypto) --------------------------------------

async function subtleDigest(algo: "SHA-1" | "SHA-256", data: Uint8Array): Promise<Uint8Array | null> {
  const subtle = typeof globalThis.crypto !== "undefined" ? globalThis.crypto.subtle : undefined;
  if (!subtle) return null;
  // Copy into a fresh ArrayBuffer so the DOM typings accept the buffer source.
  const buf = data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength) as ArrayBuffer;
  const digest = await subtle.digest(algo, buf);
  return new Uint8Array(digest);
}

/** A fingerprint in the `name:base64 (hex)` shape used by the UI. */
export interface KeyFingerprint {
  /** `SHA256` / `SHA1` / `MD5` */
  name: string;
  /** Base64 digest, or "" when unavailable (MD5 has no base64 form in SSH). */
  base64: string;
  /** Lowercase hex digest. */
  hex: string;
}

export interface KeyFingerprints {
  md5: KeyFingerprint | null;
  sha1: KeyFingerprint | null;
  sha256: KeyFingerprint | null;
}

/**
 * Fingerprints of an SSH public key line (`algorithm base64 [comment]`) or a
 * bare base64 key blob. The digest is taken over the key blob exactly as it
 * appears on the wire, matching `ssh-keygen -l`.
 */
export async function sshKeyFingerprints(keyLineOrBlob: string): Promise<KeyFingerprints> {
  const parts = keyLineOrBlob.trim().split(/\s+/).filter(Boolean);
  // A full key line is `algorithm blob [comment]`; a bare blob is one token.
  const blobToken = parts.length >= 2 ? parts[1] : parts[0] || "";
  const blob = base64ToBytes(blobToken) ?? new Uint8Array();

  const md5Digest = md5(blob);
  const sha1Digest = await subtleDigest("SHA-1", blob);
  const sha256Digest = await subtleDigest("SHA-256", blob);

  return {
    md5: { name: "MD5", base64: "", hex: toHex(md5Digest) },
    // Same unpadded base64 OpenSSH prints and compares; see `toBase64`.
    sha1: sha1Digest
      ? {
          name: "SHA1",
          base64: toOpenSSHBase64(sha1Digest),
          hex: toHex(sha1Digest),
        }
      : null,
    sha256: sha256Digest
      ? {
          name: "SHA256",
          base64: toOpenSSHBase64(sha256Digest),
          hex: toHex(sha256Digest),
        }
      : null,
  };
}
