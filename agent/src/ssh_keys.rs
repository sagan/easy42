//! Root user SSH key management.
//!
//! Two responsibilities:
//!
//! 1. Collect the root user's OpenSSH default identity public keys (the
//!    `~/.ssh/id_*.pub` files that `ssh-keygen` creates by default). Only
//!    OpenSSH client default paths are supported by design.
//! 2. Write / update the Easy42-managed block of `/root/.ssh/known_hosts` and
//!    `/root/.ssh/authorized_keys` in an atomic and idempotent way, following
//!    the same approach as the `/etc/hosts` management: the desired lines are
//!    assembled into a delimited block, every other line is preserved, and a
//!    single `fsync` + `rename` replaces the file only when its content
//!    actually changes.

use std::collections::HashSet;
use std::fs;
use std::io::Write;
use std::path::{Path, PathBuf};

use sha2::{Digest, Sha256};

use crate::proto::SshPublicKey;

/// Block markers written around the Easy42-managed lines.
pub const START_MARKER: &str = "### EASY42_START";
pub const END_MARKER: &str = "### EASY42_END";

pub const DEFAULT_SSH_DIR: &str = "/root/.ssh";

/// `authorized_keys` lines written by Easy42 are prefixed with this comment so
/// administrators can tell cluster-managed keys apart from manually added ones.
pub const AUTHORIZED_KEYS_COMMENT: &str = "easy42";

/// OpenSSH client default identity file names. `ssh-keygen` creates these with
/// default flags when run without `-f`, so their public counterparts are the
/// keys actually used by `ssh` without any extra configuration.
pub const DEFAULT_IDENTITY_FILES: &[&str] = &[
    "id_rsa",
    "id_ecdsa",
    "id_ecdsa_sk",
    "id_ed25519",
    "id_ed25519_sk",
    "id_dsa",
];

/// Returns the root SSH directory, honouring `EASY42_SSH_DIR` (used by tests).
pub fn get_ssh_dir() -> PathBuf {
    if let Ok(dir) = std::env::var("EASY42_SSH_DIR") {
        if !dir.trim().is_empty() {
            return PathBuf::from(dir.trim());
        }
    }
    PathBuf::from(DEFAULT_SSH_DIR)
}

pub fn known_hosts_path() -> PathBuf {
    get_ssh_dir().join("known_hosts")
}

pub fn authorized_keys_path() -> PathBuf {
    get_ssh_dir().join("authorized_keys")
}

/// Target file names accepted for managed SSH key files.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SshKeyFileKind {
    KnownHosts,
    AuthorizedKeys,
}

impl SshKeyFileKind {
    /// Parses the Easy42 action target file (`/root/.ssh/known_hosts` or
    /// `/root/.ssh/authorized_keys`) into a kind.
    pub fn from_target_path(path: &str) -> Option<Self> {
        let path = path.trim();
        match path {
            "/root/.ssh/known_hosts" => Some(Self::KnownHosts),
            "/root/.ssh/authorized_keys" => Some(Self::AuthorizedKeys),
            _ => None,
        }
    }

    pub fn resolve_path(&self, ssh_dir: &Path) -> PathBuf {
        match self {
            Self::KnownHosts => ssh_dir.join("known_hosts"),
            Self::AuthorizedKeys => ssh_dir.join("authorized_keys"),
        }
    }
}

/// Parsed form of a single OpenSSH public key line.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ParsedKey {
    pub algorithm: String,
    pub key: String,
    pub comment: String,
}

impl ParsedKey {
    /// Parses an OpenSSH public key line (`[options] algorithm base64 [comment]`).
    ///
    /// Only the simple `algorithm base64 [comment]` shape produced by
    /// `ssh-keygen` for identity public keys is accepted. Options and the
    /// `@cert-authority` / `@revoked` markers are rejected.
    pub fn parse(line: &str) -> Option<Self> {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            return None;
        }
        let mut parts = trimmed.splitn(3, char::is_whitespace);
        let algorithm = parts.next()?.trim();
        let key = parts.next()?.trim();
        let comment = parts.next().unwrap_or("").trim();
        Self::validate(algorithm, key)?;
        Some(ParsedKey {
            algorithm: algorithm.to_string(),
            key: key.to_string(),
            comment: comment.to_string(),
        })
    }

    /// Validates the algorithm / key pair and returns the algorithm when valid.
    pub fn validate(algorithm: &str, key: &str) -> Option<()> {
        let algorithm = algorithm.trim();
        let key = key.trim();
        if !is_supported_algorithm(algorithm) {
            return None;
        }
        if key.is_empty() || !is_valid_base64(key) {
            return None;
        }
        // An SSH public key blob is an SSH string holding the algorithm name.
        let decoded = base64_decode(key)?;
        if decoded.len() < 4 + algorithm.len() {
            return None;
        }
        let name_len = u32::from_be_bytes(decoded[0..4].try_into().ok()?) as usize;
        if name_len != algorithm.len() || &decoded[4..4 + name_len] != algorithm.as_bytes() {
            return None;
        }
        Some(())
    }

    /// Stable identity of the key material (algorithm + blob), used for dedupe.
    pub fn fingerprint(&self) -> String {
        format!("{}\u{0}{}", self.algorithm, self.key)
    }
}

/// Returns true when the algorithm is a known OpenSSH public key algorithm.
pub fn is_supported_algorithm(algorithm: &str) -> bool {
    matches!(
        algorithm,
        "ssh-rsa"
            | "ssh-dss"
            | "ssh-ed25519"
            | "ssh-ed25519-sk"
            | "ecdsa-sha2-nistp256"
            | "ecdsa-sha2-nistp384"
            | "ecdsa-sha2-nistp521"
            | "sk-ssh-ed25519@openssh.com"
            | "sk-ecdsa-sha2-nistp256@openssh.com"
    )
}

fn is_valid_base64(s: &str) -> bool {
    !s.is_empty()
        && s.len() <= 16 * 1024
        && s.chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '+' || c == '/' || c == '=')
}

/// Base64 (standard alphabet, with padding) decoder used to verify that a key
/// blob actually belongs to the claimed identity. No external dependency is
/// needed for this small, well specified task.
pub fn base64_decode(s: &str) -> Option<Vec<u8>> {
    let bytes = s.trim().as_bytes();
    if bytes.is_empty() {
        return Some(Vec::new());
    }
    if !bytes.len().is_multiple_of(4) {
        return None;
    }
    let mut out = Vec::with_capacity(bytes.len() / 4 * 3);
    let mut padding = 0;
    for (idx, chunk) in bytes.chunks(4).enumerate() {
        let mut vals = [0u8; 4];
        for (i, &b) in chunk.iter().enumerate() {
            let v = match b {
                b'A'..=b'Z' => b - b'A',
                b'a'..=b'z' => b - b'a' + 26,
                b'0'..=b'9' => b - b'0' + 52,
                b'+' => 62,
                b'/' => 63,
                b'=' => {
                    // Padding is only allowed at the end.
                    if idx + 1 != bytes.len() / 4 {
                        return None;
                    }
                    padding += 1;
                    0
                }
                _ => return None,
            };
            vals[i] = v;
        }
        if padding > 2 {
            return None;
        }
        let triple = ((vals[0] as u32) << 18)
            | ((vals[1] as u32) << 12)
            | ((vals[2] as u32) << 6)
            | (vals[3] as u32);
        out.push((triple >> 16) as u8);
        out.push((triple >> 8) as u8);
        out.push(triple as u8);
    }
    out.truncate(out.len().saturating_sub(padding));
    Some(out)
}

/// Extracts the OpenSSH wire-format public key blob (the contents after the
/// `algorithm ` prefix) from a private key file.
///
/// Only the file formats that can be parsed without cryptographic libraries are
/// handled: OpenSSH new format (`-----BEGIN OPENSSH PRIVATE KEY-----`, where the
/// public key is embedded), PEM PKCS#1 RSA keys and PEM SEC1 EC keys. For
/// encrypted keys and PKCS#8 PEM keys (`BEGIN PRIVATE KEY`) no blob can be
/// derived without decrypting / extra ASN.1 support, so `None` is returned and
/// the caller treats it as "cannot verify" rather than "mismatch".
pub fn public_key_blob_from_private_key(content: &str) -> Option<Vec<u8>> {
    let trimmed = content.trim();
    if trimmed.contains("-----BEGIN OPENSSH PRIVATE KEY-----") {
        return openssh_private_key_blob(trimmed);
    }
    if trimmed.contains("-----BEGIN RSA PRIVATE KEY-----") {
        return rsa_private_key_blob(trimmed);
    }
    if trimmed.contains("-----BEGIN EC PRIVATE KEY-----") {
        return ec_private_key_blob(trimmed);
    }
    None
}

/// OpenSSH private key format: PEM-wrapped, base64 body, unencrypted body has
/// magic "openssh-key-v1\0" followed by its SSH key blob at the end.
fn openssh_private_key_blob(content: &str) -> Option<Vec<u8>> {
    let body: String = content
        .lines()
        .filter(|l| !l.starts_with("-----"))
        .collect::<Vec<_>>()
        .join("");
    let raw = base64_decode(&body)?;
    let magic = b"openssh-key-v1\0";
    if !raw.starts_with(magic) {
        return None;
    }
    let mut pos = magic.len();
    let cipher = read_ssh_string(&raw, &mut pos)?;
    if !cipher.is_empty() && cipher != b"none" {
        // Encrypted private key: cannot verify without a passphrase.
        return None;
    }
    let _kdf = read_ssh_string(&raw, &mut pos)?;
    let _kdf_options = read_ssh_string(&raw, &mut pos)?;
    if raw.len() < pos + 4 {
        return None;
    }
    let nkeys = u32::from_be_bytes(raw[pos..pos + 4].try_into().ok()?) as usize;
    pos += 4;
    let mut last_pub: Option<Vec<u8>> = None;
    for _ in 0..nkeys {
        let blob = read_ssh_string(&raw, &mut pos)?;
        last_pub = Some(blob.to_vec());
    }
    last_pub
}

/// Parses a PEM base64 body into DER bytes.
fn pem_der_body(content: &str) -> Option<Vec<u8>> {
    let body: String = content
        .lines()
        .filter(|l| !l.starts_with("-----"))
        .collect::<Vec<_>>()
        .join("");
    base64_decode(&body)
}

/// Minimal DER reader for the handful of fields needed below.
struct Der<'a> {
    buf: &'a [u8],
    pos: usize,
}

impl<'a> Der<'a> {
    fn new(buf: &'a [u8]) -> Self {
        Self { buf, pos: 0 }
    }

    fn read_tlv(&mut self) -> Option<(u8, &'a [u8])> {
        let tag = *self.buf.get(self.pos)?;
        let first = *self.buf.get(self.pos + 1)? as usize;
        let (len, hdr) = if first & 0x80 == 0 {
            (first, 2)
        } else {
            let n = first & 0x7f;
            if n == 0 || n > 4 {
                return None;
            }
            let mut len = 0usize;
            for i in 0..n {
                len = (len << 8) | (*self.buf.get(self.pos + 2 + i)? as usize);
            }
            (len, 2 + n)
        };
        let start = self.pos + hdr;
        let end = start.checked_add(len)?;
        if end > self.buf.len() {
            return None;
        }
        self.pos = end;
        Some((tag, &self.buf[start..end]))
    }
}

fn ssh_string(data: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(data.len() + 4);
    out.extend_from_slice(&(data.len() as u32).to_be_bytes());
    out.extend_from_slice(data);
    out
}

fn ssh_mpint(data: &[u8]) -> Vec<u8> {
    let mut start = 0;
    while start < data.len() && data[start] == 0 {
        start += 1;
    }
    let trimmed = &data[start..];
    if trimmed.is_empty() {
        return ssh_string(&[]);
    }
    if trimmed[0] & 0x80 != 0 {
        let mut with_zero = Vec::with_capacity(trimmed.len() + 1);
        with_zero.push(0);
        with_zero.extend_from_slice(trimmed);
        ssh_string(&with_zero)
    } else {
        ssh_string(trimmed)
    }
}

fn rsa_private_key_blob(content: &str) -> Option<Vec<u8>> {
    let der = pem_der_body(content)?;
    let mut top = Der::new(&der);
    let (tag, seq) = top.read_tlv()?;
    if tag != 0x30 {
        return None;
    }
    let mut inner = Der::new(seq);
    let (_t, _version) = inner.read_tlv()?;
    let (_t, n) = inner.read_tlv()?;
    let (_t, e) = inner.read_tlv()?;
    let mut blob = Vec::new();
    blob.extend_from_slice(&ssh_string(b"ssh-rsa"));
    blob.extend_from_slice(&ssh_mpint(e));
    blob.extend_from_slice(&ssh_mpint(n));
    Some(blob)
}

fn ec_private_key_blob(content: &str) -> Option<Vec<u8>> {
    let der = pem_der_body(content)?;
    let mut top = Der::new(&der);
    let (tag, seq) = top.read_tlv()?;
    if tag != 0x30 {
        return None;
    }
    // ECPrivateKey ::= SEQUENCE { version, privateKey OCTET STRING,
    //                             [0] parameters OID OPTIONAL, [1] publicKey BIT STRING OPTIONAL }
    let mut inner = Der::new(seq);
    let (_t, _version) = inner.read_tlv()?;
    let (_t, priv_octets) = inner.read_tlv()?;
    let (tag, params) = inner.read_tlv()?;
    if tag != 0x06 {
        // Encrypted keys carry an ECParameters / PBES2 structure here.
        return None;
    }
    let (curve_name, alg) = if params == [0x2A, 0x86, 0x48, 0xCE, 0x3D, 0x03, 0x01, 0x07] {
        ("nistp256", "ecdsa-sha2-nistp256")
    } else if params == [0x2B, 0x81, 0x04, 0x00, 0x22] {
        ("nistp384", "ecdsa-sha2-nistp384")
    } else if params == [0x2B, 0x81, 0x04, 0x00, 0x23] {
        ("nistp521", "ecdsa-sha2-nistp521")
    } else {
        return None;
    };
    let point = ec_point_from_private_scalar(curve_name, priv_octets)?;
    let mut blob = Vec::new();
    blob.extend_from_slice(&ssh_string(alg.as_bytes()));
    blob.extend_from_slice(&ssh_string(curve_name.as_bytes()));
    blob.extend_from_slice(&ssh_string(&point));
    Some(blob)
}

/// Minimal secp256r1 scalar multiplication over the affine curve
/// `y^2 = x^3 - 3x + b (mod p)` to recover the public point from a SEC1
/// private key. Only P-256 is supported; other curves return `None`.
fn ec_point_from_private_scalar(curve_name: &str, priv_octets: &[u8]) -> Option<Vec<u8>> {
    if curve_name != "nistp256" {
        return None;
    }
    let p = U256::from_be_hex("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff")?;
    let b = U256::from_be_hex("5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b")?;
    let gx = U256::from_be_hex("6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296")?;
    let gy = U256::from_be_hex("4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5")?;
    let d = U256::from_be_bytes(priv_octets)?;
    let point = U256::scalar_mul(&(gx, gy), &d, &p, &b)?;
    let mut out = Vec::with_capacity(65);
    out.push(0x04);
    out.extend_from_slice(&point.0.as_be_bytes());
    out.extend_from_slice(&point.1.as_be_bytes());
    Some(out)
}

/// Fixed-width 256-bit unsigned integer with the modular arithmetic needed for
/// P-256 public key derivation. Presentation is big-endian.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
struct U256([u64; 4]);

impl U256 {
    fn zero() -> Self {
        U256([0, 0, 0, 0])
    }

    fn one() -> Self {
        U256([1, 0, 0, 0])
    }

    fn is_zero(&self) -> bool {
        self.0.iter().all(|&w| w == 0)
    }

    fn from_be_hex(hex: &str) -> Option<Self> {
        let bytes = (0..hex.len() / 2)
            .map(|i| u8::from_str_radix(&hex[i * 2..i * 2 + 2], 16).ok())
            .collect::<Option<Vec<u8>>>()?;
        Self::from_be_bytes(&bytes)
    }

    fn from_be_bytes(bytes: &[u8]) -> Option<Self> {
        if bytes.is_empty() || bytes.len() > 32 {
            return None;
        }
        let mut buf = [0u8; 32];
        buf[32 - bytes.len()..].copy_from_slice(bytes);
        let mut words = [0u64; 4];
        for i in 0..4 {
            let mut chunk = [0u8; 8];
            chunk.copy_from_slice(&buf[i * 8..i * 8 + 8]);
            words[i] = u64::from_be_bytes(chunk);
        }
        Some(U256(words))
    }

    fn as_be_bytes(&self) -> [u8; 32] {
        let mut out = [0u8; 32];
        for i in 0..4 {
            out[i * 8..i * 8 + 8].copy_from_slice(&self.0[i].to_be_bytes());
        }
        out
    }

    fn bit(&self, idx: usize) -> bool {
        if idx >= 256 {
            return false;
        }
        (self.0[idx / 64] >> (idx % 64)) & 1 == 1
    }

    fn bits(&self) -> usize {
        for i in (0..4).rev() {
            if self.0[i] != 0 {
                return i * 64 + (64 - self.0[i].leading_zeros() as usize);
            }
        }
        0
    }

    fn cmp(&self, other: &Self) -> std::cmp::Ordering {
        for i in (0..4).rev() {
            match self.0[i].cmp(&other.0[i]) {
                std::cmp::Ordering::Equal => continue,
                ord => return ord,
            }
        }
        std::cmp::Ordering::Equal
    }

    fn add(&self, other: &Self) -> Self {
        let mut out = [0u64; 4];
        let mut carry = 0u128;
        for i in 0..4 {
            let sum = self.0[i] as u128 + other.0[i] as u128 + carry;
            out[i] = sum as u64;
            carry = sum >> 64;
        }
        U256(out)
    }

    fn wrapping_sub(&self, other: &Self) -> Self {
        let mut out = [0u64; 4];
        let mut borrow = 0i128;
        for i in 0..4 {
            let diff = self.0[i] as i128 - other.0[i] as i128 - borrow;
            if diff < 0 {
                out[i] = (diff + (1i128 << 64)) as u64;
                borrow = 1;
            } else {
                out[i] = diff as u64;
                borrow = 0;
            }
        }
        U256(out)
    }

    fn sub(&self, other: &Self, p: &Self) -> Self {
        if self.cmp(other) != std::cmp::Ordering::Less {
            self.wrapping_sub(other)
        } else {
            self.add(p).wrapping_sub(other)
        }
    }

    fn add_mod(&self, other: &Self, p: &Self) -> Self {
        let mut sum = self.add(other);
        while sum.cmp(p) != std::cmp::Ordering::Less {
            sum = sum.wrapping_sub(p);
        }
        sum
    }

    fn mul_wide(a: &Self, b: &Self) -> [u64; 8] {
        let mut prod = [0u128; 8];
        for (i, a_word) in a.0.iter().enumerate() {
            for (j, b_word) in b.0.iter().enumerate() {
                let cur = *a_word as u128 * *b_word as u128;
                let idx = i + j;
                let total = prod[idx] + cur;
                prod[idx] = total & 0xFFFF_FFFF_FFFF_FFFF;
                let mut carry = total >> 64;
                let mut k = idx + 1;
                while carry != 0 && k < 8 {
                    let t = prod[k] + carry;
                    prod[k] = t & 0xFFFF_FFFF_FFFF_FFFF;
                    carry = t >> 64;
                    k += 1;
                }
            }
        }
        let mut out = [0u64; 8];
        for i in 0..8 {
            out[i] = prod[i] as u64;
        }
        out
    }

    fn mul_mod(&self, other: &Self, p: &Self) -> Self {
        let prod = Self::mul_wide(self, other);
        let mut rem = Self::zero();
        for bit in (0..512).rev() {
            rem = rem.add(&rem);
            while rem.cmp(p) != std::cmp::Ordering::Less {
                rem = rem.wrapping_sub(p);
            }
            if (prod[bit / 64] >> (bit % 64)) & 1 == 1 {
                rem = rem.add(&Self::one());
                while rem.cmp(p) != std::cmp::Ordering::Less {
                    rem = rem.wrapping_sub(p);
                }
            }
        }
        rem
    }

    fn inv_mod(&self, p: &Self) -> Self {
        // p is prime and self is non-zero: exponentiate by p-2.
        let exp = p.wrapping_sub(&U256([2, 0, 0, 0]));
        let mut result = Self::one();
        let mut base = *self;
        for bit in 0..exp.bits() {
            if exp.bit(bit) {
                result = result.mul_mod(&base, p);
            }
            base = base.mul_mod(&base, p);
        }
        result
    }

    fn scalar_mul(g: &(U256, U256), k: &U256, p: &U256, b: &U256) -> Option<(U256, U256)> {
        let mut result: Option<(U256, U256)> = None;
        let mut addend = *g;
        for bit in 0..k.bits() {
            if k.bit(bit) {
                result = Some(match result {
                    None => addend,
                    Some(acc) => point_add(&acc, &addend, p, b)?,
                });
            }
            addend = point_add(&addend, &addend, p, b)?;
        }
        let (x, y) = result?;
        if x.is_zero() && y.is_zero() {
            return None;
        }
        Some((x, y))
    }
}

/// Adds two points on the short Weierstrass curve `y^2 = x^3 - 3x + b`, whose
/// coefficient `a` is `-3` for every curve handled here. The point at infinity
/// is represented as `(0, 0)`; for a real point, `x == 0` implies `y != 0`.
fn point_add(
    a: &(U256, U256),
    b: &(U256, U256),
    p: &U256,
    _curve_b: &U256,
) -> Option<(U256, U256)> {
    let (x1, y1) = *a;
    let (x2, y2) = *b;
    let three = U256([3, 0, 0, 0]);
    let infinity = (U256::zero(), U256::zero());

    if x1.is_zero() && y1.is_zero() {
        return Some((x2, y2));
    }
    if x2.is_zero() && y2.is_zero() {
        return Some((x1, y1));
    }
    if x1 == x2 {
        if y1 != y2 || y1.is_zero() {
            // P + (-P) = O
            return Some(infinity);
        }
        // Doubling: lambda = (3x^2 - 3) / 2y
        let num = x1.mul_mod(&x1, p).mul_mod(&three, p).sub(&three, p);
        let den = y1.add_mod(&y1, p).inv_mod(p);
        let lambda = num.mul_mod(&den, p);
        let x3 = lambda.mul_mod(&lambda, p).sub(&x1.add_mod(&x1, p), p);
        let y3 = lambda.mul_mod(&x1.sub(&x3, p), p).sub(&y1, p);
        return Some((x3, y3));
    }
    let num = y2.sub(&y1, p);
    let den = x2.sub(&x1, p).inv_mod(p);
    let lambda = num.mul_mod(&den, p);
    let x3 = lambda.mul_mod(&lambda, p).sub(&x1, p).sub(&x2, p);
    let y3 = lambda.mul_mod(&x1.sub(&x3, p), p).sub(&y1, p);
    Some((x3, y3))
}

fn read_ssh_string<'a>(data: &'a [u8], pos: &mut usize) -> Option<&'a [u8]> {
    if data.len() < *pos + 4 {
        return None;
    }
    let len = u32::from_be_bytes(data[*pos..*pos + 4].try_into().ok()?) as usize;
    *pos += 4;
    if data.len() < *pos + len {
        return None;
    }
    let s = &data[*pos..*pos + len];
    *pos += len;
    Some(s)
}

/// Returns the key blob (base64, as it appears in a `.pub` file) from a private
/// key file content, or `None` when it cannot be determined.
pub fn public_key_base64_from_private_key(content: &str) -> Option<String> {
    let blob = public_key_blob_from_private_key(content)?;
    Some(encode_base64(&blob))
}

pub fn encode_base64(data: &[u8]) -> String {
    const ALPHABET: &[u8; 64] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    let mut out = String::with_capacity(data.len().div_ceil(3) * 4);
    for chunk in data.chunks(3) {
        let b0 = chunk[0] as u32;
        let b1 = *chunk.get(1).unwrap_or(&0) as u32;
        let b2 = *chunk.get(2).unwrap_or(&0) as u32;
        let triple = (b0 << 16) | (b1 << 8) | b2;
        out.push(ALPHABET[((triple >> 18) & 0x3F) as usize] as char);
        out.push(ALPHABET[((triple >> 12) & 0x3F) as usize] as char);
        if chunk.len() > 1 {
            out.push(ALPHABET[((triple >> 6) & 0x3F) as usize] as char);
        } else {
            out.push('=');
        }
        if chunk.len() > 2 {
            out.push(ALPHABET[(triple & 0x3F) as usize] as char);
        } else {
            out.push('=');
        }
    }
    out
}

/// Collects the root user's OpenSSH default identity public keys.
///
/// Each `.pub` file is included only when its key blob matches the
/// corresponding private key (when verification is possible), so stale or
/// hand-edited public files never make it into other nodes' key files.
pub fn collect_public_keys(ssh_dir: &Path) -> Vec<SshPublicKey> {
    let mut keys = Vec::new();
    let mut seen = HashSet::new();
    for name in DEFAULT_IDENTITY_FILES {
        let pub_path = ssh_dir.join(format!("{}.pub", name));
        let priv_path = ssh_dir.join(name);
        let pub_content = match fs::read_to_string(&pub_path) {
            Ok(c) => c,
            Err(_) => continue,
        };
        let parsed = match ParsedKey::parse(&pub_content) {
            Some(p) => p,
            None => {
                tracing::debug!("Ignoring malformed SSH public key file {:?}", pub_path);
                continue;
            }
        };
        if let Ok(priv_content) = fs::read_to_string(&priv_path) {
            if let Some(expected) = public_key_base64_from_private_key(&priv_content) {
                if expected != parsed.key {
                    tracing::warn!(
                        "Ignoring SSH public key {:?}: it does not match its private key",
                        pub_path
                    );
                    continue;
                }
            }
        }
        let fingerprint = parsed.fingerprint();
        if !seen.insert(fingerprint) {
            continue;
        }
        keys.push(SshPublicKey {
            algorithm: parsed.algorithm,
            key: parsed.key,
            comment: parsed.comment,
            path: pub_path.to_string_lossy().to_string(),
        });
    }
    keys
}

/// Builds the deterministic Easy42 block for the given keys.
pub fn build_block(keys: &[SshPublicKey], kind: SshKeyFileKind) -> String {
    let mut lines: Vec<String> = Vec::new();
    let mut seen = HashSet::new();
    for key in keys {
        let algorithm = key.algorithm.trim();
        let blob = key.key.trim();
        if ParsedKey::validate(algorithm, blob).is_none() {
            continue;
        }
        let identity = format!("{}\u{0}{}", algorithm, blob);
        if !seen.insert(identity) {
            continue;
        }
        let comment = key.comment.trim();
        let line = match kind {
            SshKeyFileKind::AuthorizedKeys => {
                if comment.is_empty() {
                    format!("{} {} {}", algorithm, blob, AUTHORIZED_KEYS_COMMENT)
                } else {
                    format!("{} {} {} {}", algorithm, blob, AUTHORIZED_KEYS_COMMENT, comment)
                }
            }
            SshKeyFileKind::KnownHosts => {
                if comment.is_empty() {
                    format!("{} {}", algorithm, blob)
                } else {
                    format!("{} {} {}", algorithm, blob, comment)
                }
            }
        };
        lines.push(line);
    }
    lines.sort();
    lines.dedup();

    let mut out = String::new();
    out.push_str(START_MARKER);
    out.push('\n');
    for line in lines {
        out.push_str(&line);
        out.push('\n');
    }
    out.push_str(END_MARKER);
    out.push('\n');
    out
}

pub fn hash_block(block: &str) -> String {
    let norm = block.replace("\r\n", "\n");
    let trimmed = norm.trim();
    if trimmed.is_empty() {
        return String::new();
    }
    let mut hasher = Sha256::new();
    hasher.update(trimmed.as_bytes());
    format!("{:x}", hasher.finalize())
}

pub fn extract_block(content: &str) -> Option<(String, String)> {
    let norm = content.replace("\r\n", "\n");
    let start_idx = norm.find(START_MARKER)?;
    let end_relative = norm[start_idx..].find(END_MARKER)?;
    let end_idx = start_idx + end_relative + END_MARKER.len();
    let block = &norm[start_idx..end_idx];
    Some((block.to_string(), hash_block(block)))
}

pub fn get_current_block_hash(path: &Path) -> String {
    if let Ok(content) = fs::read_to_string(path) {
        if let Some((_, hash)) = extract_block(&content) {
            return hash;
        }
    }
    String::new()
}

fn mode_for_kind(kind: SshKeyFileKind) -> u32 {
    match kind {
        SshKeyFileKind::KnownHosts => 0o644,
        SshKeyFileKind::AuthorizedKeys => 0o600,
    }
}

/// Locates the Easy42 block and splits the content into the raw text before it
/// and after it, dropping the single newline that terminates the end marker.
fn split_around_block(content: &str) -> (String, String, bool) {
    let norm = content.replace("\r\n", "\n");
    if let Some(start_idx) = norm.find(START_MARKER) {
        if let Some(end_relative) = norm[start_idx..].find(END_MARKER) {
            let end_idx = start_idx + end_relative + END_MARKER.len();
            let mut suffix = &norm[end_idx..];
            if suffix.starts_with('\n') {
                suffix = &suffix[1..];
            }
            return (norm[..start_idx].to_string(), suffix.to_string(), true);
        }
    }
    (norm, String::new(), false)
}

/// Replaces (or removes, when `block` has an empty body) the Easy42 block in
/// `content`, preserving every other line.
///
/// The block is always surrounded by exactly one blank line, like the
/// `/etc/hosts` block. When the block is the first thing in the file it is
/// written at the very top followed by a blank line.
fn splice_block(content: &str, block: &str) -> String {
    let block_body = block.trim_end_matches('\n');
    let (raw_prefix, suffix, had_block) = split_around_block(content);

    if block_body.is_empty() {
        if !had_block {
            return raw_prefix;
        }
        // Removing the block also drops the blank separator line written in
        // front of it, so the previous content is restored byte for byte.
        let mut prefix = raw_prefix;
        if prefix.ends_with("\n\n") {
            prefix.pop();
        } else if prefix.chars().all(|c| c == '\n') {
            prefix.clear();
        }
        return format!("{}{}", prefix, suffix);
    }

    let prefix = if raw_prefix.is_empty() {
        String::new()
    } else if raw_prefix.ends_with("\n\n") {
        raw_prefix
    } else if raw_prefix.ends_with('\n') {
        format!("{}\n", raw_prefix)
    } else {
        format!("{}\n\n", raw_prefix)
    };
    let suffix = if prefix.is_empty() || suffix.is_empty() || suffix.starts_with('\n') {
        suffix
    } else {
        format!("\n{}", suffix)
    };
    format!("{}{}\n{}", prefix, block_body, suffix)
}

/// Atomically and idempotently writes the desired Easy42 block into the target
/// SSH key file.
///
/// Returns `Ok((modified, new_hash))`. When the resulting file content is
/// unchanged no write happens and `modified` is `false`.
pub fn update_key_file(
    ssh_dir: &Path,
    kind: SshKeyFileKind,
    keys: &[SshPublicKey],
    tmp_suffix: &str,
) -> Result<(bool, String), String> {
    let block = build_block(keys, kind);
    let new_hash = hash_block(&block);
    let block_has_keys = keys.iter().any(|k| {
        ParsedKey::validate(k.algorithm.trim(), k.key.trim()).is_some()
    });

    let path = kind.resolve_path(ssh_dir);
    let existing = match fs::read_to_string(&path) {
        Ok(c) => Some(c),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => None,
        Err(e) => return Err(format!("failed to read {:?}: {}", path, e)),
    };

    let has_existing_block = existing
        .as_deref()
        .map(|c| c.contains(START_MARKER) && c.contains(END_MARKER))
        .unwrap_or(false);

    // Nothing to remove and nothing to add: keep the file untouched.
    let new_content = if !block_has_keys && !has_existing_block {
        None
    } else if !block_has_keys {
        // An empty block body removes the managed block.
        Some(splice_block(existing.as_deref().unwrap_or(""), ""))
    } else {
        Some(splice_block(existing.as_deref().unwrap_or(""), &block))
    };

    let new_content = match new_content {
        Some(c) => c,
        None => return Ok((false, new_hash)),
    };

    if existing.as_deref() == Some(new_content.as_str()) {
        return Ok((false, new_hash));
    }

    if let Err(e) = fs::create_dir_all(ssh_dir) {
        return Err(format!("failed to create directory {:?}: {}", ssh_dir, e));
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(ssh_dir, fs::Permissions::from_mode(0o700));
    }

    let tmp_path = ssh_dir.join(format!(
        ".{}{}",
        path.file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("easy42-ssh"),
        tmp_suffix
    ));
    {
        let mut tmp_file = fs::OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .open(&tmp_path)
            .map_err(|e| format!("failed to create temp file {:?}: {}", tmp_path, e))?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            let _ = fs::set_permissions(&tmp_path, fs::Permissions::from_mode(mode_for_kind(kind)));
        }
        tmp_file
            .write_all(new_content.as_bytes())
            .map_err(|e| format!("failed to write temp file {:?}: {}", tmp_path, e))?;
        tmp_file
            .sync_all()
            .map_err(|e| format!("failed to sync temp file {:?}: {}", tmp_path, e))?;
    }
    fs::rename(&tmp_path, &path).map_err(|e| {
        let _ = fs::remove_file(&tmp_path);
        format!("failed to rename {:?} to {:?}: {}", tmp_path, path, e)
    })?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let _ = fs::set_permissions(&path, fs::Permissions::from_mode(mode_for_kind(kind)));
    }
    Ok((true, new_hash))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(tag: &str) -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "easy42-ssh-{}-{}-{}",
            tag,
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    fn blob(algorithm: &str, payload: &[u8]) -> String {
        let mut data = Vec::new();
        data.extend_from_slice(&(algorithm.len() as u32).to_be_bytes());
        data.extend_from_slice(algorithm.as_bytes());
        data.extend_from_slice(payload);
        encode_base64(&data)
    }

    fn key(alg: &str, payload: &str, comment: &str) -> SshPublicKey {
        SshPublicKey {
            algorithm: alg.to_string(),
            key: blob(alg, payload.as_bytes()),
            comment: comment.to_string(),
            path: String::new(),
        }
    }

    #[test]
    fn test_parse_public_key() {
        let enc = blob("ssh-ed25519", b"\x01\x02\x03");
        let p = ParsedKey::parse(&format!("ssh-ed25519 {} root@node", enc)).unwrap();
        assert_eq!(p.algorithm, "ssh-ed25519");
        assert_eq!(p.key, enc);
        assert_eq!(p.comment, "root@node");
        // Algorithm in the text must match the blob.
        assert!(ParsedKey::parse(&format!("ssh-rsa {} root@node", enc)).is_none());
        assert!(ParsedKey::parse("# comment").is_none());
        assert!(ParsedKey::parse("unknown-alg AAAA").is_none());
        assert!(ParsedKey::parse("ssh-ed25519 bad key!!").is_none());
        assert!(ParsedKey::parse("@cert-authority ssh-ed25519 AAAA x").is_none());
    }

    #[test]
    fn test_build_block_known_hosts() {
        let keys = vec![
            key("ssh-ed25519", "e", "b"),
            key("ssh-rsa", "a", "a"),
            key("ssh-rsa", "a", "dup"),
        ];
        let block = build_block(&keys, SshKeyFileKind::KnownHosts);
        let expected = format!(
            "### EASY42_START\nssh-ed25519 {} b\nssh-rsa {} a\n### EASY42_END\n",
            blob("ssh-ed25519", b"e"),
            blob("ssh-rsa", b"a")
        );
        assert_eq!(block, expected);
        let (_, hash) = extract_block(&block).unwrap();
        assert_eq!(hash, hash_block(&block));
    }

    #[test]
    fn test_build_block_authorized_keys() {
        let keys = vec![key("ssh-ed25519", "e", "")];
        let block = build_block(&keys, SshKeyFileKind::AuthorizedKeys);
        assert_eq!(
            block,
            format!(
                "### EASY42_START\nssh-ed25519 {} easy42\n### EASY42_END\n",
                blob("ssh-ed25519", b"e")
            )
        );
    }

    #[test]
    fn test_splice_preserves_other_lines() {
        let block = build_block(
            &[key("ssh-ed25519", "cluster", "node1")],
            SshKeyFileKind::KnownHosts,
        );
        let block_line = format!("ssh-ed25519 {} node1", blob("ssh-ed25519", b"cluster"));

        // Empty file -> block at the top.
        assert_eq!(
            splice_block("", &block),
            format!("### EASY42_START\n{}\n### EASY42_END\n", block_line)
        );

        // Content without trailing newline gets one separator blank line.
        let existing = "github.com ssh-ed25519 AAAAuser\n# my comment";
        let out = splice_block(existing, &block);
        assert_eq!(
            out,
            format!(
                "github.com ssh-ed25519 AAAAuser\n# my comment\n\n### EASY42_START\n{}\n### EASY42_END\n",
                block_line
            )
        );
        // Re-splicing with the same block is a fixed point.
        assert_eq!(splice_block(&out, &block), out);

        // Content after the block is preserved behind a blank line.
        let with_suffix = format!("{}tail-line", out);
        let out2 = splice_block(&with_suffix, &block);
        assert_eq!(out2, format!("{}\ntail-line", out));
        assert_eq!(splice_block(&out2, &block), out2);

        // Removing the block restores the original content.
        assert_eq!(
            splice_block(&out, ""),
            "github.com ssh-ed25519 AAAAuser\n# my comment\n"
        );
        assert_eq!(
            splice_block(&format!("{}tail-line", out), ""),
            "github.com ssh-ed25519 AAAAuser\n# my comment\ntail-line"
        );
        assert_eq!(splice_block("", ""), "");
        assert_eq!(splice_block("untouched\n", ""), "untouched\n");
    }

    #[test]
    fn test_update_key_file_atomic_idempotent() {
        let dir = temp_dir("ker");
        fs::write(dir.join("authorized_keys"), "manual-key comment\n\n").unwrap();
        let keys = vec![key("ssh-ed25519", "cluster", "node1")];
        let (modified, hash) =
            update_key_file(&dir, SshKeyFileKind::AuthorizedKeys, &keys, ".tmp").unwrap();
        assert!(modified);
        let content = fs::read_to_string(dir.join("authorized_keys")).unwrap();
        assert!(content.contains(&format!(
            "### EASY42_START\nssh-ed25519 {} easy42 node1\n### EASY42_END\n",
            blob("ssh-ed25519", b"cluster")
        )));
        assert_eq!(get_current_block_hash(&dir.join("authorized_keys")), hash);

        // Second run must not modify the file.
        let (modified2, hash2) =
            update_key_file(&dir, SshKeyFileKind::AuthorizedKeys, &keys, ".tmp").unwrap();
        assert!(!modified2);
        assert_eq!(hash, hash2);

        // Removing the keys leaves no empty block behind and restores the
        // manually managed content.
        let (modified3, _) =
            update_key_file(&dir, SshKeyFileKind::AuthorizedKeys, &[], ".tmp").unwrap();
        assert!(modified3);
        let content = fs::read_to_string(dir.join("authorized_keys")).unwrap();
        assert!(!content.contains(START_MARKER));
        assert_eq!(content, "manual-key comment\n");
        let (modified4, _) =
            update_key_file(&dir, SshKeyFileKind::AuthorizedKeys, &[], ".tmp").unwrap();
        assert!(!modified4);
    }

    #[test]
    fn test_update_key_file_no_block_when_absent() {
        let dir = temp_dir("absent");
        let (modified, hash) =
            update_key_file(&dir, SshKeyFileKind::KnownHosts, &[], ".tmp").unwrap();
        assert!(!modified);
        assert_eq!(hash, hash_block("### EASY42_START\n### EASY42_END\n"));
        assert!(!dir.join("known_hosts").exists());
    }

    #[test]
    fn test_collect_public_keys() {
        let dir = temp_dir("collect");
        let ed = blob("ssh-ed25519", b"node-ed25519");
        let rsa = blob("ssh-rsa", b"node-rsa");
        fs::write(dir.join("id_ed25519.pub"), format!("ssh-ed25519 {} keycomment\n", ed)).unwrap();
        fs::write(dir.join("id_rsa.pub"), format!("ssh-rsa {} other\n", rsa)).unwrap();
        fs::write(dir.join("custom.pub"), format!("ssh-ed25519 {} x\n", ed)).unwrap();
        let keys = collect_public_keys(&dir);
        assert_eq!(keys.len(), 2);
        assert!(keys
            .iter()
            .any(|k| k.path.ends_with("id_ed25519.pub") && k.comment == "keycomment"));
        assert!(keys.iter().any(|k| k.path.ends_with("id_rsa.pub")));
        assert!(!keys.iter().any(|k| k.path.ends_with("custom.pub")));
    }

    #[test]
    fn test_base64_roundtrip() {
        for data in [b"".as_slice(), b"ab", b"abc", b"abcd", b"abcde"] {
            let enc = encode_base64(data);
            if enc.is_empty() {
                assert_eq!(base64_decode(&enc), Some(Vec::new()));
            } else {
                assert_eq!(base64_decode(&enc).unwrap(), data.to_vec());
            }
        }
    }

    #[test]
    fn test_private_key_derivation_failures_are_safe() {
        // Garbage, legacy encrypted PEM and PKCS#8 PEM keys cannot be verified
        // without extra ASN.1 support; they must never panic.
        assert!(public_key_blob_from_private_key("not a key").is_none());
        let encrypted_pem = "-----BEGIN RSA PRIVATE KEY-----\nProc-Type: 4,ENCRYPTED\n\nAAAA\n-----END RSA PRIVATE KEY-----";
        assert!(public_key_blob_from_private_key(encrypted_pem).is_none());
        let pkcs8 = "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkqhkiG9w0BAQEFAASCAT8wggE7AgEAAkEA\n-----END PRIVATE KEY-----";
        let _ = public_key_blob_from_private_key(pkcs8);
    }

    /// Integration check against real `ssh-keygen` output: the public key
    /// extracted from every private key must equal the `.pub` file content.
    #[test]
    fn test_real_openssh_keys_match() {
        let dir = std::env::var("EASY42_TEST_KEY_DIR").unwrap_or_default();
        if dir.is_empty() {
            return;
        }
        let dir = PathBuf::from(dir);
        for (priv_name, pub_name) in [
            ("id_ed25519", "id_ed25519.pub"),
            ("id_rsa", "id_rsa.pub"),
            ("id_ecdsa", "id_ecdsa.pub"),
            ("id_enc", "id_enc.pub"),
        ] {
            let priv_content = match fs::read_to_string(dir.join(priv_name)) {
                Ok(c) => c,
                Err(_) => continue,
            };
            let pub_content = fs::read_to_string(dir.join(pub_name)).unwrap();
            let expected = ParsedKey::parse(&pub_content).unwrap();
            match public_key_base64_from_private_key(&priv_content) {
                Some(got) => assert_eq!(got, expected.key, "mismatch for {}", priv_name),
                None => eprintln!("no blob derivable from {} (encrypted?)", priv_name),
            }
        }
    }

    #[test]
    fn test_u256_arithmetic() {
        let p = U256::from_be_hex("ffffffff00000001000000000000000000000000ffffffffffffffffffffffff").unwrap();
        let a = U256::from_be_bytes(&[0x10]).unwrap();
        let b = U256::from_be_hex("00000000000000000000000000000000000000000000000000000000ffffffff").unwrap();
        let sum = a.add_mod(&b, &p);
        assert_eq!(sum, U256::from_be_hex("000000000000000000000000000000000000000000000000000000010000000f").unwrap());
        let one = U256::one();
        assert_eq!(one.inv_mod(&p), one);
        assert_eq!(a.sub(&b, &p), a.wrapping_sub(&b).add(&p));
        let two = U256([2, 0, 0, 0]);
        assert_eq!(two.mul_mod(&a, &p), U256::from_be_bytes(&[0x20]).unwrap());
        // Order of the P-256 base point.
        let gx = U256::from_be_hex("6b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296").unwrap();
        let gy = U256::from_be_hex("4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5").unwrap();
        let bconst = U256::from_be_hex("5ac635d8aa3a93e7b3ebbd55769886bc651d06b0cc53b0f63bce3c3e27d2604b").unwrap();
        let two_g = U256::scalar_mul(&(gx, gy), &two, &p, &bconst).unwrap();
        let g_plus_g = point_add(&(gx, gy), &(gx, gy), &p, &bconst).unwrap();
        assert_eq!(two_g, g_plus_g);
    }
}
